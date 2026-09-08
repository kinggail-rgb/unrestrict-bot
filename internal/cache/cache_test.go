package cache

import (
	"path/filepath"
	"testing"
)

func TestMediaRoundTrip(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, ok := c.GetMedia(1, 2); ok {
		t.Fatal("expected miss on empty cache")
	}

	if err := c.PutMedia(1, 2, Entry{DestChatID: 99, DestMsgID: 7}); err != nil {
		t.Fatal(err)
	}
	got, ok := c.GetMedia(1, 2)
	if !ok || got.DestChatID != 99 || got.DestMsgID != 7 {
		t.Fatalf("GetMedia = %+v, ok=%v", got, ok)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}

	if err := c.DeleteMedia(1, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.GetMedia(1, 2); ok {
		t.Fatal("expected miss after delete")
	}
}

func TestForwardProgress(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if got := c.ForwardProgress("a:b"); got != 0 {
		t.Fatalf("empty progress = %d", got)
	}
	if err := c.SetForwardProgress("a:b", 512); err != nil {
		t.Fatal(err)
	}
	if got := c.ForwardProgress("a:b"); got != 512 {
		t.Fatalf("progress = %d, want 512", got)
	}
}
