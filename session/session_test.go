package session

import (
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	s := NewSession("test-key")
	if s.Key != "test-key" {
		t.Errorf("Key = %q, want %q", s.Key, "test-key")
	}
	if s.Created.IsZero() {
		t.Error("Created should be set")
	}
	if s.Updated.IsZero() {
		t.Error("Updated should be set")
	}
	if len(s.Messages) != 0 {
		t.Errorf("Messages should be empty, got %d", len(s.Messages))
	}
}

func TestSession_AddMessage(t *testing.T) {
	s := NewSession("k")
	before := s.Updated

	// Small delay to ensure timestamp changes.
	time.Sleep(time.Millisecond)

	s.AddMessage(Message{Role: "user", Content: "hello"})

	if len(s.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(s.Messages))
	}
	if s.Messages[0].Content != "hello" {
		t.Errorf("Content = %q, want %q", s.Messages[0].Content, "hello")
	}
	if !s.Updated.After(before) {
		t.Error("Updated should advance after AddMessage")
	}
}

func TestSession_MessageCount(t *testing.T) {
	s := NewSession("k")
	if s.MessageCount() != 0 {
		t.Errorf("empty session: MessageCount = %d, want 0", s.MessageCount())
	}
	s.AddMessage(Message{Role: "user", Content: "a"})
	s.AddMessage(Message{Role: "assistant", Content: "b"})
	if s.MessageCount() != 2 {
		t.Errorf("MessageCount = %d, want 2", s.MessageCount())
	}
}

func TestSession_CompactMessages(t *testing.T) {
	s := NewSession("k")
	for range 10 {
		s.AddMessage(Message{Role: "user", Content: "msg"})
	}

	removed := s.CompactMessages(4)
	if len(removed) != 6 {
		t.Errorf("removed len = %d, want 6", len(removed))
	}
	if s.MessageCount() != 4 {
		t.Errorf("after compact: MessageCount = %d, want 4", s.MessageCount())
	}
}

func TestSession_CompactMessages_UnderKeep(t *testing.T) {
	s := NewSession("k")
	s.AddMessage(Message{Role: "user", Content: "msg"})

	removed := s.CompactMessages(5)
	if removed != nil {
		t.Errorf("should return nil when under keepLast, got %d", len(removed))
	}
	if s.MessageCount() != 1 {
		t.Errorf("messages should be unchanged, got %d", s.MessageCount())
	}
}

func TestSession_TruncateHistory(t *testing.T) {
	s := NewSession("k")
	for range 8 {
		s.AddMessage(Message{Role: "user", Content: "msg"})
	}
	s.TruncateHistory(3)
	if s.MessageCount() != 3 {
		t.Errorf("after truncate: MessageCount = %d, want 3", s.MessageCount())
	}
}

func TestSession_Clear(t *testing.T) {
	s := NewSession("k")
	s.AddMessage(Message{Role: "user", Content: "msg"})
	s.Summary = "some summary"
	s.Facts = []Fact{{Content: "fact1"}}

	s.Clear()
	if s.MessageCount() != 0 {
		t.Errorf("Messages should be empty after Clear, got %d", s.MessageCount())
	}
	if s.Summary != "" {
		t.Errorf("Summary should be empty after Clear, got %q", s.Summary)
	}
	if len(s.Facts) != 0 {
		t.Errorf("Facts should be empty after Clear, got %d", len(s.Facts))
	}
}

func TestSession_Facts(t *testing.T) {
	s := NewSession("k")
	if got := s.GetFacts(); got != nil {
		t.Errorf("empty session: GetFacts = %v, want nil", got)
	}

	facts := []Fact{
		{Content: "fact1", ExtractedAt: time.Now()},
		{Content: "fact2", ExtractedAt: time.Now()},
	}
	s.AddFacts(facts)
	if len(s.Facts) != 2 {
		t.Fatalf("Facts len = %d, want 2", len(s.Facts))
	}

	// GetFacts returns a copy.
	got := s.GetFacts()
	got[0].Content = "mutated"
	if s.Facts[0].Content == "mutated" {
		t.Error("GetFacts should return a copy, not a reference")
	}

	// AddFacts appends.
	s.AddFacts([]Fact{{Content: "fact3"}})
	if len(s.Facts) != 3 {
		t.Errorf("after second AddFacts: len = %d, want 3", len(s.Facts))
	}
}
