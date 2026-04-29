package rerank

import (
	"context"
	"time"
)

// CircuitState represents the state of a circuit breaker.
// Defined here as a placeholder foundation for G1 which implements the full FSM.
type CircuitState uint8

const (
	// CircuitClosed is the normal state — calls pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the breaker has tripped — calls are short-circuited.
	CircuitOpen
	// CircuitHalfOpen means the breaker is probing for recovery.
	CircuitHalfOpen
)

// Observer receives lifecycle callbacks from the rerank client.
// All methods must be non-blocking. Panics are recovered by safeCall.
// Implement only the callbacks you care about; embed noopObserver for the rest.
type Observer interface {
	// OnBeforeCall fires before the HTTP request is sent.
	// n is the number of docs being sent to the server.
	OnBeforeCall(ctx context.Context, query string, n int)
	// OnAfterCall fires after the HTTP round-trip completes (success or error).
	// n is the number of docs in the result.
	OnAfterCall(ctx context.Context, status Status, dur time.Duration, n int)
	// OnRetry fires each time a request is retried (G1+).
	OnRetry(ctx context.Context, attempt int, err error)
	// OnCircuitTransition fires when the circuit breaker changes state (G1+).
	OnCircuitTransition(ctx context.Context, from, to CircuitState)
	// OnCacheHit fires when a cache hit short-circuits an HTTP call (G4+).
	// n is the number of docs whose scores were served from cache.
	OnCacheHit(ctx context.Context, n int)
	// OnTruncate fires when a document is truncated before being sent (G2+).
	OnTruncate(ctx context.Context, docID string, beforeTok, afterTok int)
}

// noopObserver is the default Observer — all callbacks are no-ops.
type noopObserver struct{}

func (noopObserver) OnBeforeCall(_ context.Context, _ string, _ int)                    {}
func (noopObserver) OnAfterCall(_ context.Context, _ Status, _ time.Duration, _ int)    {}
func (noopObserver) OnRetry(_ context.Context, _ int, _ error)                          {}
func (noopObserver) OnCircuitTransition(_ context.Context, _, _ CircuitState)           {}
func (noopObserver) OnCacheHit(_ context.Context, _ int)                                {}
func (noopObserver) OnTruncate(_ context.Context, _ string, _, _ int)                   {}

// safeCall invokes fn and recovers any panic it raises.
// A panic in user observer code MUST NOT kill the rerank request.
func safeCall(fn func()) {
	defer func() { _ = recover() }()
	fn()
}

// slogObserver wraps a *slog.Logger to implement Observer, preserving the
// v1 logging behavior when New(cfg, logger) is called.
// Only OnBeforeCall and OnAfterCall produce log output (matching v1 Warn on error).
// All other callbacks are no-ops — v1 never fired them.
type slogObserver struct {
	logger interface {
		Warn(msg string, args ...any)
	}
}

func (s slogObserver) OnBeforeCall(_ context.Context, _ string, _ int) {}
func (s slogObserver) OnAfterCall(_ context.Context, status Status, _ time.Duration, _ int) {
	// v1 logged on error inside Rerank body; the slogObserver delegates the
	// actual log call back to the client's existing logger field so v1
	// callers see identical log output. No action needed here — the client
	// still calls c.logger.Warn directly for v1 compat (see client.go).
}
func (s slogObserver) OnRetry(_ context.Context, _ int, _ error)               {}
func (s slogObserver) OnCircuitTransition(_ context.Context, _, _ CircuitState) {}
func (s slogObserver) OnCacheHit(_ context.Context, _ int)                     {}
func (s slogObserver) OnTruncate(_ context.Context, _ string, _, _ int)        {}
