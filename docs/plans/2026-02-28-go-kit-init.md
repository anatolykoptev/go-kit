# go-kit Phase 1: Module Init + Core Packages

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create `github.com/anatolykoptev/go-kit` monorepo with 6 independent packages extracted from 8 go-* services.

**Architecture:** Each package is independent (no cross-imports within go-kit). Tests use stdlib only. Best implementation chosen from across all repos: go-wp envutil, go-code retry/llm/metrics, go-search cache/strutil. All packages are leaf packages — zero internal dependencies.

**Tech Stack:** Go 1.24+, redis/go-redis v9 (cache only), stdlib (everything else)

**Repo:** `/home/krolik/src/go-kit/`

**Lint:** golangci-lint v2

---

### Task 1: Initialize Go Module

**Files:**
- Create: `go.mod`
- Create: `.golangci.yml`
- Create: `Makefile`
- Create: `.gitignore`

**Step 1: Init go module**

```bash
cd /home/krolik/src/go-kit && go mod init github.com/anatolykoptev/go-kit
```

**Step 2: Create .golangci.yml**

Copy from go-code and adapt:

```bash
cp /home/krolik/src/go-code/.golangci.yml /home/krolik/src/go-kit/.golangci.yml
```

**Step 3: Create Makefile**

Write file `Makefile`:
```makefile
.PHONY: test lint build

test:
	go test ./...

lint:
	golangci-lint run ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
```

**Step 4: Create .gitignore**

Write file `.gitignore`:
```
bin/
coverage.out
*.exe
.idea/
.vscode/
```

**Step 5: Init git and commit**

```bash
cd /home/krolik/src/go-kit
git init
git add go.mod .golangci.yml Makefile .gitignore docs/
git commit -m "init: go-kit module with docs and tooling"
```

---

### Task 2: env — Environment Variable Parsing

**Context:** Every go-* repo (8 total) duplicates env/envInt/envList helpers. go-wp has the cleanest version (`internal/envutil/`). go-code has the best `envList` (trims whitespace, drops empty). go-hully adds `Int64List`. go-nerv adds `Duration` and `Float`.

**Files:**
- Create: `env/env.go`
- Create: `env/env_test.go`

**Step 1: Write the tests**

