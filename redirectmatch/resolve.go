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
		// "/evil.com", "/old/%5Cevil.com" captures "\\evil.com". Expanded into
		// "/$1" that becomes "//evil.com" or "/\\evil.com", which a browser
		// resolves off-site — an open redirect from our own domain. Compile
		// (targetCaptureError) already keeps captures out of a target's host, so
		// a target with a fixed host only needs to be free of control bytes
		// (CR/LF would split the Location header in a writer that passes them);
		// any other target must also expand to a same-origin path, or the rule
		// does not apply.
		if _, fixedHost := targetAuthority(rule.Target); hasControlByte(location) || !fixedHost && !isSameOriginPath(location) {
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

// isSameOriginPath reports whether loc, the expansion of a target that
// Compile guarantees starts with "/", stays on the requesting origin: not "//"
// or "/\\", both protocol-relative to a browser.
func isSameOriginPath(loc string) bool {
	return !(len(loc) > 1 && (loc[1] == '/' || loc[1] == '\\'))
}

// hasControlByte reports whether loc holds a byte below 0x20. Browsers strip
// tab and newline anywhere in a URL ("/\t\\x" becomes "/\\x"), and CR/LF
// split headers. A space is an ordinary path byte.
func hasControlByte(loc string) bool {
	for i := 0; i < len(loc); i++ {
		if loc[i] < 0x20 {
			return true
		}
	}
	return false
}
