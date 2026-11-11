package skills_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

func pluginReviewEntries() []skills.PluginEntry {
	return []skills.PluginEntry{
		{PluginName: "alpha", SkillName: "review", Body: "---\nname: review\ndescription: from alpha\n---\nALPHA body"},
		{PluginName: "beta", SkillName: "review", Body: "---\nname: review\ndescription: from beta\n---\nBETA body"},
		{PluginName: "gamma", SkillName: "review", Body: "---\nname: review\ndescription: from gamma\n---\nGAMMA body"},
	}
}

// Catalog.List keeps the first Walk hit per name; it must be the entry bare
// Load resolves to (input order, first wins), on every run.
func TestPluginTier_ListAgreesWithLoadForDuplicateBareNames(t *testing.T) {
	for i := 0; i < 100; i++ {
		cat := skills.NewCatalog(skills.NewPluginTier("p", pluginReviewEntries()))

		body, loaded, ok := cat.Load("review")
		if !ok || body != "ALPHA body" {
			t.Fatalf("iteration %d: Load(review) = (%q, %v), want ALPHA body", i, body, ok)
		}
		listed := cat.List()
		if len(listed) != 1 {
			t.Fatalf("iteration %d: List() = %d entries, want 1", i, len(listed))
		}
		if listed[0].Source != loaded.Source || listed[0].Description != "from alpha" {
			t.Fatalf("iteration %d: List() advertises Source=%q Description=%q, but Load resolves to Source=%q",
				i, listed[0].Source, listed[0].Description, loaded.Source)
		}
	}
}

func TestPluginTier_WalkYieldsInputOrder(t *testing.T) {
	entries := append(pluginReviewEntries(),
		skills.PluginEntry{PluginName: "alpha", SkillName: "zeta", Body: "z"},
		skills.PluginEntry{PluginName: "alpha", SkillName: "audit", Body: "a"},
	)
	want := []string{"p:alpha/review", "p:beta/review", "p:gamma/review", "p:alpha/zeta", "p:alpha/audit"}

	for i := 0; i < 50; i++ {
		var got []string
		skills.NewPluginTier("p", entries).Resolver.Walk(func(si skills.SkillInfo) {
			got = append(got, si.Source+"/"+si.Name)
		})
		if len(got) != len(want) {
			t.Fatalf("iteration %d: Walk yielded %v, want %v", i, got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("iteration %d: Walk order = %v, want %v", i, got, want)
			}
		}
	}
}
