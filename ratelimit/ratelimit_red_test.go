package ratelimit

// RED tests — written to expose bugs, expected to FAIL before fix.

import (
	"context"
	"errors"
	"testing"
	"time"
)

// BUG: New(10, 0) — burst=0 means tokens can never reach 1.
// Wait() without context timeout hangs forever. Should return ErrZero immediately.
func TestLimiterWait_ZeroBurst_ReturnsError(t *testing.T) {
	l := New(10, 0)
	done := make(chan error, 1)
	go func() { done <- l.Wait(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Wait with burst=0 should return error")
		}
		if !errors.Is(err, ErrLimiterZero) {
			t.Errorf("err = %v, want ErrLimiterZero", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Wait() with burst=0 hung — should return ErrLimiterZero immediately")
	}
}

// BUG: New(0, 2) — after burst exhausted, rate=0 means tokens never refill.
// Wait() should detect and return ErrLimiterZero, not busy-loop.
func TestLimiterWait_ZeroRate_ReturnsError(t *testing.T) {
	l := New(0, 2)
	l.Allow() // consume
	l.Allow() // consume — burst exhausted

	done := make(chan error, 1)
	go func() { done <- l.Wait(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Wait with rate=0 should return error after burst exhausted")
		}
		if !errors.Is(err, ErrLimiterZero) {
			t.Errorf("err = %v, want ErrLimiterZero", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Wait() with rate=0 hung — should return ErrLimiterZero immediately")
	}
}

// rate=0, burst=0: worst case, should return immediately.
func TestLimiterWait_ZeroRateZeroBurst(t *testing.T) {
	l := New(0, 0)
	done := make(chan error, 1)
	go func() { done <- l.Wait(context.Background()) }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrLimiterZero) {
			t.Errorf("err = %v, want ErrLimiterZero", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Wait(0,0) hung — should return ErrLimiterZero immediately")
	}
}

// Allow always returns false for burst=0.
func TestLimiterAllow_ZeroBurst_ReturnsFalse(t *testing.T) {
	l := New(1000, 0)
	if l.Allow() {
		t.Error("Allow should return false with burst=0")
	}
}

// Allow with rate=0 works for burst tokens, then always false.
func TestLimiterAllow_ZeroRate_BurstThenDeny(t *testing.T) {
	l := New(0, 3)
	for i := range 3 {
		if !l.Allow() {
			t.Fatalf("Allow() #%d should succeed (burst)", i+1)
		}
	}
	if l.Allow() {
		t.Error("Allow() should fail after burst exhausted with rate=0")
	}
}

// KeyLimiter with zero burst: should return ErrLimiterZero.
func TestKeyLimiterWait_ZeroBurst_ReturnsError(t *testing.T) {
	kl := NewKeyLimiter(10, 0)
	defer kl.Close()

	done := make(chan error, 1)
	go func() { done <- kl.Wait(context.Background(), "key") }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrLimiterZero) {
			t.Errorf("err = %v, want ErrLimiterZero", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("KeyLimiter.Wait with burst=0 hung")
	}
}

// Verify refill doesn't overshoot after long idle period.
func TestLimiterRefill_LongIdle(t *testing.T) {
	now := time.Now()
	l := New(10, 5)
	l.now = func() time.Time { return now }

	// Drain all tokens.
	for l.Allow() {
	}

	// Advance 10 seconds — should refill 100 tokens, but cap at burst=5.
	l.now = func() time.Time { return now.Add(10 * time.Second) }
	count := 0
	for l.Allow() {
		count++
	}
	if count != 5 {
		t.Errorf("after long idle: got %d tokens, want 5 (burst cap)", count)
	}
}
