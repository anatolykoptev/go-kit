package embed

import (
	"context"
	"errors"
	"testing"
)

// cannedEmbedder returns pre-set vectors for DedupeTexts tests.
type cannedEmbedder struct {
	vecs [][]float32
	err  error
	dim  int
}

func (c *cannedEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return c.vecs, c.err
}
func (c *cannedEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	if len(c.vecs) == 0 {
		return nil, c.err
	}
	return c.vecs[0], c.err
}
func (c *cannedEmbedder) Dimension() int { return c.dim }
func (c *cannedEmbedder) Close() error   { return nil }

// --- DedupeGroups tests ---

func TestDedupeGroups_Empty(t *testing.T) {
	got := DedupeGroups(nil, 0.9)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestDedupeGroups_AllDistinct(t *testing.T) {
	// Three orthogonal unit vectors — no pair is similar.
	vecs := [][]float32{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
	got := DedupeGroups(vecs, 0.9)
	if len(got) != 3 {
		t.Fatalf("want 3 singleton groups, got %d: %v", len(got), got)
	}
	for i, g := range got {
		if len(g) != 1 || g[0] != i {
			t.Errorf("group %d = %v, want [%d]", i, g, i)
		}
	}
}

func TestDedupeGroups_TwoIdentical(t *testing.T) {
	v := []float32{0.6, 0.8} // unit vector
	vecs := [][]float32{v, v, {0, 1}}
	got := DedupeGroups(vecs, 0.99)
	// indices 0 and 1 are identical (cosine=1.0 ≥ 0.99); index 2 is its own group.
	if len(got) != 2 {
		t.Fatalf("want 2 groups, got %d: %v", len(got), got)
	}
	if len(got[0]) != 2 || got[0][0] != 0 || got[0][1] != 1 {
		t.Errorf("first group = %v, want [0 1]", got[0])
	}
	if len(got[1]) != 1 || got[1][0] != 2 {
		t.Errorf("second group = %v, want [2]", got[1])
	}
}

func TestDedupeGroups_TransitiveChain(t *testing.T) {
	// Construct A, B, C such that A~B and B~C (above threshold) but A~C is
	// slightly below. Transitive closure must place all three in one group.
	//
	// A = [1, 0], B = [cos θ, sin θ], C = [cos 2θ, sin 2θ]
	// cos(A,B) = cos θ, cos(B,C) = cos θ, cos(A,C) = cos 2θ
	// With θ = 0.25 rad: cos θ ≈ 0.9689, cos 2θ ≈ 0.8776.
	// Set threshold = 0.95: A~B OK, B~C OK, A~C NOT OK (0.878 < 0.95).
	const threshold = float32(0.95)
	vecs := [][]float32{
		{1.0000, 0.0000},         // A
		{0.9689, 0.2474},         // B  (≈ cos/sin 0.25rad)
		{0.8776, 0.4794},         // C  (≈ cos/sin 0.50rad)
		{0.0000, 1.0000},         // D  orthogonal singleton
	}
	got := DedupeGroups(vecs, threshold)
	if len(got) != 2 {
		t.Fatalf("want 2 groups (ABC + D), got %d: %v", len(got), got)
	}
	// First group must be [0 1 2]
	if len(got[0]) != 3 {
		t.Errorf("transitive group = %v, want [0 1 2]", got[0])
	}
	// Second group is singleton D
	if len(got[1]) != 1 || got[1][0] != 3 {
		t.Errorf("singleton group = %v, want [3]", got[1])
	}
}

func TestDedupeGroups_ThresholdBoundary(t *testing.T) {
	// Two identical vectors → cosine exactly 1.0; threshold = 1.0 counts as dup.
	v := []float32{1, 0}
	vecs := [][]float32{v, v}
	got := DedupeGroups(vecs, 1.0)
	if len(got) != 1 {
		t.Errorf("threshold=1.0 exact match: want 1 group, got %d: %v", len(got), got)
	}
}

func TestDedupeGroups_DeterministicOrder(t *testing.T) {
	// Run twice; result must be identical.
	vecs := [][]float32{
		{1, 0}, {1, 0}, {0, 1}, {0, 1},
	}
	a := DedupeGroups(vecs, 0.99)
	b := DedupeGroups(vecs, 0.99)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Errorf("group %d length differs: %v vs %v", i, a[i], b[i])
			continue
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Errorf("group[%d][%d] differs: %d vs %d", i, j, a[i][j], b[i][j])
			}
		}
	}
}

// --- DedupeTexts tests ---

func TestDedupeTexts_NilEmbedder(t *testing.T) {
	texts := []string{"alpha", "beta", "gamma"}
	groups, err := DedupeTexts(context.Background(), nil, texts, 0.9)
	if err != nil {
		t.Fatalf("nil embedder: unexpected error: %v", err)
	}
	if len(groups) != len(texts) {
		t.Fatalf("nil embedder: want %d singletons, got %d", len(texts), len(groups))
	}
	for i, g := range groups {
		if len(g) != 1 || g[0] != i {
			t.Errorf("group %d = %v, want [%d]", i, g, i)
		}
	}
}

func TestDedupeTexts_EmptyTexts(t *testing.T) {
	groups, err := DedupeTexts(context.Background(), nil, nil, 0.9)
	if err != nil {
		t.Fatalf("empty texts: unexpected error: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("empty texts: want empty groups, got %v", groups)
	}
}

func TestDedupeTexts_CannedEmbedder(t *testing.T) {
	// Two identical vectors (indices 0 and 1) plus one distinct (index 2).
	v := []float32{0.6, 0.8}
	e := &cannedEmbedder{
		vecs: [][]float32{v, v, {0, 1}},
		dim:  2,
	}
	groups, err := DedupeTexts(context.Background(), e, []string{"a", "b", "c"}, 0.99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d: %v", len(groups), groups)
	}
	if len(groups[0]) != 2 {
		t.Errorf("first group = %v, want [0 1]", groups[0])
	}
	if len(groups[1]) != 1 || groups[1][0] != 2 {
		t.Errorf("second group = %v, want [2]", groups[1])
	}
}

func TestDedupeTexts_EmbedError(t *testing.T) {
	sentinel := errors.New("embed failed")
	e := &cannedEmbedder{err: sentinel, dim: 2}
	_, err := DedupeTexts(context.Background(), e, []string{"x", "y"}, 0.9)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}