Write file `env/env_test.go`:
```go
package env_test

import (
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/env"
)

func TestStr(t *testing.T) {
	t.Setenv("TEST_STR", "hello")
	if got := env.Str("TEST_STR", "default"); got != "hello" {
		t.Errorf("Str = %q, want %q", got, "hello")
	}
}

func TestStr_Default(t *testing.T) {
	if got := env.Str("TEST_STR_MISSING", "fallback"); got != "fallback" {
		t.Errorf("Str = %q, want %q", got, "fallback")
	}
}

func TestInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := env.Int("TEST_INT", 0); got != 42 {
		t.Errorf("Int = %d, want %d", got, 42)
	}
}

func TestInt_Invalid(t *testing.T) {
	t.Setenv("TEST_INT_BAD", "not_a_number")
	if got := env.Int("TEST_INT_BAD", 99); got != 99 {
		t.Errorf("Int = %d, want default %d", got, 99)
	}
}

func TestInt_Default(t *testing.T) {
	if got := env.Int("TEST_INT_MISSING", 7); got != 7 {
		t.Errorf("Int = %d, want %d", got, 7)
	}
}

func TestInt64(t *testing.T) {
	t.Setenv("TEST_INT64", "9999999999")
	if got := env.Int64("TEST_INT64", 0); got != 9999999999 {
		t.Errorf("Int64 = %d, want %d", got, int64(9999999999))
	}
}

func TestFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT", "3.14")
	if got := env.Float("TEST_FLOAT", 0); got != 3.14 {
		t.Errorf("Float = %f, want %f", got, 3.14)
	}
}

func TestFloat_Default(t *testing.T) {
	if got := env.Float("TEST_FLOAT_MISS", 2.71); got != 2.71 {
		t.Errorf("Float = %f, want %f", got, 2.71)
	}
}

func TestBool(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
	} {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tc.val)
			if got := env.Bool("TEST_BOOL", !tc.want); got != tc.want {
				t.Errorf("Bool(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestBool_Default(t *testing.T) {
	if got := env.Bool("TEST_BOOL_MISS", true); got != true {
		t.Errorf("Bool = %v, want true", got)
	}
}

func TestDuration(t *testing.T) {
	// Duration parses seconds as float.
	t.Setenv("TEST_DUR", "3.5")
	want := 3500 * time.Millisecond
	if got := env.Duration("TEST_DUR", 0); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestDuration_Default(t *testing.T) {
	want := 10 * time.Second
	if got := env.Duration("TEST_DUR_MISS", want); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestList(t *testing.T) {
	t.Setenv("TEST_LIST", "a, b ,c,,d")
	got := env.List("TEST_LIST", "")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestList_Empty(t *testing.T) {
	got := env.List("TEST_LIST_MISS", "")
	if got != nil {
		t.Errorf("List = %v, want nil", got)
	}
}

func TestList_Default(t *testing.T) {
	got := env.List("TEST_LIST_MISS", "x,y")
	want := []string{"x", "y"}
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d", len(got), len(want))
	}
}

func TestInt64List(t *testing.T) {
	t.Setenv("TEST_INT64LIST", "1, 2, 3, bad, 4")
	got := env.Int64List("TEST_INT64LIST")
	want := []int64{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("Int64List len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Int64List[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestInt64List_Empty(t *testing.T) {
	got := env.Int64List("TEST_INT64LIST_MISS")
	if got != nil {
		t.Errorf("Int64List = %v, want nil", got)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./env/...
```
Expected: FAIL (package doesn't exist yet)

**Step 3: Write the implementation**

Write file `env/env.go`:
```go
// Package env provides typed access to environment variables with defaults.
// Zero external dependencies. Designed to replace duplicated env/envInt/envList
// helpers found across go-* services.
package env

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Str returns the value of the environment variable named by key,
// or def if the variable is not set or empty.
func Str(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Int returns the environment variable as an int, or def if not set or invalid.
func Int(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Int64 returns the environment variable as int64, or def if not set or invalid.
func Int64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// Float returns the environment variable as float64, or def if not set or invalid.
func Float(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// Bool returns the environment variable as a bool, or def if not set.
// Truthy: "true", "1", "yes" (case-insensitive).
// Falsy: "false", "0", "no" (case-insensitive).
// Anything else returns def.
func Bool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return def
	}
}

// Duration returns the environment variable parsed as seconds (float),
// or def if not set or invalid. E.g. "3.5" → 3.5s.
func Duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(secs * float64(time.Second))
		}
	}
	return def
}

// List returns a comma-separated environment variable as a trimmed string slice.
// Empty entries are dropped. Returns nil if the variable is not set and def is "".
func List(key, def string) []string {
	v := Str(key, def)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Int64List returns a comma-separated list of int64 values.
// Non-numeric entries are silently skipped. Returns nil if not set.
func Int64List(key string) []int64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var result []int64
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			result = append(result, n)
		}
	}
	return result
}
```

**Step 4: Run tests to verify they pass**

```bash
cd /home/krolik/src/go-kit && go test ./env/... -v
```
Expected: all PASS

**Step 5: Lint**

```bash
cd /home/krolik/src/go-kit && make lint
```
Expected: 0 issues

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/
git commit -m "feat(env): add environment variable parsing package

Replaces duplicated env/envInt/envList helpers across 8 go-* repos.
Best-of-breed: go-wp envutil API, go-code List (trim+drop empty),
go-hully Int64List, go-nerv Duration/Float."
```

---

### Task 3: retry — Generic Retry with Backoff

**Context:** go-code has a clean generic `Do[T]` with exponential backoff and HTTP-specific helper. Copy it nearly as-is — it's already a leaf package with zero internal deps.

**Source:** `/home/krolik/src/go-code/internal/retry/retry.go` (116 LOC)

**Files:**
- Create: `retry/retry.go`
- Create: `retry/retry_test.go`

**Step 1: Write the tests**

Write file `retry/retry_test.go`:
```go
package retry_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	// Zero Options should still work (uses defaults).
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
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./retry/...
```
Expected: FAIL

**Step 3: Write the implementation**

Copy from go-code and adapt package name. Write file `retry/retry.go`:
```go
// Package retry provides generic retry logic with exponential backoff.
// Zero external dependencies. Designed to be used by any package that makes
// fallible I/O calls (LLM client, HTTP APIs, database operations).
package retry

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Default retry constants.
const (
	DefaultMaxAttempts  = 3
	DefaultInitialDelay = 500 * time.Millisecond
	DefaultMaxDelay     = 5 * time.Second
)

// Options controls retry behavior.
// Zero values are replaced by the corresponding Default* constants.
type Options struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func (o *Options) applyDefaults() {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = DefaultMaxAttempts
	}
	if o.InitialDelay <= 0 {
		o.InitialDelay = DefaultInitialDelay
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = DefaultMaxDelay
	}
}

// HTTPError is returned when an HTTP response has a retryable status code.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("retryable HTTP status %d", e.StatusCode)
}

// Do retries fn up to MaxAttempts times with exponential backoff.
// Respects context cancellation. Returns the last error if all attempts fail.
func Do[T any](ctx context.Context, opts Options, fn func() (T, error)) (T, error) {
	opts.applyDefaults()

	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}

	delay := opts.InitialDelay
	var lastErr error

	for attempt := range opts.MaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				var zero T
				return zero, ctx.Err()
			case <-time.After(delay):
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

