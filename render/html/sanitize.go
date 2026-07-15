package html

// sanitizeMarkdown was removed in v0.92.1 (render extraction from vaelor).
//
// The function applied a hand-rolled regex over raw markdown source to block
// javascript:/vbscript:/file:/data: URL schemes. It had two problems:
//
//  1. Weaker than goldmark: percent-encoded schemes (%6aavascript:) and
//     whitespace-split variants bypassed the regex.
//
//  2. It regressed data:image/* — base64-encoded inline images are legitimate
//     markdown content and goldmark's IsDangerousURL correctly permits them,
//     but the regex blanket-replaced ALL data: prefixes with "#".
//
// URL filtering is now handled entirely by goldmark's default renderer
// (Unsafe=false, which is the zero-value and is never changed by buildGoldmark).
// Goldmark's IsDangerousURL blocks javascript:/vbscript:/file: schemes while
// explicitly allowing data:image/ — see goldmark/renderer/html/html.go.
//
// This file is retained as a tombstone so the removal intent is searchable.
