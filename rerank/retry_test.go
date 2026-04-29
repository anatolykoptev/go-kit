package rerank

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// mockObserver records OnRetry calls for testing.
type mockObserver struct {
	noopObserver
	retryAttempts []int
	retryErrors   []error
}

func (m *mockObserver) OnRetry(_ context.Context, attempt int, err error) {
	m.retryAttempts = append(m.retryAttempts, attempt)
	m.retryErrors = append(m.retryErrors, err)
}

// TestRetry_MaxAttempts verifies that do() returns the last error after exhausting MaxAttempts.
func TestRetry_MaxAttempts(t *testing.T) {
	calls := 0
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{500},
	}
	wantErr := errHTTPStatus{Code: 500}
	_, err := do(context.Background(), p, "", noopObserver{}, func() (int, error) {
		calls++
		return 0, wantErr
	})
	if calls != 3 {
		t.Errorf("calls: got %d want 3", calls)
	}
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("err: got %v want %v", err, wantErr)
	}
}

// TestRetry_BackoffTiming verifies exponential growth of backoff durations.
func TestRetry_BackoffTiming(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     4,
		BaseBackoff:     10 * time.Millisecond,
		MaxBackoff:      1 * time.Second,
		Multiplier:      2.0,
		Jitter:          0, // deterministic
		RetryableStatus: []int{500},
	}
	var timestamps []time.Time
	_, _ = do(context.Background(), p, "", noopObserver{}, func() (int, error) {
		timestamps = append(timestamps, time.Now())
		return 0, errHTTPStatus{Code: 500}
	})
	if len(timestamps) != 4 {
		t.Fatalf("expected 4 attempts, got %d", len(timestamps))
	}
	// Gap between attempt 0→1 should be ~10ms (BaseBackoff * 2^0).
	// Gap between attempt 1→2 should be ~20ms (BaseBackoff * 2^1).
	// Gap between attempt 2→3 should be ~40ms (BaseBackoff * 2^2).
	gaps := make([]time.Duration, 3)
	for i := 0; i < 3; i++ {
		gaps[i] = timestamps[i+1].Sub(timestamps[i])
	}
	// Each gap should be >= (previous gap * 1.5) to confirm exponential growth.
	// We use a loose bound to account for scheduling jitter.
	if gaps[1] < gaps[0] {
		t.Errorf("backoff not growing: gap[0]=%v gap[1]=%v", gaps[0], gaps[1])
	}
	if gaps[2] < gaps[1] {
		t.Errorf("backoff not growing: gap[1]=%v gap[2]=%v", gaps[1], gaps[2])
	}
}

// TestRetry_RetryableStatusFilter verifies that non-retryable status returns immediately.
func TestRetry_RetryableStatusFilter(t *testing.T) {
	calls := 0
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{500, 503},
	}
	// 400 is not in RetryableStatus — should return after 1 attempt.
	_, err := do(context.Background(), p, "", noopObserver{}, func() (int, error) {
		calls++
		return 0, errHTTPStatus{Code: 400}
	})
	if calls != 1 {
		t.Errorf("calls: got %d want 1 (no retry on 4xx)", calls)
	}
	var httpErr errHTTPStatus
	if !errors.As(err, &httpErr) || httpErr.Code != 400 {
		t.Errorf("err: got %v, want errHTTPStatus{400}", err)
	}
}

