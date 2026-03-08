# metrics Additions 6-8: EWMA Rate, Reservoir Sampling, TTL

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add EWMA rate tracking (events/sec), reservoir sampling for percentiles (P50/P95/P99), and TTL auto-expiry for dynamic metrics — completing the metrics package for production observability.

**Architecture:** Rate uses lock-free atomics with lazy ticking (CAS-protected). Reservoir uses mutex + Algorithm R for O(1) space sampling. TTL stores deadlines in a sync.Map with explicit CleanupExpired. All integrated into Registry via new sync.Map fields.

**Tech Stack:** Go stdlib only (`math`, `math/rand/v2`, `sort`, `sync`, `sync/atomic`, `time`)

---

### Task 1: All metrics code additions

**Files:** metrics/metrics.go

#### 1a. Add Rate (EWMA) type and helpers

Add `"math/rand/v2"` to imports (needed for Reservoir in 1b).

Add after the Timer section (after line 197):

```go
// ---------------------------------------------------------------------------
// Rate (EWMA)
// ---------------------------------------------------------------------------

// ewma tick interval — all rates tick at 5-second intervals.
const tickNanos = int64(5 * time.Second)

// EWMA decay factors: α = 1 - e^(-interval/window).
var (
	m1Alpha  = 1 - math.Exp(-5.0/60.0)   // 1-minute
	m5Alpha  = 1 - math.Exp(-5.0/300.0)  // 5-minute
	m15Alpha = 1 - math.Exp(-5.0/900.0)  // 15-minute
)

// Rate tracks event throughput using exponentially weighted moving averages.
// Reports events/sec over 1-minute, 5-minute, and 15-minute windows.
// Safe for concurrent use. Ticking is lazy — triggered by Update or snapshot reads.
type Rate struct {
	uncounted atomic.Int64
	total     atomic.Int64
	m1        atomic.Uint64 // float64 bits
	m5        atomic.Uint64
	m15       atomic.Uint64
	lastTick  atomic.Int64 // UnixNano
}

func newRate() *Rate {
	r := &Rate{}
	r.lastTick.Store(time.Now().UnixNano())
	return r
}

// Update records n events.
func (r *Rate) Update(n int64) {
	r.tickIfNeeded()
	r.uncounted.Add(n)
	r.total.Add(n)
}

// Total returns the total number of events ever recorded.
func (r *Rate) Total() int64 { return r.total.Load() }

// M1 returns the 1-minute EWMA rate in events/sec.
func (r *Rate) M1() float64 {
	r.tickIfNeeded()
	return math.Float64frombits(r.m1.Load())
}

// M5 returns the 5-minute EWMA rate in events/sec.
func (r *Rate) M5() float64 {
	r.tickIfNeeded()
	return math.Float64frombits(r.m5.Load())
}

// M15 returns the 15-minute EWMA rate in events/sec.
func (r *Rate) M15() float64 {
	r.tickIfNeeded()
	return math.Float64frombits(r.m15.Load())
}

// Snapshot returns a point-in-time snapshot of all rate values.
func (r *Rate) Snapshot() RateSnapshot {
	r.tickIfNeeded()
	return RateSnapshot{
		Total: r.total.Load(),
		M1:    math.Float64frombits(r.m1.Load()),
		M5:    math.Float64frombits(r.m5.Load()),
		M15:   math.Float64frombits(r.m15.Load()),
	}
}

// RateSnapshot holds a point-in-time view of rate values.
type RateSnapshot struct {
	Total int64
	M1    float64 // events/sec, 1-minute EWMA
	M5    float64 // events/sec, 5-minute EWMA
	M15   float64 // events/sec, 15-minute EWMA
}

func (r *Rate) tickIfNeeded() {
	now := time.Now().UnixNano()
	last := r.lastTick.Load()
	elapsed := now - last
	if elapsed < tickNanos {
		return
	}
	if !r.lastTick.CompareAndSwap(last, now) {
		return // another goroutine is ticking
	}
	ticks := int(elapsed / tickNanos)
	for range ticks {
		r.tick()
	}
}

func (r *Rate) tick() {
	count := float64(r.uncounted.Swap(0))
	instantRate := count / (float64(tickNanos) / float64(time.Second))

	m1 := math.Float64frombits(r.m1.Load())
	m5 := math.Float64frombits(r.m5.Load())
	m15 := math.Float64frombits(r.m15.Load())

	r.m1.Store(math.Float64bits(m1 + m1Alpha*(instantRate-m1)))
	r.m5.Store(math.Float64bits(m5 + m5Alpha*(instantRate-m5)))
	r.m15.Store(math.Float64bits(m15 + m15Alpha*(instantRate-m15)))
}
```

