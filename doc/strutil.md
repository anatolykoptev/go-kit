# strutil

Unicode-correct string helpers: rune-based truncation (including word-aware and
middle), case conversions for naming, UTF-8 scrub, word wrap, language sniff,
and small membership-test convenience funcs.

```
go get github.com/anatolykoptev/go-kit/strutil
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/strutil"

strutil.Truncate("héllo, world", 5)        // "héllo…"
strutil.TruncateAtWord("héllo, world", 8)   // "héllo,…"
strutil.TruncateMiddle("path/to/file.go", 8) // "pa…le.go"
strutil.ToSnakeCase("HTTPClient")           // "http_client"
strutil.WordWrap("longish text", 6)         // "longish\ntext"
```

Every truncate helper counts **runes**, not bytes — `"é"` is one position, not
two. The default placeholder is `"…"` (one rune).

## Truncation: rune-based, three variants

| Function | Behaviour | When to use |
|----------|-----------|-------------|
| `Truncate(s, max)` | Hard cut at `max` runes + placeholder | Default; titles, list rows |
| `TruncateAtWord(s, max)` | Cut at the previous word boundary | Body text; avoids "lorem…" mid-word |
| `TruncateMiddle(s, max)` | Keep both ends, replace middle with placeholder | File paths, IDs ("a1b2…d4e5") |

Each has a `*With(s, max, placeholder)` form so you can override the suffix:

```go
strutil.TruncateWith(s, 80, "[...]")
strutil.TruncateMiddleWith(longID, 10, "…")
```

A negative or zero `max` returns the original string unchanged.

## Case conversions

```go
strutil.ToSnakeCase("HTTPClient")     // "http_client"
strutil.ToKebabCase("HTTPClient")     // "http-client"
strutil.ToCamelCase("http_client_v2") // "httpClientV2"
strutil.ToPascalCase("http_client")   // "HttpClient"
```

Handles run-on acronyms (`HTTPClient` → `http_client`, not `h_t_t_p_client`)
and digit boundaries (`Client2` → `client_2`).

## UTF-8 hygiene

`Scrub(s)` replaces invalid UTF-8 sequences with U+FFFD (the standard
replacement character). Cheap to call before any operation that assumes valid
UTF-8 (regex, json.Marshal of a string field, hashing).

```go
clean := strutil.Scrub(userInput) // never returns invalid UTF-8
```

## Word wrap

`WordWrap(s, width)` inserts `\n` at word boundaries so no output line exceeds
`width` runes (when achievable — words longer than `width` are kept on their
own line).

```go
fmt.Println(strutil.WordWrap("This is a longish sentence.", 12))
// This is a
// longish
// sentence.
```

## Membership tests

| Function | Notes |
|----------|-------|
| `Contains(items, s)` | Exact match against a slice |
| `ContainsAny(s, substrs)` | True if `s` contains any element |
| `ContainsAll(s, substrs)` | True if `s` contains every element |

All three are short-circuiting and case-sensitive.

## Language detection

`DetectLang(text)` returns a coarse `Lang` value (English / Russian / Chinese /
Spanish) based on Unicode script counts. Use as a routing hint for prompt
templating or per-language pipelines.

```go
switch strutil.DetectLang(input) {
case strutil.LangRU:
    // route to Russian prompt
}
```

Not a full-fledged language ID — for high-stakes routing pair with a real
classifier (langid, fasttext). This is for cheap "should I append a Russian
system prompt?" decisions.

## API reference

| Symbol | Notes |
|--------|-------|
| `Truncate`, `TruncateWith` | Hard rune-based cut |
| `TruncateAtWord`, `TruncateAtWordWith` | Word-aware cut |
| `TruncateMiddle`, `TruncateMiddleWith` | Middle-ellipsis |
| `ToSnakeCase`, `ToKebabCase`, `ToCamelCase`, `ToPascalCase` | Case conversion |
| `Scrub` | Replace invalid UTF-8 |
| `WordWrap` | Wrap at word boundaries |
| `Contains`, `ContainsAny`, `ContainsAll` | Membership tests |
| `Lang`, `DetectLang` | Coarse script-based language sniff |

## Notes

- No allocations on the common "no truncation needed" path — checked by the
  package benchmarks.
- All helpers are pure functions and safe for concurrent use.
- Display-width truncation (CJK full-width, grapheme clusters) is deferred —
  the rune-count truncation is correct for ~all Western/Cyrillic content. File
  a bug if you hit a case where rune count under-counts a display column.
