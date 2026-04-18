# go-kit

Shared infrastructure for [go-*](https://github.com/anatolykoptev) MCP servers. One module, zero bloat.

```
go get github.com/anatolykoptev/go-kit
```

## Packages

| Package | What | Deps |
|---------|------|------|
| [`env`](#env) | Environment variable parsing | stdlib |
| [`llm`](#llm) | OpenAI-compatible LLM client with streaming, tool calling, structured output | stdlib |
| [`cache`](#cache) | L1 memory + L2 Redis tiered cache with S3-FIFO eviction | stdlib (L2: redis) |
| [`retry`](#retry) | Generic retry with exponential backoff | stdlib |
| [`metrics`](#metrics) | Atomic counters, gauges, timers, labels, sinks, rates, histograms, TTL | stdlib |
| [`hedge`](#hedge) | Hedged requests — race primary vs backup, first success wins | stdlib |
| [`ratelimit`](#ratelimit) | Token bucket rate limiter, per-key support, concurrency limiter | stdlib |
| [`strutil`](#strutil) | Unicode-aware string helpers with case conversion | stdlib |
| [`fileopt`](#fileopt) | Lossless PDF/PNG/WebP byte-level optimization via gs+qpdf/oxipng/cwebp subprocess wrappers, with per-stage Prometheus metrics | stdlib + prometheus/client_golang |

All packages are independent — no internal cross-imports. Import only what you need.

---

### env

```go
import "github.com/anatolykoptev/go-kit/env"

port := env.Int("PORT", 8080)
debug := env.Bool("DEBUG", false)

// Docker secrets / Kubernetes volumes
dbPass := env.File("DB_PASSWORD_FILE", "")

// Variable expansion
dbURL := env.Expand("DATABASE_URL", "postgres://localhost:5432/mydb")

// Binary data
cert := env.Base64("TLS_CERT", nil)
key := env.Hex("API_KEY_HEX", nil)

// Testability — decouple from os.Getenv
env.DefaultSource = env.MapSource(map[string]string{
    "PORT": "9090",
})
```

Functions: `Str`, `Int`, `Int64`, `Uint`, `Uint64`, `Float`, `Bool`, `Duration`, `List`, `Int64List`, `Map`, `URL`, `File`, `Expand`, `Base64`, `Hex`.

- Source interface for testability (MapSource for parallel-safe tests)
- File: read Docker secrets and Kubernetes volumes
- Expand: resolve ${VAR} references
- Base64/Hex: binary data from env vars

```go
headers := env.Map("HEADERS", "")              // "Content-Type:json,Accept:*/*" → map
endpoint := env.URL("API_URL", "http://localhost:8080")  // parsed *url.URL
maxConns := env.Uint("MAX_CONNS", 100)
```

#### Error handling

```go
// Error-returning variants — return ParseError on invalid values
port, err := env.IntE("PORT", 8080)        // err if PORT="abc"
debug, err := env.BoolE("DEBUG", false)     // err if DEBUG="maybe"
timeout, err := env.DurationE("TIMEOUT", 30*time.Second) // accepts "5s", "100ms", "2m30s"

// Required — must be set, returns NotSetError if missing
dbURL, err := env.Required("DATABASE_URL")

// Lookup — distinguish "not set" from "set to empty"
val, ok := env.Lookup("OPTIONAL_VAR")

// Must* — panic on invalid (for fail-fast main() init)
dbURL := env.MustRequired("DATABASE_URL")
port := env.MustInt("PORT", 8080)
```

### llm

```go
import "github.com/anatolykoptev/go-kit/llm"

client := llm.NewClient(baseURL, apiKey, model,
    llm.WithFallbackKeys([]string{backupKey}),
    llm.WithMaxTokens(16384),
    llm.WithTemperature(0.1),
)

// Simple text completion (unchanged)
response, err := client.Complete(ctx, systemPrompt, userPrompt)

// Full chat with tool calling
resp, err := client.Chat(ctx, messages,
    llm.WithTools([]llm.Tool{
        llm.NewTool("get_weather", "Get weather for a city", params),
    }),
)
for _, call := range resp.ToolCalls { ... }
fmt.Printf("Tokens: %d\n", resp.Usage.TotalTokens)

// Structured output — auto-generates JSON Schema from struct
// Schema constraint tags enrich the JSON Schema for better LLM output:
type User struct {
    Name string `json:"name" jsonschema:"description=Full legal name"`
    Age  int    `json:"age"  jsonschema:"minimum=0,maximum=150"`
    Role string `json:"role" jsonschema:"enum=admin|user|guest"`
}
var recipe Recipe
err := client.ChatTyped(ctx, messages, &recipe)

// SSE streaming
stream, err := client.Stream(ctx, messages)
defer stream.Close()
for chunk, ok := stream.Next(); ok; chunk, ok = stream.Next() {
    fmt.Print(chunk.Delta)
}

// Structured extraction with validation retry (Instructor-style)
type Recipe struct {
    Name        string   `json:"name"`
    Ingredients []string `json:"ingredients"`
}
var recipe Recipe
err := client.Extract(ctx, messages, &recipe,
    llm.WithValidator(func(v any) error {
        r := v.(*Recipe)
        if len(r.Ingredients) == 0 {
            return errors.New("need at least one ingredient")
        }
        return nil
    }),
)

// Union types — LLM chooses between multiple response types
type SearchAction struct {
    Query string `json:"query"`
}
type AnswerAction struct {
    Answer string `json:"answer"`
}
result, err := client.ExtractOneOf(ctx, messages, []llm.VariantDef{
    llm.Variant("search", SearchAction{}),
    llm.Variant("answer", AnswerAction{}),
})
switch v := result.(type) {
case *SearchAction:
    fmt.Println("Search:", v.Query)
case *AnswerAction:
    fmt.Println("Answer:", v.Answer)
}

// Model-level fallback chains
client = llm.NewClient("", "", "",
    llm.WithEndpoints([]llm.Endpoint{
        {URL: geminiURL, Key: key1, Model: "gemini-2.5-flash"},
        {URL: openaiURL, Key: key2, Model: "gpt-4o"},
    }),
)

// Request/response middleware
client = llm.NewClient(baseURL, apiKey, model,
    llm.WithMiddleware(func(ctx context.Context, req *llm.ChatRequest,
        next func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
        start := time.Now()
        resp, err := next(ctx, req)
        log.Printf("LLM call took %v", time.Since(start))
        return resp, err
    }),
)
```

- Structured errors: `APIError{StatusCode, Type, Body, Retryable}` — use `errors.As` to branch on error type
- Retry on 429/5xx with exponential backoff
- Automatic fallback key cycling
- SSE streaming via `Stream`/`Next`
- Tool/function calling via `Chat` + `WithTools`
- Structured output via `ChatTyped` + auto JSON Schema
- Extract with validation retry (Instructor-style, no Go library does this)
- Union types via `ExtractOneOf` — LLM picks between response variants
- Model-level endpoint fallback chains
- Request/response middleware for logging, metrics, caching
- Token usage reporting in `ChatResponse`
- Multimodal support via `CompleteMultimodal`
- JSON extraction from LLM output via `ExtractJSON`
- Schema constraint tags: `jsonschema:"description=...,minimum=0,enum=a|b|c"` for richer schemas

### cache

```go
import "github.com/anatolykoptev/go-kit/cache"

// L1-only (no Redis dependency at runtime)
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
})

// L1 + L2 Redis (read-through, write-through)
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
    RedisURL:   "redis://localhost:6379",
    RedisDB:    0,
    Prefix:     "myapp:",
    L2TTL:      24 * time.Hour,
})

// Custom L2 store (testing or alternative backends)
c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
c.WithL2(myCustomStore)

defer c.Close()

c.Set(ctx, "key", data)
data, ok := c.Get(ctx, "key")

// Cache-aside with singleflight (concurrent loads deduplicated)
data, err := c.GetOrLoad(ctx, "key", func(ctx context.Context) ([]byte, error) {
    return fetchFromDB(ctx, "key")
})

// Statistics
stats := c.Stats()
fmt.Printf("Hit ratio: %.1f%%, Evictions: %d\n", stats.HitRatio*100, stats.Evictions)
```

**Per-key TTL** — override global TTL for individual entries:

```go
// Short TTL for fast-changing data (e.g. job listings)
c.SetWithTTL(ctx, "jobs:123", data, 15*time.Minute)

// Cache-aside with custom TTL
data, err := c.GetOrLoadWithTTL(ctx, "company:456", 24*time.Hour,
    func(ctx context.Context) ([]byte, error) {
        return fetchCompanyData(ctx, "456")
    },
)
```

- L1 memory cache with S3-FIFO eviction for high hit rates
- L2 Redis: optional, graceful degradation (L1-only if Redis unreachable)
- Read-through: L1 miss → L2 hit → automatic L1 promotion
- Write-through: Set/Delete propagate to both tiers
- L2 interface: plug in custom backends for testing or alternatives
- GetOrLoad with inline singleflight (prevents thundering herd)
- TTL jitter (prevents cache stampedes)
- Evictions counter + HitRatio in Stats
- Background cleanup, TTL expiry
- OnEvict callback for eviction notifications (expired, capacity, explicit)
- Tag-based invalidation: group entries by tags, invalidate in bulk
- Typed JSON helpers: generic `SetJSON`/`GetJSON`/`GetOrLoadJSON`

**Tag-based invalidation** — group and invalidate related entries:

```go
c.SetWithTags(ctx, "user:1:profile", data, []string{"user:1", "profile"})
c.SetWithTags(ctx, "user:1:settings", data, []string{"user:1"})

n := c.InvalidateByTag(ctx, "user:1") // removes both entries, returns 2
tags := c.Tags("user:1:profile")      // []string{"user:1", "profile"}
```

**Typed JSON cache** — generic wrappers over `[]byte` API:

```go
cache.SetJSON(c, ctx, "user:1", User{Name: "Alice", Age: 30})

user, ok, err := cache.GetJSON[User](c, ctx, "user:1")

user, err := cache.GetOrLoadJSON[User](c, ctx, "user:1", func(ctx context.Context) (User, error) {
    return fetchUser(ctx, 1)
})
```

**OnEvict callback** — react to cache evictions:

```go
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
    OnEvict: func(key string, data []byte, reason cache.EvictReason) {
        switch reason {
        case cache.EvictCapacity:
            metrics.Incr("cache.evict.capacity")
        case cache.EvictExpired:
            metrics.Incr("cache.evict.expired")
        case cache.EvictExplicit:
            metrics.Incr("cache.evict.explicit")
        }
    },
})
```

### hedge

```go
import "github.com/anatolykoptev/go-kit/hedge"

// Start fn; if no response after 1s, launch a second call in parallel.
// First success wins, loser is cancelled automatically.
result, err := hedge.Do(ctx, time.Second, func(ctx context.Context) (string, error) {
    return callLLM(ctx)
})

// Zero/negative delay: run fn once, no goroutines.
result, err := hedge.Do(ctx, 0, fn)
```

- Generic `Do[T any]` — works with any return type
- Shared derived context — `defer cancel()` auto-cleans loser goroutine
- Primary fails before delay — returns error immediately, no hedge
- Buffered channel prevents goroutine leaks

### ratelimit

```go
import "github.com/anatolykoptev/go-kit/ratelimit"

// Single rate limiter: 10 requests/sec, burst of 5
lim := ratelimit.New(10, 5)
if lim.Allow() {
    // proceed
}

// Blocking wait (respects context cancellation)
err := lim.Wait(ctx)

// Per-key rate limiting (per-domain, per-API-key)
kl := ratelimit.NewKeyLimiter(5, 3) // 5/sec per key, burst 3
defer kl.Close()

kl.Allow("api.linkedin.com")
kl.Wait(ctx, "api.twitter.com")

// Background cleanup of idle limiters
kl.StartCleanup(time.Minute, 10*time.Minute)
```

**Concurrency limiter** (bulkhead pattern):

```go
// Limit to 5 concurrent operations
cl := ratelimit.NewConcurrencyLimiter(5)
release, err := cl.Acquire(ctx) // blocking; respects context
if err != nil { return err }
defer release()

// Non-blocking variant
release, ok := cl.TryAcquire()
cl.Available() // free slots
cl.Size()      // max slots
```

- Token bucket algorithm, zero external deps
- Non-blocking `Allow()` and blocking `Wait(ctx)`
- Per-key limiters with automatic idle cleanup
- Concurrency limiter (semaphore-based, blocking + non-blocking acquire)
- Goroutine-safe

### retry

```go
import "github.com/anatolykoptev/go-kit/retry"

result, err := retry.Do(ctx, retry.Options{
    MaxAttempts:    5,
    InitialDelay:  500 * time.Millisecond,
    MaxDelay:      10 * time.Second,
    MaxElapsedTime: 30 * time.Second,  // total budget
    Jitter:        true,               // ±25% random jitter
}, func() (string, error) {
    return callAPI()
})

// HTTP-specific: retries on 429/5xx, auto-parses Retry-After header
resp, err := retry.HTTP(ctx, retry.Options{Jitter: true}, doRequest)

// Override backoff from fn:
return "", retry.RetryAfter(5*time.Second, err)

// Abort on specific errors (never retry)
retry.Do(ctx, retry.Options{
    AbortOn: []error{context.DeadlineExceeded},
}, fn)

// Opt-in retry: only marked errors are retried
retry.Do(ctx, retry.Options{RetryableOnly: true}, func() (T, error) {
    return result, retry.MarkRetryable(err) // will retry
})

// Permanent error — stop retrying immediately
retry.Do(ctx, retry.Options{MaxAttempts: 5}, func() (T, error) {
    if isFatal(err) {
        return zero, retry.Permanent(err) // unwrapped and returned
    }
    return zero, err
})

// OnRetry callback — log each failed attempt
retry.Do(ctx, retry.Options{
    MaxAttempts: 5,
    OnRetry: func(attempt int, err error) {
        log.Printf("attempt %d failed: %v", attempt, err)
    },
}, fn)

// RetryIf — custom predicate (overrides AbortOn + RetryableOnly)
retry.Do(ctx, retry.Options{
    MaxAttempts: 5,
    RetryIf: func(err error) bool {
        var netErr net.Error
        return errors.As(err, &netErr) && netErr.Temporary()
    },
}, fn)
```

- AbortOn: never retry specific errors (e.g. context.DeadlineExceeded)
- RetryableOnly + MarkRetryable: opt-in retry mode for production safety
- RetryIf: custom predicate — full control over which errors to retry
- Permanent(err): signal from fn to stop retrying immediately
- OnRetry callback: logging/metrics per failed attempt
- Context error wrapping: `errors.Is(err, context.DeadlineExceeded)` works on timeout

### metrics

```go
import "github.com/anatolykoptev/go-kit/metrics"

reg := metrics.NewRegistry()

// Counters
reg.Incr("requests")
reg.Add("bytes", 1024)

// Gauges — track current values
reg.Gauge("connections").Inc()
reg.Gauge("cpu").Set(45.2)
reg.Gauge("queue").Dec()

// Timer — one-liner duration tracking
defer reg.StartTimer("api.latency").Stop()

// Labels — dimensional metrics
reg.Incr(metrics.Label("requests", "method", "GET"))
reg.Incr(metrics.Label("requests", "method", "POST"))

// Rate tracking (EWMA)
rate := reg.Rate("events")
rate.Update(1) // record event
rate.M1()      // events/sec, 1-minute window

// Histogram (percentiles via reservoir sampling)
h := reg.Histogram("latency")
h.Update(12.5) // record observation
snap := h.Snapshot()
// snap.P50, snap.P95, snap.P99, snap.Min, snap.Max, snap.Mean

// TTL for dynamic metrics
reg.IncrWithTTL(metrics.Label("api.calls", "path", "/users"), 10*time.Minute)
reg.CleanupExpired() // remove stale metrics

// Snapshot and reset (for periodic reporting)
all := reg.SnapshotAndReset() // reads + zeros atomically

// Output formatting
reg.WriteTo(os.Stdout, metrics.TextSink{})  // key=value lines
reg.WriteTo(os.Stdout, metrics.JSONSink{})  // JSON object
```

- Gauge type with lock-free float64 (Set/Add/Inc/Dec)
- StartTimer/Stop for one-liner latency tracking
- Label() for dimensional metric keys
- Rate (EWMA): events/sec with 1/5/15-minute moving averages
- Histogram: reservoir sampling for P50/P95/P99 without unbounded memory
- TTL: auto-expire stale per-endpoint/per-user metrics
- SnapshotAndReset for atomic read-and-zero
- Sink interface with TextSink and JSONSink

### strutil

```go
import "github.com/anatolykoptev/go-kit/strutil"

s := strutil.Truncate("Hello, world!", 5)       // "Hello..."
s = strutil.TruncateAtWord("Hello, world!", 8)  // "Hello,..."
s = strutil.TruncateMiddle("path/to/file.go", 10) // "path/...e.go"

// Custom placeholder
s = strutil.TruncateWith("Hello, world!", 5, "[...]")  // "Hello[...]"

// Case conversions
s = strutil.ToSnakeCase("myVariableName")  // "my_variable_name"
s = strutil.ToCamelCase("my_variable")     // "myVariable"
s = strutil.ToKebabCase("myVariableName")  // "my-variable-name"
s = strutil.ToPascalCase("my_variable")    // "MyVariable"

// Word wrap
wrapped := strutil.WordWrap("long text here...", 80)

// Clean invalid UTF-8
clean := strutil.Scrub(untrustedInput)

// Check all substrings present
strutil.ContainsAll(s, []string{"foo", "bar"})

ok := strutil.Contains([]string{"a", "b"}, "a")    // true
ok = strutil.ContainsAny("hello world", []string{"world"}) // true
```

- WordWrap: wrap text at word boundaries
- Scrub: replace invalid UTF-8 with U+FFFD
- ContainsAll: check all substrings present

## Consumers

| Service | Packages used |
|---------|--------------|
| [go-search](https://github.com/anatolykoptev/go-search) | cache, env, strutil |
| [go-job](https://github.com/anatolykoptev/go-job) | cache, env, llm, metrics, strutil |
| [go-wp](https://github.com/anatolykoptev/go-wp) | cache, env, llm, metrics, strutil |
| [go-code](https://github.com/anatolykoptev/go-code) | cache, env, llm |
| [go-hully](https://github.com/anatolykoptev/go-hully) | cache, env, llm, metrics, strutil |
| [go-startup](https://github.com/anatolykoptev/go-startup) | cache, env, llm, metrics, retry, strutil |
| [go-nerv](https://github.com/anatolykoptev/go-nerv) | env, llm, metrics |
| [gigiena-teksta](https://github.com/anatolykoptev/gigiena-teksta) | env, llm, metrics |

## fileopt

Lossless byte-level optimization for PDF/PNG/WebP via subprocess wrappers over `gs`+`qpdf`, `oxipng`, and `cwebp`. Designed for services that generate or receive documents and want to reduce payload size before disk writes, uploads, or LLM input.

```go
import "github.com/anatolykoptev/go-kit/fileopt"

// Dispatch by extension
opt, err := fileopt.OptimizeBytes(ctx, data,
    fileopt.KindFromExt(filepath.Ext(filename)),
    fileopt.LevelEbook, 80)

// Or call specific optimizer
opt, err := fileopt.OptimizePNG(ctx, data)
opt, err := fileopt.OptimizePDF(ctx, data, fileopt.LevelEbook)
opt, err := fileopt.OptimizeWebP(ctx, data, 80)

// Expose Prometheus metrics
mux.Handle("/metrics", fileopt.MetricsHandler())
```

**Guarantees:**
- Lossless-by-default: size-bailout guard returns original when a stage would grow the file (cwebp gradient anti-pattern).
- Content-aware: text-only PDFs skip gs stage (10-16× speedup; qpdf alone carries the work).
- Per-stage Prometheus metrics: `gokit_fileopt_{calls_total, duration_seconds, ratio, bytes_before_total, bytes_after_total}` labeled by `stage` (gs/qpdf/oxipng/cwebp) and `result` (success/skipped/error).

**System binary overrides:** `FILEOPT_GS_PATH`, `FILEOPT_QPDF_PATH`, `FILEOPT_OXIPNG_PATH`, `FILEOPT_CWEBP_PATH`. Missing binary → warn log + original bytes (never fails the caller).

## License

MIT
