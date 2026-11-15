package skills

import (
	"strings"
	"testing"
)

func TestStripFrontmatter_YAMLHappyPath(t *testing.T) {
	input := "---\nname: foo\n---\nThis is the body."
	got := StripFrontmatter(input)
	if got != "This is the body." {
		t.Errorf("expected body, got %q", got)
	}
}

func TestStripFrontmatter_NoFence_Unchanged(t *testing.T) {
	input := "Just plain content.\nNo frontmatter here."
	got := StripFrontmatter(input)
	if got != input {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestStripFrontmatter_OnlyOpeningFence_Unchanged(t *testing.T) {
	input := "---\nname: foo\nno closing fence"
	got := StripFrontmatter(input)
	if got != input {
		t.Errorf("expected unchanged (no closing fence), got %q", got)
	}
}

func TestStripFrontmatter_EmptyBodyAfterFrontmatter(t *testing.T) {
	input := "---\nname: foo\n---\n"
	got := StripFrontmatter(input)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestStripFrontmatter_CRLF_NoSurvivors(t *testing.T) {
	input := "---\r\nname: foo\r\n---\r\nBody line.\r\n"
	got := StripFrontmatter(input)
	if strings.Contains(got, "\r") {
		t.Errorf("expected no CRLF survivors, got %q", got)
	}
	if got != "Body line." {
		t.Errorf("expected 'Body line.', got %q", got)
	}
}

func TestStripFrontmatter_JSON_HappyPath(t *testing.T) {
	input := `{"name":"foo"}` + "\n\nThis is the JSON body."
	got := StripFrontmatter(input)
	if got != "This is the JSON body." {
		t.Errorf("expected JSON body, got %q", got)
	}
}

func TestStripFrontmatter_JSON_InvalidJSON_Unchanged(t *testing.T) {
	// After M1 fix: invalid JSON fails json.Decoder.Decode, frontmatter
	// detection returns ("", original), StripFrontmatter returns input
	// unchanged.
	input := "{invalid json}\n\nsome body"
	got := StripFrontmatter(input)
	if got != input {
		t.Errorf("expected unchanged on invalid JSON, got %q", got)
	}
}

func TestStripFrontmatter_JSON_NoBlankSeparator_Unchanged(t *testing.T) {
	input := `{"name":"foo"}` + "\nimmediately body"
	got := StripFrontmatter(input)
	if got != input {
		t.Errorf("expected unchanged (no blank separator), got %q", got)
	}
}

func TestParseFrontmatter_YAMLExtraction(t *testing.T) {
	input := "---\nname: my-skill\ndescription: A test\n---\nBody text."
	fm, body := ParseFrontmatter(input)
	if !strings.Contains(fm, "name: my-skill") {
		t.Errorf("expected frontmatter to contain 'name: my-skill', got %q", fm)
	}
	if body != "Body text." {
		t.Errorf("expected body 'Body text.', got %q", body)
	}
}

func TestParseFrontmatter_JSONExtraction(t *testing.T) {
	input := `{"name":"json-skill"}` + "\n\nJSON body here."
	fm, body := ParseFrontmatter(input)
	if !strings.Contains(fm, `"name":"json-skill"`) {
		t.Errorf("expected JSON frontmatter, got %q", fm)
	}
	if body != "JSON body here." {
		t.Errorf("expected body 'JSON body here.', got %q", body)
	}
}

func TestParseFrontmatter_NoFence(t *testing.T) {
	input := "Plain content without frontmatter."
	fm, body := ParseFrontmatter(input)
	if fm != "" {
		t.Errorf("expected empty frontmatter, got %q", fm)
	}
	if body != input {
		t.Errorf("expected body == input, got %q", body)
	}
}

func TestStripFrontmatter_NULBytes_Removed(t *testing.T) {
	nul := string([]byte{0})
	input := "---\nname: foo\n---\nBod" + nul + "y text."
	got := StripFrontmatter(input)
	if strings.Contains(got, nul) {
		t.Errorf("expected NUL bytes removed, got %q", got)
	}
	if got != "Body text." {
		t.Errorf("expected 'Body text.', got %q", got)
	}
}

func TestStripFrontmatter_LeadingWhitespace_Stripped(t *testing.T) {
	// Body with leading spaces and tabs (in addition to newlines) must be
	// trimmed per the documented contract: "leading whitespace".
	input := "---\nname: foo\n---\n\n  \t  Indented body."
	got := StripFrontmatter(input)
	if got != "Indented body." {
		t.Errorf("expected leading spaces+tabs stripped, got %q", got)
	}
}

func TestStripFrontmatter_JSON_BracesInsideStrings(t *testing.T) {
	// Spec divergence fixed (M1): JSON string values containing literal
	// { or } must not confuse the frontmatter detector.
	input := `{"name":"foo","msg":"has } inside"}` + "\n\nBody after."
	got := StripFrontmatter(input)
	if got != "Body after." {
		t.Errorf("expected body, got %q", got)
	}
}

func TestStripFrontmatter_JSON_TrailingGarbage_Unchanged(t *testing.T) {
	// {...}trailing — invalid frontmatter, must be returned unchanged.
	input := `{"name":"foo"}trailing` + "\n\nBody."
	got := StripFrontmatter(input)
	if got != input {
		t.Errorf("expected unchanged on trailing garbage, got %q", got)
	}
}

func BenchmarkStripFrontmatter_NoFence(b *testing.B) {
	input := "Just plain text without any frontmatter fence.\nLine two.\nLine three."
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StripFrontmatter(input)
	}
}
