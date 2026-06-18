package redirectmatch

import (
	"fmt"
	"regexp"
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

// Compile turns a [RuleSpec] into an immutable [Rule].
//
// It returns an error for:
//   - an invalid or unsupported StatusCode
//   - status/target incoherence (3xx must have non-empty Target; 410/451 must have empty Target)
//   - self-redirect (Target == SourcePath)
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

	if spec.Target != "" && spec.Target == spec.SourcePath {
		return Rule{}, fmt.Errorf("redirectmatch: self-redirect detected: Target %q equals SourcePath", spec.Target)
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
	}

	return r, nil
}
