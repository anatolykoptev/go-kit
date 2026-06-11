package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestExecuteInner_SkipCooledRace_NeverReturnsNilNil is the regression guard for
// the (nil, nil) race in executeInner (MAJOR 1):
//
//	cooldownCandidates() snapshots skipCooled=true because ≥1 model is healthy at
//	that instant. Between the snapshot and the per-iteration cooling() re-check, a
//	concurrent goroutine cools the LAST healthy model. Every iteration then hits
//	`skipCooled && cooling(ep.Model)` → continue, the loop ends with lastErr==nil,
//	and the pre-fix code returns (nil, nil) — a structural nil-deref in CompleteRaw
//	(resp.Content) and every public caller.
//
// The race is forced deterministically with a side-effecting clock: the very read
// that the in-loop cooling(secondary) check performs is the one that cools
// secondary, simulating the concurrent recordFailure landing in that exact window.
// No sleeps, no real goroutines — the window is collapsed to a single observable
// point so the test is non-flaky under -race.
//
// Post-fix invariant: when the loop completes with lastErr==nil (everything was
// skipped), executeInner unconditionally attempts endpoints[0] before returning,
// so a non-nil response OR a non-nil error always comes back — (nil, nil) is
// structurally impossible.
func TestExecuteInner_SkipCooledRace_NeverReturnsNilNil(t *testing.T) {
	// Upstream that always 429s: when the post-fix code force-attempts the
	// primary, we get a real upstream error (non-nil) — never (nil, nil).
	dead := newQuota429Server(t)
	defer dead.Close()

	c := NewClient("", "", "",
		WithEndpoints([]Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: dead.URL, Key: "k", Model: "secondary"},
		}),
		WithMaxRetries(1),
		WithModelCooldown(CooldownConfig{FailThreshold: 1, Default: time.Hour}),
	)

	// Anchor a fixed clock. primary is pre-cooled (until far in the future) so it
	// is skipped throughout. secondary starts healthy (no until entry) so the
	// cooldownCandidates() snapshot observes a healthy candidate and returns
	// skipCooled=true.
	base := time.Unix(1_000_000, 0)
	mc := c.cooldown
	mc.mu.Lock()
	mc.until["primary"] = base.Add(time.Hour) // cooled, and stays cooled
	mc.mu.Unlock()

	// Side-effecting clock that collapses the snapshot→iteration race window to a
	// single deterministic point. cooling() invokes now() ONLY when the model
	// already has an until entry (an unset model short-circuits to false before
	// touching now()). So now() fires for `primary` but not for the healthy
	// `secondary`. Call sequence for the [primary(cooled), secondary] chain:
	//
	//	#1  snapshot   cooling(primary)  → until set  → now() → true
	//	    snapshot   cooling(secondary)→ until UNSET → false (no now()) → healthy
	//	               ⇒ cooldownCandidates returns skipCooled=true
	//	#2  loop iter  cooling(primary)  → until set  → now() → true → continue
	//	    loop iter  cooling(secondary)→ now cooled by the #2 side effect → skip
	//
	// On the 2nd now() (the in-loop primary re-check) the concurrent goroutine
	// "lands": we cool secondary. By the time the loop reaches secondary it is
	// cooled, every endpoint is skipped, lastErr stays nil.
	var nowCalls int
	mc.clock = func() time.Time {
		nowCalls++
		if nowCalls == 2 {
			mc.until["secondary"] = base.Add(time.Hour)
		}
		return base
	}

	resp, err := c.executeInner(context.Background(), c.newRequest([]Message{{Role: "user", Content: "x"}}))

	// THE assertion: (nil, nil) is forbidden. Pre-fix, this fired with both nil.
	if resp == nil && err == nil {
		t.Fatal("executeInner returned (nil, nil): nil-deref bug — loop skipped every endpoint and never force-attempted the primary")
	}
}

// TestModelCooldown_PerWindowReEvent: after a model rides its cooldown window
// out SILENTLY (TTL lapses without ever seeing a 200, because it was skipped),
// a fresh quota burst re-cools it and MUST emit a second ENTRY event — one
// per active window (MINOR 2, Decision 3: per-window re-event). The old presence-
// keyed dedup swallowed this because the stale until entry was still in the map.
func TestModelCooldown_PerWindowReEvent(t *testing.T) {
	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }

	var entries atomic.Int64
	mc.onChange = func(_ string, cooling bool, _ time.Duration) {
		if cooling {
			entries.Add(1)
		}
	}

	// Window 1: one fail cools the model (FailThreshold=1) → entry event #1.
	mc.recordFailure("m", 0)
	// onChange is async; drain deterministically.
	if got := waitEntries(&entries, 1); got != 1 {
		t.Fatalf("after first cooldown: entry events = %d, want 1", got)
	}

	// Ride the window out silently: advance past the 60s TTL with NO recordSuccess.
	now = now.Add(61 * time.Second)
	if mc.cooling("m") {
		t.Fatal("model should have expired (silent TTL lapse) at t+61s")
	}

	// Window 2: a fresh fail re-cools the same model → a SECOND entry event.
	mc.recordFailure("m", 0)
	if got := waitEntries(&entries, 2); got != 2 {
		t.Errorf("re-cool after silent expiry: entry events = %d, want 2 (per-window re-event)", got)
	}
}

