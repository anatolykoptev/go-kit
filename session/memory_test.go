package session

import (
	"sync"
	"testing"
	"time"
)

func TestInMemoryStore_GetOrCreate(t *testing.T) {
	s := NewInMemoryStore(Options{})

	sess := s.GetOrCreate("k1")
	if sess.Key != "k1" {
		t.Errorf("Key = %q, want %q", sess.Key, "k1")
	}

	// Same key returns same session.
	sess2 := s.GetOrCreate("k1")
	if sess2 != sess {
		t.Error("GetOrCreate should return same pointer for same key")
	}
}

func TestInMemoryStore_AddMessage(t *testing.T) {
	s := NewInMemoryStore(Options{})
	s.AddMessage("k1", Message{Role: "user", Content: "hello"})

	msgs := s.GetHistory("k1")
	if len(msgs) != 1 {
		t.Fatalf("GetHistory len = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("Content = %q, want %q", msgs[0].Content, "hello")
	}
}

func TestInMemoryStore_GetHistory_Unknown(t *testing.T) {
	s := NewInMemoryStore(Options{})
	msgs := s.GetHistory("nonexistent")
	if msgs != nil {
		t.Errorf("unknown key should return nil, got %v", msgs)
	}
}

func TestInMemoryStore_GetHistory_ReturnsCopy(t *testing.T) {
	s := NewInMemoryStore(Options{})
	s.AddMessage("k1", Message{Role: "user", Content: "original"})

	msgs := s.GetHistory("k1")
	msgs[0].Content = "mutated"

	check := s.GetHistory("k1")
	if check[0].Content != "original" {
		t.Error("GetHistory should return a copy")
	}
}

func TestInMemoryStore_Summary(t *testing.T) {
	s := NewInMemoryStore(Options{})

	if got := s.GetSummary("k1"); got != "" {
		t.Errorf("unknown key: GetSummary = %q, want empty", got)
	}

	s.GetOrCreate("k1")
	s.SetSummary("k1", "test summary")
	if got := s.GetSummary("k1"); got != "test summary" {
		t.Errorf("GetSummary = %q, want %q", got, "test summary")
	}
}

func TestInMemoryStore_Facts(t *testing.T) {
	s := NewInMemoryStore(Options{})
	s.GetOrCreate("k1")

	s.AddFacts("k1", []Fact{{Content: "f1"}, {Content: "f2"}})
	got := s.GetFacts("k1")
	if len(got) != 2 {
		t.Fatalf("GetFacts len = %d, want 2", len(got))
	}

	// Returns copy.
	got[0].Content = "mutated"
	check := s.GetFacts("k1")
	if check[0].Content == "mutated" {
		t.Error("GetFacts should return copy")
	}
}

func TestInMemoryStore_MessageCount(t *testing.T) {
	s := NewInMemoryStore(Options{})
	if s.MessageCount("k1") != 0 {
		t.Error("unknown key should return 0")
	}
	s.AddMessage("k1", Message{Role: "user", Content: "a"})
	s.AddMessage("k1", Message{Role: "assistant", Content: "b"})
	if s.MessageCount("k1") != 2 {
		t.Errorf("MessageCount = %d, want 2", s.MessageCount("k1"))
	}
}

func TestInMemoryStore_CompactMessages(t *testing.T) {
	s := NewInMemoryStore(Options{})
	for range 10 {
		s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	removed := s.CompactMessages("k1", 3)
	if len(removed) != 7 {
		t.Errorf("removed len = %d, want 7", len(removed))
	}
	if s.MessageCount("k1") != 3 {
		t.Errorf("after compact: MessageCount = %d, want 3", s.MessageCount("k1"))
	}
}

func TestInMemoryStore_TruncateHistory(t *testing.T) {
	s := NewInMemoryStore(Options{})
	for range 8 {
		s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}
	s.TruncateHistory("k1", 2)
	if s.MessageCount("k1") != 2 {
		t.Errorf("after truncate: %d, want 2", s.MessageCount("k1"))
	}
}

func TestInMemoryStore_Clear(t *testing.T) {
	s := NewInMemoryStore(Options{})
	s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	s.SetSummary("k1", "sum")
	s.Clear("k1")

	if s.MessageCount("k1") != 0 {
		t.Error("Clear should reset messages")
	}
	if s.GetSummary("k1") != "" {
		t.Error("Clear should reset summary")
	}
}

func TestInMemoryStore_Delete(t *testing.T) {
	s := NewInMemoryStore(Options{})
	s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	if err := s.Delete("k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.GetHistory("k1") != nil {
		t.Error("after Delete, GetHistory should return nil")
	}
}

func TestInMemoryStore_Save_Noop(t *testing.T) {
	s := NewInMemoryStore(Options{})
	if err := s.Save("k1"); err != nil {
		t.Errorf("Save should be no-op, got: %v", err)
	}
}

func TestInMemoryStore_ListStale(t *testing.T) {
	s := NewInMemoryStore(Options{})

	sess := s.GetOrCreate("old")
	sess.Updated = time.Now().Add(-2 * time.Hour)

	s.GetOrCreate("fresh")

	stale := s.ListStale(time.Hour)
	if len(stale) != 1 || stale[0] != "old" {
		t.Errorf("ListStale = %v, want [old]", stale)
	}
}

func TestInMemoryStore_TTL(t *testing.T) {
	s := NewInMemoryStore(Options{TTL: 50 * time.Millisecond})
	s.AddMessage("k1", Message{Role: "user", Content: "hi"})

	if s.GetHistory("k1") == nil {
		t.Fatal("should have history before TTL")
	}

	time.Sleep(60 * time.Millisecond)

	if msgs := s.GetHistory("k1"); msgs != nil {
		t.Errorf("after TTL: GetHistory should return nil, got %d msgs", len(msgs))
	}
}

func TestInMemoryStore_MaxMessages(t *testing.T) {
	s := NewInMemoryStore(Options{MaxMessages: 3})
	for range 5 {
		s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}
	if s.MessageCount("k1") != 3 {
		t.Errorf("MaxMessages: MessageCount = %d, want 3", s.MessageCount("k1"))
	}
}

func TestInMemoryStore_ConcurrentAddMessage(t *testing.T) {
	s := NewInMemoryStore(Options{})
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			s.AddMessage("k1", Message{Role: "user", Content: "msg"})
		})
	}
	wg.Wait()
	if s.MessageCount("k1") != 100 {
		t.Errorf("concurrent: MessageCount = %d, want 100", s.MessageCount("k1"))
	}
}

func TestInMemoryStore_ConcurrentGetHistoryDuringWrites(t *testing.T) {
	s := NewInMemoryStore(Options{})
	var wg sync.WaitGroup

	// Writer goroutines.
	for range 50 {
		wg.Go(func() {
			s.AddMessage("k1", Message{Role: "user", Content: "msg"})
		})
	}

	// Reader goroutines.
	for range 50 {
		wg.Go(func() {
			_ = s.GetHistory("k1")
		})
	}

	wg.Wait()
	// No panic = pass. Also verify data integrity.
	count := s.MessageCount("k1")
	if count != 50 {
		t.Errorf("after concurrent r/w: MessageCount = %d, want 50", count)
	}
}
