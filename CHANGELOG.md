# Changelog

## [Unreleased]

### Added

* **metrics:** `Registry.Observe(name string, v float64)` — direct histogram write,
  routes to the prom bridge (Prometheus-backed registry) or in-mem Reservoir
  (in-memory registry). Matches the `StartTimer.Stop()` routing invariant.

* **metrics:** `Registry.ObserveSeconds(name string, d time.Duration)` — duration
  sugar over `Observe(name, d.Seconds())`; preferred for latency measurements where
  `time.Duration` is already in hand. Closes the gap that forced go-code PR #121
  to fall back to direct `prometheus.NewHistogramVec` for LLM-latency observability.

  ```go
  start := time.Now()
  resp, err := llm.Complete(ctx, req)
  reg.ObserveSeconds(metrics.Label("llm_request_seconds", "outcome", classify(err)), time.Since(start))
  ```

### Breaking Changes

* **httputil:** `SecurityHeaders` no longer sets `Cache-Control: no-store` by default.
  Cache policy is orthogonal to security headers — marketing pages, API endpoints,
  and authed admin pages have fundamentally different caching requirements. Setting
  a blanket `no-store` for every response forces cold round-trips on public pages
  (observed: oxpulse-admin landing at partners.oxpulse.chat, 520–1025ms TTFB on
  warm connections vs 44ms expected with browser caching).

  **Migration:** callers that relied on the implicit `no-store` must add an explicit
  declaration. Two options:

  ```go
  // Option A: via SecurityHeaders option (single call covers both concerns)
  httputil.SecurityHeaders(w, httputil.WithCacheControl("no-store"))

  // Option B: direct header set
  w.Header().Set("Cache-Control", "no-store")
  ```

  **Known downstream consumers requiring migration:**
  - `go-nerv/internal/admin/auth.go:67` — calls `httputil.SecurityHeaders(w)`; authed admin, add `WithCacheControl("no-store")`
  - `oxpulse-admin/internal/admin/auth.go:120` — calls `httputil.SecurityHeaders(w, httputil.WithCSP(...))`; authed admin, add `WithCacheControl("no-store")`

### Fixed

* **embed:** `cacheKey` now includes role (query/passage) — fixes silent collision where `EmbedQuery(text)` and `Embed([text])` shared cache entries on HTTP backend despite server-side role prefixing.
* **rerank:** `cacheKey` now includes maxCharsPerDoc + maxTokensPerDoc — config bumps no longer return stale scores.

### Features

* **rerank:** VoyageRerankClient — Voyage AI rerank-2.5 client mirroring embed/voyage.go (retries on 429/5xx, StatusSkipped on missing API key, WithTopN forwards to top_k).

## [v0.61.2] — 2026-05-18

### Fixed

* **telegram/miniapp:** `ValidateInitData` was incorrectly excluding the `signature` field (Bot API 7.x+ ed25519) from the HMAC `data_check_string`. Telegram includes `signature` in the signed payload — only `hash` itself is excluded. Confirmed against the OvyFlash/telegram-bot-api reference impl (`ValidateWebAppData` in helper_methods.go). The bug surfaced as 100% `ErrInvalidSignature` failures on iOS Bot API 9.6+ Mini App share-flow (incident 2026-05-18, oxpulse-admin debug-trace branch). Regression test added: `TestValidate_WithSignatureField_PassesHMAC`.

## [v0.61.1] — 2026-05-18

### Fixed

* **telegram/tgapi5:** `BotInvoiceSender.SendInvoice/CreateInvoiceLink`, `BotWebAppAnswerer.AnswerWebAppQuery`, and `BotPreparedSender.SavePreparedInlineMessage` now return a typed error (`"tgapi5: nil response from BotAPI"`) instead of panicking if the SDK's `RequestWithContext` ever returned `(nil, nil)`. Defence-in-depth — the SDK contract does not formally forbid that return shape.

### Improved