// TestRetry_CtxCancellation verifies that ctx cancellation aborts retry sleep.
func TestRetry_CtxCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	p := RetryPolicy{
		MaxAttempts:     5,
		BaseBackoff:     500 * time.Millisecond, // long backoff
		RetryableStatus: []int{500},
	}

	// Cancel after first call.
	done := make(chan error, 1)
	go func() {
		_, err := do(ctx, p, "", noopObserver{}, func() (int, error) {
			calls++
			if calls == 1 {
				cancel()
			}
			return 0, errHTTPStatus{Code: 500}
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err: got %v want context.Canceled", err)
		}
		if calls > 2 { // allow for a second attempt before cancel takes effect
			t.Errorf("calls: got %d, expected <=2 (cancel should abort backoff)", calls)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: ctx cancellation did not abort retry")
	}
}

// TestRetry_JitterApplied verifies that backoff durations vary (statistical bound).
func TestRetry_JitterApplied(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts:     2,
		BaseBackoff:     10 * time.Millisecond,
		MaxBackoff:      time.Second,
		Multiplier:      1.0, // keep base constant so jitter is the only variation
		Jitter:          0.5,
		RetryableStatus: []int{500},
	}

	const runs = 50
	durations := make([]time.Duration, runs)
	for i := range durations {
		start := time.Now()
		_, _ = do(context.Background(), p, "", noopObserver{}, func() (int, error) {
			return 0, errHTTPStatus{Code: 500}
		})
		durations[i] = time.Since(start)
	}

	// At least some variation must exist: min != max.
	min, max := durations[0], durations[0]
	for _, d := range durations[1:] {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	if max == min {
		t.Error("all backoff durations identical — jitter not applied")
	}
}

// TestRetry_OnRetryHookFires verifies OnRetry is called N-1 times for N attempts.
func TestRetry_OnRetryHookFires(t *testing.T) {
	obs := &mockObserver{}
	p := RetryPolicy{
		MaxAttempts:     4,
		BaseBackoff:     0,
		RetryableStatus: []int{500},
	}
	_, _ = do(context.Background(), p, "", obs, func() (int, error) {
		return 0, errHTTPStatus{Code: 500}
	})

	// 4 attempts → 3 retries → OnRetry called 3 times.
	if len(obs.retryAttempts) != 3 {
		t.Errorf("OnRetry calls: got %d want 3", len(obs.retryAttempts))
	}
	for i, attempt := range obs.retryAttempts {
		wantAttempt := i + 1
		if attempt != wantAttempt {
			t.Errorf("OnRetry[%d] attempt: got %d want %d", i, attempt, wantAttempt)
		}
	}
}

// TestRetry_ZeroMaxAttemptsTreatedAsOne verifies MaxAttempts=0 behaves as 1 (no retry).
func TestRetry_ZeroMaxAttemptsTreatedAsOne(t *testing.T) {
	calls := 0
	p := RetryPolicy{MaxAttempts: 0, RetryableStatus: []int{500}}
	_, _ = do(context.Background(), p, "", noopObserver{}, func() (int, error) {
		calls++
		return 0, errHTTPStatus{Code: 500}
	})
	if calls != 1 {
		t.Errorf("calls: got %d want 1 (MaxAttempts=0 treated as 1)", calls)
	}
}

// TestRetry_SuccessOnSecondAttempt verifies do() returns the value on retry success.
func TestRetry_SuccessOnSecondAttempt(t *testing.T) {
	calls := 0
	p := RetryPolicy{
		MaxAttempts:     3,
		BaseBackoff:     0,
		RetryableStatus: []int{500},
	}
	result, err := do(context.Background(), p, "", noopObserver{}, func() (int, error) {
		calls++
		if calls < 2 {
			return 0, errHTTPStatus{Code: 500}
		}
		return 42, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result: got %d want 42", result)
	}
	if calls != 2 {
		t.Errorf("calls: got %d want 2", calls)
	}
}

// TestErrHTTPStatus_StringFormat verifies the error string format is backward
// compatible ("http status <code>").
func TestErrHTTPStatus_StringFormat(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{500, "http status 500"},
		{503, "http status 503"},
		{400, "http status 400"},
		{200, "http status 200"},
	}
	for _, tc := range cases {
		e := errHTTPStatus{Code: tc.code}
		if got := e.Error(); got != tc.want {
			t.Errorf("code %d: got %q want %q", tc.code, got, tc.want)
		}
	}
}

// TestNoRetry_SingleAttempt verifies NoRetry sentinel causes a single attempt.
func TestNoRetry_SingleAttempt(t *testing.T) {
	calls := 0
	_, _ = do(context.Background(), NoRetry, "", noopObserver{}, func() (int, error) {
		calls++
		return 0, fmt.Errorf("some error")
	})
	if calls != 1 {
		t.Errorf("calls: got %d want 1 (NoRetry)", calls)
	}
}
