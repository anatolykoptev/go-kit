# llm Phase A: Table Stakes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add SSE streaming, tool/function calling, structured output with JSON Schema, and token usage reporting — making the llm package competitive with openai/openai-go and sashabaranov/go-openai.

**Architecture:** 3 new files (chat.go, stream.go, schema.go) + refactored client.go. Internal doRequest returns structured *ChatResponse instead of string. Complete/CompleteRaw remain as convenience wrappers. New Chat method returns full response. Backward-compatible.

**Tech Stack:** Go stdlib only (`bufio`, `reflect`, `encoding/json`, `net/http`)

---

### Task 1: Core refactor + Chat method + Tool calling

**Files:** llm/client.go (modify), llm/chat.go (create)

This task refactors the internal doRequest to return structured responses, adds the Chat method, tool calling types, and usage reporting.

#### 1a. New types in chat.go

```go
package llm

// Usage holds token usage from the API response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Tool defines a function tool for the API.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// NewTool creates a function tool with the given name, description, and JSON Schema parameters.
func NewTool(name, description string, parameters any) Tool {
	return Tool{
		Type:     "function",
		Function: ToolFunction{Name: name, Description: description, Parameters: parameters},
	}
}

// ToolCall represents a tool call from the assistant response.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse is the full response from Chat.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        *Usage
}

// ChatOption configures a per-request Chat option.
type ChatOption func(*chatConfig)

type chatConfig struct {
	tools          []Tool
	toolChoice     any
	responseFormat any
}

func (cfg *chatConfig) apply(req *chatRequest) {
	if cfg.tools != nil {
		req.Tools = cfg.tools
	}
	if cfg.toolChoice != nil {
		req.ToolChoice = cfg.toolChoice
	}
	if cfg.responseFormat != nil {
		req.ResponseFormat = cfg.responseFormat
	}
}

// WithTools sets the available tools for the request.
func WithTools(tools []Tool) ChatOption {
	return func(c *chatConfig) { c.tools = tools }
}

// WithToolChoice sets the tool choice strategy ("auto", "none", or a specific tool).
func WithToolChoice(choice any) ChatOption {
	return func(c *chatConfig) { c.toolChoice = choice }
}

// Chat sends a chat completion request and returns the full response
// including tool calls, finish reason, and token usage.
func (c *Client) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (*ChatResponse, error) {
	var cfg chatConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	req := c.newRequest(messages)
	cfg.apply(req)
	return c.execute(ctx, req)
}
```

#### 1b. Modifications to client.go

**Update Message type** — add ToolCalls and ToolCallID fields:

```go
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
```

**Expand chatRequest** — add optional fields:

```go
type chatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float64   `json:"temperature"`
	MaxTokens      int       `json:"max_tokens"`
	Stream         bool      `json:"stream,omitempty"`
	Tools          []Tool    `json:"tools,omitempty"`
	ToolChoice     any       `json:"tool_choice,omitempty"`
	ResponseFormat any       `json:"response_format,omitempty"`
}
```

**Expand chatResponse** — add tool_calls, finish_reason, usage:

```go
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}
```

**Add newRequest helper:**

```go
func (c *Client) newRequest(messages []Message) *chatRequest {
	return &chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	}
}
```

**Refactor doRequest** — accept *chatRequest, return *ChatResponse:

```go
func (c *Client) doRequest(ctx context.Context, apiKey string, req *chatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if isRetryableStatus(resp.StatusCode) {
		return nil, &retryableError{statusCode: resp.StatusCode, body: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, errors.New("llm: empty choices in response")
	}

	return &ChatResponse{
		Content:      strings.TrimSpace(chatResp.Choices[0].Message.Content),
		ToolCalls:    chatResp.Choices[0].Message.ToolCalls,
		FinishReason: chatResp.Choices[0].FinishReason,
		Usage:        chatResp.Usage,
	}, nil
}
```

**Refactor doWithRetry** — accept *chatRequest, return *ChatResponse:

```go
func (c *Client) doWithRetry(ctx context.Context, apiKey string, req *chatRequest) (*ChatResponse, error) {
	delay := retryDelay
	var lastErr error

	for attempt := range c.maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, maxRetryDelay)
		}

		result, err := c.doRequest(ctx, apiKey, req)
		if err == nil {
			return result, nil
		}
		lastErr = err

		var re *retryableError
		if !asRetryable(err, &re) {
			return nil, err
		}
	}
	return nil, lastErr
}
```

