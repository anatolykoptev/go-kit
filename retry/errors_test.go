package retry_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/retry"
)

// sentinelErr is the underlying attempt error used across the retryError tests.
var sentinelErr = errors.New("upstream boom")

// TestRetryError_PreservesContextAndCause verifies that on a single returned
// value both errors.Is(err, context.Canceled) and errors.Is(err, sentinelErr)
// are true simultaneously.
func TestRetryError_PreservesContextAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
	}, func() (string, error) {
		cancel()
		return "", sentinelErr
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("errors.Is(err, sentinelErr) = false; want true — err: %v", err)
	}
}

// TestRetryError_ErrorsAsReachesCause verifies errors.As reaches the concrete
// type of the underlying attempt error through the As method.
func TestRetryError_ErrorsAsReachesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
	}, func() (string, error) {
		cancel()
		return "", &retry.HTTPError{StatusCode: 503}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *retry.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("errors.As did not reach *HTTPError — err: %v", err)
	}
	if httpErr.StatusCode != 503 {
		t.Errorf("StatusCode = %d, want 503", httpErr.StatusCode)
	}
}

// TestRetryError_AttemptCountReal verifies the error message contains the
// real attempt count, not a hardcoded value.
func TestRetryError_AttemptCountReal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  10,
		InitialDelay: 50 * time.Millisecond,
	}, func() (string, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return "", sentinelErr
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// retry.Do waits before each call, so 2 attempts ran before cancel took effect
	// during the 3rd sleep. The leaf ladders wait after the call and count differently.
	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Errorf("err message %q does not contain real attempt count 'after 2 attempts'", err.Error())
	}
}

// TestRetryError_SingleAttemptCarriesCause verifies that even with a single
// attempt the context error still carries the cause.
func TestRetryError_SingleAttemptCarriesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  1,
		InitialDelay: 0,
	}, func() (string, error) {
		cancel()
		return "", sentinelErr
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; want true — err: %v", err)
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("single attempt should still carry cause: errors.Is(err, sentinelErr) = false — err: %v", err)
	}
}

// TestRetryError_GoJobClassifierShape verifies the exact call go-job's
// classifyEmbedError makes: errors.Is(err, context.DeadlineExceeded) on the
// retryError produced when the deadline fires during retry.
func TestRetryError_GoJobClassifierShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 200 * time.Millisecond,
	}, func() (string, error) {
		return "", sentinelErr
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; want true — err: %v", err)
	}
}
