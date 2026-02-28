// Package llm provides an OpenAI-compatible LLM client with retry and fallback keys.
// Supports text and multimodal (vision) requests. Zero external dependencies
// beyond net/http. Designed to replace duplicated LLM clients across go-* services.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default client constants.
const (
	defaultMaxTokens  = 8192
	defaultMaxRetries = 3
	defaultTimeout    = 90 * time.Second
	retryDelay        = 500 * time.Millisecond
	maxRetryDelay     = 5 * time.Second
)

// Client is an OpenAI-compatible LLM client with retry and fallback key support.
type Client struct {
	baseURL      string
	apiKey       string
	model        string
	maxTokens    int
	temperature  float64
	httpClient   *http.Client
	fallbackKeys []string
	maxRetries   int
}

// Option configures the Client.
type Option func(*Client)

// WithFallbackKeys sets fallback API keys tried when the primary gets 429/5xx.
func WithFallbackKeys(keys []string) Option {
	return func(c *Client) { c.fallbackKeys = keys }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithMaxTokens sets the max tokens for completions.
func WithMaxTokens(n int) Option {
	return func(c *Client) { c.maxTokens = n }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) Option {
	return func(c *Client) { c.temperature = t }
}

// WithMaxRetries sets how many times to retry on retryable errors.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// NewClient creates a new LLM client.
func NewClient(baseURL, apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		maxTokens:   defaultMaxTokens,
		temperature: 0.1,
		maxRetries:  defaultMaxRetries,
		httpClient:  &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentPart for multimodal
}

// ContentPart is a part of a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL holds an image reference for vision requests.
type ImageURL struct {
	URL string `json:"url"`
}

// ImagePart is a convenience type for passing images to CompleteMultimodal.
type ImagePart struct {
	URL      string
	MIMEType string // optional
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends a text completion request with optional system prompt.
// If system is empty, only the user message is sent.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	var msgs []Message
	if system != "" {
		msgs = append(msgs, Message{Role: "system", Content: system})
	}
	msgs = append(msgs, Message{Role: "user", Content: user})
	return c.CompleteRaw(ctx, msgs)
}

// CompleteMultimodal sends a vision request with text + images.
func (c *Client) CompleteMultimodal(ctx context.Context, prompt string, images []ImagePart) (string, error) {
	parts := []ContentPart{{Type: "text", Text: prompt}}
	for _, img := range images {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: img.URL},
		})
	}
	msgs := []Message{{Role: "user", Content: parts}}
	return c.CompleteRaw(ctx, msgs)
}

// CompleteRaw sends a chat completion with explicit messages.
// Retries on 429/5xx, cycles through fallback keys.
func (c *Client) CompleteRaw(ctx context.Context, messages []Message) (string, error) {
	// Try primary key.
	result, err := c.doWithRetry(ctx, c.apiKey, messages)
	if err == nil {
		return result, nil
	}

	// Try fallback keys.
	for _, key := range c.fallbackKeys {
		if key == "" {
			continue
		}
		result, err = c.doWithRetry(ctx, key, messages)
		if err == nil {
			return result, nil
		}
	}
	return "", err
}

func (c *Client) doWithRetry(ctx context.Context, apiKey string, messages []Message) (string, error) {
	delay := retryDelay
	var lastErr error

	for attempt := range c.maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, maxRetryDelay)
		}

		result, err := c.doRequest(ctx, apiKey, messages)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Only retry on retryable errors.
		var re *retryableError
		if !asRetryable(err, &re) {
			return "", err
		}
	}
	return "", lastErr
}

func (c *Client) doRequest(ctx context.Context, apiKey string, messages []Message) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL comes from caller config, not user input
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if isRetryableStatus(resp.StatusCode) {
		return "", &retryableError{statusCode: resp.StatusCode, body: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", errors.New("llm: empty choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

type retryableError struct {
	statusCode int
	body       string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable HTTP %d: %s", e.statusCode, e.body)
}

func asRetryable(err error, target **retryableError) bool {
	for err != nil {
		if re, ok := err.(*retryableError); ok { //nolint:errorlint // intentional direct type assertion for internal error type
			*target = re
			return true
		}
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok { //nolint:errorlint // intentional
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// ExtractJSON extracts a JSON object from LLM output that may be wrapped
// in markdown code fences or surrounded by text.
func ExtractJSON(s string) string {
	// Try markdown ```json ... ``` first.
	start := strings.Index(s, "```json")
	if start >= 0 {
		s = s[start+7:]
		end := strings.Index(s, "```")
		if end >= 0 {
			return strings.TrimSpace(s[:end])
		}
	}
	// Fall back to finding first { and last }.
	first := strings.IndexByte(s, '{')
	last := strings.LastIndexByte(s, '}')
	if first >= 0 && last > first {
		return s[first : last+1]
	}
	return s
}
