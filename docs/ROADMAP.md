# go-kit — Shared Infrastructure for go-* Services

> **Module**: `github.com/anatolykoptev/go-kit`
>
> **Goal**: Extract duplicated infrastructure code from 8 Go MCP servers into a single reusable module.
> Estimated savings: ~3,150 LOC of copy-paste across repos.

## Organization

**Monorepo** — один `go.mod`, один CI, один `go get`.
Мелкие пакеты (env, retry, metrics, strutil, mcpboot) живут в go-kit навсегда.
Если llm или cache вырастут в сложности — выносим в отдельный репо (как go-stealth, go-imagefy).

## Consumers

| Repo | Port | Duplication hotspots |
|------|------|---------------------|
| go-search | 8890 | env, llm, cache, retry, metrics, strutil |
| go-job | 8891 | env, llm, cache, metrics, strutil |
| go-hully | 8892 | env, cache, metrics |
| go-startup | 8893 | env, llm, cache, metrics, strutil |
| go-wp | 8894 | env (envutil), llm, cache, metrics |
| go-content | 8895 | env, metrics |
| gigiena-teksta | 8896 | env, llm, cache, metrics |
| go-code | 8897 | env, llm, cache, retry, metrics |

## Already Extracted (reference)

| Module | What | From |
|--------|------|------|
| go-stealth | TLS fingerprint, user-agents, RetryHTTP | go-job |
| go-imagefy | Image safety, search, validation | go-wp |
| go-enriche | Web content enrichment (extract, structured) | go-wp |

---

## Package Layout

```
go-kit/                         github.com/anatolykoptev/go-kit
  go.mod                        — single module, minimal deps
  env/                          — environment variable parsing (zero deps)
  llm/                          — OpenAI-compatible LLM client (net/http only)
  cache/                        — L1 memory + L2 Redis tiered cache (redis dep)
  retry/                        — generic retry with exponential backoff (zero deps)
  metrics/                      — atomic operation counters (zero deps)
  strutil/                      — unicode-aware string helpers (zero deps)
  mcpboot/                      — MCP server bootstrap (mcp-go dep)
  docs/
    ROADMAP.md                  — this file
```

**Dependencies**: `redis/go-redis` (only for cache/), `mcp-go` (only for mcpboot/).
Go module pruning ensures consumers that don't use cache/ won't pull redis.

---

## Phase 1: env — Config Parsing (Priority: HIGH)

**Status**: DONE

**Problem**: Every repo has its own `env()`, `envInt()`, `envList()` etc. 8 copies, 6 different implementations.

**Best source**: go-wp `internal/envutil/` (cleanest) + go-content (has `envDuration`, `envFloat`)

**Package API**:
```go
package env

func Str(key, def string) string
func Int(key string, def int) int
func Int64(key string, def int64) int64
func Float(key string, def float64) float64
func Bool(key string, def bool) bool
func Duration(key string, def time.Duration) time.Duration
func List(key, def string) []string       // comma-separated, trimmed, no empty
func Int64List(key string) []int64         // from go-hully
```

**Files to replace**:
- `go-search/config.go` — inline `env()`, `envInt()`, `envList()`
- `go-code/cmd/go-code/config.go:104-150` — `env()`, `envInt()`, `envList()`
- `go-wp/internal/envutil/envutil.go` — entire package
- `go-job/main.go` — inline env parsing
- `go-startup/main.go` — inline env parsing
- `go-hully/config.go` — `env()`, `envList()`, `envInt64List()`
- `go-content/internal/config/config.go` — `env()`, `envInt()`, `envFloat()`, `envDuration()`
- `gigiena-teksta/main.go` — inline env parsing

**Estimated savings**: ~500 LOC

---

## Phase 2: llm — LLM Client (Priority: HIGH)

**Status**: DONE

**Problem**: 6 repos have independent LLM clients with identical `chatMessage`/`chatRequest`/`chatResponse` structs and nearly identical `callLLM` functions. go-code has the most mature version (retry + fallback keys).

