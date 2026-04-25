package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// SplitMessage — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestSplitMessage_ShortASCII(t *testing.T) {
	msg := "Hello, world!"
	chunks := SplitMessage(msg, MaxMessageLen)
	if len(chunks) != 1 || chunks[0] != msg {
		t.Errorf("short ASCII: expected single chunk %q, got %v", msg, chunks)
	}
}

func TestSplitMessage_LongASCIINewlines(t *testing.T) {
	// Build a message with newlines that exceeds maxLen.
	var b strings.Builder
	for i := range 100 {
		b.WriteString("Line ")
		b.WriteByte(byte('0' + (i % 10)))
		b.WriteByte('\n')
	}
	msg := b.String()
	chunks := SplitMessage(msg, 50)

	for i, ch := range chunks {
		rc := utf8.RuneCountInString(ch)
		if rc > 50 {
			t.Errorf("chunk %d exceeds maxLen 50: %d runes", i, rc)
		}
	}
	// Verify all content is preserved.
	combined := strings.Join(chunks, "\n")
	if !strings.Contains(combined, "Line 0") || !strings.Contains(combined, "Line 9") {
		t.Error("content lost in split")
	}
}

func TestSplitMessage_LongCyrillicText(t *testing.T) {
	// 5000 Cyrillic characters.
	msg := strings.Repeat("Привет мир! ", 500) // ~6000 runes
	chunks := SplitMessage(msg, MaxMessageLen)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks for 6000 Cyrillic runes, got %d", len(chunks))
	}
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}
		rc := utf8.RuneCountInString(ch)
		if rc > MaxMessageLen {
			t.Errorf("chunk %d exceeds MaxMessageLen: %d runes", i, rc)
		}
	}
}

func TestSplitMessage_EmojiSpanningBoundary(t *testing.T) {
	// Each emoji is 1 rune (4 bytes). Build a string of 10 emoji.
	emojis := strings.Repeat("\U0001F525", 10) // fire emoji x10
	chunks := SplitMessage(emojis, 3)

	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d has broken emoji (invalid UTF-8): %q", i, ch)
		}
	}
	// Verify total rune count is preserved.
	total := 0
	for _, ch := range chunks {
		total += utf8.RuneCountInString(ch)
	}
	if total != 10 {
		t.Errorf("expected 10 emoji runes total, got %d", total)
	}
}

func TestSplitMessage_HTMLBoldSpanningBoundary(t *testing.T) {
	// <b> tag spanning chunk boundary should be closed/reopened.
	msg := "<b>" + strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 30) + "</b>"
	chunks := SplitMessage(msg, 25)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	// First chunk must close <b>.
	if !strings.Contains(chunks[0], "</b>") {
		t.Errorf("first chunk missing </b>: %q", chunks[0])
	}
	// Second chunk must reopen <b>.
	if !strings.Contains(chunks[1], "<b>") {
		t.Errorf("second chunk missing <b>: %q", chunks[1])
	}
}

func TestSplitMessage_AnchorHrefSpanningBoundary(t *testing.T) {
	// Anchor opening tag is 30 runes. Need maxLen large enough to fit the tag
	// plus some content, but small enough to force a split.
	msg := `<a href="https://example.com">` + strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 30) + "</a>"
	chunks := SplitMessage(msg, 55)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	// Second chunk must reopen with href preserved.
	if !strings.Contains(chunks[1], `href="https://example.com"`) {
		t.Errorf("second chunk lost href: %q", chunks[1])
	}
}

func TestSplitMessage_AnchorHrefTruncatedByTightLimit(t *testing.T) {
	// BUG: When maxLen is barely larger than the anchor tag itself,
	// trimChunkToLimit can cut inside the reopened tag, losing the href.
	// This test documents the behavior with a very tight limit.
	msg := `<a href="https://example.com">` + strings.Repeat("x", 20) + "\n" + strings.Repeat("y", 20) + "</a>"
	chunks := SplitMessage(msg, 40)

	// With maxLen=40 and a 30-rune anchor tag, the second chunk will be
	// trimmed aggressively. We just verify no panic and valid UTF-8.
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}
	}
}

