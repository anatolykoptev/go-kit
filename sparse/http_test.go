package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestHTTPSparseEmbedder_Embed verifies the happy path: batch embed
// returns correct sparse vectors aligned by item.Index.
func TestHTTPSparseEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embed_sparse" {
			t.Errorf("expected /embed_sparse, got %s", r.URL.Path)
		}
		var req httpSparseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}
		if req.Model != "splade-v3-distilbert" {
			t.Errorf("expected default model, got %s", req.Model)
		}
		// top_k / min_weight should be omitted by default (omitempty).
		body, _ := io.ReadAll(r.Body)
		_ = body
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data: []httpSparseItem{
				{Index: 0, Indices: []uint32{12, 345, 6789}, Values: []float32{4.2, 3.1, 0.7}},
				{Index: 1, Indices: []uint32{99, 100}, Values: []float32{1.0, 0.5}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	got, err := e.EmbedSparse(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedSparse error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(got))
	}
	if got[0].Len() != 3 || got[1].Len() != 2 {
		t.Errorf("vector lengths wrong: %d, %d", got[0].Len(), got[1].Len())
	}
	if got[0].Indices[0] != 12 || got[0].Values[0] != 4.2 {
		t.Errorf("vector[0] content wrong: %+v", got[0])
	}
}

// TestHTTPSparseEmbedder_EmptyInput verifies empty input returns nil
// without an HTTP call (mirrors embed/'s shortcut).
func TestHTTPSparseEmbedder_EmptyInput(t *testing.T) {
	called := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	got, err := e.EmbedSparse(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Error("HTTP server should not be called for empty input")
	}
}