**Best source**: go-code `internal/llm/` (retry, fallback keys) + go-wp (multimodal `Content any`)

**Package API**:
```go
package llm

type Client struct { ... }

func NewClient(baseURL, apiKey, model string, opts ...Option) *Client

// Options
func WithFallbackKeys(keys []string) Option
func WithHTTPClient(c *http.Client) Option
func WithMaxTokens(n int) Option
func WithTemperature(t float64) Option

func (c *Client) Complete(ctx context.Context, system, user string) (string, error)
func (c *Client) CompleteRaw(ctx context.Context, messages []Message) (string, error)

// Types
type Message struct {
    Role    string `json:"role"`
    Content any    `json:"content"` // string or []ContentPart (multimodal)
}

type ContentPart struct {
    Type     string    `json:"type"`
    Text     string    `json:"text,omitempty"`
    ImageURL *ImageURL `json:"image_url,omitempty"`
}

// Helpers
func ExtractJSON(s string) string   // strip markdown fences, find { }
```

**Files to replace**:
- `go-search/internal/engine/llm.go` (322 LOC)
- `go-code/internal/llm/llm.go` (362 LOC) — becomes thin wrapper or direct use
- `go-job/internal/engine/llm.go` (376 LOC)
- `go-startup/internal/engine/llm.go` (308 LOC)
- `go-wp/internal/engine/llm.go` (165 LOC)
- `gigiena-teksta/internal/llm/verifier.go` (182 LOC) — partial

**Estimated savings**: ~950 LOC

---

## Phase 3: cache — Tiered Cache (Priority: HIGH)

**Status**: DONE (L1 only, L2 Redis TODO)

**Problem**: 7 repos implement L1 (sync.Map + TTL) + L2 (Redis) caching. go-search/go-job/go-startup are near-identical (~300 LOC each).

**Best source**: go-search (L1+L2 architecture) + go-wp (generic `[]byte` interface) + go-code (Scan strategy)

**Package API**:
```go
package cache

type Cache struct { ... }

type Config struct {
    RedisURL   string
    RedisDB    int
    Prefix     string        // key prefix (e.g. "go-search:")
    L1MaxItems int           // max items in memory (default 1000)
    L1TTL      time.Duration // memory cache TTL (default 30m)
    L2TTL      time.Duration // Redis TTL (default 24h)
}

func New(cfg Config) *Cache

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool)
func (c *Cache) Set(ctx context.Context, key string, data []byte)
func (c *Cache) Delete(ctx context.Context, key string)
func (c *Cache) Key(parts ...string) string  // deterministic cache key from parts

type Stats struct {
    L1Hits, L1Misses int64
    L2Hits, L2Misses int64
    L1Size           int
}
func (c *Cache) Stats() Stats
func (c *Cache) Close()
```

**Files to replace**:
- `go-search/internal/engine/cache.go` (297 LOC)
- `go-code/internal/cache/cache.go` (239 LOC)
- `go-job/internal/engine/cache.go` (313 LOC)
- `go-startup/internal/engine/cache.go` (231 LOC)
- `go-wp/internal/engine/cache.go` (250 LOC)
- `go-hully/internal/storage/cache.go` (203 LOC) — partial
- `gigiena-teksta/internal/embedder/cache.go` (178 LOC) — partial

**Estimated savings**: ~800 LOC

---

## Phase 4: retry — Generic Retry (Priority: MEDIUM)

**Status**: DONE

**Problem**: go-code has a clean generic `Do[T]()` with backoff. go-search has simpler `RetryDo`. Others inline retry or use go-stealth.

**Best source**: go-code `internal/retry/retry.go`

