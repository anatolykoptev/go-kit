package admintable_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/admintable"
)

// ---------------------------------------------------------------------------
// FilterSpec.Valid
// ---------------------------------------------------------------------------

// TestFilterSpecValid_OK verifies that a well-formed FilterSpec passes Valid.
func TestFilterSpecValid_OK(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq, Allowed: []string{"free", "pro"}},
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},
		},
	}
	if err := fs.Valid(); err != nil {
		t.Fatalf("Valid() = %v; want nil", err)
	}
}

// TestFilterSpecValid_EmptyFilters verifies that a FilterSpec with no filters passes Valid
// (zero-filter is legal: Where returns empty string).
func TestFilterSpecValid_EmptyFilters(t *testing.T) {
	fs := admintable.FilterSpec{}
	if err := fs.Valid(); err != nil {
		t.Fatalf("Valid() with no filters = %v; want nil", err)
	}
}

// TestFilterSpecValid_DuplicateKey verifies that Valid returns an error for
// two filters sharing the same Key.
func TestFilterSpecValid_DuplicateKey(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
			{Key: "status", SQLExpr: "other_col", Match: admintable.Eq},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for duplicate Key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error %q should mention 'duplicate'", err.Error())
	}
}

// TestFilterSpecValid_EmptySQLExpr verifies that Valid returns an error for a
// Filter with an empty SQLExpr (would produce a broken predicate).
func TestFilterSpecValid_EmptySQLExpr(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "", Match: admintable.Eq},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for empty SQLExpr")
	}
}

// TestFilterSpecValid_UnknownMatch verifies that Valid returns an error for a
// Match value outside the declared set (Eq=0, AnyOf=1).
func TestFilterSpecValid_UnknownMatch(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Match(99)},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for unknown Match value")
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — Eq
// ---------------------------------------------------------------------------

// TestWhere_EqActive verifies that an active Eq filter produces the correct
// predicate and places the value as a bind arg (not in conds).
func TestWhere_EqActive(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{"status": {"active"}}
	conds, args := fs.Where(vals, 1)

	if conds != "subscription_status = $1" {
		t.Errorf("conds = %q; want %q", conds, "subscription_status = $1")
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %v; want [active]", args)
	}
}

// TestWhere_EqEmpty verifies that an Eq filter with an empty value is skipped:
// no predicate, no arg.
func TestWhere_EqEmpty(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{"status": {""}}
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty string (empty value skips filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil (no active filter)", args)
	}
}

// TestWhere_EqAbsentKey verifies that a filter whose key is absent from vals is skipped.
func TestWhere_EqAbsentKey(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{} // "status" not present
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty (absent key skips filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// TestWhere_EqNoFiltersActive verifies that Where returns ("", nil) when no
// filters match.
func TestWhere_EqNoFiltersActive(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq},
		},
	}
	vals := url.Values{} // nothing set
	conds, args := fs.Where(vals, 1)

	if conds != "" || len(args) != 0 {
		t.Errorf("Where with no active filters: got conds=%q args=%v; want empty", conds, args)
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — AnyOf
// ---------------------------------------------------------------------------

// TestWhere_AnyOfActive verifies that AnyOf emits the ANY() predicate and
// passes a []string bind arg (pgx-compatible with text[]).
func TestWhere_AnyOfActive(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},
		},
	}
	vals := url.Values{"source": {"organic", "referral"}}
	conds, args := fs.Where(vals, 1)

	if conds != "source = ANY($1::text[])" {
		t.Errorf("conds = %q; want %q", conds, "source = ANY($1::text[])")
	}
	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	got, ok := args[0].([]string)
	if !ok {
		t.Fatalf("args[0] type = %T; want []string (pgx text[] encoding)", args[0])
	}
	if len(got) != 2 || got[0] != "organic" || got[1] != "referral" {
		t.Errorf("args[0] = %v; want [organic referral]", got)
	}
}

