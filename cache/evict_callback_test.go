package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestOnEvict_Capacity(t *testing.T) {
	var mu sync.Mutex
	var got []cache.EvictReason

	c := cache.New(cache.Config{
		L1MaxItems: 3,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			mu.Lock()
			got = append(got, reason)
			mu.Unlock()
		},
	})
	defer c.Close()
	ctx := context.Background()

	for i := range 4 {
		c.Set(ctx, cache.Key("cap", string(rune('a'+i))), []byte("data"))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("expected at least one EvictCapacity callback")
	}
	for i, r := range got {
		if r != cache.EvictCapacity {
			t.Errorf("callback[%d] reason = %d, want EvictCapacity (%d)", i, r, cache.EvictCapacity)
		}
	}
}

func TestOnEvict_Expired(t *testing.T) {
	var mu sync.Mutex
	var reasons []cache.EvictReason

	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      50 * time.Millisecond,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			mu.Lock()
			reasons = append(reasons, reason)
			mu.Unlock()
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "expire-me", []byte("data"))
	time.Sleep(100 * time.Millisecond)

	// Get triggers expired removal.
	_, _ = c.Get(ctx, "expire-me")

	mu.Lock()
	defer mu.Unlock()
	if len(reasons) == 0 {
		t.Fatal("expected EvictExpired callback")
	}
	if reasons[0] != cache.EvictExpired {
		t.Errorf("reason = %d, want EvictExpired (%d)", reasons[0], cache.EvictExpired)
	}
}

func TestOnEvict_Explicit(t *testing.T) {
	var gotKey string
	var gotData []byte
	var gotReason cache.EvictReason

	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			gotKey = key
			gotData = data
			gotReason = reason
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "del-me", []byte("payload"))
	c.Delete(ctx, "del-me")

	if gotKey != "del-me" {
		t.Errorf("key = %q, want %q", gotKey, "del-me")
	}
	if string(gotData) != "payload" {
		t.Errorf("data = %q, want %q", gotData, "payload")
	}
	if gotReason != cache.EvictExplicit {
		t.Errorf("reason = %d, want EvictExplicit (%d)", gotReason, cache.EvictExplicit)
	}
}

func TestOnEvict_NilCallback(t *testing.T) {
	// No OnEvict set — must not panic on eviction or delete.
	c := cache.New(cache.Config{
		L1MaxItems: 2,
		L1TTL:      time.Minute,
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3")) // triggers eviction
	c.Delete(ctx, "c")
}

func TestOnEvict_NoDeadlock(t *testing.T) {
	// Callback accesses cache Stats — must not deadlock.
	var statsCount atomic.Int64
	var cp atomic.Pointer[cache.Cache]
	c := cache.New(cache.Config{
		L1MaxItems: 2,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			// This would deadlock if called under the cache lock.
			if cc := cp.Load(); cc != nil {
				s := cc.Stats()
				_ = s.L1Size
			}
			statsCount.Add(1)
		},
	})
	cp.Store(c)
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3")) // eviction — callback calls Stats()

	if statsCount.Load() == 0 {
		t.Error("callback should have fired and accessed Stats without deadlock")
	}
}

func TestOnEvict_ClearSkips(t *testing.T) {
	var called atomic.Int32
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			called.Add(1)
		},
	})
	defer c.Close()
	ctx := context.Background()

	for i := range 5 {
		c.Set(ctx, cache.Key("clear", string(rune('a'+i))), []byte("v"))
	}

	// Reset counter (Set itself shouldn't trigger callbacks for <capacity).
	called.Store(0)

	c.Clear()

	if n := called.Load(); n != 0 {
		t.Errorf("Clear fired %d callbacks, want 0", n)
	}
}
