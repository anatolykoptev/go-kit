package embed

import (
	"context"
	"fmt"
)

// embedWithFallback tries the primary client and, on StatusDegraded with a
// non-4xx error, tries the secondary client. Returns StatusFallback on
// secondary success. Returns the primary's Degraded result if:
//   - the error was a 4xx (caller error — same bug will repeat on secondary)
//   - secondary is nil
//   - secondary also fails
//
// Fallback is capped at depth 1: primary → secondary. No further chaining.
// opts are forwarded to both primary and secondary calls.
func embedWithFallback(
	ctx context.Context,
	primary *Client,
	secondary *Client,
	texts []string,
	opts ...EmbedOpt,
) *Result {
	res := primary.embedWithResultUnchained(ctx, texts, opts...)
	if res.Status != StatusDegraded {
		return res
	}
	if isClientError(res.Err) {
		// 4xx — caller error; secondary would see the same problem.
		recordGiveup(primary.model, "4xx")
		return res
	}
	if secondary == nil {
		return res
	}

	// Attempt secondary.
	fallRes := secondary.embedWithResultUnchained(ctx, texts, opts...)
	if fallRes.Status == StatusOk {
		fallRes.Status = StatusFallback
		recordFallbackUsed(primary.model, secondary.model)
		return fallRes
	}
	// Both failed — return primary's Degraded result.
	return res
}

// isClientError returns true when err represents a 4xx HTTP status code
// (a caller-side error that would repeat if retried against secondary).
func isClientError(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(errHTTPStatus) //nolint:errorlint
	if !ok {
		return false
	}
	return e.Code >= 400 && e.Code < 500
}

// errHTTPStatus is a typed error carrying the HTTP status code from a non-2xx
// response. Using a typed error (rather than a plain fmt.Errorf string) allows
// do() to type-assert the code for RetryableStatus filtering without parsing
// the error string.
//
// The Error() string is "http status <code>" — identical to the rerank package's
// errHTTPStatus format, preserving backward compatibility for any caller that
// does strings.Contains(err.Error(), "http status").
type errHTTPStatus struct {
	Code int
}

func (e errHTTPStatus) Error() string {
	return fmt.Sprintf("http status %d", e.Code)
}