// TestWhere_AnyOfEmpty verifies that AnyOf with an empty values slice is skipped.
func TestWhere_AnyOfEmpty(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},
		},
	}
	vals := url.Values{} // "source" not present → vals["source"] is nil
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty (absent key skips AnyOf filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// ---------------------------------------------------------------------------
// Allowed enum
// ---------------------------------------------------------------------------

// TestWhere_AllowedAccepts verifies that a value in Allowed passes and is used.
func TestWhere_AllowedAccepts(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq, Allowed: []string{"free", "pro", "enterprise"}},
		},
	}
	vals := url.Values{"plan": {"pro"}}
	conds, args := fs.Where(vals, 1)

	if conds == "" {
		t.Fatal("conds empty; want predicate for an allowed value")
	}
	if len(args) != 1 || args[0] != "pro" {
		t.Errorf("args = %v; want [pro]", args)
	}
}

// TestWhere_AllowedRejectsNonMember verifies that a value NOT in Allowed is
// treated as if the filter were absent — the filter is skipped, NOT errored.
// This is the safe-degrade: unknown value ⇒ no filter applied.
func TestWhere_AllowedRejectsNonMember(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq, Allowed: []string{"free", "pro"}},
		},
	}
	// "hacker" is not in Allowed — filter must be skipped (not an error).
	vals := url.Values{"plan": {"hacker"}}
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty (non-member value skips filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// TestWhere_AnyOfAllowedFiltersSlice verifies that for an AnyOf filter with
// Allowed set, only values in Allowed are included in the arg; values outside
// Allowed are stripped from the slice (not an error).
func TestWhere_AnyOfAllowedFiltersSlice(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf, Allowed: []string{"organic", "referral"}},
		},
	}
	// "spam" is not in Allowed and must be stripped.
	vals := url.Values{"source": {"organic", "spam", "referral"}}
	conds, args := fs.Where(vals, 1)

	if conds == "" {
		t.Fatal("conds empty; want predicate for partial-allowed slice")
	}
	got, ok := args[0].([]string)
	if !ok {
		t.Fatalf("args[0] type = %T; want []string", args[0])
	}
	if len(got) != 2 {
		t.Errorf("args[0] = %v; want [organic referral] (spam stripped)", got)
	}
	for _, v := range got {
		if v == "spam" {
			t.Errorf("args[0] contains disallowed value 'spam'")
		}
	}
}

// TestWhere_AnyOfAllowedAllRejected verifies that when all values in a
// multi-value AnyOf param are outside Allowed, the filter is skipped entirely.
func TestWhere_AnyOfAllowedAllRejected(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf, Allowed: []string{"organic"}},
		},
	}
	vals := url.Values{"source": {"spam", "malware"}}
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty when all values rejected by Allowed", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// ---------------------------------------------------------------------------
// $N sequencing
// ---------------------------------------------------------------------------

// TestWhere_StartArgNonOne verifies that When startArg > 1, bind placeholders
// begin from startArg (not from 1). This covers the case where $1/$2 are
// already used for pagination.
func TestWhere_StartArgNonOne(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{"status": {"active"}}
	conds, args := fs.Where(vals, 3) // $1 and $2 already used

	if conds != "subscription_status = $3" {
		t.Errorf("conds = %q; want %q (startArg=3)", conds, "subscription_status = $3")
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %v; want [active]", args)
	}
}

// TestWhere_SequencingSkippedFiltersConsumeNoIndex is the off-by-one trap test.
// Three filters declared; only the first and third are active (second is empty).
// The third must use $startArg+1, NOT $startArg+2.
func TestWhere_SequencingSkippedFiltersConsumeNoIndex(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq},               // active   → $2
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq}, // SKIPPED (empty)
			{Key: "event", SQLExpr: "event_type", Match: admintable.Eq},           // active   → $3
		},
	}
	vals := url.Values{
		"plan": {"pro"},
		// "status" absent → skipped
		"event": {"payment_succeeded"},
	}
	conds, args := fs.Where(vals, 2) // caller already used $1

	wantConds := "plan_id = $2 AND event_type = $3"
	if conds != wantConds {
		t.Errorf("conds = %q; want %q (skipped filter must not consume a $N)", conds, wantConds)
	}
	if len(args) != 2 {
		t.Fatalf("args length = %d; want 2", len(args))
	}
	if args[0] != "pro" {
		t.Errorf("args[0] = %v; want 'pro'", args[0])
	}
	if args[1] != "payment_succeeded" {
		t.Errorf("args[1] = %v; want 'payment_succeeded'", args[1])
	}
}

