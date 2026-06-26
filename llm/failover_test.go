package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// statusBodyHandler responds with a fixed HTTP status and raw JSON body,
// simulating a provider error response (Groq 413 TPM, OpenAI context overflow).
func statusBodyHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// Groq's real 413 shape: per-minute token budget exceeded by a single request.
const groq413Body = `{"error":{"message":"Request too large for model llama-3.3-70b-versatile in organization org_x service tier on_demand on tokens per minute (TPM): Limit 12000, Requested 24187, please reduce your message size and try again.","type":"tokens","code":"rate_limit_exceeded"}}`

// OpenAI-family context-window overflow shape (HTTP 400).
const contextLenBody = `{"error":{"message":"This model's maximum context length is 8192 tokens.","type":"invalid_request_error","code":"context_length_exceeded"}}`

// A genuinely malformed request — recurs identically on every model, so the
// chain must NOT advance on it.
const plainBadRequestBody = `{"error":{"message":"messages must be a non-empty array","type":"invalid_request_error","code":"invalid_request_error"}}`

// Real cliproxyapi 422 body when a provider silently swaps the backing model.
// The proxy still lists the alias in /v1/models (health-filter passes it), but
// a real call returns 422 with this shape.
const cliproxyapi422Body = `{"detail":{"error":"Model 'Qwen/Qwen3-235B-A22B-Instruct-2507-FP8' is not available","available_models":["MiniMaxAI/MiniMax-M2.7"]}}`

// OpenAI-family 400 "model not found" shape (param=model).
const modelNotFoundBody = `{"error":{"message":"Model \"gpt-X\" not found. Available: gpt-4","type":"invalid_request_error","param":"model"}}`

// TestChain_AdvancesOn413TooLarge is the core fix: a 413 "request too large for
// this model" must advance the model-fallback chain to the next (larger-budget)
// model instead of aborting. Pre-fix this FAILS — 413 is non-retryable and the
// endpoint loop returns immediately.
func TestChain_AdvancesOn413TooLarge(t *testing.T) {
	tooLarge := httptest.NewServer(statusBodyHandler(http.StatusRequestEntityTooLarge, groq413Body))
	defer tooLarge.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-bigger-model"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: tooLarge.URL, Key: "k", Model: "small-tpm-model"},
			{URL: ok.URL, Key: "k", Model: "big-context-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected chain to advance past 413, got error: %v", err)
	}
	if out != "from-bigger-model" {
		t.Errorf("output = %q, want from-bigger-model", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (413 then ok), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "small-tpm-model" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {small-tpm-model, err}", (*calls)[0])
	}
	// The observed error must be the 413 APIError so a metrics observer can
	// label the failover cause.
	var ae *llm.APIError
	if !errors.As((*calls)[0].err, &ae) || ae.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("call[0].err = %v, want APIError 413", (*calls)[0].err)
	}
	if (*calls)[1].model != "big-context-model" || (*calls)[1].err != nil {
		t.Errorf("call[1] = %+v, want {big-context-model, nil}", (*calls)[1])
	}
}

// TestChain_AdvancesOnContextLengthExceeded: a 400 with code
// context_length_exceeded is the OpenAI-family equivalent of "too large" and
// must also advance the chain.
func TestChain_AdvancesOnContextLengthExceeded(t *testing.T) {
	overflow := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, contextLenBody))
	defer overflow.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-long-ctx"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: overflow.URL, Key: "k", Model: "short-ctx"},
			{URL: ok.URL, Key: "k", Model: "long-ctx"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected advance past context_length_exceeded, got: %v", err)
	}
	if out != "from-long-ctx" {
		t.Errorf("output = %q, want from-long-ctx", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %+v", len(*calls), *calls)
	}
}

