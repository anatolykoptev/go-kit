package redirectmatch

import (
	"sort"
	"strings"
)

// BuildRuleSet compiles specs into an immutable [RuleSet].
//
// Invalid specs are collected as [CompileError] values and EXCLUDED from the
// set — one bad rule must not poison the whole set. The ordered tier is sorted
// by (Priority ASC, ID ASC) so lower numbers are tried first.
//
// Exact rules are placed into one of two maps to avoid key-space collisions:
//   - non-QExact Exact rules → set.exact, keyed by Normalize(SourcePath, policy)
//   - QExact Exact rules     → set.exactQ, keyed by Normalize(pathPart, policy) + "?" + rawQueryPart,
//     where pathPart and rawQueryPart come from splitting SourcePath on the first "?".
func BuildRuleSet(specs []RuleSpec, p Policy) (*RuleSet, []CompileError) {
	set := &RuleSet{
		policy: p,
		exact:  make(map[string]Rule),
		exactQ: make(map[string]Rule),
	}

	var compileErrs []CompileError

	for i, spec := range specs {
		rule, err := Compile(spec)
		if err != nil {
			compileErrs = append(compileErrs, CompileError{
				Index:  i,
				ID:     spec.ID,
				Source: spec.SourcePath,
				Err:    err.Error(),
			})
			continue
		}

		switch rule.MatchType {
		case Exact:
			if rule.QueryHandling == QExact {
				// Split SourcePath on the first "?" to produce the exactQ key:
				// key = Normalize(pathPart, policy) + "?" + rawQueryPart.
				// rawQueryPart is kept verbatim — QExact contract is raw byte-equality.
				pathPart, queryPart, _ := strings.Cut(rule.SourcePath, "?")
				key := Normalize(pathPart, p) + "?" + queryPart
				set.exactQ[key] = rule
			} else {
				// Non-QExact Exact rules are keyed by the normalized SourcePath.
				set.exact[Normalize(rule.SourcePath, p)] = rule
			}
		default:
			// Prefix and Regex go into the ordered tier.
			set.ordered = append(set.ordered, rule)
		}
	}

	// Sort ordered tier: Priority ASC, then ID ASC as tiebreaker.
	sort.Slice(set.ordered, func(i, j int) bool {
		a, b := set.ordered[i], set.ordered[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})

	return set, compileErrs
}
