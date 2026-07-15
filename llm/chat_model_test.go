package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// captureModelServer возвращает ok-chat-response и пишет model id из request.
func captureModelServer(t *testing.T, captured *string, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*captured, _ = req["model"].(string)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "ok"},
			}},
		})
	}))
}

func TestWithChatModel_OverridesClientDefault(t *testing.T) {
	var captured string
	var mu sync.Mutex
	srv := captureModelServer(t, &captured, &mu)
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "client-default-model")
	_, err := c.Complete(context.Background(), "", "test", llm.WithChatModel("per-call-override"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	got := captured
	mu.Unlock()
	if got != "per-call-override" {
		t.Errorf("model = %q, want per-call-override", got)
	}
}

func TestWithChatModel_EmptyStringNoOverride(t *testing.T) {
	var captured string
	var mu sync.Mutex
	srv := captureModelServer(t, &captured, &mu)
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "client-default")
	_, err := c.Complete(context.Background(), "", "test", llm.WithChatModel(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	got := captured
	mu.Unlock()
	if got != "client-default" {
		t.Errorf("empty override should fall back to client default; got model = %q", got)
	}
}

func TestWithChatModel_NoOptionUsesClientDefault(t *testing.T) {
	var captured string
	var mu sync.Mutex
	srv := captureModelServer(t, &captured, &mu)
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "client-default-model")
	_, err := c.Complete(context.Background(), "", "test") // no opts
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	got := captured
	mu.Unlock()
	if got != "client-default-model" {
		t.Errorf("no override should use client default; got model = %q", got)
	}
}

func TestWithChatModel_PerAttemptChainPattern(t *testing.T) {
	// Демонстрация use case'a — caller iterate'ит models с per-attempt
	// ctx timeout. Pattern, который go-search будет использовать после
	// migration.
	var captures []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		model, _ := req["model"].(string)
		mu.Lock()
		captures = append(captures, model)
		mu.Unlock()
		// Fail first 2 models, succeed third.
		if len(captures) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "ok"},
			}},
		})
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "default-not-used",
		llm.WithMaxRetries(1),
	)

	chain := []string{"primary", "fallback-a", "fallback-b"}
	var out string
	var lastErr error
	for _, m := range chain {
		// Per-attempt ctx timeout, simulated by NoCancel here for test simplicity.
		out, lastErr = c.Complete(context.Background(), "", "test", llm.WithChatModel(m))
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		t.Fatalf("expected success on third attempt: %v", lastErr)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	mu.Lock()
	gotCaps := append([]string{}, captures...)
	mu.Unlock()
	if len(gotCaps) != 3 {
		t.Fatalf("expected 3 attempts, got %d: %v", len(gotCaps), gotCaps)
	}
	for i, want := range []string{"primary", "fallback-a", "fallback-b"} {
		if gotCaps[i] != want {
			t.Errorf("captures[%d] = %q, want %q", i, gotCaps[i], want)
		}
	}
}
