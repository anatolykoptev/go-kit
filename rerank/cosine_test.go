package rerank

import (
	"math"
	"testing"
)

func TestCosineSim_ParallelVectors_Returns1(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	got := cosineSim(a, b)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("parallel vectors: got %v want 1.0", got)
	}
}

func TestCosineSim_OrthogonalVectors_Returns0(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	got := cosineSim(a, b)
	if math.Abs(float64(got)) > 1e-6 {
		t.Errorf("orthogonal vectors: got %v want 0.0", got)
	}
}

func TestCosineSim_AntiparallelVectors_Returns_neg1(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	got := cosineSim(a, b)
	if math.Abs(float64(got)+1.0) > 1e-6 {
		t.Errorf("antiparallel vectors: got %v want -1.0", got)
	}
}

func TestCosineSim_EmptyVectors_Returns0(t *testing.T) {
	got := cosineSim([]float32{}, []float32{})
	if got != 0 {
		t.Errorf("empty vectors: got %v want 0", got)
	}
}

func TestCosineSim_LengthMismatch_Returns0(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	got := cosineSim(a, b)
	if got != 0 {
		t.Errorf("length mismatch: got %v want 0", got)
	}
}

func TestCosineSim_ZeroNorm_NoNaN(t *testing.T) {
	zero := []float32{0, 0, 0}
	other := []float32{1, 2, 3}
	got := cosineSim(zero, other)
	if math.IsNaN(float64(got)) {
		t.Errorf("zero norm: got NaN, want 0")
	}
	if got != 0 {
		t.Errorf("zero norm: got %v want 0", got)
	}
}
