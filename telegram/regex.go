package telegram

import "regexp"

// Precompiled regexes for MarkdownToHTML conversion.
var (
	reHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reBlockquote  = regexp.MustCompile(`(?m)(^&gt;[ \t]?.*$\n?)+`)
	reLink        = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBoldStar    = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	reBold        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnder   = regexp.MustCompile(`__(.+?)__`)
	reItalicStar  = regexp.MustCompile(`\*([^*\n]+)\*`)
	reItalicUnder = regexp.MustCompile(`_([^_\n]+)_`)
	reStrike      = regexp.MustCompile(`~~(.+?)~~`)
	reListItem    = regexp.MustCompile(`(?m)^[-*]\s+`)
	reHRule       = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
)

// Precompiled regexes for StripMarkdown (plain-text fallback).
var (
	reStripCodeFence  = regexp.MustCompile("(?m)^```\\w*\n?")
	reStripBoldItalic = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	reStripBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reStripBoldU      = regexp.MustCompile(`__(.+?)__`)
	reStripItalicS    = regexp.MustCompile(`\*(.+?)\*`)
	reStripItalicU    = regexp.MustCompile(`_(.+?)_`)
	reStripStrike     = regexp.MustCompile("~~(.+?)~~")
	reStripInline     = regexp.MustCompile("`(.+?)`")
	reStripHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reStripListItem   = regexp.MustCompile(`(?m)^[-*+]\s`)
	reStripBlockquote = regexp.MustCompile(`(?m)^>\s?`)
	reStripLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// Precompiled regexes for CloseUnclosedMarkdown.
var (
	reBalancedFences = regexp.MustCompile("(?s)```\\w*\\n?.*?```")
	reBalancedInline = regexp.MustCompile("`[^`]+`")
)

// Precompiled regexes for code extraction helpers.
var (
	reCodeBlock  = regexp.MustCompile("```([\\w]*)\\n?([\\s\\S]*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
)
