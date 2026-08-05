package embed

import (
	"context"
	"errors"
	"testing"
)

// stubEmbedderFull is a test embedder that records the texts it receives and
// returns fixed-dim vectors. Implements Embedder + modelGetter for full
// decorator coverage.
type stubEmbedderFull struct {
	dim     int
	model   string
	calls   [][]string
	queryIn string
}

func (s *stubEmbedderFull) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls = append(s.calls, texts)
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{float32(i), 0.5}
	}
	return out, nil
}

func (s *stubEmbedderFull) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	s.queryIn = text
	return []float32{0.5, 0.5}, nil
}

func (s *stubEmbedderFull) Dimension() int { return s.dim }
func (s *stubEmbedderFull) Close() error   { return nil }
func (s *stubEmbedderFull) Model() string  { return s.model }

// TestPrefixed_EmptyPrefixReturnsUnchanged verifies that Prefixed with an
// empty prefix returns the original Embedder, not a wrapper.
func TestPrefixed_EmptyPrefixReturnsUnchanged(t *testing.T) {
	inner := &stubEmbedderFull{dim: 4, model: "test"}
	got := Prefixed(inner, "")
	if got != inner {
		t.Fatal("empty prefix should return the original embedder, not a wrapper")
	}
}

// TestPrefixed_EmbedPrependsPrefix verifies that the prefix is prepended to
// every text in a batch Embed call.
func TestPrefixed_EmbedPrependsPrefix(t *testing.T) {
	inner := &stubEmbedderFull{dim: 2, model: "e5"}
	p := Prefixed(inner, E5PassagePrefix)

	_, err := p.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(inner.calls))
	}
	got := inner.calls[0]
	if len(got) != 2 {
		t.Fatalf("expected 2 texts, got %d", len(got))
	}
	if got[0] != "passage: hello" {
		t.Errorf("text[0]: want %q, got %q", "passage: hello", got[0])
	}
	if got[1] != "passage: world" {
		t.Errorf("text[1]: want %q, got %q", "passage: world", got[1])
	}
}

// TestPrefixed_EmbedQueryPrependsPrefix verifies that the prefix is prepended
// to the query text in EmbedQuery.
func TestPrefixed_EmbedQueryPrependsPrefix(t *testing.T) {
	inner := &stubEmbedderFull{dim: 2, model: "e5"}
	p := Prefixed(inner, E5QueryPrefix)

	_, err := p.EmbedQuery(context.Background(), "what is go-kit?")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if inner.queryIn != "query: what is go-kit?" {
		t.Errorf("query input: want %q, got %q", "query: what is go-kit?", inner.queryIn)
	}
}

// TestPrefixed_EmptyInputPassesThrough verifies that empty input is delegated
// without modification (no prefix prepended to nothing).
func TestPrefixed_EmptyInputPassesThrough(t *testing.T) {
	inner := &stubEmbedderFull{dim: 2, model: "e5"}
	p := Prefixed(inner, E5PassagePrefix)

	got, err := p.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result for empty input, got %d vectors", len(got))
	}
}