// TestWhere_MultipleFiltersAll verifies correct sequencing when all filters
// in a multi-filter spec are active.
func TestWhere_MultipleFiltersAll(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq},
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
			{Key: "event", SQLExpr: "event_type", Match: admintable.Eq},
		},
	}
	vals := url.Values{
		"plan":   {"pro"},
		"status": {"active"},
		"event":  {"created"},
	}
	conds, args := fs.Where(vals, 1)

	wantConds := "plan_id = $1 AND subscription_status = $2 AND event_type = $3"
	if conds != wantConds {
		t.Errorf("conds = %q; want %q", conds, wantConds)
	}
	if len(args) != 3 {
		t.Fatalf("args length = %d; want 3", len(args))
	}
}

// ---------------------------------------------------------------------------
// Security invariants
// ---------------------------------------------------------------------------

// TestWhere_InjectionValueGoesToArgs is the core security test:
// an SQL injection attempt in the VALUE goes to args as a bind param,
// never appears in conds. This is the WHERE twin of TestResolve_SQLInjectionKeyFallsToDefault.
func TestWhere_InjectionValueGoesToArgs(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	injected := "'; DROP TABLE subscriptions;--"
	vals := url.Values{"status": {injected}}
	conds, args := fs.Where(vals, 1)

	// The injected string must NOT appear in conds.
	if strings.Contains(conds, injected) {
		t.Errorf("SECURITY: injection value %q appeared in conds %q; must only be in args", injected, conds)
	}
	// It must appear in args as a bind param.
	if len(args) != 1 || args[0] != injected {
		t.Errorf("injection value not in args: got %v; want [%s]", args, injected)
	}
	// conds must only contain the author-declared SQLExpr and literal operators.
	if conds != "subscription_status = $1" {
		t.Errorf("conds = %q; want exactly %q (only author-declared bytes)", conds, "subscription_status = $1")
	}
}

// TestWhere_InjectionValueGoesToArgs_AnyOf verifies the same invariant for AnyOf.
func TestWhere_InjectionValueGoesToArgs_AnyOf(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},
		},
	}
	injected := "'; DROP TABLE users;--"
	vals := url.Values{"source": {injected, "organic"}}
	conds, args := fs.Where(vals, 1)

	// The injected string must NOT appear in conds.
	if strings.Contains(conds, injected) {
		t.Errorf("SECURITY: injection value %q appeared in conds %q; must only be in args", injected, conds)
	}
	// conds must only be the AnyOf literal.
	if conds != "source = ANY($1::text[])" {
		t.Errorf("conds = %q; want %q (only author-declared bytes)", conds, "source = ANY($1::text[])")
	}
	// args[0] must be a []string containing the injected value as a data element.
	got, ok := args[0].([]string)
	if !ok {
		t.Fatalf("args[0] type = %T; want []string", args[0])
	}
	found := false
	for _, v := range got {
		if v == injected {
			found = true
		}
	}
	if !found {
		t.Errorf("injected value not in args slice: %v", got)
	}
}

// TestWhere_UndeclaredKeyIgnored verifies the spec boundary: a URL parameter
// whose key is NOT declared in the FilterSpec is silently ignored — it cannot
// inject a predicate.
func TestWhere_UndeclaredKeyIgnored(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{
		"status":       {"active"},
		"UNDECLARED":   {"evil"},                                // not in spec
		"status; DROP": {"anything"},                            // hostile key not in spec
		"source":       {"'; SELECT * FROM secrets WHERE ''='"}, // another undeclared hostile key
	}
	conds, args := fs.Where(vals, 1)

	// Only the declared "status" filter must appear.
	if conds != "subscription_status = $1" {
		t.Errorf("conds = %q; want only declared filter predicate", conds)
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %v; want only [active]", args)
	}

	// Hostile key bytes must not appear in conds.
	hostileBytes := []string{"UNDECLARED", "DROP", "SELECT", "secrets"}
	for _, h := range hostileBytes {
		if strings.Contains(conds, h) {
			t.Errorf("SECURITY: hostile byte %q appeared in conds %q", h, conds)
		}
	}
}

