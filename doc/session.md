# session

Conversation memory for chat agents: ordered message history, summary text,
extracted facts, pluggable stores (in-memory / file / Redis), an LLM-driven
`Compactor` for old messages, and intent helpers.

```
go get github.com/anatolykoptev/go-kit/session
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/session"

store := session.NewInMemoryStore(session.Options{
    MaxMessages:    200,
    MaxContentSize: 16 * 1024,
    MaxFacts:       50,
    TTL:            24 * time.Hour,
})

key := "chat:" + chatID
store.AddMessage(key, session.Message{Role: "user", Content: text})

history := store.GetHistory(key)
```

`session.Store` is the interface every backend implements (`GetOrCreate`,
`AddMessage`, `GetHistory`, `GetSummary`, `SetSummary`, `GetFacts`, `AddFacts`,
`MessageCount`, `CompactMessages`, `TruncateHistory`, `Clear`, `Delete`,
`Save`, `ListStale`). Anything taking a `Store` can be tested against the
in-memory implementation without spinning up Redis.

## Stores

| Constructor | Backend | Notes |
|-------------|---------|-------|
| `NewInMemoryStore(Options)` | in-process | per-key locking; loses everything on restart |
| `NewFileStore(dir, Options)` | JSON files in `dir/<key>.json` | `Save(key)` persists; otherwise behaves as in-memory |
| `session/redis.New(client, redis.Options)` | Redis | scalable + multi-process safe |

`Options` is shared across all three:

| Field | Default | Description |
|-------|---------|-------------|
| `TTL` | `0` (no expiry) | `ListStale` reports keys older than this |
| `MaxMessages` | `0` (unlimited) | Trim oldest messages once exceeded |
| `MaxContentSize` | `0` (unlimited) | Truncate per-message `Content` byte length |
| `MaxFacts` | `0` (unlimited) | Rotate oldest facts when exceeded |

## Messages and facts

```go
type Message struct {
    Role       string       // "user" | "assistant" | "system" | "tool"
    Content    string
    ToolCalls  []ToolCall   // assistant tool invocations
    // …other fields for tool-call results
}

type Fact struct {
    Content     string
    ExtractedAt time.Time
}
```

Facts are short, durable statements distilled from a conversation (user
preferences, declared constraints) that survive compaction. Pair with
[`llm`](llm.md)'s `Extract[T]` to populate them from the assistant output.

## Compactor — keep history within an LLM's context window

Once a conversation crosses a message threshold, summarise the oldest portion
into a compact text blob and keep only the last N raw messages.

```go
c := &session.Compactor{
    Store:        store,
    Summarize:    func(ctx context.Context, prompt string) (string, error) {
        return llmClient.Complete(ctx, "Summarise concisely.", prompt)
    },
    Threshold:    100,  // run when MessageCount >= 100
    KeepLast:     30,   // keep last 30 raw messages
    ExtractFacts: true, // parse "- " bullets in the summary into Fact entries
    MultiPart:    true, // split large histories before summarising
}

c.Compact(ctx, key)
```

`Compactor` calls `Store.CompactMessages(key, KeepLast)` to peel off the old
messages, runs `Summarize`, merges the new summary with any existing one via
`Store.SetSummary`, optionally appends extracted facts, and then
`TruncateHistory(key, KeepLast)`. The whole sequence is atomic at the store
level for the in-memory and Redis backends.

`MultiPartMin` (default 10) is the minimum message count to bother splitting
before summarising. `MaxTokensGuard` skips messages whose
`len(Content)/4 > MaxTokensGuard` (cheap byte-based token estimate) so
runaway pastes don't blow the LLM call.

## Cleanup — archive stale sessions

```go
archived := session.Cleanup(store, 7*24*time.Hour, func(s *session.Session) error {
    return writeToBlob(s)
})
log.Printf("archived %d stale sessions", archived)
```

`Cleanup` calls `store.ListStale(maxAge)`, invokes `archiveFn` for each, and
`store.Delete`s those that archive successfully. Returns the count
successfully archived. Run on a ticker (e.g. nightly) — not in request path.

## Intent helpers

`NeedsMemoryContext(text)` is a cheap heuristic that reports whether an input
references prior conversation ("as I said earlier", "you mentioned…",
Russian equivalents). Use as a fast pre-filter so you only inject summary +
facts into the LLM prompt when they're actually relevant.

```go
if session.NeedsMemoryContext(userMsg) {
    sys += "\n\n[Summary]\n" + store.GetSummary(key)
    for _, f := range store.GetFacts(key) {
        sys += "\n- " + f.Content
    }
}
```

## Streaming assistant responses

`UpdateLastMessage(key, content)` overwrites the last message's content —
exactly the shape needed for SSE streaming where each chunk extends the
buffered assistant reply.

```go
store.AddMessage(key, session.Message{Role: "assistant"})
for chunk := range chunks {
    buf.WriteString(chunk.Content)
    store.UpdateLastMessage(key, buf.String())
}
```

## API reference

| Symbol | Notes |
|--------|-------|
| `Store` (interface) | All backends implement this; pass it into agent code |
| `Options` | Bounds and TTL shared across stores |
| `Message`, `Fact`, `Session`, `ToolCall`, `FunctionCall` | DTOs |
| `Compactor` | Bundles a `Store`, summariser, and thresholds |
| `Cleanup(store, maxAge, archiveFn) int` | Archive-and-delete stale sessions |
| `NeedsMemoryContext(text) bool` | Cheap prior-context-reference heuristic |

## Notes

- `Save` is a no-op for `InMemoryStore`; it persists for `FileStore` and is
  unused by Redis (writes are immediate).
- The in-memory store uses per-key locking, so unrelated sessions don't
  contend on a global mutex under load.
- For multi-process services (multiple replicas) use the Redis backend —
  in-memory and file stores assume single-process ownership of the key.