// TestChain_PlainBadRequestStillAborts guards against over-broadening: a plain
// 400 (malformed request) recurs identically on every model, so the chain must
// abort, not advance.
func TestChain_PlainBadRequestStillAborts(t *testing.T) {
	bad := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, plainBadRequestBody))
	defer bad.Close()
	ok := httptest.NewServer(okChatHandler(t, "should-not-reach"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: bad.URL, Key: "k", Model: "first"},
			{URL: ok.URL, Key: "k", Model: "second"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected plain 400 to abort the chain (malformed request recurs on every model)")
	}
	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 call (abort, no advance), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "first" {
		t.Errorf("call[0].model = %q, want first", (*calls)[0].model)
	}
}

// TestSingleEndpoint_413StillErrors: with no chain to advance to, a 413 must
// surface as the caller's error (unchanged single-endpoint behaviour).
func TestSingleEndpoint_413StillErrors(t *testing.T) {
	tooLarge := httptest.NewServer(statusBodyHandler(http.StatusRequestEntityTooLarge, groq413Body))
	defer tooLarge.Close()

	c := llm.NewClient(tooLarge.URL, "k", "m", llm.WithMaxRetries(2))
	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("single endpoint 413 must surface the error (no chain to advance)")
	}
	var ae *llm.APIError
	if !errors.As(err, &ae) || ae.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("err = %v, want APIError 413", err)
	}
}

// TestSingleEndpoint_413NotRetried: a 413 is non-retryable on the same endpoint
// (the identical request will recur), so it must be sent exactly once even with
// MaxRetries>1. Locks in "failover, not retry".
func TestSingleEndpoint_413NotRetried(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(groq413Body))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(3))
	_, _ = c.Complete(context.Background(), "", "test")
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("413 must not be retried on the same endpoint: server hit %d times, want 1", hits)
	}
}

// TestAPIError_CodeParsedFromJSON: the error.code field is parsed onto APIError
// so failover classification can distinguish context_length_exceeded from a
// generic 400.
func TestAPIError_CodeParsedFromJSON(t *testing.T) {
	srv := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, contextLenBody))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(1))
	_, err := c.Complete(context.Background(), "", "test")
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if ae.Code != "context_length_exceeded" {
		t.Errorf("Code = %q, want context_length_exceeded", ae.Code)
	}
	if ae.Type != "invalid_request_error" {
		t.Errorf("Type = %q, want invalid_request_error", ae.Type)
	}
}

// TestChain_StreamAdvancesOn413 pins the same fix for the streaming endpoint
// chain (Stream/StreamExtract): a 413 on the first model must advance to the
// next, and the observer must fire per endpoint.
func TestChain_StreamAdvancesOn413(t *testing.T) {
	tooLarge := httptest.NewServer(statusBodyHandler(http.StatusRequestEntityTooLarge, groq413Body))
	defer tooLarge.Close()
	okStream := httptest.NewServer(sseHandler([]string{`{"choices":[{"delta":{"content":"hello"}}]}`}))
	defer okStream.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: tooLarge.URL, Key: "k", Model: "small-tpm-model"},
			{URL: okStream.URL, Key: "k", Model: "big-context-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	sr, err := c.Stream(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("expected stream chain to advance past 413, got: %v", err)
	}
	defer sr.Close()
	var got string
	for {
		chunk, ok := sr.Next()
		if !ok {
			break
		}
		got += chunk.Delta
	}
	if err := sr.Err(); err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if got != "hello" {
		t.Errorf("delta = %q, want hello", got)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (413 then ok), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "small-tpm-model" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {small-tpm-model, err}", (*calls)[0])
	}
	if (*calls)[1].model != "big-context-model" || (*calls)[1].err != nil {
		t.Errorf("call[1] = %+v, want {big-context-model, nil}", (*calls)[1])
	}
}

// TestAPIError_EmptyTypeCodeForJSONWithoutErrorObject guards the newAPIError
// parse change: valid JSON without an "error" object yields empty Type/Code.
func TestAPIError_EmptyTypeCodeForJSONWithoutErrorObject(t *testing.T) {
	srv := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, `{}`))
	defer srv.Close()
	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(1))
	_, err := c.Complete(context.Background(), "", "test")
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if ae.Type != "" || ae.Code != "" {
		t.Errorf("Type=%q Code=%q, want both empty for body without error object", ae.Type, ae.Code)
	}
}

