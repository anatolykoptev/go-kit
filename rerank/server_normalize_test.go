package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithServerNormalize_SigmoidSendsField(t *testing.T) {
	var gotNormalize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cohereRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotNormalize = req.Normalize
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL,
		WithModel("bge"),
		WithTimeout(time.Second),
		WithServerNormalize(ServerNormalizeSigmoid),
	)
	_, _ = c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})

	if gotNormalize != "sigmoid" {
		t.Errorf("normalize field: got %q want %q", gotNormalize, "sigmoid")
	}
}

func TestWithServerNormalize_DefaultEmptyOmitsField(t *testing.T) {
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		rawBody = append(rawBody, buf[:n]...)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{{Index: 0, RelevanceScore: 0.5}}})
	}))
	defer srv.Close()

	// Default client — no WithServerNormalize.
	c := NewClient(srv.URL, WithModel("bge"), WithTimeout(time.Second))
	_, _ = c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})

	// "normalize" key must be absent from the JSON body.
	var parsed map[string]interface{}
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if _, found := parsed["normalize"]; found {
		t.Errorf("normalize key present in request body when not set (breaks Cohere compat)")
	}
}

func TestServerNormalize_NoneOmitsField(t *testing.T) {
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		rawBody = append(rawBody, buf[:n]...)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{{Index: 0, RelevanceScore: 0.5}}})
	}))
	defer srv.Close()

	// Explicitly setting ServerNormalizeNone should also omit the field.
	c := NewClient(srv.URL,
		WithModel("bge"),
		WithTimeout(time.Second),
		WithServerNormalize(ServerNormalizeNone),
	)
	_, _ = c.RerankWithResult(context.Background(), "q", []Doc{{ID: "a", Text: "x"}})

	var parsed map[string]interface{}
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if _, found := parsed["normalize"]; found {
		t.Errorf("normalize key present when ServerNormalizeNone set (should be omitted)")
	}
}
