package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Usage.UnmarshalJSON must accept OpenAI's shape and populate
// CachedTokens from prompt_tokens_details.cached_tokens.
func TestUsageUnmarshal_OpenAI(t *testing.T) {
	body := `{
		"prompt_tokens": 1500,
		"completion_tokens": 80,
		"total_tokens": 1580,
		"prompt_tokens_details": {"cached_tokens": 1200}
	}`
	var u Usage
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.PromptTokens != 1500 || u.CompletionTokens != 80 || u.TotalTokens != 1580 {
		t.Errorf("base counts wrong: %+v", u)
	}
	if u.CachedTokens != 1200 {
		t.Errorf("CachedTokens = %d, want 1200", u.CachedTokens)
	}
	if u.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0 (OpenAI doesn't report write)", u.CacheCreationTokens)
	}
}

// Usage.UnmarshalJSON must accept Anthropic's shape and populate
// PromptTokens / CompletionTokens / CachedTokens / CacheCreationTokens.
func TestUsageUnmarshal_Anthropic(t *testing.T) {
	body := `{
		"input_tokens": 800,
		"output_tokens": 60,
		"cache_read_input_tokens": 700,
		"cache_creation_input_tokens": 100
	}`
	var u Usage
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.PromptTokens != 800 {
		t.Errorf("PromptTokens = %d, want 800 (mapped from input_tokens)", u.PromptTokens)
	}
	if u.CompletionTokens != 60 {
		t.Errorf("CompletionTokens = %d, want 60 (mapped from output_tokens)", u.CompletionTokens)
	}
	if u.TotalTokens != 860 {
		t.Errorf("TotalTokens = %d, want 860 (input+output)", u.TotalTokens)
	}
	if u.CachedTokens != 700 {
		t.Errorf("CachedTokens = %d, want 700 (cache_read_input_tokens)", u.CachedTokens)
	}
	if u.CacheCreationTokens != 100 {
		t.Errorf("CacheCreationTokens = %d, want 100", u.CacheCreationTokens)
	}
}

// Empty/missing usage fields should yield zero values, not errors.
func TestUsageUnmarshal_Empty(t *testing.T) {
	var u Usage
	if err := json.Unmarshal([]byte(`{}`), &u); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if u != (Usage{}) {
		t.Errorf("expected zero Usage, got %+v", u)
	}
}

// ContentPart with CacheControl must serialise to the wire shape
// Anthropic expects.
func TestContentPart_CacheControl_Marshal(t *testing.T) {
	cp := ContentPart{
		Type:         "text",
		Text:         "huge stable system prompt",
		CacheControl: Ephemeral(),
	}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `"cache_control":{"type":"ephemeral"}`
	if !strings.Contains(got, want) {
		t.Errorf("missing %s in %s", want, got)
	}
	// Without CacheControl, the field must be omitted.
	cp2 := ContentPart{Type: "text", Text: "no cache"}
	b2, _ := json.Marshal(cp2)
	if strings.Contains(string(b2), "cache_control") {
		t.Errorf("unexpected cache_control in %s", b2)
	}
}

// EphemeralExtended sets ttl=1h.
func TestEphemeralExtended_TTL(t *testing.T) {
	cc := EphemeralExtended()
	if cc.Type != "ephemeral" || cc.TTL != "1h" {
		t.Errorf("got %+v, want type=ephemeral ttl=1h", cc)
	}
	b, _ := json.Marshal(cc)
	if !strings.Contains(string(b), `"ttl":"1h"`) {
		t.Errorf("missing ttl in %s", b)
	}
}

// NewCachedSystemMessage with nil cc returns a plain string-content
// message (no array wrapping — keeps OpenAI-compat clean).
func TestNewCachedSystemMessage_NilCC_PlainString(t *testing.T) {
	m := NewCachedSystemMessage("hello", nil)
	if m.Role != "system" {
		t.Errorf("Role = %q, want system", m.Role)
	}
	s, ok := m.Content.(string)
	if !ok {
		t.Fatalf("Content = %T, want string", m.Content)
	}
	if s != "hello" {
		t.Errorf("Content = %q, want hello", s)
	}
}

// NewCachedSystemMessage with cc wraps content in [ContentPart] with
// cache_control on the part.
func TestNewCachedSystemMessage_WithCC_WrapsArray(t *testing.T) {
	m := NewCachedSystemMessage("big prompt", Ephemeral())
	parts, ok := m.Content.([]ContentPart)
	if !ok {
		t.Fatalf("Content = %T, want []ContentPart", m.Content)
	}
	if len(parts) != 1 {
		t.Fatalf("len(parts) = %d, want 1", len(parts))
	}
	want := ContentPart{Type: "text", Text: "big prompt", CacheControl: Ephemeral()}
	if !reflect.DeepEqual(parts[0], want) {
		t.Errorf("parts[0] = %+v, want %+v", parts[0], want)
	}
}

// Tool.CacheControl must serialise alongside type+function.
func TestTool_CacheControl_Marshal(t *testing.T) {
	tool := Tool{
		Type:         "function",
		Function:     ToolFunction{Name: "search", Description: "Search docs", Parameters: map[string]any{}},
		CacheControl: Ephemeral(),
	}
	b, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("missing cache_control in %s", got)
	}
}

// NewCachedUserMessage parallel: tests the user-role variant works the
// same way (long retrieved-context block).
func TestNewCachedUserMessage_WithCC(t *testing.T) {
	m := NewCachedUserMessage("retrieved doc", Ephemeral())
	if m.Role != "user" {
		t.Errorf("Role = %q, want user", m.Role)
	}
	parts, ok := m.Content.([]ContentPart)
	if !ok || len(parts) != 1 || parts[0].CacheControl == nil {
		t.Errorf("expected wrapped ContentPart with cache_control, got %+v", m.Content)
	}
}
