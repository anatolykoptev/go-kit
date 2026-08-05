package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestOllamaClient_Embed verifies the happy path: server returns embeddings for all inputs.
func TestOllamaClient_Embed(t *testing.T) {
	want := [][]float32{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/embed" {
			t.Errorf("expected /api/embed, got %s", r.URL.Path)
		}
		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}
		resp := ollamaEmbedResponse{Model: req.Model, Embeddings: want}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "nomic-embed-text", testLogger())
	got, err := c.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d embeddings, got %d", len(want), len(got))
	}
	for i, vec := range got {
		if len(vec) != len(want[i]) {
			t.Errorf("[%d] dim mismatch: want %d, got %d", i, len(want[i]), len(vec))
		}
	}
}

// TestOllamaClient_EmptyInput verifies that empty input returns nil without HTTP call.
func TestOllamaClient_EmptyInput(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "nomic-embed-text", testLogger())
	got, err := c.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if called {
		t.Error("HTTP server should not be called for empty input")
	}
}

// TestOllamaClient_ServerError verifies that non-200 responses return an error.
func TestOllamaClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "unknown-model", testLogger())
	_, err := c.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

// TestOllamaClient_CountMismatch verifies that mismatched embedding count returns error.
func TestOllamaClient_CountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaEmbedResponse{
			Model:      "nomic-embed-text",
			Embeddings: [][]float32{{0.1, 0.2}}, // only 1 embedding for 2 inputs
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "nomic-embed-text", testLogger())
	_, err := c.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
}

// TestOllamaClient_Dimension verifies default and overridden dimensions.
func TestOllamaClient_Dimension(t *testing.T) {
	c := NewOllamaClient("", "", testLogger())
	if c.Dimension() != ollamaDefaultDim {
		t.Errorf("default dim: want %d, got %d", ollamaDefaultDim, c.Dimension())
	}
	if ollamaDefaultDim != 1024 {
		t.Errorf("default dim must be 1024 to match pgvector schema, got %d", ollamaDefaultDim)
	}

	c2 := NewOllamaClient("", "", testLogger(), WithOllamaDimension(768))
	if c2.Dimension() != 768 {
		t.Errorf("override dim: want 768, got %d", c2.Dimension())
	}
}

// TestOllamaClient_Defaults verifies that empty URL/model use defaults.
func TestOllamaClient_Defaults(t *testing.T) {
	c := NewOllamaClient("", "", testLogger())
	if c.baseURL != ollamaDefaultURL {
		t.Errorf("default URL: want %q, got %q", ollamaDefaultURL, c.baseURL)
	}
	if c.model != ollamaDefaultModel {
		t.Errorf("default model: want %q, got %q", ollamaDefaultModel, c.model)
	}
}

// TestOllamaClient_TrailingSlash verifies that trailing slash in URL is stripped.
func TestOllamaClient_TrailingSlash(t *testing.T) {
	c := NewOllamaClient("http://ollama:11434/", "nomic-embed-text", testLogger())
	if c.baseURL != "http://ollama:11434" {
		t.Errorf("trailing slash not stripped: %q", c.baseURL)
	}
}

// TestOllamaClient_Timeout verifies that WithOllamaTimeout applies correctly.
func TestOllamaClient_Timeout(t *testing.T) {
	c := NewOllamaClient("", "", testLogger(), WithOllamaTimeout(5*time.Second))
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout: want 5s, got %v", c.httpClient.Timeout)
	}
}

// TestOllamaClient_Close verifies that Close is a no-op.
func TestOllamaClient_Close(t *testing.T) {
	c := NewOllamaClient("", "", testLogger())
	if err := c.Close(); err != nil {
		t.Errorf("Close should return nil, got %v", err)
	}
}

// TestOllamaClient_TextPrefix verifies that WithTextPrefix prepends to every input.
func TestOllamaClient_TextPrefix(t *testing.T) {
	var capturedInputs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		capturedInputs = req.Input
		resp := ollamaEmbedResponse{
			Embeddings: [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "test", testLogger(), WithTextPrefix("passage: "))
	_, err := c.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(capturedInputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(capturedInputs))
	}
	if capturedInputs[0] != "passage: hello" {
		t.Errorf("input[0]: want %q, got %q", "passage: hello", capturedInputs[0])
	}
	if capturedInputs[1] != "passage: world" {
		t.Errorf("input[1]: want %q, got %q", "passage: world", capturedInputs[1])
	}
}

