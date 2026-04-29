package rerank

import "testing"

func TestApplySourceWeights_NilMap_NoOp(t *testing.T) {
	scores := []float32{0.9, 0.5, 0.1}
	head := []Doc{{Source: "wiki"}, {Source: "web"}, {Source: "forum"}}
	out := applySourceWeights(scores, head, nil)
	want := []float32{0.9, 0.5, 0.1}
	for i, v := range out {
		if v != want[i] {
			t.Errorf("pos %d: got %v want %v", i, v, want[i])
		}
	}
}

func TestApplySourceWeights_KnownSources(t *testing.T) {
	scores := []float32{1.0, 1.0, 1.0}
	head := []Doc{{Source: "wiki"}, {Source: "web"}, {Source: "forum"}}
	weights := map[string]float32{"wiki": 2.0, "web": 0.5, "forum": 1.0}
	out := applySourceWeights(scores, head, weights)
	want := []float32{2.0, 0.5, 1.0}
	for i, v := range out {
		if v != want[i] {
			t.Errorf("pos %d: got %v want %v", i, v, want[i])
		}
	}
}

func TestApplySourceWeights_MissingSource_DefaultOne(t *testing.T) {
	scores := []float32{1.0}
	head := []Doc{{Source: "unknown-src"}}
	weights := map[string]float32{"wiki": 2.0}
	out := applySourceWeights(scores, head, weights)
	// "unknown-src" not in map → weight 1.0, score unchanged.
	if out[0] != 1.0 {
		t.Errorf("missing source: got %v want 1.0", out[0])
	}
}

func TestApplySourceWeights_NoPanicOnNil(t *testing.T) {
	// Empty scores and head — should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
	}()
	applySourceWeights(nil, nil, map[string]float32{"a": 1.0})
}
