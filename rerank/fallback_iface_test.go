package rerank

// Tests that fallback works with arbitrary Reranker implementations
// (not just *Client). Regression for go-kit v0.40 — Voyage / Jina / future
// custom rerankers must compose with *Client primaries via WithFallback.

import (
	"context"
	"net/http"
	"testing"
)

// stubReranker is a hand-rolled Reranker for tests — returns whatever
// Result it's configured with, ignoring the input. Lets us simulate a
// Voyage/Jina-shaped backend without an HTTP server.
type stubReranker struct {
	available bool
	result    *Result
	err       error
	called    int
}

func (s *stubReranker) Available() bool { return s.available }

func (s *stubReranker) Rerank(ctx context.Context, query string, docs []Doc) []Scored {
	res, _ := s.RerankWithResult(ctx, query, docs)
	if res == nil {
		return nil
	}
	return res.Scored
}

func (s *stubReranker) RerankWithResult(ctx context.Context, query string, docs []Doc, opts ...RerankOpt) (*Result, error) {
	s.called++
	return s.result, s.err
}

// degradedPrimary builds a *Client whose backend always returns 503 →
// rerankInternal yields StatusDegraded with a non-4xx error, triggering
// the fallback branch.
func degradedPrimary(t *testing.T) *Client {
	t.Helper()
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	return newFallbackTestClient(t, srv.URL)
}

// TestFallback_NonClientSecondaryServesOnPrimaryDegraded confirms that a
// VoyageRerankClient-shaped secondary (i.e. any Reranker, not *Client) is
// invoked when the primary returns StatusDegraded with a non-4xx error.
func TestFallback_NonClientSecondaryServesOnPrimaryDegraded(t *testing.T) {
	primary := degradedPrimary(t)

	secOk := []Scored{{Doc: Doc{ID: "a"}, Score: 0.9}, {Doc: Doc{ID: "b"}, Score: 0.5}}
	secondary := &stubReranker{
		available: true,
		result:    &Result{Status: StatusOk, Scored: secOk, Model: "voyage"},
	}

	res := rerankWithFallback(
		context.Background(), primary, secondary, "voyage",
		"q", []Doc{{ID: "a"}, {ID: "b"}},
	)

	if res.Status != StatusFallback {
		t.Fatalf("expected StatusFallback, got %v", res.Status)
	}
	if secondary.called != 1 {
		t.Errorf("secondary should be called once, got %d", secondary.called)
	}
	if len(res.Scored) != 2 || res.Scored[0].Doc.ID != "a" {
		t.Errorf("scores not propagated correctly: %+v", res.Scored)
	}
}

// TestFallback_NonClientSecondaryUnavailable: if secondary.Available() is
// false (e.g. Voyage with no API key), we treat it like nil — return
// primary's Degraded result and DO NOT call the secondary.
func TestFallback_NonClientSecondaryUnavailable(t *testing.T) {
	primary := degradedPrimary(t)
	secondary := &stubReranker{available: false}

	res := rerankWithFallback(
		context.Background(), primary, secondary, "voyage",
		"q", []Doc{{ID: "a"}},
	)
	if res.Status != StatusDegraded {
		t.Errorf("unavailable secondary must not be called; expected primary's Degraded, got %v", res.Status)
	}
	if secondary.called != 0 {
		t.Errorf("unavailable secondary must not be called, got %d", secondary.called)
	}
}

// TestFallback_NonClientSecondaryAlsoFails: when both primary and secondary
// return non-Ok, primary's Degraded result is returned (as documented).
func TestFallback_NonClientSecondaryAlsoFails(t *testing.T) {
	primary := degradedPrimary(t)
	secondary := &stubReranker{
		available: true,
		result:    &Result{Status: StatusDegraded},
	}
	res := rerankWithFallback(
		context.Background(), primary, secondary, "voyage",
		"q", []Doc{{ID: "a"}},
	)
	if res.Status != StatusDegraded {
		t.Errorf("both failed → expected primary's Degraded, got %v", res.Status)
	}
	if secondary.called != 1 {
		t.Errorf("secondary called %d times, want 1", secondary.called)
	}
}

// TestWithFallback_AcceptsRerankerInterface: API-compat check — passing
// a non-*Client Reranker to WithFallback compiles and stores the
// reference + auto-derives "fallback" as the metric label.
func TestWithFallback_AcceptsRerankerInterface(t *testing.T) {
	stub := &stubReranker{available: true}
	cfg := defaultCfg()
	WithFallback(stub)(cfg)
	if cfg.fallback != stub {
		t.Errorf("fallback not stored: got %T, want *stubReranker", cfg.fallback)
	}
	if cfg.fallbackName != "fallback" {
		t.Errorf("auto-derived name: want %q, got %q", "fallback", cfg.fallbackName)
	}
}

// TestWithFallbackName_OverridesLabel: explicit name takes precedence
// for non-*Client secondaries.
func TestWithFallbackName_OverridesLabel(t *testing.T) {
	stub := &stubReranker{available: true}
	cfg := defaultCfg()
	WithFallbackName("voyage-rerank-2.5")(cfg)
	WithFallback(stub)(cfg)
	if cfg.fallbackName != "voyage-rerank-2.5" {
		t.Errorf("explicit name lost: got %q, want %q", cfg.fallbackName, "voyage-rerank-2.5")
	}
}