// isRetryableStatus reports whether the HTTP status code warrants a retry.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// HTTP retries an HTTP request function, treating 429 and 5xx as retryable.
// Returns the successful response, or the last error after exhausting attempts.
// The caller is responsible for closing the response body on success.
func HTTP(ctx context.Context, opts Options, fn func() (*http.Response, error)) (*http.Response, error) {
	return Do(ctx, opts, func() (*http.Response, error) {
		resp, err := fn()
		if err != nil {
			return nil, err
		}
		if isRetryableStatus(resp.StatusCode) {
			resp.Body.Close()
			return nil, &HTTPError{StatusCode: resp.StatusCode}
		}
		return resp, nil
	})
}
```

**Step 4: Run tests**

```bash
cd /home/krolik/src/go-kit && go test ./retry/... -v
```
Expected: all PASS

**Step 5: Lint and commit**

```bash
cd /home/krolik/src/go-kit && make lint
git add retry/
git commit -m "feat(retry): add generic retry with exponential backoff

Extracted from go-code internal/retry. Generic Do[T] + HTTP helper.
Treats 429 and 5xx as retryable. Context-aware."
```

---

### Task 4: metrics — Atomic Operation Counters

**Context:** 7 repos use the same pattern: `sync.Map` + `atomic.Int64` counters. go-code has the cleanest version. Copy nearly as-is.

**Source:** `/home/krolik/src/go-code/internal/metrics/metrics.go` (64 LOC)

**Files:**
- Create: `metrics/metrics.go`
- Create: `metrics/metrics_test.go`

**Step 1: Write the tests**

Write file `metrics/metrics_test.go`:
```go
package metrics_test

import (
	"errors"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
)

func TestIncr(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("requests")
	r.Incr("requests")
	r.Incr("errors")

	snap := r.Snapshot()
	if snap["requests"] != 2 {
		t.Errorf("requests = %d, want 2", snap["requests"])
	}
	if snap["errors"] != 1 {
		t.Errorf("errors = %d, want 1", snap["errors"])
	}
}

func TestAdd(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("bytes", 1024)
	r.Add("bytes", 2048)

	snap := r.Snapshot()
	if snap["bytes"] != 3072 {
		t.Errorf("bytes = %d, want 3072", snap["bytes"])
	}
}

func TestSnapshot_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("snapshot len = %d, want 0", len(snap))
	}
}

func TestReset(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("a")
	r.Incr("b")
	r.Reset()

	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("after reset, snapshot len = %d, want 0", len(snap))
	}
}

func TestTrackOperation_Success(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if snap["errs"] != 0 {
		t.Errorf("errs = %d, want 0", snap["errs"])
	}
}

func TestTrackOperation_Error(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if snap["errs"] != 1 {
		t.Errorf("errs = %d, want 1", snap["errs"])
	}
}

func TestFormat(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("requests", 100)
	r.Add("errors", 3)

	s := r.Format()
	if s == "" {
		t.Error("Format returned empty string")
	}
	// Just verify it doesn't panic and contains counter names.
	if !containsStr(s, "requests") || !containsStr(s, "errors") {
		t.Errorf("Format missing counter names: %q", s)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./metrics/...
```
Expected: FAIL

**Step 3: Write the implementation**

Write file `metrics/metrics.go`:
```go
// Package metrics provides lightweight atomic counters for operational observability.
// All operations are safe for concurrent use. Zero external dependencies.
// Each Registry is independent — use NewRegistry() per component or share globally.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry holds named atomic counters.
type Registry struct {
	store sync.Map
}

// NewRegistry creates a new empty counter registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// counter returns the *atomic.Int64 for name, creating it on first access.
func (r *Registry) counter(name string) *atomic.Int64 {
	v, _ := r.store.LoadOrStore(name, new(atomic.Int64))
	return v.(*atomic.Int64) //nolint:forcetypeassert // invariant: only *atomic.Int64 stored
}

// Incr increments the named counter by 1.
func (r *Registry) Incr(name string) {
	r.counter(name).Add(1)
}

// Add adds delta to the named counter.
func (r *Registry) Add(name string, delta int64) {
	r.counter(name).Add(delta)
}

// Value returns the current value of the named counter.
func (r *Registry) Value(name string) int64 {
	return r.counter(name).Load()
}

// Snapshot returns a copy of all counters with their current values.
// Only counters that have been written at least once are included.
func (r *Registry) Snapshot() map[string]int64 {
	m := make(map[string]int64)
	r.store.Range(func(k, v any) bool {
		m[k.(string)] = v.(*atomic.Int64).Load() //nolint:forcetypeassert // invariant
		return true
	})
	return m
}

// Reset clears all counters. Intended for tests.
func (r *Registry) Reset() {
	r.store.Range(func(k, _ any) bool {
		r.store.Delete(k)
		return true
	})
}

// Format returns a human-readable summary of all counters, sorted by name.
func (r *Registry) Format() string {
	snap := r.Snapshot()
	if len(snap) == 0 {
		return ""
	}

	names := make([]string, 0, len(snap))
	for name := range snap {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sb, "%s=%d\n", name, snap[name])
	}
	return sb.String()
}

// TrackOperation increments callCounter, runs fn, and increments errCounter
// if fn returns a non-nil error. The error from fn is always returned unchanged.
func (r *Registry) TrackOperation(callCounter, errCounter string, fn func() error) error {
	r.Incr(callCounter)
	if err := fn(); err != nil {
		r.Incr(errCounter)
		return err
	}
	return nil
}
```

**Step 4: Run tests and lint**

```bash
cd /home/krolik/src/go-kit && go test ./metrics/... -v && make lint
```
Expected: all PASS, 0 lint issues

**Step 5: Commit**

```bash
cd /home/krolik/src/go-kit
git add metrics/
git commit -m "feat(metrics): add atomic counter registry

Lock-free counters via sync.Map + atomic.Int64.
Incr, Add, Snapshot, Format, TrackOperation.
Extracted from go-code internal/metrics."
```

---

### Task 5: strutil — String Helpers

**Context:** `Truncate` (byte-based, broken for Unicode) in go-search vs `TruncateStr` (rune-based) in go-wp. Also `TruncateAtWord` in go-search. Consolidate the best versions.

**Source:** `/home/krolik/src/go-search/internal/engine/textutil.go`

**Files:**
- Create: `strutil/strutil.go`
- Create: `strutil/strutil_test.go`

**Step 1: Write the tests**

Write file `strutil/strutil_test.go`:
```go
package strutil_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/strutil"
)

