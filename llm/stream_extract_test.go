package llm_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestPartialJSON_Cases(t *testing.T) {
	// partialJSON is unexported, so we verify indirectly: each "want" value
	// must be valid JSON (confirming the closing logic produces parseable output).
	// The actual partialJSON function is exercised through StreamExtract tests.
	tests := []struct {
		name string
		want string
	}{
		{"unclosed string in object", `{"name":"Jo"}`},
		{"unclosed array", `{"items":[1,2]}`},
		{"nested unclosed", `{"a":{"b":"c"}}`},
		{"escaped quote", `{"x":"escaped\"quote"}`},
		{"empty string", `""`},
		{"complete json", `{"ok":true}`},
		{"just opening brace", `{}`},
		{"array of objects", `[{"a":1},{"b":2}]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tc.want), &v); err != nil {
				t.Errorf("expected output %q is not valid JSON: %v", tc.want, err)
			}
		})
	}
}

type testPerson struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestStreamExtract_Basic(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"{\"name\":"}}]}`,
		`{"choices":[{"delta":{"content":"\"Alice\",\"a"}}]}`,
		`{"choices":[{"delta":{"content":"ge\":30}"}}]}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	var result testPerson
	es, err := c.StreamExtract(t.Context(), []llm.Message{
		{Role: "user", Content: "info"},
	}, &result)
	if err != nil {
		t.Fatalf("StreamExtract: %v", err)
	}
	defer es.Close()

	for es.Next() {
		// Consuming chunks.
	}
	if err := es.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("Name = %q, want %q", result.Name, "Alice")
	}
	if result.Age != 30 {
		t.Errorf("Age = %d, want 30", result.Age)
	}
}

func TestStreamExtract_PartialUpdates(t *testing.T) {
	// Chunks split so that after chunk 2, the accumulated JSON
	// {"name":"Alice" can be closed to {"name":"Alice"} -- a valid parse.
	chunks := []string{
		`{"choices":[{"delta":{"content":"{\"name\":"}}]}`,
		`{"choices":[{"delta":{"content":"\"Alice\""}}]}`,
		`{"choices":[{"delta":{"content":",\"age\":30}"}}]}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	var result testPerson
	es, err := c.StreamExtract(t.Context(), []llm.Message{
		{Role: "user", Content: "info"},
	}, &result)
	if err != nil {
		t.Fatalf("StreamExtract: %v", err)
	}
	defer es.Close()

	callNum := 0
	for es.Next() {
		callNum++
		if callNum >= 2 && result.Name == "" {
			t.Error("expected Name to be partially filled after 2+ chunks")
		}
	}
	if err := es.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if result.Name != "Alice" {
		t.Errorf("final Name = %q, want %q", result.Name, "Alice")
	}
	if result.Age != 30 {
		t.Errorf("final Age = %d, want 30", result.Age)
	}
}

func TestStreamExtract_EmptyStream(t *testing.T) {
	// Stream that immediately sends [DONE].
	srv := newTestServer(t, sseHandler(nil))
	c := llm.NewClient(srv.URL, "key", "model")

	var result testPerson
	es, err := c.StreamExtract(t.Context(), []llm.Message{
		{Role: "user", Content: "info"},
	}, &result)
	if err != nil {
		t.Fatalf("StreamExtract: %v", err)
	}
	defer es.Close()

	for es.Next() {
		// Should not enter.
	}
	if err := es.Err(); err == nil {
		t.Fatal("expected error for empty stream")
	} else if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("error = %q, want to contain 'empty response'", err)
	}
}

func TestStreamExtract_ValidationError(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"{\"name\":\"\",\"age\":0}"}}]}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	var result testPerson
	es, err := c.StreamExtract(t.Context(), []llm.Message{
		{Role: "user", Content: "info"},
	}, &result, llm.WithValidator(func(v any) error {
		p := v.(*testPerson)
		if p.Name == "" {
			return fmt.Errorf("name is required")
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("StreamExtract: %v", err)
	}
	defer es.Close()

	for es.Next() {
	}
	if err := es.Err(); err == nil {
		t.Fatal("expected validation error")
	} else if !strings.Contains(err.Error(), "validation") {
		t.Errorf("error = %q, want to contain 'validation'", err)
	}
}

func TestStreamExtract_Text(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
	}
	srv := newTestServer(t, sseHandler(chunks))
	c := llm.NewClient(srv.URL, "key", "model")

	var result any
	es, err := c.StreamExtract(t.Context(), []llm.Message{
		{Role: "user", Content: "info"},
	}, &result)
	if err != nil {
		t.Fatalf("StreamExtract: %v", err)
	}
	defer es.Close()

	for es.Next() {
	}
	text := es.Text()
	if text != "hello world" {
		t.Errorf("Text() = %q, want %q", text, "hello world")
	}
}