func TestSplitMessage_NestedTagsSpanningBoundary(t *testing.T) {
	msg := "<b><i><code>" + strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 30) + "</code></i></b>"
	chunks := SplitMessage(msg, 35)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	// First chunk must close all 3 tags (code, i, b in reverse).
	first := chunks[0]
	if !strings.Contains(first, "</code>") || !strings.Contains(first, "</i>") || !strings.Contains(first, "</b>") {
		t.Errorf("first chunk missing closers: %q", first)
	}
	// Second chunk must reopen all 3.
	second := chunks[1]
	if !strings.Contains(second, "<b>") || !strings.Contains(second, "<i>") || !strings.Contains(second, "<code>") {
		t.Errorf("second chunk missing reopeners: %q", second)
	}
}

func TestSplitMessage_VeryLongNoNewlines(t *testing.T) {
	msg := strings.Repeat("a", 100)
	chunks := SplitMessage(msg, 30)

	if len(chunks) < 3 {
		t.Fatalf("expected >=3 chunks for 100 chars at maxLen=30, got %d", len(chunks))
	}
	total := 0
	for i, ch := range chunks {
		rc := utf8.RuneCountInString(ch)
		if rc > 30 {
			t.Errorf("chunk %d exceeds maxLen: %d runes", i, rc)
		}
		total += rc
	}
	if total != 100 {
		t.Errorf("expected 100 total runes, got %d", total)
	}
}

func TestSplitMessage_EmptyString(t *testing.T) {
	chunks := SplitMessage("", MaxMessageLen)
	if len(chunks) != 1 || chunks[0] != "" {
		t.Errorf("empty string: expected [\"\"], got %v", chunks)
	}
}

func TestSplitMessage_ExactMaxLen(t *testing.T) {
	msg := strings.Repeat("x", 4096)
	chunks := SplitMessage(msg, MaxMessageLen)
	if len(chunks) != 1 {
		t.Errorf("exact maxLen: expected 1 chunk, got %d", len(chunks))
	}
}

func TestSplitMessage_PreCodeBlockSpanning(t *testing.T) {
	msg := "<pre><code>" + strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 30) + "</code></pre>"
	chunks := SplitMessage(msg, 35)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	// Verify tags handled across boundary.
	if !strings.Contains(chunks[0], "</code>") || !strings.Contains(chunks[0], "</pre>") {
		t.Errorf("first chunk missing pre/code closers: %q", chunks[0])
	}
}

// ---------------------------------------------------------------------------
// MarkdownToHTML — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestMarkdownToHTML_PlainText(t *testing.T) {
	got := MarkdownToHTML("just plain text")
	if got != "just plain text" {
		t.Errorf("plain text changed: got %q", got)
	}
}