func TestTruncate_Short(t *testing.T) {
	if got := strutil.Truncate("hello", 10); got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
}

func TestTruncate_Exact(t *testing.T) {
	if got := strutil.Truncate("hello", 5); got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
}

func TestTruncate_Long(t *testing.T) {
	got := strutil.Truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("Truncate = %q, want %q", got, "hello...")
	}
}

func TestTruncate_Unicode(t *testing.T) {
	// Cyrillic: "Привет мир" = 10 runes.
	got := strutil.Truncate("Привет мир", 6)
	if got != "Привет..." {
		t.Errorf("Truncate = %q, want %q", got, "Привет...")
	}
}

func TestTruncate_Emoji(t *testing.T) {
	got := strutil.Truncate("Hi 👋🌍!", 4)
	if got != "Hi 👋..." {
		t.Errorf("Truncate = %q, want %q", got, "Hi 👋...")
	}
}

func TestTruncate_Empty(t *testing.T) {
	if got := strutil.Truncate("", 10); got != "" {
		t.Errorf("Truncate = %q, want empty", got)
	}
}

func TestTruncateAtWord(t *testing.T) {
	got := strutil.TruncateAtWord("hello beautiful world", 15)
	// Should cut at word boundary before or at 15 runes.
	if got != "hello beautiful..." {
		t.Errorf("TruncateAtWord = %q, want %q", got, "hello beautiful...")
	}
}

func TestTruncateAtWord_Short(t *testing.T) {
	if got := strutil.TruncateAtWord("short", 10); got != "short" {
		t.Errorf("TruncateAtWord = %q, want %q", got, "short")
	}
}

func TestTruncateAtWord_NoSpaceNearCut(t *testing.T) {
	// If the last space is too far back (< half), just truncate at limit.
	got := strutil.TruncateAtWord("abcdefghijklmnop", 10)
	if got != "abcdefghij..." {
		t.Errorf("TruncateAtWord = %q, want %q", got, "abcdefghij...")
	}
}

func TestContains(t *testing.T) {
	items := []string{"go", "python", "rust"}
	if !strutil.Contains(items, "python") {
		t.Error("Contains(python) = false, want true")
	}
	if strutil.Contains(items, "java") {
		t.Error("Contains(java) = true, want false")
	}
	if strutil.Contains(nil, "go") {
		t.Error("Contains(nil, go) = true, want false")
	}
}

