package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// ===========================================================================
// 1. Unicode Edge Cases
// ===========================================================================

func TestHardRed_Unicode_ZWJEmojiSequence(t *testing.T) {
	// Family emoji: 👨‍👩‍👧‍👦 is 7 code points (4 people + 3 ZWJ)
	// but visually 1 "character". The converter must not split it.
	family := "👨\u200D👩\u200D👧\u200D👦"
	runeCount := utf8.RuneCountInString(family)

	t.Run("MarkdownToHTML preserves ZWJ sequence", func(t *testing.T) {
		got := MarkdownToHTML(family)
		if !strings.Contains(got, family) {
			t.Errorf("ZWJ emoji sequence broken: got %q", got)
		}
	})

	t.Run("SplitMessage does not break ZWJ sequence at boundary", func(t *testing.T) {
		// Put the emoji right at the maxLen boundary.
		prefix := strings.Repeat("a", runeCount-1)
		msg := prefix + family
		chunks := SplitMessage(msg, runeCount)
		// The family emoji should be intact in one of the chunks.
		allText := strings.Join(chunks, "")
		if !strings.Contains(allText, family) {
			t.Errorf("ZWJ emoji sequence split across chunks: chunks=%v", chunks)
		}
	})
}

func TestHardRed_Unicode_RTLMixedWithMarkdown(t *testing.T) {
	// Arabic text with markdown bold markers.
	input := "**مرحبا** and *hello*"

	t.Run("MarkdownToHTML with RTL text", func(t *testing.T) {
		got := MarkdownToHTML(input)
		if !strings.Contains(got, "<b>مرحبا</b>") {
			t.Errorf("RTL bold not converted: got %q", got)
		}
		if !strings.Contains(got, "<i>hello</i>") {
			t.Errorf("LTR italic not converted: got %q", got)
		}
	})

	t.Run("StripMarkdown with RTL text", func(t *testing.T) {
		got := StripMarkdown(input)
		if !strings.Contains(got, "مرحبا") {
			t.Errorf("RTL text lost in StripMarkdown: got %q", got)
		}
	})
}

func TestHardRed_Unicode_CombiningCharacters(t *testing.T) {
	// é as e + combining acute accent (U+0301) — 2 code points, 1 visual char.
	combining := "e\u0301" // visually "é"
	precomposed := "é"     // single code point U+00E9

	t.Run("MarkdownToHTML preserves combining chars", func(t *testing.T) {
		input := "**" + combining + "**"
		got := MarkdownToHTML(input)
		if !strings.Contains(got, combining) {
			t.Errorf("combining character lost: got %q", got)
		}
	})

	t.Run("SplitMessage at combining char boundary", func(t *testing.T) {
		// "e" is 1 rune, combining accent is 1 rune.
		// With maxLen=1, the split should ideally not separate them,
		// but since SplitMessage works on runes, it will split.
		// At minimum: no panic, valid UTF-8.
		msg := combining + combining + combining
		chunks := SplitMessage(msg, 1)
		for i, ch := range chunks {
			if !utf8.ValidString(ch) {
				t.Errorf("chunk %d invalid UTF-8: %q", i, ch)
			}
		}
		// Verify all runes preserved.
		total := 0
		for _, ch := range chunks {
			total += utf8.RuneCountInString(ch)
		}
		// combining has 2 runes, so 3 * 2 = 6 runes total.
		if total != 6 {
			t.Errorf("rune count mismatch: got %d, want 6", total)
		}
	})

	_ = precomposed // used only for documentation
}

func TestHardRed_Unicode_OnlyNewlines(t *testing.T) {
	t.Run("MarkdownToHTML with only newlines", func(t *testing.T) {
		got := MarkdownToHTML("\n\n\n")
		// Should not panic; result should be some form of whitespace.
		if !utf8.ValidString(got) {
			t.Errorf("invalid UTF-8 from only-newlines input: %q", got)
		}
	})

	t.Run("SplitMessage with only newlines", func(t *testing.T) {
		chunks := SplitMessage("\n\n\n\n\n", 2)
		// Should not produce empty chunks (all content is newlines, trimmed).
		for i, ch := range chunks {
			if ch == "" {
				t.Errorf("chunk %d is empty from only-newlines input", i)
			}
		}
	})
}

