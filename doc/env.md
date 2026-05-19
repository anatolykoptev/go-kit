# env

Functional environment-variable reader: zero reflection, scalar + collection
getters, `*E` variants that surface errors, `Must*` panickers, pluggable
`Source` for parallel-safe tests, secret-file resolver, `${VAR}` expansion.

```
go get github.com/anatolykoptev/go-kit/env
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/env"

port    := env.Int("PORT", 8080)             // silent: returns default on parse error
tags    := env.List("TAGS", "a,b,c")          // comma-separated -> []string
timeout := env.Duration("HTTP_TIMEOUT", 3*time.Second)

// Surface parse errors:
n, err := env.IntE("WORKERS", 4)
if err != nil {
    return err
}

// Panic on missing required:
key := env.MustRequired("API_KEY")
```

## Three call shapes

| Suffix | Behaviour on parse error / missing |
|--------|------------------------------------|
| _none_ (`Int`, `Duration`, `Bool`, …) | Silently fall back to default, no log |
| `*E` (`IntE`, `DurationE`, `URLE`, …) | Return `(*ParseError)` or `(*NotSetError)`; default still returned alongside |
| `Must*` (`MustInt`, `MustRequired`, `MustURL`, …) | `panic` on parse error; default still respected for missing |

Pick one shape per service and stick with it. Mixing leads to silent
mis-configurations because some bad values become defaults and others crash.

## Scalars

`Str`, `Int`, `Int64`, `Float`, `Uint`, `Uint64`, `Bool`, `Duration`, `URL` —
plus their `*E` and `Must*` variants.

```go
url, err := env.URLE("DATABASE_URL", "postgres://localhost/dev")
```

## Collections

| Function | Format |
|----------|--------|
| `List(key, def)` | `"a,b,c"` → `[]string{"a","b","c"}` |
| `Int64List(key)` | `"1,2,3"` → `[]int64{1,2,3}` (no default) |
| `Map(key, def)` | `"a=1,b=2"` → `map[string]string{"a":"1","b":"2"}` |

## Secret files — Docker / k8s style

`File(key, def)` reads a path from env and returns the file's content. Mirrors
the caarlos0 `envconfig` `file:""` tag pattern for `/run/secrets/*` mounts.

```go
secret := env.File("OPENAI_API_KEY_FILE", "/run/secrets/openai")
```

`FileE` surfaces "file not found" as `*os.PathError`. Both helpers trim
trailing whitespace.

## ${VAR} expansion

`Expand` resolves shell-style `${VAR}` references in the value, using the
current `Source` for the inner lookups.

```go
// CONNSTR="postgres://${PGUSER}:${PGPASS}@localhost/${PGDB}"
cs := env.Expand("CONNSTR", "")
```

## Binary blobs

| Function | Encoding |
|----------|----------|
| `Base64`, `Base64E` | std encoding |
| `Hex`, `HexE` | lowercase hex |

## Source interface — parallel-safe tests

Production reads from `DefaultSource` (which calls `os.LookupEnv`). In tests
that run in parallel, swap to a `MapSource`:

```go
env.DefaultSource = env.MapSource(map[string]string{
    "PORT":      "1234",
    "DB_URL":    "postgres://test",
})
defer func() { env.DefaultSource = osSourceDefault }() // restore via your own helper

n := env.Int("PORT", 0) // -> 1234
```

`Source` is `interface { Lookup(key string) (string, bool) }` — implement it
yourself for layered configs (env over file over baked-in defaults).

## Errors

| Type | Returned by | When |
|------|-------------|------|
| `*NotSetError` | `*E` getters | Key is absent AND no default was supplied (e.g. `Required`, `Int64List`) |
| `*ParseError` | `*E` getters | Value is set but malformed; default is returned alongside |

`Unwrap()` on `ParseError` exposes the underlying `strconv` / `url.Parse`
error for `errors.Is` checks.

## API reference (subset)

| Function | Notes |
|----------|-------|
| `Lookup(key) (string, bool)` | Direct passthrough to current `Source` |
| `Exists(key) bool` | True if the key is set (even to `""`) |
| `Required(key) (string, error)` | Empty value treated as missing |
| `Str/Int/Bool/Duration/URL(key, def)` | Silent defaults |
| `*E` variants | Return errors |
| `Must*` variants | Panic on parse error |
| `List`, `Int64List`, `Map` | Collections |
| `File`, `FileE` | Read content from path in env |
| `Expand` | Resolve `${VAR}` in the value |
| `Base64`, `Hex` (+ `*E`) | Binary decoders |

## Notes

- All getters that take a `def` return that default on missing **and** on parse
  failure. Use `*E` if you need to distinguish.
- `Bool` accepts `true/false/1/0/yes/no/on/off` (case-insensitive); anything
  else is a parse error.
- The package does NOT parse struct tags. If you want a single struct to
  describe your config, build it from these functions — it's ~10 lines and
  gives you typed defaults + per-field error handling without reflection.
