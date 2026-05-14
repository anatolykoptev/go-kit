package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/anatolykoptev/go-kit/ratelimit"
	"github.com/redis/go-redis/v9"
)

func newSW(t *testing.T, window time.Duration, limit int) (*ratelimit.SlidingWindow, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "test",
		Window:    window,
		Limit:     limit,
		FailOpen:  true,
	})
	return sw, mr
}

func TestSlidingWindow_AllowsUntilLimit(t *testing.T) {
	sw, _ := newSW(t, time.Minute, 3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ok, _, err := sw.Allow(ctx, "u1")
		if err != nil || !ok {
			t.Fatalf("hit %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, _, _ := sw.Allow(ctx, "u1")
	if ok {
		t.Fatal("expected block on 4th hit")
	}
}

func TestSlidingWindow_BucketRollover(t *testing.T) {
	// Use a shared time variable so both miniredis and the limiter agree on
	// the current time. Advance both by setting mr.SetTime and updating the
	// variable; the Now func captures the variable by pointer.
	now := time.Now().Truncate(time.Second)
	mr := miniredis.RunT(t)
	mr.SetTime(now)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "test",
		Window:    2 * time.Second,
		Limit:     1,
		FailOpen:  true,
		Now:       func() time.Time { return now },
	})

	ctx := context.Background()
	ok, _, _ := sw.Allow(ctx, "u1")
	if !ok {
		t.Fatal("1st allow failed")
	}
	ok, _, _ = sw.Allow(ctx, "u1")
	if ok {
		t.Fatal("2nd should block")
	}

	// Advance time past the window so old bucket keys expire.
	advance := 3 * time.Second
	now = now.Add(advance)
	mr.FastForward(advance)

	ok, _, _ = sw.Allow(ctx, "u1")
	if !ok {
		t.Fatal("after rollover should allow")
	}
}

func TestSlidingWindow_FailOpenOnRedisError(t *testing.T) {
	sw, mr := newSW(t, time.Minute, 3)
	mr.Close()
	ok, _, err := sw.Allow(context.Background(), "u1")
	if !ok || err == nil {
		t.Fatalf("fail-open broken: ok=%v err=%v", ok, err)
	}
}

func TestSlidingWindow_FailClosedOnRedisError(t *testing.T) {
	// FailOpen=false is the production default for login throttling:
	// Redis down → deny access (fail-closed).
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "test",
		Window:    time.Minute,
		Limit:     5,
		FailOpen:  false,
	})
	mr.Close()
	ok, rem, err := sw.Allow(context.Background(), "u1")
	if ok {
		t.Fatal("fail-closed: expected deny when Redis is down")
	}
	if rem != 0 {
		t.Fatalf("fail-closed: expected remaining=0 on error, got %d", rem)
	}
	if err == nil {
		t.Fatal("fail-closed: expected non-nil error")
	}
}

func TestSlidingWindow_Reset(t *testing.T) {
	sw, _ := newSW(t, time.Minute, 1)
	ctx := context.Background()
	sw.Allow(ctx, "u1") //nolint:errcheck
	if err := sw.Reset(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	ok, _, _ := sw.Allow(ctx, "u1")
	if !ok {
		t.Fatal("expected allow after reset")
	}
}

func TestSlidingWindow_RemainingDecreases(t *testing.T) {
	sw, _ := newSW(t, time.Minute, 5)
	ctx := context.Background()
	_, rem, err := sw.Allow(ctx, "u2")
	if err != nil {
		t.Fatal(err)
	}
	if rem != 4 {
		t.Fatalf("remaining after 1st hit = %d, want 4", rem)
	}
	_, rem, _ = sw.Allow(ctx, "u2")
	if rem != 3 {
		t.Fatalf("remaining after 2nd hit = %d, want 3", rem)
	}
}

func TestSlidingWindow_PanicsOnNilRedis(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil Redis")
		}
	}()
	ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		KeyPrefix: "test",
		Window:    time.Minute,
		Limit:     5,
	})
}

func TestSlidingWindow_PanicsOnNonPositiveLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on non-positive Limit")
		}
	}()
	ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "test",
		Window:    time.Minute,
		Limit:     0,
	})
}

func TestSlidingWindow_PanicsOnEmptyKeyPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty KeyPrefix")
		}
	}()
	ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:  rdb,
		Window: time.Minute,
		Limit:  5,
	})
}

// ---------------------------------------------------------------------------
// Commit 4: Atomic INCR+EXPIRE via Lua
// ---------------------------------------------------------------------------

// TestSlidingWindow_TTLSetOnFirstIncr verifies that after the first Allow call
// the bucket key has a TTL (not -1 / no TTL), confirming the Lua EXPIRE ran.
func TestSlidingWindow_TTLSetOnFirstIncr(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	now := time.Now().Truncate(time.Second)
	window := 2 * time.Minute
	sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "ttltest",
		Window:    window,
		Limit:     10,
		FailOpen:  true,
		Now:       func() time.Time { return now },
	})

	ok, _, err := sw.Allow(context.Background(), "user1")
	if !ok || err != nil {
		t.Fatalf("Allow: ok=%v err=%v", ok, err)
	}

	// Scan all keys with the prefix and check each has a TTL.
	keys, err := rdb.Keys(context.Background(), "ttltest:user1:*").Result()
	if err != nil {
		t.Fatalf("KEYS: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no bucket keys found after Allow")
	}
	for _, k := range keys {
		ttl := mr.TTL(k)
		if ttl <= 0 {
			t.Errorf("key %q has TTL=%v (want > 0; no-TTL = orphaned key)", k, ttl)
		}
	}
}

// TestSlidingWindow_TTLNotResetOnSubsequentIncr verifies that the set-once Lua
// semantics (EXPIRE only when v==1) do NOT reset the TTL on subsequent calls.
// This distinguishes the Lua implementation from the previous pipeline which
// sent EXPIRE on every call — with set-once, the TTL should decrease between
// the first and second call (not jump back to the initial value).
func TestSlidingWindow_TTLNotResetOnSubsequentIncr(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	now := time.Now().Truncate(time.Second)
	window := 2 * time.Minute
	sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
		Redis:     rdb,
		KeyPrefix: "ttltest2",
		Window:    window,
		Limit:     10,
		FailOpen:  true,
		Now:       func() time.Time { return now },
	})

	ctx := context.Background()
	sw.Allow(ctx, "user2") //nolint:errcheck

	// Capture TTL after first call.
	keys, _ := rdb.Keys(ctx, "ttltest2:user2:*").Result()
	if len(keys) == 0 {
		t.Fatal("no bucket keys after first Allow")
	}
	ttl1 := mr.TTL(keys[0])

	// Advance miniredis clock slightly so TTL decreases.
	mr.FastForward(5 * time.Second)

	// Second Allow in the same bucket.
	sw.Allow(ctx, "user2") //nolint:errcheck

	ttl2 := mr.TTL(keys[0])

	// With set-once semantics: TTL2 < TTL1 (TTL was not reset to Window+bucket).
	// With reset-every semantics: TTL2 ≈ TTL1 (TTL was refreshed).
	// We assert set-once: TTL must have decreased, not been reset.
	if ttl2 >= ttl1 {
		t.Errorf("TTL reset on 2nd call (set-once expected): ttl1=%v ttl2=%v", ttl1, ttl2)
	}
}
