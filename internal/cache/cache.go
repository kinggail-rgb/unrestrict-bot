// Package cache is a bbolt-backed store for result dedup and forward progress.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketMedia   = []byte("media")
	bucketForward = []byte("forward")
)

// Entry points at a message this service already posted for a source message.
type Entry struct {
	DestChatID int64     `json:"dest_chat_id,omitempty"`
	DestMsgID  int       `json:"dest_msg_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Cache is a bbolt-backed store.
type Cache struct {
	db *bolt.DB
}

// Open opens (creating if needed) the cache database at path.
func Open(path string) (*Cache, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open cache %s: %w", path, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketMedia, bucketForward} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

// DB exposes the underlying database so the peer store can share the file.
func (c *Cache) DB() *bolt.DB { return c.db }

// Close closes the underlying database.
func (c *Cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func mediaKey(srcChatID int64, srcMsgID int) []byte {
	return []byte(fmt.Sprintf("%d:%d", srcChatID, srcMsgID))
}

// GetMedia returns the cached entry for a source message, if any.
func (c *Cache) GetMedia(srcChatID int64, srcMsgID int) (Entry, bool) {
	var e Entry
	found := false
	_ = c.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketMedia).Get(mediaKey(srcChatID, srcMsgID))
		if v == nil {
			return nil
		}
		if err := json.Unmarshal(v, &e); err != nil {
			return nil
		}
		found = true
		return nil
	})
	return e, found
}

// PutMedia stores the result pointer for a source message.
func (c *Cache) PutMedia(srcChatID int64, srcMsgID int, e Entry) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	v, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMedia).Put(mediaKey(srcChatID, srcMsgID), v)
	})
}

// DeleteMedia removes a cache entry.
func (c *Cache) DeleteMedia(srcChatID int64, srcMsgID int) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMedia).Delete(mediaKey(srcChatID, srcMsgID))
	})
}

// ForwardProgress returns the highest already-forwarded source message id for
// the given FROM:TO pair.
func (c *Cache) ForwardProgress(key string) int {
	var last int
	_ = c.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketForward).Get([]byte(key))
		if len(v) == 0 {
			return nil
		}
		_ = json.Unmarshal(v, &last)
		return nil
	})
	return last
}

// SetForwardProgress records progress for the given FROM:TO pair.
func (c *Cache) SetForwardProgress(key string, lastID int) error {
	v, err := json.Marshal(lastID)
	if err != nil {
		return err
	}
	return c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketForward).Put([]byte(key), v)
	})
}
