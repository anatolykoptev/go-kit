package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDefaultFile_ExistingFileKeptByteForByte(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SOUL.md")
	original := []byte("user edit that must survive")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}

	created, err := createDefaultFile(path, []byte("DEFAULT"))
	if err != nil {
		t.Fatalf("createDefaultFile on an existing file: err=%v, want nil", err)
	}
	if created {
		t.Error("createDefaultFile reported created=true for an existing file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("existing file changed: got %q, want %q", got, original)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
		t.Errorf("existing file mode changed to %04o", fi.Mode().Perm())
	}
}

func TestCreateDefaultFile_NewFileWrittenWithOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SOUL.md")

	created, err := createDefaultFile(path, []byte("DEFAULT"))
	if err != nil || !created {
		t.Fatalf("createDefaultFile = (%v, %v), want (true, nil)", created, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "DEFAULT" {
		t.Errorf("content = %q, want DEFAULT", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != filePerm {
		t.Errorf("mode = %04o, want %04o", fi.Mode().Perm(), filePerm)
	}
}
