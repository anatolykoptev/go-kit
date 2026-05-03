package rerank

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// voyageRedirectTransport intercepts outgoing requests targeting the real
// Voyage endpoint and routes them to a local httptest server instead.
type voyageRedirectTransport struct {
	targetURL string
	base      http.RoundTripper
}

func (t *voyageRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	parsed := strings.TrimPrefix(t.targetURL, "http://")
	req.URL.Scheme = "http"
	req.URL.Host = parsed
	return t.base.RoundTrip(req)
}

// newVoyageRerankTestClient builds a VoyageRerankClient routed at the supplied
// httptest handler. Retry timing is shortened to keep tests fast.
func newVoyageRerankTestClient(t *testing.T, handler http.Handler) *VoyageRerankClient {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	c := NewVoyageRerankClient("test-api-key", "rerank-2.5", slog.Default())
	c.httpClient = &http.Client{
		Transport: &voyageRedirectTransport{
			targetURL: ts.URL,
			base:      http.DefaultTransport,
		},
	}
	return c
}

func TestVoyageRerank_HappyPath(t *testing.T) {
	var capturedAuth, capturedCT string
	var capturedBody voyageRerankRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedCT = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}

		// Voyage returns sorted DESC by relevance_score.
		resp := voyageRerankResponse{
			Object: "list",
			Data: []voyageRerankResult{
				{Index: 2, RelevanceScore: 0.91},
				{Index: 0, RelevanceScore: 0.55},
				{Index: 1, RelevanceScore: 0.10},
			},
			Model: "rerank-2.5",
		}
		resp.Usage.TotalTokens = 42
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})

	c := newVoyageRerankTestClient(t, handler)
	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Status != StatusOk {
		t.Errorf("Status = %v, want StatusOk", res.Status)
	}
	if res.Model != "rerank-2.5" {
		t.Errorf("Model = %q, want %q", res.Model, "rerank-2.5")
	}
	if capturedAuth != "Bearer test-api-key" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer test-api-key")
	}
	if capturedCT != "application/json" {
		t.Errorf("Content-Type = %q, want %q", capturedCT, "application/json")
	}
	if capturedBody.Model != "rerank-2.5" {
		t.Errorf("request model = %q, want %q", capturedBody.Model, "rerank-2.5")
	}
	if capturedBody.Query != "q" {
		t.Errorf("request query = %q, want %q", capturedBody.Query, "q")
	}
	if !capturedBody.Truncation {
		t.Error("request truncation = false, want true")
	}
	if capturedBody.ReturnDocuments {
		t.Error("request return_documents = true, want false")
	}
	if capturedBody.TopK != nil {
		t.Errorf("request top_k = %v, want nil (no WithTopN passed)", capturedBody.TopK)
	}
	if len(capturedBody.Documents) != 3 {
		t.Fatalf("documents length = %d, want 3", len(capturedBody.Documents))
	}

	// Result order matches Voyage's server-returned order (sorted desc).
	if len(res.Scored) != 3 {
		t.Fatalf("Scored length = %d, want 3", len(res.Scored))
	}
	wantIDs := []string{"c", "a", "b"}
	wantOrigRank := []int{2, 0, 1}
	wantScores := []float32{0.91, 0.55, 0.10}
	for i, s := range res.Scored {
		if s.ID != wantIDs[i] {
			t.Errorf("pos %d ID = %q, want %q", i, s.ID, wantIDs[i])
		}
		if s.OrigRank != wantOrigRank[i] {
			t.Errorf("pos %d OrigRank = %d, want %d", i, s.OrigRank, wantOrigRank[i])
		}
		if s.Score != wantScores[i] {
			t.Errorf("pos %d Score = %v, want %v", i, s.Score, wantScores[i])
		}
	}
}

func TestVoyageRerank_RerankWrapperDropsResult(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := voyageRerankResponse{
			Object: "list",
			Data: []voyageRerankResult{
				{Index: 0, RelevanceScore: 0.5},
			},
			Model: "rerank-2.5",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	c := newVoyageRerankTestClient(t, handler)

	got := c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "alpha"}})
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("Rerank returned %+v", got)
	}
	if got[0].Score != 0.5 {
		t.Errorf("Score = %v, want 0.5", got[0].Score)
	}
}

func TestVoyageRerank_EmptyDocs_Skipped(t *testing.T) {
	var called atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	c := newVoyageRerankTestClient(t, handler)

	res, err := c.RerankWithResult(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status = %v, want StatusSkipped", res.Status)
	}
	if len(res.Scored) != 0 {
		t.Errorf("Scored length = %d, want 0", len(res.Scored))
	}
	if called.Load() != 0 {
		t.Errorf("HTTP server called %d times, want 0", called.Load())
	}
}

