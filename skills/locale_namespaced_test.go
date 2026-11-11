package skills_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

func namespacedLocaleTier() skills.Tier {
	return skills.NewPluginTier("p", []skills.PluginEntry{
		{PluginName: "translate", SkillName: "rewrite", Body: "---\nname: rewrite\nlocale: en\n---\nEN body"},
		{PluginName: "translate", SkillName: "rewrite-ru", Body: "---\nname: rewrite\nlocale: ru\n---\nRU body"},
		{PluginName: "other", SkillName: "rewrite", Body: "---\nname: rewrite\n---\nOTHER body"},
	})
}

func TestCatalogLoad_NamespacedPluginSkill_WithoutLocale(t *testing.T) {
	cat := skills.NewCatalog(namespacedLocaleTier())

	body, info, ok := cat.Load("translate:rewrite")
	if !ok || body != "EN body" {
		t.Fatalf("Load(translate:rewrite) = (%q, %v), want (\"EN body\", true)", body, ok)
	}
	if info.Source != "p:translate" {
		t.Errorf("Source = %q, want p:translate", info.Source)
	}
	if body, _, ok := cat.Load("other:rewrite"); !ok || body != "OTHER body" {
		t.Errorf("Load(other:rewrite) = (%q, %v)", body, ok)
	}
}

func TestCatalogLoad_NamespacedPluginSkill_WithLocale(t *testing.T) {
	for _, tc := range []struct {
		locale, name, want string
	}{
		{"en", "translate:rewrite", "EN body"},
		{"ru", "translate:rewrite", "RU body"},
		{"RU", "translate:rewrite", "RU body"},
		{"en", "other:rewrite", "OTHER body"},
		{"ru", "other:rewrite", "OTHER body"},
	} {
		cat := skills.NewCatalog(namespacedLocaleTier()).WithLocale(tc.locale)
		body, _, ok := cat.Load(tc.name)
		if !ok || body != tc.want {
			t.Errorf("locale %q: Load(%q) = (%q, %v), want (%q, true)", tc.locale, tc.name, body, ok, tc.want)
		}
	}
}

func TestCatalogLoad_NamespacedPluginSkill_WithLocale_UnknownPluginMisses(t *testing.T) {
	cat := skills.NewCatalog(namespacedLocaleTier()).WithLocale("en")

	for _, name := range []string{"nosuch:rewrite", "translate:nosuch", "p:rewrite"} {
		if _, _, ok := cat.Load(name); ok {
			t.Errorf("Load(%q) with locale = ok, want miss", name)
		}
	}
}

func TestCatalogLoadCtx_NamespacedPluginSkill_WithLocale(t *testing.T) {
	cat := skills.NewCatalog(namespacedLocaleTier()).WithLocale("ru")

	body, _, ok, err := cat.LoadCtx(t.Context(), "translate:rewrite")
	if err != nil || !ok || body != "RU body" {
		t.Fatalf("LoadCtx(translate:rewrite) = (%q, %v, %v), want (\"RU body\", true, nil)", body, ok, err)
	}
}
