package llm

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFormatChatTime_RoundTrip(t *testing.T) {
	want := time.Date(2026, 5, 4, 6, 30, 0, 0, time.UTC)
	s := FormatChatTime(want)
	if s != "2026-05-04T06:30:00Z" {
		t.Errorf("FormatChatTime = %q, want 2026-05-04T06:30:00Z", s)
	}
	got := ParseChatTime(s)
	if !got.Equal(want) {
		t.Errorf("ParseChatTime round-trip got %v, want %v", got, want)
	}
}

func TestFormatChatTime_Zero(t *testing.T) {
	if got := FormatChatTime(time.Time{}); got != "" {
		t.Errorf("FormatChatTime(zero) = %q, want empty", got)
	}
}

func TestParseChatTime_Empty(t *testing.T) {
	if got := ParseChatTime(""); !got.IsZero() {
		t.Errorf("ParseChatTime('') = %v, want zero", got)
	}
}

func TestParseChatTime_Bad(t *testing.T) {
	if got := ParseChatTime("not a date"); !got.IsZero() {
		t.Errorf("ParseChatTime(bad) = %v, want zero", got)
	}
}

// applyMessageTimestamps prepends a bracketed UTC prefix only when
// ChatTime is set and Content is a string.
func TestApplyMessageTimestamps_Prepends(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello", ChatTime: "2026-05-04T06:30:00Z"},
		{Role: "system", Content: "stable preamble"}, // no ChatTime → untouched
	}
	applyMessageTimestamps(msgs)
	got, _ := msgs[0].Content.(string)
	want := "[2026-05-04 06:30 UTC] hello"
	if got != want {
		t.Errorf("user msg = %q, want %q", got, want)
	}
	if got2, _ := msgs[1].Content.(string); got2 != "stable preamble" {
		t.Errorf("system msg should be untouched, got %q", got2)
	}
}

func TestApplyMessageTimestamps_MultimodalUntouched(t *testing.T) {
	parts := []ContentPart{{Type: "text", Text: "hi"}}
	msgs := []Message{{Role: "user", Content: parts, ChatTime: "2026-05-04T06:30:00Z"}}
	applyMessageTimestamps(msgs)
	gotParts, ok := msgs[0].Content.([]ContentPart)
	if !ok {
		t.Fatalf("Content type changed: %T", msgs[0].Content)
	}
	if !reflect.DeepEqual(gotParts, parts) {
		t.Errorf("multimodal Content modified: got %+v, want %+v", gotParts, parts)
	}
}

func TestApplyMessageTimestamps_BadChatTime(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hello", ChatTime: "garbage"}}
	applyMessageTimestamps(msgs)
	if got, _ := msgs[0].Content.(string); got != "hello" {
		t.Errorf("bad ChatTime should not modify content; got %q", got)
	}
}

// Verify the WithMessageTimestamps ChatOption flips the chatConfig flag
// and that apply() honors it on the request messages.
func TestWithMessageTimestamps_End2End(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "ping", ChatTime: "2026-05-04T06:30:00Z"},
		},
	}
	cfg := chatConfig{}
	WithMessageTimestamps()(&cfg)
	cfg.apply(req)
	if got := req.Messages[0].Content.(string); !strings.HasPrefix(got, "[2026-05-04 06:30 UTC] ") {
		t.Errorf("Content not prefixed: %q", got)
	}
}

func TestWithMessageTimestamps_OffByDefault(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "ping", ChatTime: "2026-05-04T06:30:00Z"},
		},
	}
	cfg := chatConfig{}
	cfg.apply(req)
	if got := req.Messages[0].Content.(string); got != "ping" {
		t.Errorf("Content modified without opt-in: %q", got)
	}
}
