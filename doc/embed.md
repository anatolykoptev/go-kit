# embed

Pluggable text-embedding client with HTTP / Ollama / Voyage backends, batching,
L1 cache, retry + circuit breaker, secondary fallback, observer hooks, and a
typed `Result` for ablation.

```
go get github.com/anatolykoptev/go-kit/embed
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/embed"

e, err := embed.New(embed.Config{
    Type:        "http",
    HTTPBaseURL: "http://localhost:8081",
    Model:       "intfloat/e5-large-v2",
    HTTPDim:     1024,
}, slog.Default())
if err != nil {
    return err
}
defer e.Close()

vec, err := e.EmbedQuery(ctx, "what is rust good for?")
```

`Embedder` is the interface every backend implements:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
    Dimension() int
    Close() error
}
```

## Backends

| `Type` | Backend | Required config |
|--------|---------|-----------------|
| `"http"` | embed-server sidecar | `HTTPBaseURL`, `Model`, `HTTPDim` |
| `"ollama"` | Ollama HTTP API | `OllamaURL`, `Model`; optional `OllamaPrefix`/`OllamaQuery`/`OllamaDim` |
| `"voyage"` | Voyage AI | `VoyageAPIKey`, `Model` |
| `"onnx"` | in-process ONNX | not built into the factory — see `embed/onnx` subpackage |

`onnx` is not registered in `New` to keep cgo-free builds clean — wire it
explicitly via `WithEmbedder` when needed.

## Client wrapper — retry, circuit breaker, cache, fallback

`NewClient` wraps a `*url`-addressed backend with production middleware. Use
this when calling embed-server in production code; use `New(Config, …)` when
you want a raw backend (no breaker, no cache).

```go
c, err := embed.NewClient("http://localhost:8081",
    embed.WithModel("e5-large-v2"),
    embed.WithDim(1024),
    embed.WithTimeout(5*time.Second),
    embed.WithRetry(embed.RetryPolicy{
        MaxAttempts: 3,
        BaseBackoff: 100 * time.Millisecond,
        MaxBackoff:  2 * time.Second,
        Multiplier:  2.0,
        Jitter:      0.25,
        RetryableStatus: []int{502, 503, 504},
    }),
    embed.WithCircuit(embed.CircuitConfig{
        FailThreshold: 5,
        OpenDuration:  30 * time.Second,
    }),
    embed.WithCache(myCache),         // any cache.Cache or your own impl
    embed.WithFallback(secondaryClient), // used on ErrCircuitOpen or terminal err
)
defer c.Close()

vec, err := c.EmbedQuery(ctx, "query")
if errors.Is(err, embed.ErrCircuitOpen) {
    // fast-failed; consider serving cached/degraded
}
```

## EmbedWithResult — ablation visibility

`Result` exposes per-call telemetry without sprinkling hooks across the call
site: hit/miss, retries, fallback firings, latency, truncations.

```go
res, err := c.EmbedWithResult(ctx, []string{"a", "b", "c"})
log.Printf("calls=%d cache_hits=%d retries=%d fallback=%v dur=%s",
    res.Calls, res.CacheHits, res.Retries, res.Fallback, res.Duration)
```

Pass `embed.WithDryRun()` to validate config + backend reachability without
spending tokens (returns an empty vector set).

## Observer hooks

For metric/log integration without owning the call site:

```go
type myObs struct{}
func (myObs) OnBeforeEmbed(ctx context.Context, model string, n int)       { /* ... */ }
func (myObs) OnAfterEmbed(ctx context.Context, s Status, d time.Duration, n int) { /* ... */ }
func (myObs) OnRetry(ctx context.Context, attempt int, err error)            { /* ... */ }
func (myObs) OnCircuitTransition(ctx context.Context, from, to CircuitState) { /* ... */ }
func (myObs) OnCacheHit(ctx context.Context, n int)                          { /* ... */ }
func (myObs) OnTruncate(ctx context.Context, idx, in, out int)               { /* ... */ }

embed.NewClient(url, embed.WithObserver(myObs{}))
```

## Chunking & truncation

Large `Embed` batches are split into chunks of `WithChunkSize(n)` (default
backend-specific). Inputs longer than the model's token limit are truncated
client-side; observer's `OnTruncate` reports byte counts before/after.

## Config reference

| Field | Type | Notes |
|-------|------|-------|
| `Type` | string | `"http"`, `"ollama"`, `"voyage"`, `"onnx"`; empty = `"onnx"` (errors with `ErrONNXNotInFactory`) |
| `Model` | string | Backend-specific identifier |
| `HTTPBaseURL` | string | embed-server URL |
| `HTTPDim` | int | Dimension hint (default 1024) |
| `OllamaURL` | string | Ollama base URL |
| `OllamaDim` | int | 0 = auto-detect from first response |
| `OllamaPrefix` | string | Client-side document prefix (e.g. `"passage: "`) |
| `OllamaQuery` | string | Client-side query prefix (e.g. `"query: "`) |
| `VoyageAPIKey` | string | Voyage API key |
| `ONNXModelDir` | string | Local model directory (cgo path) |

## Errors

| Error | Meaning |
|-------|---------|
| `ErrCircuitOpen` | Breaker is open; backend was not called |
| `*ErrDimMismatch` | Returned vector dimension != configured `Dim` |
| `ErrONNXNotInFactory` | `Type=onnx` requested; wire backend manually |

## Notes

- Use `EmbedQueryViaEmbed(ctx, e, text)` as a portable single-query helper for
  any `Embedder` that doesn't override `EmbedQuery`.
- `L2Normalize(vec)` is exported for callers that need to normalise vectors
  from backends that don't (Voyage already normalises; e5/Jina/SPLADE do not).
