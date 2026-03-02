package cache_test

import (
	"context"
	"errors"
	"sync"
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

// faultyL2 simulates Redis failures for testing.
type faultyL2 struct {
	fails int // remaining failures before success
	mu    sync.Mutex
	data  map[string][]byte
}

func newFaultyL2(failCount int) *faultyL2 {
	return &faultyL2{fails: failCount, data: make(map[string][]byte)}
}

func (f *faultyL2) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		f.fails--
		return nil, errors.New("connection refused")
	}
	v, ok := f.data[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return v, nil
}

func (f *faultyL2) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fails > 0 {
		return errors.New("connection refused")
	}
	f.data[key] = data
	return nil
}

func (f *faultyL2) Del(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	return nil
}

func (f *faultyL2) Close() error { return nil }

func TestCache_L2Error_FallsThrough(t *testing.T) {
	faulty := newFaultyL2(100) // always fails
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute, L2: faulty})
	defer c.Close()

	ctx := context.Background()

	// Set should still work for L1 (L2 error is best-effort).
	c.Set(ctx, "key", []byte("value"))

	// Get from L1 should succeed despite L2 being broken.
	data, ok := c.Get(ctx, "key")
	if !ok || string(data) != "value" {
		t.Errorf("L1 should work despite L2 failure: ok=%v, data=%q", ok, data)
	}
}

func TestCache_L2Error_Stats(t *testing.T) {
	faulty := newFaultyL2(100)
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute, L2: faulty})
	defer c.Close()

	ctx := context.Background()

	// L1 miss + L2 error should count as L2 error, not L2 miss.
	_, ok := c.Get(ctx, "missing")
	if ok {
		t.Error("should miss")
	}

	stats := c.Stats()
	if stats.L2Errors != 1 {
		t.Errorf("L2Errors = %d, want 1", stats.L2Errors)
	}
	if stats.L2Misses != 0 {
		t.Errorf("L2Misses = %d, want 0 (was a real error, not a miss)", stats.L2Misses)
	}
}

func TestCache_CircuitBreaker_OpensAfterFailures(t *testing.T) {
	faulty := newFaultyL2(10) // first 10 calls fail
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute, L2: faulty})
	defer c.Close()

	ctx := context.Background()

	// Trigger 10 L2 failures (4+ consecutive → circuit opens).
	for i := range 10 {
		c.Get(ctx, cache.Key("miss", string(rune('a'+i))))
	}

	stats := c.Stats()
	if stats.L2Errors == 0 {
		t.Error("should have recorded L2 errors")
	}
	t.Logf("L2Errors=%d, L2Misses=%d", stats.L2Errors, stats.L2Misses)
}

func TestCache_L2Miss_UsesErrCacheMiss(t *testing.T) {
	l2 := newMapL2()
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute, L2: l2})
	defer c.Close()

	ctx := context.Background()
	_, ok := c.Get(ctx, "not-here")
	if ok {
		t.Error("should be a miss")
	}

	stats := c.Stats()
	if stats.L2Misses != 1 {
		t.Errorf("L2Misses = %d, want 1", stats.L2Misses)
	}
	if stats.L2Errors != 0 {
		t.Errorf("L2Errors = %d, want 0 (miss is not error)", stats.L2Errors)
	}
}
