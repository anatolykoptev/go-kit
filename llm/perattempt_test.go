package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
)

// slowThenFastServers returns two httptest servers: one that sleeps for
// slowDelay before returning 200, and one that returns 200 immediately.
// Both return the given content string on success.
func slowThenFastServers(t *testing.T, slowDelay time.Duration, fastContent string) (slow, fast *httptest.Server) {
	t.Helper()
	slow = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			// Request was cancelled (per-attempt timeout fired) — do not write
			// a response; the client already got a context error.
			return
		case <-time.After(slowDelay):
		}
		// If we reach here the test waited the full delay (unset-path test).
		w.Header().Set("Content-Type", "application/json")
		_ = writeOKJSON(w, "slow-ok")
	}))
	fast = httptest.NewServer(okChatHandler(t, fastContent))
	return slow, fast
}

// writeOKJSON is a helper that writes a minimal valid LLM response JSON.
func writeOKJSON(w http.ResponseWriter, content string) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(`{"choices":[{"message":{"content":"` + content + `"}}]}`))
	return err
}

// TestPerAttemptTimeout_SlowEndpointFailsOverFast verifies that a slow first
// endpoint is bounded by the per-attempt timeout and the chain advances to the
// fast second endpoint, completing well within the outer ctx deadline.
func TestPerAttemptTimeout_SlowEndpointFailsOverFast(t *testing.T) {
	const perAttemptD = 120 * time.Millisecond
	const slowSleep = 600 * time.Millisecond

	slowSrv, fastSrv := slowThenFastServers(t, slowSleep, "from-fast")
	defer slowSrv.Close()
	defer fastSrv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: slowSrv.URL, Key: "k", Model: "slow-model"},
			{URL: fastSrv.URL, Key: "k", Model: "fast-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithPerAttemptTimeout(perAttemptD),
		llm.WithEndpointAttemptObserver(obs),
	)

	outerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	out, err := c.Complete(outerCtx, "", "test")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "from-fast" {
		t.Errorf("output = %q, want %q", out, "from-fast")
	}
	// Should complete in roughly perAttemptD + fast latency, well under slowSleep.
	// Use 2.5× perAttemptD as a generous upper bound.
	maxExpected := perAttemptD*25/10 + 100*time.Millisecond
	if elapsed >= slowSleep {
		t.Errorf("elapsed %v >= slow sleep %v — per-attempt timeout did not fire", elapsed, slowSleep)
	}
	if elapsed > maxExpected {
		t.Logf("elapsed %v > maxExpected %v (may be slow CI — not a hard failure, but worth noting)", elapsed, maxExpected)
	}

	// Observer must have fired twice: once for slow-model (with timeout err),
	// once for fast-model (with nil err).
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls, got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "slow-model" {
		t.Errorf("calls[0].model = %q, want %q", (*calls)[0].model, "slow-model")
	}
	if (*calls)[0].err == nil {
		t.Errorf("calls[0].err = nil, want a timeout error for slow-model")
	}
	if (*calls)[1].model != "fast-model" {
		t.Errorf("calls[1].model = %q, want %q", (*calls)[1].model, "fast-model")
	}
	if (*calls)[1].err != nil {
		t.Errorf("calls[1].err = %v, want nil (fast-model success)", (*calls)[1].err)
	}
}