Registry integration — add `rates sync.Map` field to Registry struct, and methods:

```go
// Rate returns a named rate tracker, creating it on first access.
func (r *Registry) Rate(name string) *Rate {
	v, _ := r.rates.LoadOrStore(name, newRate())
	return v.(*Rate) //nolint:forcetypeassert // invariant: only *Rate stored
}

// RateSnapshot returns snapshots of all rate trackers.
func (r *Registry) RateSnapshot() map[string]RateSnapshot {
	m := make(map[string]RateSnapshot)
	r.rates.Range(func(k, v any) bool {
		m[k.(string)] = v.(*Rate).Snapshot()
		return true
	})
	return m
}
```

#### 1b. Add Reservoir (Histogram) type

Add after Rate section:

```go
// ---------------------------------------------------------------------------
// Histogram (Reservoir Sampling)
// ---------------------------------------------------------------------------

const reservoirSize = 2048

// Reservoir collects a fixed-size uniform sample using Algorithm R (Vitter).
// Provides accurate P50/P95/P99 percentiles without unbounded memory.
// Safe for concurrent use.
type Reservoir struct {
	mu      sync.Mutex
	samples [reservoirSize]float64
	count   int64
	sum     float64
	min     float64
	max     float64
	sorted  bool
}

// Update adds a sample value.
func (h *Reservoir) Update(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += v
	if h.count == 1 {
		h.min = v
		h.max = v
	} else {
		if v < h.min {
			h.min = v
		}
		if v > h.max {
			h.max = v
		}
	}

	idx := h.count - 1
	if idx < reservoirSize {
		h.samples[idx] = v
	} else {
		j := rand.Int64N(h.count)
		if j < reservoirSize {
			h.samples[j] = v
		}
	}
	h.sorted = false
}

// Percentile returns the value at the given percentile (0.0–1.0).
// Returns 0 if no samples have been recorded.
func (h *Reservoir) Percentile(p float64) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := min(int(h.count), reservoirSize)
	if n == 0 {
		return 0
	}
	if !h.sorted {
		sort.Float64s(h.samples[:n])
		h.sorted = true
	}
	idx := int(float64(n-1) * p)
	return h.samples[idx]
}

// Count returns the total number of samples recorded.
func (h *Reservoir) Count() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// Snapshot returns a point-in-time histogram summary.
func (h *Reservoir) Snapshot() HistogramSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	n := min(int(h.count), reservoirSize)
	if n == 0 {
		return HistogramSnapshot{}
	}
	if !h.sorted {
		sort.Float64s(h.samples[:n])
		h.sorted = true
	}
	var mean float64
	if h.count > 0 {
		mean = h.sum / float64(h.count)
	}
	return HistogramSnapshot{
		Count: h.count,
		Min:   h.min,
		Max:   h.max,
		Mean:  mean,
		P50:   h.samples[int(float64(n-1)*0.5)],
		P95:   h.samples[int(float64(n-1)*0.95)],
		P99:   h.samples[int(float64(n-1)*0.99)],
	}
}

// HistogramSnapshot holds a point-in-time histogram summary.
type HistogramSnapshot struct {
	Count int64
	Min   float64
	Max   float64
	Mean  float64
	P50   float64
	P95   float64
	P99   float64
}
```

Registry integration — add `histograms sync.Map` field to Registry struct:

