package llm_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestAPIError_RetryAfter_Seconds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate limit"}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model", llm.WithMaxRetries(1))
	_, err := c.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("expected error from 429")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *llm.APIError, got %T: %v", err, err)
	}
	if ae.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter: got %v, want 30s", ae.RetryAfter)
	}
}

func TestAPIError_RetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().UTC().Add(45 * time.Second).Format(http.TimeFormat)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model", llm.WithMaxRetries(1))
	_, err := c.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *llm.APIError, got %T: %v", err, err)
	}
	// HTTP-date form → relative duration. Allow ±5s drift for test scheduling.
	if ae.RetryAfter < 40*time.Second || ae.RetryAfter > 50*time.Second {
		t.Errorf("RetryAfter: got %v, want ~45s", ae.RetryAfter)
	}
}

func TestAPIError_RetryAfter_Absent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model", llm.WithMaxRetries(1))
	_, err := c.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *llm.APIError, got %T", err)
	}
	if ae.RetryAfter != 0 {
		t.Errorf("RetryAfter: got %v, want 0 when header absent", ae.RetryAfter)
	}
}

func TestAPIError_RetryAfter_Invalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "not-a-number")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model", llm.WithMaxRetries(1))
	_, err := c.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *llm.APIError, got %T", err)
	}
	if ae.RetryAfter != 0 {
		t.Errorf("RetryAfter: got %v, want 0 on invalid header", ae.RetryAfter)
	}
}
