# toolutil

Argument-extraction and error-formatting helpers for MCP tool handlers. Works
with the `map[string]any` argument maps produced by JSON-decoded tool inputs.

```
go get github.com/anatolykoptev/go-kit/toolutil
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/toolutil"

func handleSearch(ctx context.Context, args map[string]any) (string, error) {
    query := toolutil.ArgString(args, "query")
    limit := toolutil.ArgInt(args, "limit")
    if limit <= 0 {
        limit = 20
    }

    body, status, err := callUpstream(ctx, query, limit)
    if err != nil {
        return "", err
    }
    if msg := toolutil.CheckHTTPStatus(body, status); msg != "" {
        return "", errors.New(msg)
    }
    return string(body), nil
}
```

## Why this package exists

MCP tool calls deliver arguments as `map[string]any`. Reaching into the map
manually means writing the same `v, _ := args["query"].(string)` boilerplate
in every handler — and getting the int/float JSON dance wrong, because
`encoding/json` decodes numbers as `float64` unless you decode into a typed
struct. `toolutil` normalises this once.

## Argument extractors

| Function | JSON shape | Behaviour |
|----------|-----------|-----------|
| `ArgString(args, key) string` | string | empty `""` on missing / wrong type |
| `ArgStringDefault(args, key, def) string` | string | non-empty value or `def` |
| `ArgInt(args, key) int` | number | handles `int`, `int64`, `float64`; `0` on missing |
| `ArgFloat64(args, key) float64` | number | `0` on missing |
| `ArgBool(args, key) bool` | bool | `false` on missing |
| `ArgIntSlice(arr []any) []int` | array of numbers | filters non-numeric entries |

`ArgInt` and friends are intentionally silent on missing. If a tool needs to
distinguish "not provided" from "explicitly zero", validate the raw map first:

```go
if _, ok := args["page"]; !ok {
    return "", errors.New("page is required")
}
page := toolutil.ArgInt(args, "page")
```

## Error formatting

`CheckHTTPStatus(body, status)` returns an error string for non-2xx HTTP
responses, preferring a `"code"` field in a JSON body over `"message"`:

```go
if msg := toolutil.CheckHTTPStatus(body, resp.StatusCode); msg != "" {
    return "", errors.New(msg)
}
```

Why prefer `"code"`? It's always English (`"rest_post_invalid_id"`), while
`"message"` follows the server locale and may surface non-ASCII characters
that an LLM tool result can mishandle.

`SafeDate(d)` truncates ISO date strings to `YYYY-MM-DD` and returns
`"unknown"` for empty input. Useful when relaying upstream timestamps into
tool output where users only need the day.

## Miscellaneous

| Function | Notes |
|----------|-------|
| `Coalesce(vals ...string) string` | First non-empty value; rough analogue of SQL COALESCE |
| `TruncateStr(s, n) string` | Re-export of `strutil.Truncate`; prefer `strutil` directly in new code |
| `RecoverLog(context string)` | `defer toolutil.RecoverLog("tool:search")` — recovers from panics and logs stack trace via `slog` |

## Panic recovery in goroutines

Tool handlers that spawn goroutines (background fetches, fan-out) must recover
panics — an unrecovered panic in a goroutine kills the whole MCP server. The
canonical pattern:

```go
go func() {
    defer toolutil.RecoverLog("tool:search async-fetch")
    enrich(ctx, id)
}()
```

The recovered panic is logged with `slog.Error("panic recovered", context=,
panic=, stack=)`.

## API reference

| Symbol | Notes |
|--------|-------|
| `ArgString`, `ArgStringDefault`, `ArgInt`, `ArgFloat64`, `ArgBool`, `ArgIntSlice` | Argument extractors |
| `Coalesce` | First non-empty string |
| `TruncateStr` | Re-export of `strutil.Truncate` |
| `CheckHTTPStatus` | Format non-2xx HTTP response as error string |
| `SafeDate` | ISO date to `YYYY-MM-DD` or `"unknown"` |
| `RecoverLog` | `defer`-friendly panic handler with structured log |

## Notes

- The package is intentionally tiny and dependency-light. If you find yourself
  reaching for shared business logic, that probably belongs in your service,
  not in `toolutil`.
- For full MCP request/response middleware (metrics, tracing), see
  [`metrics/mcpmw`](metrics.md) and [`tracing/mcpmw`](tracing.md).
