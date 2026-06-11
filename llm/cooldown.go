// Package llm — cooldown.go: per-model quota-aware cooldown for the endpoint
// fallback chain (WithEndpoints).
//
// Why: free-tier LLM quotas are GLOBAL across the fleet (e.g. cerebras 1M
// tok/day). When a chain's primary model is exhausted it returns 429 (or a
// 503 marking auth/quota unavailable) on EVERY call. Without cooldown each call
// pays the dead-primary hop (1 RTT) and logs one error line. After
// FailThreshold observed quota-fails this puts the model in a short cooldown and
// the chain SKIPS it, going straight to the next healthy model — dropping both
// the per-call dead hop and ~99% of this path's log noise at steady state.
//
// This is the per-model reactive circuit-skip pattern used by LiteLLM (each
// deployment has health + cooldown status; repeated 429 stops sending to that
// deployment for recovery_timeout, honouring retry_after). It is orthogonal to
// and composes with the existing outer WithCircuitBreaker (which is keyed on the
// single construction model and wraps the WHOLE client — wrong granularity for
// a multi-model chain).
//
// Invariants:
//   - Default-off: no WithModelCooldown option → c.cooldown is nil → zero state,
//     zero behaviour change vs the no-cooldown path.
//   - Never fail-closed: the executeInner loop never skips the LAST non-cooled
//     candidate; if EVERY model is cooled it still attempts the primary
//     (degraded > dead) and surfaces the real upstream error.
//   - Concurrency-safe: the map is RWMutex-guarded (clients live in multi-
//     goroutine services). Mirrors CircuitBreaker's RWMutex discipline.
package llm

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Cooldown defaults — applied when a CooldownConfig field is zero.
const (
	defaultCooldownFailThreshold = 2
	defaultCooldownDuration      = 60 * time.Second
	defaultCooldownMax           = 10 * time.Minute
)

// CooldownConfig tunes the per-model cooldown. Zero values fill from defaults.
type CooldownConfig struct {
	// FailThreshold — consecutive quota-class fails on a model before it is put
	// in cooldown. Default 2 (need two, not one, to ride out a transient 429).
	FailThreshold int
	// Default — cooldown duration when the upstream gives no Retry-After.
	// Default 60s.
	Default time.Duration
	// Max — clamp on Retry-After (defends against absurd server values like
	// "Retry-After: 999999999"). Default 10m.
	Max time.Duration
}

func (cfg CooldownConfig) withDefaults() CooldownConfig {
	if cfg.FailThreshold <= 0 {
		cfg.FailThreshold = defaultCooldownFailThreshold
	}
	if cfg.Default <= 0 {
		cfg.Default = defaultCooldownDuration
	}
	if cfg.Max <= 0 {
		cfg.Max = defaultCooldownMax
	}
	return cfg
}

// modelCooldown tracks per-model cooldown state. Thread-safe: reads (cooling)
// take RLock, writes (recordFailure/recordSuccess) take Lock.
type modelCooldown struct {
	mu    sync.RWMutex
	until map[string]time.Time // model -> earliest-retry instant
	fails map[string]int       // model -> consecutive quota-fails
	cfg   CooldownConfig
	clock func() time.Time // injectable for tests; default time.Now
	// onChange fires once on cooldown entry (cooling=true) and once on recovery
	// (cooling=false). Optional, non-blocking — the caller must not block/panic.
	onChange func(model string, cooling bool, d time.Duration)
}

func newModelCooldown(cfg CooldownConfig) *modelCooldown {
	return &modelCooldown{
		until: make(map[string]time.Time),
		fails: make(map[string]int),
		cfg:   cfg.withDefaults(),
		clock: time.Now,
	}
}

func (m *modelCooldown) now() time.Time {
	if m.clock != nil {
		return m.clock()
	}
	return time.Now()
}

// cooling reports whether model is currently in cooldown. nil-safe (returns
// false) so a missing cooldown is never a reason to skip.
func (m *modelCooldown) cooling(model string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	until, ok := m.until[model]
	if !ok {
		return false
	}
	return m.now().Before(until)
}

// recordFailure is called from executeInner ONLY for quota-class errors. It
// bumps the consecutive-fail counter; on reaching FailThreshold it enters
// cooldown for retryAfter (clamped to cfg.Max) or cfg.Default when retryAfter
// is zero. nil-safe no-op.
func (m *modelCooldown) recordFailure(model string, retryAfter time.Duration) {
	if m == nil {
		return
	}
	d := m.cfg.Default
	if retryAfter > 0 {
		d = retryAfter
	}
	if d > m.cfg.Max {
		d = m.cfg.Max
	}

	m.mu.Lock()
	m.fails[model]++
	reached := m.fails[model] >= m.cfg.FailThreshold
	_, wasCooling := m.until[model]
	if reached {
		m.until[model] = m.now().Add(d)
	}
	onChange := m.onChange
	m.mu.Unlock()

	// Fire onChange only on the transition INTO cooldown (entry), de-duped:
	// once per cooldown window, not once per fail.
	if reached && !wasCooling && onChange != nil {
		onChange(model, true, d)
	}
}

// recordSuccess clears fails+until for model (a 200 means the quota recovered).
// nil-safe no-op.
func (m *modelCooldown) recordSuccess(model string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	_, wasCooling := m.until[model]
	delete(m.fails, model)
	delete(m.until, model)
	onChange := m.onChange
	m.mu.Unlock()

	if wasCooling && onChange != nil {
		onChange(model, false, 0)
	}
}

// quotaBodyMarkers are substrings (lowercased) that mark a 503 as quota/auth
// exhaustion rather than a transient gateway blip. Observed on the cliproxyapi
// fleet: "no auth available (model=...)" with type "auth_unavailable".
var quotaBodyMarkers = []string{
	"auth_unavailable",
	"no auth available",
	"quota",
	"rate_limit",
	"rate limit",
	"insufficient_quota",
}

// isQuotaError reports whether err is a quota-class failure that should drive
// per-model cooldown. Conservative by design (risk register row 4):
//   - HTTP 429 is ALWAYS quota-class (any body).
//   - HTTP 503 is quota-class ONLY when its parsed Type or body marks
//     quota/auth-unavailable. A bare 503 (transient gateway blip) is NOT cooled.
//
// All other statuses (500/413/4xx) are not quota-class.
func isQuotaError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusServiceUnavailable:
		hay := strings.ToLower(apiErr.Type + " " + apiErr.Code + " " + apiErr.Body)
		for _, marker := range quotaBodyMarkers {
			if strings.Contains(hay, marker) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
