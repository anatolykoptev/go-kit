package sparse

import (
	"testing"
)

// TestFactory_HTTP verifies type=http builds an HTTPSparseEmbedder.
func TestFactory_HTTP(t *testing.T) {
	cfg := Config{
		Type:        "http",
		HTTPBaseURL: "http://embed:8082",
		Model:       "splade-v3-distilbert",
	}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*HTTPSparseEmbedder); !ok {
		t.Errorf("expected *HTTPSparseEmbedder, got %T", e)
	}
	if e.VocabSize() != 30522 {
		t.Errorf("default vocab: want 30522, got %d", e.VocabSize())
	}
}

// TestFactory_HTTPDefaultsModel verifies omitting Model uses the default.
func TestFactory_HTTPDefaultsModel(t *testing.T) {
	cfg := Config{Type: "http", HTTPBaseURL: "http://embed:8082"}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h, ok := e.(*HTTPSparseEmbedder)
	if !ok {
		t.Fatalf("type: %T", e)
	}
	if h.Model() != "splade-v3-distilbert" {
		t.Errorf("default model: %s", h.Model())
	}
}

// TestFactory_HTTPMissingURL verifies missing HTTPBaseURL is rejected.
func TestFactory_HTTPMissingURL(t *testing.T) {
	cfg := Config{Type: "http"}
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("expected error for missing HTTPBaseURL")
	}
}

// TestFactory_UnknownType rejects unknown backend types.
func TestFactory_UnknownType(t *testing.T) {
	cfg := Config{Type: "bogus"}
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

// TestFactory_EmptyTypeDefaultsToHTTP verifies a zero-value Type takes the
// HTTP branch (matches the v2 behaviour).
func TestFactory_EmptyTypeDefaultsToHTTP(t *testing.T) {
	cfg := Config{HTTPBaseURL: "http://embed:8082"}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*HTTPSparseEmbedder); !ok {
		t.Errorf("expected *HTTPSparseEmbedder, got %T", e)
	}
}

// TestFactory_ConfigPropagation verifies TopK / MinWeight / VocabSize
// from Config reach the HTTPSparseEmbedder.
func TestFactory_ConfigPropagation(t *testing.T) {
	cfg := Config{
		Type:        "http",
		HTTPBaseURL: "http://embed:8082",
		TopK:        100,
		MinWeight:   0.5,
		VocabSize:   50000,
	}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := e.(*HTTPSparseEmbedder)
	if h.topK != 100 {
		t.Errorf("topK: want 100, got %d", h.topK)
	}
	if h.minWeight != 0.5 {
		t.Errorf("minWeight: want 0.5, got %v", h.minWeight)
	}
	if h.vocabSize != 50000 {
		t.Errorf("vocabSize: want 50000, got %d", h.vocabSize)
	}
}
