package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestCache_SetGet(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "key1", []byte("value1"))

	got, ok := c.Get(ctx, "key1")
	if !ok {
		t.Fatal("Get returned not ok")
	}
	if string(got) != "value1" {
		t.Errorf("Get = %q, want %q", got, "value1")
	}
}

func TestCache_Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	_, ok := c.Get(context.Background(), "missing")
	if ok {
		t.Error("Get returned ok for missing key")
	}
}

func TestCache_Expiry(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      50 * time.Millisecond,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "expire-me", []byte("data"))

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx, "expire-me")
	if ok {
		t.Error("Get returned ok for expired key")
	}
}

func TestCache_Eviction(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 3,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	for i := range 4 {
		c.Set(ctx, cache.Key("item", string(rune('a'+i))), []byte("data"))
	}

	stats := c.Stats()
	if stats.L1Size > 3 {
		t.Errorf("L1Size = %d, want <= 3", stats.L1Size)
	}
}

func TestCache_Delete(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "del-me", []byte("data"))
	c.Delete(ctx, "del-me")

	_, ok := c.Get(ctx, "del-me")
	if ok {
		t.Error("Get returned ok after Delete")
	}
}

func TestKey(t *testing.T) {
	k := cache.Key("search", "golang", "patterns")
	if k == "" {
		t.Error("Key returned empty string")
	}
	k2 := cache.Key("search", "golang", "patterns")
	if k != k2 {
		t.Errorf("Key not deterministic: %q != %q", k, k2)
	}
	k3 := cache.Key("search", "rust", "patterns")
	if k == k3 {
		t.Error("different inputs produced same key")
	}
}

func TestStats(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "a", []byte("1"))
	c.Get(ctx, "a") // hit
	c.Get(ctx, "b") // miss

	stats := c.Stats()
	if stats.L1Hits != 1 {
		t.Errorf("L1Hits = %d, want 1", stats.L1Hits)
	}
	if stats.L1Misses != 1 {
		t.Errorf("L1Misses = %d, want 1", stats.L1Misses)
	}
	if stats.L1Size != 1 {
		t.Errorf("L1Size = %d, want 1", stats.L1Size)
	}
}

func TestCache_NoRedis(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "local", []byte("only"))

	got, ok := c.Get(ctx, "local")
	if !ok || string(got) != "only" {
		t.Errorf("L1-only mode failed: ok=%v, got=%q", ok, got)
	}
}

func TestCache_S3FIFO_Promotion(t *testing.T) {
	// Cache with capacity 3: small=1, main=2.
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	// Insert 3 entries.
	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3"))

	// Access "a" to boost its frequency (will be promoted on eviction).
	c.Get(ctx, "a")

	// Insert "d" — triggers eviction.
	c.Set(ctx, "d", []byte("4"))

	// "a" should survive (promoted to main), some other entry evicted.
	if _, ok := c.Get(ctx, "a"); !ok {
		t.Error("accessed entry 'a' should have been promoted and survived eviction")
	}
}

func TestCache_S3FIFO_OneHitWonder(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "hot", []byte("1"))
	c.Set(ctx, "cold1", []byte("2"))
	c.Set(ctx, "cold2", []byte("3"))

	// Access "hot" multiple times.
	c.Get(ctx, "hot")
	c.Get(ctx, "hot")

	// Fill cache to trigger eviction of cold entries.
	c.Set(ctx, "new1", []byte("4"))
	c.Set(ctx, "new2", []byte("5"))

	// "hot" should still be in cache.
	if _, ok := c.Get(ctx, "hot"); !ok {
		t.Error("frequently accessed 'hot' should survive eviction")
	}
}

func TestCache_S3FIFO_Ghost(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 2, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	// Insert and evict "a" (goes to ghost).
	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3")) // "a" evicted to ghost

	// Verify "a" is gone.
	if _, ok := c.Get(ctx, "a"); ok {
		t.Error("evicted 'a' should not be in cache")
	}

	// Re-insert "a" (should get ghost frequency boost).
	c.Set(ctx, "a", []byte("1-back"))

	got, ok := c.Get(ctx, "a")
	if !ok {
		t.Fatal("re-inserted 'a' should be in cache")
	}
	if string(got) != "1-back" {
		t.Errorf("got %q, want %q", got, "1-back")
	}
}

func TestCache_S3FIFO_FreqCap(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 5, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "x", []byte("data"))

	// Access many times — should not panic or behave oddly.
	for range 100 {
		if _, ok := c.Get(ctx, "x"); !ok {
			t.Fatal("entry should still exist")
		}
	}

	// Fill cache and trigger evictions — "x" should survive due to high freq.
	for i := range 10 {
		c.Set(ctx, cache.Key("fill", string(rune('a'+i))), []byte("filler"))
	}

	if _, ok := c.Get(ctx, "x"); !ok {
		t.Error("highly accessed 'x' should survive evictions")
	}
}
