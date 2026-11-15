package stt

import (
	"context"
	"math/rand"
	"net/http"
	"testing"
	"time"
)

// forAll runs fn on n seeded pseudo-random cases and reports the first failure
// message fn returns (empty = property holds). The seed is fixed so a failure
// reproduces.
func forAll(t *testing.T, n int, fn func(r *rand.Rand) string) {
	t.Helper()
	r := rand.New(rand.NewSource(42)) //nolint:gosec,mnd // deterministic test data
	for i := range n {
		if msg := fn(r); msg != "" {
			t.Fatalf("case %d: %s", i, msg)
		}
	}
}

func intIn(r *rand.Rand, lo, hi int) int { return lo + r.Intn(hi-lo+1) }

// TestRetryBackoffNeverExceedsMaxDelay verifies the doubling-with-cap rule
// used by doWithRetry never yields a delay above maxDelay, whatever the
// attempt count. (It replays the loop's arithmetic rather than observing real
// sleeps; the retry_maxdelay tests cover the observable behaviour.)
func TestRetryBackoffNeverExceedsMaxDelay(t *testing.T) {
	forAll(t, 200, func(r *rand.Rand) string {
		rc := &retryConfig{
			maxAttempts: intIn(r, 1, 50),
			baseDelay:   time.Duration(intIn(r, 1, 1000)) * time.Millisecond,
			maxDelay:    time.Duration(intIn(r, 1, 10000)) * time.Millisecond,
		}
		delay := min(rc.baseDelay, rc.maxDelay)
		for range rc.maxAttempts - 1 {
			if delay > rc.maxDelay {
				return "delay exceeded maxDelay"
			}
			delay = min(delay*2, rc.maxDelay)
		}
		return ""
	})
}

// TestRetryAttemptsRespected verifies that doWithRetry calls fn exactly
// maxAttempts times for a persistently transient error.
func TestRetryAttemptsRespected(t *testing.T) {
	forAll(t, 100, func(r *rand.Rand) string {
		maxAttempts := intIn(r, 1, 20)
		rc := &retryConfig{maxAttempts: maxAttempts, baseDelay: time.Microsecond, maxDelay: 10 * time.Microsecond}
		calls := 0
		_, _ = doWithRetry(context.Background(), rc, nil, func() (string, error) {
			calls++
			return "", &Error{StatusCode: http.StatusServiceUnavailable, Message: "fail"}
		})
		if calls != maxAttempts {
			return "fn not called exactly maxAttempts times"
		}
		return ""
	})
}

// TestRetryNonTransientStopsImmediately verifies that a non-transient error
// stops retrying after one call, regardless of maxAttempts.
func TestRetryNonTransientStopsImmediately(t *testing.T) {
	forAll(t, 50, func(r *rand.Rand) string {
		rc := &retryConfig{maxAttempts: intIn(r, 1, 20), baseDelay: time.Microsecond, maxDelay: 10 * time.Microsecond}
		calls := 0
		_, _ = doWithRetry(context.Background(), rc, nil, func() (string, error) {
			calls++
			return "", &Error{StatusCode: http.StatusBadRequest, Message: "bad request"}
		})
		if calls != 1 {
			return "non-transient should stop after 1 call"
		}
		return ""
	})
}
