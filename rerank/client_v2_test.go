package rerank

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// ── TestNewClient_EquivalentToV1 ──────────────────────────────────────────────

func TestNewClient_EquivalentToV1(t *testing.T) {
	// NewClient(url, opts...) must produce a Client that behaves identically to
	// New(Config{...}, nil) on the existing BasicReorder fixture.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model: "test-model",
			Results: []cohereResult{
				{Index: 2, RelevanceScore: 0.9},
				{Index: 0, RelevanceScore: 0.5},
				{Index: 1, RelevanceScore: 0.1},
			},
		})
	})
	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}

	// v1 client
	v1 := New(Config{URL: srv.URL, Model: "test-model", Timeout: time.Second}, nil)
	// v2 client with equivalent opts
	v2 := NewClient(srv.URL,
		WithModel("test-model"),
		WithTimeout(time.Second),
	)

	gotV1 := v1.Rerank(context.Background(), "query", docs)
	gotV2 := v2.Rerank(context.Background(), "query", docs)

	if len(gotV1) != len(gotV2) {
		t.Fatalf("len mismatch: v1=%d v2=%d", len(gotV1), len(gotV2))
	}
	for i := range gotV1 {
		if gotV1[i].ID != gotV2[i].ID || gotV1[i].Score != gotV2[i].Score || gotV1[i].OrigRank != gotV2[i].OrigRank {
			t.Errorf("pos %d: v1=%+v v2=%+v", i, gotV1[i], gotV2[i])
		}
	}
}

// ── TestRerankWithResult_StatusOk ─────────────────────────────────────────────

func TestRerankWithResult_StatusOk(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model: "m",
			Results: []cohereResult{
				{Index: 1, RelevanceScore: 0.8},
				{Index: 0, RelevanceScore: 0.3},
			},
		})
	})
	c := NewClient(srv.URL, WithTimeout(time.Second))
	docs := []Doc{{ID: "x"}, {ID: "y"}}

	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("Status: got %v want StatusOk", res.Status)
	}
	if res.Err != nil {
		t.Errorf("Err: got %v want nil", res.Err)
	}
	if len(res.Scored) != 2 {
		t.Fatalf("Scored len: got %d want 2", len(res.Scored))
	}
	if res.Scored[0].ID != "y" || res.Scored[1].ID != "x" {
		t.Errorf("wrong order: %v %v", res.Scored[0].ID, res.Scored[1].ID)
	}
}

// ── TestRerankWithResult_StatusSkipped_EmptyDocs ──────────────────────────────

func TestRerankWithResult_StatusSkipped_EmptyDocs(t *testing.T) {
	c := NewClient("http://embed:8082", WithTimeout(time.Second))

	res, err := c.RerankWithResult(context.Background(), "q", []Doc{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status: got %v want StatusSkipped", res.Status)
	}
	if len(res.Scored) != 0 {
		t.Errorf("Scored: got %v want empty", res.Scored)
	}
}

// ── TestRerankWithResult_StatusSkipped_NoURL ──────────────────────────────────

func TestRerankWithResult_StatusSkipped_NoURL(t *testing.T) {
	c := NewClient("") // empty URL

	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("Status: got %v want StatusSkipped", res.Status)
	}
	// Passthrough: input order preserved, Score=0.
	if len(res.Scored) != 1 || res.Scored[0].ID != "a" {
		t.Errorf("Scored: got %+v", res.Scored)
	}
	if res.Scored[0].Score != 0 {
		t.Errorf("Score: got %v want 0", res.Scored[0].Score)
	}
}

// ── TestRerankWithResult_StatusDegraded_HTTP500 ───────────────────────────────

func TestRerankWithResult_StatusDegraded_HTTP500(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	c := NewClient(srv.URL, WithTimeout(time.Second))
	docs := []Doc{{ID: "a", Text: "x"}, {ID: "b", Text: "y"}}

	res, err := c.RerankWithResult(context.Background(), "q", docs)
	// RerankWithResult returns err on Degraded.
	if err == nil {
		t.Error("expected non-nil error on HTTP 500")
	}
	if res == nil {
		t.Fatal("expected non-nil Result even on degraded")
	}
	if res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded", res.Status)
	}
	if res.Err == nil {
		t.Error("Result.Err must be non-nil on StatusDegraded")
	}
	// Passthrough must preserve input order.
	if len(res.Scored) != 2 || res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("passthrough order broken: %+v", res.Scored)
	}
}

