package rerank

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newJinaTestClient wires a JinaRerankClient at the given mock URL.
// We can't change the real endpoint constant, so we point httpClient at a
// custom RoundTripper that rewrites the destination to the test server.
func newJinaTestClient(t *testing.T, apiKey, model, mockURL string) *JinaRerankClient {
	t.Helper()
	c := NewJinaRerankClient(apiKey, model, nil)
	// Give each test its own transport so idle keep-alive connections from
	// one test don't bleed into another and so we can close them cleanly.
	tr := &http.Transport{}
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: mockURL, inner: tr},
		Timeout:   2 * time.Second,
	}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})
	return c
}

// rewriteTransport routes every request to a fixed target host while
// preserving method, headers, and body. Used so tests can mount an
// httptest.Server in front of jinaEndpoint.
type rewriteTransport struct {
	target string
	inner  http.RoundTripper
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	dst, err := http.NewRequestWithContext(req.Context(), req.Method, r.target, req.Body)
	if err != nil {
		return nil, err
	}
	dst.Header = req.Header.Clone()
	return r.inner.RoundTrip(dst)
}

func TestJina_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("missing/wrong Authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("wrong Content-Type: %q", got)
		}
		var body jinaRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "jina-reranker-v2-base-multilingual" {
			t.Errorf("unexpected model: %q", body.Model)
		}
		if body.Query != "best laptops for go developers" {
			t.Errorf("unexpected query: %q", body.Query)
		}
		if len(body.Documents) != 3 {
			t.Errorf("expected 3 docs, got %d", len(body.Documents))
		}
		if body.ReturnDocuments {
			t.Errorf("return_documents must be false")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jinaResponse{
			Model: "jina-reranker-v2-base-multilingual",
			Results: []jinaResult{
				{Index: 2, RelevanceScore: 0.91},
				{Index: 0, RelevanceScore: 0.55},
				{Index: 1, RelevanceScore: 0.12},
			},
		})
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "secret-key", "", srv.URL)
	docs := []Doc{
		{ID: "a", Text: "Go is great"},
		{ID: "b", Text: "Python notebooks"},
		{ID: "c", Text: "Best Go laptops 2025"},
	}

	res, err := c.RerankWithResult(context.Background(), "best laptops for go developers", docs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Status != StatusOk {
		t.Fatalf("expected StatusOk, got %v", res.Status)
	}
	if res.Model != "jina-reranker-v2-base-multilingual" {
		t.Errorf("unexpected model: %q", res.Model)
	}
	if len(res.Scored) != 3 {
		t.Fatalf("expected 3 scored, got %d", len(res.Scored))
	}
	// Top result must be index 2 ("c").
	if res.Scored[0].Doc.ID != "c" {
		t.Errorf("expected top doc id=c, got %q", res.Scored[0].Doc.ID)
	}
	// OrigRank carries the original input index.
	if res.Scored[0].OrigRank != 2 {
		t.Errorf("expected OrigRank=2, got %d", res.Scored[0].OrigRank)
	}
	// Scores must be descending.
	for i := 1; i < len(res.Scored); i++ {
		if res.Scored[i-1].Score < res.Scored[i].Score {
			t.Errorf("scores not descending at %d: %f < %f", i, res.Scored[i-1].Score, res.Scored[i].Score)
		}
	}
}

func TestJina_EmptyDocs(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	res, err := c.RerankWithResult(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("expected StatusSkipped, got %v", res.Status)
	}
	if called {
		t.Errorf("API must not be called for empty docs")
	}
}

func TestJina_EmptyAPIKey(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "", "", srv.URL)
	if c.Available() {
		t.Errorf("Available() must be false when apiKey is empty")
	}
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{Text: "x"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("expected StatusSkipped, got %v", res.Status)
	}
	if called {
		t.Errorf("API must not be called when apiKey is empty")
	}
	// Passthrough preserves input order.
	if len(res.Scored) != 1 || res.Scored[0].OrigRank != 0 {
		t.Errorf("unexpected passthrough: %+v", res.Scored)
	}
}

