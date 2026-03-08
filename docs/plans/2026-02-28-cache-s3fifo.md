# cache Phase A: S3-FIFO + Fast Hashing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace FIFO eviction with S3-FIFO algorithm (+10-30% hit rate), replace SHA-256 key hashing with FNV-128a (10x+ faster), replace sync.Map with sync.Mutex + map.

**Architecture:** Complete rewrite of cache.go internals. S3-FIFO uses 3 FIFO queues (Small 10%, Main 90%, Ghost = Main size). On Get: increment 2-bit freq counter (capped at 3). On evict from Small: freq > 0 → promote to Main, freq == 0 → Ghost. On evict from Main: freq > 0 → decrement + reinsert, freq == 0 → fully evict. Public API unchanged (New, Get, Set, Delete, Key, Stats, Close).

**Tech Stack:** Go stdlib only (`container/list`, `hash/fnv`, `encoding/hex`)

---

### Task 1: Complete S3-FIFO cache rewrite

**Files:** cache/cache.go (complete rewrite)

Replace entire cache.go with S3-FIFO implementation. Keep the same public API: `Config`, `New`, `Get`, `Set`, `Delete`, `Key`, `Stats`, `Close`.

**Imports change:**
- Remove: `crypto/sha256`, `sync` (sync.Map no longer used as type — keep for sync.Mutex)
- Add: `container/list`, `hash/fnv`
- Keep: `context`, `encoding/hex`, `strings`, `sync`, `sync/atomic`, `time`

**New internal types:**

```go
type entry struct {
	key       string
	data      []byte
	expiresAt time.Time
	freq      uint8          // 0-3, S3-FIFO frequency counter
	elem      *list.Element  // back-ref in small or main list
	inMain    bool           // false=small, true=main
}
```

**New Cache struct:**

```go
type Cache struct {
	cfg Config

	mu       sync.Mutex
	items    map[string]*entry      // all active entries
	small    *list.List             // probation queue (10% capacity)
	main     *list.List             // main queue (90% capacity)
	ghost    *list.List             // ghost queue (evicted keys, no values)
	ghostMap map[string]*list.Element // ghost key lookups

	smallCap int // 10% of L1MaxItems
	mainCap  int // 90% of L1MaxItems
	ghostCap int // = mainCap

	hits   atomic.Int64
	misses atomic.Int64

	done chan struct{}
}
```

**New() constructor:**

```go
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

	interval := cfg.L1TTL / 10
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	go c.cleanupLoop(interval)

	return c
}
```

**Get()** — lookup entry, increment freq (capped at 3), handle expiry:

```go
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		c.removeEntry(e)
		c.misses.Add(1)
		return nil, false
	}

	if e.freq < 3 {
		e.freq++
	}
	c.hits.Add(1)
	return e.data, true
}
```

**Set()** — check existing, evict if full, check ghost for freq boost, insert to small:

```go
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
}
```

**evict()** — S3-FIFO core: try small first, then main:

```go
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
```

**addToGhost()** — maintain ghost queue with FIFO eviction:

```go
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
```

**removeEntry()** — helper for removing an active entry from its queue:

```go
func (c *Cache) removeEntry(e *entry) {
	if e.inMain {
		c.main.Remove(e.elem)
	} else {
		c.small.Remove(e.elem)
	}
	delete(c.items, e.key)
}
```

**Delete():**

```go
func (c *Cache) Delete(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		c.removeEntry(e)
	}
	_ = ctx
}
```

**Stats():**

```go
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
```

**Key()** — FNV-128a (replaces SHA-256):

```go
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
```

**Close()** — unchanged.

**cleanupLoop()** — updated for new structure:

```go
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
```

**Config and Stats types** — unchanged from current.

**Step 1:** Write the complete new cache.go with all code above.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All 7 tests PASS (TestCache_SetGet, TestCache_Miss, TestCache_Expiry, TestCache_Eviction, TestCache_Delete, TestKey, TestStats, TestCache_NoRedis).

**Step 3:** Fix any test failures.

**Step 4:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: replace FIFO with S3-FIFO eviction, SHA-256 with FNV-128a

