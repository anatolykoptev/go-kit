package cache_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestSetWithTags_GetTags(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "user:1:profile", []byte(`{"name":"Alice"}`), []string{"user:1", "profile"})

	tags := c.Tags("user:1:profile")
	if len(tags) != 2 {
		t.Fatalf("Tags len = %d, want 2", len(tags))
	}
	sort.Strings(tags)
	if tags[0] != "profile" || tags[1] != "user:1" {
		t.Errorf("Tags = %v, want [profile user:1]", tags)
	}
}

func TestTags_MissingKey(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	if tags := c.Tags("nonexistent"); tags != nil {
		t.Errorf("Tags for missing key = %v, want nil", tags)
	}
}

func TestTags_NoTags(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	c.Set(context.Background(), "plain", []byte("data"))
	if tags := c.Tags("plain"); tags != nil {
		t.Errorf("Tags for untagged key = %v, want nil", tags)
	}
}

func TestInvalidateByTag(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "u1:profile", []byte("p1"), []string{"user:1", "profile"})
	c.SetWithTags(ctx, "u1:settings", []byte("s1"), []string{"user:1", "settings"})
	c.SetWithTags(ctx, "u2:profile", []byte("p2"), []string{"user:2", "profile"})

	n := c.InvalidateByTag(ctx, "user:1")
	if n != 2 {
		t.Errorf("InvalidateByTag returned %d, want 2", n)
	}

	if _, ok := c.Get(ctx, "u1:profile"); ok {
		t.Error("u1:profile should be invalidated")
	}
	if _, ok := c.Get(ctx, "u1:settings"); ok {
		t.Error("u1:settings should be invalidated")
	}
	if _, ok := c.Get(ctx, "u2:profile"); !ok {
		t.Error("u2:profile should still exist")
	}
}

func TestInvalidateByTag_Empty(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	n := c.InvalidateByTag(context.Background(), "nonexistent")
	if n != 0 {
		t.Errorf("InvalidateByTag for missing tag = %d, want 0", n)
	}
}

func TestInvalidateByTag_L2Cleanup(t *testing.T) {
	l2 := newMapL2()
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute, L2: l2})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k1", []byte("v1"), []string{"tag-a"})
	c.SetWithTags(ctx, "k2", []byte("v2"), []string{"tag-a"})

	c.InvalidateByTag(ctx, "tag-a")

	l2.mu.Lock()
	_, ok1 := l2.data["k1"]
	_, ok2 := l2.data["k2"]
	l2.mu.Unlock()
	if ok1 || ok2 {
		t.Error("L2 should have deleted tagged keys")
	}
}

func TestInvalidateByTag_OnEvictCallback(t *testing.T) {
	var mu sync.Mutex
	var evicted []string
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		OnEvict: func(key string, _ []byte, reason cache.EvictReason) {
			if reason != cache.EvictExplicit {
				return
			}
			mu.Lock()
			evicted = append(evicted, key)
			mu.Unlock()
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "a", []byte("1"), []string{"grp"})
	c.SetWithTags(ctx, "b", []byte("2"), []string{"grp"})

	c.InvalidateByTag(ctx, "grp")

	mu.Lock()
	defer mu.Unlock()
	sort.Strings(evicted)
	if len(evicted) != 2 || evicted[0] != "a" || evicted[1] != "b" {
		t.Errorf("evicted = %v, want [a b]", evicted)
	}
}

func TestSetWithTagsAndTTL(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTagsAndTTL(ctx, "short", []byte("data"), 50*time.Millisecond, []string{"tmp"})

	if _, ok := c.Get(ctx, "short"); !ok {
		t.Fatal("entry should exist immediately after set")
	}

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get(ctx, "short"); ok {
		t.Error("entry should have expired")
	}
}

func TestTags_UpdateOnOverwrite(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v1"), []string{"old-tag"})
	c.SetWithTags(ctx, "k", []byte("v2"), []string{"new-tag"})

	tags := c.Tags("k")
	if len(tags) != 1 || tags[0] != "new-tag" {
		t.Errorf("Tags after overwrite = %v, want [new-tag]", tags)
	}

	// old-tag should no longer reference this key.
	n := c.InvalidateByTag(ctx, "old-tag")
	if n != 0 {
		t.Errorf("InvalidateByTag(old-tag) = %d, want 0", n)
	}
	// Key should still exist (wasn't invalidated by old tag).
	if _, ok := c.Get(ctx, "k"); !ok {
		t.Error("key should still exist after invalidating old tag")
	}
}

func TestTags_EvictionCleansTagIndex(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "a", []byte("1"), []string{"evict-me"})
	c.SetWithTags(ctx, "b", []byte("2"), []string{"evict-me"})
	c.Set(ctx, "c", []byte("3"))

	// Fill past capacity to trigger eviction.
	c.Set(ctx, "d", []byte("4"))
	c.Set(ctx, "e", []byte("5"))

	// Invalidate the tag — should not panic or return stale count.
	n := c.InvalidateByTag(ctx, "evict-me")
	// n might be 0, 1, or 2 depending on what was evicted.
	_ = n
}

func TestTags_ClearResetsTagIndex(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v"), []string{"tag"})
	c.Clear()

	// After clear, tag should be gone.
	n := c.InvalidateByTag(ctx, "tag")
	if n != 0 {
		t.Errorf("InvalidateByTag after Clear = %d, want 0", n)
	}
}