func TestJina_429RetrySucceed(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jinaResponse{
			Model: "jina-reranker-v2-base-multilingual",
			Results: []jinaResult{
				{Index: 0, RelevanceScore: 0.5},
			},
		})
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "t"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != StatusOk {
		t.Fatalf("expected StatusOk after retry, got %v", res.Status)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

func TestJina_5xxPersistent(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"service down"}`)
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "t"}})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if res == nil || res.Status != StatusDegraded {
		t.Fatalf("expected StatusDegraded, got %+v", res)
	}
	if res.Err == nil {
		t.Errorf("expected Result.Err to be populated")
	}
	if got := atomic.LoadInt32(&attempts); got != int32(jinaMaxAttempts) {
		t.Errorf("expected %d attempts, got %d", jinaMaxAttempts, got)
	}
	// Passthrough must preserve input order on degraded result.
	if len(res.Scored) != 1 || res.Scored[0].OrigRank != 0 {
		t.Errorf("expected passthrough on degraded, got %+v", res.Scored)
	}
}

func TestJina_4xxFailFast(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "wrong-key", "", srv.URL)
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "t"}})
	if err == nil {
		t.Fatal("expected error on 4xx")
	}
	if res == nil || res.Status != StatusDegraded {
		t.Fatalf("expected StatusDegraded, got %+v", res)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("expected fail-fast (1 attempt) on 4xx, got %d", got)
	}
	var hErr *jinaHTTPError
	if !errors.As(err, &hErr) {
		t.Errorf("expected *jinaHTTPError, got %T", err)
	} else if hErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", hErr.StatusCode)
	}
}

func TestJina_ContextCancellation(t *testing.T) {
	// release lets the test signal the handler to return cleanly so the
	// httptest server's WaitGroup unblocks during Close().
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	c := newJinaTestClient(t, "k", "", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := c.RerankWithResult(ctx, "q", []Doc{{ID: "a", Text: "t"}})
	if err == nil {
		t.Fatal("expected ctx error")
	}
	if res == nil || res.Status != StatusDegraded {
		t.Fatalf("expected StatusDegraded on ctx cancel, got %+v", res)
	}
}

func TestJina_WithTopN(t *testing.T) {
	var got jinaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jinaResponse{
			Model: "jina-reranker-v2-base-multilingual",
			Results: []jinaResult{
				{Index: 0, RelevanceScore: 0.9},
				{Index: 1, RelevanceScore: 0.8},
			},
		})
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	docs := []Doc{
		{ID: "a", Text: "x"}, {ID: "b", Text: "y"}, {ID: "c", Text: "z"},
	}
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithTopN(2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != StatusOk {
		t.Fatalf("expected StatusOk, got %v", res.Status)
	}
	if got.TopN != 2 {
		t.Errorf("expected top_n=2 in body, got %d", got.TopN)
	}
}

func TestJina_WithTopN_OmittedWhenZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		// `omitempty` on TopN means the literal key "top_n" must be absent
		// from the JSON when no WithTopN was passed.
		if strings.Contains(string(buf), `"top_n"`) {
			t.Errorf("top_n must be omitted when zero, got body: %s", string(buf))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jinaResponse{
			Results: []jinaResult{{Index: 0, RelevanceScore: 1}},
		})
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	_, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestJina_DryRun(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}}, WithDryRun())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("expected StatusSkipped on dry-run, got %v", res.Status)
	}
	if called {
		t.Errorf("API must not be called on dry-run")
	}
}

func TestJina_RerankShimReturnsScored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jinaResponse{
			Results: []jinaResult{
				{Index: 1, RelevanceScore: 0.9},
				{Index: 0, RelevanceScore: 0.1},
			},
		})
	}))
	defer srv.Close()

	c := newJinaTestClient(t, "k", "", srv.URL)
	out := c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "1"}, {ID: "b", Text: "2"}})
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	if out[0].Doc.ID != "b" {
		t.Errorf("expected top id=b, got %q", out[0].Doc.ID)
	}
}

func TestJina_AvailableNilSafe(t *testing.T) {
	var c *JinaRerankClient
	if c.Available() {
		t.Errorf("nil client must not be Available")
	}
}
