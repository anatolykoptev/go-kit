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

## Migration Status

| Repo | Imported | Remaining |
|------|----------|-----------|
| go-search | cache, env, strutil | — (llm uses go-engine) |
| go-job | cache, env, llm, metrics, strutil | — |
| go-wp | cache, env, llm, metrics, strutil | — |
| go-code | cache, env, llm | — |
| go-hully | cache, env, llm, metrics, strutil | — |
| go-startup | cache, env, llm, metrics, retry, strutil | — |
| go-content | env, llm, metrics | — |
| gigiena-teksta | env, llm, metrics | — |

---

## Wave 2 (complete)

| # | Feature | Package | Status |
|---|---------|---------|--------|
| 1 | Per-key TTL | cache | **DONE** |
| 2 | OnEvict callback | cache | **DONE** |
| 3 | ratelimit package | ratelimit | **DONE** |
| 4 | Union types (ExtractOneOf) | llm | **DONE** |
| 5 | StreamExtract + partialJSON | llm | **DONE** |
| 6 | Fibonacci backoff | retry | **DONE** |
| 7 | Doorkeeper bloom filter | cache | **DEFERRED** — ghost queue sufficient |

---

## Wave 3: Competitive Edge (planned)

Based on analysis of 19 competing libraries across Go and Python ecosystems.
See [competitive-analysis.md](competitive-analysis.md) for full breakdown.

### Tier 1 — High Impact

| # | Feature | Package | Source | Effort | Rationale |
|---|---------|---------|--------|--------|-----------|
| W3-1 | Schema constraint tags | llm | instructor-go, bellman | M | **DONE** — LLM sees `minimum`, `enum`, `description` in schema → dramatically better structured output quality |
| W3-2 | Hedge policy | new: `hedge` | failsafe-go | M | **DONE** — Start 2nd LLM request after timeout, take first response. Cuts p99 latency 2-3x |
| W3-3 | Structured LLM errors | llm | go-openai | S | **DONE** — `APIError{StatusCode, Type, Body}` instead of `fmt.Errorf`. Callers can `errors.As` and branch |
| W3-4 | Tag-based cache invalidation | cache | eko/gocache | M | `Set(key, val, WithTags("user:123"))` → `InvalidateByTag("user:123")`. Removes manual key tracking |

### Tier 2 — Quality of Life

| # | Feature | Package | Source | Effort | Rationale |
|---|---------|---------|--------|--------|-----------|
| W3-5 | OnRetry callback | retry | avast/retry-go | S | **DONE** — `OnRetry func(attempt int, err error)` for logging/metrics per attempt |
| W3-6 | RetryIf predicate | retry | avast/retry-go | S | **DONE** — `RetryIf func(error) bool` — more flexible than AbortOn + RetryableOnly combined |
| W3-7 | Concurrency limiter (bulkhead) | ratelimit | failsafe-go | S | **DONE** — `NewConcurrencyLimiter(5)` — "max N in-flight", complements token bucket |
| W3-8 | Typed cache Get/Set | cache | eko/gocache | S | `GetJSON[T](key)` / `SetJSON(key, val)` — auto-marshal, removes boilerplate |
| W3-9 | Permanent(err) from fn | retry | cenkalti/backoff | S | **DONE** — fn returns `retry.Permanent(err)` → never retry. Better than pre-declaring AbortOn |
| W3-10 | Context error wrapping | retry | avast/retry-go | S | **DONE** — `fmt.Errorf("after %d attempts: %w: %v", n, ctx.Err(), lastErr)` for diagnostics |

### Tier 3 — Future / Experimental

