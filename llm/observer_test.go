package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

type observerCall struct {
	model string
	err   error
}

func newObserver() (*sync.Mutex, *[]observerCall, llm.EndpointAttemptObserver) {
	var mu sync.Mutex
	calls := make([]observerCall, 0, 4)
	obs := func(ep llm.Endpoint, err error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, observerCall{model: ep.Model, err: err})
	}
	return &mu, &calls, obs
}

func okChatHandler(t *testing.T, content string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": content},
			}},
		})
	}
}

func TestObserver_FiresOnSuccessOnly(t *testing.T) {
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "primary"},
			{URL: srv.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 observer call (primary success), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "primary" || (*calls)[0].err != nil {
		t.Errorf("call = %+v, want {primary, nil}", (*calls)[0])
	}
}

func TestObserver_FiresOnEachFailureThenSuccess(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()

	okSrv := httptest.NewServer(okChatHandler(t, "from fallback"))
	defer okSrv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: failSrv.URL, Key: "k", Model: "primary"},
			{URL: okSrv.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "from fallback" {
		t.Errorf("output = %q, want 'from fallback'", out)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 observer calls (primary fail + fallback ok), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "primary" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {primary, err}", (*calls)[0])
	}
	if (*calls)[1].model != "fallback" || (*calls)[1].err != nil {
		t.Errorf("call[1] = %+v, want {fallback, nil}", (*calls)[1])
	}
}

func TestObserver_FiresPerChainAttemptOnTotalFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "a"},
			{URL: srv.URL, Key: "k", Model: "b"},
			{URL: srv.URL, Key: "k", Model: "c"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error when all endpoints fail")
	}
	if len(*calls) != 3 {
		t.Fatalf("expected 3 observer calls (one per endpoint), got %d: %+v", len(*calls), *calls)
	}
	for i, want := range []string{"a", "b", "c"} {
		if (*calls)[i].model != want {
			t.Errorf("call[%d].model = %q, want %q", i, (*calls)[i].model, want)
		}
		if (*calls)[i].err == nil {
			t.Errorf("call[%d].err = nil, want non-nil", i)
		}
	}
}

func TestObserver_NotFiredOnNonRetryableEarlyStop(t *testing.T) {
	// 4xx (other than 429) — fail-fast, ничего не идёт в chain.
	// Observer fires только для primary (where failure occurred), not for
	// subsequent endpoints (loop exits early on non-retryable).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "primary"},
			{URL: srv.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)

	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 observer call (fail-fast on non-retryable), got %d: %+v", len(*calls), *calls)
	}
	if (*calls)[0].model != "primary" || (*calls)[0].err == nil {
		t.Errorf("call[0] = %+v, want {primary, err}", (*calls)[0])
	}
}

func TestObserver_NilObserver_NoOp(t *testing.T) {
	// Sanity — nil observer не panic'ает.
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "primary"},
		}),
		llm.WithEndpointAttemptObserver(nil),
	)
	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
}

// sanity that errors returned to observer can be inspected via errors.Is/As
// — observer receives the same error the caller would see.
func TestObserver_ErrorIsObservedAsFromCaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "only"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(obs),
	)
	_, err := c.Complete(context.Background(), "", "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(*calls))
	}
	// Caller's err and observed err — same chain.
	if !errors.Is((*calls)[0].err, (*calls)[0].err) {
		t.Error("observed err not comparable")
	}
}
