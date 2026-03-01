package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func TestGetOrLoad_CacheHit(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "pre", []byte("cached"))

	var called bool
	got, err := c.GetOrLoad(ctx, "pre", func(_ context.Context) ([]byte, error) {
		called = true
		return []byte("loaded"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("loader should not be called on cache hit")
	}
	if string(got) != "cached" {
		t.Errorf("got %q, want %q", got, "cached")
	}
}

func TestGetOrLoad_CacheMiss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	got, err := c.GetOrLoad(ctx, "miss", func(_ context.Context) ([]byte, error) {
		return []byte("loaded"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "loaded" {
		t.Errorf("got %q, want %q", got, "loaded")
	}

	// Should now be in cache.
	cached, ok := c.Get(ctx, "miss")
	if !ok {
		t.Fatal("loaded value should be cached")
	}
	if string(cached) != "loaded" {
		t.Errorf("cached %q, want %q", cached, "loaded")
	}
}

func TestGetOrLoad_Singleflight(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	var calls atomic.Int32
	loader := func(_ context.Context) ([]byte, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return []byte("result"), nil
	}

	// Launch 10 concurrent loads for the same key.
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.GetOrLoad(ctx, "shared", loader)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if string(got) != "result" {
				t.Errorf("got %q, want %q", got, "result")
			}
		}()
	}
	wg.Wait()

	// Singleflight should deduplicate — only 1 loader call.
	if n := calls.Load(); n != 1 {
		t.Errorf("loader called %d times, want 1 (singleflight)", n)
	}
}

func TestJitter_Varies(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems:    100,
		L1TTL:         time.Second,
		JitterPercent: 0.5, // ±50% — extreme to make variation obvious
	})
	defer c.Close()
	ctx := context.Background()

	// Insert many items and verify they don't all expire at the same time.
	for i := range 20 {
		c.Set(ctx, cache.Key("jitter", string(rune('a'+i))), []byte("data"))
	}

	// Sleep for half TTL — with ±50% jitter, some should expire, some shouldn't.
	time.Sleep(600 * time.Millisecond)

	alive := 0
	for i := range 20 {
		if _, ok := c.Get(ctx, cache.Key("jitter", string(rune('a'+i)))); ok {
			alive++
		}
	}

	// With ±50% jitter on 1s TTL, after 600ms:
	// - Entries with TTL < 600ms (TTL range 500ms-1500ms) should have expired
	// - Not all should be alive, not all should be dead
	if alive == 0 || alive == 20 {
		t.Errorf("alive = %d, jitter should produce varying expiry", alive)
	}
}

func TestStats_Evictions(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	// Fill cache (3 items) then add 3 more — triggers 3 evictions.
	for i := range 6 {
		c.Set(ctx, cache.Key("ev", string(rune('a'+i))), []byte("data"))
	}

	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Error("Evictions should be > 0 after overfilling")
	}
}

func TestStats_HitRatio(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"))
	c.Get(ctx, "a") // hit
	c.Get(ctx, "a") // hit
	c.Get(ctx, "b") // miss

	stats := c.Stats()
	// 2 hits / 3 total = 0.666...
	if stats.HitRatio < 0.65 || stats.HitRatio > 0.68 {
		t.Errorf("HitRatio = %f, want ~0.667", stats.HitRatio)
	}
}

// mapL2 is a mock L2 for testing (no Redis needed).
type mapL2 struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapL2() *mapL2 {
	return &mapL2{data: make(map[string][]byte)}
}

func (m *mapL2) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (m *mapL2) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *mapL2) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mapL2) Close() error { return nil }

func TestL2_WriteThrough(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	c.WithL2(l2)

	ctx := context.Background()
	c.Set(ctx, "key1", []byte("value1"))

	// Verify L2 received the write.
	l2.mu.Lock()
	got, ok := l2.data["key1"]
	l2.mu.Unlock()
	if !ok {
		t.Fatal("L2 should have received write-through")
	}
	if string(got) != "value1" {
		t.Errorf("L2 got %q, want %q", got, "value1")
	}
}

