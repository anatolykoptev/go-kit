package llm_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestAPIError_NonRetryable400(t *testing.T) {
	err := &llm.APIError{StatusCode: 400, Body: "bad request", Retryable: false}
	if err.Retryable {
		t.Fatal("expected Retryable=false for 400")
	}
}

func TestAPIError_Retryable429(t *testing.T) {
	err := &llm.APIError{StatusCode: 429, Body: "rate limited", Retryable: true}
	if !err.Retryable {
		t.Fatal("expected Retryable=true for 429")
	}
}

func TestAPIError_Retryable500(t *testing.T) {
	err := &llm.APIError{StatusCode: 500, Body: "internal error", Retryable: true}
	if !err.Retryable {
		t.Fatal("expected Retryable=true for 500")
	}
}

func TestAPIError_TypeParsedFromJSON(t *testing.T) {
	body := `{"error":{"type":"rate_limit_error","message":"too many requests"}}`
	err := &llm.APIError{StatusCode: 429, Body: body, Type: "rate_limit_error", Retryable: true}
	if err.Type != "rate_limit_error" {
		t.Fatalf("expected Type=rate_limit_error, got %q", err.Type)
	}
}

func TestAPIError_TypeEmptyForNonJSON(t *testing.T) {
	err := &llm.APIError{StatusCode: 400, Body: "plain text error"}
	if err.Type != "" {
		t.Fatalf("expected empty Type for non-JSON body, got %q", err.Type)
	}
}

func TestAPIError_ErrorStringWithType(t *testing.T) {
	err := &llm.APIError{StatusCode: 429, Body: "too many", Type: "rate_limit_error"}
	want := "llm: HTTP 429 (rate_limit_error): too many"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestAPIError_ErrorStringWithoutType(t *testing.T) {
	err := &llm.APIError{StatusCode: 400, Body: "bad request"}
	want := "llm: HTTP 400: bad request"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestAPIError_ErrorsAs(t *testing.T) {
	original := &llm.APIError{StatusCode: 502, Body: "bad gateway", Retryable: true}
	wrapped := fmt.Errorf("call failed: %w", original)

	var apiErr *llm.APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As should find APIError in wrapped error")
	}
	if apiErr.StatusCode != 502 {
		t.Fatalf("expected StatusCode=502, got %d", apiErr.StatusCode)
	}
	if !apiErr.Retryable {
		t.Fatal("expected Retryable=true")
	}
}

func TestAPIError_ErrorsAs_NotPresent(t *testing.T) {
	err := fmt.Errorf("some other error")
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		t.Fatal("errors.As should not find APIError in unrelated error")
	}
}
