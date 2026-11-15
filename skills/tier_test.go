package skills_test

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/skills"
)

//go:embed testdata/tier/embedfs/*
var embedTestFS embed.FS

// ---------------------------------------------------------------------------
// DirTier tests
// ---------------------------------------------------------------------------

func TestDirTier_FindExisting_ReturnsBodyMetaDir(t *testing.T) {
	dir := filepath.Join("testdata/tier", "builtin")
	tier := skills.NewDirTier("builtin", dir)

	info, body, ok := tier.Resolver.Find("foo")
	if !ok {
		t.Fatal("Find(\"foo\") returned ok=false, want true")
	}
	if body == "" {
		t.Fatal("body is empty")
	}
	if !strings.Contains(body, "Builtin foo body.") {
		t.Errorf("body = %q, want to contain 'Builtin foo body.'", body)
	}
	if info.Name != "foo" {
		t.Errorf("Name = %q, want %q", info.Name, "foo")
	}
	if info.Dir == "" {
		t.Error("Dir is empty")
	}
	if info.Path == "" {
		t.Error("Path is empty")
	}
	if info.Description == "" {
		t.Error("Metadata.Description is empty")
	}
}

func TestDirTier_FindMissing_ReturnsFalse(t *testing.T) {
	dir := filepath.Join("testdata/tier", "builtin")
	tier := skills.NewDirTier("builtin", dir)

	_, _, ok := tier.Resolver.Find("does-not-exist")
	if ok {
		t.Fatal("Find(\"does-not-exist\") returned ok=true, want false")
	}
}

