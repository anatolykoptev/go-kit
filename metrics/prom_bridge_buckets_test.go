package metrics

import (
	"math"
	"strings"
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

// collectHistogramVecBuckets returns bucket cumulative counts keyed by upper-bound
// for a specific label value set on a *prometheus.HistogramVec.
// Only finite upper-bounds are included (+Inf is excluded).
//
// We gather all metrics from the Vec and find the one matching the requested
// label values, since GetMetricWithLabelValues returns prometheus.Observer
// (not prometheus.Collector) and cannot be used to scrape bucket state.
func collectHistogramVecBuckets(t *testing.T, vec *prometheus.HistogramVec, labelVals ...string) map[float64]uint64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 16)
	vec.Collect(ch)
	close(ch)
	for m := range ch {
		var dm dto.Metric
		if err := m.Write(&dm); err != nil {
			t.Fatalf("write dto: %v", err)
		}
		// Match the metric whose label values align with the requested set.
		labels := dm.GetLabel()
		if len(labels) != len(labelVals) {
			continue
		}
		match := true
		for i, lv := range labelVals {
			if labels[i].GetValue() != lv {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out := make(map[float64]uint64)
		for _, b := range dm.GetHistogram().GetBucket() {
			if b.GetUpperBound() < 1e308 { // exclude +Inf bucket
				out[b.GetUpperBound()] = b.GetCumulativeCount()
			}
		}
		return out
	}
	t.Fatalf("no metric found in HistogramVec matching label values %v", labelVals)
	return nil
}

// ---------------------------------------------------------------------------
// MAJOR #1 — validate buckets at RegisterHistogram time (not at Observe time)
// ---------------------------------------------------------------------------

// TestRegisterHistogram_InvalidBuckets_Panics verifies that invalid bucket
// configurations panic at RegisterHistogram (startup), not deferred to the
// first Observe call. This satisfies CLAUDE.md "no panic outside startup".
//
// Per prometheus semantics, valid buckets must be strictly ascending and finite.
func TestRegisterHistogram_InvalidBuckets_Panics(t *testing.T) {
	t.Run("descending_order", func(t *testing.T) {
		reg := NewPrometheusRegistry("inv_desc")
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic for descending buckets, got none")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("panic value is not a string: %T %v", r, r)
			}
			if !strings.Contains(msg, "invalid buckets") {
				t.Fatalf("panic message %q does not contain 'invalid buckets'", msg)
			}
		}()
		// {5, 3, 1} is descending — must panic at registration, not first Observe.
		reg.RegisterHistogram("bad_desc", WithBuckets([]float64{5, 3, 1}))
	})

	t.Run("NaN_bucket", func(t *testing.T) {
		reg := NewPrometheusRegistry("inv_nan")
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic for NaN bucket, got none")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("panic value is not a string: %T %v", r, r)
			}
			if !strings.Contains(msg, "invalid buckets") {
				t.Fatalf("panic message %q does not contain 'invalid buckets'", msg)
			}
		}()
		reg.RegisterHistogram("bad_nan", WithBuckets([]float64{1, math.NaN(), 100}))
	})

	t.Run("single_element_is_valid", func(t *testing.T) {
		// Single-element slice is strictly ascending (vacuously) — must NOT panic.
		reg := NewPrometheusRegistry("inv_single")
		reg.RegisterHistogram("single_bucket", WithBuckets([]float64{42}))
		reg.Observe("single_bucket", 10)
	})

	t.Run("nil_buckets_is_valid_noop", func(t *testing.T) {
		// nil buckets = use default; must NOT panic.
		reg := NewPrometheusRegistry("inv_nil")
		reg.RegisterHistogram("nil_bucket", WithBuckets(nil))
		reg.Observe("nil_bucket", 0.001)
	})
}

// ---------------------------------------------------------------------------
// MAJOR #2 — load-bearing "first wins" contracts
// ---------------------------------------------------------------------------

