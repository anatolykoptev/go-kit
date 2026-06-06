# admintable

SQL-injection-safe declarative sortable-table resolver for server-rendered admin pages.

Ported from `go-nerv internal/admin/table` — first go-kit/go-panel admin primitive.

## Security model

URL sort and dir parameters are **equality-matched** against a closed, author-declared
set of column keys. They are **never interpolated** into SQL. The only bytes that
ever reach an `ORDER BY` clause are:

- `Column.SQLExpr` — author-declared compile-time constant
- `Column.TieBreakSQLExpr` — author-declared compile-time constant (optional)
- The literal strings `"ASC"`, `"DESC"`, and `" NULLS LAST"`

## Usage

```go
var tableSpec = admintable.Spec{
    Columns: []admintable.Column{
        {Key: "name",    Label: "Name",    Sortable: true,  SQLExpr: "u.name"},
        {Key: "updated", Label: "Updated", Sortable: true,  SQLExpr: "u.updated_at", NullsLast: true},
        {Key: "notes",   Label: "Notes",   Sortable: false},
    },
    DefaultKey: "updated",
    DefaultDir: admintable.Desc,
}

func init() {
    if err := tableSpec.Valid(); err != nil {
        panic(err) // misconfigured Spec — catch at startup, not at query time
    }
}

func handleList(w http.ResponseWriter, r *http.Request) {
    st := tableSpec.Resolve(
        r.URL.Query().Get("sort"),
        r.URL.Query().Get("dir"),
    )
    //nolint:gosec // only Spec-owned SQLExpr + literal "ASC"/"DESC" reach SQL
    query := fmt.Sprintf("SELECT ... ORDER BY %s LIMIT $1 OFFSET $2", tableSpec.OrderBy(st))
    // st.Key and st.Dir are safe to pass to templates for active-column indicators
}
```

## API

| Type / Function | Purpose |
|---|---|
| `Column` | Declarative column definition (Key, Label, Sortable, SQLExpr, NullsLast, TieBreakSQLExpr, Width, Align) |
| `Dir` (`Asc` / `Desc`) | Sort direction constants |
| `Spec` | Table contract: Columns + DefaultKey + DefaultDir |
| `Spec.Valid() error` | Startup validation: no sortable cols / bad DefaultKey / duplicate keys |
| `Spec.Resolve(sort, dir string) State` | Parse URL params safely; always returns a valid State |
| `Spec.OrderBy(State) string` | Build the ORDER BY fragment; only author-declared bytes reach SQL |
| `State` | Resolved sort: Key (validated column key) + Dir (Asc or Desc) |

## NullsLast

Set `Column.NullsLast: true` for nullable date/time columns. `OrderBy` emits the
direction keyword **before** `NULLS LAST`, which is the only valid Postgres syntax:

```
"i.updated_at DESC NULLS LAST"   -- correct
"i.updated_at NULLS LAST DESC"   -- SQLSTATE 42601 syntax error
```

## TieBreakSQLExpr

Set `Column.TieBreakSQLExpr` to a verbatim secondary sort term (author-constant,
already includes its own direction keyword) for stable pagination on low-cardinality
columns:

```go
{Key: "score", Sortable: true, SQLExpr: "fit_score", NullsLast: true,
 TieBreakSQLExpr: "last_seen_at DESC"}
// → "fit_score DESC NULLS LAST, last_seen_at DESC"
```

## No external dependencies

Pure stdlib: `errors`, `fmt`, `strings`.
