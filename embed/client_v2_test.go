package embed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- NewClient tests ---

// TestNewClient_HTTPBackend verifies that NewClient with no explicit backend
// builds an HTTPEmbedder when a URL is provided.
func TestNewClient_HTTPBackend(t *testing.T) {
	e, err := NewClient("http://embed:8082",
		WithModel("multilingual-e5-large"),
		WithDim(1024),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := e.(*HTTPEmbedder); !ok {
		t.Errorf("expected *HTTPEmbedder, got %T", e)
	}
	if e.Dimension() != 1024 {
		t.Errorf("dim: want 1024, got %d", e.Dimension())
	}
}

// TestNewClient_OllamaBackend verifies that NewClient with WithBackend("ollama")
// builds an OllamaClient with prefix opts applied.
func TestNewClient_OllamaBackend(t *testing.T) {
	e, err := NewClient("http://ollama:11434",
		WithBackend("ollama"),
		WithModel("nomic-embed-text"),
		WithOllamaDocPrefix("passage: "),
		WithOllamaQueryPrefix("query: "),
		WithOllamaDim(1024),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c, ok := e.(*OllamaClient)
	if !ok {
		t.Fatalf("expected *OllamaClient, got %T", e)
	}
	if c.textPrefix != "passage: " {
		t.Errorf("textPrefix: want %q, got %q", "passage: ", c.textPrefix)
	}
	if c.queryPrefix != "query: " {
		t.Errorf("queryPrefix: want %q, got %q", "query: ", c.queryPrefix)
	}
}

// TestNewClient_VoyageBackend_RequiresAPIKey verifies that missing API key
// causes NewClient to return an error for the Voyage backend.
func TestNewClient_VoyageBackend_RequiresAPIKey(t *testing.T) {
	_, err := NewClient("",
		WithBackend("voyage"),
		WithModel("voyage-4-lite"),
		// no WithVoyageAPIKey
	)
	if err == nil {
		t.Fatal("expected error for missing voyage API key")
	}
}

// TestNewClient_VoyageBackend_WithAPIKey verifies that a valid API key builds
// a VoyageClient successfully.
func TestNewClient_VoyageBackend_WithAPIKey(t *testing.T) {
	e, err := NewClient("",
		WithBackend("voyage"),
		WithModel("voyage-4-lite"),
		WithVoyageAPIKey("sk-test"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := e.(*VoyageClient); !ok {
		t.Errorf("expected *VoyageClient, got %T", e)
	}
}

// TestNewClient_HTTPMissingURL verifies that HTTP backend without URL errors.
func TestNewClient_HTTPMissingURL(t *testing.T) {
	_, err := NewClient("") // empty URL, default backend = "http"
	if err == nil {
		t.Fatal("expected error for missing HTTP URL")
	}
}

// TestNewClient_UnknownBackend verifies unknown backend returns an error.
func TestNewClient_UnknownBackend(t *testing.T) {
	_, err := NewClient("http://x", WithBackend("bogus"))
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

// TestNewClient_WithTimeout verifies WithTimeout applies to Ollama client.
func TestNewClient_WithTimeout(t *testing.T) {
	e, err := NewClient("http://ollama:11434",
		WithBackend("ollama"),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c := e.(*OllamaClient)
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout: want 5s, got %v", c.httpClient.Timeout)
	}
}

// TestNewClient_WithObserver verifies WithObserver sets observer (not noop).
func TestNewClient_WithObserver(t *testing.T) {
	obs := &countingObserver{}
	// Observer is applied to cfgInternal — verify WithObserver does not panic
	// and nil observer is ignored (noop stays active).
	WithObserver(nil)(defaultCfg()) // nil ignored
	WithObserver(obs)(defaultCfg()) // non-nil applied

	// Verify nil-ignored rule: noop stays active.
	cfg := defaultCfg()
	WithObserver(nil)(cfg)
	if _, ok := cfg.observer.(noopObserver); !ok {
		t.Errorf("nil observer should leave noop active, got %T", cfg.observer)
	}
}

// --- EmbedWithResult tests ---

// mockHTTPServer creates a test HTTP server that returns the given embeddings.
func mockHTTPServer(t *testing.T, embeddings [][]float32, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, "server error", statusCode)
			return
		}
		data := make([]httpEmbedData, len(embeddings))
		for i, emb := range embeddings {
			data[i] = httpEmbedData{Embedding: emb, Index: i}
		}
		resp := httpEmbedResponse{Data: data}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestEmbedWithResult_StatusOk verifies happy path: correct vectors, StatusOk.
func TestEmbedWithResult_StatusOk(t *testing.T) {
	want := [][]float32{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}
	srv := mockHTTPServer(t, want, http.StatusOK)
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "test-model", 3, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedWithResult error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status: want StatusOk, got %s", res.Status)
	}
	if len(res.Vectors) != 2 {
		t.Fatalf("Vectors len: want 2, got %d", len(res.Vectors))
	}
	for i, v := range res.Vectors {
		if v.Status != StatusOk {
			t.Errorf("[%d] Vector.Status: want StatusOk, got %s", i, v.Status)
		}
		if v.Dim != 3 {
			t.Errorf("[%d] Vector.Dim: want 3, got %d", i, v.Dim)
		}
	}
	if res.Err != nil {
		t.Errorf("Err: want nil, got %v", res.Err)
	}
}

// TestEmbedWithResult_StatusSkipped_EmptyTexts verifies that empty input
// returns StatusSkipped without making a backend call.
func TestEmbedWithResult_StatusSkipped_EmptyTexts(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "m", 1024, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status: want StatusSkipped, got %s", res.Status)
	}
	if callCount != 0 {
		t.Errorf("backend should not be called for empty texts, got %d calls", callCount)
	}
}

// TestEmbedWithResult_StatusSkipped_NilEmbedder verifies nil embedder returns
// StatusSkipped immediately.
func TestEmbedWithResult_StatusSkipped_NilEmbedder(t *testing.T) {
	res, err := EmbedWithResult(context.Background(), nil, []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status: want StatusSkipped, got %s", res.Status)
	}
}

// TestEmbedWithResult_StatusDegraded_HTTPError verifies that a backend error
// produces StatusDegraded with Err set.
func TestEmbedWithResult_StatusDegraded_HTTPError(t *testing.T) {
	srv := mockHTTPServer(t, nil, http.StatusInternalServerError)
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "m", 1024, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{"test"})
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status: want StatusDegraded, got %s", res.Status)
	}
	if res.Err == nil {
		t.Error("Err: want non-nil for degraded status")
	}
}

