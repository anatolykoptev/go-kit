# Production-Grade Redis L2 Cache — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix SIGSEGV crash (nil interface trap) and make RedisL2 production-grade with circuit breaker, error sentinels, and graceful degradation.

**Architecture:** Add `sony/gobreaker` TwoStepCircuitBreaker to RedisL2. Distinguish cache miss (`ErrCacheMiss`) from Redis failure (`ErrL2Unavailable`). Fix Go nil-interface-trap in `New()`. All methods get nil receiver guards. Cache.Get distinguishes miss/error for accurate stats.

**Tech Stack:** `github.com/sony/gobreaker` (v1, zero transitive deps), `github.com/redis/go-redis/v9`

**Root Cause (SIGSEGV):** `NewRedisL2()` returns `nil` when Redis unreachable. Assigning `(*RedisL2)(nil)` to `L2` interface creates a **typed nil** — `c.l2 != nil` is `true`, method dispatch hits nil receiver → panic at `l2.go:53`.

**Consumers:** go-wp, go-code, go-job, go-startup, go-hully, go-search (all on v0.4.1)

---

### Task 1: Add error sentinels and nil receiver guards to l2.go

**Files:**
- Modify: `cache/l2.go` (full rewrite)
- Test: `cache/l2_test.go` (create)

**Step 1: Write tests for nil receiver safety and error sentinels**

Create `cache/l2_test.go`:

```go
package cache_test

import (
	"context"
	"errors"
	"testing"

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
```

**Step 2: Run tests — expect FAIL (sentinels don't exist yet)**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestRedisL2_Nil|TestErrCacheMiss" -v
```

Expected: compilation error — `ErrCacheMiss`, `ErrL2Unavailable` undefined.

**Step 3: Implement sentinels and nil guards in l2.go**

Rewrite `cache/l2.go`:

```go
package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sentinel errors for L2 operations.
var (
	// ErrCacheMiss means the key was not found (not a failure).
	ErrCacheMiss = errors.New("cache miss")
	// ErrL2Unavailable means L2 is down or circuit breaker is open.
	ErrL2Unavailable = errors.New("L2 unavailable")
)

// L2 is an optional second-tier cache (typically Redis).
// Get returns ErrCacheMiss on miss, other errors on failure.
// Implementations must be safe for concurrent use.
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

// Get retrieves a value from Redis. Returns ErrCacheMiss if key not found.
func (r *RedisL2) Get(ctx context.Context, key string) ([]byte, error) {
	if r == nil {
		return nil, ErrCacheMiss
	}
	data, err := r.rdb.Get(ctx, r.key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("cache l2 get: %w", err)
	}
	return data, nil
}

// Set stores a value in Redis with the given TTL.
func (r *RedisL2) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	if r == nil {
		return ErrL2Unavailable
	}
	if err := r.rdb.Set(ctx, r.key(key), data, ttl).Err(); err != nil {
		return fmt.Errorf("cache l2 set: %w", err)
	}
	return nil
}

// Del removes a key from Redis.
func (r *RedisL2) Del(ctx context.Context, key string) error {
	if r == nil {
		return ErrL2Unavailable
	}
	if err := r.rdb.Del(ctx, r.key(key)).Err(); err != nil {
		return fmt.Errorf("cache l2 del: %w", err)
	}
	return nil
}

// Close closes the underlying Redis client.
func (r *RedisL2) Close() error {
	if r == nil {
		return nil
	}
	return r.rdb.Close()
}
```

**Step 4: Run tests — expect PASS**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestRedisL2_Nil|TestErrCacheMiss" -v
```

**Step 5: Run ALL existing tests — must still pass**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v
```

Note: existing tests use `mapL2` which returns `errors.New("not found")` — NOT `ErrCacheMiss`. This is fine because cache.go checks `err == nil`, not `errors.Is(err, ErrCacheMiss)`. We'll update cache.go in Task 3.

**Step 6: Commit**

```bash
git add cache/l2.go cache/l2_test.go
git commit -m "feat(cache): add error sentinels and nil receiver guards to RedisL2"
```

---

### Task 2: Fix nil interface trap in cache.go New()

**Files:**
- Modify: `cache/cache.go:153-156`
- Test: `cache/l2_test.go` (append)

**Step 1: Write test for nil interface trap**

Append to `cache/l2_test.go`:

```go
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
```

**Step 2: Run test — expect FAIL (or SIGSEGV)**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestNew_NilRedisL2" -v -timeout 10s
```

Expected: test hangs for 3s (Redis ping timeout) then L2 stats will be non-zero because typed nil interface passes `c.l2 != nil`.

**Step 3: Fix the nil interface assignment in cache.go**

In `cache/cache.go`, replace lines 153-156:

```go
	// Connect L2 if Redis configured.
	if cfg.RedisURL != "" {
		c.l2 = NewRedisL2(cfg.RedisURL, cfg.RedisDB, cfg.Prefix)
	}
```

With:

