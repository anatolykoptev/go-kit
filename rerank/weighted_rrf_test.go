package rerank

import (
	"math"
	"testing"
)

// TestWeightedRRF_AllOnesEqualsRRF asserts the canonical invariant: with all
// weights == 1.0, WeightedRRF must produce identical scores and order as plain
// RRF for the same inputs. This is the migration safety net.
func TestWeightedRRF_AllOnesEqualsRRF(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c", "d"},
		{"b", "a", "e", "f"},
		{"a", "c", "g", "h"},
	}
	weights := []float64{1.0, 1.0, 1.0}

	wantRRF := RRF(DefaultRRFK, lists...)
	got := WeightedRRF(DefaultRRFK, weights, lists...)

	if len(got) != len(wantRRF) {
		t.Fatalf("len: got %d want %d", len(got), len(wantRRF))
	}
	for i := range got {
		if got[i].ID != wantRRF[i].ID {
			t.Errorf("position %d ID: got %q want %q", i, got[i].ID, wantRRF[i].ID)
		}
		if math.Abs(got[i].Score-wantRRF[i].Score) > 1e-12 {
			t.Errorf("position %d score: got %v want %v", i, got[i].Score, wantRRF[i].Score)
		}
	}
}

// TestWeightedRRF_TableDriven covers the weighted-fusion formula with hand-
// computed expected scores: score(d) = sum w_i / (k + rank).
func TestWeightedRRF_TableDriven(t *testing.T) {
	const k = 60
	tests := []struct {
		name      string
		k         int
		weights   []float64
		lists     [][]string
		wantOrder []string
	}{
		{
			name:      "empty",
			k:         k,
			weights:   []float64{},
			lists:     [][]string{},
			wantOrder: nil,
		},
		{
			name:    "single list, weight 1.0 — pass-through",
			k:       k,
			weights: []float64{1.0},
			lists:   [][]string{{"x", "y", "z"}},
			// score(x) = 1/61, score(y) = 1/62, score(z) = 1/63 → x > y > z.
			wantOrder: []string{"x", "y", "z"},
		},
		{
			name:    "weight=0 skips list entirely",
			k:       k,
			weights: []float64{1.0, 0.0},
			lists: [][]string{
				{"a", "b"},
				{"z", "y"}, // contributes nothing
			},
			// Only first list counts: a > b. z, y absent from output.
			wantOrder: []string{"a", "b"},
		},
		{
			name:    "asymmetric weights swap winner",
			k:       k,
			weights: []float64{0.1, 1.0},
			lists: [][]string{
				{"a", "b"}, // a at rank 1, contribution 0.1/61
				{"b", "a"}, // b at rank 1, contribution 1.0/61
			},
			// score(a) = 0.1/61 + 1.0/62 ≈ 0.001639 + 0.016129 = 0.017769
			// score(b) = 0.1/62 + 1.0/61 ≈ 0.001613 + 0.016393 = 0.018006
			// b wins despite a being at rank 1 in list 0 (low-weight list).
			wantOrder: []string{"b", "a"},
		},
		{
			name:    "negative weight pushes id down",
			k:       k,
			weights: []float64{1.0, -1.0},
			lists: [][]string{
				{"a", "b"},
				{"a", "b"}, // identical list with negative weight cancels
			},
			// score(a) = 1/61 - 1/61 = 0
			// score(b) = 1/62 - 1/62 = 0
			// Tied at 0 → stable first-seen: a, b.
			wantOrder: []string{"a", "b"},
		},
		{
			name:    "k=0 falls back to default",
			k:       0,
			weights: []float64{1.0, 1.0},
			lists: [][]string{
				{"x", "y"},
				{"y", "x"},
			},
			// With k=60: x = 1/61+1/62, y = 1/62+1/61 → tied, stable first-seen.
			wantOrder: []string{"x", "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeightedRRF(tt.k, tt.weights, tt.lists...)
			if len(got) != len(tt.wantOrder) {
				t.Fatalf("len = %d, want %d (got = %+v)", len(got), len(tt.wantOrder), got)
			}
			for i, id := range tt.wantOrder {
				if got[i].ID != id {
					t.Errorf("position %d: ID = %q, want %q (full got = %+v)", i, got[i].ID, id, got)
				}
			}
		})
	}
}

// TestWeightedRRF_ScoreFormula verifies the exact numerical score for an
// asymmetric-weight input. Catches w_i factor placement bugs.
func TestWeightedRRF_ScoreFormula(t *testing.T) {
	const k = 60
	// "a" at rank 1 in list 0 (weight 2.0) and rank 1 in list 1 (weight 0.5).
	// score(a) = 2.0/(60+1) + 0.5/(60+1) = 2.5/61.
	got := WeightedRRF(k, []float64{2.0, 0.5}, []string{"a"}, []string{"a"})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := 2.5 / 61.0
	if math.Abs(got[0].Score-want) > 1e-12 {
		t.Errorf("score(a) = %v, want %v", got[0].Score, want)
	}
}

// TestWeightedRRF_LengthMismatchPanics asserts the explicit programmer-error
// guard: len(weights) != len(lists) must panic, not silently degrade.
func TestWeightedRRF_LengthMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on len(weights) != len(lists), got none")
		}
	}()
	WeightedRRF(DefaultRRFK, []float64{1.0}, []string{"a"}, []string{"b"})
}

// TestWeightedRRF_LengthMismatchPanics_ZeroLists covers the other direction:
// non-empty weights with zero lists must also panic.
func TestWeightedRRF_LengthMismatchPanics_ZeroLists(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on weights with no lists, got none")
		}
	}()
	WeightedRRF(DefaultRRFK, []float64{1.0})
}

// TestWeightedRRF_DuplicateIDsBestRankOnly mirrors the RRF duplicate-handling
// invariant: a duplicate ID in a single list contributes only its best rank.
func TestWeightedRRF_DuplicateIDsBestRankOnly(t *testing.T) {
	const k = 60
	got := WeightedRRF(k, []float64{1.0}, []string{"a", "b", "a"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// score(a) = 1/61 (rank 1 only)
	// score(b) = 1/62
	if math.Abs(got[0].Score-1.0/61.0) > 1e-12 {
		t.Errorf("score(a) = %v, want %v", got[0].Score, 1.0/61.0)
	}
	if math.Abs(got[1].Score-1.0/62.0) > 1e-12 {
		t.Errorf("score(b) = %v, want %v", got[1].Score, 1.0/62.0)
	}
}