**Package API**:
```go
package retry

type Config struct {
    MaxAttempts  int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Jitter       bool
}

var DefaultConfig = Config{MaxAttempts: 3, InitialDelay: 500*time.Millisecond, MaxDelay: 10*time.Second, Jitter: true}

func Do[T any](ctx context.Context, cfg Config, fn func() (T, error)) (T, error)
func HTTP(ctx context.Context, cfg Config, fn func() (*http.Response, error)) (*http.Response, error)
```

**Files to replace**:
- `go-code/internal/retry/retry.go` (116 LOC)
- `go-search/internal/engine/retry.go` (24 LOC)

**Estimated savings**: ~100 LOC

---

## Phase 5: metrics — Atomic Counters (Priority: MEDIUM)

**Status**: DONE

**Problem**: 7 repos use identical pattern: `atomic.Int64` counters + `Snapshot()` + `Format()`. Domain-specific counters vary, but infrastructure is copy-paste.

**Best source**: go-code `internal/metrics/`

**Package API**:
```go
package metrics

type Counter struct { ... }  // wraps atomic.Int64

func NewCounter(name string) *Counter
func (c *Counter) Add(n int64)
func (c *Counter) Value() int64

type Registry struct { ... }

func NewRegistry() *Registry
func (r *Registry) Counter(name string) *Counter
func (r *Registry) Snapshot() map[string]int64
func (r *Registry) Format() string  // human-readable summary
func (r *Registry) TrackOperation(name string, fn func() error) error
```

**Files to replace**: 7 repos, each 40-145 LOC of metrics boilerplate.

**Estimated savings**: ~300 LOC

---

## Phase 6: strutil — String Helpers (Priority: LOW)

**Status**: DONE

**Problem**: `Truncate` (byte-based, broken for Unicode) vs `TruncateStr` (rune-based). `contains()` helpers duplicated.

**Best source**: go-wp `TruncateStr` (Unicode-correct + ellipsis)

**Package API**:
```go
package strutil

func Truncate(s string, maxRunes int) string          // rune-aware + "..."
func Contains(haystack []string, needle string) bool
func ContainsAny(s string, substrs []string) bool
func Slugify(s string) string                          // safe URL slugs
```

**Estimated savings**: ~100 LOC

---

## Phase 7: mcpboot — MCP Server Bootstrap (Priority: LOW)

**Status**: Not started

**Problem**: Every repo has similar `main()` with MCP server setup, stdio/HTTP transport, recovery middleware, graceful shutdown. ~30% overlap.

**Best source**: go-code `cmd/go-code/main.go` (cleanest separation)

**Package API**:
```go
package mcpboot

type ServerConfig struct {
    Name    string
    Version string
    Port    string
}

func Run(cfg ServerConfig, setup func(server *mcp.Server)) error
// Handles: stdio detection, HTTP/SSE transport, recovery middleware,
// signal handling, graceful shutdown, slog configuration.
```

**Estimated savings**: ~400 LOC (but requires careful per-repo adaptation)

---

## Migration Strategy

### Per-phase rollout:
1. Implement package with tests in go-kit
2. Pick ONE consumer repo as pilot (go-code or go-search — best test coverage)
3. Replace internal code with go-kit import, verify tests pass
4. Roll out to remaining repos one by one
5. Delete replaced internal code

### Dependency direction:
```
go-kit (stdlib + redis + mcp-go)
  ↑
go-stealth, go-imagefy, go-enriche (domain libs)
  ↑
go-search, go-code, go-wp, go-job, go-startup, go-hully, go-content, gigiena-teksta (services)
```

### Risk mitigation:
- go-kit has NO business logic — pure infrastructure
- Each package is independent (no internal cross-imports)
- Consumers can adopt packages incrementally (env first, then llm, etc.)
- Existing `internal/` code works until migrated — no big-bang switch

### Graduation to own repo:
If `llm/` or `cache/` grow beyond ~500 LOC or need their own release cycle,
extract to `github.com/anatolykoptev/go-llm` or `go-cache` — same pattern
as go-stealth and go-imagefy. go-kit keeps thin re-export wrappers for
backward compat during transition.

---

## Summary

