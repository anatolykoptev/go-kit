package redirectmatch_test

// fixes_test.go — RED tests for the reviewer-flagged bugs.
// Each test is tagged with the finding it guards.
// These tests were written BEFORE the implementation; run them to confirm RED.

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

// ── CRITICAL: QExact key collision ──────────────────────────────────────────
//
// The CRITICAL bug: a single exact map keyed by "path?query" for QExact rules
// and "path" for non-QExact rules means a QExact "/?p=1" and a QIgnore "/"
// collide at the normalization stage, and one clobbers the other.

// TestResolve_QExactAndQIgnore_SamePath verifies that a QExact rule and a
// QIgnore rule on the same path coexist without collision:
//
//   - request with the matching query   → QExact target
//   - request with no query             → QIgnore target (falls through QExact)
//   - request with a DIFFERENT query    → QIgnore target (falls through QExact)
//
// This is the realistic WordPress ?p=ID migration scenario.
func TestResolve_QExactAndQIgnore_SamePath(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		// QExact: "/" with query "p=123" → "/post/123"
		{ID: 1, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/post/123", StatusCode: 301, QueryHandling: redirectmatch.QExact},
		// QIgnore: "/" (any or no query) → "/home"
		{ID: 2, SourcePath: "/", MatchType: redirectmatch.Exact, Target: "/home", StatusCode: 301, QueryHandling: redirectmatch.QIgnore},
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) > 0 {
		t.Fatalf("BuildRuleSet: unexpected errors: %v", errs)
	}

	t.Run("matching query routes to QExact target", func(t *testing.T) {
		dec := redirectmatch.Resolve(set, "/", "p=123")
		if !dec.Matched {
			t.Fatal("expected match")
		}
		if dec.Location != "/post/123" {
			t.Errorf("Location = %q, want /post/123 (QExact)", dec.Location)
		}
	})

	t.Run("no query routes to QIgnore target", func(t *testing.T) {
		dec := redirectmatch.Resolve(set, "/", "")
		if !dec.Matched {
			t.Fatal("expected match")
		}
		if dec.Location != "/home" {
			t.Errorf("Location = %q, want /home (QIgnore fallback)", dec.Location)
		}
	})

	t.Run("wrong query falls through to QIgnore target", func(t *testing.T) {
		dec := redirectmatch.Resolve(set, "/", "p=999")
		if !dec.Matched {
			t.Fatal("expected match on QIgnore fallback with wrong query")
		}
		if dec.Location != "/home" {
			t.Errorf("Location = %q, want /home (QIgnore fallback for unmatched query)", dec.Location)
		}
	})
}

