package telegram

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// RepairHTMLNesting
// ---------------------------------------------------------------------------

func TestRepairHTMLNesting(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "no tags passthrough",
			in:   "hello world",
			want: "hello world",
		},
		{
			name: "well-formed nesting unchanged",
			in:   "<b><i>text</i></b>",
			want: "<b><i>text</i></b>",
		},
		{
			name: "unclosed tag gets closed at end",
			in:   "<b>hello",
			want: "<b>hello</b>",
		},
		{
			name: "two unclosed tags closed in reverse",
			in:   "<b><i>hello",
			want: "<b><i>hello</i></b>",
		},
		{
			name: "interleaved tags get reordered",
			in:   "<b><i>text</b></i>",
			want: "<b><i>text</i></b><i></i>",
		},
		{
			name: "unmatched close is discarded",
			in:   "hello</b>world",
			want: "helloworld",
		},
		{
			name: "unclosed anchor gets closed",
			in:   `<a href="url">click here`,
			want: `<a href="url">click here</a>`,
		},
		{
			name: "code inside pre unclosed",
			in:   "<pre><code>snippet",
			want: "<pre><code>snippet</code></pre>",
		},
		{
			name: "multiple well-formed siblings",
			in:   "<b>a</b><i>b</i>",
			want: "<b>a</b><i>b</i>",
		},
		{
			name: "incomplete tag at end of string",
			in:   "text<b",
			want: "text<b",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RepairHTMLNesting(tc.in)
			if got != tc.want {
				t.Errorf("RepairHTMLNesting(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SplitMessage
// ---------------------------------------------------------------------------

func TestSplitMessage(t *testing.T) {
	t.Run("short message returned as-is", func(t *testing.T) {
		chunks := SplitMessage("hello world", 100)
		if len(chunks) != 1 || chunks[0] != "hello world" {
			t.Errorf("expected single chunk %q, got %v", "hello world", chunks)
		}
	})

	t.Run("exact length returned as-is", func(t *testing.T) {
		msg := strings.Repeat("a", 50)
		chunks := SplitMessage(msg, 50)
		if len(chunks) != 1 || chunks[0] != msg {
			t.Errorf("expected single exact-length chunk, got %v", chunks)
		}
	})

	t.Run("split at newline boundary", func(t *testing.T) {
		msg := "first line\nsecond line\nthird line"
		chunks := SplitMessage(msg, 12)
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
		}
		combined := strings.Join(chunks, "\n")
		if !strings.Contains(combined, "first line") || !strings.Contains(combined, "second line") {
			t.Errorf("content lost in split: chunks = %v", chunks)
		}
	})

	t.Run("unclosed HTML tag closed at chunk boundary and reopened", func(t *testing.T) {
		msg := "<b>hello world\nsecond line of bold content</b>"
		chunks := SplitMessage(msg, 20)
		if len(chunks) < 2 {
			t.Fatalf("expected split into >=2 chunks, got %d: %v", len(chunks), chunks)
		}
		if !strings.Contains(chunks[0], "</b>") {
			t.Errorf("first chunk should close <b>: %q", chunks[0])
		}
		if !strings.Contains(chunks[1], "<b>") {
			t.Errorf("second chunk should reopen <b>: %q", chunks[1])
		}
	})

	t.Run("multiple chunks all non-empty", func(t *testing.T) {
		msg := strings.Repeat("line\n", 30)
		chunks := SplitMessage(msg, 20)
		for i, ch := range chunks {
			if ch == "" {
				t.Errorf("chunk %d is empty", i)
			}
		}
	})

	t.Run("no newline forces hard split at maxLen", func(t *testing.T) {
		msg := strings.Repeat("a", 50)
		chunks := SplitMessage(msg, 20)
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks for long no-newline content, got %d", len(chunks))
		}
	})
}

// ---------------------------------------------------------------------------
// unclosedTags
// ---------------------------------------------------------------------------

func TestUnclosedTags(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "empty string",
			html: "",
			want: nil,
		},
		{
			name: "no tags",
			html: "plain text",
			want: nil,
		},
		{
			name: "fully closed single tag",
			html: "<b>hello</b>",
			want: nil,
		},
		{
			name: "unclosed bold",
			html: "<b>hello",
			want: []string{"<b>"},
		},
		{
			name: "nested unclosed",
			html: "<b><i>text",
			want: []string{"<b>", "<i>"},
		},
		{
			name: "anchor with href",
			html: `<a href="https://example.com">link`,
			want: []string{`<a href="https://example.com">`},
		},
		{
			name: "pre+code unclosed",
			html: "<pre><code>snippet",
			want: []string{"<pre>", "<code>"},
		},
		{
			name: "all closed",
			html: "<b>b</b><i>i</i><s>s</s><u>u</u><code>c</code><pre>p</pre><blockquote>q</blockquote>",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unclosedTags(tc.html)
			if len(got) != len(tc.want) {
				t.Fatalf("unclosedTags(%q) = %v (len %d), want %v (len %d)",
					tc.html, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("unclosedTags[%d]: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// EscapeHTML
// ---------------------------------------------------------------------------

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{"a < b > c & d", "a &lt; b &gt; c &amp; d"},
		{"", ""},
	}
	for _, tc := range tests {
		got := EscapeHTML(tc.in)
		if got != tc.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SanitizeUTF8
// ---------------------------------------------------------------------------

func TestSanitizeUTF8(t *testing.T) {
	t.Run("valid utf8 passthrough", func(t *testing.T) {
		in := "hello, мир"
		if got := SanitizeUTF8(in); got != in {
			t.Errorf("SanitizeUTF8(%q) = %q, want unchanged", in, got)
		}
	})

	t.Run("null byte removed", func(t *testing.T) {
		in := "hel\x00lo"
		got := SanitizeUTF8(in)
		if strings.Contains(got, "\x00") {
			t.Errorf("SanitizeUTF8: null byte not removed: %q", got)
		}
		if got != "hello" {
			t.Errorf("SanitizeUTF8(%q) = %q, want %q", in, got, "hello")
		}
	})

	t.Run("invalid utf8 removed", func(t *testing.T) {
		in := "hello\xff\xfe world"
		got := SanitizeUTF8(in)
		if got == in {
			t.Errorf("SanitizeUTF8: invalid UTF-8 bytes should be removed")
		}
		if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
			t.Errorf("SanitizeUTF8: valid content lost: %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// StripHTMLTags
// ---------------------------------------------------------------------------

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "basic tag removal",
			in:   "<b>bold</b> and <i>italic</i>",
			want: "bold and italic",
		},
		{
			name: "nested tags",
			in:   "<pre><code>snippet</code></pre>",
			want: "snippet",
		},
		{
			name: "anchor with attributes",
			in:   `<a href="https://example.com">link</a>`,
			want: "link",
		},
		{
			name: "plain text passthrough",
			in:   "no tags here",
			want: "no tags here",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StripHTMLTags(tc.in)
			if got != tc.want {
				t.Errorf("StripHTMLTags(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CompactForTelegram
// ---------------------------------------------------------------------------

func TestCompactForTelegram(t *testing.T) {
	t.Run("short text passthrough", func(t *testing.T) {
		in := "short"
		got := CompactForTelegram(in, 100)
		if got != in {
			t.Errorf("expected passthrough, got %q", got)
		}
	})

	t.Run("large code block stripped", func(t *testing.T) {
		code := strings.Repeat("x", 510)
		in := "header\n```go\n" + code + "\n```\nfooter"
		maxChars := len(in) - 10
		got := CompactForTelegram(in, maxChars)
		if strings.Contains(got, code) {
			t.Errorf("large code block should be stripped")
		}
		if !strings.Contains(got, "(code block trimmed)") {
			t.Errorf("trimmed placeholder missing in: %q", got)
		}
	})

	t.Run("verbose section heading truncates", func(t *testing.T) {
		prefix := strings.Repeat("a", 250)
		verboseBody := strings.Repeat("log line\n", 50)
		in := prefix + "\n# Raw Logs\n" + verboseBody
		got := CompactForTelegram(in, 400)
		if strings.Contains(got, "Raw Logs") {
			t.Errorf("verbose section should be truncated, got: %q", got)
		}
		if !strings.Contains(got, "_(truncated)_") {
			t.Errorf("truncated marker missing in: %q", got)
		}
	})

	t.Run("hard truncate at newline boundary", func(t *testing.T) {
		in := strings.Repeat("line\n", 60)
		got := CompactForTelegram(in, 100)
		if len(got) > 150 {
			t.Errorf("result too long: %d chars", len(got))
		}
		if !strings.Contains(got, "_(truncated)_") {
			t.Errorf("truncated marker missing")
		}
	})
}
