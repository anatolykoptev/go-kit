package llm

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestModelCooldown_TTLRecovery_FiresEdgeWithoutSuccess is the regression guard
// for the MAJOR cooldown-gauge-stuck-at-1 bug (P2 review finding):
//
//	A cooled non-primary model is SKIPPED by executeInner while a healthier
//	candidate remains (cooldownCandidates → skipCooled=true), so it never sees a
//	200 and recordSuccess never fires for it. Pre-fix, recovery was driven ONLY
//	by recordSuccess, so once the TTL lapsed the until entry leaked in the map AND
//	the recovery observer edge (cooling=false) never fired — the
//	llm_model_cooldown_active gauge stayed pinned at 1 for hours after the quota
//	actually recovered.
//
// Post-fix invariant: the FIRST cooling() read after the window expires is the
// recovery authority for the skipped-model path. It (a) returns false, (b)
// deletes the lapsed until entry, and (c) fires the recovery edge (cooling=false)
// — all WITHOUT any recordSuccess. This is the mirror of recordSuccess for the
// attempted-model path.
func TestModelCooldown_TTLRecovery_FiresEdgeWithoutSuccess(t *testing.T) {
	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }

	var entries, recoveries atomic.Int64
	mc.onChange = func(_ string, cooling bool, _ time.Duration) {
		if cooling {
			entries.Add(1)
			return
		}
		recoveries.Add(1)
	}

	// Cool the model (FailThreshold=1) → entry edge. NO recordSuccess anywhere in
	// this test: recovery must come purely from the TTL lapse seen by cooling().
	mc.recordFailure("m", 0)
	if got := waitCount(&entries, 1); got != 1 {
		t.Fatalf("after recordFailure: entry edges = %d, want 1", got)
	}
	if !mc.cooling("m") {
		t.Fatal("model should be cooling immediately after recordFailure")
	}
	// No recovery yet — window is still open.
	if got := recoveries.Load(); got != 0 {
		t.Fatalf("recovery edge fired while window still open: recoveries = %d, want 0", got)
	}

	// Advance past the 60s TTL. The next cooling() read drives recovery.
	now = now.Add(61 * time.Second)

	if mc.cooling("m") {
		t.Fatal("model should NOT be cooling after the TTL lapsed at t+61s")
	}
	// THE assertion: the recovery edge fired from the TTL lapse alone — no 200,
	// no recordSuccess. Pre-fix this stayed 0 and the gauge stuck at 1.
	if got := waitCount(&recoveries, 1); got != 1 {
		t.Fatalf("TTL lapse did not fire the recovery edge: recoveries = %d, want 1 "+
			"(gauge would stick at 1)", got)
	}

	// State must be cleaned: the lapsed until entry is gone, so a subsequent read
	// is a plain miss (no double recovery edge).
	if _, present := peekUntil(mc, "m"); present {
		t.Error("until[m] should be deleted after TTL-driven recovery")
	}
	if mc.cooling("m") {
		t.Fatal("second cooling() read after cleanup must still be false")
	}
	// A second read must NOT fire another recovery edge (idempotent cleanup).
	if got := recoveries.Load(); got != 1 {
		t.Errorf("recovery edge fired more than once: recoveries = %d, want exactly 1", got)
	}
}

// TestModelCooldown_TTLRecovery_ReCoolBeforeRead: if a fresh quota burst re-cools
// the model BEFORE any cooling() read sees the lapse, cooling() observes the new
// (active) window under the write-lock re-check and reports true — it must NOT
// clean the fresh window nor fire a spurious recovery edge. Guards the lock-upgrade
// re-check path.
func TestModelCooldown_TTLRecovery_ReCoolBeforeRead(t *testing.T) {
	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }

	var recoveries atomic.Int64
	mc.onChange = func(_ string, cooling bool, _ time.Duration) {
		if !cooling {
			recoveries.Add(1)
		}
	}

	mc.recordFailure("m", 0) // window 1: [0, 60s)
	now = now.Add(61 * time.Second)
	mc.recordFailure("m", 0) // window 2: [61s, 121s) — re-cooled before any read

	if !mc.cooling("m") {
		t.Fatal("model re-cooled into a fresh window must read as cooling")
	}
	// Give any (erroneous) async recovery edge a chance to land, then assert none.
	time.Sleep(20 * time.Millisecond)
	if got := recoveries.Load(); got != 0 {
		t.Errorf("no recovery edge should fire for a model re-cooled into a fresh "+
			"window: recoveries = %d, want 0", got)
	}
}

// TestModelCooldown_TTLRecovery_DrivesGaugeToZero is the end-to-end MAJOR guard
// through the REAL public observer: a cooled-then-skipped model that rides its
// window out silently must drive llm_model_cooldown_active back to 0 via the
// TTL-lapse recovery edge alone (no recordSuccess). Pre-fix the gauge stuck at 1.
//
// Internal test so it can advance the injectable clock deterministically while
// wiring the production ChainMetrics.CooldownObserver().
func TestModelCooldown_TTLRecovery_DrivesGaugeToZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	cm := NewChainMetrics(reg)

	now := time.Unix(0, 0)
	mc := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: 60 * time.Second})
	mc.clock = func() time.Time { return now }
	mc.onChange = cm.CooldownObserver()

	gauge := cm.CooldownActive().WithLabelValues("m")

	mc.recordFailure("m", 0) // cool → entry edge → gauge 1 (async)
	waitGauge(t, gauge, 1, "TTL-recovery entry")

	now = now.Add(61 * time.Second) // lapse the window
	if mc.cooling("m") {
		t.Fatal("model should not be cooling after TTL lapse")
	}
	// THE assertion: the TTL lapse seen by cooling() drove the gauge to 0 with no
	// recordSuccess — pre-fix it stayed pinned at 1.
	waitGauge(t, gauge, 0, "TTL-recovery edge")
}

// waitGauge polls a prometheus gauge (set from an async observer goroutine) until
// it reads want or a short bound elapses.
func waitGauge(t *testing.T, g prometheus.Gauge, want float64, phase string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for testutil.ToFloat64(g) != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s: gauge never reached %v, got %v", phase, want, testutil.ToFloat64(g))
		}
		time.Sleep(time.Millisecond)
	}
}

// waitCount polls a counter mutated from an async observer goroutine until it
// reaches want or a short bound elapses, then returns the observed value.
func waitCount(n *atomic.Int64, want int64) int64 {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v := n.Load(); v >= want {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	return n.Load()
}

// peekUntil reads the until entry for model under the lock (test-only helper).
func peekUntil(mc *modelCooldown, model string) (time.Time, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	t, ok := mc.until[model]
	return t, ok
}
