package stt

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCircuitBreakerConcurrentAllow verifies that under concurrent access,
// the circuit breaker never allows more than one probe in half-open state.
// This is the blocking test from bug-hunt t17.
func TestCircuitBreakerConcurrentAllow(t *testing.T) {
	cb := &circuitBreaker{
		maxFails: 3, //nolint:mnd
		cooldown: 10 * time.Second,
	}

	// Trip the breaker: 3 failures.
	for range 3 {
		cb.recordFailure()
	}

	// Now breaker is open. Advance time past cooldown using a fake clock.
	fakeNow := time.Now().Add(15 * time.Second) //nolint:mnd
	cb.setClock(func() time.Time { return fakeNow })

	// 100 goroutines call allow() concurrently. Exactly 1 should get true
	// (the probe); the rest should get false.
	const goroutines = 100
	var allowed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if cb.allow() {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != 1 {
		t.Errorf("concurrent allow() in half-open: %d allowed, want exactly 1 (single probe)", got)
	}
}

// TestCircuitBreakerClockInjection verifies that the clock function is used
// for cooldown timing, allowing deterministic tests without real sleeps.
func TestCircuitBreakerClockInjection(t *testing.T) {
	cb := &circuitBreaker{
		maxFails: 1,
		cooldown: 1 * time.Hour,
	}

	// Trip the breaker.
	cb.recordFailure()
	if cb.allow() {
		t.Error("breaker should be open immediately after trip")
	}

	// Advance fake clock past cooldown.
	baseTime := time.Now()
	t1 := baseTime.Add(30 * time.Minute) //nolint:mnd // still within cooldown
	cb.setClock(func() time.Time { return t1 })
	if cb.allow() {
		t.Error("breaker should still be open 30min into 1h cooldown")
	}

	t2 := baseTime.Add(2 * time.Hour) // past cooldown
	cb.setClock(func() time.Time { return t2 })
	if !cb.allow() {
		t.Error("breaker should allow probe after cooldown elapsed")
	}
}

// TestCircuitBreakerHalfOpenProbeSuccess verifies that a successful probe
// closes the circuit, allowing subsequent requests.
func TestCircuitBreakerHalfOpenProbeSuccess(t *testing.T) {
	cb := &circuitBreaker{
		maxFails: 1,
		cooldown: 1 * time.Second,
	}

	cb.recordFailure() // trip

	// Advance past cooldown.
	cb.setClock(func() time.Time { return time.Now().Add(2 * time.Second) })

	if !cb.allow() {
		t.Fatal("first allow() after cooldown should return true (probe)")
	}
	if cb.allow() {
		t.Error("second allow() while probe in flight should return false")
	}

	cb.recordSuccess() // probe succeeded

	if !cb.allow() {
		t.Error("after successful probe, breaker should be closed and allow all")
	}
}

// TestCircuitBreakerHalfOpenProbeFailure verifies that a failed probe
// re-opens the circuit with a fresh cooldown.
func TestCircuitBreakerHalfOpenProbeFailure(t *testing.T) {
	cb := &circuitBreaker{
		maxFails: 1,
		cooldown: 1 * time.Second,
	}

	cb.recordFailure() // trip

	baseTime := time.Now()
	cb.setClock(func() time.Time { return baseTime.Add(2 * time.Second) })

	if !cb.allow() {
		t.Fatal("probe should be allowed after cooldown")
	}

	cb.recordFailure() // probe failed

	// Should be open again with fresh cooldown.
	cb.setClock(func() time.Time { return baseTime.Add(2 * time.Second) }) // still in new cooldown
	if cb.allow() {
		t.Error("breaker should be re-opened after failed probe")
	}

	// Advance past new cooldown.
	cb.setClock(func() time.Time { return baseTime.Add(4 * time.Second) })
	if !cb.allow() {
		t.Error("breaker should allow new probe after second cooldown")
	}
}

// TestDoWithRetryCircuitBreakerBlocks verifies that doWithRetry respects
// the circuit breaker and stops retrying when the breaker opens.
func TestDoWithRetryCircuitBreakerBlocks(t *testing.T) {
	cb := &circuitBreaker{
		maxFails: 2, //nolint:mnd
		cooldown: 1 * time.Hour,
	}
	rc := &retryConfig{
		maxAttempts: 10, //nolint:mnd
		baseDelay:   1 * time.Millisecond,
		maxDelay:    10 * time.Millisecond,
	}

	var calls atomic.Int32
	fn := func() (string, error) {
		calls.Add(1)
		return "", &Error{StatusCode: http.StatusServiceUnavailable, Message: "unavailable"}
	}

	_, err := doWithRetry(context.Background(), rc, cb, fn)
	if err == nil {
		t.Fatal("expected error")
	}

	// With maxFails=2, the breaker opens after 2 failures. doWithRetry
	// checks the breaker before each attempt, so it should stop after
	// 2 calls (not 10).
	if got := calls.Load(); got > 2 {
		t.Errorf("calls = %d, want <= 2 (breaker should stop retries)", got)
	}
}
