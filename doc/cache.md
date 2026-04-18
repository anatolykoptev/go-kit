# cache

L1 memory + optional L2 Redis tiered cache with S3-FIFO eviction, tag invalidation,
byte-bounded eviction, and time-to-idle support.

```
go get github.com/anatolykoptev/go-kit/cache
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/cache"

// L1-only (no Redis at runtime)
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
})
defer c.Close()

c.Set(ctx, "key", data)
data, ok, err := c.Get(ctx, "key")

// Cache-aside with singleflight — concurrent callers deduplicated
data, err := c.GetOrLoad(ctx, "key", func(ctx context.Context) ([]byte, error) {
    return fetchFromDB(ctx, "key")
})
```

## L1 + L2 Redis

```go
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
    RedisURL:   "redis://localhost:6379",
    RedisDB:    0,
    Prefix:     "myapp:",
    L2TTL:      24 * time.Hour,
})
```

L1 miss → L2 hit → automatic L1 promotion. Set/Delete propagate to both tiers.
L2 is optional; on Redis failure the cache degrades to L1-only (no error returned).

## Weigher + MaxWeight (byte-bounded eviction)

Prevents memory bloat when entry sizes vary widely (e.g. parsed ASTs, HTTP responses).

```go
c := cache.New(cache.Config{
    L1MaxItems: 5000,          // count bound (upper guard)
    L1TTL:      24 * time.Hour,
    MaxWeight:  256 << 20,     // 256 MB hard cap
    Weigher: func(key string, data []byte) int64 {
        return int64(len(data))
    },
})
```

- `Weigher` receives the raw `[]byte` stored in L1. Return approximate bytes.
- `MaxWeight` triggers an extra eviction pass (S3-FIFO order) after each `Set`
  until `totalWeight <= MaxWeight`.
- Both bounds apply simultaneously when set: evict if count > `L1MaxItems` OR
  weight > `MaxWeight`, whichever triggers first.
- `Weigher == nil` (default) disables all weight tracking — zero overhead, exact
  existing behavior.
- Current total weight is available as `Stats().TotalWeight`.

```go
stats := c.Stats()
log.Printf("L1 size: %d entries, weight: %d bytes", stats.L1Size, stats.TotalWeight)
```

## IdleTTL (time-to-idle)

Evicts entries that have not been accessed for a given duration. Useful for dropping
cold entries that are still within their absolute TTL.

```go
c := cache.New(cache.Config{
    L1MaxItems: 2000,
    L1TTL:      24 * time.Hour,  // absolute expiry
    IdleTTL:    2 * time.Hour,   // evict if untouched for 2h
})
```

- `Get` checks idle age **before** refreshing `lastAccess`. A stale entry is
  removed on first access attempt (returned as miss).
- A background housekeeper sweeps at `IdleTTL / 4` intervals (min 1 s) to catch
  entries never accessed again after `Set`.
- `IdleTTL == 0` (default) disables everything: no goroutine is spawned, no
  `lastAccess` field is updated.

## Per-key TTL

```go
c.SetWithTTL(ctx, "jobs:123", data, 15*time.Minute)

data, err := c.GetOrLoadWithTTL(ctx, "company:456", 24*time.Hour,
    func(ctx context.Context) ([]byte, error) {
        return fetchCompanyData(ctx, "456")
    },
)
```

## Tag-based invalidation

```go
c.SetWithTags(ctx, "user:1:profile", data, []string{"user:1", "profile"})
c.SetWithTags(ctx, "user:1:settings", data, []string{"user:1"})

n := c.InvalidateByTag(ctx, "user:1") // removes both, returns 2
tags := c.Tags("user:1:profile")      // []string{"user:1", "profile"}
```

## Typed JSON helpers

```go
cache.SetJSON(c, ctx, "user:1", User{Name: "Alice"})

user, ok, err := cache.GetJSON[User](c, ctx, "user:1")

user, err := cache.GetOrLoadJSON[User](c, ctx, "user:1", func(ctx context.Context) (User, error) {
    return fetchUser(ctx, 1)
})
```

## OnEvict callback

```go
c := cache.New(cache.Config{
    L1MaxItems: 1000,
    L1TTL:      30 * time.Minute,
    OnEvict: func(key string, data []byte, reason cache.EvictReason) {
        switch reason {
        case cache.EvictCapacity: metrics.Incr("cache.evict.capacity")
        case cache.EvictExpired:  metrics.Incr("cache.evict.expired")
        case cache.EvictExplicit: metrics.Incr("cache.evict.explicit")
        }
    },
})
```

## Config reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `L1MaxItems` | `int` | `1000` | Max entries in L1 (count-based eviction) |
| `L1TTL` | `Duration` | `30m` | Absolute TTL for L1 entries |
| `L2TTL` | `Duration` | `24h` | TTL for Redis entries |
| `RedisURL` | `string` | `""` | Redis URL; empty = L1-only |
| `RedisDB` | `int` | `0` | Redis database index |
| `Prefix` | `string` | `""` | Key prefix for Redis |
| `JitterPercent` | `float64` | `0` | ±N% TTL jitter to prevent stampedes (e.g. `0.1`) |
| `MaxWeight` | `int64` | `0` | Max total weight in bytes; 0 = disabled |
| `Weigher` | `func(string, []byte) int64` | `nil` | Weight function; nil = no weight tracking |
| `IdleTTL` | `Duration` | `0` | Time-to-idle; 0 = disabled |
| `OnEvict` | `func(string, []byte, EvictReason)` | `nil` | Eviction hook (called outside the lock) |
| `L2` | `L2` | `nil` | Custom L2 backend (overrides RedisURL) |

## Migration cheatsheet

| Old pattern | kitcache equivalent |
|-------------|---------------------|
| Fixed-size `map` + manual mutex | `cache.New(cache.Config{L1MaxItems: N, L1TTL: ttl})` |
| `sync.Map` with no expiry | Add `L1TTL` for automatic cleanup |
| Hand-rolled LRU (count-bound) | `Config{L1MaxItems: N, L1TTL: ttl}` — S3-FIFO beats LRU hit rate |
| Hand-rolled LRU with variable-size values | `Config{MaxWeight: cap, Weigher: sizeFunc}` for byte-bounded eviction |
| Redis cache without local tier | Add `L1MaxItems` + `L1TTL` for read-through L1 |
| Manual TTL refresh on access | `Config{IdleTTL: d}` — evict entries cold for longer than `d` |