| Phase | Package | Priority | Savings | Repos affected |
|-------|---------|----------|---------|----------------|
| 1 | env | HIGH | ~500 LOC | 8 |
| 2 | llm | HIGH | ~950 LOC | 6 |
| 3 | cache | HIGH | ~800 LOC | 7 |
| 4 | retry | MEDIUM | ~100 LOC | 2+ |
| 5 | metrics | MEDIUM | ~300 LOC | 7 |
| 6 | strutil | LOW | ~100 LOC | 4+ |
| 7 | mcpboot | LOW | ~400 LOC | 8 |
| **Total** | | | **~3,150 LOC** | **8 repos** |

Special case: **go-startup + go-job** share 7+ identical engine files. After phases 2-3 (llm + cache), their `internal/engine/` overlap drops from ~1,300 LOC to ~400 LOC.

---

# Competitive Improvements (2026)

> **Goal**: Make each go-kit package best-in-class, not just "our copy-paste extracted".
> Based on competitive analysis of 25+ libraries across Go, Rust, Python, TypeScript, Java.

## Priority Overview

| # | Package | Effort | Impact | Key insight |
|---|---------|--------|--------|-------------|
| 1 | **llm** | HIGH | CRITICAL | Missing streaming/tools/structured output = unusable for agentic AI |
| 2 | **cache** | HIGH | HIGH | FIFO eviction → S3-FIFO = +10-30% hit rate; sync.Map → sharded = +2-5x throughput |
| 3 | **env** | LOW | HIGH | Error variants + Required + Duration format — max ROI |
| 4 | **retry** | LOW | MEDIUM | RetryAfter + MaxElapsedTime + Jitter = production-grade |
| 5 | **metrics** | MEDIUM | MEDIUM | Gauge + Timer + Labels = observability toolkit |
| 6 | **strutil** | LOW | LOW | TruncateMiddle + case conversions — nice to have |

---

## Improvement 1: llm — From Utility to Competitive Client

**Competitors**: sashabaranov/go-openai (10.5K stars), openai/openai-go (3K, official), tmc/langchaingo (8.7K)

**Our unique edge**: API key cycling/fallback — no competitor does this.

### Phase A: Table Stakes (must-have for 2026)

**Status**: Not started

#### A1. SSE Streaming

```go
stream, err := client.Stream(ctx, messages, opts...)
for chunk := range stream.Chunks() {
    fmt.Print(chunk.Delta)
}
if err := stream.Err(); err != nil { ... }
usage := stream.Usage()
```

- Parse `data: ` lines from `text/event-stream` via `bufio.Scanner`
- Handle `[DONE]` signal
- Zero new deps (stdlib `bufio` + `net/http`)
- Reference: openai/openai-go `ssestream.Stream[T]`

#### A2. Tool/Function Calling

```go
tools := []llm.Tool{{
    Name:        "get_weather",
    Description: "Get weather for a location",
    Parameters:  llm.SchemaFrom(WeatherParams{}),
}}
resp, err := client.Chat(ctx, messages, llm.WithTools(tools))
for _, call := range resp.ToolCalls {
    // call.Name, call.Arguments (json.RawMessage)
}
```

- Add `tools`, `tool_choice` to request body
- Parse `tool_calls` array from response
- Support `tool` role messages for sending results back
- Reference: sashabaranov/go-openai tool calling

#### A3. Structured Output / JSON Schema

```go
type Recipe struct {
    Name        string   `json:"name" jsonschema:"description=Recipe name"`
    Ingredients []string `json:"ingredients"`
    Steps       []string `json:"steps"`
}
var recipe Recipe
err := client.ChatTyped(ctx, messages, &recipe)
```

- `reflect` → JSON Schema generation from Go struct tags
- Set `response_format={type:"json_schema", json_schema:...}`
- Auto-unmarshal response into target struct
- Reference: sashabaranov/go-openai `jsonschema` package

#### A4. Token Usage Reporting