func TestHardRed_Unicode_OnlySpaces(t *testing.T) {
	got := MarkdownToHTML("     ")
	if got != "     " {
		t.Errorf("only-spaces input mangled: got %q", got)
	}
}

func TestHardRed_Unicode_4ByteEmojiAtMaxLenBoundary(t *testing.T) {
	// Single 4-byte emoji exactly at maxLen boundary.
	emoji := "🔥" // 1 rune, 4 bytes
	msg := strings.Repeat("a", 4095) + emoji
	runeCount := utf8.RuneCountInString(msg)
	if runeCount != 4096 {
		t.Fatalf("setup: expected 4096 runes, got %d", runeCount)
	}

	chunks := SplitMessage(msg, 4096)
	if len(chunks) != 1 {
		t.Errorf("4096-rune message with emoji at end should be 1 chunk, got %d", len(chunks))
	}
	if !strings.HasSuffix(chunks[0], emoji) {
		t.Errorf("emoji at boundary lost: last chars = %q", chunks[0][len(chunks[0])-10:])
	}
}

// ===========================================================================
// 2. Markdown Parser Attacks
// ===========================================================================

func TestHardRed_Markdown_UnclosedCodeFence(t *testing.T) {
	// Unclosed code fence at start — should not eat the rest of the text.
	input := "```\nsome code\nmore code"
	got := MarkdownToHTML(input)
	// With no closing ```, the regex won't match.
	// The raw ``` should appear (escaped or not).
	if !utf8.ValidString(got) {
		t.Errorf("invalid UTF-8: %q", got)
	}
	// The text after the unclosed fence should still be present.
	if !strings.Contains(got, "some code") {
		t.Errorf("text after unclosed code fence lost: got %q", got)
	}
}

func TestHardRed_Markdown_NestedCodeFences(t *testing.T) {
	// ``` inside ``` — inner backticks should be treated as content.
	input := "```\n```inner```\n```"
	got := MarkdownToHTML(input)
	if !utf8.ValidString(got) {
		t.Errorf("invalid UTF-8: %q", got)
	}
}

