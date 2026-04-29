package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
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

// makePartialServer returns a server that only emits results for the given
// subset of indexes (by position in the resp.Results slice), ignoring the rest.
func makePartialServer(t *testing.T, partial []cohereResult) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: partial})
	}))
}

// TestRerank_NormalizeIgnoresUnseenZeros verifies that unseen docs (server
// partial response) do not poison the MinMax normalization range.
//
// Without fix: Normalize sees [0.7, 0.9, 0.0] → MinMax gives [0.778, 1.0, 0.0]
// (the seen min is pulled down by the injected 0).
// With fix: Normalize sees only [0.7, 0.9] → MinMax gives [0.0, 1.0];
// the unseen index 2 stays at 0 untouched.
func TestRerank_NormalizeIgnoresUnseenZeros(t *testing.T) {
	// Server returns only indexes 0 and 1; index 2 is absent (unseen).
	partial := []cohereResult{
		{Index: 0, RelevanceScore: 0.7},
		{Index: 1, RelevanceScore: 0.9},
	}
	srv := makePartialServer(t, partial)
	defer srv.Close()

	c := NewClient(srv.URL,
		WithModel("m"),
		WithTimeout(time.Second),
		WithNormalize(NormalizeMinMax),
	)
	docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build a score map by doc ID for easy lookup.
	scoreByID := make(map[string]float32, len(res.Scored))
	for _, s := range res.Scored {
		scoreByID[s.ID] = s.Score
	}

	// With fix, MinMax operates on [0.7, 0.9]:
	//   min=0.7, max=0.9, rng=0.2
	//   score_a = (0.7-0.7)/0.2 = 0.0
	//   score_b = (0.9-0.7)/0.2 = 1.0
	//   score_c = 0 (unseen, untouched)
	const eps = 1e-5
	if got := scoreByID["a"]; got > eps {
		t.Errorf("doc a: got score %v, want ~0.0 (seen min after MinMax on subset)", got)
	}
	if got := scoreByID["b"]; got < 1.0-eps {
		t.Errorf("doc b: got score %v, want ~1.0 (seen max after MinMax on subset)", got)
	}
	if got := scoreByID["c"]; got != 0.0 {
		t.Errorf("doc c (unseen): got score %v, want 0.0 (untouched)", got)
	}
}

// TestRerank_TruncateChars_MetricEmits verifies that rerank_truncate_total
// with reason="chars" is incremented when WithMaxCharsPerDoc truncates a doc.
func TestRerank_TruncateChars_MetricEmits(t *testing.T) {
	srv := makeG2Server(t, []float32{0.9})
	defer srv.Close()

	// Use a unique model name to avoid counter pollution from other tests.
	model := "m-chars-metric-test"
	c := NewClient(srv.URL,
		WithModel(model),
		WithTimeout(time.Second),
		WithMaxCharsPerDoc(5),
	)

	// "привет мир" is 10 runes — must truncate to 5.
	docs := []Doc{{ID: "x", Text: "привет мир"}}
	_, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := testutil.ToFloat64(rerankTruncateTotal.WithLabelValues(model, "chars"))
	if got <= 0 {
		t.Errorf("rerank_truncate_total{model=%q, reason=chars} = %v, want > 0", model, got)
	}
}