// TestWhere_PropertyInjectionInvariant is the property sweep: for a range of
// hostile/random URL values, the conds output MUST consist ONLY of
// author-declared SQLExpr bytes, literal operators, and bind placeholders.
// No URL value bytes may appear in conds.
func TestWhere_PropertyInjectionInvariant(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq, Allowed: []string{"free", "pro"}},
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},
		},
	}

	// These hostile strings are placed as VALUES (not keys) — they must
	// never appear verbatim in conds as injected SQL fragments.
	// Strings that happen to also be substrings of author-declared SQLExpr
	// bytes (e.g. the column name itself) or of literal operators are excluded
	// from the property check below, because those bytes reach conds legitimately
	// via the author-constant path, not via the user-value path.
	//
	// The specific SQLExpr values in this spec are:
	//   "subscription_status", "plan_id", "source"
	// and literal operators contain "$1", "$2", etc.
	//
	// Hostile strings that don't overlap with those author-declared bytes must
	// not appear in conds in any form.
	hostileValues := []string{
		"'; DROP TABLE subscriptions;--",
		"1 OR 1=1",
		"active'; DELETE FROM users;--",
		"\x00null\x00",
		"<script>alert(1)</script>",
		"../../../etc/passwd",
		"UNION SELECT password FROM users",
		"1; DROP TABLE plans;--",
	}

	for _, hv := range hostileValues {
		vals := url.Values{
			"status": {hv},
			"source": {hv, "organic"},
		}
		conds, _ := fs.Where(vals, 1)

		if strings.Contains(conds, hv) {
			t.Errorf("SECURITY: hostile value %q appeared in conds %q", hv, conds)
		}
	}
}

// ---------------------------------------------------------------------------
// Combined sort + filter startArg integration
// ---------------------------------------------------------------------------

// TestWhere_ComposedWithSortArgs verifies the typical usage pattern where the
// caller builds "ORDER BY ... LIMIT $1 OFFSET $2" and passes startArg=3 for
// filter params — placeholders must be $3, $4, ...
func TestWhere_ComposedWithSortArgs(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq},
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	// Simulates: ... LIMIT $1 OFFSET $2 already placed, so filter starts at $3.
	vals := url.Values{
		"plan":   {"pro"},
		"status": {"active"},
	}
	conds, args := fs.Where(vals, 3)

	wantConds := "plan_id = $3 AND subscription_status = $4"
	if conds != wantConds {
		t.Errorf("conds = %q; want %q", conds, wantConds)
	}
	if len(args) != 2 {
		t.Fatalf("args length = %d; want 2", len(args))
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Valid — ILike-specific validation
// ---------------------------------------------------------------------------

// TestFilterSpecValid_ILike_OK verifies that a well-formed ILike filter passes Valid.
func TestFilterSpecValid_ILike_OK(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike},
		},
	}
	if err := fs.Valid(); err != nil {
		t.Fatalf("Valid() = %v; want nil", err)
	}
}

// TestFilterSpecValid_ILike_SingleColumn verifies that a single-column ILike filter passes Valid.
func TestFilterSpecValid_ILike_SingleColumn(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	if err := fs.Valid(); err != nil {
		t.Fatalf("Valid() = %v; want nil for single-column ILike", err)
	}
}

// TestFilterSpecValid_ILike_ZeroColumns verifies that Valid rejects an ILike filter
// with no columns declared.
func TestFilterSpecValid_ILike_ZeroColumns(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: nil, Match: admintable.ILike},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for ILike with zero SQLExprs")
	}
	if !strings.Contains(err.Error(), "SQLExprs") {
		t.Errorf("error %q should mention 'SQLExprs'", err.Error())
	}
}

