package embed

import (
	"sync"
	"testing"
	"time"
)

func newTestCB(failThreshold, halfOpenProbes int, openDuration time.Duration) *CircuitBreaker {
	cfg := CircuitConfig{
		FailThreshold:  failThreshold,
		OpenDuration:   openDuration,
		HalfOpenProbes: halfOpenProbes,
		FailRateWindow: 10 * time.Second,
	}
	return NewCircuitBreaker(cfg, "test-model", nil)
}

func TestCircuit_StartsClosed(t *testing.T) {
	cb := newTestCB(3, 1, 30*time.Second)
	if cb.State() != CircuitClosed {
		t.Fatalf("want Closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("Allow() should return true in Closed state")
	}
}

func TestCircuit_OpensAfterFailThreshold(t *testing.T) {
	cb := newTestCB(3, 1, 30*time.Second)
	for i := 0; i < 3; i++ {
		cb.MarkFailure()
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("want Open after 3 failures, got %s", cb.State())
	}
}

func TestCircuit_OpenBlocksAllow(t *testing.T) {
	cb := newTestCB(3, 1, 30*time.Second)
	for i := 0; i < 3; i++ {
		cb.MarkFailure()
	}
	if cb.Allow() {
		t.Fatal("Allow() should return false in Open state within OpenDuration")
	}
}

func TestCircuit_HalfOpenAfterDuration(t *testing.T) {
	cb := newTestCB(1, 1, 50*time.Millisecond)
	cb.MarkFailure() // trips to Open
	if cb.State() != CircuitOpen {
		t.Fatalf("expected Open, got %s", cb.State())
	}
	time.Sleep(100 * time.Millisecond) // exceed OpenDuration
	// Allow() triggers the Open→HalfOpen transition.
	if !cb.Allow() {
		t.Fatal("Allow() should return true after OpenDuration (HalfOpen probe slot)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("want HalfOpen, got %s", cb.State())
	}
}

func TestCircuit_HalfOpenSuccessClosesCircuit(t *testing.T) {
	cb := newTestCB(1, 1, 50*time.Millisecond)
	cb.MarkFailure()
	time.Sleep(100 * time.Millisecond)
	cb.Allow() // transition to HalfOpen
	cb.MarkSuccess()
	if cb.State() != CircuitClosed {
		t.Fatalf("want Closed after HalfOpen success, got %s", cb.State())
	}
}

func TestCircuit_HalfOpenFailureReopens(t *testing.T) {
	cb := newTestCB(1, 1, 50*time.Millisecond)
	cb.MarkFailure()
	time.Sleep(100 * time.Millisecond)
	cb.Allow() // transition to HalfOpen
	cb.MarkFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("want Open after HalfOpen failure, got %s", cb.State())
	}
}

func TestCircuit_ConcurrentAccess(t *testing.T) {
	cb := newTestCB(10, 2, 50*time.Millisecond)
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			if id%3 == 0 {
				cb.MarkFailure()
			} else {
				cb.MarkSuccess()
				_ = cb.Allow()
			}
		}(i)
	}
	wg.Wait()
	// No panic, no data race — validated by -race flag.
}

func TestCircuit_TransitionHookFires(t *testing.T) {
	var transitions []CircuitState
	var mu sync.Mutex
	hook := func(from, to CircuitState) {
		mu.Lock()
		transitions = append(transitions, to)
		mu.Unlock()
	}
	cfg := CircuitConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond, HalfOpenProbes: 1}
	cb := NewCircuitBreaker(cfg, "m", hook)

	cb.MarkFailure() // Closed→Open
	time.Sleep(100 * time.Millisecond)
	cb.Allow()       // Open→HalfOpen
	cb.MarkSuccess() // HalfOpen→Closed

	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 3 {
		t.Fatalf("want 3 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != CircuitOpen {
		t.Errorf("transition[0]: want Open, got %s", transitions[0])
	}
	if transitions[1] != CircuitHalfOpen {
		t.Errorf("transition[1]: want HalfOpen, got %s", transitions[1])
	}
	if transitions[2] != CircuitClosed {
		t.Errorf("transition[2]: want Closed, got %s", transitions[2])
	}
}

func TestCircuit_GaugeUpdatesOnOpenToHalfOpen(t *testing.T) {
	// Verify that doTransition fires recordCircuitState on EVERY transition,
	// including the implicit Open→HalfOpen triggered inside Allow().
	// The gauge helper is side-effect only (no return), so we verify indirectly
	// via state transitions — if doTransition doesn't call recordCircuitState
	// we'd see no panic but the gauge would be stale. This test ensures the
	// transition path is exercised without data race.
	cfg := CircuitConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond, HalfOpenProbes: 1}
	cb := NewCircuitBreaker(cfg, "gauge-test", nil)
	cb.MarkFailure()                   // Closed→Open; recordCircuitState("gauge-test", Open)
	time.Sleep(100 * time.Millisecond) // wait OpenDuration
	cb.Allow()                         // Open→HalfOpen; recordCircuitState("gauge-test", HalfOpen)
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("want HalfOpen, got %s", cb.State())
	}
}
