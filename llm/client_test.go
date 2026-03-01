package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// chatHandler returns a handler that sends a full ChatResponse JSON.
func chatHandler(content string, toolCalls []llm.ToolCall, finishReason string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		msg := map[string]any{"content": content}
		if len(toolCalls) > 0 {
			msg["tool_calls"] = toolCalls
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": msg, "finish_reason": finishReason},
			},
			"usage": map[string]int{
				"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30,
			},
		})
	}
}

// sseHandler returns a handler that streams SSE chunks.
func sseHandler(chunks []string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func TestChat_Usage(t *testing.T) {
	srv := newTestServer(t, chatHandler("hello", nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, want 30", resp.Usage.TotalTokens)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
}

func TestChat_ToolCalls(t *testing.T) {
	calls := []llm.ToolCall{{
		ID:   "call_1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "get_weather",
			Arguments: `{"city":"London"}`,
		},
	}}
	srv := newTestServer(t, chatHandler("", calls, "tool_calls"))
	c := llm.NewClient(srv.URL, "key", "model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "weather?"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("function name = %q, want %q", resp.ToolCalls[0].Function.Name, "get_weather")
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_calls")
	}
}

func TestChat_WithTools(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Errorf("tools len = %v, want 1", req["tools"])
		}
		chatHandler("ok", nil, "stop")(w, r)
	})
	c := llm.NewClient(srv.URL, "key", "model")

	tool := llm.NewTool("search", "Search the web", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	})
	_, err := c.Chat(context.Background(),
		[]llm.Message{{Role: "user", Content: "search"}},
		llm.WithTools([]llm.Tool{tool}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChat_FinishReason(t *testing.T) {
	srv := newTestServer(t, chatHandler("done", nil, "length"))
	c := llm.NewClient(srv.URL, "key", "model")

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FinishReason != "length" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "length")
	}
}

func TestChatTyped(t *testing.T) {
	srv := newTestServer(t, chatHandler(`{"name":"Alice","age":30}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	err := c.ChatTyped(context.Background(), []llm.Message{{Role: "user", Content: "info"}}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name = %q, want %q", result.Name, "Alice")
	}
	if result.Age != 30 {
		t.Errorf("Age = %d, want 30", result.Age)
	}
}

func TestSchemaOf(t *testing.T) {
	type Example struct {
		Name    string   `json:"name"`
		Age     int      `json:"age"`
		Tags    []string `json:"tags"`
		Score   float64  `json:"score,omitempty"`
		Address *string  `json:"address"`
	}

	schema := llm.SchemaOf(Example{})
	if schema["type"] != "object" {
		t.Errorf("type = %v, want object", schema["type"])
	}
	props := schema["properties"].(map[string]any)
	if props["name"].(map[string]any)["type"] != "string" {
		t.Error("name should be string")
	}
	if props["age"].(map[string]any)["type"] != "integer" {
		t.Error("age should be integer")
	}
	tags := props["tags"].(map[string]any)
	if tags["type"] != "array" {
		t.Error("tags should be array")
	}
	required := schema["required"].([]string)
	// "score" has omitempty, "address" is pointer — both should be optional.
	for _, r := range required {
		if r == "score" {
			t.Error("score (omitempty) should not be required")
		}
		if r == "address" {
			t.Error("address (pointer) should not be required")
		}
	}
}

func TestStream_Basic(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}`,
		`{"choices":[{"delta":{"content":" World"},"finish_reason":""}]}`,
		`{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	stream, err := c.Stream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var content string
	for chunk, ok := stream.Next(); ok; chunk, ok = stream.Next() {
		content += chunk.Delta
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if content != "Hello World" {
		t.Errorf("content = %q, want %q", content, "Hello World")
	}
}

func TestStream_Usage(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"hi"},"finish_reason":""}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	stream, err := c.Stream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	for _, ok := stream.Next(); ok; _, ok = stream.Next() {
	}
	usage := stream.Usage()
	if usage == nil {
		t.Fatal("Usage should not be nil after streaming")
	}
	if usage.TotalTokens != 6 {
		t.Errorf("TotalTokens = %d, want 6", usage.TotalTokens)
	}
}

func TestComplete_PerCallOverrides(t *testing.T) {
	var capturedTemp float64
	var capturedMaxTokens float64
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		capturedTemp, _ = req["temperature"].(float64)
		capturedMaxTokens, _ = req["max_tokens"].(float64)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	// Client defaults: temp=0.1, maxTokens=8192
	c := llm.NewClient(srv.URL, "key", "model")

	_, err := c.Complete(context.Background(), "", "test",
		llm.WithChatTemperature(0.7),
		llm.WithChatMaxTokens(250),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTemp != 0.7 {
		t.Errorf("temperature = %v, want 0.7", capturedTemp)
	}
	if capturedMaxTokens != 250 {
		t.Errorf("max_tokens = %v, want 250", capturedMaxTokens)
	}
}

func TestStream_Error(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	})
	c := llm.NewClient(srv.URL, "key", "model")

	_, err := c.Stream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}
