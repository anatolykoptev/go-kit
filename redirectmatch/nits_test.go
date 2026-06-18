package redirectmatch

import (
	"strings"
	"testing"
)

// A QExact rule whose SourcePath has no embedded query ("?") is never matchable,
// because Resolve only probes the QExact map with a key of the form path+"?"+query.
// Compile must reject it rather than admit a silent dead rule.
func TestCompile_QExact_WithoutQuery_IsRejected(t *testing.T) {
	_, err := Compile(RuleSpec{
		ID:            1,
		SourcePath:    "/foo",
		MatchType:     Exact,
		Target:        "/bar",
		StatusCode:    301,
		QueryHandling: QExact,
	})
	if err == nil {
		t.Fatal("expected error for QExact rule without '?' in SourcePath, got nil")
	}
	if !strings.Contains(err.Error(), "embedded query") {
		t.Fatalf("expected embedded-query error, got: %v", err)
	}
}

// A QExact rule WITH an embedded query compiles fine.
func TestCompile_QExact_WithQuery_IsAccepted(t *testing.T) {
	if _, err := Compile(RuleSpec{
		ID:            2,
		SourcePath:    "/foo?a=1",
		MatchType:     Exact,
		Target:        "/bar",
		StatusCode:    301,
		QueryHandling: QExact,
	}); err != nil {
		t.Fatalf("expected QExact rule with embedded query to compile, got: %v", err)
	}
}

// A regex whose literal ($-containing) Target is matched by its own pattern is a
// self-redirect. The tightened capture-ref detection (a bare "$" is NOT a capture
// ref) must let the static self-loop check fire for such targets.
func TestCompile_RegexLiteralDollarTarget_SelfRedirectCaught(t *testing.T) {
	_, err := Compile(RuleSpec{
		ID:            3,
		SourcePath:    `^/cost\$$`,
		MatchType:     Regex,
		Target:        "/cost$",
		StatusCode:    301,
		QueryHandling: QIgnore,
	})
	if err == nil {
		t.Fatal("expected self-redirect rejection for a regex matching its own literal-$ target, got nil")
	}
}