// TestFilterSpecValid_ILike_SQLExprSetIsError verifies that Valid rejects an ILike
// filter that also sets SQLExpr (field reserved for Eq/AnyOf).
func TestFilterSpecValid_ILike_SQLExprSetIsError(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExpr: "name", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for ILike with SQLExpr set")
	}
}

// TestFilterSpecValid_ILike_AllowedIsError verifies that Valid rejects an ILike
// filter that sets Allowed (meaningless for free-text search; fail-fast at startup).
func TestFilterSpecValid_ILike_AllowedIsError(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike, Allowed: []string{"foo"}},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for ILike with Allowed set")
	}
	if !strings.Contains(err.Error(), "Allowed") {
		t.Errorf("error %q should mention 'Allowed'", err.Error())
	}
}

// TestFilterSpecValid_EqWithSQLExprsIsError verifies that Valid rejects an Eq
// filter that sets SQLExprs (field reserved for ILike).
func TestFilterSpecValid_EqWithSQLExprsIsError(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "status", SQLExpr: "subscription_status", SQLExprs: []string{"extra"}, Match: admintable.Eq},
		},
	}
	err := fs.Valid()
	if err == nil {
		t.Fatal("Valid() = nil; want error for Eq with SQLExprs set")
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — ILike: predicate shape
// ---------------------------------------------------------------------------

// TestWhere_ILike_MultiColumn verifies the core case: two columns, one bind.
// The placeholder $N must appear twice in conds (once per column) but only one
// arg is appended. The wrapped value must be the escaped term with % prefix/suffix.
func TestWhere_ILike_MultiColumn(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {"alice"}}
	conds, args := fs.Where(vals, 3) // $1/$2 already used

	wantConds := `(name ILIKE $3 ESCAPE '\' OR notes ILIKE $3 ESCAPE '\')`
	if conds != wantConds {
		t.Errorf("conds = %q; want %q", conds, wantConds)
	}
	// Exactly one arg despite two columns: $3 is reused, not doubled.
	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1 (single bind for multi-column ILike)", len(args))
	}
	if args[0] != "%alice%" {
		t.Errorf("args[0] = %q; want %q (term wrapped as %%term%%)", args[0], "%alice%")
	}
}

// TestWhere_ILike_SingleColumn verifies that a single-column ILike emits the
// predicate WITHOUT outer parentheses (no OR needed).
func TestWhere_ILike_SingleColumn(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {"bob"}}
	conds, args := fs.Where(vals, 1)

	wantConds := `name ILIKE $1 ESCAPE '\'`
	if conds != wantConds {
		t.Errorf("conds = %q; want %q", conds, wantConds)
	}
	if len(args) != 1 || args[0] != "%bob%" {
		t.Errorf("args = %v; want [%%bob%%]", args)
	}
}

