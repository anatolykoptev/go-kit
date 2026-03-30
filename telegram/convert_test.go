package telegram

import (
	"strings"
	"testing"
)

func TestMarkdownToHTML_Empty(t *testing.T) {
	if got := MarkdownToHTML(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMarkdownToHTML_Bold(t *testing.T) {
	got := MarkdownToHTML("**hello**")
	want := "<b>hello</b>"
	if got != want {
		t.Errorf("bold: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_ItalicStar(t *testing.T) {
	got := MarkdownToHTML("*hello*")
	want := "<i>hello</i>"
	if got != want {
		t.Errorf("italic star: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_Heading(t *testing.T) {
	got := MarkdownToHTML("## Title")
	want := "<b>Title</b>"
	if got != want {
		t.Errorf("heading: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_Link(t *testing.T) {
	got := MarkdownToHTML("[click](https://example.com)")
	want := `<a href="https://example.com">click</a>`
	if got != want {
		t.Errorf("link: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_InlineCodeProtected(t *testing.T) {
	// Inline code should NOT have bold applied inside it.
	// Content is extracted raw, then only EscapeHTML (& < >) is applied.
	got := MarkdownToHTML("`**not bold**`")
	want := "<code>**not bold**</code>"
	if got != want {
		t.Errorf("inline code protected: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_CodeBlockWithLanguage(t *testing.T) {
	input := "```go\nfmt.Println()\n```"
	got := MarkdownToHTML(input)
	want := "<pre><code class=\"language-go\">fmt.Println()\n</code></pre>"
	if got != want {
		t.Errorf("code block with lang: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_HTMLEscape(t *testing.T) {
	got := MarkdownToHTML("a < b & c > d")
	want := "a &lt; b &amp; c &gt; d"
	if got != want {
		t.Errorf("html escape: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_Strikethrough(t *testing.T) {
	got := MarkdownToHTML("~~deleted~~")
	want := "<s>deleted</s>"
	if got != want {
		t.Errorf("strikethrough: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_BoldItalicTripleStar(t *testing.T) {
	got := MarkdownToHTML("***both***")
	want := "<b><i>both</i></b>"
	if got != want {
		t.Errorf("bold-italic: got %q, want %q", got, want)
	}
}

func TestMarkdownToHTML_ListItemsNotItalic(t *testing.T) {
	got := MarkdownToHTML("* item one\n* item two")
	// List items should become bullet points, not italic.
	if strings.Contains(got, "<i>") {
		t.Errorf("list items should not be italic: got %q", got)
	}
	if !strings.Contains(got, "\u2022 item one") {
		t.Errorf("expected bullet point, got %q", got)
	}
}

func TestMarkdownToHTML_HorizontalRule(t *testing.T) {
	got := MarkdownToHTML("above\n---\nbelow")
	if !strings.Contains(got, "\u2014\u2014\u2014") {
		t.Errorf("expected em-dash rule, got %q", got)
	}
}

func TestMarkdownToHTML_Blockquote(t *testing.T) {
	got := MarkdownToHTML("> quoted text")
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("expected blockquote tag, got %q", got)
	}
	if !strings.Contains(got, "quoted text") {
		t.Errorf("expected quoted content, got %q", got)
	}
}

// --- StripMarkdown tests ---

func TestStripMarkdown_Bold(t *testing.T) {
	got := StripMarkdown("**hello**")
	want := "hello"
	if got != want {
		t.Errorf("strip bold: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_Italic(t *testing.T) {
	got := StripMarkdown("*hello*")
	want := "hello"
	if got != want {
		t.Errorf("strip italic: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_Heading(t *testing.T) {
	got := StripMarkdown("## Title")
	want := "Title"
	if got != want {
		t.Errorf("strip heading: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_Link(t *testing.T) {
	got := StripMarkdown("[click](https://example.com)")
	want := "click (https://example.com)"
	if got != want {
		t.Errorf("strip link: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_InlineCode(t *testing.T) {
	got := StripMarkdown("`code`")
	want := "code"
	if got != want {
		t.Errorf("strip inline code: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_Strikethrough(t *testing.T) {
	got := StripMarkdown("~~deleted~~")
	want := "deleted"
	if got != want {
		t.Errorf("strip strikethrough: got %q, want %q", got, want)
	}
}

func TestStripMarkdown_CodeFence(t *testing.T) {
	got := StripMarkdown("```go\ncode\n```")
	want := "code\n"
	if got != want {
		t.Errorf("strip code fence: got %q, want %q", got, want)
	}
}

// --- CloseUnclosedMarkdown tests ---

func TestCloseUnclosedMarkdown_CompleteText(t *testing.T) {
	input := "**bold** and *italic*"
	got := CloseUnclosedMarkdown(input)
	if got != input {
		t.Errorf("complete text should be unchanged: got %q, want %q", got, input)
	}
}

func TestCloseUnclosedMarkdown_UnclosedBold(t *testing.T) {
	got := CloseUnclosedMarkdown("**unclosed bold")
	want := "**unclosed bold**"
	if got != want {
		t.Errorf("unclosed bold: got %q, want %q", got, want)
	}
}

func TestCloseUnclosedMarkdown_UnclosedItalic(t *testing.T) {
	got := CloseUnclosedMarkdown("*unclosed italic")
	want := "*unclosed italic*"
	if got != want {
		t.Errorf("unclosed italic: got %q, want %q", got, want)
	}
}

func TestCloseUnclosedMarkdown_UnclosedCodeFence(t *testing.T) {
	got := CloseUnclosedMarkdown("```go\nsome code")
	want := "```go\nsome code\n```"
	if got != want {
		t.Errorf("unclosed code fence: got %q, want %q", got, want)
	}
}

func TestCloseUnclosedMarkdown_UnclosedStrikethrough(t *testing.T) {
	got := CloseUnclosedMarkdown("~~unclosed strike")
	want := "~~unclosed strike~~"
	if got != want {
		t.Errorf("unclosed strikethrough: got %q, want %q", got, want)
	}
}

func TestCloseUnclosedMarkdown_UnclosedBacktick(t *testing.T) {
	got := CloseUnclosedMarkdown("`unclosed code")
	want := "`unclosed code`"
	if got != want {
		t.Errorf("unclosed backtick: got %q, want %q", got, want)
	}
}
