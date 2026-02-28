// Package strutil provides Unicode-aware string helpers.
// Zero external dependencies.
package strutil

import "strings"

// Truncate caps s at maxRunes runes, appending "..." if truncated.
// Safe for UTF-8 (Cyrillic, CJK, emoji).
func Truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// TruncateAtWord truncates s to maxRunes at a word boundary.
// If the last space is too far back (< half of maxRunes), truncates at maxRunes.
// Appends "..." if truncated.
func TruncateAtWord(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	truncated := string(runes[:maxRunes])
	cut := strings.LastIndex(truncated, " ")
	if cut < len(truncated)/2 {
		return truncated + "..."
	}
	return truncated[:cut] + "..."
}

// Contains reports whether items contains s.
func Contains(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}

// ContainsAny reports whether s contains any of the given substrings.
func ContainsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
