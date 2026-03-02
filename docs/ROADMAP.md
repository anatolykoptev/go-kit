# go-kit Roadmap

> **Module**: `github.com/anatolykoptev/go-kit`
> **Current version**: v0.6.0 | **Migrated**: 8/8 repos

See also: [architecture.md](architecture.md) | [design.md](design.md)

## Extraction Phases (all complete)

| Phase | Package | Savings | Repos |
|-------|---------|---------|-------|
| 1 | env | ~500 LOC | 8 |
| 2 | llm | ~950 LOC | 6 |
| 3 | cache | ~800 LOC | 7 |
| 4 | retry | ~100 LOC | 2+ |
| 5 | metrics | ~300 LOC | 7 |
| 6 | strutil | ~100 LOC | 4+ |
| 7 | mcpserver | ~560 LOC | 7 (separate repo: go-mcpserver) |
| **Total** | | **~3,310 LOC** | **8 repos** |

## Competitive Improvements (all complete)

Every package upgraded to be competitive with best-in-class Go libraries.
See [design.md](design.md) for full competitive analysis and rationale.

| Package | Key additions |
|---------|--------------|
| env | Error variants (`*E`, `Must*`), Source interface, File, Expand, Base64/Hex |
| llm | Streaming, tool calling, structured output, Extract[T], endpoint fallback, middleware |
| cache | S3-FIFO eviction, sharded map, singleflight, TTL jitter, L2 Redis interface, per-key TTL |
| retry | RetryAfter, MaxElapsedTime, Jitter, Timer, AbortOn, RetryableOnly |
| metrics | Gauge, Timer, Labels, Sink, Rate/EWMA, Histogram, TTL, SnapshotAndReset |
| strutil | TruncateMiddle, case conversions, WordWrap, Scrub, ContainsAll |

## Migration Status (v0.6.0)

| Repo | Imported | Remaining |
|------|----------|-----------|
| go-search | cache, env, strutil | llm, metrics, retry |
| go-job | cache, env, llm, metrics, strutil | retry |
| go-wp | cache, env, llm, metrics, strutil | retry |
| go-code | cache, env, llm | metrics, retry, strutil |
| go-hully | cache, env, metrics, strutil | llm, retry |
| go-startup | cache, env, llm, metrics, strutil | retry |
| go-content | env, metrics | llm, cache, retry |
| gigiena-teksta | env, metrics | llm, cache, retry |

---

## Wave 2: Active Items (March 2026)

| # | Feature | Package | Effort | Impact | Status |
|---|---------|---------|--------|--------|--------|
| 1 | Per-key TTL | cache | Low | High | **DONE** |
| 2 | OnEvict callback | cache | Low | Medium | **DONE** |
| 3 | ratelimit package | ratelimit | Medium | High | **DONE** |
| 4 | Union types in Extract | llm | Medium | Medium | **DONE** |
| 5 | Streaming structured output | llm | High | Medium | **P2** |
| 6 | Fibonacci backoff | retry | Low | Low | **P3** |
| 7 | Doorkeeper bloom filter | cache | Medium | Low | **P3** |

### W2-2. cache: OnEvict Callback (P1)

**Gap**: no way to react when entries are evicted (metrics, dependent invalidation).

Competitors: golang-fifo `SetOnEvicted(key, value, reason)`,
theine-go `RemovalListener`, otter `WithDeletionListener`.

```go
type EvictReason int
const (
    EvictExpired  EvictReason = iota
    EvictCapacity
    EvictExplicit
)

type Config struct {
    // ...existing...
    OnEvict func(key string, data []byte, reason EvictReason)
}
```

Effort: ~50 LOC

### W2-3. ratelimit: New Package (P1)

**Gap**: all go-* services hit rate-limited APIs (LinkedIn, Twitter, LLM).
Each service uses ad-hoc sleeps or retry. No shared rate limiter.

Design: token bucket (`time.Ticker` based), zero external deps.

```go
package ratelimit

func New(rate float64, burst int) *Limiter
func (l *Limiter) Wait(ctx context.Context) error
func (l *Limiter) Allow() bool

// Per-key rate limiting (per-domain, per-API-key)
func NewKeyLimiter(rate float64, burst int) *KeyLimiter
func (kl *KeyLimiter) Wait(ctx context.Context, key string) error
```

Note: consider wrapping `golang.org/x/time/rate` if acceptable as dep.
Otherwise ~150 LOC from scratch.

### W2-4. llm: Union Types in Extract (P2)

**Gap**: `Extract[T]()` returns one fixed type. Agent patterns need LLM to choose
between multiple response types.

Competitor: instructor-go `CreateChatCompletionUnion()`.

```go
result, err := llm.ExtractOneOf(ctx, messages,
    llm.UnionType[SearchAction]("search"),
    llm.UnionType[AnswerAction]("answer"),
)
switch v := result.(type) {
case *SearchAction: ...
case *AnswerAction: ...
}
```

### W2-5. llm: Streaming Structured Output (P2)

**Gap**: `Stream()` returns raw text. `Extract[T]()` waits for full response.
No progressive parsing of partial JSON during streaming.

Competitor: instructor-go `CreateChatCompletionStream()` + `stream.Scan(&partial)`.

```go
stream, err := client.StreamExtract(ctx, messages, &target)
for stream.Next() {
    partial := stream.Partial()
}
final := stream.Value()
```

Effort: ~300 LOC (partial JSON parser)

### W2-6. retry: Fibonacci Backoff (P3)

Gentler curve than exponential (1,1,2,3,5,8 vs 1,2,4,8,16,32).
Reference: sethvargo/go-retry `NewFibonacci()`. Effort: ~20 LOC.

### W2-7. cache: Doorkeeper Bloom Filter (P3)

Ghost queue protects against one-hit wonders but uses memory (map of keys).
Bloom filter is more space-efficient for high-cardinality workloads.

**Deferred** — ghost queue sufficient for current workloads (~1000 items).
Revisit if L1MaxItems grows to 100K+.

---

## Timeline

```
March 2026 (immediate):
  W2-1  Per-key TTL              — DONE
  W2-2  OnEvict callback         — DONE
  W2-3  ratelimit package        — DONE

April 2026:
  W2-4  Union types in Extract   — DONE

Later / as needed:
  W2-5  Streaming structured     — 2-3 days
  W2-6  Fibonacci backoff        — 0.5 day
  W2-7  Doorkeeper bloom filter  — defer
```
