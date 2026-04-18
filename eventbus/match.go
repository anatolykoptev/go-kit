package eventbus

import "strings"

// matchTopic returns true if pattern matches topic using segment-aware glob rules.
//
// Rules:
//   - Exact match: "a.b" matches "a.b" only.
//   - "*" in a segment position matches exactly one segment (no dots).
//   - "**" as the final segment matches zero or more trailing segments.
//
// Examples:
//
//	"alerts.*"   matches "alerts.twitter" but not "alerts.a.b"
//	"a.**"       matches "a.b", "a.b.c.d"
//	"**"         matches any topic (zero or more segments)
func matchTopic(pattern, topic string) bool {
	// Fast path: exact match.
	if pattern == topic {
		return true
	}

	// Split into segments and match.
	pParts := strings.Split(pattern, ".")
	tParts := strings.Split(topic, ".")

	return matchSegments(pParts, tParts)
}

// matchSegments recursively matches pattern segments against topic segments.
func matchSegments(pParts, tParts []string) bool {
	if len(pParts) == 0 {
		return len(tParts) == 0
	}

	head := pParts[0]
	rest := pParts[1:]

	if head == "**" {
		// "**" must be the last segment (tail wildcard).
		// Matches zero or more remaining topic segments.
		if len(rest) == 0 {
			return true
		}
		// "**" in non-tail position: try consuming 0..n topic segments.
		for i := 0; i <= len(tParts); i++ {
			if matchSegments(rest, tParts[i:]) {
				return true
			}
		}
		return false
	}

	if len(tParts) == 0 {
		return false
	}

	if head == "*" || head == tParts[0] {
		return matchSegments(rest, tParts[1:])
	}

	return false
}
