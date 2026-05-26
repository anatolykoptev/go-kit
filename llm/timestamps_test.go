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

// chat_time is not a standard OpenAI message field — strict gateways reject it
// (HTTP 400 wrong_api_format). After materialization, ChatTime must be cleared
// on EVERY message (string, multimodal, bad) so it never reaches the wire.
func TestApplyMessageTimestamps_ClearsChatTimeFromWire(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello", ChatTime: "2026-05-04T06:30:00Z"},
		{Role: "user", Content: []ContentPart{{Type: "text", Text: "mm"}}, ChatTime: "2026-05-04T06:31:00Z"},
		{Role: "user", Content: "x", ChatTime: "garbage"},
	}
	applyMessageTimestamps(msgs)
	for i, m := range msgs {
		if m.ChatTime != "" {
			t.Errorf("msg[%d] ChatTime not cleared (would leak chat_time to wire): %q", i, m.ChatTime)
		}
	}
	if got, _ := msgs[0].Content.(string); !strings.HasPrefix(got, "[2026-05-04 06:30 UTC] ") {
		t.Errorf("string content not materialized: %q", got)
	}
}

// newRequest must clone the caller's messages — request-time options
// (WithMessageTimestamps) mutate in place and must not corrupt the caller's
// stored messages (e.g. dozor's session store round-trips ChatTime).
func TestNewRequest_ClonesMessages(t *testing.T) {
	c := &Client{model: "m"}
	orig := []Message{{Role: "user", Content: "hi", ChatTime: "2026-05-04T06:30:00Z"}}
	req := c.newRequest(orig)
	applyMessageTimestamps(req.Messages)
	if got, _ := orig[0].Content.(string); got != "hi" {
		t.Errorf("caller Content mutated by request processing: %q", got)
	}
	if orig[0].ChatTime != "2026-05-04T06:30:00Z" {
		t.Errorf("caller ChatTime mutated: %q", orig[0].ChatTime)
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
