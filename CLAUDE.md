# CLAUDE.md

Only release-please assigns versions — manual `git tag`/`gh release create` is forbidden.

Notes for coding agents working in go-kit, a single Go module (`github.com/anatolykoptev/go-kit`)
of small shared packages for the go-* services. Every document is indexed in
[docs/INDEX.md](docs/INDEX.md).

## Layout

One directory per package at the repository root (`env`, `llm`, `cache`, `retry`, `metrics`,
`httputil`, `telegram/...`, `render/...`, `voice/...` and so on). `doc/<pkg>.md` is the per-package
guide, `docs/` holds architecture, design and roadmap, `scripts/` holds the CI helpers
(`test-db.sh`, `install-gostall.sh`).

## Gate

`make preflight` is what CI runs: gostall (lock-order, missing-unlock, starvation), `gofmt -l`,
`go vet ./...`, `go build ./...`, then `make test-db`. `make lint` (golangci-lint, config in
`.golangci.yml`) is run locally; it is commented out in CI until golangci-lint supports the
module's Go version. Always run with `GOWORK=off`, as the Makefile does.

A docs-only PR (Markdown, `docs/`, `doc/`, licence and templates) skips the heavy `preflight` job
through the `docs-scope` job in `.github/workflows/preflight.yml`. That block is canonical and is
compared byte for byte by the fleet tooling, so edit it nowhere but at the source.

## Invariants that are easy to break

- **Skip-to-green is forbidden.** The Postgres-backed tests (`telegram/fsm`, `pgutil`) `t.Skip`
  when `TEST_DATABASE_URL` is unset; `make test-db` provisions a throwaway Postgres 16 so they
  really run. A test that needs a dependency gets the dependency provisioned, not skipped. The
  same applies to `render/typst` (pandoc, typst, IBM Plex fonts) and `voice` (ffmpeg).
- **CI runs with `CGO_ENABLED=0`.** `embed/onnx` needs `libtokenizers` and `libonnxruntime`; without
  cgo it compiles to a stub returning `ErrNoCGO`. The race detector needs cgo, so the Makefile
  drops `-race` when cgo is off.
- **Keep dependencies out of the small packages.** `env`, `retry`, `metrics`, `strutil`, `ratelimit`
  and friends are stdlib-only; a package that needs a heavy dependency belongs behind its own
  import path so consumers who do not import it do not pay for it.
- **Security-sensitive helpers fail closed.** `httputil` (SSRF guard, trusted-proxy client IP),
  `svcauth` (credential sent only to its intended destination) and `telegram/tgsafe` (bot-token
  scrubbing) must not gain a fallback that sends or logs more than intended.
- **Do not hand-edit `CHANGELOG.md`, `.release-please-manifest.json` or version numbers.**
  release-please writes them from Conventional Commit messages (`feat`, `fix`, `perf`, `refactor`,
  `docs`, `deps`, `revert`; `chore`, `ci`, `test`, `build`, `style` are hidden from the changelog).
- **This repository is public.** No hostnames, operator paths, account names, tokens or personal
  data in any tracked file, tests and fixtures included; the repo-standards check scans for them.
