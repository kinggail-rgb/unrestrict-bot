package tgx

import (
	"context"
	"encoding/binary"
	"encoding/json"

	"github.com/gotd/td/telegram/peers"
	bolt "go.etcd.io/bbolt"
)

// boltPeerStorage is a persistent implementation of peers.Storage so that access
// hashes for private channels survive restarts.
type boltPeerStorage struct {
	db *bolt.DB
}

var (
	bktPeers  = []byte("peers")
	bktPhones = []byte("peer_phones")
	bktMeta   = []byte("peer_meta")
)

func newBoltPeerStorage(db *bolt.DB) (*boltPeerStorage, error) {
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bktPeers, bktPhones, bktMeta} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &boltPeerStorage{db: db}, nil
}

func (s *boltPeerStorage) Save(ctx context.Context, key peers.Key, value peers.Value) error {
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktPeers).Put([]byte(key.Prefix+itoa(key.ID)), v)
	})
}

func (s *boltPeerStorage) Find(ctx context.Context, key peers.Key) (peers.Value, bool, error) {
	var value peers.Value
	found := false
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bktPeers).Get([]byte(key.Prefix + itoa(key.ID)))
		if raw == nil {
			return nil
		}
		found = true
		return json.Unmarshal(raw, &value)
	})
	return value, found, err
}

func (s *boltPeerStorage) SavePhone(ctx context.Context, phone string, key peers.Key) error {
	v, err := json.Marshal(key)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktPhones).Put([]byte(phone), v)
	})
}

func (s *boltPeerStorage) FindPhone(ctx context.Context, phone string) (key peers.Key, value peers.Value, found bool, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bktPhones).Get([]byte(phone))
		if raw == nil {
			return nil
		}
		if err := json.Unmarshal(raw, &key); err != nil {
			return err
		}
		v := tx.Bucket(bktPeers).Get([]byte(key.Prefix + itoa(key.ID)))
		if v != nil {
			found = true
			return json.Unmarshal(v, &value)
		}
		return nil
	})
	return key, value, found, err
}

func (s *boltPeerStorage) GetContactsHash(ctx context.Context) (int64, error) {
	var h int64
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bktMeta).Get([]byte("contacts_hash"))
		if len(raw) == 8 {
			h = int64(binary.BigEndian.Uint64(raw))
		}
		return nil
	})
	return h, err
}

func (s *boltPeerStorage) SaveContactsHash(ctx context.Context, hash int64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(hash))
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktMeta).Put([]byte("contacts_hash"), buf[:])
	})
}

func itoa(v int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	return string(buf[:])
}
