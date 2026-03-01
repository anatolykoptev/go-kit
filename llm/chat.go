package llm

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
)

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
