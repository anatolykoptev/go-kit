package redirectmatch

import (
	"strings"
)

// Resolve is the hot-path contract. It is single-hop and first-match-wins.
//
// Resolution steps:
//  1. Normalize rawPath with the set's policy → np.
//  2. Exact tier (O(1)):
//     a. If rawQuery != "" → probe exactQ[np+"?"+rawQuery] for a QExact rule.
//     b. Probe exact[np] for a non-QExact rule.
//  3. Ordered tier: iterate prefix and regex rules (Priority ASC, ID ASC);
//     first match wins.
//  4. Miss → Decision{Matched: false}.
//  5. Location construction (3xx only):
//     - Regex: expand $1, $2… from submatches into Target.
//     - QPass: append rawQuery to Location (see doc.go for exact semantics).
//     - 410/451: Location is always "".
//
// Resolve is safe for concurrent use; it never mutates set.
func Resolve(set *RuleSet, rawPath, rawQuery string) Decision {
	np := Normalize(rawPath, set.policy)

	// --- Exact tier ---
	// Step a: QExact lookup — only when rawQuery is non-empty.
	// exactQ is keyed by Normalize(pathPart,policy)+"?"+rawQueryPart (verbatim).
	if rawQuery != "" {
		key := np + "?" + rawQuery
		if rule, ok := set.exactQ[key]; ok {
			return buildDecision(rule, np, rawQuery, nil)
		}
	}

	// Step b: non-QExact exact lookup.
	if rule, ok := set.exact[np]; ok {
		return buildDecision(rule, np, rawQuery, nil)
	}

	// --- Ordered tier ---
	for _, rule := range set.ordered {
		switch rule.MatchType {
		case Prefix:
			if strings.HasPrefix(np, rule.SourcePath) {
				return buildDecision(rule, np, rawQuery, nil)
			}
		case Regex:
			if rule.re != nil {
				loc := rule.re.FindStringSubmatchIndex(np)
				if loc != nil {
					return buildDecision(rule, np, rawQuery, loc)
				}
			}
		}
	}

	return Decision{Matched: false}
}

// buildDecision constructs the Decision for a matched rule.
// submatch is the result of FindStringSubmatchIndex (nil for non-regex rules).
func buildDecision(rule Rule, np, rawQuery string, submatch []int) Decision {
	if isGone(rule.StatusCode) {
		return Decision{Matched: true, StatusCode: rule.StatusCode, Location: ""}
	}

	location := rule.Target

	// Expand regex captures ($1, $2, …) into location.
	if rule.MatchType == Regex && submatch != nil {
		dst := rule.re.ExpandString(nil, rule.Target, np, submatch)
		location = string(dst)
		// A capture carries whatever the request path held, and the matched path
		// is percent-DECODED (Policy.DecodeOnce): "/old/%2Fevil.com" captures
		// "/evil.com", "/old/%5Cevil.com" captures "\evil.com". Expanded into a
		// relative target such as "/$1" that becomes "//evil.com" or "/\evil.com",
		// which a browser resolves off-site — an open redirect from our own
		// domain. A relative template must therefore expand to a same-origin
		// path; if it does not, the rule does not apply. (An absolute template
		// is an operator-chosen destination and is left alone.)
		if !isAbsoluteURL(rule.Target) && !isSameOriginPath(location) {
			return Decision{Matched: false}
		}
	}

	// Apply query propagation.
	// Contract (see doc.go): if rawQuery == "" append nothing;
	// else append "?" + rawQuery when target has no "?", or "&" + rawQuery when it does.
	// rawQuery is assumed URL-clean (no fragment).
	if rule.QueryHandling == QPass && rawQuery != "" {
		if strings.Contains(location, "?") {
			location = location + "&" + rawQuery
		} else {
			location = location + "?" + rawQuery
		}
	}

	return Decision{
		Matched:    true,
		StatusCode: rule.StatusCode,
		Location:   location,
	}
}

// isAbsoluteURL reports whether target names its own scheme and host.
func isAbsoluteURL(target string) bool {
	t := strings.ToLower(target)
	return strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")
}

// isSameOriginPath reports whether loc, used as a Location, stays on the
// requesting origin: one leading "/", not "//" or "/\" (both protocol-relative
// to a browser), and no control characters or spaces, which browsers strip or
// that split headers.
func isSameOriginPath(loc string) bool {
	if loc == "" || loc[0] != '/' {
		return false
	}
	if len(loc) > 1 && (loc[1] == '/' || loc[1] == '\\') {
		return false
	}
	for i := 0; i < len(loc); i++ {
		if c := loc[i]; c <= 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}
