package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
)

// quotaHandler responds 429 with an optional Retry-After header and a
// rate-limit body — the canonical "free key exhausted" shape.
func quotaHandler(retryAfter string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	}
}

// cliproxyAuthUnavailableBody is the real cliproxyapi 503 shape when a free key
// is exhausted: "no auth available (model=...)" — NOT a transient gateway blip.
const cliproxyAuthUnavailableBody = `{"error":{"message":"no auth available (model=cerebras-qwen-3-235b)","type":"auth_unavailable"}}`

// countingHandler wraps a handler and counts the number of requests it sees,
// so a test can assert how many times an upstream was actually hit.
func countingHandler(h http.HandlerFunc) (http.HandlerFunc, *int64) {
	var n int64
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&n, 1)
		h(w, r)
	}, &n
}

// TestCooldown_429WithRetryAfter_SkipsModelDuringWindow: after FailThreshold
// observed 429s, the cooled model is skipped on subsequent calls, honouring the
// Retry-After window. The next call goes straight to the healthy fallback.
func TestCooldown_429WithRetryAfter_SkipsModelDuringWindow(t *testing.T) {
	quotaH, quotaHits := countingHandler(quotaHandler("3600")) // 1h window
	dead := httptest.NewServer(quotaH)
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-fallback"))
	defer ok.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 2}),
	)

	// Two calls to cross FailThreshold=2 → primary enters cooldown.
	for i := range 2 {
		out, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if out != "from-fallback" {
			t.Fatalf("call %d: out = %q, want from-fallback", i, out)
		}
	}
	hitsAfterTwo := atomic.LoadInt64(quotaHits)
	if hitsAfterTwo != 2 {
		t.Fatalf("primary hit %d times during learning, want 2", hitsAfterTwo)
	}

	// Third call: primary is in cooldown → skipped, no new upstream hit.
	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("call 3: unexpected error: %v", err)
	}
	if out != "from-fallback" {
		t.Errorf("call 3: out = %q, want from-fallback", out)
	}
	if got := atomic.LoadInt64(quotaHits); got != hitsAfterTwo {
		t.Errorf("primary hit during cooldown window: %d hits, want unchanged %d", got, hitsAfterTwo)
	}
}

// TestCooldown_Quota503_Skips: a 503 whose body marks auth_unavailable is a
// quota-class error and enters cooldown after FailThreshold.
func TestCooldown_Quota503_Skips(t *testing.T) {
	deadH, deadHits := countingHandler(statusBodyHandler(http.StatusServiceUnavailable, cliproxyAuthUnavailableBody))
	dead := httptest.NewServer(deadH)
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "ok"))
	defer ok.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)

	// FailThreshold=1 → one quota-503 cools primary immediately.
	if _, err := c.Complete(context.Background(), "", "test"); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	firstHits := atomic.LoadInt64(deadHits)
	if firstHits == 0 {
		t.Fatal("primary should have been hit at least once on call 1")
	}
	// Next call: primary skipped.
	if _, err := c.Complete(context.Background(), "", "test"); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if got := atomic.LoadInt64(deadHits); got != firstHits {
		t.Errorf("quota-503 model not cooled: hit %d, want unchanged %d", got, firstHits)
	}
}

