package embed

// v1_compat_test.go — pins v1 API behaviour byte-identical across all E0-E4 streams.
//
// Rule: TestV1ApiUnchanged MUST be green on every PR that touches embed/.
// Any change that breaks these assertions is a breaking API change and must
// go through a major version bump + migration guide.
//
// Coverage (9 subtests):
//   1. HTTPDefaults — default model + dim apply when omitted from Config
//   2. HTTP — explicit model + dim round-trip
//   3. HTTPMissingURL — empty HTTPBaseURL is rejected with an error
//   4. Ollama — OllamaClient built with prefix/dim opts
//   5. VoyageMissingKey — missing VoyageAPIKey is rejected
//   6. Voyage — VoyageClient built with explicit model
//   7. ONNXReturnsErrONNXNotInFactory (type="onnx")
//   8. ONNXReturnsErrONNXNotInFactory (type="") — empty type also triggers ONNX error
//   9. UnknownType — returns error for unrecognised type string

import (
	"errors"
	"testing"
)

// TestV1ApiUnchanged pins all v1 New(cfg, logger) fixtures.
// Each subtest asserts the exact same contract documented in the original
// factory_test.go, executed via the unchanged v1 entry point.
func TestV1ApiUnchanged(t *testing.T) {
	// 1. HTTPDefaults — defaults applied when model + dim omitted.
	t.Run("HTTPDefaults", func(t *testing.T) {
		cfg := Config{Type: "http", HTTPBaseURL: "http://embed:8082"}
		e, err := New(cfg, testLogger())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, ok := e.(*HTTPEmbedder); !ok {
			t.Errorf("expected *HTTPEmbedder, got %T", e)
		}
		if e.Dimension() != defaultHTTPDim {
			t.Errorf("default dim: want %d, got %d", defaultHTTPDim, e.Dimension())
		}
	})

	// 2. HTTP — explicit model + dim round-trip.
	t.Run("HTTP", func(t *testing.T) {
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
	})

	// 3. HTTPMissingURL — missing HTTPBaseURL must return a non-nil error.
	t.Run("HTTPMissingURL", func(t *testing.T) {
		cfg := Config{Type: "http"}
		_, err := New(cfg, testLogger())
		if err == nil {
			t.Fatal("expected error for missing HTTPBaseURL")
		}
	})

	// 4. Ollama — OllamaClient with prefix + dim opts.
	t.Run("Ollama", func(t *testing.T) {
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
		c, ok := e.(*OllamaClient)
		if !ok {
			t.Errorf("expected *OllamaClient, got %T", e)
		}
		if e.Dimension() != 768 {
			t.Errorf("dim: want 768, got %d", e.Dimension())
		}
		// Verify prefix opts applied (internal fields — same package).
		if c.textPrefix != "passage: " {
			t.Errorf("textPrefix: want %q, got %q", "passage: ", c.textPrefix)
		}
		if c.queryPrefix != "query: " {
			t.Errorf("queryPrefix: want %q, got %q", "query: ", c.queryPrefix)
		}
	})

	// 5. VoyageMissingKey — empty VoyageAPIKey must return a non-nil error.
	t.Run("VoyageMissingKey", func(t *testing.T) {
		cfg := Config{Type: "voyage"}
		_, err := New(cfg, testLogger())
		if err == nil {
			t.Fatal("expected error for missing VoyageAPIKey")
		}
	})

	// 6. Voyage — VoyageClient built with explicit model.
	t.Run("Voyage", func(t *testing.T) {
		cfg := Config{Type: "voyage", VoyageAPIKey: "sk-test", Model: "voyage-4-lite"}
		e, err := New(cfg, testLogger())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, ok := e.(*VoyageClient); !ok {
			t.Errorf("expected *VoyageClient, got %T", e)
		}
	})

	// 7. ONNXReturnsErrONNXNotInFactory — type="onnx".
	t.Run("ONNXReturnsErrONNXNotInFactory_Explicit", func(t *testing.T) {
		cfg := Config{Type: "onnx", ONNXModelDir: "/models/e5"}
		_, err := New(cfg, testLogger())
		if !errors.Is(err, ErrONNXNotInFactory) {
			t.Errorf("type=onnx: want ErrONNXNotInFactory, got %v", err)
		}
	})

	// 8. ONNXReturnsErrONNXNotInFactory — type="" (empty type also triggers ONNX error).
	t.Run("ONNXReturnsErrONNXNotInFactory_Empty", func(t *testing.T) {
		cfg := Config{Type: "", ONNXModelDir: "/models/e5"}
		_, err := New(cfg, testLogger())
		if !errors.Is(err, ErrONNXNotInFactory) {
			t.Errorf("type=empty: want ErrONNXNotInFactory, got %v", err)
		}
	})

	// 9. UnknownType — unknown backend string is rejected.
	t.Run("UnknownType", func(t *testing.T) {
		cfg := Config{Type: "bogus"}
		_, err := New(cfg, testLogger())
		if err == nil {
			t.Fatal("expected error for unknown type")
		}
		// Must NOT be ErrONNXNotInFactory — it's a different error path.
		if errors.Is(err, ErrONNXNotInFactory) {
			t.Errorf("unknown type should return format error, not ErrONNXNotInFactory")
		}
	})
}
