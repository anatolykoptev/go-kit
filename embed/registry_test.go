package embed

import (
	"context"
	"testing"
)

// fakeEmbedder is a minimal Embedder used by registry tests.
type fakeEmbedder struct {
	name    string
	dim     int
	closed  bool
	embedFn func(context.Context, []string) ([][]float32, error)
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.embedFn != nil {
		return f.embedFn(ctx, texts)
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, f.dim)
	}
	return out, nil
}

func (f *fakeEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return EmbedQueryViaEmbed(ctx, f, text)
}
func (f *fakeEmbedder) Dimension() int { return f.dim }
func (f *fakeEmbedder) Close() error   { f.closed = true; return nil }

func TestRegistry_GetFallback(t *testing.T) {
	r := NewRegistry("default-model")
	def := &fakeEmbedder{name: "default", dim: 1024}
	r.Register("default-model", def)

	got, ok := r.Get("")
	if !ok {
		t.Fatal("expected Get(\"\") to fall back to default-model")
	}
	if got != def {
		t.Errorf("Get(\"\") returned wrong embedder")
	}
}

func TestRegistry_GetByName(t *testing.T) {
	r := NewRegistry("default")
	a := &fakeEmbedder{name: "a", dim: 1024}
	b := &fakeEmbedder{name: "b", dim: 768}
	r.Register("a", a)
	r.Register("b", b)

	got, ok := r.Get("b")
	if !ok || got != b {
		t.Errorf("Get(b) failed")
	}
	got, ok = r.Get("a")
	if !ok || got != a {
		t.Errorf("Get(a) failed")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry("default")
	if _, ok := r.Get("nope"); ok {
		t.Error("expected ok=false for missing name")
	}
}

func TestRegistry_Close(t *testing.T) {
	r := NewRegistry("default")
	a := &fakeEmbedder{name: "a", dim: 1024}
	b := &fakeEmbedder{name: "b", dim: 768}
	r.Register("a", a)
	r.Register("b", b)
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !a.closed || !b.closed {
		t.Error("expected all embedders to be closed")
	}
}