// TestOllamaClient_EmptyPrefix verifies that empty prefix sends texts unchanged.
func TestOllamaClient_EmptyPrefix(t *testing.T) {
	var capturedInputs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaEmbedRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		capturedInputs = req.Input
		resp := ollamaEmbedResponse{Embeddings: [][]float32{{0.1}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "test", testLogger(), WithTextPrefix(""))
	_, err := c.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if capturedInputs[0] != "hello" {
		t.Errorf("empty prefix should not modify input: got %q", capturedInputs[0])
	}
}

// TestOllamaClient_NormalizeL2 verifies that WithNormalizeL2 produces unit vectors.
func TestOllamaClient_NormalizeL2(t *testing.T) {
	// Return a non-normalized vector [3, 4] — L2 norm = 5, normalized = [0.6, 0.8]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaEmbedResponse{
			Embeddings: [][]float32{{3.0, 4.0}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "test", testLogger(), WithNormalizeL2(true))
	got, err := c.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}

	vec := got[0]
	// Check unit length: sum of squares should be ~1.0
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	if sumSq < 0.999 || sumSq > 1.001 {
		t.Errorf("L2 norm: want ~1.0, got %f (vec=%v)", sumSq, vec)
	}
	// Check values: [3/5, 4/5] = [0.6, 0.8]
	if vec[0] < 0.599 || vec[0] > 0.601 {
		t.Errorf("vec[0]: want ~0.6, got %f", vec[0])
	}
	if vec[1] < 0.799 || vec[1] > 0.801 {
		t.Errorf("vec[1]: want ~0.8, got %f", vec[1])
	}
}