// TestAPIError_EmptyTypeCodeForNonJSON guards the parse change for non-JSON
// bodies: json.Unmarshal fails, so Type/Code stay empty.
func TestAPIError_EmptyTypeCodeForNonJSON(t *testing.T) {
	srv := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, "plain text error"))
	defer srv.Close()
	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(1))
	_, err := c.Complete(context.Background(), "", "test")
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if ae.Type != "" || ae.Code != "" {
		t.Errorf("Type=%q Code=%q, want both empty for non-JSON body", ae.Type, ae.Code)
	}
}

// TestChain_AdvancesOn422ModelNotAvailable: a 422 "model not available"
// (cliproxyapi shape) must advance the chain to the next model. The alias
// still lists in /v1/models so the health-filter did not drop it; the call
// itself fails with 422. Model-not-found is model-specific (the next model
// differs and may exist), so the chain must advance, not abort.
func TestChain_AdvancesOn422ModelNotAvailable(t *testing.T) {
	dead := httptest.NewServer(statusBodyHandler(http.StatusUnprocessableEntity, cliproxyapi422Body))
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-live-model"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "dead-alias"},
			{URL: ok.URL, Key: "k", Model: "live-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected chain to advance past 422 model-not-available, got error: %v", err)
	}
	if out != "from-live-model" {
		t.Errorf("output = %q, want from-live-model", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (422 then ok), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "dead-alias" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {dead-alias, err}", (*calls)[0])
	}
	var ae *llm.APIError
	if !errors.As((*calls)[0].err, &ae) || ae.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("call[0].err = %v, want APIError 422", (*calls)[0].err)
	}
	if llm.ClassifyErrorType((*calls)[0].err) != "model_unavailable" {
		t.Errorf("ClassifyErrorType for 422 = %q, want model_unavailable", llm.ClassifyErrorType((*calls)[0].err))
	}
	if (*calls)[1].model != "live-model" || (*calls)[1].err != nil {
		t.Errorf("call[1] = %+v, want {live-model, nil}", (*calls)[1])
	}
}

// TestChain_AdvancesOn400ModelNotFound: a 400 with param=model "model not
// found" is model-specific (next model differs, may exist). Chain must advance.
// REGRESSION: plain malformed-400 (no model marker) must still abort.
func TestChain_AdvancesOn400ModelNotFound(t *testing.T) {
	dead := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, modelNotFoundBody))
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-fallback"))
	defer ok.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "missing-model"},
			{URL: ok.URL, Key: "k", Model: "real-model"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected chain to advance past 400 model-not-found, got error: %v", err)
	}
	if out != "from-fallback" {
		t.Errorf("output = %q, want from-fallback", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (400 model-not-found then ok), got %d: %+v", len(*calls), *calls)
	}
	if llm.ClassifyErrorType((*calls)[0].err) != "model_unavailable" {
		t.Errorf("ClassifyErrorType for 400 model-not-found = %q, want model_unavailable", llm.ClassifyErrorType((*calls)[0].err))
	}
}

// TestAPIError_ParamParsedFromJSON: the error.param field is parsed onto
// APIError.Param for 400 "model not found" shapes (OpenAI-family).
func TestAPIError_ParamParsedFromJSON(t *testing.T) {
	srv := httptest.NewServer(statusBodyHandler(http.StatusBadRequest, modelNotFoundBody))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(1))
	_, err := c.Complete(context.Background(), "", "test")
	var ae *llm.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want APIError, got %T: %v", err, err)
	}
	if ae.Param != "model" {
		t.Errorf("Param = %q, want model", ae.Param)
	}
	if ae.Type != "invalid_request_error" {
		t.Errorf("Type = %q, want invalid_request_error", ae.Type)
	}
}
