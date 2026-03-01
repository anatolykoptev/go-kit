// Package metrics provides lightweight atomic counters and gauges for operational observability.
// All operations are safe for concurrent use. Zero external dependencies.
// Each Registry is independent — use NewRegistry() per component or share globally.
package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry holds named atomic counters and gauges.
type Registry struct {
	store  sync.Map // counters: *atomic.Int64
	gauges sync.Map // gauges: *Gauge
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
	r.store.Range(func(k, _ any) bool {
		r.store.Delete(k)
		return true
	})
	r.gauges.Range(func(k, _ any) bool {
		r.gauges.Delete(k)
		return true
	})
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
