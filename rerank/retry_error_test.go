package rerank

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// rerankSentinel is the underlying attempt error used across the rerank retryError tests.
var rerankSentinel = errors.New("rerank upstream boom")

// TestDo_RetryError_PreservesContextAndCause verifies that do's retryError
// satisfies both errors.Is(err, context.Canceled) and errors.Is(err, cause).
func TestDo_RetryError_PreservesContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     200 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{500},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		cancel()
		return 0, errHTTPStatus{Code: 500}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	var hs errHTTPStatus
	if !errors.As(err, &hs) {
		t.Errorf("errors.As did not reach errHTTPStatus — err: %v", err)
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
		RetryableStatus: []int{500},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return 0, errHTTPStatus{Code: 500}
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
		RetryableStatus: []int{500},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		cancel()
		return 0, errHTTPStatus{Code: 500}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	var hs errHTTPStatus
	if !errors.As(err, &hs) {
		t.Errorf("single attempt should still carry cause: errors.As did not reach errHTTPStatus — err: %v", err)
	}
}

// TestDo_RetryError_GoJobClassifierShape verifies errors.Is(err,
// context.DeadlineExceeded) on do's retryError.
func TestDo_RetryError_GoJobClassifierShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     200 * time.Millisecond,
		Multiplier:      2.0,
		RetryableStatus: []int{500},
	}
	_, err := do(ctx, p, "model", noopObserver{}, func() (int, error) {
		return 0, errHTTPStatus{Code: 500}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; want true — err: %v", err)
	}
}