// waitEntries polls the entry counter (mutated from an async observer goroutine)
// until it reaches want or a short bound elapses, then returns the observed
// value. Deterministic enough for tests — the goroutine is dispatched eagerly.
func waitEntries(n *atomic.Int64, want int64) int64 {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v := n.Load(); v >= want {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	return n.Load()
}

// newQuota429Server returns an httptest server that always answers 429 with a
// rate-limit body (the canonical quota-class shape).
func newQuota429Server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit reached","type":"rate_limit_error"}}`))
	}))
}

// TestModelCooldown_ExpiryViaClock: a cooled model stops being cooled once the
// injectable clock advances past the until instant. Deterministic — no sleep.
func TestModelCooldown_ExpiryViaClock(t *testing.T) {
	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }

	mc.recordFailure("m", 0) // FailThreshold=1 → cools for Default=60s
	if !mc.cooling("m") {
		t.Fatal("model should be cooling immediately after recordFailure")
	}
	// Advance 59s — still cooling.
	now = now.Add(59 * time.Second)
	if !mc.cooling("m") {
		t.Error("model should still be cooling at t+59s (window=60s)")
	}
	// Advance past the window — no longer cooling.
	now = now.Add(2 * time.Second) // t+61s
	if mc.cooling("m") {
		t.Error("model should NOT be cooling at t+61s (window=60s expired)")
	}
}

// TestModelCooldown_RetryAfterClamp: an absurd Retry-After is clamped to cfg.Max.
func TestModelCooldown_RetryAfterClamp(t *testing.T) {
	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second, Max: 10 * time.Minute})
	mc.clock = func() time.Time { return now }

	mc.recordFailure("m", 999999999*time.Second) // absurd
	// Still cooling at Max-1s.
	now = now.Add(10*time.Minute - time.Second)
	if !mc.cooling("m") {
		t.Error("clamped cooldown should still be active just before Max")
	}
	// Past Max → not cooling (proves the clamp, not the absurd value, applied).
	now = now.Add(2 * time.Second) // Max + 1s
	if mc.cooling("m") {
		t.Error("cooldown should be clamped to Max=10m, expired by Max+1s")
	}
}

// TestModelCooldown_FailThresholdGate: needs FailThreshold consecutive quota
// fails before cooling; below the threshold the model is not cooled.
func TestModelCooldown_FailThresholdGate(t *testing.T) {
	mc := newModelCooldown(CooldownConfig{FailThreshold: 2, Default: 60 * time.Second})
	mc.recordFailure("m", 0)
	if mc.cooling("m") {
		t.Error("one fail below threshold=2 should NOT cool")
	}
	mc.recordFailure("m", 0)
	if !mc.cooling("m") {
		t.Error("two fails reaching threshold=2 SHOULD cool")
	}
}

// TestModelCooldown_RecordSuccessClears: recordSuccess resets the consecutive
// fail counter (a 200 means quota recovered).
func TestModelCooldown_RecordSuccessClears(t *testing.T) {
	mc := newModelCooldown(CooldownConfig{FailThreshold: 2, Default: 60 * time.Second})
	mc.recordFailure("m", 0) // 1 fail
	mc.recordSuccess("m")    // clears
	mc.recordFailure("m", 0) // 1 fail again (not 2)
	if mc.cooling("m") {
		t.Error("recordSuccess must reset the fail counter; one post-success fail should not cool")
	}
}

// TestIsQuotaError classifies the canonical quota shapes and rejects non-quota.
func TestIsQuotaError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"429 any body", &APIError{StatusCode: http.StatusTooManyRequests}, true},
		{"429 with type", &APIError{StatusCode: http.StatusTooManyRequests, Type: "rate_limit_error"}, true},
		{
			"503 auth_unavailable type",
			&APIError{StatusCode: http.StatusServiceUnavailable, Type: "auth_unavailable"},
			true,
		},
		{
			"503 quota in body",
			&APIError{StatusCode: http.StatusServiceUnavailable, Body: `{"error":{"message":"no auth available (model=x)"}}`},
			true,
		},
		{
			"503 quota word in body",
			&APIError{StatusCode: http.StatusServiceUnavailable, Body: "quota exceeded for this key"},
			true,
		},
		{
			"bare 503 transient",
			&APIError{StatusCode: http.StatusServiceUnavailable, Body: ""},
			false,
		},
		{
			"503 generic upstream",
			&APIError{StatusCode: http.StatusServiceUnavailable, Body: "upstream connect error"},
			false,
		},
		{"500 not quota", &APIError{StatusCode: http.StatusInternalServerError}, false},
		{"413 not quota", &APIError{StatusCode: http.StatusRequestEntityTooLarge}, false},
		{"nil not quota", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isQuotaError(tt.err); got != tt.want {
				t.Errorf("isQuotaError(%+v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestModelCooldown_NilSafe: a nil *modelCooldown is a no-op (cooling=false),
// mirroring the CircuitBreaker nil-receiver discipline. The executeInner guard
// `c.cooldown != nil` relies on this being safe even if reached.
func TestModelCooldown_NilSafe(t *testing.T) {
	var mc *modelCooldown
	if mc.cooling("anything") {
		t.Error("nil cooldown must report cooling=false")
	}
	mc.recordFailure("m", 0) // must not panic
	mc.recordSuccess("m")    // must not panic
}
