package cache_test

// RED tests — written to expose bugs, expected to FAIL before fix.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

// BUG: Get() uses `if expData != nil` to decide whether to fire callback.
// If data is nil (negative caching / tombstone), callback is silently skipped.
func TestOnEvict_NilData_Get(t *testing.T) {
	var called atomic.Int32
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      50 * time.Millisecond,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			called.Add(1)
		},
	})
	defer c.Close()
	ctx := context.Background()

	// Store nil data (tombstone pattern — "we checked, nothing exists").
	c.Set(ctx, "tombstone", nil)
	time.Sleep(100 * time.Millisecond)

	// Get triggers expiry removal. Callback MUST fire even though data is nil.
	_, _ = c.Get(ctx, "tombstone")

	if n := called.Load(); n != 1 {
		t.Errorf("OnEvict called %d times, want 1 (nil data should still fire callback)", n)
	}
}

// BUG: Delete() uses `if delData != nil` to decide whether to fire callback.
// Same nil data problem.
func TestOnEvict_NilData_Delete(t *testing.T) {
	var gotKey string
	var callCount atomic.Int32
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			gotKey = key
			callCount.Add(1)
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "nil-value", nil)
	c.Delete(ctx, "nil-value")

	if n := callCount.Load(); n != 1 {
		t.Errorf("OnEvict called %d times, want 1 (nil data should still fire callback)", n)
	}
	if gotKey != "nil-value" {
		t.Errorf("key = %q, want %q", gotKey, "nil-value")
	}
}

// Verify that empty []byte{} (distinct from nil) also triggers callback.
func TestOnEvict_EmptyData_Delete(t *testing.T) {
	var gotData []byte
	var called bool
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			gotData = data
			called = true
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "empty", []byte{})
	c.Delete(ctx, "empty")

	if !called {
		t.Fatal("OnEvict not called for empty data")
	}
	if gotData == nil {
		t.Error("data should be empty slice, not nil")
	}
}

// Capacity eviction with nil data — should also work.
func TestOnEvict_NilData_Capacity(t *testing.T) {
	var called atomic.Int32
	c := cache.New(cache.Config{
		L1MaxItems: 2,
		L1TTL:      time.Minute,
		OnEvict: func(key string, data []byte, reason cache.EvictReason) {
			called.Add(1)
		},
	})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", nil) // nil data
	c.Set(ctx, "b", nil) // nil data
	c.Set(ctx, "c", []byte("triggers eviction"))

	if n := called.Load(); n == 0 {
		t.Error("capacity eviction should fire callback even when evicted data is nil")
	}
}
