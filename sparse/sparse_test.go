package sparse

import (
	"context"
	"errors"
	"testing"
)

// TestSparseVector_IsEmpty covers the empty-vector predicate.
func TestSparseVector_IsEmpty(t *testing.T) {
	cases := []struct {
		name string
		v    SparseVector
		want bool
	}{
		{"zero", SparseVector{}, true},
		{"nil_slices", SparseVector{Indices: nil, Values: nil}, true},
		{"populated", SparseVector{Indices: []uint32{1}, Values: []float32{0.5}}, false},
		{"only_indices", SparseVector{Indices: []uint32{1}, Values: nil}, false},
		{"only_values", SparseVector{Indices: nil, Values: []float32{0.5}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.IsEmpty(); got != tc.want {
				t.Errorf("IsEmpty: want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestSparseVector_Len returns the smaller of the two slice lengths.
func TestSparseVector_Len(t *testing.T) {
	cases := []struct {
		name string
		v    SparseVector
		want int
	}{
		{"empty", SparseVector{}, 0},
		{"matched", SparseVector{Indices: []uint32{1, 2}, Values: []float32{0.5, 0.6}}, 2},
		{"more_indices", SparseVector{Indices: []uint32{1, 2}, Values: []float32{0.5}}, 1},
		{"more_values", SparseVector{Indices: []uint32{1}, Values: []float32{0.5, 0.6}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.Len(); got != tc.want {
				t.Errorf("Len: want %d, got %d", tc.want, got)
			}
		})
	}
}

// fakeEmbedder is a SparseEmbedder used to test the
// EmbedSparseQueryViaEmbed helper without any network.
type fakeEmbedder struct {
	embedded []string
	out      []SparseVector
	err      error
}

func (f *fakeEmbedder) EmbedSparse(_ context.Context, texts []string) ([]SparseVector, error) {
	f.embedded = append(f.embedded, texts...)
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}
func (f *fakeEmbedder) EmbedSparseQuery(_ context.Context, _ string) (SparseVector, error) {
	return SparseVector{}, nil
}
func (f *fakeEmbedder) VocabSize() int { return 30522 }
func (f *fakeEmbedder) Close() error   { return nil }

// TestEmbedSparseQueryViaEmbed_Success verifies the helper unwraps a
// single-element slice correctly.
func TestEmbedSparseQueryViaEmbed_Success(t *testing.T) {
	want := SparseVector{Indices: []uint32{1, 2}, Values: []float32{0.5, 0.6}}
	f := &fakeEmbedder{out: []SparseVector{want}}
	got, err := EmbedSparseQueryViaEmbed(context.Background(), f, "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Len() != 2 {
		t.Errorf("len: %d", got.Len())
	}
	if len(f.embedded) != 1 || f.embedded[0] != "hello" {
		t.Errorf("delegated input wrong: %v", f.embedded)
	}
}

// TestEmbedSparseQueryViaEmbed_Error propagates the underlying error.
func TestEmbedSparseQueryViaEmbed_Error(t *testing.T) {
	f := &fakeEmbedder{err: errors.New("boom")}
	_, err := EmbedSparseQueryViaEmbed(context.Background(), f, "hello")
	if err == nil || err.Error() != "boom" {
		t.Errorf("want boom, got %v", err)
	}
}

// TestEmbedSparseQueryViaEmbed_EmptyResult covers the no-vector case.
func TestEmbedSparseQueryViaEmbed_EmptyResult(t *testing.T) {
	f := &fakeEmbedder{out: nil}
	v, err := EmbedSparseQueryViaEmbed(context.Background(), f, "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !v.IsEmpty() {
		t.Errorf("want empty vector, got %+v", v)
	}
}
