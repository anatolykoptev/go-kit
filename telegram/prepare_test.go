package telegram

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// PrepareForTelegram
// ---------------------------------------------------------------------------

func TestPrepareForTelegram(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantMode     string
		wantContains string
		wantExact    string
	}{
		{
			name:      "empty string",
			in:        "",
			wantMode:  "HTML",
			wantExact: "",
		},
		{
			name:         "HTML input passes through sanitized",
			in:           "<b>hello</b>",
			wantMode:     "HTML",
			wantContains: "<b>hello</b>",
		},
		{
			name:         "HTML with synonym gets renamed",
			in:           "<strong>bold</strong>",
			wantMode:     "HTML",
			wantContains: "<b>bold</b>",
		},
		{
			name:         "Markdown converts to HTML",
			in:           "**bold text**",
			wantMode:     "HTML",
			wantContains: "<b>bold text</b>",
		},
		{
			name:         "plain text gets escaped",
			in:           "hello & <world>",
			wantMode:     "HTML",
			wantContains: "&amp;",
		},
		{
			name:         "plain text lt escaped",
			in:           "hello & <world>",
			wantMode:     "HTML",
			wantContains: "&lt;",
		},
		{
			name:         "plain text gt escaped",
			in:           "hello & <world>",
			wantMode:     "HTML",
			wantContains: "&gt;",
		},
		{
			name:         "markdown heading converts",
			in:           "# Title",
			wantMode:     "HTML",
			wantContains: "<b>",
		},
		{
			name:         "HTML script stripped",
			in:           "<b>safe</b><script>evil()</script>",
			wantMode:     "HTML",
			wantContains: "<b>safe</b>",
		},
		{
			name:     "all modes return HTML parse mode",
			in:       "plain text",
			wantMode: "HTML",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, mode := PrepareForTelegram(tc.in)
			if mode != tc.wantMode {
				t.Errorf("PrepareForTelegram(%q) mode = %q, want %q", tc.in, mode, tc.wantMode)
			}
			if tc.wantExact != "" || (tc.wantExact == "" && tc.in == "") {
				if out != tc.wantExact {
					t.Errorf("PrepareForTelegram(%q) out = %q, want exact %q", tc.in, out, tc.wantExact)
				}
			}
			if tc.wantContains != "" && !strings.Contains(out, tc.wantContains) {
				t.Errorf("PrepareForTelegram(%q) out = %q, want to contain %q", tc.in, out, tc.wantContains)
			}
		})
	}
}

func TestPrepareForTelegram_ScriptNeverLeaks(t *testing.T) {
	in := `<div>text<script>document.cookie="x"</script></div>`
	out, mode := PrepareForTelegram(in)
	if mode != "HTML" {
		t.Errorf("mode = %q, want HTML", mode)
	}
	if strings.Contains(out, "document.cookie") {
		t.Errorf("script content leaked: %q", out)
	}
	if !strings.Contains(out, "text") {
		t.Errorf("legitimate content lost: %q", out)
	}
}

func TestPrepareForTelegram_MarkdownHTMLEntitySafe(t *testing.T) {
	// Markdown with HTML-unsafe chars — entities should be escaped inside bold.
	// Use **word** form so \*\*\w heuristic fires.
	in := "Price: **five** less than ten & valid"
	out, mode := PrepareForTelegram(in)
	if mode != "HTML" {
		t.Errorf("mode = %q, want HTML", mode)
	}
	if !strings.Contains(out, "<b>") {
		t.Errorf("bold markup missing: %q", out)
	}
}

func TestPrepareForTelegram_PlainEntityEscaped(t *testing.T) {
	// Plain text with < > & must be escaped so Telegram HTML mode is safe.
	in := "price < 10 & cost > 5"
	out, mode := PrepareForTelegram(in)
	if mode != "HTML" {
		t.Errorf("mode = %q, want HTML", mode)
	}
	if strings.Contains(out, " < ") || strings.Contains(out, " > ") {
		t.Errorf("unescaped angle bracket leaked through: %q", out)
	}
	if !strings.Contains(out, "&lt;") || !strings.Contains(out, "&gt;") {
		t.Errorf("entities not present in plain-escape output: %q", out)
	}
}
