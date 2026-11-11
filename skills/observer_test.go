package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// recorder — stub Observer that captures all events
// ---------------------------------------------------------------------------

type recorder struct {
	bodyCalls    []string // "name:source"
	envFallbacks []string // "name:reason"
	bodyBytes    []int
	catalogLoads []string // "name:outcome"
	catalogSizes []string // "tier:count"
}

func (r *recorder) observer() *Observer {
	return &Observer{
		BodyCall:    func(name, source string) { r.bodyCalls = append(r.bodyCalls, name+":"+source) },
		EnvFallback: func(name, reason string) { r.envFallbacks = append(r.envFallbacks, name+":"+reason) },
		BodyBytes:   func(name string, bytes int) { r.bodyBytes = append(r.bodyBytes, bytes) },
		CatalogLoad: func(name, outcome string) { r.catalogLoads = append(r.catalogLoads, name+":"+outcome) },
		CatalogSize: func(tier string, count int) { r.catalogSizes = append(r.catalogSizes, tier+":"+itoa(count)) },
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const obsEnvKey = "SKILLS_OBS_TEST_PATH"

// sampleObsRaw is a minimal valid skill string used in observer tests.
const sampleObsRaw = "---\nname: d10\ndescription: Test skill.\n---\n\nSample body for observer tests."

// newObsEmbedded creates an Embedded with the recorder's observer attached.
func newObsEmbedded(rec *recorder) *Embedded {
	return NewEmbedded("d10", obsEnvKey, sampleObsRaw, WithObserver(rec.observer()))
}

// writeTempSkill writes content to a temp file and returns its path.
// Duplicated here (package-level, not exported) to avoid coupling with
// the _test package helpers.
func writeTempSkill(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTempSkill: %v", err)
	}
	return path
}

// setObsEnv sets obsEnvKey and restores it on cleanup.
func setObsEnv(t *testing.T, val string) {
	t.Helper()
	prev, had := os.LookupEnv(obsEnvKey)
	if err := os.Setenv(obsEnvKey, val); err != nil {
		t.Fatalf("setObsEnv: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(obsEnvKey, prev) //nolint:errcheck
		} else {
			os.Unsetenv(obsEnvKey) //nolint:errcheck
		}
	})
}

// unsetObsEnv clears obsEnvKey and restores on cleanup.
func unsetObsEnv(t *testing.T) {
	t.Helper()
	prev, had := os.LookupEnv(obsEnvKey)
	os.Unsetenv(obsEnvKey) //nolint:errcheck
	t.Cleanup(func() {
		if had {
			os.Setenv(obsEnvKey, prev) //nolint:errcheck
		}
	})
}

// ---------------------------------------------------------------------------
// Embedded observer tests
// ---------------------------------------------------------------------------

func TestEmbedded_Observer_EmbeddedFastPath(t *testing.T) {
	unsetObsEnv(t)
	rec := &recorder{}
	e := newObsEmbedded(rec)

	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}

	if len(rec.bodyCalls) != 1 || rec.bodyCalls[0] != "d10:embedded" {
		t.Errorf("bodyCalls = %v, want [d10:embedded]", rec.bodyCalls)
	}
	if len(rec.bodyBytes) != 1 || rec.bodyBytes[0] != len(body) {
		t.Errorf("bodyBytes = %v, want [%d]", rec.bodyBytes, len(body))
	}
	if len(rec.envFallbacks) != 0 {
		t.Errorf("envFallbacks = %v, want none", rec.envFallbacks)
	}
}

func TestEmbedded_Observer_EnvSuccess(t *testing.T) {
	content := "---\nname: d10\ndescription: Override.\n---\n\nOverride body content."
	path := writeTempSkill(t, content)
	setObsEnv(t, path)

	rec := &recorder{}
	e := newObsEmbedded(rec)

	body := e.Body()
	if !strings.Contains(body, "Override body content.") {
		t.Fatalf("unexpected body: %q", body)
	}

	if len(rec.bodyCalls) != 1 || rec.bodyCalls[0] != "d10:env" {
		t.Errorf("bodyCalls = %v, want [d10:env]", rec.bodyCalls)
	}
	if len(rec.bodyBytes) != 1 || rec.bodyBytes[0] != len(body) {
		t.Errorf("bodyBytes = %v, want [%d]", rec.bodyBytes, len(body))
	}
	if len(rec.envFallbacks) != 0 {
		t.Errorf("envFallbacks = %v, want none", rec.envFallbacks)
	}
}

