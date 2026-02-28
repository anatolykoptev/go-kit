package retry_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/retry"
)

func TestDo_Success(t *testing.T) {
	calls := 0
	result, err := retry.Do(context.Background(), retry.Options{MaxAttempts: 3}, func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_RetryThenSucceed(t *testing.T) {
	calls := 0
	result, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("not yet")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_AllFail(t *testing.T) {
	calls := 0
	_, err := retry.Do(context.Background(), retry.Options{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
	}, func() (string, error) {
		calls++
		return "", errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := retry.Do(ctx, retry.Options{MaxAttempts: 5}, func() (string, error) {
		return "", errors.New("should not reach")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestDo_ContextCancelledBetweenRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retry.Do(ctx, retry.Options{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
	}, func() (string, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return "", errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestHTTP_RetryOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	}))
	defer srv.Close()

	resp, err := retry.HTTP(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (*http.Response, error) {
		return http.Get(srv.URL) //nolint:noctx // test only
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestHTTP_Non5xxNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	resp, err := retry.HTTP(context.Background(), retry.Options{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func() (*http.Response, error) {
		return http.Get(srv.URL) //nolint:noctx // test only
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (404 not retried)", calls)
	}
}

func TestOptions_Defaults(t *testing.T) {
	result, err := retry.Do(context.Background(), retry.Options{}, func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
}