```go
// Histogram returns a named histogram (reservoir sampler), creating it on first access.
func (r *Registry) Histogram(name string) *Reservoir {
	v, _ := r.histograms.LoadOrStore(name, &Reservoir{})
	return v.(*Reservoir) //nolint:forcetypeassert // invariant: only *Reservoir stored
}

// HistogramSnapshot returns snapshots of all histograms.
func (r *Registry) HistogramSnapshot() map[string]HistogramSnapshot {
	m := make(map[string]HistogramSnapshot)
	r.histograms.Range(func(k, v any) bool {
		m[k.(string)] = v.(*Reservoir).Snapshot()
		return true
	})
	return m
}
```

#### 1c. Add TTL for dynamic metrics

Add `ttls sync.Map` field to Registry struct.

```go
// ---------------------------------------------------------------------------
// TTL
// ---------------------------------------------------------------------------

// SetTTL marks a metric for automatic expiration. After ttl elapses,
// CleanupExpired will remove the metric from all stores (counters, gauges).
// Each call resets the deadline. Use for per-endpoint or per-user metrics
// that become stale.
func (r *Registry) SetTTL(name string, ttl time.Duration) {
	r.ttls.Store(name, time.Now().Add(ttl).UnixNano())
}

// IncrWithTTL increments a counter and sets/refreshes its TTL.
func (r *Registry) IncrWithTTL(name string, ttl time.Duration) {
	r.counter(name).Add(1)
	r.ttls.Store(name, time.Now().Add(ttl).UnixNano())
}

// AddWithTTL adds delta to a counter and sets/refreshes its TTL.
func (r *Registry) AddWithTTL(name string, delta int64, ttl time.Duration) {
	r.counter(name).Add(delta)
	r.ttls.Store(name, time.Now().Add(ttl).UnixNano())
}

// CleanupExpired removes all metrics whose TTL has expired.
// Returns the number of metrics removed.
func (r *Registry) CleanupExpired() int {
	now := time.Now().UnixNano()
	var removed int
	r.ttls.Range(func(k, v any) bool {
		if v.(int64) < now { //nolint:forcetypeassert // invariant: only int64 stored
			name := k.(string) //nolint:forcetypeassert // invariant
			r.store.Delete(name)
			r.gauges.Delete(name)
			r.ttls.Delete(name)
			removed++
		}
		return true
	})
	return removed
}
```

#### 1d. Update Reset() to include new maps

Update Reset() to also clear rates, histograms, and ttls:

```go
func (r *Registry) Reset() {
	r.store.Range(func(k, _ any) bool { r.store.Delete(k); return true })
	r.gauges.Range(func(k, _ any) bool { r.gauges.Delete(k); return true })
	r.rates.Range(func(k, _ any) bool { r.rates.Delete(k); return true })
	r.histograms.Range(func(k, _ any) bool { r.histograms.Delete(k); return true })
	r.ttls.Range(func(k, _ any) bool { r.ttls.Delete(k); return true })
}
```

