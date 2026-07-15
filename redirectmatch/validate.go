package redirectmatch

import "fmt"

// ValidateNoLoop rejects a candidate [RuleSpec] if it creates a self-redirect
// or a direct 2-cycle (A→B when an existing rule has B→A).
//
// Comparisons use [Normalize] under [DefaultPolicy] so that case/encoding-equal
// paths (e.g. /a → /B with /b → /a) are detected as cycles.
//
// It is intended to be called at write time by the redirect store before
// persisting a new rule.
//
// Note: only direct cycles (depth ≤ 2) are detected. Longer chains (A→B→C→A)
// are not checked here; they must be detected by a full graph walk at the store
// layer if required.
func ValidateNoLoop(candidate RuleSpec, existing []RuleSpec) error {
	dp := DefaultPolicy()
	normCandidateSource := Normalize(candidate.SourcePath, dp)
	normCandidateTarget := Normalize(candidate.Target, dp)

	// Self-redirect: normalized target equals normalized source.
	if candidate.Target != "" && normCandidateTarget == normCandidateSource {
		return fmt.Errorf("redirectmatch: self-redirect: source %q and target %q are identical after normalization", candidate.SourcePath, candidate.Target)
	}

	// Direct 2-cycle: candidate is A→B; check if any existing rule is B→A
	// (using normalized comparison so /B and /b are treated as the same path).
	if candidate.Target == "" {
		// No target (410/451), cannot form a cycle.
		return nil
	}

	for _, ex := range existing {
		normExSource := Normalize(ex.SourcePath, dp)
		normExTarget := Normalize(ex.Target, dp)
		if normExSource == normCandidateTarget && normExTarget == normCandidateSource {
			return fmt.Errorf(
				"redirectmatch: direct redirect cycle detected: %q → %q while existing rule %d has %q → %q",
				candidate.SourcePath, candidate.Target,
				ex.ID, ex.SourcePath, ex.Target,
			)
		}
	}

	return nil
}
