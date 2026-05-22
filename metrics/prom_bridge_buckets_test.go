package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// collectHistogramBuckets returns the bucket cumulative counts keyed by upper-bound.
// Only finite upper-bounds are included (+Inf is excluded so tests stay bucket-shape agnostic
// when asserting specific boundary values).
func collectHistogramBuckets(t *testing.T, h prometheus.Histogram) map[float64]uint64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 1)
	h.Collect(ch)
	close(ch)
	m := <-ch
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatalf("write dto: %v", err)
	}
	out := make(map[float64]uint64)
	for _, b := range dm.GetHistogram().GetBucket() {
		if b.GetUpperBound() < 1e308 { // exclude +Inf bucket
			out[b.GetUpperBound()] = b.GetCumulativeCount()
		}
	}
	return out
}

// TestRegisterHistogram_CustomBuckets — register a name with byte-range
// buckets, observe a value, verify the correct buckets are used.
//
// Contract:
//   - le=10 bucket count == 0 (value 50 > 10)
//   - le=100 bucket count == 1 (value 50 ≤ 100)
//   - le=1000 bucket count == 1 (cumulative, value ≤ 1000)
func TestRegisterHistogram_CustomBuckets(t *testing.T) {
	reg := NewPrometheusRegistry("buck_custom")
	reg.RegisterHistogram("file_size_bytes", WithBuckets([]float64{10, 100, 1000}))
	reg.Observe("file_size_bytes", 50)

	v, ok := reg.promBridge.histogramsNoLabel.Load("file_size_bytes")
	if !ok {
		t.Fatal("histogram not in bridge after Observe")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	if got := buckets[10]; got != 0 {
		t.Fatalf("le=10 count = %d, want 0 (50 > 10)", got)
	}
	if got := buckets[100]; got != 1 {
		t.Fatalf("le=100 count = %d, want 1 (50 ≤ 100)", got)
	}
	if got := buckets[1000]; got != 1 {
		t.Fatalf("le=1000 count = %d, want 1 (cumulative)", got)
	}
	// Sanity: the seconds-shaped default bucket (0.001) must NOT be present
	// for a custom-bucket histogram.
	if _, present := buckets[0.001]; present {
		t.Fatal("seconds-default bucket 0.001 leaked into custom-bucket histogram")
	}
}

// TestRegisterHistogram_DefaultBuckets — Observe without RegisterHistogram
// falls back to the seconds-shaped default (ExponentialBuckets(0.001,2,16)).
// We verify the 0.001 bucket is present to confirm the default was applied.
func TestRegisterHistogram_DefaultBuckets(t *testing.T) {
	reg := NewPrometheusRegistry("buck_default")
	// No RegisterHistogram call — pure fallback.
	reg.Observe("latency_seconds", 0.0005)

	v, ok := reg.promBridge.histogramsNoLabel.Load("latency_seconds")
	if !ok {
		t.Fatal("histogram not in bridge after Observe")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	// ExponentialBuckets(0.001, 2, 16) first bucket is 0.001.
	if _, present := buckets[0.001]; !present {
		t.Fatal("default seconds bucket 0.001 not found — default was not applied")
	}
}

// TestRegisterHistogram_Idempotent — calling RegisterHistogram twice with the
// same name must not panic and must not corrupt the stored config.
func TestRegisterHistogram_Idempotent(t *testing.T) {
	reg := NewPrometheusRegistry("buck_idem")
	reg.RegisterHistogram("upload_bytes", WithBuckets([]float64{512, 4096, 65536}))
	// Second call — must be a no-op (same name, same buckets).
	reg.RegisterHistogram("upload_bytes", WithBuckets([]float64{512, 4096, 65536}))
	// Observe must work without panic.
	reg.Observe("upload_bytes", 1000)

	v, ok := reg.promBridge.histogramsNoLabel.Load("upload_bytes")
	if !ok {
		t.Fatal("histogram not in bridge after second RegisterHistogram + Observe")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	if got := buckets[512]; got != 0 {
		t.Fatalf("le=512 count = %d, want 0 (1000 > 512)", got)
	}
	if got := buckets[4096]; got != 1 {
		t.Fatalf("le=4096 count = %d, want 1 (1000 ≤ 4096)", got)
	}
}

// TestRegisterHistogram_BucketsBeforeObserve — confirms that the configured
// buckets are actually applied to the prom histogram at observation time.
// We observe two values and verify both bucket boundaries are honoured
// (i.e. histogram_quantile would compute correct quantiles from the data).
func TestRegisterHistogram_BucketsBeforeObserve(t *testing.T) {
	reg := NewPrometheusRegistry("buck_order")
	reg.RegisterHistogram("queue_depth", WithBuckets([]float64{1, 5, 10, 50, 100}))
	reg.Observe("queue_depth", 3)  // falls in le=5 bucket
	reg.Observe("queue_depth", 75) // falls in le=100 bucket

	v, ok := reg.promBridge.histogramsNoLabel.Load("queue_depth")
	if !ok {
		t.Fatal("histogram not registered")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	// le=1: neither 3 nor 75 falls here.
	if got := buckets[1]; got != 0 {
		t.Fatalf("le=1 count = %d, want 0", got)
	}
	// le=5: only the first observation (3) falls here.
	if got := buckets[5]; got != 1 {
		t.Fatalf("le=5 count = %d, want 1", got)
	}
	// le=10: still 1 cumulative (3 ≤ 10; 75 > 10).
	if got := buckets[10]; got != 1 {
		t.Fatalf("le=10 count = %d, want 1", got)
	}
	// le=100: 2 cumulative (both 3 and 75 ≤ 100).
	if got := buckets[100]; got != 2 {
		t.Fatalf("le=100 count = %d, want 2", got)
	}
}

// TestRegisterHistogram_NilSafe — RegisterHistogram on a nil *Registry must
// not panic (mirrors the nil-safe contract of all other Registry methods).
func TestRegisterHistogram_NilSafe(t *testing.T) {
	var reg *Registry
	reg.RegisterHistogram("should_not_panic", WithBuckets([]float64{1, 2, 3}))
}
