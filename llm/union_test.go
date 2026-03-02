package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

type SearchAction struct {
	Query string `json:"query"`
}

type AnswerAction struct {
	Answer string `json:"answer"`
}

func unionJSON(action, content string) string {
	inner := map[string]any{"action": action}
	_ = json.Unmarshal([]byte(content), &inner)
	b, _ := json.Marshal(map[string]any{"result": inner})
	return string(b)
}

func TestExtractOneOf_Search(t *testing.T) {
	body := unionJSON("search", `{"query":"golang generics"}`)
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	result, err := c.ExtractOneOf(ctx(t), msgs("pick an action"), []llm.VariantDef{
		llm.Variant("search", SearchAction{}),
		llm.Variant("answer", AnswerAction{}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sa, ok := result.(*SearchAction)
	if !ok {
		t.Fatalf("got %T, want *SearchAction", result)
	}
	if sa.Query != "golang generics" {
		t.Errorf("Query = %q, want %q", sa.Query, "golang generics")
	}
}

func TestExtractOneOf_Answer(t *testing.T) {
	body := unionJSON("answer", `{"answer":"42"}`)
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	result, err := c.ExtractOneOf(ctx(t), msgs("pick an action"), []llm.VariantDef{
		llm.Variant("search", SearchAction{}),
		llm.Variant("answer", AnswerAction{}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	aa, ok := result.(*AnswerAction)
	if !ok {
		t.Fatalf("got %T, want *AnswerAction", result)
	}
	if aa.Answer != "42" {
		t.Errorf("Answer = %q, want %q", aa.Answer, "42")
	}
}

func TestExtractOneOf_UnknownVariant(t *testing.T) {
	body := unionJSON("unknown", `{}`)
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	_, err := c.ExtractOneOf(ctx(t), msgs("pick"), []llm.VariantDef{
		llm.Variant("search", SearchAction{}),
	}, llm.WithExtractRetries(1))
	if err == nil {
		t.Fatal("expected error for unknown variant")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("error = %q, want to contain 'unknown action'", err)
	}
}

func TestExtractOneOf_ValidationRetry(t *testing.T) {
	var calls atomic.Int32
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var body string
		if n == 1 {
			body = unionJSON("search", `{"query":""}`)
		} else {
			body = unionJSON("search", `{"query":"valid"}`)
		}
		chatHandler(body, nil, "stop")(w, r)
	})
	c := llm.NewClient(srv.URL, "key", "model")

	result, err := c.ExtractOneOf(ctx(t), msgs("pick"), []llm.VariantDef{
		llm.Variant("search", SearchAction{}),
		llm.Variant("answer", AnswerAction{}),
	}, llm.WithValidator(func(v any) error {
		if sa, ok := v.(*SearchAction); ok && sa.Query == "" {
			return errors.New("query is required")
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sa := result.(*SearchAction)
	if sa.Query != "valid" {
		t.Errorf("Query = %q, want %q", sa.Query, "valid")
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestExtractOneOf_Schema(t *testing.T) {
	schema := llm.TestUnionSchema([]llm.VariantDef{
		llm.Variant("search", SearchAction{}),
		llm.Variant("answer", AnswerAction{}),
	})

	if schema["type"] != "object" {
		t.Errorf("root type = %v, want object", schema["type"])
	}
	props := schema["properties"].(map[string]any)
	resultProp := props["result"].(map[string]any)
	anyOf := resultProp["anyOf"].([]any)
	if len(anyOf) != 2 {
		t.Fatalf("anyOf len = %d, want 2", len(anyOf))
	}

	// Check first variant has "action" enum with "search".
	v0 := anyOf[0].(map[string]any)
	v0Props := v0["properties"].(map[string]any)
	actionSchema := v0Props["action"].(map[string]any)
	enums := actionSchema["enum"].([]string)
	if len(enums) != 1 || enums[0] != "search" {
		t.Errorf("action enum = %v, want [search]", enums)
	}

	// Check "action" is first in required.
	req := v0["required"].([]string)
	if len(req) == 0 || req[0] != "action" {
		t.Errorf("required[0] = %v, want action", req)
	}
}

type ActionConflict struct {
	Action string `json:"action"`
	Data   string `json:"data"`
}

func TestExtractOneOf_VariantWithActionField(t *testing.T) {
	// User struct already has an "action" field — discriminator overwrites it.
	body := unionJSON("conflict", `{"data":"hello","action":"conflict"}`)
	srv := newTestServer(t, chatHandler(body, nil, "stop"))
	c := llm.NewClient(srv.URL, "key", "model")

	result, err := c.ExtractOneOf(ctx(t), msgs("test"), []llm.VariantDef{
		llm.Variant("conflict", ActionConflict{}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ac := result.(*ActionConflict)
	if ac.Data != "hello" {
		t.Errorf("Data = %q, want %q", ac.Data, "hello")
	}
	// The action field gets the discriminator value.
	if ac.Action != "conflict" {
		t.Errorf("Action = %q, want %q", ac.Action, "conflict")
	}
}

func TestVariant_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty Variant")
		}
	}()
	llm.Variant("", nil)
}

// Helpers to reduce boilerplate.
func ctx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func msgs(content string) []llm.Message {
	return []llm.Message{{Role: "user", Content: content}}
}
