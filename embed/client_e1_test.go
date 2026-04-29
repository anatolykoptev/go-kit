package embed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newClientWithStub builds a *Client wrapping a stub Embedder with given retry policy.
func newClientWithStub(model string, retry RetryPolicy, fn func(context.Context, []string) ([][]float32, error)) *Client {
	return &Client{
		inner:    &stubEmbedder{embedFn: fn, model: model},
		observer: noopObserver{},
		model:    model,
		retry:    retry,
	}
}

// TestClient_RetryOn5xx verifies that EmbedWithResult retries on 503 and succeeds.
func TestClient_RetryOn5xx(t *testing.T) {
	attempts := 0
	p := RetryPolicy{MaxAttempts: 3, BaseBackoff: 0, RetryableStatus: []int{503}}

	c := newClientWithStub("m", p, func(_ context.Context, texts []string) ([][]float32, error) {
		attempts++
		if attempts < 2 {
			return nil, &errHTTPStatus{Code: 503}
		}
		return okVecs(len(texts)), nil
	})

	res, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("want StatusOk, got %s", res.Status)
	}
	if attempts != 2 {
		t.Errorf("want 2 attempts, got %d", attempts)
	}
}

// TestClient_NoRetryOn4xx verifies that 4xx errors are returned immediately without retry.
func TestClient_NoRetryOn4xx(t *testing.T) {
	attempts := 0
	p := RetryPolicy{MaxAttempts: 5, BaseBackoff: 0, RetryableStatus: []int{503}}

	c := newClientWithStub("m", p, func(_ context.Context, _ []string) ([][]float32, error) {
		attempts++
		return nil, &errHTTPStatus{Code: 400}
	})

	res, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error for 4xx")
	}
	if res.Status != StatusDegraded {
		t.Errorf("want StatusDegraded, got %s", res.Status)
	}
	if attempts != 1 {
		t.Errorf("4xx should not retry: want 1 attempt, got %d", attempts)
	}
}

// TestClient_CircuitOpenSkipsCall verifies that a tripped circuit breaker blocks calls.
func TestClient_CircuitOpenSkipsCall(t *testing.T) {
	backendCalls := 0

	inner := &stubEmbedder{
		model: "m",
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			backendCalls++
			return nil, errors.New("embed: failure")
		},
	}
	cb := NewCircuitBreaker(
		CircuitConfig{FailThreshold: 1, OpenDuration: 30 * time.Second, HalfOpenProbes: 1},
		"m", nil,
	)
	c := &Client{
		inner:    inner,
		observer: noopObserver{},
		model:    "m",
		retry:    NoRetry,
		circuit:  cb,
	}

	// First call fails → trips circuit.
	res, _ := c.EmbedWithResult(context.Background(), []string{"hello"})
	if res.Status != StatusDegraded {
		t.Fatalf("want StatusDegraded on first failure, got %s", res.Status)
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("circuit should be Open, got %s", cb.State())
	}
	firstCallCount := backendCalls

	// Second call should be blocked by the open circuit.
	res2, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("want ErrCircuitOpen, got %v", err)
	}
	if res2.Status != StatusDegraded {
		t.Errorf("want StatusDegraded on circuit open, got %s", res2.Status)
	}
	// Backend should NOT have been called again.
	if backendCalls != firstCallCount {
		t.Errorf("backend should not be called when circuit is open; got %d calls", backendCalls)
	}
}

// TestClient_FallbackChain verifies that a degraded primary routes to secondary.
func TestClient_FallbackChain(t *testing.T) {
	secondaryCalls := 0

	primary := newClientWithStub("primary", NoRetry, func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, errors.New("embed: primary timeout")
	})
	secondary := newClientWithStub("secondary", NoRetry, func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalls++
		return okVecs(len(texts)), nil
	})
	primary.fallback = secondary

	res, err := primary.EmbedWithResult(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected success via fallback, got: %v", err)
	}
	if res.Status != StatusFallback {
		t.Errorf("want StatusFallback, got %s", res.Status)
	}
	if secondaryCalls != 1 {
		t.Errorf("want 1 secondary call, got %d", secondaryCalls)
	}
}

