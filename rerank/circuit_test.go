package rerank

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestCircuit(threshold int, openDur time.Duration) *CircuitBreaker {
	return NewCircuitBreaker(CircuitConfig{
		FailThreshold:  threshold,
		OpenDuration:   openDur,
		HalfOpenProbes: 1,
		FailRateWindow: 10 * time.Second,
	}, nil)
}

// TestCircuit_StartsClosed verifies the initial state is Closed.
func TestCircuit_StartsClosed(t *testing.T) {
	cb := newTestCircuit(5, 30*time.Second)
	if cb.State() != CircuitClosed {
		t.Errorf("initial state: got %v want CircuitClosed", cb.State())
	}
	if !cb.Allow() {
		t.Error("Closed circuit must allow requests")
	}
}

// TestCircuit_OpensAfterFailThreshold verifies Closed→Open transition.
func TestCircuit_OpensAfterFailThreshold(t *testing.T) {
	cb := newTestCircuit(5, 30*time.Second)
	for i := 0; i < 4; i++ {
		cb.MarkFailure()
		if cb.State() != CircuitClosed {
			t.Errorf("after %d failures: expected Closed, got %v", i+1, cb.State())
		}
	}
	cb.MarkFailure() // 5th failure trips the breaker.
	if cb.State() != CircuitOpen {
		t.Errorf("after threshold failures: got %v want CircuitOpen", cb.State())
	}
}

// TestCircuit_OpenBlocksAllow verifies that Open state blocks requests.
func TestCircuit_OpenBlocksAllow(t *testing.T) {
	cb := newTestCircuit(1, 30*time.Second) // trips on first failure
	cb.MarkFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected Open, got %v", cb.State())
	}
	if cb.Allow() {
		t.Error("Open circuit must block requests")
	}
}

// TestCircuit_HalfOpenAfterDuration verifies Open→HalfOpen after OpenDuration.
func TestCircuit_HalfOpenAfterDuration(t *testing.T) {
	cb := newTestCircuit(1, 20*time.Millisecond)
	cb.MarkFailure() // → Open
	if cb.Allow() {
		t.Error("should block immediately after opening")
	}
	time.Sleep(30 * time.Millisecond) // wait past OpenDuration
	if !cb.Allow() {
		t.Error("should allow probe after OpenDuration elapsed")
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("state: got %v want CircuitHalfOpen", cb.State())
	}
}

// TestCircuit_HalfOpenSuccessClosesCircuit verifies HalfOpen→Closed on MarkSuccess.
func TestCircuit_HalfOpenSuccessClosesCircuit(t *testing.T) {
	cb := newTestCircuit(1, 20*time.Millisecond)
	cb.MarkFailure()                  // → Open
	time.Sleep(30 * time.Millisecond) // wait for OpenDuration
	cb.Allow()                        // transitions to HalfOpen
	cb.MarkSuccess()                  // → Closed
	if cb.State() != CircuitClosed {
		t.Errorf("after success in HalfOpen: got %v want CircuitClosed", cb.State())
	}
	if !cb.Allow() {
		t.Error("Closed circuit must allow requests after recovery")
	}
}

// TestCircuit_HalfOpenFailureReopens verifies HalfOpen→Open on MarkFailure.
func TestCircuit_HalfOpenFailureReopens(t *testing.T) {
	cb := newTestCircuit(1, 20*time.Millisecond)
	cb.MarkFailure()                  // → Open
	time.Sleep(30 * time.Millisecond) // wait for OpenDuration
	cb.Allow()                        // → HalfOpen
	cb.MarkFailure()                  // probe failed → Open again
	if cb.State() != CircuitOpen {
		t.Errorf("after failure in HalfOpen: got %v want CircuitOpen", cb.State())
	}
	// New openedAt should be set — circuit should block again immediately.
	if cb.Allow() {
		t.Error("should block after HalfOpen failure")
	}
}

// TestCircuit_ConcurrentAccess verifies thread safety under parallel use.
// Run with -race to detect data races.
func TestCircuit_ConcurrentAccess(t *testing.T) {
	cb := newTestCircuit(10, 10*time.Millisecond)
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if cb.Allow() {
					if id%3 == 0 {
						cb.MarkFailure()
					} else {
						cb.MarkSuccess()
					}
				}
				_ = cb.State()
			}
		}(i)
	}
	wg.Wait()
	// No assertion on final state — the test validates race-freedom.
}