func TestHardRed_Markdown_BoldInsideInlineCode(t *testing.T) {
	// **not bold** inside backticks must not become <b>.
	input := "`**not bold**`"
	got := MarkdownToHTML(input)
	want := "<code>**not bold**</code>"
	if got != want {
		t.Errorf("bold inside inline code:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestHardRed_Markdown_ItalicUnderscoreInURL(t *testing.T) {
	// Underscores in URL should NOT become italic tags.
	input := "[link](http://a_b_c.com/d_e)"
	got := MarkdownToHTML(input)
	if strings.Contains(got, "<i>") {
		t.Errorf("underscore in URL became italic: got %q", got)
	}
	if !strings.Contains(got, `href="http://a_b_c.com/d_e"`) {
		t.Errorf("URL with underscores mangled: got %q", got)
	}
}

func TestHardRed_Markdown_MarkdownInsideHTMLTags(t *testing.T) {
	// If input already has HTML tags with markdown inside.
	// EscapeHTML should escape the tags, so <b> becomes &lt;b&gt;.
	input := "<b>**double bold**</b>"
	got := MarkdownToHTML(input)
	// The <b> and </b> should be escaped to &lt;b&gt; and &lt;/b&gt;.
	// Then **double bold** should become <b>double bold</b>.
	if strings.Contains(got, "<b><b>") || strings.Contains(got, "</b></b>") {
		t.Errorf("double nesting of bold tags: got %q", got)
	}
	if !strings.Contains(got, "&lt;b&gt;") {
		t.Errorf("original <b> not escaped: got %q", got)
	}
}

func TestHardRed_Markdown_EmptyLink(t *testing.T) {
	// []() — empty link text and URL.
	input := "[]()"
	got := MarkdownToHTML(input)
	// reLink requires [^\]]+, so at least one char in text. Empty text shouldn't match.
	// This means []() should pass through as literal text.
	if strings.Contains(got, "<a") {
		t.Errorf("empty link should not produce <a> tag: got %q", got)
	}
}

func TestHardRed_Markdown_LinkWithSpecialCharsInURL(t *testing.T) {
	input := "[x](https://a.com/b?c=d&e=f#g)"
	got := MarkdownToHTML(input)
	// The URL should be preserved in href (not HTML-escaped, since it was extracted before EscapeHTML).
	if !strings.Contains(got, `href="https://a.com/b?c=d&e=f#g"`) {
		t.Errorf("URL special chars mangled: got %q", got)
	}
}

func TestHardRed_Markdown_ConsecutiveBoldItalicNoSpace(t *testing.T) {
	// **bold***italic* — 5 asterisks: 2 close bold, then 1 open italic? Or 3 close bold-italic?
	input := "**bold***italic*"
	got := MarkdownToHTML(input)
	// At minimum, the output should contain the text "bold" and "italic".
	if !strings.Contains(got, "bold") || !strings.Contains(got, "italic") {
		t.Errorf("text content lost: got %q", got)
	}
	// Verify valid HTML nesting (RepairHTMLNesting should fix).
	repaired := RepairHTMLNesting(got)
	if got != repaired {
		t.Logf("output needed repair: before=%q after=%q", got, repaired)
	}
}

func TestHardRed_Markdown_AsteriskInMathContext(t *testing.T) {
	// "2 * 3 = 6" — the single * should NOT become italic tags.
	// Because reItalicStar requires [^*\n]+, "2 " and " 3 = 6" match, but
	// "2 * 3" has spaces around *, so let's see if the regex eats it.
	input := "2 * 3 = 6"
	got := MarkdownToHTML(input)
	if strings.Contains(got, "<i>") {
		t.Errorf("math asterisk became italic: got %q", got)
	}
	if !strings.Contains(got, "2") || !strings.Contains(got, "3") || !strings.Contains(got, "6") {
		t.Errorf("math content lost: got %q", got)
	}
}

func TestHardRed_Markdown_UnderscoreInVariableName(t *testing.T) {
	// my_var_name — underscores between word chars should NOT become italic.
	input := "my_var_name"
	got := MarkdownToHTML(input)
	if strings.Contains(got, "<i>") {
		t.Errorf("variable name underscores became italic: got %q", got)
	}
	if !strings.Contains(got, "my_var_name") || strings.Contains(got, "my<i>var</i>name") {
		t.Errorf("variable name mangled: got %q", got)
	}
}

func TestHardRed_Markdown_TripleBacktickWithoutNewline(t *testing.T) {
	// ```code``` on a single line — should it be treated as a code block?
	input := "```code```"
	got := MarkdownToHTML(input)
	// The regex reCodeBlock expects optional \n after ```. "```code```" should match
	// with lang="" and code="code".
	if !strings.Contains(got, "code") {
		t.Errorf("triple backtick inline content lost: got %q", got)
	}
}

func TestHardRed_Markdown_BlockquoteWithFormattingInside(t *testing.T) {
	input := "> **bold** and *italic*"
	got := MarkdownToHTML(input)
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("blockquote tag missing: got %q", got)
	}
	if !strings.Contains(got, "<b>bold</b>") {
		t.Errorf("bold inside blockquote not converted: got %q", got)
	}
	if !strings.Contains(got, "<i>italic</i>") {
		t.Errorf("italic inside blockquote not converted: got %q", got)
	}
}

func TestHardRed_Markdown_MultipleHeadings(t *testing.T) {
	input := "# H1\n## H2\n### H3"
	got := MarkdownToHTML(input)
	if !strings.Contains(got, "<b>H1</b>") {
		t.Errorf("H1 not converted: got %q", got)
	}
	if !strings.Contains(got, "<b>H2</b>") {
		t.Errorf("H2 not converted: got %q", got)
	}
	if !strings.Contains(got, "<b>H3</b>") {
		t.Errorf("H3 not converted: got %q", got)
	}
}

func TestHardRed_Markdown_HeadingWithSpecialChars(t *testing.T) {
	input := `# Title: "quoted" & <angled>`
	got := MarkdownToHTML(input)
	// Heading should be bold, special chars should be escaped.
	if !strings.Contains(got, "<b>") {
		t.Errorf("heading not bold: got %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("ampersand not escaped: got %q", got)
	}
	if !strings.Contains(got, "&lt;angled&gt;") {
		t.Errorf("angle brackets not escaped: got %q", got)
	}
}

// ===========================================================================
// 3. SplitMessage Stress
// ===========================================================================

func TestHardRed_Split_Exactly4096Runes(t *testing.T) {
	msg := strings.Repeat("a", 4096)
	chunks := SplitMessage(msg, 4096)
	if len(chunks) != 1 {
		t.Errorf("4096-rune message should be 1 chunk, got %d", len(chunks))
	}
}

func TestHardRed_Split_4097Runes(t *testing.T) {
	msg := strings.Repeat("a", 4097)
	chunks := SplitMessage(msg, 4096)
	if len(chunks) != 2 {
		t.Errorf("4097-rune message should be 2 chunks, got %d", len(chunks))
	}
	total := 0
	for _, ch := range chunks {
		total += utf8.RuneCountInString(ch)
	}
	if total != 4097 {
		t.Errorf("runes lost: got %d, want 4097", total)
	}
}

func TestHardRed_Split_LongHrefTag(t *testing.T) {
	// Single HTML tag with 100+ char href — must not be split mid-tag.
	longURL := "https://example.com/" + strings.Repeat("x", 100)
	msg := `<a href="` + longURL + `">click</a>` + strings.Repeat("\ny", 200)
	chunks := SplitMessage(msg, 80)

	for i, ch := range chunks {
		// Check that no chunk has an incomplete tag (< without matching >).
		opens := strings.Count(ch, "<")
		closes := strings.Count(ch, ">")
		if opens != closes {
			t.Errorf("chunk %d has mismatched angle brackets (%d < vs %d >): %q", i, opens, closes, ch)
		}
	}
}

func TestHardRed_Split_ReopenedTagsPlusContentExceedMaxLen(t *testing.T) {
	// Deeply nested tags: when reopened on next chunk, the tags alone
	// may approach maxLen. Verify no chunk exceeds maxLen.
	tags := "<b><i><s><u><code>"
	closers := "</code></u></s></i></b>"
	content := strings.Repeat("x", 50) + "\n" + strings.Repeat("y", 50)
	msg := tags + content + closers
	maxLen := 40

	chunks := SplitMessage(msg, maxLen)
	for i, ch := range chunks {
		rc := utf8.RuneCountInString(ch)
		if rc > maxLen {
			t.Errorf("chunk %d exceeds maxLen %d: %d runes: %q", i, maxLen, rc, ch)
		}
	}
}

func TestHardRed_Split_10LevelsNestedTags(t *testing.T) {
	// 10 levels of nesting spanning a boundary.
	var open, close strings.Builder
	for _, tag := range []string{"b", "i", "s", "u", "code", "b", "i", "s", "u", "code"} {
		open.WriteString("<" + tag + ">")
		close.WriteString("</" + tag + ">") // Note: wrong order, but let's test handling.
	}
	content := strings.Repeat("x", 30) + "\n" + strings.Repeat("y", 30)
	msg := open.String() + content + close.String()

	chunks := SplitMessage(msg, 50)
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8", i)
		}
		rc := utf8.RuneCountInString(ch)
		if rc > 50 {
			t.Errorf("chunk %d exceeds maxLen 50: %d runes", i, rc)
		}
	}
}