// TestResolve_QExactCollision_QIgnoreOverwritesQExact demonstrates the CRITICAL
// bug: when both a QIgnore rule and a QExact rule share the same raw SourcePath
// string (e.g. "/?p=123"), they collide in the single map and the later one
// overwrites the earlier. The QExact rule becomes unreachable.
//
// This test MUST FAIL under the old single-map implementation.
func TestResolve_QExactCollision_QIgnoreOverwritesQExact(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		// QExact registered first
		{ID: 1, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/qexact-target", StatusCode: 301, QueryHandling: redirectmatch.QExact},
		// A QIgnore rule whose SourcePath is the literal string "/?p=123" —
		// it gets keyed identically and overwrites the QExact slot in the single map.
		{ID: 2, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/qignore-target", StatusCode: 302, QueryHandling: redirectmatch.QIgnore},
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) > 0 {
		t.Fatalf("BuildRuleSet: unexpected errors: %v", errs)
	}

	// With rawPath="/?p=123" (normalized to "/?p=123") and rawQuery="":
	// Exact tier should find the QIgnore rule at its dedicated path.
	// But more importantly, with rawQuery="p=123" the QExact rule on path "/"
	// is different from a QIgnore rule on path "/?p=123" — these are fundamentally
	// different sources. The two-map design separates them structurally.
	//
	// What we're testing here: QExact "/?p=123" should be reachable via
	// path="/" + query="p=123", while QIgnore "/?p=123" should be reachable
	// via path="/?p=123" + query="".
	//
	// Under the single-map: the key "/?p=123" is used by both, so one is lost.
	// Under the two-map: QExact goes to exactQ["/?p=123"] keyed from pathPart+query;
	// QIgnore goes to exact[Normalize("/?p=123",p)] = exact["/?p=123"].
	// They don't collide.

	// QIgnore lookup: request path "/?p=123", no query
	dec := redirectmatch.Resolve(set, "/?p=123", "")
	if !dec.Matched {
		t.Fatal("QIgnore rule on path '/?p=123' should match request path='/?p=123' query=''")
	}
	if dec.StatusCode != 302 || dec.Location != "/qignore-target" {
		t.Errorf("QIgnore: got status=%d loc=%q, want 302 /qignore-target", dec.StatusCode, dec.Location)
	}

	// QExact lookup: request path "/", query "p=123"
	dec = redirectmatch.Resolve(set, "/", "p=123")
	if !dec.Matched {
		t.Fatal("QExact rule on path '/' query 'p=123' should match")
	}
	if dec.StatusCode != 301 || dec.Location != "/qexact-target" {
		t.Errorf("QExact: got status=%d loc=%q, want 301 /qexact-target", dec.StatusCode, dec.Location)
	}
}

// TestResolve_TwoMapIndependence verifies that a QExact "/?p=1" and an
// unrelated QIgnore "/foo" resolve independently (regression guard for
// namespace collision).
func TestResolve_TwoMapIndependence(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/?p=1", MatchType: redirectmatch.Exact, Target: "/post/1", StatusCode: 301, QueryHandling: redirectmatch.QExact},
		{ID: 2, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 302, QueryHandling: redirectmatch.QIgnore},
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) > 0 {
		t.Fatalf("BuildRuleSet: unexpected errors: %v", errs)
	}

	dec := redirectmatch.Resolve(set, "/", "p=1")
	if !dec.Matched || dec.Location != "/post/1" {
		t.Errorf("QExact rule: Matched=%v Location=%q, want true /post/1", dec.Matched, dec.Location)
	}

	dec = redirectmatch.Resolve(set, "/foo", "")
	if !dec.Matched || dec.Location != "/bar" {
		t.Errorf("QIgnore rule: Matched=%v Location=%q, want true /bar", dec.Matched, dec.Location)
	}
}

// TestResolve_QExact_OrderSensitive pins that QExact query matching is
// RAW byte-equality: order matters, "b=2&a=1" ≠ "a=1&b=2".
func TestResolve_QExact_OrderSensitive(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		// stored as "/?a=1&b=2"
		{ID: 1, SourcePath: "/?a=1&b=2", MatchType: redirectmatch.Exact, Target: "/ordered", StatusCode: 301, QueryHandling: redirectmatch.QExact},
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) > 0 {
		t.Fatalf("BuildRuleSet: unexpected errors: %v", errs)
	}

	t.Run("correct order matches", func(t *testing.T) {
		dec := redirectmatch.Resolve(set, "/", "a=1&b=2")
		if !dec.Matched {
			t.Fatal("expected match for exact order")
		}
	})

	t.Run("reversed order is a MISS", func(t *testing.T) {
		dec := redirectmatch.Resolve(set, "/", "b=2&a=1")
		if dec.Matched {
			t.Error("must NOT match when query param order is reversed (raw byte-equality contract)")
		}
	})
}

// ── MAJOR: reject QExact on non-Exact match types ───────────────────────────

func TestCompile_QExact_OnRegex_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:            1,
		SourcePath:    `^/foo$`,
		MatchType:     redirectmatch.Regex,
		Target:        "/bar",
		StatusCode:    301,
		QueryHandling: redirectmatch.QExact,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject QExact on Regex match type")
	}
}

func TestCompile_QExact_OnPrefix_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:            1,
		SourcePath:    "/foo",
		MatchType:     redirectmatch.Prefix,
		Target:        "/bar",
		StatusCode:    301,
		QueryHandling: redirectmatch.QExact,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject QExact on Prefix match type")
	}
}

