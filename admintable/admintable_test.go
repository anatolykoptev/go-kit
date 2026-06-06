package admintable_test

import (
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/admintable"
)

// testSpec is a representative Spec used across all test cases.
// Columns: person (sortable), stage (sortable), kind (sortable),
//
//	notes (NOT sortable), updated (sortable key="updated", NOT "updated_at").
//
// Default: updated DESC.
var testSpec = admintable.Spec{
	Columns: []admintable.Column{
		{Key: "person", Label: "Person / Firm", Sortable: true, SQLExpr: "p.name"},
		{Key: "stage", Label: "Stage", Sortable: true, SQLExpr: "i.stage"},
		{Key: "kind", Label: "Kind", Sortable: true, SQLExpr: "i.kind"},
		{Key: "notes", Label: "Notes", Sortable: false, SQLExpr: "i.notes"},
		{Key: "updated", Label: "Updated", Sortable: true, SQLExpr: "i.updated_at"},
	},
	DefaultKey: "updated",
	DefaultDir: admintable.Desc,
}

// TestResolve_ValidKeyAndDir verifies that a valid sortable key + valid dir
// are accepted as-is.
func TestResolve_ValidKeyAndDir(t *testing.T) {
	st := testSpec.Resolve("stage", "asc")
	if st.Key != "stage" {
		t.Errorf("Key: got %q; want %q", st.Key, "stage")
	}
	if st.Dir != admintable.Asc {
		t.Errorf("Dir: got %q; want Asc", st.Dir)
	}
}

// TestResolve_SQLInjectionKeyFallsToDefault verifies that an adversarial sort
// key containing SQL injection tokens is rejected and falls back to DefaultKey.
// This is the core injection invariant for the Key axis.
func TestResolve_SQLInjectionKeyFallsToDefault(t *testing.T) {
	st := testSpec.Resolve("'; DROP TABLE users;--", "asc")
	if st.Key != testSpec.DefaultKey {
		t.Errorf("Key: got %q; want default %q", st.Key, testSpec.DefaultKey)
	}
	if st.Dir != admintable.Asc {
		t.Errorf("Dir: got %q; want Asc (valid dir accepted)", st.Dir)
	}
}

// TestResolve_RawSQLColumnNameRejected verifies that "updated_at" (a plausible
// SQL column name that is NOT a declared Column.Key — only "updated" is) falls
// back to DefaultKey. PROVES only Spec.Column.Key values pass, not arbitrary
// SQL column names from the URL.
func TestResolve_RawSQLColumnNameRejected(t *testing.T) {
	st := testSpec.Resolve("updated_at", "ASC")
	if st.Key != testSpec.DefaultKey {
		t.Errorf("Key: got %q; want default %q (raw SQL column name must be rejected)", st.Key, testSpec.DefaultKey)
	}
}

// TestResolve_NonSortableColumnRejected verifies that a Column with Sortable:false
// is not accepted as a sort key even though its Key is declared in the Spec.
func TestResolve_NonSortableColumnRejected(t *testing.T) {
	st := testSpec.Resolve("notes", "asc")
	if st.Key != testSpec.DefaultKey {
		t.Errorf("Key: got %q; want default %q (non-sortable column must be rejected)", st.Key, testSpec.DefaultKey)
	}
}

// TestResolve_InvalidDirFallsToDefault verifies that an invalid dir string falls
// back to DefaultDir while a valid key is still accepted.
func TestResolve_InvalidDirFallsToDefault(t *testing.T) {
	st := testSpec.Resolve("stage", "; DELETE")
	if st.Key != "stage" {
		t.Errorf("Key: got %q; want %q", st.Key, "stage")
	}
	if st.Dir != testSpec.DefaultDir {
		t.Errorf("Dir: got %q; want default %q", st.Dir, testSpec.DefaultDir)
	}
}

// TestResolve_DirCaseInsensitive verifies that "DESC" (uppercase) is accepted and
// normalized to Dir:Desc.
func TestResolve_DirCaseInsensitive(t *testing.T) {
	st := testSpec.Resolve("stage", "DESC")
	if st.Dir != admintable.Desc {
		t.Errorf("Dir: got %q; want Desc (uppercase accepted)", st.Dir)
	}
}