| # | Feature | Package | Source | Notes |
|---|---------|---------|--------|-------|
| W3-11 | Tool call mode for Extract | llm | instructor-go | `ModeToolCall` vs `ModeJSON` — some models more reliable with tool calling |
| W3-12 | Classify / Generate helpers | llm | marvin (Python) | `Classify(text, labels)`, `Generate(n, target)` — semantic taxonomy |
| W3-13 | Composable policy chain | new: `resilience` | failsafe-go | `Run(fn, retry, circuitBreaker, timeout)` — generic policy composition |
| W3-14 | Adaptive rate limiter | ratelimit | failsafe-go | Auto-adjusts limits based on observed latency/error rate |
| W3-15 | Prometheus/OTEL metrics export | metrics | eko/gocache | Export Stats as Prometheus counters, not just text |

---

### W3-1. llm: Schema Constraint Tags

**Gap**: `SchemaOf()` reads only `json` tags. LLMs produce better output when the schema
includes `description`, `minimum`, `maximum`, `enum`, `pattern`.

**Competitors**: instructor-go uses `jsonschema` package tags. bellman uses
`json-description`, `json-enum`, `json-minimum`. Both generate richer schemas.

```go
type User struct {
    Name string  `json:"name" jsonschema:"description=Full legal name"`
    Age  int     `json:"age"  jsonschema:"minimum=0,maximum=150"`
    Role string  `json:"role" jsonschema:"enum=admin|user|guest"`
}
// SchemaOf(&User{}) now emits:
// {"properties":{"name":{"type":"string","description":"Full legal name"}, ...}}
```

Effort: extend `typeSchema()` and `structSchema()` in `schema.go` to parse
`jsonschema` struct tag. ~80 LOC.

### W3-2. Hedge Policy

**Gap**: LLM APIs have high p99 variance (5s median, 30s+ tail).
No way to mitigate without manual goroutine management.

**Competitor**: failsafe-go `hedge.With(time.Second).Get(fn)`.

```go
package hedge

// Do starts fn. If it hasn't returned after delay, starts fn again
// in parallel. Returns the first successful result, cancels the other.
func Do[T any](ctx context.Context, delay time.Duration, fn func(context.Context) (T, error)) (T, error)
```

Effort: ~80 LOC. Uses `context.WithCancel` + goroutine + select.

### W3-3. llm: Structured Error Types

**Gap**: LLM errors are `fmt.Errorf("llm: HTTP %d: %s")`. Callers must
parse strings to detect rate limits vs auth failures vs server errors.

```go
type APIError struct {
    StatusCode int
    Body       string
    Type       string // "rate_limit_error", "invalid_api_key", etc.
    Retryable  bool
}
func (e *APIError) Error() string { ... }

// Usage:
var apiErr *llm.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == 401 {
    // rotate API key
}
```

Effort: ~40 LOC. Replace `fmt.Errorf` in `transport.go` with `&APIError{}`.

### W3-4. cache: Tag-Based Invalidation

**Gap**: deletion is key-by-key. When a user profile changes, you must
manually track and delete all related cache keys.

**Competitor**: eko/gocache `store.WithTags("user", "profile")`.

```go
c.SetWithTags(ctx, "user:123:profile", data, []string{"user:123"})
c.SetWithTags(ctx, "user:123:posts", data, []string{"user:123"})

// One call invalidates both:
c.InvalidateByTag(ctx, "user:123")
```

Effort: ~100 LOC. Add `tags map[string]map[string]bool` to cache, update
Set/Delete to maintain the index.

---

## Timeline

```
March 2026 (done):
  W2-1..W2-6  All Wave 2 items complete
  Migration   go-content, gigiena-teksta, go-hully, go-startup migrated

April 2026 (done):
  W3-1  Schema constraint tags     — DONE
  W3-3  Structured LLM errors      — DONE
  W3-5  OnRetry callback           — DONE
  W3-9  Permanent(err) from fn     — DONE
  W3-10 Context error wrapping     — DONE

May 2026 (done):
  W3-2  Hedge policy               — DONE
  W3-6  RetryIf predicate          — DONE
  W3-7  Concurrency limiter        — DONE

Later / as needed:
  W3-4  Tag-based invalidation     — 1-2 days
  W3-8  Typed cache Get/Set        — 0.5 day
  W3-11..W3-15  Tier 3 items
```