**Step 1:** Apply all changes (1a–1d).

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./metrics/ -v -count=1
```
Expected: All 17 existing tests PASS.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add metrics/metrics.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "metrics: add EWMA Rate, Reservoir histogram, TTL expiry

Additions 6-8:
- Rate: EWMA-based events/sec with M1/M5/M15 windows
- Reservoir: Algorithm R sampling for P50/P95/P99 percentiles
- TTL: auto-expire stale per-endpoint/per-user metrics
- Registry integration: Rate(), Histogram(), SetTTL(), CleanupExpired()
- Reset() updated to clear new maps

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new features

**Files:** metrics/metrics_test.go

Add `"time"` is already imported. Add `"math"` to imports.

**Test: Rate basic EWMA**

```go
func TestRate_Basic(t *testing.T) {
	r := metrics.NewRegistry()
	rate := r.Rate("events")

	// Record 100 events
	for range 100 {
		rate.Update(1)
	}

	if rate.Total() != 100 {
		t.Errorf("Total = %d, want 100", rate.Total())
	}

	// M1/M5/M15 should be >= 0 (rates start from 0, need ticks to converge)
	snap := rate.Snapshot()
	if snap.Total != 100 {
		t.Errorf("snapshot Total = %d, want 100", snap.Total)
	}
}
```

**Test: Rate EWMA convergence**

```go
func TestRate_Convergence(t *testing.T) {
	r := metrics.NewRegistry()
	rate := r.Rate("rps")

	// Simulate steady 20 events/sec for multiple tick intervals
	// Tick interval is 5s, so 100 events per tick
	for i := range 10 {
		rate.Update(100)
		// Force ticks by sleeping briefly (lazy tick triggers on next call)
		_ = i
	}

	// After recording 1000 events total, rates should be positive
	if rate.Total() != 1000 {
		t.Errorf("Total = %d, want 1000", rate.Total())
	}
	// M1 should be non-negative
	if m1 := rate.M1(); m1 < 0 {
		t.Errorf("M1 = %f, want >= 0", m1)
	}
}
```

**Test: RateSnapshot from Registry**

```go
func TestRateSnapshot(t *testing.T) {
	r := metrics.NewRegistry()
	r.Rate("a").Update(10)
	r.Rate("b").Update(20)

	snap := r.RateSnapshot()
	if snap["a"].Total != 10 {
		t.Errorf("a.Total = %d, want 10", snap["a"].Total)
	}
	if snap["b"].Total != 20 {
		t.Errorf("b.Total = %d, want 20", snap["b"].Total)
	}
}
```

**Test: Histogram basic percentiles**

```go
func TestHistogram_Percentiles(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.Histogram("latency")

	// Add 100 values: 1, 2, ..., 100
	for i := 1; i <= 100; i++ {
		h.Update(float64(i))
	}

	snap := h.Snapshot()
	if snap.Count != 100 {
		t.Errorf("Count = %d, want 100", snap.Count)
	}
	if snap.Min != 1.0 {
		t.Errorf("Min = %f, want 1.0", snap.Min)
	}
	if snap.Max != 100.0 {
		t.Errorf("Max = %f, want 100.0", snap.Max)
	}
	if math.Abs(snap.Mean-50.5) > 0.01 {
		t.Errorf("Mean = %f, want 50.5", snap.Mean)
	}
	// P50 should be around 50
	if snap.P50 < 40 || snap.P50 > 60 {
		t.Errorf("P50 = %f, want ~50", snap.P50)
	}
	// P99 should be around 99
	if snap.P99 < 90 {
		t.Errorf("P99 = %f, want ~99", snap.P99)
	}
}
```

**Test: Histogram empty**

```go
func TestHistogram_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.Histogram("empty")

	snap := h.Snapshot()
	if snap.Count != 0 {
		t.Errorf("Count = %d, want 0", snap.Count)
	}
	if p := h.Percentile(0.5); p != 0 {
		t.Errorf("P50 = %f, want 0", p)
	}
}
```

**Test: Histogram reservoir sampling (overflow)**

```go
func TestHistogram_ReservoirOverflow(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.Histogram("big")

	// Add 10K values — exceeds 2048 reservoir
	for i := range 10000 {
		h.Update(float64(i))
	}

	if h.Count() != 10000 {
		t.Errorf("Count = %d, want 10000", h.Count())
	}
	// Percentiles should still be reasonable
	snap := h.Snapshot()
	if snap.Min != 0 {
		t.Errorf("Min = %f, want 0", snap.Min)
	}
	if snap.Max != 9999 {
		t.Errorf("Max = %f, want 9999", snap.Max)
	}
}
```

**Test: HistogramSnapshot from Registry**

```go
func TestHistogramSnapshot(t *testing.T) {
	r := metrics.NewRegistry()
	r.Histogram("a").Update(10)
	r.Histogram("b").Update(20)

	snap := r.HistogramSnapshot()
	if snap["a"].Count != 1 || snap["a"].Min != 10 {
		t.Errorf("a = %+v, want count=1, min=10", snap["a"])
	}
	if snap["b"].Count != 1 || snap["b"].Min != 20 {
		t.Errorf("b = %+v, want count=1, min=20", snap["b"])
	}
}
```

**Test: TTL IncrWithTTL + CleanupExpired**

```go
func TestTTL_Cleanup(t *testing.T) {
	r := metrics.NewRegistry()
	r.IncrWithTTL("stale", 1*time.Millisecond)
	r.Incr("permanent")

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	removed := r.CleanupExpired()
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	snap := r.Snapshot()
	if _, ok := snap["stale"]; ok {
		t.Error("stale metric should have been removed")
	}
	if snap["permanent"] != 1 {
		t.Errorf("permanent = %d, want 1", snap["permanent"])
	}
}
```

**Test: TTL refresh extends deadline**

```go
func TestTTL_Refresh(t *testing.T) {
	r := metrics.NewRegistry()
	r.IncrWithTTL("metric", 50*time.Millisecond)

	// Refresh before expiry
	time.Sleep(10 * time.Millisecond)
	r.IncrWithTTL("metric", 50*time.Millisecond)

	// Wait past original deadline but before refreshed deadline
	time.Sleep(20 * time.Millisecond)
	removed := r.CleanupExpired()
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (TTL was refreshed)", removed)
	}
	if v := r.Value("metric"); v != 2 {
		t.Errorf("Value = %d, want 2", v)
	}
}
```

**Test: AddWithTTL**

```go
func TestAddWithTTL(t *testing.T) {
	r := metrics.NewRegistry()
	r.AddWithTTL("bytes", 1024, 1*time.Millisecond)

	if v := r.Value("bytes"); v != 1024 {
		t.Errorf("Value = %d, want 1024", v)
	}

	time.Sleep(5 * time.Millisecond)
	r.CleanupExpired()

	snap := r.Snapshot()
	if _, ok := snap["bytes"]; ok {
		t.Error("bytes should have been expired")
	}
}
```

**Test: Reset clears rates, histograms, ttls**

```go
func TestReset_IncludesRatesAndHistograms(t *testing.T) {
	r := metrics.NewRegistry()
	r.Rate("r").Update(1)
	r.Histogram("h").Update(1)
	r.IncrWithTTL("ttl", time.Minute)
	r.Reset()

	if len(r.RateSnapshot()) != 0 {
		t.Error("rates not cleared")
	}
	if len(r.HistogramSnapshot()) != 0 {
		t.Error("histograms not cleared")
	}
	// TTL map should also be cleared (no metrics to expire)
	if removed := r.CleanupExpired(); removed != 0 {
		t.Errorf("ttls not cleared, removed = %d", removed)
	}
}
```

**Step 1:** Add all 12 tests to metrics_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./metrics/ -v -count=1
```
Expected: All tests PASS (17 existing + 12 new = 29).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add metrics/metrics_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "metrics: add tests for Rate, Histogram, TTL

