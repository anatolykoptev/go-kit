// Package cache provides a tiered L1 (memory) + optional L2 (Redis) cache.
// L1 uses S3-FIFO eviction with 3 queues (small, main, ghost) for high hit rates.
// If RedisURL is empty, operates as L1-only (no external dependencies needed at runtime).
package cache

import (
	"container/list"
	"context"
	"encoding/hex"
	"hash/fnv"
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

// entry is an item stored in the S3-FIFO cache.
type entry struct {
	key       string
	data      []byte
	expiresAt time.Time
	freq      uint8         // 0-3, S3-FIFO frequency counter
	elem      *list.Element // back-ref in small or main list
	inMain    bool          // false=small, true=main
}

// Cache is a tiered L1 (memory) + optional L2 (Redis) cache.
// L1 uses the S3-FIFO eviction algorithm with three queues.
type Cache struct {
	cfg Config

	mu       sync.Mutex
	items    map[string]*entry       // all active entries
	small    *list.List              // probation queue (10% capacity)
	main     *list.List              // main queue (90% capacity)
	ghost    *list.List              // ghost queue (evicted keys, no values)
	ghostMap map[string]*list.Element // ghost key lookups

	smallCap int // 10% of L1MaxItems
	mainCap  int // 90% of L1MaxItems
	ghostCap int // = mainCap

	hits   atomic.Int64
	misses atomic.Int64

	done chan struct{}
}

// New creates a new Cache. If cfg.RedisURL is empty, L2 is disabled.
// Call Close() when done to stop the background cleanup goroutine.
func New(cfg Config) *Cache {
	cfg.applyDefaults()

	smallCap := cfg.L1MaxItems / 10
	if smallCap < 1 {
		smallCap = 1
	}
	mainCap := cfg.L1MaxItems - smallCap

	c := &Cache{
		cfg:      cfg,
		items:    make(map[string]*entry),
		small:    list.New(),
		main:     list.New(),
		ghost:    list.New(),
		ghostMap: make(map[string]*list.Element),
		smallCap: smallCap,
		mainCap:  mainCap,
		ghostCap: mainCap,
		done:     make(chan struct{}),
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
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		c.misses.Add(1)
		// TODO: L2 Redis lookup (Phase 2 — add redis/go-redis dependency).
		_ = ctx
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		c.removeEntry(e)
		c.misses.Add(1)
		_ = ctx
		return nil, false
	}

	if e.freq < 3 {
		e.freq++
	}
	c.hits.Add(1)
	return e.data, true
}

// Set stores a value in L1 (and L2 if configured).
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if e, ok := c.items[key]; ok {
		e.data = data
		e.expiresAt = time.Now().Add(c.cfg.L1TTL)
		return
	}

	// Evict until under capacity.
	for len(c.items) >= c.cfg.L1MaxItems {
		if !c.evict() {
			break
		}
	}

	// Check ghost for frequency boost.
	var initFreq uint8
	if ge, ok := c.ghostMap[key]; ok {
		c.ghost.Remove(ge)
		delete(c.ghostMap, key)
		initFreq = 1 // ghost re-admission boost
	}

	// Insert into small queue.
	e := &entry{
		key:       key,
		data:      data,
		expiresAt: time.Now().Add(c.cfg.L1TTL),
		freq:      initFreq,
	}
	e.elem = c.small.PushBack(e)
	c.items[key] = e

	// TODO: L2 Redis set (Phase 2).
	_ = ctx
}

// Delete removes a key from both L1 and L2.
func (c *Cache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		c.removeEntry(e)
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
	c.mu.Lock()
	size := len(c.items)
	c.mu.Unlock()
	return Stats{
		L1Hits:   c.hits.Load(),
		L1Misses: c.misses.Load(),
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

// Key builds a deterministic cache key from parts using FNV-128a.
func Key(parts ...string) string {
	h := fnv.New128a()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// evict removes one entry from the cache using S3-FIFO policy.
func (c *Cache) evict() bool {
	now := time.Now()

	// Phase 1: evict from small queue.
	for c.small.Len() > 0 {
		front := c.small.Front()
		e := front.Value.(*entry)
		c.small.Remove(front)

		if now.After(e.expiresAt) {
			delete(c.items, e.key)
			return true
		}

		if e.freq > 0 {
			// Accessed while in small — promote to main.
			e.freq = 0
			e.inMain = true
			e.elem = c.main.PushBack(e)
			continue
		}

		// One-hit wonder — evict to ghost.
		delete(c.items, e.key)
		c.addToGhost(e.key)
		return true
	}

	// Phase 2: evict from main queue (CLOCK-like second chance).
	limit := c.main.Len()
	for i := 0; i < limit && c.main.Len() > 0; i++ {
		front := c.main.Front()
		e := front.Value.(*entry)
		c.main.Remove(front)

		if now.After(e.expiresAt) {
			delete(c.items, e.key)
			return true
		}

		if e.freq > 0 {
			e.freq--
			e.elem = c.main.PushBack(e)
			continue
		}

		delete(c.items, e.key)
		return true
	}

	// Safety: force evict front of main if all had freq > 0.
	if front := c.main.Front(); front != nil {
		e := front.Value.(*entry)
		c.main.Remove(front)
		delete(c.items, e.key)
		return true
	}

	return false
}

// addToGhost adds a key to the ghost queue, evicting the oldest ghost if full.
func (c *Cache) addToGhost(key string) {
	for len(c.ghostMap) >= c.ghostCap {
		front := c.ghost.Front()
		if front == nil {
			break
		}
		old := front.Value.(string)
		c.ghost.Remove(front)
		delete(c.ghostMap, old)
	}
	elem := c.ghost.PushBack(key)
	c.ghostMap[key] = elem
}

// removeEntry removes an active entry from its queue and the items map.
func (c *Cache) removeEntry(e *entry) {
	if e.inMain {
		c.main.Remove(e.elem)
	} else {
		c.small.Remove(e.elem)
	}
	delete(c.items, e.key)
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
			c.mu.Lock()
			now := time.Now()
			for key, e := range c.items {
				if now.After(e.expiresAt) {
					if e.inMain {
						c.main.Remove(e.elem)
					} else {
						c.small.Remove(e.elem)
					}
					delete(c.items, key)
				}
			}
			c.mu.Unlock()
		}
	}
}
