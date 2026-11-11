package skills

import (
	"strings"
	"testing"
)

// yamlFM wraps content in YAML frontmatter.
func yamlFM(content string) string {
	return "---\n" + content + "\n---\nBody."
}

// jsonFM wraps content as JSON frontmatter.
func jsonFM(content string) string {
	return content + "\n\nBody."
}

// ---- YAML scalar fields ----

func TestParseMetadata_YAML_AllScalarFields(t *testing.T) {
	input := yamlFM(strings.Join([]string{
		"name: my-skill",
		"description: A test skill",
		"license: MIT",
		"compatibility: claude-3+",
		"version: 1.2.3",
		"locale: en",
		"when_to_use: Use when testing",
		"model: claude-3-opus",
	}, "\n"))
	m := ParseMetadata(input)
	if m.Name != "my-skill" {
		t.Errorf("Name: got %q", m.Name)
	}
	if m.Description != "A test skill" {
		t.Errorf("Description: got %q", m.Description)
	}
	if m.License != "MIT" {
		t.Errorf("License: got %q", m.License)
	}
	if m.Compatibility != "claude-3+" {
		t.Errorf("Compatibility: got %q", m.Compatibility)
	}
	if m.Version != "1.2.3" {
		t.Errorf("Version: got %q", m.Version)
	}
	if m.Locale != "en" {
		t.Errorf("Locale: got %q", m.Locale)
	}
	if m.WhenToUse != "Use when testing" {
		t.Errorf("WhenToUse: got %q", m.WhenToUse)
	}
	// model is not a typed slot → Extra
	if m.Extra["model"] != "claude-3-opus" {
		t.Errorf("Extra[model]: got %q", m.Extra["model"])
	}
}

func TestParseMetadata_YAML_QuotedStrings(t *testing.T) {
	input := yamlFM(strings.Join([]string{
		`name: "quoted-skill"`,
		`description: 'single-quoted description'`,
	}, "\n"))
	m := ParseMetadata(input)
	if m.Name != "quoted-skill" {
		t.Errorf("Name: got %q", m.Name)
	}
	if m.Description != "single-quoted description" {
		t.Errorf("Description: got %q", m.Description)
	}
}

// ---- Tags ----

func TestParseMetadata_YAML_TagsFlowList(t *testing.T) {
	input := yamlFM("name: skill\ntags: [go, testing, agent]")
	m := ParseMetadata(input)
	if len(m.Tags) != 3 {
		t.Fatalf("Tags: expected 3, got %v", m.Tags)
	}
	if m.Tags[0] != "go" || m.Tags[1] != "testing" || m.Tags[2] != "agent" {
		t.Errorf("Tags: got %v", m.Tags)
	}
}

func TestParseMetadata_YAML_TagsCommaSeparated(t *testing.T) {
	input := yamlFM("name: skill\ntags: go, testing, agent")
	m := ParseMetadata(input)
	if len(m.Tags) != 3 {
		t.Fatalf("Tags: expected 3, got %v", m.Tags)
	}
	if m.Tags[0] != "go" || m.Tags[1] != "testing" || m.Tags[2] != "agent" {
		t.Errorf("Tags: got %v", m.Tags)
	}
}

// ---- AllowedTools ----

func TestParseMetadata_YAML_AllowedToolsFlowList(t *testing.T) {
	input := yamlFM("name: skill\nallowed-tools: [Read, Grep, Bash]")
	m := ParseMetadata(input)
	if len(m.AllowedTools) != 3 {
		t.Fatalf("AllowedTools: expected 3, got %v", m.AllowedTools)
	}
	if m.AllowedTools[0] != "Read" || m.AllowedTools[1] != "Grep" || m.AllowedTools[2] != "Bash" {
		t.Errorf("AllowedTools: got %v", m.AllowedTools)
	}
}

func TestParseMetadata_YAML_AllowedToolsSpaceSeparated(t *testing.T) {
	input := yamlFM("name: skill\nallowed-tools: Read Grep Bash")
	m := ParseMetadata(input)
	if len(m.AllowedTools) != 3 {
		t.Fatalf("AllowedTools: expected 3, got %v", m.AllowedTools)
	}
	if m.AllowedTools[0] != "Read" || m.AllowedTools[1] != "Grep" || m.AllowedTools[2] != "Bash" {
		t.Errorf("AllowedTools: got %v", m.AllowedTools)
	}
}

