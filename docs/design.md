# go-kit Design Rationale

> Competitive analysis of 25+ libraries across Go, Rust, Python, TypeScript, Java.
> Research dates: Q1 2026. All improvements are implemented and released.

## Competitive Summary

| Package | Best Competitor | Our Unique Edge |
|---------|----------------|-----------------|
| env | caarlos0/env (6K stars) | Simplest API (functions, not struct tags), zero reflection |
| llm | openai/openai-go (3K) | Extract[T] with validation retry + API key cycling |
| cache | maypok86/otter (5.8K) | L1+L2 tiered cache with S3-FIFO (no competitor combines both) |
| retry | cenkalti/backoff (3.9K) | RetryAfter + AbortOn + RetryableOnly in one simple API |
| metrics | hashicorp/go-metrics | Zero-dep: Counter + Gauge + Rate/EWMA + Histogram + TTL |
| strutil | huandu/xstrings | Unicode-correct truncation with case conversion |

---

## env — Close the Gaps

**Competitors**: caarlos0/env (6K), kelseyhightower/envconfig (5.4K), sethvargo/go-envconfig (1.2K)

**Key decisions**:
- Functions over struct tags — simpler API, zero reflection, fastest compile
- `*E` variants return errors, `Must*` panic — backward-compatible addition
- `Source` interface (inspired by sethvargo's `Lookuper`) enables parallel-safe tests
- `File()` for Docker secrets pattern (`/run/secrets/`), from caarlos0 `file` tag
- `Expand()` for `${VAR}` resolution, from caarlos0 + sethvargo
- Phase D (struct parsing) deferred — implement only if 3+ consumers need it

**Gap closed**: every competitor returns errors; we silently swallowed them.

---

## llm — From Utility to Competitive Client

**Competitors**: sashabaranov/go-openai (10.5K), openai/openai-go (3K), tmc/langchaingo (8.7K)

**Phase A — Table stakes** (all implemented):
1. SSE Streaming — `bufio.Scanner` on `text/event-stream`, zero new deps
2. Tool/Function Calling — `tools`, `tool_choice` in request; parse `tool_calls` from response
3. Structured Output — `reflect` → JSON Schema from struct tags; `response_format` with `json_schema`
4. Token Usage — parse `usage` object including `cached_tokens`

**Phase B — Differentiators** (all implemented):
1. **Extract[T] with retry** — JSON Schema → structured output → validate → retry with error feedback. No Go library does this well (Python Instructor is the gold standard). Killer feature.
2. **Endpoint fallback chains** — extend key cycling to full URL+key+model fallback. Natural evolution of `WithFallbackKeys`.
3. **Request/response middleware** — `type Middleware func(req, next) (resp, error)`. Enables logging, metrics, caching without polluting core.

**Phase C — Nice-to-have** (deferred): cost estimation, OpenTelemetry spans, streaming typed extraction.

---

## cache — From FIFO to Best-in-Class

**Competitors**: maypok86/otter (5.8K), dgraph-io/ristretto (5.6K), eko/gocache (2.6K), jellydator/ttlcache (1.1K), samber/hot (243)

**Phase A — Core performance** (all implemented):
1. **S3-FIFO eviction** — 3 FIFO queues (Small + Main + Ghost). +10-30% hit rate over plain FIFO. Simpler than W-TinyLFU. Reference: samber/hot, SOSP '23 paper.
2. **Sharded map** — replaced `sync.Map` with 64-shard map. +2-5x throughput under contention. Reference: ristretto (256 shards).
3. **Fast key hashing** — replaced `crypto/sha256` with `hash/maphash`. 10-50x faster, stdlib, zero deps.

**Phase B — Modern API** (all implemented):
1. Generics — **deferred**: all 7 consumers use `[]byte`; generics add complexity without current benefit.
2. **GetOrLoad with singleflight** — prevents thundering herd. Inline implementation, no external dep.
3. **TTL jitter** — configurable ±% random jitter prevents cache stampedes. Reference: samber/hot.
4. **Statistics** — hits, misses, hit ratio, evictions via `atomic.Int64` counters.

**Phase C — L2 Redis** (all implemented):
1. **L2 interface** — pluggable backends for testing or alternatives.
2. **Read-through + write-through** — L1 miss → L2 hit → auto L1 promotion; Set writes both tiers.

---

## retry — Production Grade

**Competitors**: cenkalti/backoff (3.9K), avast/retry-go (2.9K), failsafe-go (2K), sethvargo/go-retry (708)

**Additions** (all implemented):
1. **RetryAfter** — let fn return `RetryAfter(duration)` to override backoff; auto-parse HTTP `Retry-After` header
2. **MaxElapsedTime** — total wall-clock budget across all attempts
3. **Jitter** — `±25%` random variation on delay
4. **Timer interface** — `type Timer interface { After(d) <-chan time.Time }` for deterministic tests
5. **AbortOn** — never retry specific errors (e.g. `context.DeadlineExceeded`)
6. **RetryableOnly** — invert default: only `MarkRetryable(err)` errors trigger retry

**Not planned**: circuit breaker, bulkhead, hedge policy → use failsafe-go directly.

---

## metrics — Observability Toolkit

**Competitors**: daniel-nichter/go-metrics, hashicorp/go-metrics, VictoriaMetrics/metrics

**Additions** (all implemented):
1. **SnapshotAndReset** — atomic read + zero, no lost counts
2. **Gauge** — current values (queue depth, latency) with min/max/mean
3. **Timer/MeasureSince** — `defer reg.StartTimer("latency").Stop()`
4. **Labels** — `reg.Incr(Label("requests", "method", "GET"))` dimensional metrics
5. **Sink interface** — `TextSink`, `JSONSink` for output formatting
6. **Rate/EWMA** — events/sec with 1/5/15-minute moving averages
7. **Reservoir sampling** — fixed 2K sample for P50/P95/P99 without unbounded memory
8. **TTL** — auto-expire stale per-endpoint/per-user metrics

**Not planned**: Prometheus replacement, push to remote (consumers wrap via Sink).

---

## strutil — Unicode-Correct Toolkit

**Competitors**: huandu/xstrings, aquilax/truncate, Rust unicode-truncate

**Additions** (all implemented):
1. **TruncateMiddle** — `"path/to/ver...file.go"` keeps start + end
2. **Configurable placeholder** — `WithPlaceholder("[...]")` instead of hardcoded "..."
3. **Case conversions** — `ToSnakeCase`, `ToCamelCase`, `ToKebabCase`, `ToPascalCase`
4. **WordWrap** — wrap at word boundaries
5. **Scrub** — remove/replace invalid UTF-8
6. **ContainsAll** — complement to existing `ContainsAny`

**Deferred**: display-width truncation (CJK), grapheme clusters — add when bugs appear.

---

## Wave 2 Competitor Analysis (March 2026)

| Repo | Stars | Relevant to |
|------|-------|-------------|
| scalalang2/golang-fifo | 170 | cache — S3-FIFO/SIEVE reference impl |
| Yiling-J/theine-go | 362 | cache — W-TinyLFU, secondary cache, doorkeeper |
| 567-labs/instructor-go | 193 | llm — streaming structured output, union types |
| teilomillet/gollm | 639 | llm — prompt templates, memory, provider switching |
| sethvargo/go-retry | 708 | retry — middleware pattern, Fibonacci backoff |
| avast/retry-go | 2888 | retry — simple API reference |

See [ROADMAP.md](ROADMAP.md) for active Wave 2 items.