// ── TestNewClient_Available ───────────────────────────────────────────────────

func TestNewClient_Available(t *testing.T) {
	if !NewClient("http://x").Available() {
		t.Error("NewClient with URL must be Available")
	}
	if NewClient("").Available() {
		t.Error("NewClient with empty URL must not be Available")
	}
}

// --- WithRequireAuth tests (pf-5: EMBED_TOKEN validation, deferred to call) ---

// TestRerank_RequireAuth_EmptyToken verifies that WithRequireAuth makes
// RerankWithResult return ErrNoToken on the first call when no token is
// configured. Goes RED if the deferred guard in rerankInternal is removed
// (the call would instead hit the stub server and return StatusOk).
func TestRerank_RequireAuth_EmptyToken(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model: "test-model",
			Results: []cohereResult{
				{Index: 0, RelevanceScore: 0.9},
			},
		})
	})
	t.Setenv("EMBED_TOKEN", "")
	c := NewClient(srv.URL,
		WithModel("test-model"),
		WithRequireAuth(),
	)
	docs := []Doc{{ID: "a", Text: "alpha"}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("want ErrNoToken, got err=%v", err)
	}
	if res == nil || res.Status != StatusDegraded {
		t.Fatalf("want StatusDegraded result, got %+v", res)
	}
}

// TestRerank_RequireAuth_WhitespaceToken verifies whitespace-only EMBED_TOKEN
// is treated as "no token" under WithRequireAuth.
func TestRerank_RequireAuth_WhitespaceToken(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model:   "test-model",
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}},
		})
	})
	t.Setenv("EMBED_TOKEN", "   ")
	c := NewClient(srv.URL,
		WithModel("test-model"),
		WithRequireAuth(),
	)
	_, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "alpha"}})
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("want ErrNoToken for whitespace-only token, got %v", err)
	}
}

// TestRerank_RequireAuth_ValidToken verifies a non-empty EMBED_TOKEN passes
// the deferred guard and reaches the backend.
func TestRerank_RequireAuth_ValidToken(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{
			Model: "test-model",
			Results: []cohereResult{
				{Index: 0, RelevanceScore: 0.9},
			},
		})
	})
	t.Setenv("EMBED_TOKEN", "real-token")
	c := NewClient(srv.URL,
		WithModel("test-model"),
		WithRequireAuth(),
	)
	res, err := c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "alpha"}})
	if err != nil {
		t.Fatalf("unexpected error with valid token: %v", err)
	}
	if res.Status != StatusOk {
		t.Fatalf("want StatusOk, got %s", res.Status)
	}
}

// TestRerank_NoRequireAuth_EmptyToken verifies backward compatibility:
// without WithRequireAuth, an empty EMBED_TOKEN does NOT produce ErrNoToken
// (DryRun path returns StatusSkipped without touching auth).
func TestRerank_NoRequireAuth_EmptyToken(t *testing.T) {
	t.Setenv("EMBED_TOKEN", "")
	c := NewClient("http://embed:8082",
		WithModel("test-model"),
	)
	res, err := c.RerankWithResult(context.Background(), "q",
		[]Doc{{ID: "a", Text: "alpha"}}, WithDryRun())
	if err != nil {
		t.Fatalf("DryRun without WithRequireAuth should not error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("want StatusSkipped, got %s", res.Status)
	}
}

// TestRerank_RequireAuth_DryRunExempt verifies DryRun is exempt from the
// deferred auth check (wiring tests without a server should still work).
func TestRerank_RequireAuth_DryRunExempt(t *testing.T) {
	t.Setenv("EMBED_TOKEN", "")
	c := NewClient("http://embed:8082",
		WithModel("test-model"),
		WithRequireAuth(),
	)
	res, err := c.RerankWithResult(context.Background(), "q",
		[]Doc{{ID: "a", Text: "alpha"}}, WithDryRun())
	if err != nil {
		t.Fatalf("DryRun should be exempt from auth check: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Fatalf("want StatusSkipped, got %s", res.Status)
	}
}
