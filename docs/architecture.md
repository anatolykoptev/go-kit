# go-kit Architecture

> **Module**: `github.com/anatolykoptev/go-kit`
>
> Shared infrastructure for 8 Go MCP servers. One module, zero bloat.

## Organization

**Monorepo** — one `go.mod`, one CI, one `go get`.

Small packages (env, retry, metrics, strutil) live in go-kit permanently.
`mcpserver` → separate repo (`go-mcpserver`) because it pulls `mcp-go` dependency.
If `llm` or `cache` grow beyond ~500 LOC or need their own release cycle,
extract to `go-llm` / `go-cache` — same pattern as go-stealth and go-imagefy.

## Package Layout

```
go-kit/
  go.mod        — single module, minimal deps
  env/          — environment variable parsing (zero deps)
  llm/          — OpenAI-compatible LLM client (net/http only)
  cache/        — L1 memory + L2 Redis tiered cache (redis dep)
  ratelimit/    — token bucket rate limiter with per-key support (zero deps)
  retry/        — generic retry with exponential backoff (zero deps)
  metrics/      — atomic operation counters (zero deps)
  strutil/      — unicode-aware string helpers (zero deps)
  docs/
    ROADMAP.md  — active roadmap and status
```

**Dependencies**: `redis/go-redis` (only for `cache/`).
Go module pruning ensures consumers that don't use `cache/` won't pull redis.

## Already Extracted (separate repos)

| Module | What | Extracted from |
|--------|------|----------------|
| go-stealth | TLS fingerprint, user-agents, RetryHTTP | go-job |
| go-imagefy | Image safety, search, validation | go-wp |
| go-enriche | Web content enrichment | go-wp |
| go-mcpserver | MCP server bootstrap (pulls mcp-go) | 7 services |

## Dependency Direction

```
go-kit (stdlib + redis), go-mcpserver (mcp-go)
  ↑
go-stealth, go-imagefy, go-enriche, go-engine (domain libs)
  ↑
go-search, go-code, go-wp, go-job, go-startup, go-hully, go-content, gigiena-teksta
```

## Migration Strategy

### Per-phase rollout

1. Implement package with tests in go-kit
2. Pick ONE consumer as pilot (go-code or go-search — best test coverage)
3. Replace internal code with go-kit import, verify tests pass
4. Roll out to remaining repos one by one
5. Delete replaced internal code

### Risk mitigation

- go-kit has NO business logic — pure infrastructure
- Each package is independent (no internal cross-imports)
- Consumers can adopt packages incrementally
- Existing `internal/` code works until migrated — no big-bang switch

## Cross-Project Deduplication (not go-kit)

Duplications better solved by importing existing domain libraries:

| Duplicate | Projects | Solution |
|-----------|----------|----------|
| `fetch_html.go` — content extraction (~180 LOC) | go-job, go-startup | Import `go-engine/fetch` |
| `DetectQueryType` — query classification (~66 LOC) | go-job, go-startup | Import `go-engine/text` |
| `promptBase`, `promptDeep` — LLM templates (~72 LOC) | go-job, go-startup | Import `go-engine/llm` |
| SearXNG client (~132 LOC) | go-code | Import `go-engine/search` |

## Out of Scope

| Feature | Why not go-kit | Where it belongs |
|---------|----------------|------------------|
| `BuildSourcesText`, `SearchResult` | Search-engine domain logic | go-engine |
| `CleanHTML` | Web scraping logic | go-enriche |
| `UserAgentChrome` | Anti-bot/stealth | go-stealth |
| TrackOperation with slog | Mixes metrics + logging | Not needed |

**Principle**: go-kit = pure infrastructure with zero business logic.
