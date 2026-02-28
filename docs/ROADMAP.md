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

**Status**: Not started

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

**Status**: Not started

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

**Status**: Not started

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

**Status**: Not started

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

**Status**: Not started

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

**Status**: Not started

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
