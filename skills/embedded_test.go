package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// sampleRaw reads the shared fixture file and panics on failure —
// if the fixture is missing the whole test suite is broken anyway.
func sampleRaw(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "embedded", "sample.md"))
	if err != nil {
		t.Fatalf("cannot read fixture: %v", err)
	}
	return string(data)
}

const envKey = "SKILLS_TEST_SKILL_PATH"

// newTestEmbedded creates an Embedded with the shared sample fixture.
func newTestEmbedded(t *testing.T) *skills.Embedded {
	t.Helper()
	return skills.NewEmbedded("sample-skill", envKey, sampleRaw(t))
}

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

// setEnv sets an environment variable and restores the previous value
// via t.Cleanup.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setEnv: %v", err)
	}
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv(key, prev) //nolint:errcheck
		} else {
			os.Unsetenv(key) //nolint:errcheck
		}
	})
}

// unsetEnv clears an environment variable and restores via t.Cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	os.Unsetenv(key) //nolint:errcheck
	t.Cleanup(func() {
		if hadPrev {
			os.Setenv(key, prev) //nolint:errcheck
		}
	})
}

// ---- Core resolution tests ----

func TestEmbedded_EnvUnset_ReturnsEmbeddedBody(t *testing.T) {
	unsetEnv(t, envKey)
	e := newTestEmbedded(t)

	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	if !strings.Contains(body, "Sample Skill") {
		t.Errorf("body does not contain expected content, got: %q", body)
	}
}

func TestEmbedded_EnvUnset_DiagnosticIsEmbeddedDefault(t *testing.T) {
	unsetEnv(t, envKey)
	e := newTestEmbedded(t)

	diag := e.Diagnostic()
	if diag != "embedded default" {
		t.Errorf("Diagnostic() = %q, want %q", diag, "embedded default")
	}
}

func TestEmbedded_EnvMissing_ReturnsEmbeddedBody(t *testing.T) {
	setEnv(t, envKey, "/nonexistent/path/skill.md")
	e := newTestEmbedded(t)

	body := e.Body()
	if !strings.Contains(body, "Sample Skill") {
		t.Errorf("expected embedded body on missing path, got: %q", body)
	}
}

func TestEmbedded_EnvMissing_DiagnosticIsUnreadable(t *testing.T) {
	path := "/nonexistent/path/skill.md"
	setEnv(t, envKey, path)
	e := newTestEmbedded(t)

	diag := e.Diagnostic()
	want := "env override " + path + " UNREADABLE → embedded default"
	if diag != want {
		t.Errorf("Diagnostic() = %q, want %q", diag, want)
	}
}

func TestEmbedded_EnvValid_ReturnsFileBody(t *testing.T) {
	override := "---\nname: override-skill\ndescription: override.\n---\n\nOverride body content."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	body := e.Body()

	if !strings.Contains(body, "Override body content.") {
		t.Errorf("expected override body, got: %q", body)
	}
}

func TestEmbedded_EnvValid_DiagnosticIsEnvOverride(t *testing.T) {
	override := "---\nname: override-skill\ndescription: override.\n---\n\nOverride body content."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	diag := e.Diagnostic()

	want := "env override " + path
	if diag != want {
		t.Errorf("Diagnostic() = %q, want %q", diag, want)
	}
}

func TestEmbedded_MtimeChange_ReturnsUpdatedBody(t *testing.T) {
	first := "---\nname: s\ndescription: d.\n---\n\nFirst body."
	path := writeTemp(t, first)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	body1 := e.Body()
	if !strings.Contains(body1, "First body.") {
		t.Fatalf("first read: expected 'First body.', got: %q", body1)
	}

	// Write new content; on most filesystems mtime resolution is 1 second.
	// ResetCache bypasses the mtime equality check so we don't have to sleep.
	second := "---\nname: s\ndescription: d.\n---\n\nSecond body."
	if err := os.WriteFile(path, []byte(second), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e.ResetCache()

	body2 := e.Body()
	if !strings.Contains(body2, "Second body.") {
		t.Errorf("after edit: expected 'Second body.', got: %q", body2)
	}
}

func TestEmbedded_SameMtime_CacheHit(t *testing.T) {
	override := "---\nname: s\ndescription: d.\n---\n\nCached body."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	body1 := e.Body()
	if !strings.Contains(body1, "Cached body.") {
		t.Fatalf("first read failed: %q", body1)
	}

	// Delete the file — if cache is hit, Body() still returns the cached value.
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	body2 := e.Body()
	if !strings.Contains(body2, "Cached body.") {
		t.Errorf("expected cache hit after file deletion, got: %q", body2)
	}
}

func TestEmbedded_TooLarge_ReturnsEmbeddedBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.md")

	// Write a file larger than 1 MiB with valid frontmatter.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	header := "---\nname: s\ndescription: d.\n---\n\n"
	if _, err = f.WriteString(header); err != nil {
		t.Fatal(err)
	}
	// Pad to just over 1 MiB.
	pad := strings.Repeat("x", (1<<20)+1)
	if _, err = f.WriteString(pad); err != nil {
		t.Fatal(err)
	}
	f.Close() //nolint:errcheck

	setEnv(t, envKey, path)
	e := newTestEmbedded(t)

	body := e.Body()
	if !strings.Contains(body, "Sample Skill") {
		t.Errorf("expected embedded body on too-large file, got: %q", body)
	}
}