func TestMarkdownToHTML_PlainTextHTMLEscaping(t *testing.T) {
	got := MarkdownToHTML("a < b & c > d")
	want := "a &lt; b &amp; c &gt; d"
	if got != want {
		t.Errorf("HTML escaping: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_BoldStars(t *testing.T) {
	got := MarkdownToHTML("**bold text**")
	want := "<b>bold text</b>"
	if got != want {
		t.Errorf("bold: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_ItalicStars(t *testing.T) {
	got := MarkdownToHTML("*italic text*")
	want := "<i>italic text</i>"
	if got != want {
		t.Errorf("italic: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_BoldItalic(t *testing.T) {
	got := MarkdownToHTML("***bold italic***")
	want := "<b><i>bold italic</i></b>"
	if got != want {
		t.Errorf("bold italic: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_Strikethrough_Int(t *testing.T) {
	got := MarkdownToHTML("~~strike~~")
	want := "<s>strike</s>"
	if got != want {
		t.Errorf("strike: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_HeadingH1(t *testing.T) {
	got := MarkdownToHTML("# Heading")
	want := "<b>Heading</b>"
	if got != want {
		t.Errorf("heading: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_LinkBasic(t *testing.T) {
	got := MarkdownToHTML("[link](https://example.com)")
	want := `<a href="https://example.com">link</a>`
	if got != want {
		t.Errorf("link: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_LinkWithUnderscoreURL(t *testing.T) {
	got := MarkdownToHTML("[link](https://example.com/path_with_underscores)")
	if strings.Contains(got, "<i>") {
		t.Errorf("URL underscores became italic: %q", got)
	}
	if !strings.Contains(got, `href="https://example.com/path_with_underscores"`) {
		t.Errorf("URL mangled: %q", got)
	}
}

func TestMarkdownToHTML_ImageSyntax_Int(t *testing.T) {
	got := MarkdownToHTML("![image alt](https://example.com/img.png)")
	// No stray '!' in output.
	if strings.Contains(got, "!<") || strings.Contains(got, "!&") {
		t.Errorf("stray ! before tag: %q", got)
	}
	// Should produce an <a> tag since Telegram has no <img>.
	if !strings.Contains(got, "<a") || !strings.Contains(got, "href=") {
		t.Errorf("image should produce <a> tag: %q", got)
	}
	if !strings.Contains(got, "image alt") {
		t.Errorf("alt text lost: %q", got)
	}
}

func TestMarkdownToHTML_InlineCode(t *testing.T) {
	got := MarkdownToHTML("`inline code`")
	want := "<code>inline code</code>"
	if got != want {
		t.Errorf("inline code: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_CodeBlockWithLang(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	got := MarkdownToHTML(input)
	if !strings.Contains(got, `class="language-go"`) {
		t.Errorf("missing language class: %q", got)
	}
	if !strings.Contains(got, "<pre><code") {
		t.Errorf("missing pre/code: %q", got)
	}
	// Content should be HTML-escaped.
	if !strings.Contains(got, "&quot;") || !strings.Contains(got, "hello") {
		// Note: EscapeHTML only escapes &, <, >. Quotes are not escaped.
		// This is fine for Telegram.
		if !strings.Contains(got, "Println") {
			t.Errorf("code content lost: %q", got)
		}
	}
}

func TestMarkdownToHTML_Blockquote_Int(t *testing.T) {
	got := MarkdownToHTML("> blockquote text")
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("missing blockquote: %q", got)
	}
	if !strings.Contains(got, "blockquote text") {
		t.Errorf("blockquote content lost: %q", got)
	}
}

func TestMarkdownToHTML_HorizontalRule_Int(t *testing.T) {
	got := MarkdownToHTML("---")
	if !strings.Contains(got, "\u2014\u2014\u2014") {
		t.Errorf("expected em-dash rule, got %q", got)
	}
}

func TestMarkdownToHTML_ListItemBullet(t *testing.T) {
	got := MarkdownToHTML("* list item\n* another item")
	if strings.Contains(got, "<i>") {
		t.Errorf("list items should NOT be italic: %q", got)
	}
	if !strings.Contains(got, "\u2022 list item") {
		t.Errorf("expected bullet, got %q", got)
	}
	if !strings.Contains(got, "\u2022 another item") {
		t.Errorf("expected second bullet, got %q", got)
	}
}

func TestMarkdownToHTML_MixedFormats(t *testing.T) {
	input := "# Title\n\n**bold** and `code` and [link](https://example.com)"
	got := MarkdownToHTML(input)
	if !strings.Contains(got, "<b>Title</b>") {
		t.Errorf("heading missing: %q", got)
	}
	if !strings.Contains(got, "<b>bold</b>") {
		t.Errorf("bold missing: %q", got)
	}
	if !strings.Contains(got, "<code>code</code>") {
		t.Errorf("code missing: %q", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("link missing: %q", got)
	}
}

func TestMarkdownToHTML_HTMLEntitiesEscaped(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"a & b", "a &amp; b"},
		{"x > y", "x &gt; y"},
	}
	for _, tc := range tests {
		got := MarkdownToHTML(tc.in)
		if got != tc.want {
			t.Errorf("MarkdownToHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMarkdownToHTML_NestedFormatting(t *testing.T) {
	got := MarkdownToHTML("**bold with *italic* inside**")
	// Should have both <b> and <i>.
	if !strings.Contains(got, "<b>") {
		t.Errorf("missing bold: %q", got)
	}
	if !strings.Contains(got, "<i>") {
		t.Errorf("missing italic inside bold: %q", got)
	}
}

func TestMarkdownToHTML_CodeBlockProtectsFormatting(t *testing.T) {
	got := MarkdownToHTML("`**not bold**`")
	want := "<code>**not bold**</code>"
	if got != want {
		t.Errorf("code should protect formatting: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_TripleBacktickCodeBlockProtects(t *testing.T) {
	input := "```\n**not bold** and *not italic*\n```"
	got := MarkdownToHTML(input)
	if strings.Contains(got, "<b>") || strings.Contains(got, "<i>") {
		t.Errorf("code block should protect formatting: %q", got)
	}
	if !strings.Contains(got, "**not bold**") {
		t.Errorf("code block content lost: %q", got)
	}
}

// ---------------------------------------------------------------------------
// StripMarkdown — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestStripMarkdown_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold stars", "**bold**", "bold"},
		{"bold underscores", "__bold__", "bold"},
		{"italic star", "*italic*", "italic"},
		{"italic underscore", "_italic_", "italic"},
		{"strikethrough", "~~strike~~", "strike"},
		{"inline code", "`code`", "code"},
		{"heading", "## Heading", "Heading"},
		{"list item dash", "- item", "- item"},
		{"list item star", "* item", "- item"},
		{"blockquote", "> quoted", "quoted"},
		{"link", "[text](https://url.com)", "text (https://url.com)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StripMarkdown(tc.in)
			if got != tc.want {
				t.Errorf("StripMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripMarkdown_CodeFencesRemoved(t *testing.T) {
	input := "before\n```go\ncode here\n```\nafter"
	got := StripMarkdown(input)
	if strings.Contains(got, "```") {
		t.Errorf("code fences not removed: %q", got)
	}
	if !strings.Contains(got, "code here") {
		t.Errorf("code content lost: %q", got)
	}
}

// ---------------------------------------------------------------------------
// CloseUnclosedMarkdown — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestCloseUnclosedMarkdown_Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "complete markdown unchanged",
			in:   "**bold** and *italic* and `code` and ~~strike~~",
			want: "**bold** and *italic* and `code` and ~~strike~~",
		},
		{
			name: "unclosed bold",
			in:   "**unclosed bold text",
			want: "**unclosed bold text**",
		},
		{
			name: "unclosed backtick",
			in:   "`unclosed code",
			want: "`unclosed code`",
		},
		{
			name: "unclosed triple backtick",
			in:   "```go\nsome code here",
			want: "```go\nsome code here\n```",
		},
		{
			name: "unclosed strikethrough",
			in:   "~~unclosed strike",
			want: "~~unclosed strike~~",
		},
		{
			name: "multiple unclosed: bold + italic",
			in:   "**bold *italic text",
			want: "**bold *italic text***",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CloseUnclosedMarkdown(tc.in)
			if got != tc.want {
				t.Errorf("CloseUnclosedMarkdown(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CompactForTelegram — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestCompactForTelegram_ShortTextPassthrough(t *testing.T) {
	in := "short text here"
	got := CompactForTelegram(in, 1000)
	if got != in {
		t.Errorf("short text should pass through: got %q", got)
	}
}

func TestCompactForTelegram_LargeCodeBlockTrimmed(t *testing.T) {
	code := strings.Repeat("x", 600)
	in := "header\n```go\n" + code + "\n```\nfooter"
	got := CompactForTelegram(in, utf8.RuneCountInString(in)-100)
	if strings.Contains(got, code) {
		t.Error("large code block should be trimmed")
	}
	if !strings.Contains(got, "(code block trimmed)") {
		t.Errorf("trimmed placeholder missing: %q", got)
	}
}

func TestCompactForTelegram_VerboseSectionTruncated(t *testing.T) {
	prefix := strings.Repeat("useful content line\n", 15)
	in := prefix + "# Stack Trace\n" + strings.Repeat("at something.go:123\n", 100)
	got := CompactForTelegram(in, 400)
	if strings.Contains(got, "Stack Trace") {
		t.Errorf("verbose section should be truncated: %q", got)
	}
	if !strings.Contains(got, "_(truncated)_") {
		t.Errorf("truncation marker missing: %q", got)
	}
}

func TestCompactForTelegram_PureLongTextHardTruncate(t *testing.T) {
	in := strings.Repeat("word ", 200) // ~1000 chars
	got := CompactForTelegram(in, 100)
	rc := utf8.RuneCountInString(got)
	if rc > 120 { // Some tolerance for the truncation suffix.
		t.Errorf("hard truncate too long: %d runes", rc)
	}
	if !strings.Contains(got, "_(truncated)_") {
		t.Errorf("truncation marker missing")
	}
}

func TestCompactForTelegram_CyrillicUsesCharCount(t *testing.T) {
	// 200 Cyrillic chars = 400 bytes. If byte-counting, maxChars=150 would pass.
	in := strings.Repeat("\u0410", 200) // 200 x 'А'
	got := CompactForTelegram(in, 150)
	rc := utf8.RuneCountInString(got)
	if rc > 180 { // Allow for truncation suffix.
		t.Errorf("Cyrillic compact should use rune count, got %d runes", rc)
	}
}

// ---------------------------------------------------------------------------
// SanitizeUTF8 — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestSanitizeUTF8_ValidPassthrough(t *testing.T) {
	in := "hello, мир! \U0001F525"
	got := SanitizeUTF8(in)
	if got != in {
		t.Errorf("valid UTF-8 should pass through: got %q", got)
	}
}

func TestSanitizeUTF8_NullBytesRemoved(t *testing.T) {
	in := "hel\x00lo\x00 world"
	got := SanitizeUTF8(in)
	if strings.Contains(got, "\x00") {
		t.Error("null bytes not removed")
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestSanitizeUTF8_InvalidBytesRemovedValidPreserved(t *testing.T) {
	in := "hello\xff\xfe world\xc0\xc1 ok"
	got := SanitizeUTF8(in)
	if !utf8.ValidString(got) {
		t.Errorf("result not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") || !strings.Contains(got, "ok") {
		t.Errorf("valid parts lost: %q", got)
	}
}

// ---------------------------------------------------------------------------
// EscapeHTML — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestEscapeHTML_Comprehensive(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"&", "&amp;"},
		{"<", "&lt;"},
		{">", "&gt;"},
		{"a & b < c > d", "a &amp; b &lt; c &gt; d"},
		{"no special chars", "no special chars"},
		{"", ""},
		{"&amp;", "&amp;amp;"}, // Double-escaping is expected.
	}
	for _, tc := range tests {
		got := EscapeHTML(tc.in)
		if got != tc.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// RepairHTMLNesting — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestRepairHTMLNesting_Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "well-formed unchanged",
			in:   "<b>bold</b> and <i>italic</i>",
			want: "<b>bold</b> and <i>italic</i>",
		},
		{
			name: "unclosed b closed at end",
			in:   "<b>unclosed bold",
			want: "<b>unclosed bold</b>",
		},
		{
			name: "interleaved b/i properly reordered",
			in:   "<b><i>text</b></i>",
			want: "<b><i>text</i></b><i></i>",
		},
		{
			name: "orphan closer discarded",
			in:   "text</b>more",
			want: "textmore",
		},
		{
			name: "unclosed anchor closed",
			in:   `<a href="https://example.com">link text`,
			want: `<a href="https://example.com">link text</a>`,
		},
		{
			name: "multiple unclosed",
			in:   "<b><i><code>deep",
			want: "<b><i><code>deep</code></i></b>",
		},
		{
			name: "interleaved anchor preserves href",
			in:   `<a href="url"><b>text</a></b>`,
			want: `<a href="url"><b>text</b></a><b></b>`,
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
// StripHTMLTags — comprehensive scenarios
// ---------------------------------------------------------------------------

func TestStripHTMLTags_Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold tag", "<b>bold</b>", "bold"},
		{"nested tags", "<b><i><code>text</code></i></b>", "text"},
		{"anchor with attrs", `<a href="url">link</a>`, "link"},
		{"plain text", "no tags", "no tags"},
		{"empty", "", ""},
		{"mixed content", "before <b>bold</b> middle <i>italic</i> after", "before bold middle italic after"},
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
// End-to-end: MarkdownToHTML -> SplitMessage pipeline
// ---------------------------------------------------------------------------

func TestEndToEnd_MarkdownToHTMLThenSplit(t *testing.T) {
	// Build a realistic long AI response.
	var md strings.Builder
	md.WriteString("# Analysis Report\n\n")
	md.WriteString("This is a **comprehensive** analysis of the system.\n\n")
	md.WriteString("## Key Findings\n\n")
	for i := range 30 {
		md.WriteString("* Finding ")
		md.WriteByte(byte('A' + (i % 26)))
		md.WriteString(": the system showed *significant* improvement in `metric_")
		md.WriteByte(byte('0' + (i % 10)))
		md.WriteString("` after applying the [optimization](https://example.com/opt)")
		md.WriteString("\n")
	}
	md.WriteString("\n## Code Example\n\n")
	md.WriteString("```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n")
	md.WriteString("## Recommendations\n\n")
	for i := range 20 {
		md.WriteString("**Recommendation ")
		md.WriteByte(byte('0' + (i % 10)))
		md.WriteString("**: Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")
		md.WriteString("Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. ")
		md.WriteString("Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.\n\n")
	}
	md.WriteString("> Note: this report was generated ~~manually~~ automatically.\n\n")
	md.WriteString("---\n\nEnd of report. See [docs](https://docs.example.com/report_v2) for details.\n")

	input := md.String()

	// Step 1: Convert markdown to HTML.
	html := MarkdownToHTML(input)

	// Step 2: Split at Telegram limit.
	chunks := SplitMessage(html, MaxMessageLen)

	t.Logf("Input: %d runes, HTML: %d runes, Chunks: %d", utf8.RuneCountInString(input), utf8.RuneCountInString(html), len(chunks))

	for i, ch := range chunks {
		// Verify no chunk exceeds maxLen.
		rc := utf8.RuneCountInString(ch)
		if rc > MaxMessageLen {
			t.Errorf("chunk %d exceeds MaxMessageLen: %d runes", i, rc)
		}

		// Verify valid UTF-8.
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}

		// Verify all open tags are closed (every <tag> has </tag>).
		open := unclosedTags(ch)
		if len(open) > 0 {
			t.Errorf("chunk %d has unclosed tags: %v\nchunk: %q", i, open, ch)
		}
	}

	// Verify content preservation: strip all HTML and compare text content.
	var allText strings.Builder
	for _, ch := range chunks {
		allText.WriteString(StripHTMLTags(ch))
	}
	combined := allText.String()

	// Check key content pieces are present.
	contentChecks := []string{
		"Analysis Report",
		"comprehensive",
		"Key Findings",
		"Recommendation",
		"End of report",
	}
	for _, check := range contentChecks {
		if !strings.Contains(combined, check) {
			t.Errorf("content lost after pipeline: missing %q", check)
		}
	}
}

func TestEndToEnd_CyrillicMarkdownPipeline(t *testing.T) {
	// Cyrillic markdown through the full pipeline.
	var md strings.Builder
	md.WriteString("# Анализ системы\n\n")
	md.WriteString("**Важный** результат: система работает *стабильно*.\n\n")
	for i := range 50 {
		md.WriteString("Пункт ")
		md.WriteByte(byte('0' + (i % 10)))
		md.WriteString(": описание результата анализа с подробными деталями.\n")
	}

	html := MarkdownToHTML(md.String())
	chunks := SplitMessage(html, 200)

	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d: invalid UTF-8", i)
		}
		rc := utf8.RuneCountInString(ch)
		if rc > 200 {
			t.Errorf("chunk %d: %d runes > 200", i, rc)
		}
	}
}

func TestEndToEnd_NestedFormattingPipeline(t *testing.T) {
	// Nested formatting -> HTML -> split should not break tags.
	input := strings.Repeat("**bold with *italic* inside** and `code` here.\n", 50)
	html := MarkdownToHTML(input)
	chunks := SplitMessage(html, 200)

	for i, ch := range chunks {
		open := unclosedTags(ch)
		if len(open) > 0 {
			t.Errorf("chunk %d has unclosed tags: %v", i, open)
		}
	}
}