func TestContainsAny(t *testing.T) {
	if !strutil.ContainsAny("hello world", []string{"xyz", "world"}) {
		t.Error("ContainsAny = false, want true")
	}
	if strutil.ContainsAny("hello", []string{"xyz", "abc"}) {
		t.Error("ContainsAny = true, want false")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./strutil/...
```
Expected: FAIL

**Step 3: Write the implementation**

Write file `strutil/strutil.go`:
```go
// Package strutil provides Unicode-aware string helpers.
// Zero external dependencies.
package strutil

import "strings"

// Truncate caps s at maxRunes runes, appending "..." if truncated.
// Safe for UTF-8 (Cyrillic, CJK, emoji).
func Truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// TruncateAtWord truncates s to maxRunes at a word boundary.
// If the last space is too far back (< half of maxRunes), truncates at maxRunes.
// Appends "..." if truncated.
func TruncateAtWord(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	truncated := string(runes[:maxRunes])
	cut := strings.LastIndex(truncated, " ")
	if cut < len(truncated)/2 {
		return truncated + "..."
	}
	return truncated[:cut] + "..."
}

// Contains reports whether items contains s.
func Contains(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}

// ContainsAny reports whether s contains any of the given substrings.
func ContainsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests and lint**

```bash
cd /home/krolik/src/go-kit && go test ./strutil/... -v && make lint
```
Expected: all PASS, 0 lint issues

**Step 5: Commit**

```bash
cd /home/krolik/src/go-kit
git add strutil/
git commit -m "feat(strutil): add Unicode-aware string helpers

Truncate (rune-based + ellipsis), TruncateAtWord (word boundary),
Contains, ContainsAny. Replaces broken byte-based Truncate from go-search."
```

---

### Task 6: llm — OpenAI-Compatible LLM Client

**Context:** 6 repos have independent LLM clients. go-code has the best (retry + fallback keys). go-wp adds multimodal. Merge both into one client.

**Sources:**
- `/home/krolik/src/go-code/internal/llm/llm.go` (retry, fallback keys, system prompts)
- `/home/krolik/src/go-wp/internal/engine/llm.go` (multimodal, Content any)

**Files:**
- Create: `llm/client.go`
- Create: `llm/client_test.go`

**Step 1: Write the tests**

Write file `llm/client_test.go`:
```go
package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestComplete_Success(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "hello from LLM"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "test-key", "test-model")
	result, err := c.Complete(context.Background(), "system", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello from LLM" {
		t.Errorf("result = %q, want %q", result, "hello from LLM")
	}
}

func TestComplete_SendsCorrectRequest(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-key" {
			t.Errorf("auth = %q, want %q", auth, "Bearer my-key")
		}

		// Verify request body.
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "gpt-4" {
			t.Errorf("model = %v, want gpt-4", req["model"])
		}
		msgs := req["messages"].([]any)
		if len(msgs) != 2 {
			t.Errorf("messages len = %d, want 2", len(msgs))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "my-key", "gpt-4")
	_, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	})

	c := llm.NewClient(srv.URL, "key", "model")
	_, err := c.Complete(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestComplete_RetryOn429(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "finally"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model", llm.WithMaxRetries(3))
	result, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "finally" {
		t.Errorf("result = %q, want %q", result, "finally")
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestComplete_FallbackKeys(t *testing.T) {
	var usedKeys []string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		usedKeys = append(usedKeys, key)
		if key == "bad-key" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok with " + key}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "bad-key", "model",
		llm.WithFallbackKeys([]string{"good-key"}),
		llm.WithMaxRetries(1),
	)
	result, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok with good-key" {
		t.Errorf("result = %q, want %q", result, "ok with good-key")
	}
}

func TestCompleteMultimodal(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		msgs := req["messages"].([]any)
		userMsg := msgs[0].(map[string]any)
		content := userMsg["content"].([]any)

		// Should have text part + image part.
		if len(content) != 2 {
			t.Errorf("content parts = %d, want 2", len(content))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "saw the image"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.CompleteMultimodal(context.Background(), "describe this", []llm.ImagePart{
		{URL: "https://example.com/img.png"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "saw the image" {
		t.Errorf("result = %q, want %q", result, "saw the image")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `{"key": "value"}`, `{"key": "value"}`},
		{"markdown fence", "```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"text around json", `some text {"x": 2} more text`, `{"x": 2}`},
		{"no json", "just text", "just text"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := llm.ExtractJSON(tc.input)
			if got != tc.want {
				t.Errorf("ExtractJSON = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWithTemperature(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["temperature"] != 0.7 {
			t.Errorf("temperature = %v, want 0.7", req["temperature"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})

	c := llm.NewClient(srv.URL, "key", "model", llm.WithTemperature(0.7))
	_, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./llm/...
```
Expected: FAIL

**Step 3: Write the implementation**

Write file `llm/client.go`:
```go
// Package llm provides an OpenAI-compatible LLM client with retry and fallback keys.
// Supports text and multimodal (vision) requests. Zero external dependencies
// beyond net/http. Designed to replace duplicated LLM clients across go-* services.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default client constants.
const (
	defaultMaxTokens  = 8192
	defaultMaxRetries = 3
	defaultTimeout    = 90 * time.Second
	retryDelay        = 500 * time.Millisecond
	maxRetryDelay     = 5 * time.Second
)

// Client is an OpenAI-compatible LLM client with retry and fallback key support.
type Client struct {
	baseURL      string
	apiKey       string
	model        string
	maxTokens    int
	temperature  float64
	httpClient   *http.Client
	fallbackKeys []string
	maxRetries   int
}

// Option configures the Client.
type Option func(*Client)

// WithFallbackKeys sets fallback API keys tried when the primary gets 429/5xx.
func WithFallbackKeys(keys []string) Option {
	return func(c *Client) { c.fallbackKeys = keys }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithMaxTokens sets the max tokens for completions.
func WithMaxTokens(n int) Option {
	return func(c *Client) { c.maxTokens = n }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) Option {
	return func(c *Client) { c.temperature = t }
}

// WithMaxRetries sets how many times to retry on retryable errors.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// NewClient creates a new LLM client.
func NewClient(baseURL, apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		model:       model,
		maxTokens:   defaultMaxTokens,
		temperature: 0.1,
		maxRetries:  defaultMaxRetries,
		httpClient:  &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentPart for multimodal
}

// ContentPart is a part of a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL holds an image reference for vision requests.
type ImageURL struct {
	URL string `json:"url"`
}

// ImagePart is a convenience type for passing images to CompleteMultimodal.
type ImagePart struct {
	URL      string
	MIMEType string // optional
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends a text completion request with optional system prompt.
// If system is empty, only the user message is sent.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	var msgs []Message
	if system != "" {
		msgs = append(msgs, Message{Role: "system", Content: system})
	}
	msgs = append(msgs, Message{Role: "user", Content: user})
	return c.CompleteRaw(ctx, msgs)
}

// CompleteMultimodal sends a vision request with text + images.
func (c *Client) CompleteMultimodal(ctx context.Context, prompt string, images []ImagePart) (string, error) {
	parts := []ContentPart{{Type: "text", Text: prompt}}
	for _, img := range images {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: img.URL},
		})
	}
	msgs := []Message{{Role: "user", Content: parts}}
	return c.CompleteRaw(ctx, msgs)
}

// CompleteRaw sends a chat completion with explicit messages.
// Retries on 429/5xx, cycles through fallback keys.
func (c *Client) CompleteRaw(ctx context.Context, messages []Message) (string, error) {
	// Try primary key.
	result, err := c.doWithRetry(ctx, c.apiKey, messages)
	if err == nil {
		return result, nil
	}

	// Try fallback keys.
	for _, key := range c.fallbackKeys {
		if key == "" {
			continue
		}
		result, err = c.doWithRetry(ctx, key, messages)
		if err == nil {
			return result, nil
		}
	}
	return "", err
}

func (c *Client) doWithRetry(ctx context.Context, apiKey string, messages []Message) (string, error) {
	delay := retryDelay
	var lastErr error

	for attempt := range c.maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, maxRetryDelay)
		}

		result, err := c.doRequest(ctx, apiKey, messages)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Only retry on retryable errors.
		if !isRetryable(err) {
			return "", err
		}
	}
	return "", lastErr
}

