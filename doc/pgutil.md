# pgutil

Postgres connection helpers: bounded-retry `Connect` for blocking startup
dependencies, and `Lazy[T]` for optional / non-critical pools that should not
keep the service from booting.

```
go get github.com/anatolykoptev/go-kit/pgutil
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/pgutil"

// Blocking — retry until pg is reachable or attempts exhausted.
pool, err := pgutil.Connect(ctx, pgutil.Options{Name: "main"},
    func(ctx context.Context) (*pgxpool.Pool, error) {
        return pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    })
if err != nil {
    log.Fatal(err)
}
defer pool.Close()
```

`Connect[T]` is generic — it works with `*pgxpool.Pool`, `*sql.DB`, NATS,
Redis, anything that needs retrying-on-startup.

## Optional databases — `Optional: true`

For features that *can* degrade if a database is missing (analytics, archive,
audit log), return `(nil, nil)` instead of erroring:

```go
analytics, _ := pgutil.Connect(ctx, pgutil.Options{
    Name:     "analytics",
    Optional: true,
}, connectAnalytics)

if analytics != nil {
    go writeStatsLoop(analytics)
}
```

When `Optional` is true and all retries are exhausted, `Connect` logs a warn
and returns the zero value plus a nil error — the caller decides whether to
proceed.

## Lazy — connect in the background

`Lazy[T]` initialises a value in a background goroutine and exposes `Ready()`
+ `Get()` to consumers. Use when the dependency is non-blocking and you don't
want startup to wait at all.

```go
lazy := pgutil.NewLazy[*pgxpool.Pool](pgutil.Options{
    Name:           "archive",
    HealthInterval: 15 * time.Second,
}, connectFn, closeFn, healthFn)

lazy.Start(ctx)
defer lazy.Close()

// in a request handler:
if !lazy.Ready() {
    http.Error(w, "archive backend warming up", http.StatusServiceUnavailable)
    return
}
pool := lazy.Get()
```

The health check runs every `HealthInterval` and toggles `Ready()` if the
backend becomes unreachable after a successful start.

## Options reference

| Field | Default | Description |
|-------|---------|-------------|
| `MaxAttempts` | `10` | Retry count for `Connect` |
| `InitDelay` | `1s` | Initial backoff |
| `MaxDelay` | `30s` | Backoff cap |
| `Optional` | `false` | If true and exhausted, return zero value + nil error (and log warn) |
| `Logger` | `slog.Default()` | Per-attempt warn logs |
| `Name` | `""` | Identifies the connection in log lines |
| `HealthInterval` | `15s` | `Lazy` only; period of background health check |

## When to use which

| Scenario | Pick |
|----------|------|
| App can't start without this DB | `Connect` with `Optional: false` |
| App should warn-and-continue if DB is missing | `Connect` with `Optional: true` |
| App should boot instantly; the DB feature lights up when it's ready | `NewLazy` + `Start` |

## Errors

`Connect` wraps the underlying `connectFn` errors after retries are exhausted.
Use `errors.Is` to inspect — for example, `pgx.ErrNoRows` from a smoke-test
query inside the connect function is preserved unmodified.

## Notes

- Backoff and retry use [`kit/retry`](retry.md) internally with `Jitter: true`.
- `Lazy.Close()` cancels the background loop and calls `closeFn` exactly once,
  whether or not the value was ever Ready.
- For pool lifecycle inside a service, prefer one `Connect`+`defer Close` per
  database; don't share a `Lazy` between unrelated features.
