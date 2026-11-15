package stt_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestRetryMaxAttemptsOne(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	client := stt.New(srv.URL, stt.WithRetry(1, time.Millisecond))
	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := count.Load(); got != 1 {
		t.Errorf("request count = %d, want 1 (no retry when maxAttempts=1)", got)
	}
}

func TestRetryWithNilCircuitBreaker(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("down"))
			return
		}
		okTranscriptionHandler(w, r)
	}))
	defer srv.Close()

	// WithRetry only, no WithCircuitBreaker — must not panic.
	client := stt.New(srv.URL, stt.WithRetry(3, time.Millisecond))
	resp, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q, want ok", resp.Text)
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	const cooldown = 30 * time.Millisecond
	// maxFails=1: a single failure opens the CB; after cooldown one probe failure re-opens it.
	client := stt.New(srv.URL,
		stt.WithRetry(1, time.Millisecond),
		stt.WithCircuitBreaker(1, cooldown),
	)

	// First failure → CB opens (maxFails=1).
	_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if got := count.Load(); got != 1 {
		t.Fatalf("expected 1 request before open, got %d", got)
	}

	// CB open — next request blocked without hitting server.
	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	var sttErr *stt.Error
	if !errors.As(err, &sttErr) || sttErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected CB-open error, got: %v", err)
	}
	if got := count.Load(); got != 1 {
		t.Errorf("request leaked through open CB: count = %d", got)
	}

	// Wait for cooldown then send a probe — it fails → CB re-opens.
	time.Sleep(cooldown + 10*time.Millisecond)
	_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav") // half-open probe fails
	if got := count.Load(); got != 2 {
		t.Errorf("expected 2 total requests after half-open probe, got %d", got)
	}

	// CB is open again — blocked.
	_, err = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if err == nil {
		t.Fatal("expected CB-open error after re-open")
	}
	if got := count.Load(); got != 2 {
		t.Errorf("request leaked through re-opened CB: count = %d", got)
	}
}

func TestCircuitBreakerConcurrent(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	client := stt.New(srv.URL,
		stt.WithRetry(1, time.Millisecond),
		stt.WithCircuitBreaker(5, 10*time.Second),
	)

	done := make(chan struct{})
	for range 10 {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
		}()
	}
	for range 10 {
		<-done
	}
	// If we reach here without race/panic, the test passes.
}

func TestRetryExponentialBackoffCapped(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	// baseDelay=1ms, maxDelay=30s (internal). 3 attempts = 2 backoffs.
	// Without capping: 1ms + 2ms = 3ms. With a 1s cap it's the same here.
	// Use large baseDelay to verify capping doesn't allow exceeding maxDelay.
	// Instead verify: 5 attempts with baseDelay=5ms finish in well under 30s.
	const attempts = 5
	start := time.Now()
	client := stt.New(srv.URL, stt.WithRetry(attempts, 5*time.Millisecond))
	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error")
	}
	if got := count.Load(); got != attempts {
		t.Errorf("request count = %d, want %d", got, attempts)
	}
	// 5 attempts with 5ms base: total backoff 5+10+20+40 = 75ms max (uncapped).
	// With 30s cap that's irrelevant. Just ensure we finish well under 1s.
	if elapsed > time.Second {
		t.Errorf("elapsed %v > 1s — backoff not working correctly", elapsed)
	}
}
