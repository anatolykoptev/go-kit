# metrics

Zero-dep observability toolkit: counters, gauges, histograms (reservoir), rates
(EWMA), labels, TTL, named timers, Prometheus bridge, HTTP + MCP middleware.

```
go get github.com/anatolykoptev/go-kit/metrics
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/metrics"

reg := metrics.NewRegistry()

reg.Incr("requests_total")
reg.Add("bytes_in_total", int64(n))
reg.Gauge("queue_depth").Set(42)
reg.Histogram("request_duration_seconds").Update(0.123)
reg.Rate("events_per_sec").Update(1)

fmt.Println(reg.Format())
```

For Prometheus exposition, use `NewPrometheusRegistry` instead — it scrapes via
`promhttp` and is the default for services already in the gowp/gonerv/gosearch
mesh.

```go
reg := metrics.NewPrometheusRegistry("gosearch")
mux.Handle("/metrics", metrics.MetricsHandler())
```

`FromEnv(defaultNamespace)` picks the right backend based on
`METRICS_PROMETHEUS_NAMESPACE` env (set → prometheus; unset → in-memory).

## Naming conventions

The `NAMING.md` doc in the package ships the project-wide rules. Summary:

- Counters MUST end in `_total`.
- Histograms / timers MUST end in `_seconds`, `_bytes`, or `_ratio`.
- Gauges use the base unit name (no suffix).
- Service-namespace prefix everywhere (`gowp_*`, `gosearch_*`, …) — pass it to
  `NewPrometheusRegistry`.
- Per-request dimensions go through `metrics.Label(name, kv...)`, never via
  string concatenation on the name (that produces a cardinality bomb and
  invalid Prometheus identifiers).

```go
reg.Incr(metrics.Label("api_requests_total",
    "endpoint", endpoint,
    "status", "200",
))
```

## Counters: SnapshotAndReset

Atomic read-and-zero pattern for periodic flush jobs:

```go
go func() {
    tick := time.NewTicker(15 * time.Second)
    for range tick.C {
        snap := reg.SnapshotAndReset()
        flushToInflux(snap)
    }
}()
```

Snapshot returns the values; the registry's counters are zeroed in the same
critical section so no event is double-counted or dropped.

## Timers

```go
t := reg.StartTimer("db_query_seconds")
result, err := db.Query(ctx, sql)
t.Stop() // records elapsed into the histogram named "db_query_seconds"
```

`defer reg.StartTimer("x").Stop()` is the canonical one-liner for function-level
timing.

## Rate / EWMA

Sliding 1/5/15-minute exponentially-weighted moving averages — the same maths
you'd see in `uptime` load averages, but applied to event rate:

```go
r := reg.Rate("ingest_events_total")
r.Update(1) // call on each event

snap := r.Snapshot()
log.Printf("m1=%.2f m5=%.2f m15=%.2f total=%d", snap.M1, snap.M5, snap.M15, snap.Total)
```

## Histograms — Reservoir sampling

Bounded-memory sampling for P50/P95/P99 over open-ended streams. Fixed 2K
sample size; eviction is uniform random — every event has equal probability of
remaining in the sample regardless of when it arrived.

```go
h := reg.Histogram("request_duration_seconds")
h.Update(0.142)

s := h.Snapshot()
log.Printf("p50=%.3f p95=%.3f p99=%.3f n=%d", s.P50, s.P95, s.P99, s.Count)
```

## TTL — auto-expire stale metrics

Per-user / per-endpoint counters explode in cardinality if you keep them
forever. `SetTTL` / `IncrWithTTL` / `AddWithTTL` flag entries for cleanup:

```go
reg.SetTTL("logins_by_user_total", 24*time.Hour)
reg.IncrWithTTL(metrics.Label("logins_by_user_total", "user", userID), 24*time.Hour)

removed := reg.CleanupExpired() // call periodically (e.g. cron, ticker)
```

## Sinks — custom output

`Sink` is the formatter interface (`TextSink`, `JSONSink` shipped).

```go
reg.WriteTo(os.Stdout, metrics.JSONSink{})
```

Implement your own to push to a custom backend without depending on the full
Prometheus exposition format.

## TrackCall / TrackCallTimed — wrap-and-measure

Common shape: "run this, increment success/error counters, optionally record
latency". One-liner instead of repeated boilerplate.

```go
err := metrics.TrackCallTimed(reg, "calls_total", "errors_total", "duration_seconds",
    func() error { return upstreamCall(ctx) },
)
```

## HTTP middleware (`metrics/httpmw`)

Records request count, in-flight gauge, status, and optionally duration /
response-size for an `http.Handler`.

```go
import httpmw "github.com/anatolykoptev/go-kit/metrics/httpmw"

mw := httpmw.Middleware(reg, "tool",
    httpmw.WithStdlibPattern(),                                    // use r.Pattern (Go 1.22+) as label
    httpmw.WithDurationHistogram(promRegisterer, 0.01, 0.1, 1, 5), // opt-in latency histogram
    httpmw.WithResponseSize(),                                     // record response body size
)

mux := http.NewServeMux()
mux.Handle("GET /search", searchHandler())
http.ListenAndServe(":8080", mw(mux))
```

`WithPathLabel(fn)` lets you collapse parametric routes (`/users/{id}`) to a
template label and avoid cardinality blow-up.

> **Middleware ordering**: when combining `metrics/httpmw` with
> `tracing/httpmw` and `otelhttp`, tracing MUST wrap metrics, never the other
> way (otelhttp rewraps the request with `WithContext`, breaking metrics that
> read `r.Pattern` from the original).

## MCP middleware (`metrics/mcpmw`)

Same shape as the HTTP middleware but for `modelcontextprotocol/go-sdk`. Counts
calls per tool, records errors and durations, and gauges in-flight calls.

```go
import mcpmw "github.com/anatolykoptev/go-kit/metrics/mcpmw"

mcpserver.Run(srv, mcpserver.Config{
    MCPReceivingMiddleware: []mcp.Middleware{
        mcpmw.Middleware(reg, "tool"),
    },
})
```

## API reference

| Type | Notes |
|------|-------|
| `Registry` | All-in-one container; safe for concurrent use |
| `Gauge` | `Set`, `Add`, `Inc`, `Dec`, `Value` |
| `Reservoir` (histogram) | `Update`, `Percentile(p)`, `Snapshot` |
| `Rate` | `Update`, `M1`, `M5`, `M15`, `Total`, `Snapshot` |
| `TimerHandle` | returned by `StartTimer`, `Stop` records duration |
| `Sink` / `TextSink` / `JSONSink` | output formatters |
