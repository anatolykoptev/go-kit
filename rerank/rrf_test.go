package rerank

import (
	"math"
	"testing"
)

func TestRRF_DefaultK(t *testing.T) {
	if DefaultRRFK != 60 {
		t.Fatalf("DefaultRRFK = %d, want 60 (Cormack-Clarke 2009)", DefaultRRFK)
	}
}

// TestRRF_TableDriven covers fundamental fusion behavior with hand-computed
// expected scores using the Cormack-Clarke formula score = sum 1/(k + rank).
func TestRRF_TableDriven(t *testing.T) {
	const k = 60
	tests := []struct {
		name      string
		k         int
		lists     [][]string
		wantOrder []string // expected ID order desc by score
	}{
		{
			name:      "empty lists",
			k:         k,
			lists:     [][]string{},
			wantOrder: nil,
		},
		{
			name:      "single all-empty list",
			k:         k,
			lists:     [][]string{{}},
			wantOrder: nil,
		},
		{
			name:      "single list passes through preserving order",
			k:         k,
			lists:     [][]string{{"a", "b", "c"}},
			wantOrder: []string{"a", "b", "c"},
		},
		{
			name: "cormack-clarke 3-list example: shared top boosts winner",
			k:    k,
			lists: [][]string{
				{"a", "b", "c", "d"},
				{"b", "a", "e", "f"},
				{"a", "c", "g", "h"},
			},
			// score(a) = 1/61 + 1/62 + 1/61 ≈ 0.04892
			// score(b) = 1/62 + 1/61         ≈ 0.03252
			// score(c) = 1/63 + 1/62         ≈ 0.03200
			// score(e) = 1/63                ≈ 0.01587
			// score(g) = 1/63                ≈ 0.01587 (tied with e)
			// score(d) = 1/64                ≈ 0.01563
			// score(f) = 1/64                ≈ 0.01563 (tied with d)
			// score(h) = 1/64                ≈ 0.01563 (tied with d, f)
			// First-seen order: a, b, c, d, e, f, g, h.
			// After stable sort desc: e before g (e seen first), d before f before h.
			wantOrder: []string{"a", "b", "c", "e", "g", "d", "f", "h"},
		},
		{
			name: "K=0 falls back to default",
			k:    0,
			lists: [][]string{
				{"x", "y"},
				{"y", "x"},
			},
			// With k=60: x=1/61+1/62, y=1/62+1/61 → tied, stable order = first-seen = x, y.
			wantOrder: []string{"x", "y"},
		},
		{
			name: "negative K falls back to default",
			k:    -5,
			lists: [][]string{
				{"x", "y"},
			},
			wantOrder: []string{"x", "y"},
		},
		{
			name: "duplicate ids in one list use best rank only",
			k:    k,
			// "a" appears at rank 1 AND rank 3 in the same list. Only rank 1 counts.
			// score(a) from list = 1/61 (NOT 1/61 + 1/63).
			// score(b) from list = 1/62.
			// Therefore a > b strictly, by 1/61 - 1/62.
			lists: [][]string{
				{"a", "b", "a"},
			},
			wantOrder: []string{"a", "b"},
		},
		{
			name: "two lists, identical order",
			k:    k,
			lists: [][]string{
				{"p", "q", "r"},
				{"p", "q", "r"},
			},
			wantOrder: []string{"p", "q", "r"},
		},
		{
			name: "tie broken by stable first-seen order",
			k:    k,
			lists: [][]string{
				{"first", "second"},
				{"second", "first"},
			},
			// score(first)  = 1/61 + 1/62
			// score(second) = 1/62 + 1/61 → tied → stable: "first" seen before "second".
			wantOrder: []string{"first", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RRF(tt.k, tt.lists...)
			if len(got) != len(tt.wantOrder) {
				t.Fatalf("RRF len = %d, want %d (got = %+v)", len(got), len(tt.wantOrder), got)
			}
			for i, id := range tt.wantOrder {
				if got[i].ID != id {
					t.Errorf("position %d: ID = %q, want %q (full got = %+v)", i, got[i].ID, id, got)
				}
			}
		})
	}
}

// TestRRF_ScoreFormula verifies the exact numerical score for a simple input
// matches the Cormack-Clarke formula. Catches off-by-one (0-based vs 1-based)
// rank bugs.
func TestRRF_ScoreFormula(t *testing.T) {
	const k = 60
	// "a" at rank 1 in list 1 and rank 1 in list 2.
	// score(a) = 1/(60+1) + 1/(60+1) = 2/61.
	got := RRF(k, []string{"a"}, []string{"a"})
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	want := 2.0 / 61.0
	if math.Abs(got[0].Score-want) > 1e-12 {
		t.Errorf("score(a) = %v, want %v", got[0].Score, want)
	}
}

// TestRRF_ImmuneToScoreScaleDifferences is a structural test demonstrating
// the rank-only property: RRF output depends only on rank, not on any
// underlying score scale (BM25 vs cosine etc.).
func TestRRF_ImmuneToScoreScaleDifferences(t *testing.T) {
	listA := []string{"x", "y", "z"} // imagine BM25 scores 95, 12, 1
	listB := []string{"x", "y", "z"} // imagine cosine scores 0.99, 0.98, 0.97
	a := RRF(DefaultRRFK, listA, listB)

	listC := []string{"x", "y", "z"} // BM25 scores 5000, 4000, 3000
	listD := []string{"x", "y", "z"} // cosine scores 0.1, 0.05, 0.01
	b := RRF(DefaultRRFK, listC, listD)

	if len(a) != len(b) {
		t.Fatalf("len mismatch: a=%d b=%d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("position %d differs: a=%q b=%q", i, a[i].ID, b[i].ID)
		}
		if math.Abs(a[i].Score-b[i].Score) > 1e-12 {
			t.Errorf("position %d score differs: a=%v b=%v", i, a[i].Score, b[i].Score)
		}
	}
}

// TestRRF_VariadicAcceptsAnyCount verifies the API accepts 0, 1, 2, ... N lists.
func TestRRF_VariadicAcceptsAnyCount(t *testing.T) {
	// 0 lists.
	if got := RRF(DefaultRRFK); len(got) != 0 {
		t.Errorf("0 lists: got len %d, want 0", len(got))
	}
	// 1 list.
	if got := RRF(DefaultRRFK, []string{"a"}); len(got) != 1 || got[0].ID != "a" {
		t.Errorf("1 list: got %+v", got)
	}
	// 5 lists (exercises ">=5" metric bucket).
	got := RRF(DefaultRRFK,
		[]string{"a", "b"},
		[]string{"a"},
		[]string{"b", "a"},
		[]string{"a", "c"},
		[]string{"a", "b", "c"},
	)
	if len(got) == 0 {
		t.Errorf("5 lists: got empty")
	}
	// "a" should win (appears at top in 4 of 5 lists).
	if got[0].ID != "a" {
		t.Errorf("5 lists: top ID = %q, want %q", got[0].ID, "a")
	}
}
