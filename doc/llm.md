# llm

Production-grade chat-completion client for OpenAI-compatible APIs with
streaming, tool calling, structured output via `Extract[T]`, endpoint
fallback chains, request middleware, and an integrated circuit breaker.

```
go get github.com/anatolykoptev/go-kit/llm
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/llm"

c := llm.NewClient("https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"), "gpt-4o-mini",
    llm.WithMaxRetries(2),
    llm.WithTemperature(0.2),
)

reply, err := c.Complete(ctx, "You are a helpful assistant.", "Say hi.")
if err != nil {
    return err
}
fmt.Println(reply)
```

## Three call shapes

| Method | Returns | Use when |
|--------|---------|----------|
| `Complete(ctx, system, user, ...)` | plain text | one-shot string answer |
| `CompleteMultimodal(ctx, prompt, images, ...)` | plain text | prompt + base64/URL images |
| `CompleteRaw(ctx, messages, ...)` | plain text | you build the message slice |
| `Chat(ctx, messages, ...)` | `*ChatResponse` | you need tool calls, usage, finish_reason |
| `ChatTyped(ctx, messages, target)` | fills `target` | one-shot structured output, no retry |
| `Extract(ctx, messages, target, ...)` | validates & retries | structured output with validation feedback |
| `Stream(ctx, messages, ...)` | `*StreamResponse` | SSE chunks (incremental text + final usage) |
| `StreamExtract(ctx, messages, ...)` | `*ExtractStream` | streamed text + final typed extraction |

## Extract[T] — structured output with validation retry

The differentiating feature. Builds a JSON Schema from struct tags, sends it as
`response_format`, validates the response, and retries up to N times with the
validator error fed back into the conversation.

```go
type Verdict struct {
    Severity string `json:"severity" jsonschema:"enum=low|medium|high"`
    Reason   string `json:"reason"   jsonschema:"required"`
}

var v Verdict
err := c.Extract(ctx, messages, &v,
    llm.WithValidator(func(x any) error {
        got := x.(*Verdict)
        if got.Reason == "" {
            return fmt.Errorf("reason must not be empty")
        }
        return nil
    }),
    llm.WithExtractRetries(2),
)
```

`SchemaOf(v)` exposes the inferred schema if you want to inspect it.

`ExtractOneOf` lets you ask the model to pick one of several typed variants — useful
for action selection ("search vs answer vs ask for clarification"):

```go
out, name, err := c.ExtractOneOf(ctx, messages,
    llm.Variant("search", llm.SearchAction{}),
    llm.Variant("answer", llm.AnswerAction{}),
)
```

## Endpoint fallback chains

Cycle keys *and* base URLs when an endpoint goes down or rate-limits.

```go
c := llm.NewClient("https://api.openai.com/v1", primaryKey, "gpt-4o-mini",
    llm.WithEndpoints([]llm.Endpoint{
        {URL: "https://api.openai.com/v1",   Key: primaryKey,  Model: "gpt-4o-mini"},
        {URL: "https://api.deepinfra.com/v1/openai", Key: backupKey, Model: "Qwen/Qwen2.5-72B-Instruct"},
    }),
)
```

On `429`, transport errors, or 5xx, the client advances to the next endpoint and
re-issues. `WithFallbackKeys` is the legacy single-URL form (same key list,
same base URL).

## Tool calling

```go
weather := llm.NewTool("get_weather", "Look up current weather", map[string]any{
    "type": "object",
    "properties": map[string]any{
        "city": map[string]any{"type": "string"},
    },
    "required": []string{"city"},
})

resp, err := c.Chat(ctx, messages,
    llm.WithTools([]llm.Tool{weather}),
    llm.WithToolChoice("auto"),
)
for _, call := range resp.Choices[0].Message.ToolCalls {
    // dispatch on call.Function.Name with call.Function.Arguments (JSON string)
}
```

## Streaming

```go
s, err := c.Stream(ctx, messages, llm.WithChatMaxTokens(512))
if err != nil {
    return err
}
defer s.Close()
for {
    chunk, ok := s.Next()
    if !ok {
        break
    }
    fmt.Print(chunk.Content)
}
if err := s.Err(); err != nil {
    return err
}
log.Printf("tokens: %d in, %d out", s.Usage().PromptTokens, s.Usage().CompletionTokens)
```