// TestClient_RetryEmitsObserverHook verifies that OnRetry fires for each retry attempt.
func TestClient_RetryEmitsObserverHook(t *testing.T) {
	var hookFired []int
	obs := &retryCapturingObserver{
		onRetry: func(attempt int) { hookFired = append(hookFired, attempt) },
	}

	p := RetryPolicy{MaxAttempts: 3, BaseBackoff: 0, RetryableStatus: []int{503}}
	attempts := 0
	c := &Client{
		inner: &stubEmbedder{
			model: "m",
			embedFn: func(_ context.Context, texts []string) ([][]float32, error) {
				attempts++
				if attempts < 3 {
					return nil, &errHTTPStatus{Code: 503}
				}
				return okVecs(len(texts)), nil
			},
		},
		observer: obs,
		model:    "m",
		retry:    p,
	}

	res, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("expected success on 3rd attempt, got: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("want StatusOk, got %s", res.Status)
	}
	// 2 retries → 2 OnRetry hook calls.
	if len(hookFired) != 2 {
		t.Errorf("want 2 OnRetry calls, got %d: %v", len(hookFired), hookFired)
	}
}

// TestClient_CircuitTransitionEmitsObserverHook verifies OnCircuitTransition fires.
func TestClient_CircuitTransitionEmitsObserverHook(t *testing.T) {
	var transitions []CircuitState
	obs := &circuitCapturingObserver{
		onTransition: func(from, to CircuitState) {
			transitions = append(transitions, to)
		},
	}

	model := "m"
	cbCfg := CircuitConfig{FailThreshold: 1, OpenDuration: 50 * time.Millisecond, HalfOpenProbes: 1}
	cb := NewCircuitBreaker(cbCfg, model, makeCircuitHook(model, obs))

	c := &Client{
		inner: &stubEmbedder{
			model: model,
			embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
				return nil, fmt.Errorf("fail")
			},
		},
		observer: obs,
		model:    model,
		retry:    NoRetry,
		circuit:  cb,
	}

	// First call trips circuit: Closed→Open.
	c.EmbedWithResult(context.Background(), []string{"hello"}) //nolint:errcheck

	if len(transitions) == 0 {
		t.Fatal("expected OnCircuitTransition to fire on Closed→Open")
	}
	if transitions[0] != CircuitOpen {
		t.Errorf("want first transition to Open, got %s", transitions[0])
	}
}

// circuitCapturingObserver captures OnCircuitTransition calls.
type circuitCapturingObserver struct {
	noopObserver
	onTransition func(from, to CircuitState)
}

func (o *circuitCapturingObserver) OnCircuitTransition(_ context.Context, from, to CircuitState) {
	if o.onTransition != nil {
		o.onTransition(from, to)
	}
}

// TestClient_NoRetryOn4xx_TypedError verifies the 4xx-no-retry guard works
// END-TO-END through the real HTTPEmbedder backend (not via direct errHTTPStatus
// injection).
//
// Pre-fix this test would have FAILED: backends returned string-formatted errors,
// retry.do() type-assert missed, every 4xx caused 3 retry attempts instead of 1.
func TestClient_NoRetryOn4xx_TypedError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest) // 400
		w.Write([]byte(`{"error":"bad request"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel("test"),
		WithDim(8),
		WithRetry(RetryPolicy{
			MaxAttempts:     3,
			BaseBackoff:     1 * time.Millisecond,
			RetryableStatus: []int{500, 502, 503, 504},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	res, _ := c.EmbedWithResult(context.Background(), []string{"hello"})
	if res.Status != StatusDegraded {
		t.Errorf("status: got %v, want StatusDegraded", res.Status)
	}
	// CRITICAL: 4xx must trigger ONE attempt only — no retries.
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts: got %d, want 1 (4xx must not retry)", got)
	}
}

// TestClient_NoFallbackOn4xx_TypedError verifies that a 400 from the primary
// backend does NOT trigger the fallback — secondary must not be called.
//
// Pre-fix: isClientError() used a naked type-assert on the unboxed error value,
// which never matched strings returned by backends — every 4xx triggered fallback.
func TestClient_NoFallbackOn4xx_TypedError(t *testing.T) {
	var secondaryCalls atomic.Int32

	// Primary returns 400 via httptest server (real HTTPEmbedder path).
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`)) //nolint:errcheck
	}))
	defer primarySrv.Close()

	primary, err := NewClient(primarySrv.URL,
		WithBackend("http"),
		WithModel("primary"),
		WithDim(8),
		WithRetry(NoRetry),
	)
	if err != nil {
		t.Fatal(err)
	}

	secondary := newClientWithStub("secondary", NoRetry, func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalls.Add(1)
		return okVecs(len(texts)), nil
	})
	primary.fallback = secondary

	res, _ := primary.EmbedWithResult(context.Background(), []string{"hello"})

	if res.Status != StatusDegraded {
		t.Errorf("status: got %v, want StatusDegraded", res.Status)
	}
	// Secondary MUST NOT be called when primary returns 4xx.
	if got := secondaryCalls.Load(); got != 0 {
		t.Errorf("secondary calls: got %d, want 0 (4xx must not trigger fallback)", got)
	}
}
