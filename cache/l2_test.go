package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestRedisL2_NilReceiver_Get(t *testing.T) {
	var r *cache.RedisL2
	_, err := r.Get(context.Background(), "key")
	if !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("nil receiver Get: got %v, want ErrCacheMiss", err)
	}
}

func TestRedisL2_NilReceiver_Set(t *testing.T) {
	var r *cache.RedisL2
	err := r.Set(context.Background(), "key", []byte("v"), 0)
	if !errors.Is(err, cache.ErrL2Unavailable) {
		t.Errorf("nil receiver Set: got %v, want ErrL2Unavailable", err)
	}
}

func TestRedisL2_NilReceiver_Del(t *testing.T) {
	var r *cache.RedisL2
	err := r.Del(context.Background(), "key")
	if !errors.Is(err, cache.ErrL2Unavailable) {
		t.Errorf("nil receiver Del: got %v, want ErrL2Unavailable", err)
	}
}

func TestRedisL2_NilReceiver_Close(t *testing.T) {
	var r *cache.RedisL2
	if err := r.Close(); err != nil {
		t.Errorf("nil receiver Close: got %v, want nil", err)
	}
}

func TestErrCacheMiss_Sentinel(t *testing.T) {
	if !errors.Is(cache.ErrCacheMiss, cache.ErrCacheMiss) {
		t.Error("ErrCacheMiss should match itself")
	}
	if errors.Is(cache.ErrCacheMiss, cache.ErrL2Unavailable) {
		t.Error("ErrCacheMiss should not match ErrL2Unavailable")
	}
}

func TestNew_NilRedisL2_NoSIGSEGV(t *testing.T) {
	// Simulate: valid Redis URL but unreachable Redis.
	// NewRedisL2 returns nil → must NOT cause typed nil in interface.
	c := cache.New(cache.Config{
		RedisURL:   "redis://192.0.2.1:6379", // RFC 5737 TEST-NET, unreachable
		L1MaxItems: 10,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()

	// This used to SIGSEGV because c.l2 was typed nil interface.
	c.Set(ctx, "key", []byte("value"))
	data, ok := c.Get(ctx, "key")
	if !ok || string(data) != "value" {
		t.Errorf("L1 should work when Redis unreachable: ok=%v, data=%q", ok, data)
	}

	// Verify L2 stats are zero (no L2 active).
	stats := c.Stats()
	if stats.L2Hits != 0 || stats.L2Misses != 0 {
		t.Errorf("L2 should be disabled: hits=%d, misses=%d", stats.L2Hits, stats.L2Misses)
	}
}