`StreamExtract` does the same but populates a typed target at the end while
exposing the in-flight text.

## Middleware

Cross-cutting concerns without polluting the core call path. Compose with
`WithMiddleware`; called in registration order, outer middleware sees the
request first and the response last.

```go
logMW := func(ctx context.Context, req *llm.ChatRequest, next func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
    start := time.Now()
    resp, err := next(ctx, req)
    slog.Info("llm.chat", "model", req.Model, "dur", time.Since(start), "err", err)
    return resp, err
}

c := llm.NewClient(url, key, model, llm.WithMiddleware(logMW))
```

## Circuit breaker

Per-client breaker counting upstream failures. Trips to open on consecutive
failures, half-opens after `OpenDuration`, closes on a probe success.

```go
c := llm.NewClient(url, key, model,
    llm.WithCircuitBreaker(llm.CircuitConfig{
        FailThreshold: 5,
        OpenDuration:  30 * time.Second,
    }),
)

_, err := c.Complete(ctx, sys, usr)
if errors.Is(err, llm.ErrCircuitOpen) {
    // upstream is presumed dead — skip and serve cached/degraded
}
```

## Prompt caching

`CacheControl` lets you mark system or user messages as cacheable so providers
that support it (Anthropic; OpenAI is auto-detected) can deduplicate prompt
prefixes across calls.

```go
sys := llm.NewCachedSystemMessage(longSystemPrompt, llm.Ephemeral())
resp, err := c.Chat(ctx, []llm.Message{sys, userMsg})
fmt.Println(resp.Usage.CachedTokens)
```

## Completer interface (opt-in consumers)

Use `Completer` + `NoOp` + `NewOptional` when an LLM is *optional*. Consumers
that don't configure an API key get a no-op client that returns
`ErrUnavailable` — no nil checks at every call site.

```go
type Completer = llm.Completer // Complete(ctx, system, user, ...) (string, error)

c, ok := llm.NewOptional(baseURL, apiKey, model)
if !ok {
    c = llm.NoOp{} // every Complete returns llm.ErrUnavailable
}

reply, err := c.Complete(ctx, "system", "user")
if errors.Is(err, llm.ErrUnavailable) {
    return fallbackPath(ctx)
}
```

## Errors

| Error | Meaning |
|-------|---------|
| `*APIError` | Non-2xx upstream — includes `Status`, `Code`, `Message`, and `RetryAfter` parsed from the header |
| `ErrCircuitOpen` | Breaker is open; upstream was not called |
| `ErrUnavailable` | `NoOp` client; consumer didn't configure a real backend |

`APIError.RetryAfter` is wired into `retry`-aware callers via
`retry.RetryAfter`, so callers using `kit/retry` honour upstream
back-pressure automatically.

## Client options reference

| Option | Type | Notes |
|--------|------|-------|
| `WithMaxTokens(n)` | int | Default cap on `max_tokens` per request |
| `WithTemperature(t)` | float64 | Default `temperature` |
| `WithMaxRetries(n)` | int | Network retries on 5xx / transport errors |
| `WithFallbackKeys(keys)` | []string | Rotate API keys on 401/429 |
| `WithEndpoints(eps)` | []Endpoint | Rotate URL+Key+Model triples |
| `WithMiddleware(m)` | Middleware | Wrap every Chat call |
| `WithCircuitBreaker(cfg)` | CircuitConfig | Per-client breaker |
| `WithHTTPClient(hc)` | *http.Client | Bring your own transport (e.g. proxy, tracing) |

## Per-call (`ChatOption`) reference

| Option | Notes |
|--------|-------|
| `WithTools(tools)` | Add tool definitions |
| `WithToolChoice(choice)` | `"auto"`, `"none"`, `"required"`, or `{"type":"function","function":{"name":"X"}}` |
| `WithJSONSchema(name, schema)` | Force `response_format=json_schema` |
| `WithChatTemperature(t)` | Per-call override |
| `WithChatMaxTokens(n)` | Per-call override |
| `WithMessageTimestamps()` | Prepend ISO-8601 timestamps to each message; pair with `FormatChatTime/ParseChatTime` |
