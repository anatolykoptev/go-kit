package redirectmatch

import (
	"fmt"
	"regexp"
	"strings"
)

// validStatusCodes is the set of status codes this package supports.
var validStatusCodes = map[int]bool{
	301: true,
	302: true,
	307: true,
	308: true,
	410: true,
	451: true,
}

// is3xx returns true for redirect status codes.
func is3xx(code int) bool {
	return code == 301 || code == 302 || code == 307 || code == 308
}

// isGone returns true for non-redirect suppression codes (410/451).
func isGone(code int) bool {
	return code == 410 || code == 451
}

// hasCaptureRefs reports whether target contains at least one $n capture reference.
func hasCaptureRefs(target string) bool {
	return strings.Contains(target, "$")
}

// Compile turns a [RuleSpec] into an immutable [Rule].
//
// It returns an error for:
//   - an invalid or unsupported StatusCode
//   - status/target incoherence (3xx must have non-empty Target; 410/451 must have empty Target)
//   - empty SourcePath (always rejected)
//   - Exact or Prefix with a relative SourcePath (no leading "/")
//   - Regex with an empty pattern
//   - [QExact] on a non-[Exact] MatchType (Regex and Prefix cannot honor embedded-query matching)
//   - self-redirect: Normalize(Target) == Normalize(SourcePath) under [DefaultPolicy]
//   - Regex identity self-redirect (static-target case, no $n refs): the compiled RE matches its own Target
//   - an un-RE2-compilable regex pattern (surfaces the error; never silently drops)
func Compile(spec RuleSpec) (Rule, error) {
	if !validStatusCodes[spec.StatusCode] {
		return Rule{}, fmt.Errorf("redirectmatch: invalid status code %d: must be one of 301, 302, 307, 308, 410, 451", spec.StatusCode)
	}

	if is3xx(spec.StatusCode) && spec.Target == "" {
		return Rule{}, fmt.Errorf("redirectmatch: status %d requires a non-empty Target", spec.StatusCode)
	}

	if isGone(spec.StatusCode) && spec.Target != "" {
		return Rule{}, fmt.Errorf("redirectmatch: status %d must have an empty Target, got %q", spec.StatusCode, spec.Target)
	}

	// Reject QExact on non-Exact match types: regex and prefix cannot honor
	// embedded-query matching semantics.
	if spec.QueryHandling == QExact && spec.MatchType != Exact {
		return Rule{}, fmt.Errorf("redirectmatch: QueryHandling %q is only supported for MatchType %q, got %q", QExact, Exact, spec.MatchType)
	}

	// Reject empty SourcePath for all match types.
	if spec.SourcePath == "" {
		return Rule{}, fmt.Errorf("redirectmatch: SourcePath must not be empty")
	}

	// For Exact and Prefix, require an absolute path (leading "/").
	// An empty prefix matches every path (strings.HasPrefix(x,"") == true).
	if spec.MatchType == Exact || spec.MatchType == Prefix {
		if spec.SourcePath[0] != '/' {
			return Rule{}, fmt.Errorf("redirectmatch: SourcePath %q must start with '/' for match type %q", spec.SourcePath, spec.MatchType)
		}
	}

	// Self-redirect detection: compare normalized forms so case/encoding-equal
	// loops are caught (e.g. /a → /A under a lowercasing policy).
	dp := DefaultPolicy()
	if spec.Target != "" && Normalize(spec.Target, dp) == Normalize(spec.SourcePath, dp) {
		return Rule{}, fmt.Errorf("redirectmatch: self-redirect detected: Target %q equals SourcePath %q after normalization", spec.Target, spec.SourcePath)
	}

	r := Rule{
		ID:            spec.ID,
		SourcePath:    spec.SourcePath,
		MatchType:     spec.MatchType,
		Target:        spec.Target,
		StatusCode:    spec.StatusCode,
		QueryHandling: spec.QueryHandling,
		Priority:      spec.Priority,
	}

	if spec.MatchType == Regex {
		re, err := regexp.Compile(spec.SourcePath)
		if err != nil {
			return Rule{}, fmt.Errorf("redirectmatch: invalid regex pattern %q: %w", spec.SourcePath, err)
		}
		r.re = re

		// Regex identity self-redirect (static-target case):
		// If the target contains no $n capture references, we can statically
		// check whether the pattern matches its own target.  If it does, the
		// rule would 301-loop every matched request.
		//
		// For targets WITH $n refs this cannot be proven statically; those are
		// left to the store-layer loop guard (see doc.go for the residual gap).
		if spec.Target != "" && !hasCaptureRefs(spec.Target) {
			if re.MatchString(Normalize(spec.Target, dp)) {
				return Rule{}, fmt.Errorf("redirectmatch: regex identity self-redirect: pattern %q matches its own Target %q", spec.SourcePath, spec.Target)
			}
		}
	}

	return r, nil
}
