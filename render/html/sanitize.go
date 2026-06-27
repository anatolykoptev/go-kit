package html

import "regexp"

// unsafeURLRe matches markdown image/link URLs using dangerous schemes.
// Goldmark parses the raw markdown, so rewriting the source before parsing is
// the simplest way to strip these references without traversing the AST.
var unsafeURLRe = regexp.MustCompile(`(?i)(file:|javascript:|data:|vbscript:)[^\s\)]*`)

// sanitizeMarkdown replaces dangerous URL schemes (file:, javascript:, data:,
// vbscript:) with "#" so they cannot leak into the rendered HTML.
//
// Percent-encoded schemes (e.g. `%66ile:`) are NOT covered. TODO: replace
// regex with AST-level URL filter using goldmark's renderer hooks before
// exposing this to untrusted inputs.
func sanitizeMarkdown(markdown string) string {
	return unsafeURLRe.ReplaceAllString(markdown, "#")
}
