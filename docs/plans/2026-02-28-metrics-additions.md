# metrics: Observability Toolkit Additions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add SnapshotAndReset, Gauge type, Timer, Label builder, and Sink interface — making the metrics package a mini observability toolkit matching hashicorp/go-metrics and daniel-nichter/go-metrics features.

**Architecture:** All additions to metrics.go. Registry gains a second sync.Map for gauges. Gauge uses atomic CAS for lock-free float64. Timer is syntactic sugar over Gauge + Counter. Labels build composite keys. Sink interface enables JSON/text output. Fully backward-compatible.

**Tech Stack:** Go stdlib only (`math`, `encoding/json`, `io`, `time`)

---

### Task 1: All metrics code additions

**Files:** metrics/metrics.go

**Add to imports:** `"encoding/json"`, `"io"`, `"math"`, `"time"`

**Update Registry struct** — add gauges map:

```go
type Registry struct {
	store  sync.Map // counters: *atomic.Int64
	gauges sync.Map // gauges: *Gauge
}
```

#### 1a. SnapshotAndReset

```go
// SnapshotAndReset returns current counter values and atomically resets them to zero.
func (r *Registry) SnapshotAndReset() map[string]int64 {
	m := make(map[string]int64)
	r.store.Range(func(k, v any) bool {
		m[k.(string)] = v.(*atomic.Int64).Swap(0) //nolint:forcetypeassert // invariant
		return true
	})
	return m
}
```

#### 1b. Gauge type

```go
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
```

Registry gauge methods:

```go
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
```

#### 1c. Timer

```go
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
	h.reg.Gauge(h.name).Set(float64(d.Microseconds()) / 1000.0)
	h.reg.Incr(h.name + ".count")
	return d
}
```

#### 1d. Label

```go
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
```

#### 1e. Sink interface

```go
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
```

#### 1f. Update existing methods

**Update Reset()** — also clear gauges:

```go
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
```

**Update Format()** — include gauges in output:

```go
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
```

