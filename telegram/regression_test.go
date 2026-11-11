package telegram

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/strutil"
)

// reAmpNotEntity finds an `&` that does not start a complete entity — what
// Telegram rejects with "can't parse entities".
var reAmpNotEntity = regexp.MustCompile(`&(?:#[0-9]*|#[xX][0-9a-fA-F]*|[A-Za-z][A-Za-z0-9]*)?(?:[^A-Za-z0-9#;]|$)`)

func assertChunksWellFormed(t *testing.T, chunks []string, maxLen int) {
	t.Helper()
	for i, c := range chunks {
		if n := strutil.UTF16Len(c); n > maxLen {
			t.Errorf("chunk %d is %d units, over %d", i, n, maxLen)
		}
		if loc := reAmpNotEntity.FindStringIndex(c); loc != nil {
			t.Errorf("chunk %d has a broken entity near %q", i, c[loc[0]:min(loc[1]+8, len(c))])
		}
		if open := unclosedTags(c); len(open) != 0 {
			t.Errorf("chunk %d leaves tags open: %v", i, open)
		}
	}
}

func TestSplitMessage_EntitySafe_LongLine(t *testing.T) {
	text := strings.Repeat("a&amp;", 2000) // 12000 chars, no newline
	chunks := SplitMessage(text, MaxMessageLen)
	if len(chunks) < 3 {
		t.Fatalf("expected >=3 chunks, got %d", len(chunks))
	}
	assertChunksWellFormed(t, chunks, MaxMessageLen)
	if got := strings.Join(chunks, ""); got != text {
		t.Errorf("text changed by split: len %d vs %d", len(got), len(text))
	}
}

func TestSplitMessage_EntitySafe_Boundary(t *testing.T) {
	// The entity straddles the 4096 cut: '&' at unit 4094, ';' at 4098.
	for _, ent := range []string{"&amp;", "&#38;", "&#x26;", "&lt;", "&quot;"} {
		t.Run(ent, func(t *testing.T) {
			head := strings.Repeat("x", MaxMessageLen-2)
			text := head + ent + strings.Repeat("y", 200)
			chunks := SplitMessage(text, MaxMessageLen)
			if len(chunks) != 2 {
				t.Fatalf("want 2 chunks, got %d", len(chunks))
			}
			assertChunksWellFormed(t, chunks, MaxMessageLen)
			if chunks[0] != head {
				t.Errorf("split should land before the entity; chunk0 tail %q", chunks[0][len(chunks[0])-6:])
			}
			if !strings.HasPrefix(chunks[1], ent) {
				t.Errorf("entity must open chunk 1, got %q", chunks[1][:8])
			}
		})
	}
}

func TestSplitMessage_BareAmpersandStillSplits(t *testing.T) {
	// A '&' that never becomes an entity must not pin the split point.
	text := strings.Repeat("a & b ", 1500)
	chunks := SplitMessage(text, MaxMessageLen)
	if len(chunks) < 2 {
		t.Fatalf("want split, got %d chunk(s)", len(chunks))
	}
	if got := strings.Join(chunks, ""); got != text {
		t.Error("text changed by split")
	}
}

func TestSplitMessage_LongBoldRunKeepsAllText(t *testing.T) {
	for name, body := range map[string]string{
		"words":   strings.Repeat("word ", 1200), // 6000 chars
		"solid":   strings.Repeat("x", 6000),
		"entity":  strings.Repeat("a&amp;", 1000),
		"newline": strings.Repeat("line of text\n", 500),
	} {
		t.Run(name, func(t *testing.T) {
			text := "<b>" + body + "</b>"
			chunks := SplitMessage(text, MaxMessageLen)
			if len(chunks) < 2 {
				t.Fatalf("want >=2 chunks, got %d", len(chunks))
			}
			assertChunksWellFormed(t, chunks, MaxMessageLen)
			var got strings.Builder
			for i, c := range chunks {
				if !strings.HasPrefix(c, "<b>") || !strings.HasSuffix(c, "</b>") {
					t.Errorf("chunk %d not wrapped in <b>…</b>: %.12q…%.12q", i, c, c[max(0, len(c)-12):])
				}
				got.WriteString(StripHTMLTags(c))
			}
			// Only newlines at chunk joints may be trimmed; nothing else.
			if a, b := strings.ReplaceAll(got.String(), "\n", ""), strings.ReplaceAll(body, "\n", ""); a != b {
				t.Errorf("text dropped or duplicated: got %d chars, want %d", len(a), len(b))
			}
		})
	}
}

func TestSplitMessage_NestedTagsAcrossChunks(t *testing.T) {
	body := strings.Repeat("ab ", 3000)
	text := "<b><i>" + body + "</i></b>"
	chunks := SplitMessage(text, MaxMessageLen)
	assertChunksWellFormed(t, chunks, MaxMessageLen)
	var got strings.Builder
	for _, c := range chunks {
		got.WriteString(StripHTMLTags(c))
	}
	if got.String() != body {
		t.Errorf("text dropped or duplicated: got %d, want %d", got.Len(), len(body))
	}
}

