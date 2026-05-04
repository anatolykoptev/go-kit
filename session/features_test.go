package session

import (
	"strings"
	"sync"
	"testing"
)

// --- MaxContentSize ---

func TestMaxContentSize_TruncatesOnAdd(t *testing.T) {
	store := NewInMemoryStore(Options{MaxContentSize: 20})
	long := strings.Repeat("x", 50)
	store.AddMessage("k", Message{Role: "user", Content: long})

	msgs := store.GetHistory("k")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if len(msgs[0].Content) >= 50 {
		t.Errorf("content not truncated: len=%d", len(msgs[0].Content))
	}
	if !strings.HasSuffix(msgs[0].Content, contentTruncSuffix) {
		t.Errorf("missing truncation suffix: %q", msgs[0].Content)
	}
}

func TestMaxContentSize_ShortContentUntouched(t *testing.T) {
	store := NewInMemoryStore(Options{MaxContentSize: 100})
	store.AddMessage("k", Message{Role: "user", Content: "short"})

	msgs := store.GetHistory("k")
	if msgs[0].Content != "short" {
		t.Errorf("short content modified: %q", msgs[0].Content)
	}
}

func TestMaxContentSize_ZeroMeansUnlimited(t *testing.T) {
	store := NewInMemoryStore(Options{})
	long := strings.Repeat("x", 10000)
	store.AddMessage("k", Message{Role: "user", Content: long})

	msgs := store.GetHistory("k")
	if len(msgs[0].Content) != 10000 {
		t.Errorf("content truncated when MaxContentSize=0: len=%d", len(msgs[0].Content))
	}
}

func TestMaxContentSize_UpdateLastMessage(t *testing.T) {
	store := NewInMemoryStore(Options{MaxContentSize: 10})
	store.AddMessage("k", Message{Role: "assistant", Content: "hi"})
	store.UpdateLastMessage("k", strings.Repeat("y", 30))

	msgs := store.GetHistory("k")
	if len(msgs[0].Content) >= 30 {
		t.Errorf("UpdateLastMessage not truncated: len=%d", len(msgs[0].Content))
	}
}

// --- MaxFacts ---

func TestMaxFacts_RotatesOldest(t *testing.T) {
	store := NewInMemoryStore(Options{MaxFacts: 3})

	for i := range 5 {
		store.AddFacts("k", []Fact{{Content: string(rune('A' + i))}})
	}

	facts := store.GetFacts("k")
	if len(facts) != 3 {
		t.Fatalf("got %d facts, want 3", len(facts))
	}
	if facts[0].Content != "C" || facts[1].Content != "D" || facts[2].Content != "E" {
		t.Errorf("wrong facts: %v", facts)
	}
}

func TestMaxFacts_ZeroMeansUnlimited(t *testing.T) {
	store := NewInMemoryStore(Options{})

	for range 100 {
		store.AddFacts("k", []Fact{{Content: "f"}})
	}

	facts := store.GetFacts("k")
	if len(facts) != 100 {
		t.Errorf("facts limited when MaxFacts=0: %d", len(facts))
	}
}

func TestMaxFacts_ExactLimit(t *testing.T) {
	store := NewInMemoryStore(Options{MaxFacts: 5})

	store.AddFacts("k", []Fact{
		{Content: "a"}, {Content: "b"}, {Content: "c"},
		{Content: "d"}, {Content: "e"},
	})

	facts := store.GetFacts("k")
	if len(facts) != 5 {
		t.Fatalf("got %d, want 5", len(facts))
	}
}

// --- UpdateLastMessage ---

func TestUpdateLastMessage_Basic(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.AddMessage("k", Message{Role: "assistant", Content: "chunk1"})
	store.UpdateLastMessage("k", "chunk1 chunk2")

	msgs := store.GetHistory("k")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if msgs[0].Content != "chunk1 chunk2" {
		t.Errorf("content = %q", msgs[0].Content)
	}
}

func TestUpdateLastMessage_EmptySession(t *testing.T) {
	store := NewInMemoryStore(Options{})
	// Should not panic.
	store.UpdateLastMessage("nonexistent", "data")
}

func TestUpdateLastMessage_PreservesRole(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.AddMessage("k", Message{Role: "assistant", Content: "v1"})
	store.AddMessage("k", Message{Role: "user", Content: "question"})
	store.UpdateLastMessage("k", "updated question")

	msgs := store.GetHistory("k")
	if msgs[1].Role != "user" || msgs[1].Content != "updated question" {
		t.Errorf("last message = %+v", msgs[1])
	}
	if msgs[0].Content != "v1" {
		t.Errorf("first message modified: %q", msgs[0].Content)
	}
}

// --- Per-key locking concurrency ---

func TestPerKeyLocking_Concurrent(t *testing.T) {
	store := NewInMemoryStore(Options{})
	const goroutines = 50
	const msgsPerKey = 100

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		key := string(rune('A' + i%5)) // 5 different keys
		go func() {
			defer wg.Done()
			for range msgsPerKey {
				store.AddMessage(key, Message{Role: "user", Content: "msg"})
			}
		}()
	}
	wg.Wait()

	// Each of 5 keys should have goroutines/5 * msgsPerKey messages.
	for i := range 5 {
		key := string(rune('A' + i))
		expected := (goroutines / 5) * msgsPerKey
		if got := store.MessageCount(key); got != expected {
			t.Errorf("key %s: got %d, want %d", key, got, expected)
		}
	}
}

func TestPerKeyLocking_UpdateDuringRead(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.AddMessage("k", Message{Role: "assistant", Content: "initial"})

	var wg sync.WaitGroup
	// Concurrent updates and reads shouldn't race.
	for range 100 {
		wg.Add(2) //nolint:mnd
		go func() {
			defer wg.Done()
			store.UpdateLastMessage("k", "updated")
		}()
		go func() {
			defer wg.Done()
			_ = store.GetHistory("k")
		}()
	}
	wg.Wait()
}
