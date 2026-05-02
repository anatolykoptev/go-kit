package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestPromBridge_NoLabelAndLabeledCoexist guards against the type-collision
// panic fixed in fix/prom-bridge-type-collision.
//
// Repro of the original panic:
//
//	reg.Incr("search_failures")                       // stores Counter
//	reg.Incr("search_failures{source=reddit}")        // PANIC: Counter is not *CounterVec
//
// Contract after the fix:
//   - Neither call panics.
//   - The first-registered shape stays on /metrics; the losing shape's
//     observations are dropped silently and gokit_metrics_shape_collisions_total
//     increments. (Prometheus rejects two collectors sharing a fqName with
//     different label sets, so true co-existence on /metrics is impossible
//     by design of the upstream registry.)
func TestPromBridge_NoLabelAndLabeledCoexist(t *testing.T) {
	t.Run("counter_unlabeled_then_labeled", func(t *testing.T) {
		startCol := collisionCount()
		reg := NewPrometheusRegistry("collide_c1")
		reg.Incr("search_failures")
		// Following call panicked before the fix.
		reg.Incr(Label("search_failures", "source", "reddit"))
		reg.Incr(Label("search_failures", "source", "reddit"))

		// First shape (unlabeled) wins on /metrics.
		c := reg.promBridge.counterNoLabels("search_failures")
		if got := testutil.ToFloat64(c); got != 1 {
			t.Fatalf("unlabeled counter = %v, want 1", got)
		}
		if collisionCount() <= startCol {
			t.Fatalf("collision counter did not increment")
		}
	})

	t.Run("counter_labeled_then_unlabeled", func(t *testing.T) {
		startCol := collisionCount()
		reg := NewPrometheusRegistry("collide_c2")
		reg.Incr(Label("api_calls", "ep", "list"))
		reg.Incr(Label("api_calls", "ep", "list"))
		reg.Incr("api_calls") // panicked before the fix in the reverse direction
		reg.Incr("api_calls")

		// First shape (labeled) wins on /metrics.
		vec := reg.promBridge.counterVec("api_calls", []string{"ep"})
		if got := testutil.ToFloat64(vec.WithLabelValues("list")); got != 2 {
			t.Fatalf("labeled counter = %v, want 2", got)
		}
		if collisionCount() <= startCol {
			t.Fatalf("collision counter did not increment")
		}
	})

	t.Run("gauge_both_forms", func(t *testing.T) {
		startCol := collisionCount()
		reg := NewPrometheusRegistry("collide_g1")
		reg.Gauge("queue_depth").Set(5)
		reg.Gauge(Label("queue_depth", "name", "a")).Set(7) // would-be panic

		g := reg.promBridge.gaugeNoLabels("queue_depth")
		if got := testutil.ToFloat64(g); got != 5 {
			t.Fatalf("unlabeled gauge = %v, want 5", got)
		}
		if collisionCount() <= startCol {
			t.Fatalf("collision counter did not increment")
		}
	})

	t.Run("histogram_both_forms", func(t *testing.T) {
		startCol := collisionCount()
		reg := NewPrometheusRegistry("collide_h1")
		reg.promBridge.observeHistogram("api_seconds", 0.01)
		// Was a panic before the fix.
		reg.promBridge.observeHistogram(Label("api_seconds", "ep", "list"), 0.02)

		if _, ok := reg.promBridge.histogramsNoLabel.Load("api_seconds"); !ok {
			t.Fatal("unlabeled histogram missing")
		}
		if _, ok := reg.promBridge.histogramsVec.Load("api_seconds"); !ok {
			t.Fatal("labeled histogram missing (kept off-meter so same-shape calls are idempotent)")
		}
		if collisionCount() <= startCol {
			t.Fatalf("collision counter did not increment")
		}
	})

	t.Run("same_shape_repeated_no_collision", func(t *testing.T) {
		// Re-using the same registry name in the same shape must be a no-op,
		// not a collision. This guards against false-positive sentinel bumps.
		reg := NewPrometheusRegistry("collide_same1")
		reg.Incr("ok_metric")
		reg.Incr("ok_metric")
		startCol := collisionCount()
		reg.Incr("ok_metric") // would re-register without LoadOrStore guard
		if collisionCount() != startCol {
			t.Fatalf("same-shape repeat falsely counted as collision")
		}
	})
}

// collisionCount reads the sentinel counter; returns 0 if not yet initialized.
func collisionCount() float64 {
	if shapeCollisionCounter == nil {
		return 0
	}
	return testutil.ToFloat64(shapeCollisionCounter)
}