// TestPerAttemptTimeout_UnsetIsByteIdentical verifies that when
// WithPerAttemptTimeout is NOT used, the attempt is bounded only by the outer
// ctx — the slow endpoint is NOT killed early, the chain does NOT advance, and
// the slow endpoint's result is returned.
func TestPerAttemptTimeout_UnsetIsByteIdentical(t *testing.T) {
	const slowSleep = 200 * time.Millisecond

	// Slow endpoint returns "slow-result" after slowSleep (no ctx cancel path
	// since outer ctx is generous).
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(slowSleep):
		}
		_ = writeOKJSON(w, "slow-result")
	}))
	defer slowSrv.Close()

	fastSrv := httptest.NewServer(okChatHandler(t, "fast-result"))
	defer fastSrv.Close()

	_, calls, obs := newObserver()
	// NO WithPerAttemptTimeout — pre-feature byte-identical path.
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: slowSrv.URL, Key: "k", Model: "slow-model"},
			{URL: fastSrv.URL, Key: "k", Model: "fast-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	outerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	out, err := c.Complete(outerCtx, "", "test")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without per-attempt timeout, slow-model succeeds after its full sleep.
	if out != "slow-result" {
		t.Errorf("output = %q, want %q (slow model should have returned without timeout)", out, "slow-result")
	}
	// Elapsed must be >= slowSleep (the slow model was not cut short).
	if elapsed < slowSleep {
		t.Errorf("elapsed %v < slow sleep %v — slow endpoint was cut short without WithPerAttemptTimeout", elapsed, slowSleep)
	}
	// Observer fires only once (slow model succeeded — no chain advance).
	if len(*calls) != 1 {
		t.Fatalf("expected 1 observer call (slow success, no failover), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "slow-model" || (*calls)[0].err != nil {
		t.Errorf("calls[0] = %+v, want {slow-model, nil}", (*calls)[0])
	}
}

// TestPerAttemptTimeout_OuterCtxStillCeiling verifies that the outer ctx is
// the absolute ceiling: when outer < perAttemptD, the call aborts at the outer
// deadline — the per-attempt timeout does NOT extend the outer ctx.
func TestPerAttemptTimeout_OuterCtxStillCeiling(t *testing.T) {
	const outerTimeout = 80 * time.Millisecond
	const perAttemptD = 500 * time.Millisecond // larger than outer
	const slowSleep = 500 * time.Millisecond   // only needs to outlive the 80ms outer deadline

	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(slowSleep):
		}
		_ = writeOKJSON(w, "should-not-reach")
	}))
	defer slowSrv.Close()

	fastSrv := httptest.NewServer(okChatHandler(t, "fast-result"))
	defer fastSrv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: slowSrv.URL, Key: "k", Model: "slow-model"},
			{URL: fastSrv.URL, Key: "k", Model: "fast-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithPerAttemptTimeout(perAttemptD),
		llm.WithEndpointAttemptObserver(obs),
	)

	outerCtx, cancel := context.WithTimeout(context.Background(), outerTimeout)
	defer cancel()

	start := time.Now()
	_, err := c.Complete(outerCtx, "", "test")
	elapsed := time.Since(start)

	// Must return an error (outer ctx deadline exceeded).
	if err == nil {
		t.Fatal("expected error when outer ctx expires first, got nil")
	}
	// Elapsed must be roughly outerTimeout, not perAttemptD or slowSleep.
	if elapsed >= perAttemptD {
		t.Errorf("elapsed %v >= perAttemptD %v — outer ctx did not act as ceiling", elapsed, perAttemptD)
	}
	// The outer ctx is also done — confirms it was the outer deadline, not per-attempt.
	if outerCtx.Err() == nil {
		t.Error("outer ctx should be done after test completes")
	}
	// Observer fires once for slow-model (with the deadline error).
	if len(*calls) != 1 {
		t.Fatalf("expected 1 observer call (slow-model aborted by outer ctx), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "slow-model" {
		t.Errorf("calls[0].model = %q, want %q", (*calls)[0].model, "slow-model")
	}
	if (*calls)[0].err == nil {
		t.Error("calls[0].err = nil, want deadline error")
	}
}

// TestChainAdvanceOnHTTPClientTimeout verifies that when the HTTP client's own
// timeout fires (not a per-attempt timeout) and the outer ctx is still alive,
// the chain advances to the next endpoint. This is the fix for the issue where
// go-wp (which does NOT set WithPerAttemptTimeout) would pay the full 90s HTTP
// client timeout per model and then abort, instead of advancing to the next
// model in the chain.
func TestChainAdvanceOnHTTPClientTimeout(t *testing.T) {
	const httpTimeout = 150 * time.Millisecond
	const slowSleep = 600 * time.Millisecond

	slowSrv, fastSrv := slowThenFastServers(t, slowSleep, "from-fast")
	defer slowSrv.Close()
	defer fastSrv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: slowSrv.URL, Key: "k", Model: "slow-model"},
			{URL: fastSrv.URL, Key: "k", Model: "fast-model"},
		}),
		llm.WithMaxRetries(1),
		// NO WithPerAttemptTimeout — the HTTP client timeout is the only bound.
		llm.WithHTTPClient(&http.Client{Timeout: httpTimeout}),
		llm.WithEndpointAttemptObserver(obs),
	)

	outerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	out, err := c.Complete(outerCtx, "", "test")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "from-fast" {
		t.Errorf("output = %q, want %q (chain should have advanced to fast-model)", out, "from-fast")
	}
	// Should complete in roughly httpTimeout + fast latency, well under slowSleep.
	if elapsed >= slowSleep {
		t.Errorf("elapsed %v >= slow sleep %v — HTTP client timeout did not fire / chain did not advance", elapsed, slowSleep)
	}

	// Observer must have fired twice: slow-model (timeout) + fast-model (success).
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (slow timeout + fast success), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "slow-model" || (*calls)[0].err == nil {
		t.Errorf("calls[0] = %+v, want {slow-model, non-nil err}", (*calls)[0])
	}
	if (*calls)[1].model != "fast-model" || (*calls)[1].err != nil {
		t.Errorf("calls[1] = %+v, want {fast-model, nil err}", (*calls)[1])
	}
}
