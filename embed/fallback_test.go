package embed

import (
	"context"
	"errors"
	"testing"
)

// stubEmbedder is a test helper that returns fixed results or errors.
type stubEmbedder struct {
	embedFn func(ctx context.Context, texts []string) ([][]float32, error)
	model   string
	dim     int
}

func (s *stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return s.embedFn(ctx, texts)
}
func (s *stubEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.Embed(ctx, []string{text})
	if err != nil || len(vecs) == 0 {
		return nil, err
	}
	return vecs[0], nil
}
func (s *stubEmbedder) Dimension() int { return s.dim }
func (s *stubEmbedder) Close() error   { return nil }
func (s *stubEmbedder) Model() string  { return s.model }

// makeStubClient builds a *Client backed by a stub embedder.
func makeStubClient(model string, fn func(context.Context, []string) ([][]float32, error)) *Client {
	return &Client{
		inner:    &stubEmbedder{embedFn: fn, model: model},
		observer: noopObserver{},
		model:    model,
		retry:    NoRetry, // no retry in fallback unit tests — test fallback logic only
	}
}

// okVecs returns n unit vectors for testing.
func okVecs(n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = []float32{1.0, 0.0, 0.0}
	}
	return out
}

func TestFallback_PrimarySuccessNoSecondaryCall(t *testing.T) {
	secondaryCalled := false

	primary := makeStubClient("primary", func(_ context.Context, texts []string) ([][]float32, error) {
		return okVecs(len(texts)), nil
	})
	secondary := makeStubClient("secondary", func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalled = true
		return okVecs(len(texts)), nil
	})

	res := embedWithFallback(context.Background(), primary, secondary, []string{"hello"})
	if res.Status != StatusOk {
		t.Errorf("want StatusOk, got %s", res.Status)
	}
	if secondaryCalled {
		t.Error("secondary should not be called on primary success")
	}
}

func TestFallback_PrimaryDegradedSecondaryCalled(t *testing.T) {
	primaryErr := errors.New("embed: backend timeout")
	secondaryCalled := false

	primary := makeStubClient("primary", func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, primaryErr
	})
	secondary := makeStubClient("secondary", func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalled = true
		return okVecs(len(texts)), nil
	})

	res := embedWithFallback(context.Background(), primary, secondary, []string{"hello"})
	if !secondaryCalled {
		t.Error("secondary should be called on primary degraded")
	}
	if res.Status != StatusFallback {
		t.Errorf("want StatusFallback, got %s", res.Status)
	}
}

func TestFallback_PrimaryClientErrorNoSecondary(t *testing.T) {
	// 4xx error — should not try secondary.
	secondaryCalled := false
	clientErr := errHTTPStatus{Code: 400}

	primary := makeStubClient("primary", func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, clientErr
	})
	secondary := makeStubClient("secondary", func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalled = true
		return okVecs(len(texts)), nil
	})

	res := embedWithFallback(context.Background(), primary, secondary, []string{"hello"})
	if secondaryCalled {
		t.Error("secondary should NOT be called on 4xx error")
	}
	if res.Status != StatusDegraded {
		t.Errorf("want StatusDegraded, got %s", res.Status)
	}
}

func TestFallback_BothFail(t *testing.T) {
	primaryErr := errors.New("embed: timeout")
	secondaryErr := errors.New("embed: also timeout")

	primary := makeStubClient("primary", func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, primaryErr
	})
	secondary := makeStubClient("secondary", func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, secondaryErr
	})

	res := embedWithFallback(context.Background(), primary, secondary, []string{"hello"})
	if res.Status != StatusDegraded {
		t.Errorf("want StatusDegraded when both fail, got %s", res.Status)
	}
	// Should return primary's error (per spec).
	if !errors.Is(res.Err, primaryErr) {
		t.Errorf("want primary error, got %v", res.Err)
	}
}

func TestFallback_NilSecondaryDegraded(t *testing.T) {
	primaryErr := errors.New("embed: timeout")
	primary := makeStubClient("primary", func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, primaryErr
	})

	res := embedWithFallback(context.Background(), primary, nil, []string{"hello"})
	if res.Status != StatusDegraded {
		t.Errorf("want StatusDegraded with nil secondary, got %s", res.Status)
	}
}

func TestIsClientError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errHTTPStatus{Code: 400}, true},
		{errHTTPStatus{Code: 422}, true},
		{errHTTPStatus{Code: 499}, true},
		{errHTTPStatus{Code: 500}, false},
		{errHTTPStatus{Code: 200}, false},
		{errors.New("some other error"), false},
	}
	for _, tc := range cases {
		got := isClientError(tc.err)
		if got != tc.want {
			t.Errorf("isClientError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
