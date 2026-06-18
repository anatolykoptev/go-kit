package redirectmatch

import "regexp"

// MatchType controls how SourcePath is interpreted during resolution.
type MatchType string

const (
	// Exact requires the normalized incoming path to equal SourcePath exactly.
	Exact MatchType = "exact"

	// Prefix matches when the normalized incoming path has SourcePath as a prefix.
	Prefix MatchType = "prefix"

	// Regex matches when the compiled RE2 pattern matches the normalized incoming path.
	Regex MatchType = "regex"
)

// QueryMode controls what happens to the raw query string during resolution.
type QueryMode string

const (
	// QIgnore (default) strips the query string from the final Location.
	QIgnore QueryMode = "ignore"

	// QPass appends the original rawQuery to the final Location.
	QPass QueryMode = "pass"

	// QExact embeds the query string in SourcePath (e.g. "/?p=123") and only
	// matches when the incoming query equals that embedded query exactly.
	QExact QueryMode = "exact"
)

// RuleSpec is the un-compiled input as it comes from storage or import.
// SourcePath must already be normalized (call [Normalize] at import/store time).
type RuleSpec struct {
	ID            int64
	SourcePath    string // already-normalized; for QExact includes "?query"
	MatchType     MatchType
	Target        string    // "" means no Location (410/451)
	StatusCode    int       // 301/302/307/308/410/451
	QueryHandling QueryMode // default QIgnore
	Priority      int       // ordered-tier sort key (ASC); lower = tried first
}

// Rule is one immutable compiled rule. Build via [Compile]; never mutate after.
type Rule struct {
	ID            int64
	SourcePath    string
	MatchType     MatchType
	Target        string
	StatusCode    int
	QueryHandling QueryMode
	Priority      int
	re            *regexp.Regexp // non-nil only when MatchType == Regex
}

// Policy configures how incoming paths are normalized before matching.
type Policy struct {
	StripTrailingSlash bool // strip exactly one trailing "/" (root "/" is kept)
	LowerCase          bool // convert to ASCII lowercase
	DecodeOnce         bool // apply url.PathUnescape once
}

// DefaultPolicy returns the recommended production policy:
// strip trailing slash, lowercase, and decode percent-encoding once.
func DefaultPolicy() Policy {
	return Policy{
		StripTrailingSlash: true,
		LowerCase:          true,
		DecodeOnce:         true,
	}
}

// Decision is the result returned by [Resolve].
type Decision struct {
	Matched    bool
	StatusCode int
	Location   string // empty for 410/451 or when Matched is false
}

// CompileError records why a RuleSpec failed to compile.
type CompileError struct {
	Index  int
	ID     int64
	Source string
	Err    string
}

// RuleSet is the compiled, immutable resolver input. Built once per ruleset
// version; shared read-only across goroutines.
type RuleSet struct {
	policy  Policy
	exact   map[string]Rule // keyed by SourcePath (or "path?query" for QExact)
	ordered []Rule          // prefix + regex rules, sorted (Priority ASC, ID ASC)
}
