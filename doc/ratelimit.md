# ratelimit

In-process token-bucket limiter, per-key limiter with LRU eviction, in-process
concurrency limiter, and a Redis-backed sliding-window limiter for
multi-process services.

```
go get github.com/anatolykoptev/go-kit/ratelimit
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/ratelimit"

// 50 events/sec, burst of 100. One limiter shared by all goroutines.
lim := ratelimit.New(50, 100)

if !lim.Allow() {
    http.Error(w, "rate limit", 429)
    return
}

// Or block until a token is available, honouring ctx cancellation:
if err := lim.Wait(ctx); err != nil {
    return err
}
```

`New` returns `ErrLimiterZero` only via the constructor path of the per-key
variant; `Limiter` itself is always usable.

## Per-key limiter

Tracks an independent token bucket per string key (typically user ID, chat ID,
IP). Idle keys are reclaimed by periodic cleanup so the map doesn't grow
unbounded.

```go
kl := ratelimit.NewKeyLimiter(5, 10) // 5/sec, burst 10 — PER KEY
defer kl.Close()

// Optional background cleanup; otherwise call kl.Cleanup(maxIdle) yourself.
kl.StartCleanup(5*time.Minute, 30*time.Minute)

if !kl.Allow(userID) {
    return ErrThrottled
}
// or:
if err := kl.Wait(ctx, userID); err != nil {
    return err
}
```

`kl.Cleanup(maxIdle)` returns the number of evicted entries, useful for
metrics. `kl.Len()` reports the current count for the same purpose.

## Concurrency limiter (bulkhead)

Caps the number of in-flight operations regardless of rate.

```go
cl := ratelimit.NewConcurrencyLimiter(64)

release, err := cl.Acquire(ctx) // blocks until a slot is free
if err != nil {
    return err
}
defer release()

// inside the critical section, at most 64 goroutines run concurrently
```

`TryAcquire()` is the non-blocking variant; returns `(noop, false)` if the
limit is saturated. `Available()` and `Size()` expose live state.

## Sliding-window — Redis-backed, multi-process

The previous three are in-process. When traffic spans multiple instances
behind a load balancer, use the Redis-backed sliding window.

```go
sw := ratelimit.NewSlidingWindow(ratelimit.SlidingWindowConfig{
    Redis:     redisClient,         // any redis.Cmdable
    KeyPrefix: "ratelimit:webhook",
    Window:    10 * time.Minute,    // must be a whole number of minutes
    Limit:     1000,
    FailOpen:  true,                // on Redis error, allow the call
})

allowed, remaining, err := sw.Allow(ctx, userID)
```

`Window` is bucketed into 1-minute slots internally; the limiter sums the
current minute + prior `Window/1min - 1` minutes. `FailOpen` controls error
handling: `true` favours availability, `false` favours strict enforcement.

Reset a key (e.g. after admin unblock):

```go
err := sw.Reset(ctx, userID)
```

## Which one do I want?

| Need | Pick |
|------|------|
| Cap rate of a single endpoint, one process | `New(rate, burst)` |
| Per-user / per-tenant rate within one process | `NewKeyLimiter` |
| Cap in-flight concurrency (bulkhead) | `NewConcurrencyLimiter` |
| Per-user rate enforced across N processes | `NewSlidingWindow` (Redis) |

## API reference

| Type | Notes |
|------|-------|
| `Limiter` | Token bucket. `Allow()`, `Wait(ctx)` |
| `KeyLimiter` | Per-key buckets with cleanup. `Allow(key)`, `Wait(ctx, key)`, `Cleanup`, `StartCleanup`, `Len`, `Close` |
| `ConcurrencyLimiter` | Counted semaphore. `Acquire`, `TryAcquire`, `Available`, `Size` |
| `SlidingWindow` | Redis sliding window. `Allow`, `Reset` |
| `SlidingWindowConfig` | `Redis`, `KeyPrefix`, `Window`, `Limit`, `FailOpen`, `Now` (test hook) |
| `ErrLimiterZero` | Returned when `rate == 0 && burst < 1` (token can never become available) |

## Notes & contracts

- The in-process `Limiter` is **not safe** to share across processes — each
  instance has independent state.
- `KeyLimiter.Close()` stops `StartCleanup` if it was started. Forgetting to
  `Close` leaks a goroutine.
- `SlidingWindowConfig.Window` must be a multiple of one minute. Bucketing at
  finer granularity would explode Redis key counts; coarser-than-minute is
  rarely useful in practice.
- The `Now` hook on `SlidingWindowConfig` is for tests; do not wire it to a
  custom clock in production code.