// TestOllamaClient_NormalizeL2_Disabled verifies that without WithNormalizeL2
// the raw (non-unit) vector is returned as-is.
func TestOllamaClient_NormalizeL2_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaEmbedResponse{
			Embeddings: [][]float32{{3.0, 4.0}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewOllamaClient(srv.URL, "test", testLogger()) // no WithNormalizeL2
	got, err := c.Embed(context.Background(), []string{"test"})
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if got[0][0] != 3.0 || got[0][1] != 4.0 {
		t.Errorf("without normalize: want [3 4], got %v", got[0])
	}
}

// TestOllamaClient_E5PrefixesNotInterchangeable is the regression guard for
// issue #259: the e5 "passage: "/"query: " prefixes and raw text produce
// vectors in different regions of the space, and a caller that stores raw-text
// embeddings against a passage-prefixed corpus silently degrades retrieval
// (cosine ~0.97 — high enough to look fine, low enough to reorder results).
//
// The mock server simulates the e5 model's real behaviour: it returns a
// deterministic, prefix-distinct vector per distinct input string. We then
// assert that the same text embedded once with E5PassagePrefix and once with
// no prefix yields cosine < 1.0 — i.e. the two are NOT interchangeable. Had
// this test existed when the misleading doc comment was written, the drift
// would have been caught at the point it was introduced.
func TestOllamaClient_E5PrefixesNotInterchangeable(t *testing.T) {
	// mockE5Vec returns a stable, well-separated unit vector per distinct input
	// string. Two inputs that differ only by prefix must land in different
	// regions — that is the property we are proving is enforced.
	mockE5Vec := func(input string) []float32 {
		// Hash the input into two float32 coordinates in [-1, 1], then L2-normalize.
		// Distinct inputs → distinct directions; identical inputs → identical vector.
		var h1, h2 uint32
		for i := 0; i < len(input); i++ {
			h1 = h1*31 + uint32(input[i])
			h2 = h2*37 + uint32(input[i])
		}
		v := []float32{
			float32(int32(h1%2000)-1000) / 1000.0,
			float32(int32(h2%2000)-1000) / 1000.0,
		}
		l2Normalize(v)
		return v
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		embs := make([][]float32, len(req.Input))
		for i, in := range req.Input {
			embs[i] = mockE5Vec(in)
		}
		resp := ollamaEmbedResponse{Embeddings: embs}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	const fixedText = "Senior Go engineer with distributed-systems experience"

	// Embed the SAME text three ways: passage-prefixed (storage convention),
	// query-prefixed (retrieval convention), and raw (the old, wrong advice).
	passageC := NewOllamaClient(srv.URL, "multilingual-e5-large", testLogger(), WithTextPrefix(E5PassagePrefix))
	queryC := NewOllamaClient(srv.URL, "multilingual-e5-large", testLogger(), WithTextPrefix(E5QueryPrefix))
	rawC := NewOllamaClient(srv.URL, "multilingual-e5-large", testLogger()) // no prefix — the drift

	passageVec, err := passageC.Embed(context.Background(), []string{fixedText})
	if err != nil {
		t.Fatalf("passage Embed: %v", err)
	}
	queryVec, err := queryC.Embed(context.Background(), []string{fixedText})
	if err != nil {
		t.Fatalf("query Embed: %v", err)
	}
	rawVec, err := rawC.Embed(context.Background(), []string{fixedText})
	if err != nil {
		t.Fatalf("raw Embed: %v", err)
	}

	// The three prefixes must produce three distinct vectors — if any two
	// collide, the mock is not discriminating enough to prove the property.
	if Cosine(passageVec[0], rawVec[0]) >= 0.999 {
		t.Fatalf("mock not discriminating: passage vs raw cosine = %f (need <0.999 to prove the property)",
			Cosine(passageVec[0], rawVec[0]))
	}

	// The actual regression assertion: passage-prefixed and raw embeddings of
	// the same text are NOT interchangeable. This is the exact drift issue #259
	// describes — a caller following the old "raw text" advice writes vectors
	// that do not match the passage-prefixed corpus.
	if got := Cosine(passageVec[0], rawVec[0]); got >= 0.999 {
		t.Errorf("passage vs raw cosine = %f, want < 0.999 — prefixes are NOT interchangeable; "+
			"raw-text embeddings cannot retrieve against a passage-prefixed e5 corpus", got)
	}
	// And query vs passage are distinct too — they are asymmetric by design.
	if got := Cosine(passageVec[0], queryVec[0]); got >= 0.999 {
		t.Errorf("passage vs query cosine = %f, want < 0.999 — e5 query/passage prefixes are asymmetric", got)
	}
}

// TestOllamaClient_E5PrefixConstantsAreStable guards against accidental edits
// to the exported prefix constants — a change here is a corpus-compatibility
// break, not a refactor.
func TestOllamaClient_E5PrefixConstantsAreStable(t *testing.T) {
	if E5PassagePrefix != "passage: " {
		t.Errorf("E5PassagePrefix = %q, want %q — changing this breaks existing e5 corpora", E5PassagePrefix, "passage: ")
	}
	if E5QueryPrefix != "query: " {
		t.Errorf("E5QueryPrefix = %q, want %q — changing this breaks existing e5 query convention", E5QueryPrefix, "query: ")
	}
}

// TestOllamaClient_E5ModelWarnsOnEmptyPrefix verifies that constructing an
// OllamaClient with an e5-family model name and no prefixes emits the
// silent-degradation warning from issue #259. The warning is informational,
// not a hard error — the caller may intentionally use raw text.
func TestOllamaClient_E5ModelWarnsOnEmptyPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// e5 model + no prefixes → should warn.
	_ = NewOllamaClient("http://x", "multilingual-e5-large", logger)
	out := buf.String()
	if !strings.Contains(out, "e5 family") || !strings.Contains(out, "quietly degrade") {
		t.Errorf("e5 model + empty prefixes: expected warning about e5 family / quiet degradation, got: %s", out)
	}

	// e5 model + passage prefix set → should NOT warn.
	buf.Reset()
	_ = NewOllamaClient("http://x", "multilingual-e5-large", logger, WithTextPrefix(E5PassagePrefix))
	if strings.Contains(buf.String(), "quietly degrade") {
		t.Errorf("e5 model + passage prefix set: should not warn, got: %s", buf.String())
	}

	// non-e5 model + no prefixes → should NOT warn.
	buf.Reset()
	_ = NewOllamaClient("http://x", "nomic-embed-text", logger)
	if strings.Contains(buf.String(), "e5 family") {
		t.Errorf("non-e5 model: should not warn about e5 family, got: %s", buf.String())
	}
}
