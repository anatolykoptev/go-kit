package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// TestStream_ReasoningEffortAllowlist_PerEndpoint verifies that the stream
// path applies the same reasoning_effort allowlist gating as the non-stream
// path (attemptEndpoint). Before the fix, Stream() built the per-endpoint
// request inline without calling prepareEndpointRequest, so an unsupported
// model received reasoning_effort and could 400.
func TestStream_ReasoningEffortAllowlist_PerEndpoint(t *testing.T) {
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

	// Override srv3 to return a streaming success.
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
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunk := `{"choices":[{"delta":{"content":"ok"}}]}`
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
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

	sr, err := c.Stream(context.Background(), []llm.Message{
		{Role: "user", Content: "test"},
	}, llm.WithReasoningEffort("none"))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()

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
