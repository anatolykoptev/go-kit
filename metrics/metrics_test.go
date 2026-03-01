package metrics_test

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/metrics"
)

func TestIncr(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("requests")
	r.Incr("requests")
	r.Incr("errors")

	snap := r.Snapshot()
	if snap["requests"] != 2 {
		t.Errorf("requests = %d, want 2", snap["requests"])
	}
	if snap["errors"] != 1 {
		t.Errorf("errors = %d, want 1", snap["errors"])
	}
}

func TestAdd(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("bytes", 1024)
	r.Add("bytes", 2048)

	snap := r.Snapshot()
	if snap["bytes"] != 3072 {
		t.Errorf("bytes = %d, want 3072", snap["bytes"])
	}
}

func TestValue(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("counter", 5)
	if got := r.Value("counter"); got != 5 {
		t.Errorf("Value = %d, want 5", got)
	}
}

func TestSnapshot_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("snapshot len = %d, want 0", len(snap))
	}
}

func TestReset(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("a")
	r.Incr("b")
	r.Reset()

	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("after reset, snapshot len = %d, want 0", len(snap))
	}
}

func TestTrackOperation_Success(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if v, ok := snap["errs"]; ok && v != 0 {
		t.Errorf("errs = %d, want 0", v)
	}
}

func TestTrackOperation_Error(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if snap["errs"] != 1 {
		t.Errorf("errs = %d, want 1", snap["errs"])
	}
}

func TestFormat(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("requests", 100)
	r.Add("errors", 3)

	s := r.Format()
	if s == "" {
		t.Error("Format returned empty string")
	}
	if !strings.Contains(s, "requests") || !strings.Contains(s, "errors") {
		t.Errorf("Format missing counter names: %q", s)
	}
}

func TestFormat_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	if got := r.Format(); got != "" {
		t.Errorf("Format = %q, want empty", got)
	}
}

func TestSnapshotAndReset(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("a", 10)
	r.Add("b", 20)

	snap := r.SnapshotAndReset()
	if snap["a"] != 10 || snap["b"] != 20 {
		t.Errorf("snap = %v, want a=10, b=20", snap)
	}

	if v := r.Value("a"); v != 0 {
		t.Errorf("after reset, a = %d, want 0", v)
	}
	if v := r.Value("b"); v != 0 {
		t.Errorf("after reset, b = %d, want 0", v)
	}
}

func TestGauge_SetValue(t *testing.T) {
	r := metrics.NewRegistry()
	g := r.Gauge("temperature")
	g.Set(36.6)
	if v := g.Value(); v != 36.6 {
		t.Errorf("Value = %f, want 36.6", v)
	}
}

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

func TestStartTimer(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.StartTimer("api.latency")
	time.Sleep(5 * time.Millisecond)
	d := h.Stop()

	if d < 5*time.Millisecond {
		t.Errorf("duration = %v, want >= 5ms", d)
	}

	v := r.Gauge("api.latency").Value()
	if v < 5.0 {
		t.Errorf("gauge = %.2f ms, want >= 5.0", v)
	}

	if c := r.Value("api.latency.count"); c != 1 {
		t.Errorf("count = %d, want 1", c)
	}
}

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

func TestRate_Basic(t *testing.T) {
	r := metrics.NewRegistry()
	rate := r.Rate("events")

	for range 100 {
		rate.Update(1)
	}

	if rate.Total() != 100 {
		t.Errorf("Total = %d, want 100", rate.Total())
	}

	snap := rate.Snapshot()
	if snap.Total != 100 {
		t.Errorf("snapshot Total = %d, want 100", snap.Total)
	}
}

func TestRate_Convergence(t *testing.T) {
	r := metrics.NewRegistry()
	rate := r.Rate("rps")

	for i := range 10 {
		rate.Update(100)
		_ = i
	}

	if rate.Total() != 1000 {
		t.Errorf("Total = %d, want 1000", rate.Total())
	}
	if m1 := rate.M1(); m1 < 0 {
		t.Errorf("M1 = %f, want >= 0", m1)
	}
}

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

func TestHistogram_Percentiles(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.Histogram("latency")

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
	if snap.P50 < 40 || snap.P50 > 60 {
		t.Errorf("P50 = %f, want ~50", snap.P50)
	}
	if snap.P99 < 90 {
		t.Errorf("P99 = %f, want ~99", snap.P99)
	}
}

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

func TestHistogram_ReservoirOverflow(t *testing.T) {
	r := metrics.NewRegistry()
	h := r.Histogram("big")

	for i := range 10000 {
		h.Update(float64(i))
	}

	if h.Count() != 10000 {
		t.Errorf("Count = %d, want 10000", h.Count())
	}
	snap := h.Snapshot()
	if snap.Min != 0 {
		t.Errorf("Min = %f, want 0", snap.Min)
	}
	if snap.Max != 9999 {
		t.Errorf("Max = %f, want 9999", snap.Max)
	}
}

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

func TestTTL_Cleanup(t *testing.T) {
	r := metrics.NewRegistry()
	r.IncrWithTTL("stale", 1*time.Millisecond)
	r.Incr("permanent")

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

func TestTTL_Refresh(t *testing.T) {
	r := metrics.NewRegistry()
	r.IncrWithTTL("metric", 50*time.Millisecond)

	time.Sleep(10 * time.Millisecond)
	r.IncrWithTTL("metric", 50*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	removed := r.CleanupExpired()
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (TTL was refreshed)", removed)
	}
	if v := r.Value("metric"); v != 2 {
		t.Errorf("Value = %d, want 2", v)
	}
}

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
	if removed := r.CleanupExpired(); removed != 0 {
		t.Errorf("ttls not cleared, removed = %d", removed)
	}
}
