package telegram

import (
	"regexp"
	"strings"
)

// Format represents the detected markup format of a text string.
type Format int

const (
	// FormatPlain indicates plain text with no markup.
	FormatPlain Format = iota
	// FormatHTML indicates HTML markup (Telegram-compatible or raw HTML).
	FormatHTML
	// FormatMarkdown indicates Markdown markup.
	FormatMarkdown
)

// String returns a human-readable name for the format.
func (f Format) String() string {
	switch f {
	case FormatHTML:
		return "HTML"
	case FormatMarkdown:
		return "Markdown"
	default:
		return "Plain"
	}
}

// fieldSet builds a set from a space-separated list of names.
func fieldSet(names string) map[string]bool {
	set := map[string]bool{}
	for _, n := range strings.Fields(names) {
		set[n] = true
	}
	return set
}

// htmlTagNames is the whitelist of tags that route text to the HTML path.
// Every name here is handled by SanitizeHTML (kept, renamed or flattened).
var htmlTagNames = fieldSet("b strong i em u ins s strike del a code pre blockquote " +
	"span p div br ul ol li tg-spoiler h1 h2 h3 h4 h5 h6")

// htmlVoidTags never take a closing tag, so they do not count toward balance.
var htmlVoidTags = fieldSet("br")

// htmlNoAttrTags are pure formatting tags that must not carry attributes; a
// match like `<b and c>` is comparison text, not markup.
var htmlNoAttrTags = fieldSet("b strong i em u ins s strike del tg-spoiler br")

// reHTMLTagAt matches one well-formed tag anchored at the start of the input:
// `<name attr="v" flag>`, `</name>` or `<name/>`.
var reHTMLTagAt = regexp.MustCompile(
	`^<(/?)([A-Za-z][A-Za-z0-9-]*)((?:\s+[A-Za-z][A-Za-z0-9_:-]*(?:\s*=\s*(?:"[^"<>]*"|'[^'<>]*'))?)*)\s*(/?)>`,
)

var (
	reDetectFence  = regexp.MustCompile("(?s)```.*?```")
	reDetectInline = regexp.MustCompile("`[^`\\n]+`")
)

// isTagLikeStart reports whether text[i:] begins a tag-like token: `<`
// followed by a letter or `/`. Comparisons (`a < b`, `x<5`, `<-`) are not.
func isTagLikeStart(text string, i int) bool {
	if text[i] != '<' || i+1 >= len(text) {
		return false
	}
	c := text[i+1]
	return c == '/' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// applyHTMLTag validates one matched tag against the whitelist and updates
// the open-tag depth. It reports false for a tag that disqualifies the text.
func applyHTMLTag(m []string, depth map[string]int) bool {
	closing, name, attrs, selfClose := m[1] == "/", strings.ToLower(m[2]), m[3], m[4] == "/"
	if !htmlTagNames[name] || (closing && (attrs != "" || selfClose)) {
		return false
	}
	if attrs != "" && htmlNoAttrTags[name] {
		return false
	}
	switch {
	case htmlVoidTags[name] || selfClose:
	case closing:
		if depth[name]--; depth[name] < 0 {
			return false
		}
	default:
		depth[name]++
	}
	return true
}

// looksLikeHTML reports whether text is made of well-formed, whitelisted
// Telegram/HTML tags that are balanced. Code fences and inline code are
// ignored (their `<` is literal). Any tag-like token that is not a
// whitelisted well-formed tag, or an unbalanced tag, disqualifies the text:
// it is then treated as Markdown/plain, whose escaping never drops text.
func looksLikeHTML(text string) bool {
	text = reDetectFence.ReplaceAllString(text, "")
	text = reDetectInline.ReplaceAllString(text, "")

	depth := map[string]int{}
	seen := false
	for i := 0; i < len(text); i++ {
		if !isTagLikeStart(text, i) {
			continue
		}
		m := reHTMLTagAt.FindStringSubmatch(text[i:])
		if m == nil || !applyHTMLTag(m, depth) {
			return false
		}
		seen = true
		i += len(m[0]) - 1
	}
	if !seen {
		return false
	}
	for _, n := range depth {
		if n != 0 {
			return false
		}
	}
	return true
}

// reMDHeuristics holds individual markdown signals, each sufficient for detection.
//
// Note: in Go regex \w is ASCII-only ([0-9A-Za-z_]), so the bold heuristics use
// [\p{L}\p{N}_] instead — this also matches non-ASCII letters (Cyrillic, CJK, …)
// so that **Жирный** / __жирный__ / **日本語** are detected as markdown rather
// than misclassified as plain text and sent unformatted with literal ** / __.
var reMDHeuristics = []*regexp.Regexp{
	regexp.MustCompile(`\*\*[\p{L}\p{N}_]`),   // **word (Unicode-aware)
	regexp.MustCompile(`__[\p{L}\p{N}_]`),     // __word (Unicode-aware)
	regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`), // [text](url)
	regexp.MustCompile("(?m)^```"),            // fenced code block
	regexp.MustCompile(`(?m)^# `),             // ATX heading
	regexp.MustCompile(`(?m)^- `),             // list item
}

// Detect heuristically identifies the markup format of text.
// Returns FormatHTML only if the text consists of well-formed, whitelisted,
// balanced HTML tags (see looksLikeHTML); text that merely contains `<b`,
// `<script>` or `a<b` falls through to Markdown/plain so nothing is dropped.
// Returns FormatMarkdown if Markdown patterns are found.
// Returns FormatPlain otherwise.
func Detect(text string) Format {
	if looksLikeHTML(text) {
		return FormatHTML
	}
	for _, re := range reMDHeuristics {
		if re.MatchString(text) {
			return FormatMarkdown
		}
	}
	return FormatPlain
}
