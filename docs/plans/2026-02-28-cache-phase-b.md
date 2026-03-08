# cache Phase B: GetOrLoad, TTL Jitter, Enhanced Stats

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add GetOrLoad with inline singleflight (prevents thundering herd), TTL jitter (prevents cache stampedes), and enhanced statistics (evictions, hit ratio) — making the cache production-grade.

**Architecture:** All additions to cache.go. Inline singleflight (~25 LOC) avoids external deps. TTL jitter uses `math/rand/v2`. Stats adds evictions counter + HitRatio. B1 (generics) deferred — all 8 consumers use `[]byte`, no immediate value from generics.

**Tech Stack:** Go stdlib only (`math/rand/v2` for jitter, `sync` for singleflight)

---

### Task 1: All cache code additions

**Files:** cache/cache.go

**Add to imports:** `"math/rand/v2"`

#### 1a. Inline singleflight

```go
// group deduplicates concurrent loads for the same key.
type group struct {
	mu    sync.Mutex
	calls map[string]*groupCall
}

type groupCall struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

func (g *group) do(key string, fn func() ([]byte, error)) ([]byte, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*groupCall)
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &groupCall{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()

	return c.val, c.err
}
```

#### 1b. Update Config — add JitterPercent

```go
type Config struct {
	RedisURL   string
	RedisDB    int
	Prefix     string
	L1MaxItems int
	L1TTL      time.Duration
	L2TTL      time.Duration

	// JitterPercent adds random TTL variation to prevent cache stampedes.
	// 0.1 means ±10% jitter. 0 disables jitter (default).
	JitterPercent float64
}
```

#### 1c. Update Cache struct — add group + evictions

```go
type Cache struct {
	cfg Config

	mu       sync.Mutex
	items    map[string]*entry
	small    *list.List
	main     *list.List
	ghost    *list.List
	ghostMap map[string]*list.Element

	smallCap int
	mainCap  int
	ghostCap int

	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64

	flight group
	done   chan struct{}
}
```

#### 1d. TTL jitter helper

```go
func (c *Cache) jitteredTTL() time.Duration {
	ttl := c.cfg.L1TTL
	if c.cfg.JitterPercent <= 0 {
		return ttl
	}
	jitter := int64(float64(ttl) * c.cfg.JitterPercent)
	if jitter <= 0 {
		return ttl
	}
	return ttl + time.Duration(rand.Int64N(2*jitter+1)-jitter)
}
```

#### 1e. Update Set() — use jitteredTTL

Replace all `time.Now().Add(c.cfg.L1TTL)` in Set with `time.Now().Add(c.jitteredTTL())`:

```go
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if e, ok := c.items[key]; ok {
		e.data = data
		e.expiresAt = time.Now().Add(c.jitteredTTL())
		return
	}

	// Evict until under capacity.
	for len(c.items) >= c.cfg.L1MaxItems {
		if !c.evict() {
			break
		}
	}

	// Check ghost for frequency boost.
	var initFreq uint8
	if ge, ok := c.ghostMap[key]; ok {
		c.ghost.Remove(ge)
		delete(c.ghostMap, key)
		initFreq = 1
	}

	// Insert into small queue.
	e := &entry{
		key:       key,
		data:      data,
		expiresAt: time.Now().Add(c.jitteredTTL()),
		freq:      initFreq,
	}
	e.elem = c.small.PushBack(e)
	c.items[key] = e

	_ = ctx
}
```

#### 1f. Update evict() — count evictions

Add `c.evictions.Add(1)` every time an entry is fully evicted (removed from items). There are 5 places in evict() where `delete(c.items, e.key)` is called (but NOT when promoting to main). Add the counter at each eviction point:

```go
// In evict(), after each delete(c.items, e.key):
c.evictions.Add(1)
```

Specifically, the 5 eviction points are:
1. Expired entry from small queue (line ~246)
2. One-hit wonder from small queue (line ~259)
3. Expired entry from main queue (line ~272)
4. Zero-freq entry from main queue (line ~282)
5. Safety force-evict from main (line ~290)

#### 1g. GetOrLoad — cache-aside with singleflight

```go
// GetOrLoad returns the value for key, loading it via loader on cache miss.
// Concurrent loads for the same key are deduplicated (singleflight).
// The loaded value is stored in L1.
func (c *Cache) GetOrLoad(ctx context.Context, key string, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	if data, ok := c.Get(ctx, key); ok {
		return data, nil
	}

	data, err := c.flight.do(key, func() ([]byte, error) {
		return loader(ctx)
	})
	if err != nil {
		return nil, err
	}

	c.Set(ctx, key, data)
	return data, nil
}
```

#### 1h. Update Stats — add Evictions + HitRatio

```go
type Stats struct {
	L1Hits    int64
	L1Misses  int64
	L1Size    int
	Evictions int64
	HitRatio  float64
}

func (c *Cache) Stats() Stats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	var ratio float64
	if total := hits + misses; total > 0 {
		ratio = float64(hits) / float64(total)
	}
	c.mu.Lock()
	size := len(c.items)
	c.mu.Unlock()
	return Stats{
		L1Hits:    hits,
		L1Misses:  misses,
		L1Size:    size,
		Evictions: c.evictions.Load(),
		HitRatio:  ratio,
	}
}
```

**Step 1:** Write the complete updated cache.go with all additions.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All 11 existing tests PASS.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: add GetOrLoad, TTL jitter, enhanced stats

Three production-grade additions:
- GetOrLoad: cache-aside with inline singleflight (zero deps)
- JitterPercent: random ±N% TTL variation to prevent stampedes
- Stats: evictions counter + HitRatio convenience field

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new features

**Files:** cache/cache_test.go

**Add to imports:** `"sync"`, `"sync/atomic"`

**Test: GetOrLoad cache hit (no loader call)**

```go
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
```

**Test: GetOrLoad cache miss (loader called, result stored)**

```go
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
```

**Test: GetOrLoad singleflight deduplication**

```go
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
```

**Test: TTL jitter produces varying expiry times**

```go
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
```

**Test: Stats includes evictions**

```go
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
```

**Test: Stats HitRatio**

```go
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
```

**Step 1:** Add all 6 tests to cache_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All 17 tests PASS (11 existing + 6 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: add tests for GetOrLoad, singleflight, jitter, stats

6 new tests covering GetOrLoad (hit/miss/singleflight),
TTL jitter variation, evictions counting, and HitRatio.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update cache section with new features:

```go
import "github.com/anatolykoptev/go-kit/cache"

c := cache.New(cache.Config{
    L1MaxItems:    1000,
    L1TTL:         30 * time.Minute,
    JitterPercent: 0.1,  // ±10% TTL jitter prevents stampedes
})
defer c.Close()

c.Set(ctx, "key", data)
data, ok := c.Get(ctx, "key")

// Cache-aside with singleflight (concurrent loads deduplicated)
data, err := c.GetOrLoad(ctx, "key", func(ctx context.Context) ([]byte, error) {
    return fetchFromDB(ctx, "key")
})

// Statistics
stats := c.Stats()
fmt.Printf("Hit ratio: %.1f%%, Evictions: %d\n", stats.HitRatio*100, stats.Evictions)
```

Update bullet points:
- GetOrLoad with inline singleflight (prevents thundering herd)
- TTL jitter (prevents cache stampedes)
- Evictions counter + HitRatio in Stats

**ROADMAP changes:**
- Mark cache Phase B as partially DONE (B2-B4). Note B1 (generics) deferred.

**Step 1:** Update README.md cache section.

**Step 2:** Update ROADMAP.md cache Phase B status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update cache section for Phase B features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
