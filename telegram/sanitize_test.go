package telegram

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SanitizeHTML
// ---------------------------------------------------------------------------

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// TG-native tags pass through unchanged
		{
			name: "bold pass-through",
			in:   "<b>hello</b>",
			want: "<b>hello</b>",
		},
		{
			name: "italic pass-through",
			in:   "<i>italic</i>",
			want: "<i>italic</i>",
		},
		{
			name: "underline pass-through",
			in:   "<u>under</u>",
			want: "<u>under</u>",
		},
		{
			name: "strikethrough pass-through",
			in:   "<s>strike</s>",
			want: "<s>strike</s>",
		},
		{
			name: "code pass-through",
			in:   "<code>x := 1</code>",
			want: "<code>x := 1</code>",
		},
		{
			name: "pre pass-through",
			in:   "<pre>block</pre>",
			want: "<pre>block</pre>",
		},
		{
			name: "blockquote pass-through",
			in:   "<blockquote>quote</blockquote>",
			want: "<blockquote>quote</blockquote>",
		},
		// Synonym renames
		{
			name: "strong → b",
			in:   "<strong>bold</strong>",
			want: "<b>bold</b>",
		},
		{
			name: "em → i",
			in:   "<em>italic</em>",
			want: "<i>italic</i>",
		},
		{
			name: "ins → u",
			in:   "<ins>inserted</ins>",
			want: "<u>inserted</u>",
		},
		{
			name: "strike → s",
			in:   "<strike>strike</strike>",
			want: "<s>strike</s>",
		},
		{
			name: "del → s",
			in:   "<del>deleted</del>",
			want: "<s>deleted</s>",
		},
		// Headings → bold + newline
		{
			name: "h1 → bold",
			in:   "<h1>Title</h1>",
			want: "<b>Title</b>\n",
		},
		{
			name: "h3 → bold",
			in:   "<h3>Section</h3>",
			want: "<b>Section</b>\n",
		},
		// Block elements
		{
			name: "br → newline",
			in:   "line1<br>line2",
			want: "line1\nline2",
		},
		{
			name: "p → content + double newline",
			in:   "<p>paragraph</p>",
			want: "paragraph\n\n",
		},
		// Lists
		{
			name: "ul with li → bullets",
			in:   "<ul><li>one</li><li>two</li></ul>",
			want: "• one\n• two\n",
		},
		{
			name: "ol with li → numbered",
			in:   "<ol><li>first</li><li>second</li></ol>",
			want: "1. first\n2. second\n",
		},
		// Anchor handling
		{
			name: "anchor keeps href only",
			in:   `<a href="https://example.com" class="btn">link</a>`,
			want: `<a href="https://example.com">link</a>`,
		},
		{
			name: "anchor with tg scheme",
			in:   `<a href="tg://user?id=123">user</a>`,
			want: `<a href="tg://user?id=123">user</a>`,
		},
		{
			name: "anchor with mailto",
			in:   `<a href="mailto:a@b.com">mail</a>`,
			want: `<a href="mailto:a@b.com">mail</a>`,
		},
		{
			name: "anchor with javascript scheme stripped to text",
			in:   `<a href="javascript:alert(1)">click</a>`,
			want: "click",
		},
		{
			name: "anchor without href stripped to text",
			in:   "<a>no-href</a>",
			want: "no-href",
		},
		// Span handling
		{
			name: "span.tg-spoiler kept",
			in:   `<span class="tg-spoiler">secret</span>`,
			want: `<span class="tg-spoiler">secret</span>`,
		},
		{
			name: "span other class stripped to text",
			in:   `<span class="highlight">text</span>`,
			want: "text",
		},
		{
			name: "span no class stripped to text",
			in:   "<span>text</span>",
			want: "text",
		},
		// Code with language class
		{
			name: "code with language class kept",
			in:   `<code class="language-go">x := 1</code>`,
			want: `<code class="language-go">x := 1</code>`,
		},
		{
			name: "code with non-language class stripped",
			in:   `<code class="highlight">x := 1</code>`,
			want: `<code>x := 1</code>`,
		},
		// Security: script/style dropped entirely
		{
			name: "script dropped entirely",
			in:   "<p>text</p><script>alert(1)</script>",
			want: "text\n\n",
		},
		{
			name: "style dropped entirely",
			in:   "before<style>.x{color:red}</style>after",
			want: "beforeafter",
		},
		// img → alt text
		{
			name: "img with alt",
			in:   `<img src="x.png" alt="photo">`,
			want: "[photo]",
		},
		{
			name: "img without alt",
			in:   `<img src="x.png">`,
			want: "[image]",
		},
		// hr → separator
		{
			name: "hr → separator",
			in:   "before<hr>after",
			want: "before\n———\nafter",
		},
		// div/section → content + newline
		{
			name: "div → content + newline",
			in:   "<div>content</div>",
			want: "content\n",
		},
		// Unknown tags → strip, keep content
		{
			name: "unknown tag stripped",
			in:   "<foo>text</foo>",
			want: "text",
		},
		{
			name: "mark stripped to content",
			in:   "<mark>highlighted</mark>",
			want: "highlighted",
		},
		// Edge cases
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "nested valid tags",
			in:   "<b><i>bold italic</i></b>",
			want: "<b><i>bold italic</i></b>",
		},
		{
			name: "mixed valid and invalid nested",
			in:   "<b><strong>text</strong></b>",
			want: "<b><b>text</b></b>",
		},
		{
			name: "attribute-only change on anchor",
			in:   `<a href="https://x.com" onclick="bad()">x</a>`,
			want: `<a href="https://x.com">x</a>`,
		},
		{
			name: "tg-spoiler tag pass-through",
			in:   "<tg-spoiler>secret</tg-spoiler>",
			want: "<tg-spoiler>secret</tg-spoiler>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeHTML(tc.in)
			if got != tc.want {
				t.Errorf("SanitizeHTML(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeHTML_ScriptInjection(t *testing.T) {
	// Ensures script content is dropped, not rendered.
	in := `<b>safe</b><script>document.cookie="x"</script><i>also safe</i>`
	got := SanitizeHTML(in)
	if strings.Contains(got, "document.cookie") {
		t.Errorf("SanitizeHTML: script content leaked into output: %q", got)
	}
	if !strings.Contains(got, "safe") {
		t.Errorf("SanitizeHTML: legitimate content lost: %q", got)
	}
}

func TestSanitizeHTML_UnclosedTags(t *testing.T) {
	// x/net/html parser repairs unclosed tags automatically.
	in := "<b>unclosed"
	got := SanitizeHTML(in)
	if !strings.Contains(got, "unclosed") {
		t.Errorf("SanitizeHTML: content lost on unclosed tag: %q", got)
	}
}