```go
resp, err := client.Chat(ctx, messages)
fmt.Printf("Tokens: %d prompt, %d completion, %d cached\n",
    resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens)
```

- Parse `usage` object from response JSON
- Include `prompt_tokens_details.cached_tokens` for prompt caching

### Phase B: Differentiators

**Status**: Not started

#### B1. Instructor-Style Typed Extraction with Retry

```go
var result MyStruct
err := client.Extract(ctx, prompt, &result,
    llm.WithMaxRetries(3),
    llm.WithValidator(validateFunc),
)
```

- JSON Schema → structured output → validate → retry with error feedback if invalid
- **No Go library does this well** (Python Instructor is the gold standard)
- Killer feature that justifies the library's existence

#### B2. Model-Level Fallback Chains

```go
client := llm.NewClient(
    llm.WithEndpoints(
        llm.Endpoint{URL: geminiURL, Key: key1, Model: "gemini-2.5-flash"},
        llm.Endpoint{URL: openaiURL, Key: key2, Model: "gpt-4o"},
        llm.Endpoint{URL: localURL,  Key: "",   Model: "llama3"},
    ),
)
```

- Extend existing key cycling to full endpoint+model fallback
- Each endpoint can have its own base URL, key, model
- Natural evolution of our existing `WithFallbackKeys`

#### B3. Request/Response Middleware

```go
client := llm.NewClient(baseURL, key, model,
    llm.WithMiddleware(loggingMiddleware),
    llm.WithMiddleware(cachingMiddleware),
)
```

- `type Middleware func(req *Request, next func(*Request) (*Response, error)) (*Response, error)`
- Enables: logging, metrics, caching, rate limiting without polluting core code

### Phase C: Nice-to-Have

- Cost estimation (hardcoded token pricing per model)
- OpenTelemetry spans (optional, via interface injection)
- Streaming typed extraction (partial JSON → progressive unmarshal)

---

## Improvement 2: cache — From FIFO to Best-in-Class

**Competitors**: maypok86/otter (5.8K), dgraph-io/ristretto (5.6K), eko/gocache (2.6K), jellydator/ttlcache (1.1K), samber/hot (243)

### Phase A: Core Performance (highest impact)

**Status**: Not started

#### A1. S3-FIFO Eviction Algorithm

Replace FIFO (oldest-first) with S3-FIFO (Small + Main + Ghost queues).

- 3 FIFO queues — no complex data structures needed
- +10-30% hit rate over plain FIFO on real workloads
- Simpler than W-TinyLFU (no Count-Min Sketch for basic version)
- Near-zero per-operation overhead
- Reference: samber/hot, SOSP '23 paper "FIFO queues are all you need"
- Benchmark data: twitter_cluster52 — S3-FIFO 76.66% vs FIFO ~60%

#### A2. Sharded Map (replace sync.Map)

```go
type shard struct {
    mu    sync.RWMutex
    items map[string]*entry
}
type shardedMap [64]shard  // 64 shards
```

- `sync.Map` is optimized for read-heavy, few-writes — we have mixed workloads
- Sharded map with 64 shards: +2-5x throughput under contention
- Reference: dgraph-io/ristretto (256-shard `shardedMap`)

#### A3. Fast Key Hashing (replace SHA-256)

Replace `crypto/sha256` with `hash/maphash` (stdlib, zero deps) or `hash/fnv`.

- SHA-256 is cryptographic — 10-50x slower than needed for cache keys
- We need fast distribution, not collision resistance
- `hash/maphash` is stdlib, no external deps needed
- Reference: allegro/bigcache uses FNV-1a, ristretto uses xxhash

### Phase B: Modern API

**Status**: Not started

#### B1. Generics

```go
type Cache[K comparable, V any] struct { ... }
func New[K comparable, V any](cfg Config) *Cache[K, V]
func (c *Cache[K, V]) Get(key K) (V, bool)
func (c *Cache[K, V]) Set(key K, value V)
```

