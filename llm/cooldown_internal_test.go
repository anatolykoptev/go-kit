package llm

import (
	"net/http"
	"testing"
	"time"
)

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
