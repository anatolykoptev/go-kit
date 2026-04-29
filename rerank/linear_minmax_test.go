package rerank

import (
	"math"
	"testing"
)

// TestLinearMinMax_HandComputed locks in the exact MinMax formula on a
// two-list weighted input. All numbers verified by hand.
func TestLinearMinMax_HandComputed(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 80},
		{ID: "c", Score: 60},
	}
	listB := ScoredIDList{
		{ID: "a", Score: 0.5},
		{ID: "b", Score: 1.0},
		{ID: "d", Score: 0.0},
	}
	// Per-list MinMax:
	//   listA: min=60, max=100, span=40 → a=1.0, b=0.5, c=0.0
	//   listB: min=0,  max=1,   span=1  → a=0.5, b=1.0, d=0.0
	// Weights = [1.0, 2.0]:
	//   a = 1.0*1.0 + 2.0*0.5 = 1.0 + 1.0 = 2.0
	//   b = 1.0*0.5 + 2.0*1.0 = 0.5 + 2.0 = 2.5
	//   c = 1.0*0.0          = 0.0
	//   d =          2.0*0.0 = 0.0
	// Order: b > a > c == d (c first-seen before d → c, d).
	got := LinearMinMax([]float64{1.0, 2.0}, listA, listB)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}

	wantOrder := []string{"b", "a", "c", "d"}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Errorf("position %d: ID = %q, want %q (full: %+v)", i, got[i].ID, id, got)
		}
	}

	wantScores := map[string]float64{
		"a": 2.0,
		"b": 2.5,
		"c": 0.0,
		"d": 0.0,
	}
	for _, f := range got {
		if math.Abs(f.Score-wantScores[f.ID]) > 1e-12 {
			t.Errorf("score(%s) = %v, want %v", f.ID, f.Score, wantScores[f.ID])
		}
	}
}

// TestLinearMinMax_WeightZeroSkips asserts that weight=0 produces zero
// contribution from a list — even one whose top entry would otherwise win.
func TestLinearMinMax_WeightZeroSkips(t *testing.T) {
	listA := ScoredIDList{{ID: "winner", Score: 100}}
	listB := ScoredIDList{{ID: "irrelevant", Score: 999}}

	got := LinearMinMax([]float64{1.0, 0.0}, listA, listB)

	// listA: span=0 → winner gets 0.5 * w=1.0 = 0.5.
	// listB: w=0 → not processed at all → "irrelevant" never enters output.
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (weight=0 must skip its list entirely): %+v", len(got), got)
	}
	if got[0].ID != "winner" {
		t.Errorf("top ID = %q, want %q", got[0].ID, "winner")
	}
	if math.Abs(got[0].Score-linearMinMaxFlatScore) > 1e-12 {
		t.Errorf("score(winner) = %v, want %v", got[0].Score, linearMinMaxFlatScore)
	}
}

// TestLinearMinMax_NegativeWeight asserts negative weights act as a penalty
// (push items down) without corrupting other contributions.
func TestLinearMinMax_NegativeWeight(t *testing.T) {
	listA := ScoredIDList{
		{ID: "good", Score: 10},
		{ID: "bad", Score: 0},
	}
	listB := ScoredIDList{
		{ID: "good", Score: 0},  // good penalized by listB
		{ID: "bad", Score: 10},
	}
	// Per-list MinMax: span=10 in both.
	//   listA normalized: good=1.0, bad=0.0
	//   listB normalized: good=0.0, bad=1.0
	// Weights = [+1, -1]:
	//   good = +1*1.0 + (-1)*0.0 = +1.0
	//   bad  = +1*0.0 + (-1)*1.0 = -1.0
	got := LinearMinMax([]float64{1.0, -1.0}, listA, listB)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "good" || got[1].ID != "bad" {
		t.Errorf("order: got %+v, want [good, bad]", got)
	}
	if math.Abs(got[0].Score-1.0) > 1e-12 {
		t.Errorf("score(good) = %v, want 1.0", got[0].Score)
	}
	if math.Abs(got[1].Score-(-1.0)) > 1e-12 {
		t.Errorf("score(bad) = %v, want -1.0", got[1].Score)
	}
}

// TestLinearMinMax_MaxEqMinFlat covers the degenerate per-list path: all
// identical scores or a single item. Each ID gets the flat 0.5 contribution
// (weighted).
func TestLinearMinMax_MaxEqMinFlat(t *testing.T) {
	tests := []struct {
		name string
		list ScoredIDList
	}{
		{"single item", ScoredIDList{{ID: "x", Score: 7}}},
		{"all identical", ScoredIDList{
			{ID: "a", Score: 5},
			{ID: "b", Score: 5},
			{ID: "c", Score: 5},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinearMinMax([]float64{2.0}, tt.list)
			if len(got) != len(tt.list) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.list))
			}
			want := 2.0 * linearMinMaxFlatScore
			for _, f := range got {
				if math.Abs(f.Score-want) > 1e-12 {
					t.Errorf("score(%q) = %v, want %v", f.ID, f.Score, want)
				}
			}
		})
	}
}

// TestLinearMinMax_LengthMismatchPanics asserts the explicit programmer-error
// guard.
func TestLinearMinMax_LengthMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on len(weights) != len(lists), got none")
		}
	}()
	LinearMinMax([]float64{1.0}, ScoredIDList{{ID: "a", Score: 1}}, ScoredIDList{{ID: "b", Score: 2}})
}

// TestLinearMinMax_EmptyInputs covers no-list and empty-list inputs.
func TestLinearMinMax_EmptyInputs(t *testing.T) {
	if got := LinearMinMax([]float64{}); len(got) != 0 {
		t.Errorf("LinearMinMax() = %+v, want empty", got)
	}
	if got := LinearMinMax([]float64{1.0}, ScoredIDList{}); len(got) != 0 {
		t.Errorf("LinearMinMax(empty) = %+v, want empty", got)
	}
}

// TestLinearMinMax_DuplicateIDsBestFirst mirrors the RRF/DBSF first-occurrence
// rule for within-list duplicates.
func TestLinearMinMax_DuplicateIDsBestFirst(t *testing.T) {
	// "a" appears twice; only the first (score=10) counts.
	list := ScoredIDList{
		{ID: "a", Score: 10},
		{ID: "b", Score: 0},
		{ID: "a", Score: -1000}, // ignored
	}
	got := LinearMinMax([]float64{1.0}, list)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (deduped): %+v", len(got), got)
	}
	// After dedup: {a:10, b:0} → span=10 → a=1.0, b=0.0.
	if got[0].ID != "a" || math.Abs(got[0].Score-1.0) > 1e-12 {
		t.Errorf("top = %+v, want a=1.0", got[0])
	}
	if got[1].ID != "b" || math.Abs(got[1].Score-0.0) > 1e-12 {
		t.Errorf("bottom = %+v, want b=0.0", got[1])
	}
}

// TestLinearMinMax_RangeBound asserts the post-normalization invariant: with
// non-negative weights summing to W and no degenerate lists, every fused
// score lies in [0, W].
func TestLinearMinMax_RangeBound(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 50},
		{ID: "c", Score: 1},
	}
	listB := ScoredIDList{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.1},
		{ID: "d", Score: 0.5},
	}
	weights := []float64{0.7, 0.3}
	totalW := weights[0] + weights[1]

	got := LinearMinMax(weights, listA, listB)
	for _, f := range got {
		if f.Score < -1e-12 || f.Score > totalW+1e-12 {
			t.Errorf("score(%s) = %v, expected in [0, %v]", f.ID, f.Score, totalW)
		}
	}
}
