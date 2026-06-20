package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestWithReasoningEffort_Serializes(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"blue"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model")
	_, err := c.Complete(context.Background(), "", "sky color?", llm.WithReasoningEffort("none"))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got["reasoning_effort"] != "none" {
		t.Errorf("reasoning_effort = %v, want %q", got["reasoning_effort"], "none")
	}
}

func TestWithReasoningEffort_OmittedWhenEmpty(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"blue"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "test-model")
	_, err := c.Complete(context.Background(), "", "sky color?")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, exists := got["reasoning_effort"]; exists {
		t.Errorf("reasoning_effort should be absent when not set, got: %v", got["reasoning_effort"])
	}
}

// TestReasoningEffortAllowlist_PerEndpoint verifies that WithReasoningEffortModels
// gates reasoning_effort per-endpoint in a chain:
// - endpoints in the allowlist receive reasoning_effort:"none"
// - endpoints NOT in the allowlist have reasoning_effort stripped (empty)
//
// This test MUST FAIL if the per-endpoint gating block is removed from attemptEndpoint.
func TestReasoningEffortAllowlist_PerEndpoint(t *testing.T) {
	type capture struct {
		reasoningEffort string
		present         bool
	}

	makeCapture := func() (chan capture, *httptest.Server) {
		ch := make(chan capture, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			c := capture{}
			if v, ok := body["reasoning_effort"]; ok {
				c.present = true
				c.reasoningEffort, _ = v.(string)
			}
			select {
			case ch <- c:
			default:
			}
			// Return 500 to make the chain fall through to the next endpoint.
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		return ch, srv
	}

	ch1, srv1 := makeCapture() // supported1
	ch2, srv2 := makeCapture() // unsupported
	ch3, srv3 := makeCapture() // supported2 — returns success
	defer srv1.Close()
	defer srv2.Close()
	defer srv3.Close()

	// Override srv3 to return success (end of chain).
	srv3.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		c := capture{}
		if v, ok := body["reasoning_effort"]; ok {
			c.present = true
			c.reasoningEffort, _ = v.(string)
		}
		select {
		case ch3 <- c:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)) //nolint:errcheck
	})

	const (
		model1 = "supported1-model"
		model2 = "unsupported-model"
		model3 = "supported2-model"
	)

	endpoints := []llm.Endpoint{
		{URL: srv1.URL, Key: "k", Model: model1},
		{URL: srv2.URL, Key: "k", Model: model2},
		{URL: srv3.URL, Key: "k", Model: model3},
	}

	c := llm.NewClient("", "k", model1,
		llm.WithEndpoints(endpoints),
		llm.WithReasoningEffortModels([]string{model1, model3}),
	)

	_, err := c.Complete(context.Background(), "", "test", llm.WithReasoningEffort("none"))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	cap1 := <-ch1
	cap2 := <-ch2
	cap3 := <-ch3

	// supported1 MUST receive reasoning_effort:"none"
	if !cap1.present || cap1.reasoningEffort != "none" {
		t.Errorf("endpoint1 (supported): reasoning_effort=%q present=%v, want present=true value=%q",
			cap1.reasoningEffort, cap1.present, "none")
	}
	// unsupported MUST NOT receive reasoning_effort
	if cap2.present {
		t.Errorf("endpoint2 (unsupported): reasoning_effort present (value=%q), want absent",
			cap2.reasoningEffort)
	}
	// supported2 MUST receive reasoning_effort:"none"
	if !cap3.present || cap3.reasoningEffort != "none" {
		t.Errorf("endpoint3 (supported): reasoning_effort=%q present=%v, want present=true value=%q",
			cap3.reasoningEffort, cap3.present, "none")
	}
}
