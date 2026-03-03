package retry_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/retry"
)

func TestDo_Success(t *testing.T) {
	calls := 0
	result, err := retry.Do(context.Background(), retry.Options{MaxAttempts: 3}, func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_RetryThenSucceed(t *testing.T) {
	calls := 0
	result, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("not yet")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_AllFail(t *testing.T) {
	calls := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
	}, func() (string, error) {
		calls++
		return "", errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := retry.Do(ctx, retry.Options{MaxAttempts: 5}, func() (string, error) {
		return "", errors.New("should not reach")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestDo_ContextCancelledBetweenRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
	}, func() (string, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return "", errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestHTTP_RetryOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	}))
	defer srv.Close()

	resp, err := retry.HTTP(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (*http.Response, error) {
		return http.Get(srv.URL) //nolint:noctx // test only
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestHTTP_Non5xxNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	resp, err := retry.HTTP(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (*http.Response, error) {
		return http.Get(srv.URL) //nolint:noctx // test only
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (404 not retried)", calls)
	}
}

func TestOptions_Defaults(t *testing.T) {
	result, err := retry.Do(context.Background(), retry.Options{}, func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}

// instantTimer implements retry.Timer for deterministic tests.
// It records requested delays and returns immediately.
type instantTimer struct {
	delays []time.Duration
}

func (t *instantTimer) After(d time.Duration) <-chan time.Time {
	t.delays = append(t.delays, d)
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	return ch
}

func TestDo_RetryAfter(t *testing.T) {
	timer := &instantTimer{}
	calls := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Second,
		Timer:        timer,
	}, func() (string, error) {
		calls++
		if calls < 3 {
			return "", retry.RetryAfter(5*time.Second, errors.New("rate limited"))
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First retry should use RetryAfter duration (5s), not InitialDelay (1s).
	if len(timer.delays) < 1 || timer.delays[0] != 5*time.Second {
		t.Errorf("first delay = %v, want 5s (from RetryAfter)", timer.delays[0])
	}
}

func TestRetryAfterError_Unwrap(t *testing.T) {
	inner := errors.New("rate limited")
	err := retry.RetryAfter(time.Second, inner)
	if !errors.Is(err, inner) {
		t.Error("RetryAfter error should unwrap to inner error")
	}
	var ra *retry.RetryAfterError
	if !errors.As(err, &ra) {
		t.Fatal("should be RetryAfterError")
	}
	if ra.Delay != time.Second {
		t.Errorf("Delay = %v, want 1s", ra.Delay)
	}
}

func TestDo_MaxElapsedTime(t *testing.T) {
	calls := 0
	start := time.Now()
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:    100,
		InitialDelay:  10 * time.Millisecond,
		MaxElapsedTime: 50 * time.Millisecond,
	}, func() (string, error) {
		calls++
		return "", errors.New("fail")
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls >= 100 {
		t.Errorf("MaxElapsedTime should have stopped retries before 100 attempts, got %d", calls)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed %v, MaxElapsedTime should have capped it", elapsed)
	}
}

func TestDo_Jitter(t *testing.T) {
	timer := &instantTimer{}
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     time.Second,
		Jitter:       true,
		Timer:        timer,
	}, func() (string, error) {
		return "", errors.New("fail")
	})
	if len(timer.delays) < 2 {
		t.Fatal("expected at least 2 delays")
	}
	// With jitter, delays should be within ±25% of computed backoff.
	// 100ms ±25% = [75ms, 125ms]
	d := timer.delays[0]
	if d < 75*time.Millisecond || d > 125*time.Millisecond {
		t.Errorf("first delay %v not in jitter range [75ms, 125ms]", d)
	}
}

func TestDo_Timer(t *testing.T) {
	timer := &instantTimer{}
	calls := 0
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Second,
		Timer:        timer,
	}, func() (string, error) {
		calls++
		return "", errors.New("fail")
	})
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	// Timer should have been called twice (between attempt 1-2 and 2-3).
	if len(timer.delays) != 2 {
		t.Errorf("timer called %d times, want 2", len(timer.delays))
	}
}

func TestHTTP_RetryAfterHeader(t *testing.T) {
	timer := &instantTimer{}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := retry.HTTP(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		Timer:        timer,
	}, func() (*http.Response, error) {
		return http.Get(srv.URL) //nolint:noctx // test only
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	// Timer should have received 3s delay (from Retry-After header), not 100ms.
	if len(timer.delays) < 1 || timer.delays[0] != 3*time.Second {
		t.Errorf("delay = %v, want 3s (from Retry-After header)", timer.delays[0])
	}
}

func TestDo_AbortOn(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		AbortOn:     []error{context.DeadlineExceeded},
	}, func() (int, error) {
		attempts++
		return 0, context.DeadlineExceeded
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (should abort on first error)", attempts)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestDo_AbortOn_NonMatching(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		AbortOn:      []error{context.DeadlineExceeded},
		Timer:        &instantTimer{},
	}, func() (int, error) {
		attempts++
		return 0, errors.New("transient")
	})

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (non-matching error should retry)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetryableOnly(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:   3,
		InitialDelay:  time.Millisecond,
		RetryableOnly: true,
		Timer:         &instantTimer{},
	}, func() (int, error) {
		attempts++
		return 0, retry.MarkRetryable(errors.New("temporary"))
	})

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (retryable error should retry)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetryableOnly_Abort(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:   5,
		RetryableOnly: true,
	}, func() (int, error) {
		attempts++
		return 0, errors.New("permanent")
	})

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (unmarked error should abort)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkRetryable_Unwrap(t *testing.T) {
	inner := errors.New("db connection failed")
	wrapped := retry.MarkRetryable(inner)

	if !errors.Is(wrapped, inner) {
		t.Error("MarkRetryable should preserve error chain")
	}
	if !retry.IsRetryable(wrapped) {
		t.Error("IsRetryable should return true for MarkRetryable error")
	}
	if retry.IsRetryable(inner) {
		t.Error("IsRetryable should return false for plain error")
	}
}

func TestIsRetryable(t *testing.T) {
	if retry.IsRetryable(nil) {
		t.Error("IsRetryable(nil) should be false")
	}
	if retry.IsRetryable(errors.New("plain")) {
		t.Error("IsRetryable(plain error) should be false")
	}
	if !retry.IsRetryable(retry.MarkRetryable(errors.New("temp"))) {
		t.Error("IsRetryable(MarkRetryable) should be true")
	}
}

func TestFibonacciBackoff_Sequence(t *testing.T) {
	timer := &instantTimer{}
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  7,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Backoff:      retry.BackoffFibonacci,
		Timer:        timer,
	}, func() (string, error) {
		return "", errors.New("fail")
	})

	// 7 attempts = 6 delays between them.
	// Fibonacci sequence: 1,1,2,3,5,8 × 100ms = 100,100,200,300,500,800ms
	expected := []time.Duration{
		100 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		500 * time.Millisecond,
		800 * time.Millisecond,
	}
	if len(timer.delays) != len(expected) {
		t.Fatalf("got %d delays, want %d: %v", len(timer.delays), len(expected), timer.delays)
	}
	for i, want := range expected {
		if timer.delays[i] != want {
			t.Errorf("delay[%d] = %v, want %v", i, timer.delays[i], want)
		}
	}
}

