package sparse

import (
	"context"
	"testing"
)

// fakeRegEmbedder is a minimal SparseEmbedder for registry tests. Names
// the type with a "Reg" suffix to avoid colliding with fakeEmbedder in
// sparse_test.go.
type fakeRegEmbedder struct {
	name   string
	closed bool
}

func (f *fakeRegEmbedder) EmbedSparse(_ context.Context, texts []string) ([]SparseVector, error) {
	return make([]SparseVector, len(texts)), nil
}
func (f *fakeRegEmbedder) EmbedSparseQuery(_ context.Context, _ string) (SparseVector, error) {
	return SparseVector{}, nil
}
func (f *fakeRegEmbedder) VocabSize() int { return 30522 }
func (f *fakeRegEmbedder) Close() error   { f.closed = true; return nil }

// TestRegistry_GetFallback verifies Get("") falls back to the default
// model name.
func TestRegistry_GetFallback(t *testing.T) {
	r := NewRegistry("default-model")
	def := &fakeRegEmbedder{name: "default"}
	r.Register("default-model", def)

	got, ok := r.Get("")
	if !ok {
		t.Fatal("expected Get(\"\") to fall back to default-model")
	}
	if got != def {
		t.Errorf("Get(\"\") returned wrong embedder")
	}
}

// TestRegistry_GetByName verifies named lookups return the registered
// embedder.
func TestRegistry_GetByName(t *testing.T) {
	r := NewRegistry("default")
	a := &fakeRegEmbedder{name: "a"}
	b := &fakeRegEmbedder{name: "b"}
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

// TestRegistry_GetMissing verifies an unregistered name returns
// ok=false.
func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry("default")
	if _, ok := r.Get("nope"); ok {
		t.Errorf("expected ok=false for unregistered name")
	}
}

// TestRegistry_Replace verifies Register replaces an existing entry.
func TestRegistry_Replace(t *testing.T) {
	r := NewRegistry("default")
	a := &fakeRegEmbedder{name: "a"}
	b := &fakeRegEmbedder{name: "b"}
	r.Register("k", a)
	r.Register("k", b)

	got, _ := r.Get("k")
	if got != b {
		t.Errorf("expected b after replace, got %v", got)
	}
}

// TestRegistry_CloseClosesAll verifies Close calls Close on every
// registered embedder.
func TestRegistry_CloseClosesAll(t *testing.T) {
	r := NewRegistry("default")
	a := &fakeRegEmbedder{name: "a"}
	b := &fakeRegEmbedder{name: "b"}
	r.Register("a", a)
	r.Register("b", b)

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !a.closed || !b.closed {
		t.Errorf("Close did not close all: a=%v b=%v", a.closed, b.closed)
	}
}
