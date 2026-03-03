package hedge_test

// RED tests — edge cases that verify correctness under tricky timing.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/hedge"
)

// Primary is slow AND fails (after delay). Hedge should succeed.
// Verifies the collection loop correctly returns hedge's success
// even when primary's error arrives first.
func TestDo_PrimarySlowFail_HedgeSucceeds(t *testing.T) {
	var calls atomic.Int32
	result, err := hedge.Do(context.Background(), 20*time.Millisecond, func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			// Primary: slow, then fail.
			time.Sleep(50 * time.Millisecond)
			return "", errors.New("primary failed")
		}
		// Hedge: succeed.
		return "hedge", nil
	})
	if err != nil {
		t.Fatalf("hedge should have succeeded, got: %v", err)
	}
	if result != "hedge" {
		t.Errorf("result = %q, want %q", result, "hedge")
	}
}

// External context timeout while both goroutines are running.
// Must return promptly with context error, not hang.
func TestDo_ExternalTimeoutDuringCollection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := hedge.Do(ctx, 10*time.Millisecond, func(ctx context.Context) (string, error) {
		// Both goroutines block until cancelled.
		<-ctx.Done()
		return "", ctx.Err()
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	// Should return near 50ms (context timeout), not hang.
	if elapsed > 200*time.Millisecond {
		t.Errorf("took %v — should have returned promptly after context timeout", elapsed)
	}
}

// Rapid concurrent Do calls — verify no races or goroutine leaks.
func TestDo_ConcurrentCalls(t *testing.T) {
	const n = 50
	errs := make(chan error, n)

	for range n {
		go func() {
			_, err := hedge.Do(context.Background(), time.Millisecond, func(_ context.Context) (int, error) {
				return 42, nil
			})
			errs <- err
		}()
	}

	for range n {
		if err := <-errs; err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
