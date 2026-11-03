package wowa

import (
	"errors"
	"fmt"
)

// ErrNoToken is returned by NewClient when WithRequireAuth was set and no
// internal secret resolved (neither WithToken nor INTERNAL_SERVICE_SECRET).
// It fails construction instead of building a client that 401s on every call.
var ErrNoToken = errors.New("wowa: auth required but no secret configured")

// ErrBodyTruncated is returned when a response body exceeds the read cap
// (5 MiB). A truncated payload cannot be trusted — without the check it
// would surface only as a cryptic "unexpected end of JSON" decode error.
var ErrBodyTruncated = errors.New("response body exceeded 5 MiB cap")

// TimeoutError wraps a call that died on a deadline — the caller's ctx, the
// http.Client timeout, or the timeout_secs+4s per-call bound. Unwrap exposes
// the underlying error, so errors.Is(err, context.DeadlineExceeded) works
// through it.
type TimeoutError struct {
	Endpoint string // "fetch" | "read" | "render" | "interact" | "extract"
	Err      error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("wowa: %s: %v", e.Endpoint, e.Err)
}

func (e *TimeoutError) Unwrap() error { return e.Err }

// StatusError is a non-200 response whose body is not a go-wowa error
// envelope — e.g. a reverse-proxy error page. Body holds a truncated
// snippet for diagnostics.
type StatusError struct {
	Endpoint   string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("wowa: %s: server returned %d", e.Endpoint, e.StatusCode)
}

// RemoteError is a failure reported by go-wowa itself. Two wire conventions
// collapse here: endpoints answering HTTP 200 with a non-empty "error"
// field (render, interact), and endpoints answering non-200 with an
// {"error": "..."} envelope (extract, proxied ox fetch/read). StatusCode
// records which shape fired; Message is the remote's error string verbatim.
type RemoteError struct {
	Endpoint   string
	StatusCode int
	Message    string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("wowa: %s: remote error (http %d): %s", e.Endpoint, e.StatusCode, e.Message)
}