func TestHardRed_Split_EmptyTagsAtBoundary(t *testing.T) {
	// <b></b> right at the split point.
	msg := strings.Repeat("a", 30) + "<b></b>" + "\n" + strings.Repeat("b", 30)
	chunks := SplitMessage(msg, 35)
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8", i)
		}
	}
}

func TestHardRed_Split_AllNewlines(t *testing.T) {
	// Message that's all newlines — should not produce empty chunks.
	msg := strings.Repeat("\n", 100)
	chunks := SplitMessage(msg, 10)
	for i, ch := range chunks {
		if ch == "" {
			t.Errorf("chunk %d is empty from all-newlines message", i)
		}
	}
}

func TestHardRed_Split_MaxLenOne(t *testing.T) {
	// maxLen=1 — should not panic, should produce single-rune chunks.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SplitMessage panicked with maxLen=1: %v", r)
		}
	}()

	msg := "hello"
	chunks := SplitMessage(msg, 1)
	if len(chunks) < 5 {
		t.Errorf("expected at least 5 chunks for 'hello' with maxLen=1, got %d: %v", len(chunks), chunks)
	}
	for i, ch := range chunks {
		rc := utf8.RuneCountInString(ch)
		if rc > 1 {
			t.Errorf("chunk %d has %d runes, want <=1: %q", i, rc, ch)
		}
	}
}

