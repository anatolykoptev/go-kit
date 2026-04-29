package rerank

import "context"

// NewClient is the v2 constructor. Use functional options to configure.
//
// Example:
//
//	c := rerank.NewClient("http://embed:8082",
//	    rerank.WithModel("bge-reranker-v2-m3"),
//	    rerank.WithTimeout(2*time.Second))
func NewClient(url string, opts ...Opt) *Client {
	cfg := defaultCfg()
	cfg.url = url
	for _, opt := range opts {
		opt(cfg)
	}
	return newFromInternal(cfg)
}

// newFromInternal builds a *Client from an already-resolved cfgInternal.
// Used by both NewClient (v2) and the v1 New() wrapper after option translation.
func newFromInternal(cfg *cfgInternal) *Client {
	return &Client{cfg: cfg}
}

// rerankCallCfg holds per-call options passed to RerankWithResult.
// Empty in G0; G2/G4 will add TopN, Threshold, DryRun fields.
type rerankCallCfg struct{}

// RerankOpt is a per-call option for RerankWithResult.
type RerankOpt func(*rerankCallCfg)

// RerankWithResult is the v2 Rerank API. Returns a typed Result with Status
// so callers can distinguish failure modes:
//   - StatusOk       — request succeeded, scores valid
//   - StatusDegraded — request failed, Scored contains input order Score=0
//   - StatusSkipped  — no URL configured or docs slice is empty
//
// StatusFallback is reserved for G1 (multi-model fallback) and never produced here.
func (c *Client) RerankWithResult(ctx context.Context, query string, docs []Doc, opts ...RerankOpt) (*Result, error) {
	// Per-call config (empty in G0; populated in G2/G4).
	_ = opts // consumed by future streams

	res := c.rerankInternal(ctx, query, docs)
	if res.Status == StatusDegraded {
		return res, res.Err
	}
	return res, nil
}