// TestPrefixed_DimensionAndCloseDelegate verifies that Dimension and Close
// delegate to the inner embedder.
func TestPrefixed_DimensionAndCloseDelegate(t *testing.T) {
	inner := &stubEmbedderFull{dim: 1024, model: "e5"}
	p := Prefixed(inner, E5PassagePrefix)

	if p.Dimension() != 1024 {
		t.Errorf("Dimension: want 1024, got %d", p.Dimension())
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestPrefixed_ModelDelegates verifies that the decorator is transparent to
// modelFromEmbedder's resolution chain.
func TestPrefixed_ModelDelegates(t *testing.T) {
	inner := &stubEmbedderFull{dim: 2, model: "multilingual-e5-large"}
	p := Prefixed(inner, E5PassagePrefix)

	if m := modelFromEmbedder(p); m != "multilingual-e5-large" {
		t.Errorf("modelFromEmbedder: want %q, got %q", "multilingual-e5-large", m)
	}
}

// TestPrefixed_TwoPrefixesOneBackend verifies the composition-over-configuration
// use case from issue #259: one backend, two prefix conventions (passage for
// storage, query for retrieval), without creating two HTTP connection pools.
func TestPrefixed_TwoPrefixesOneBackend(t *testing.T) {
	inner := &stubEmbedderFull{dim: 2, model: "e5"}
	docs := Prefixed(inner, E5PassagePrefix)
	queries := Prefixed(inner, E5QueryPrefix)

	// Embed documents with passage prefix.
	_, _ = docs.Embed(context.Background(), []string{"doc1", "doc2"})
	// Embed a query with query prefix.
	_, _ = queries.EmbedQuery(context.Background(), "search term")

	// Both went through the same inner embedder (one instance, one HTTP pool).
	if len(inner.calls) != 1 {
		t.Fatalf("expected 1 Embed call (from docs), got %d", len(inner.calls))
	}
	if inner.calls[0][0] != "passage: doc1" {
		t.Errorf("doc[0]: want %q, got %q", "passage: doc1", inner.calls[0][0])
	}
	if inner.queryIn != "query: search term" {
		t.Errorf("query: want %q, got %q", "query: search term", inner.queryIn)
	}
}

// TestPrefixed_ImplementsEmbedder is a compile-time check that *prefixed
// satisfies the Embedder interface (already asserted in prefixed.go, but
// this makes the intent visible in the test file).
func TestPrefixed_ImplementsEmbedder(t *testing.T) {
	var _ Embedder = Prefixed(&stubEmbedderFull{dim: 1}, "x")
}

// TestPrefixed_ErrorPropagates verifies that backend errors are returned
// unchanged through the decorator.
func TestPrefixed_ErrorPropagates(t *testing.T) {
	inner := &errEmbedderPrefixed{}
	p := Prefixed(inner, "prefix: ")

	_, err := p.Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errors.Is(err, errPrefixedSentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

var errPrefixedSentinel = errors.New("stub: backend down")

type errEmbedderPrefixed struct{}

func (errEmbedderPrefixed) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errPrefixedSentinel
}
func (errEmbedderPrefixed) EmbedQuery(context.Context, string) ([]float32, error) {
	return nil, errPrefixedSentinel
}
func (errEmbedderPrefixed) Dimension() int { return 1 }
func (errEmbedderPrefixed) Close() error   { return nil }

// --- BatchSizer tests ---

// batchSizerStub is a test embedder that implements BatchSizer.
type batchSizerStub struct {
	maxBatch int
}

func (b *batchSizerStub) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{float32(i)}
	}
	return out, nil
}
func (b *batchSizerStub) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	return []float32{0.1}, nil
}
func (b *batchSizerStub) Dimension() int    { return 1 }
func (b *batchSizerStub) Close() error      { return nil }
func (b *batchSizerStub) MaxBatchSize() int { return b.maxBatch }
func (b *batchSizerStub) Model() string     { return "stub-batch" }

// TestBatchSizer_ChunkSizeFromBackend verifies that when the inner embedder
// implements BatchSizer, the client's chunk size defaults to MaxBatchSize()
// instead of the hardcoded defaultChunkSize.
func TestBatchSizer_ChunkSizeFromBackend(t *testing.T) {
	const customBatch = 64
	inner := &batchSizerStub{maxBatch: customBatch}

	c, err := NewClient("", WithEmbedder(inner))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.chunkSize != customBatch {
		t.Errorf("chunkSize: want %d (from MaxBatchSize), got %d", customBatch, c.chunkSize)
	}
}

// TestBatchSizer_ExplicitChunkSizeOverridesBatchSizer verifies that
// WithChunkSize takes priority over MaxBatchSize.
func TestBatchSizer_ExplicitChunkSizeOverridesBatchSizer(t *testing.T) {
	inner := &batchSizerStub{maxBatch: 64}

	c, err := NewClient("", WithEmbedder(inner), WithChunkSize(16))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.chunkSize != 16 {
		t.Errorf("chunkSize: want 16 (explicit opt), got %d", c.chunkSize)
	}
}