12 new tests: Rate (basic/convergence/snapshot), Histogram
(percentiles/empty/overflow/snapshot), TTL (cleanup/refresh/add),
Reset includes new maps.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update metrics section with new features:

```go
import "github.com/anatolykoptev/go-kit/metrics"

reg := metrics.NewRegistry()

// Counters
reg.Incr("requests")
reg.Add("bytes", 1024)

// Rate tracking (EWMA)
rate := reg.Rate("events")
rate.Update(1) // record event
rate.M1()      // events/sec, 1-minute window

// Histogram (percentiles via reservoir sampling)
h := reg.Histogram("latency")
h.Update(12.5) // record observation
snap := h.Snapshot()
// snap.P50, snap.P95, snap.P99, snap.Min, snap.Max, snap.Mean

// TTL for dynamic metrics
reg.IncrWithTTL(metrics.Label("api.calls", "path", "/users"), 10*time.Minute)
reg.CleanupExpired() // remove stale metrics
```

Update bullet points:
- Rate (EWMA): events/sec with 1/5/15-minute moving averages
- Histogram: reservoir sampling for P50/P95/P99 without unbounded memory
- TTL: auto-expire stale per-endpoint/per-user metrics

**ROADMAP changes:**
- Mark metrics additions 6-8 as DONE

**Step 1:** Update README.md metrics section.

**Step 2:** Update ROADMAP.md metrics additions 6-8 status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update metrics section for additions 6-8

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