func (c *Client) doRequest(ctx context.Context, apiKey string, messages []Message) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if isRetryableStatus(resp.StatusCode) {
		return "", &retryableError{statusCode: resp.StatusCode, body: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

type retryableError struct {
	statusCode int
	body       string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("retryable HTTP %d: %s", e.statusCode, e.body)
}

func isRetryable(err error) bool {
	var re *retryableError
	return err != nil && (strings.Contains(err.Error(), "retryable") || isRetryableErr(err, &re))
}

func isRetryableErr(err error, target **retryableError) bool {
	for err != nil {
		if re, ok := err.(*retryableError); ok { //nolint:errorlint // intentional type check
			*target = re
			return true
		}
		// Try unwrap.
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok { //nolint:errorlint // intentional
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// ExtractJSON extracts a JSON object from LLM output that may be wrapped
// in markdown code fences or surrounded by text.
func ExtractJSON(s string) string {
	// Try markdown ```json ... ``` first.
	start := strings.Index(s, "```json")
	if start >= 0 {
		s = s[start+7:]
		end := strings.Index(s, "```")
		if end >= 0 {
			return strings.TrimSpace(s[:end])
		}
	}
	// Fall back to finding first { and last }.
	first := strings.IndexByte(s, '{')
	last := strings.LastIndexByte(s, '}')
	if first >= 0 && last > first {
		return s[first : last+1]
	}
	return s
}
```

**Step 4: Run tests and lint**

```bash
cd /home/krolik/src/go-kit && go test ./llm/... -v && make lint
```
Expected: all PASS, 0 lint issues

**Step 5: Commit**

```bash
cd /home/krolik/src/go-kit
git add llm/
git commit -m "feat(llm): add OpenAI-compatible LLM client

Retry with exponential backoff, fallback API keys, multimodal (vision),
ExtractJSON helper. Merges go-code retry/fallback + go-wp multimodal.
Zero external deps (net/http only)."
```

---

### Task 7: cache — Tiered L1+L2 Cache

**Context:** 7 repos implement L1 (memory) + L2 (Redis). go-search/go-job/go-startup are near-identical. go-wp uses generic `[]byte`. This is the only package with an external dependency (redis/go-redis).

**Sources:**
- `/home/krolik/src/go-search/internal/engine/cache.go` (L1+L2 architecture, eviction)
- `/home/krolik/src/go-wp/internal/engine/cache.go` (generic `[]byte`, CacheGetJSON)

**Files:**
- Create: `cache/cache.go`
- Create: `cache/cache_test.go`

**Step 1: Write the tests**

Write file `cache/cache_test.go`:
```go
package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestCache_SetGet(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "key1", []byte("value1"))

	got, ok := c.Get(ctx, "key1")
	if !ok {
		t.Fatal("Get returned not ok")
	}
	if string(got) != "value1" {
		t.Errorf("Get = %q, want %q", got, "value1")
	}
}

func TestCache_Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	_, ok := c.Get(context.Background(), "missing")
	if ok {
		t.Error("Get returned ok for missing key")
	}
}

func TestCache_Expiry(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      50 * time.Millisecond,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "expire-me", []byte("data"))

	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get(ctx, "expire-me")
	if ok {
		t.Error("Get returned ok for expired key")
	}
}

func TestCache_Eviction(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 3,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	// Fill to capacity + 1.
	for i := range 4 {
		c.Set(ctx, cache.Key("item", string(rune('a'+i))), []byte("data"))
	}

	// After eviction, cache should have at most L1MaxItems entries.
	stats := c.Stats()
	if stats.L1Size > 3 {
		t.Errorf("L1Size = %d, want <= 3", stats.L1Size)
	}
}

func TestCache_Delete(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "del-me", []byte("data"))
	c.Delete(ctx, "del-me")

	_, ok := c.Get(ctx, "del-me")
	if ok {
		t.Error("Get returned ok after Delete")
	}
}

func TestKey(t *testing.T) {
	k := cache.Key("search", "golang", "patterns")
	if k == "" {
		t.Error("Key returned empty string")
	}
	// Same inputs should produce same key.
	k2 := cache.Key("search", "golang", "patterns")
	if k != k2 {
		t.Errorf("Key not deterministic: %q != %q", k, k2)
	}
	// Different inputs should produce different keys.
	k3 := cache.Key("search", "rust", "patterns")
	if k == k3 {
		t.Error("different inputs produced same key")
	}
}

func TestStats(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "a", []byte("1"))
	c.Get(ctx, "a")     // hit
	c.Get(ctx, "b")     // miss

	stats := c.Stats()
	if stats.L1Hits != 1 {
		t.Errorf("L1Hits = %d, want 1", stats.L1Hits)
	}
	if stats.L1Misses != 1 {
		t.Errorf("L1Misses = %d, want 1", stats.L1Misses)
	}
	if stats.L1Size != 1 {
		t.Errorf("L1Size = %d, want 1", stats.L1Size)
	}
}