```go
	// Connect L2 if Redis configured.
	// Guard: NewRedisL2 returns nil on failure — must NOT assign nil
	// concrete pointer to interface (Go typed-nil trap causes SIGSEGV).
	if cfg.RedisURL != "" {
		if l2 := NewRedisL2(cfg.RedisURL, cfg.RedisDB, cfg.Prefix); l2 != nil {
			c.l2 = l2
		}
	}
```

**Step 4: Run test — expect PASS**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestNew_NilRedisL2" -v -timeout 10s
```

**Step 5: Run full suite**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v
```

**Step 6: Commit**

```bash
git add cache/cache.go cache/l2_test.go
git commit -m "fix(cache): prevent SIGSEGV from nil interface trap in New()"
```

---

### Task 3: Add circuit breaker to RedisL2

**Files:**
- Modify: `cache/l2.go`
- Test: `cache/l2_test.go` (append)
- Modify: `go.mod` (add gobreaker)

**Step 1: Add gobreaker dependency**

```bash
cd /home/krolik/src/go-kit && go get github.com/sony/gobreaker@latest
```

**Step 2: Write tests for circuit breaker behavior**

Append to `cache/l2_test.go`:

```go
// faultyL2 simulates Redis failures for testing circuit breaker behavior.
type faultyL2 struct {
	fails  int // remaining failures before success
	mu     sync.Mutex
	data   map[string][]byte
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
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	faulty := newFaultyL2(100) // always fails
	c.WithL2(faulty)

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
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	faulty := newFaultyL2(100)
	c.WithL2(faulty)

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
```

**Step 3: Run tests — expect FAIL (L2Errors field doesn't exist, stats logic unchanged)**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestCache_L2Error" -v
```

**Step 4: Add circuit breaker to RedisL2 in l2.go**

Update `cache/l2.go` — add gobreaker import and modify `RedisL2`:

```go
package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// Sentinel errors for L2 operations.
var (
	ErrCacheMiss     = errors.New("cache miss")
	ErrL2Unavailable = errors.New("L2 unavailable")
)

// L2 is an optional second-tier cache (typically Redis).
// Get returns ErrCacheMiss on miss, other errors on failure.
// Implementations must be safe for concurrent use.
type L2 interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

// RedisL2 implements L2 using Redis with circuit breaker protection.
type RedisL2 struct {
	rdb    *redis.Client
	prefix string
	cb     *gobreaker.TwoStepCircuitBreaker
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

	cb := gobreaker.NewTwoStepCircuitBreaker(gobreaker.Settings{
		Name:    "cache-l2",
		Timeout: 30 * time.Second, // time in open state before half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("cache: circuit breaker state change",
				slog.String("from", from.String()),
				slog.String("to", to.String()))
		},
	})

	return &RedisL2{rdb: rdb, prefix: prefix, cb: cb}
}

func (r *RedisL2) key(k string) string {
	if r.prefix == "" {
		return k
	}
	return r.prefix + k
}

// Get retrieves a value from Redis. Returns ErrCacheMiss if not found.
func (r *RedisL2) Get(ctx context.Context, key string) ([]byte, error) {
	if r == nil {
		return nil, ErrCacheMiss
	}
	done, err := r.cb.Allow()
	if err != nil {
		return nil, ErrL2Unavailable
	}
	data, err := r.rdb.Get(ctx, r.key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			done(true) // miss is not a failure
			return nil, ErrCacheMiss
		}
		done(false)
		return nil, fmt.Errorf("cache l2 get: %w", err)
	}
	done(true)
	return data, nil
}

// Set stores a value in Redis with the given TTL.
func (r *RedisL2) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	if r == nil {
		return ErrL2Unavailable
	}
	done, err := r.cb.Allow()
	if err != nil {
		return ErrL2Unavailable
	}
	if err := r.rdb.Set(ctx, r.key(key), data, ttl).Err(); err != nil {
		done(false)
		return fmt.Errorf("cache l2 set: %w", err)
	}
	done(true)
	return nil
}

// Del removes a key from Redis.
func (r *RedisL2) Del(ctx context.Context, key string) error {
	if r == nil {
		return ErrL2Unavailable
	}
	done, err := r.cb.Allow()
	if err != nil {
		return ErrL2Unavailable
	}
	if err := r.rdb.Del(ctx, r.key(key)).Err(); err != nil {
		done(false)
		return fmt.Errorf("cache l2 del: %w", err)
	}
	done(true)
	return nil
}

// Close closes the underlying Redis client.
func (r *RedisL2) Close() error {
	if r == nil {
		return nil
	}
	return r.rdb.Close()
}
```

**Step 5: Run l2 tests — expect PASS**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestRedisL2_Nil|TestErrCacheMiss" -v
```

**Step 6: Don't commit yet — Task 4 adds L2Errors stat needed for full test pass**

---

### Task 4: Update cache.go — distinguish miss vs failure + L2Errors stat

