package stt_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestCircuitBreakerHalfOpenSingleProbe verifies that in half-open state, exactly
// ONE request (the probe) reaches the server when N concurrent requests are fired.
// All other requests must be rejected with a "circuit breaker open" error.
// The server blocks on a release channel so all goroutines call allow() before any
// completes, guaranteeing the half-open stampede is exercised. Run under -race.
func TestCircuitBreakerHalfOpenSingleProbe(t *testing.T) {
	t.Parallel()

	var serverHits atomic.Int32
	// phase: 0 = initial (no blocking), 1 = concurrent (block on releaseCh)
	var phase atomic.Int32
	releaseCh := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverHits.Add(1)
		if phase.Load() == 1 {
			<-releaseCh // block until all goroutines have called allow()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	const cooldown = 30 * time.Millisecond
	// maxFails=1: a single failure opens the CB immediately.
	client := stt.New(srv.URL,
		stt.WithRetry(1, time.Millisecond),
		stt.WithCircuitBreaker(1, cooldown),
	)

	// Phase 0: one failure opens the CB (maxFails=1, server doesn't block in phase 0).
	_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
	if got := serverHits.Load(); got != 1 {
		t.Fatalf("expected 1 server hit from initial failure, got %d", got)
	}

	// Wait past cooldown → CB is open but cooldown has elapsed (half-open on next allow()).
	time.Sleep(cooldown + 10*time.Millisecond)

	// Phase 1: fire N concurrent requests. The server blocks on releaseCh so
	// all goroutines have time to call allow() before any returns.
	phase.Store(1)
	const N = 10
	var wg sync.WaitGroup
	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
		}()
	}

	// Give all goroutines time to call allow() (they'll either hit the server
	// and block on releaseCh, or be rejected immediately by the CB).
	time.Sleep(100 * time.Millisecond)
	close(releaseCh)
	wg.Wait()

	// Exactly ONE probe should have reached the server during half-open.
	// Total server hits: 1 (initial) + 1 (probe) = 2.
	got := serverHits.Load()
	if got != 2 { //nolint:mnd
		t.Errorf("server hit count = %d, want 2 (1 initial + 1 half-open probe)", got)
	}
}

// TestCircuitBreakerCheckedPerAttempt verifies that cb.allow() is checked before
// EACH retry attempt, not just once before the loop. With maxFails=1, the first
// attempt's failure opens the CB; subsequent attempts must be blocked (fn not called).
func TestCircuitBreakerCheckedPerAttempt(t *testing.T) {
	t.Parallel()

	var fnCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fnCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	const maxAttempts = 3
	const cooldown = 10 * time.Second // long enough to stay open during retries
	client := stt.New(srv.URL,
		stt.WithRetry(maxAttempts, 50*time.Millisecond),
		stt.WithCircuitBreaker(1, cooldown), // maxFails=1: one failure opens CB
	)

	_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")

	// The first attempt fails and opens the CB (failures=1 >= maxFails=1).
	// With per-attempt checking, attempts 2 and 3 are blocked by the CB.
	// So fn should be called exactly 1 time.
	if got := fnCalls.Load(); got != 1 {
		t.Errorf("fn called %d times, want 1 (CB should block subsequent attempts)", got)
	}

	// The error should be "circuit breaker open" since the CB blocked attempts 2+.
	var sttErr *stt.Error
	if !errors.As(err, &sttErr) || sttErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected circuit breaker open error, got: %v", err)
	}
}

// TestCircuitBreakerCheckedPerAttempt_MidRetry verifies that a CB opened by a
// concurrent request mid-retry blocks subsequent retry attempts.
func TestCircuitBreakerCheckedPerAttempt_MidRetry(t *testing.T) {
	t.Parallel()

	var fnCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fnCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	// maxFails=2: need 2 failures to open. The retry loop's first attempt is 1,
	// a concurrent request provides the 2nd failure to open the CB.
	const maxAttempts = 3
	const cooldown = 10 * time.Second
	client := stt.New(srv.URL,
		stt.WithRetry(maxAttempts, 200*time.Millisecond), // long backoff for concurrent request to sneak in
		stt.WithCircuitBreaker(2, cooldown),              //nolint:mnd
	)

	// Start the retry loop.
	done := make(chan error, 1)
	go func() {
		_, err := client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")
		done <- err
	}()

	// Wait for the first attempt to complete (fn called once).
	deadline := time.After(2 * time.Second)
	for fnCalls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first attempt")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Fire a concurrent request that fails, pushing failures to 2 → CB opens.
	_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")

	// Wait for the retry loop to finish.
	select {
	case err := <-done:
		// The retry loop should have been blocked by the CB on attempts 2+.
		// fnCalls should be 2: 1 from retry attempt 1 + 1 from concurrent request.
		if got := fnCalls.Load(); got != 2 { //nolint:mnd
			t.Errorf("fn called %d times, want 2 (1 retry + 1 concurrent; CB should block retry attempts 2+)", got)
		}
		var sttErr *stt.Error
		if !errors.As(err, &sttErr) || sttErr.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected circuit breaker open error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for retry loop to finish")
	}
}
