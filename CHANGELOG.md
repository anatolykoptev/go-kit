# Changelog

## [0.38.0](https://github.com/anatolykoptev/go-kit/compare/v0.37.0...v0.38.0) (2026-05-26)


### ⚠ BREAKING CHANGES

* **telegram/kb:** Cache-Control is no longer set by default. Any handler relying on SecurityHeaders for implicit no-store must add an explicit WithCacheControl("no-store") or equivalent w.Header().Set call.

### Features

* **embed,sparse:** WithBearerToken option ([ccf90ce](https://github.com/anatolykoptev/go-kit/commit/ccf90cee7b198996113f010af1140580f71dad40))
* **embed,sparse:** WithBearerToken option for self-hosted endpoints ([feeb59c](https://github.com/anatolykoptev/go-kit/commit/feeb59ce846340c0e0514016c2a5c79d59e2478b))
* **embed:** client-side chunking on shared Client (lift from memdb-go HTTPEmbedder) ([#48](https://github.com/anatolykoptev/go-kit/issues/48)) ([d3ebe14](https://github.com/anatolykoptev/go-kit/commit/d3ebe1413868e1b272702ad7f3e0a47e8ad6d940))
* **embed:** v2 NewClient auto-resolves EMBED_TOKEN env ([15c01f7](https://github.com/anatolykoptev/go-kit/commit/15c01f79693b13e0958dbe9dda05badc7776d0ac))
* **embed:** v2 NewClient auto-resolves EMBED_TOKEN env for bearer auth ([0d0e5d1](https://github.com/anatolykoptev/go-kit/commit/0d0e5d17c8155e6baabd0725328a46096e164816))
* **hedge:** DoFallback for primary→fallback chains ([aa0a636](https://github.com/anatolykoptev/go-kit/commit/aa0a636831b54b3f18e1fbfe6d0754d396aea26c))
* **httputil:** ClientIP with trusted-header validation ([b3bd98e](https://github.com/anatolykoptev/go-kit/commit/b3bd98e86fd1f95282aa6f3ff6af6192dde3ef8f))
* **httputil:** SecurityHeaders with CSP option ([263edee](https://github.com/anatolykoptev/go-kit/commit/263edee14ff5aaa0a853c4195a2ea57177fb0a6a))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([a8b0919](https://github.com/anatolykoptev/go-kit/commit/a8b09198e96be490e96dbde6f7799a5017c434e7))
* **llm/budget:** token usage tracker with warn/switch/hard-stop tiers ([c84187e](https://github.com/anatolykoptev/go-kit/commit/c84187ef872c7972226f1e7a823fd62f8ac5cab8))
* **llm:** add ParseModelFallbackChain + BuildModelChainEndpoints helpers ([61c1a3f](https://github.com/anatolykoptev/go-kit/commit/61c1a3f6dd4505e1f675ca636df00557b56fc696))
* **llm:** Completer + NoOp + NewOptional for opt-in consumers ([#83](https://github.com/anatolykoptev/go-kit/issues/83)) ([2811e06](https://github.com/anatolykoptev/go-kit/commit/2811e06e5a629bfcfc2adb644574bf8f3c98a80d))
* **llm:** MemDB-aligned message metadata + WithMessageTimestamps option ([41e7cb8](https://github.com/anatolykoptev/go-kit/commit/41e7cb83343ef72e2c9972bf4855cf563460aff5))
* **llm:** parse Retry-After header into APIError.RetryAfter ([#85](https://github.com/anatolykoptev/go-kit/issues/85)) ([fa2697c](https://github.com/anatolykoptev/go-kit/commit/fa2697c1e519ca8cab6b69e4f1673827196fc429))
* **llm:** ParseModelFallbackChain + BuildModelChainEndpoints helpers ([d17261f](https://github.com/anatolykoptev/go-kit/commit/d17261f24b5d32cc423d90010bf59aeb4522d429))
* **llm:** prompt caching support — Anthropic cache_control + universal cache token visibility ([f6a6b8a](https://github.com/anatolykoptev/go-kit/commit/f6a6b8aa7ea510242cf8321bdf25ddaf72234d96))
* **llm:** WithChatModel ChatOption (per-call override) ([96d42b0](https://github.com/anatolykoptev/go-kit/commit/96d42b0933bce3f43b6bb3844a40fa183896d6fa))
* **llm:** WithChatModel ChatOption for per-call model override ([514a64c](https://github.com/anatolykoptev/go-kit/commit/514a64c382e38b1d3d5c9c0bef01400df657215d))
* **llm:** WithEndpointAttemptObserver Option ([fc96b7e](https://github.com/anatolykoptev/go-kit/commit/fc96b7e494bf98e55ed3c12ccc4b4c63a03ccf40))
* **llm:** WithEndpointAttemptObserver Option for per-endpoint observability ([ff4a9d4](https://github.com/anatolykoptev/go-kit/commit/ff4a9d417da320f01f500409348ff8722ee548a2))
* **llm:** WithJSONMode + WithResponseFormat ChatOptions ([b686a2d](https://github.com/anatolykoptev/go-kit/commit/b686a2d4b0039474a9e7ea85b533cb921c6686dc))
* **llm:** WithJSONMode + WithResponseFormat ChatOptions ([53a2846](https://github.com/anatolykoptev/go-kit/commit/53a2846060b7a3f088270002897e6800a7dfde52))
* **metrics/httpmw:** WithStdlibPattern convenience option ([#55](https://github.com/anatolykoptev/go-kit/issues/55)) ([64eb877](https://github.com/anatolykoptev/go-kit/commit/64eb877071cbe297ea54b1c26abf32a3ecb28194))
* **metrics:** ObserveSeconds API + Registry.Observe ([#87](https://github.com/anatolykoptev/go-kit/issues/87)) ([518d7f9](https://github.com/anatolykoptev/go-kit/commit/518d7f9e1527ff3973f9ecbb1f930f061591a01c))
* **metrics:** per-histogram custom buckets via RegisterHistogram + WithBuckets ([#89](https://github.com/anatolykoptev/go-kit/issues/89)) ([a0070e3](https://github.com/anatolykoptev/go-kit/commit/a0070e391dddb98821c24c6686aa0423308256b6))
* **ratelimit:** Redis sliding-window limiter (login flows) ([dae4d84](https://github.com/anatolykoptev/go-kit/commit/dae4d84c8aff4fd549a765c51b27165092965a9b))
* **rerank:** VoyageRerankClient + JinaRerankClient + llm.ExtractJSON fix ([#39](https://github.com/anatolykoptev/go-kit/issues/39)) ([9e6a4bb](https://github.com/anatolykoptev/go-kit/commit/9e6a4bbc90b21f12fb6900a1ca1c64221e67c7c8))
* **rerank:** WithFallback now accepts any Reranker (not just *Client) ([#41](https://github.com/anatolykoptev/go-kit/issues/41)) ([98c6678](https://github.com/anatolykoptev/go-kit/commit/98c66781cb036253fbf8bd1dab6570791fc43185))
* **score:** add 5-tier Severity vocabulary + helpers ([#46](https://github.com/anatolykoptev/go-kit/issues/46)) ([97cd9b5](https://github.com/anatolykoptev/go-kit/commit/97cd9b563fca5166d0a8221c4743e6a31f8ae601))
* **score:** new package — ConfidenceFromScore + generic Bucket ([#45](https://github.com/anatolykoptev/go-kit/issues/45)) ([7ef7cca](https://github.com/anatolykoptev/go-kit/commit/7ef7ccad9a35a5b0e4bbca8ee1fb61063a634cae))
* **session:** integrate go-session into go-kit ([01b9938](https://github.com/anatolykoptev/go-kit/commit/01b9938fa2f95bd06988475e95592ad25a599e6e))
* **sparse,rerank:** v2 NewClient auto-resolves EMBED_TOKEN env ([9a45dca](https://github.com/anatolykoptev/go-kit/commit/9a45dcafa18f95f6fef572f13031d9c6bf8bc409))
* **sparse,rerank:** v2 NewClient auto-resolves EMBED_TOKEN env ([ce65274](https://github.com/anatolykoptev/go-kit/commit/ce65274014028c4bddde74e90daf3c13df12284e))
* **telegram/botusers:** reusable bot-user store package (v0.62.0) ([#88](https://github.com/anatolykoptev/go-kit/issues/88)) ([2cdb62a](https://github.com/anatolykoptev/go-kit/commit/2cdb62a8078fd6edc535ef1518e0ba4971ac08fd))
* **telegram/callback:** HMAC-signed CallbackData codec ([#74](https://github.com/anatolykoptev/go-kit/issues/74)) ([9a6a735](https://github.com/anatolykoptev/go-kit/commit/9a6a73518e676877576def3e6342837769ce05d3))
* **telegram/cmd:** predicate filters — chat-type, user, regex, custom (aiogram-style) ([#75](https://github.com/anatolykoptev/go-kit/issues/75)) ([90e9307](https://github.com/anatolykoptev/go-kit/commit/90e93071b05d529260428e172fb5f482e759995c))
* **telegram/cmd:** Router — fluent text-command router with auto /help ([#63](https://github.com/anatolykoptev/go-kit/issues/63)) ([d8495b8](https://github.com/anatolykoptev/go-kit/commit/d8495b8cce02e453fbbbe2228b5262be1211e065))
* **telegram/forum:** supergroup forum-topics manager + cmd predicates (v0.60.1) ([#78](https://github.com/anatolykoptev/go-kit/issues/78)) ([9c7ed4b](https://github.com/anatolykoptev/go-kit/commit/9c7ed4b36986b3701d2564742ae5f4659404b356))
* **telegram/fsm:** conversation Machine + MemoryStore + PostgresStore ([#60](https://github.com/anatolykoptev/go-kit/issues/60)) ([03d056a](https://github.com/anatolykoptev/go-kit/commit/03d056ac2ee10233c0a87dc1adb5a98b9595508e))
* **telegram/kb:** keyboard builder + Registry (vendored from go-telegram/ui MIT) ([#59](https://github.com/anatolykoptev/go-kit/issues/59)) ([64702a6](https://github.com/anatolykoptev/go-kit/commit/64702a6937ae85f6dc27dd51ed80d1e978edc8f2))
* **telegram/kb:** ReplyBuilder — persistent reply keyboard with request_*+web_app buttons ([#73](https://github.com/anatolykoptev/go-kit/issues/73)) ([5dd9076](https://github.com/anatolykoptev/go-kit/commit/5dd90765fa81d506fcda8a62af5ebc5142cce663))
* **telegram/middleware:** AutoRespond + DeletePrev + OperatorOnly + RateLimit + ShadowBan + Metrics + Recover ([#58](https://github.com/anatolykoptev/go-kit/issues/58)) ([888e58a](https://github.com/anatolykoptev/go-kit/commit/888e58ab46af462688c5374444dd516fbf4e0386))
* **telegram/middleware:** NewChatQuota — anti-spam unique-chat counter ([#62](https://github.com/anatolykoptev/go-kit/issues/62)) ([eadbdeb](https://github.com/anatolykoptev/go-kit/commit/eadbdeb205938bfb69b1880d34e82ff77541fd72))
* **telegram/middleware:** Tracing — span-per-update via OTel ([#71](https://github.com/anatolykoptev/go-kit/issues/71)) ([7a1ed2d](https://github.com/anatolykoptev/go-kit/commit/7a1ed2d913747028641d733b314549d6bc26893c))
* **telegram/middleware:** WebhookSecretToken — timing-attack-safe header validation ([#69](https://github.com/anatolykoptev/go-kit/issues/69)) ([9d1fc4a](https://github.com/anatolykoptev/go-kit/commit/9d1fc4ac12682d67ab1073c9c7fc22545ae3a0a0))
* **telegram/miniapp:** add SavePreparedInlineMessage helper + BotAPI adapter ([#81](https://github.com/anatolykoptev/go-kit/issues/81)) ([a2453b5](https://github.com/anatolykoptev/go-kit/commit/a2453b56710c58ffc521341e29894318f6250cb9))
* **telegram/miniapp:** initData signature validator (per Telegram Bot API spec) ([#76](https://github.com/anatolykoptev/go-kit/issues/76)) ([d845318](https://github.com/anatolykoptev/go-kit/commit/d84531840b63266d1ef1d08eea5e01330fc2358f))
* **telegram/miniapp:** server-side Reply + SendInvoice + CreateInvoiceLink (v0.60.0) ([#77](https://github.com/anatolykoptev/go-kit/issues/77)) ([266563c](https://github.com/anatolykoptev/go-kit/commit/266563c52d02ead5ab65670113c2a228d0b1f6d6))
* **telegram/tgapi5:** default tgbotapi/v5 adapter impls ([#66](https://github.com/anatolykoptev/go-kit/issues/66)) ([cd45336](https://github.com/anatolykoptev/go-kit/commit/cd4533686714606b26a7adf9c4b777834e1e9dfe))
* **telegram:** add Locale (YAML i18n) at root pkg ([#57](https://github.com/anatolykoptev/go-kit/issues/57)) ([0c54572](https://github.com/anatolykoptev/go-kit/commit/0c545723cca18062110133f1a52fa36cbad4a1bd))
* **telegram:** batched copy/delete broadcast + setMessageReaction (v0.60.2) ([#79](https://github.com/anatolykoptev/go-kit/issues/79)) ([df1156c](https://github.com/anatolykoptev/go-kit/commit/df1156c5f851a27885b9eade8e1ab4de87d37993))
* **telegram:** smart HTML sanitizer + format auto-detect + PrepareForTelegram ([#43](https://github.com/anatolykoptev/go-kit/issues/43)) ([96fac07](https://github.com/anatolykoptev/go-kit/commit/96fac07d8d092643b15149a2b586a520aec8e584))
* **telegram:** transport swap tgbotapi/v5 (dead) → OvyFlash/telegram-bot-api (active) ([#70](https://github.com/anatolykoptev/go-kit/issues/70)) ([4893815](https://github.com/anatolykoptev/go-kit/commit/48938156cea18a12336f34eed06e31f7cbd5aad4))
* **telegram:** v0.57.0 — polish (review residuals) ([#64](https://github.com/anatolykoptev/go-kit/issues/64)) ([5fe4c04](https://github.com/anatolykoptev/go-kit/commit/5fe4c04f5b1a906643f2cf26b4f10ef9ffe28bb8))
* **telegram:** WithCancelCmds (fsm) + broadcast.Pacer + ops.Notifier ([#67](https://github.com/anatolykoptev/go-kit/issues/67)) ([b3e330f](https://github.com/anatolykoptev/go-kit/commit/b3e330ffacfa23d356c45e3914af134f70692aee))
* **tracing/httpmw:** drop-in *Mux wrapper + chi.Walk auto-registration ([#51](https://github.com/anatolykoptev/go-kit/issues/51)) ([f9f60d4](https://github.com/anatolykoptev/go-kit/commit/f9f60d4a14888a083548c239587cd180820a2cf2))
* **tracing/httpmw:** emit OTEL code.* attributes from handler reflection ([#49](https://github.com/anatolykoptev/go-kit/issues/49)) ([12d3ba3](https://github.com/anatolykoptev/go-kit/commit/12d3ba38314dad69ad0549837290e3eebbf59f7b))
* **tracing/httpmw:** gin route adapter + multi-framework walk docstrings ([#52](https://github.com/anatolykoptev/go-kit/issues/52)) ([fc3ef08](https://github.com/anatolykoptev/go-kit/commit/fc3ef08a5d9a3c8515080d516fa6ebf20ae7b95f))
* **tracing/httpmw:** startup-time route registry for OTEL code.* attrs ([1e6fe48](https://github.com/anatolykoptev/go-kit/commit/1e6fe4868e2dd5a0b2463a57ef59fd0aeb6de972))
* **tracing/httpmw:** startup-time route registry for OTEL code.* attrs ([2e13509](https://github.com/anatolykoptev/go-kit/commit/2e13509c6a49a2e0eb280c5c16585978db204aaf))
* **tracing:** correctness polish — Start scope, bad-endpoint graceful, typed mcp params, nil-ctx, shutdown doc ([#47](https://github.com/anatolykoptev/go-kit/issues/47)) ([7f17ada](https://github.com/anatolykoptev/go-kit/commit/7f17ada8522dfe61f86abd5d87a47a10ba268868))


### Bug Fixes

* **cache:** embed role + rerank truncation cap in cache keys (PR C — v0.39.0) ([#40](https://github.com/anatolykoptev/go-kit/issues/40)) ([c5fcc99](https://github.com/anatolykoptev/go-kit/commit/c5fcc990b33ae1df7b7c25915ebbd884e7e74501))
* **httputil:** drop slog.Warn, document ClientIP trust model ([9daa6ab](https://github.com/anatolykoptev/go-kit/commit/9daa6ab59972b5d7c89855394841874799db71a5))
* **httputil:** include 'self' in defaultCSP style-src ([#90](https://github.com/anatolykoptev/go-kit/issues/90)) ([012dcdd](https://github.com/anatolykoptev/go-kit/commit/012dcddb744f9b503c172a5fa93cbf960247cb4c))
* **llm:** strip chat_time from the wire + clone request messages ([bcb9338](https://github.com/anatolykoptev/go-kit/commit/bcb933800cf50afb50d459670154123ea6fde61f))
* **llm:** strip chat_time from wire + clone request messages ([5413f79](https://github.com/anatolykoptev/go-kit/commit/5413f7972ebb44118c7276b81363232ba07d69fe))
* **metrics:** prevent panic on prom_bridge shape collisions ([#37](https://github.com/anatolykoptev/go-kit/issues/37)) ([c8a4f2e](https://github.com/anatolykoptev/go-kit/commit/c8a4f2e3f4f7819c410d07d63c84fd93ea60e80d))
* **ratelimit:** SlidingWindow tidy, fail-closed test, clock injection, fail-fast constructor ([70fc367](https://github.com/anatolykoptev/go-kit/commit/70fc3679ecdbc25f03dbff9887b5cac38f90245a))
* **telegram/cmd:** AutoHelp takes Handler arg (v0.57.0 API contradiction) ([#68](https://github.com/anatolykoptev/go-kit/issues/68)) ([b7bf40a](https://github.com/anatolykoptev/go-kit/commit/b7bf40a065fff78a009d70f3d8d3cdf8cb562ea4))
* **telegram/miniapp:** include signature in HMAC data_check_string ([#84](https://github.com/anatolykoptev/go-kit/issues/84)) ([90c99b2](https://github.com/anatolykoptev/go-kit/commit/90c99b2f2dbe2a353e2c164c36ad8341c5b4fe64))
* **telegram/tgapi5,miniapp:** nil-resp defence-in-depth + enumerate accepted result types ([#82](https://github.com/anatolykoptev/go-kit/issues/82)) ([b6e2a4f](https://github.com/anatolykoptev/go-kit/commit/b6e2a4fd0302b9ee8369f00b9e14228c846d4aab))
* **telegram:** v0.56.1 concurrent-safety + error propagation followups ([#61](https://github.com/anatolykoptev/go-kit/issues/61)) ([601d467](https://github.com/anatolykoptev/go-kit/commit/601d467302e5842567bd5183f81e66c8225e6499))
* **telegram:** v0.60.3 quality-review remediation (Reactor rename, broadcast Pacer, miniapp adapters) ([#80](https://github.com/anatolykoptev/go-kit/issues/80)) ([e7189cf](https://github.com/anatolykoptev/go-kit/commit/e7189cf24b553da8ea8ee25c3ae2f6fb85640c5f))
* **tracing/httpmw:** stdlibFormatter must not duplicate the HTTP method ([5e37087](https://github.com/anatolykoptev/go-kit/commit/5e370871984f3aa0b6f9a3e6f21d3ddb3ed78a2f))


### Performance

* **telegram:** Locale cache compiled templates + zero-alloc hot path ([#72](https://github.com/anatolykoptev/go-kit/issues/72)) ([b106021](https://github.com/anatolykoptev/go-kit/commit/b1060216f3938287fe6a5ef8811795a462da8d65))


### Documentation

* **changelog:** retitle botusers entry to v0.65.0 (v0.62-v0.64 already taken) ([842d273](https://github.com/anatolykoptev/go-kit/commit/842d273d0ac963018fe6191ccf94a00947ce293c))
* per-package specs for llm, retry, hedge, embed, rerank, score, metrics, ([#86](https://github.com/anatolykoptev/go-kit/issues/86)) ([6ac3e0c](https://github.com/anatolykoptev/go-kit/commit/6ac3e0c05035a2b26f21691988aee9c827159b20))

## [Unreleased]

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
