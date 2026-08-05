package embed

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubCorpusInspector is a test CorpusInspector with a configurable stored dim.
type stubCorpusInspector struct {
	dim int
	err error
}

func (s *stubCorpusInspector) StoredDim(_ context.Context) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.dim, nil
}

// stubDimEmbedder is a minimal Embedder with a configurable dimension.
type stubDimEmbedder struct{ dim int }

func (s *stubDimEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
}
func (s *stubDimEmbedder) EmbedQuery(context.Context, string) ([]float32, error) {
	return nil, nil
}
func (s *stubDimEmbedder) Dimension() int { return s.dim }
func (s *stubDimEmbedder) Close() error   { return nil }

// TestCheckCorpusDim_Match verifies that matching dimensions return nil.
func TestCheckCorpusDim_Match(t *testing.T) {
	e := &stubDimEmbedder{dim: 1024}
	c := &stubCorpusInspector{dim: 1024}
	if err := CheckCorpusDim(context.Background(), e, c); err != nil {
		t.Errorf("expected nil for matching dims, got %v", err)
	}
}

// TestCheckCorpusDim_Mismatch verifies that different dimensions return
// *ErrCorpusDimMismatch with the correct fields.
func TestCheckCorpusDim_Mismatch(t *testing.T) {
	e := &stubDimEmbedder{dim: 1024}
	c := &stubCorpusInspector{dim: 768}

	err := CheckCorpusDim(context.Background(), e, c)
	if err == nil {
		t.Fatal("expected ErrCorpusDimMismatch, got nil")
	}
	var dimErr *ErrCorpusDimMismatch
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *ErrCorpusDimMismatch, got %T: %v", err, err)
	}
	if dimErr.EmbedderDim != 1024 {
		t.Errorf("EmbedderDim: want 1024, got %d", dimErr.EmbedderDim)
	}
	if dimErr.CorpusDim != 768 {
		t.Errorf("CorpusDim: want 768, got %d", dimErr.CorpusDim)
	}
}

// TestCheckCorpusDim_EmptyCorpus verifies that an empty corpus (StoredDim=0)
// returns nil — no drift possible on an empty corpus.
func TestCheckCorpusDim_EmptyCorpus(t *testing.T) {
	e := &stubDimEmbedder{dim: 1024}
	c := &stubCorpusInspector{dim: 0}
	if err := CheckCorpusDim(context.Background(), e, c); err != nil {
		t.Errorf("expected nil for empty corpus, got %v", err)
	}
}

// TestCheckCorpusDim_AutoDetectEmbedder verifies that an embedder with
// Dimension()=0 (auto-detect mode) returns nil — no invariant to check.
func TestCheckCorpusDim_AutoDetectEmbedder(t *testing.T) {
	e := &stubDimEmbedder{dim: 0}
	c := &stubCorpusInspector{dim: 768}
	if err := CheckCorpusDim(context.Background(), e, c); err != nil {
		t.Errorf("expected nil for auto-detect embedder, got %v", err)
	}
}

// TestCheckCorpusDim_NilArgs verifies that nil embedder or inspector returns
// nil — the check is a no-op when either side is missing.
func TestCheckCorpusDim_NilArgs(t *testing.T) {
	e := &stubDimEmbedder{dim: 1024}
	c := &stubCorpusInspector{dim: 768}
	if err := CheckCorpusDim(context.Background(), nil, c); err != nil {
		t.Errorf("nil embedder: expected nil, got %v", err)
	}
	if err := CheckCorpusDim(context.Background(), e, nil); err != nil {
		t.Errorf("nil inspector: expected nil, got %v", err)
	}
}

// TestCheckCorpusDim_InspectorError verifies that a StoredDim error is
// wrapped and returned — the caller should see the storage failure.
func TestCheckCorpusDim_InspectorError(t *testing.T) {
	sentinel := errors.New("storage: connection refused")
	e := &stubDimEmbedder{dim: 1024}
	c := &stubCorpusInspector{err: sentinel}

	err := CheckCorpusDim(context.Background(), e, c)
	if err == nil {
		t.Fatal("expected error from inspector, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error wrapped, got %v", err)
	}
}

// TestCheckCorpusDim_ErrorMessage verifies the error message is actionable
// and mentions re-embedding.
func TestCheckCorpusDim_ErrorMessage(t *testing.T) {
	err := &ErrCorpusDimMismatch{EmbedderDim: 1024, CorpusDim: 768}
	msg := err.Error()
	if !strings.Contains(msg, "1024") || !strings.Contains(msg, "768") {
		t.Errorf("error message should contain both dims: %s", msg)
	}
	if !strings.Contains(msg, "re-embed") {
		t.Errorf("error message should mention re-embedding: %s", msg)
	}
}
