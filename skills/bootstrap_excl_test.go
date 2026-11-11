package skills_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// os.Stat reports a dangling symlink as "not exist", which is the same view
// InitWorkspace has of a file another initializer creates between the stat and
// the write. An exclusive create must treat the entry as present and leave it
// alone; a plain truncating write follows the link and creates its target.
func TestInitWorkspace_DanglingSymlinkIsKeptNotWrittenThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere.md")
	link := filepath.Join(dir, "SOUL.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	skills.InitWorkspace(dir, map[string][]byte{"SOUL.md": []byte("DEFAULT")})

	if _, err := os.Lstat(target); err == nil {
		t.Errorf("InitWorkspace wrote through the existing entry and created %s", target)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("existing symlink entry was replaced: err=%v", err)
	}
}
