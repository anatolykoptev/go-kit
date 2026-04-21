package rerank

import (
	"log/slog"
	"net/http"
	"time"
)

// defaultMaxDocs caps docs shipped to the server when Config.MaxDocs is 0.
const defaultMaxDocs = 50

// respBodyLimit bounds response body read to avoid runaway allocations on a
// misbehaving server. Rerank responses are small JSON; 256 KB covers
// pathological top_n values.
const respBodyLimit = 256 * 1024

// Config configures a rerank client. Zero URL disables all calls.
type Config struct {
	URL            string        // base URL, e.g. "http://embed-server:8082"
	Model          string        // model name in request body
	APIKey         string        // optional Bearer token (Cohere hosted providers)
	Timeout        time.Duration // per-request HTTP timeout
	MaxDocs        int           // cap on docs sent (0 → defaultMaxDocs)
	MaxCharsPerDoc int           // rune-aware truncation (0 disables)
}

// Doc is a query-document pair. ID is opaque, returned unchanged in Scored.
type Doc struct {
	ID   string
	Text string
}

// Scored pairs an input Doc with its relevance score from the reranker.
// OrigRank is the original index of this doc in the input slice.
type Scored struct {
	Doc
	Score    float32
	OrigRank int
}

// Client is the rerank HTTP client. Safe for concurrent use.
type Client struct {
	cfg    Config
	logger *slog.Logger
	http   *http.Client
}

// New returns a configured client. logger=nil uses slog.Default().
func New(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		logger: logger,
		http:   &http.Client{},
	}
}

// Available reports whether the client is configured to make calls.
func (c *Client) Available() bool {
	return c != nil && c.cfg.URL != ""
}
