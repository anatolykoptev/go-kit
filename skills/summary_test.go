package skills_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// ---------------------------------------------------------------------------
// BuildSummary helpers
// ---------------------------------------------------------------------------

// longDesc builds a description string of exactly n chars.
func longDesc(n int) string {
	const char = "x"
	return strings.Repeat(char, n)
}

// stubBodyWithDesc builds a minimal SKILL.md body with the given description.
func stubBodyWithDesc(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + name + " body."
}

// ---------------------------------------------------------------------------
// BuildSummary XML
// ---------------------------------------------------------------------------

func TestBuildSummary_XML_StandardSkills(t *testing.T) {
	cat := skills.NewCatalog(
		skills.NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")),
	)
	out := cat.BuildSummary(skills.SummaryXML)

	if !strings.Contains(out, "<skills>") {
		t.Error("XML missing <skills>")
	}
	if !strings.Contains(out, "</skills>") {
		t.Error("XML missing </skills>")
	}
	if !strings.Contains(out, `<skill name="foo"`) {
		t.Errorf("XML missing skill foo; got:\n%s", out)
	}
	if !strings.Contains(out, `<skill name="bar"`) {
		t.Errorf("XML missing skill bar; got:\n%s", out)
	}
	if !strings.Contains(out, "<description>") {
		t.Error("XML missing <description>")
	}
	if !strings.Contains(out, "</description>") {
		t.Error("XML missing </description>")
	}
}

func TestBuildSummary_XML_PluginSkillGrouped(t *testing.T) {
	entries := []skills.PluginEntry{
		{
			PluginName: "foo",
			SkillName:  "plugin-skill",
			Body:       "---\nname: plugin-skill\ndescription: Plugin skill desc.\n---\n\nPlugin body.",
		},
	}
	pluginTier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(pluginTier)
	out := cat.BuildSummary(skills.SummaryXML)

	if !strings.Contains(out, `<skill-group plugin="foo">`) {
		t.Errorf("XML missing plugin group; got:\n%s", out)
	}
	if !strings.Contains(out, "</skill-group>") {
		t.Error("XML missing </skill-group>")
	}
	if !strings.Contains(out, `<skill name="plugin-skill"`) {
		t.Errorf("XML missing plugin-skill inside group; got:\n%s", out)
	}
}

func TestBuildSummary_XML_EscapesSpecialChars(t *testing.T) {
	// Create a tier using plugin entry with special chars in description.
	entries := []skills.PluginEntry{
		{
			PluginName: "esc",
			SkillName:  "esc-skill",
			Body:       "---\nname: esc-skill\ndescription: a < b & c > d\n---\n\nBody.",
		},
	}
	tier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(tier)
	out := cat.BuildSummary(skills.SummaryXML)

	if strings.Contains(out, "a < b") {
		t.Error("XML did not escape '<' in description")
	}
	if strings.Contains(out, "c > d") {
		t.Error("XML did not escape '>' in description")
	}
	if strings.Contains(out, "b & c") {
		t.Error("XML did not escape '&' in description")
	}
	if !strings.Contains(out, "&lt;") {
		t.Error("XML missing &lt; for '<'")
	}
	if !strings.Contains(out, "&gt;") {
		t.Error("XML missing &gt; for '>'")
	}
	if !strings.Contains(out, "&amp;") {
		t.Error("XML missing &amp; for '&'")
	}
}

func TestBuildSummary_XML_DescriptionTruncatedAt180(t *testing.T) {
	desc := longDesc(200) //nolint:mnd
	entries := []skills.PluginEntry{
		{
			PluginName: "p",
			SkillName:  "long-skill",
			Body:       stubBodyWithDesc("long-skill", desc),
		},
	}
	tier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(tier)
	out := cat.BuildSummary(skills.SummaryXML)

	// Truncated description should be 180 x's, not 200.
	truncated := strings.Repeat("x", 180) //nolint:mnd
	if !strings.Contains(out, truncated) {
		t.Error("XML description not truncated to 180 chars")
	}
	full := strings.Repeat("x", 200) //nolint:mnd
	if strings.Contains(out, full) {
		t.Error("XML description not truncated; still contains 200-char string")
	}
}