func TestEmbedded_Observer_CacheHit(t *testing.T) {
	content := "---\nname: d10\ndescription: Override.\n---\n\nCached body."
	path := writeTempSkill(t, content)
	setObsEnv(t, path)

	rec := &recorder{}
	e := newObsEmbedded(rec)

	// First call: env read (miss)
	body1 := e.Body()
	if !strings.Contains(body1, "Cached body.") {
		t.Fatalf("first call: %q", body1)
	}

	// Second call: same mtime → cache_hit
	body2 := e.Body()
	if body2 != body1 {
		t.Errorf("second call body changed: %q", body2)
	}

	if len(rec.bodyCalls) != 2 {
		t.Fatalf("bodyCalls count = %d, want 2; calls = %v", len(rec.bodyCalls), rec.bodyCalls)
	}
	if rec.bodyCalls[0] != "d10:env" {
		t.Errorf("first call = %q, want d10:env", rec.bodyCalls[0])
	}
	if rec.bodyCalls[1] != "d10:cache_hit" {
		t.Errorf("second call = %q, want d10:cache_hit", rec.bodyCalls[1])
	}
}

func TestEmbedded_Observer_LastKnownGood(t *testing.T) {
	content := "---\nname: d10\ndescription: Override.\n---\n\nLast known good body."
	path := writeTempSkill(t, content)
	setObsEnv(t, path)

	rec := &recorder{}
	e := newObsEmbedded(rec)

	// First call: populate cache
	body1 := e.Body()
	if !strings.Contains(body1, "Last known good body.") {
		t.Fatalf("first call: %q", body1)
	}

	// Delete the file, then reset cache so stat fails next call
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// ResetCache clears mtime but keeps cachedBody —
	// however stat will fail before mtime is checked, so last_known_good fires
	// from the cached body set during first call.
	// We do NOT call ResetCache here: the file is deleted, so stat fails
	// → last_known_good path (cachedBody is still set from call 1).

	body2 := e.Body()
	if !strings.Contains(body2, "Last known good body.") {
		t.Errorf("second call: expected last_known_good body, got: %q", body2)
	}

	// Second call should have BodyCall "d10:last_known_good"
	if len(rec.bodyCalls) < 2 || rec.bodyCalls[1] != "d10:last_known_good" {
		t.Errorf("bodyCalls = %v, want second entry d10:last_known_good", rec.bodyCalls)
	}
	// Second call should have EnvFallback "d10:unreadable"
	if len(rec.envFallbacks) < 1 || rec.envFallbacks[0] != "d10:unreadable" {
		t.Errorf("envFallbacks = %v, want [d10:unreadable]", rec.envFallbacks)
	}
}

func TestEmbedded_Observer_TooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.md")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	header := "---\nname: d10\ndescription: d.\n---\n\n"
	if _, err = f.WriteString(header); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("x", (1<<20)+1)
	if _, err = f.WriteString(pad); err != nil {
		t.Fatal(err)
	}
	f.Close() //nolint:errcheck

	setObsEnv(t, path)
	rec := &recorder{}
	e := newObsEmbedded(rec)

	body := e.Body()
	if strings.Contains(body, "x") {
		t.Errorf("expected embedded body, not override; got prefix: %.20q", body)
	}

	if len(rec.envFallbacks) != 1 || rec.envFallbacks[0] != "d10:too_large" {
		t.Errorf("envFallbacks = %v, want [d10:too_large]", rec.envFallbacks)
	}
	if len(rec.bodyCalls) != 1 || rec.bodyCalls[0] != "d10:embedded" {
		t.Errorf("bodyCalls = %v, want [d10:embedded]", rec.bodyCalls)
	}
}

