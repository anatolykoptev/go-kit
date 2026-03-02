package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterAllow(t *testing.T) {
	l := New(10, 3) // 10/s, burst 3
	// Burst should be available immediately.
	for i := range 3 {
		if !l.Allow() {
			t.Fatalf("Allow() = false on token %d, want true", i)
		}
	}
	// 4th should be denied.
	if l.Allow() {
		t.Error("Allow() = true after burst exhausted, want false")
	}
}

func TestLimiterWait(t *testing.T) {
	l := New(100, 1) // 100/s, burst 1
	// Consume the burst token.
	if !l.Allow() {
		t.Fatal("first Allow should succeed")
	}
	// Wait should block ~10ms for next token.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Errorf("Wait returned too fast: %v", elapsed)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait took too long: %v", elapsed)
	}
}

func TestLimiterWaitCancel(t *testing.T) {
	l := New(1, 1) // 1/s, burst 1
	l.Allow()      // exhaust
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err := l.Wait(ctx)
	if err == nil {
		t.Error("Wait should return error on cancelled context")
	}
}

func TestLimiterRate(t *testing.T) {
	l := New(100, 1) // 100/s
	l.Allow()        // exhaust burst
	start := time.Now()
	ctx := context.Background()
	for range 10 {
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	// 10 tokens at 100/s = ~100ms. Allow ±20%.
	if elapsed < 80*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("sustained rate: got %v for 10 tokens at 100/s, want ~100ms", elapsed)
	}
}

func TestKeyLimiterIndependence(t *testing.T) {
	kl := NewKeyLimiter(10, 2)
	defer kl.Close()
	// Exhaust key "a".
	kl.Allow("a")
	kl.Allow("a")
	if kl.Allow("a") {
		t.Error("key 'a' should be exhausted")
	}
	// Key "b" should still have tokens.
	if !kl.Allow("b") {
		t.Error("key 'b' should have tokens")
	}
}

func TestKeyLimiterCleanup(t *testing.T) {
	kl := NewKeyLimiter(10, 1)
	defer kl.Close()
	now := time.Now()
	kl.now = func() time.Time { return now }
	kl.Allow("a")
	kl.Allow("b")
	if kl.Len() != 2 {
		t.Fatalf("Len = %d, want 2", kl.Len())
	}
	// Advance time past idle threshold.
	kl.now = func() time.Time { return now.Add(2 * time.Minute) }
	removed := kl.Cleanup(time.Minute)
	if removed != 2 {
		t.Errorf("Cleanup removed %d, want 2", removed)
	}
	if kl.Len() != 0 {
		t.Errorf("Len after cleanup = %d, want 0", kl.Len())
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := New(1000, 100) // high rate to avoid blocking
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if l.Allow() {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	// Total allowed should not exceed burst + replenished during test.
	// Main check: no race detector failures.
	if allowed.Load() == 0 {
		t.Error("expected some allows to succeed")
	}
}

func TestKeyLimiterConcurrent(t *testing.T) {
	kl := NewKeyLimiter(1000, 10)
	defer kl.Close()
	keys := []string{"a", "b", "c", "d", "e"}
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				kl.Allow(key)
			}
		}()
	}
	wg.Wait()
	if kl.Len() != len(keys) {
		t.Errorf("Len = %d, want %d", kl.Len(), len(keys))
	}
}
