# skills

Loader for `SKILL.md` files in the [agentskills.io](https://agentskills.io)
open standard: YAML/JSON frontmatter parsing, spec-conformant name
validation, tiered skill catalogs over filesystem dirs / `embed.FS` /
plugin entries, locale routing, and nil-safe observer/tracer hooks.
Standard library only — no external dependencies.

```
go get github.com/anatolykoptev/go-kit/skills
```

## Two APIs

- **`Embedded`** — one skill baked into a binary via `//go:embed`, with an
  optional env-path override for hot-reload during prompt iteration. For
  services that ship exactly one skill.
- **`Catalog`** — multi-skill discovery across tiered sources searched in
  priority order (first match wins). For agent CLIs that load many skills
  and inject summaries into a system prompt.

Both share the primitives `StripFrontmatter`, `ParseFrontmatter`,
`ParseMetadata`, and `ValidateName`.

## Embedded — single skill

```go
import (
    _ "embed"

    "github.com/anatolykoptev/go-kit/skills"
)

//go:embed skills/answer-extractor/SKILL.md
var rawSkill string

// Panics at init if the name is invalid or the embedded body is empty —
// configuration errors surface at binary startup, not at request time.
var skill = skills.NewEmbedded(
    "answer-extractor",       // skill name (spec: matches its dir name)
    "MYSERVICE_SKILL_PATH",   // env var: file path override (optional)
    rawSkill,
)

func systemPrompt() string {
    return skill.Body() // always non-empty; hot-reloads from env path if set
}
```

`Body()` resolution per call:

1. Env var set + path readable + file ≤ 1 MiB + body non-empty after
   frontmatter strip → mtime-cached body.
2. Env path transiently unreadable but a previous read populated the
   cache → last-known-good cached body (a stat blip does not revert the
   binary to the embedded default mid-edit).
3. Otherwise → embedded default, parsed once at construction.

Other methods: `Metadata()` returns frontmatter parsed from the embedded
raw (stable across hot-reloads — never read from the env file);
`Diagnostic()` re-stats the env path and returns a human-readable state
line for startup logs; `BodyCtx(ctx)` is the context-aware variant used
by the Tracer hook; `ResetCache()` clears the mtime cache (tests only).

## Catalog — tiered multi-skill discovery

```go
cat := skills.NewCatalog(
    skills.NewDirTier("workspace", workspaceDir), // user-editable, wins
    skills.NewDirTier("builtin",   builtinDir),   // shipped with agent
    skills.NewPluginTier("plugin", pluginEntries),
    skills.NewEmbedFSTier("embed", embeddedSkills, "skills"),
)

body, info, ok := cat.Load("doc-review")
// info.Source — which tier matched ("workspace", "builtin", "plugin:<name>")
// info.Dir    — skill dir; use it to resolve siblings (references/, scripts/)

// Concatenate bodies for a system prompt:
// "### Skill: <name>\n\n<body>" per skill, joined with "\n\n---\n\n".
block := cat.LoadMany([]string{"doc-review", "translate"})

// Compact catalog summary for the top of a system prompt.
summary := cat.BuildSummary(skills.SummaryXML) // or SummaryMarkdown, SummaryJSON

all := cat.List() // unique skills across tiers, sorted by name
```

`LoadCtx` / `LoadManyCtx` are the context-aware variants; a tier whose
resolver implements `ContextResolver` (`FindCtx`) gets the caller's
context — extension point for network-backed sources.

### Tier constructors

| Constructor | Source | Notes |
|---|---|---|
| `NewDirTier(name, dir, opts...)` | `<dir>/<skill-name>/SKILL.md`, one level deep | Options: `WithSkillFilename("AGENT.md")`, `WithMtimeCache()`. Symlink skill dirs skipped; symlink SKILL.md files must not escape the tier root. |
| `NewEmbedFSTier(name, fsys, root, opts...)` | `embed.FS` rooted at `<root>/<skill>/SKILL.md` | Parsed once at construction. `SkillInfo.Dir` is a forward-slash FS path — read siblings via `fsys.ReadFile`, not `os.Open`. Option: `WithEmbedSkillFilename`. |
| `NewPluginTier(name, []PluginEntry)` | Pre-resolved entries from a plugin layer | `Body` wins over `Path` (no disk read). Lookups work by bare name and `plugin:skill` namespaced name; `SkillInfo.Source` becomes `"<tier>:<plugin>"`. Body-only entries have empty `Dir`. |

## Locale routing

`cat.WithLocale("ru")` adds an opt-in locale preference: skills with
`locale:` frontmatter matching the tag (case-insensitive) win inside each
tier, then locale-neutral skills (`locale` unset), then any name match.
Tier priority always wins over locale — a tier-1 neutral variant beats a
tier-2 exact match. Variants live in differently-named subdirs but share
one frontmatter `name:`; matching is on `Metadata.Name` first.

```go
cat := skills.NewCatalog(tiers...).WithLocale("ru")
body, info, ok := cat.Load("greeter") // prefers the locale: ru variant
```

`cat.Locale()` returns the configured tag ("" when unset). `WithLocale("")`
is a no-op; it does not reset a previously configured locale. The tag is
matched by exact string equality — the package does not validate BCP-47
format or normalize hyphen/underscore.

## Observer and Tracer hooks

Both APIs accept optional hooks into the same hot paths, wired to whatever
metrics/tracing backend the service already uses. All hooks are nil-safe.

```go
obs := &skills.Observer{
    BodyCall:    func(name, source string) { bodyCalls.WithLabelValues(name, source).Inc() },
    EnvFallback: func(name, reason string) { fallbacks.WithLabelValues(name, reason).Inc() },
    BodyBytes:   func(name string, n int)   { bodyBytes.WithLabelValues(name).Observe(float64(n)) },
    CatalogLoad: func(name, outcome string) { loads.WithLabelValues(name, outcome).Inc() },
    CatalogSize: func(tier string, n int)   { tierSize.WithLabelValues(tier).Set(float64(n)) },
}

e := skills.NewEmbedded(name, envVar, raw, skills.WithObserver(obs))
cat := skills.NewCatalog(tiers...).WithObserver(obs).WithTracer(tr)
```

| Hook | Fires on | Label values |
|---|---|---|
| `BodyCall` | every `Embedded.Body()` | source: `embedded`, `env`, `cache_hit`, `last_known_good` |
| `EnvFallback` | env-override rejection | reason: `unreadable`, `too_large`, `empty_body` |
| `BodyBytes` | every `Body()` | byte length of returned body |
| `CatalogLoad` | every `Load`/`LoadCtx` | outcome: `hit`, `miss` |
| `CatalogSize` | once per tier on `WithObserver` attach | skill count the tier walks |

`Tracer` mirrors this for spans: `StartBody` wraps `Embedded.BodyCtx`,
`StartCatalogLoad` wraps `Catalog.LoadCtx`, each returning a derived
context plus an end-fn called with the same source/outcome label. The
end-fn **must** be non-nil (the runtime guards the field, not the return
value). See the `Tracer` doc comment for a 5-line OpenTelemetry adapter.

Attach hooks once at startup — `WithObserver`/`WithTracer`/`WithLocale`
mutate the catalog and are not safe after concurrent `Load` callers start.

## Frontmatter rules

Two formats, both requiring a blank line between frontmatter and body:

```markdown
---
name: d10-extractor
description: Extracts the ten most important atomic facts from a document.
license: MIT
when_to_use: Use when summarizing dense technical documents.
allowed-tools: [Read, Bash]
locale: en
tags: [extraction, summarization]
metadata:
  effort: low
---

# D10 Extractor

Body text...
```

or a JSON object on the first line followed by a blank line (`{...}\n\nbody`).
`allowed-tools` also accepts a space-separated string (`Read Bash`);
`tags` also accepts comma-separated (`extraction, summarization`).

| Field | Key | Type | Notes |
|---|---|---|---|
| `Name` | `name` | string | required by spec |
| `Description` | `description` | string | required by spec; debug-logged past 1024 chars |
| `License` | `license` | string | SPDX id |
| `Compatibility` | `compatibility` | string | free-form |
| `WhenToUse` | `when_to_use` | string | Claude Code discovery hint |
| `AllowedTools` | `allowed-tools` | []string | flow list or space-separated |
| `DisableModelInvocation` | `disable-model-invocation` | bool | skip LLM call |
| `UserInvocable` | `user-invocable` | *bool | nil = spec default (true) |
| `Version` | `version` | string | extension, not in spec |
| `Locale` | `locale` | string | extension; routed by `WithLocale` |
| `Tags` | `tags` | []string | extension |
| `Extra` | unknown top-level keys + `metadata:` map | map[string]string | catch-all — new spec fields land here without a package update |

The YAML parser is intentionally minimal: top-level scalars, quoted
strings, flow lists, comments; `metadata:` sub-keys flatten into `Extra`.
No nested maps, anchors, or multi-docs — consumers needing full YAML can
call `ParseFrontmatter` and run a real YAML library on the raw block.
Malformed frontmatter is never fatal: the body is always returned.

## Name validation

`ValidateName(name, dirName)` enforces the spec rules: 1–64 chars,
lowercase ASCII alphanumerics and hyphens, no leading/trailing/consecutive
hyphens, and the on-disk directory name must equal `name` (pass `dirName
=""` to skip that check). `NewEmbedded` validates eagerly and panics.
`DirTier` uses the directory name as the skill name without
re-validating — spec conformance is enforced at authoring time.

## Hot-reload mechanics

`Embedded.Body()` and `DirTier` with `WithMtimeCache` share an mtime-poll
strategy: one `os.Stat` per call, re-read only when `ModTime` changes.
No fsnotify — inotify does not fire on NFS/SMB/several Docker bind-mount
configurations, and one stat is negligible next to the LLM call that
follows. `Embedded` is safe for concurrent use; a per-instance mutex
covers stat+read+cache-update so concurrent callers never see a torn read.

## InitWorkspace

```go
skills.InitWorkspace(workspaceDir, map[string][]byte{
    "SOUL.md":                 defaultSoul,
    "skills/foo/SKILL.md":     defaultSkill, // nested keys supported
})
```

Creates the dir at mode `0750`, writes missing files at `0600`, never
overwrites. Errors are logged, not returned — best-effort first-run UX.

## What this package does NOT do

- Skill installation, publishing, or a registry — separate concerns.
- Skill execution (running `scripts/` inside a skill dir) — the caller's job.
- Full YAML — see the frontmatter section above.
- Locale detection — the caller passes the tag to `WithLocale`.
