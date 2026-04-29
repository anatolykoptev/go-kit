package embed

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetryPolicy_PublicAPIExists verifies that RetryPolicy and NoRetry are exported.
func TestRetryPolicy_PublicAPIExists(t *testing.T) {
	_ = RetryPolicy{}
	_ = NoRetry
	p := defaultRetryPolicy()
	if p.MaxAttempts == 0 {
		t.Error("defaultRetryPolicy MaxAttempts must be > 0")
	}
	if p.BaseBackoff == 0 {
		t.Error("defaultRetryPolicy BaseBackoff must be > 0")
	}
}

// TestRetryPolicy_NoRetryDisablesRetry verifies that NoRetry sentinel causes
// exactly 1 attempt and returns the error on the first failure.
func TestRetryPolicy_NoRetryDisablesRetry(t *testing.T) {
	attempts := 0
	_, err := do(context.Background(), NoRetry, "model", noopObserver{}, func() (int, error) {
		attempts++
		return 0, errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("NoRetry: want 1 attempt, got %d", attempts)
	}
}

// TestRetryPolicy_MaxAttempts verifies that do() makes exactly MaxAttempts tries.
func TestRetryPolicy_MaxAttempts(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		MaxBackoff:      0,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	attempts := 0
	_, err := do(context.Background(), p, "model", noopObserver{}, func() (int, error) {
		attempts++
		return 0, &errHTTPStatus{Code: 503}
	})
	if err == nil {
		t.Fatal("expected error after all attempts")
	}
	if attempts != 3 {
		t.Errorf("want 3 attempts, got %d", attempts)
	}
}

// TestRetryPolicy_SucceedsOnSecondAttempt verifies early exit on success.
func TestRetryPolicy_SucceedsOnSecondAttempt(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{503},
	}
	attempts := 0
	result, err := do(context.Background(), p, "model", noopObserver{}, func() (string, error) {
		attempts++
		if attempts < 2 {
			return "", &errHTTPStatus{Code: 503}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("want ok, got %q", result)
	}
	if attempts != 2 {
		t.Errorf("want 2 attempts, got %d", attempts)
	}
}

// TestRetryPolicy_4xxNoRetry verifies that 4xx errors return immediately.
func TestRetryPolicy_4xxNoRetry(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     0,
		RetryableStatus: []int{503},
	}
	attempts := 0
	_, err := do(context.Background(), p, "model", noopObserver{}, func() (int, error) {
		attempts++
		return 0, &errHTTPStatus{Code: 400}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("4xx should not retry: want 1 attempt, got %d", attempts)
	}
}

// TestRetryPolicy_OnRetryHookFires verifies that obs.OnRetry fires for each retry.
func TestRetryPolicy_OnRetryHookFires(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{503},
	}
	var hookAttempts []int
	obs := &retryCapturingObserver{
		onRetry: func(attempt int) { hookAttempts = append(hookAttempts, attempt) },
	}
	attempts := 0
	do(context.Background(), p, "model", obs, func() (int, error) { //nolint:errcheck
		attempts++
		return 0, &errHTTPStatus{Code: 503}
	})
	// Expect 2 OnRetry calls (retries 1 and 2; initial attempt not a retry).
	if len(hookAttempts) != 2 {
		t.Errorf("want 2 OnRetry calls, got %d: %v", len(hookAttempts), hookAttempts)
	}
	if hookAttempts[0] != 1 || hookAttempts[1] != 2 {
		t.Errorf("want attempts [1,2], got %v", hookAttempts)
	}
}

// TestRetryPolicy_ContextCancel verifies that ctx cancellation aborts the retry sleep.
func TestRetryPolicy_ContextCancel(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     500 * time.Millisecond,
		MaxBackoff:      2 * time.Second,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		return 0, &errHTTPStatus{Code: 503}
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("context cancel should abort retry sleep early; elapsed = %v", elapsed)
	}
}

// retryCapturingObserver captures OnRetry calls for inspection.
type retryCapturingObserver struct {
	noopObserver
	onRetry func(attempt int)
}

func (o *retryCapturingObserver) OnRetry(_ context.Context, attempt int, _ error) {
	if o.onRetry != nil {
		o.onRetry(attempt)
	}
}
