package embed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// embedSentinel is the underlying attempt error used across the embed retryError tests.
var embedSentinel = errors.New("embed upstream boom")

// --- do (v2) tests ---

// TestDo_RetryError_PreservesContextAndCause verifies that do's retryError
// satisfies both errors.Is(err, context.Canceled) and errors.Is(err, cause).
func TestDo_RetryError_PreservesContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     200 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		cancel()
		return 0, &errHTTPStatus{Code: 503}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	var hs *errHTTPStatus
	if !errors.As(err, &hs) {
		t.Errorf("errors.As did not reach *errHTTPStatus — err: %v", err)
	}
}

// TestDo_RetryError_AttemptCountReal verifies the error message contains the
// real attempt count from do's retryError.
func TestDo_RetryError_AttemptCountReal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	p := RetryPolicy{
		MaxAttempts:     10,
		BaseBackoff:     50 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return 0, &errHTTPStatus{Code: 503}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Errorf("err message %q does not contain 'after 2 attempts'", err.Error())
	}
}

// TestDo_RetryError_SingleAttemptCarriesCause verifies that do's retryError
// carries the cause even when only one attempt ran.
func TestDo_RetryError_SingleAttemptCarriesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     200 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		cancel()
		return 0, &errHTTPStatus{Code: 503}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	var hs *errHTTPStatus
	if !errors.As(err, &hs) {
		t.Errorf("single attempt should still carry cause: errors.As did not reach *errHTTPStatus — err: %v", err)
	}
}

// TestDo_RetryError_GoJobClassifierShape verifies errors.Is(err, context.DeadlineExceeded)
// on do's retryError — the exact call classifyEmbedError makes.
func TestDo_RetryError_GoJobClassifierShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     200 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{503},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		return 0, &errHTTPStatus{Code: 503}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; want true — err: %v", err)
	}
}

// --- withRetry (v1) tests ---

// TestWithRetry_RetryError_PreservesContextAndCause verifies that withRetry's
// retryError satisfies both errors.Is(err, context.Canceled) and errors.Is(err, cause).
func TestWithRetry_RetryError_PreservesContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := retryConfig{maxAttempts: 5, baseDelay: 200 * time.Millisecond, maxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, func() (string, int, error) {
		cancel()
		return "", 503, embedSentinel
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, embedSentinel) {
		t.Errorf("errors.Is(err, embedSentinel) = false; want true — err: %v", err)
	}
}

// TestWithRetry_RetryError_AttemptCountReal verifies the error message contains
// the real attempt count from withRetry's retryError.
func TestWithRetry_RetryError_AttemptCountReal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	cfg := retryConfig{maxAttempts: 10, baseDelay: 50 * time.Millisecond, maxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, func() (string, int, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return "", 503, embedSentinel
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Errorf("err message %q does not contain 'after 2 attempts'", err.Error())
	}
}

// TestWithRetry_RetryError_SingleAttemptCarriesCause verifies that withRetry's
// retryError carries the cause even when only one attempt ran.
func TestWithRetry_RetryError_SingleAttemptCarriesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := retryConfig{maxAttempts: 5, baseDelay: 200 * time.Millisecond, maxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, func() (string, int, error) {
		cancel()
		return "", 503, embedSentinel
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, embedSentinel) {
		t.Errorf("single attempt should still carry cause: errors.Is(err, embedSentinel) = false — err: %v", err)
	}
}