// TestOrderBy_ReturnsSpecOwnedExprPlusDirection verifies that OrderBy for a
// specific column returns exactly that column's SQLExpr + " DESC" (literal),
// with no URL parameter bytes present.
func TestOrderBy_ReturnsSpecOwnedExprPlusDirection(t *testing.T) {
	st := admintable.State{Key: "stage", Dir: admintable.Desc}
	got := testSpec.OrderBy(st)
	want := "i.stage DESC"
	if got != want {
		t.Errorf("OrderBy: got %q; want %q", got, want)
	}
	if !strings.HasSuffix(got, " DESC") && !strings.HasSuffix(got, " ASC") {
		t.Errorf("OrderBy result %q does not end with literal direction", got)
	}
}

// TestOrderBy_PropertyInjectionInvariant is the property test:
// for a fixed Spec and a slice of hostile/random sort+dir pairs,
// OrderBy(Resolve(sort,dir)) MUST always be a member of the finite set
// { each_sortable_SQLExpr × {" ASC"," DESC"} }.
//
// This proves that no URL input can produce an ORDER BY fragment outside
// the author-defined closed set — SQL injection via sort/dir is impossible.
func TestOrderBy_PropertyInjectionInvariant(t *testing.T) {
	// Pre-compute the finite allowed set.
	allowed := make(map[string]bool)
	for _, col := range testSpec.Columns {
		if col.Sortable {
			allowed[col.SQLExpr+" ASC"] = true
			allowed[col.SQLExpr+" DESC"] = true
		}
	}

	hostile := []struct{ sort, dir string }{
		{"'; DROP TABLE users;--", "asc"},
		{"1 OR 1=1", "desc"},
		{"updated_at", "ASC"},  // raw SQL col name not in Spec
		{"notes", "asc"},       // non-sortable key
		{"stage", "; DELETE"},  // hostile dir
		{"stage", "ASCENDING"}, // invalid dir variant
		{"", ""},               // empty both
		{"stage\x00", "asc"},   // null byte in key
		{"person", "1; DROP"},  // hostile dir with valid key
		{"kind", "   asc   "},  // whitespace-padded dir (spec says trim)
	}

	for _, tc := range hostile {
		st := testSpec.Resolve(tc.sort, tc.dir)
		result := testSpec.OrderBy(st)
		if !allowed[result] {
			t.Errorf("OrderBy(Resolve(%q, %q)) = %q; not in allowed set %v",
				tc.sort, tc.dir, result, allowedKeys(allowed))
		}
	}
}

// ---------------------------------------------------------------------------
// TieBreakSQLExpr: when set, OrderBy must emit "<primary> <DIR> [NULLS LAST], <tiebreak>"
// ---------------------------------------------------------------------------

// TestOrderBy_TieBreak verifies that a Column with TieBreakSQLExpr set emits
// the tie-break literal appended after the primary clause, separated by a comma.
func TestOrderBy_TieBreak(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "fit", Sortable: true, SQLExpr: "fit_score", NullsLast: true, TieBreakSQLExpr: "last_seen_at DESC"},
			{Key: "posted", Sortable: true, SQLExpr: "posted_at", NullsLast: true, TieBreakSQLExpr: "last_seen_at DESC"},
			{Key: "recent", Sortable: true, SQLExpr: "last_seen_at"},
		},
		DefaultKey: "fit",
		DefaultDir: admintable.Desc,
	}

	t.Run("fit_desc_with_tiebreak", func(t *testing.T) {
		st := admintable.State{Key: "fit", Dir: admintable.Desc}
		got := sp.OrderBy(st)
		want := "fit_score DESC NULLS LAST, last_seen_at DESC"
		if got != want {
			t.Errorf("OrderBy TieBreak+Desc: got %q; want %q", got, want)
		}
	})

	t.Run("fit_asc_with_tiebreak", func(t *testing.T) {
		st := admintable.State{Key: "fit", Dir: admintable.Asc}
		got := sp.OrderBy(st)
		want := "fit_score ASC NULLS LAST, last_seen_at DESC"
		if got != want {
			t.Errorf("OrderBy TieBreak+Asc: got %q; want %q", got, want)
		}
	})

	t.Run("posted_with_tiebreak", func(t *testing.T) {
		st := admintable.State{Key: "posted", Dir: admintable.Desc}
		got := sp.OrderBy(st)
		want := "posted_at DESC NULLS LAST, last_seen_at DESC"
		if got != want {
			t.Errorf("OrderBy TieBreak posted: got %q; want %q", got, want)
		}
	})

	t.Run("no_tiebreak_unchanged", func(t *testing.T) {
		st := admintable.State{Key: "recent", Dir: admintable.Asc}
		got := sp.OrderBy(st)
		want := "last_seen_at ASC"
		if got != want {
			t.Errorf("OrderBy without TieBreak: got %q; want %q", got, want)
		}
	})
}