// ---- Boolean fields ----

func TestParseMetadata_YAML_DisableModelInvocation_True(t *testing.T) {
	input := yamlFM("name: skill\ndisable-model-invocation: true")
	m := ParseMetadata(input)
	if !m.DisableModelInvocation {
		t.Error("DisableModelInvocation: expected true")
	}
}

func TestParseMetadata_YAML_DisableModelInvocation_False(t *testing.T) {
	input := yamlFM("name: skill\ndisable-model-invocation: false")
	m := ParseMetadata(input)
	if m.DisableModelInvocation {
		t.Error("DisableModelInvocation: expected false")
	}
}

func TestParseMetadata_YAML_DisableModelInvocation_Absent(t *testing.T) {
	input := yamlFM("name: skill")
	m := ParseMetadata(input)
	if m.DisableModelInvocation {
		t.Error("DisableModelInvocation: expected false when absent")
	}
}

func TestParseMetadata_YAML_UserInvocable_True(t *testing.T) {
	input := yamlFM("name: skill\nuser-invocable: true")
	m := ParseMetadata(input)
	if m.UserInvocable == nil {
		t.Fatal("UserInvocable: expected non-nil")
	}
	if !*m.UserInvocable {
		t.Error("UserInvocable: expected true")
	}
}

func TestParseMetadata_YAML_UserInvocable_False(t *testing.T) {
	input := yamlFM("name: skill\nuser-invocable: false")
	m := ParseMetadata(input)
	if m.UserInvocable == nil {
		t.Fatal("UserInvocable: expected non-nil")
	}
	if *m.UserInvocable {
		t.Error("UserInvocable: expected false")
	}
}

func TestParseMetadata_YAML_UserInvocable_Absent(t *testing.T) {
	input := yamlFM("name: skill")
	m := ParseMetadata(input)
	if m.UserInvocable != nil {
		t.Errorf("UserInvocable: expected nil, got %v", m.UserInvocable)
	}
}

// ---- metadata: block ----

func TestParseMetadata_YAML_MetadataBlock_FlattensIntoExtra(t *testing.T) {
	input := yamlFM("name: skill\nmetadata:\n  author: alice\n  repo: github.com/alice/skill")
	m := ParseMetadata(input)
	if m.Extra["author"] != "alice" {
		t.Errorf("Extra[author]: got %q", m.Extra["author"])
	}
	if m.Extra["repo"] != "github.com/alice/skill" {
		t.Errorf("Extra[repo]: got %q", m.Extra["repo"])
	}
}

func TestParseMetadata_YAML_UnknownTopLevelKeys_LandInExtra(t *testing.T) {
	input := yamlFM("name: skill\nunknown-key: some-value\nanother: 42")
	m := ParseMetadata(input)
	if m.Extra["unknown-key"] != "some-value" {
		t.Errorf("Extra[unknown-key]: got %q", m.Extra["unknown-key"])
	}
	if m.Extra["another"] != "42" {
		t.Errorf("Extra[another]: got %q", m.Extra["another"])
	}
}

// ---- Comments and blanks ----

func TestParseMetadata_YAML_CommentsAndBlanksIgnored(t *testing.T) {
	input := yamlFM("# This is a comment\n\nname: skill\n# another comment\ndescription: Works")
	m := ParseMetadata(input)
	if m.Name != "skill" {
		t.Errorf("Name: got %q", m.Name)
	}
	if m.Description != "Works" {
		t.Errorf("Description: got %q", m.Description)
	}
}

// ---- JSON path ----

