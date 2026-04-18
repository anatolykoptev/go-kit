# breaker

3-state circuit breaker (closed → open → half-open) with exponential cooldown,
optional jitter, and probe-slot semantics.

```
go get github.com/anatolykoptev/go-kit/breaker
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/breaker"

b := breaker.New(breaker.Options{
    Name:          "payments-api",
    FailThreshold: 3,
    OpenDuration:  10 * time.Second,
})

// Preferred: Execute wraps Allow+Record automatically.
result, err := breaker.Execute(b, func() (string, error) {
    return callPaymentsAPI(ctx)
})
if errors.Is(err, breaker.ErrOpen) {
    // fast-failed, upstream not called
}
```

## Cycle-based vs request-based modes

| Mode | When to use | How to record |
|------|-------------|---------------|
| **Count-based** (default) | Individual requests; trip on N consecutive failures | `Record(success bool)` |
| **Rate-based** | Batch jobs, periodic health checks; trip when failure % exceeds threshold | `RecordCycle(total, failed int)` |

Do not mix `Record` and `RecordCycle` on the same breaker instance.

```go
// Rate-based: trip when >20% of a cycle fails
b := breaker.New(breaker.Options{
    FailRateThreshold: 0.20,
    OpenDuration:      30 * time.Second,
})

// After each scan cycle:
b.RecordCycle(total, failed)
```

## Execute[T] vs manual Allow/Record

`Execute[T]` is the safe default. It calls `Allow`, runs your function, then
calls `Record` — always paired, even on panic-free early returns.

Use manual `Allow`/`Record` only when you need to inspect the result before
deciding how to record it:

```go
if !b.Allow() {
    return cached, nil // serve stale
}
resp, err := callUpstream(ctx)
// count 404 as success (endpoint is up), 5xx as failure
b.Record(err == nil || resp.StatusCode == http.StatusNotFound)
```

**Contract**: every `Allow()` that returns `true` MUST be paired with exactly
one `Record(bool)`. Skipping `Record` after a half-open probe permanently
wedges the breaker — the probe slot is never released.

## HTTPDoer preset

Wraps any `*http.Client` (or anything with a `Do` method). 5xx and transport
errors count as failures; 2xx–4xx count as successes.

```go
import (
    "net/http"
    "github.com/anatolykoptev/go-kit/breaker"
)

b := breaker.New(breaker.Options{Name: "search-api"})
client := breaker.NewHTTPDoer(http.DefaultClient, b)

// client.Do(req) — same interface as *http.Client
resp, err := client.Do(req)
if errors.Is(err, breaker.ErrOpen) {
    return nil, ErrUpstreamUnavailable
}
```

On 5xx the response body is closed and a sentinel error is returned — the
caller never sees the 5xx response object.

## Pool — per-key/tenant breakers

`Pool` lazily creates one `Breaker` per key using a shared factory function.
Useful for isolating per-tenant or per-domain failures.

```go
pool := breaker.NewPool(func(key string) breaker.Options {
    return breaker.Options{
        Name:          key,
        FailThreshold: 5,
        OpenDuration:  30 * time.Second,
    }
})

// Each tenant gets its own breaker; created on first Get.
b := pool.Get(tenantID)
result, err := breaker.Execute(b, func() (T, error) {
    return callTenantAPI(ctx, tenantID)
})

// Iterate for metrics/display.
for key, b := range pool.Snapshot() {
    log.Printf("%s: %s", key, b.State())
}
```

## Options reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Name` | `string` | `"breaker"` | Label for logs/metrics |
| `FailThreshold` | `uint32` | `5` | Consecutive failures before opening |
| `FailRateThreshold` | `float64` | `0` (disabled) | Rate trip threshold; `(0,1]`; requires `RecordCycle` |
| `OpenDuration` | `time.Duration` | `60s` | Base cooldown |
| `BackoffMultiplier` | `float64` | `1.0` | Multiply `OpenDuration` on each consecutive trip |
| `MaxOpenDuration` | `time.Duration` | `0` (no cap) | Cap on grown cooldown |
| `JitterPct` | `int` | `0` | ±N% random jitter on cooldown |
| `MaxHalfOpenCalls` | `uint32` | `1` | Probe slots in half-open state |
| `OnTrip` | `func(name string)` | nil | Hook fired when breaker opens (new goroutine) |
| `OnRecover` | `func(name string)` | nil | Hook fired when breaker closes (new goroutine) |

## Migration cheatsheet

| Old pattern | Breaker config |
|-------------|---------------|
| Fixed cooldown (same wait every trip) | `BackoffMultiplier: 1.0` (default) |
| Exponential cooldown (doubles each trip) | `BackoffMultiplier: 2.0`, `MaxOpenDuration: 5*time.Minute` |
| Rate-based trip (% of a batch cycle) | `FailRateThreshold: 0.20`, use `RecordCycle` |