**Step 1:** Write the complete updated metrics.go.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./metrics/ -v -count=1
```
Expected: All 9 existing tests PASS (backward-compatible).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add metrics/metrics.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "metrics: add Gauge, Timer, Label, SnapshotAndReset, Sink

Five observability additions:
- SnapshotAndReset: atomic read-and-zero for periodic reporting
- Gauge: lock-free float64 via CAS (Set/Add/Inc/Dec/Value)
- StartTimer/Stop: one-liner duration tracking to gauge + counter
- Label: builds dimensional metric keys (Prometheus-style)
- Sink interface + TextSink + JSONSink + WriteTo for output

All backward-compatible. Format() extended to include gauges.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new features

**Files:** metrics/metrics_test.go

**Test: SnapshotAndReset returns values and zeros counters**

```go
func TestSnapshotAndReset(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("a", 10)
	r.Add("b", 20)

	snap := r.SnapshotAndReset()
	if snap["a"] != 10 || snap["b"] != 20 {
		t.Errorf("snap = %v, want a=10, b=20", snap)
	}

	// Counters should be zero after reset.
	if v := r.Value("a"); v != 0 {
		t.Errorf("after reset, a = %d, want 0", v)
	}
	if v := r.Value("b"); v != 0 {
		t.Errorf("after reset, b = %d, want 0", v)
	}
}
```

**Test: Gauge Set/Value**

```go
func TestGauge_SetValue(t *testing.T) {
	r := metrics.NewRegistry()
	g := r.Gauge("temperature")
	g.Set(36.6)
	if v := g.Value(); v != 36.6 {
		t.Errorf("Value = %f, want 36.6", v)
	}
}
```

**Test: Gauge Add/Inc/Dec**

```go
func TestGauge_AddIncDec(t *testing.T) {
	r := metrics.NewRegistry()
	g := r.Gauge("connections")
	g.Inc()
	g.Inc()
	g.Dec()
	if v := g.Value(); v != 1.0 {
		t.Errorf("Value = %f, want 1.0", v)
	}
	g.Add(2.5)
	if v := g.Value(); v != 3.5 {
		t.Errorf("Value = %f, want 3.5", v)
	}
}
```

**Test: GaugeSnapshot**

```go
func TestGaugeSnapshot(t *testing.T) {
	r := metrics.NewRegistry()
	r.Gauge("cpu").Set(45.2)
	r.Gauge("mem").Set(78.9)

	snap := r.GaugeSnapshot()
	if snap["cpu"] != 45.2 {
		t.Errorf("cpu = %f, want 45.2", snap["cpu"])
	}
	if snap["mem"] != 78.9 {
		t.Errorf("mem = %f, want 78.9", snap["mem"])
	}
}
```

**Test: Timer records gauge and count**

```go
func TestStartTimer(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.StartTimer("api.latency")
	time.Sleep(5 * time.Millisecond)
	d := h.Stop()

	if d < 5*time.Millisecond {
		t.Errorf("duration = %v, want >= 5ms", d)
	}

	// Gauge should have the duration in ms.
	v := r.Gauge("api.latency").Value()
	if v < 5.0 {
		t.Errorf("gauge = %.2f ms, want >= 5.0", v)
	}

	// Count should be 1.
	if c := r.Value("api.latency.count"); c != 1 {
		t.Errorf("count = %d, want 1", c)
	}
}
```

**Test: Label builds keys**

```go
func TestLabel(t *testing.T) {
	tests := []struct {
		name string
		kvs  []string
		want string
	}{
		{"requests", []string{"method", "GET"}, "requests{method=GET}"},
		{"rpc", []string{"service", "auth", "method", "login"}, "rpc{service=auth,method=login}"},
		{"plain", nil, "plain"},
		{"odd", []string{"key"}, "odd"},
	}
	for _, tt := range tests {
		got := metrics.Label(tt.name, tt.kvs...)
		if got != tt.want {
			t.Errorf("Label(%q, %v) = %q, want %q", tt.name, tt.kvs, got, tt.want)
		}
	}
}
```

**Test: Label works with Incr**

```go
func TestLabel_WithIncr(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr(metrics.Label("requests", "method", "GET"))
	r.Incr(metrics.Label("requests", "method", "GET"))
	r.Incr(metrics.Label("requests", "method", "POST"))

	snap := r.Snapshot()
	if snap["requests{method=GET}"] != 2 {
		t.Errorf("GET = %d, want 2", snap["requests{method=GET}"])
	}
	if snap["requests{method=POST}"] != 1 {
		t.Errorf("POST = %d, want 1", snap["requests{method=POST}"])
	}
}
```

**Test: TextSink output**

```go
func TestTextSink(t *testing.T) {
	var buf strings.Builder
	err := metrics.TextSink{}.WriteMetrics(&buf,
		map[string]int64{"requests": 100},
		map[string]float64{"cpu": 45.20},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "cpu=45.20") {
		t.Errorf("missing cpu gauge: %q", got)
	}
	if !strings.Contains(got, "requests=100") {
		t.Errorf("missing requests counter: %q", got)
	}
}
```

**Test: JSONSink output**

```go
func TestJSONSink(t *testing.T) {
	var buf strings.Builder
	err := metrics.JSONSink{}.WriteMetrics(&buf,
		map[string]int64{"errors": 5},
		map[string]float64{"latency": 12.34},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["errors"] != float64(5) {
		t.Errorf("errors = %v, want 5", data["errors"])
	}
	if data["latency"] != 12.34 {
		t.Errorf("latency = %v, want 12.34", data["latency"])
	}
}
```

**Test: WriteTo integration**

```go
func TestWriteTo(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("ops", 42)
	r.Gauge("temp").Set(22.5)

	var buf strings.Builder
	if err := r.WriteTo(&buf, metrics.TextSink{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "ops=42") {
		t.Errorf("missing ops: %q", got)
	}
	if !strings.Contains(got, "temp=22.50") {
		t.Errorf("missing temp: %q", got)
	}
}
```

**Test: Reset clears gauges too**

```go
func TestReset_IncludesGauges(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("a")
	r.Gauge("g").Set(1.0)
	r.Reset()

	if len(r.Snapshot()) != 0 {
		t.Error("counters not cleared")
	}
	if len(r.GaugeSnapshot()) != 0 {
		t.Error("gauges not cleared")
	}
}
```

**Step 1:** Add all tests to metrics_test.go. Add `"encoding/json"` and `"time"` to test imports.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./metrics/ -v -count=1
```
Expected: All tests PASS (9 existing + 11 new = 20).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add metrics/metrics_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "metrics: add tests for Gauge, Timer, Label, SnapshotAndReset, Sink

11 new tests covering all 5 additions. Tests verify atomic
reset, gauge CAS operations, timer recording, label building,
text/JSON sink output, and WriteTo integration.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update metrics section:

```go
import "github.com/anatolykoptev/go-kit/metrics"

reg := metrics.NewRegistry()

// Counters (unchanged)
reg.Incr("requests")
reg.Add("bytes", 1024)

// Gauges — track current values
reg.Gauge("connections").Inc()
reg.Gauge("cpu").Set(45.2)
reg.Gauge("queue").Dec()

// Timer — one-liner duration tracking
defer reg.StartTimer("api.latency").Stop()

// Labels — dimensional metrics
reg.Incr(metrics.Label("requests", "method", "GET"))
reg.Incr(metrics.Label("requests", "method", "POST"))

// Snapshot and reset (for periodic reporting)
snap := reg.SnapshotAndReset() // reads + zeros atomically

// Output formatting
reg.WriteTo(os.Stdout, metrics.TextSink{})  // key=value lines
reg.WriteTo(os.Stdout, metrics.JSONSink{})  // JSON object
```

Update bullet points:
- Gauge type with lock-free float64 (Set/Add/Inc/Dec)
- StartTimer/Stop for one-liner latency tracking
- Label() for dimensional metric keys
- SnapshotAndReset for atomic read-and-zero
- Sink interface with TextSink and JSONSink

**ROADMAP changes:**
- Mark metrics additions 1-5 as DONE

**Step 1:** Update README.md metrics section.

**Step 2:** Update ROADMAP.md metrics status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update metrics section for new features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
