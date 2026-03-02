package llm_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestExtractOneOf_EmptyVariants(t *testing.T) {
	srv := newTestServer(t, chatHandler(`{"result":{"action":"x"}}`, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	_, err := c.ExtractOneOf(ctx(t), msgs("test"), nil, llm.WithExtractRetries(1))
	if err == nil {
		t.Fatal("expected error for empty variants")
	}
}

func TestExtractOneOf_EmptyStruct(t *testing.T) {
	type NoFields struct{}
	body := mj(t, map[string]any{"result": map[string]any{"action": "empty"}})
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("test"), []llm.VariantDef{llm.Variant("empty", NoFields{})})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(*NoFields); !ok {
		t.Fatalf("got %T, want *NoFields", result)
	}
}

func TestExtractOneOf_NestedStruct(t *testing.T) {
	type Inner struct{ Value int `json:"value"` }
	type Outer struct{ Inner Inner `json:"inner"` }
	body := mj(t, map[string]any{"result": map[string]any{"action": "n", "inner": map[string]any{"value": 42}}})
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{llm.Variant("n", Outer{})})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.(*Outer).Inner.Value != 42 {
		t.Errorf("Inner.Value = %d, want 42", result.(*Outer).Inner.Value)
	}
}

func TestExtractOneOf_SliceAndMap(t *testing.T) {
	type C struct {
		Tags []string          `json:"tags"`
		Meta map[string]string `json:"meta"`
	}
	body := mj(t, map[string]any{"result": map[string]any{"action": "c", "tags": []string{"a", "b"}, "meta": map[string]string{"k": "v"}}})
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{llm.Variant("c", C{})})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cx := result.(*C)
	if len(cx.Tags) != 2 || cx.Meta["k"] != "v" {
		t.Errorf("got tags=%v meta=%v", cx.Tags, cx.Meta)
	}
}

func TestExtractOneOf_MalformedJSON(t *testing.T) {
	cases := []struct{ name, body string }{
		{"missing_wrapper", `{"action":"search","query":"q"}`},
		{"null_result", `{"result":null}`},
		{"action_wrong_type", `{"result":{"action":123}}`},
		{"empty_json", `{}`},
		{"invalid_json", `not json`},
		{"result_is_array", `{"result":[1,2,3]}`},
		{"result_is_string", `{"result":"oops"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, chatHandler(tc.body, nil, "stop"))
			c := llm.NewClient(srv.URL, "key", "model")
			_, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{
				llm.Variant("search", SearchAction{}),
			}, llm.WithExtractRetries(1))
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestExtractOneOf_EmptyAction(t *testing.T) {
	body := mj(t, map[string]any{"result": map[string]any{"action": ""}})
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	_, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{
		llm.Variant("search", SearchAction{}),
	}, llm.WithExtractRetries(1))
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("expected 'unknown action' error, got: %v", err)
	}
}

func TestExtractOneOf_DuplicateNames_FirstWins(t *testing.T) {
	srv := newTestServer(t, chatHandler(unionJSON("dup", `{"query":"test"}`), nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{
		llm.Variant("dup", SearchAction{}), llm.Variant("dup", AnswerAction{}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(*SearchAction); !ok {
		t.Fatalf("got %T, want *SearchAction (first match)", result)
	}
}

func TestExtractOneOf_PointerVariant(t *testing.T) {
	srv := newTestServer(t, chatHandler(unionJSON("search", `{"query":"ptr"}`), nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{
		llm.Variant("search", &SearchAction{}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.(*SearchAction).Query != "ptr" {
		t.Errorf("Query = %q, want ptr", result.(*SearchAction).Query)
	}
}

func TestExtractOneOf_OptionalFields(t *testing.T) {
	type W struct {
		Name  string  `json:"name"`
		Extra *string `json:"extra"`
	}
	srv := newTestServer(t, chatHandler(mj(t, map[string]any{
		"result": map[string]any{"action": "o", "name": "test"},
	}), nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{llm.Variant("o", W{})})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w := result.(*W)
	if w.Name != "test" || w.Extra != nil {
		t.Errorf("got Name=%q Extra=%v, want test/nil", w.Name, w.Extra)
	}
}

func TestExtractOneOf_SchemaStrictness(t *testing.T) {
	schema := llm.TestUnionSchema([]llm.VariantDef{llm.Variant("a", SearchAction{})})
	if schema["additionalProperties"] != false {
		t.Error("root missing additionalProperties: false")
	}
	anyOf := schema["properties"].(map[string]any)["result"].(map[string]any)["anyOf"].([]any)
	if anyOf[0].(map[string]any)["additionalProperties"] != false {
		t.Error("variant missing additionalProperties: false")
	}
}

func TestExtractOneOf_RetryFeedsErrorBack(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			chatHandler(`broken`, nil, "stop")(w, r)
		} else {
			chatHandler(unionJSON("search", `{"query":"fixed"}`), nil, "stop")(w, r)
		}
	})
	c := llm.NewClient(srv.URL, "key", "model")
	result, err := c.ExtractOneOf(ctx(t), msgs("t"), []llm.VariantDef{llm.Variant("search", SearchAction{})})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.(*SearchAction).Query != "fixed" {
		t.Errorf("Query = %q, want fixed", result.(*SearchAction).Query)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (retry after bad JSON)", calls.Load())
	}
}

func mj(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mj: %v", err)
	}
	return string(b)
}
