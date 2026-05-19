# sparse

Sparse-vector client for SPLADE-style models served by embed-server. Mirrors
the [`embed`](embed.md) package's wrapping (retry / circuit breaker / cache /
fallback / observer) but produces `(indices, weights)` rather than dense
`[]float32`.

```
go get github.com/anatolykoptev/go-kit/sparse
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/sparse"

c, err := sparse.NewClient("http://localhost:8081",
    sparse.WithModel("splade-v3-distilbert"),
    sparse.WithTimeout(5*time.Second),
)
if err != nil {
    return err
}
defer c.Close()

vec, err := c.EmbedSparseQuery(ctx, "what is rust good for?")
// vec.Indices: []uint32 — token IDs with non-zero weights
// vec.Values:  []float32 — corresponding weights
```

## SparseEmbedder interface

```go
type SparseEmbedder interface {
    EmbedSparse(ctx, texts []string) ([]SparseVector, error)
    EmbedSparseQuery(ctx, text string) (SparseVector, error)
    VocabSize() int
    Close() error
}
```

`VocabSize()` is the dimension of the sparse space (e.g. 30522 for BERT-base
SPLADE). It is **configured at construction**, not validated against the
model's actual head — callers using `pgvector`'s `sparsevec` literal need
the right number here.

## Factory — config-driven

```go
emb, err := sparse.New(sparse.Config{
    Type:        "http",
    HTTPBaseURL: "http://localhost:8081",
    Model:       "splade-v3-distilbert",
    VocabSize:   30522,
    TopK:        256,   // server-side keep-top-K
    MinWeight:   0.0,   // server-side prune below
}, slog.Default())
```

Or read everything from env (`SPARSE_BACKEND`, `SPARSE_HTTP_BASE_URL`,
`SPARSE_MODEL`, `SPARSE_VOCAB_SIZE`, …):

```go
emb, err := sparse.NewFromEnv(slog.Default())
```

## Client wrapper — production middleware

Same shape as `embed.NewClient`:

```go
c, err := sparse.NewClient("http://localhost:8081",
    sparse.WithModel("splade-v3-distilbert"),
    sparse.WithClientTopK(256),
    sparse.WithClientMinWeight(0.0),
    sparse.WithClientVocabSize(30522),
    sparse.WithRetry(sparse.RetryConfig{MaxAttempts: 3, BaseBackoff: 100*time.Millisecond}),
    sparse.WithCircuit(sparse.CircuitConfig{FailThreshold: 5, OpenDuration: 30*time.Second}),
    sparse.WithCache(myCache),
    sparse.WithFallback(secondaryClient),
    sparse.WithObserver(myObserver),
)
```

`sparse.NoRetry` opts out of retries (`RetryConfig{MaxAttempts: 1}`).

## SparseVector — the wire shape

```go
type SparseVector struct {
    Indices []uint32  // sorted token IDs
    Values  []float32 // same len as Indices, parallel ordering
}

func (v SparseVector) Len() int          // number of non-zero entries
func (v SparseVector) IsEmpty() bool     // len(Indices) == 0
```

`(Indices, Values)` are positionally aligned. The slices come straight from the
server and are not copied by the client; do not mutate them in place.

## EmbedSparseWithResult — ablation visibility

```go
res, err := c.EmbedSparseWithResult(ctx, texts, sparse.WithDryRun())
log.Printf("retries=%d cache_hits=%d fallback=%v dur=%s",
    res.Retries, res.CacheHits, res.Fallback, res.Duration)
```

## Backend registry

When a service routes between multiple sparse models (e.g. English SPLADE +
Russian SPLADE), wrap them in `sparse.Registry`:

```go
reg := sparse.NewRegistry("splade-en")
reg.Register("splade-en", englishEmbedder)
reg.Register("splade-ru", russianEmbedder)

emb, ok := reg.Get(lang) // falls back to fallback name if unknown
```

## Errors

| Error | Meaning |
|-------|---------|
| `ErrCircuitOpen` | Breaker is open; backend was not called |
| `ErrModelNotConfigured` | Server replied that the SPLADE model is not loaded — usually means the embed-server config doesn't include the requested model |

## Notes

- `EmbedSparseQuery` may apply a query-specific prefix or instruction
  depending on the model; default delegates to `EmbedSparse([text])` and
  returns the first vector.
- The single-call HTTP embedder (`NewHTTPSparseEmbedder`) is exposed for tests
  and bespoke setups; production code should use `New` or `NewClient`.
- Pair with [`embed`](embed.md) for dense embeddings and
  [`rerank`](rerank.md)'s fusion (RRF / DBSF) to combine dense + sparse + BM25.
