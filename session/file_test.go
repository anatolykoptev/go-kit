package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})

	s.AddMessage("chat1", Message{Role: "user", Content: "hello"})
	s.SetSummary("chat1", "test summary")
	if err := s.Save("chat1"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	// Load into new store.
	s2 := NewFileStore(dir, Options{})
	msgs := s2.GetHistory("chat1")
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Errorf("loaded messages = %v, want [{user hello}]", msgs)
	}
	if got := s2.GetSummary("chat1"); got != "test summary" {
		t.Errorf("loaded summary = %q, want %q", got, "test summary")
	}
}

func TestFileStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})

	s.AddMessage("k1", Message{Role: "user", Content: "data"})
	if err := s.Save("k1"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No .tmp files should remain.
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("temp file should not remain: %s", f.Name())
		}
	}
}

func TestFileStore_LoadSkipsCorrupt(t *testing.T) {
	dir := t.TempDir()

	// Write corrupt JSON.
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	// Write valid session.
	validStore := NewFileStore(t.TempDir(), Options{})
	validStore.AddMessage("good", Message{Role: "user", Content: "ok"})
	_ = validStore.Save("good")
	// Copy valid file.
	data, _ := os.ReadFile(filepath.Join(validStore.(*FileStore).dir, "good.json"))
	_ = os.WriteFile(filepath.Join(dir, "good.json"), data, 0600)

	s := NewFileStore(dir, Options{})
	if s.GetHistory("good") == nil {
		t.Error("valid session should load")
	}
	// bad session should be silently skipped.
}

func TestFileStore_LoadSkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0600)

	s := NewFileStore(dir, Options{})
	// Should not panic or error.
	_ = s.GetHistory("readme")
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})
	s.AddMessage("k1", Message{Role: "user", Content: "data"})
	_ = s.Save("k1")

	if err := s.Delete("k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if s.GetHistory("k1") != nil {
		t.Error("after Delete, GetHistory should return nil")
	}

	// File should be gone.
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.Name() == "k1.json" {
			t.Error("file should be deleted from disk")
		}
	}
}

func TestFileStore_SanitizesKey(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})
	s.AddMessage("user/chat:123", Message{Role: "user", Content: "hi"})
	if err := s.Save("user/chat:123"); err != nil {
		t.Fatalf("Save with special chars: %v", err)
	}

	// Should create a sanitized filename (no / or :).
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	name := files[0].Name()
	for _, c := range []string{"/", ":"} {
		if filepath.Base(name) != name {
			t.Errorf("filename contains path separator: %s", name)
		}
		_ = c
	}
}

func TestFileStore_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})
	// Should not panic.
	if s.GetHistory("k1") != nil {
		t.Error("empty dir: should return nil")
	}
}

func TestFileStore_ConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir, Options{})

	for range 10 {
		s.AddMessage("k1", Message{Role: "user", Content: "msg"})
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_ = s.Save("k1")
		})
	}
	wg.Wait()

	// Verify data integrity after concurrent saves.
	s2 := NewFileStore(dir, Options{})
	if s2.MessageCount("k1") != 10 {
		t.Errorf("after concurrent save: MessageCount = %d, want 10", s2.MessageCount("k1"))
	}
}