// TestCircuit_TransitionHookFires verifies the onTransition callback is invoked.
func TestCircuit_TransitionHookFires(t *testing.T) {
	type transition struct{ from, to CircuitState }
	var mu sync.Mutex
	var transitions []transition

	cb := NewCircuitBreaker(CircuitConfig{
		FailThreshold:  2,
		OpenDuration:   20 * time.Millisecond,
		HalfOpenProbes: 1,
	}, func(from, to CircuitState) {
		mu.Lock()
		transitions = append(transitions, transition{from, to})
		mu.Unlock()
	})

	// Closed → Open (2 failures)
	cb.MarkFailure()
	cb.MarkFailure()

	time.Sleep(30 * time.Millisecond)
	cb.Allow() // Open → HalfOpen

	cb.MarkSuccess() // HalfOpen → Closed

	mu.Lock()
	got := transitions
	mu.Unlock()

	wantTransitions := []transition{
		{CircuitClosed, CircuitOpen},
		{CircuitOpen, CircuitHalfOpen},
		{CircuitHalfOpen, CircuitClosed},
	}
	if len(got) != len(wantTransitions) {
		t.Fatalf("transitions: got %d want %d: %v", len(got), len(wantTransitions), got)
	}
	for i, tr := range got {
		if tr != wantTransitions[i] {
			t.Errorf("transition[%d]: got %v want %v", i, tr, wantTransitions[i])
		}
	}
}

// TestCircuit_SuccessResetsFails verifies consecutive fail counter resets on success.
func TestCircuit_SuccessResetsFails(t *testing.T) {
	cb := newTestCircuit(5, 30*time.Second)
	// Fail 4 times (just under threshold).
	for i := 0; i < 4; i++ {
		cb.MarkFailure()
	}
	cb.MarkSuccess() // reset
	// 4 more failures should not open the circuit (counter reset).
	for i := 0; i < 4; i++ {
		cb.MarkFailure()
	}
	if cb.State() != CircuitClosed {
		t.Errorf("state: got %v want Closed (counter was reset)", cb.State())
	}
}

// TestCircuitState_String verifies String() labels for metric use.
func TestCircuitState_String(t *testing.T) {
	cases := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("%v.String(): got %q want %q", tc.state, got, tc.want)
		}
	}
}

// TestCircuit_GaugeUpdatesOnOpenToHalfOpen verifies that the circuit state gauge
// is updated when Allow() drives the implicit Open→HalfOpen transition.
// Before this fix, recordCircuitState was only called from MarkSuccess/MarkFailure,
// so the gauge stayed at state="open" even after the breaker moved to HalfOpen.
func TestCircuit_GaugeUpdatesOnOpenToHalfOpen(t *testing.T) {
	const model = "gauge-test-model"
	cb := NewCircuitBreaker(CircuitConfig{
		FailThreshold:  1,
		OpenDuration:   20 * time.Millisecond,
		HalfOpenProbes: 1,
	}, nil)
	cb.model = model

	cb.MarkFailure() // Closed → Open; gauge should reflect "open"=1

	openGauge := testutil.ToFloat64(rerankCircuitStateGauge.WithLabelValues(model, "open"))
	if openGauge != 1 {
		t.Errorf("after MarkFailure: state=open gauge = %.0f, want 1", openGauge)
	}

	time.Sleep(30 * time.Millisecond) // wait past OpenDuration

	// Allow() triggers the implicit Open→HalfOpen transition.
	allowed := cb.Allow()
	if !allowed {
		t.Fatal("Allow() should return true after OpenDuration elapsed")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("state after Allow(): got %v want CircuitHalfOpen", cb.State())
	}

	// Gauge must now reflect half-open=1, open=0.
	halfOpenGauge := testutil.ToFloat64(rerankCircuitStateGauge.WithLabelValues(model, "half-open"))
	if halfOpenGauge != 1 {
		t.Errorf("after Allow() transition: state=half-open gauge = %.0f, want 1", halfOpenGauge)
	}
	openGauge = testutil.ToFloat64(rerankCircuitStateGauge.WithLabelValues(model, "open"))
	if openGauge != 0 {
		t.Errorf("after Allow() transition: state=open gauge = %.0f, want 0", openGauge)
	}
}
