package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestRetryConfig_ExportedFields verifies external callers can construct
// a custom RetryConfig with exported fields and pass it via WithRetry.
// Guards against the v0 mistake where retryConfig was unexported and
// only NoRetry could be customised.
func TestRetryConfig_ExportedFields(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      0.1,
	}
	c, err := NewClient("http://localhost", WithRetry(cfg))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.retry.MaxAttempts != 5 {
		t.Errorf("MaxAttempts: want 5, got %d", c.retry.MaxAttempts)
	}
	if c.retry.Jitter != 0.1 {
		t.Errorf("Jitter: want 0.1, got %v", c.retry.Jitter)
	}
}

// TestNoRetry_StaysSingleAttempt verifies the NoRetry sentinel still does
// what its docs claim after the rename: MaxAttempts=1 means one call,
// no retry.
func TestNoRetry_StaysSingleAttempt(t *testing.T) {
	if NoRetry.MaxAttempts != 1 {
		t.Errorf("NoRetry.MaxAttempts: want 1, got %d", NoRetry.MaxAttempts)
	}
}

// TestApplyJitter_DeterministicWhenZero asserts Jitter=0 returns the input
// unchanged (no randomness).
func TestApplyJitter_DeterministicWhenZero(t *testing.T) {
	d := 200 * time.Millisecond
	for i := 0; i < 10; i++ {
		if got := applyJitter(d, 0); got != d {
			t.Fatalf("jitter=0 should be deterministic; got %v != %v", got, d)
		}
	}
}

// TestApplyJitter_AddsRandomness verifies jitter > 0 produces a range of
// durations across repeated calls AND every result lies within
// [base, base*(1+jitter)). math/rand/v2 is auto-seeded so we cannot pin
// exact values; instead we assert the sequence is not constant.
func TestApplyJitter_AddsRandomness(t *testing.T) {
	base := 200 * time.Millisecond
	jitter := 0.1
	maxAllowed := time.Duration(float64(base) * (1 + jitter))

	seen := make(map[time.Duration]struct{}, 50)
	for i := 0; i < 50; i++ {
		got := applyJitter(base, jitter)
		if got < base {
			t.Fatalf("jitter result below base: %v < %v", got, base)
		}
		if got > maxAllowed {
			t.Fatalf("jitter result above cap: %v > %v", got, maxAllowed)
		}
		seen[got] = struct{}{}
	}
	if len(seen) < 5 {
		t.Errorf("expected jitter to produce varied delays across 50 calls; only %d unique", len(seen))
	}
}

// TestWithRetry_FiresObserverOnRetry verifies the OnRetry hook is invoked
// once per retried failure (NOT for the initial attempt) and the
// retry_attempt_total metric increments correspondingly. Mirrors
// embed/'s retry observability pattern.
func TestWithRetry_FiresObserverOnRetry(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1}, Values: []float32{0.5}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	spy := &retrySpy{}
	// Snapshot the per-attempt counter before the test so concurrent
	// tests can't poison the assertion.
	before2 := testutil.ToFloat64(sparseRetryAttemptTotal.WithLabelValues("http", "2"))
	before3 := testutil.ToFloat64(sparseRetryAttemptTotal.WithLabelValues("http", "3"))

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger(), WithHTTPObserver(spy))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := e.EmbedSparse(ctx, []string{"x"}); err != nil {
		t.Fatalf("want success after retries: %v", err)
	}

	// Two retries fire (attempts 2 and 3), so OnRetry must be called twice.
	if got := atomic.LoadInt32(&spy.onRetry); got != 2 {
		t.Errorf("OnRetry calls: want 2, got %d", got)
	}
	after2 := testutil.ToFloat64(sparseRetryAttemptTotal.WithLabelValues("http", "2"))
	after3 := testutil.ToFloat64(sparseRetryAttemptTotal.WithLabelValues("http", "3"))
	if after2-before2 < 1 {
		t.Errorf("retry_attempt_total{attempt=2} did not increment: before=%v after=%v", before2, after2)
	}
	if after3-before3 < 1 {
		t.Errorf("retry_attempt_total{attempt=3} did not increment: before=%v after=%v", before3, after3)
	}
}

// retrySpy captures only the OnRetry hook for the test above.
type retrySpy struct {
	onRetry int32
}

func (r *retrySpy) OnBeforeEmbed(_ context.Context, _ string, _ int) {}
func (r *retrySpy) OnAfterEmbed(_ context.Context, _ Status, _ time.Duration, _ int) {
}
func (r *retrySpy) OnRetry(_ context.Context, _ int, _ error) {
	atomic.AddInt32(&r.onRetry, 1)
}
func (r *retrySpy) OnCircuitTransition(_ context.Context, _, _ CircuitState) {}
func (r *retrySpy) OnCacheHit(_ context.Context, _ int)                      {}

// TestWithRetry_NilObserverIsSafe verifies passing nil obs into withRetry
// does not panic — the guard substitutes noopObserver{} internally.
func TestWithRetry_NilObserverIsSafe(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Second}
	calls := 0
	_, err := withRetry(context.Background(), cfg, "http", nil, func() (int, int, error) {
		calls++
		return 0, http.StatusServiceUnavailable, errors.New("boom")
	})
	if err == nil {
		t.Fatalf("want error, got nil")
	}
	if calls != 2 {
		t.Errorf("want 2 calls (1 retry), got %d", calls)
	}
}
