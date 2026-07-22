package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// TestExecuteInnerNoValidAPIKey reproduces the silent-downgrade bug: when the
// primary apiKey is empty AND every fallback key is empty AND no endpoint chain
// is configured, executeInner used to fire doWithRetry with an empty Bearer
// token, receive an opaque 401/403, and (after the fallback loop silently
// skipped every empty key) return that opaque auth error to the caller. The
// fix returns the explicit ErrNoValidAPIKey sentinel before any HTTP call.
//
// The test server fails the test if reached — proving the guard short-circuits
// before any network attempt.
func TestExecuteInnerNoValidAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("HTTP server must not be reached when no API key is configured")
	}))
	t.Cleanup(srv.Close)

	// Empty apiKey, no fallback keys, no endpoints → no valid key anywhere.
	c := llm.NewClient(srv.URL, "", "test-model")

	_, err := c.CompleteRaw(context.Background(), nil)
	if !errors.Is(err, llm.ErrNoValidAPIKey) {
		t.Fatalf("expected ErrNoValidAPIKey, got %v", err)
	}
}

// TestExecuteInnerNoValidAPIKeyWithEmptyFallbackKeys ensures that a non-empty
// fallback key list where EVERY entry is empty is treated the same as having no
// fallback keys at all (the fallback loop's `if key == "" { continue }` would
// skip them all silently).
func TestExecuteInnerNoValidAPIKeyWithEmptyFallbackKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("HTTP server must not be reached when no API key is configured")
	}))
	t.Cleanup(srv.Close)

	c := llm.NewClient(srv.URL, "", "test-model", llm.WithFallbackKeys([]string{"", ""}))

	_, err := c.CompleteRaw(context.Background(), nil)
	if !errors.Is(err, llm.ErrNoValidAPIKey) {
		t.Fatalf("expected ErrNoValidAPIKey, got %v", err)
	}
}

// TestExecuteInnerNonEmptyKeyStillWorks is the regression guard: a non-empty
// apiKey must NOT trip the new guard — the call proceeds to the HTTP layer
// (and here succeeds against the test server).
func TestExecuteInnerNonEmptyKeyStillWorks(t *testing.T) {
	srv := newTestServer(t, okHandler("ok"))
	c := llm.NewClient(srv.URL, "real-key", "test-model")

	_, err := c.CompleteRaw(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
