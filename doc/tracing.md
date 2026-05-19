# tracing

OpenTelemetry distributed-tracing helpers for go-kit services: one-call
`Setup`, a `Start` shortcut, HTTP server middleware that uses Go 1.22 route
patterns for span names, MCP server middleware (no canonical upstream exists),
and per-call sampling.

```
go get github.com/anatolykoptev/go-kit/tracing
```

## Quick start

```go
import (
    "context"
    "log/slog"

    "github.com/anatolykoptev/go-kit/tracing"
    "github.com/anatolykoptev/go-kit/tracing/httpmw"
    "go.opentelemetry.io/otel/attribute"
)

func main() {
    ctx := context.Background()

    shutdown, err := tracing.Setup(ctx, "go-search",
        tracing.WithSampleRatio(0.1), // 10% in prod
        tracing.WithAttributes(attribute.String("version", version)),
    )
    if err != nil {
        slog.Error("tracing setup", "error", err)
    }
    defer shutdown(context.Background())

    mux := http.NewServeMux()
    mux.HandleFunc("GET /research", handleResearch)

    handler := httpmw.Handler("go-search", mux)
    http.ListenAndServe(":8890", handler)
}

func handleResearch(w http.ResponseWriter, r *http.Request) {
    ctx, span := tracing.Start(r.Context(), "search.research",
        attribute.String("query", r.URL.Query().Get("q")))
    defer span.End()

    results, err := doSearch(ctx, r.URL.Query().Get("q"))
    if err != nil {
        tracing.RecordError(span, err)
        return
    }
    span.SetAttributes(attribute.Int("results.count", len(results)))
}
```

## Why this package

| Concern | Use this | Use canonical |
|---------|----------|---------------|
| TracerProvider bootstrap, propagators, OTLP/HTTP exporter | `tracing.Setup(ctx, "service")` | — (no established OSS helper, only 0-star repos) |
| HTTP server middleware | `tracing/httpmw.Handler` (route-pattern span names) | `otelhttp.NewHandler` directly when route-label customisation isn't needed |
| MCP server middleware | `tracing/mcpmw.Middleware("service")` | — (no canonical otelmcp exists upstream, verified 2026-04-30) |
| gRPC client/server | — | `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` |
| `database/sql` queries | — | `github.com/XSAM/otelsql` |

## MCP wiring

Use side-by-side with `metrics/mcpmw` for RED metrics + per-call spans.

```go
import (
    metricsmw "github.com/anatolykoptev/go-kit/metrics/mcpmw"
    tracemw   "github.com/anatolykoptev/go-kit/tracing/mcpmw"
)

mcpserver.Run(server, mcpserver.Config{
    MCPReceivingMiddleware: []mcp.Middleware{
        hooks.Middleware(),
        metricsmw.Middleware(reg, "tool"), // gowp_tool_calls_total etc.
        tracemw.Middleware("go-wp"),       // span per tools/call
    },
})
```

> **Middleware ordering**: tracing MUST wrap metrics, never the other way.
> `otelhttp` (and `tracing/httpmw`, which uses it) rewraps the request via
> `r.WithContext`, which breaks metrics middleware that reads `r.Pattern` from
> the original.

## Endpoint configuration

Resolved in this order (first match wins):

1. `tracing.WithEndpoint("…")` option — overrides everything.
2. `OTEL_EXPORTER_OTLP_ENDPOINT` env var.
3. Unset → no exporter created. `Start` returns no-op spans, but propagators
   still install so trace context flows through the process — matches the
   "no collector deployed" production posture.

Format: full URL with scheme — `http://host:port` (insecure) or
`https://host:port` (TLS). Matches the canonical OTel env-var spec. Bare
`host:port` is NOT supported — pass the scheme.

## Sampling

Default `1.0` (record all). Drop in prod with `WithSampleRatio(0.05)` for
high-traffic services. The sampler is `ParentBased(TraceIDRatioBased(r))` —
incoming sampled traces always continue, only roots get sampled.

```go
shutdown, _ := tracing.Setup(ctx, "go-search",
    tracing.WithSampleRatio(0.05),     // 5% of root spans
    tracing.WithBatchTimeout(2*time.Second),
)
```

## Subpackages

| Path | Notes |
|------|-------|
| `tracing/httpmw` | Server middleware. `Handler("service", mux)` wraps a `*http.ServeMux` and names spans from `r.Pattern`. Use `HandlerWithFormatter` for chi/gorilla/echo. |
| `tracing/mcpmw` | MCP server middleware. `Middleware("service")` emits a span per `tools/call`. |
| `tracing/pgxotel` | pgx hook for `database/sql`-style query spans (call sites' choice — XSAM/otelsql is the canonical alternative). |
| `tracing/slogh` | `slog.Handler` that injects active trace + span IDs into log records, so trace and log streams join in the backend. |

## Compatibility

- Built against `github.com/modelcontextprotocol/go-sdk v1.5+`.
- HTTP wrapper assumes Go 1.22+ `ServeMux` (uses `r.Pattern`); for
  chi/gorilla/echo use `HandlerWithFormatter` and supply your router's
  pattern accessor.
- Wire format is OTLP/HTTP only. Add OTLP/gRPC if the collector layer needs
  it (one extra exporter, ~10 LOC).

## API reference

| Symbol | Notes |
|--------|-------|
| `Setup(ctx, serviceName, ...Option) (ShutdownFunc, error)` | Boot the global TracerProvider; call `defer shutdown(ctx)` |
| `WithEndpoint(url)` | Override OTLP collector URL |
| `WithSampleRatio(r)` | `r` in `[0,1]`; default `1.0` |
| `WithBatchTimeout(d)` | Span-batcher flush interval |
| `WithAttributes(kv...)` | Resource attributes attached to every span |
| `Tracer(name) trace.Tracer` | Direct OTel Tracer access |
| `Start(ctx, name, attrs...) (ctx, span)` | Shortcut over `Tracer.Start` |
| `RecordError(span, err)` | Record + set status to error in one call |
| `ShutdownFunc` | Returned from `Setup`; flushes and shuts down the provider |
