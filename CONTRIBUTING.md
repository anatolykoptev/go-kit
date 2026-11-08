# Contributing to go-kit

go-kit is a Go library of small shared packages. Issues and pull requests are welcome.

## Build and test

You need Go 1.27.3 or newer (see `go.mod`). Docker is needed only for the database tests.

```sh
GOWORK=off go build ./...
make test         # go test ./...; Postgres-backed tests skip without a database
make test-db      # the full suite against a throwaway Postgres 16 started in Docker
make lint         # golangci-lint, config in .golangci.yml
```

## The gate

`make preflight` is the merge gate that CI runs. It builds gostall (`scripts/install-gostall.sh`),
then checks `gofmt -l`, `go vet ./...`, `go build ./...` and runs `make test-db`. Run it before you
open a pull request. A pull request that only changes documents (Markdown, `docs/`, `doc/`, licence,
issue and pull-request templates) skips the heavy job automatically.

## Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), because
release-please builds the changelog and picks the next version from them:

- `feat(pkg): ...` new behaviour; `fix(pkg): ...` a bug fix; `perf`, `refactor`, `docs`, `deps` and
  `revert` appear in the changelog.
- `chore`, `ci`, `test`, `build` and `style` are hidden from it.
- A breaking change is marked with `!` (`feat(pkg)!: ...`) or a `BREAKING CHANGE:` footer.

Use the package directory as the scope. Do not create tags or releases by hand and do not edit
`CHANGELOG.md`; release-please opens a release pull request when changes land on `main`.

## Pull requests

- Keep a pull request to one change. Say what changed, why, and how you tested it; the template asks.
- Add or update tests for behaviour changes. A test must really run in CI, never skip to green.
- Update the package guide in `doc/` (or the README section) when the public API changes, and add a
  row to [docs/INDEX.md](docs/INDEX.md) when you add a document under `docs/`.
- Do not commit secrets, hostnames, personal paths or other personal data.

## Conduct and security

Participation is covered by the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities
privately as described in [SECURITY.md](SECURITY.md); do not open a public issue.

Contributions are licensed under the repository's [Apache-2.0 licence](LICENSE).