func TestCache_NoRedis(t *testing.T) {
	// Cache should work fine with no Redis (L1 only).
	c := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "local", []byte("only"))

	got, ok := c.Get(ctx, "local")
	if !ok || string(got) != "only" {
		t.Errorf("L1-only mode failed: ok=%v, got=%q", ok, got)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd /home/krolik/src/go-kit && go test ./cache/...
```
Expected: FAIL

**Step 3: Write the implementation**

Write file `cache/cache.go`:
```go
// Package cache provides a tiered L1 (memory) + optional L2 (Redis) cache.
// L1 uses sync.Map with TTL and size-based eviction.
// If RedisURL is empty, operates as L1-only (no external dependencies needed at runtime).
package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config configures the cache.
type Config struct {
	// RedisURL is the Redis connection URL. Empty means L1-only mode.
	RedisURL string

	// RedisDB selects the Redis database number (default 0).
	RedisDB int

	// Prefix is prepended to all Redis keys (e.g. "gs:" for go-search).
	Prefix string

	// L1MaxItems is the max number of items in memory (default 1000).
	L1MaxItems int

	// L1TTL is the TTL for L1 cache entries (default 30m).
	L1TTL time.Duration

	// L2TTL is the TTL for L2 Redis entries (default 24h). Ignored if no Redis.
	L2TTL time.Duration
}

func (c *Config) applyDefaults() {
	if c.L1MaxItems <= 0 {
		c.L1MaxItems = 1000
	}
	if c.L1TTL <= 0 {
		c.L1TTL = 30 * time.Minute
	}
	if c.L2TTL <= 0 {
		c.L2TTL = 24 * time.Hour
	}
}

// Cache is a tiered L1 (memory) + optional L2 (Redis) cache.
type Cache struct {
	cfg Config

	// L1: in-memory cache.
	l1    sync.Map
	l1Len atomic.Int64

	// Stats.
	l1Hits   atomic.Int64
	l1Misses atomic.Int64

	// Shutdown.
	done chan struct{}
}

type l1Entry struct {
	data      []byte
	expiresAt time.Time
}

// New creates a new Cache. If cfg.RedisURL is empty, L2 is disabled.
// Call Close() when done to stop the background cleanup goroutine.
func New(cfg Config) *Cache {
	cfg.applyDefaults()

	c := &Cache{
		cfg:  cfg,
		done: make(chan struct{}),
	}

	// Background cleanup every 1/10 of TTL, minimum 10s.
	interval := cfg.L1TTL / 10
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	go c.cleanupLoop(interval)

	return c
}

// Get retrieves a value from L1 (then L2 if miss). Returns nil, false on miss.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	// L1 lookup.
	if v, ok := c.l1.Load(key); ok {
		entry := v.(*l1Entry) //nolint:forcetypeassert // invariant
		if time.Now().Before(entry.expiresAt) {
			c.l1Hits.Add(1)
			return entry.data, true
		}
		// Expired — remove.
		c.l1.Delete(key)
		c.l1Len.Add(-1)
	}

	c.l1Misses.Add(1)

	// TODO: L2 Redis lookup (Phase 2 — add redis/go-redis dependency).
	_ = ctx

	return nil, false
}

