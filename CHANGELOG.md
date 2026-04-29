# Changelog

## [0.27.0](https://github.com/anatolykoptev/go-kit/compare/v0.26.0...v0.27.0) (2026-04-29)


### Features

* **rerank:** G0 foundation — functional options, Result type, Observer hooks ([#8](https://github.com/anatolykoptev/go-kit/issues/8)) ([85fc55c](https://github.com/anatolykoptev/go-kit/commit/85fc55c10026da2b3a3698ba3f629b66efbd8f30))
* **rerank:** G1 resiliency — retry + circuit + fallback (M12.10) ([#9](https://github.com/anatolykoptev/go-kit/issues/9)) ([cf76413](https://github.com/anatolykoptev/go-kit/commit/cf764134766732952cea8e3a718057b34fa79a32))
* **rerank:** G2-client quality api (M12.10, post-split) ([#10](https://github.com/anatolykoptev/go-kit/issues/10)) ([85ac44f](https://github.com/anatolykoptev/go-kit/commit/85ac44fcf78c825cc521f4aa82190764285e4d06))
* **rerank:** G3 cascade — multi-stage Reranker chain (M12.10) ([#11](https://github.com/anatolykoptev/go-kit/issues/11)) ([215aac5](https://github.com/anatolykoptev/go-kit/commit/215aac53dfd41dc94623a560d2ac34742386cddd))
* **rerank:** G4 boosters — MultiQuery + Cache interface (M12.10 closure) ([#12](https://github.com/anatolykoptev/go-kit/issues/12)) ([edf8540](https://github.com/anatolykoptev/go-kit/commit/edf8540e9789ea6d11a7e91c132a21a26f3eb0f6))


### Bug Fixes

* **release:** bump-minor-pre-major in package config (was root-only) ([#15](https://github.com/anatolykoptev/go-kit/issues/15)) ([d9bad3c](https://github.com/anatolykoptev/go-kit/commit/d9bad3cd02dac94592b073d003b2c1e11b73eed3))
* **release:** remove bump-patch-for-minor-pre-major (was forcing feat→patch) ([#16](https://github.com/anatolykoptev/go-kit/issues/16)) ([df4f15e](https://github.com/anatolykoptev/go-kit/commit/df4f15efb2448f86778ea7d82c3d9324c3233f64))

## [Unreleased] — G1: Resiliency + Multi-Model Fallback

### Added (G1)
- `RetryPolicy` struct with `MaxAttempts`, `BaseBackoff`, `MaxBackoff`, `Multiplier`, `Jitter`, `RetryableStatus`
- `NoRetry` sentinel — opt-out from default retry policy via `WithRetry(rerank.NoRetry)`
- `WithRetry(RetryPolicy) Opt` — configure retry per client
- `CircuitBreaker` FSM (Closed → Open → HalfOpen) — thread-safe via `sync.RWMutex`
- `CircuitConfig{FailThreshold, OpenDuration, HalfOpenProbes, FailRateWindow}`
- `WithCircuit(CircuitConfig) Opt` — enable circuit breaker (off by default)
- `ErrCircuitOpen` sentinel error — returned when circuit is open
- `WithFallback(*Client) Opt` — configure secondary client for multi-model fallback
- `errHTTPStatus{Code int}` typed error — enables retry filter by status code
- 5 new Prometheus metrics:
  - `rerank_retry_attempt_total{model, attempt}` — counter per retry attempt
  - `rerank_circuit_state{model, state}` — gauge (closed/open/half-open)
  - `rerank_circuit_transition_total{model, from, to}` — state change counter
  - `rerank_giveup_total{model, reason}` — exhausted / circuit_open / 4xx
  - `rerank_fallback_used_total{primary, secondary}` — successful fallbacks
- `CircuitState.String()` — human-readable label for metric use

### Changed (G1)
- `callCohere` non-2xx error now returns `errHTTPStatus{Code}` instead of
  `fmt.Errorf("http status %d")`. **String format preserved**: `err.Error()` still
  returns `"http status 500"`. Callers doing `strings.Contains(err.Error(), "http status")`
  continue to work unchanged.
- `defaultCfg()` now sets `retry = defaultRetryPolicy()` (3 attempts, 200ms exp backoff, retries on 500/502/503/504).

### Behavior change ⚠️ — v1.x → v2.0 migration note

**Retry on 5xx is now ON by default for all clients** including those created via
the v1 `New(cfg Config, logger *slog.Logger)` constructor.

Before G1: any 5xx HTTP status returned immediately as `StatusDegraded`.
After G1: 5xx responses are retried up to 3 times with exponential backoff
(200ms → 400ms → 2s, +10% jitter) before returning `StatusDegraded`.

**Impact on existing v1 callers**:
- Final output (`[]Scored`) is identical — passthrough on exhaustion.
- Added latency on failure: up to ~600ms backoff (200ms + 400ms) before giving up.
- Server call count on 5xx: up to 3× (was 1×).

**Opt-out** for instant-fail behavior:
```go
// v1 constructor — no opt-out available directly; use NewClient instead:
c := rerank.NewClient(cfg.URL,
    rerank.WithModel(cfg.Model),
    rerank.WithTimeout(cfg.Timeout),
    rerank.WithRetry(rerank.NoRetry), // ← opt out
)

// v2 constructor:
c := rerank.NewClient(url, rerank.WithRetry(rerank.NoRetry))
```

**Not affected by retry**: 4xx errors (client errors) never trigger retry.
Ctx cancellation aborts backoff sleep immediately.

---

## [v0.19.0] — G0: Foundation (Options + Result + Hooks)

### Added (G0)
- Functional options pattern (`Opt = func(*cfgInternal)`)
- `NewClient(url string, opts ...Opt) *Client` — v2 constructor
- `RerankWithResult(ctx, query, docs, opts...) (*Result, error)` — typed Result API
- `Result{Scored, Status, Model, Err}` — typed return
- `Status` enum: `StatusOk`, `StatusDegraded`, `StatusFallback`, `StatusSkipped`
- `Observer` interface with 6 callbacks + `noopObserver` default
- `safeCall` — panic-safe observer dispatch
- `CircuitState` enum (Closed/Open/HalfOpen) — placeholder for G1
- Options: `WithModel`, `WithAPIKey`, `WithTimeout`, `WithMaxDocs`, `WithMaxCharsPerDoc`, `WithObserver`, `WithHTTPClient`

### Preserved (G0)
- `New(cfg Config, logger *slog.Logger) *Client` — v1 constructor unchanged
- `Rerank(ctx, query, docs) []Scored` — v1 API unchanged
- `Available() bool` — unchanged
