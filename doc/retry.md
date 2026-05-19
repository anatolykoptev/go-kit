# retry

Generic retry loop with exponential backoff, jitter, total-wall-clock budget,
HTTP `Retry-After` parsing, and per-call success/abort predicates.

```
go get github.com/anatolykoptev/go-kit/retry
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/retry"

result, err := retry.Do(ctx, retry.Options{
    MaxAttempts:  5,
    InitialDelay: 200 * time.Millisecond,
    MaxDelay:     5 * time.Second,
    Jitter:       true,
}, func() (User, error) {
    return fetchUser(ctx, id)
})
```

`Do[T]` is generic; the function can return any type. `MaxAttempts: 0` is
treated as 1 (single attempt, no retry).

## RetryAfter — let the function dictate the next delay

Wrap an error with `RetryAfter(d, err)` to override backoff for that attempt
only. Useful for honouring rate-limit responses.

```go
_, err := retry.Do(ctx, opts, func() (Resp, error) {
    resp, err := http.Get(url)
    if resp.StatusCode == 429 {
        d, _ := time.ParseDuration(resp.Header.Get("Retry-After") + "s")
        return Resp{}, retry.RetryAfter(d, fmt.Errorf("rate-limited"))
    }
    return parse(resp)
})
```

The `HTTP` helper does this automatically by parsing the `Retry-After` header
(both delta-seconds and HTTP-date formats):

```go
resp, err := retry.HTTP(ctx, retry.Options{MaxAttempts: 4}, func() (*http.Response, error) {
    return http.Get(url)
})
```

`HTTP` retries on 5xx and 429 with the `Retry-After`-aware delay, and stops on
2xx / 4xx.

## AbortOn — never retry specific errors

```go
retry.Do(ctx, retry.Options{
    MaxAttempts: 5,
    AbortOn:     []error{context.DeadlineExceeded, sql.ErrNoRows},
}, fn)
```

Checked via `errors.Is`. The first match short-circuits and the error is returned
as-is (no wrapping).

## RetryableOnly — invert the default

By default every error triggers a retry until `MaxAttempts` is exhausted. With
`RetryableOnly: true`, only errors explicitly marked retryable are retried:

```go
err := callDB()
if isDeadlock(err) {
    return retry.MarkRetryable(err)
}
return retry.Permanent(err) // never retried even without RetryableOnly
```

| Helper | Effect |
|--------|--------|
| `MarkRetryable(err)` | Tag the error so `RetryableOnly` retries it; `IsRetryable(err)` reports true |
| `Permanent(err)` | Tag the error so `IsPermanent(err)` returns true; treated as non-retryable everywhere |

## Total wall-clock budget

`MaxElapsedTime` puts an upper bound on the loop. After it elapses, the current
error is returned without further attempts. Independent of `ctx.Done()`.

```go
retry.Do(ctx, retry.Options{
    MaxAttempts:    100,
    MaxElapsedTime: 30 * time.Second,
    InitialDelay:   100 * time.Millisecond,
}, fn)
```

## Custom retry predicate

`RetryIf` overrides `AbortOn` + `RetryableOnly`:

```go
retry.Do(ctx, retry.Options{
    RetryIf: func(err error) bool {
        var apiErr *llm.APIError
        return errors.As(err, &apiErr) && apiErr.Status >= 500
    },
}, fn)
```

## HTTPError sentinel

Surfaces a structured 5xx/429 from `retry.HTTP` consumers:

```go
var herr *retry.HTTPError
if errors.As(err, &herr) {
    log.Printf("upstream %d after %d attempts", herr.Status, herr.Attempt)
}
```

## Deterministic tests with `Timer`

`Timer` lets tests bypass real time. `Options.Timer = instantTimer{}` makes the
loop run as fast as the function does — no `time.Sleep`.

```go
opts := retry.Options{
    MaxAttempts:  10,
    InitialDelay: time.Second, // ignored — Timer fires immediately
    Timer:        &instantTimer{},
}
```

## Options reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `MaxAttempts` | int | `1` | Total attempt count (1 = no retry) |
| `InitialDelay` | time.Duration | `200ms` | First inter-attempt sleep |
| `MaxDelay` | time.Duration | `30s` | Cap on exponential growth |
| `MaxElapsedTime` | time.Duration | `0` (unlimited) | Wall-clock budget across the whole loop |
| `Jitter` | bool | `false` | Add ±25% random variation to each delay |
| `Backoff` | Backoff | exponential | Reserved for alt strategies; exponential ×2 today |
| `AbortOn` | []error | nil | Errors that immediately stop the loop (`errors.Is`) |
| `RetryableOnly` | bool | `false` | Retry only `MarkRetryable`-tagged errors |
| `RetryIf` | func(error) bool | nil | Custom predicate; overrides `AbortOn` + `RetryableOnly` |
| `OnRetry` | func(int, error) | nil | Called after each failed attempt with attempt index (1-based) |
| `Timer` | Timer | real `time.After` | Inject for tests |

## Migration cheatsheet

| Old (cenkalti/backoff) | retry equivalent |
|------------------------|------------------|
| `backoff.WithMaxRetries(b, 5)` | `retry.Options{MaxAttempts: 5}` |
| `backoff.Permanent(err)` | `retry.Permanent(err)` |
| `backoff.NewExponentialBackOff()` with `MaxElapsedTime` | `retry.Options{MaxElapsedTime: ...}` |
| Manual `Retry-After` parsing in callback | `retry.HTTP(ctx, opts, fn)` |

## What this package does NOT do

- Circuit breaking — see [breaker](breaker.md)
- Hedged parallel calls — see [hedge](hedge.md)
- Bulkhead / concurrency limit — see [ratelimit](ratelimit.md)

Compose those packages separately.