- Every modern competitor uses generics
- Eliminates `[]byte` serialization for in-memory use cases
- Keep a `ByteCache = Cache[string, []byte]` alias for backward compat

#### B2. GetOrLoad with Singleflight

```go
func (c *Cache[K, V]) GetOrLoad(key K, loader func(K) (V, error)) (V, error)
```

- Prevents thundering herd — 100 goroutines same key = 1 backend call
- Single dep: `golang.org/x/sync/singleflight`
- Reference: otter, ttlcache, gocache all have this

#### B3. TTL Jitter

```go
func New[K, V](cfg Config) *Cache[K, V]  // cfg.JitterPercent float64 (default 0.1)
```

- Many entries same TTL → all expire at once → cache stampede
- Add 10% random jitter to each entry's TTL
- Trivial implementation, prevents coordinated expiration spikes
- Reference: samber/hot

#### B4. Statistics

```go
type Stats struct {
    Hits, Misses  int64
    HitRatio      float64
    Evictions     int64
    CurrentSize   int64
}
```

- Track: hits, misses, hit ratio, evictions (by reason), current size
- Use `atomic.Int64` counters (lock-free)
- Reference: ristretto (comprehensive), otter

### Phase C: L2 Redis Integration

**Status**: Not started (planned from initial roadmap)

#### C1. Store Interface

```go
type Store[K comparable, V any] interface {
    Get(ctx context.Context, key K) (V, bool, error)
    Set(ctx context.Context, key K, value V, ttl time.Duration) error
    Delete(ctx context.Context, key K) error
}
```

- L1 (memory) and L2 (Redis) both implement `Store`
- Reference: eko/gocache `StoreInterface`

#### C2. ChainCache with Read-Through Promotion

- L1 miss + L2 hit → asynchronously populate L1
- Write-through: Set writes to both L1 and L2
- Reference: eko/gocache `ChainCache[T]`

#### C3. Serialization

- `encoding/json` for zero-dep core
- Optional `msgpack` for performance (what gocache uses)

---

## Improvement 3: env — Close the Gaps

**Competitors**: caarlos0/env (6K), kelseyhightower/envconfig (5.4K), sethvargo/go-envconfig (1.2K)

**Our edge**: simplest API (functions, not struct tags), zero reflection, fastest compile.

### Phase A: Error Handling (P0)

**Status**: DONE (8 commits: NotSetError, ParseError, Lookup, Exists, Required, *E variants, Must* variants, Duration Go format)

```go
// Error-returning variants — biggest gap vs ALL competitors
func IntE(key string, def int) (int, error)       // error on parse failure
func MustInt(key string, def int) int              // panic on parse failure

// Required — no default, must be set
func Required(key string) (string, error)          // error if not set
func MustStr(key string) string                    // panic if not set

// Distinguish "not set" vs "set to empty"
func Lookup(key string) (string, bool)
func Exists(key string) bool
```

- Every competitor returns errors; we silently swallow them
- `*E` variants return errors, `Must*` panic — keep originals for backward compat
- Inspired by: all 5 competitors + Rust's `Result<T, E>` pattern

### Phase B: Type Completeness (P1)

**Status**: DONE (Uint, Uint64, Map, URL with *E and Must* variants)

```go
func Duration(key string, def time.Duration) time.Duration  // now accepts "5s", "100ms", "2m30s"
func Uint(key string, def uint) uint
func Uint64(key string, def uint64) uint64
func Map(key, def string) map[string]string                 // "k1:v1,k2:v2"
func URL(key string, def string) *url.URL
```

- `Duration`: add `time.ParseDuration` with fallback to float-seconds for compat
- `Map`: common for headers, labels — caarlos0, sethvargo, kelseyhightower support it
- `URL`: first-class type in caarlos0 and sethvargo

### Phase C: Power Features (P2)

**Status**: Not started