// TestWhere_ILike_EmptyTermSkipped verifies that an empty search term skips the
// filter — no predicate emitted, no arg index consumed.
func TestWhere_ILike_EmptyTermSkipped(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {""}}
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty (empty term skips ILike filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// TestWhere_ILike_AbsentKeySkipped verifies that an absent key skips the filter.
func TestWhere_ILike_AbsentKeySkipped(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{} // "q" not present
	conds, args := fs.Where(vals, 1)

	if conds != "" {
		t.Errorf("conds = %q; want empty (absent key skips ILike filter)", conds)
	}
	if len(args) != 0 {
		t.Errorf("args = %v; want nil", args)
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — ILike: metacharacter escaping
// ---------------------------------------------------------------------------

// TestWhere_ILike_EscapePercent verifies that a % in the search term is escaped
// to \% in the bound value, so it matches the literal "%" not every character.
// The probe: searching "50%" must bind "%50\%%" not "%50%%".
func TestWhere_ILike_EscapePercent(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {"50%"}}
	_, args := fs.Where(vals, 1)

	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	// Expected: \ escapes %, then wrapped as %...%
	want := `%50\%%`
	if args[0] != want {
		t.Errorf("args[0] = %q; want %q (percent must be escaped to prevent wildcard match)", args[0], want)
	}
}

// TestWhere_ILike_EscapeUnderscore verifies that _ in the search term is escaped
// to \_ in the bound value, so it matches the literal "_" not any single character.
func TestWhere_ILike_EscapeUnderscore(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {"user_name"}}
	_, args := fs.Where(vals, 1)

	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	want := `%user\_name%`
	if args[0] != want {
		t.Errorf("args[0] = %q; want %q (underscore must be escaped)", args[0], want)
	}
}

// TestWhere_ILike_EscapeBackslash verifies that a backslash in the search term
// is escaped to \\ so that it matches the literal "\" and doesn't interfere with
// the escaping of subsequent % or _ characters.
func TestWhere_ILike_EscapeBackslash(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	vals := url.Values{"q": {`path\file`}}
	_, args := fs.Where(vals, 1)

	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	want := `%path\\file%`
	if args[0] != want {
		t.Errorf("args[0] = %q; want %q (backslash must be escaped first)", args[0], want)
	}
}

// TestWhere_ILike_EscapeAllMetachars verifies that a term containing all three
// metacharacters is correctly escaped. The backslash must be escaped BEFORE % and _
// to avoid double-escaping.
func TestWhere_ILike_EscapeAllMetachars(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name"}, Match: admintable.ILike},
		},
	}
	// term: 50%_\end
	vals := url.Values{"q": {`50%_\end`}}
	_, args := fs.Where(vals, 1)

	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	// Expected escaping order: \ → \\, % → \%, _ → \_
	// "50%_\end" → "50\%\_\\end" → wrapped → "%50\%\_\\end%"
	want := `%50\%\_\\end%`
	if args[0] != want {
		t.Errorf("args[0] = %q; want %q", args[0], want)
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — ILike: $N sequencing
// ---------------------------------------------------------------------------

// TestWhere_ILike_ConsumesOneIndex verifies the core sequencing invariant:
// an ILike filter with N columns advances the arg index by exactly 1, even
// though $N appears N times in conds.
func TestWhere_ILike_ConsumesOneIndex(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			// ILike with 3 columns: $2 appears 3 times but only one arg consumed.
			{Key: "q", SQLExprs: []string{"name", "notes", "tags"}, Match: admintable.ILike},
			// Next filter must get $3, not $5.
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},
		},
	}
	vals := url.Values{
		"q":      {"alice"},
		"status": {"active"},
	}
	conds, args := fs.Where(vals, 2) // $1 already used

	wantConds := `(name ILIKE $2 ESCAPE '\' OR notes ILIKE $2 ESCAPE '\' OR tags ILIKE $2 ESCAPE '\') AND subscription_status = $3`
	if conds != wantConds {
		t.Errorf("conds = %q; want %q", conds, wantConds)
	}
	// Two args: one for ILike (the bound term), one for Eq.
	if len(args) != 2 {
		t.Fatalf("args length = %d; want 2 (ILike consumes one index)", len(args))
	}
	if args[0] != "%alice%" {
		t.Errorf("args[0] = %q; want %%alice%%", args[0])
	}
	if args[1] != "active" {
		t.Errorf("args[1] = %q; want active", args[1])
	}
}

// TestWhere_ILike_SkippedConsumesNoIndex verifies that a skipped ILike filter
// (empty term) does NOT advance the arg index — the next active filter gets the
// same $N the ILike would have consumed.
func TestWhere_ILike_SkippedConsumesNoIndex(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike}, // SKIPPED
			{Key: "status", SQLExpr: "subscription_status", Match: admintable.Eq},    // active → $1
		},
	}
	vals := url.Values{
		// "q" absent → ILike skipped
		"status": {"active"},
	}
	conds, args := fs.Where(vals, 1)

	if conds != "subscription_status = $1" {
		t.Errorf("conds = %q; want %q (skipped ILike must not consume $N)", conds, "subscription_status = $1")
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %v; want [active]", args)
	}
}