// ---------------------------------------------------------------------------
// BuildSummary Markdown
// ---------------------------------------------------------------------------

func TestBuildSummary_Markdown_Format(t *testing.T) {
	cat := skills.NewCatalog(
		skills.NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")),
	)
	out := cat.BuildSummary(skills.SummaryMarkdown)

	if !strings.Contains(out, "### foo") {
		t.Errorf("Markdown missing '### foo'; got:\n%s", out)
	}
	if !strings.Contains(out, "### bar") {
		t.Errorf("Markdown missing '### bar'; got:\n%s", out)
	}
	if !strings.Contains(out, "Builtin foo skill") {
		t.Errorf("Markdown missing foo description; got:\n%s", out)
	}
}

func TestBuildSummary_Markdown_DescriptionTruncatedAt180(t *testing.T) {
	desc := longDesc(200) //nolint:mnd
	entries := []skills.PluginEntry{
		{
			PluginName: "p",
			SkillName:  "long-md",
			Body:       stubBodyWithDesc("long-md", desc),
		},
	}
	tier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(tier)
	out := cat.BuildSummary(skills.SummaryMarkdown)

	truncated := strings.Repeat("x", 180) //nolint:mnd
	if !strings.Contains(out, truncated) {
		t.Errorf("Markdown description not truncated to 180; got:\n%s", out)
	}
	full := strings.Repeat("x", 200) //nolint:mnd
	if strings.Contains(out, full) {
		t.Error("Markdown description not truncated; still contains 200-char string")
	}
}

// ---------------------------------------------------------------------------
// BuildSummary JSON
// ---------------------------------------------------------------------------

func TestBuildSummary_JSON_ValidRoundtrip(t *testing.T) {
	cat := skills.NewCatalog(
		skills.NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")),
	)
	out := cat.BuildSummary(skills.SummaryJSON)

	var skills []skills.SkillInfo
	if err := json.Unmarshal([]byte(out), &skills); err != nil {
		t.Fatalf("JSON unmarshal failed: %v\noutput: %s", err, out)
	}
	if len(skills) == 0 {
		t.Error("JSON roundtrip yielded empty slice")
	}
}

func TestBuildSummary_JSON_FullDescription(t *testing.T) {
	desc := longDesc(200) //nolint:mnd
	entries := []skills.PluginEntry{
		{
			PluginName: "p",
			SkillName:  "full-json",
			Body:       stubBodyWithDesc("full-json", desc),
		},
	}
	tier := skills.NewPluginTier("plugins", entries)
	cat := skills.NewCatalog(tier)
	out := cat.BuildSummary(skills.SummaryJSON)

	full := strings.Repeat("x", 200) //nolint:mnd
	if !strings.Contains(out, full) {
		t.Errorf("JSON description truncated; want full 200 chars. Got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// BuildSummary XML escape: quotes in attributes (M1 regression)
// ---------------------------------------------------------------------------

func TestBuildSummary_XML_EscapesQuotesInAttributes(t *testing.T) {
	// M1 fix: operator-controlled values (path, plugin name) containing
	// " must not break XML attribute syntax.
	cat := skills.NewCatalog(skills.NewPluginTier("plugins", []skills.PluginEntry{{
		PluginName: `weird"plugin`,
		SkillName:  "foo",
		Body:       "---\nname: foo\ndescription: A skill with \"quotes\" in it.\n---\nbody",
	}}))
	out := cat.BuildSummary(skills.SummaryXML)
	if strings.Contains(out, `plugin="weird"plugin"`) {
		t.Error("XML attribute injection: unescaped quote in plugin name")
	}
	if !strings.Contains(out, "&#34;") && !strings.Contains(out, "&quot;") {
		t.Error("expected escaped quote (either &#34; or &quot;), got neither")
	}
}

// ---------------------------------------------------------------------------
// BuildSummary empty Catalog
// ---------------------------------------------------------------------------

func TestBuildSummary_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	cat := skills.NewCatalog()

	for _, format := range []skills.SummaryFormat{
		skills.SummaryXML, skills.SummaryMarkdown, skills.SummaryJSON,
	} {
		out := cat.BuildSummary(format)
		if out != "" {
			t.Errorf("BuildSummary(%v) with no tiers = %q, want ''", format, out)
		}
	}
}
