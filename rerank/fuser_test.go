package rerank

import (
	"math"
	"testing"
)

// TestNewRRF_HappyPath: valid config returns a non-nil RankFuser, no error.
func TestNewRRF_HappyPath(t *testing.T) {
	f, err := NewRRF(60)
	if err != nil {
		t.Fatalf("NewRRF: unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("NewRRF: returned nil RankFuser")
	}
}

// TestNewRRF_NegativeK is the architect-flagged invalid config.
func TestNewRRF_NegativeK(t *testing.T) {
	if _, err := NewRRF(-1); err == nil {
		t.Fatal("NewRRF(-1): expected error, got nil")
	}
}

// TestNewRRF_KZeroAllowed: k=0 is documented to fall back to DefaultRRFK at
// Fuse time. The constructor must accept it.
func TestNewRRF_KZeroAllowed(t *testing.T) {
	if _, err := NewRRF(0); err != nil {
		t.Fatalf("NewRRF(0): expected nil error (k=0 → DefaultRRFK), got %v", err)
	}
}

// TestNewRRF_NegativeTopK rejects WithTopK(-1).
func TestNewRRF_NegativeTopK(t *testing.T) {
	if _, err := NewRRF(60, WithTopK(-1)); err == nil {
		t.Fatal("NewRRF(60, WithTopK(-1)): expected error, got nil")
	}
}

// TestNewRRF_FuseMatchesPackageLevel: NewRRF(k).Fuse(lists...) must be
// byte-equal to RRF(k, lists...) for the TopK=0 (uncapped) case.
func TestNewRRF_FuseMatchesPackageLevel(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c", "d"},
		{"b", "a", "e", "f"},
	}
	f, err := NewRRF(60)
	if err != nil {
		t.Fatalf("NewRRF: %v", err)
	}
	got := f.Fuse(lists...)
	want := RRF(60, lists...)

	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d ID: got %q want %q", i, got[i].ID, want[i].ID)
		}
		if math.Abs(got[i].Score-want[i].Score) > 1e-12 {
			t.Errorf("position %d score: got %v want %v", i, got[i].Score, want[i].Score)
		}
	}
}

// TestNewRRF_TopKCapsOutput: 8 unique IDs across two lists, WithTopK(3) must
// return exactly 3.
func TestNewRRF_TopKCapsOutput(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c", "d"},
		{"e", "f", "g", "h"},
	}
	f, err := NewRRF(60, WithTopK(3))
	if err != nil {
		t.Fatalf("NewRRF: %v", err)
	}
	got := f.Fuse(lists...)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (TopK cap)", len(got))
	}
}

// TestNewRRF_TopKZeroUncapped: WithTopK(0) is documented as uncapped; output
// length must match the package-level RRF function.
func TestNewRRF_TopKZeroUncapped(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c", "d"},
		{"e", "f", "g", "h"},
	}
	f, err := NewRRF(60, WithTopK(0))
	if err != nil {
		t.Fatalf("NewRRF: %v", err)
	}
	got := f.Fuse(lists...)
	want := RRF(60, lists...)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (TopK=0 should be uncapped)", len(got), len(want))
	}
}

// --- NewWeightedRRF ---

