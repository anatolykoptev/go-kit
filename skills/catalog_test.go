package skills_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// ---------------------------------------------------------------------------
// stubCtxResolver — ContextResolver for catalog tests
// ---------------------------------------------------------------------------

type stubCtxResolverD struct {
	findInfo skills.SkillInfo
	findBody string
	findOK   bool
	err      error
}

func (s stubCtxResolverD) Find(_ string) (skills.SkillInfo, string, bool) {
	return s.findInfo, s.findBody, s.findOK
}

func (s stubCtxResolverD) Walk(yield func(skills.SkillInfo)) {
	if s.findOK {
		yield(s.findInfo)
	}
}

func (s stubCtxResolverD) FindCtx(_ context.Context, _ string) (skills.SkillInfo, string, bool, error) {
	return s.findInfo, s.findBody, s.findOK, s.err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func builtinTier() skills.Tier {
	return skills.NewDirTier("builtin", filepath.Join("testdata/tier", "builtin"))
}

func workspaceTier() skills.Tier {
	return skills.NewDirTier("workspace", filepath.Join("testdata/tier", "workspace"))
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestCatalog_List_DedupsFirstTierWins(t *testing.T) {
	// workspace has "foo", builtin has "foo" and "bar".
	// Expected: workspace wins for foo, builtin bar, sorted: [bar, foo].
	cat := skills.NewCatalog(workspaceTier(), builtinTier())
	skills := cat.List()

	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}

	if len(skills) != 2 { //nolint:mnd
		t.Fatalf("List returned %d skills, want 2; names=%v", len(skills), names)
	}
	if names[0] != "bar" || names[1] != "foo" {
		t.Errorf("List names = %v, want [bar foo]", names)
	}
	// foo should come from workspace (first tier).
	for _, s := range skills {
		if s.Name == "foo" && !strings.Contains(s.Description, "Workspace foo") {
			t.Errorf("foo.Description = %q, want workspace description", s.Description)
		}
	}
}

func TestCatalog_List_SortedByName(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	skills := cat.List()
	for i := 1; i < len(skills); i++ {
		if skills[i].Name < skills[i-1].Name {
			t.Errorf("List not sorted at index %d: %q > %q", i, skills[i-1].Name, skills[i].Name)
		}
	}
}

func TestCatalog_List_ZeroTiers_Empty(t *testing.T) {
	cat := skills.NewCatalog()
	if got := cat.List(); len(got) != 0 {
		t.Errorf("List() with no tiers returned %d skills, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

func TestCatalog_Load_Tier1Hit(t *testing.T) {
	// Only workspace has "foo" in tier1 sense; bar only in builtin.
	cat := skills.NewCatalog(workspaceTier(), builtinTier())

	body, info, ok := cat.Load("bar")
	if !ok {
		t.Fatal("Load(bar) returned ok=false, want true (bar only in tier2)")
	}
	if !strings.Contains(body, "Builtin bar body.") {
		t.Errorf("body = %q, want to contain 'Builtin bar body.'", body)
	}
	if info.Name != "bar" {
		t.Errorf("info.Name = %q, want 'bar'", info.Name)
	}
}

func TestCatalog_Load_Tier2Hit(t *testing.T) {
	// Both tiers have "foo"; workspace (tier1) wins.
	cat := skills.NewCatalog(workspaceTier(), builtinTier())

	body, info, ok := cat.Load("foo")
	if !ok {
		t.Fatal("Load(foo) returned ok=false")
	}
	if !strings.Contains(body, "Workspace foo body.") {
		t.Errorf("body = %q, want workspace body", body)
	}
	_ = info
}

func TestCatalog_Load_AllMiss(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	_, _, ok := cat.Load("nonexistent-skill")
	if ok {
		t.Error("Load(nonexistent) returned ok=true, want false")
	}
}

func TestCatalog_Load_ZeroTiers_ReturnsFalse(t *testing.T) {
	cat := skills.NewCatalog()
	_, _, ok := cat.Load("foo")
	if ok {
		t.Error("Load with no tiers returned ok=true, want false")
	}
}

// ---------------------------------------------------------------------------
// LoadMany
// ---------------------------------------------------------------------------

func TestCatalog_LoadMany_AllHit(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadMany([]string{"foo", "bar"})

	if !strings.Contains(result, "### Skill: foo") {
		t.Error("LoadMany missing '### Skill: foo'")
	}
	if !strings.Contains(result, "### Skill: bar") {
		t.Error("LoadMany missing '### Skill: bar'")
	}
	if !strings.Contains(result, "---") {
		t.Error("LoadMany missing separator '---'")
	}
}

func TestCatalog_LoadMany_PartialHit_MissingSkipped(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadMany([]string{"foo", "no-such-skill", "bar"})

	if !strings.Contains(result, "### Skill: foo") {
		t.Error("LoadMany missing '### Skill: foo'")
	}
	if !strings.Contains(result, "### Skill: bar") {
		t.Error("LoadMany missing '### Skill: bar'")
	}
	if strings.Contains(result, "no-such-skill") {
		t.Error("LoadMany included missing skill name in output")
	}
}

func TestCatalog_LoadMany_EmptyInput_EmptyString(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadMany([]string{})
	if result != "" {
		t.Errorf("LoadMany(empty) = %q, want ''", result)
	}
}

func TestCatalog_LoadMany_AllMiss_EmptyString(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadMany([]string{"ghost1", "ghost2"})
	if result != "" {
		t.Errorf("LoadMany(all-miss) = %q, want ''", result)
	}
}

func TestCatalog_LoadMany_TrailingSeparatorStripped(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadMany([]string{"foo"})
	if strings.HasSuffix(result, "---\n\n") {
		t.Errorf("LoadMany single skill has trailing separator: %q", result)
	}
}

// ---------------------------------------------------------------------------
// LoadCtx
// ---------------------------------------------------------------------------

func TestCatalog_LoadCtx_WithContextResolver_ReturnsBody(t *testing.T) {
	stub := stubCtxResolverD{
		findInfo: skills.SkillInfo{Name: "ctx-skill"},
		findBody: "ctx body",
		findOK:   true,
	}
	tier := skills.Tier{Name: "stub", Resolver: stub}
	cat := skills.NewCatalog(tier)

	body, info, ok, err := cat.LoadCtx(context.Background(), "ctx-skill")
	if err != nil {
		t.Fatalf("LoadCtx returned err: %v", err)
	}
	if !ok {
		t.Fatal("LoadCtx returned ok=false, want true")
	}
	if body != "ctx body" {
		t.Errorf("body = %q, want 'ctx body'", body)
	}
	if info.Name != "ctx-skill" {
		t.Errorf("info.Name = %q, want 'ctx-skill'", info.Name)
	}
}

func TestCatalog_LoadCtx_WithContextResolver_ErrorPropagated(t *testing.T) {
	sentinelErr := errors.New("network timeout")
	stub := stubCtxResolverD{
		findOK: false,
		err:    sentinelErr,
	}
	tier := skills.Tier{Name: "stub", Resolver: stub}
	cat := skills.NewCatalog(tier)

	_, _, _, err := cat.LoadCtx(context.Background(), "any")
	if !errors.Is(err, sentinelErr) {
		t.Errorf("err = %v, want sentinel error", err)
	}
}

func TestCatalog_LoadCtx_NonContextResolver_FallsBackToFind(t *testing.T) {
	// DirTier does not implement ContextResolver — falls back to Find.
	cat := skills.NewCatalog(builtinTier())

	body, info, ok, err := cat.LoadCtx(context.Background(), "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCtx returned ok=false for existing skill")
	}
	if !strings.Contains(body, "Builtin foo body.") {
		t.Errorf("body = %q", body)
	}
	if info.Name != "foo" {
		t.Errorf("info.Name = %q", info.Name)
	}
}

// ---------------------------------------------------------------------------
// LoadManyCtx
// ---------------------------------------------------------------------------

func TestCatalog_LoadManyCtx_AllHit(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadManyCtx(context.Background(), []string{"foo", "bar"})
	if !strings.Contains(result, "### Skill: foo") {
		t.Error("LoadManyCtx missing 'foo'")
	}
	if !strings.Contains(result, "### Skill: bar") {
		t.Error("LoadManyCtx missing 'bar'")
	}
}

func TestCatalog_LoadManyCtx_EmptyInput(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadManyCtx(context.Background(), []string{})
	if result != "" {
		t.Errorf("LoadManyCtx(empty) = %q, want ''", result)
	}
}

func TestCatalog_LoadManyCtx_CtxError_SkipsThatSkill(t *testing.T) {
	// Stub that returns an error — skill should be skipped.
	sentinelErr := errors.New("ctx err")
	stub := stubCtxResolverD{
		findInfo: skills.SkillInfo{Name: "errskill"},
		findBody: "err body",
		findOK:   true,
		err:      sentinelErr,
	}
	tier := skills.Tier{Name: "stub", Resolver: stub}
	cat := skills.NewCatalog(tier)

	// LoadManyCtx skips skills with errors.
	result := cat.LoadManyCtx(context.Background(), []string{"errskill"})
	if result != "" {
		t.Errorf("LoadManyCtx with error skill = %q, want ''", result)
	}
}

// ---------------------------------------------------------------------------
// Source tagging
// ---------------------------------------------------------------------------

func TestCatalog_SourceTagging_DirTier_SetsTierName(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	_, info, ok := cat.Load("foo")
	if !ok {
		t.Fatal("Load(foo) returned false")
	}
	if info.Source != "builtin" {
		t.Errorf("Source = %q, want 'builtin'", info.Source)
	}
}

func TestCatalog_SourceTagging_PluginTier_KeepsPluginSource(t *testing.T) {
	entries := []skills.PluginEntry{
		{
			PluginName: "myplugin",
			SkillName:  "myskill",
			Body:       "---\nname: myskill\ndescription: Plugin skill.\n---\n\nPlugin body.",
		},
	}
	tier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(tier)

	_, info, ok := cat.Load("myskill")
	if !ok {
		t.Fatal("Load(myskill) returned false")
	}
	// PluginTier sets Source = "<tierName>:<pluginName>"
	if info.Source != "plugins:myplugin" {
		t.Errorf("Source = %q, want 'plugins:myplugin'", info.Source)
	}
}

func TestCatalog_SourceTagging_DirTier_InList(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	skills := cat.List()
	for _, s := range skills {
		if s.Source != "builtin" {
			t.Errorf("skill %q has Source = %q, want 'builtin'", s.Name, s.Source)
		}
	}
}

// ---------------------------------------------------------------------------
// LoadCtx: context-aware miss + fallthrough
// ---------------------------------------------------------------------------

func TestCatalog_LoadCtx_CtxMiss_FallsThroughToNextTier(t *testing.T) {
	// Stub returns ok=false with no error; fallthrough to builtin tier.
	stubMiss := stubCtxResolverD{findOK: false}
	stubTier := skills.Tier{Name: "stub", Resolver: stubMiss}
	cat := skills.NewCatalog(stubTier, builtinTier())

	body, info, ok, err := cat.LoadCtx(context.Background(), "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCtx should have fallen through to builtin tier and found foo")
	}
	if !strings.Contains(body, "Builtin foo body.") {
		t.Errorf("body = %q, want builtin body", body)
	}
	if info.Name != "foo" {
		t.Errorf("info.Name = %q", info.Name)
	}
}

func TestCatalog_LoadCtx_ZeroTiers_ReturnsFalse(t *testing.T) {
	cat := skills.NewCatalog()
	_, _, ok, err := cat.LoadCtx(context.Background(), "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("LoadCtx with no tiers returned ok=true, want false")
	}
}

func TestCatalog_LoadCtx_NonCtxMiss_FallsThroughToNextTier(t *testing.T) {
	// First tier (DirTier, non-ContextResolver) doesn't have "baz",
	// second tier (embedFS) does.
	cat := skills.NewCatalog(
		builtinTier(), // has foo, bar — not baz
		skills.NewEmbedFSTier("embed", embedTestFS, "testdata/tier/embedfs"),
	)
	body, info, ok, err := cat.LoadCtx(context.Background(), "baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCtx should find 'baz' in second tier")
	}
	if !strings.Contains(body, "Embedded baz body.") {
		t.Errorf("body = %q", body)
	}
	if info.Name != "baz" {
		t.Errorf("info.Name = %q", info.Name)
	}
}

// ---------------------------------------------------------------------------
// LoadManyCtx: miss (not-found) path
// ---------------------------------------------------------------------------

func TestCatalog_LoadManyCtx_MissingSkillSkipped(t *testing.T) {
	cat := skills.NewCatalog(builtinTier())
	result := cat.LoadManyCtx(context.Background(), []string{"foo", "ghost", "bar"})
	if !strings.Contains(result, "### Skill: foo") {
		t.Error("LoadManyCtx missing foo")
	}
	if strings.Contains(result, "ghost") {
		t.Error("LoadManyCtx should have skipped 'ghost'")
	}
	if !strings.Contains(result, "### Skill: bar") {
		t.Error("LoadManyCtx missing bar")
	}
}
