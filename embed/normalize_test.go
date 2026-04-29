package embed

import (
	"math"
	"testing"
)

func TestL2Normalize(t *testing.T) {
	vec := []float32{3, 4}
	l2Normalize(vec)
	// norm = 5, so [0.6, 0.8]
	if math.Abs(float64(vec[0]-0.6)) > 1e-5 {
		t.Errorf("vec[0] = %f, want 0.6", vec[0])
	}
	if math.Abs(float64(vec[1]-0.8)) > 1e-5 {
		t.Errorf("vec[1] = %f, want 0.8", vec[1])
	}
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	vec := []float32{0, 0, 0}
	l2Normalize(vec) // should not panic
	for i, v := range vec {
		if v != 0 {
			t.Errorf("vec[%d] = %f, want 0", i, v)
		}
	}
}

func TestL2Normalize_AlreadyUnit(t *testing.T) {
	// Input is already unit norm: [1, 0, 0]
	vec := []float32{1, 0, 0}
	l2Normalize(vec)
	if math.Abs(float64(vec[0]-1.0)) > 1e-5 {
		t.Errorf("vec[0] = %f, want 1.0", vec[0])
	}
	if vec[1] != 0 || vec[2] != 0 {
		t.Errorf("vec = %v, want [1, 0, 0]", vec)
	}
}

func TestL2Normalize_NegativeValues(t *testing.T) {
	vec := []float32{-3, 4}
	l2Normalize(vec)
	// norm = 5, so [-0.6, 0.8]
	if math.Abs(float64(vec[0]-(-0.6))) > 1e-5 {
		t.Errorf("vec[0] = %f, want -0.6", vec[0])
	}
	if math.Abs(float64(vec[1]-0.8)) > 1e-5 {
		t.Errorf("vec[1] = %f, want 0.8", vec[1])
	}
}

func TestL2Normalize_HighDim(t *testing.T) {
	// 1024-dimensional vector (matching e5 output dimension)
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = float32(i) * 0.01
	}
	l2Normalize(vec)

	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("L2 norm = %f, want 1.0", norm)
	}
}

func TestL2Normalize_ExportedAlias(t *testing.T) {
	// Verify L2Normalize matches l2Normalize behaviour (used by embed/onnx).
	vec := []float32{3, 4}
	L2Normalize(vec)
	if math.Abs(float64(vec[0]-0.6)) > 1e-5 || math.Abs(float64(vec[1]-0.8)) > 1e-5 {
		t.Errorf("L2Normalize: vec = %v, want [0.6, 0.8]", vec)
	}
}
