package embed

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"
)

// TestAblation_503WithRetryAndFallback simulates 1000 requests with 30% 503
// error rate and verifies that retry+fallback achieves ≥99% success rate.
func TestAblation_503WithRetryAndFallback(t *testing.T) {
	const totalRequests = 1000
	const failRate = 0.30

	// Primary backend: fails 30% with 503.
	var primaryCalls atomic.Int64
	primaryFn := func(_ context.Context, texts []string) ([][]float32, error) {
		primaryCalls.Add(1)
		if rand.Float64() < failRate { //nolint:gosec
			return nil, errHTTPStatus{Code: 503}
		}
		return okVecs(len(texts)), nil
	}

	// Secondary backend: always succeeds.
	var secondaryCalls atomic.Int64
	secondaryFn := func(_ context.Context, texts []string) ([][]float32, error) {
		secondaryCalls.Add(1)
		return okVecs(len(texts)), nil
	}

	// RetryPolicy: 3 attempts with 0 backoff for speed.
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{503},
	}

	primary := &Client{
		inner:    &stubEmbedder{model: "primary", embedFn: primaryFn},
		observer: noopObserver{},
		model:    "primary",
		retry:    p,
	}
	secondary := &Client{
		inner:    &stubEmbedder{model: "secondary", embedFn: secondaryFn},
		observer: noopObserver{},
		model:    "secondary",
		retry:    NoRetry,
	}
	primary.fallback = secondary

	success := 0
	for i := 0; i < totalRequests; i++ {
		res, err := primary.EmbedWithResult(context.Background(), []string{"text"})
		if err == nil && (res.Status == StatusOk || res.Status == StatusFallback) {
			success++
		}
	}

	pct := float64(success) / float64(totalRequests) * 100
	t.Logf("Ablation 503 harness: %d/%d success (%.1f%%), primary calls=%d, secondary calls=%d",
		success, totalRequests, pct, primaryCalls.Load(), secondaryCalls.Load())

	if pct < 99.0 {
		t.Errorf("ablation: want ≥99%% success rate, got %.1f%%", pct)
	}
}

// TestAblation_CircuitOpensAndFastFails verifies that:
// 1. Circuit opens after FailThreshold consecutive failures.
// 2. Subsequent requests fast-fail with ErrCircuitOpen (no backend call).
// 3. After OpenDuration, the circuit transitions to HalfOpen and allows a probe.
func TestAblation_CircuitOpensAndFastFails(t *testing.T) {
	var backendCalls atomic.Int64

	inner := &stubEmbedder{
		model: "ablation-cb",
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			backendCalls.Add(1)
			return nil, errors.New("embed: backend failure")
		},
	}

	cbCfg := CircuitConfig{
		FailThreshold:  5,
		OpenDuration:   100 * time.Millisecond,
		HalfOpenProbes: 1,
	}
	cb := NewCircuitBreaker(cbCfg, "ablation-cb", nil)

	c := &Client{
		inner:    inner,
		observer: noopObserver{},
		model:    "ablation-cb",
		retry:    NoRetry,
		circuit:  cb,
	}

	// Trip the circuit: FailThreshold consecutive failures.
	for i := 0; i < 5; i++ {
		c.EmbedWithResult(context.Background(), []string{"text"}) //nolint:errcheck
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("circuit should be Open after %d failures, got %s", 5, cb.State())
	}

	// Next 10 requests must fast-fail with ErrCircuitOpen.
	trippedAt := backendCalls.Load()
	openErrors := 0
	for i := 0; i < 10; i++ {
		_, err := c.EmbedWithResult(context.Background(), []string{"text"})
		if errors.Is(err, ErrCircuitOpen) {
			openErrors++
		}
	}
	if openErrors != 10 {
		t.Errorf("want 10 ErrCircuitOpen fast-fails, got %d", openErrors)
	}
	if backendCalls.Load() != trippedAt {
		t.Errorf("backend should not be called when circuit is Open; extra calls: %d",
			backendCalls.Load()-trippedAt)
	}

	t.Logf("Circuit fast-fail: %d open-errors, backend calls after trip: %d",
		openErrors, backendCalls.Load()-trippedAt)

	// Wait for OpenDuration to elapse.
	time.Sleep(150 * time.Millisecond)

	// After OpenDuration, Allow() should let a probe through (HalfOpen).
	if !cb.Allow() {
		t.Fatal("circuit should allow probe after OpenDuration")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("want HalfOpen after OpenDuration, got %s", cb.State())
	}
}