func TestFibonacciBackoff_CappedAtMax(t *testing.T) {
	timer := &instantTimer{}
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  10,
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Second,
		Backoff:      retry.BackoffFibonacci,
		Timer:        timer,
	}, func() (string, error) {
		return "", errors.New("fail")
	})

	for i, d := range timer.delays {
		if d > 5*time.Second {
			t.Errorf("delay[%d] = %v, exceeds MaxDelay 5s", i, d)
		}
	}
}

// --- OnRetry tests ---

func TestDo_OnRetry(t *testing.T) {
	timer := &instantTimer{}
	var calls []int
	var errs []string
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		Timer:        timer,
		OnRetry: func(attempt int, err error) {
			calls = append(calls, attempt)
			errs = append(errs, err.Error())
		},
	}, func() (string, error) {
		return "", errors.New("fail")
	})
	// OnRetry called for attempts 0, 1, 2
	if len(calls) != 3 {
		t.Errorf("OnRetry called %d times, want 3", len(calls))
	}
}

func TestDo_OnRetry_NotCalledOnSuccess(t *testing.T) {
	called := false
	_, _ = retry.Do(context.Background(), retry.Options{
		OnRetry: func(int, error) { called = true },
	}, func() (string, error) {
		return "ok", nil
	})
	if called {
		t.Error("OnRetry should not be called on success")
	}
}