// TestEmbedWithResult_StatusDegraded_PartialResponse verifies that a backend
// returning the wrong number of vectors produces StatusDegraded.
func TestEmbedWithResult_StatusDegraded_PartialResponse(t *testing.T) {
	// Return only 1 vector for 2 texts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := httpEmbedResponse{
			Data: []httpEmbedData{
				{Embedding: []float32{0.1, 0.2}, Index: 0},
				// missing second embedding
			},
		}
		// Lie: return 200 but wrong count — simulates the mismatch check
		// However httpEmbedder.Embed checks len(parsed.Data) != len(texts)
		// so it will error before EmbedWithResult sees it.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "m", 1024, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error for partial response")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status: want StatusDegraded, got %s", res.Status)
	}
	if len(res.Vectors) != 2 {
		t.Errorf("Vectors len: want 2 (empty placeholders), got %d", len(res.Vectors))
	}
}

// TestEmbedWithResult_DryRunOpt_SkipsHTTP verifies that WithDryRun() prevents
// any backend call and returns StatusSkipped vectors.
func TestEmbedWithResult_DryRunOpt_SkipsHTTP(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "m", 1024, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{"a", "b"}, WithDryRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 0 {
		t.Errorf("DryRun: backend should not be called, got %d calls", callCount)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status: want StatusSkipped, got %s", res.Status)
	}
	if len(res.Vectors) != 2 {
		t.Fatalf("Vectors len: want 2, got %d", len(res.Vectors))
	}
	for i, v := range res.Vectors {
		if v.Status != StatusSkipped {
			t.Errorf("[%d] Vector.Status: want StatusSkipped, got %s", i, v.Status)
		}
		if v.Embedding != nil {
			t.Errorf("[%d] Embedding: want nil for DryRun, got %v", i, v.Embedding)
		}
	}
}

// TestModelFromEmbedder_HTTPOllamaVoyage verifies that modelFromEmbedder
// returns the correct model string for each known backend type.
func TestModelFromEmbedder_HTTPOllamaVoyage(t *testing.T) {
	cases := []struct {
		name  string
		e     Embedder
		model string
	}{
		{"http", NewHTTPEmbedder("http://x", "http-model", 1024, testLogger()), "http-model"},
		{"ollama", NewOllamaClient("", "ollama-model", testLogger()), "ollama-model"},
		{"voyage", NewVoyageClient("key", "voyage-model", testLogger()), "voyage-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := modelFromEmbedder(tc.e)
			if got != tc.model {
				t.Errorf("modelFromEmbedder: want %q, got %q", tc.model, got)
			}
		})
	}
}

