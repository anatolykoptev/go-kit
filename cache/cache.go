// Package cache provides a tiered L1 (memory) + optional L2 (Redis) cache.
// L1 uses sync.Map with TTL and size-based eviction.
// If RedisURL is empty, operates as L1-only (no external dependencies needed at runtime).
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config configures the cache.
type Config struct {
	// RedisURL is the Redis connection URL. Empty means L1-only mode.
	RedisURL string

	// RedisDB selects the Redis database number (default 0).
	RedisDB int

	// Prefix is prepended to all Redis keys (e.g. "gs:" for go-search).
	Prefix string

	// L1MaxItems is the max number of items in memory (default 1000).
	L1MaxItems int

	// L1TTL is the TTL for L1 cache entries (default 30m).
	L1TTL time.Duration

	// L2TTL is the TTL for L2 Redis entries (default 24h). Ignored if no Redis.
	L2TTL time.Duration
}

func (c *Config) applyDefaults() {
	if c.L1MaxItems <= 0 {
		c.L1MaxItems = 1000
	}
	if c.L1TTL <= 0 {
		c.L1TTL = 30 * time.Minute
	}
	if c.L2TTL <= 0 {
		c.L2TTL = 24 * time.Hour
	}
}

// Cache is a tiered L1 (memory) + optional L2 (Redis) cache.
type Cache struct {
	cfg Config

	// L1: in-memory cache.
	l1    sync.Map
	l1Len atomic.Int64

	// Stats.
	l1Hits   atomic.Int64
	l1Misses atomic.Int64

	// Shutdown.
	done chan struct{}
}

type l1Entry struct {
	data      []byte
	expiresAt time.Time
}

// New creates a new Cache. If cfg.RedisURL is empty, L2 is disabled.
// Call Close() when done to stop the background cleanup goroutine.
func New(cfg Config) *Cache {
	cfg.applyDefaults()

	c := &Cache{
		cfg:  cfg,
		done: make(chan struct{}),
	}

	// Background cleanup every 1/10 of TTL, minimum 10s.
	interval := cfg.L1TTL / 10
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	go c.cleanupLoop(interval)

	return c
}

// Get retrieves a value from L1 (then L2 if miss). Returns nil, false on miss.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	// L1 lookup.
	if v, ok := c.l1.Load(key); ok {
		entry := v.(*l1Entry) //nolint:forcetypeassert // invariant
		if time.Now().Before(entry.expiresAt) {
			c.l1Hits.Add(1)
			return entry.data, true
		}
		// Expired — remove.
		c.l1.Delete(key)
		c.l1Len.Add(-1)
	}

	c.l1Misses.Add(1)

	// TODO: L2 Redis lookup (Phase 2 — add redis/go-redis dependency).
	_ = ctx

	return nil, false
}

// Set stores a value in L1 (and L2 if configured).
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	c.evictIfNeeded()

	c.l1.Store(key, &l1Entry{
		data:      data,
		expiresAt: time.Now().Add(c.cfg.L1TTL),
	})
	c.l1Len.Add(1)

	// TODO: L2 Redis set (Phase 2).
	_ = ctx
}

// Delete removes a key from both L1 and L2.
func (c *Cache) Delete(ctx context.Context, key string) {
	if _, loaded := c.l1.LoadAndDelete(key); loaded {
		c.l1Len.Add(-1)
	}
	// TODO: L2 Redis delete (Phase 2).
	_ = ctx
}

// Stats holds cache statistics.
type Stats struct {
	L1Hits   int64
	L1Misses int64
	L1Size   int
}

// Stats returns a snapshot of cache statistics.
func (c *Cache) Stats() Stats {
	size := 0
	c.l1.Range(func(_, _ any) bool {
		size++
		return true
	})
	return Stats{
		L1Hits:   c.l1Hits.Load(),
		L1Misses: c.l1Misses.Load(),
		L1Size:   size,
	}
}

// Close stops the background cleanup goroutine.
func (c *Cache) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

// Key builds a deterministic cache key from parts using SHA-256.
func Key(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}

// evictIfNeeded removes entries when L1 exceeds max size.
// First pass: remove expired. Second pass: remove oldest if still over limit.
func (c *Cache) evictIfNeeded() {
	if c.l1Len.Load() < int64(c.cfg.L1MaxItems) {
		return
	}

	now := time.Now()
	removed := int64(0)

	// Pass 1: remove expired entries.
	c.l1.Range(func(k, v any) bool {
		if now.After(v.(*l1Entry).expiresAt) { //nolint:forcetypeassert // invariant
			c.l1.Delete(k)
			removed++
		}
		return true
	})
	c.l1Len.Add(-removed)

	if c.l1Len.Load() < int64(c.cfg.L1MaxItems) {
		return
	}

	// Pass 2: remove oldest entry (closest expiry = was set earliest).
	var oldest struct {
		key string
		at  time.Time
	}
	oldest.at = now.Add(time.Hour) // sentinel

	c.l1.Range(func(k, v any) bool {
		entry := v.(*l1Entry) //nolint:forcetypeassert // invariant
		if entry.expiresAt.Before(oldest.at) {
			oldest.key = k.(string) //nolint:forcetypeassert // invariant
			oldest.at = entry.expiresAt
		}
		return true
	})

	if oldest.key != "" {
		c.l1.Delete(oldest.key)
		c.l1Len.Add(-1)
	}
}

// cleanupLoop periodically removes expired entries from L1.
func (c *Cache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()
			removed := int64(0)
			c.l1.Range(func(k, v any) bool {
				if now.After(v.(*l1Entry).expiresAt) { //nolint:forcetypeassert // invariant
					c.l1.Delete(k)
					removed++
				}
				return true
			})
			c.l1Len.Add(-removed)
		}
	}
}