// TestOrderBy_PropertyInjectionInvariant_WithTieBreak extends the injection
// invariant to cover TieBreakSQLExpr. The allowed set includes tie-break suffix
// forms. The output must not contain URL bytes.
func TestOrderBy_PropertyInjectionInvariant_WithTieBreak(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "fit", Sortable: true, SQLExpr: "fit_score", NullsLast: true, TieBreakSQLExpr: "last_seen_at DESC"},
			{Key: "recent", Sortable: true, SQLExpr: "last_seen_at"},
			{Key: "stage", Sortable: true, SQLExpr: "stage"},
		},
		DefaultKey: "fit",
		DefaultDir: admintable.Desc,
	}

	// Build allowed set including tie-break variants.
	allowed := make(map[string]bool)
	for _, col := range sp.Columns {
		if !col.Sortable {
			continue
		}
		for _, dir := range []string{" ASC", " DESC"} {
			base := col.SQLExpr + dir
			if col.NullsLast {
				base += " NULLS LAST"
			}
			if col.TieBreakSQLExpr != "" {
				base += ", " + col.TieBreakSQLExpr
			}
			allowed[base] = true
		}
	}

	hostile := []struct{ sort, dir string }{
		{"'; DROP TABLE users;--", "asc"},
		{"1 OR 1=1", "desc"},
		{"fit", "asc"},
		{"fit", "desc"},
		{"fit", "; DELETE"},
		{"recent", "asc"},
		{"stage", "desc"},
		{"", ""},
	}
	hostileBytes := []string{"DROP", "DELETE", "SELECT", "OR 1=1", "--", ";"}

	for _, tc := range hostile {
		st := sp.Resolve(tc.sort, tc.dir)
		result := sp.OrderBy(st)
		if !allowed[result] {
			t.Errorf("OrderBy(Resolve(%q,%q)) = %q; not in allowed set %v",
				tc.sort, tc.dir, result, allowedKeys(allowed))
		}
		for _, hb := range hostileBytes {
			if strings.Contains(result, hb) {
				t.Errorf("OrderBy(%q,%q) = %q; contains hostile byte %q",
					tc.sort, tc.dir, result, hb)
			}
		}
	}
}

// TestSpecValid_WithTieBreakSQLExpr verifies that Valid() passes on a Spec
// that has TieBreakSQLExpr set.
func TestSpecValid_WithTieBreakSQLExpr(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "fit", Sortable: true, SQLExpr: "fit_score", NullsLast: true, TieBreakSQLExpr: "last_seen_at DESC"},
			{Key: "recent", Sortable: true, SQLExpr: "last_seen_at"},
		},
		DefaultKey: "fit",
		DefaultDir: admintable.Desc,
	}
	if err := sp.Valid(); err != nil {
		t.Errorf("Valid() = %v; want nil for spec with TieBreakSQLExpr", err)
	}
}

// ---------------------------------------------------------------------------
// NullsLast: direction must precede NULLS LAST in valid Postgres ORDER BY syntax.
// Regression guard for SQLSTATE 42601 caused by "expr NULLS LAST ASC".
// ---------------------------------------------------------------------------

// TestOrderBy_NullsLast verifies that a Column with NullsLast:true emits
// "<SQLExpr> ASC NULLS LAST" / "<SQLExpr> DESC NULLS LAST" (direction before
// the NULLS clause — the only valid Postgres ORDER BY syntax).
func TestOrderBy_NullsLast(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "updated", Sortable: true, SQLExpr: "i.updated_at"},
			{Key: "followup", Sortable: true, SQLExpr: "i.next_follow_up", NullsLast: true},
		},
		DefaultKey: "updated",
		DefaultDir: admintable.Desc,
	}

	t.Run("nullslast_asc", func(t *testing.T) {
		st := admintable.State{Key: "followup", Dir: admintable.Asc}
		got := sp.OrderBy(st)
		want := "i.next_follow_up ASC NULLS LAST"
		if got != want {
			t.Errorf("OrderBy NullsLast+Asc: got %q; want %q", got, want)
		}
	})

	t.Run("nullslast_desc", func(t *testing.T) {
		st := admintable.State{Key: "followup", Dir: admintable.Desc}
		got := sp.OrderBy(st)
		want := "i.next_follow_up DESC NULLS LAST"
		if got != want {
			t.Errorf("OrderBy NullsLast+Desc: got %q; want %q", got, want)
		}
	})

	t.Run("no_nullslast_unchanged", func(t *testing.T) {
		st := admintable.State{Key: "updated", Dir: admintable.Asc}
		got := sp.OrderBy(st)
		want := "i.updated_at ASC"
		if got != want {
			t.Errorf("OrderBy without NullsLast: got %q; want %q", got, want)
		}
	})
}

