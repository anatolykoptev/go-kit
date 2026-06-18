package redirectmatch

import "fmt"

// ValidateNoLoop rejects a candidate [RuleSpec] if it creates a self-redirect
// or a direct 2-cycle (A→B when an existing rule has B→A).
//
// It is intended to be called at write time by the redirect store before
// persisting a new rule.
//
// Note: only direct cycles (depth ≤ 2) are detected. Longer chains (A→B→C→A)
// are not checked here; they must be detected by a full graph walk at the store
// layer if required.
func ValidateNoLoop(candidate RuleSpec, existing []RuleSpec) error {
	// Self-redirect: target equals source.
	if candidate.Target != "" && candidate.Target == candidate.SourcePath {
		return fmt.Errorf("redirectmatch: self-redirect: source %q and target %q are identical", candidate.SourcePath, candidate.Target)
	}

	// Direct 2-cycle: candidate is A→B; check if any existing rule is B→A.
	if candidate.Target == "" {
		// No target (410/451), cannot form a cycle.
		return nil
	}

	for _, ex := range existing {
		if ex.SourcePath == candidate.Target && ex.Target == candidate.SourcePath {
			return fmt.Errorf(
				"redirectmatch: direct redirect cycle detected: %q → %q while existing rule %d has %q → %q",
				candidate.SourcePath, candidate.Target,
				ex.ID, ex.SourcePath, ex.Target,
			)
		}
	}

	return nil
}
