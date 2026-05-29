package embed

import (
	"math"
	"testing"
)

func TestCosine_Identical(t *testing.T) {
	v := []float32{1, 2, 3}
	got := Cosine(v, v)
	if math.Abs(float64(got-1.0)) > 1e-6 {
		t.Errorf("Cosine(v,v) = %f, want 1.0", got)
	}
}

func TestCosine_Orthogonal(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	got := Cosine(a, b)
	if math.Abs(float64(got)) > 1e-6 {
		t.Errorf("Cosine(orthogonal) = %f, want 0", got)
	}
}

func TestCosine_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	got := Cosine(a, b)
	if math.Abs(float64(got+1.0)) > 1e-6 {
		t.Errorf("Cosine(opposite) = %f, want -1.0", got)
	}
}

func TestCosine_MismatchedLen(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	got := Cosine(a, b)
	if got != 0 {
		t.Errorf("Cosine(mismatched) = %f, want 0", got)
	}
}

func TestCosine_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	got := Cosine(a, b)
	if got != 0 {
		t.Errorf("Cosine(zero, v) = %f, want 0", got)
	}
	got = Cosine(b, a)
	if got != 0 {
		t.Errorf("Cosine(v, zero) = %f, want 0", got)
	}
}

func TestCosine_EmptyInput(t *testing.T) {
	got := Cosine(nil, nil)
	if got != 0 {
		t.Errorf("Cosine(nil,nil) = %f, want 0", got)
	}
	got = Cosine([]float32{}, []float32{})
	if got != 0 {
		t.Errorf("Cosine([],[]) = %f, want 0", got)
	}
}
