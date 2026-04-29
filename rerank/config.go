package rerank

import (
	"net/http"
	"time"
)

// cfgInternal holds the resolved configuration for a Client.
// Built from functional options (v2) or translated from Config (v1 wrapper).
type cfgInternal struct {
	url            string
	model          string
	apiKey         string
	timeout        time.Duration
	maxDocs        int
	maxCharsPerDoc int
	observer       Observer
	hc             *http.Client
}

// Opt is a functional option for NewClient.
type Opt func(*cfgInternal)

// defaultCfg returns a cfgInternal with sensible defaults.
func defaultCfg() *cfgInternal {
	return &cfgInternal{
		maxDocs:  defaultMaxDocs,
		observer: noopObserver{},
		hc:       &http.Client{},
	}
}

// WithModel sets the model name sent in the request body.
func WithModel(model string) Opt {
	return func(c *cfgInternal) { c.model = model }
}

// WithAPIKey sets the Bearer token for hosted reranker providers (e.g. Cohere).
func WithAPIKey(key string) Opt {
	return func(c *cfgInternal) { c.apiKey = key }
}

// WithTimeout sets the per-request HTTP timeout applied via context.WithTimeout.
func WithTimeout(d time.Duration) Opt {
	return func(c *cfgInternal) { c.timeout = d }
}

// WithMaxDocs caps the number of docs sent to the server per call.
// Docs beyond the cap are preserved in original order after the reranked head.
// 0 keeps the default (50).
func WithMaxDocs(n int) Opt {
	return func(c *cfgInternal) {
		if n > 0 {
			c.maxDocs = n
		}
	}
}

// WithMaxCharsPerDoc enables rune-aware truncation of each document text.
// 0 disables truncation. Kept for v1 compatibility; G2 will add WithMaxTokensPerDoc.
func WithMaxCharsPerDoc(n int) Opt {
	return func(c *cfgInternal) { c.maxCharsPerDoc = n }
}

// WithObserver registers an Observer that receives lifecycle callbacks.
// A nil observer is ignored (noopObserver stays active).
func WithObserver(obs Observer) Opt {
	return func(c *cfgInternal) {
		if obs != nil {
			c.observer = obs
		}
	}
}

// WithHTTPClient replaces the default *http.Client with the provided one.
// Useful for injecting custom transports, TLS config, or test round-trippers.
func WithHTTPClient(hc *http.Client) Opt {
	return func(c *cfgInternal) {
		if hc != nil {
			c.hc = hc
		}
	}
}
