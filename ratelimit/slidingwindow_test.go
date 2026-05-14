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
