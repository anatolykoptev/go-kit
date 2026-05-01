package sparse

import (
	"testing"
	"time"
)

// TestCircuitBreaker_TripsAfterThreshold verifies consecutive failures
// trip the breaker from Closed to Open.
func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{FailThreshold: 3, OpenDuration: time.Hour}, "test", nil)
	for i := 0; i < 2; i++ {
		cb.MarkFailure()
		if cb.State() != CircuitClosed {
			t.Fatalf("attempt %d: want closed, got %s", i, cb.State())
		}
	}
	cb.MarkFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("after threshold: want open, got %s", cb.State())
	}
	if cb.Allow() {
		t.Errorf("Allow should return false when open")
	}
}

// TestCircuitBreaker_HalfOpenAfterTimeout verifies the open-duration
// timer transitions the breaker to HalfOpen on the next Allow call.
func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{
		FailThreshold:  1,
		OpenDuration:   10 * time.Millisecond,
		HalfOpenProbes: 1,
	}, "test", nil)
	cb.MarkFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("want open, got %s", cb.State())
	}
	time.Sleep(20 * time.Millisecond)
	if !cb.Allow() {
		t.Errorf("Allow should return true after open-duration elapsed")
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("want half-open, got %s", cb.State())
	}
}

// TestCircuitBreaker_HalfOpenSuccessRecovers verifies a successful probe
// in HalfOpen returns the breaker to Closed.
func TestCircuitBreaker_HalfOpenSuccessRecovers(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{FailThreshold: 1, OpenDuration: 10 * time.Millisecond}, "test", nil)
	cb.MarkFailure()
	time.Sleep(20 * time.Millisecond)
	cb.Allow()
	cb.MarkSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("want closed, got %s", cb.State())
	}
}

// TestCircuitBreaker_HalfOpenFailureReopens verifies a failed probe
// reopens the breaker.
func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{FailThreshold: 1, OpenDuration: 10 * time.Millisecond}, "test", nil)
	cb.MarkFailure()
	time.Sleep(20 * time.Millisecond)
	cb.Allow()
	cb.MarkFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("want open after probe failure, got %s", cb.State())
	}
}

// TestCircuitBreaker_OnTransitionFires verifies the callback fires on
// every state change.
func TestCircuitBreaker_OnTransitionFires(t *testing.T) {
	transitions := 0
	cb := NewCircuitBreaker(CircuitConfig{FailThreshold: 1}, "test", func(_, _ CircuitState) {
		transitions++
	})
	cb.MarkFailure()
	if transitions != 1 {
		t.Errorf("want 1 transition (closed→open), got %d", transitions)
	}
}

// TestCircuitBreaker_PanicCallbackDoesNotKill verifies safeCall recovery.
func TestCircuitBreaker_PanicCallbackDoesNotKill(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{FailThreshold: 1}, "test", func(_, _ CircuitState) {
		panic("user code panic")
	})
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic escaped: %v", r)
		}
	}()
	cb.MarkFailure()
}