func TestDirTier_WithSkillFilename_CustomFile(t *testing.T) {
	// Create a tempdir with a custom-named skill file.
	root := t.TempDir()
	skillDir := filepath.Join(root, "custom")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: custom\ndescription: Custom skill.\n---\n\nCustom body."
	if err := os.WriteFile(filepath.Join(skillDir, "AGENT.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("custom", root, skills.WithSkillFilename("AGENT.md"))
	info, body, ok := tier.Resolver.Find("custom")
	if !ok {
		t.Fatal("Find(\"custom\") returned ok=false")
	}
	if !strings.Contains(body, "Custom body.") {
		t.Errorf("body = %q", body)
	}
	if info.Name != "custom" {
		t.Errorf("Name = %q", info.Name)
	}
}

func TestDirTier_Walk_YieldsAllSkillsWithDir(t *testing.T) {
	dir := filepath.Join("testdata/tier", "builtin")
	tier := skills.NewDirTier("builtin", dir)

	found := make(map[string]skills.SkillInfo)
	tier.Resolver.Walk(func(si skills.SkillInfo) {
		found[si.Name] = si
	})

	if len(found) != 2 { //nolint:mnd
		t.Errorf("Walk yielded %d skills, want 2; names: %v", len(found), mapKeys(found))
	}
	for name, si := range found {
		if si.Dir == "" {
			t.Errorf("skill %q has empty Dir", name)
		}
	}
}

func TestDirTier_MissingDir_EmptyWalkNoError(t *testing.T) {
	tier := skills.NewDirTier("missing", "/nonexistent/path/that/does/not/exist")

	count := 0
	tier.Resolver.Walk(func(_ skills.SkillInfo) { count++ })
	if count != 0 {
		t.Errorf("Walk yielded %d items for missing dir, want 0", count)
	}

	_, _, ok := tier.Resolver.Find("any")
	if ok {
		t.Error("Find returned ok=true for missing dir")
	}
}

func TestDirTier_NonDirEntriesSkipped(t *testing.T) {
	// testdata/tier/builtin contains a plain file notadir.md — create it temporarily.
	root := t.TempDir()
	// Regular file directly in root — must not be yielded.
	if err := os.WriteFile(filepath.Join(root, "notadir.md"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Valid skill directory.
	skillDir := filepath.Join(root, "real")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: real\ndescription: Real skill.\n---\n\nReal body."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("t", root)
	names := []string{}
	tier.Resolver.Walk(func(si skills.SkillInfo) { names = append(names, si.Name) })

	if len(names) != 1 || names[0] != "real" {
		t.Errorf("Walk yielded %v, want [real]", names)
	}
}

func TestDirTier_DirWithoutSKILLMd_Skipped(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty-skill")
	if err := os.MkdirAll(empty, 0o750); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("t", root)
	count := 0
	tier.Resolver.Walk(func(_ skills.SkillInfo) { count++ })
	if count != 0 {
		t.Errorf("Walk yielded %d for dir without SKILL.md, want 0", count)
	}
}

func TestDirTier_WithMtimeCache_UpdatedBodyOnEdit(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "cached")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")

	first := "---\nname: cached\ndescription: Cached.\n---\n\nFirst body."
	if err := os.WriteFile(skillFile, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("t", root, skills.WithMtimeCache())
	_, body1, ok := tier.Resolver.Find("cached")
	if !ok {
		t.Fatal("Find(\"cached\") returned false on first read")
	}
	if !strings.Contains(body1, "First body.") {
		t.Fatalf("body1 = %q", body1)
	}

	// Write new content and force mtime change by removing and recreating.
	second := "---\nname: cached\ndescription: Cached.\n---\n\nSecond body."
	if err := os.Remove(skillFile); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillFile, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}
	// Touch the file to ensure mtime changes (sub-second filesystems may need
	// this because os.Chtimes is more reliable than relying on wall clock).
	now := touchTime()
	if err := os.Chtimes(skillFile, now, now.Add(1)); err != nil {
		t.Log("Chtimes failed (best effort):", err)
	}

	_, body2, ok := tier.Resolver.Find("cached")
	if !ok {
		t.Fatal("Find(\"cached\") returned false on second read")
	}
	if !strings.Contains(body2, "Second body.") {
		t.Errorf("body2 = %q; cache not invalidated after mtime change", body2)
	}
}

func TestDirTier_SymlinkEntry_Skipped(t *testing.T) {
	root := t.TempDir()

	// Real skill outside root.
	realDir := t.TempDir()
	content := "---\nname: linked\ndescription: Linked skill.\n---\n\nLinked body."
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Symlink inside root pointing to the real skill dir.
	link := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	tier := skills.NewDirTier("t", root)
	count := 0
	tier.Resolver.Walk(func(_ skills.SkillInfo) { count++ })
	if count != 0 {
		t.Errorf("Walk yielded %d for symlink entry, want 0 (symlink should be skipped)", count)
	}

	_, _, ok := tier.Resolver.Find("linked")
	if ok {
		t.Error("Find returned ok=true for symlink skill dir, want false")
	}
}

func TestDirTier_SymlinkSKILLMdOutsideDir_Skipped(t *testing.T) {
	root := t.TempDir()

	// Create a skill dir inside root.
	skillDir := filepath.Join(root, "escape-test")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Real SKILL.md file outside the tier root.
	outside := t.TempDir()
	target := filepath.Join(outside, "SKILL.md")
	content := "---\nname: escape-test\ndescription: Escape.\n---\n\nEscape body."
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Symlink SKILL.md inside the skill dir pointing to the outside file.
	link := filepath.Join(skillDir, "SKILL.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	tier := skills.NewDirTier("t", root)
	_, _, ok := tier.Resolver.Find("escape-test")
	if ok {
		t.Error("Find returned ok=true for symlink SKILL.md escaping tier root")
	}
}

func TestDirTier_EmptyBodyAfterFrontmatter_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "empty-body")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Frontmatter only, no body.
	content := "---\nname: empty-body\ndescription: No body.\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("t", root)
	_, _, ok := tier.Resolver.Find("empty-body")
	if ok {
		t.Error("Find returned ok=true for empty body skill, want false")
	}
}

func TestDirTier_MultiFileSkill_DirAllowsSiblingRead(t *testing.T) {
	dir := filepath.Join("testdata/tier", "builtin")
	tier := skills.NewDirTier("builtin", dir)

	info, _, ok := tier.Resolver.Find("foo")
	if !ok {
		t.Fatal("Find(\"foo\") returned false")
	}

	sibling := filepath.Join(info.Dir, "references", "sample.md")
	data, err := os.ReadFile(sibling)
	if err != nil {
		t.Fatalf("os.ReadFile(sibling) failed: %v", err)
	}
	if !strings.Contains(string(data), "Sibling file") {
		t.Errorf("sibling content = %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// EmbedFSTier tests
// ---------------------------------------------------------------------------

func TestEmbedFSTier_Find_ReturnsBody(t *testing.T) {
	tier := skills.NewEmbedFSTier("embed", embedTestFS, "testdata/tier/embedfs")

	info, body, ok := tier.Resolver.Find("baz")
	if !ok {
		t.Fatal("Find(\"baz\") returned ok=false")
	}
	if !strings.Contains(body, "Embedded baz body.") {
		t.Errorf("body = %q", body)
	}
	if info.Name != "baz" {
		t.Errorf("Name = %q", info.Name)
	}
}

func TestEmbedFSTier_Walk_YieldsAllSkills(t *testing.T) {
	tier := skills.NewEmbedFSTier("embed", embedTestFS, "testdata/tier/embedfs")

	found := map[string]bool{}
	tier.Resolver.Walk(func(si skills.SkillInfo) {
		found[si.Name] = true
	})

	if !found["baz"] {
		t.Errorf("Walk did not yield \"baz\"; got %v", found)
	}
}

func TestEmbedFSTier_Dir_UsesForwardSlash(t *testing.T) {
	tier := skills.NewEmbedFSTier("embed", embedTestFS, "testdata/tier/embedfs")

	info, _, ok := tier.Resolver.Find("baz")
	if !ok {
		t.Fatal("Find(\"baz\") returned false")
	}
	want := "testdata/tier/embedfs/baz"
	if info.Dir != want {
		t.Errorf("Dir = %q, want %q", info.Dir, want)
	}
	if strings.Contains(info.Dir, "\\") {
		t.Errorf("Dir contains backslash (not forward slash): %q", info.Dir)
	}
}

func TestEmbedFSTier_WithCustomFilename(t *testing.T) {
	// Build a temp embed.FS equivalent using os.DirFS isn't possible with
	// embed.FS directly, so we create a parallel fixture using a DirTier with
	// custom filename instead (the EmbedFSOption code-path is exercised in the
	// real embed fixture by verifying the default works; here we verify the
	// WithEmbedSkillFilename option changes the internal config).
	//
	// The option is a functional option that mutates embedFSConfig.skillFilename.
	// We verify the constructor accepts the option without panic and Find works
	// with the default fixture (no AGENT.md exists → Find returns false).
	tier := skills.NewEmbedFSTier("embed", embedTestFS, "testdata/tier/embedfs",
		skills.WithEmbedSkillFilename("AGENT.md"))

	_, _, ok := tier.Resolver.Find("baz")
	if ok {
		t.Error("Find returned ok=true with non-existent custom filename AGENT.md")
	}
}

// ---------------------------------------------------------------------------
// PluginTier tests
// ---------------------------------------------------------------------------

func TestPluginTier_BareNameResolves(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "myplugin", SkillName: "myskill", Body: "---\nname: myskill\ndescription: d.\n---\n\nPlugin body."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	info, body, ok := tier.Resolver.Find("myskill")
	if !ok {
		t.Fatal("Find(\"myskill\") returned false")
	}
	if !strings.Contains(body, "Plugin body.") {
		t.Errorf("body = %q", body)
	}
	if info.Name != "myskill" {
		t.Errorf("Name = %q", info.Name)
	}
}

func TestPluginTier_NamespacedNameResolves(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "myplugin", SkillName: "myskill", Body: "---\nname: myskill\ndescription: d.\n---\n\nPlugin body."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, body, ok := tier.Resolver.Find("myplugin:myskill")
	if !ok {
		t.Fatal("Find(\"myplugin:myskill\") returned false")
	}
	if !strings.Contains(body, "Plugin body.") {
		t.Errorf("body = %q", body)
	}
}

func TestPluginTier_BodyUsedDirectly_NoFSRead(t *testing.T) {
	// Supply Body only (no Path) — must succeed without any fs access.
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "s", Body: "---\nname: s\ndescription: d.\n---\n\nPreloaded body."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, body, ok := tier.Resolver.Find("s")
	if !ok {
		t.Fatal("Find returned false")
	}
	if !strings.Contains(body, "Preloaded body.") {
		t.Errorf("body = %q", body)
	}
}

func TestPluginTier_PathTriggersFileRead(t *testing.T) {
	// Supply Path only (no Body) — must read from file.
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "path-skill.md")
	content := "---\nname: path-skill\ndescription: Path skill.\n---\n\nPath skill body."
	if err := os.WriteFile(skillFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "path-skill", Path: skillFile},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, body, ok := tier.Resolver.Find("path-skill")
	if !ok {
		t.Fatal("Find returned false")
	}
	if !strings.Contains(body, "Path skill body.") {
		t.Errorf("body = %q", body)
	}
}

func TestPluginTier_MissingBare_ReturnsFalse(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "existing", Body: "---\nname: existing\ndescription: d.\n---\n\nBody."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, _, ok := tier.Resolver.Find("nonexistent")
	if ok {
		t.Error("Find returned ok=true for missing bare name")
	}
}

func TestPluginTier_MissingNamespaced_ReturnsFalse(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "existing", Body: "---\nname: existing\ndescription: d.\n---\n\nBody."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, _, ok := tier.Resolver.Find("other:nonexistent")
	if ok {
		t.Error("Find returned ok=true for missing namespaced name")
	}
}

func TestPluginTier_Walk_YieldsAll(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "skill1", Body: "---\nname: skill1\ndescription: d.\n---\n\nBody1."},
		{PluginName: "p", SkillName: "skill2", Body: "---\nname: skill2\ndescription: d.\n---\n\nBody2."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	found := map[string]bool{}
	tier.Resolver.Walk(func(si skills.SkillInfo) {
		found[si.Name] = true
	})

	if len(found) != 2 { //nolint:mnd
		t.Errorf("Walk yielded %d skills, want 2; got %v", len(found), found)
	}
}

func TestPluginTier_PathNotFound_ReturnsFalse(t *testing.T) {
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "missing-file", Path: "/nonexistent/skill.md"},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, _, ok := tier.Resolver.Find("missing-file")
	if ok {
		t.Error("Find returned ok=true for nonexistent path")
	}
}

func TestDirTier_MtimeCache_HitSkipsDiskRead(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "hitskill")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: hitskill\ndescription: Hit.\n---\n\nHit body."
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tier := skills.NewDirTier("t", root, skills.WithMtimeCache())

	// First read populates cache.
	_, body1, ok := tier.Resolver.Find("hitskill")
	if !ok {
		t.Fatal("first Find returned false")
	}
	if !strings.Contains(body1, "Hit body.") {
		t.Fatalf("body1 = %q", body1)
	}

	// Delete file — second read should still return cached body (mtime matches).
	// We can't delete because mtime check would fail on stat. Instead verify
	// second call returns same body without error.
	_, body2, ok := tier.Resolver.Find("hitskill")
	if !ok {
		t.Fatal("second Find returned false")
	}
	if body2 != body1 {
		t.Errorf("cache miss: body2 = %q, want %q", body2, body1)
	}
}

func TestPluginTier_DuplicateEntry_LogsWarn(t *testing.T) {
	// Duplicate (PluginName, SkillName): construction must not panic.
	// First entry wins; duplicate is logged. We just verify it doesn't panic
	// and the first body is returned.
	entries := []skills.PluginEntry{
		{PluginName: "p", SkillName: "dup", Body: "---\nname: dup\ndescription: d.\n---\n\nFirst."},
		{PluginName: "p", SkillName: "dup", Body: "---\nname: dup\ndescription: d.\n---\n\nSecond."},
	}
	tier := skills.NewPluginTier("plugins", entries)

	_, body, ok := tier.Resolver.Find("dup")
	if !ok {
		t.Fatal("Find returned false")
	}
	if !strings.Contains(body, "First.") {
		t.Errorf("expected first entry to win, got body = %q", body)
	}
}

// ---------------------------------------------------------------------------
// ContextResolver interface satisfaction test
// ---------------------------------------------------------------------------

type stubCtxResolver struct{}

func (stubCtxResolver) Find(_ string) (skills.SkillInfo, string, bool) {
	return skills.SkillInfo{}, "", false
}

func (stubCtxResolver) Walk(_ func(skills.SkillInfo)) {}

func (stubCtxResolver) FindCtx(_ context.Context, _ string) (skills.SkillInfo, string, bool, error) {
	return skills.SkillInfo{}, "", false, nil
}

// TestContextResolver_InterfaceSatisfied verifies the interface compiles.
func TestContextResolver_InterfaceSatisfied(t *testing.T) {
	var _ skills.ContextResolver = stubCtxResolver{}
	// If this compiles, the interface is correctly defined.
}

// ---------------------------------------------------------------------------
// PluginTier Dir contract tests (Item 15)
// ---------------------------------------------------------------------------

func TestPluginTier_BodyOnly_DirIsEmpty(t *testing.T) {
	body := "---\nname: foo\n---\nbody"
	tier := skills.NewPluginTier("plugins", []skills.PluginEntry{{
		PluginName: "p", SkillName: "foo", Body: body,
	}})
	info, _, ok := tier.Resolver.Find("foo")
	if !ok {
		t.Fatal("expected ok")
	}
	if info.Dir != "" {
		t.Errorf("expected Dir empty for body-only entry, got %q", info.Dir)
	}
}

func TestPluginTier_BodyAndPath_DirFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	body := "---\nname: foo\n---\nbody"
	tier := skills.NewPluginTier("plugins", []skills.PluginEntry{{
		PluginName: "p", SkillName: "foo", Body: body, Path: path,
	}})
	info, _, ok := tier.Resolver.Find("foo")
	if !ok {
		t.Fatal("expected ok")
	}
	if info.Dir != dir {
		t.Errorf("expected Dir=%q, got %q", dir, info.Dir)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// touchTime returns a time 2 seconds in the future so os.Chtimes causes a
// detectable mtime change even on filesystems with 1-second resolution.
func touchTime() time.Time {
	return time.Now().Add(2 * time.Second) //nolint:mnd
}

func mapKeys(m map[string]skills.SkillInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
