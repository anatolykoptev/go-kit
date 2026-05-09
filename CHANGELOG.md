# Changelog

## [0.38.0](https://github.com/anatolykoptev/go-kit/compare/v0.37.0...v0.38.0) (2026-05-09)


### Features

* **embed:** client-side chunking on shared Client (lift from memdb-go HTTPEmbedder) ([#48](https://github.com/anatolykoptev/go-kit/issues/48)) ([d3ebe14](https://github.com/anatolykoptev/go-kit/commit/d3ebe1413868e1b272702ad7f3e0a47e8ad6d940))
* **hedge:** DoFallback for primary→fallback chains ([aa0a636](https://github.com/anatolykoptev/go-kit/commit/aa0a636831b54b3f18e1fbfe6d0754d396aea26c))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([a8b0919](https://github.com/anatolykoptev/go-kit/commit/a8b09198e96be490e96dbde6f7799a5017c434e7))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([c84187e](https://github.com/anatolykoptev/go-kit/commit/c84187ef872c7972226f1e7a823fd62f8ac5cab8))
* **llm:** MemDB-aligned message metadata + WithMessageTimestamps option ([41e7cb8](https://github.com/anatolykoptev/go-kit/commit/41e7cb83343ef72e2c9972bf4855cf563460aff5))
* **llm:** prompt caching support — Anthropic cache_control + universal cache token visibility ([f6a6b8a](https://github.com/anatolykoptev/go-kit/commit/f6a6b8aa7ea510242cf8321bdf25ddaf72234d96))
* **rerank:** VoyageRerankClient + JinaRerankClient + llm.ExtractJSON fix ([#39](https://github.com/anatolykoptev/go-kit/issues/39)) ([9e6a4bb](https://github.com/anatolykoptev/go-kit/commit/9e6a4bbc90b21f12fb6900a1ca1c64221e67c7c8))
* **rerank:** WithFallback now accepts any Reranker (not just *Client) ([#41](https://github.com/anatolykoptev/go-kit/issues/41)) ([98c6678](https://github.com/anatolykoptev/go-kit/commit/98c66781cb036253fbf8bd1dab6570791fc43185))
* **score:** add 5-tier Severity vocabulary + helpers ([#46](https://github.com/anatolykoptev/go-kit/issues/46)) ([97cd9b5](https://github.com/anatolykoptev/go-kit/commit/97cd9b563fca5166d0a8221c4743e6a31f8ae601))
* **score:** new package — ConfidenceFromScore + generic Bucket ([#45](https://github.com/anatolykoptev/go-kit/issues/45)) ([7ef7cca](https://github.com/anatolykoptev/go-kit/commit/7ef7ccad9a35a5b0e4bbca8ee1fb61063a634cae))
* **session:** integrate go-session into go-kit ([01b9938](https://github.com/anatolykoptev/go-kit/commit/01b9938fa2f95bd06988475e95592ad25a599e6e))
* **telegram:** smart HTML sanitizer + format auto-detect + PrepareForTelegram ([#43](https://github.com/anatolykoptev/go-kit/issues/43)) ([96fac07](https://github.com/anatolykoptev/go-kit/commit/96fac07d8d092643b15149a2b586a520aec8e584))
* **tracing/httpmw:** drop-in *Mux wrapper + chi.Walk auto-registration ([#51](https://github.com/anatolykoptev/go-kit/issues/51)) ([f9f60d4](https://github.com/anatolykoptev/go-kit/commit/f9f60d4a14888a083548c239587cd180820a2cf2))
* **tracing/httpmw:** emit OTEL code.* attributes from handler reflection ([#49](https://github.com/anatolykoptev/go-kit/issues/49)) ([12d3ba3](https://github.com/anatolykoptev/go-kit/commit/12d3ba38314dad69ad0549837290e3eebbf59f7b))
* **tracing/httpmw:** gin route adapter + multi-framework walk docstrings ([#52](https://github.com/anatolykoptev/go-kit/issues/52)) ([fc3ef08](https://github.com/anatolykoptev/go-kit/commit/fc3ef08a5d9a3c8515080d516fa6ebf20ae7b95f))
* **tracing/httpmw:** startup-time route registry for OTEL code.* attrs ([1e6fe48](https://github.com/anatolykoptev/go-kit/commit/1e6fe4868e2dd5a0b2463a57ef59fd0aeb6de972))
* **tracing/httpmw:** startup-time route registry for OTEL code.* attrs ([2e13509](https://github.com/anatolykoptev/go-kit/commit/2e13509c6a49a2e0eb280c5c16585978db204aaf))
* **tracing:** correctness polish — Start scope, bad-endpoint graceful, typed mcp params, nil-ctx, shutdown doc ([#47](https://github.com/anatolykoptev/go-kit/issues/47)) ([7f17ada](https://github.com/anatolykoptev/go-kit/commit/7f17ada8522dfe61f86abd5d87a47a10ba268868))


### Bug Fixes

* **cache:** embed role + rerank truncation cap in cache keys (PR C — v0.39.0) ([#40](https://github.com/anatolykoptev/go-kit/issues/40)) ([c5fcc99](https://github.com/anatolykoptev/go-kit/commit/c5fcc990b33ae1df7b7c25915ebbd884e7e74501))
* **metrics:** prevent panic on prom_bridge shape collisions ([#37](https://github.com/anatolykoptev/go-kit/issues/37)) ([c8a4f2e](https://github.com/anatolykoptev/go-kit/commit/c8a4f2e3f4f7819c410d07d63c84fd93ea60e80d))
* **tracing/httpmw:** stdlibFormatter must not duplicate the HTTP method ([5e37087](https://github.com/anatolykoptev/go-kit/commit/5e370871984f3aa0b6f9a3e6f21d3ddb3ed78a2f))

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