func TestVoyageRerank_EmptyAPIKey_Skipped(t *testing.T) {
	var called atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	c := NewVoyageRerankClient("", "rerank-2.5", slog.Default())
	c.httpClient = &http.Client{
		Transport: &voyageRedirectTransport{targetURL: ts.URL, base: http.DefaultTransport},
	}

	if c.Available() {
		t.Error("Available() = true with empty apiKey, want false")
	}

	docs := []Doc{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status = %v, want StatusSkipped", res.Status)
	}
	// Passthrough preserves order with Score=0 and OrigRank=i.
	if len(res.Scored) != 2 {
		t.Fatalf("Scored length = %d, want 2", len(res.Scored))
	}
	if res.Scored[0].ID != "a" || res.Scored[0].OrigRank != 0 || res.Scored[0].Score != 0 {
		t.Errorf("Scored[0] = %+v, want passthrough a/0/0", res.Scored[0])
	}
	if res.Scored[1].ID != "b" || res.Scored[1].OrigRank != 1 || res.Scored[1].Score != 0 {
		t.Errorf("Scored[1] = %+v, want passthrough b/1/0", res.Scored[1])
	}
	if called.Load() != 0 {
		t.Errorf("HTTP server called %d times, want 0", called.Load())
	}
}

func TestVoyageRerank_RetryOn429ThenSucceed(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := voyageRerankResponse{
			Object: "list",
			Data: []voyageRerankResult{
				{Index: 0, RelevanceScore: 0.7},
			},
			Model: "rerank-2.5",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	c := newVoyageRerankTestClient(t, handler)

	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "alpha"}})
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status = %v, want StatusOk", res.Status)
	}
	if calls.Load() != 2 {
		t.Errorf("server calls = %d, want 2 (1 retry after 429)", calls.Load())
	}
	if len(res.Scored) != 1 || res.Scored[0].Score != 0.7 {
		t.Errorf("Scored = %+v, want one entry score=0.7", res.Scored)
	}
}

func TestVoyageRerank_PersistentServerError_Degraded(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	c := newVoyageRerankTestClient(t, handler)

	docs := []Doc{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res == nil {
		t.Fatal("nil result on degraded — contract violation")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", res.Status)
	}
	if res.Err == nil {
		t.Error("res.Err = nil, want non-nil on degraded")
	}
	if calls.Load() != 3 {
		t.Errorf("server calls = %d, want 3 (full retry budget)", calls.Load())
	}
	// Passthrough preserves docs in original order with Score=0.
	if len(res.Scored) != 2 || res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("passthrough Scored = %+v", res.Scored)
	}
	if res.Scored[0].Score != 0 || res.Scored[1].Score != 0 {
		t.Errorf("passthrough Score should be 0, got %v / %v", res.Scored[0].Score, res.Scored[1].Score)
	}
}

func TestVoyageRerank_4xxFailFast_Degraded(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})
	c := newVoyageRerankTestClient(t, handler)

	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "alpha"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res == nil {
		t.Fatal("nil result on degraded — contract violation")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", res.Status)
	}
	if calls.Load() != 1 {
		t.Errorf("server calls = %d, want 1 (4xx must fail fast)", calls.Load())
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
}

func TestVoyageRerank_ContextCancellation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block long enough that ctx cancellation wins.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	})
	c := newVoyageRerankTestClient(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before call

	res, err := c.RerankWithResult(ctx, "q", []Doc{{ID: "a", Text: "alpha"}})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error should reflect context cancellation, got: %v", err)
	}
	if res == nil {
		t.Fatal("nil result on cancellation — contract violation")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", res.Status)
	}
}

func TestVoyageRerank_WithTopN_ForwardedAsTopK(t *testing.T) {
	var capturedBody voyageRerankRequest
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Echo back a minimal valid response.
		resp := voyageRerankResponse{
			Object: "list",
			Data: []voyageRerankResult{
				{Index: 0, RelevanceScore: 0.5},
			},
			Model: "rerank-2.5",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	c := newVoyageRerankTestClient(t, handler)

	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}
	_, err := c.RerankWithResult(context.Background(), "q", docs, WithTopN(2))
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if capturedBody.TopK == nil {
		t.Fatal("request top_k = nil, want pointer to 2")
	}
	if *capturedBody.TopK != 2 {
		t.Errorf("request top_k = %d, want 2", *capturedBody.TopK)
	}
}

func TestVoyageRerank_DryRun_NoHTTP(t *testing.T) {
	var called atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
	})
	c := newVoyageRerankTestClient(t, handler)

	docs := []Doc{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithDryRun())
	if err != nil {
		t.Fatalf("RerankWithResult: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status = %v, want StatusSkipped", res.Status)
	}
	if called.Load() != 0 {
		t.Errorf("HTTP called %d times, want 0", called.Load())
	}
	if len(res.Scored) != 2 || res.Scored[0].ID != "a" {
		t.Errorf("DryRun should return passthrough, got %+v", res.Scored)
	}
}

func TestVoyageRerank_AvailableTrueWithKey(t *testing.T) {
	c := NewVoyageRerankClient("k", "", slog.Default())
	if !c.Available() {
		t.Error("Available() = false, want true with non-empty apiKey")
	}
	if c.model != voyageDefaultModel {
		t.Errorf("model = %q, want default %q", c.model, voyageDefaultModel)
	}
}

func TestVoyageRerank_NilLoggerFallsBackToDefault(t *testing.T) {
	c := NewVoyageRerankClient("k", "rerank-2.5", nil)
	if c.logger == nil {
		t.Error("logger = nil, want slog.Default() fallback")
	}
}