```go
// Testability — decouple from os.Getenv
type Source interface { Lookup(key string) (string, bool) }
var DefaultSource Source = OSSource{}
func MapSource(m map[string]string) Source  // for tests

// Docker secrets / Kubernetes
func File(key, def string) string           // read file path from env, return contents

// Variable expansion
func Expand(key, def string) string         // resolve ${OTHER_VAR} references

// Binary data
func Base64(key string, def []byte) []byte  // base64-decode
func Hex(key string, def []byte) []byte     // hex-decode
```

- `Source` interface: inspired by sethvargo's `Lookuper` — enables parallel-safe tests
- `File`: Docker secrets pattern (`/run/secrets/db_password`) — caarlos0 `file` tag
- `Expand`: `os.Expand` based — caarlos0, sethvargo support this

### Phase D: Optional Struct Parsing (P3)

Consider a separate `env/config` sub-package with reflection-based `Parse`:

```go
type Config struct {
    Port    int           `env:"PORT" default:"8080"`
    Debug   bool          `env:"DEBUG"`
    Timeout time.Duration `env:"TIMEOUT" default:"30s"`
    DBUrl   string        `env:"DATABASE_URL" required:"true"`
}
err := config.Parse(&cfg)
```

- Keeps core `env/` zero-reflection
- Competes directly with caarlos0/env but within our ecosystem
- Decision: implement only if struct-based parsing is needed by 3+ consumers

---

## Improvement 4: retry — Production Grade

**Competitors**: cenkalti/backoff (3.9K), avast/retry-go (2.9K), failsafe-go (2K), sethvargo/go-retry (708)

### Planned Additions

**Status**: Not started

| # | Feature | Reference | Effort |
|---|---------|-----------|--------|
| 1 | **RetryAfter error** — let fn return `RetryAfter(duration)` to override backoff; auto-parse `Retry-After` header in HTTP retry | cenkalti, failsafe-go | Low |
| 2 | **MaxElapsedTime** — total wall-clock budget across all attempts (not just max attempts) | cenkalti | Low |
| 3 | **Jitter** — `WithJitter(bool)` adds random ±25% to delay; `FullJitter` strategy where delay is `[0, calculated_delay]` | avast/retry-go v5 | Low |
| 4 | **Timer interface** — `type Timer interface { After(d) <-chan time.Time }` for deterministic tests without real sleeps | avast, cenkalti | Low |
| 5 | **Per-error-type limits** — `AttemptsForError(0, context.DeadlineExceeded)` to never retry specific errors | avast/retry-go v5 | Low |
| 6 | **RetryableError marker** — invert default: only explicitly marked errors trigger retry (safer for production) | sethvargo/go-retry | Low |

### Not Planned (too heavy for our scope)

- Circuit breaker, bulkhead, hedge policy → use failsafe-go directly
- Policy composition framework → overkill for a utility library

---

## Improvement 5: metrics — Observability Toolkit

**Competitors**: daniel-nichter/go-metrics (zero-dep), hashicorp/go-metrics (enterprise), VictoriaMetrics/metrics

### Planned Additions

**Status**: Not started

| # | Feature | Reference | Effort |
|---|---------|-----------|--------|
| 1 | **Atomic snapshot-and-reset** — `Snapshot(reset bool)` reads + zeroes atomically, no lost counts between read and reset | daniel-nichter/go-metrics | Low |
| 2 | **Gauge type** — track current values (queue depth, latency), optional min/max/mean over window | hashicorp, daniel-nichter | Medium |
| 3 | **Timer/MeasureSince** — `defer reg.Timer("api.latency").Stop()` — one-liner for duration tracking | hashicorp `MeasureSince` | Low |
| 4 | **Labels/tags** — `reg.Counter("requests", "method", "GET").Inc()` dimensional metrics | All serious systems | Medium |
| 5 | **Sink interface** — `type Sink interface { WriteMetrics(w io.Writer) }` for JSON/text/Prometheus output | hashicorp | Low |
| 6 | **Rate tracking (EWMA)** — events/sec with 1min, 5min, 15min exponentially weighted moving averages | Dropwizard Metrics (Java) | Medium |
| 7 | **Reservoir sampling** — fixed 2K sample for accurate P50/P95/P99 without unbounded memory | daniel-nichter Algorithm R | Medium |
| 8 | **TTL for dynamic metrics** — auto-expire stale counters created per-endpoint/per-user | mattrobenolt/go-metrics | Low |

