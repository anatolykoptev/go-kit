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
| [`cache`](#cache) | L1 memory cache with S3-FIFO eviction | stdlib |
| [`retry`](#retry) | Generic retry with exponential backoff | stdlib |
| [`metrics`](#metrics) | Atomic operation counters | stdlib |
| [`strutil`](#strutil) | Unicode-aware string helpers with case conversion | stdlib |

All packages are independent — no internal cross-imports. Import only what you need.

---

### env

```go
import "github.com/anatolykoptev/go-kit/env"

port := env.Str("PORT", "8080")
workers := env.Int("WORKERS", 4)
debug := env.Bool("DEBUG", false)
timeout := env.Duration("TIMEOUT", 30*time.Second)
hosts := env.List("ALLOWED_HOSTS", "localhost")  // comma-separated
```

Functions: `Str`, `Int`, `Int64`, `Uint`, `Uint64`, `Float`, `Bool`, `Duration`, `List`, `Int64List`, `Map`, `URL`.

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
var recipe Recipe
err := client.ChatTyped(ctx, messages, &recipe)

// SSE streaming
stream, err := client.Stream(ctx, messages)
defer stream.Close()
for chunk, ok := stream.Next(); ok; chunk, ok = stream.Next() {
    fmt.Print(chunk.Delta)
}
```

- Retry on 429/5xx with exponential backoff
- Automatic fallback key cycling
- SSE streaming via `Stream`/`Next`
- Tool/function calling via `Chat` + `WithTools`
- Structured output via `ChatTyped` + auto JSON Schema
- Token usage reporting in `ChatResponse`
- Multimodal support via `CompleteMultimodal`
- JSON extraction from LLM output via `ExtractJSON`

### cache

```go
import "github.com/anatolykoptev/go-kit/cache"

c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
})
defer c.Close()

c.Set(ctx, key, data)
data, ok := c.Get(ctx, key)
key := c.Key("prefix", query)  // deterministic FNV-128a key
```

L1 memory cache with S3-FIFO eviction for high hit rates. Background cleanup, TTL expiry. L2 Redis planned.

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
```

### metrics

```go
import "github.com/anatolykoptev/go-kit/metrics"

reg := metrics.NewRegistry()
reg.Incr("requests")
reg.Add("bytes_sent", 1024)

snap := reg.Snapshot()  // map[string]int64
fmt.Print(reg.Format()) // "bytes_sent=1024\nrequests=1\n"

err := reg.TrackOperation("api_calls", "api_errors", func() error {
    return callAPI()
})
```

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

ok := strutil.Contains([]string{"a", "b"}, "a")    // true
ok = strutil.ContainsAny("hello world", []string{"world"}) // true
```

## Consumers

| Service | Packages used |
|---------|--------------|
| [go-code](https://github.com/anatolykoptev/go-code) | env, llm |
| go-search | planned |
| go-job | planned |
| go-startup | planned |
| go-wp | planned |
| go-hully | planned |
| go-content | planned |
| gigiena-teksta | planned |

## License

MIT
