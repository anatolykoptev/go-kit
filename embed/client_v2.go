package embed

import (
	"context"
	"errors"
	"fmt"
)

// NewClient is the v2 entry point — returns an Embedder configured via
// functional options. v1 callers continue to use New(cfg, logger) which
// calls the per-backend helpers directly.
//
// url is the backend URL when applicable. For Ollama/HTTP backends, pass the
// base URL. For Voyage, url is ignored (endpoint is hardcoded by the API).
// For ONNX, use the embed/onnx subpackage directly.
//
// At least one backend-specific Opt must be applied; otherwise NewClient
// returns an error from the underlying constructor.
func NewClient(url string, opts ...Opt) (Embedder, error) {
	cfg := defaultCfg()
	cfg.url = url
	for _, opt := range opts {
		opt(cfg)
	}
	return newFromInternal(cfg)
}

// EmbedOpt is a per-call option for EmbedWithResult.
type EmbedOpt func(*embedCallCfg)

type embedCallCfg struct {
	DryRun bool
}

// WithDryRun skips the backend call entirely and returns Status=Skipped vectors
// of zero length. For testing pipeline wiring without a live server.
func WithDryRun() EmbedOpt {
	return func(c *embedCallCfg) { c.DryRun = true }
}

// EmbedWithResult is the v2 Embed API. Returns a typed Result with Status so
// callers can distinguish failure modes:
//   - StatusOk       — request succeeded, vectors are valid
//   - StatusDegraded — request failed, Err is set
//   - StatusSkipped  — nil embedder, empty texts, or DryRun enabled
//
// E1 wires retry/circuit/fallback on top of this shim.
// E2 wires auto-batching, E3 wires cache, E4 wires per-text Status reasoning.
func EmbedWithResult(ctx context.Context, e Embedder, texts []string, opts ...EmbedOpt) (*Result, error) {
	callCfg := embedCallCfg{}
	for _, o := range opts {
		o(&callCfg)
	}

	if e == nil {
		return &Result{Status: StatusSkipped}, nil
	}
	if len(texts) == 0 {
		return &Result{
			Status: StatusSkipped,
			Model:  modelFromEmbedder(e),
		}, nil
	}
	if callCfg.DryRun {
		zeros := make([]*Vector, len(texts))
		for i := range zeros {
			zeros[i] = &Vector{Status: StatusSkipped}
		}
		return &Result{
			Vectors: zeros,
			Status:  StatusSkipped,
			Model:   modelFromEmbedder(e),
		}, nil
	}

	raw, err := e.Embed(ctx, texts)
	if err != nil {
		return &Result{
			Vectors: emptyVectors(len(texts)),
			Status:  StatusDegraded,
			Model:   modelFromEmbedder(e),
			Err:     err,
		}, err
	}

	if len(raw) != len(texts) {
		partialErr := fmt.Errorf("embed: backend returned %d vectors, expected %d", len(raw), len(texts))
		return &Result{
			Vectors: emptyVectors(len(texts)),
			Status:  StatusDegraded,
			Model:   modelFromEmbedder(e),
			Err:     partialErr,
		}, errors.New("embed: partial response from backend")
	}

	out := make([]*Vector, len(raw))
	for i, v := range raw {
		out[i] = &Vector{
			Embedding: v,
			Dim:       len(v),
			Status:    StatusOk,
		}
	}
	return &Result{
		Vectors: out,
		Status:  StatusOk,
		Model:   modelFromEmbedder(e),
	}, nil
}

// modelFromEmbedder returns the backend model name when available.
// Resolution order:
//  1. Model() string interface — caller-supplied or custom Embedder that
//     exposes its model name (e.g. future embed/onnx extension).
//  2. Concrete type-switch for built-in backends (HTTPEmbedder, OllamaClient,
//     VoyageClient) — avoids requiring a public Model() method on each type.
//  3. Falls back to "" for unknown / opaque types.
func modelFromEmbedder(e Embedder) string {
	if e == nil {
		return ""
	}
	type modelGetter interface{ Model() string }
	if m, ok := e.(modelGetter); ok {
		return m.Model()
	}
	switch v := e.(type) {
	case *HTTPEmbedder:
		return v.model
	case *OllamaClient:
		return v.model
	case *VoyageClient:
		return v.model
	default:
		return ""
	}
}

// emptyVectors returns n placeholder Vector entries with Status=StatusSkipped.
func emptyVectors(n int) []*Vector {
	out := make([]*Vector, n)
	for i := range out {
		out[i] = &Vector{Status: StatusSkipped}
	}
	return out
}