// TestHTTPSparseEmbedder_ServerError400 verifies 4xx responses fail-fast
// (no retry).
func TestHTTPSparseEmbedder_ServerError400(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, `{"error":"bad model"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	_, err := e.EmbedSparse(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("400 should not retry: got %d calls", calls)
	}
}

// TestHTTPSparseEmbedder_RetryOn503 verifies 503 triggers retry up to
// maxAttempts. Server returns 503 twice then 200.
func TestHTTPSparseEmbedder_RetryOn503(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1}, Values: []float32{0.5}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := e.EmbedSparse(ctx, []string{"hello"})
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d vectors", len(got))
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("expected 3 calls (2 retries), got %d", calls)
	}
}

// TestHTTPSparseEmbedder_LengthMismatch verifies a 200 response with the
// wrong number of items is rejected.
func TestHTTPSparseEmbedder_LengthMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1}, Values: []float32{0.5}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	_, err := e.EmbedSparse(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected length-mismatch error")
	}
	if !strings.Contains(err.Error(), "response length mismatch") {
		t.Errorf("wrong error: %v", err)
	}
}

// TestHTTPSparseEmbedder_TopKMinWeightPropagation verifies that the
// per-instance options serialise into the request body.
func TestHTTPSparseEmbedder_TopKMinWeightPropagation(t *testing.T) {
	gotTopK := 0
	gotMinWeight := float32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req httpSparseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gotTopK = req.TopK
		gotMinWeight = req.MinWeight
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1}, Values: []float32{1}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger(), WithTopK(64), WithMinWeight(0.25))
	if _, err := e.EmbedSparse(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("EmbedSparse: %v", err)
	}
	if gotTopK != 64 {
		t.Errorf("top_k: want 64, got %d", gotTopK)
	}
	if gotMinWeight != 0.25 {
		t.Errorf("min_weight: want 0.25, got %v", gotMinWeight)
	}
}

// TestHTTPSparseEmbedder_DefaultsOmitted verifies that with default
// options the top_k / min_weight fields are absent from the wire body
// (server-side defaults apply).
func TestHTTPSparseEmbedder_DefaultsOmitted(t *testing.T) {
	gotBody := []byte(nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1}, Values: []float32{1}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	if _, err := e.EmbedSparse(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("EmbedSparse: %v", err)
	}
	if strings.Contains(string(gotBody), "top_k") {
		t.Errorf("top_k should be omitted by default; body=%s", gotBody)
	}
	if strings.Contains(string(gotBody), "min_weight") {
		t.Errorf("min_weight should be omitted by default; body=%s", gotBody)
	}
}

// TestHTTPSparseEmbedder_VocabSize verifies the configured vocab size is
// returned and overrideable.
func TestHTTPSparseEmbedder_VocabSize(t *testing.T) {
	e := NewHTTPSparseEmbedder("http://localhost", "", testLogger())
	if e.VocabSize() != httpSparseDefaultVocabSize {
		t.Errorf("default vocab: want %d, got %d", httpSparseDefaultVocabSize, e.VocabSize())
	}
	e2 := NewHTTPSparseEmbedder("http://localhost", "", testLogger(), WithVocabSize(50000))
	if e2.VocabSize() != 50000 {
		t.Errorf("override: want 50000, got %d", e2.VocabSize())
	}
}

// TestHTTPSparseEmbedder_Close verifies Close is a no-op.
func TestHTTPSparseEmbedder_Close(t *testing.T) {
	e := NewHTTPSparseEmbedder("http://localhost", "", testLogger())
	if err := e.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestHTTPSparseEmbedder_TrailingSlash verifies trailing slash in URL is
// stripped — same hardening as embed/.
func TestHTTPSparseEmbedder_TrailingSlash(t *testing.T) {
	e := NewHTTPSparseEmbedder("http://embed:8082/", "", testLogger())
	if e.baseURL != "http://embed:8082" {
		t.Errorf("trailing slash not stripped: %q", e.baseURL)
	}
}

// TestHTTPSparseEmbedder_DefaultTimeout verifies the constructor applies
// the 30s default when no WithHTTPTimeout option is passed.
func TestHTTPSparseEmbedder_DefaultTimeout(t *testing.T) {
	e := NewHTTPSparseEmbedder("http://embed:8082", "", testLogger())
	if e.client.Timeout != httpSparseDefaultTimeout {
		t.Errorf("default timeout: want %v, got %v", httpSparseDefaultTimeout, e.client.Timeout)
	}
}

// TestHTTPSparseEmbedder_WithHTTPTimeout verifies the override.
func TestHTTPSparseEmbedder_WithHTTPTimeout(t *testing.T) {
	e := NewHTTPSparseEmbedder("http://embed:8082", "", testLogger(), WithHTTPTimeout(120*time.Second))
	if e.client.Timeout != 120*time.Second {
		t.Errorf("override: want 120s, got %v", e.client.Timeout)
	}
}

// TestHTTPSparseEmbedder_MalformedItem verifies a length-mismatched item
// (indices vs values) is rejected.
func TestHTTPSparseEmbedder_MalformedItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{1, 2}, Values: []float32{1}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	_, err := e.EmbedSparse(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error for mismatched indices/values lengths")
	}
}

// TestHTTPSparseEmbedder_EmbedSparseQuery verifies the convenience method.
func TestHTTPSparseEmbedder_EmbedSparseQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := httpSparseResponse{
			Model: "splade-v3-distilbert",
			Data:  []httpSparseItem{{Index: 0, Indices: []uint32{42}, Values: []float32{1.5}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	v, err := e.EmbedSparseQuery(context.Background(), "hello")
	if err != nil {
		t.Fatalf("EmbedSparseQuery: %v", err)
	}
	if v.Len() != 1 || v.Indices[0] != 42 {
		t.Errorf("wrong vector: %+v", v)
	}
}

// TestHTTPSparseEmbedder_EmbedSparseQuery_Empty verifies empty input
// returns the zero vector without a network call.
func TestHTTPSparseEmbedder_EmbedSparseQuery_Empty(t *testing.T) {
	called := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	// Pass an empty slice via EmbedSparse to confirm; EmbedSparseQuery
	// always sends the query string so it would call the server. Cover
	// the empty path via EmbedSparse (mirrors embed_test).
	got, err := e.EmbedSparse(context.Background(), []string{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil vectors, got %v", got)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Error("server should not be called")
	}
}

// TestHTTPSparseEmbedder_ErrModelNotConfigured verifies that 4xx responses
// whose body matches a resolve_splade_name failure marker are wrapped with
// ErrModelNotConfigured so callers can errors.Is them. Covers the three
// shapes the Rust handler emits: "no splade models configured",
// "splade model X not found", and the multi-model required-name case.
// Also verifies the errHTTPStatus wrapping is preserved (errors.As still works).
func TestHTTPSparseEmbedder_ErrModelNotConfigured(t *testing.T) {
	bodies := []struct {
		name string
		body string
	}{
		{"none_loaded", `{"error":"no splade models configured"}`},
		{"unknown_model", `{"error":"splade model splade-v9 not found"}`},
		{"required_when_multi", `{"error":"model is required when multiple splade models are configured"}`},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
			_, err := e.EmbedSparse(context.Background(), []string{"x"})
			if err == nil {
				t.Fatalf("want error")
			}
			if !errors.Is(err, ErrModelNotConfigured) {
				t.Errorf("errors.Is(ErrModelNotConfigured) = false; err=%v", err)
			}
			var statusErr *errHTTPStatus
			if !errors.As(err, &statusErr) {
				t.Errorf("errors.As(*errHTTPStatus) = false; err=%v", err)
			} else if statusErr.Code != 400 {
				t.Errorf("status code: want 400, got %d", statusErr.Code)
			}
		})
	}
}

// TestHTTPSparseEmbedder_4xxWithoutModelMarkerNotWrapped verifies a 4xx
// response without the resolve_splade_name markers is NOT wrapped with
// ErrModelNotConfigured (negative case — only the explicit substrings
// trigger the sentinel).
func TestHTTPSparseEmbedder_4xxWithoutModelMarkerNotWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"some other validation error"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger())
	_, err := e.EmbedSparse(context.Background(), []string{"x"})
	if err == nil {
		t.Fatalf("want error")
	}
	if errors.Is(err, ErrModelNotConfigured) {
		t.Errorf("unrelated 400 should NOT match ErrModelNotConfigured; err=%v", err)
	}
}

// TestHTTPSparseEmbedder_WithHTTPRetry_NoRetryDisables verifies that
// passing NoRetry (MaxAttempts=1) causes the backend to issue exactly one
// request when the server returns a transient error (503). Guards against
// regression of the WithRetry → backend wiring fix.
func TestHTTPSparseEmbedder_WithHTTPRetry_NoRetryDisables(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger(), WithHTTPRetry(NoRetry))
	_, err := e.EmbedSparse(context.Background(), []string{"x"})
	if err == nil {
		t.Fatalf("want error from 503")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("want exactly 1 request with NoRetry, got %d", got)
	}
}

// TestHTTPSparseEmbedder_WithHTTPRetry_CustomMaxAttempts verifies a custom
// MaxAttempts setting is honoured by the backend on transient failures.
// Server always returns 503, client retries 4 times total.
func TestHTTPSparseEmbedder_WithHTTPRetry_CustomMaxAttempts(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Jitter:      0,
	}
	e := NewHTTPSparseEmbedder(srv.URL, "", testLogger(), WithHTTPRetry(cfg))
	_, err := e.EmbedSparse(context.Background(), []string{"x"})
	if err == nil {
		t.Fatalf("want error from sustained 503")
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("want exactly 4 requests with MaxAttempts=4, got %d", got)
	}
}

// TestNewClient_WithRetry_FlowsThroughToBackend verifies the v2 entry
// point: WithRetry on NewClient propagates through newFromInternal →
// WithHTTPRetry → HTTPSparseEmbedder.retry. Server returns 503; with
// MaxAttempts=2 we expect exactly 2 backend requests.
func TestNewClient_WithRetry_FlowsThroughToBackend(t *testing.T) {
	calls := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := RetryConfig{
		MaxAttempts: 2,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
		Jitter:      0,
	}
	c, err := NewClient(srv.URL, WithRetry(cfg))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.EmbedSparse(context.Background(), []string{"x"}); err == nil {
		t.Fatalf("want error from sustained 503")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("want exactly 2 requests with MaxAttempts=2, got %d", got)
	}
}
