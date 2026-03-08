# llm Phase B: Extract, Endpoint Fallback, Middleware

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add instructor-style Extract with validation retry, model-level endpoint fallback chains, and request/response middleware — making the LLM client a competitive alternative to sashabaranov/go-openai.

**Architecture:** Extract builds on ChatTyped (new file extract.go). Endpoint fallback refactors execute/doRequest to accept baseURL. Middleware chains wrap execute(). `chatRequest` renamed to `ChatRequest` (exported) for middleware access. stdlib only.

**Tech Stack:** Go stdlib only

---

### Task 1: All llm code changes

**Files:** llm/client.go, llm/chat.go, llm/stream.go, llm/extract.go (new)

#### 1a. Rename chatRequest → ChatRequest (all files)

Rename the internal `chatRequest` type to exported `ChatRequest` in client.go, and update ALL references in client.go, chat.go, stream.go:

```go
// ChatRequest is a chat completion request. Exported for use with Middleware.
type ChatRequest struct {
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

References to update:
- client.go: `newRequest() *chatRequest` → `*ChatRequest`, `execute(req *chatRequest)` → `*ChatRequest`, `doWithRetry(apiKey string, req *chatRequest)` → `*ChatRequest`, `doRequest(apiKey string, req *chatRequest)` → `*ChatRequest`
- chat.go: `chatConfig.apply(req *chatRequest)` → `*ChatRequest`
- stream.go: `doStreamRequest(apiKey string, req *chatRequest)` → `*ChatRequest`, local `req` variable in Stream()

#### 1b. Add Endpoint + WithEndpoints + Middleware + WithMiddleware to client.go

Add new types after existing Option functions:

```go
// Endpoint defines a complete API endpoint for fallback chains.
// Each endpoint can have its own URL, API key, and model.
type Endpoint struct {
	URL   string
	Key   string
	Model string
}

// WithEndpoints sets fallback endpoint chains. Each endpoint is tried
// in order on retryable errors. Overrides the base URL/key/model when set.
func WithEndpoints(endpoints []Endpoint) Option {
	return func(c *Client) { c.endpoints = endpoints }
}

// Middleware wraps chat completion calls. Use for logging, metrics, caching.
// The next function sends the request to the API (or the next middleware).
// First added middleware is the outermost wrapper.
type Middleware func(ctx context.Context, req *ChatRequest, next func(context.Context, *ChatRequest) (*ChatResponse, error)) (*ChatResponse, error)

