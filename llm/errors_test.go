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

func TestClassifyErrorType(t *testing.T) {
	// wrappedAPIErr wraps an APIError in a fmt.Errorf layer to verify errors.As
	// unwrapping works correctly in ClassifyErrorType.
	wrappedAPIErr := func(e *llm.APIError) error {
		return fmt.Errorf("wrapped: %w", e)
	}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil returns empty",
			err:  nil,
			want: "",
		},
		{
			name: "non-APIError returns unknown",
			err:  wrappedAPIErr(&llm.APIError{StatusCode: 500, Retryable: true}), // control — uses APIError
			want: "transient",
		},
		{
			name: "plain fmt.Errorf returns unknown",
			err:  errors.New("oops"),
			want: "unknown",
		},
		{
			name: "401 returns auth_expiry",
			err:  &llm.APIError{StatusCode: 401},
			want: "auth_expiry",
		},
		{
			name: "403 returns auth_expiry (not client)",
			err:  &llm.APIError{StatusCode: 403},
			want: "auth_expiry",
		},
		{
			name: "403 with quota marker returns dependency_block (not auth_expiry)",
			err:  &llm.APIError{StatusCode: 403, Type: "auth_unavailable"},
			want: "dependency_block",
		},
		{
			name: "429 returns dependency_block (not transient despite Retryable)",
			err:  &llm.APIError{StatusCode: 429, Retryable: true},
			want: "dependency_block",
		},
		{
			name: "503 with quota marker returns dependency_block",
			err:  &llm.APIError{StatusCode: 503, Type: "auth_unavailable", Retryable: true},
			want: "dependency_block",
		},
		{
			name: "bare 503 without quota marker returns transient (not dependency_block)",
			err:  &llm.APIError{StatusCode: 503, Retryable: true},
			want: "transient",
		},
		{
			name: "413 returns context_overflow (not client)",
			err:  &llm.APIError{StatusCode: 413},
			want: "context_overflow",
		},
		{
			// Pins ordering: asFailover (context_overflow) must be checked BEFORE
			// apiErr.Retryable (transient). A hypothetical Retryable 413 must still
			// classify as context_overflow, not transient.
			name: "413 with Retryable=true returns context_overflow (not transient)",
			err:  &llm.APIError{StatusCode: 413, Retryable: true},
			want: "context_overflow",
		},
		{
			name: "400 with context_length_exceeded returns context_overflow (not client)",
			err:  &llm.APIError{StatusCode: 400, Code: "context_length_exceeded"},
			want: "context_overflow",
		},
		{
			name: "400 plain returns client",
			err:  &llm.APIError{StatusCode: 400, Retryable: false},
			want: "client",
		},
		// model_unavailable: 422 status-alone → advances chain (mirrors 413 status-alone trade-off)
		{
			name: "422 bare returns model_unavailable (not client)",
			err:  &llm.APIError{StatusCode: 422, Retryable: false},
			want: "model_unavailable",
		},
		{
			// Real cliproxyapi body when a provider silently swaps the backing model.
			name: "422 with model-not-available body returns model_unavailable",
			err: &llm.APIError{
				StatusCode: 422,
				Body:       `{"detail":{"error":"Model 'Qwen/Qwen3-235B-A22B-Instruct-2507-FP8' is not available","available_models":["MiniMaxAI/MiniMax-M2.7"]}}`,
				Retryable:  false,
			},
			want: "model_unavailable",
		},
		{
			// OpenAI-family 400 with param="model" — model-specific, next model may exist.
			name: "400 with param=model returns model_unavailable (not client)",
			err:  &llm.APIError{StatusCode: 400, Param: "model", Type: "invalid_request_error", Retryable: false},
			want: "model_unavailable",
		},
		{
			// 400 with a model-not-found body marker and param=model — full real shape.
			name: "400 model-not-found body+param=model returns model_unavailable",
			err: &llm.APIError{
				StatusCode: 400,
				Body:       `{"error":{"message":"Model \"gpt-X\" not found. Available: gpt-4","type":"invalid_request_error","param":"model"}}`,
				Type:       "invalid_request_error",
				Param:      "model",
				Retryable:  false,
			},
			want: "model_unavailable",
		},
		{
			// REGRESSION: plain malformed 400 (no model marker) must NOT become model_unavailable.
			// A bad request recurs on every model → chain must abort.
			name: "400 plain malformed returns client (regression — chain-abort preserved)",
			err:  &llm.APIError{StatusCode: 400, Body: `{"error":{"message":"bad json","type":"invalid_request_error"}}`, Retryable: false},
			want: "client",
		},
		{
			name: "500 retryable returns transient",
			err:  &llm.APIError{StatusCode: 500, Retryable: true},
			want: "transient",
		},
		{
			name: "502 retryable returns transient",
			err:  &llm.APIError{StatusCode: 502, Retryable: true},
			want: "transient",
		},
		{
			name: "wrapped APIError 401 unwraps correctly",
			err:  fmt.Errorf("call failed: %w", &llm.APIError{StatusCode: 401}),
			want: "auth_expiry",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := llm.ClassifyErrorType(tc.err)
			if got != tc.want {
				t.Errorf("ClassifyErrorType(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
