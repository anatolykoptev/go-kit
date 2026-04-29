package embed

import (
	"errors"
	"testing"
)

// TestFactory_HTTP verifies type=http builds an HTTPEmbedder.
func TestFactory_HTTP(t *testing.T) {
	cfg := Config{
		Type:        "http",
		HTTPBaseURL: "http://embed:8082",
		Model:       "multilingual-e5-large",
		HTTPDim:     1024,
	}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*HTTPEmbedder); !ok {
		t.Errorf("expected *HTTPEmbedder, got %T", e)
	}
	if e.Dimension() != 1024 {
		t.Errorf("dim: want 1024, got %d", e.Dimension())
	}
}

// TestFactory_HTTPDefaults verifies the default model + dim apply when omitted.
func TestFactory_HTTPDefaults(t *testing.T) {
	cfg := Config{Type: "http", HTTPBaseURL: "http://embed:8082"}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if e.Dimension() != 1024 {
		t.Errorf("default dim: want 1024, got %d", e.Dimension())
	}
}

// TestFactory_HTTPMissingURL verifies missing HTTPBaseURL is rejected.
func TestFactory_HTTPMissingURL(t *testing.T) {
	cfg := Config{Type: "http"}
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("expected error for missing HTTPBaseURL")
	}
}

// TestFactory_Ollama verifies type=ollama builds an OllamaClient with options.
func TestFactory_Ollama(t *testing.T) {
	cfg := Config{
		Type:         "ollama",
		OllamaURL:    "http://ollama:11434",
		Model:        "nomic-embed-text",
		OllamaDim:    768,
		OllamaPrefix: "passage: ",
		OllamaQuery:  "query: ",
	}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*OllamaClient); !ok {
		t.Errorf("expected *OllamaClient, got %T", e)
	}
	if e.Dimension() != 768 {
		t.Errorf("dim: want 768, got %d", e.Dimension())
	}
}

// TestFactory_VoyageMissingKey verifies the API key requirement.
func TestFactory_VoyageMissingKey(t *testing.T) {
	cfg := Config{Type: "voyage"}
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("expected error for missing VoyageAPIKey")
	}
}

// TestFactory_Voyage verifies type=voyage builds a VoyageClient.
func TestFactory_Voyage(t *testing.T) {
	cfg := Config{Type: "voyage", VoyageAPIKey: "k", Model: "voyage-4-lite"}
	e, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*VoyageClient); !ok {
		t.Errorf("expected *VoyageClient, got %T", e)
	}
}

// TestFactory_ONNXReturnsErrONNXNotInFactory documents the ONNX subpackage
// contract: callers must use embed/onnx for cgo-backed inference.
func TestFactory_ONNXReturnsErrONNXNotInFactory(t *testing.T) {
	for _, typ := range []string{"onnx", ""} {
		cfg := Config{Type: typ, ONNXModelDir: "/models/e5"}
		_, err := New(cfg, testLogger())
		if !errors.Is(err, ErrONNXNotInFactory) {
			t.Errorf("type=%q: want ErrONNXNotInFactory, got %v", typ, err)
		}
	}
}

// TestFactory_UnknownType rejects unknown backend types.
func TestFactory_UnknownType(t *testing.T) {
	cfg := Config{Type: "bogus"}
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("expected error for unknown type")
	}
}