// Set stores a value in L1 (and L2 if configured).
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	c.evictIfNeeded()

	c.l1.Store(key, &l1Entry{
		data:      data,
		expiresAt: time.Now().Add(c.cfg.L1TTL),
	})
	c.l1Len.Add(1)

	// TODO: L2 Redis set (Phase 2).
	_ = ctx
}

// Delete removes a key from both L1 and L2.
func (c *Cache) Delete(ctx context.Context, key string) {
	if _, loaded := c.l1.LoadAndDelete(key); loaded {
		c.l1Len.Add(-1)
	}
	// TODO: L2 Redis delete (Phase 2).
	_ = ctx
}

// Stats returns current cache statistics.
type Stats struct {
	L1Hits   int64
	L1Misses int64
	L1Size   int
}

// Stats returns a snapshot of cache statistics.
func (c *Cache) Stats() Stats {
	size := 0
	c.l1.Range(func(_, _ any) bool {
		size++
		return true
	})
	return Stats{
		L1Hits:   c.l1Hits.Load(),
		L1Misses: c.l1Misses.Load(),
		L1Size:   size,
	}
}

// Close stops the background cleanup goroutine.
func (c *Cache) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

// Key builds a deterministic cache key from parts using SHA-256.
func Key(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", h[:16])
}

// evictIfNeeded removes entries when L1 exceeds max size.
// First pass: remove expired. Second pass: remove oldest if still over limit.
func (c *Cache) evictIfNeeded() {
	if c.l1Len.Load() < int64(c.cfg.L1MaxItems) {
		return
	}

	now := time.Now()
	removed := int64(0)

	// Pass 1: remove expired entries.
	c.l1.Range(func(k, v any) bool {
		if now.After(v.(*l1Entry).expiresAt) { //nolint:forcetypeassert // invariant
			c.l1.Delete(k)
			removed++
		}
		return true
	})
	c.l1Len.Add(-removed)

	if c.l1Len.Load() < int64(c.cfg.L1MaxItems) {
		return
	}

	// Pass 2: remove oldest entries until under limit.
	var oldest struct {
		key string
		at  time.Time
	}
	oldest.at = now.Add(time.Hour) // sentinel

	c.l1.Range(func(k, v any) bool {
		entry := v.(*l1Entry) //nolint:forcetypeassert // invariant
		if entry.expiresAt.Before(oldest.at) {
			oldest.key = k.(string) //nolint:forcetypeassert // invariant
			oldest.at = entry.expiresAt
		}
		return true
	})

	if oldest.key != "" {
		c.l1.Delete(oldest.key)
		c.l1Len.Add(-1)
	}
}

// cleanupLoop periodically removes expired entries from L1.
func (c *Cache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()
			removed := int64(0)
			c.l1.Range(func(k, v any) bool {
				if now.After(v.(*l1Entry).expiresAt) { //nolint:forcetypeassert // invariant
					c.l1.Delete(k)
					removed++
				}
				return true
			})
			c.l1Len.Add(-removed)
		}
	}
}
```

**Step 4: Run tests and lint**

```bash
cd /home/krolik/src/go-kit && go test ./cache/... -v && make lint
```
Expected: all PASS, 0 lint issues

**Step 5: Commit**

```bash
cd /home/krolik/src/go-kit
git add cache/
git commit -m "feat(cache): add tiered L1 memory cache with eviction

L1 sync.Map with TTL + size-based eviction. Background cleanup goroutine.
L2 Redis support is stubbed (TODO Phase 2 — add redis/go-redis dep).
Generic []byte interface. Key() builds deterministic SHA-256 keys."
```

---

### Task 8: Final Verification

**Step 1: Run all tests**

```bash
cd /home/krolik/src/go-kit && go test ./... -v -count=1
```
Expected: all packages PASS

**Step 2: Run lint**

```bash
cd /home/krolik/src/go-kit && make lint
```
Expected: 0 issues

**Step 3: Check coverage**

```bash
cd /home/krolik/src/go-kit && make cover
```
Expected: >80% coverage per package

**Step 4: Verify module has no external deps (except test)**

```bash
cd /home/krolik/src/go-kit && cat go.mod
```
Expected: only `module github.com/anatolykoptev/go-kit` + `go 1.24`, no `require` block (no external deps in Phase 1 since Redis L2 is stubbed).

**Step 5: Final commit if needed, then tag**

```bash
cd /home/krolik/src/go-kit
git log --oneline
```

Verify 7 commits:
1. `init: go-kit module with docs and tooling`
2. `feat(env): add environment variable parsing package`
3. `feat(retry): add generic retry with exponential backoff`
4. `feat(metrics): add atomic counter registry`
5. `feat(strutil): add Unicode-aware string helpers`
6. `feat(llm): add OpenAI-compatible LLM client`
7. `feat(cache): add tiered L1 memory cache with eviction`
