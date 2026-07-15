# hedge

Tail-latency reducer: start one call, and if it hasn't returned after a delay,
launch a second call in parallel; first success wins.

```
go get github.com/anatolykoptev/go-kit/hedge
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/hedge"

// Same backend, retry after delay (combats single-instance flakes).
result, err := hedge.Do(ctx, 250*time.Millisecond, func(ctx context.Context) (Doc, error) {
    return search(ctx, query)
})
```

## Do vs DoFallback

| | `Do` | `DoFallback` |
|---|------|--------------|
| Both calls invoke | same `fn` | `primary`, then `fallback` |
| Primary fails before delay | error returned immediately | `fallback` starts **right away** (no waiting) |
| Use case | one flaky instance behind a load balancer | distinct providers (OpenAI → DeepInfra; primary DB → replica) |

```go
// Primary LLM with cheaper fallback.
reply, err := hedge.DoFallback(ctx, 800*time.Millisecond,
    func(ctx context.Context) (string, error) {
        return openai.Complete(ctx, sys, usr)
    },
    func(ctx context.Context) (string, error) {
        return deepinfra.Complete(ctx, sys, usr)
    },
)
```

## Semantics

- The winning call's `context` is cancelled when the other returns — provided
  the inner function honours `ctx`. Always thread the inner ctx through to your
  HTTP client / DB driver, otherwise the loser keeps running until completion.
- If both calls fail, the **primary** error is returned. Fallback errors are
  usually not what callers want surfaced; switch the order if you want the
  opposite.
- `delay <= 0` short-circuits to a single call of `primary` with no goroutines —
  zero overhead path for "feature flag off".
- `ctx.Done()` always wins over both calls; a cancelled context returns
  `ctx.Err()` without waiting for in-flight goroutines.

## When NOT to hedge

- The operation is **expensive** (LLM completion at the 99th percentile, $$ per
  call). Hedging doubles cost on slow tails; budget before enabling.
- The operation has **side effects** (POST, payments, mutations). Hedging may
  execute twice — only safe on idempotent reads.
- The backend is **single-instance**. Retrying the same flake doesn't help —
  use `retry` with backoff instead.

## API reference

| Function | Signature |
|----------|-----------|
| `Do[T]` | `func(ctx, delay, fn func(ctx) (T, error)) (T, error)` |
| `DoFallback[T]` | `func(ctx, delay, primary, fallback func(ctx) (T, error)) (T, error)` |

No options struct on purpose — the only knob is `delay`, which the caller picks
per call site based on observed p50/p95 latency.

## Reference

Google's "The Tail at Scale" paper (Dean & Barroso, CACM 2013) is the canonical
write-up of why this works for read-heavy systems with predictable tails.
