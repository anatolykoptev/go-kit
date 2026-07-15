package llm_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestDoRequest_StripsLeadingThink(t *testing.T) {
	// chatHandler sends proper ChatResponse JSON with <think> prefix in content.
	srv := newTestServer(t, chatHandler(`<think>I think therefore</think>{"ok":true}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "k", "test-model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"ok":true}` {
		t.Errorf("Content = %q, want clean JSON", resp.Content)
	}
	if resp.Reasoning != "I think therefore" {
		t.Errorf("Reasoning = %q, want %q", resp.Reasoning, "I think therefore")
	}
}

func TestDoRequest_ReasoningContent_Field(t *testing.T) {
	// Simulate DeepSeek/vLLM style: reasoning_content sibling field, clean content.
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"city\":\"Paris\"}","reasoning_content":"field reasoning"},"finish_reason":"stop"}]}`))
	})
	c := llm.NewClient(srv.URL, "k", "test-model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"city":"Paris"}` {
		t.Errorf("Content = %q, want clean JSON", resp.Content)
	}
	if resp.Reasoning != "field reasoning" {
		t.Errorf("Reasoning = %q, want %q", resp.Reasoning, "field reasoning")
	}
}

func TestDoRequest_NoThink_Unchanged(t *testing.T) {
	// Clean model (cerebras/GPT): response unchanged, Reasoning empty.
	srv := newTestServer(t, chatHandler(`{"city":"Paris"}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "k", "test-model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"city":"Paris"}` {
		t.Errorf("Content = %q, want unchanged JSON", resp.Content)
	}
	if resp.Reasoning != "" {
		t.Errorf("Reasoning = %q, want empty for clean models", resp.Reasoning)
	}
}
