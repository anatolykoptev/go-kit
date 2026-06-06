// Package admintable provides a SQL-injection-safe declarative sortable-table
// resolver for server-rendered admin pages.
//
// # Security model
//
// Only [Column.SQLExpr] and [Column.TieBreakSQLExpr] values — author-declared
// compile-time constants — ever reach an ORDER BY clause. URL sort and dir
// parameters are equality-matched against a closed set of declared keys; they
// are NEVER interpolated into SQL. Callers that pass [Spec.OrderBy] output to
// fmt.Sprintf should annotate the call site with:
//
//	//nolint:gosec // only Spec-owned SQLExpr + literal "ASC"/"DESC" + optional
//	// literal " NULLS LAST" reach SQL; URL params are equality-matched against
//	// a closed set, never interpolated.
//
// # Startup validation
//
// Each [Spec] should call [Spec.Valid] at package-init or program startup to
// catch misconfiguration early — it returns a non-nil error for zero sortable
// columns, a [Spec.DefaultKey] that does not name a Sortable column, and
// duplicate column keys.
//
// # Typical usage
//
//	var mySpec = admintable.Spec{
//	    Columns: []admintable.Column{
//	        {Key: "name",    Label: "Name",    Sortable: true,  SQLExpr: "u.name"},
//	        {Key: "updated", Label: "Updated", Sortable: true,  SQLExpr: "u.updated_at", NullsLast: true},
//	        {Key: "notes",   Label: "Notes",   Sortable: false},
//	    },
//	    DefaultKey: "updated",
//	    DefaultDir: admintable.Desc,
//	}
//
//	func init() {
//	    if err := mySpec.Valid(); err != nil {
//	        panic(err)
//	    }
//	}
//
//	func handleList(w http.ResponseWriter, r *http.Request) {
//	    sort := r.URL.Query().Get("sort")
//	    dir  := r.URL.Query().Get("dir")
//	    st   := mySpec.Resolve(sort, dir) // always safe; falls back to defaults
//
//	    //nolint:gosec // only Spec-owned SQLExpr + literal "ASC"/"DESC" reach SQL
//	    query := fmt.Sprintf("SELECT ... ORDER BY %s", mySpec.OrderBy(st))
//	    // ...
//	}
package admintable
