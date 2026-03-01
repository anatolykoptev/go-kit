// Package retry provides generic retry logic with exponential backoff.
// Zero external dependencies. Designed to be used by any package that makes
// fallible I/O calls (LLM client, HTTP APIs, database operations).
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Default retry constants.
const (
	DefaultMaxAttempts  = 3
	DefaultInitialDelay = 500 * time.Millisecond
	DefaultMaxDelay     = 5 * time.Second
)

// Timer abstracts time.After for testability.
// Implement this to control delays in tests.
type Timer interface {
	After(d time.Duration) <-chan time.Time
}

// Options controls retry behavior.
// Zero values are replaced by the corresponding Default* constants.
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

// HTTPError is returned when an HTTP response has a retryable status code.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("retryable HTTP status %d", e.StatusCode)
}

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

// Do retries fn up to MaxAttempts times with exponential backoff.
// Respects context cancellation. Returns the last error if all attempts fail.
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

		// Check abort conditions.
		if shouldAbort(&opts, lastErr) {
			break
		}
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
