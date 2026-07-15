package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// emptyContentHandler returns HTTP 200 with a well-formed chat response whose
// assistant message has empty content and finish_reason=length. This is the
// reasoning-model-truncation shape observed in production: a reasoning model
// (e.g. cerebras-glm-4.7) on a large academic prompt with a tight max_tokens
// budget spends the entire output budget on reasoning tokens and emits no
// visible content. The provider returns 200 (no API error), so pre-fix the
// transport treats it as success.
func emptyContentHandler(content, finishReason string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": finishReason,
				"message":       map[string]any{"content": content},
			}},
		})
	}
}

// TestChain_AdvancesOnEmptyContent is the core fix for the go-search deep-research
// empty-answer regression: a 200-OK response whose assistant content is empty
// (reasoning model truncated by max_tokens before emitting the JSON answer) must
// advance the model-fallback chain to the next model, which can answer. Pre-fix
// this FAILS — empty content is returned as success and the chain short-circuits
// on the first (reasoning) model, propagating an empty answer with no error.
func TestChain_AdvancesOnEmptyContent(t *testing.T) {
	empty := httptest.NewServer(emptyContentHandler("", "length"))
	defer empty.Close()
	ok := httptest.NewServer(okChatHandler(t, "real answer from next model"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: empty.URL, Key: "k", Model: "reasoning-truncated"},
			{URL: ok.URL, Key: "k", Model: "answering-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected chain to advance past empty content, got error: %v", err)
	}
	if out != "real answer from next model" {
		t.Errorf("output = %q, want the answering model's content", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (empty then ok), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "reasoning-truncated" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {reasoning-truncated, err}", (*calls)[0])
	}
	if (*calls)[1].model != "answering-model" || (*calls)[1].err != nil {
		t.Errorf("call[1] = %+v, want {answering-model, nil}", (*calls)[1])
	}
}

// TestSingleEndpoint_EmptyContentErrors: with no chain to advance to, an empty
// completion must surface as the caller's error rather than a silent empty
// string. This is what converts the production silent_downgrade (answer:"",
// error=false) into a visible failure that triggers the pipeline's degrade path.
func TestSingleEndpoint_EmptyContentErrors(t *testing.T) {
	empty := httptest.NewServer(emptyContentHandler("", "length"))
	defer empty.Close()

	c := llm.NewClient(empty.URL, "k", "m", llm.WithMaxRetries(1))
	out, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatalf("empty completion must surface an error (got out=%q, err=nil)", out)
	}
}

// TestSingleEndpoint_EmptyContentNotRetried: an empty completion is non-retryable
// on the same endpoint — re-issuing the identical request to a deterministic
// reasoning model recurs the same truncation. It must be sent exactly once even
// with MaxRetries>1 (failover, not same-endpoint retry).
func TestSingleEndpoint_EmptyContentNotRetried(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "length",
				"message":       map[string]any{"content": ""},
			}},
		})
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(3))
	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Error("empty completion must surface an error on single endpoint")
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("empty completion must not be retried on the same endpoint: server hit %d times, want 1", hits)
	}
}

// TestEmptyContent_ErrorIsClassifiable: the empty-completion error must be an
// APIError so a metrics observer (ClassifyErrorType) can label the failover
// cause rather than bucketing it as "unknown".
func TestEmptyContent_ErrorIsClassifiable(t *testing.T) {
	empty := httptest.NewServer(emptyContentHandler("", "length"))
	defer empty.Close()

	c := llm.NewClient(empty.URL, "k", "m", llm.WithMaxRetries(1))
	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("want error for empty completion")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if got := llm.ClassifyErrorType(err); got == "" || got == "unknown" {
		t.Errorf("ClassifyErrorType = %q, want a specific non-unknown class", got)
	}
}

// TestChain_AllEmptyTerminatesWithError: when every endpoint in the chain returns
// empty content, the chain must terminate with a typed APIError (never (nil, nil),
// never panic). Each endpoint must be attempted exactly once.
func TestChain_AllEmptyTerminatesWithError(t *testing.T) {
	var mu0, mu1 sync.Mutex
	hits0, hits1 := 0, 0

	srv0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu0.Lock()
		hits0++
		mu0.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "length",
				"message":       map[string]any{"content": ""},
			}},
		})
	}))
	defer srv0.Close()

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu1.Lock()
		hits1++
		mu1.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "length",
				"message":       map[string]any{"content": ""},
			}},
		})
	}))
	defer srv1.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv0.URL, Key: "k", Model: "model-a"},
			{URL: srv1.URL, Key: "k", Model: "model-b"},
		}),
		llm.WithMaxRetries(1),
	)

	_, err := c.Complete(context.Background(), "", "test")

	// Must terminate with an error — never (nil, nil), never panic.
	if err == nil {
		t.Fatal("chain with all empty endpoints must return a non-nil error")
	}

	// Error must be a typed *APIError so metrics can classify it.
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *llm.APIError, got %T: %v", err, err)
	}

	// The terminal error must be the empty-completion sentinel.
	if got := llm.ClassifyErrorType(err); got != "empty_completion" {
		t.Errorf("ClassifyErrorType = %q, want \"empty_completion\"", got)
	}

	// Each server must have been hit exactly once (no extra retries on the same endpoint).
	mu0.Lock()
	n0 := hits0
	mu0.Unlock()
	mu1.Lock()
	n1 := hits1
	mu1.Unlock()
	if n0 != 1 {
		t.Errorf("srv0 hit %d times, want 1", n0)
	}
	if n1 != 1 {
		t.Errorf("srv1 hit %d times, want 1", n1)
	}
}
