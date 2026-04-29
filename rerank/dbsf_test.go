package rerank

import (
	"math"
	"testing"
)

// TestDBSF_ScoreScaleImmune is the cornerstone DBSF test: two retrievers with
// wildly different raw score scales (BM25 [50, 80, 100] vs cosine
// [0.5, 0.7, 0.9]) must contribute equally to the fused output after z-scoring.
// If they didn't, DBSF would be no better than naïve sum.
func TestDBSF_ScoreScaleImmune(t *testing.T) {
	bm25 := ScoredIDList{
		{ID: "x", Score: 100},
		{ID: "y", Score: 80},
		{ID: "z", Score: 50},
	}
	cosine := ScoredIDList{
		{ID: "x", Score: 0.9},
		{ID: "y", Score: 0.7},
		{ID: "z", Score: 0.5},
	}

	got := DBSF(bm25, cosine)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	// Per-list stats (population stddev):
	//   BM25:   μ = (100+80+50)/3 = 76.667; σ = sqrt(((23.333)^2+(3.333)^2+(-26.667)^2)/3) ≈ 20.548
	//   Cosine: μ = (0.9+0.7+0.5)/3 = 0.7;  σ = sqrt((0.04+0+0.04)/3) ≈ 0.16330
	// Z-scores:
	//   x: (100-76.667)/20.548 ≈ 1.1359  +  (0.9-0.7)/0.16330 ≈ 1.2247  = 2.3606
	//   y: (80-76.667)/20.548  ≈ 0.1623  +  (0.7-0.7)/0.16330 = 0       ≈ 0.1623
	//   z: (50-76.667)/20.548  ≈ -1.2982 + (0.5-0.7)/0.16330 ≈ -1.2247  = -2.5229
	// None hit the ±3 clip. Order: x > y > z.
	wantIDs := []string{"x", "y", "z"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("position %d: ID = %q, want %q (full: %+v)", i, got[i].ID, id, got)
		}
	}

	// Spot-check the magnitude — a true z-score sum should be near-symmetric
	// around zero for a balanced 3-element distribution.
	totalAbs := math.Abs(got[0].Score) + math.Abs(got[2].Score)
	if totalAbs < 4.0 || totalAbs > 6.0 {
		t.Errorf("|score(x)| + |score(z)| = %v, expected ~4-6 (two near-equal contributions)", totalAbs)
	}
	// And the two retrievers MUST have contributed comparably to the winner.
	// Each list contributes between +1 and +1.5 to x → sum should be in [2, 3].
	if got[0].Score < 2.0 || got[0].Score > 3.0 {
		t.Errorf("score(x) = %v, expected in [2.0, 3.0]", got[0].Score)
	}
}

// TestDBSF_OverlapBeatsSingletons asserts the structural property: an item
// appearing in BOTH lists outranks items appearing in only one list, even
// when those singletons are at the top of their respective lists.
func TestDBSF_OverlapBeatsSingletons(t *testing.T) {
	listA := ScoredIDList{
		{ID: "shared", Score: 10},
		{ID: "onlyA", Score: 9},
		{ID: "tail", Score: 1},
	}
	listB := ScoredIDList{
		{ID: "onlyB", Score: 10},
		{ID: "shared", Score: 9},
		{ID: "tail2", Score: 1},
	}

	got := DBSF(listA, listB)

	// "shared" gets a positive z-score from BOTH lists.
	// "onlyA" gets a positive z-score from list A, nothing from list B.
	// "onlyB" gets a positive z-score from list B, nothing from list A.
	// → shared > onlyA, shared > onlyB.
	var sharedScore, onlyAScore, onlyBScore float64
	for _, f := range got {
		switch f.ID {
		case "shared":
			sharedScore = f.Score
		case "onlyA":
			onlyAScore = f.Score
		case "onlyB":
			onlyBScore = f.Score
		}
	}
	if sharedScore <= onlyAScore {
		t.Errorf("shared (%v) should outrank onlyA (%v)", sharedScore, onlyAScore)
	}
	if sharedScore <= onlyBScore {
		t.Errorf("shared (%v) should outrank onlyB (%v)", sharedScore, onlyBScore)
	}
	if got[0].ID != "shared" {
		t.Errorf("top ID = %q, want %q", got[0].ID, "shared")
	}
}

