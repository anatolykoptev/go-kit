package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestWithRetryHugeMaxAttemptsRejected verifies that an absurdly large maxAttempts
// (1_000_000) is rejected at construction time rather than looping 1M times against
// a failing endpoint. The guard must fire in New/WithRetry (panic or error), OR the
// attempt count must be clamped to <= 100. In all cases the server must NOT be hit
// 1M times.
func TestWithRetryHugeMaxAttemptsRejected(t *testing.T) {
	t.Parallel()
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	// Construction must reject (panic) the unbounded value.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected New to panic on WithRetry(1_000_000, 1s), but it did not")
			}
		}()
		_ = stt.New(srv.URL, stt.WithRetry(1_000_000, time.Second))
	}()

	// If we got here via panic, no client was built; nothing to call.
	// The server must not have been hit 1M times. Since construction panicked,
	// count must be 0.
	if got := count.Load(); got > 100 {
		t.Errorf("server was hit %d times; expected rejection before any loop", got)
	}
}

// TestWithRetryZeroMaxAttemptsRejected verifies that maxAttempts=0 is rejected.
func TestWithRetryZeroMaxAttemptsRejected(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected New to panic on WithRetry(0, 1s), but it did not")
		}
	}()
	_ = stt.New(srv.URL, stt.WithRetry(0, time.Second))
}

// TestWithRetryNegativeMaxAttemptsRejected verifies that a negative maxAttempts is rejected.
func TestWithRetryNegativeMaxAttemptsRejected(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected New to panic on WithRetry(-1, 1s), but it did not")
		}
	}()
	_ = stt.New(srv.URL, stt.WithRetry(-1, time.Second))
}

// TestWithRetryNegativeBaseDelayRejected verifies that a negative baseDelay is rejected.
func TestWithRetryNegativeBaseDelayRejected(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected New to panic on WithRetry(3, -1s), but it did not")
		}
	}()
	_ = stt.New(srv.URL, stt.WithRetry(3, -time.Second))
}

// TestWithRetryZeroBaseDelayRejected verifies that a zero baseDelay is rejected.
func TestWithRetryZeroBaseDelayRejected(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected New to panic on WithRetry(3, 0), but it did not")
		}
	}()
	_ = stt.New(srv.URL, stt.WithRetry(3, 0))
}

// TestWithRetryValidBoundsAccepted verifies that valid boundary values
// (maxAttempts=1 and maxAttempts=100) are accepted at construction time
// without panic.
func TestWithRetryValidBoundsAccepted(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	for _, maxAttempts := range []int{1, 100} { //nolint:mnd
		maxAttempts := maxAttempts
		t.Run("maxAttempts="+itoa(maxAttempts), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("New panicked on valid WithRetry(%d, 1ms): %v", maxAttempts, r)
				}
			}()
			_ = stt.New(srv.URL, stt.WithRetry(maxAttempts, time.Millisecond))
		})
	}
}

// TestWithRetryValidLoopsExactlyMaxAttempts verifies that a small valid
// maxAttempts loops exactly that many times against a failing endpoint.
func TestWithRetryValidLoopsExactlyMaxAttempts(t *testing.T) {
	t.Parallel()
	const maxAttempts = 5
	var count atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down"))
	}))
	defer srv.Close()

	client := stt.New(srv.URL, stt.WithRetry(maxAttempts, time.Millisecond))
	_, _ = client.TranscribeReader(context.Background(), emptyAudio(), "test.wav")

	if got := count.Load(); int(got) != maxAttempts {
		t.Errorf("request count = %d, want %d", got, maxAttempts)
	}
}

// TestWithRetryAboveUpperBoundRejected verifies that maxAttempts=101 (just above
// the 100 ceiling) is rejected.
func TestWithRetryAboveUpperBoundRejected(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected New to panic on WithRetry(101, 1s), but it did not")
		}
	}()
	_ = stt.New(srv.URL, stt.WithRetry(101, time.Second))
}

// itoa is a tiny helper to avoid importing strconv in a test file that uses
// t.Run subtest names.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte //nolint:mnd
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10) //nolint:mnd
		n /= 10                   //nolint:mnd
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