// TestBatchSizer_NonImplementerFallsBackToDefault verifies that an embedder
// that does NOT implement BatchSizer falls back to defaultChunkSize.
func TestBatchSizer_NonImplementerFallsBackToDefault(t *testing.T) {
	inner := &stubEmbedderFull{dim: 1, model: "no-batch"}

	c, err := NewClient("", WithEmbedder(inner))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.chunkSize != defaultChunkSize {
		t.Errorf("chunkSize: want %d (default), got %d", defaultChunkSize, c.chunkSize)
	}
}

// TestBatchSizer_ConcreteBackendsImplementIt verifies that all built-in
// backends implement BatchSizer — a regression guard against accidental
// removal of the MaxBatchSize method.
func TestBatchSizer_ConcreteBackendsImplementIt(t *testing.T) {
	cases := []struct {
		name string
		bs   BatchSizer
		want int
	}{
		{"OllamaClient", &OllamaClient{}, ollamaMaxBatch},
		{"HTTPEmbedder", &HTTPEmbedder{}, defaultChunkSize},
		{"VoyageClient", &VoyageClient{}, voyageMaxBatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.bs.MaxBatchSize(); got != tc.want {
				t.Errorf("%s.MaxBatchSize() = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestPrefixed_BatchSizerPropagates verifies that the Prefixed decorator is
// transparent to the BatchSizer interface — wrapping a BatchSizer embedder
// preserves MaxBatchSize() so the chunk-size resolution in NewClient still
// sees the backend's self-reported optimum. Regression guard for the bug
// found in pr-review-council: prefixed initially did not implement BatchSizer,
// causing wrapped backends to silently fall back to defaultChunkSize.
func TestPrefixed_BatchSizerPropagates(t *testing.T) {
	const customBatch = 64
	inner := &batchSizerStub{maxBatch: customBatch}
	p := Prefixed(inner, "passage: ")

	// The decorator must satisfy BatchSizer.
	bs, ok := p.(BatchSizer)
	if !ok {
		t.Fatal("Prefixed wrapper of a BatchSizer embedder must implement BatchSizer")
	}
	if got := bs.MaxBatchSize(); got != customBatch {
		t.Errorf("MaxBatchSize through decorator: want %d, got %d", customBatch, got)
	}
}

// TestPrefixed_NonBatchSizerReturnsZero verifies that wrapping a non-BatchSizer
// embedder with Prefixed yields MaxBatchSize()=0 (falls back to default).
func TestPrefixed_NonBatchSizerReturnsZero(t *testing.T) {
	inner := &stubEmbedderFull{dim: 1, model: "no-batch"}
	p := Prefixed(inner, "x")

	bs, ok := p.(BatchSizer)
	if !ok {
		t.Fatal("Prefixed must always implement BatchSizer (returns 0 for non-implementers)")
	}
	if got := bs.MaxBatchSize(); got != 0 {
		t.Errorf("MaxBatchSize for non-BatchSizer inner: want 0, got %d", got)
	}
}

// TestPrefixed_WithBatchSizer_NewClient verifies the full composition path:
// a BatchSizer embedder wrapped with Prefixed, passed to NewClient via
// WithEmbedder, must yield the correct chunkSize from the backend's
// MaxBatchSize() — not fall back to defaultChunkSize.
func TestPrefixed_WithBatchSizer_NewClient(t *testing.T) {
	const customBatch = 48
	inner := &batchSizerStub{maxBatch: customBatch}
	wrapped := Prefixed(inner, E5PassagePrefix)

	c, err := NewClient("", WithEmbedder(wrapped))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.chunkSize != customBatch {
		t.Errorf("chunkSize through Prefixed+BatchSizer: want %d, got %d (fell back to default?)",
			customBatch, c.chunkSize)
	}
}