func TestParseMetadata_JSON_FullRoundtrip(t *testing.T) {
	input := jsonFM(`{
  "name": "json-skill",
  "description": "A JSON skill",
  "license": "Apache-2.0",
  "compatibility": "all",
  "version": "2.0.0",
  "locale": "ru",
  "when_to_use": "Use for JSON tests",
  "allowed-tools": ["Read", "Bash"],
  "disable-model-invocation": true,
  "user-invocable": false,
  "tags": ["json", "test"],
  "metadata": {"author": "bob"}
}`)
	m := ParseMetadata(input)
	if m.Name != "json-skill" {
		t.Errorf("Name: got %q", m.Name)
	}
	if m.Description != "A JSON skill" {
		t.Errorf("Description: got %q", m.Description)
	}
	if m.License != "Apache-2.0" {
		t.Errorf("License: got %q", m.License)
	}
	if m.Compatibility != "all" {
		t.Errorf("Compatibility: got %q", m.Compatibility)
	}
	if m.Version != "2.0.0" {
		t.Errorf("Version: got %q", m.Version)
	}
	if m.Locale != "ru" {
		t.Errorf("Locale: got %q", m.Locale)
	}
	if m.WhenToUse != "Use for JSON tests" {
		t.Errorf("WhenToUse: got %q", m.WhenToUse)
	}
	if len(m.AllowedTools) != 2 || m.AllowedTools[0] != "Read" || m.AllowedTools[1] != "Bash" {
		t.Errorf("AllowedTools: got %v", m.AllowedTools)
	}
	if !m.DisableModelInvocation {
		t.Error("DisableModelInvocation: expected true")
	}
	if m.UserInvocable == nil || *m.UserInvocable {
		t.Errorf("UserInvocable: expected *false, got %v", m.UserInvocable)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "json" || m.Tags[1] != "test" {
		t.Errorf("Tags: got %v", m.Tags)
	}
	if m.Extra["author"] != "bob" {
		t.Errorf("Extra[author]: got %q", m.Extra["author"])
	}
}

func TestParseMetadata_JSON_MissingOptionals_ZeroValues(t *testing.T) {
	input := jsonFM(`{"name":"minimal"}`)
	m := ParseMetadata(input)
	if m.Name != "minimal" {
		t.Errorf("Name: got %q", m.Name)
	}
	if m.Description != "" {
		t.Errorf("Description: expected empty, got %q", m.Description)
	}
	if m.License != "" {
		t.Errorf("License: expected empty, got %q", m.License)
	}
	if m.AllowedTools != nil {
		t.Errorf("AllowedTools: expected nil, got %v", m.AllowedTools)
	}
	if m.UserInvocable != nil {
		t.Errorf("UserInvocable: expected nil, got %v", m.UserInvocable)
	}
}

// ---- Empty input ----

func TestParseMetadata_EmptyInput_EmptyMetadata(t *testing.T) {
	m := ParseMetadata("")
	if m.Name != "" || m.Description != "" {
		t.Errorf("expected empty Metadata, got %+v", m)
	}
}

// ---- ValidateName ----

func TestValidateName_SingleChar(t *testing.T) {
	if err := ValidateName("f", ""); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidateName_64CharAlpha(t *testing.T) {
	name := strings.Repeat("a", 64)
	if err := ValidateName(name, ""); err != nil {
		t.Errorf("expected valid 64-char name, got %v", err)
	}
}

func TestValidateName_65Char_Fail(t *testing.T) {
	name := strings.Repeat("a", 65)
	if err := ValidateName(name, ""); err == nil {
		t.Error("expected error for 65-char name")
	}
}

func TestValidateName_Uppercase_Fail(t *testing.T) {
	if err := ValidateName("Foo", ""); err == nil {
		t.Error("expected error for uppercase name")
	}
}

func TestValidateName_Underscore_Fail(t *testing.T) {
	if err := ValidateName("foo_bar", ""); err == nil {
		t.Error("expected error for underscore in name")
	}
}

func TestValidateName_LeadingHyphen_Fail(t *testing.T) {
	if err := ValidateName("-foo", ""); err == nil {
		t.Error("expected error for leading hyphen")
	}
}

func TestValidateName_TrailingHyphen_Fail(t *testing.T) {
	if err := ValidateName("foo-", ""); err == nil {
		t.Error("expected error for trailing hyphen")
	}
}

func TestValidateName_ConsecutiveHyphens_Fail(t *testing.T) {
	if err := ValidateName("foo--bar", ""); err == nil {
		t.Error("expected error for consecutive hyphens")
	}
}

func TestValidateName_Empty_Fail(t *testing.T) {
	if err := ValidateName("", ""); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateName_DirMismatch_Fail(t *testing.T) {
	if err := ValidateName("foo", "bar"); err == nil {
		t.Error("expected error for dir name mismatch")
	}
}

func TestValidateName_DirEmpty_SkipCheck(t *testing.T) {
	if err := ValidateName("foo", ""); err != nil {
		t.Errorf("expected valid (no dir check), got %v", err)
	}
}

func TestValidateName_DirMatches_OK(t *testing.T) {
	if err := ValidateName("my-skill", "my-skill"); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidateName_ValidWithHyphen(t *testing.T) {
	if err := ValidateName("my-skill", ""); err != nil {
		t.Errorf("expected valid name with hyphen, got %v", err)
	}
}

func TestValidateName_ValidAlphaNum(t *testing.T) {
	if err := ValidateName("skill42", ""); err != nil {
		t.Errorf("expected valid alphanumeric name, got %v", err)
	}
}

// ---- Extra coverage ----

func TestParseMetadata_YAML_LongDescription_NoTruncation(t *testing.T) {
	// Description > 1024 chars: slog Debug warning, no truncation.
	longDesc := strings.Repeat("x", 1025)
	input := yamlFM("name: skill\ndescription: " + longDesc)
	m := ParseMetadata(input)
	if len(m.Description) != 1025 {
		t.Errorf("expected description length 1025, got %d", len(m.Description))
	}
}

func TestParseMetadata_JSON_LongDescription_NoTruncation(t *testing.T) {
	longDesc := strings.Repeat("y", 1025)
	input := jsonFM(`{"name":"skill","description":"` + longDesc + `"}`)
	m := ParseMetadata(input)
	if len(m.Description) != 1025 {
		t.Errorf("expected description length 1025, got %d", len(m.Description))
	}
}

func TestParseMetadata_YAML_NoFrontmatter_EmptyMetadata(t *testing.T) {
	// Content with no frontmatter → empty Metadata.
	m := ParseMetadata("Just body text, no frontmatter.")
	if m.Name != "" {
		t.Errorf("expected empty Metadata, got Name=%q", m.Name)
	}
}

func TestParseMetadata_YAML_EmptyFlowList(t *testing.T) {
	input := yamlFM("name: skill\ntags: []")
	m := ParseMetadata(input)
	if m.Tags != nil {
		t.Errorf("expected nil tags for empty flow list, got %v", m.Tags)
	}
}

func TestParseMetadata_JSON_InvalidJSON_EmptyMetadata(t *testing.T) {
	// JSON frontmatter that doesn't unmarshal → empty Metadata.
	input := `{"name": "skill", "broken":}` + "\n\nBody."
	m := ParseMetadata(input)
	if m.Name != "" {
		t.Errorf("expected empty Metadata for invalid JSON, got Name=%q", m.Name)
	}
}

func TestParseMetadata_YAML_FlowList_UnquotesElements(t *testing.T) {
	fm := "---\nname: x\ntags: ['foo bar', \"baz\", qux]\n---\nbody"
	m := ParseMetadata(fm)
	want := []string{"foo bar", "baz", "qux"}
	if len(m.Tags) != 3 || m.Tags[0] != want[0] || m.Tags[1] != want[1] || m.Tags[2] != want[2] {
		t.Errorf("expected %v, got %v", want, m.Tags)
	}
}

func TestParseMetadata_YAML_AllowedToolsFlowList_UnquotesElements(t *testing.T) {
	fm := "---\nname: x\nallowed-tools: [\"Read\", 'Grep', Bash]\n---\nbody"
	m := ParseMetadata(fm)
	if len(m.AllowedTools) != 3 || m.AllowedTools[0] != "Read" || m.AllowedTools[1] != "Grep" || m.AllowedTools[2] != "Bash" {
		t.Errorf("expected [Read Grep Bash], got %v", m.AllowedTools)
	}
}