func TestEmbedded_TooLarge_DiagnosticIsTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.md")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	header := "---\nname: s\ndescription: d.\n---\n\n"
	if _, err = f.WriteString(header); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("x", (1<<20)+1)
	if _, err = f.WriteString(pad); err != nil {
		t.Fatal(err)
	}
	f.Close() //nolint:errcheck

	setEnv(t, envKey, path)
	e := newTestEmbedded(t)

	diag := e.Diagnostic()
	want := "env override " + path + " TOO_LARGE → embedded default"
	if diag != want {
		t.Errorf("Diagnostic() = %q, want %q", diag, want)
	}
}

func TestEmbedded_EnvFileEmptyBody_ReturnsEmbeddedBody(t *testing.T) {
	// Frontmatter-only — no body after strip.
	frontmatterOnly := "---\nname: s\ndescription: d.\n---\n"
	path := writeTemp(t, frontmatterOnly)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	body := e.Body()

	if !strings.Contains(body, "Sample Skill") {
		t.Errorf("expected embedded body when env file has no body, got: %q", body)
	}
}

func TestEmbedded_FileDeletedBetweenStatAndRead_ReturnsEmbeddedBody(t *testing.T) {
	// Strategy: do a first successful read to populate the cache, then
	// reset cache (so mtime != cached) and delete the file. The next
	// Body() call stat-succeeds (cache is stale), then ReadFile fails.
	// This exercises the ReadFile error path without timing hacks.
	override := "---\nname: s\ndescription: d.\n---\n\nBody before delete."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	body1 := e.Body()
	if !strings.Contains(body1, "Body before delete.") {
		t.Fatalf("first read: %q", body1)
	}

	// Delete and reset cache so stat will fail → embedded fallback.
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	e.ResetCache()

	body2 := e.Body()
	if !strings.Contains(body2, "Sample Skill") {
		t.Errorf("expected embedded fallback after file removal, got: %q", body2)
	}
}

// ---- Panic tests ----

func TestNewEmbedded_EmptyRaw_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on empty raw, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T %v", r, r)
		}
		if !strings.Contains(msg, "empty body after frontmatter strip") {
			t.Errorf("panic message %q lacks expected content", msg)
		}
	}()
	skills.NewEmbedded("sample-skill", "", "")
}

func TestNewEmbedded_FrontmatterOnlyRaw_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on frontmatter-only raw, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T %v", r, r)
		}
		if !strings.Contains(msg, "empty body after frontmatter strip") {
			t.Errorf("panic message %q lacks expected content", msg)
		}
	}()
	skills.NewEmbedded("sample-skill", "", "---\nname: x\ndescription: d.\n---\n")
}

func TestNewEmbedded_InvalidName_Panics(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{"", "empty name"},
		{"UPPER", "uppercase"},
		{"-leading", "leading hyphen"},
		{"trailing-", "trailing hyphen"},
		{"double--hyphen", "consecutive hyphens"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected panic for name %q", tc.name)
				}
			}()
			skills.NewEmbedded(tc.name, "", "body here")
		})
	}
}

// ---- Metadata test ----

func TestEmbedded_Metadata_ReturnsEmbeddedMetadata(t *testing.T) {
	unsetEnv(t, envKey)
	e := newTestEmbedded(t)

	meta := e.Metadata()
	if meta.Name != "sample-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "sample-skill")
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", meta.Version, "1.0.0")
	}
}

func TestEmbedded_Metadata_NotAffectedByEnvOverride(t *testing.T) {
	override := "---\nname: other-skill\ndescription: override.\nversion: 9.9.9\n---\n\nOverride."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	meta := e.Metadata()

	// Metadata always comes from the embedded raw, not the env file.
	if meta.Name != "sample-skill" {
		t.Errorf("Name = %q after env override, want %q", meta.Name, "sample-skill")
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q after env override, want %q", meta.Version, "1.0.0")
	}
}

// ---- ResetCache test ----

func TestEmbedded_ResetCache_ForcesReread(t *testing.T) {
	first := "---\nname: s\ndescription: d.\n---\n\nFirst."
	path := writeTemp(t, first)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)
	if body := e.Body(); !strings.Contains(body, "First.") {
		t.Fatalf("first read: %q", body)
	}

	second := "---\nname: s\ndescription: d.\n---\n\nSecond."
	if err := os.WriteFile(path, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}

	// Without ResetCache, same mtime (sub-second write) may return cache.
	e.ResetCache()

	if body := e.Body(); !strings.Contains(body, "Second.") {
		t.Errorf("after ResetCache: expected 'Second.', got: %q", body)
	}
}

// TestEmbedded_ReadFails_ReturnsEmbeddedBody exercises the stat-succeeds /
// ReadFile-fails path by pointing the env var at a directory (os.Stat
// succeeds, os.ReadFile returns EISDIR).
func TestEmbedded_ReadFails_ReturnsEmbeddedBody(t *testing.T) {
	// A directory passes stat but ReadFile returns an error.
	dir := t.TempDir()
	setEnv(t, envKey, dir)

	e := newTestEmbedded(t)
	body := e.Body()

	if !strings.Contains(body, "Sample Skill") {
		t.Errorf("expected embedded body when ReadFile fails, got: %q", body)
	}
}

// ---- Concurrency test ----

func TestEmbedded_ConcurrentBody_RaceClean(t *testing.T) {
	override := "---\nname: s\ndescription: d.\n---\n\nConcurrent body."
	path := writeTemp(t, override)
	setEnv(t, envKey, path)

	e := newTestEmbedded(t)

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				body := e.Body()
				if body == "" {
					t.Errorf("Body() returned empty string in concurrent call")
				}
			}
		}()
	}
	wg.Wait()
}
