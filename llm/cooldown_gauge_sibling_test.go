package llm

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestCooldownGauge_LapsedSiblingWindowDoesNotClearTheModel guards the OTHER
// recovery path of issue #313: the TTL sweep in cooling(). Two dead-model
// windows for the same model start at different times, so proxy A lapses while
// proxy B is still cooled. Sweeping A must not fire the model-level recovery
// edge — the gauge is labelled by model, and the model is still not serving.
//
// Internal test: the injectable clock is the only way to lapse one window while
// the other is still open, and it stays wired to the production
// ChainMetrics.CooldownObserver so the assertion is on the real gauge.
func TestCooldownGauge_LapsedSiblingWindowDoesNotClearTheModel(t *testing.T) {
	cm := NewChainMetrics(prometheus.NewRegistry())

	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }
	mc.onChange = cm.CooldownObserver()

	epA := Endpoint{URL: "http://proxy-a/v1", Model: "m"}
	epB := Endpoint{URL: "http://proxy-b/v1", Model: "m"}
	mc.recordFailure(deadCooldownKey(epA), 0) // window [0s, 60s)
	now = now.Add(30 * time.Second)
	mc.recordFailure(deadCooldownKey(epB), 0) // window [30s, 90s)

	gauge := cm.CooldownActive().WithLabelValues("m")
	waitGauge(t, gauge, 1, "both endpoints cooled")

	// t=61s: A's window lapsed, B's is still open.
	now = now.Add(31 * time.Second)
	if mc.cooling(deadCooldownKey(epA)) {
		t.Fatal("proxy A must not read as cooling after its window lapsed")
	}
	// Poll instead of sleeping once: the observer runs on its own goroutine, so a
	// recovery edge that lands late has to be caught too, not just one that lands
	// within a fixed delay.
	settled := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(settled) {
		if got := testutil.ToFloat64(gauge); got != 1 {
			t.Fatalf("gauge = %v after proxy A lapsed while proxy B is still cooled, want 1", got)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// t=91s: B lapses too — the last live window, so now the gauge clears.
	now = now.Add(30 * time.Second)
	if mc.cooling(deadCooldownKey(epB)) {
		t.Fatal("proxy B must not read as cooling after its window lapsed")
	}
	waitGauge(t, gauge, 0, "last endpoint recovered")
}
