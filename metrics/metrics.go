// Package metrics provides lightweight atomic counters and gauges for operational observability.
// All operations are safe for concurrent use. Zero external dependencies.
// Each Registry is independent — use NewRegistry() per component or share globally.
package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry holds named atomic counters and gauges.
type Registry struct {
	store      sync.Map // counters: *atomic.Int64
	gauges     sync.Map // gauges: *Gauge
	rates      sync.Map // rates: *Rate
	histograms sync.Map // histograms: *Reservoir
	ttls       sync.Map // name -> int64 (deadline UnixNano)
}

// NewRegistry creates a new empty counter registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// counter returns the *atomic.Int64 for name, creating it on first access.
func (r *Registry) counter(name string) *atomic.Int64 {
	v, _ := r.store.LoadOrStore(name, new(atomic.Int64))
	return v.(*atomic.Int64) //nolint:forcetypeassert // invariant: only *atomic.Int64 stored
}

// Incr increments the named counter by 1.
func (r *Registry) Incr(name string) {
	r.counter(name).Add(1)
}

// Add adds delta to the named counter.
func (r *Registry) Add(name string, delta int64) {
	r.counter(name).Add(delta)
}

// Value returns the current value of the named counter.
func (r *Registry) Value(name string) int64 {
	return r.counter(name).Load()
}

// Snapshot returns a copy of all counters with their current values.
// Only counters that have been written at least once are included.
func (r *Registry) Snapshot() map[string]int64 {
	m := make(map[string]int64)
	r.store.Range(func(k, v any) bool {
		m[k.(string)] = v.(*atomic.Int64).Load() //nolint:forcetypeassert // invariant
		return true
	})
	return m
}

// SnapshotAndReset returns current counter values and atomically resets them to zero.
func (r *Registry) SnapshotAndReset() map[string]int64 {
	m := make(map[string]int64)
	r.store.Range(func(k, v any) bool {
		m[k.(string)] = v.(*atomic.Int64).Swap(0) //nolint:forcetypeassert // invariant
		return true
	})
	return m
}

// Reset clears all counters and gauges. Intended for tests.
func (r *Registry) Reset() {
	r.store.Range(func(k, _ any) bool { r.store.Delete(k); return true })
	r.gauges.Range(func(k, _ any) bool { r.gauges.Delete(k); return true })
	r.rates.Range(func(k, _ any) bool { r.rates.Delete(k); return true })
	r.histograms.Range(func(k, _ any) bool { r.histograms.Delete(k); return true })
	r.ttls.Range(func(k, _ any) bool { r.ttls.Delete(k); return true })
}

