package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestRerank_BasicReorder(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return doc[2] first (score 0.9), then doc[0] (0.5), then doc[1] (0.1)
		json.NewEncoder(w).Encode(cohereResponse{
			Model: "test-model",
			Results: []cohereResult{
				{Index: 2, RelevanceScore: 0.9},
				{Index: 0, RelevanceScore: 0.5},
				{Index: 1, RelevanceScore: 0.1},
			},
		})
	})

	c := New(Config{URL: srv.URL, Model: "test-model", Timeout: time.Second}, nil)
	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}
	got := c.Rerank(context.Background(), "query", docs)

	if len(got) != 3 {
		t.Fatalf("len: got %d want 3", len(got))
	}
	wantOrder := []string{"c", "a", "b"}
	for i, s := range got {
		if s.ID != wantOrder[i] {
			t.Errorf("pos %d: got %q want %q", i, s.ID, wantOrder[i])
		}
	}
	// OrigRank preserved from input order.
	if got[0].OrigRank != 2 {
		t.Errorf("top doc OrigRank: got %d want 2", got[0].OrigRank)
	}
	if got[0].Score != 0.9 {
		t.Errorf("top doc Score: got %v want 0.9", got[0].Score)
	}
}