// TestOrderBy_PropertyInjectionInvariant_WithNullsLast strengthens the injection
// property test by including a NullsLast column in the spec.
// The output must still ∈ the finite closed set:
//
//	{ sqlExpr } × { " ASC", " DESC", " ASC NULLS LAST", " DESC NULLS LAST" }
//
// and must contain no URL bytes.
func TestOrderBy_PropertyInjectionInvariant_WithNullsLast(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "person", Sortable: true, SQLExpr: "p.name"},
			{Key: "followup", Sortable: true, SQLExpr: "i.next_follow_up", NullsLast: true},
			{Key: "updated", Sortable: true, SQLExpr: "i.updated_at"},
		},
		DefaultKey: "updated",
		DefaultDir: admintable.Desc,
	}

	// Build the finite allowed set (includes " ASC NULLS LAST" / " DESC NULLS LAST").
	allowed := make(map[string]bool)
	for _, col := range sp.Columns {
		if col.Sortable {
			if col.NullsLast {
				allowed[col.SQLExpr+" ASC NULLS LAST"] = true
				allowed[col.SQLExpr+" DESC NULLS LAST"] = true
			} else {
				allowed[col.SQLExpr+" ASC"] = true
				allowed[col.SQLExpr+" DESC"] = true
			}
		}
	}

	hostile := []struct{ sort, dir string }{
		{"'; DROP TABLE users;--", "asc"},
		{"1 OR 1=1", "desc"},
		{"followup", "asc"},
		{"followup", "desc"},
		{"followup", "; DELETE"},
		{"followup", "NULLS FIRST"},
		{"person", "asc"},
		{"updated", "desc"},
		{"", ""},
	}

	hostileBytes := []string{
		"DROP", "DELETE", "SELECT", "INSERT", "UPDATE", "OR 1=1",
		"--", ";", "<script>", "../", "\x00",
	}

	for _, tc := range hostile {
		st := sp.Resolve(tc.sort, tc.dir)
		result := sp.OrderBy(st)

		if !allowed[result] {
			t.Errorf("OrderBy(Resolve(%q, %q)) = %q; not in allowed set %v",
				tc.sort, tc.dir, result, allowedKeys(allowed))
		}
		for _, hb := range hostileBytes {
			if strings.Contains(result, hb) {
				t.Errorf("OrderBy(%q, %q) = %q; contains hostile byte %q",
					tc.sort, tc.dir, result, hb)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Defensive: OrderBy on empty/misconfigured Spec must never produce bare fragment.
// ---------------------------------------------------------------------------

// TestOrderBy_EmptySpec_NeverEmptyFragment asserts that Spec{} (zero columns,
// empty DefaultKey) or a Spec with a bad DefaultKey must never produce an ORDER BY
// fragment that starts with a space or is empty.
func TestOrderBy_EmptySpec_NeverEmptyFragment(t *testing.T) {
	t.Run("zero_columns", func(t *testing.T) {
		sp := admintable.Spec{}
		st := sp.Resolve("x", "asc")
		got := sp.OrderBy(st)
		if got == "" {
			t.Errorf("OrderBy on empty Spec returned empty string")
		}
		if strings.HasPrefix(got, " ") {
			t.Errorf("OrderBy on empty Spec starts with space: %q", got)
		}
		if got == " ASC" || got == " DESC" {
			t.Errorf("OrderBy on empty Spec emitted bare direction %q", got)
		}
	})

	t.Run("bad_default_key_with_sortable_columns", func(t *testing.T) {
		// DefaultKey "typo" matches nothing → must fall back to first sortable column.
		sp := admintable.Spec{
			Columns: []admintable.Column{
				{Key: "name", Sortable: true, SQLExpr: "p.name"},
				{Key: "stage", Sortable: true, SQLExpr: "i.stage"},
			},
			DefaultKey: "typo",
			DefaultDir: admintable.Asc,
		}
		st := sp.Resolve("x", "asc")
		got := sp.OrderBy(st)
		if strings.HasPrefix(got, " ") {
			t.Errorf("OrderBy with bad DefaultKey starts with space: %q", got)
		}
		if got == " ASC" || got == " DESC" {
			t.Errorf("OrderBy with bad DefaultKey emitted bare direction %q", got)
		}
		// Must use first sortable column's expr.
		if got != "p.name ASC" {
			t.Errorf("OrderBy with bad DefaultKey: got %q; want %q", got, "p.name ASC")
		}
	})
}

// ---------------------------------------------------------------------------
// Resolve with unset DefaultDir must normalize to Asc.
// ---------------------------------------------------------------------------

// TestResolve_UnsetDefaultDir_NormalizesToAsc asserts that when DefaultDir is
// neither Asc nor Desc (zero value ""), and an invalid dir is passed, Resolve
// must return Dir == Asc (canonical fallback), not Dir("").
func TestResolve_UnsetDefaultDir_NormalizesToAsc(t *testing.T) {
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: "name", Sortable: true, SQLExpr: "p.name"},
		},
		DefaultKey: "name",
		DefaultDir: "", // unset — zero value
	}
	st := sp.Resolve("name", "INVALID")
	if st.Dir != admintable.Asc {
		t.Errorf("Dir: got %q; want Asc (unset DefaultDir must normalize to Asc)", st.Dir)
	}
}