**Extract execute helper** — shared fallback logic:

```go
func (c *Client) execute(ctx context.Context, req *chatRequest) (*ChatResponse, error) {
	result, err := c.doWithRetry(ctx, c.apiKey, req)
	if err == nil {
		return result, nil
	}
	for _, key := range c.fallbackKeys {
		if key == "" {
			continue
		}
		result, err = c.doWithRetry(ctx, key, req)
		if err == nil {
			return result, nil
		}
	}
	return nil, err
}
```

**Update CompleteRaw** — use refactored internals:

```go
func (c *Client) CompleteRaw(ctx context.Context, messages []Message) (string, error) {
	req := c.newRequest(messages)
	resp, err := c.execute(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
```

**Step 1:** Read current client.go and create chat.go with new types.
**Step 2:** Modify client.go with refactored internals.
**Step 3:** Run existing tests: `cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1`
**Step 4:** All 9 existing tests must PASS.
**Step 5:** Commit both files.

---

### Task 2: Structured Output (JSON Schema + ChatTyped)

**Files:** llm/schema.go (create), llm/chat.go (add ChatTyped + WithJSONSchema)

#### 2a. schema.go — JSON Schema from Go structs

```go
package llm

import (
	"reflect"
	"strings"
)

// SchemaOf generates a JSON Schema from a Go struct.
// Uses struct field types and json tags for field names.
// Pointer fields and omitempty fields are optional (not in "required").
func SchemaOf(v any) map[string]any {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return typeSchema(t)
}

func typeSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return map[string]any{"type": "object", "additionalProperties": typeSchema(t.Elem())}
		}
		return map[string]any{"type": "object"}
	case reflect.Struct:
		return structSchema(t)
	default:
		return map[string]any{"type": "string"}
	}
}

func structSchema(t reflect.Type) map[string]any {
	props := make(map[string]any)
	var required []string

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		omit := false
		if tag := f.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				name = parts[0]
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					omit = true
				}
			}
		}
		props[name] = typeSchema(f.Type)
		if !omit && f.Type.Kind() != reflect.Ptr {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
```

#### 2b. Add to chat.go — WithJSONSchema + ChatTyped

```go
// WithJSONSchema sets the response format to structured JSON output.
func WithJSONSchema(name string, schema any) ChatOption {
	return func(c *chatConfig) {
		c.responseFormat = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   name,
				"strict": true,
				"schema": schema,
			},
		}
	}
}

// ChatTyped sends a structured output request and unmarshals the response into target.
// Generates JSON Schema from target's type, sends it as response_format,
// and unmarshals the JSON response directly into target.
func (c *Client) ChatTyped(ctx context.Context, messages []Message, target any) error {
	schema := SchemaOf(target)
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := strings.ToLower(t.Name())
	if name == "" {
		name = "response"
	}

	resp, err := c.Chat(ctx, messages, WithJSONSchema(name, schema))
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(resp.Content), target)
}
```

**Step 1:** Create schema.go.
**Step 2:** Add WithJSONSchema and ChatTyped to chat.go (add `"reflect"`, `"strings"`, `"encoding/json"` imports to chat.go).
**Step 3:** Run tests: `cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1`
**Step 4:** Commit.

---

### Task 3: SSE Streaming

**Files:** llm/stream.go (create)

