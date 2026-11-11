package skills_test

import (
	"os"
	"strings"
	"testing"
	"time"
)

// pinMtime forces path's modification time to ts.
func pinMtime(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}

// Switching the env var to a different file must serve that file's body even
// when both files carry the same mtime (and the same size).
func TestEmbedded_EnvPathChange_SameMtimeServesNewBody(t *testing.T) {
	e := newTestEmbedded(t)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	a := writeTemp(t, "---\nname: sample-skill\n---\nalpha body")
	b := writeTemp(t, "---\nname: sample-skill\n---\nbravo body")
	pinMtime(t, a, ts)
	pinMtime(t, b, ts)

	setEnv(t, envKey, a)
	if got := e.Body(); got != "alpha body" {
		t.Fatalf("Body() with path A = %q, want %q", got, "alpha body")
	}

	setEnv(t, envKey, b)
	if got := e.Body(); got != "bravo body" {
		t.Errorf("Body() after switching to path B (same mtime, same size) = %q, want %q", got, "bravo body")
	}
}

// A rewrite that keeps the mtime but changes the size must not be served from
// the cache.
func TestEmbedded_SamePathSameMtimeDifferentSize_ServesNewBody(t *testing.T) {
	e := newTestEmbedded(t)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	p := writeTemp(t, "short body")
	pinMtime(t, p, ts)
	setEnv(t, envKey, p)
	if got := e.Body(); got != "short body" {
		t.Fatalf("first Body() = %q", got)
	}

	if err := os.WriteFile(p, []byte("a much longer replacement body"), 0o600); err != nil {
		t.Fatal(err)
	}
	pinMtime(t, p, ts)
	if got := e.Body(); got != "a much longer replacement body" {
		t.Errorf("Body() after same-mtime rewrite = %q, want the new body", got)
	}
}

// Last-known-good applies to the path it was read from. When the operator
// points the env var at a file that cannot be read, the previous path's body
// must not be served as if it were the new file.
func TestEmbedded_EnvPathChange_UnreadableNewPathDoesNotServeOldPathBody(t *testing.T) {
	e := newTestEmbedded(t)

	a := writeTemp(t, "old path body")
	setEnv(t, envKey, a)
	if got := e.Body(); got != "old path body" {
		t.Fatalf("first Body() = %q", got)
	}

	setEnv(t, envKey, a+".missing")
	got := e.Body()
	if got == "old path body" {
		t.Errorf("Body() for an unreadable new path served the previous path's cached body")
	}
	if !strings.Contains(got, "This is the sample body") {
		t.Errorf("Body() = %q, want the embedded default", got)
	}
}

// Same path going transiently unreadable still returns last-known-good.
func TestEmbedded_SamePathUnreadable_ServesLastKnownGood(t *testing.T) {
	e := newTestEmbedded(t)

	p := writeTemp(t, "stable body")
	setEnv(t, envKey, p)
	if got := e.Body(); got != "stable body" {
		t.Fatalf("first Body() = %q", got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if got := e.Body(); got != "stable body" {
		t.Errorf("Body() after the same path vanished = %q, want last-known-good", got)
	}
}
