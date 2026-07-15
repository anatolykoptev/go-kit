package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// TestServedBy_PrimarySuccess: a chain whose primary returns 200 sets
// ChatResponse.ServedBy to the primary's model id.
func TestServedBy_PrimarySuccess(t *testing.T) {
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "primary"},
			{URL: srv.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
	)

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ServedBy != "primary" {
		t.Errorf("ServedBy = %q, want primary", resp.ServedBy)
	}
}

// TestServedBy_FallbackSuccess: when the primary fails and the chain falls over
// to the second model, ServedBy carries the model that actually returned 200.
func TestServedBy_FallbackSuccess(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-fallback"))
	defer ok.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
	)

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from-fallback" {
		t.Fatalf("Content = %q, want from-fallback", resp.Content)
	}
	if resp.ServedBy != "fallback" {
		t.Errorf("ServedBy = %q, want fallback (primary fell over)", resp.ServedBy)
	}
}

// TestServedBy_AllCooledRaceGuard_StillSet: the never-fail-closed last-resort
// path (every model cooled → force-attempt the primary) must ALSO set ServedBy
// when that last-resort attempt succeeds. attemptEndpoint is the single authority
// for "try one endpoint", so both the loop and the race guard set ServedBy.
func TestServedBy_AllCooledRaceGuard_StillSet(t *testing.T) {
	// A handler that fails once (to cool the model) then succeeds, so the
	// force-attempt-primary path returns a 200 we can attribute.
	var hits int
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			quotaHandler("")(w, &http.Request{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer flaky.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: flaky.URL, Key: "k", Model: "only"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)

	// Call 1: 429 cools the sole model.
	if _, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("call 1: expected the upstream 429 error")
	}
	// Call 2: the only model is cooled → race-guard force-attempts the primary,
	// which now returns 200. ServedBy must be populated on that path too.
	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if resp.ServedBy != "only" {
		t.Errorf("ServedBy = %q, want only (last-resort race guard must still set it)", resp.ServedBy)
	}
}

// TestServedBy_SingleEndpointPath_Empty: the single-endpoint (no WithEndpoints)
// path is not a chain — there is nothing to attribute, so ServedBy stays "".
func TestServedBy_SingleEndpointPath_Empty(t *testing.T) {
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "solo", llm.WithMaxRetries(1))

	resp, err := c.Chat(context.Background(), []llm.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ServedBy != "" {
		t.Errorf("ServedBy = %q, want empty (single-endpoint path is not a chain)", resp.ServedBy)
	}
}
