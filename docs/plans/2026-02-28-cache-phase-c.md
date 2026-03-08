# cache Phase C: L2 Redis Integration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add optional L2 Redis layer to go-kit cache — L1 miss falls through to Redis, L2 hits promote to L1, writes go to both tiers, Redis failures are silent.

**Architecture:** Minimal `L2` interface decouples Redis from cache core. `RedisL2` implements it via go-redis/v9. Integrated into existing Get/Set/Delete/GetOrLoad — no API changes. When `Config.RedisURL` is empty, L2 is nil (current behavior preserved). Stats extended with L2Hits/L2Misses. Tests use a `mapL2` mock — no real Redis needed.

**Tech Stack:** `github.com/redis/go-redis/v9` (same version all consumers use)

---

### Task 1: Add go-redis dependency and L2 interface

**Files:** go.mod, cache/l2.go (new)

#### 1a. Add go-redis dependency

```bash
cd /home/krolik/src/go-kit && go get github.com/redis/go-redis/v9
```

#### 1b. Create cache/l2.go with L2 interface and RedisL2 implementation

```go
package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// L2 is an optional second-tier cache (typically Redis).
// Get returns the value and nil error on hit, redis.Nil on miss,
// or a non-nil error on failure. Implementations must be safe
// for concurrent use.
type L2 interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

// RedisL2 implements L2 using Redis.
type RedisL2 struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisL2 connects to Redis and returns an L2 store.
// Returns nil if the URL is empty or Redis is unreachable (logs a warning).
func NewRedisL2(redisURL string, db int, prefix string) *RedisL2 {
	if redisURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Warn("cache: invalid redis URL, L2 disabled", slog.Any("error", err))
		return nil
	}
	opts.DB = db

	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("cache: redis unreachable, L2 disabled", slog.Any("error", err))
		rdb.Close()
		return nil
	}

	return &RedisL2{rdb: rdb, prefix: prefix}
}

func (r *RedisL2) key(k string) string {
	if r.prefix == "" {
		return k
	}
	return r.prefix + k
}

func (r *RedisL2) Get(ctx context.Context, key string) ([]byte, error) {
	return r.rdb.Get(ctx, r.key(key)).Bytes()
}

func (r *RedisL2) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return r.rdb.Set(ctx, r.key(key), data, ttl).Err()
}

func (r *RedisL2) Del(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, r.key(key)).Err()
}

func (r *RedisL2) Close() error {
	return r.rdb.Close()
}
```

**Step 1:** Add go-redis dependency, create `cache/l2.go`.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All existing tests PASS (L2 is nil, no behavior change).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add go.mod go.sum cache/l2.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: add L2 interface and RedisL2 implementation

L2 interface decouples Redis from cache core. RedisL2 wraps
go-redis/v9 with prefix support and graceful degradation
(nil on empty URL or unreachable Redis).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Integrate L2 into Cache struct and Get/Set/Delete

**Files:** cache/cache.go

#### 2a. Add L2 field to Cache struct and wire in New()

Add `l2` field to Cache struct (after `flight group`):

```go
type Cache struct {
	// ... existing fields ...
	flight group
	l2     L2 // optional L2 store; nil = L1-only
	done   chan struct{}
}
```

In `New()`, after creating the cache, auto-connect Redis if configured:

```go
func New(cfg Config) *Cache {
	cfg.applyDefaults()
	// ... existing code ...
	c := &Cache{
		// ... existing fields ...
	}

	// Connect L2 if Redis configured.
	if cfg.RedisURL != "" {
		c.l2 = NewRedisL2(cfg.RedisURL, cfg.RedisDB, cfg.Prefix)
	}

	// ... cleanup goroutine ...
	return c
}
```

Also add a `WithL2` option for injecting a custom L2 (used in tests):

```go
// WithL2 sets a custom L2 store. Use in tests with a mock.
// Overrides any RedisURL in Config.
func (c *Cache) WithL2(l2 L2) { c.l2 = l2 }
```

#### 2b. Update Get() — L1 miss falls through to L2

Replace the TODO block in Get(). On L1 miss, if L2 is set, try L2.
On L2 hit, promote to L1 and return. On L2 miss/error, return miss.

```go
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	c.mu.Lock()

	e, ok := c.items[key]
	if ok && !time.Now().After(e.expiresAt) {
		if e.freq < 3 {
			e.freq++
		}
		data := e.data
		c.mu.Unlock()
		c.hits.Add(1)
		return data, true
	}

	// L1 miss or expired.
	if ok {
		c.removeEntry(e)
	}
	c.mu.Unlock()

	// Try L2.
	if c.l2 != nil {
		data, err := c.l2.Get(ctx, key)
		if err == nil {
			c.l2hits.Add(1)
			// Promote to L1.
			c.Set(ctx, key, data)
			return data, true
		}
		c.l2misses.Add(1)
	} else {
		c.misses.Add(1)
	}

	return nil, false
}
```

