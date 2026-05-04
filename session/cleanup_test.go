package session

import (
	"testing"
	"time"
)

func TestCleanup_FindsStale(t *testing.T) {
	store := NewInMemoryStore(Options{})

	old := store.GetOrCreate("old")
	old.Updated = time.Now().Add(-48 * time.Hour)

	store.GetOrCreate("fresh")

	stale := store.ListStale(24 * time.Hour)
	if len(stale) != 1 || stale[0] != "old" {
		t.Errorf("ListStale = %v, want [old]", stale)
	}
}

func TestCleanup_ArchiveCallbackBeforeDelete(t *testing.T) {
	store := NewInMemoryStore(Options{})

	store.AddMessage("old", Message{Role: "user", Content: "important"})
	old := store.GetOrCreate("old")
	old.Updated = time.Now().Add(-48 * time.Hour)

	var archived []string
	archiveFn := func(s *Session) error {
		archived = append(archived, s.Key)
		return nil
	}

	cleaned := Cleanup(store, 24*time.Hour, archiveFn)
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1", cleaned)
	}
	if len(archived) != 1 || archived[0] != "old" {
		t.Errorf("archived = %v, want [old]", archived)
	}
	if store.GetHistory("old") != nil {
		t.Error("stale session should be deleted")
	}
}

func TestCleanup_PreservesFresh(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.AddMessage("fresh", Message{Role: "user", Content: "hi"})

	cleaned := Cleanup(store, 24*time.Hour, nil)
	if cleaned != 0 {
		t.Errorf("cleaned = %d, want 0", cleaned)
	}
	if store.GetHistory("fresh") == nil {
		t.Error("fresh session should be preserved")
	}
}

func TestCleanup_ArchiveErrorDoesNotPreventDeletion(t *testing.T) {
	store := NewInMemoryStore(Options{})

	old := store.GetOrCreate("old")
	old.Updated = time.Now().Add(-48 * time.Hour)

	archiveFn := func(_ *Session) error {
		return errForTest
	}

	cleaned := Cleanup(store, 24*time.Hour, archiveFn)
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1 (should delete even on archive error)", cleaned)
	}
}

func TestCleanup_ZeroRetention(t *testing.T) {
	store := NewInMemoryStore(Options{})
	store.AddMessage("k1", Message{Role: "user", Content: "hi"})

	// Zero maxAge = disable cleanup.
	cleaned := Cleanup(store, 0, nil)
	if cleaned != 0 {
		t.Errorf("zero retention: cleaned = %d, want 0", cleaned)
	}
}

var errForTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test archive error" }