func TestNewWeightedRRF_HappyPath(t *testing.T) {
	f, err := NewWeightedRRF(60, []float64{1.0, 2.0})
	if err != nil {
		t.Fatalf("NewWeightedRRF: unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("NewWeightedRRF: returned nil RankFuser")
	}
}

func TestNewWeightedRRF_NegativeK(t *testing.T) {
	if _, err := NewWeightedRRF(-1, []float64{1.0}); err == nil {
		t.Fatal("NewWeightedRRF(-1): expected error, got nil")
	}
}

func TestNewWeightedRRF_EmptyWeights(t *testing.T) {
	if _, err := NewWeightedRRF(60, []float64{}); err == nil {
		t.Fatal("NewWeightedRRF([]): expected error, got nil")
	}
}

func TestNewWeightedRRF_NegativeWeight(t *testing.T) {
	if _, err := NewWeightedRRF(60, []float64{1.0, -0.5}); err == nil {
		t.Fatal("NewWeightedRRF(neg): expected error, got nil")
	}
}

func TestNewWeightedRRF_NegativeTopK(t *testing.T) {
	if _, err := NewWeightedRRF(60, []float64{1.0}, WithTopK(-2)); err == nil {
		t.Fatal("NewWeightedRRF(WithTopK(-2)): expected error, got nil")
	}
}

func TestNewWeightedRRF_FuseMatchesPackageLevel(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c"},
		{"b", "a", "d"},
	}
	weights := []float64{0.7, 0.3}
	f, err := NewWeightedRRF(60, weights)
	if err != nil {
		t.Fatalf("NewWeightedRRF: %v", err)
	}
	got := f.Fuse(lists...)
	want := WeightedRRF(60, weights, lists...)

	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d ID: got %q want %q", i, got[i].ID, want[i].ID)
		}
		if math.Abs(got[i].Score-want[i].Score) > 1e-12 {
			t.Errorf("position %d score: got %v want %v", i, got[i].Score, want[i].Score)
		}
	}
}

func TestNewWeightedRRF_TopKCapsOutput(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c", "d"},
		{"e", "f", "g", "h"},
	}
	f, err := NewWeightedRRF(60, []float64{1.0, 1.0}, WithTopK(3))
	if err != nil {
		t.Fatalf("NewWeightedRRF: %v", err)
	}
	got := f.Fuse(lists...)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (TopK cap)", len(got))
	}
}

// TestNewWeightedRRF_DefensiveCopy: mutating the weights slice after
// construction must not change runtime behavior.
func TestNewWeightedRRF_DefensiveCopy(t *testing.T) {
	weights := []float64{1.0, 1.0}
	f, err := NewWeightedRRF(60, weights)
	if err != nil {
		t.Fatalf("NewWeightedRRF: %v", err)
	}
	weights[0] = 999.0 // post-construction mutation

	lists := [][]string{{"a"}, {"a"}}
	got := f.Fuse(lists...)
	// With original weights [1.0, 1.0]: score(a) = 1/61 + 1/61 = 2/61.
	want := 2.0 / 61.0
	if math.Abs(got[0].Score-want) > 1e-12 {
		t.Errorf("score(a) = %v, want %v (defensive copy violated)", got[0].Score, want)
	}
}

// --- NewDBSF ---

func TestNewDBSF_HappyPath(t *testing.T) {
	f, err := NewDBSF()
	if err != nil {
		t.Fatalf("NewDBSF: unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("NewDBSF: returned nil ScoreFuser")
	}
}

func TestNewDBSF_NegativeTopK(t *testing.T) {
	if _, err := NewDBSF(WithTopK(-1)); err == nil {
		t.Fatal("NewDBSF(WithTopK(-1)): expected error, got nil")
	}
}

func TestNewDBSF_FuseMatchesPackageLevel(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 80},
		{ID: "c", Score: 50},
	}
	listB := ScoredIDList{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.7},
		{ID: "c", Score: 0.5},
	}
	f, err := NewDBSF()
	if err != nil {
		t.Fatalf("NewDBSF: %v", err)
	}
	got := f.Fuse(listA, listB)
	want := DBSF(listA, listB)

	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d ID: got %q want %q", i, got[i].ID, want[i].ID)
		}
		if math.Abs(got[i].Score-want[i].Score) > 1e-12 {
			t.Errorf("position %d score: got %v want %v", i, got[i].Score, want[i].Score)
		}
	}
}