Three improvements in one cohesive rewrite:
- S3-FIFO eviction: 3 queues (small/main/ghost) for +10-30% hit rate
- sync.Mutex + map replaces sync.Map for simpler, faster mixed workloads
- FNV-128a replaces SHA-256 for 10x+ faster key hashing

Public API unchanged. All existing tests pass.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: S3-FIFO behavior tests

**Files:** cache/cache_test.go

Add tests verifying S3-FIFO-specific behavior. All tests use the public API only.

**Test 1: Promotion from small to main (accessed entries survive eviction)**

```go
func TestCache_S3FIFO_Promotion(t *testing.T) {
	// Cache with capacity 3: small=1, main=2.
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	// Insert 3 entries.
	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3"))

	// Access "a" to boost its frequency (will be promoted on eviction).
	c.Get(ctx, "a")

	// Insert "d" — triggers eviction.
	c.Set(ctx, "d", []byte("4"))

	// "a" should survive (promoted to main), some other entry evicted.
	if _, ok := c.Get(ctx, "a"); !ok {
		t.Error("accessed entry 'a' should have been promoted and survived eviction")
	}
}
```

**Test 2: One-hit wonder eviction (unaccessed entries evicted first)**

```go
func TestCache_S3FIFO_OneHitWonder(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 3, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "hot", []byte("1"))
	c.Set(ctx, "cold1", []byte("2"))
	c.Set(ctx, "cold2", []byte("3"))

	// Access "hot" multiple times.
	c.Get(ctx, "hot")
	c.Get(ctx, "hot")

	// Fill cache to trigger eviction of cold entries.
	c.Set(ctx, "new1", []byte("4"))
	c.Set(ctx, "new2", []byte("5"))

	// "hot" should still be in cache.
	if _, ok := c.Get(ctx, "hot"); !ok {
		t.Error("frequently accessed 'hot' should survive eviction")
	}
}
```

**Test 3: Ghost readmission boost**

```go
func TestCache_S3FIFO_Ghost(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 2, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	// Insert and evict "a" (goes to ghost).
	c.Set(ctx, "a", []byte("1"))
	c.Set(ctx, "b", []byte("2"))
	c.Set(ctx, "c", []byte("3")) // "a" evicted to ghost

	// Verify "a" is gone.
	if _, ok := c.Get(ctx, "a"); ok {
		t.Error("evicted 'a' should not be in cache")
	}

	// Re-insert "a" (should get ghost frequency boost).
	c.Set(ctx, "a", []byte("1-back"))

	got, ok := c.Get(ctx, "a")
	if !ok {
		t.Fatal("re-inserted 'a' should be in cache")
	}
	if string(got) != "1-back" {
		t.Errorf("got %q, want %q", got, "1-back")
	}
}
```

**Test 4: Frequency capping at 3**

```go
func TestCache_S3FIFO_FreqCap(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 5, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "x", []byte("data"))

	// Access many times — should not panic or behave oddly.
	for range 100 {
		if _, ok := c.Get(ctx, "x"); !ok {
			t.Fatal("entry should still exist")
		}
	}

	// Fill cache and trigger evictions — "x" should survive due to high freq.
	for i := range 10 {
		c.Set(ctx, cache.Key("fill", string(rune('a'+i))), []byte("filler"))
	}

	if _, ok := c.Get(ctx, "x"); !ok {
		t.Error("highly accessed 'x' should survive evictions")
	}
}
```

**Step 1:** Add the 4 tests to cache_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./cache/ -v -count=1
```
Expected: All 11 tests PASS (7 existing + 4 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add cache/cache_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "cache: add S3-FIFO behavior tests

Test promotion, one-hit wonder eviction, ghost readmission, and
frequency capping. Validates S3-FIFO algorithm properties through
the public API.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes:**
- Cache section: update description from "LRU eviction" to "S3-FIFO eviction"
- Cache section: update Key() comment from "SHA-256" to "FNV-128a"
- Package table: update cache description

**ROADMAP changes:**
- Mark cache Phase A as DONE
- Add completion notes

**Step 1:** Update README.md cache section.

**Step 2:** Update ROADMAP.md cache Phase A status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update cache section for S3-FIFO and FNV-128a

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
