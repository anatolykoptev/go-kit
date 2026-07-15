package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func captureFormatServer(t *testing.T, format *map[string]any, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResponseFormat map[string]any `json:"response_format,omitempty"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*format = req.ResponseFormat
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "{}"},
			}},
		})
	}))
}

func TestWithJSONMode_SendsJSONObject(t *testing.T) {
	var got map[string]any
	var mu sync.Mutex
	srv := captureFormatServer(t, &got, &mu)
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m")
	_, err := c.Complete(context.Background(), "", "test", llm.WithJSONMode())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("response_format not sent")
	}
	if got["type"] != "json_object" {
		t.Errorf("type = %v, want json_object", got["type"])
	}
}

func TestWithResponseFormat_PassesThroughRaw(t *testing.T) {
	var got map[string]any
	var mu sync.Mutex
	srv := captureFormatServer(t, &got, &mu)
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k", "m")
	custom := map[string]any{"type": "custom_format", "extra": "value"}
	_, err := c.Complete(context.Background(), "", "test", llm.WithResponseFormat(custom))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got["type"] != "custom_format" || got["extra"] != "value" {
		t.Errorf("got = %v, want custom_format+extra", got)
	}
}
