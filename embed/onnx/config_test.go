package onnx

import "testing"

func TestDefaultModelConfig(t *testing.T) {
	got := DefaultModelConfig()
	want := ModelConfig{Dim: 1024, MaxLen: 512, PadID: 1}
	if got != want {
		t.Errorf("DefaultModelConfig = %+v, want %+v", got, want)
	}
}

func TestKnownModels_E5(t *testing.T) {
	models := KnownModels()
	cfg, ok := models["multilingual-e5-large"]
	if !ok {
		t.Fatal("multilingual-e5-large not in KnownModels")
	}
	if cfg.Dim != 1024 || cfg.MaxLen != 512 || cfg.PadID != 1 {
		t.Errorf("e5 cfg = %+v", cfg)
	}
	if cfg.HasTokenTypeID {
		t.Error("e5 should not need token_type_ids")
	}
}

func TestKnownModels_Jina(t *testing.T) {
	models := KnownModels()
	cfg, ok := models["jina-code-v2"]
	if !ok {
		t.Fatal("jina-code-v2 not in KnownModels")
	}
	if cfg.Dim != 768 || cfg.MaxLen != 512 || cfg.PadID != 0 {
		t.Errorf("jina cfg = %+v", cfg)
	}
	if !cfg.HasTokenTypeID {
		t.Error("jina-code-v2 needs token_type_ids (BERT-family)")
	}
}

func TestKnownModels_IsCopy(t *testing.T) {
	// Verify mutating the returned map does not affect subsequent lookups.
	a := KnownModels()
	delete(a, "multilingual-e5-large")
	b := KnownModels()
	if _, ok := b["multilingual-e5-large"]; !ok {
		t.Error("KnownModels mutation leaked into package state")
	}
}
