// Package telegram provides SDK-agnostic Telegram formatting utilities.
// Stdlib-only, zero external dependencies.
package telegram

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxMessageLen is the Telegram Bot API limit for a single message.
const MaxMessageLen = 4096

const (
	// htmlTagMinLen is the minimum length of an HTML tag (e.g. "<b>").
	htmlTagMinLen = 3

	// compactMinChars is the minimum allowed maxChars for CompactForTelegram.
	compactMinChars = 50

	// truncateSuffixReserve is the number of runes reserved for the
	// "... _(truncated)_" suffix when hard-truncating.
	truncateSuffixReserve = 30
)

// trackedTags is the set of Telegram-supported formatting tags.
var trackedTags = map[string]bool{
	"b": true, "i": true, "s": true, "u": true,
	"a": true, "code": true, "pre": true, "blockquote": true,
}

// Precompiled regexes for CompactForTelegram.
var (
	reVerboseSection = regexp.MustCompile(
		`(?mi)^#+\s+(recent errors|error log|raw logs?|detailed analysis|full output|stack trace|verbose).*$`,
	)
	reLargeCodeBlock = regexp.MustCompile("(?s)```[\\w]*\n.{500,}?```")
)

// EscapeHTML escapes &, <, > for Telegram HTML mode.
func EscapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// SanitizeUTF8 removes null bytes and invalid UTF-8 sequences.
func SanitizeUTF8(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = strings.ReplaceAll(text, "\x00", "")
	return text
}

// StripHTMLTags removes all HTML tags, returning plain text.
func StripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompactForTelegram truncates verbose LLM responses for Telegram delivery.
// Runs on raw markdown BEFORE HTML conversion. Pass-through if text <= maxChars.
// maxChars is measured in runes (Unicode code points), not bytes.
func CompactForTelegram(text string, maxChars int) string {
	if maxChars < compactMinChars {
		maxChars = compactMinChars
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}

	// Strip large code blocks (>500 chars) -- replace with one-liner.
	text = reLargeCodeBlock.ReplaceAllString(text, "```\n(code block trimmed)\n```")

	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}

	// Find first verbose section heading and truncate there.
	if loc := reVerboseSection.FindStringIndex(text); loc != nil && loc[0] > 200 {
		text = strings.TrimRight(text[:loc[0]], "\n ") + "\n\n... _(truncated)_"
		if utf8.RuneCountInString(text) <= maxChars {
			return text
		}
	}

	// Hard truncate at maxChars on a newline boundary.
	cutOff := runeOffset(text, maxChars-truncateSuffixReserve)
	cut := text[:cutOff]
	if nl := strings.LastIndex(cut, "\n"); nl > len(cut)/2 {
		cut = cut[:nl]
	}
	return strings.TrimRight(cut, "\n ") + "\n\n... _(truncated)_"
}

// tagPos records an open HTML tag and its position in the output buffer.
type tagPos struct {
	tag     string // tag name: "b", "i", "a", etc.
	openTag string // full opening tag: `<a href="...">`, `<b>`, etc.
	start   int
}

// RepairHTMLNesting fixes malformed HTML tag nesting from regex-based conversion.
// Tracks Telegram-supported tags (b, i, s, u, a, code, pre, blockquote).
// Closes unclosed tags, discards unmatched closers, reorders interleaved tags.
func RepairHTMLNesting(html string) string {
	var result strings.Builder
	result.Grow(len(html))
	var stack []tagPos
	i := 0

	for i < len(html) {
		if html[i] != '<' {
			result.WriteByte(html[i])
			i++
			continue
		}

		j := strings.IndexByte(html[i:], '>')
		if j < 0 {
			result.WriteString(html[i:])
			break
		}
		j += i
		tag := html[i : j+1]

		if len(tag) >= htmlTagMinLen && tag[1] == '/' {
			stack = handleCloseTag(tag, stack, &result)
		} else {
			stack = handleOpenTag(tag, stack, &result)
		}
		i = j + 1
	}

	for k := len(stack) - 1; k >= 0; k-- {
		result.WriteString("</" + stack[k].tag + ">")
	}

	return result.String()
}

// handleCloseTag processes a closing HTML tag: finds the matching opener in the
// stack, emits closing tags for any intervening openers, emits the matched close
// tag, then reopens the intervening tags and returns the updated stack.
func handleCloseTag(tag string, stack []tagPos, result *strings.Builder) []tagPos {
	closeTag := tag[2 : len(tag)-1]

	matchIdx := -1
	for k := len(stack) - 1; k >= 0; k-- {
		if stack[k].tag == closeTag {
			matchIdx = k
			break
		}
	}

	if matchIdx < 0 {
		return stack // unmatched closer -- discard
	}

	for k := len(stack) - 1; k > matchIdx; k-- {
		result.WriteString("</" + stack[k].tag + ">")
	}
	result.WriteString(tag)

	reopened := make([]tagPos, 0, len(stack)-matchIdx-1)
	for k := matchIdx + 1; k < len(stack); k++ {
		result.WriteString(stack[k].openTag)
		reopened = append(reopened, tagPos{tag: stack[k].tag, openTag: stack[k].openTag, start: result.Len()})
	}
	return append(stack[:matchIdx], reopened...)
}

