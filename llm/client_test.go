package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func okHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}
}

func TestComplete_Success(t *testing.T) {
	srv := newTestServer(t, okHandler("hello from LLM"))
	c := llm.NewClient(srv.URL, "test-key", "test-model")

	result, err := c.Complete(context.Background(), "system", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello from LLM" {
		t.Errorf("result = %q, want %q", result, "hello from LLM")
	}
}

func TestComplete_SendsCorrectRequest(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-key" {
			t.Errorf("auth = %q, want %q", auth, "Bearer my-key")
		}

		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "gpt-4" {
			t.Errorf("model = %v, want gpt-4", req["model"])
		}
		msgs := req["messages"].([]any)
		if len(msgs) != 2 {
			t.Errorf("messages len = %d, want 2", len(msgs))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "my-key", "gpt-4")
	_, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	})

	c := llm.NewClient(srv.URL, "key", "model")
	_, err := c.Complete(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestComplete_RetryOn429(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "finally"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model", llm.WithMaxRetries(3))
	result, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "finally" {
		t.Errorf("result = %q, want %q", result, "finally")
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestComplete_FallbackKeys(t *testing.T) {
	var usedKeys []string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		usedKeys = append(usedKeys, key)
		if key == "bad-key" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok with " + key}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "bad-key", "model",
		llm.WithFallbackKeys([]string{"good-key"}),
		llm.WithMaxRetries(1),
	)
	result, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok with good-key" {
		t.Errorf("result = %q, want %q", result, "ok with good-key")
	}
}

func TestCompleteMultimodal(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		msgs := req["messages"].([]any)
		userMsg := msgs[0].(map[string]any)
		content := userMsg["content"].([]any)

		if len(content) != 2 {
			t.Errorf("content parts = %d, want 2", len(content))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "saw the image"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.CompleteMultimodal(context.Background(), "describe this", []llm.ImagePart{
		{URL: "https://example.com/img.png"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "saw the image" {
		t.Errorf("result = %q, want %q", result, "saw the image")
	}
}

func TestExtractJSON(t *testing.T) {
	fence := "```"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `{"key": "value"}`, `{"key": "value"}`},
		{"markdown fence", fence + "json\n{\"a\": 1}\n" + fence, `{"a": 1}`},
		{"text around json", `some text {"x": 2} more text`, `{"x": 2}`},
		{"no json", "just text", "just text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := llm.ExtractJSON(tc.input)
			if got != tc.want {
				t.Errorf("ExtractJSON = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWithTemperature(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["temperature"] != 0.7 {
			t.Errorf("temperature = %v, want 0.7", req["temperature"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model", llm.WithTemperature(0.7))
	_, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_NoSystemPrompt(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		msgs := req["messages"].([]any)
		if len(msgs) != 1 {
			t.Errorf("messages len = %d, want 1 (no system)", len(msgs))
		}
		okHandler("ok")(w, r)
	})

	c := llm.NewClient(srv.URL, "key", "model")
	_, err := c.Complete(context.Background(), "", "user only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