func TestHardRed_Split_MaxLenZero(t *testing.T) {
	// maxLen=0 — should not panic or infinite loop.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SplitMessage panicked with maxLen=0: %v", r)
		}
	}()

	msg := "hello world"
	chunks := SplitMessage(msg, 0)
	if len(chunks) == 0 {
		t.Errorf("expected at least 1 chunk, got 0")
	}
}

func TestHardRed_Split_MaxLenNegative(t *testing.T) {
	// maxLen=-1 — should not panic or infinite loop.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SplitMessage panicked with maxLen=-1: %v", r)
		}
	}()

	msg := "hello world"
	chunks := SplitMessage(msg, -1)
	if len(chunks) == 0 {
		t.Errorf("expected at least 1 chunk, got 0")
	}
}

// ===========================================================================
// 4. RepairHTMLNesting Attacks
// ===========================================================================

func TestHardRed_Repair_DeeplyNested20Levels(t *testing.T) {
	// 20 levels of proper nesting — should pass through unchanged.
	tags := []string{"b", "i", "s", "u", "code"}
	var open, closeReverse strings.Builder
	for i := 0; i < 20; i++ {
		tag := tags[i%len(tags)]
		open.WriteString("<" + tag + ">")
		closeReverse.WriteString("</" + tag + ">")
	}
	// Build reversed closer.
	// We need to reverse the order of close tags (not the string itself).
	var properClose strings.Builder
	for i := 19; i >= 0; i-- {
		tag := tags[i%len(tags)]
		properClose.WriteString("</" + tag + ">")
	}
	input := open.String() + "deep content" + properClose.String()
	got := RepairHTMLNesting(input)
	// Should be unchanged since it's already properly nested.
	if got != input {
		t.Errorf("well-nested 20-level input changed:\n  got:  %q\n  want: %q", got, input)
	}
}

func TestHardRed_Repair_WrongOrderClose(t *testing.T) {
	// Tags opened as <b><i> but closed as </b></i> (wrong order).
	input := "<b><i>text</b></i>"
	got := RepairHTMLNesting(input)
	// After repair, should have valid nesting.
	unclosed := unclosedTags(got)
	if len(unclosed) > 0 {
		t.Errorf("repaired HTML still has unclosed tags %v: %q", unclosed, got)
	}
}

func TestHardRed_Repair_MultipleOrphanClosers(t *testing.T) {
	// Multiple unmatched closing tags — should all be discarded.
	input := "</b></i></s>"
	got := RepairHTMLNesting(input)
	if got != "" {
		t.Errorf("orphan closers should be discarded, got %q", got)
	}
}

func TestHardRed_Repair_TagWithGreaterThanInAttribute(t *testing.T) {
	// <a href="x>y"> — the > inside the attribute value will break the naive parser.
	input := `<a href="x>y">text</a>`
	got := RepairHTMLNesting(input)
	// The naive parser sees <a href="x as a tag, then >y"> as text.
	// At minimum, "text" should survive and result should be valid.
	if !strings.Contains(got, "text") {
		t.Errorf("text content lost: got %q", got)
	}
	// Check if the parser was confused.
	if strings.Contains(got, "</a>") && !strings.Contains(got, `href=`) {
		t.Logf("parser was confused by > in attribute: got %q", got)
	}
}

func TestHardRed_Repair_MalformedTagNoClose(t *testing.T) {
	// <b without > at end of string.
	input := "text<b"
	got := RepairHTMLNesting(input)
	// The incomplete tag should be preserved as-is (no matching >).
	if got != "text<b" {
		t.Errorf("incomplete tag handling: got %q, want %q", got, "text<b")
	}
}

