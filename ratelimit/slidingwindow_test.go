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
	sw, mr := newSW(t, 2*time.Second, 1)
	ctx := context.Background()
	ok, _, _ := sw.Allow(ctx, "u1")
	if !ok {
		t.Fatal("1st allow failed")
	}
	ok, _, _ = sw.Allow(ctx, "u1")
	if ok {
		t.Fatal("2nd should block")
	}
	mr.FastForward(3 * time.Second)
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