// TestRegisterHistogram_ObserveBeforeRegister_DefaultsLock verifies that once
// a histogram has been observed (and thus locked into default buckets), a
// subsequent RegisterHistogram call is a no-op — default buckets remain.
func TestRegisterHistogram_ObserveBeforeRegister_DefaultsLock(t *testing.T) {
	reg := NewPrometheusRegistry("lock_default")
	// First Observe — no RegisterHistogram yet; defaults (ExponentialBuckets(0.001,2,16)) lock in.
	reg.Observe("lock_metric", 0.0005)

	// Try to override with custom buckets after first Observe — must be ignored.
	reg.RegisterHistogram("lock_metric", WithBuckets([]float64{10, 100, 1000}))

	// Second Observe — must still use default buckets.
	reg.Observe("lock_metric", 0.0005)

	v, ok := reg.promBridge.histogramsNoLabel.Load("lock_metric")
	if !ok {
		t.Fatal("histogram not in bridge")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert
	// Default bucket 0.001 must be present (confirms default was applied).
	if _, present := buckets[0.001]; !present {
		t.Fatal("default seconds bucket 0.001 not found — defaults did not lock in")
	}
	// Custom bucket 10 must NOT be present (custom registration was ignored).
	if _, present := buckets[10]; present {
		t.Fatal("custom bucket le=10 found — post-observe RegisterHistogram was not ignored")
	}
}

// TestRegisterHistogram_DoubleRegister_FirstWins verifies that calling
// RegisterHistogram twice for the same name uses the first bucket set, not
// the second. Extends TestRegisterHistogram_Idempotent to use DIFFERENT slices.
func TestRegisterHistogram_DoubleRegister_FirstWins(t *testing.T) {
	reg := NewPrometheusRegistry("first_wins")
	reg.RegisterHistogram("rh_first", WithBuckets([]float64{1, 2, 3}))
	// Second call with a completely different bucket set — must be ignored.
	reg.RegisterHistogram("rh_first", WithBuckets([]float64{10, 20, 30}))

	reg.Observe("rh_first", 15) // falls in 20-bucket of second set, but NOT in 3 of first set

	v, ok := reg.promBridge.histogramsNoLabel.Load("rh_first")
	if !ok {
		t.Fatal("histogram not in bridge after Observe")
	}
	buckets := collectHistogramBuckets(t, v.(prometheus.Histogram)) //nolint:forcetypeassert

	// First bucket set {1, 2, 3}: value 15 is above all three → all le counts == 0.
	if got := buckets[1]; got != 0 {
		t.Fatalf("le=1 count = %d, want 0 (first set used; 15 > 1)", got)
	}
	if got := buckets[2]; got != 0 {
		t.Fatalf("le=2 count = %d, want 0 (first set used; 15 > 2)", got)
	}
	if got := buckets[3]; got != 0 {
		t.Fatalf("le=3 count = %d, want 0 (first set used; 15 > 3)", got)
	}
	// Second bucket set must not exist: le=20 would have count==1 if second set won.
	if _, present := buckets[20]; present {
		t.Fatal("le=20 found — second RegisterHistogram overwrote first (first-wins violated)")
	}
}

// ---------------------------------------------------------------------------
// MAJOR #3 — labelled histogram path coverage
// ---------------------------------------------------------------------------

// TestRegisterHistogram_Labelled verifies the labeled path (parseLabeled →
// histogramVec) honours custom bucket boundaries configured via RegisterHistogram.
//
// Contracts verified via scrape (bucket inspection on the HistogramVec):
//   - le=10 for label=foo: count 0 (value 50 > 10)
//   - le=100 for label=foo: count 1 (value 50 ≤ 100)
//   - le=10 for label=bar: count 0 (value 500 > 10)
//   - le=100 for label=bar: count 0 (value 500 > 100)
//   - le=1000 for label=bar: count 1 (value 500 ≤ 1000)
//   - seconds-default bucket 0.001 must NOT be present for either label set
func TestRegisterHistogram_Labelled(t *testing.T) {
	reg := NewPrometheusRegistry("lab_custom")
	reg.RegisterHistogram("mymetric", WithBuckets([]float64{10, 100, 1000}))
	reg.Observe(Label("mymetric", "label", "foo"), 50)
	reg.Observe(Label("mymetric", "label", "bar"), 500)

	vecRaw, ok := reg.promBridge.histogramsVec.Load("mymetric")
	if !ok {
		t.Fatal("histogram vec not in bridge after Observe")
	}
	vec := vecRaw.(*prometheus.HistogramVec) //nolint:forcetypeassert

	// --- label=foo (observed value 50) ---
	fooB := collectHistogramVecBuckets(t, vec, "foo")
	if got := fooB[10]; got != 0 {
		t.Fatalf("[foo] le=10 count = %d, want 0 (50 > 10)", got)
	}
	if got := fooB[100]; got != 1 {
		t.Fatalf("[foo] le=100 count = %d, want 1 (50 ≤ 100)", got)
	}
	if got := fooB[1000]; got != 1 {
		t.Fatalf("[foo] le=1000 count = %d, want 1 (cumulative)", got)
	}
	if _, present := fooB[0.001]; present {
		t.Fatal("[foo] seconds-default bucket 0.001 leaked into labelled custom-bucket histogram")
	}

	// --- label=bar (observed value 500) ---
	barB := collectHistogramVecBuckets(t, vec, "bar")
	if got := barB[10]; got != 0 {
		t.Fatalf("[bar] le=10 count = %d, want 0 (500 > 10)", got)
	}
	if got := barB[100]; got != 0 {
		t.Fatalf("[bar] le=100 count = %d, want 0 (500 > 100)", got)
	}
	if got := barB[1000]; got != 1 {
		t.Fatalf("[bar] le=1000 count = %d, want 1 (500 ≤ 1000)", got)
	}
	if _, present := barB[0.001]; present {
		t.Fatal("[bar] seconds-default bucket 0.001 leaked into labelled custom-bucket histogram")
	}
}