// Format returns a human-readable summary of all counters and gauges, sorted by name.
func (r *Registry) Format() string {
	counters := r.Snapshot()
	gauges := r.GaugeSnapshot()
	if len(counters) == 0 && len(gauges) == 0 {
		return ""
	}

	type entry struct {
		name string
		text string
	}
	entries := make([]entry, 0, len(counters)+len(gauges))
	for k, v := range counters {
		entries = append(entries, entry{k, fmt.Sprintf("%s=%d", k, v)})
	}
	for k, v := range gauges {
		entries = append(entries, entry{k, fmt.Sprintf("%s=%.2f", k, v)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TrackOperation increments callCounter, runs fn, and increments errCounter
// if fn returns a non-nil error. The error from fn is always returned unchanged.
func (r *Registry) TrackOperation(callCounter, errCounter string, fn func() error) error {
	r.Incr(callCounter)
	if err := fn(); err != nil {
		r.Incr(errCounter)
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Gauge
// ---------------------------------------------------------------------------

// Gauge tracks a float64 value that can increase or decrease.
// All operations are lock-free using atomic compare-and-swap.
type Gauge struct {
	bits atomic.Uint64
}

// Set sets the gauge to v.
func (g *Gauge) Set(v float64) { g.bits.Store(math.Float64bits(v)) }

// Value returns the current gauge value.
func (g *Gauge) Value() float64 { return math.Float64frombits(g.bits.Load()) }

// Add adds delta to the gauge value.
func (g *Gauge) Add(delta float64) {
	for {
		old := g.bits.Load()
		newVal := math.Float64bits(math.Float64frombits(old) + delta)
		if g.bits.CompareAndSwap(old, newVal) {
			return
		}
	}
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() { g.Add(1) }

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() { g.Add(-1) }

// Gauge returns the named gauge, creating it on first access.
func (r *Registry) Gauge(name string) *Gauge {
	v, _ := r.gauges.LoadOrStore(name, &Gauge{})
	return v.(*Gauge) //nolint:forcetypeassert // invariant: only *Gauge stored
}

// GaugeSnapshot returns a copy of all gauges with their current values.
func (r *Registry) GaugeSnapshot() map[string]float64 {
	m := make(map[string]float64)
	r.gauges.Range(func(k, v any) bool {
		m[k.(string)] = v.(*Gauge).Value() //nolint:forcetypeassert // invariant
		return true
	})
	return m
}

// ---------------------------------------------------------------------------
// Timer
// ---------------------------------------------------------------------------

// TimerHandle tracks a started timer. Call Stop to record the duration.
type TimerHandle struct {
	reg   *Registry
	name  string
	start time.Time
}

// StartTimer starts a timer for the named metric.
// Call Stop to record the duration. Usage: defer reg.StartTimer("api.latency").Stop()
func (r *Registry) StartTimer(name string) *TimerHandle {
	return &TimerHandle{reg: r, name: name, start: time.Now()}
}

// Stop records the elapsed duration since StartTimer.
// Sets gauge "name" to the duration in milliseconds (float64 for sub-ms precision).
// Increments counter "name.count".
func (h *TimerHandle) Stop() time.Duration {
	d := time.Since(h.start)
	h.reg.Gauge(h.name).Set(float64(d.Microseconds()) / 1000.0) //nolint:mnd // ms conversion
	h.reg.Incr(h.name + ".count")
	return d
}

// ---------------------------------------------------------------------------
// Rate (EWMA)
// ---------------------------------------------------------------------------

// ewma tick interval — all rates tick at 5-second intervals.
const tickNanos = int64(5 * time.Second)

// EWMA decay factors: α = 1 - e^(-interval/window).
var (
	m1Alpha  = 1 - math.Exp(-5.0/60.0)  // 1-minute
	m5Alpha  = 1 - math.Exp(-5.0/300.0) // 5-minute
	m15Alpha = 1 - math.Exp(-5.0/900.0) // 15-minute
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

// Percentile returns the value at the given percentile (0.0-1.0).
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

// ---------------------------------------------------------------------------
// Label
// ---------------------------------------------------------------------------

// Label builds a metric key with labels. Labels are alternating key-value pairs.
// Label("requests", "method", "GET") returns "requests{method=GET}".
// Label("rpc", "service", "auth", "method", "login") returns "rpc{service=auth,method=login}".
// Returns name unchanged if no labels or odd number of label values.
func Label(name string, kvs ...string) string {
	if len(kvs) == 0 || len(kvs)%2 != 0 {
		return name
	}
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i := 0; i < len(kvs); i += 2 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(kvs[i])
		sb.WriteByte('=')
		sb.WriteString(kvs[i+1])
	}
	sb.WriteByte('}')
	return sb.String()
}

// ---------------------------------------------------------------------------
// Sink
// ---------------------------------------------------------------------------

// Sink formats metrics for output.
type Sink interface {
	WriteMetrics(w io.Writer, counters map[string]int64, gauges map[string]float64) error
}

// TextSink outputs metrics as sorted key=value lines.
type TextSink struct{}

func (TextSink) WriteMetrics(w io.Writer, counters map[string]int64, gauges map[string]float64) error {
	type entry struct {
		name string
		text string
	}
	entries := make([]entry, 0, len(counters)+len(gauges))
	for k, v := range counters {
		entries = append(entries, entry{k, fmt.Sprintf("%s=%d", k, v)})
	}
	for k, v := range gauges {
		entries = append(entries, entry{k, fmt.Sprintf("%s=%.2f", k, v)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		fmt.Fprintln(w, e.text)
	}
	return nil
}

// JSONSink outputs metrics as a JSON object.
type JSONSink struct{}

func (JSONSink) WriteMetrics(w io.Writer, counters map[string]int64, gauges map[string]float64) error {
	data := make(map[string]any, len(counters)+len(gauges))
	for k, v := range counters {
		data[k] = v
	}
	for k, v := range gauges {
		data[k] = v
	}
	return json.NewEncoder(w).Encode(data)
}

// WriteTo writes all metrics to w using the given Sink.
func (r *Registry) WriteTo(w io.Writer, sink Sink) error {
	return sink.WriteMetrics(w, r.Snapshot(), r.GaugeSnapshot())
}