func TestHardRed_Repair_SelfClosingBR(t *testing.T) {
	// <br/> — self-closing tag in Telegram context.
	input := "line1<br/>line2"
	got := RepairHTMLNesting(input)
	// <br/> is not in trackedTags, so it should pass through without stacking.
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("content around <br/> lost: got %q", got)
	}
	// Should not add a </br> closer.
	if strings.Contains(got, "</br>") {
		t.Errorf("self-closing <br/> got a spurious closer: got %q", got)
	}
}

func TestHardRed_Repair_OrphanCloserBetweenValidTags(t *testing.T) {
	// </s> orphan between valid tags.
	input := "<b>bold</b></s><i>italic</i>"
	got := RepairHTMLNesting(input)
	// The orphan </s> should be discarded, rest should be intact.
	want := "<b>bold</b><i>italic</i>"
	if got != want {
		t.Errorf("orphan closer between valid tags:\n  got:  %q\n  want: %q", got, want)
	}
}

// ===========================================================================
// 5. CompactForTelegram Edge Cases
// ===========================================================================

func TestHardRed_Compact_MaxCharsZero(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CompactForTelegram panicked with maxChars=0: %v", r)
		}
	}()

	got := CompactForTelegram("some text that is longer than zero", 0)
	// Should clamp to compactMinChars (50) and not panic.
	if got == "" {
		t.Errorf("empty result with maxChars=0")
	}
}

func TestHardRed_Compact_MaxCharsOne(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CompactForTelegram panicked with maxChars=1: %v", r)
		}
	}()

	got := CompactForTelegram("some text that is longer than one", 1)
	// Should clamp to compactMinChars (50) and not panic.
	if got == "" {
		t.Errorf("empty result with maxChars=1")
	}
}

func TestHardRed_Compact_OnlyLargeCodeBlock(t *testing.T) {
	// Text that is ONLY a large code block.
	code := strings.Repeat("x", 600)
	input := "```go\n" + code + "\n```"
	got := CompactForTelegram(input, 100)
	if strings.Contains(got, code) {
		t.Errorf("large code block should be trimmed")
	}
	if !strings.Contains(got, "(code block trimmed)") {
		t.Errorf("trimmed placeholder missing: got %q", got)
	}
}

func TestHardRed_Compact_VerboseSectionAtVeryStart(t *testing.T) {
	// Verbose section heading at position < 200 — should not truncate there
	// (the code requires loc[0] > 200).
	input := "# Raw Logs\n" + strings.Repeat("log line\n", 100)
	got := CompactForTelegram(input, 100)
	// Since the verbose section is at position 0 (< 200), it should NOT
	// truncate at that heading. It should fall through to hard truncate.
	if !strings.Contains(got, "_(truncated)_") {
		t.Errorf("should still be truncated: got %q", got)
	}
}

func TestHardRed_Compact_NegativeMaxChars(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CompactForTelegram panicked with negative maxChars: %v", r)
		}
	}()

	got := CompactForTelegram("some text", -100)
	// Should clamp to compactMinChars.
	if got == "" {
		t.Errorf("empty result with negative maxChars")
	}
}

// ===========================================================================
// 6. CloseUnclosedMarkdown Edge Cases
// ===========================================================================

func TestHardRed_CloseUnclosed_OnlyBoldMarkers(t *testing.T) {
	// Just "**" with no content.
	input := "**"
	got := CloseUnclosedMarkdown(input)
	// There's 1 occurrence of "**", which is odd, so it should close: "****"
	want := "****"
	if got != want {
		t.Errorf("only bold markers: got %q, want %q", got, want)
	}
}

func TestHardRed_CloseUnclosed_TripleAsterisk(t *testing.T) {
	// "***" is ambiguous — bold+italic? After removing **, 1 * remains odd.
	input := "***"
	got := CloseUnclosedMarkdown(input)
	// Algorithm: ** count = 1 (odd), add "**"; then strip ** -> "*", * count = 1 (odd), add "*".
	// So result should be "***" + "**" + "*" = "******"
	// Let's verify at least it doesn't panic and produces something.
	if got == input {
		t.Errorf("triple asterisk should be closed, got unchanged %q", got)
	}
}

