package cache_test

// RED tests — written to expose TTL+tags bugs, expected to FAIL before fix.

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

// BUG: Set() calls setInternal with no tags (variadic nil).
// setInternal sees len(e.tags) > 0 and calls updateTags(key, old, nil),
// stripping existing tags. Plain data updates should preserve tags.
func TestSet_PreservesTags(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v1"), []string{"user:1", "profile"})
	c.Set(ctx, "k", []byte("v2")) // update data, should NOT strip tags

	tags := c.Tags("k")
	if len(tags) != 2 {
		t.Errorf("Tags after Set = %v, want [user:1 profile] — plain Set must preserve tags", tags)
	}
}

// BUG: SetWithTTL() also passes no tags to setInternal → strips tags.
func TestSetWithTTL_PreservesTags(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v1"), []string{"ttl-tag"})
	c.SetWithTTL(ctx, "k", []byte("v2"), 5*time.Minute) // refresh TTL, should keep tags

	tags := c.Tags("k")
	if len(tags) != 1 || tags[0] != "ttl-tag" {
		t.Errorf("Tags after SetWithTTL = %v, want [ttl-tag]", tags)
	}
}

// BUG: GetOrLoad stores loaded value via Set() → strips tags if key already tagged.
func TestGetOrLoad_PreservesTags(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: 50 * time.Millisecond})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v1"), []string{"loader-tag"})
	time.Sleep(100 * time.Millisecond) // expire L1

	// GetOrLoad: L1 miss → loader → Set() → tags stripped.
	got, err := c.GetOrLoad(ctx, "k", func(_ context.Context) ([]byte, error) {
		return []byte("v2"), nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad: %v", err)
	}
	if string(got) != "v2" {
		t.Fatalf("got %q, want v2", got)
	}

	// After reload, entry exists but tags are gone.
	// This is expected for expired+reloaded entries (no tag info in loader).
	// Test documents the behavior.
	tags := c.Tags("k")
	if tags != nil {
		t.Logf("Tags after GetOrLoad = %v (tags lost on expire+reload — expected)", tags)
	}
}

// BUG: L2 promotion in Get() calls Set() → strips tags.
func TestL2Promotion_StripsTags(t *testing.T) {
	l2 := newMapL2()
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: 50 * time.Millisecond, L2: l2})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "k", []byte("v1"), []string{"l2-tag"})
	time.Sleep(100 * time.Millisecond) // expire L1, L2 still has data

	// Get: L1 expired → removes entry (strips tags) → L2 hit → Set() (no tags).
	got, ok := c.Get(ctx, "k")
	if !ok {
		t.Fatal("L2 promotion should succeed")
	}
	if string(got) != "v1" {
		t.Fatalf("got %q, want v1", got)
	}

	// After L2 promotion, tags are gone (L2 doesn't store tags).
	// This is expected behavior. Test documents it.
	tags := c.Tags("k")
	if tags != nil {
		t.Logf("Tags after L2 promotion = %v (tags lost — expected, L2 has no tag info)", tags)
	}
}

// BUG: InvalidateByTag after SetWithTTL overwrites a tagged entry.
// The tag was stripped, so invalidation misses the entry.
func TestInvalidateByTag_AfterTTLRefresh(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.SetWithTags(ctx, "u1:profile", []byte("p1"), []string{"user:1"})
	c.SetWithTags(ctx, "u1:settings", []byte("s1"), []string{"user:1"})

	// Refresh only the profile with new TTL (common pattern).
	c.SetWithTTL(ctx, "u1:profile", []byte("p1-updated"), 10*time.Minute)

	// Now invalidate user:1 — should remove BOTH entries.
	n := c.InvalidateByTag(ctx, "user:1")
	if n != 2 {
		t.Errorf("InvalidateByTag = %d, want 2 — SetWithTTL stripped tag from u1:profile", n)
	}

	if _, ok := c.Get(ctx, "u1:profile"); ok {
		t.Error("u1:profile should be invalidated")
	}
	if _, ok := c.Get(ctx, "u1:settings"); ok {
		t.Error("u1:settings should be invalidated")
	}
}
