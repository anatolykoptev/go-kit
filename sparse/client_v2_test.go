package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fixedItem returns a single-text response for httptest servers below.
func fixedItem(idx int, ind uint32, val float32) httpSparseItem {
	return httpSparseItem{Index: idx, Indices: []uint32{ind}, Values: []float32{val}}
}

func successHandler(_ *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req httpSparseRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := make([]httpSparseItem, len(req.Input))
		for i := range req.Input {
			data[i] = fixedItem(i, uint32(i+1), float32(i)+0.5)
		}
		_ = json.NewEncoder(w).Encode(httpSparseResponse{Model: req.Model, Data: data})
	})
}

// TestNewClient_BasicEmbed verifies the v2 client wraps an
// HTTPSparseEmbedder and routes through the typed Result API.
func TestNewClient_BasicEmbed(t *testing.T) {
	srv := httptest.NewServer(successHandler(t))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithModel("splade-v3-distilbert"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	res, err := c.EmbedSparseWithResult(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedSparseWithResult: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: %s", res.Status)
	}
	if len(res.Vectors) != 2 {
		t.Errorf("vectors: %d", len(res.Vectors))
	}
}

// TestNewClient_EmbedSparse_Interface verifies the *Client satisfies
// SparseEmbedder via Embed → EmbedSparseWithResult routing.
func TestNewClient_EmbedSparse_Interface(t *testing.T) {
	srv := httptest.NewServer(successHandler(t))
	defer srv.Close()

	var emb SparseEmbedder
	c, err := NewClient(srv.URL, WithModel("splade-v3-distilbert"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	emb = c

	got, err := emb.EmbedSparse(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("EmbedSparse: %v", err)
	}
	if len(got) != 1 || got[0].Indices[0] != 1 {
		t.Errorf("got %+v", got)
	}
}

// TestNewClient_DryRun returns Skipped vectors without hitting the server.
func TestNewClient_DryRun(t *testing.T) {
	called := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		atomic.StoreInt32(&called, 1)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithModel("m"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	res, err := c.EmbedSparseWithResult(context.Background(), []string{"a"}, WithDryRun())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("status: %s", res.Status)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Error("server called on dry-run")
	}
}

// TestNewClient_EmptyTextsSkipped verifies len(texts)==0 returns
// StatusSkipped without hitting the backend.
func TestNewClient_EmptyTextsSkipped(t *testing.T) {
	c, err := NewClient("http://localhost", WithModel("m"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	res, err := c.EmbedSparseWithResult(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("status: %s", res.Status)
	}
}

// TestNewClient_CacheHit verifies the WithCache layer short-circuits the
// backend on a full-batch hit.
func TestNewClient_CacheHit(t *testing.T) {
	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		successHandler(t).ServeHTTP(w, r)
	}))
	defer srv.Close()

	cache := newMemCache()
	c, err := NewClient(srv.URL, WithModel("m"), WithCache(cache))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// First call: miss, populates cache.
	if _, err := c.EmbedSparseWithResult(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("first call should hit backend: %d", hits)
	}
	// Second call: full hit.
	if _, err := c.EmbedSparseWithResult(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("second call should not hit backend: %d", hits)
	}
}

// stubBackend lets us test fallback without spinning up two httptest
// servers.
type stubBackend struct {
	model string
	out   []SparseVector
	err   error
	calls int32
}

func (s *stubBackend) EmbedSparse(_ context.Context, texts []string) ([]SparseVector, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	if s.out != nil {
		return s.out, nil
	}
	return make([]SparseVector, len(texts)), nil
}
func (s *stubBackend) EmbedSparseQuery(_ context.Context, _ string) (SparseVector, error) {
	return SparseVector{}, nil
}
func (s *stubBackend) VocabSize() int { return 30522 }
func (s *stubBackend) Close() error   { return nil }
func (s *stubBackend) Model() string  { return s.model }

// TestFallback_PrimaryFailsSecondarySucceeds verifies StatusFallback.
func TestFallback_PrimaryFailsSecondarySucceeds(t *testing.T) {
	primaryStub := &stubBackend{model: "primary", err: errors.New("primary down")}
	secondaryStub := &stubBackend{model: "secondary", out: []SparseVector{{Indices: []uint32{1}, Values: []float32{0.5}}}}
	secondary, err := NewClient("", WithEmbedder(secondaryStub))
	if err != nil {
		t.Fatalf("secondary: %v", err)
	}
	primary, err := NewClient("", WithEmbedder(primaryStub), WithFallback(secondary), WithRetry(NoRetry))
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	res, err := primary.EmbedSparseWithResult(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("expected fallback success, got err: %v", err)
	}
	if res.Status != StatusFallback {
		t.Errorf("status: %s", res.Status)
	}
}

// TestFallback_4xxNoFallback verifies a 4xx error from the primary skips
// the secondary (caller error).
func TestFallback_4xxNoFallback(t *testing.T) {
	primaryStub := &stubBackend{model: "primary", err: &errHTTPStatus{Code: 400, Body: "bad"}}
	secondaryStub := &stubBackend{model: "secondary"}
	secondary, _ := NewClient("", WithEmbedder(secondaryStub))
	primary, _ := NewClient("", WithEmbedder(primaryStub), WithFallback(secondary), WithRetry(NoRetry))

	_, err := primary.EmbedSparseWithResult(context.Background(), []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&secondaryStub.calls) != 0 {
		t.Errorf("secondary should not be called on 4xx: %d", secondaryStub.calls)
	}
}

// TestNewClient_VocabSize verifies the helper threads the vocab size
// from the inner backend or the override.
func TestNewClient_VocabSize(t *testing.T) {
	c, err := NewClient("http://localhost", WithModel("m"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.VocabSize() != 30522 {
		t.Errorf("default: want 30522, got %d", c.VocabSize())
	}

	c2, err := NewClient("http://localhost", WithModel("m"), WithClientVocabSize(50000))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c2.VocabSize() != 50000 {
		t.Errorf("override: want 50000, got %d", c2.VocabSize())
	}
}

// observerSpy captures hook invocations.
type observerSpy struct {
	before, after, hit int32
}

func (o *observerSpy) OnBeforeEmbed(_ context.Context, _ string, _ int) {
	atomic.AddInt32(&o.before, 1)
}
func (o *observerSpy) OnAfterEmbed(_ context.Context, _ Status, _ time.Duration, _ int) {
	atomic.AddInt32(&o.after, 1)
}
func (o *observerSpy) OnRetry(_ context.Context, _ int, _ error)                {}
func (o *observerSpy) OnCircuitTransition(_ context.Context, _, _ CircuitState) {}
func (o *observerSpy) OnCacheHit(_ context.Context, _ int) {
	atomic.AddInt32(&o.hit, 1)
}

// TestNewClient_ObserverFires verifies before/after hooks fire on a normal
// embed.
func TestNewClient_ObserverFires(t *testing.T) {
	srv := httptest.NewServer(successHandler(t))
	defer srv.Close()

	spy := &observerSpy{}
	c, err := NewClient(srv.URL, WithModel("m"), WithObserver(spy))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.EmbedSparseWithResult(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if atomic.LoadInt32(&spy.before) != 1 || atomic.LoadInt32(&spy.after) != 1 {
		t.Errorf("hooks: before=%d after=%d", spy.before, spy.after)
	}
}

// TestNewClient_NilSafeMethods verifies nil-receiver safety.
func TestNewClient_NilSafeMethods(t *testing.T) {
	var c *Client
	if v, err := c.EmbedSparse(context.Background(), []string{"a"}); err != nil || v != nil {
		t.Errorf("nil-safe EmbedSparse: %v %v", v, err)
	}
	if v, err := c.EmbedSparseQuery(context.Background(), "a"); err != nil || !v.IsEmpty() {
		t.Errorf("nil-safe EmbedSparseQuery: %v %v", v, err)
	}
	if c.VocabSize() != 0 {
		t.Errorf("nil-safe VocabSize")
	}
	if c.Model() != "" {
		t.Errorf("nil-safe Model")
	}
	if err := c.Close(); err != nil {
		t.Errorf("nil-safe Close: %v", err)
	}
}

// TestNewClient_EmbedSparseEmptyReturnsNil verifies the v1 contract that
// Client.EmbedSparse(nil) and EmbedSparse([]) return (nil, nil) — never an
// allocated empty []SparseVector{}. Guards Fix 2: previously
// EmbedSparseWithResult returned StatusSkipped with nil Vectors but
// EmbedSparse converted that to make([]SparseVector, 0) — wrong.
func TestNewClient_EmbedSparseEmptyReturnsNil(t *testing.T) {
	c, err := NewClient("http://localhost", WithModel("m"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for _, name := range []string{"nil", "empty"} {
		t.Run(name, func(t *testing.T) {
			var input []string
			if name == "empty" {
				input = []string{}
			}
			got, err := c.EmbedSparse(context.Background(), input)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != nil {
				t.Errorf("want nil slice, got %v (len=%d)", got, len(got))
			}
		})
	}
}
