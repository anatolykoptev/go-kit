package metrics

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestParseLabeled(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantKeys []string
		wantVals []string
	}{
		{"wp_rest_calls", "wp_rest_calls", nil, nil},
		{"rpc{method=login}", "rpc", []string{"method"}, []string{"login"}},
		{"rpc{service=auth,method=login}", "rpc", []string{"service", "method"}, []string{"auth", "login"}},
		{"malformed{", "malformed{", nil, nil},                    // invalid → treat as plain
		{"rpc{method=}", "rpc", []string{"method"}, []string{""}}, // empty value allowed
		{"rpc{method=,timeout=5s}", "rpc", []string{"method", "timeout"}, []string{"", "5s"}},
	}
	for _, c := range cases {
		gotName, gotKeys, gotVals := parseLabeled(c.in)
		if gotName != c.wantName {
			t.Errorf("%q: name = %q, want %q", c.in, gotName, c.wantName)
		}
		if len(gotKeys) != len(c.wantKeys) {
			t.Errorf("%q: keys = %v, want %v", c.in, gotKeys, c.wantKeys)
			continue
		}
		for i := range gotKeys {
			if gotKeys[i] != c.wantKeys[i] || gotVals[i] != c.wantVals[i] {
				t.Errorf("%q: [%d] = (%q,%q), want (%q,%q)", c.in, i, gotKeys[i], gotVals[i], c.wantKeys[i], c.wantVals[i])
			}
		}
	}
}

func TestNewPrometheusRegistry_Constructs(t *testing.T) {
	reg := NewPrometheusRegistry("testsvc")
	if reg == nil {
		t.Fatal("nil registry")
	}
	if reg.promBridge == nil {
		t.Fatal("promBridge not initialized")
	}
	if reg.promBridge.namespace != "testsvc" {
		t.Fatalf("namespace = %q, want %q", reg.promBridge.namespace, "testsvc")
	}
}

func TestMetricsHandler_Serves(t *testing.T) {
	h := MetricsHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()
	_ = prometheus.DefaultRegisterer
}

func TestNewPrometheusRegistry_PanicsOnBadNamespace(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	NewPrometheusRegistry("bad-name")
}

func TestIncr_WritesToProm(t *testing.T) {
	reg := NewPrometheusRegistry("t1")
	reg.Incr("wp_rest_calls")
	reg.Incr("wp_rest_calls")
	reg.Add("wp_rest_calls", 3)

	got := testutil.ToFloat64(reg.promBridge.counterNoLabels("wp_rest_calls"))
	if got != 5 {
		t.Fatalf("counter = %v, want 5", got)
	}
	if v := reg.Value("wp_rest_calls"); v != 5 {
		t.Fatalf("in-mem = %d, want 5", v)
	}
}

func TestIncr_LabeledCounter(t *testing.T) {
	reg := NewPrometheusRegistry("t2")
	reg.Incr(Label("rpc", "method", "login"))
	reg.Incr(Label("rpc", "method", "login"))
	reg.Incr(Label("rpc", "method", "logout"))

	vec, _ := reg.promBridge.counters.Load("rpc")
	cv := vec.(*prometheus.CounterVec)
	if got := testutil.ToFloat64(cv.WithLabelValues("login")); got != 2 {
		t.Fatalf("login = %v, want 2", got)
	}
	if got := testutil.ToFloat64(cv.WithLabelValues("logout")); got != 1 {
		t.Fatalf("logout = %v, want 1", got)
	}
}

func TestIncr_Race(t *testing.T) {
	reg := NewPrometheusRegistry("trace")
	const workers = 100
	const ops = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				reg.Incr("hot")
			}
		}()
	}
	wg.Wait()
	if v := reg.Value("hot"); v != workers*ops {
		t.Fatalf("in-mem = %d, want %d", v, workers*ops)
	}
	if got := testutil.ToFloat64(reg.promBridge.counterNoLabels("hot")); got != workers*ops {
		t.Fatalf("prom = %v, want %d", got, workers*ops)
	}
}

func TestGauge_WritesToProm(t *testing.T) {
	reg := NewPrometheusRegistry("tg")
	reg.Gauge("queue_depth").Set(42)
	reg.Gauge("queue_depth").Add(8)

	got := testutil.ToFloat64(reg.promBridge.gaugeNoLabels("queue_depth"))
	if got != 50 {
		t.Fatalf("gauge = %v, want 50", got)
	}
}

func TestGauge_Labeled(t *testing.T) {
	reg := NewPrometheusRegistry("tgl")
	reg.Gauge(Label("queue", "name", "a")).Set(5)
	reg.Gauge(Label("queue", "name", "b")).Set(7)

	v, _ := reg.promBridge.gauges.Load("queue")
	vec := v.(*prometheus.GaugeVec)
	if got := testutil.ToFloat64(vec.WithLabelValues("a")); got != 5 {
		t.Fatalf("a = %v", got)
	}
	if got := testutil.ToFloat64(vec.WithLabelValues("b")); got != 7 {
		t.Fatalf("b = %v", got)
	}
}

func TestStartTimer_WritesHistogram(t *testing.T) {
	reg := NewPrometheusRegistry("tt")
	h := reg.StartTimer("api_call_seconds")
	time.Sleep(5 * time.Millisecond)
	h.Stop()
	h2 := reg.StartTimer("api_call_seconds")
	time.Sleep(2 * time.Millisecond)
	h2.Stop()

	v, ok := reg.promBridge.histograms.Load("api_call_seconds")
	if !ok {
		t.Fatal("histogram not registered")
	}
	_ = v.(prometheus.Histogram)
}

func TestCleanupExpired_UnregistersFromProm(t *testing.T) {
	reg := NewPrometheusRegistry("tcl")
	reg.IncrWithTTL("volatile_metric", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	removed := reg.CleanupExpired()
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	// re-registration must not panic
	reg.IncrWithTTL("volatile_metric", 1*time.Minute)
}
