# rerank

Pluggable cross-encoder / API reranker (Jina, Voyage, math-cosine) with cascade
staging, multiple fusion algorithms (RRF, weighted RRF, DBSF, linear min-max),
multi-query expansion, optional score normalisation, retry, circuit breaker,
secondary fallback, and a typed `Result` for ablation.

```
go get github.com/anatolykoptev/go-kit/rerank
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/rerank"

c := rerank.NewClient("http://localhost:8082",
    rerank.WithModel("BAAI/bge-reranker-v2-m3"),
    rerank.WithTimeout(3*time.Second),
    rerank.WithMaxDocs(50),
)

docs := []rerank.Doc{
    {ID: "1", Text: "Rust is a systems programming language."},
    {ID: "2", Text: "Go is concurrent and garbage-collected."},
}
out := c.Rerank(ctx, "best language for kernels", docs)
for _, s := range out {
    fmt.Println(s.ID, s.Score)
}
```

`Reranker` is the interface every backend / cascade stage implements:

```go
type Reranker interface {
    Rerank(ctx, query, docs) []Scored
    RerankWithResult(ctx, query, docs, ...RerankOpt) (*Result, error)
    Available() bool
}
```

## Cascade — multi-stage reranking

Run a cheap stage first (math/cosine) over many docs, then a heavier
cross-encoder over the survivors. Each stage cuts the field via `KeepTopN`;
`StopBelowThreshold` shortcuts if the top score collapses.

```go
casc := rerank.Cascade{
    Stages: []rerank.StageConfig{
        {Reranker: math, KeepTopN: 100, Label: "math"},
        {Reranker: bge,  KeepTopN: 20,  Label: "bge",  StopBelowThreshold: 0.05},
        {Reranker: vye,  KeepTopN: 5,   Label: "voyage"},
    },
}

scored := casc.Rerank(ctx, query, allDocs)
```

`Label` is required for per-stage Prometheus metrics. `StopBelowThreshold`
compares raw stage scores; if you need normalised comparisons across stages,
configure the inner client with `WithNormalize`.

## Fusion (hybrid search merging)

Combine result lists from different retrievers (vector / BM25 / SPLADE) into
one consolidated ordering.

| Algorithm | Constructor | Inputs | When to use |
|-----------|-------------|--------|-------------|
| RRF | `NewRRF(k, ...)` | `[]string` per list | Default: scores not comparable across lists |
| Weighted RRF | `NewWeightedRRF(k, weights, ...)` | `[]string` per list | RRF + per-source weights |
| DBSF | `NewDBSF(...)` / `DBSF(lists...)` | `ScoredIDList` | Distribution-based score fusion — score-aware, robust to outliers |
| Linear min-max | `NewLinearMinMax(weights, ...)` | `ScoredIDList` | Simple weighted blend after per-list min-max normalisation |

```go
rrf, _ := rerank.NewRRF(60, rerank.WithTopK(20))
fused := rrf.Fuse(vectorIDs, bm25IDs, spladeIDs)
for _, f := range fused {
    fmt.Println(f.ID, f.Score)
}
```

DBSF is the "score-aware default" — same call shape as RRF but using actual
scores instead of ranks; better when the retrievers' score distributions are
roughly Gaussian.

## MultiQuery — query expansion

`MultiQuery` runs the same doc set against multiple rephrased queries and
combines per-query scores via RRF or score averaging. Wire as a `Reranker`
inside a `Cascade` stage when a single query under-recalls.

```go
mq := rerank.MultiQuery{Inner: bge}
res, err := mq.RerankMulti(ctx, []string{"v1 of query", "rephrased v2"}, docs)
```

## Score normalisation

| Mode | Effect |
|------|--------|
| `NormalizeNone` | passthrough (default) |
| `NormalizeMinMax` | maps each result set to `[0,1]` via `(x-min)/(max-min)` |
| `NormalizeZScore` | zero mean / unit variance — for relative ranking only |

```go
c := rerank.NewClient(url, rerank.WithNormalize(rerank.NormalizeMinMax))
```

Pair with `StopBelowThreshold` in a cascade when the threshold needs to mean
the same thing across stages.

## Math reranker (no backend)

`MathReranker` ranks by cosine similarity using embeddings the caller already
holds — zero-cost baseline and a useful first cascade stage.

```go
math := rerank.MathReranker{ /* see embed pkg for vector inputs */ }
```

## Result + ablation

```go
res, err := c.RerankWithResult(ctx, query, docs,
    rerank.WithTopN(10),
    rerank.WithThreshold(0.1),
)
log.Printf("retries=%d cache_hits=%d fallback=%v dur=%s",
    res.Retries, res.CacheHits, res.Fallback, res.Duration)
```

`WithDryRun()` validates config + backend reachability without spending
tokens.

## Production middleware

```go
c := rerank.NewClient(url,
    rerank.WithModel("bge-reranker-v2-m3"),
    rerank.WithCache(myCache),
    rerank.WithRetry(rerank.RetryPolicy{
        MaxAttempts:     3,
        BaseBackoff:     100 * time.Millisecond,
        MaxBackoff:      2 * time.Second,
        Multiplier:      2.0,
        Jitter:          0.25,
        RetryableStatus: []int{502, 503, 504},
    }),
    rerank.WithCircuit(rerank.CircuitConfig{
        FailThreshold: 5,
        OpenDuration:  30 * time.Second,
    }),
    rerank.WithFallback(mathFallback),
    rerank.WithFallbackName("math"),
)
```

`rerank.NoRetry` is the explicit opt-out (`RetryPolicy{MaxAttempts: 1}`) — pass
to `WithRetry` to disable the default 5xx retry.

## Per-call options

| Option | Effect |
|--------|--------|
| `WithTopN(n)` | Truncate output to top N after scoring |
| `WithThreshold(t)` | Drop scores below `t` |
| `WithDryRun()` | Skip backend call; return zero results |

## Client options

| Option | Notes |
|--------|-------|
| `WithModel(name)` | Backend model identifier |
| `WithAPIKey(key)` | For backends that authenticate (Jina cloud, Voyage) |
| `WithTimeout(d)` | Per-request timeout |
| `WithMaxDocs(n)` | Truncate the input list before sending |
| `WithMaxCharsPerDoc(n)` / `WithMaxTokensPerDoc(n)` | Per-doc truncation guards |
| `WithInstruction(qPfx, dPfx)` | Add model-specific instructions to query/doc |
| `WithSourceWeights(map)` | Per-source weights for fused inputs |
| `WithHTTPClient(hc)` | Bring your own transport |
| `WithObserver(obs)` | Hook for metrics/logs |

## Errors

| Error | Meaning |
|-------|---------|
| `ErrCircuitOpen` | Breaker is open; backend was not called |
| `ErrEmptyQueries` | `MultiQuery.RerankMulti` called with empty slice |
| `ErrAllQueriesFailed` | All MultiQuery passes failed without surfacing an error |

`Available()` reports `false` when the backend is not configured (e.g. no
API key set) — use to skip rerank when caller is optional.
