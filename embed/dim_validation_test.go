package embed

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// vecsOfDim returns n vectors each of length dim (filled with 0.5).
func vecsOfDim(n, dim int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		v := make([]float32, dim)
		for j := range v {
			v[j] = 0.5
		}
		out[i] = v
	}
	return out
}

// makeDimStubClient builds a *Client backed by a stub returning fixed-dim vectors.
// expectedDim mirrors WithDim; backendDim is what the stub actually returns.
func makeDimStubClient(model string, expectedDim, backendDim int) *Client {
	return &Client{
		inner: &stubEmbedder{
			model: model,
			embedFn: func(_ context.Context, texts []string) ([][]float32, error) {
				return vecsOfDim(len(texts), backendDim), nil
			},
		},
		observer:    noopObserver{},
		model:       model,
		expectedDim: expectedDim,
		retry:       NoRetry,
	}
}

// TestClient_DimMismatch_Embed verifies that Embed surfaces *ErrDimMismatch
// when the backend returns vectors that don't match WithDim(N).
func TestClient_DimMismatch_Embed(t *testing.T) {
	cases := []struct {
		name     string
		expected int
		backend  int
		wantErr  bool
	}{
		{"match_1024", 1024, 1024, false},
		{"backend_smaller", 1024, 768, true},
		{"backend_larger", 768, 1024, true},
		{"unset_skips_validation", 0, 768, false}, // cfg.dim == 0 disables check
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := makeDimStubClient("m-"+tc.name, tc.expected, tc.backend)
			_, err := c.Embed(context.Background(), []string{"hello", "world"})
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertDimMismatch(t, err, tc.backend, tc.expected, "m-"+tc.name)
		})
	}
}

// assertDimMismatch fails the test unless err is *ErrDimMismatch with matching fields.
func assertDimMismatch(t *testing.T, err error, wantGot, wantWant int, wantModel string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected ErrDimMismatch, got nil")
	}
	var dimErr *ErrDimMismatch
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *ErrDimMismatch, got %T: %v", err, err)
	}
	if dimErr.Got != wantGot {
		t.Errorf("Got: want %d, got %d", wantGot, dimErr.Got)
	}
	if dimErr.Want != wantWant {
		t.Errorf("Want: want %d, got %d", wantWant, dimErr.Want)
	}
	if dimErr.Model != wantModel {
		t.Errorf("Model: want %q, got %q", wantModel, dimErr.Model)
	}
	if !strings.Contains(dimErr.Error(), "dimension mismatch") {
		t.Errorf("Error message missing 'dimension mismatch': %q", dimErr.Error())
	}
}

// TestClient_DimMismatch_EmbedQuery verifies that EmbedQuery also validates
// the returned vector length against WithDim.
func TestClient_DimMismatch_EmbedQuery(t *testing.T) {
	c := makeDimStubClient("m-query-mismatch", 1024, 768)
	_, err := c.EmbedQuery(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected ErrDimMismatch, got nil")
	}
	var dimErr *ErrDimMismatch
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *ErrDimMismatch, got %T: %v", err, err)
	}
	if dimErr.Got != 768 || dimErr.Want != 1024 {
		t.Errorf("dim fields: got=%d want=%d (expected got=768 want=1024)", dimErr.Got, dimErr.Want)
	}
}

// TestClient_DimMismatch_EmbedQueryUnset verifies EmbedQuery skips validation
// when WithDim was not set (cfg.dim == 0).
func TestClient_DimMismatch_EmbedQueryUnset(t *testing.T) {
	c := makeDimStubClient("m-query-unset", 0, 1234) // odd dim, no validation
	got, err := c.EmbedQuery(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error with cfg.dim=0: %v", err)
	}
	if len(got) != 1234 {
		t.Errorf("expected 1234-dim vector, got %d", len(got))
	}
}

// TestClient_DimMismatch_EmbedWithResult verifies the v2 API surfaces the
// mismatch as Status=Degraded with Err=*ErrDimMismatch.
func TestClient_DimMismatch_EmbedWithResult(t *testing.T) {
	c := makeDimStubClient("m-ewr", 1024, 768)
	res, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected ErrDimMismatch via EmbedWithResult, got nil")
	}
	if res == nil {
		t.Fatal("expected non-nil Result")
	}
	if res.Status != StatusDegraded {
		t.Errorf("status: want StatusDegraded, got %s", res.Status)
	}
	var dimErr *ErrDimMismatch
	if !errors.As(res.Err, &dimErr) {
		t.Fatalf("Result.Err should wrap *ErrDimMismatch, got %T: %v", res.Err, res.Err)
	}
}

// TestClient_DimMismatch_Metric verifies embed_dim_mismatch_total is bumped
// once per offending vector (per-text granularity, not per-request).
func TestClient_DimMismatch_Metric(t *testing.T) {
	const model = "m-metric"
	before := counterValue(embedDimMismatchTotal.WithLabelValues(model))

	c := makeDimStubClient(model, 1024, 768)
	// 3 texts → 3 wrong-dim vectors → counter must be +3.
	_, err := c.Embed(context.Background(), []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected ErrDimMismatch")
	}
	after := counterValue(embedDimMismatchTotal.WithLabelValues(model))
	if delta := after - before; delta != 3 {
		t.Errorf("embed_dim_mismatch_total{model=%q} delta: want 3, got %v", model, delta)
	}
}

// TestClient_DimMismatch_NoMetricOnUnset verifies the counter is NOT bumped
// when WithDim was not set (cfg.dim == 0 → validation skipped).
func TestClient_DimMismatch_NoMetricOnUnset(t *testing.T) {
	const model = "m-metric-unset"
	before := counterValue(embedDimMismatchTotal.WithLabelValues(model))

	c := makeDimStubClient(model, 0, 768) // unset → no validation
	if _, err := c.Embed(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := counterValue(embedDimMismatchTotal.WithLabelValues(model))
	if delta := after - before; delta != 0 {
		t.Errorf("embed_dim_mismatch_total{model=%q} delta: want 0, got %v", model, delta)
	}
}

// TestClient_DimMismatch_NoCacheWriteOnMismatch verifies that a dim-mismatch
// response is NOT written to cache (would corrupt L1/L2 with bad-dim entries).
func TestClient_DimMismatch_NoCacheWriteOnMismatch(t *testing.T) {
	const model = "m-cache-dim"
	cache := newMapCache()

	c := &Client{
		inner: &stubEmbedder{
			model: model,
			embedFn: func(_ context.Context, texts []string) ([][]float32, error) {
				return vecsOfDim(len(texts), 768), nil // bad dim
			},
		},
		observer:    noopObserver{},
		model:       model,
		expectedDim: 1024,
		retry:       NoRetry,
		cache:       cache,
	}

	_, err := c.EmbedWithResult(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected ErrDimMismatch")
	}
	cache.mu.Lock()
	n := len(cache.m)
	cache.mu.Unlock()
	if n != 0 {
		t.Errorf("cache should be empty after mismatch, has %d entries", n)
	}
}
