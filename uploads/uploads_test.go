package uploads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoot_DefaultUsesHome(t *testing.T) {
	t.Setenv(EnvRoot, "")
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no $HOME on this system")
	}
	got := Root()
	want := filepath.Join(home, DefaultRootRel)
	if got != want {
		t.Errorf("Root() default = %q, want %q", got, want)
	}
}

func TestRoot_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvRoot, dir)
	if got := Root(); got != dir {
		t.Errorf("Root() = %q, want %q", got, dir)
	}
}

func TestService_CreatesDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)
	dir, err := Service("test-svc")
	if err != nil {
		t.Fatalf("Service: %v", err)
	}
	want := filepath.Join(root, "test-svc")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestService_EmptyName(t *testing.T) {
	if _, err := Service(""); err == nil || !strings.Contains(err.Error(), "empty service name") {
		t.Errorf("expected empty-service error, got %v", err)
	}
}

func TestBucket_CreatesDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)
	dir, err := Bucket("svc", "b1")
	if err != nil {
		t.Fatalf("Bucket: %v", err)
	}
	want := filepath.Join(root, "svc", "b1")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

func TestBucket_EmptyBucketFallsBackToService(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)
	dir, err := Bucket("svc", "")
	if err != nil {
		t.Fatalf("Bucket: %v", err)
	}
	want := filepath.Join(root, "svc")
	if dir != want {
		t.Errorf("empty-bucket fallback: got %q, want %q", dir, want)
	}
}

func TestPath_JoinsAndCreates(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvRoot, root)
	got, err := Path("svc", "b", "file.png")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(root, "svc", "b", "file.png")
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	parent := filepath.Dir(got)
	if _, err := os.Stat(parent); err != nil {
		t.Errorf("parent not created: %v", err)
	}
}
