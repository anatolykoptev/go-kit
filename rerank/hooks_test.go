package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// countingObserver counts how many times each Observer callback fires.
// Used across hooks_test.go and config_test.go (same package).
type countingObserver struct {
	beforeCall        int
	afterCall         int
	retry             int
	circuitTransition int
	cacheHit          int
	truncate          int
	lastStatus        Status
}

func (o *countingObserver) OnBeforeCall(_ context.Context, _ string, _ int) { o.beforeCall++ }
func (o *countingObserver) OnAfterCall(_ context.Context, s Status, _ time.Duration, _ int) {
	o.afterCall++
	o.lastStatus = s
}
func (o *countingObserver) OnRetry(_ context.Context, _ int, _ error) { o.retry++ }
func (o *countingObserver) OnCircuitTransition(_ context.Context, _, _ CircuitState) {
	o.circuitTransition++
}
func (o *countingObserver) OnCacheHit(_ context.Context, _ int)              { o.cacheHit++ }
func (o *countingObserver) OnTruncate(_ context.Context, _ string, _, _ int) { o.truncate++ }

// ── TestNoopObserver_AllMethodsNoop ──────────────────────────────────────────

func TestNoopObserver_AllMethodsNoop(t *testing.T) {
	// noopObserver must not panic on any method call.
	var obs noopObserver
	ctx := context.Background()
	obs.OnBeforeCall(ctx, "q", 3)
	obs.OnAfterCall(ctx, StatusOk, time.Millisecond, 3)
	obs.OnRetry(ctx, 1, nil)
	obs.OnCircuitTransition(ctx, CircuitClosed, CircuitOpen)
	obs.OnCacheHit(ctx, 5)
	obs.OnTruncate(ctx, "doc-1", 512, 256)
}

// ── TestSafeCall_RecoversFromPanic ───────────────────────────────────────────

func TestSafeCall_RecoversFromPanic(t *testing.T) {
	// A panic inside the callback must NOT propagate to the caller.
	panicked := false
	defer func() {
		if r := recover(); r != nil {
			// If we reach here the panic escaped safeCall — that's a bug.
			t.Errorf("panic escaped safeCall: %v", r)
		}
	}()
	safeCall(func() {
		panicked = true
		panic("observer panic")
	})
	if !panicked {
		t.Error("callback was not called at all")
	}
}

func TestSafeCall_NoPanic_RunsNormally(t *testing.T) {
	ran := false
	safeCall(func() { ran = true })
	if !ran {
		t.Error("safeCall must run non-panicking callback")
	}
}

// ── TestObserver_AllCallbacksFire ────────────────────────────────────────────

func TestObserver_CallbacksFire_OnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model: "test",
			Results: []cohereResult{
				{Index: 0, RelevanceScore: 0.9},
			},
		})
	}))
	t.Cleanup(srv.Close)

	obs := &countingObserver{}
	c := NewClient(srv.URL, WithModel("test"), WithTimeout(time.Second), WithObserver(obs))

	docs := []Doc{{ID: "a", Text: "hello"}}
	_ = c.Rerank(context.Background(), "q", docs)

	if obs.beforeCall != 1 {
		t.Errorf("OnBeforeCall: got %d want 1", obs.beforeCall)
	}
	if obs.afterCall != 1 {
		t.Errorf("OnAfterCall: got %d want 1", obs.afterCall)
	}
	if obs.lastStatus != StatusOk {
		t.Errorf("OnAfterCall status: got %v want StatusOk", obs.lastStatus)
	}
	// G0: OnRetry, OnCircuit, OnCache, OnTruncate do not fire yet.
	if obs.retry != 0 {
		t.Errorf("OnRetry: got %d want 0 (G0)", obs.retry)
	}
	if obs.circuitTransition != 0 {
		t.Errorf("OnCircuitTransition: got %d want 0 (G0)", obs.circuitTransition)
	}
}

func TestObserver_CallbacksFire_OnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	obs := &countingObserver{}
	c := NewClient(srv.URL, WithTimeout(time.Second), WithObserver(obs))

	_ = c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})

	if obs.beforeCall != 1 {
		t.Errorf("OnBeforeCall: got %d want 1", obs.beforeCall)
	}
	if obs.afterCall != 1 {
		t.Errorf("OnAfterCall: got %d want 1", obs.afterCall)
	}
	if obs.lastStatus != StatusDegraded {
		t.Errorf("OnAfterCall status: got %v want StatusDegraded", obs.lastStatus)
	}
}

func TestObserver_PanicInCallback_DoesNotKillRerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.5}},
		})
	}))
	t.Cleanup(srv.Close)

	panicObs := &panicObserver{}
	c := NewClient(srv.URL, WithTimeout(time.Second), WithObserver(panicObs))

	docs := []Doc{{ID: "a", Text: "x"}}
	got := c.Rerank(context.Background(), "q", docs)
	// Despite observer panic the rerank must complete and return results.
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d — observer panic killed rerank", len(got))
	}
}

// panicObserver panics on every callback to exercise safeCall recovery.
type panicObserver struct{}

func (panicObserver) OnBeforeCall(_ context.Context, _ string, _ int) { panic("before") }
func (panicObserver) OnAfterCall(_ context.Context, _ Status, _ time.Duration, _ int) {
	panic("after")
}
func (panicObserver) OnRetry(_ context.Context, _ int, _ error) { panic("retry") }
func (panicObserver) OnCircuitTransition(_ context.Context, _, _ CircuitState) {
	panic("circuit")
}
func (panicObserver) OnCacheHit(_ context.Context, _ int)              { panic("cache") }
func (panicObserver) OnTruncate(_ context.Context, _ string, _, _ int) { panic("truncate") }