func TestDetect_StrictHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Format
	}{
		{"comparison in code fence", "## Position\n\nHeld:\n- **AAPL** ...\n```go\nx := a<b\n```", FormatMarkdown},
		{"bare lt letter", "if a<b then", FormatPlain},
		{"generic type", "returns Vec<String> here", FormatPlain},
		{"script tag", "see <script>alert(1)</script> there", FormatPlain},
		{"script after bold", "<b>safe</b><script>x()</script>", FormatPlain},
		{"unclosed bold", "<b>never closed", FormatPlain},
		{"stray closer", "oops </b> here", FormatPlain},
		{"attrs on bold", "if x<b and c>d then", FormatPlain},
		{"tag in fence only", "```html\n<b>x</b>\n```", FormatMarkdown},
		{"tag in inline code only", "use `<b>` for bold", FormatPlain},
		{"spaced lt", "x < y and y > z", FormatPlain},
		{"balanced bold", "<b>ok</b>", FormatHTML},
		{"balanced nested", "<b>a <i>b</i></b> tail", FormatHTML},
		{"anchor", `<a href="https://x.com">l</a>`, FormatHTML},
		{"code with class", `<pre><code class="language-go">x</code></pre>`, FormatHTML},
		{"void br", "a<br>b", FormatHTML},
		{"self-closing br", "a<br/>b", FormatHTML},
		{"uppercase", "<B>ok</B>", FormatHTML},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.in); got != tc.want {
				t.Errorf("Detect(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrepareForTelegram_NoTextDropped(t *testing.T) {
	in := "## Position\n\nHeld:\n- **AAPL** ...\n```go\nx := a<b\n```"
	out, _ := PrepareForTelegram(in)
	for _, want := range []string{"<b>AAPL</b>", "x := a&lt;b", "</code></pre>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	in = "Before\n<script>alert(1)</script>\nAfter"
	out, _ = PrepareForTelegram(in)
	for _, want := range []string{"Before", "alert(1)", "After"} {
		if !strings.Contains(out, want) {
			t.Errorf("text %q silently dropped: %q", want, out)
		}
	}
	if strings.Contains(out, "<script") {
		t.Errorf("raw <script> reached output: %q", out)
	}

	for _, in := range []string{"a<b and more text", "Vec<String> is a type", "x <b>never closed"} {
		out, _ := PrepareForTelegram(in)
		if got := html.UnescapeString(StripHTMLTags(out)); !strings.Contains(got, strings.Fields(in)[len(strings.Fields(in))-1]) {
			t.Errorf("tail of %q dropped: %q", in, out)
		}
	}
}

func TestMarkdownToHTML_UnsafeLinks(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantText string // must appear
	}{
		{"javascript", "[click](javascript:alert(1))", "click"},
		{"javascript upper", "[click](JaVaScRiPt:alert(1))", "click"},
		{"data", "[click](data:text/html;base64,AAAA)", "click"},
		{"vbscript", "[click](vbscript:msgbox(1))", "click"},
		{"relative", "[click](/etc/passwd)", "click"},
		{"image js", "![pic](javascript:alert(1))", "pic"},
		{"nested outer unsafe", "[[a](x)](javascript:alert(1))", "a"},
		{"nested both unsafe", "[[a](javascript:1)](javascript:alert(1))", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MarkdownToHTML(tc.in)
			if strings.Contains(got, "<a") || strings.Contains(strings.ToLower(got), "javascript") ||
				strings.Contains(got, "href") {
				t.Errorf("unsafe link survived: %q", got)
			}
			if !strings.Contains(got, tc.wantText) {
				t.Errorf("label %q dropped: %q", tc.wantText, got)
			}
		})
	}
}

func TestMarkdownToHTML_DeepParensNotLinked(t *testing.T) {
	// Only one level of balanced parentheses is supported; deeper nesting is
	// left as literal text rather than guessed at.
	got := MarkdownToHTML("[x](https://a.com/(a(b)))")
	if strings.Contains(got, "<a") {
		t.Errorf("two-level parens must not produce a link: %q", got)
	}
	if !strings.Contains(got, "[x](https://a.com/(a(b)))") {
		t.Errorf("literal text altered: %q", got)
	}
}

func TestMarkdownToHTML_UnsafeLinkTrailingTextIntact(t *testing.T) {
	got := MarkdownToHTML("[click](javascript:alert(1)) tail")
	if got != "click tail" {
		t.Errorf("got %q, want %q", got, "click tail")
	}
}

func TestMarkdownToHTML_SafeLinks(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"https", "[x](https://a.com/b)", `<a href="https://a.com/b">x</a>`},
		{"mailto", "[m](mailto:a@b.co)", `<a href="mailto:a@b.co">m</a>`},
		{"balanced parens", "[w](https://en.wikipedia.org/wiki/Go_(game))", `<a href="https://en.wikipedia.org/wiki/Go_(game)">w</a>`},
		{"balanced parens then text", "(see [w](https://x.com/a_(b)))", `(see <a href="https://x.com/a_(b)">w</a>)`},
		{"nested safe inner", "[[a](https://in.example)](https://out.example)", `<a href="https://out.example">a</a>`},
		{"quote in url", `[x](https://a.com/"onclick=1)`, `<a href="https://a.com/&quot;onclick=1">x</a>`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MarkdownToHTML(tc.in); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestPrepareForTelegram_UnsafeLinkEndToEnd(t *testing.T) {
	out, _ := PrepareForTelegram("see [click](javascript:alert(1)) now")
	if strings.Contains(out, "javascript") || strings.Contains(out, "<a") {
		t.Errorf("unsafe link survived PrepareForTelegram: %q", out)
	}
	if !strings.Contains(out, "click") || !strings.Contains(out, "now") {
		t.Errorf("text dropped: %q", out)
	}
}