// ── MAJOR: reject empty / relative SourcePath ───────────────────────────────

func TestCompile_EmptySourcePath_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "",
		MatchType:  redirectmatch.Exact,
		Target:     "/bar",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject empty SourcePath")
	}
}

func TestCompile_RelativeSourcePath_Exact_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "foo/bar",
		MatchType:  redirectmatch.Exact,
		Target:     "/bar",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject relative SourcePath for Exact (no leading /)")
	}
}

func TestCompile_RelativeSourcePath_Prefix_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "foo",
		MatchType:  redirectmatch.Prefix,
		Target:     "/bar",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject relative SourcePath for Prefix (no leading /)")
	}
}

func TestCompile_EmptyPrefix_IsRejected(t *testing.T) {
	// An empty-string prefix is a catch-all (strings.HasPrefix(x,"") == true).
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "",
		MatchType:  redirectmatch.Prefix,
		Target:     "/bar",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject empty SourcePath for Prefix")
	}
}

func TestCompile_EmptyRegex_IsRejected(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "",
		MatchType:  redirectmatch.Regex,
		Target:     "/bar",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject empty pattern for Regex")
	}
}

// Anchored regex starting with ^ is fine even without a leading slash.
func TestCompile_AnchoredRegex_IsAccepted(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: `^/foo/(\d+)$`,
		MatchType:  redirectmatch.Regex,
		Target:     "/bar/$1",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err != nil {
		t.Errorf("Compile should accept valid anchored regex: %v", err)
	}
}

// ── MAJOR: catch regex identity self-redirect (static-target case) ───────────

// TestCompile_RegexIdentitySelfRedirect checks that a Regex rule whose target
// contains no $n captures is rejected when the compiled regexp matches the
// target (e.g. ^/x$ → /x).
func TestCompile_RegexIdentitySelfRedirect(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: `^/x$`,
		MatchType:  redirectmatch.Regex,
		Target:     "/x",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject regex identity self-redirect (^/x$ → /x)")
	}
}

// TestCompile_RegexWithCaptures_NotRejectedStatically verifies that a Regex
// rule with $n captures in the target is NOT rejected even when the pattern
// would technically match the target string — because static analysis can't
// prove it's a loop.
func TestCompile_RegexWithCaptures_NotRejectedStatically(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: `^/old/(\d+)$`,
		MatchType:  redirectmatch.Regex,
		Target:     "/new/$1",
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err != nil {
		t.Errorf("Compile should NOT reject regex with capture refs in target: %v", err)
	}
}

// ── MAJOR: normalize self-redirect & loop comparisons ───────────────────────

// TestCompile_SelfRedirect_NormalizationAware verifies that a self-redirect
// is caught even when Target and SourcePath differ only by case / encoding.
// Example: SourcePath="/a", Target="/A" — after normalization both are "/a".
func TestCompile_SelfRedirect_NormalizationAware(t *testing.T) {
	spec := redirectmatch.RuleSpec{
		ID:         1,
		SourcePath: "/a",
		MatchType:  redirectmatch.Exact,
		Target:     "/A", // normalized to "/a" under DefaultPolicy (LowerCase=true)
		StatusCode: 301,
	}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should catch self-redirect after normalization (/a and /A are the same normalized path)")
	}
}

// TestValidateNoLoop_NormalizationAware verifies that the 2-cycle check
// catches case/encoding-equal loops.
// candidate: /a → /B; existing: /b → /a  →  /b and /B normalize to same, so it is a 2-cycle.
func TestValidateNoLoop_NormalizationAware(t *testing.T) {
	candidate := redirectmatch.RuleSpec{ID: 2, SourcePath: "/a", MatchType: redirectmatch.Exact, Target: "/B", StatusCode: 301}
	existing := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/b", MatchType: redirectmatch.Exact, Target: "/a", StatusCode: 301},
	}
	err := redirectmatch.ValidateNoLoop(candidate, existing)
	if err == nil {
		t.Error("ValidateNoLoop should catch 2-cycle via normalized comparison (/a→/B with /b→/a, /B normalizes to /b)")
	}
}