// ---------------------------------------------------------------------------
// OrderBy must pick the SORTABLE column, not a non-sortable dup.
// ---------------------------------------------------------------------------

// TestOrderBy_DuplicateKey_PicksSortableColumn asserts that when two columns
// share a Key (first non-sortable, second sortable), both Resolve and OrderBy
// must pick the sortable one.
func TestOrderBy_DuplicateKey_PicksSortableColumn(t *testing.T) {
	const dupKey = "dup"
	sp := admintable.Spec{
		Columns: []admintable.Column{
			{Key: dupKey, Sortable: false, SQLExpr: "BAD_EXPR"},
			{Key: dupKey, Sortable: true, SQLExpr: "good.expr"},
		},
		DefaultKey: dupKey,
		DefaultDir: admintable.Asc,
	}
	st := sp.Resolve(dupKey, "asc")
	got := sp.OrderBy(st)
	if strings.Contains(got, "BAD_EXPR") {
		t.Errorf("OrderBy picked non-sortable column expr: %q", got)
	}
	if got != "good.expr ASC" {
		t.Errorf("OrderBy: got %q; want %q", got, "good.expr ASC")
	}
}

// ---------------------------------------------------------------------------
// Spec.Valid() — startup-time Spec validation.
// ---------------------------------------------------------------------------

