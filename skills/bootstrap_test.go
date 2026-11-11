package skills_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

func TestInitWorkspace_EmptyDirDefaultsWritten(t *testing.T) {
	dir := t.TempDir()
	defaults := map[string][]byte{
		"foo.md":  []byte("hello"),
		"bar.txt": []byte("world"),
	}

	skills.InitWorkspace(dir, defaults)

	for key, want := range defaults {
		got, err := os.ReadFile(filepath.Join(dir, key))
		if err != nil {
			t.Fatalf("file %q not written: %v", key, err)
		}
		if string(got) != string(want) {
			t.Errorf("file %q content = %q, want %q", key, got, want)
		}
	}
}

func TestInitWorkspace_ExistingDirNoopAndFilesWritten(t *testing.T) {
	dir := t.TempDir()
	// dir already exists — MkdirAll must be a no-op
	defaults := map[string][]byte{
		"skill.md": []byte("content"),
	}

	skills.InitWorkspace(dir, defaults)

	got, err := os.ReadFile(filepath.Join(dir, "skill.md"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "content" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestInitWorkspace_ExistingFileNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	sentinel := []byte("SENTINEL")
	path := filepath.Join(dir, "existing.md")
	if err := os.WriteFile(path, sentinel, 0600); err != nil {
		t.Fatal(err)
	}

	defaults := map[string][]byte{
		"existing.md": []byte("OVERWRITE_ATTEMPT"),
	}

	skills.InitWorkspace(dir, defaults)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Errorf("file was overwritten: got %q, want %q", got, sentinel)
	}
}

func TestInitWorkspace_NestedKeyCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	defaults := map[string][]byte{
		"skills/foo/SKILL.md": []byte("nested"),
	}

	skills.InitWorkspace(dir, defaults)

	// Verify the file was written
	got, err := os.ReadFile(filepath.Join(dir, "skills/foo/SKILL.md"))
	if err != nil {
		t.Fatalf("nested file not written: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("nested file content = %q, want %q", got, "nested")
	}

	// Verify intermediate directories have mode 0750
	for _, sub := range []string{"skills", "skills/foo"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil {
			t.Fatalf("dir %q not created: %v", sub, err)
		}
		if perm := info.Mode().Perm(); perm != 0750 {
			t.Errorf("dir %q perm = %04o, want 0750", sub, perm)
		}
	}
}

func TestInitWorkspace_ReadOnlyParentNosPanic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions don't behave the same")
	}

	outer := t.TempDir()
	roDir := filepath.Join(outer, "readonly")
	if err := os.MkdirAll(roDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(roDir, 0700) //nolint:errcheck // best-effort cleanup for TempDir

	innerDir := filepath.Join(roDir, "workspace")
	defaults := map[string][]byte{
		"file.md": []byte("data"),
	}

	// Must not panic — best-effort UX
	skills.InitWorkspace(innerDir, defaults)
}

func TestInitWorkspace_EmptyDefaultsNoPanic(t *testing.T) {
	dir := t.TempDir()

	// nil map
	skills.InitWorkspace(dir, nil)

	// empty map
	skills.InitWorkspace(dir, map[string][]byte{})

	// dir must exist
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

func TestInitWorkspace_FileMode0600(t *testing.T) {
	dir := t.TempDir()
	defaults := map[string][]byte{
		"perm.md": []byte("check"),
	}

	skills.InitWorkspace(dir, defaults)

	info, err := os.Stat(filepath.Join(dir, "perm.md"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %04o, want 0600", perm)
	}
}

func TestInitWorkspace_DirMode0750(t *testing.T) {
	outer := t.TempDir()
	dir := filepath.Join(outer, "workspace")

	skills.InitWorkspace(dir, nil)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0750 {
		t.Errorf("dir perm = %04o, want 0750", perm)
	}
}

// TestInitWorkspace_StatErrorNotErrNotExist exercises the branch where
// os.Stat returns an error that is NOT fs.ErrNotExist. We create a file
// at "blocker" and then ask for "blocker/child" — stat on "blocker/child"
// returns ENOTDIR, which is not ErrNotExist.
func TestInitWorkspace_StatErrorNotErrNotExist(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file named "blocker"
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// Ask InitWorkspace to create "blocker/child" — parent of "blocker/child"
	// equals dir, so no extra MkdirAll is attempted; the stat of
	// "blocker/child" itself fails with ENOTDIR (not ErrNotExist).
	defaults := map[string][]byte{
		"blocker/child": []byte("data"),
	}

	// Must not panic; the bad entry is skipped via slog.Warn + continue.
	skills.InitWorkspace(dir, defaults)
}

// TestInitWorkspace_WriteFileFails exercises the WriteFile error branch.
// We create the workspace, write a file, then replace it with a directory
// so that WriteFile on that path fails (EISDIR on Linux).
func TestInitWorkspace_WriteFileFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions don't behave the same")
	}

	// Make the workspace read-only AFTER it is created so WriteFile on a
	// new file inside it fails (EACCES). This exercises the WriteFile error
	// branch (slog.Warn + continue).
	roDir := t.TempDir()
	// Pre-create the workspace so MkdirAll succeeds.
	workspace := filepath.Join(roDir, "ws")
	if err := os.Mkdir(workspace, 0750); err != nil {
		t.Fatal(err)
	}
	// Make workspace read-only so WriteFile on a new file inside it fails.
	if err := os.Chmod(workspace, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(workspace, 0700) //nolint:errcheck // best-effort cleanup

	defaults2 := map[string][]byte{
		"newfile.md": []byte("data"),
	}
	// Must not panic — WriteFile fails, slog.Warn is emitted, continue.
	skills.InitWorkspace(workspace, defaults2)
}

// TestInitWorkspace_NestedKeyUnwritableParentContinues exercises the
// MkdirAll(parent) error path for nested keys: the intermediate directory
// cannot be created, so the entry is skipped (continue), leaving other
// entries unaffected.
func TestInitWorkspace_NestedKeyUnwritableParentContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions don't behave the same")
	}

	dir := t.TempDir()
	// Make dir read-only so MkdirAll for nested parent inside it fails.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700) //nolint:errcheck // best-effort cleanup

	defaults := map[string][]byte{
		"sub/file.md": []byte("data"),
	}

	// Must not panic.
	skills.InitWorkspace(dir, defaults)
}
