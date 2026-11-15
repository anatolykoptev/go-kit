package skills_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// An entry that fails to parse must not reserve its (plugin, skill) pair.
func TestPluginTier_InvalidFirstEntryDoesNotHideValidLaterOne(t *testing.T) {
	for name, bad := range map[string]skills.PluginEntry{
		"unreadable path": {PluginName: "alpha", SkillName: "review", Path: "/nonexistent/skill.md"},
		"empty body":      {PluginName: "alpha", SkillName: "review", Body: "---\nname: review\n---\n"},
		"neither":         {PluginName: "alpha", SkillName: "review"},
	} {
		tier := skills.NewPluginTier("p", []skills.PluginEntry{
			bad,
			{PluginName: "alpha", SkillName: "review", Body: "GOOD body"},
		})

		for _, lookup := range []string{"alpha:review", "review"} {
			_, body, ok := tier.Resolver.Find(lookup)
			if !ok || body != "GOOD body" {
				t.Errorf("%s: Find(%q) = (%q, %v), want (\"GOOD body\", true)", name, lookup, body, ok)
			}
		}
	}
}

// A valid first entry still wins over a valid later duplicate.
func TestPluginTier_ValidDuplicateStillFirstWins(t *testing.T) {
	tier := skills.NewPluginTier("p", []skills.PluginEntry{
		{PluginName: "alpha", SkillName: "review", Body: "FIRST"},
		{PluginName: "alpha", SkillName: "review", Body: "SECOND"},
	})

	if _, body, ok := tier.Resolver.Find("alpha:review"); !ok || body != "FIRST" {
		t.Errorf("Find(alpha:review) = (%q, %v), want FIRST", body, ok)
	}
}
