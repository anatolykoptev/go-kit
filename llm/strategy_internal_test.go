package llm

import (
	"math/rand"
	"testing"
	"time"
)

// TestEligibleEndpoints_GuardA verifies that eligibleEndpoints (Guard A of the
// cooled-model exclusion invariant) filters out cooled models and retains healthy
// ones. This test is a DIRECT unit test of the filtering function; it goes RED
// when Guard A is deleted or simplified to return all endpoints unchanged.
//
// Mutation proof for Guard A:
//   - Delete eligibleEndpoints body (return all endpoints): out has len=2, cooled
//     model is included → assertion "quota-model absent" fails. RED.
//   - Guard B (transport.go loop-level cooling() check) is NOT in scope here;
//     this test targets the filtering step only.
func TestEligibleEndpoints_GuardA(t *testing.T) {
	cd := newModelCooldown(CooldownConfig{
		FailThreshold: 1,
		Default:       time.Hour,
	})
	// Cool quota-model immediately (one failure, threshold=1).
	cd.recordFailure("quota-model", 0)

	if !cd.cooling("quota-model") {
		t.Fatal("precondition: quota-model must be in cooldown before testing eligibleEndpoints")
	}

	all := []Endpoint{
		{Model: "quota-model"},
		{Model: "healthy-model"},
	}
	got := eligibleEndpoints(all, cd)

	// Exactly one endpoint should survive: the healthy one.
	if len(got) != 1 {
		t.Errorf("eligibleEndpoints: want 1 eligible endpoint, got %d: %v", len(got), got)
	}
	for _, ep := range got {
		if ep.Model == "quota-model" {
			t.Errorf("eligibleEndpoints: cooled quota-model must not appear in result; got=%v", got)
		}
	}
	if len(got) == 1 && got[0].Model != "healthy-model" {
		t.Errorf("eligibleEndpoints: surviving endpoint must be healthy-model, got %q", got[0].Model)
	}
}

// TestEligibleEndpoints_AllCooled verifies that when ALL endpoints are cooled,
// eligibleEndpoints returns an empty slice (the cooldownCandidates() → endpoints[:1]
// path handles the never-fail-closed case; eligibleEndpoints is not called on that path).
func TestEligibleEndpoints_AllCooled(t *testing.T) {
	cd := newModelCooldown(CooldownConfig{
		FailThreshold: 1,
		Default:       time.Hour,
	})
	cd.recordFailure("a", 0)
	cd.recordFailure("b", 0)

	all := []Endpoint{{Model: "a"}, {Model: "b"}}
	got := eligibleEndpoints(all, cd)
	if len(got) != 0 {
		t.Errorf("eligibleEndpoints all-cooled: want empty, got %v", got)
	}
}

// TestEligibleEndpoints_NoCooled verifies that when no endpoints are cooled,
// eligibleEndpoints returns all of them unchanged.
func TestEligibleEndpoints_NoCooled(t *testing.T) {
	cd := newModelCooldown(CooldownConfig{FailThreshold: 1, Default: time.Hour})
	all := []Endpoint{{Model: "a"}, {Model: "b"}, {Model: "c"}}
	got := eligibleEndpoints(all, cd)
	if len(got) != 3 {
		t.Errorf("eligibleEndpoints none-cooled: want 3, got %d: %v", len(got), got)
	}
}

// TestWeightedShuffleEndpoints_Unit is an internal unit test for
// weightedShuffleEndpoints covering: exclusion of weight-0 models,
// correct output length, proportional ordering (m1 at pos 0 more than m2),
// and no mutation of the input slice.
func TestWeightedShuffleEndpoints_Unit(t *testing.T) {
	eps := []Endpoint{
		{Model: "m1"},
		{Model: "m2"},
		{Model: "m3-zero"},
	}
	weights := map[string]int{
		"m1":      4,
		"m2":      1,
		"m3-zero": 0,
	}
	rng := rand.New(rand.NewSource(42))

	firstPos := make(map[string]int)
	for range 200 {
		out := weightedShuffleEndpoints(eps, weights, rng)

		// m3-zero must never appear.
		for _, ep := range out {
			if ep.Model == "m3-zero" {
				t.Errorf("weight-0 model m3-zero appeared in output: %v", out)
				return
			}
		}

		// Output must have exactly 2 elements (m1 + m2).
		if len(out) != 2 {
			t.Errorf("expected len=2 (m1+m2), got len=%d: %v", len(out), out)
			return
		}

		if len(out) > 0 {
			firstPos[out[0].Model]++
		}
	}

	// m1 (weight=4) should appear at position 0 more often than m2 (weight=1).
	if firstPos["m1"] < firstPos["m2"]*2 {
		t.Errorf("m1 (weight=4) should appear at pos 0 at least 2x more than m2 (weight=1): m1=%d m2=%d", firstPos["m1"], firstPos["m2"])
	}

	// Input slice must NOT be mutated.
	if len(eps) != 3 {
		t.Errorf("input slice was mutated: len=%d, want 3", len(eps))
	}
	if eps[0].Model != "m1" || eps[1].Model != "m2" || eps[2].Model != "m3-zero" {
		t.Errorf("input slice order was mutated: %v", eps)
	}
}