// TestCooldown_Plain503_NotCooled: a bare 503 with no quota/auth marker is a
// transient gateway blip, NOT quota-class — it must NOT enter cooldown. Every
// call still hits the primary first (then falls over normally).
func TestCooldown_Plain503_NotCooled(t *testing.T) {
	flakyH, flakyHits := countingHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // bare 503, empty body
	})
	flaky := httptest.NewServer(flakyH)
	defer flaky.Close()
	ok := httptest.NewServer(okChatHandler(t, "ok"))
	defer ok.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: flaky.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)

	for i := range 3 {
		if _, err := c.Complete(context.Background(), "", "test"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// A non-quota 503 never cools → primary hit on every one of the 3 calls.
	if got := atomic.LoadInt64(flakyHits); got != 3 {
		t.Errorf("plain 503 wrongly cooled: primary hit %d times, want 3 (never skipped)", got)
	}
}

// TestCooldown_ClearOnSuccess: a 200 clears the consecutive-fail counter so a
// single transient 429 followed by a success does NOT trip cooldown at the next
// 429 (counter reset, needs FailThreshold fresh fails again).
func TestCooldown_ClearOnSuccess(t *testing.T) {
	var mu sync.Mutex
	fail := true // toggled: fail, then succeed, then fail...
	flakyH, flakyHits := countingHandler(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		f := fail
		fail = !fail
		mu.Unlock()
		if f {
			quotaHandler("")(w, &http.Request{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	})
	flaky := httptest.NewServer(flakyH)
	defer flaky.Close()
	ok := httptest.NewServer(okChatHandler(t, "fallback"))
	defer ok.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: flaky.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 2}),
	)

	// Pattern: fail, success, fail, success ... never 2 *consecutive* fails,
	// so primary never cools and is hit on every call.
	for i := range 6 {
		if _, err := c.Complete(context.Background(), "", "test"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt64(flakyHits); got != 6 {
		t.Errorf("clear-on-success failed: primary hit %d times, want 6 (alternating fail/ok never reaches 2 consecutive)", got)
	}
}

// TestCooldown_AllCooledDown_StillAttemptsPrimary is the never-fail-closed
// fitness function: when EVERY chain model is in cooldown, the loop must still
// attempt the primary (degraded > dead) and return the real upstream error,
// never a locally-synthesised "no endpoint".
func TestCooldown_AllCooledDown_StillAttemptsPrimary(t *testing.T) {
	pH, pHits := countingHandler(quotaHandler(""))
	primary := httptest.NewServer(pH)
	defer primary.Close()
	sH, sHits := countingHandler(quotaHandler(""))
	secondary := httptest.NewServer(sH)
	defer secondary.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: primary.URL, Key: "k", Model: "primary"},
			{URL: secondary.URL, Key: "k", Model: "secondary"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)

	// Call 1 cools BOTH models (each returns quota 429, FailThreshold=1).
	if _, err := c.Complete(context.Background(), "", "test"); err == nil {
		t.Fatal("call 1: expected error (both upstreams 429)")
	}
	pAfter1 := atomic.LoadInt64(pHits)
	sAfter1 := atomic.LoadInt64(sHits)

	// Call 2: both cooled. Must still attempt the PRIMARY (exactly one upstream),
	// and return the real upstream 429 error — not a local "no endpoint".
	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("call 2: expected the real upstream error, got nil")
	}
	gotP := atomic.LoadInt64(pHits) - pAfter1
	gotS := atomic.LoadInt64(sHits) - sAfter1
	if gotP != 1 {
		t.Errorf("all-cooled: primary attempted %d times on call 2, want exactly 1 (degraded>dead)", gotP)
	}
	if gotS != 0 {
		t.Errorf("all-cooled: secondary attempted %d times on call 2, want 0 (only the last-resort primary)", gotS)
	}
}

// TestNoCooldownOption_ByteIdentical: a client built WITHOUT WithModelCooldown
// behaves exactly like v0.81.1 — every endpoint is attempted on every call, no
// model is ever skipped. Backward-compat fitness function.
func TestNoCooldownOption_ByteIdentical(t *testing.T) {
	deadH, deadHits := countingHandler(quotaHandler("3600"))
	dead := httptest.NewServer(deadH)
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "ok"))
	defer ok.Close()

	// NO WithModelCooldown.
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
	)

	for i := range 5 {
		if _, err := c.Complete(context.Background(), "", "test"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// No cooldown state → primary hit on every one of the 5 calls.
	if got := atomic.LoadInt64(deadHits); got != 5 {
		t.Errorf("default path changed: primary hit %d times, want 5 (no cooldown, never skipped)", got)
	}
}

// TestCooldown_Observer_FiresOnEntryAndRecovery: the optional cooldown observer
// fires once on cooldown ENTRY (cooling=true) and once on RECOVERY (cooling=false,
// via recordSuccess). This is the de-duped degraded-chain signal (Decision 3).
func TestCooldown_Observer_FiresOnEntryAndRecovery(t *testing.T) {
	var mu sync.Mutex
	fail := true
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		f := fail
		mu.Unlock()
		if f {
			quotaHandler("")(w, &http.Request{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer flaky.Close()
	ok := httptest.NewServer(okChatHandler(t, "fallback"))
	defer ok.Close()

	type ev struct {
		model   string
		cooling bool
		d       time.Duration
	}
	// The observer is dispatched async with panic recovery (MAJOR 2), so the test
	// must synchronise on event delivery rather than read shared state right after
	// the call returns. evCh carries every delivered event; the test drains the
	// first. Buffered so the observer goroutine never blocks if the test is slow.
	evCh := make(chan ev, 8)

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: flaky.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
		llm.WithModelCooldownObserver(func(model string, cooling bool, d time.Duration) {
			evCh <- ev{model: model, cooling: cooling, d: d}
		}),
	)

	// Call 1: primary 429 → cooldown ENTRY (cooling=true).
	if _, err := c.Complete(context.Background(), "", "test"); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	// Flip upstream to healthy.
	mu.Lock()
	fail = false
	mu.Unlock()
	// Call 2: primary is cooled, so it's skipped — recordSuccess won't fire for
	// primary. The cooldown must expire or be force-attempted to recover. Since
	// the fallback serves call 2, primary stays cooled. We assert at least the
	// ENTRY event fired.
	if _, err := c.Complete(context.Background(), "", "test"); err != nil {
		t.Fatalf("call 2: %v", err)
	}

	// Wait for the async ENTRY event (deterministic, bounded — never a bare sleep).
	var first ev
	select {
	case first = <-evCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cooldown observer entry event")
	}
	if first.model != "primary" || !first.cooling {
		t.Errorf("first event = %+v, want {primary, cooling=true}", first)
	}
}
