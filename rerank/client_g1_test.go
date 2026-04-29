package rerank

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// TestClient_RetryOn5xx verifies that the client retries on 503 and succeeds
// on the third attempt.
func TestClient_RetryOn5xx(t *testing.T) {
	calls := atomic.Int32{}
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Third call succeeds.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Results: []cohereResult{
				{Index: 1, RelevanceScore: 0.9},
				{Index: 0, RelevanceScore: 0.3},
			},
		})
	})

	c := NewClient(srv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(RetryPolicy{
			MaxAttempts:     3,
			BaseBackoff:     0, // no sleep for fast tests
			RetryableStatus: []int{503},
		}),
	)

	docs := []Doc{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error after retry success: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status: got %v want StatusOk", res.Status)
	}
	// After retry success, scores should be reordered: b (0.9) > a (0.3).
	if len(res.Scored) != 2 || res.Scored[0].ID != "b" {
		t.Errorf("reranked order wrong: %v", res.Scored)
	}
	if calls.Load() != 3 {
		t.Errorf("server calls: got %d want 3", calls.Load())
	}
}

// TestClient_NoRetryOn4xx verifies that 4xx errors return immediately with
// no retry.
func TestClient_NoRetryOn4xx(t *testing.T) {
	calls := atomic.Int32{}
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	})

	c := NewClient(srv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(RetryPolicy{
			MaxAttempts:     3,
			BaseBackoff:     0,
			RetryableStatus: []int{500, 503},
		}),
	)

	docs := []Doc{{ID: "a"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err == nil {
		t.Error("expected non-nil error on 400")
	}
	if res == nil || res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded", res.Status)
	}
	if calls.Load() != 1 {
		t.Errorf("server calls: got %d want 1 (no retry on 4xx)", calls.Load())
	}
}

// TestClient_CircuitOpenSkipsCall verifies that a pre-opened circuit returns
// ErrCircuitOpen without calling the server.
func TestClient_CircuitOpenSkipsCall(t *testing.T) {
	serverCalls := atomic.Int32{}
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		serverCalls.Add(1)
		v1JSONResp(w, cohereResponse{})
	})

	c := NewClient(srv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(NoRetry),
		WithCircuit(CircuitConfig{
			FailThreshold:  1, // opens after 1 failure
			OpenDuration:   30 * time.Second,
			HalfOpenProbes: 1,
		}),
	)

	// Manually open the circuit by marking a failure.
	c.cfg.circuit.MarkFailure()
	if c.cfg.circuit.State() != CircuitOpen {
		t.Fatal("circuit should be Open after MarkFailure")
	}

	docs := []Doc{{ID: "a"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err: got %v want ErrCircuitOpen", err)
	}
	if res == nil || res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded", res.Status)
	}
	if serverCalls.Load() != 0 {
		t.Errorf("server should never be called when circuit is open, got %d calls", serverCalls.Load())
	}
}

// TestClient_HookCallbacksFire verifies OnBeforeCall, OnRetry, OnAfterCall, and
// OnCircuitTransition fire in the expected order during a 503-retry-success scenario.
func TestClient_HookCallbacksFire(t *testing.T) {
	type event struct{ name string }
	var events []event
	obs := &hookRecorder{
		onBefore: func() { events = append(events, event{"before"}) },
		onRetry:  func(attempt int) { events = append(events, event{"retry"}) },
		onAfter:  func(status Status) { events = append(events, event{"after_" + status.String()}) },
	}

	calls := atomic.Int32{}
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		v1JSONResp(w, cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.8}},
		})
	})

	c := NewClient(srv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(RetryPolicy{
			MaxAttempts:     3,
			BaseBackoff:     0,
			RetryableStatus: []int{503},
		}),
		WithObserver(obs),
	)

	docs := []Doc{{ID: "a"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status: got %v want Ok", res.Status)
	}

	// Expected sequence: before → retry(1) → after_ok
	wantLen := 3
	if len(events) != wantLen {
		t.Errorf("events: got %v want %d events", events, wantLen)
	}
	if len(events) >= 1 && events[0].name != "before" {
		t.Errorf("events[0]: got %q want %q", events[0].name, "before")
	}
	if len(events) >= 2 && events[1].name != "retry" {
		t.Errorf("events[1]: got %q want %q", events[1].name, "retry")
	}
	if len(events) >= 3 && events[2].name != "after_ok" {
		t.Errorf("events[2]: got %q want %q", events[2].name, "after_ok")
	}
}

// hookRecorder is a minimal Observer for testing callback order.
type hookRecorder struct {
	noopObserver
	onBefore func()
	onRetry  func(attempt int)
	onAfter  func(status Status)
	onTrans  func(from, to CircuitState)
}

func (h *hookRecorder) OnBeforeCall(_ context.Context, _ string, _ int) {
	if h.onBefore != nil {
		h.onBefore()
	}
}
func (h *hookRecorder) OnRetry(_ context.Context, attempt int, _ error) {
	if h.onRetry != nil {
		h.onRetry(attempt)
	}
}
func (h *hookRecorder) OnAfterCall(_ context.Context, status Status, _ time.Duration, _ int) {
	if h.onAfter != nil {
		h.onAfter(status)
	}
}
func (h *hookRecorder) OnCircuitTransition(_ context.Context, from, to CircuitState) {
	if h.onTrans != nil {
		h.onTrans(from, to)
	}
}

// TestClient_FallbackChain verifies end-to-end fallback via NewClient + WithFallback.
func TestClient_FallbackChain(t *testing.T) {
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	secondarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		v1JSONResp(w, cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.6}},
		})
	})

	secondary := NewClient(secondarySrv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(NoRetry),
	)
	primary := NewClient(primarySrv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(NoRetry),
		WithFallback(secondary),
	)

	docs := []Doc{{ID: "a"}}
	res, err := primary.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error with fallback: %v", err)
	}
	if res.Status != StatusFallback {
		t.Errorf("Status: got %v want StatusFallback", res.Status)
	}
}

// TestClient_WithRetryNoRetry verifies NoRetry sentinel prevents any retry.
func TestClient_WithRetryNoRetry(t *testing.T) {
	calls := atomic.Int32{}
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := NewClient(srv.URL,
		WithTimeout(500*time.Millisecond),
		WithRetry(NoRetry),
	)
	_, _ = c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a"}})
	if calls.Load() != 1 {
		t.Errorf("calls: got %d want 1 (NoRetry)", calls.Load())
	}
}