// handleOpenTag writes the opening tag to result and, for tracked Telegram tags,
// pushes an entry onto the stack. Returns the updated stack.
func handleOpenTag(tag string, stack []tagPos, result *strings.Builder) []tagPos {
	tagContent := tag[1 : len(tag)-1]
	tagContent = strings.TrimSuffix(tagContent, "/")
	parts := strings.Fields(tagContent)
	result.WriteString(tag)
	if len(parts) > 0 && trackedTags[parts[0]] {
		stack = append(stack, tagPos{tag: parts[0], openTag: tag, start: result.Len()})
	}
	return stack
}

// runeOffset returns the byte offset of the n-th rune in s.
// If s has fewer than n runes, returns len(s).
func runeOffset(s string, n int) int {
	off := 0
	for i := 0; i < n && off < len(s); i++ {
		_, size := utf8.DecodeRuneInString(s[off:])
		off += size
	}
	return off
}

// SplitMessage splits text into chunks respecting maxLen (in runes, not bytes),
// preferring newline boundaries. Second pass fixes HTML tags across chunk
// boundaries by closing open tags at end of each chunk and reopening them at
// start of next chunk.
func SplitMessage(text string, maxLen int) []string {
	if utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}

	// First pass: split on newline boundaries (raw, ignoring tags).
	var rawChunks []string
	for len(text) > 0 {
		if utf8.RuneCountInString(text) <= maxLen {
			rawChunks = append(rawChunks, text)
			break
		}
		byteOff := runeOffset(text, maxLen)
		chunk := text[:byteOff]
		splitAt := strings.LastIndex(chunk, "\n")
		if splitAt <= 0 {
			splitAt = byteOff
		}
		rawChunks = append(rawChunks, strings.TrimRight(text[:splitAt], "\n"))
		text = strings.TrimLeft(text[splitAt:], "\n")
	}

	// Second pass: fix HTML tags across chunk boundaries.
	var openTags []string // tags open at end of previous chunk
	result := make([]string, 0, len(rawChunks))

	for _, chunk := range rawChunks {
		// Prepend reopening tags from previous chunk.
		if len(openTags) > 0 {
			chunk = strings.Join(openTags, "") + chunk
		}

		// Track which tags are open at end of this chunk.
		openTags = unclosedTags(chunk)

		// Append closing tags in reverse order.
		if len(openTags) > 0 {
			var closers strings.Builder
			for i := len(openTags) - 1; i >= 0; i-- {
				tagName := parseTagName(openTags[i])
				closers.WriteString("</" + tagName + ">")
			}
			chunk += closers.String()
		}

		// Safety trim: tag repair may push chunk past maxLen.
		if utf8.RuneCountInString(chunk) > maxLen {
			chunk = trimChunkToLimit(chunk, maxLen)
		}

		result = append(result, chunk)
	}
	return result
}

// trimChunkToLimit trims an HTML chunk to fit within maxLen runes.
// Cuts content at a rune boundary and repairs HTML nesting (adds closers).
func trimChunkToLimit(chunk string, maxLen int) string {
	if utf8.RuneCountInString(chunk) <= maxLen {
		return chunk
	}
	reserve := 20
	if maxLen < reserve+10 {
		reserve = maxLen / 3
	}
	cutOff := runeOffset(chunk, maxLen-reserve)
	if cutOff > len(chunk) {
		cutOff = len(chunk)
	}
	return RepairHTMLNesting(chunk[:cutOff])
}

// parseTagName extracts the tag name from an opening tag string.
// E.g. `<a href="...">` -> `a`, `<b>` -> `b`.
func parseTagName(openTag string) string {
	inner := openTag[1 : len(openTag)-1]
	if sp := strings.IndexByte(inner, ' '); sp > 0 {
		return inner[:sp]
	}
	return inner
}

// popMatchingTag removes the rightmost opener whose tag name matches closeTag
// from stack, returning the updated slice.
func popMatchingTag(stack []string, closeTag string) []string {
	for k := len(stack) - 1; k >= 0; k-- {
		if parseTagName(stack[k]) == closeTag {
			return append(stack[:k], stack[k+1:]...)
		}
	}
	return stack
}

// unclosedTags returns opening tags (e.g. "<b>", "<code>") that remain
// unclosed at the end of the HTML fragment.
func unclosedTags(html string) []string {
	var stack []string
	i := 0
	for i < len(html) {
		lt := strings.IndexByte(html[i:], '<')
		if lt < 0 {
			break
		}
		lt += i
		gt := strings.IndexByte(html[lt:], '>')
		if gt < 0 {
			break
		}
		gt += lt
		tag := html[lt : gt+1]
		i = gt + 1

		if len(tag) < htmlTagMinLen {
			continue
		}

		if tag[1] == '/' {
			stack = popMatchingTag(stack, tag[2:len(tag)-1])
		} else {
			// Opening tag -- only track Telegram-supported formatting tags.
			tagContent := strings.TrimSuffix(tag[1:len(tag)-1], "/")
			parts := strings.Fields(tagContent)
			if len(parts) > 0 && trackedTags[parts[0]] {
				stack = append(stack, tag)
			}
		}
	}
	return stack
}