func TestHardRed_CloseUnclosed_NestedUnclosedBoldItalic(t *testing.T) {
	// "**bold *italic" — both unclosed.
	input := "**bold *italic"
	got := CloseUnclosedMarkdown(input)
	// Should close both: at minimum, result should convert cleanly.
	html := MarkdownToHTML(got)
	if !utf8.ValidString(html) {
		t.Errorf("closed markdown produces invalid HTML: %q", html)
	}
}

func TestHardRed_CloseUnclosed_CodeFenceWithBackticksInside(t *testing.T) {
	// ``` with `code` inside — the inner backticks are content.
	input := "```\n`code`\n"
	got := CloseUnclosedMarkdown(input)
	// Count of ``` is 1 (odd), so should add \n```.
	if !strings.HasSuffix(got, "\n```") {
		t.Errorf("unclosed code fence with inner backticks: got %q", got)
	}
}

func TestHardRed_CloseUnclosed_EmptyString(t *testing.T) {
	got := CloseUnclosedMarkdown("")
	if got != "" {
		t.Errorf("empty string should return empty: got %q", got)
	}
}

func TestHardRed_CloseUnclosed_EvenBackticksShouldNotClose(t *testing.T) {
	// "``" — 2 backticks, even count. Should not add any suffix.
	input := "``"
	got := CloseUnclosedMarkdown(input)
	if got != input {
		t.Errorf("even backticks should not be modified: got %q, want %q", got, input)
	}
}

// ===========================================================================
// 7. EscapeHTML + SanitizeUTF8 Edge Cases
// ===========================================================================

func TestHardRed_EscapeHTML_AlreadyEscaped(t *testing.T) {
	// Already-escaped text should be double-escaped.
	input := "&amp;"
	got := EscapeHTML(input)
	want := "&amp;amp;"
	if got != want {
		t.Errorf("double escape: got %q, want %q", got, want)
	}
}

func TestHardRed_EscapeHTML_AllEntities(t *testing.T) {
	input := `<script>alert("xss")&</script>`
	got := EscapeHTML(input)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("angle brackets not escaped: got %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("ampersand not escaped: got %q", got)
	}
}

func TestHardRed_SanitizeUTF8_MixedInvalidWithNullBytes(t *testing.T) {
	input := "hello\x00\xff\xfeworld\x00ok"
	got := SanitizeUTF8(input)
	if strings.Contains(got, "\x00") {
		t.Errorf("null bytes not removed: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") || !strings.Contains(got, "ok") {
		t.Errorf("valid parts lost: %q", got)
	}
}

func TestHardRed_SanitizeUTF8_EmptyString(t *testing.T) {
	got := SanitizeUTF8("")
	if got != "" {
		t.Errorf("empty string should return empty: got %q", got)
	}
}

func TestHardRed_SanitizeUTF8_OnlyNullBytes(t *testing.T) {
	got := SanitizeUTF8("\x00\x00\x00")
	if got != "" {
		t.Errorf("only null bytes should return empty: got %q", got)
	}
}

func TestHardRed_SanitizeUTF8_OnlyInvalidBytes(t *testing.T) {
	got := SanitizeUTF8("\xff\xfe\xfd")
	if got != "" {
		t.Errorf("only invalid bytes should return empty: got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result not valid UTF-8: %q", got)
	}
}

// ===========================================================================
// 8. Cross-Function Interaction Stress
// ===========================================================================

func TestHardRed_Pipeline_MarkdownToHTML_ThenSplit_UnderscoreVarNames(t *testing.T) {
	// Variable names with underscores in a markdown context, piped through the full chain.
	input := "The variable `my_var_name` and function `get_user_data()` are important.\n"
	input = strings.Repeat(input, 50) // Make it long enough to split.

	html := MarkdownToHTML(input)
	chunks := SplitMessage(html, 200)

	for i, ch := range chunks {
		// Variable names should be in <code> tags, not italicized.
		if strings.Contains(ch, "<i>var</i>") || strings.Contains(ch, "<i>user</i>") {
			t.Errorf("chunk %d: underscore in code became italic: %q", i, ch)
		}
		rc := utf8.RuneCountInString(ch)
		if rc > 200 {
			t.Errorf("chunk %d exceeds maxLen: %d runes", i, rc)
		}
	}
}

func TestHardRed_Pipeline_FullChainWithEmojiAndCyrillic(t *testing.T) {
	// Mix of emoji, Cyrillic, markdown, and HTML-unsafe chars.
	input := "# Привет 🔥\n\n**Важно**: формула `a < b & c > d` работает!\n\n" +
		"* Пункт 1: 👨\u200D👩\u200D👧\u200D👦 семья\n" +
		"* Пункт 2: ~~удалено~~ *курсив*\n\n" +
		"```go\nfmt.Println(\"Привет мир\")\n```\n"

	html := MarkdownToHTML(input)

	// HTML-unsafe chars inside code should be escaped.
	if !strings.Contains(html, "&lt;") || !strings.Contains(html, "&gt;") || !strings.Contains(html, "&amp;") {
		t.Errorf("HTML entities not escaped in formula: %q", html)
	}

	// Emoji should survive.
	if !strings.Contains(html, "🔥") {
		t.Errorf("emoji lost: %q", html)
	}

	// Split should produce valid UTF-8 chunks.
	chunks := SplitMessage(html, 100)
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8", i)
		}
	}
}

