package embed

import (
	"context"
	"log/slog"
)

// Client wraps an Embedder backend with v2 features (Observer hooks, future
// retry/circuit/cache from E1+ streams). Built via NewClient(url, opts...).
//
// Client itself implements Embedder, so it is drop-in replaceable for v1
// backends. v1 callers that hold the result as Embedder continue to work
// unchanged; v2 callers cast to *Client to call EmbedWithResult directly.
type Client struct {
	inner    Embedder     // underlying backend (HTTP/Ollama/Voyage/custom)
	observer Observer     // wired via WithObserver, fires lifecycle hooks
	logger   *slog.Logger // optional, defaults to slog.Default()
	model    string       // resolved model name (for Result.Model)

	// Reserved for E1+ streams:
	// retry RetryPolicy
	// circuit *CircuitBreaker
	// fallback *Client
	// cache Cache
}

// Embed satisfies the Embedder interface — delegates to the inner backend.
// E1+ wires retry/circuit/fallback/cache around this call.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if c == nil || c.inner == nil {
		return nil, nil
	}
	return c.inner.Embed(ctx, texts)
}

// EmbedQuery satisfies Embedder; delegates to inner.
func (c *Client) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if c == nil || c.inner == nil {
		return nil, nil
	}
	return c.inner.EmbedQuery(ctx, text)
}

// Dimension satisfies Embedder.
func (c *Client) Dimension() int {
	if c == nil || c.inner == nil {
		return 0
	}
	return c.inner.Dimension()
}

// Close satisfies Embedder; closes the inner backend.
func (c *Client) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// Model returns the resolved model name. Satisfies the optional modelGetter
// interface used by modelFromEmbedder's fallback chain.
func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

// Compile-time interface satisfaction.
var _ Embedder = (*Client)(nil)
