# Changelog

## [0.97.5](https://github.com/anatolykoptev/go-kit/compare/v0.97.4...v0.97.5) (2026-07-23)


### Added

* **embed/sparse/rerank:** add WithRequireAuth to validate EMBED_TOKEN at startup ([fdedee9](https://github.com/anatolykoptev/go-kit/commit/fdedee9334adf752be789271cb0569994d8f6ccb))
* **embed/sparse/rerank:** add WithRequireAuth to validate EMBED_TOKEN at startup ([94d10b1](https://github.com/anatolykoptev/go-kit/commit/94d10b192f9dbc0968d5eb55df5f7b6eac133f61))
* **telegram:** add reusable Glossary primitive (STT name-correction + bolding) ([#225](https://github.com/anatolykoptev/go-kit/issues/225)) ([4933207](https://github.com/anatolykoptev/go-kit/commit/493320707aa67f8b48c32f6483e5eece7ded1da0))


### Fixed

* **breaker:** add panic guard to Execute to prevent half-open wedge ([9988199](https://github.com/anatolykoptev/go-kit/commit/998819979639abafb994a7eee55cae816ea0877e))
* **breaker:** add panic guard to Execute to prevent half-open wedge ([4102e75](https://github.com/anatolykoptev/go-kit/commit/4102e751e29f842ac2620ef723b2d7628e6bd86c))
* **cache:** expose L2Available() to detect silent Redis downgrade ([83831cc](https://github.com/anatolykoptev/go-kit/commit/83831cc5c3a5c76b9bbc33db1dae415e4fd7e5e1))
* **cache:** expose L2Available() to detect silent Redis downgrade ([27f8ffc](https://github.com/anatolykoptev/go-kit/commit/27f8ffc7bb82c1b5853eb13896d831eb3133aea0))
* **cache:** log L2 Set/Del failures at Warn and count them in l2errors ([373d901](https://github.com/anatolykoptev/go-kit/commit/373d9015e94194065fad9d42d4f627f2a5725d8b))
* **cache:** log L2 Set/Del failures at Warn and count them in l2errors ([64bac8a](https://github.com/anatolykoptev/go-kit/commit/64bac8ad83ddaac85cfc4ff6997e6d34c1f558ac))
* **cache:** prevent goroutine leak when Close() is never called ([a34715f](https://github.com/anatolykoptev/go-kit/commit/a34715fe470377ac768a8caf19b80ba358cac909))
* **cache:** prevent goroutine leak when Close() is never called ([edaebee](https://github.com/anatolykoptev/go-kit/commit/edaebeead5f4f79cef48a18f0a5b171e263a9abd))
* **llm:** defer cancelAttempt to guarantee context cancel on panic ([9017251](https://github.com/anatolykoptev/go-kit/commit/9017251ae1435f9c2cec27c11a480ce13cb58c16))
* **llm:** defer cancelAttempt to guarantee context cancel on panic in attemptEndpoint ([9f7f492](https://github.com/anatolykoptev/go-kit/commit/9f7f492eb2a96fbd3c0c70502d1c397178400366))
* **llm:** return ErrNoValidAPIKey when no key is configured ([ac6f153](https://github.com/anatolykoptev/go-kit/commit/ac6f1535e5b3c5d648d15d02334cfe96bba7813e))
* **llm:** return ErrNoValidAPIKey when no key is configured ([2f77a3c](https://github.com/anatolykoptev/go-kit/commit/2f77a3c87a5112a4a0ecb759ac3f87e6d97b37c3))
* log resp.Body.Close() errors instead of silently ignoring ([e41678c](https://github.com/anatolykoptev/go-kit/commit/e41678cd2a4678e051e854f30cad6ab497b183d4))
* **ratelimit:** prevent goroutine leak + bound map growth in KeyLimiter ([3c9c89b](https://github.com/anatolykoptev/go-kit/commit/3c9c89b165ba5c41493d9d76a3b183f2f6421629))
* **ratelimit:** prevent goroutine leak + bound map growth in KeyLimiter ([c54e396](https://github.com/anatolykoptev/go-kit/commit/c54e396ef6dbf896064c3576a6db7dbcc27e242b))
* replace nolint:errcheck on Body.Close() with slog.Warn across 9 files ([e6cd6c4](https://github.com/anatolykoptev/go-kit/commit/e6cd6c4f8a358771d7034e0d54544138f3becc94))

## [0.97.4](https://github.com/anatolykoptev/go-kit/compare/v0.97.3...v0.97.4) (2026-07-22)


### Documentation

* **pgutil:** document the -- soft migration marker on RunMigrations ([#190](https://github.com/anatolykoptev/go-kit/issues/190)) ([b6073d4](https://github.com/anatolykoptev/go-kit/commit/b6073d4888d24ebe52b36ca52cb38653d64e81c0))

## [0.97.3](https://github.com/anatolykoptev/go-kit/compare/v0.97.2...v0.97.3) (2026-07-20)


### Added

* **cli,watcher:** restore generic CLI cobra scaffold + file-watcher packages ([#184](https://github.com/anatolykoptev/go-kit/issues/184)) ([e22fa13](https://github.com/anatolykoptev/go-kit/commit/e22fa1339771ebef51766bf23b8f993a2065c6b5))

## [0.97.2](https://github.com/anatolykoptev/go-kit/compare/v0.97.1...v0.97.2) (2026-07-19)


### Added

* add langdetect package — trigram-based language detection ([#182](https://github.com/anatolykoptev/go-kit/issues/182)) ([deb2518](https://github.com/anatolykoptev/go-kit/commit/deb2518ab823bf5e0d408a24ee9d5c3cc2430747))

## [0.97.1](https://github.com/anatolykoptev/go-kit/compare/v0.97.0...v0.97.1) (2026-07-15)


### Added

* **llm:** multi-proxy endpoint builder for proxy-level redundancy ([#177](https://github.com/anatolykoptev/go-kit/issues/177)) ([bb14a9d](https://github.com/anatolykoptev/go-kit/commit/bb14a9df21ecee8641fcfeb7bf470541580abc51))

## [0.97.0](https://github.com/anatolykoptev/go-kit/compare/v0.96.0...v0.97.0) (2026-07-15)


### ⚠ BREAKING CHANGES

* **httputil:** Cache-Control is no longer set by default. Any handler relying on SecurityHeaders for implicit no-store must add an explicit WithCacheControl("no-store") or equivalent w.Header().Set call.

### Fixed

* **breaker:** recover-guard OnTrip/OnRecover callbacks — panic no longer crashes process ([#167](https://github.com/anatolykoptev/go-kit/issues/167)) ([352ab2d](https://github.com/anatolykoptev/go-kit/commit/352ab2df4432d83872bbb758c2684edc8548222c))
* **httputil:** drop implicit Cache-Control: no-store from SecurityHeaders ([#56](https://github.com/anatolykoptev/go-kit/issues/56)) ([82110a3](https://github.com/anatolykoptev/go-kit/commit/82110a306f70d9da1cd6adb402ac807654ef1daf))
* **llm:** advance fallback chain on HTTP client timeout without perAttemptTimeout ([#176](https://github.com/anatolykoptev/go-kit/issues/176)) ([daf3e36](https://github.com/anatolykoptev/go-kit/commit/daf3e366ef3eaafc9708c3a8fa729184fecd989a))
* **llm:** Stream() now applies reasoning_effort gating + perAttemptTimeout ([#169](https://github.com/anatolykoptev/go-kit/issues/169)) ([c81acd1](https://github.com/anatolykoptev/go-kit/commit/c81acd111b646603a7f724a2f85729a1700f6e9f))
* **render/html:** sniffImageMIME returns "" for unrecognized content — close content-type bypass ([#165](https://github.com/anatolykoptev/go-kit/issues/165)) ([cabcbd2](https://github.com/anatolykoptev/go-kit/commit/cabcbd2742fa04fdf48ce44d66f866c13c768938))
* **session/redis:** log + count Redis persist failures instead of silent swallow ([#168](https://github.com/anatolykoptev/go-kit/issues/168)) ([38d1fe3](https://github.com/anatolykoptev/go-kit/commit/38d1fe307512dec3aa03a310cbf29b03f4c493ff))
* widen fsm session flow/step columns from VARCHAR to TEXT ([#155](https://github.com/anatolykoptev/go-kit/issues/155)) ([af84d1a](https://github.com/anatolykoptev/go-kit/commit/af84d1aab184183544ca205b49bb25b3d2455620))

## [0.96.0](https://github.com/anatolykoptev/go-kit/compare/v0.95.2...v0.96.0) (2026-07-15)


### ⚠ BREAKING CHANGES

* **httputil:** Cache-Control is no longer set by default. Any handler relying on SecurityHeaders for implicit no-store must add an explicit WithCacheControl("no-store") or equivalent w.Header().Set call.

### Fixed

* **breaker:** recover-guard OnTrip/OnRecover callbacks — panic no longer crashes process ([#167](https://github.com/anatolykoptev/go-kit/issues/167)) ([352ab2d](https://github.com/anatolykoptev/go-kit/commit/352ab2df4432d83872bbb758c2684edc8548222c))
* **httputil:** drop implicit Cache-Control: no-store from SecurityHeaders ([#56](https://github.com/anatolykoptev/go-kit/issues/56)) ([82110a3](https://github.com/anatolykoptev/go-kit/commit/82110a306f70d9da1cd6adb402ac807654ef1daf))
* **llm:** Stream() now applies reasoning_effort gating + perAttemptTimeout ([#169](https://github.com/anatolykoptev/go-kit/issues/169)) ([c81acd1](https://github.com/anatolykoptev/go-kit/commit/c81acd111b646603a7f724a2f85729a1700f6e9f))
* **render/html:** sniffImageMIME returns "" for unrecognized content — close content-type bypass ([#165](https://github.com/anatolykoptev/go-kit/issues/165)) ([cabcbd2](https://github.com/anatolykoptev/go-kit/commit/cabcbd2742fa04fdf48ce44d66f866c13c768938))
* **session/redis:** log + count Redis persist failures instead of silent swallow ([#168](https://github.com/anatolykoptev/go-kit/issues/168)) ([38d1fe3](https://github.com/anatolykoptev/go-kit/commit/38d1fe307512dec3aa03a310cbf29b03f4c493ff))

## [0.95.2](https://github.com/anatolykoptev/go-kit/compare/v0.95.1...v0.95.2) (2026-07-13)


### Fixed

* widen fsm session flow/step columns from VARCHAR to TEXT ([#155](https://github.com/anatolykoptev/go-kit/issues/155)) ([af84d1a](https://github.com/anatolykoptev/go-kit/commit/af84d1aab184183544ca205b49bb25b3d2455620))

## [0.95.1](https://github.com/anatolykoptev/go-kit/compare/v0.95.0...v0.95.1) (2026-07-13)


### Added

* **httputil:** export DenyBlockedAddress + add SSRFGuards() with owned hop-cap (step 1/4) ([#151](https://github.com/anatolykoptev/go-kit/issues/151)) ([bbd818a](https://github.com/anatolykoptev/go-kit/commit/bbd818a159a9cb9b5eec701cc46f0f0ec7921172))


### Documentation

* add release-please guard rule to CLAUDE.md ([#148](https://github.com/anatolykoptev/go-kit/issues/148)) ([ec9fc24](https://github.com/anatolykoptev/go-kit/commit/ec9fc24382429b96668770e9e66097d5e950bded))
* translate CLAUDE.md rule to English ([#150](https://github.com/anatolykoptev/go-kit/issues/150)) ([a9dc2f6](https://github.com/anatolykoptev/go-kit/commit/a9dc2f64e20eb9506c0d6477e7a01269df69a40b))
* trim release-please guard rule to one line ([#149](https://github.com/anatolykoptev/go-kit/issues/149)) ([88bfc63](https://github.com/anatolykoptev/go-kit/commit/88bfc63b386093a0a3dc519fd501b3458301bd27))

## [0.95.0](https://github.com/anatolykoptev/go-kit/compare/v0.94.0...v0.95.0) (2026-07-01)


### Features

* **httputil:** export the SSRF guard as a single framework-owned primitive ([448f4bc](https://github.com/anatolykoptev/go-kit/commit/448f4bc6113b343c79d8cd5388e410ecee0d6121))

## [0.94.0](https://github.com/anatolykoptev/go-kit/compare/v0.93.0...v0.94.0) (2026-06-29)


### Features

* **typst:** gold-standard resume theme typography (margins, rhythm, ATS) ([44ccaf4](https://github.com/anatolykoptev/go-kit/commit/44ccaf49e92ac50656efb40fff841e673e3a3b84))
* **typst:** polish resume theme typography to gold-standard ([1b12d4e](https://github.com/anatolykoptev/go-kit/commit/1b12d4e36bd67df89a0c3c8c8ab683270c8a8443))

## [0.93.0](https://github.com/anatolykoptev/go-kit/compare/v0.92.1...v0.93.0) (2026-06-29)


### Features

* **render/typst:** add compact "resume" theme for dense single-page CVs ([25a5ddd](https://github.com/anatolykoptev/go-kit/commit/25a5dddb987d07a5f0959f331f7e505fffcc1349))
* **render/typst:** add compact "resume" theme for dense single-page CVs ([6bca740](https://github.com/anatolykoptev/go-kit/commit/6bca7405b4a0e6143704a6110a80e088ac12d662))


### Bug Fixes

* **render/typst:** resume theme US design review — Letter, left-align, wider margin, no footer ([041af28](https://github.com/anatolykoptev/go-kit/commit/041af28093a04fcafb87f05d9c02e10e2f85f4dc))

## [0.92.1](https://github.com/anatolykoptev/go-kit/compare/v0.92.0...v0.92.1) (2026-06-27)


### Bug Fixes

* **render:** concurrent printer rotation, drop redundant md sanitizer, cover-logo SSRF, env-rename fallback ([#139](https://github.com/anatolykoptev/go-kit/issues/139)) ([3d5379e](https://github.com/anatolykoptev/go-kit/commit/3d5379e6e32764a5b9b0fa2a7f4e4cbfaf42f815))

## [0.92.0](https://github.com/anatolykoptev/go-kit/compare/v0.91.1...v0.92.0) (2026-06-27)


### Features

* promote vaelor/pkg/pdf into go-kit/render as shared adapter ([#137](https://github.com/anatolykoptev/go-kit/issues/137)) ([5b46a9f](https://github.com/anatolykoptev/go-kit/commit/5b46a9f60d0cb1460a0f383812dfd3eebe8c4af0))

## [0.91.1](https://github.com/anatolykoptev/go-kit/compare/v0.91.0...v0.91.1) (2026-06-26)


### Bug Fixes

* **llm:** asFailover advances chain on model-not-found 422/400 ([#135](https://github.com/anatolykoptev/go-kit/issues/135)) ([a89335e](https://github.com/anatolykoptev/go-kit/commit/a89335e3b06913ec3ce330818ca9897be5c45c0a))

## [0.91.0](https://github.com/anatolykoptev/go-kit/compare/v0.90.0...v0.91.0) (2026-06-24)


### Features

* **telegram:** add notify package — governed AlertSink + product ProductSink ([#132](https://github.com/anatolykoptev/go-kit/issues/132)) ([4f9a182](https://github.com/anatolykoptev/go-kit/commit/4f9a182c8d48ae086a687839ca1a4f9e8cba3fbd))

## [0.90.0](https://github.com/anatolykoptev/go-kit/compare/v0.89.0...v0.90.0) (2026-06-20)


### Features

* **llm:** per-endpoint reasoning_effort allowlist gating ([#131](https://github.com/anatolykoptev/go-kit/issues/131)) ([09853f1](https://github.com/anatolykoptev/go-kit/commit/09853f1f69c0dd8ac2c3f6bcca608184d82df825))

## [0.89.0](https://github.com/anatolykoptev/go-kit/compare/v0.88.0...v0.89.0) (2026-06-20)


### Bug Fixes

* **llm:** empty completion fails over instead of silently returning "" ([#130](https://github.com/anatolykoptev/go-kit/issues/130)) ([c7c4ee1](https://github.com/anatolykoptev/go-kit/commit/c7c4ee1f8077619ad8479193c5f16eb1c0e3d8f0))

## [0.88.0](https://github.com/anatolykoptev/go-kit/compare/v0.87.0...v0.88.0) (2026-06-20)


### Features

* **llm:** weighted model selection strategy ([#129](https://github.com/anatolykoptev/go-kit/issues/129)) ([b6c984c](https://github.com/anatolykoptev/go-kit/commit/b6c984ccb10c5d0e7873e9260b0345e209936ca4))

## [0.87.0](https://github.com/anatolykoptev/go-kit/compare/v0.86.0...v0.87.0) (2026-06-20)


### Features

* **llm:** randomized model selection strategy ([#128](https://github.com/anatolykoptev/go-kit/issues/128)) ([89e3757](https://github.com/anatolykoptev/go-kit/commit/89e3757397681eddc2cf1a0861ffd4a0ef823765))

## [0.86.0](https://github.com/anatolykoptev/go-kit/compare/v0.85.0...v0.86.0) (2026-06-20)


### Features

* **llm:** ClassifyErrorType + error_type label on llm_chain_attempt_total ([#127](https://github.com/anatolykoptev/go-kit/issues/127)) ([a4b76fd](https://github.com/anatolykoptev/go-kit/commit/a4b76fd4ac84deb606aaf3cb4724a758b5a0837b))

## [0.85.0](https://github.com/anatolykoptev/go-kit/compare/v0.84.0...v0.85.0) (2026-06-24)


### Bug Fixes

* **llm:** authenticate ModelRegistry /v1/models fetch (health-aware chain was dark -- proxy 401) ([#125](https://github.com/anatolykoptev/go-kit/issues/125)) ([0f209a0](https://github.com/anatolykoptev/go-kit/commit/0f209a02be3819b8dfa344d2a0f45b862cab255c))

## [0.84.0](https://github.com/anatolykoptev/go-kit/compare/v0.83.0...v0.84.0) (2026-06-18)


### Features

* **redirectmatch:** storage-free redirect matching primitive ([#123](https://github.com/anatolykoptev/go-kit/issues/123)) ([85f86e9](https://github.com/anatolykoptev/go-kit/commit/85f86e9f8344592f5d4a9cef2730516daadc4aff))

## [0.83.0](https://github.com/anatolykoptev/go-kit/compare/v0.82.0...v0.83.0) (2026-06-12)


### Features

* **llm:** health-aware model-chain filtering against live /v1/models ([#120](https://github.com/anatolykoptev/go-kit/issues/120)) ([0c58419](https://github.com/anatolykoptev/go-kit/commit/0c58419ecf1d82a046071db48129e3e97d72c2a8))

## [0.82.0](https://github.com/anatolykoptev/go-kit/compare/v0.81.1...v0.82.0) (2026-06-11)


### Features

* **llm:** per-model quota-aware cooldown for fallback chain (P1) ([#117](https://github.com/anatolykoptev/go-kit/issues/117)) ([23dfc4f](https://github.com/anatolykoptev/go-kit/commit/23dfc4fcff5ee86a0b23d566421b4724bb80e29f))
* **llm:** served-model observability for fallback chain (P2) ([#119](https://github.com/anatolykoptev/go-kit/issues/119)) ([23769c5](https://github.com/anatolykoptev/go-kit/commit/23769c5ca3237187ddf04bc1793127e0b511f31f))

## [0.81.1](https://github.com/anatolykoptev/go-kit/compare/v0.81.0...v0.81.1) (2026-06-09)


### Bug Fixes

* **llm:** advance model-fallback chain on 413 / context-length errors ([#115](https://github.com/anatolykoptev/go-kit/issues/115)) ([95c814e](https://github.com/anatolykoptev/go-kit/commit/95c814ea69305b66617e4b6ca4d540d2dadbb429))

## [0.81.0](https://github.com/anatolykoptev/go-kit/compare/v0.80.0...v0.81.0) (2026-06-06)


### Features

* **admintable:** ILike Match for case-insensitive multi-column search ([#114](https://github.com/anatolykoptev/go-kit/issues/114)) ([fd76e51](https://github.com/anatolykoptev/go-kit/commit/fd76e510fc1126bd045da9ac055c292548421820))


### Documentation

* **admintable:** complete the Where godoc example + note AnyOf empty-element behavior ([#112](https://github.com/anatolykoptev/go-kit/issues/112)) ([6b68828](https://github.com/anatolykoptev/go-kit/commit/6b68828481cc6a481b7e7a752589c37baf8e8934))

## [0.80.0](https://github.com/anatolykoptev/go-kit/compare/v0.79.0...v0.80.0) (2026-06-06)


### Features

* **admintable:** SQL-safe declarative filter-allowlist (FilterSpec) ([#109](https://github.com/anatolykoptev/go-kit/issues/109)) ([a3ffa5b](https://github.com/anatolykoptev/go-kit/commit/a3ffa5b93c396747d7bf7ba8aad1d3cc35e13157))

## [0.77.0](https://github.com/anatolykoptev/go-kit/compare/v0.76.0...v0.77.0) (2026-05-30)


### Features

* **llm:** WithPerAttemptTimeout bounds each endpoint attempt in the chain ([#104](https://github.com/anatolykoptev/go-kit/issues/104)) ([3e5e80d](https://github.com/anatolykoptev/go-kit/commit/3e5e80d4f0fe2c6c8e4a2409f2b86e1338e0fa42))

## [Unreleased]

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

* **httputil:** `SecurityHeaders` default CSP now includes `'self'` in
  `style-src` (`default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`).
  The previous default omitted `'self'`, permitting only inline `<style>` blocks and
  blocking every `<link rel="stylesheet">` regardless of origin — including the
  service's own `/static/*.css`. Discovered on go-nerv admin (2026-05-22): pm7.css
  served 200+text/css but the browser silently cancelled the stylesheet, leaving
  every admin page unstyled. Pure RELAXATION (adds `'self'`); consumers that
  override via `WithCSP(...)` are unaffected.

### Added

* **metrics:** `Registry.RegisterHistogram(name string, opts ...HistogramOption)` and
  `metrics.WithBuckets([]float64)` — per-metric bucket configuration. Call
  `RegisterHistogram` before the first `Observe` to opt into custom bucket boundaries
  (e.g. byte ranges, queue depth). Unconfigured histograms keep the seconds-shaped
  default (`ExponentialBuckets(0.001, 2, 16)`) — fully backwards compatible.

  ```go
  reg.RegisterHistogram("gojob_oversize_bytes",
      metrics.WithBuckets([]float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304}))
  reg.Observe("gojob_oversize_bytes", float64(len(payload)))
  ```

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

## [v0.65.0] — 2026-05-19

### Added

* **telegram/botusers:** New package for tracking Telegram bot users across bots,
  with multi-tenancy, privacy controls, keyset pagination, and GDPR forget support.

  Core interfaces and types:
  - `Store` interface: `UpsertFromInitData`, `UpsertFromCommand`, `Get`, `List`,
    `Aggregate`, `Forget`, `DeleteInactive` — all scoped by `(bot_id, tg_id)` PK
  - `Privacy` enum: `Off` (no-op), `SoftOptIn` (default); `HardOptIn` planned for a future release
  - `Cursor` — opaque keyset pagination token (`last_seen_at DESC, tg_id ASC`)
  - `SchemaSQL()` — embedded DDL via `go:embed`; idempotent (`IF NOT EXISTS`)

  Sub-packages:
  - `telegram/botusers/pg` — PostgreSQL implementer via pgxpool; `pg.New` applies
    schema on first call; `Apply` is safe to call again for upgrades
  - `telegram/botusers/botuserstest` — `RunContract(t, newStore)` contract suite +
    `MemStore` in-memory reference implementation

  Helpers:
  - `RetentionSweeper.Run(ctx)` — caller-managed sweep loop; no background goroutines
  - `EmitGauges` — emits `bot_users.{total,active_1d,active_7d,active_30d}` via
    a `MetricsEmitter` interface; caller controls frequency

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

* Spec: internal design spec (telegram bot pkg v0.1)
* Research: internal research (tg bot lib landscape)

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
