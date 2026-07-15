package directives

import "strings"

// ParseAttrs parses a directive attribute string of the form
// `{key=value key2="quoted value" flag}` into a map. The surrounding braces
// are optional — the function accepts either the substring between braces
// or the full `{...}` form. An empty input returns an empty map.
//
// Supported syntax:
//   - key=value       (unquoted single-word value)
//   - key="quoted"    (quoted value supports embedded spaces)
//   - flag            (bare identifier sets key with empty-string value)
//
// Malformed input (e.g. unclosed quote) produces a best-effort partial
// parse of the tokens that did parse cleanly; remaining bytes are skipped.
func ParseAttrs(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	// Strip surrounding braces if present.
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = s[1 : len(s)-1]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return out
	}

	i := 0
	n := len(s)
	for i < n {
		// Skip whitespace.
		for i < n && isAttrSpace(s[i]) {
			i++
		}
		if i >= n {
			break
		}
		// Read key.
		keyStart := i
		for i < n && !isAttrSpace(s[i]) && s[i] != '=' {
			i++
		}
		key := s[keyStart:i]
		if key == "" {
			// Skip unrecognized char.
			i++
			continue
		}
		// No '=' → bare flag.
		if i >= n || s[i] != '=' {
			out[key] = ""
			continue
		}
		// Consume '='.
		i++
		if i >= n {
			out[key] = ""
			break
		}
		// Read value — quoted or unquoted.
		if s[i] == '"' {
			i++ // skip opening quote
			valStart := i
			for i < n && s[i] != '"' {
				i++
			}
			val := s[valStart:i]
			out[key] = val
			if i < n {
				i++ // skip closing quote
			}
			// Malformed unterminated quote: val captured best-effort.
			continue
		}
		// Unquoted value — runs until whitespace.
		valStart := i
		for i < n && !isAttrSpace(s[i]) {
			i++
		}
		out[key] = s[valStart:i]
	}
	return out
}

func isAttrSpace(c byte) bool {
	return c == ' ' || c == '\t'
}