```go
package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamChunk represents one chunk from a streaming response.
type StreamChunk struct {
	Delta        string
	FinishReason string
}

// StreamResponse reads chunks from a streaming chat completion.
type StreamResponse struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	usage   *Usage
	err     error
	done    bool
}

// streamEvent is the SSE JSON payload for a streaming chunk.
type streamEvent struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

// Next returns the next chunk. Returns false when streaming is done or on error.
// Check Err() after Next returns false.
func (s *StreamResponse) Next() (StreamChunk, bool) {
	if s.done || s.err != nil {
		return StreamChunk{}, false
	}
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			s.done = true
			return StreamChunk{}, false
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = err
			return StreamChunk{}, false
		}
		if event.Usage != nil {
			s.usage = event.Usage
		}
		if len(event.Choices) == 0 {
			continue
		}
		chunk := StreamChunk{
			Delta:        event.Choices[0].Delta.Content,
			FinishReason: event.Choices[0].FinishReason,
		}
		if chunk.Delta != "" || chunk.FinishReason != "" {
			return chunk, true
		}
	}
	if err := s.scanner.Err(); err != nil {
		s.err = err
	}
	s.done = true
	return StreamChunk{}, false
}

// Err returns any error encountered during streaming.
func (s *StreamResponse) Err() error { return s.err }

// Close closes the underlying response body.
func (s *StreamResponse) Close() error { return s.body.Close() }

// Usage returns token usage. Available after streaming completes.
func (s *StreamResponse) Usage() *Usage { return s.usage }

// Stream starts a streaming chat completion. The caller must call Close() when done.
// Iterate with Next() or range over Chunks().
func (c *Client) Stream(ctx context.Context, messages []Message, opts ...ChatOption) (*StreamResponse, error) {
	var cfg chatConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	req := c.newRequest(messages)
	req.Stream = true
	cfg.apply(req)

	// Try primary key, then fallbacks.
	keys := make([]string, 0, 1+len(c.fallbackKeys))
	keys = append(keys, c.apiKey)
	keys = append(keys, c.fallbackKeys...)

	var lastErr error
	for _, key := range keys {
		if key == "" {
			continue
		}
		sr, err := c.doStreamRequest(ctx, key, req)
		if err == nil {
			return sr, nil
		}
		lastErr = err
		var re *retryableError
		if !asRetryable(err, &re) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) doStreamRequest(ctx context.Context, apiKey string, req *chatRequest) (*StreamResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if isRetryableStatus(resp.StatusCode) {
			return nil, &retryableError{statusCode: resp.StatusCode, body: string(respBody)}
		}
		return nil, fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return &StreamResponse{
		body:    resp.Body,
		scanner: bufio.NewScanner(resp.Body),
	}, nil
}
```

**Step 1:** Create stream.go.
**Step 2:** Run tests: `cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1`
**Step 3:** Commit.

---

### Task 4: Tests for all new features

**Files:** llm/client_test.go (add tests)

Tests use httptest.Server to mock the OpenAI API. Group new tests logically.

**Test helpers to add:**

```go
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
```

**Tests to add:**

1. **TestChat_Usage** — verify Usage is populated in ChatResponse
2. **TestChat_ToolCalls** — server returns tool_calls, verify they're parsed
3. **TestChat_WithTools** — verify tools sent in request body
4. **TestChat_FinishReason** — verify finish_reason is parsed
5. **TestChatTyped** — server returns JSON, verify it's unmarshaled into struct
6. **TestSchemaOf** — verify JSON Schema generated from struct
7. **TestStream_Basic** — server sends SSE chunks, verify they're read
8. **TestStream_Usage** — verify usage is available after streaming
9. **TestStream_Error** — server returns error status, verify error handling

**Step 1:** Add all tests to client_test.go.
**Step 2:** Run all tests: `cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1`
**Step 3:** All tests pass (9 existing + new).
**Step 4:** Commit.

---

### Task 5: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — expand llm section with new features:

```go
import "github.com/anatolykoptev/go-kit/llm"

client := llm.NewClient(baseURL, apiKey, model,
    llm.WithFallbackKeys([]string{backupKey}),
    llm.WithMaxTokens(16384),
    llm.WithTemperature(0.1),
)

// Simple text completion (unchanged)
response, err := client.Complete(ctx, systemPrompt, userPrompt)

// Full chat with tool calling
resp, err := client.Chat(ctx, messages,
    llm.WithTools([]llm.Tool{
        llm.NewTool("get_weather", "Get weather for a city", params),
    }),
)
for _, call := range resp.ToolCalls { ... }
fmt.Printf("Tokens: %d\n", resp.Usage.TotalTokens)

// Structured output — auto-generates JSON Schema from struct
var recipe Recipe
err := client.ChatTyped(ctx, messages, &recipe)

// SSE streaming
stream, err := client.Stream(ctx, messages)
defer stream.Close()
for chunk, ok := stream.Next(); ok; chunk, ok = stream.Next() {
    fmt.Print(chunk.Delta)
}
```

Update bullet points:
- SSE streaming via Stream/Next
- Tool/function calling via Chat + WithTools
- Structured output via ChatTyped + auto JSON Schema
- Token usage reporting in ChatResponse

**ROADMAP changes:**
- Mark llm Phase A as DONE

**Step 1:** Update README.md llm section.
**Step 2:** Update ROADMAP.md llm Phase A status.
**Step 3:** Commit.
