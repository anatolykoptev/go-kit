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

// captureRefRE matches a capture-expansion reference in Regexp.ExpandString syntax
// ($1, ${name}, $name). A bare literal "$" (e.g. a target like "/cost$") is NOT a
// capture reference, so such targets remain subject to the static self-loop check.
var captureRefRE = regexp.MustCompile(`\$(\d|\{|[A-Za-z_])`)

// hasCaptureRefs reports whether target contains at least one capture-expansion reference.
func hasCaptureRefs(target string) bool {
	return captureRefRE.MatchString(target)
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
//   - a Regex target whose capture references could choose the redirect's host
//     (see targetCaptureError)
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

	// A QExact rule must embed a query in its SourcePath ("path?query"); without a
	// "?" it can never match, since Resolve only probes the QExact map when a query
	// is present (key = path + "?" + query).
	if spec.QueryHandling == QExact && !strings.Contains(spec.SourcePath, "?") {
		return Rule{}, fmt.Errorf("redirectmatch: QueryHandling %q requires an embedded query in SourcePath (e.g. %q), got %q", QExact, "/path?key=val", spec.SourcePath)
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

		if is3xx(spec.StatusCode) && hasCaptureRefs(spec.Target) {
			if err := targetCaptureError(spec.Target); err != nil {
				return Rule{}, err
			}
		}
	}

	return r, nil
}

// targetCaptureError rejects a capture-bearing target whose captures could
// choose where the redirect goes. A capture carries request input, and the
// matched path is percent-decoded, so it can hold "@", ".", "/" or "\\":
//   - in the authority of an absolute or protocol-relative target
//     ("https://blog.example.org$1", "//$1"), "/blog@evil.example" would
//     become "https://blog.example.org@evil.example" — host evil.example;
//   - in a target that does not start with "/" ("$1", "news/$1"), the capture
//     can supply a scheme and host outright.
//
// A target that starts with "/" is allowed: Resolve checks that its expansion
// stays a same-origin path.
func targetCaptureError(target string) error {
	if authority, ok := targetAuthority(target); ok {
		if hasCaptureRefs(authority) {
			return fmt.Errorf("redirectmatch: Target %q has a capture reference in its host; a request could choose the redirect's destination", target)
		}
		return nil
	}
	if !strings.HasPrefix(target, "/") {
		return fmt.Errorf("redirectmatch: Target %q with capture references must start with '/' or be an absolute URL with a fixed host", target)
	}
	return nil
}

// targetAuthority returns the authority (userinfo, host, port) of an absolute
// ("http://", "https://") or protocol-relative ("//") target, and false for
// any other target. The authority ends at the first "/", "?", "#" or "\\"
// (browsers read "\\" as "/" in http URLs).
func targetAuthority(target string) (string, bool) {
	var rest string
	switch t := strings.ToLower(target); {
	case strings.HasPrefix(t, "http://"):
		rest = target[len("http://"):]
	case strings.HasPrefix(t, "https://"):
		rest = target[len("https://"):]
	case strings.HasPrefix(target, "//"):
		rest = target[len("//"):]
	default:
		return "", false
	}
	if i := strings.IndexAny(rest, "/?#\\"); i >= 0 {
		rest = rest[:i]
	}
	return rest, true
}