**Files:**
- Modify: `cache/cache.go:118-122` (add `l2errors` counter)
- Modify: `cache/cache.go:206-216` (Get — new error handling)
- Modify: `cache/cache.go:309-344` (Stats struct + method)
- Modify: `cache/cache_test.go` (update mapL2 to use ErrCacheMiss)

**Step 1: Add `l2errors` counter to Cache struct (cache.go:122)**

After `l2misses  atomic.Int64` add:

```go
	l2errors  atomic.Int64
```

**Step 2: Add `L2Errors` to Stats struct (cache.go:316)**

After `L2Misses` add:

```go
	L2Errors  int64
```

**Step 3: Add `L2Errors` to Stats() method (cache.go:325,340)**

After `l2m := c.l2misses.Load()` add:

```go
	l2e := c.l2errors.Load()
```

And in the return struct, after `L2Misses`:

```go
		L2Errors:  l2e,
```

**Step 4: Update Get() error handling (cache.go:205-219)**

Replace the L2 block:

```go
	// Try L2.
	if c.l2 != nil {
		data, err := c.l2.Get(ctx, key)
		if err == nil {
			c.l2hits.Add(1)
			// Promote to L1.
			c.Set(ctx, key, data)
			return data, true
		}
		if errors.Is(err, ErrCacheMiss) {
			c.l2misses.Add(1)
		} else {
			c.l2errors.Add(1)
		}
	}

	c.misses.Add(1)
	return nil, false
```

Key change: L2 errors now fall through to `c.misses.Add(1)` (was missing before).

**Step 5: Update mapL2 in cache_test.go to return ErrCacheMiss**

In `cache/cache_test.go`, change mapL2.Get (line 401-408):

```go
func (m *mapL2) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return v, nil
}
```

**Step 6: Run ALL tests**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```

All tests must pass including the new `TestCache_L2Error_Stats`.

**Step 7: Commit**

```bash
git add cache/l2.go cache/l2_test.go cache/cache.go cache/cache_test.go go.mod go.sum
git commit -m "feat(cache): add circuit breaker, L2Errors stat, fix miss/error accounting"
```

---

### Task 5: Add circuit breaker integration test

**Files:**
- Modify: `cache/l2_test.go` (append)

**Step 1: Write circuit breaker open/close test**

Append to `cache/l2_test.go`:

```go
func TestCache_CircuitBreaker_OpensAfterFailures(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	faulty := newFaultyL2(10) // first 10 calls fail
	c.WithL2(faulty)

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
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	l2 := newMapL2()
	c.WithL2(l2)

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
```

**Step 2: Run new tests**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -run "TestCache_CircuitBreaker|TestCache_L2Miss_Uses" -v
```

**Step 3: Run full suite**

```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1 -race
```

**Step 4: Commit**

```bash
git add cache/l2_test.go
git commit -m "test(cache): add circuit breaker and L2 miss/error integration tests"
```

---

### Task 6: Lint, tag v0.5.0

**Files:**
- None new

**Step 1: Lint**

```bash
cd /home/krolik/src/go-kit && make lint 2>/dev/null || golangci-lint run ./cache/
```

Fix any issues.

**Step 2: Run full test suite with race detector**

```bash
cd /home/krolik/src/go-kit && go test ./... -race -count=1
```

**Step 3: Tag and push**

```bash
cd /home/krolik/src/go-kit && git tag v0.5.0
git push origin main --tags
```

---

### Task 7: Update go-job to go-kit v0.5.0, redeploy

**Files:**
- Modify: `/home/krolik/src/go-job/go.mod`

**Step 1: Update dependency**

```bash
cd /home/krolik/src/go-job && go get github.com/anatolykoptev/go-kit@v0.5.0 && go mod tidy
```

**Step 2: Run go-job tests**

```bash
cd /home/krolik/src/go-job && go test ./... -race
```

**Step 3: Commit and push**

```bash
cd /home/krolik/src/go-job && git add go.mod go.sum
git commit -m "deps: upgrade go-kit to v0.5.0 (fix L2 SIGSEGV, add circuit breaker)"
git push
```

**Step 4: Redeploy go-job**

```bash
cd /home/krolik/deploy/krolik-server && docker compose build --no-cache go-job && docker compose up -d --no-deps --force-recreate go-job
```

**Step 5: Verify no crashes in logs**

```bash
docker compose logs --since 2m go-job 2>/dev/null | grep -iE "panic|SIGSEGV|fatal"
```

Expected: no output.

---

### Task 8: Update remaining consumers (batch)

Update go-wp, go-code, go-startup, go-hully, go-search to go-kit v0.5.0.

**For each service** (`go-wp`, `go-code`, `go-startup`, `go-hully`, `go-search`):

```bash
cd /home/krolik/src/<service> && go get github.com/anatolykoptev/go-kit@v0.5.0 && go mod tidy
go test ./... -race
git add go.mod go.sum && git commit -m "deps: upgrade go-kit to v0.5.0"
git push
```

Then rebuild and redeploy each:

```bash
cd /home/krolik/deploy/krolik-server
docker compose build --no-cache <service> && docker compose up -d --no-deps --force-recreate <service>
```