Note: this changes the Get() structure — the unlock must happen before L2 call (we don't hold the mutex during network IO). We need to add `l2hits` and `l2misses` atomic counters.

#### 2c. Update Set() — write-through to L2

After L1 set, write to L2 (best-effort, log on error):

```go
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	c.mu.Lock()
	// ... existing L1 set logic (unchanged) ...
	c.mu.Unlock()

	// Write-through to L2 (best-effort).
	if c.l2 != nil {
		if err := c.l2.Set(ctx, key, data, c.cfg.L2TTL); err != nil {
			slog.Debug("cache: L2 set failed", slog.Any("error", err))
		}
	}
}
```

Note: Set() currently holds the lock for its entire body. We need to restructure so the L2 write happens outside the lock.

#### 2d. Update Delete() — delete from L2

```go
func (c *Cache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		c.removeEntry(e)
	}
	c.mu.Unlock()

	// Delete from L2 (best-effort).
	if c.l2 != nil {
		if err := c.l2.Del(ctx, key); err != nil {
			slog.Debug("cache: L2 del failed", slog.Any("error", err))
		}
	}
}
```

#### 2e. Add L2 counters and update Stats

Add to Cache struct:

```go
	l2hits   atomic.Int64
	l2misses atomic.Int64
```

Update Stats struct and Stats() method:

```go
type Stats struct {
	L1Hits    int64
	L1Misses  int64
	L1Size    int
	L2Hits    int64
	L2Misses  int64
	Evictions int64
	HitRatio  float64
}

func (c *Cache) Stats() Stats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	l2h := c.l2hits.Load()
	l2m := c.l2misses.Load()
	totalHits := hits + l2h
	totalMisses := misses + l2m
	var ratio float64
	if total := totalHits + totalMisses; total > 0 {
		ratio = float64(totalHits) / float64(total)
	}
	c.mu.Lock()
	size := len(c.items)
	c.mu.Unlock()
	return Stats{
		L1Hits:    hits,
		L1Misses:  misses,
		L1Size:    size,
		L2Hits:    l2h,
		L2Misses:  l2m,
		Evictions: c.evictions.Load(),
		HitRatio:  ratio,
	}
}
```

#### 2f. Update Close() to close L2

```go
func (c *Cache) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	if c.l2 != nil {
		c.l2.Close()
	}
}
```

**Step 1:** Apply all changes (2a-2f).

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All existing tests PASS (L2 is nil, behavior unchanged).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: integrate L2 into Get/Set/Delete/Close

L1 miss falls through to L2 (read-through with L1 promotion).
Set writes both L1 and L2 (write-through, best-effort).
Delete removes from both tiers. L2 failures are silent (logged).
Stats extended with L2Hits/L2Misses. WithL2() for test injection.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Tests for L2 integration

**Files:** cache/cache_test.go

Add a `mapL2` mock to the test file for testing L2 behavior without Redis:

```go
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
```

**Test: L2 write-through on Set**

```go
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
```

**Test: L2 read-through (L1 miss → L2 hit → L1 promote)**

```go
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
```

**Test: L2 miss returns false**

```go
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
```

**Test: Delete removes from L2**

```go
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
```

**Test: GetOrLoad populates L2**

```go
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
```

**Test: L2 stats**

```go
func TestL2_Stats(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	l2.data["exists"] = []byte("yes")
	c.WithL2(l2)

	ctx := context.Background()
	c.Get(ctx, "exists")   // L2 hit
	c.Get(ctx, "exists")   // L1 hit (promoted)
	c.Get(ctx, "nope")     // L2 miss

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
```

**Test: No L2 — original behavior unchanged**

```go
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
```

**Step 1:** Add mapL2 mock and all 7 tests.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All tests PASS (existing + 7 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: add tests for L2 integration

7 new tests: write-through, read-through with L1 promotion,
L2 miss, delete propagation, GetOrLoad populates L2,
L2 stats tracking, no-L2 backward compat.
Uses mapL2 mock — no Redis needed.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update cache section with L2 examples:

```go
// L1-only (no Redis dependency at runtime)
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
})

// L1 + L2 Redis (read-through, write-through)
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
    RedisURL:   "redis://localhost:6379",
    RedisDB:    0,
    Prefix:     "myapp:",
    L2TTL:      24 * time.Hour,
})

// Custom L2 store (testing or alternative backends)
c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
c.WithL2(myCustomStore)
```

Add bullet points:
- L2 Redis: optional, graceful degradation (L1-only if Redis unreachable)
- Read-through: L1 miss → L2 hit → automatic L1 promotion
- Write-through: Set/Delete propagate to both tiers
- L2 interface: plug in custom backends for testing or alternatives

**ROADMAP changes:**
- Mark cache Phase C as DONE
- Update cache status in Summary table: "DONE (L1) → DONE"

**Step 1:** Update README.md cache section.

**Step 2:** Update ROADMAP.md cache Phase C status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update cache section for Phase C L2 Redis

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