### Not Planned

- Prometheus client_golang replacement → different scope
- Push to remote (VictoriaMetrics, StatsD) → consumers can wrap via Sink interface

---

## Improvement 6: strutil — Unicode-Correct Toolkit

**Competitors**: huandu/xstrings, aquilax/truncate, Rust unicode-truncate

### Planned Additions

**Status**: Not started

| # | Feature | Reference | Effort |
|---|---------|-----------|--------|
| 1 | **TruncateMiddle** — `"path/to/ver...file.go"` — keeps start + end, cuts middle | aquilax/truncate `PositionMiddle` | Low |
| 2 | **Configurable placeholder** — `WithPlaceholder("[...]")` instead of hardcoded "..." | aquilax, Ruby `truncate` | Low |
| 3 | **Case conversions** — `ToSnakeCase`, `ToCamelCase`, `ToKebabCase`, `ToPascalCase` | huandu/xstrings | Low |
| 4 | **WordWrap(s, width)** — wrap text at word boundaries | Python `textwrap.fill()` | Low |
| 5 | **Scrub(s)** — remove/replace invalid UTF-8 sequences | huandu/xstrings | Trivial |
| 6 | **ContainsAll** — complement to existing `ContainsAny` | Common pattern | Trivial |

### Considered but Deferred

- Display-width truncation (CJK = 2 columns) → needs `go-runewidth` dep or UAX #11 table
- Grapheme cluster boundaries → needs `rivo/uniseg` dep
- Decision: add these only if CJK/emoji truncation bugs appear in production

---

## Implementation Order

```
Quarter 1 (immediate):
  env Phase A (error variants)      — 1 day, backwards-compatible
  env Phase B (Duration fix + types) — 1 day
  retry additions (1-4)             — 1 day
  strutil additions (1-3)           — 1 day
  cache A3 (fast key hashing)       — 0.5 day

Quarter 2:
  llm Phase A (streaming + tools + structured output) — 3-5 days, biggest effort
  cache Phase A (S3-FIFO + sharded map)              — 2-3 days
  cache Phase B (generics + singleflight + stats)    — 1-2 days
  metrics additions (1-5)                            — 2 days

Quarter 3:
  llm Phase B (instructor-style extraction, model fallback) — 2-3 days
  cache Phase C (L2 Redis via Store interface)              — 2-3 days
  env Phase C (Source interface, File, Expand)              — 1 day
  metrics additions (6-8)                                   — 2 days

Ongoing:
  llm Phase C, env Phase D, strutil deferred items — as needed
```

## Competitive Reference Table

| Package | Our Current | Best Competitor | Gap Level | After Improvements |
|---------|-------------|-----------------|-----------|-------------------|
| env | Function getters, silent defaults | caarlos0/env (6K stars) | Medium | On par (different niche: functions vs struct tags) |
| llm | Text completion + retry + key cycling | openai/openai-go (3K, official) | **Critical** | Unique: Instructor-style extraction + key cycling |
| cache | sync.Map + FIFO + TTL | maypok86/otter (5.8K) | **High** | Competitive: S3-FIFO + generics + singleflight |
| retry | Generic Do[T] + HTTP | cenkalti/backoff (3.9K) | Low | On par with better jitter + RetryAfter |
| metrics | Atomic counters only | hashicorp/go-metrics | Medium | Mini hashicorp: gauges + timers + labels + sinks |
| strutil | Truncate + Contains | huandu/xstrings | Low | Adequate for our needs |
