package rerank

import (
	"context"
	"log/slog"
	"sort"
	"time"
)

// defaultMaxDocs caps docs shipped to the server when MaxDocs is 0.
const defaultMaxDocs = 50

// respBodyLimit bounds response body read to avoid runaway allocations on a
// misbehaving server. Rerank responses are small JSON; 256 KB covers
// pathological top_n values.
const respBodyLimit = 256 * 1024

// Config configures a rerank client. Zero URL disables all calls.
// Deprecated: use NewClient with functional options (Opt) instead.
type Config struct {
	URL            string        // base URL, e.g. "http://embed-server:8082"
	Model          string        // model name in request body
	APIKey         string        // optional Bearer token (Cohere hosted providers)
	Timeout        time.Duration // per-request HTTP timeout (applied via context.WithTimeout, NOT http.Client.Timeout)
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
// v2: internally holds *cfgInternal; v1 New(cfg, logger) translates Config to options.
type Client struct {
	cfg    *cfgInternal
	logger *slog.Logger // kept for v1 compat; v1 logs via c.logger.Warn directly
}

// New returns a configured client using the v1 Config struct.
// logger=nil uses slog.Default().
// Deprecated: use NewClient(url, opts...) for new code.
//
// G1 note: the default retry policy (retry-on-5xx, 3 attempts) is now active
// for v1 callers. Opt out via NewClient(url, WithRetry(rerank.NoRetry)).
func New(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	opts := []Opt{
		WithModel(cfg.Model),
		WithAPIKey(cfg.APIKey),
		WithTimeout(cfg.Timeout),
		WithMaxDocs(cfg.MaxDocs),
		WithMaxCharsPerDoc(cfg.MaxCharsPerDoc),
	}
	c := NewClient(cfg.URL, opts...)
	c.logger = logger
	return c
}

// Available reports whether the client is configured to make calls.
func (c *Client) Available() bool {
	return c != nil && c.cfg != nil && c.cfg.url != ""
}

// Rerank returns docs sorted by cross-encoder relevance score (desc). Best-
// effort: any error returns input unchanged (preserving order, Score=0,
// OrigRank=i). Docs beyond MaxDocs are preserved as-is after the reranked
// head.
//
// Deprecated: use RerankWithResult for new code (typed Result with Status).
func (c *Client) Rerank(ctx context.Context, query string, docs []Doc) []Scored {
	res, _ := c.RerankWithResult(ctx, query, docs)
	if res == nil {
		out := make([]Scored, len(docs))
		for i, d := range docs {
			out[i] = Scored{Doc: d, OrigRank: i}
		}
		return out
	}
	return res.Scored
}

// rerankInternal executes the full rerank pipeline and returns a Result.
// Shared by RerankWithResult (and transitively the v1 Rerank shim).
func (c *Client) rerankInternal(ctx context.Context, query string, docs []Doc) *Result {
	pass := func() []Scored {
		out := make([]Scored, len(docs))
		for i, d := range docs {
			out[i] = Scored{Doc: d, OrigRank: i}
		}
		return out
	}

	if len(docs) == 0 || c == nil || c.cfg == nil || c.cfg.url == "" {
		return &Result{
			Scored: pass(),
			Status: StatusSkipped,
			Model:  c.cfgModel(),
		}
	}

	maxDocs := c.cfg.maxDocs
	if maxDocs <= 0 {
		maxDocs = defaultMaxDocs
	}

	head := docs
	var tail []Doc
	if len(docs) > maxDocs {
		head = docs[:maxDocs]
		tail = docs[maxDocs:]
	}

	// Extract texts (with optional rune-aware truncation).
	texts := make([]string, len(head))
	for i, d := range head {
		t := d.Text
		if c.cfg.maxCharsPerDoc > 0 {
			t = truncateRunes(t, c.cfg.maxCharsPerDoc)
		}
		texts[i] = t
	}

	// Fire OnBeforeCall hook.
	safeCall(func() { c.cfg.observer.OnBeforeCall(ctx, query, len(texts)) })

	start := time.Now()
	// G1: callCohereResilient wraps callCohere with retry + circuit breaker.
	resp, err := c.callCohereResilient(ctx, query, texts)
	dur := time.Since(start)
	recordDuration(c.cfg.model, dur)

	if err != nil {
		if c.logger != nil {
			c.logger.Warn("rerank failed",
				slog.String("url", c.cfg.url),
				slog.String("model", c.cfg.model),
				slog.Int("docs", len(texts)),
				slog.Any("err", err),
			)
		}
		recordStatus(c.cfg.model, "error")
		scored := pass()
		safeCall(func() { c.cfg.observer.OnAfterCall(ctx, StatusDegraded, dur, len(scored)) })
		return &Result{
			Scored: scored,
			Status: StatusDegraded,
			Model:  c.cfg.model,
			Err:    err,
		}
	}
	recordStatus(c.cfg.model, "ok")

	// Build scored head in server-returned order. Missing docs keep score=0
	// and get sorted to tail of the head block.
	scores := make([]float32, len(head))
	seen := make([]bool, len(head))
	for _, r := range resp.Results {
		if r.Index < 0 || r.Index >= len(head) {
			continue // defensive
		}
		scores[r.Index] = float32(r.RelevanceScore)
		seen[r.Index] = true
	}

	// Sort indices by score desc (stable). Sort through a permutation so the
	// comparator reads from a stable scores array — NOT from shuffled items.
	order := make([]int, len(head))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		// Unseen docs go to the end of the reranked block.
		if seen[order[i]] != seen[order[j]] {
			return seen[order[i]]
		}
		return scores[order[i]] > scores[order[j]]
	})

	out := make([]Scored, 0, len(docs))
	for _, origIdx := range order {
		out = append(out, Scored{
			Doc:      head[origIdx],
			Score:    scores[origIdx],
			OrigRank: origIdx,
		})
	}
	// Preserve tail in original order at the end.
	for i, d := range tail {
		out = append(out, Scored{
			Doc:      d,
			Score:    0,
			OrigRank: maxDocs + i,
		})
	}

	model := c.cfg.model
	if resp.Model != "" {
		model = resp.Model
	}
	safeCall(func() { c.cfg.observer.OnAfterCall(ctx, StatusOk, dur, len(out)) })
	return &Result{
		Scored: out,
		Status: StatusOk,
		Model:  model,
	}
}

// callCohereResilient wraps callCohere with:
//  1. Circuit breaker check (if configured) — returns ErrCircuitOpen immediately if open.
//  2. Retry loop via retry.do (default: 3 attempts on 5xx, exp backoff).
//  3. Circuit breaker feedback (MarkSuccess/MarkFailure if configured).
//
// This is the single wrap point per G1 spec.
func (c *Client) callCohereResilient(ctx context.Context, query string, texts []string) (*cohereResponse, error) {
	cb := c.cfg.circuit

	// 1. Circuit breaker guard.
	if cb != nil && !cb.Allow() {
		recordGiveup(c.cfg.model, "circuit_open")
		return nil, ErrCircuitOpen
	}

	// 2. Retry loop.
	resp, err := do(ctx, c.cfg.retry, c.cfg.model, c.cfg.observer, func() (*cohereResponse, error) {
		return c.callCohere(ctx, query, texts)
	})

	// 3. Circuit breaker feedback.
	if cb != nil {
		if err != nil {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
	}

	return resp, err
}

// cfgModel returns model name safely (nil-safe helper for Result.Model).
func (c *Client) cfgModel() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return c.cfg.model
}

// truncateRunes returns the first maxRunes runes of s. UTF-8 safe.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