* **telegram/miniapp:** `PreparedSender.SavePreparedInlineMessage` godoc enumerates the 20 acceptable `tgbotapi.InlineQueryResult*` concrete types, recovering the compile-time discoverability lost to the SDK's `type InlineQueryResult any` alias.

## [v0.61.0] — 2026-05-18

### Added

* **telegram/miniapp:** `SavePrepared` helper + `PreparedSender` interface — wraps Bot API 7.8+ `savePreparedInlineMessage` with sentinel-error validation (userID, result nil, no-chat-type-allowed).
* **telegram/tgapi5:** `BotPreparedSender` adapter + `NewPreparedSender` constructor — honours context via `MakeRequestWithContext`.

  **Implementation note:** the adapter bypasses the SDK's generic `tgbotapi.SavePreparedInlineMessage[T]` helper and builds `tgbotapi.Params` manually, because `tgbotapi.InlineQueryResult` (= `any`) does not satisfy the `InlineQueryResults` type-union constraint at compile time. The manual Params body mirrors `SavePreparedInlineMessageConfig[T].params()` byte-for-byte; behaviour is identical.

## [v0.57.1] — 2026-05-16

### Fixed

* **telegram/cmd:** `Router.AutoHelp` API contradiction — doc said "wrap or
  replace" the no-op handler but `On()` panics on dup-registration, making
  replace impossible. AutoHelp now takes the Handler directly:
  `r.AutoHelp("/help", h)`. Caller controls send; HelpText generation +
  helpCmd tracking stay in Router. Breaking change vs v0.57.0; only known
  caller (oxpulse-admin) already worked around with bare On in hotfix (PR #43).

## [v0.56.1] -- 2026-05-16

### Fixed

* **telegram/kb:** Registry -- add sync.RWMutex around keyboards map.
  Register takes write lock; Dispatch takes read lock. Prevents data race
  and concurrent-map-write panic under concurrent update dispatch. (H1)
* **telegram/fsm:** Machine.Feed -- per-chatID sync.Mutex around the full
  Get->execute->Put window. Prevents TOCTOU race on duplicate Telegram delivery
  for the same chatID. chatLocks entries accumulate indefinitely (one per
  unique chatID); acceptable at our scale. (H2)
* **telegram/fsm:** store_postgres.go -- explicit errors.Is(err, pgx.ErrNoRows)
  guard replaces //nolint:nilerr that silently swallowed context.DeadlineExceeded
  and other errors as "session not found". (M1)
* **telegram/fsm:** funcName -- use runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
  instead of fmt.Sprintf("%p", fn). Returns stable symbol name across recompiles;
  prevents silent flow restart on every deploy. Anonymous closures produce
  .func1/.func2 suffixes -- prefer named StateFns for long-lived Postgres flows. (M3)

### Added

* CHANGELOG: add missing v0.56.0 entry. (M2)

## [v0.56.0] -- 2026-05-16

### Added

* **telegram:** new Locale (YAML i18n) at root pkg level -- vendored from
  tucnak/telebot/v4/layout (MIT). See LICENSE.telebot.md.
* **telegram/kb:** new subpkg -- inline keyboard builder + Registry,
  vendored from go-telegram/ui/keyboard/inline (MIT). See LICENSE.go-telegram.
* **telegram/middleware:** new subpkg -- 7 composable middlewares (AutoRespond,
  DeletePrev, OperatorOnly, RateLimit, ShadowBan, Metrics, Recover) + Chain
  primitive. Reuses go-kit/ratelimit.KeyLimiter.
* **telegram/fsm:** new subpkg -- conversation Machine + MemoryStore + PostgresStore
  (echotron-style one-assignment StateFn idiom).

### Added (transitive dep)

* github.com/go-telegram-bot-api/telegram-bot-api/v5 -- first go-kit subpkg
  requiring it; used by kb / middleware / fsm.

### Reference

* Spec: ~/deploy/krolik-server/reports/go-kit/architecture/2026-05-16-telegram-bot-pkg-v0.1.md
* Research: ~/deploy/krolik-server/reports/go-kit/research/2026-05-16-tg-bot-lib-landscape.md

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
