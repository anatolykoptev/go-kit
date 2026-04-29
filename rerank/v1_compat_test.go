package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestV1ApiUnchanged pins byte-identical output for all existing v1 fixtures.
// Every future stream (G1-G4) must keep this test green.
//
// The test runs each v1 test scenario via New(Config{...}, logger) and asserts
// that the []Scored slice has the same IDs, Scores, and OrigRanks as the
// original test expectations document.
func TestV1ApiUnchanged(t *testing.T) {
	t.Run("BasicReorder", func(t *testing.T) {
		srv := v1TestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			v1JSONResp(w, cohereResponse{
				Model: "test-model",
				Results: []cohereResult{
					{Index: 2, RelevanceScore: 0.9},
					{Index: 0, RelevanceScore: 0.5},
					{Index: 1, RelevanceScore: 0.1},
				},
			})
		})
		c := New(Config{URL: srv.URL, Model: "test-model", Timeout: time.Second}, nil)
		docs := []Doc{{ID: "a", Text: "alpha"}, {ID: "b", Text: "beta"}, {ID: "c", Text: "gamma"}}
		got := c.Rerank(context.Background(), "query", docs)

		assertScoredIDs(t, got, []string{"c", "a", "b"})
		if got[0].OrigRank != 2 {
			t.Errorf("top OrigRank: got %d want 2", got[0].OrigRank)
		}
		if got[0].Score != 0.9 {
			t.Errorf("top Score: got %v want 0.9", got[0].Score)
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		c := New(Config{URL: "http://nope"}, nil)
		got := c.Rerank(context.Background(), "q", nil)
		if len(got) != 0 {
			t.Errorf("want empty, got %d", len(got))
		}
	})

	t.Run("ZeroURLPassthrough", func(t *testing.T) {
		c := New(Config{URL: ""}, nil)
		docs := []Doc{{ID: "a", Text: "x"}, {ID: "b", Text: "y"}}
		got := c.Rerank(context.Background(), "q", docs)
		assertScoredIDs(t, got, []string{"a", "b"})
		if got[0].OrigRank != 0 || got[1].OrigRank != 1 {
			t.Errorf("OrigRank not preserved: %+v", got)
		}
		for _, s := range got {
			if s.Score != 0 {
				t.Errorf("passthrough score: got %v want 0", s.Score)
			}
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		srv := v1TestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})
		c := New(Config{URL: srv.URL, Timeout: 20 * time.Millisecond}, nil)
		got := c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})
		if len(got) != 1 || got[0].ID != "a" {
			t.Errorf("timeout: want passthrough, got %+v", got)
		}
	})

	t.Run("ServerError", func(t *testing.T) {
		srv := v1TestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})
		c := New(Config{URL: srv.URL, Timeout: time.Second}, nil)
		got := c.Rerank(context.Background(), "q", []Doc{{ID: "a"}, {ID: "b"}})
		assertScoredIDs(t, got, []string{"a", "b"})
	})

	t.Run("MaxDocsHeadTail", func(t *testing.T) {
		var gotCount int
		srv := v1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
			var req cohereRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotCount = len(req.Documents)
			results := make([]cohereResult, len(req.Documents))
			for i := range req.Documents {
				results[i] = cohereResult{
					Index:          i,
					RelevanceScore: float64(len(req.Documents)-i) * 0.1,
				}
			}
			v1JSONResp(w, cohereResponse{Results: results})
		})
		c := New(Config{URL: srv.URL, MaxDocs: 3, Timeout: time.Second}, nil)
		docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}}
		got := c.Rerank(context.Background(), "q", docs)

		if gotCount != 3 {
			t.Errorf("server saw %d docs, want 3 (MaxDocs cap)", gotCount)
		}
		if len(got) != 5 {
			t.Fatalf("len: got %d want 5 (head+tail preserved)", len(got))
		}
		assertScoredIDs(t, got, []string{"a", "b", "c", "d", "e"})
		if got[3].OrigRank != 3 || got[4].OrigRank != 4 {
			t.Errorf("tail OrigRank wrong: %d %d", got[3].OrigRank, got[4].OrigRank)
		}
	})

	t.Run("MaxCharsPerDocTruncation", func(t *testing.T) {
		var sentTexts []string
		srv := v1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
			var req cohereRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			sentTexts = req.Documents
			v1JSONResp(w, cohereResponse{Results: []cohereResult{}})
		})
		c := New(Config{URL: srv.URL, MaxCharsPerDoc: 5, Timeout: time.Second}, nil)
		docs := []Doc{{ID: "a", Text: "привет мир"}} // 10 Cyrillic runes
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
	})

	t.Run("PartialResponse", func(t *testing.T) {
		srv := v1TestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			v1JSONResp(w, cohereResponse{Results: []cohereResult{
				{Index: 1, RelevanceScore: 0.9},
				{Index: 2, RelevanceScore: 0.5},
			}})
		})
		c := New(Config{URL: srv.URL, Timeout: time.Second}, nil)
		docs := []Doc{{ID: "a"}, {ID: "b"}, {ID: "c"}}
		got := c.Rerank(context.Background(), "q", docs)
		assertScoredIDs(t, got, []string{"b", "c", "a"})
		if got[2].Score != 0 {
			t.Errorf("unseen doc score: got %v want 0", got[2].Score)
		}
	})

	t.Run("APIKeyHeader", func(t *testing.T) {
		var gotAuth string
		srv := v1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			v1JSONResp(w, cohereResponse{Results: []cohereResult{}})
		})
		c := New(Config{URL: srv.URL, APIKey: "secret-token", Timeout: time.Second}, nil)
		_ = c.Rerank(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})
		if gotAuth != "Bearer secret-token" {
			t.Errorf("auth header: got %q want %q", gotAuth, "Bearer secret-token")
		}
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func v1TestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func v1JSONResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func assertScoredIDs(t *testing.T, got []Scored, wantIDs []string) {
	t.Helper()
	if len(got) != len(wantIDs) {
		t.Fatalf("len: got %d want %d", len(got), len(wantIDs))
	}
	for i, s := range got {
		if s.ID != wantIDs[i] {
			t.Errorf("pos %d: got %q want %q", i, s.ID, wantIDs[i])
		}
	}
}
