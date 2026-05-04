package session

import (
	"encoding/json"
	"testing"
)

func TestMessage_JSONRoundTrip(t *testing.T) {
	msg := Message{
		Role:       "assistant",
		Content:    "hello",
		ToolCallID: "call_123",
		ToolCalls: []ToolCall{
			{
				ID:   "tc_1",
				Name: "search",
				Args: `{"query":"test"}`,
				Function: &FunctionCall{
					Name:      "search",
					Arguments: `{"query":"test"}`,
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Role != msg.Role {
		t.Errorf("Role = %q, want %q", got.Role, msg.Role)
	}
	if got.Content != msg.Content {
		t.Errorf("Content = %q, want %q", got.Content, msg.Content)
	}
	if got.ToolCallID != msg.ToolCallID {
		t.Errorf("ToolCallID = %q, want %q", got.ToolCallID, msg.ToolCallID)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0].Function == nil {
		t.Fatal("ToolCalls[0].Function is nil")
	}
	if got.ToolCalls[0].Function.Name != "search" {
		t.Errorf("Function.Name = %q, want %q", got.ToolCalls[0].Function.Name, "search")
	}
}

func TestMessage_JSONOmitEmpty(t *testing.T) {
	msg := Message{Role: "user", Content: "hi"}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if contains(s, "tool_call_id") {
		t.Errorf("empty ToolCallID should be omitted, got: %s", s)
	}
	if contains(s, "tool_calls") {
		t.Errorf("nil ToolCalls should be omitted, got: %s", s)
	}
}

func TestToolCall_WithFunctionCall(t *testing.T) {
	tc := ToolCall{
		ID: "tc_1",
		Function: &FunctionCall{
			Name:      "get_weather",
			Arguments: `{"city":"SPb"}`,
		},
	}
	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolCall
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Function == nil || got.Function.Name != "get_weather" {
		t.Errorf("Function round-trip failed: %+v", got)
	}
}

func TestMessage_Empty(t *testing.T) {
	var msg Message
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != "" || got.Content != "" {
		t.Errorf("empty message round-trip: %+v", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && jsonContains(s, substr)
}

func jsonContains(s, key string) bool {
	for i := 0; i <= len(s)-len(key); i++ {
		if s[i:i+len(key)] == key {
			return true
		}
	}
	return false
}