func TestDo_OnRetry_Nil(t *testing.T) {
	// nil OnRetry should not panic
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		Timer:        &instantTimer{},
	}, func() (string, error) {
		return "", errors.New("fail")
	})
}

// --- Permanent error tests ---

func TestDo_Permanent(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		Timer:       &instantTimer{},
	}, func() (string, error) {
		attempts++
		return "", retry.Permanent(errors.New("fatal"))
	})
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if err == nil || err.Error() != "fatal" {
		t.Errorf("err = %v, want 'fatal'", err)
	}
}

func TestPermanent_Nil(t *testing.T) {
	if retry.Permanent(nil) != nil {
		t.Error("Permanent(nil) should return nil")
	}
}

func TestIsPermanent(t *testing.T) {
	if retry.IsPermanent(errors.New("normal")) {
		t.Error("normal error should not be permanent")
	}
	if !retry.IsPermanent(retry.Permanent(errors.New("fatal"))) {
		t.Error("Permanent error should be permanent")
	}
}

func TestPermanent_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	err := retry.Permanent(inner)
	// After Do unwraps, the returned error should be the inner error
	_, retErr := retry.Do(context.Background(), retry.Options{MaxAttempts: 3}, func() (string, error) {
		return "", err
	})
	if !errors.Is(retErr, inner) {
		t.Errorf("unwrapped error should be inner: got %v", retErr)
	}
}

func TestPermanent_NoOnRetry(t *testing.T) {
	called := false
	_, _ = retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		OnRetry:     func(int, error) { called = true },
	}, func() (string, error) {
		return "", retry.Permanent(errors.New("fatal"))
	})
	if called {
		t.Error("OnRetry should not be called for permanent errors")
	}
}

// --- Context wrapping tests ---

func TestDo_ContextWrapping_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
	}, func() (string, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return "", errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err should wrap context.Canceled, got: %v", err)
	}
	if !strings.Contains(err.Error(), "after") {
		t.Errorf("err should contain attempt count, got: %v", err)
	}
}

func TestDo_ContextWrapping_NoContextErr(t *testing.T) {
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		Timer:        &instantTimer{},
	}, func() (string, error) {
		return "", errors.New("fail")
	})
	// No context error — should return raw lastErr
	if err.Error() != "fail" {
		t.Errorf("err = %v, want 'fail'", err)
	}
}

// --- RetryIf tests ---

func TestDo_RetryIf_Retries(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		Timer:        &instantTimer{},
		RetryIf:      func(err error) bool { return err.Error() == "transient" },
	}, func() (string, error) {
		attempts++
		return "", errors.New("transient")
	})
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (RetryIf returned true)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetryIf_Aborts(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		RetryIf:     func(err error) bool { return err.Error() != "fatal" },
	}, func() (string, error) {
		attempts++
		return "", errors.New("fatal")
	})
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (RetryIf returned false)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetryIf_OverridesAbortOn(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		Timer:        &instantTimer{},
		AbortOn:      []error{context.DeadlineExceeded},
		RetryIf:      func(error) bool { return true }, // always retry
	}, func() (string, error) {
		attempts++
		return "", context.DeadlineExceeded
	})
	// RetryIf overrides AbortOn — should retry all 3 attempts.
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (RetryIf overrides AbortOn)", attempts)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetryIf_PermanentTakesPrecedence(t *testing.T) {
	attempts := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		RetryIf:     func(error) bool { return true }, // always retry
	}, func() (string, error) {
		attempts++
		return "", retry.Permanent(errors.New("fatal"))
	})
	// Permanent is checked before shouldAbort, so it takes precedence.
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (Permanent takes precedence over RetryIf)", attempts)
	}
	if err == nil || err.Error() != "fatal" {
		t.Errorf("err = %v, want 'fatal'", err)
	}
}
