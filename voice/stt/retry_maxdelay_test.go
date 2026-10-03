package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestWithRetryMaxDelayConfigurable verifies that WithRetryWithMaxDelay
// respects a custom maxDelay instead of the hardcoded 30s default.
// With baseDelay=1s, maxDelay=2s, and 5 attempts, the total backoff
// (1+2+2+2 = 7s) is much less than the default (1+2+4+8 = 15s).
func TestWithRetryMaxDelayConfigurable(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := stt.New(srv.URL, stt.WithRetryWithMaxDelay(5, 100*time.Millisecond, 200*time.Millisecond))

	start := time.Now()
	_, err := c.TranscribeReader(context.Background(), strings.NewReader("audio"), "test.wav")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from 503 server")
	}
	// 5 attempts with backoff: 100+200+200+200 = 700ms (capped at 200ms after 2nd).
	// Without maxDelay cap, it would be 100+200+400+800 = 1500ms.
	if elapsed > 1500*time.Millisecond {
		t.Errorf("elapsed = %v, expected < 1.5s with maxDelay=200ms cap", elapsed)
	}
	if calls.Load() != 5 {
		t.Errorf("calls = %d, want 5", calls.Load())
	}
}

// TestWithRetryMaxDelayValidation verifies that invalid maxDelay panics.
func TestWithRetryMaxDelayValidation(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for maxDelay <= 0")
		}
	}()
	stt.WithRetryWithMaxDelay(3, 100*time.Millisecond, 0)
}

// TestWithRetryMaxDelayLessThanBaseDelay verifies that maxDelay < baseDelay
// is accepted (baseDelay is capped immediately on first doubling).
func TestWithRetryMaxDelayLessThanBaseDelay(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// baseDelay=200ms, maxDelay=50ms — first backoff is 200ms, then capped to 50ms.
	c := stt.New(srv.URL, stt.WithRetryWithMaxDelay(3, 200*time.Millisecond, 50*time.Millisecond))

	start := time.Now()
	_, _ = c.TranscribeReader(context.Background(), strings.NewReader("audio"), "test.wav")
	elapsed := time.Since(start)

	// 3 attempts: backoff after 1st = 200ms, after 2nd = 50ms (capped). Total = 250ms.
	if elapsed > 600*time.Millisecond {
		t.Errorf("elapsed = %v, expected ~250ms with maxDelay=50ms cap", elapsed)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}