// TestWhere_ILike_MixedSequencing is the full off-by-one matrix:
// Eq(active) → ILike(skipped) → AnyOf(active) → ILike(active) → Eq(active).
// Verifies each placeholder is exactly correct.
func TestWhere_ILike_MixedSequencing(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "plan", SQLExpr: "plan_id", Match: admintable.Eq},                       // $3
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike},      // SKIPPED
			{Key: "source", SQLExpr: "source", Match: admintable.AnyOf},                   // $4
			{Key: "search", SQLExprs: []string{"title", "body"}, Match: admintable.ILike}, // $5 (one index)
			{Key: "event", SQLExpr: "event_type", Match: admintable.Eq},                   // $6
		},
	}
	vals := url.Values{
		"plan": {"pro"},
		// "q" absent → ILike skipped
		"source": {"organic", "referral"},
		"search": {"hello"},
		"event":  {"created"},
	}
	conds, args := fs.Where(vals, 3) // $1/$2 already used

	wantConds := `plan_id = $3 AND source = ANY($4::text[]) AND (title ILIKE $5 ESCAPE '\' OR body ILIKE $5 ESCAPE '\') AND event_type = $6`
	if conds != wantConds {
		t.Errorf("conds =\n  %q\nwant\n  %q", conds, wantConds)
	}
	if len(args) != 4 {
		t.Fatalf("args length = %d; want 4", len(args))
	}
	if args[0] != "pro" {
		t.Errorf("args[0] = %q; want pro", args[0])
	}
	sourceSlice, ok := args[1].([]string)
	if !ok || len(sourceSlice) != 2 {
		t.Errorf("args[1] = %v; want []string{organic, referral}", args[1])
	}
	if args[2] != "%hello%" {
		t.Errorf("args[2] = %q; want %%hello%%", args[2])
	}
	if args[3] != "created" {
		t.Errorf("args[3] = %q; want created", args[3])
	}
}

// ---------------------------------------------------------------------------
// FilterSpec.Where — ILike: security invariants
// ---------------------------------------------------------------------------

// TestWhere_ILike_InjectionValueGoesToArgs verifies the core security invariant
// for ILike: an SQL injection attempt in the search term goes to args as a bind
// param, never appears in conds. The conds must contain only author-declared
// column names, literal ILIKE operators, and $N placeholders.
func TestWhere_ILike_InjectionValueGoesToArgs(t *testing.T) {
	fs := admintable.FilterSpec{
		Filters: []admintable.Filter{
			{Key: "q", SQLExprs: []string{"name", "notes"}, Match: admintable.ILike},
		},
	}
	injected := "'; DROP TABLE accounts;--"
	vals := url.Values{"q": {injected}}
	conds, args := fs.Where(vals, 1)

	// The raw injected string must NOT appear in conds.
	if strings.Contains(conds, injected) {
		t.Errorf("SECURITY: injection value %q appeared in conds %q; must only be in args", injected, conds)
	}
	// conds must be exactly the author-declared predicate with $1.
	wantConds := `(name ILIKE $1 ESCAPE '\' OR notes ILIKE $1 ESCAPE '\')`
	if conds != wantConds {
		t.Errorf("conds = %q; want exactly %q (only author-declared bytes)", conds, wantConds)
	}
	// The escaped-and-wrapped term must appear in args, not conds.
	if len(args) != 1 {
		t.Fatalf("args length = %d; want 1", len(args))
	}
	// Verify the bound value starts and ends with % (wrapped), and does not
	// appear verbatim in conds.
	bound, ok := args[0].(string)
	if !ok {
		t.Fatalf("args[0] type = %T; want string", args[0])
	}
	if !strings.HasPrefix(bound, "%") || !strings.HasSuffix(bound, "%") {
		t.Errorf("bound value %q is not wrapped as %%...%%", bound)
	}
	if strings.Contains(conds, bound) {
		t.Errorf("SECURITY: bound value %q appeared in conds %q", bound, conds)
	}
}
