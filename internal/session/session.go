// Package session persists the user-account session in the bbolt database.
// It also accepts a SESSION_STRING seed (base64 of a raw gotd session, or a
// Telethon StringSession) which is imported once when no session is stored yet.
package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	gotd "github.com/gotd/td/session"
	bolt "go.etcd.io/bbolt"
)

var bucket = []byte("session")

// Store is a bbolt-backed gotd session.Storage.
type Store struct {
	db  *bolt.DB
	key []byte
}

// NewStore opens the session store in db under the given key ("user", "bot").
func NewStore(db *bolt.DB, key string) (*Store, error) {
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	}); err != nil {
		return nil, err
	}
	if key == "" {
		key = "user"
	}
	return &Store{db: db, key: []byte(key)}, nil
}

func (s *Store) LoadSession(context.Context) ([]byte, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		if v := tx.Bucket(bucket).Get(s.key); v != nil {
			out = append([]byte(nil), v...)
		}
		return nil
	})
	return out, err
}

func (s *Store) StoreSession(_ context.Context, data []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).Put(s.key, data)
	})
}

// Has reports whether a session is already stored.
func (s *Store) Has() bool {
	b, _ := s.LoadSession(context.Background())
	return len(b) > 0
}

// Seed imports a SESSION_STRING when no session is stored yet. Empty is a no-op.
func (s *Store) Seed(ctx context.Context, sessionString string) error {
	if strings.TrimSpace(sessionString) == "" || s.Has() {
		return nil
	}
	raw, err := decode(sessionString)
	if err != nil {
		return err
	}
	return s.StoreSession(ctx, raw)
}

func decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)

	// Native: base64 of the raw gotd session JSON.
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if raw, err := enc.DecodeString(s); err == nil && json.Valid(raw) {
			return raw, nil
		}
	}

	// Telethon StringSession.
	data, err := gotd.TelethonSession(s)
	if err != nil {
		return nil, fmt.Errorf("SESSION_STRING is neither a native nor a Telethon session: %w", err)
	}
	mem := &gotd.StorageMemory{}
	if err := (&gotd.Loader{Storage: mem}).Save(context.Background(), data); err != nil {
		return nil, fmt.Errorf("convert telethon session: %w", err)
	}
	return mem.Bytes(nil)
}

// Encode renders raw gotd session bytes as a portable SESSION_STRING.
func Encode(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}
