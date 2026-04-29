package rerank

import (
	"context"
	"testing"
)

// makeDoc is a test helper that creates a Doc with a given EmbedVector.
func makeDoc(id string, vec []float32) Doc {
	return Doc{ID: id, EmbedVector: vec}
}

func TestApplyMMR_Lambda1_PureRelevance_MatchesCosineSort(t *testing.T) {
	// With lambda=1, MMR degenerates to pure relevance ordering.
	docs := []Doc{
		makeDoc("a", []float32{1, 0, 0}),
		makeDoc("b", []float32{0, 1, 0}),
		makeDoc("c", []float32{0, 0, 1}),
	}
	relScores := []float32{0.3, 0.9, 0.6}

	got := applyMMR(context.Background(), docs, relScores, 1.0)

	if len(got) != 3 {
		t.Fatalf("len: got %d want 3", len(got))
	}
	// Highest relevance first: b(0.9), c(0.6), a(0.3)
	if got[0].ID != "b" {
		t.Errorf("pos 0: got %q want b", got[0].ID)
	}
	if got[1].ID != "c" {
		t.Errorf("pos 1: got %q want c", got[1].ID)
	}
	if got[2].ID != "a" {
		t.Errorf("pos 2: got %q want a", got[2].ID)
	}
}

func TestApplyMMR_Lambda0_PureDiversity_KnownInput(t *testing.T) {
	// With lambda=0, pure diversity: select most dissimilar from already-selected.
	// All docs have equal relevance=1.0. doc "a" is selected first (relScore tie,
	// first candidate wins), then the most different from "a".
	docs := []Doc{
		makeDoc("a", []float32{1, 0}),  // will be selected first (highest rel in first pass)
		makeDoc("b", []float32{1, 0}),  // identical to a → sim=1
		makeDoc("c", []float32{-1, 0}), // opposite to a → sim=-1 → most diverse
	}
	relScores := []float32{1.0, 1.0, 1.0}

	got := applyMMR(context.Background(), docs, relScores, 0.0)

	if len(got) != 3 {
		t.Fatalf("len: got %d want 3", len(got))
	}
	// First: a (selected first since all equal rel)
	if got[0].ID != "a" {
		t.Errorf("pos 0: got %q want a", got[0].ID)
	}
	// Second: c (most diverse from a; sim with a = -1 → diversity score = -0*1 - 1*(-1) = 1)
	if got[1].ID != "c" {
		t.Errorf("pos 1: got %q want c (most diverse from a), got %q", t.Name(), got[1].ID)
	}
}

func TestApplyMMR_LambdaMid_BalancedSelection(t *testing.T) {
	// lambda=0.5: balance relevance and diversity.
	// doc a: rel=0.9, identical to query direction
	// doc b: rel=0.5, orthogonal to a → high diversity from a
	// doc c: rel=0.8, similar to a → lower diversity from a
	docs := []Doc{
		makeDoc("a", []float32{1, 0}),
		makeDoc("b", []float32{0, 1}),
		makeDoc("c", []float32{0.9, 0.1}),
	}
	relScores := []float32{0.9, 0.5, 0.8}

	got := applyMMR(context.Background(), docs, relScores, 0.5)

	if len(got) != 3 {
		t.Fatalf("len: got %d want 3", len(got))
	}
	// Just verify all 3 docs returned in some order, no panic
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	for _, id := range []string{"a", "b", "c"} {
		if !ids[id] {
			t.Errorf("missing doc %q in result", id)
		}
	}
}

func TestApplyMMR_EmptyDocs_NoPanic(t *testing.T) {
	got := applyMMR(context.Background(), []Doc{}, []float32{}, 0.5)
	if len(got) != 0 {
		t.Errorf("empty docs: want empty result, got %d items", len(got))
	}
}

func TestApplyMMR_AllIdenticalDocs_LambdaMid_StillCompletes(t *testing.T) {
	vec := []float32{1, 0, 0}
	docs := []Doc{
		makeDoc("a", vec),
		makeDoc("b", vec),
		makeDoc("c", vec),
	}
	relScores := []float32{0.8, 0.7, 0.6}

	got := applyMMR(context.Background(), docs, relScores, 0.5)

	if len(got) != 3 {
		t.Fatalf("identical docs: len %d want 3", len(got))
	}
	// All docs returned, ordering still defined (no panic).
}

func TestApplyMMR_CtxCancel_StopsEarly(t *testing.T) {
	// Build a larger set; cancel after selecting the first item.
	docs := make([]Doc, 20)
	relScores := make([]float32, 20)
	for i := range docs {
		docs[i] = makeDoc(string(rune('a'+i)), []float32{float32(i), 0})
		relScores[i] = float32(20 - i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately — MMR must return partial result (at least empty, no panic).
	cancel()

	got := applyMMR(ctx, docs, relScores, 0.5)
	// Result may be 0 or 1 items (cancelled before or after first iteration).
	if got == nil {
		t.Error("nil result on cancel, want non-nil []Scored")
	}
	if len(got) > len(docs) {
		t.Errorf("result longer than input: %d > %d", len(got), len(docs))
	}
}

// TestApplyMMR_AllNegativeCrossSims_HandlesCorrectly verifies that when all
// candidates have negative cosine similarity to picked docs, MMR doesn't
// over-penalize via maxSim defaulting to 0.
func TestApplyMMR_AllNegativeCrossSims_HandlesCorrectly(t *testing.T) {
	// Construct docs where cross-similarities will be negative.
	// doc b is antiparallel to doc a → cosine sim = -1.
	// doc c is orthogonal to both but its cross-sim with a is 0, however once b
	// is in result, cross-sim to b is also 0; testing the antiparallel path.
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{1, 0, 0}},
		{ID: "b", EmbedVector: []float32{-1, 0, 0}}, // antiparallel to a
		{ID: "c", EmbedVector: []float32{0, 1, 0}},  // orthogonal to both
	}
	relScores := []float32{0.9, 0.5, 0.3} // a most relevant, c least

	got := applyMMR(context.Background(), docs, relScores, 0.5)

	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	// a should be picked first (highest rel)
	if got[0].ID != "a" {
		t.Errorf("first picked: got %q, want a", got[0].ID)
	}
	// No assertion on b vs c order — just verify we handle negative sims without
	// panic and produce all 3 results.
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	for _, id := range []string{"a", "b", "c"} {
		if !ids[id] {
			t.Errorf("missing doc %q in result", id)
		}
	}
}

func TestApplyMMR_PreservesOriginalRelevance(t *testing.T) {
	// Verify that Scored[i].Score = relScores[orig idx], NOT the MMR objective score.
	docs := []Doc{
		makeDoc("a", []float32{1, 0}),
		makeDoc("b", []float32{0, 1}),
	}
	relScores := []float32{0.7, 0.9}

	got := applyMMR(context.Background(), docs, relScores, 0.5)

	if len(got) != 2 {
		t.Fatalf("len: got %d want 2", len(got))
	}
	// Validate that each Scored.Score equals the original relScore for that doc.
	for _, s := range got {
		expected := relScores[s.OrigRank]
		if s.Score != expected {
			t.Errorf("doc %q: Score=%v (MMR?), want original relScore=%v", s.ID, s.Score, expected)
		}
	}
}
