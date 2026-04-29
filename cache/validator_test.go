package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestGetIfValid_TrueKeepsEntry(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "fresh", []byte("data"))

	var calls int
	got, ok := c.GetIfValid(ctx, "fresh", func(_ []byte) bool {
		calls++
		return true
	})
	if !ok {
		t.Fatal("validator returned true but cache reported miss")
	}
	if string(got) != "data" {
		t.Errorf("got = %q, want %q", got, "data")
	}
	if calls != 1 {
		t.Errorf("validator called %d times, want 1", calls)
	}
}

func TestGetIfValid_FalseEvictsEntryAndReportsMiss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "stale", []byte("old"))

	_, ok := c.GetIfValid(ctx, "stale", func(_ []byte) bool { return false })
	if ok {
		t.Fatal("validator returned false but cache reported hit")
	}

	// Plain Get must also miss now (entry evicted).
	if _, ok := c.Get(ctx, "stale"); ok {
		t.Error("entry not evicted after validator returned false")
	}
}

func TestGetIfValid_NilValidator_BehavesLikeGet(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k", []byte("v"))

	got, ok := c.GetIfValid(ctx, "k", nil)
	if !ok {
		t.Fatal("nil validator: cache reported miss")
	}
	if string(got) != "v" {
		t.Errorf("got = %q, want %q", got, "v")
	}

	// Plain Get on a different missing key — both should miss equivalently.
	_, ok1 := c.Get(ctx, "missing")
	_, ok2 := c.GetIfValid(ctx, "missing", nil)
	if ok1 != ok2 {
		t.Errorf("Get and GetIfValid(nil) disagree on miss: %v vs %v", ok1, ok2)
	}
}

func TestGetIfValid_NotCalledOnL1Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	var called atomic.Bool
	_, ok := c.GetIfValid(context.Background(), "absent", func(_ []byte) bool {
		called.Store(true)
		return true
	})
	if ok {
		t.Error("got hit on missing key")
	}
	if called.Load() {
		t.Error("validator called on L1 miss; must only run on L1 hit")
	}
}

// fakeL2 is a minimal in-memory L2 used to verify L1-stale fall-through.
type fakeL2 struct {
	mu    sync.Mutex
	items map[string][]byte
}

func newFakeL2() *fakeL2 { return &fakeL2{items: map[string][]byte{}} }

func (f *fakeL2) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.items[key]; ok {
		return v, nil
	}
	return nil, cache.ErrCacheMiss
}

func (f *fakeL2) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[key] = data
	return nil
}

func (f *fakeL2) Del(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, key)
	return nil
}

func (f *fakeL2) Close() error { return nil }

func TestGetIfValid_L1StaleEvicts_FallsThroughToL2(t *testing.T) {
	l2 := newFakeL2()
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
		L2:         l2,
	})
	defer c.Close()

	ctx := context.Background()
	// Set populates L1 AND write-through to L2 — both end up with the same
	// stale payload at this point.
	c.Set(ctx, "k", []byte("from-l1-stale"))
	// Now overwrite L2 directly with the fresh value, behind cache's back. This
	// simulates a producer updating the source-of-truth while L1 still holds
	// the old value (the exact failure mode Validator targets).
	if err := l2.Set(ctx, "k", []byte("from-l2"), time.Minute); err != nil {
		t.Fatalf("l2 set: %v", err)
	}

	// Validator declares L1 entry stale → should fall through to L2.
	got, ok := c.GetIfValid(ctx, "k", func(data []byte) bool {
		return !equalBytes(data, []byte("from-l1-stale"))
	})
	if !ok {
		t.Fatal("expected L2 fallback hit, got miss")
	}
	if string(got) != "from-l2" {
		t.Errorf("got %q, want from-l2 (L1 should have been evicted, L2 served)", got)
	}
}

func TestGetIfValid_NilValidator_FallsThroughToL2(t *testing.T) {
	l2 := newFakeL2()
	if err := l2.Set(context.Background(), "only-in-l2", []byte("v"), time.Minute); err != nil {
		t.Fatalf("l2 set: %v", err)
	}
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute, L2: l2})
	defer c.Close()

	got, ok := c.GetIfValid(context.Background(), "only-in-l2", nil)
	if !ok {
		t.Fatal("nil validator must fall through to L2")
	}
	if string(got) != "v" {
		t.Errorf("got %q, want %q", got, "v")
	}
}

func TestGetIfValid_ConcurrentSafe(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k", []byte("v"))

	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Half of goroutines validate true, half validate false (forcing
				// evict + L2 miss path). Both must be race-free.
				keepFresh := (seed+i)%2 == 0
				_, _ = c.GetIfValid(ctx, "k", func(_ []byte) bool { return keepFresh })
				if !keepFresh {
					// Repopulate so the next iteration has something to validate.
					c.Set(ctx, "k", []byte("v"))
				}
			}
		}(g)
	}
	wg.Wait()
	// If we got here without -race firing, concurrency is safe.
}

func TestGetIfValid_StaleCounted_AsMiss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k", []byte("v"))

	beforeMisses := c.Stats().L1Misses

	if _, ok := c.GetIfValid(ctx, "k", func(_ []byte) bool { return false }); ok {
		t.Fatal("stale entry returned ok=true")
	}

	// L1 miss counter should reflect the stale-eviction follow-through (no L2,
	// so the eviction collapses into a miss).
	afterMisses := c.Stats().L1Misses
	if afterMisses <= beforeMisses {
		t.Errorf("L1Misses did not increase after stale evict: before=%d after=%d", beforeMisses, afterMisses)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
