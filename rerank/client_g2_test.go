package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// makeG2Server creates a test server returning scores for docs in request order.
func makeG2Server(t *testing.T, scores []float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cohereRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		n := len(req.Documents)
		results := make([]cohereResult, n)
		for i := 0; i < n; i++ {
			sc := float32(0)
			if i < len(scores) {
				sc = scores[i]
			}
			results[i] = cohereResult{Index: i, RelevanceScore: float64(sc)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: results})
	}))
}

func TestRerank_ThresholdFilter(t *testing.T) {
	// Server returns [0.9, 0.5, 0.1]. Threshold 0.4 → keep first 2.
	srv := makeG2Server(t, []float32{0.9, 0.5, 0.1})
	defer srv.Close()

	c := NewClient(srv.URL, WithModel("m"), WithTimeout(time.Second))
	docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithThreshold(0.4))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only docs with score >= 0.4 (i.e. "a"=0.9, "b"=0.5).
	if len(res.Scored) != 2 {
		t.Fatalf("got %d docs after threshold, want 2 (got IDs: %v)", len(res.Scored), scoredIDs(res.Scored))
	}
	if res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("wrong order: %v", scoredIDs(res.Scored))
	}
}

func TestRerank_TopN(t *testing.T) {
	srv := makeG2Server(t, []float32{0.9, 0.7, 0.5, 0.3, 0.1})
	defer srv.Close()

	c := NewClient(srv.URL, WithModel("m"), WithTimeout(time.Second))
	docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithTopN(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 2 {
		t.Fatalf("got %d docs after TopN(2), want 2", len(res.Scored))
	}
	if res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("wrong docs: %v", scoredIDs(res.Scored))
	}
}

func TestRerank_TopN_LargerThanInput_NoPanic(t *testing.T) {
	srv := makeG2Server(t, []float32{0.9, 0.5})
	defer srv.Close()

	c := NewClient(srv.URL, WithModel("m"), WithTimeout(time.Second))
	docs := []Doc{{ID: "a"}, {ID: "b"}}
	// TopN(10) > len(docs) — must not panic.
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithTopN(10))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 2 {
		t.Errorf("got %d docs, want 2", len(res.Scored))
	}
}

func TestRerank_DryRun_SkipsHTTP(t *testing.T) {
	var hitCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hitCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithModel("m"), WithTimeout(time.Second))
	docs := []Doc{{ID: "x"}, {ID: "y"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs, WithDryRun())
	if err != nil {
		t.Fatalf("DryRun should not return error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("DryRun status: got %v want StatusSkipped", res.Status)
	}
	if atomic.LoadInt64(&hitCount) != 0 {
		t.Errorf("DryRun made %d HTTP requests, want 0", hitCount)
	}
	// Original order preserved.
	if len(res.Scored) != 2 || res.Scored[0].ID != "x" || res.Scored[1].ID != "y" {
		t.Errorf("DryRun passthrough wrong: %v", scoredIDs(res.Scored))
	}
}

func TestRerank_PipelineOrder(t *testing.T) {
	// Verify pipeline stages fire in the correct order:
	// truncate(tokens) → POST → normalize → weight → sort → threshold → TopN
	//
	// Setup: 3 docs, server returns scores [0.6, 0.9, 0.3].
	// After sort (no normalize/weight): order = [b(0.9), a(0.6), c(0.3)].
	// After threshold 0.5: c(0.3) dropped → [b, a].
	// After TopN(1): only b.
	srv := makeG2Server(t, []float32{0.6, 0.9, 0.3})
	defer srv.Close()

	c := NewClient(srv.URL, WithModel("m"), WithTimeout(time.Second))
	docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs,
		WithThreshold(0.5),
		WithTopN(1),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 1 {
		t.Fatalf("got %d docs, want 1 (after threshold+TopN): %v", len(res.Scored), scoredIDs(res.Scored))
	}
	if res.Scored[0].ID != "b" {
		t.Errorf("top doc: got %q want %q", res.Scored[0].ID, "b")
	}
}

func scoredIDs(s []Scored) []string {
	ids := make([]string, len(s))
	for i, d := range s {
		ids[i] = d.ID
	}
	return ids
}
