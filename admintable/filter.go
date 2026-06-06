package admintable

import (
	"fmt"
	"net/url"
	"strings"
)

// Match selects the SQL predicate shape for a [Filter].
type Match int

const (
	// Eq emits an exact-equality predicate: "col = $N".
	// The bind arg is the string returned by [url.Values.Get](key).
	Eq Match = iota

	// AnyOf emits a set-membership predicate: "col = ANY($N::text[])".
	// The bind arg is a []string of all values for the key (url.Values[key]).
	//
	// pgx encodes a Go []string as a Postgres text[], which is what the
	// ::text[] cast expects.  Do not use AnyOf with drivers that do not
	// handle []string → text[] encoding.
	//
	// Empty-string elements inside the slice are NOT stripped — they become
	// bound array elements (harmless: they just won't match a row). Only an
	// entirely-empty slice (or one where Allowed rejects every value) skips
	// the filter.
	AnyOf
)

// Filter is a single vetted WHERE condition.
//
// SQLExpr is the ONLY column-side expression that ever reaches SQL — it is an
// AUTHOR-DECLARED compile-time constant.  URL parameter values go exclusively
// into bind args ($N); they are NEVER interpolated into the SQL string.
//
// IMPORTANT: SQLExpr must be an author-supplied constant, never derived from
// request data.  The security guarantee of the package depends on it.
type Filter struct {
	// Key is the request-parameter name, e.g. "status" or "plan".
	// A URL parameter whose key is not declared in the [FilterSpec] is silently ignored.
	Key string

	// SQLExpr is the author-constant column expression used in the WHERE predicate,
	// e.g. "subscription_status" or "u.plan_id".
	// Never derive this from user input.
	SQLExpr string

	// Match selects the predicate shape: [Eq] or [AnyOf].
	Match Match

	// Allowed is an optional closed set of accepted values.
	// When non-empty, a request value not present in Allowed is treated as if
	// the parameter were absent (the filter is skipped — NOT an error).
	// This is the safe-degrade convention: unknown value → no filter applied.
	// When Allowed is empty every non-empty value is accepted.
	Allowed []string
}

// FilterSpec is the declarative WHERE-filter contract for one admin list page.
//
// It mirrors [Spec] for sort: each [Filter] declares a single SQL condition;
// [FilterSpec.Valid] validates at startup; [FilterSpec.Where] resolves URL
// values into a safe, parameterised WHERE fragment at request time.
//
// Callers should invoke [FilterSpec.Valid] once at package-init or startup to
// detect misconfigured specs at boot rather than at query time.
type FilterSpec struct {
	Filters []Filter
}

// Valid reports whether the FilterSpec is correctly configured. It returns an
// error if:
//   - any two Filters share the same Key (duplicate keys cause ambiguous resolution),
//   - a Filter has an empty SQLExpr (Where would emit a broken predicate),
//   - a Filter has a Match value outside the declared set ([Eq] or [AnyOf]).
//
// Intended to be called once at startup (e.g. in an init() function or a
// package-level var) to catch programmer errors early.
func (fs FilterSpec) Valid() error {
	seen := make(map[string]bool, len(fs.Filters))
	for i, f := range fs.Filters {
		if seen[f.Key] {
			return fmt.Errorf("admintable.FilterSpec: duplicate Filter Key %q", f.Key)
		}
		seen[f.Key] = true

		if f.SQLExpr == "" {
			return fmt.Errorf("admintable.FilterSpec: Filter[%d] Key %q has empty SQLExpr", i, f.Key)
		}

		if f.Match != Eq && f.Match != AnyOf {
			return fmt.Errorf("admintable.FilterSpec: Filter[%d] Key %q has unknown Match value %d", i, f.Key, f.Match)
		}
	}
	return nil
}

// Where builds the AND-joined WHERE conditions from the given URL values.
//
// startArg is the next available bind-parameter index (e.g. 1 for a query with
// no prior args, or 3 if $1 and $2 are already used for pagination).
// Placeholders ($N) are numbered sequentially from startArg, in Filter
// declaration order, ONLY for active filters — skipped filters consume no index.
// args aligns 1:1 with the placeholders emitted in conds.
//
// A filter is skipped when:
//   - [Eq]: vals.Get(key) is empty.
//   - [AnyOf]: vals[key] is empty or nil.
//   - The resolved value(s) are not in Filter.Allowed (when Allowed is non-empty).
//
// Returns ("", nil) when no filters are active.
// The returned conds string contains ONLY [Filter.SQLExpr] values (author
// constants), the literal operators "= $N" / "= ANY($N::text[])", and the
// literal conjunctive " AND ".  No URL value bytes ever appear in conds.
//
// The returned conds does NOT include a leading WHERE or AND keyword — the
// caller is responsible for composing those:
//
//	conds, args := filterSpec.Where(r.URL.Query(), 1)
//	if conds != "" {
//	    query += " WHERE " + conds
//	}
//
//	// When combining with existing WHERE conditions:
//	conds, args := filterSpec.Where(r.URL.Query(), 3) // $1,$2 already used
//	if conds != "" {
//	    query += " AND " + conds
//	}
//
// Callers that pass conds to fmt.Sprintf should annotate:
//
//	//nolint:gosec // only FilterSpec-owned SQLExpr + literal operators + $N
//	// placeholders reach SQL; URL values are bind args, never interpolated.
func (fs FilterSpec) Where(vals url.Values, startArg int) (conds string, args []any) {
	n := startArg
	var parts []string

	for _, f := range fs.Filters {
		switch f.Match {
		case Eq:
			v := vals.Get(f.Key)
			if v == "" {
				continue
			}
			if !f.allowed(v) {
				continue
			}
			//nolint:gosec // only author-declared SQLExpr + literal "= $N" reach SQL; v is a bind arg.
			parts = append(parts, fmt.Sprintf("%s = $%d", f.SQLExpr, n))
			args = append(args, v)
			n++

		case AnyOf:
			vs := vals[f.Key]
			if len(vs) == 0 {
				continue
			}
			filtered := f.allowedSlice(vs)
			if len(filtered) == 0 {
				continue
			}
			//nolint:gosec // only author-declared SQLExpr + literal "= ANY($N::text[])" reach SQL; filtered is a bind arg.
			parts = append(parts, fmt.Sprintf("%s = ANY($%d::text[])", f.SQLExpr, n))
			args = append(args, filtered)
			n++
		}
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " AND "), args
}

// allowed reports whether v is an acceptable value for f.
// When f.Allowed is empty every non-empty string is accepted.
func (f Filter) allowed(v string) bool {
	if len(f.Allowed) == 0 {
		return true
	}
	for _, a := range f.Allowed {
		if a == v {
			return true
		}
	}
	return false
}

// allowedSlice returns the subset of vs whose elements are in f.Allowed.
// When f.Allowed is empty every element passes.
func (f Filter) allowedSlice(vs []string) []string {
	if len(f.Allowed) == 0 {
		return vs
	}
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		if f.allowed(v) {
			out = append(out, v)
		}
	}
	return out
}