// TestModelFromEmbedder_UnknownType verifies unknown types return empty string.
func TestModelFromEmbedder_UnknownType(t *testing.T) {
	if got := modelFromEmbedder(nil); got != "" {
		t.Errorf("nil embedder: want empty, got %q", got)
	}
}

// --- Opt application tests ---

// TestOpt_DefaultCfg verifies defaultCfg has sensible defaults.
func TestOpt_DefaultCfg(t *testing.T) {
	cfg := defaultCfg()
	if cfg.observer == nil {
		t.Error("observer: want noopObserver, got nil")
	}
	if _, ok := cfg.observer.(noopObserver); !ok {
		t.Errorf("observer: want noopObserver, got %T", cfg.observer)
	}
	if cfg.timeout != 30*time.Second {
		t.Errorf("timeout: want 30s, got %v", cfg.timeout)
	}
}

// TestOpt_AllOptsApply verifies each Opt function modifies cfgInternal correctly.
func TestOpt_AllOptsApply(t *testing.T) {
	obs := &countingObserver{}
	logger := testLogger()

	cfg := defaultCfg()
	WithModel("my-model")(cfg)
	WithDim(768)(cfg)
	WithTimeout(5 * time.Second)(cfg)
	WithObserver(obs)(cfg)
	WithLogger(logger)(cfg)
	WithBackend("ollama")(cfg)
	WithVoyageAPIKey("sk-test")(cfg)
	WithOllamaDocPrefix("passage: ")(cfg)
	WithOllamaQueryPrefix("query: ")(cfg)
	WithOllamaDim(512)(cfg)

	if cfg.model != "my-model" {
		t.Errorf("model: want %q, got %q", "my-model", cfg.model)
	}
	if cfg.dim != 768 {
		t.Errorf("dim: want 768, got %d", cfg.dim)
	}
	if cfg.timeout != 5*time.Second {
		t.Errorf("timeout: want 5s, got %v", cfg.timeout)
	}
	if cfg.observer != obs {
		t.Errorf("observer: want countingObserver, got %T", cfg.observer)
	}
	if cfg.logger != logger {
		t.Errorf("logger: not applied")
	}
	if cfg.backend != "ollama" {
		t.Errorf("backend: want %q, got %q", "ollama", cfg.backend)
	}
	if cfg.voyageAPIKey != "sk-test" {
		t.Errorf("voyageAPIKey: want %q, got %q", "sk-test", cfg.voyageAPIKey)
	}
	if cfg.ollamaDocPrefix != "passage: " {
		t.Errorf("ollamaDocPrefix: want %q, got %q", "passage: ", cfg.ollamaDocPrefix)
	}
	if cfg.ollamaQueryPrefix != "query: " {
		t.Errorf("ollamaQueryPrefix: want %q, got %q", "query: ", cfg.ollamaQueryPrefix)
	}
	if cfg.ollamaDim != 512 {
		t.Errorf("ollamaDim: want 512, got %d", cfg.ollamaDim)
	}
}

// TestOpt_NilObserverIgnored verifies WithObserver(nil) keeps the noop active.
func TestOpt_NilObserverIgnored(t *testing.T) {
	cfg := defaultCfg()
	WithObserver(nil)(cfg)
	if _, ok := cfg.observer.(noopObserver); !ok {
		t.Errorf("nil WithObserver: want noop to stay, got %T", cfg.observer)
	}
}

// TestOpt_NilLoggerIgnored verifies WithLogger(nil) keeps cfg.logger nil
// (backends handle nil by falling back to slog.Default()).
func TestOpt_NilLoggerIgnored(t *testing.T) {
	cfg := defaultCfg()
	cfg.logger = testLogger() // set a non-nil logger first
	WithLogger(nil)(cfg)
	if cfg.logger == nil {
		t.Error("nil WithLogger: should preserve existing logger, not set to nil")
	}
}

// TestEmbedWithResult_ResultHasModel verifies that Result.Model is populated
// from the embedder when available.
func TestEmbedWithResult_ResultHasModel(t *testing.T) {
	want := [][]float32{{0.1, 0.2}}
	srv := mockHTTPServer(t, want, http.StatusOK)
	defer srv.Close()

	e := NewHTTPEmbedder(srv.URL, "my-model", 2, testLogger())
	res, err := EmbedWithResult(context.Background(), e, []string{"hello"})
	if err != nil {
		t.Fatalf("EmbedWithResult: %v", err)
	}
	if res.Model != "my-model" {
		t.Errorf("Model: want %q, got %q", "my-model", res.Model)
	}
}

