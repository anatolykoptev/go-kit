package session

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func mockSummarize(response string) SummarizeFn {
	return func(_ context.Context, _ string) (string, error) {
		return response, nil
	}
}

func mockSummarizeErr(errMsg string) SummarizeFn {
	return func(_ context.Context, _ string) (string, error) {
		return "", errors.New(errMsg)
	}
}

func TestCompactor_TriggersAtThreshold(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	called := false
	c := Compactor{
		Store:     store,
		Threshold: 15,
		KeepLast:  5,
		Summarize: func(_ context.Context, _ string) (string, error) {
			called = true
			return "summary", nil
		},
	}

	c.Compact(context.Background(), "k1")
	if !called {
		t.Error("SummarizeFn should be called when over threshold")
	}
	if store.MessageCount("k1") != 5 {
		t.Errorf("after compact: MessageCount = %d, want 5", store.MessageCount("k1"))
	}
	if store.GetSummary("k1") != "summary" {
		t.Errorf("summary = %q, want %q", store.GetSummary("k1"), "summary")
	}
}

func TestCompactor_NoopUnderThreshold(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 5 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	called := false
	c := Compactor{
		Store:     store,
		Threshold: 10,
		KeepLast:  3,
		Summarize: func(_ context.Context, _ string) (string, error) {
			called = true
			return "summary", nil
		},
	}

	c.Compact(context.Background(), "k1")
	if called {
		t.Error("SummarizeFn should NOT be called under threshold")
	}
	if store.MessageCount("k1") != 5 {
		t.Error("messages should be unchanged")
	}
}

func TestCompactor_SummaryMode(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "test message"})
	}

	var receivedPrompt string
	c := Compactor{
		Store:     store,
		Threshold: 10,
		KeepLast:  5,
		Summarize: func(_ context.Context, prompt string) (string, error) {
			receivedPrompt = prompt
			return "concise summary", nil
		},
	}

	c.Compact(context.Background(), "k1")

	if !strings.Contains(receivedPrompt, "Summarize") {
		t.Error("summary mode prompt should contain 'Summarize'")
	}
	if store.GetSummary("k1") != "concise summary" {
		t.Errorf("summary = %q", store.GetSummary("k1"))
	}
}

func TestCompactor_FactExtraction(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "test message"})
	}

	c := Compactor{
		Store:        store,
		Threshold:    10,
		KeepLast:     5,
		ExtractFacts: true,
		Summarize:    mockSummarize("- User prefers dark mode\n- Deployment uses docker\n- API key rotated weekly"),
	}

	c.Compact(context.Background(), "k1")

	facts := store.GetFacts("k1")
	if len(facts) != 3 {
		t.Fatalf("facts len = %d, want 3", len(facts))
	}
	if facts[0].Content != "User prefers dark mode" {
		t.Errorf("fact[0] = %q", facts[0].Content)
	}
}

func TestCompactor_FactExtraction_MalformedOutput(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	c := Compactor{
		Store:        store,
		Threshold:    10,
		KeepLast:     5,
		ExtractFacts: true,
		Summarize:    mockSummarize("No bullet points here, just prose.\nAnother line."),
	}

	c.Compact(context.Background(), "k1")

	// Malformed output — no "- " prefixes — should still set summary as fallback.
	facts := store.GetFacts("k1")
	if len(facts) != 0 {
		t.Errorf("malformed output should yield 0 facts, got %d", len(facts))
	}
	// Summary should be set as fallback.
	if store.GetSummary("k1") == "" {
		t.Error("summary should be set as fallback when no facts extracted")
	}
}

func TestCompactor_MultiPart(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 30 {
		store.AddMessage("k1", Message{Role: "user", Content: "message content here"})
	}

	callCount := 0
	c := Compactor{
		Store:     store,
		Threshold: 10,
		KeepLast:  5,
		MultiPart: true,
		Summarize: func(_ context.Context, _ string) (string, error) {
			callCount++
			return "part summary", nil
		},
	}

	c.Compact(context.Background(), "k1")

	// Multi-part: 2 halves + 1 merge = 3 calls.
	if callCount != 3 {
		t.Errorf("multi-part: SummarizeFn called %d times, want 3", callCount)
	}
}

func TestCompactor_OversizedMessageGuard(t *testing.T) {
	store := NewInMemoryStore(Options{})
	// Add a huge message.
	store.AddMessage("k1", Message{Role: "user", Content: strings.Repeat("x", 50000)})
	for range 15 {
		store.AddMessage("k1", Message{Role: "user", Content: "normal"})
	}

	var receivedPrompt string
	c := Compactor{
		Store:          store,
		Threshold:      10,
		KeepLast:       5,
		MaxTokensGuard: 10000,
		Summarize: func(_ context.Context, prompt string) (string, error) {
			receivedPrompt = prompt
			return "summary", nil
		},
	}

	c.Compact(context.Background(), "k1")

	// The 50k message should be excluded from summarization.
	if strings.Contains(receivedPrompt, strings.Repeat("x", 1000)) {
		t.Error("oversized message should be excluded from summarization prompt")
	}
}

func TestCompactor_SummarizeFnError(t *testing.T) {
	store := NewInMemoryStore(Options{})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	c := Compactor{
		Store:     store,
		Threshold: 10,
		KeepLast:  5,
		Summarize: mockSummarizeErr("LLM failed"),
	}

	c.Compact(context.Background(), "k1")

	// On error, messages should still be compacted (truncated) but no summary set.
	// The compaction already happened (CompactMessages was called).
	// Summary should remain empty.
	if store.GetSummary("k1") != "" {
		t.Error("on SummarizeFn error, summary should not be set")
	}
}

func TestCompactor_ExistingSummaryInPrompt(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.GetOrCreate("k1")
	store.SetSummary("k1", "previous context about nginx")
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	var receivedPrompt string
	c := Compactor{
		Store:     store,
		Threshold: 10,
		KeepLast:  5,
		Summarize: func(_ context.Context, prompt string) (string, error) {
			receivedPrompt = prompt
			return "updated summary", nil
		},
	}

	c.Compact(context.Background(), "k1")

	if !strings.Contains(receivedPrompt, "previous context about nginx") {
		t.Error("existing summary should be included in prompt")
	}
}

func TestCompactor_ExistingFactsPreserved(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.GetOrCreate("k1")
	store.AddFacts("k1", []Fact{{Content: "old fact"}})
	for range 20 {
		store.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	c := Compactor{
		Store:        store,
		Threshold:    10,
		KeepLast:     5,
		ExtractFacts: true,
		Summarize:    mockSummarize("- new fact"),
	}

	c.Compact(context.Background(), "k1")

	facts := store.GetFacts("k1")
	if len(facts) != 2 {
		t.Fatalf("facts len = %d, want 2 (1 old + 1 new)", len(facts))
	}
	if facts[0].Content != "old fact" {
		t.Errorf("old fact should be preserved, got %q", facts[0].Content)
	}
}
