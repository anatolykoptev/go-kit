package rerank

import (
	"math"
	"sort"
	"testing"
)

func TestNormalize_None_Identity(t *testing.T) {
	in := []float32{0.9, 0.5, 0.1, -1.0}
	orig := append([]float32(nil), in...)
	out := Normalize(in, NormalizeNone)
	for i, v := range out {
		if v != orig[i] {
			t.Errorf("pos %d: got %v want %v", i, v, orig[i])
		}
	}
}

func TestNormalize_MinMax_Range(t *testing.T) {
	scores := []float32{0.2, 0.8, 0.5}
	out := Normalize(scores, NormalizeMinMax)
	// min=0.2, max=0.8, range=0.6
	want := []float32{0, 1.0, 0.5}
	for i, v := range out {
		if math.Abs(float64(v-want[i])) > 1e-5 {
			t.Errorf("pos %d: got %v want %v", i, v, want[i])
		}
	}
}

func TestNormalize_MinMax_AllEqual_NoDivZero(t *testing.T) {
	scores := []float32{0.5, 0.5, 0.5}
	out := Normalize(scores, NormalizeMinMax)
	for i, v := range out {
		if math.Abs(float64(v-0.5)) > 1e-5 {
			t.Errorf("pos %d: got %v want 0.5", i, v)
		}
	}
}

func TestNormalize_ZScore_StdDevZero(t *testing.T) {
	scores := []float32{3.0, 3.0, 3.0}
	out := Normalize(scores, NormalizeZScore)
	for i, v := range out {
		if v != 0 {
			t.Errorf("pos %d: got %v want 0 (stddev=0 case)", i, v)
		}
	}
}

func TestNormalize_SortOrderPreserved(t *testing.T) {
	// MinMax and ZScore are monotonic transforms — they must preserve sort order.
	for _, mode := range []NormalizeMode{NormalizeMinMax, NormalizeZScore} {
		scores := []float32{0.9, 0.3, 0.7, 0.1, 0.5}
		// Build expected order from original scores.
		origOrder := argsort(scores)

		// Copy before normalizing (Normalize modifies in-place).
		cp := append([]float32(nil), scores...)
		out := Normalize(cp, mode)
		gotOrder := argsort(out)

		for i := range origOrder {
			if origOrder[i] != gotOrder[i] {
				t.Errorf("mode %s: sort order changed at rank %d: orig idx %d, got idx %d",
					mode, i, origOrder[i], gotOrder[i])
			}
		}
	}
}

// argsort returns indices that would sort scores descending.
func argsort(scores []float32) []int {
	idx := make([]int, len(scores))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return scores[idx[i]] > scores[idx[j]] })
	return idx
}
