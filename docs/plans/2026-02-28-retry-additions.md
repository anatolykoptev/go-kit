# retry: Production-Grade Additions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add RetryAfter, MaxElapsedTime, Jitter, and Timer interface to make the retry package production-grade — matching cenkalti/backoff and avast/retry-go features.

**Architecture:** All 4 additions modify the same `Do` function loop and `Options` struct. Implemented together as one cohesive change. Backward-compatible — new Options fields default to zero values (disabled).

**Tech Stack:** Go stdlib only (`math/rand/v2` for jitter, `strconv` for Retry-After parsing)

---

### Task 1: All 4 retry additions

**Files:** retry/retry.go

**Changes to Options struct:**

```go
type Options struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	MaxElapsedTime time.Duration // total wall-clock budget; 0 = no limit
	Jitter         bool          // add ±25% random jitter to delay
	Timer          Timer         // custom timer for tests; nil = real time.After
}
```

**New Timer interface:**

```go
// Timer abstracts time.After for testability.
// Implement this to control delays in tests.
type Timer interface {
	After(d time.Duration) <-chan time.Time
}
```

**New RetryAfterError type:**

```go
// RetryAfterError wraps an error with a retry-after duration hint.
// Return this from fn to override the computed backoff for the next attempt.
type RetryAfterError struct {
	Delay time.Duration
	Err   error
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// RetryAfter wraps an error with a retry-after duration.
// When Do receives this error, it uses d instead of the exponential backoff.
func RetryAfter(d time.Duration, err error) error {
	return &RetryAfterError{Delay: d, Err: err}
}
```

**Updated Do function:**

```go
func Do[T any](ctx context.Context, opts Options, fn func() (T, error)) (T, error) {
	opts.applyDefaults()

	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}

	start := time.Now()
	delay := opts.InitialDelay
	var lastErr error

	for attempt := range opts.MaxAttempts {
		// Check elapsed time budget before each retry.
		if opts.MaxElapsedTime > 0 && attempt > 0 && time.Since(start) >= opts.MaxElapsedTime {
			break
		}

		if attempt > 0 {
			actualDelay := delay

			// Override delay if fn returned RetryAfter hint.
			var ra *RetryAfterError
			if errors.As(lastErr, &ra) && ra.Delay > 0 {
				actualDelay = ra.Delay
			}

			// Apply jitter (±25%).
			if opts.Jitter && actualDelay > 0 {
				quarter := int64(actualDelay) / 4
				actualDelay = time.Duration(int64(actualDelay) - quarter + rand.Int64N(2*quarter+1))
			}

			// Wait using Timer or real time.
			afterCh := time.After(actualDelay)
			if opts.Timer != nil {
				afterCh = opts.Timer.After(actualDelay)
			}
			select {
			case <-ctx.Done():
				var zero T
				return zero, ctx.Err()
			case <-afterCh:
			}

			delay = min(delay*2, opts.MaxDelay)
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	var zero T
	return zero, lastErr
}
```

**Updated imports:** add `"errors"`, `"math/rand/v2"`, `"strconv"`.

**Updated HTTP function** — parse Retry-After header:

```go
func HTTP(ctx context.Context, opts Options, fn func() (*http.Response, error)) (*http.Response, error) {
	return Do(ctx, opts, func() (*http.Response, error) {
		resp, err := fn()
		if err != nil {
			return nil, err
		}
		if isRetryableStatus(resp.StatusCode) {
			retryDelay := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			httpErr := &HTTPError{StatusCode: resp.StatusCode}
			if retryDelay > 0 {
				return nil, RetryAfter(retryDelay, httpErr)
			}
			return nil, httpErr
		}
		return resp, nil
	})
}

// parseRetryAfter parses the Retry-After header value as seconds.
func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
```

**Step 1:** Write the complete updated retry.go.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./retry/ -v -count=1
```
Expected: All 8 existing tests PASS (backward-compatible changes).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add retry/retry.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "retry: add RetryAfter, MaxElapsedTime, Jitter, Timer

Four production-grade additions:
- RetryAfter: fn can return duration hint to override backoff
- MaxElapsedTime: total wall-clock budget across all attempts
- Jitter: ±25% random jitter on delays to prevent thundering herd
- Timer interface: custom time.After for deterministic tests
- HTTP: auto-parse Retry-After header on 429/5xx responses

All backward-compatible (new Options fields default to disabled).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all 4 additions

**Files:** retry/retry_test.go

Add tests for each new feature. Use an instant timer helper to avoid real delays.

**Instant timer helper (unexported, test-only):**

```go
type instantTimer struct {
	delays []time.Duration
}

func (t *instantTimer) After(d time.Duration) <-chan time.Time {
	t.delays = append(t.delays, d)
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	return ch
}
```

**Test: RetryAfter overrides delay**

```go
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
```

**Test: RetryAfterError unwraps correctly**

```go
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
```

**Test: MaxElapsedTime stops retries**

```go
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
```

**Test: Jitter varies delays**

```go
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
```

**Test: Timer interface used for delays**

```go
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
```

**Test: HTTP Retry-After header**

```go
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
		return http.Get(srv.URL)
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
```

**Step 1:** Add all 6 tests + instantTimer helper to retry_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./retry/ -v -count=1
```
Expected: All 14 tests PASS (8 existing + 6 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add retry/retry_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "retry: add tests for RetryAfter, MaxElapsedTime, Jitter, Timer

6 new tests covering all 4 additions. Uses instantTimer helper
for deterministic delay verification.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update retry section to show new features:

```go
import "github.com/anatolykoptev/go-kit/retry"

result, err := retry.Do(ctx, retry.Options{
    MaxAttempts:    5,
    InitialDelay:  500 * time.Millisecond,
    MaxDelay:      10 * time.Second,
    MaxElapsedTime: 30 * time.Second,  // total budget
    Jitter:        true,               // ±25% random jitter
}, func() (string, error) {
    return callAPI()
})

// HTTP-specific: retries on 429/5xx, auto-parses Retry-After header
resp, err := retry.HTTP(ctx, retry.Options{Jitter: true}, doRequest)

// Override backoff from fn:
return "", retry.RetryAfter(5*time.Second, err)
```

Update retry bullet points:
- Exponential backoff with optional jitter
- MaxElapsedTime wall-clock budget
- RetryAfter hint from fn or HTTP Retry-After header
- Timer interface for deterministic tests

**ROADMAP changes:**
- Mark retry additions 1-4 as DONE

**Step 1:** Update README.md retry section.

**Step 2:** Update ROADMAP.md retry status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update retry section for new features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
