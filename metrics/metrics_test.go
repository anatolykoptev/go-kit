package metrics_test

import (
	"encoding/json"
	"errors"
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