// WithMiddleware adds a middleware to the execution pipeline.
func WithMiddleware(m Middleware) Option {
	return func(c *Client) { c.middleware = append(c.middleware, m) }
}
```

Add fields to Client struct:

```go
type Client struct {
	baseURL      string
	apiKey       string
	model        string
	maxTokens    int
	temperature  float64
	httpClient   *http.Client
	fallbackKeys []string
	maxRetries   int
	endpoints    []Endpoint
	middleware   []Middleware
}
```

#### 1c. Refactor doRequest + doWithRetry — add baseURL parameter

Change signatures to accept `baseURL` as first parameter after ctx:

```go
func (c *Client) doWithRetry(ctx context.Context, baseURL, apiKey string, req *ChatRequest) (*ChatResponse, error) {
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

		result, err := c.doRequest(ctx, baseURL, apiKey, req)
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

func (c *Client) doRequest(ctx context.Context, baseURL, apiKey string, req *ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq) //nolint:gosec // G704: URL comes from caller config, not user input
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

#### 1d. Refactor execute — middleware chain + endpoint support

Replace the current `execute` method with three methods:

```go
func (c *Client) execute(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(c.middleware) == 0 {
		return c.executeInner(ctx, req)
	}
	return c.buildChain(0)(ctx, req)
}

func (c *Client) buildChain(i int) func(context.Context, *ChatRequest) (*ChatResponse, error) {
	if i >= len(c.middleware) {
		return c.executeInner
	}
	return func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
		return c.middleware[i](ctx, req, c.buildChain(i+1))
	}
}

func (c *Client) executeInner(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if len(c.endpoints) > 0 {
		var lastErr error
		for _, ep := range c.endpoints {
			epReq := *req
			if ep.Model != "" {
				epReq.Model = ep.Model
			}
			result, err := c.doWithRetry(ctx, ep.URL, ep.Key, &epReq)
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
	result, err := c.doWithRetry(ctx, c.baseURL, c.apiKey, req)
	if err == nil {
		return result, nil
	}
	for _, key := range c.fallbackKeys {
		if key == "" {
			continue
		}
		result, err = c.doWithRetry(ctx, c.baseURL, key, req)
		if err == nil {
			return result, nil
		}
	}
	return nil, err
}
```

#### 1e. Refactor Stream — endpoint support + baseURL

Update `doStreamRequest` to accept baseURL:

```go
func (c *Client) doStreamRequest(ctx context.Context, baseURL, apiKey string, req *ChatRequest) (*StreamResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", strings.NewReader(string(body)))
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

Update `Stream` to use endpoints when available:

```go
func (c *Client) Stream(ctx context.Context, messages []Message, opts ...ChatOption) (*StreamResponse, error) {
	var cfg chatConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	req := c.newRequest(messages)
	req.Stream = true
	cfg.apply(req)

	if len(c.endpoints) > 0 {
		var lastErr error
		for _, ep := range c.endpoints {
			epReq := *req
			if ep.Model != "" {
				epReq.Model = ep.Model
			}
			sr, err := c.doStreamRequest(ctx, ep.URL, ep.Key, &epReq)
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

	keys := make([]string, 0, 1+len(c.fallbackKeys))
	keys = append(keys, c.apiKey)
	keys = append(keys, c.fallbackKeys...)

	var lastErr error
	for _, key := range keys {
		if key == "" {
			continue
		}
		sr, err := c.doStreamRequest(ctx, c.baseURL, key, req)
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
```

#### 1f. Extract — new file llm/extract.go

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ExtractOption configures Extract behavior.
type ExtractOption func(*extractConfig)

type extractConfig struct {
	maxRetries int
	validator  func(any) error
}

// WithValidator sets a validation function called after unmarshalling.
// If it returns an error, Extract retries with the error fed back to the LLM.
func WithValidator(fn func(any) error) ExtractOption {
	return func(c *extractConfig) { c.validator = fn }
}

// WithExtractRetries sets the maximum number of extraction retries (default 3).
func WithExtractRetries(n int) ExtractOption {
	return func(c *extractConfig) { c.maxRetries = n }
}

// Extract sends a structured output request, unmarshals into target,
// and validates with the optional validator. On validation failure, the error
// is fed back to the LLM and the request is retried.
//
// This is the Go equivalent of Python's Instructor pattern:
// JSON Schema → structured output → validate → retry with error feedback.
func (c *Client) Extract(ctx context.Context, messages []Message, target any, opts ...ExtractOption) error {
	cfg := extractConfig{maxRetries: 3}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.maxRetries < 1 {
		cfg.maxRetries = 1
	}

	schema := SchemaOf(target)
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := strings.ToLower(t.Name())
	if name == "" {
		name = "response"
	}

	msgs := make([]Message, len(messages))
	copy(msgs, messages)

	for attempt := range cfg.maxRetries {
		resp, err := c.Chat(ctx, msgs, WithJSONSchema(name, schema))
		if err != nil {
			return err
		}

		// Reset target before unmarshal to avoid stale fields from previous attempt.
		reflect.ValueOf(target).Elem().SetZero()

		if err := json.Unmarshal([]byte(resp.Content), target); err != nil {
			if attempt == cfg.maxRetries-1 {
				return fmt.Errorf("extract: unmarshal failed after %d attempts: %w", cfg.maxRetries, err)
			}
			msgs = append(msgs,
				Message{Role: "assistant", Content: resp.Content},
				Message{Role: "user", Content: "JSON parsing failed: " + err.Error() + ". Please fix the JSON and try again."},
			)
			continue
		}

		if cfg.validator == nil {
			return nil
		}

		if err := cfg.validator(target); err != nil {
			if attempt == cfg.maxRetries-1 {
				return fmt.Errorf("extract: validation failed after %d attempts: %w", cfg.maxRetries, err)
			}
			msgs = append(msgs,
				Message{Role: "assistant", Content: resp.Content},
				Message{Role: "user", Content: "Validation error: " + err.Error() + ". Please fix and try again."},
			)
			continue
		}

		return nil
	}

	return fmt.Errorf("extract: exhausted %d retries", cfg.maxRetries)
}
```

**Step 1:** Apply all changes (1a-1f).

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1
```
Expected: All 18 existing tests PASS.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add llm/client.go llm/chat.go llm/stream.go llm/extract.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "llm: add Extract, endpoint fallback, middleware

Three Phase B additions:
- Extract: instructor-style typed extraction with validation retry
- Endpoint: model-level fallback chains (URL + key + model per endpoint)
- Middleware: request/response interceptor chain for logging/metrics/caching
- ChatRequest exported for middleware access

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new features

**Files:** llm/client_test.go

Add `"fmt"` to imports if not already present (it is).

**Test: Extract without validator**

```go
func TestExtract_Success(t *testing.T) {
	srv := newTestServer(t, chatHandler(`{"name":"Alice","age":30}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	err := c.Extract(context.Background(),
		[]llm.Message{{Role: "user", Content: "info"}},
		&result,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("got %+v, want Alice/30", result)
	}
}
```

**Test: Extract with validation retry**

```go
func TestExtract_ValidationRetry(t *testing.T) {
	type person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	var calls atomic.Int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			chatHandler(`{"name":"","age":0}`, nil, "stop")(w, r)
		} else {
			chatHandler(`{"name":"Alice","age":30}`, nil, "stop")(w, r)
		}
	})
	c := llm.NewClient(srv.URL, "key", "model")

	var result person
	err := c.Extract(context.Background(),
		[]llm.Message{{Role: "user", Content: "info"}},
		&result,
		llm.WithValidator(func(v any) error {
			p := v.(*person)
			if p.Name == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", result.Name)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (retry on validation)", calls.Load())
	}
}
```

**Test: Extract exhausted retries**

```go
func TestExtract_ExhaustedRetries(t *testing.T) {
	srv := newTestServer(t, chatHandler(`{"name":""}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	type person struct {
		Name string `json:"name"`
	}
	var result person
	err := c.Extract(context.Background(),
		[]llm.Message{{Role: "user", Content: "info"}},
		&result,
		llm.WithValidator(func(v any) error {
			p := v.(*person)
			if p.Name == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		}),
		llm.WithExtractRetries(2),
	)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error = %q, want to contain 'validation failed'", err)
	}
}
```

**Test: Endpoint fallback**

```go
func TestEndpoints_Fallback(t *testing.T) {
	primary := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	fallback := newTestServer(t, chatHandler("from fallback", nil, "stop"))

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: primary.URL, Key: "key1", Model: "fast"},
			{URL: fallback.URL, Key: "key2", Model: "big"},
		}),
		llm.WithMaxRetries(1),
	)

	result, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "from fallback" {
		t.Errorf("result = %q, want %q", result, "from fallback")
	}
}
```

**Test: Endpoint model override**

```go
func TestEndpoints_ModelOverride(t *testing.T) {
	var capturedModel string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		capturedModel, _ = req["model"].(string)
		chatHandler("ok", nil, "stop")(w, r)
	})

	c := llm.NewClient("", "", "default-model",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "key", Model: "custom-model"},
		}),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedModel != "custom-model" {
		t.Errorf("model = %q, want %q", capturedModel, "custom-model")
	}
}
```

**Test: Endpoint stream fallback**

```go
func TestEndpoints_StreamFallback(t *testing.T) {
	primary := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	fallback := newTestServer(t, sseHandler([]string{
		`{"choices":[{"delta":{"content":"Hi"},"finish_reason":""}]}`,
	}))

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: primary.URL, Key: "key1", Model: "fast"},
			{URL: fallback.URL, Key: "key2", Model: "big"},
		}),
	)

	stream, err := c.Stream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	chunk, ok := stream.Next()
	if !ok {
		t.Fatal("expected at least one chunk")
	}
	if chunk.Delta != "Hi" {
		t.Errorf("delta = %q, want Hi", chunk.Delta)
	}
}
```

**Test: Middleware called**

```go
func TestMiddleware_Called(t *testing.T) {
	srv := newTestServer(t, chatHandler("ok", nil, "stop"))

	var called bool
	c := llm.NewClient(srv.URL, "key", "model",
		llm.WithMiddleware(func(ctx context.Context, req *llm.ChatRequest, next func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
			called = true
			return next(ctx, req)
		}),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("middleware should have been called")
	}
}
```

**Test: Middleware chain order**

```go
func TestMiddleware_Chain(t *testing.T) {
	srv := newTestServer(t, chatHandler("ok", nil, "stop"))

	var order []string
	mw := func(name string) llm.Middleware {
		return func(ctx context.Context, req *llm.ChatRequest, next func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
			order = append(order, name+":before")
			resp, err := next(ctx, req)
			order = append(order, name+":after")
			return resp, err
		}
	}

	c := llm.NewClient(srv.URL, "key", "model",
		llm.WithMiddleware(mw("first")),
		llm.WithMiddleware(mw("second")),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "first:before,second:before,second:after,first:after"
	got := strings.Join(order, ",")
	if got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}
```

**Step 1:** Add all 8 tests to client_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./llm/ -v -count=1
```
Expected: All 26 tests PASS (18 existing + 8 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add llm/client_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "llm: add tests for Extract, endpoints, middleware

8 new tests: Extract (success/retry/exhausted), Endpoint fallback
(complete/model-override/stream), Middleware (called/chain-order).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update llm section with new features:

```go
import "github.com/anatolykoptev/go-kit/llm"

client := llm.NewClient(baseURL, apiKey, model)

// Basic completion
result, _ := client.Complete(ctx, "system prompt", "user message")

// Structured extraction with validation retry (Instructor-style)
type Recipe struct {
    Name        string   `json:"name"`
    Ingredients []string `json:"ingredients"`
}
var recipe Recipe
err := client.Extract(ctx, messages, &recipe,
    llm.WithValidator(func(v any) error {
        r := v.(*Recipe)
        if len(r.Ingredients) == 0 {
            return errors.New("need at least one ingredient")
        }
        return nil
    }),
)

// Model-level fallback chains
client = llm.NewClient("", "", "",
    llm.WithEndpoints([]llm.Endpoint{
        {URL: geminiURL, Key: key1, Model: "gemini-2.5-flash"},
        {URL: openaiURL, Key: key2, Model: "gpt-4o"},
    }),
)

// Middleware for logging, metrics, caching
client = llm.NewClient(baseURL, apiKey, model,
    llm.WithMiddleware(func(ctx context.Context, req *llm.ChatRequest, next func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
        start := time.Now()
        resp, err := next(ctx, req)
        log.Printf("LLM call took %v", time.Since(start))
        return resp, err
    }),
)
```

Update bullet points:
- Extract with validation retry (Instructor-style, no Go library does this)
- Model-level endpoint fallback chains (URL + key + model per endpoint)
- Request/response middleware for logging, metrics, caching

**ROADMAP changes:**
- Mark llm Phase B as DONE (B1 Extract, B2 Endpoints, B3 Middleware)

**Step 1:** Update README.md llm section.

**Step 2:** Update ROADMAP.md llm Phase B status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update llm section for Phase B features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
