package redirectmatch

import (
	"net/url"
	"strings"
)

// Normalize canonicalizes rawPath according to p before it is used for
// matching. It is pure and deterministic: Normalize(Normalize(x, p), p) ==
// Normalize(x, p) for all x.
//
// Transformation order:
//  1. Decode percent-encoding once (when [Policy.DecodeOnce]).
//  2. Lower-case (when [Policy.LowerCase]).
//  3. Strip exactly one trailing "/" (when [Policy.StripTrailingSlash]),
//     except the root "/" which is always kept as "/".
//
// An empty rawPath is treated as "/".
func Normalize(rawPath string, p Policy) string {
	if rawPath == "" {
		return "/"
	}

	s := rawPath

	if p.DecodeOnce {
		if decoded, err := url.PathUnescape(s); err == nil {
			s = decoded
		}
		// On error we keep the original string.
	}

	if p.LowerCase {
		s = strings.ToLower(s)
	}

	if p.StripTrailingSlash && len(s) > 1 && strings.HasSuffix(s, "/") {
		s = s[:len(s)-1]
	}

	if s == "" {
		return "/"
	}

	return s
}
