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
