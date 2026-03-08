# retry Additions 5-6: Per-Error Abort, RetryableError Marker

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add per-error-type abort (never retry certain errors) and RetryableError marker interface (opt-in retry mode) — completing the retry package for production safety.

**Architecture:** `AbortOn` field in Options lists errors to never retry (checked via errors.Is). `RetryableOnly` field inverts default: only errors implementing `Retryable` interface are retried. Both checked via `shouldAbort()` helper after each failed attempt. stdlib only.

**Tech Stack:** Go stdlib only (`errors`)

---

### Task 1: All retry code additions

**Files:** retry/retry.go

#### 1a. Add AbortOn and RetryableOnly to Options

Add to Options struct:

```go
type Options struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	MaxElapsedTime time.Duration // total wall-clock budget; 0 = no limit
	Jitter         bool          // add ±25% random jitter to delay
	Timer          Timer         // custom timer for tests; nil = real time.After
	AbortOn        []error       // never retry these errors (checked via errors.Is)
	RetryableOnly  bool          // if true, only retry errors implementing Retryable
}
```

#### 1b. Add Retryable interface, MarkRetryable, IsRetryable

Add after RetryAfter/HTTPError section:

```go
// Retryable is an interface that errors can implement to signal
// whether they should be retried. Used with Options.RetryableOnly.
type Retryable interface {
	Retryable() bool
}

type retryableError struct {
	err error
}

func (e *retryableError) Error() string   { return e.err.Error() }
func (e *retryableError) Unwrap() error   { return e.err }
func (e *retryableError) Retryable() bool { return true }

// MarkRetryable wraps an error to signal it should be retried.
// Use with Options.RetryableOnly = true.
func MarkRetryable(err error) error {
	return &retryableError{err: err}
}

// IsRetryable reports whether err should be retried.
// Returns true if err implements Retryable and Retryable() returns true.
func IsRetryable(err error) bool {
	var r Retryable
	if errors.As(err, &r) {
		return r.Retryable()
	}
	return false
}
```

#### 1c. Add shouldAbort helper and integrate into Do()

Add helper function:

```go
func shouldAbort(opts *Options, err error) bool {
	for _, target := range opts.AbortOn {
		if errors.Is(err, target) {
			return true
		}
	}
	if opts.RetryableOnly && !IsRetryable(err) {
		return true
	}
	return false
}
```

Modify Do() — after `lastErr = err` (line 131), add abort check:

```go
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Check abort conditions.
		if shouldAbort(&opts, lastErr) {
			break
		}
```

**Step 1:** Apply all changes (1a-1c).

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./retry/ -v -count=1
```
Expected: All 13 existing tests PASS.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add retry/retry.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "retry: add AbortOn, RetryableOnly, MarkRetryable

Additions 5-6:
- AbortOn: never retry specific errors (checked via errors.Is)
- RetryableOnly: opt-in retry mode via Retryable interface
- MarkRetryable: wrap errors to signal retryability
- IsRetryable: check if error implements Retryable
- shouldAbort: unified abort check after each failed attempt

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for new features

**Files:** retry/retry_test.go

**Test: AbortOn stops retry on matching error**

```go
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
```

**Test: AbortOn allows retry on non-matching error**

```go
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
```

**Test: RetryableOnly retries marked errors**

```go
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
```

**Test: RetryableOnly aborts on unmarked error**

```go
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
```

**Test: MarkRetryable preserves error chain**

```go
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
```

**Test: IsRetryable on nil and plain errors**

```go
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
```

**Step 1:** Add all 6 tests to retry_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./retry/ -v -count=1
```
Expected: All tests PASS (13 existing + 6 new = 19).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add retry/retry_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "retry: add tests for AbortOn, RetryableOnly, MarkRetryable

6 new tests: AbortOn (match/non-match), RetryableOnly
(retry/abort), MarkRetryable unwrap, IsRetryable.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update retry section to include:

```go
// Abort on specific errors (never retry)
retry.Do(ctx, retry.Options{
    AbortOn: []error{context.DeadlineExceeded},
}, fn)

// Opt-in retry: only marked errors are retried
retry.Do(ctx, retry.Options{
    RetryableOnly: true,
}, func() (T, error) {
    return result, retry.MarkRetryable(err) // will retry
    return result, err                       // won't retry
})
```

Add bullet points:
- AbortOn: never retry specific errors
- RetryableOnly + MarkRetryable: opt-in retry mode for production safety

**ROADMAP changes:**
- Update retry status to "DONE (items 1-6)"

**Step 1:** Update README.md retry section.

**Step 2:** Update ROADMAP.md retry status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update retry section for additions 5-6

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
