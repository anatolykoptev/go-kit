package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// gatherHistogramDTO collects a histogram metric into a dto.Histogram proto.
func gatherHistogramDTO(t *testing.T, h prometheus.Histogram) *dto.Histogram {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	h.Collect(ch)
	close(ch)
	m := <-ch
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatalf("write dto: %v", err)
	}
	return dm.GetHistogram()
}

// gatherObserverDTO collects a labeled histogram observation into a dto.Histogram
// by writing the prometheus.Observer (which also implements prometheus.Metric) to dto.
func gatherObserverDTO(t *testing.T, obs prometheus.Observer) *dto.Histogram {
	t.Helper()
	// prometheus.Observer from a HistogramVec also implements prometheus.Metric.
	m, ok := obs.(prometheus.Metric)
	if !ok {
		t.Fatal("observer does not implement prometheus.Metric")
	}
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatalf("write dto: %v", err)
	}
	return dm.GetHistogram()
}

// TestObserve_PromBackend_NoLabel verifies that Registry.Observe routes through
// the prom bridge for a plain (non-labeled) metric name.
func TestObserve_PromBackend_NoLabel(t *testing.T) {
	reg := NewPrometheusRegistry("obs_nl")
	reg.Observe("llm_latency_seconds", 1.0)
	reg.Observe("llm_latency_seconds", 2.0)

	v, ok := reg.promBridge.histogramsNoLabel.Load("llm_latency_seconds")
	if !ok {
		t.Fatal("histogram not registered in prom bridge after Observe")
	}
	h := v.(prometheus.Histogram) //nolint:forcetypeassert
	dh := gatherHistogramDTO(t, h)
	if got := dh.GetSampleCount(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	if got := dh.GetSampleSum(); got != 3.0 {
		t.Fatalf("sum = %v, want 3.0", got)
	}
}

// TestObserve_PromBackend_Labeled verifies that Registry.Observe routes through
// histogramsVec for a labeled metric name.
func TestObserve_PromBackend_Labeled(t *testing.T) {
	reg := NewPrometheusRegistry("obs_l")
	reg.Observe(Label("llm_latency_seconds", "outcome", "ok"), 0.5)
	reg.Observe(Label("llm_latency_seconds", "outcome", "error"), 5.0)

	v, ok := reg.promBridge.histogramsVec.Load("llm_latency_seconds")
	if !ok {
		t.Fatal("histogram vec not registered in prom bridge after labeled Observe")
	}
	vec := v.(*prometheus.HistogramVec) //nolint:forcetypeassert
	dh := gatherObserverDTO(t, vec.WithLabelValues("ok"))
	if got := dh.GetSampleCount(); got != 1 {
		t.Fatalf("ok count = %d, want 1", got)
	}
	dh2 := gatherObserverDTO(t, vec.WithLabelValues("error"))
	if got := dh2.GetSampleCount(); got != 1 {
		t.Fatalf("error count = %d, want 1", got)
	}
}

// TestObserve_InMemFallback verifies that on a plain (non-prom) Registry,
// Observe feeds the in-mem Reservoir so HistogramSnapshot reflects it.
func TestObserve_InMemFallback(t *testing.T) {
	reg := NewRegistry()
	reg.Observe("latency_seconds", 0.1)
	reg.Observe("latency_seconds", 0.2)
	reg.Observe("latency_seconds", 0.3)

	snap := reg.Histogram("latency_seconds").Snapshot()
	if snap.Count != 3 {
		t.Fatalf("count = %d, want 3", snap.Count)
	}
	if snap.Mean < 0.19 || snap.Mean > 0.21 {
		t.Fatalf("mean = %v, want ~0.2", snap.Mean)
	}
}

// TestObserveSeconds_ConvertsToSeconds verifies that ObserveSeconds(d) is
// equivalent to Observe(d.Seconds()) — a 1s duration produces sum=1.0.
func TestObserveSeconds_ConvertsToSeconds(t *testing.T) {
	reg := NewPrometheusRegistry("obs_sec")
	reg.ObserveSeconds("llm_request_seconds", 1*time.Second)
	reg.ObserveSeconds("llm_request_seconds", 2*time.Second)

	v, ok := reg.promBridge.histogramsNoLabel.Load("llm_request_seconds")
	if !ok {
		t.Fatal("histogram not registered after ObserveSeconds")
	}
	dh := gatherHistogramDTO(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	if got := dh.GetSampleCount(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	if got := dh.GetSampleSum(); got != 3.0 {
		t.Fatalf("sum = %v, want 3.0", got)
	}
}

// TestObserveSeconds_LabelDispatch verifies label routing for ObserveSeconds.
func TestObserveSeconds_LabelDispatch(t *testing.T) {
	reg := NewPrometheusRegistry("obs_sec_l")
	reg.ObserveSeconds(Label("llm_request_seconds", "outcome", "ok"), 500*time.Millisecond)
	reg.ObserveSeconds(Label("llm_request_seconds", "outcome", "error"), 5*time.Second)

	v, ok := reg.promBridge.histogramsVec.Load("llm_request_seconds")
	if !ok {
		t.Fatal("histogramVec not registered after labeled ObserveSeconds")
	}
	vec := v.(*prometheus.HistogramVec) //nolint:forcetypeassert
	dh := gatherObserverDTO(t, vec.WithLabelValues("ok"))
	if got := dh.GetSampleCount(); got != 1 {
		t.Fatalf("ok count = %d, want 1", got)
	}
	if got := dh.GetSampleSum(); got != 0.5 {
		t.Fatalf("ok sum = %v, want 0.5", got)
	}
	dh2 := gatherObserverDTO(t, vec.WithLabelValues("error"))
	if got := dh2.GetSampleCount(); got != 1 {
		t.Fatalf("error count = %d, want 1", got)
	}
	if got := dh2.GetSampleSum(); got != 5.0 {
		t.Fatalf("error sum = %v, want 5.0", got)
	}
}

// TestObserveSeconds_NilSafe confirms that calling ObserveSeconds on a nil
// registry does not panic.
func TestObserveSeconds_NilSafe(t *testing.T) {
	var reg *Registry
	reg.ObserveSeconds("should_not_panic", 1*time.Second)
}
