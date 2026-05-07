# Changelog

## [0.38.0](https://github.com/anatolykoptev/go-kit/compare/v0.37.0...v0.38.0) (2026-05-07)


### Features

* **hedge:** DoFallback for primary→fallback chains ([b1461de](https://github.com/anatolykoptev/go-kit/commit/b1461dee059c60de417ce0abb907f6168a89b22a))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([a6f7999](https://github.com/anatolykoptev/go-kit/commit/a6f7999baa57059a43ad4398d11f4b05d6fbd910))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([822abbb](https://github.com/anatolykoptev/go-kit/commit/822abbb1e62d207bb76a652b894685759010caa5))
* **llm:** MemDB-aligned message metadata + WithMessageTimestamps option ([ed534b8](https://github.com/anatolykoptev/go-kit/commit/ed534b8b85ea1dccaf22e803b7ab8412b7483fba))
* **llm:** prompt caching support — Anthropic cache_control + universal cache token visibility ([801284c](https://github.com/anatolykoptev/go-kit/commit/801284cfce854e881e7349e25acac7b61e3212dd))
* **rerank:** VoyageRerankClient + JinaRerankClient + llm.ExtractJSON fix ([#39](https://github.com/anatolykoptev/go-kit/issues/39)) ([a88a3b6](https://github.com/anatolykoptev/go-kit/commit/a88a3b6fb57f1c995351661d6c4e466c9c5e789e))
* **rerank:** WithFallback now accepts any Reranker (not just *Client) ([#41](https://github.com/anatolykoptev/go-kit/issues/41)) ([09f795a](https://github.com/anatolykoptev/go-kit/commit/09f795a0ee32f4f0ceaf18894cfae1f2e2e0dc58))
* **session:** integrate go-session into go-kit ([aec99c6](https://github.com/anatolykoptev/go-kit/commit/aec99c6ee59226120857d4f0166c71045bcd05bf))
* **telegram:** smart HTML sanitizer + format auto-detect + PrepareForTelegram ([#43](https://github.com/anatolykoptev/go-kit/issues/43)) ([aefb5c0](https://github.com/anatolykoptev/go-kit/commit/aefb5c0da79ada181e185ec57d71a3de191922ec))


### Bug Fixes

* **cache:** embed role + rerank truncation cap in cache keys (PR C — v0.39.0) ([#40](https://github.com/anatolykoptev/go-kit/issues/40)) ([581b2aa](https://github.com/anatolykoptev/go-kit/commit/581b2aa03a538bc2bba0d1c3825fdb0aff82eafb))
* **metrics:** prevent panic on prom_bridge shape collisions ([#37](https://github.com/anatolykoptev/go-kit/issues/37)) ([1aea60c](https://github.com/anatolykoptev/go-kit/commit/1aea60cfa1957f47b46f31653d316ccbfc87ee40))
* **tracing/httpmw:** stdlibFormatter must not duplicate the HTTP method ([48a5152](https://github.com/anatolykoptev/go-kit/commit/48a5152d6940c6c8dcb0e7c6514278bb0ad2f244))

## [Unreleased]

### Fixed

* **embed:** `cacheKey` now includes role (query/passage) — fixes silent collision where `EmbedQuery(text)` and `Embed([text])` shared cache entries on HTTP backend despite server-side role prefixing.
* **rerank:** `cacheKey` now includes maxCharsPerDoc + maxTokensPerDoc — config bumps no longer return stale scores.

### Features

* **rerank:** VoyageRerankClient — Voyage AI rerank-2.5 client mirroring embed/voyage.go (retries on 429/5xx, StatusSkipped on missing API key, WithTopN forwards to top_k).

## [0.37.0](https://github.com/anatolykoptev/go-kit/compare/v0.36.0...v0.37.0) (2026-05-01)


### Features

* **cache:** WithMetrics opt-in Prometheus auto-emission via CounterFunc ([#29](https://github.com/anatolykoptev/go-kit/issues/29)) ([7ac814e](https://github.com/anatolykoptev/go-kit/commit/7ac814eb5a25e83f91d09bbeed8578aae599b5c8))
* **llm:** CircuitBreaker + Cache middleware; fix(embed): cache-bypass on Client.Embed ([#35](https://github.com/anatolykoptev/go-kit/issues/35)) ([b7985aa](https://github.com/anatolykoptev/go-kit/commit/b7985aad9a1594853dc5b14ebb35e71eaec2cdcd))
* **rerank:** fusion palette — WeightedRRF + DBSF + LinearMinMax ([#28](https://github.com/anatolykoptev/go-kit/issues/28)) ([b3b792e](https://github.com/anatolykoptev/go-kit/commit/b3b792eb497d36722fd909606bd2dd24ab26c59d))
* RRF helper + Cache Validator hook (promote from go-code) ([#27](https://github.com/anatolykoptev/go-kit/issues/27)) ([9f721d6](https://github.com/anatolykoptev/go-kit/commit/9f721d6792f428332adc4fb7007132c867f37ead))
* **sparse:** SPLADE sparse-embedding client mirroring embed/ conventions ([#36](https://github.com/anatolykoptev/go-kit/issues/36)) ([b555559](https://github.com/anatolykoptev/go-kit/commit/b5555598c48cc40830d7614eb9e6d49ff07a610a))
* **tracing/httpmw:** outgoing HTTP client wrapper ([#33](https://github.com/anatolykoptev/go-kit/issues/33)) ([2138ad3](https://github.com/anatolykoptev/go-kit/commit/2138ad329cb937c9347b037c19ab58f9df3d018c))
* **tracing/pgxotel:** pgx pool tracing via canonical otelpgx ([#34](https://github.com/anatolykoptev/go-kit/issues/34)) ([e797123](https://github.com/anatolykoptev/go-kit/commit/e7971238bcbab46b7becd28cb6fbc396e216f1e3))
* **tracing/slogh:** slog handler with auto trace_id injection ([#32](https://github.com/anatolykoptev/go-kit/issues/32)) ([d0e8458](https://github.com/anatolykoptev/go-kit/commit/d0e8458924a502cbb4d8e4510e30788cb457cbfa))
* **tracing:** OTel distributed tracing helpers + MCP middleware ([#30](https://github.com/anatolykoptev/go-kit/issues/30)) ([7afcf2d](https://github.com/anatolykoptev/go-kit/commit/7afcf2d68b501b4eb875a7d7dc0c844889cd3862))


### Bug Fixes

* **embed:** close two HTTP backend correctness gaps (G6+G7) ([#24](https://github.com/anatolykoptev/go-kit/issues/24)) ([1a4c53f](https://github.com/anatolykoptev/go-kit/commit/1a4c53ffce747b85a3b859df00432a7a8186609d))
* **tracing:** use WithEndpointURL for canonical OTel env semantics ([#31](https://github.com/anatolykoptev/go-kit/issues/31)) ([4587484](https://github.com/anatolykoptev/go-kit/commit/4587484032616bb696306360b7221a54ea8d8936))


### Documentation

* **embed:** WithDim mentions runtime validation behavior ([#26](https://github.com/anatolykoptev/go-kit/issues/26)) ([9a46f8b](https://github.com/anatolykoptev/go-kit/commit/9a46f8b911641d83a8f4369651d1ad263277bef0))

## [0.30.0](https://github.com/anatolykoptev/go-kit/compare/v0.29.0...v0.30.0) (2026-04-29)


### Features

* **embed:** E0 foundation — options, Result, hooks, ONNX-via-WithEmbedder (M14) ([#20](https://github.com/anatolykoptev/go-kit/issues/20)) ([9018988](https://github.com/anatolykoptev/go-kit/commit/90189885bdbf77365b374bee944d4079105c2555))
* **embed:** E1 resiliency — CircuitBreaker, RetryPolicy, multi-model fallback (M14) ([#22](https://github.com/anatolykoptev/go-kit/issues/22)) ([73863ea](https://github.com/anatolykoptev/go-kit/commit/73863ea4a19254e5489269c8369d89bed9cc041e))
* **embed:** E3 cache — pluggable Cache interface (M14) ([#23](https://github.com/anatolykoptev/go-kit/issues/23)) ([549ffff](https://github.com/anatolykoptev/go-kit/commit/549ffff78dea23fcdb5ad494369a74aabd0935a8))

## [0.29.0](https://github.com/anatolykoptev/go-kit/compare/v0.28.0...v0.29.0) (2026-04-29)


### Features

* **rerank:** G5 MathReranker — cosine + MMR (post-M12.10 bonus) ([#18](https://github.com/anatolykoptev/go-kit/issues/18)) ([40a5d14](https://github.com/anatolykoptev/go-kit/commit/40a5d14a137810aa95c858419e93657579ec8d5d))

## [0.28.0](https://github.com/anatolykoptev/go-kit/compare/v0.27.0...v0.28.0) (2026-04-29)


### Features

* **embed/onnx:** split ONNX runtime backend into cgo-only subpackage ([e74495a](https://github.com/anatolykoptev/go-kit/commit/e74495aafe13821a4ed088216a099b014fb72417))
* **embed:** extract memdb-go embedder into go-kit/embed shared package ([#6](https://github.com/anatolykoptev/go-kit/issues/6)) ([e144435](https://github.com/anatolykoptev/go-kit/commit/e144435dc398a1a2a9b0a3591101a613553e0d9a))

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
