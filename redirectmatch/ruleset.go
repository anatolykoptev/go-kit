package redirectmatch

import "sort"

// BuildRuleSet compiles specs into an immutable [RuleSet].
//
// Invalid specs are collected as [CompileError] values and EXCLUDED from the
// set — one bad rule must not poison the whole set. The ordered tier is sorted
// by (Priority ASC, ID ASC) so lower numbers are tried first.
func BuildRuleSet(specs []RuleSpec, p Policy) (*RuleSet, []CompileError) {
	set := &RuleSet{
		policy: p,
		exact:  make(map[string]Rule),
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
			// QExact rules are keyed by their full SourcePath (which already
			// contains the embedded query, e.g. "/?p=123"). Non-QExact exact
			// rules are keyed by path only.
			set.exact[rule.SourcePath] = rule
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
