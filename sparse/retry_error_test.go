package sparse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// sparseSentinel is the underlying attempt error used across the sparse retryError tests.
var sparseSentinel = errors.New("sparse upstream boom")

// TestWithRetry_RetryError_PreservesContextAndCause verifies that withRetry's
// retryError satisfies both errors.Is(err, context.Canceled) and errors.Is(err, cause).
func TestWithRetry_RetryError_PreservesContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second}
	cause := &errHTTPStatus{Code: 503, Body: "svc unavailable"}
	_, err := withRetry(ctx, cfg, "http", noopObserver{}, func() (int, int, error) {
		cancel()
		return 0, 503, cause
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false; want true — err: %v", err)
	}
	var hs *errHTTPStatus
	if !errors.As(err, &hs) {
		t.Errorf("errors.As did not reach *errHTTPStatus — err: %v", err)
	}
}

// TestWithRetry_RetryError_AttemptCountReal verifies the error message contains
// the real attempt count from withRetry's retryError.
func TestWithRetry_RetryError_AttemptCountReal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	cfg := RetryConfig{MaxAttempts: 10, BaseDelay: 50 * time.Millisecond, MaxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, "http", noopObserver{}, func() (int, int, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return 0, 503, sparseSentinel
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
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, "http", noopObserver{}, func() (int, int, error) {
		cancel()
		return 0, 503, sparseSentinel
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, sparseSentinel) {
		t.Errorf("single attempt should still carry cause: errors.Is(err, sparseSentinel) = false — err: %v", err)
	}
}

// TestWithRetry_RetryError_GoJobClassifierShape verifies errors.Is(err,
// context.DeadlineExceeded) on withRetry's retryError.
func TestWithRetry_RetryError_GoJobClassifierShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second}
	_, err := withRetry(ctx, cfg, "http", noopObserver{}, func() (int, int, error) {
		return 0, 503, sparseSentinel
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; want true — err: %v", err)
	}
}