func TestL2_ReadThrough(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	l2.data["pre-seeded"] = []byte("from-redis")
	c.WithL2(l2)

	ctx := context.Background()

	// L1 miss, L2 hit — should return data and promote to L1.
	got, ok := c.Get(ctx, "pre-seeded")
	if !ok {
		t.Fatal("should hit L2")
	}
	if string(got) != "from-redis" {
		t.Errorf("got %q, want %q", got, "from-redis")
	}

	// Now should be in L1 — verify by checking stats.
	stats := c.Stats()
	if stats.L2Hits != 1 {
		t.Errorf("L2Hits = %d, want 1", stats.L2Hits)
	}

	// Second Get should hit L1 (not L2 again).
	got2, ok2 := c.Get(ctx, "pre-seeded")
	if !ok2 || string(got2) != "from-redis" {
		t.Errorf("L1 promotion failed: ok=%v, got=%q", ok2, got2)
	}
	stats2 := c.Stats()
	if stats2.L1Hits != 1 {
		t.Errorf("L1Hits after promotion = %d, want 1", stats2.L1Hits)
	}
}

func TestL2_Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	c.WithL2(l2)

	ctx := context.Background()
	_, ok := c.Get(ctx, "nowhere")
	if ok {
		t.Error("should miss both L1 and L2")
	}

	stats := c.Stats()
	if stats.L2Misses != 1 {
		t.Errorf("L2Misses = %d, want 1", stats.L2Misses)
	}
}

func TestL2_Delete(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	c.WithL2(l2)

	ctx := context.Background()
	c.Set(ctx, "del-me", []byte("data"))
	c.Delete(ctx, "del-me")

	// Verify L2 also deleted.
	l2.mu.Lock()
	_, ok := l2.data["del-me"]
	l2.mu.Unlock()
	if ok {
		t.Error("L2 should have deleted the key")
	}
}

func TestL2_GetOrLoad(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	c.WithL2(l2)

	ctx := context.Background()
	got, err := c.GetOrLoad(ctx, "loaded", func(_ context.Context) ([]byte, error) {
		return []byte("from-loader"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "from-loader" {
		t.Errorf("got %q, want %q", got, "from-loader")
	}

	// Verify L2 received via write-through from Set.
	l2.mu.Lock()
	v, ok := l2.data["loaded"]
	l2.mu.Unlock()
	if !ok {
		t.Fatal("L2 should have received loaded value")
	}
	if string(v) != "from-loader" {
		t.Errorf("L2 got %q, want %q", v, "from-loader")
	}
}

func TestL2_Stats(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	l2.data["exists"] = []byte("yes")
	c.WithL2(l2)

	ctx := context.Background()
	c.Get(ctx, "exists") // L2 hit
	c.Get(ctx, "exists") // L1 hit (promoted)
	c.Get(ctx, "nope")   // L2 miss

	stats := c.Stats()
	if stats.L1Hits != 1 {
		t.Errorf("L1Hits = %d, want 1", stats.L1Hits)
	}
	if stats.L2Hits != 1 {
		t.Errorf("L2Hits = %d, want 1", stats.L2Hits)
	}
	if stats.L2Misses != 1 {
		t.Errorf("L2Misses = %d, want 1", stats.L2Misses)
	}
}

func TestNoL2_Unchanged(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "a", []byte("1"))
	got, ok := c.Get(ctx, "a")
	if !ok || string(got) != "1" {
		t.Errorf("L1-only: ok=%v, got=%q", ok, got)
	}
	_, ok2 := c.Get(ctx, "miss")
	if ok2 {
		t.Error("should miss without L2")
	}

	stats := c.Stats()
	if stats.L2Hits != 0 || stats.L2Misses != 0 {
		t.Errorf("L2 stats should be 0 without L2: hits=%d, misses=%d", stats.L2Hits, stats.L2Misses)
	}
}