func TestEmbedded_Observer_EmptyBody(t *testing.T) {
	// Frontmatter-only file — no body after strip.
	content := "---\nname: d10\ndescription: d.\n---\n"
	path := writeTempSkill(t, content)
	setObsEnv(t, path)

	rec := &recorder{}
	e := newObsEmbedded(rec)

	body := e.Body()
	if !strings.Contains(body, "Sample body") {
		t.Errorf("expected embedded body; got: %q", body)
	}

	if len(rec.envFallbacks) != 1 || rec.envFallbacks[0] != "d10:empty_body" {
		t.Errorf("envFallbacks = %v, want [d10:empty_body]", rec.envFallbacks)
	}
	if len(rec.bodyCalls) != 1 || rec.bodyCalls[0] != "d10:embedded" {
		t.Errorf("bodyCalls = %v, want [d10:embedded]", rec.bodyCalls)
	}
}

func TestEmbedded_Observer_NilSafe(t *testing.T) {
	// Observer with all nil fields — must not panic on Body().
	unsetObsEnv(t)
	obs := &Observer{} // all func fields are nil
	e := NewEmbedded("d10", obsEnvKey, sampleObsRaw, WithObserver(obs))

	// Must not panic
	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestEmbedded_Observer_NoObserver(t *testing.T) {
	// NewEmbedded without WithObserver — no panic, no callbacks.
	unsetObsEnv(t)
	e := NewEmbedded("d10", obsEnvKey, sampleObsRaw)

	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	// If we got here without panic, the no-observer path is clean.
}

func TestEmbedded_Observer_NilObserverWithObserver(t *testing.T) {
	// WithObserver(nil) must be a no-op (no panic, no hooks called).
	unsetObsEnv(t)
	e := NewEmbedded("d10", obsEnvKey, sampleObsRaw, WithObserver(nil))

	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
}

// ---------------------------------------------------------------------------
// Catalog observer tests
// ---------------------------------------------------------------------------

func TestCatalog_Observer_LoadHit(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recorder{}
	cat = cat.WithObserver(rec.observer())

	_, _, ok := cat.Load("foo")
	if !ok {
		t.Fatal("Load(foo) returned ok=false")
	}

	if len(rec.catalogLoads) != 1 || rec.catalogLoads[0] != "foo:hit" {
		t.Errorf("catalogLoads = %v, want [foo:hit]", rec.catalogLoads)
	}
}

func TestCatalog_Observer_LoadMiss(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recorder{}
	cat = cat.WithObserver(rec.observer())

	_, _, ok := cat.Load("nonexistent-skill")
	if ok {
		t.Fatal("Load(nonexistent) returned ok=true")
	}

	if len(rec.catalogLoads) != 1 || rec.catalogLoads[0] != "nonexistent-skill:miss" {
		t.Errorf("catalogLoads = %v, want [nonexistent-skill:miss]", rec.catalogLoads)
	}
}

func TestCatalog_Observer_SizeOnAttach(t *testing.T) {
	// builtin has foo and bar → count = 2
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recorder{}
	cat.WithObserver(rec.observer())

	// CatalogSize should have fired once for the "builtin" tier at attach time.
	if len(rec.catalogSizes) != 1 {
		t.Fatalf("catalogSizes count = %d, want 1; values = %v", len(rec.catalogSizes), rec.catalogSizes)
	}
	// builtin tier has foo + bar → "builtin:2"
	if rec.catalogSizes[0] != "builtin:2" {
		t.Errorf("catalogSizes[0] = %q, want \"builtin:2\"", rec.catalogSizes[0])
	}
}

func TestCatalog_Observer_SizeOnAttach_MultipleTiers(t *testing.T) {
	cat := NewCatalog(
		NewDirTier("workspace", filepath.Join("testdata/tier", "workspace")),
		NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")),
	)
	rec := &recorder{}
	cat.WithObserver(rec.observer())

	if len(rec.catalogSizes) != 2 { //nolint:mnd
		t.Fatalf("catalogSizes count = %d, want 2; values = %v", len(rec.catalogSizes), rec.catalogSizes)
	}
	// workspace has only foo → "workspace:1", builtin has foo+bar → "builtin:2"
	if rec.catalogSizes[0] != "workspace:1" {
		t.Errorf("catalogSizes[0] = %q, want \"workspace:1\"", rec.catalogSizes[0])
	}
	if rec.catalogSizes[1] != "builtin:2" {
		t.Errorf("catalogSizes[1] = %q, want \"builtin:2\"", rec.catalogSizes[1])
	}
}

func TestCatalog_Observer_NilObserver(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	// WithObserver(nil) must return receiver unchanged (no panic).
	cat2 := cat.WithObserver(nil)
	if cat2 != cat {
		t.Error("WithObserver(nil) should return receiver unchanged")
	}
}

func TestCatalog_Observer_LoadCtx_Hit(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recorder{}
	cat = cat.WithObserver(rec.observer())

	// Reset catalogSizes to focus on loads.
	rec.catalogSizes = nil

	body, _, ok, err := cat.LoadCtx(t.Context(), "foo")
	if err != nil || !ok {
		t.Fatalf("LoadCtx: err=%v ok=%v", err, ok)
	}
	if body == "" {
		t.Fatal("empty body")
	}

	if len(rec.catalogLoads) != 1 || rec.catalogLoads[0] != "foo:hit" {
		t.Errorf("catalogLoads = %v, want [foo:hit]", rec.catalogLoads)
	}
}

func TestCatalog_Observer_LoadCtx_Miss(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recorder{}
	cat = cat.WithObserver(rec.observer())
	rec.catalogSizes = nil

	_, _, ok, err := cat.LoadCtx(t.Context(), "ghost")
	if err != nil || ok {
		t.Fatalf("LoadCtx: err=%v ok=%v (want ok=false)", err, ok)
	}

	if len(rec.catalogLoads) != 1 || rec.catalogLoads[0] != "ghost:miss" {
		t.Errorf("catalogLoads = %v, want [ghost:miss]", rec.catalogLoads)
	}
}

func TestEmbedded_Observer_ReadFailWithCache_LastKnownGood(t *testing.T) {
	// First: populate cache via a real file.
	content := "---\nname: d10\ndescription: d.\n---\n\nCached via file."
	path := writeTempSkill(t, content)
	setObsEnv(t, path)

	rec := &recorder{}
	e := newObsEmbedded(rec)

	body1 := e.Body()
	if !strings.Contains(body1, "Cached via file.") {
		t.Fatalf("first call: %q", body1)
	}

	// Now point env at a directory: stat succeeds, ReadFile fails,
	// but cache is populated → last_known_good.
	dir := t.TempDir()
	setObsEnv(t, dir) // stat on dir succeeds; ReadFile(dir) → EISDIR

	// Reset mtime cache so it doesn't return cache_hit (mtime will differ).
	e.ResetCache()
	// ResetCache clears cachedBody too — but we need the cache to be populated
	// to test the last_known_good path after ReadFile failure. Re-call with the
	// directory only works if cachedBody is non-empty. We must re-prime it.
	// Strategy: do NOT call ResetCache. Instead rely on mtime mismatch.
	// The dir's mtime != the file's cached mtime, so it will proceed to ReadFile.
	// But we DID call ResetCache which cleared cachedBody.
	// So instead: re-prime cache by pointing at the original file again, then
	// redirect to the directory WITHOUT clearing cache.
	setObsEnv(t, path)                    // prime cache
	_ = e.Body()                          // populates cachedBody
	setObsEnv(t, dir)                     // now point at directory
	e.cachedMtime = e.cachedMtime.Add(-1) // force mtime mismatch to skip cache_hit path
	rec2 := &recorder{}
	e.observer = rec2.observer() // fresh recorder

	body2 := e.Body()
	if !strings.Contains(body2, "Cached via file.") {
		t.Errorf("expected last_known_good body, got: %q", body2)
	}
	if len(rec2.envFallbacks) != 1 || rec2.envFallbacks[0] != "d10:unreadable" {
		t.Errorf("envFallbacks = %v, want [d10:unreadable]", rec2.envFallbacks)
	}
	if len(rec2.bodyCalls) != 1 || rec2.bodyCalls[0] != "d10:last_known_good" {
		t.Errorf("bodyCalls = %v, want [d10:last_known_good]", rec2.bodyCalls)
	}
}

// ---------------------------------------------------------------------------
// Backward compat: existing 3-arg call still compiles and behaves identically
// ---------------------------------------------------------------------------

func TestNewEmbedded_BackwardCompat_ThreeArgs(t *testing.T) {
	// Ensure the variadic signature doesn't break existing call sites.
	e := NewEmbedded("d10", "", sampleObsRaw)
	body := e.Body()
	if body == "" {
		t.Fatal("expected non-empty body from 3-arg NewEmbedded")
	}
}
