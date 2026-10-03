package stt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// emptyAudio returns a minimal valid io.Reader representing fake audio data.
func emptyAudio() io.Reader {
	return bytes.NewReader([]byte("fake audio"))
}

func okTranscriptionHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stt.Response{Text: "ok", Language: "ru"})
}

// TestRetryTransient verifies that a transient 503 is retried and succeeds on the 3rd attempt.
func TestRetryTransient(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n < 3 { //nolint:mnd
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("overloaded"))
			return
		}
		okTranscriptionHandler(w, r)
	}))
	defer srv.Close()

	client := stt.New(srv.URL,
		stt.WithRetry(5, time.Millisecond),
	)

	resp, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q, want ok", resp.Text)
	}
	if got := count.Load(); got != 3 {
		t.Errorf("request count = %d, want 3", got)
	}
}

// TestRetryNonTransient verifies that a 400 is not retried (only 1 request made).
func TestRetryNonTransient(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	client := stt.New(srv.URL,
		stt.WithRetry(5, time.Millisecond),
	)

	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := count.Load(); got != 1 {
		t.Errorf("request count = %d, want 1 (no retry for non-transient)", got)
	}
}

// TestCircuitBreakerOpens verifies that after maxFails failures, the CB blocks further requests.
func TestCircuitBreakerOpens(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	const maxFails = 3
	client := stt.New(srv.URL,
		stt.WithRetry(1, time.Millisecond), // 1 attempt = no retries, just records failures
		stt.WithCircuitBreaker(maxFails, 10*time.Second),
	)

	// First maxFails calls should reach the server.
	for range maxFails {
		_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	}

	if got := count.Load(); got != maxFails {
		t.Fatalf("expected %d requests before CB opens, got %d", maxFails, got)
	}

	// Next call must be blocked by CB (no HTTP request).
	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if got := count.Load(); got != maxFails {
		t.Errorf("request count = %d, want %d (CB should have blocked)", got, maxFails)
	}
}

// TestCircuitBreakerResets verifies that after the cooldown, the CB lets requests through.
func TestCircuitBreakerResets(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		// First 3 calls fail, then succeed.
		if n <= 3 { //nolint:mnd
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("down"))
			return
		}
		okTranscriptionHandler(w, r)
	}))
	defer srv.Close()

	const cooldown = 50 * time.Millisecond
	client := stt.New(srv.URL,
		stt.WithRetry(1, time.Millisecond),
		stt.WithCircuitBreaker(3, cooldown), //nolint:mnd
	)

	for range 3 {
		_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	}

	// CB is now open; wait for cooldown.
	time.Sleep(cooldown + 10*time.Millisecond)

	// After cooldown, request should go through (and succeed).
	resp, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err != nil {
		t.Fatalf("expected success after CB reset, got: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q, want ok", resp.Text)
	}
}

// TestRetryContextCancel verifies that cancelling the context during backoff returns ctx.Err().
func TestRetryContextCancel(t *testing.T) {
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Use a long base delay so cancellation happens during backoff.
	client := stt.New(srv.URL,
		stt.WithRetry(5, 500*time.Millisecond),
	)

	done := make(chan error, 1)
	go func() {
		_, err := client.TranscribeReader(ctx, emptyAudio(), "test.wav")
		done <- err
	}()

	// Cancel after first attempt has had time to fail.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after context cancel")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled (or wrapping it)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled call to return")
	}
}
