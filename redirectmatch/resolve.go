package redirectmatch

import (
	"strings"
)

// Resolve is the hot-path contract. It is single-hop and first-match-wins.
//
// Resolution steps:
//  1. Normalize rawPath with the set's policy → np.
//  2. Exact tier (O(1)):
//     a. If rawQuery != "" → probe "np?rawQuery" for a QExact rule.
//     b. Probe "np" for a non-QExact rule.
//  3. Ordered tier: iterate prefix and regex rules (Priority ASC, ID ASC);
//     first match wins.
//  4. Miss → Decision{Matched: false}.
//  5. Location construction (3xx only):
//     - Regex: expand $1, $2… from submatches into Target.
//     - QPass: append rawQuery to Location.
//     - 410/451: Location is always "".
//
// Resolve is safe for concurrent use; it never mutates set.
func Resolve(set *RuleSet, rawPath, rawQuery string) Decision {
	np := Normalize(rawPath, set.policy)

	// --- Exact tier ---
	if rawQuery != "" {
		key := np + "?" + rawQuery
		if rule, ok := set.exact[key]; ok && rule.QueryHandling == QExact {
			return buildDecision(rule, np, rawQuery, nil)
		}
	}

	if rule, ok := set.exact[np]; ok && rule.QueryHandling != QExact {
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
	}

	// Apply query propagation.
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
