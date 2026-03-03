package ratelimit

// RED tests — written to expose bugs, expected to FAIL before fix.

import (
	"context"
	"testing"
	"time"
)

// BUG: release() is the shared method cl.release, not a per-acquire closure.
// Calling release() twice after a single Acquire:
// - First call: <-cl.sem drains the acquired slot (correct).
// - Second call: <-cl.sem tries to drain again — blocks forever on empty channel.
// Should be safe: either idempotent (no-op) or panic.
func TestConcurrencyLimiter_DoubleRelease_Deadlocks(t *testing.T) {
	cl := NewConcurrencyLimiter(1)
	release, _ := cl.Acquire(context.Background())
	release() // correct first release

	done := make(chan struct{})
	go func() {
		defer close(done)
		release() // BUG: blocks forever on empty channel
	}()
	select {
	case <-done:
		// Fixed: second call was a no-op
	case <-time.After(200 * time.Millisecond):
		t.Fatal("double release() deadlocked — second <-cl.sem blocks on empty channel")
	}
}

// BUG: With limit=2, double-releasing one slot frees ANOTHER holder's slot.
// This silently allows more concurrent operations than the configured limit.
//
// Sequence: Acquire r1, Acquire r2 (full). Release r1 twice.
// Second release drains r2's slot → Available() returns 2, but r2 is still held.
func TestConcurrencyLimiter_DoubleRelease_ExceedsLimit(t *testing.T) {
	cl := NewConcurrencyLimiter(2)
	r1, _ := cl.Acquire(context.Background())
	_, _ = cl.Acquire(context.Background()) // r2 — held, never released

	r1()  // correct release — Available should be 1
	r1()  // BUG: drains r2's slot — Available becomes 2

	// r2 is still logically held. Correct: Available = 1.
	if avail := cl.Available(); avail != 1 {
		t.Errorf("Available = %d after double-release, want 1 (r2 still held)", avail)
	}
}

// BUG: TryAcquire returns the same shared release method.
// Double-release via TryAcquire has the same deadlock behavior.
func TestConcurrencyLimiter_TryAcquire_DoubleRelease(t *testing.T) {
	cl := NewConcurrencyLimiter(1)
	release, ok := cl.TryAcquire()
	if !ok {
		t.Fatal("TryAcquire should succeed")
	}
	release() // correct

	done := make(chan struct{})
	go func() {
		defer close(done)
		release() // BUG: deadlocks
	}()
	select {
	case <-done:
		// Fixed
	case <-time.After(200 * time.Millisecond):
		t.Fatal("TryAcquire double release() deadlocked")
	}
}