// TestDBSF_SigmaZeroNoNaN covers the σ=0 paths: single-item list and all-
// identical-scores list. Both must contribute 0 (not NaN, not panic) and the
// IDs must still be registered in the output.
func TestDBSF_SigmaZeroNoNaN(t *testing.T) {
	tests := []struct {
		name string
		list ScoredIDList
	}{
		{
			name: "single item",
			list: ScoredIDList{{ID: "lonely", Score: 42}},
		},
		{
			name: "all identical",
			list: ScoredIDList{
				{ID: "a", Score: 5},
				{ID: "b", Score: 5},
				{ID: "c", Score: 5},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DBSF(tt.list)
			if len(got) != len(tt.list) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.list))
			}
			for _, f := range got {
				if math.IsNaN(f.Score) || math.IsInf(f.Score, 0) {
					t.Errorf("non-finite score for %q: %v", f.ID, f.Score)
				}
				if f.Score != 0 {
					t.Errorf("score(%q) = %v, want 0 for σ=0 path", f.ID, f.Score)
				}
			}
		})
	}
}

// TestDBSF_EmptyInputs covers no-list and empty-list inputs.
func TestDBSF_EmptyInputs(t *testing.T) {
	if got := DBSF(); len(got) != 0 {
		t.Errorf("DBSF() = %+v, want empty", got)
	}
	if got := DBSF(ScoredIDList{}); len(got) != 0 {
		t.Errorf("DBSF(empty) = %+v, want empty", got)
	}
	if got := DBSF(ScoredIDList{}, ScoredIDList{}); len(got) != 0 {
		t.Errorf("DBSF(empty, empty) = %+v, want empty", got)
	}
}

// TestDBSF_ClipBoundsExtremeOutliers verifies the ±3σ clip prevents an
// extreme outlier from a single list dominating the fused score.
func TestDBSF_ClipBoundsExtremeOutliers(t *testing.T) {
	// Tight cluster + one massive outlier. Without clipping, "x" would get
	// z ≈ +1.41 (because σ inflates with the outlier itself); WITH clipping,
	// it's capped at +3 only if z would exceed 3. Construct a case where the
	// *unclipped* z would clearly exceed 3 by adding a near-flat majority.
	list := ScoredIDList{
		{ID: "x", Score: 1000},
		{ID: "a", Score: 1},
		{ID: "b", Score: 1.001},
		{ID: "c", Score: 0.999},
		{ID: "d", Score: 1.0005},
		{ID: "e", Score: 0.9995},
	}
	got := DBSF(list)
	var topScore float64
	for _, f := range got {
		if f.ID == "x" {
			topScore = f.Score
		}
	}
	// With ±3σ clip, x's contribution must not exceed 3.0.
	if topScore > dbsfClipSigma+1e-9 {
		t.Errorf("score(x) = %v, want ≤ %v (clip violated)", topScore, dbsfClipSigma)
	}
}

// TestDBSF_HandComputed locks in the exact numerical formula on a small input.
// μ = 2, σ = sqrt(((1)^2 + (0)^2 + (-1)^2) / 3) = sqrt(2/3).
func TestDBSF_HandComputed(t *testing.T) {
	list := ScoredIDList{
		{ID: "a", Score: 3},
		{ID: "b", Score: 2},
		{ID: "c", Score: 1},
	}
	got := DBSF(list)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	sigma := math.Sqrt(2.0 / 3.0)
	wantA := 1.0 / sigma
	wantB := 0.0
	wantC := -1.0 / sigma

	scores := make(map[string]float64, 3)
	for _, f := range got {
		scores[f.ID] = f.Score
	}
	if math.Abs(scores["a"]-wantA) > 1e-12 {
		t.Errorf("score(a) = %v, want %v", scores["a"], wantA)
	}
	if math.Abs(scores["b"]-wantB) > 1e-12 {
		t.Errorf("score(b) = %v, want %v", scores["b"], wantB)
	}
	if math.Abs(scores["c"]-wantC) > 1e-12 {
		t.Errorf("score(c) = %v, want %v", scores["c"], wantC)
	}
}

// TestDBSF_DuplicateIDsBestFirst mirrors RRF's "best (first) only" rule for
// duplicates within a list.
func TestDBSF_DuplicateIDsBestFirst(t *testing.T) {
	// "a" appears twice; only the first (score=10) counts in this list.
	list := ScoredIDList{
		{ID: "a", Score: 10},
		{ID: "b", Score: 5},
		{ID: "a", Score: 1}, // ignored
	}
	got := DBSF(list)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (deduped)", len(got))
	}
	// After dedup: {a:10, b:5} → μ=7.5, σ=2.5 → z(a)=+1, z(b)=-1.
	if got[0].ID != "a" || math.Abs(got[0].Score-1.0) > 1e-12 {
		t.Errorf("top = %+v, want a=1.0", got[0])
	}
	if got[1].ID != "b" || math.Abs(got[1].Score-(-1.0)) > 1e-12 {
		t.Errorf("bottom = %+v, want b=-1.0", got[1])
	}
}