func TestNewDBSF_TopKCapsOutput(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 80},
		{ID: "c", Score: 50},
		{ID: "d", Score: 30},
	}
	listB := ScoredIDList{
		{ID: "e", Score: 0.9},
		{ID: "f", Score: 0.7},
		{ID: "g", Score: 0.5},
		{ID: "h", Score: 0.3},
	}
	f, err := NewDBSF(WithTopK(3))
	if err != nil {
		t.Fatalf("NewDBSF: %v", err)
	}
	got := f.Fuse(listA, listB)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (TopK cap)", len(got))
	}
}

// --- NewLinearMinMax ---

func TestNewLinearMinMax_HappyPath(t *testing.T) {
	f, err := NewLinearMinMax([]float64{1.0, 2.0})
	if err != nil {
		t.Fatalf("NewLinearMinMax: unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("NewLinearMinMax: returned nil ScoreFuser")
	}
}

func TestNewLinearMinMax_EmptyWeights(t *testing.T) {
	if _, err := NewLinearMinMax([]float64{}); err == nil {
		t.Fatal("NewLinearMinMax([]): expected error, got nil")
	}
}

func TestNewLinearMinMax_NegativeWeight(t *testing.T) {
	if _, err := NewLinearMinMax([]float64{1.0, -0.5}); err == nil {
		t.Fatal("NewLinearMinMax(neg): expected error, got nil")
	}
}

func TestNewLinearMinMax_NegativeTopK(t *testing.T) {
	if _, err := NewLinearMinMax([]float64{1.0}, WithTopK(-3)); err == nil {
		t.Fatal("NewLinearMinMax(WithTopK(-3)): expected error, got nil")
	}
}

func TestNewLinearMinMax_FuseMatchesPackageLevel(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 80},
	}
	listB := ScoredIDList{
		{ID: "a", Score: 0.5},
		{ID: "b", Score: 1.0},
	}
	weights := []float64{1.0, 2.0}
	f, err := NewLinearMinMax(weights)
	if err != nil {
		t.Fatalf("NewLinearMinMax: %v", err)
	}
	got := f.Fuse(listA, listB)
	want := LinearMinMax(weights, listA, listB)

	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d ID: got %q want %q", i, got[i].ID, want[i].ID)
		}
		if math.Abs(got[i].Score-want[i].Score) > 1e-12 {
			t.Errorf("position %d score: got %v want %v", i, got[i].Score, want[i].Score)
		}
	}
}

func TestNewLinearMinMax_TopKCapsOutput(t *testing.T) {
	listA := ScoredIDList{
		{ID: "a", Score: 100},
		{ID: "b", Score: 80},
		{ID: "c", Score: 50},
		{ID: "d", Score: 30},
	}
	listB := ScoredIDList{
		{ID: "e", Score: 0.9},
		{ID: "f", Score: 0.7},
		{ID: "g", Score: 0.5},
		{ID: "h", Score: 0.3},
	}
	f, err := NewLinearMinMax([]float64{1.0, 1.0}, WithTopK(3))
	if err != nil {
		t.Fatalf("NewLinearMinMax: %v", err)
	}
	got := f.Fuse(listA, listB)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (TopK cap)", len(got))
	}
}

func TestNewLinearMinMax_DefensiveCopy(t *testing.T) {
	weights := []float64{1.0, 2.0}
	f, err := NewLinearMinMax(weights)
	if err != nil {
		t.Fatalf("NewLinearMinMax: %v", err)
	}
	weights[0] = 999.0 // post-construction mutation

	listA := ScoredIDList{{ID: "a", Score: 10}, {ID: "b", Score: 0}}
	listB := ScoredIDList{{ID: "a", Score: 1}, {ID: "b", Score: 0}}
	got := f.Fuse(listA, listB)
	// Original weights [1.0, 2.0]: a = 1.0*1.0 + 2.0*1.0 = 3.0, b = 0.
	wantTop := 3.0
	if math.Abs(got[0].Score-wantTop) > 1e-12 {
		t.Errorf("score(top) = %v, want %v (defensive copy violated)", got[0].Score, wantTop)
	}
}