func TestHardRed_Markdown_BareUnderscoresInProse(t *testing.T) {
	// Common prose with underscores that should NOT be italic.
	// file_name, MAX_VALUE, __init__ — these appear in technical writing.
	tests := []struct {
		name  string
		input string
	}{
		{"snake_case variable", "Use my_variable_name here"},
		{"constant", "Set MAX_RETRY_COUNT to 5"},
		{"Python dunder", "Define __init__ method"},
		{"multiple underscores", "a_b_c_d_e_f"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MarkdownToHTML(tc.input)
			if strings.Contains(got, "<i>") {
				t.Errorf("underscores in %q became italic: got %q", tc.input, got)
			}
		})
	}
}

func TestHardRed_StripHTMLTags_UnclosedTag(t *testing.T) {
	// Unclosed tag at end of string — everything after < is eaten.
	input := "text<b"
	got := StripHTMLTags(input)
	// Since there's no >, the parser stays inTag=true and eats "b".
	// The result should still contain "text".
	if !strings.Contains(got, "text") {
		t.Errorf("text before unclosed tag lost: got %q", got)
	}
	// But "b" after < will be eaten. Is this the intended behavior?
	if got != "text" {
		t.Logf("unclosed tag ate trailing content: got %q (may be intended)", got)
	}
}

func TestHardRed_Split_HTMLTagLongerThanMaxLen(t *testing.T) {
	// A single HTML tag (long href) that's longer than maxLen.
	// This is a degenerate case — the tag itself can't fit in one chunk.
	longURL := strings.Repeat("x", 100)
	msg := `<a href="` + longURL + `">click here</a>`
	chunks := SplitMessage(msg, 50)

	// Should not panic. Content should be split somehow.
	if len(chunks) == 0 {
		t.Errorf("no chunks produced for long-tag message")
	}
	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8", i)
		}
	}
}

func TestHardRed_Repair_EmptyString(t *testing.T) {
	got := RepairHTMLNesting("")
	if got != "" {
		t.Errorf("empty input should return empty: got %q", got)
	}
}

func TestHardRed_Repair_PlainTextNoTags(t *testing.T) {
	input := "just plain text with no tags at all"
	got := RepairHTMLNesting(input)
	if got != input {
		t.Errorf("plain text changed: got %q", got)
	}
}

func TestHardRed_Repair_OnlyOpenTags(t *testing.T) {
	input := "<b><i><code>"
	got := RepairHTMLNesting(input)
	want := "<b><i><code></code></i></b>"
	if got != want {
		t.Errorf("only open tags:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestHardRed_Repair_OnlyCloseTags(t *testing.T) {
	input := "</b></i></code>"
	got := RepairHTMLNesting(input)
	// All orphan closers should be discarded.
	if got != "" {
		t.Errorf("only close tags should produce empty: got %q", got)
	}
}