// TestEmbedWithResult_EmptyVectors verifies emptyVectors helper.
func TestEmbedWithResult_EmptyVectors(t *testing.T) {
	vecs := emptyVectors(3)
	if len(vecs) != 3 {
		t.Fatalf("want 3, got %d", len(vecs))
	}
	for i, v := range vecs {
		if v.Status != StatusSkipped {
			t.Errorf("[%d] Status: want StatusSkipped, got %s", i, v.Status)
		}
		if v.Embedding != nil {
			t.Errorf("[%d] Embedding: want nil, got %v", i, v.Embedding)
		}
	}
}

// errEmbedder is a minimal Embedder that always returns an error.
type errEmbedder struct{ model string }

func (e *errEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("embed: backend unavailable")
}
func (e *errEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embed: backend unavailable")
}
func (e *errEmbedder) Dimension() int { return 1024 }
func (e *errEmbedder) Close() error   { return nil }

// namedStubEmbedder is a stub Embedder that also exposes Model() string.
// Simulates a caller-supplied embedder (e.g. onnx.Embedder) that advertises
// its model name via the optional interface.
type namedStubEmbedder struct {
	modelName string
	embedFn   func(context.Context, []string) ([][]float32, error)
}

func (s *namedStubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if s.embedFn != nil {
		return s.embedFn(ctx, texts)
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}
func (s *namedStubEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return EmbedQueryViaEmbed(ctx, s, text)
}
func (s *namedStubEmbedder) Dimension() int { return 3 }
func (s *namedStubEmbedder) Close() error   { return nil }
func (s *namedStubEmbedder) Model() string  { return s.modelName }

// TestNewClient_CustomEmbedder verifies that WithEmbedder bypasses backend
// factory dispatch entirely and returns the caller-supplied Embedder as-is.
func TestNewClient_CustomEmbedder(t *testing.T) {
	stub := &namedStubEmbedder{modelName: "custom-model"}
	e, err := NewClient("http://ignored-url",
		WithBackend("http"), // should be ignored when WithEmbedder is set
		WithEmbedder(stub),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if e != stub {
		t.Errorf("expected caller-supplied stub, got %T", e)
	}
}

// TestNewClient_OnnxLikeViaCustom simulates the ONNX usage pattern:
// caller builds an embedder externally (e.g. *onnx.Embedder from embed/onnx)
// and passes it via WithEmbedder. URL and backend are ignored.
func TestNewClient_OnnxLikeViaCustom(t *testing.T) {
	onnxLike := &namedStubEmbedder{modelName: "multilingual-e5-large"}

	e, err := NewClient("", // ONNX has no HTTP URL
		WithEmbedder(onnxLike),
		WithObserver(&countingObserver{}), // observer is applied to cfg but not wired in E0
	)
	if err != nil {
		t.Fatalf("NewClient with ONNX-like: %v", err)
	}
	if e != onnxLike {
		t.Errorf("expected onnxLike stub, got %T", e)
	}
	// Verify it works end-to-end: EmbedWithResult should succeed.
	res, err := EmbedWithResult(context.Background(), e, []string{"test"})
	if err != nil {
		t.Fatalf("EmbedWithResult: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status: want StatusOk, got %s", res.Status)
	}
	if res.Model != "multilingual-e5-large" {
		t.Errorf("Model: want %q, got %q", "multilingual-e5-large", res.Model)
	}
}

// TestNewClient_WithEmbedder_NilIgnored verifies that WithEmbedder(nil) is
// a no-op — backend dispatch proceeds normally.
func TestNewClient_WithEmbedder_NilIgnored(t *testing.T) {
	e, err := NewClient("http://embed:8082",
		WithEmbedder(nil), // must be ignored
		WithModel("multilingual-e5-large"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := e.(*HTTPEmbedder); !ok {
		t.Errorf("expected *HTTPEmbedder after nil WithEmbedder, got %T", e)
	}
}

// TestModelFromEmbedder_CustomEmbedderViaInterface verifies that an Embedder
// implementing Model() string has its name extracted via the interface path
// (not the type-switch path).
func TestModelFromEmbedder_CustomEmbedderViaInterface(t *testing.T) {
	stub := &namedStubEmbedder{modelName: "my-custom-model"}
	got := modelFromEmbedder(stub)
	if got != "my-custom-model" {
		t.Errorf("modelFromEmbedder via interface: want %q, got %q", "my-custom-model", got)
	}
}

// TestModelFromEmbedder_FakeEmbedder verifies that a plain Embedder without
// Model() returns empty string.
func TestModelFromEmbedder_FakeEmbedder(t *testing.T) {
	stub := &fakeEmbedder{dim: 1024}
	got := modelFromEmbedder(stub)
	if got != "" {
		t.Errorf("modelFromEmbedder for opaque type: want empty, got %q", got)
	}
}
