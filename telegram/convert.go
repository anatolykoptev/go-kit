// Package telegram provides SDK-agnostic Telegram formatting utilities.
// Stdlib-only, zero external dependencies.
package telegram

import (
	"fmt"
	"strings"
)

// codeBlockMatchGroups is the expected number of regex match groups
// for code block extraction.
const codeBlockMatchGroups = 3

// MarkdownToHTML converts markdown to Telegram-compatible HTML.
//
// Handles: headings (bold), bold, italic, bold-italic, strikethrough,
// links, code blocks with language, inline code, blockquotes,
// horizontal rules, and lists. Code blocks and inline codes are
// extracted first via placeholders to protect from transformation.
// Calls RepairHTMLNesting at the end.
func MarkdownToHTML(text string) string {
	if text == "" {
		return ""
	}

	// 1. Extract code blocks and inline codes (protect from all transformations).
	codeBlocks := extractCodeBlocks(text)
	text = codeBlocks.text

	inlineCodes := extractInlineCodes(text)
	text = inlineCodes.text

	// 2. Escape HTML entities (must happen before adding HTML tags).
	text = EscapeHTML(text)

	// 3. Headings -> bold.
	text = reHeading.ReplaceAllString(text, "<b>$1</b>")

	// 4. Blockquotes -> <blockquote>.
	text = convertBlockquotes(text)

	// 5. Horizontal rules -> thin line.
	text = reHRule.ReplaceAllString(text, "\u2014\u2014\u2014")

	// 6. Links.
	text = reLink.ReplaceAllString(text, `<a href="$2">$1</a>`)

	// 7. Bold + italic combos (order matters: *** before ** before *).
	text = reBoldStar.ReplaceAllString(text, "<b><i>$1</i></b>")
	text = reBold.ReplaceAllString(text, "<b>$1</b>")
	text = reBoldUnder.ReplaceAllString(text, "<b>$1</b>")

	// 8. Strikethrough.
	text = reStrike.ReplaceAllString(text, "<s>$1</s>")

	// 9. List items - MUST be before single-* italic to avoid * item -> <i>.
	text = reListItem.ReplaceAllString(text, "\u2022 ")

	// 10. Italic (*text* and _text_) - after lists consumed the leading *.
	text = reItalicStar.ReplaceAllString(text, "<i>$1</i>")
	text = reItalicUnder.ReplaceAllString(text, "<i>$1</i>")

	// 11. Restore inline codes.
	for i, code := range inlineCodes.codes {
		escaped := EscapeHTML(code)
		placeholder := fmt.Sprintf("\x00IC%d\x00", i)
		text = strings.ReplaceAll(text, placeholder, "<code>"+escaped+"</code>")
	}

	// 12. Restore code blocks with language tags.
	for i, code := range codeBlocks.codes {
		escaped := EscapeHTML(code)
		lang := codeBlocks.langs[i]
		placeholder := fmt.Sprintf("\x00CB%d\x00", i)
		if lang != "" {
			text = strings.ReplaceAll(text, placeholder,
				"<pre><code class=\"language-"+lang+"\">"+escaped+"</code></pre>")
		} else {
			text = strings.ReplaceAll(text, placeholder,
				"<pre><code>"+escaped+"</code></pre>")
		}
	}

	// 13. Repair any mismatched HTML nesting.
	text = RepairHTMLNesting(text)

	return text
}

// StripMarkdown removes all markdown syntax for plain-text fallback.
// Produces clean readable text without formatting markers.
func StripMarkdown(text string) string {
	text = reStripCodeFence.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "```", "")
	text = reStripBoldItalic.ReplaceAllString(text, "$1")
	text = reStripBold.ReplaceAllString(text, "$1")
	text = reStripBoldU.ReplaceAllString(text, "$1")
	text = reStripItalicS.ReplaceAllString(text, "$1")
	text = reStripItalicU.ReplaceAllString(text, "$1")
	text = reStripStrike.ReplaceAllString(text, "$1")
	text = reStripInline.ReplaceAllString(text, "$1")
	text = reStripHeading.ReplaceAllString(text, "")
	text = reStripListItem.ReplaceAllString(text, "- ")
	text = reStripBlockquote.ReplaceAllString(text, "")
	text = reStripLink.ReplaceAllString(text, "$1 ($2)")
	return text
}

// CloseUnclosedMarkdown closes any unclosed markdown constructs at the
// end of partial streaming text, so it can be safely converted to HTML
// mid-stream.
func CloseUnclosedMarkdown(text string) string {
	// 1. Code fences - if odd count, we're inside a code block.
	if strings.Count(text, "```")%2 != 0 {
		return text + "\n```"
	}

	// Remove balanced code fences for inline analysis.
	stripped := reBalancedFences.ReplaceAllString(text, "")

	var suffix string

	// 2. Inline backticks.
	if strings.Count(stripped, "`")%2 != 0 {
		suffix += "`"
		stripped += "`"
	}
	stripped = reBalancedInline.ReplaceAllString(stripped, "")

	// 3. Bold (**).
	if strings.Count(stripped, "**")%2 != 0 {
		suffix += "**"
	}
	stripped = strings.ReplaceAll(stripped, "**", "")

	// 4. Italic (*) - after ** removed.
	if strings.Count(stripped, "*")%2 != 0 {
		suffix += "*"
	}

	// 5. Strikethrough (~~).
	if strings.Count(stripped, "~~")%2 != 0 {
		suffix += "~~"
	}

	if suffix == "" {
		return text
	}

	return text + suffix
}

// convertBlockquotes converts consecutive > lines to <blockquote> tags.
// Operates on HTML-escaped text where > is &gt;.
func convertBlockquotes(text string) string {
	return reBlockquote.ReplaceAllStringFunc(text, func(block string) string {
		lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
		var cleaned []string
		for _, line := range lines {
			line = strings.TrimPrefix(line, "&gt; ")
			line = strings.TrimPrefix(line, "&gt;")
			cleaned = append(cleaned, line)
		}
		return "<blockquote>" + strings.Join(cleaned, "\n") + "</blockquote>\n"
	})
}

type codeBlockMatch struct {
	text  string
	codes []string
	langs []string
}

func extractCodeBlocks(text string) codeBlockMatch {
	var codes []string
	var langs []string
	idx := 0
	text = reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		match := reCodeBlock.FindStringSubmatch(m)
		lang := ""
		code := m
		if len(match) >= codeBlockMatchGroups {
			lang = match[1]
			code = match[2]
		}
		langs = append(langs, lang)
		codes = append(codes, code)
		placeholder := fmt.Sprintf("\x00CB%d\x00", idx)
		idx++
		return placeholder
	})

	return codeBlockMatch{text: text, codes: codes, langs: langs}
}

type inlineCodeMatch struct {
	text  string
	codes []string
}

func extractInlineCodes(text string) inlineCodeMatch {
	var codes []string
	idx := 0
	text = reInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		match := reInlineCode.FindStringSubmatch(m)
		code := m
		if len(match) >= 2 { //nolint:mnd // regex submatch: full match + 1 capture
			code = match[1]
		}
		codes = append(codes, code)
		placeholder := fmt.Sprintf("\x00IC%d\x00", idx)
		idx++
		return placeholder
	})

	return inlineCodeMatch{text: text, codes: codes}
}
