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

func TestRerank_EmptyInput(t *testing.T) {
	c := New(Config{URL: "http://nope"}, nil)
	got := c.Rerank(context.Background(), "q", nil)
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestRerank_ZeroURLPassthrough(t *testing.T) {
	c := New(Config{URL: ""}, nil)
	docs := []Doc{{ID: "a", Text: "x"}, {ID: "b", Text: "y"}}
	got := c.Rerank(context.Background(), "q", docs)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("passthrough broken: %+v", got)
	}
	// Score defaults to 0, OrigRank is input index.
	if got[0].OrigRank != 0 || got[1].OrigRank != 1 {
		t.Errorf("OrigRank not preserved: %+v", got)
	}
}

func TestRerank_Timeout(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	c := New(Config{URL: srv.URL, Timeout: 20 * time.Millisecond}, nil)
	docs := []Doc{{ID: "a", Text: "x"}}
	got := c.Rerank(context.Background(), "q", docs)
	// On timeout we return input unchanged in order.
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("want passthrough, got %+v", got)
	}
}

func TestRerank_ServerError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	c := New(Config{URL: srv.URL, Timeout: time.Second}, nil)
	docs := []Doc{{ID: "a", Text: "x"}, {ID: "b", Text: "y"}}
	got := c.Rerank(context.Background(), "q", docs)
	if len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("want passthrough on 500, got %+v", got)
	}
}

func TestRerank_MaxDocsHeadTail(t *testing.T) {
	var gotCount int
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req cohereRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotCount = len(req.Documents)
		// Server gives highest score to index 0, descending. So order after
		// rerank is [0,1,2] (already the server order for head).
		results := make([]cohereResult, len(req.Documents))
		for i := range req.Documents {
			results[i] = cohereResult{
				Index:          i,
				RelevanceScore: float64(len(req.Documents)-i) * 0.1,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: results})
	})
	c := New(Config{URL: srv.URL, MaxDocs: 3, Timeout: time.Second}, nil)
	docs := []Doc{
		{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"},
	}
	got := c.Rerank(context.Background(), "q", docs)

	if gotCount != 3 {
		t.Errorf("server saw %d docs, want 3 (MaxDocs cap)", gotCount)
	}
	if len(got) != 5 {
		t.Fatalf("len: got %d want 5 (head+tail preserved)", len(got))
	}
	// Head reordered by descending score: a (0.3) > b (0.2) > c (0.1).
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Errorf("head order wrong: %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
	// Tail unchanged.
	if got[3].ID != "d" || got[4].ID != "e" {
		t.Errorf("tail not preserved: %v %v", got[3].ID, got[4].ID)
	}
	if got[3].OrigRank != 3 || got[4].OrigRank != 4 {
		t.Errorf("tail OrigRank wrong: %d %d", got[3].OrigRank, got[4].OrigRank)
	}
}

func TestRerank_MaxCharsPerDocTruncation(t *testing.T) {
	var sentTexts []string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req cohereRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sentTexts = req.Documents
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{}})
	})
	c := New(Config{URL: srv.URL, MaxCharsPerDoc: 5, Timeout: time.Second}, nil)
	// 10 Cyrillic runes = 20 bytes. After truncate to 5 runes = 10 bytes.
	docs := []Doc{{ID: "a", Text: "привет мир"}}
	_ = c.Rerank(context.Background(), "q", docs)
	if len(sentTexts) != 1 {
		t.Fatalf("want 1 text sent, got %d", len(sentTexts))
	}
	runeCount := 0
	for range sentTexts[0] {
		runeCount++
	}
	if runeCount != 5 {
		t.Errorf("sent %d runes, want 5 (truncation)", runeCount)
	}
}

func TestRerank_PartialResponse(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Server returns only 2 of 3 docs scored.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{
			{Index: 1, RelevanceScore: 0.9},
			{Index: 2, RelevanceScore: 0.5},
		}})
	})
	c := New(Config{URL: srv.URL, Timeout: time.Second}, nil)
	docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := c.Rerank(context.Background(), "q", docs)
	// Expected order: b (0.9), c (0.5), a (unseen → tail of head).
	wantOrder := []string{"b", "c", "a"}
	for i, s := range got {
		if s.ID != wantOrder[i] {
			t.Errorf("pos %d: got %q want %q", i, s.ID, wantOrder[i])
		}
	}
	if got[2].Score != 0 {
		t.Errorf("unseen doc score: got %v want 0", got[2].Score)
	}
}

func TestRerank_APIKeyHeader(t *testing.T) {
	var gotAuth string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{}})
	})
	c := New(Config{URL: srv.URL, APIKey: "secret-token", Timeout: time.Second}, nil)
	_ = c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})
	if gotAuth != "Bearer secret-token" {
		t.Errorf("auth header: got %q want %q", gotAuth, "Bearer secret-token")
	}
}

func TestAvailable(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var c *Client
		if c.Available() {
			t.Error("nil client must not be available")
		}
	})
	t.Run("zero URL", func(t *testing.T) {
		c := New(Config{URL: ""}, nil)
		if c.Available() {
			t.Error("zero URL must not be available")
		}
	})
	t.Run("configured", func(t *testing.T) {
		c := New(Config{URL: "http://x"}, nil)
		if !c.Available() {
			t.Error("configured client must be available")
		}
	})
}