// TestSpecValid exercises Spec.Valid() with a table of valid and invalid Specs.
func TestSpecValid(t *testing.T) {
	cases := []struct {
		name    string
		spec    admintable.Spec
		wantErr bool
	}{
		{
			name: "valid_spec",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
					{Key: "stage", Sortable: true, SQLExpr: "i.stage"},
					{Key: "notes", Sortable: false, SQLExpr: "i.notes"},
				},
				DefaultKey: "name",
				DefaultDir: admintable.Asc,
			},
			wantErr: false,
		},
		{
			name: "zero_sortable_columns",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "notes", Sortable: false, SQLExpr: "i.notes"},
				},
				DefaultKey: "notes",
				DefaultDir: admintable.Asc,
			},
			wantErr: true,
		},
		{
			name: "default_key_not_sortable",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
					{Key: "notes", Sortable: false, SQLExpr: "i.notes"},
				},
				DefaultKey: "notes", // exists but not sortable
				DefaultDir: admintable.Asc,
			},
			wantErr: true,
		},
		{
			name: "default_key_missing",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
				},
				DefaultKey: "missing",
				DefaultDir: admintable.Asc,
			},
			wantErr: true,
		},
		{
			name: "duplicate_key",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
					{Key: "name", Sortable: true, SQLExpr: "p.name2"}, // dup
				},
				DefaultKey: "name",
				DefaultDir: admintable.Asc,
			},
			wantErr: true,
		},
		{
			name: "sortable_empty_sqlexpr",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					// Sortable but no SQLExpr → OrderBy would emit a bare " ASC".
					{Key: "name", Sortable: true, SQLExpr: ""},
				},
				DefaultKey: "name",
				DefaultDir: admintable.Asc,
			},
			wantErr: true,
		},
		{
			name:    "empty_spec",
			spec:    admintable.Spec{},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Valid()
			if tc.wantErr && err == nil {
				t.Errorf("Valid() = nil; want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Valid() = %v; want nil", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property test across multiple Spec shapes.
// ---------------------------------------------------------------------------

// TestOrderBy_PropertyInjectionInvariant_MultiSpec extends the base property
// test across multiple Spec shapes: well-formed, empty, bad DefaultKey,
// unset DefaultDir, and duplicate keys. For each, asserts that
// OrderBy(Resolve(hostile, hostile)):
//
//	(a) is non-empty,
//	(b) ends in exactly " ASC" or " DESC",
//	(c) contains none of the hostile URL bytes.
func TestOrderBy_PropertyInjectionInvariant_MultiSpec(t *testing.T) {
	hostile := []struct{ sort, dir string }{
		{"'; DROP TABLE users;--", "asc"},
		{"1 OR 1=1", "desc"},
		{"updated_at", "ASC"},
		{"notes", "asc"},
		{"stage", "; DELETE"},
		{"stage", "ASCENDING"},
		{"", ""},
		{"stage\x00", "asc"},
		{"person", "1; DROP"},
		{"kind", "   asc   "},
		{"<script>alert(1)</script>", "asc"},
		{"../../../etc/passwd", "desc"},
	}

	hostileBytes := []string{
		"DROP", "DELETE", "SELECT", "INSERT", "UPDATE", "OR 1=1",
		"--", ";", "<script>", "../", "\x00",
	}

	specs := []struct {
		name string
		spec admintable.Spec
	}{
		{
			name: "well_formed",
			spec: testSpec,
		},
		{
			name: "empty_spec",
			spec: admintable.Spec{},
		},
		{
			name: "bad_default_key",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
				},
				DefaultKey: "typo",
				DefaultDir: admintable.Asc,
			},
		},
		{
			name: "unset_default_dir",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "name", Sortable: true, SQLExpr: "p.name"},
				},
				DefaultKey: "name",
				DefaultDir: "", // zero value
			},
		},
		{
			name: "duplicate_key_one_sortable",
			spec: admintable.Spec{
				Columns: []admintable.Column{
					{Key: "dup", Sortable: false, SQLExpr: "BAD_EXPR"},
					{Key: "dup", Sortable: true, SQLExpr: "good.expr"},
				},
				DefaultKey: "dup",
				DefaultDir: admintable.Asc,
			},
		},
	}

	for _, sp := range specs {
		sp := sp
		t.Run(sp.name, func(t *testing.T) {
			// Build allowed set for valid specs.
			var allowed map[string]bool
			if sp.spec.Valid() == nil {
				allowed = make(map[string]bool)
				for _, col := range sp.spec.Columns {
					if col.Sortable {
						allowed[col.SQLExpr+" ASC"] = true
						allowed[col.SQLExpr+" DESC"] = true
					}
				}
			}

			for _, tc := range hostile {
				st := sp.spec.Resolve(tc.sort, tc.dir)
				result := sp.spec.OrderBy(st)

				// (a) non-empty
				if result == "" {
					t.Errorf("[%s] Resolve(%q,%q) → OrderBy = empty string", sp.name, tc.sort, tc.dir)
				}
				// (b) ends in " ASC" or " DESC"
				if !strings.HasSuffix(result, " ASC") && !strings.HasSuffix(result, " DESC") {
					t.Errorf("[%s] Resolve(%q,%q) → OrderBy = %q; does not end with ' ASC' or ' DESC'",
						sp.name, tc.sort, tc.dir, result)
				}
				// (c) no hostile bytes in output
				for _, hb := range hostileBytes {
					if strings.Contains(result, hb) {
						t.Errorf("[%s] OrderBy(%q,%q) = %q; contains hostile byte %q",
							sp.name, tc.sort, tc.dir, result, hb)
					}
				}
				// For valid specs: must be in allowed set.
				if allowed != nil && !allowed[result] {
					t.Errorf("[%s] OrderBy(Resolve(%q,%q)) = %q; not in allowed set %v",
						sp.name, tc.sort, tc.dir, result, allowedKeys(allowed))
				}
			}
		})
	}
}

// allowedKeys returns the allowed set as a slice for error messages.
func allowedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
