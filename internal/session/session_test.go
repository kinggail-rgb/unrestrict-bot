package session

import (
	"context"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func open(t *testing.T) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "s.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreRoundTrip(t *testing.T) {
	s, err := NewStore(open(t), "user")
	if err != nil {
		t.Fatal(err)
	}
	if s.Has() {
		t.Fatal("expected empty store")
	}

	want := []byte(`{"Version":1}`)
	if err := s.StoreSession(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if !s.Has() {
		t.Fatal("Has() false after store")
	}
	got, err := s.LoadSession(context.Background())
	if err != nil || string(got) != string(want) {
		t.Fatalf("LoadSession = %q, %v", got, err)
	}
}

func TestSeed(t *testing.T) {
	db := open(t)
	s, _ := NewStore(db, "user")

	raw := []byte(`{"Version":1,"Data":{}}`)
	if err := s.Seed(context.Background(), Encode(raw)); err != nil {
		t.Fatal(err)
	}
	got, _ := s.LoadSession(context.Background())
	if string(got) != string(raw) {
		t.Fatalf("seeded %q, want %q", got, raw)
	}

	// Seeding again is a no-op once a session exists.
	if err := s.Seed(context.Background(), Encode([]byte(`{"Version":1,"other":true}`))); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadSession(context.Background())
	if string(got) != string(raw) {
		t.Fatalf("seed overwrote existing session: %q", got)
	}
}

func TestSeedEmptyIsNoop(t *testing.T) {
	s, _ := NewStore(open(t), "user")
	if err := s.Seed(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if s.Has() {
		t.Fatal("empty seed should not create a session")
	}
}
