.RECIPEPREFIX = >

# Race detector requires CGO. On runners without libtokenizers/libonnxruntime
# (e.g. GitHub-hosted ubuntu-latest), go-kit falls back to the !cgo stubs, so
# skip -race when CGO_ENABLED is 0.
RACE := $(if $(filter 1,$(shell GOWORK=off go env CGO_ENABLED)),-race,)

.PHONY: test test-db lint cover preflight

test:
> @GOWORK=off go test ./...

# DB-backed suites against a THROWAWAY ephemeral Postgres 16 (127.0.0.1:55434,
# spun up + torn down by scripts/test-db.sh — never the live shared Postgres
# on :5432). This is the ONLY way the telegram/fsm Postgres-backed guard tests
# actually run: they t.Skip when TEST_DATABASE_URL is unset, and skip-to-green
# is forbidden (a DB test that t.Skips for a missing dep gets the dep
# provisioned, not skipped).
test-db:
> @bash scripts/test-db.sh

lint:
> @GOWORK=off golangci-lint run --allow-parallel-runners ./...

cover:
> @GOWORK=off go test -coverprofile=coverage.out ./...
> @GOWORK=off go tool cover -func=coverage.out

# Merge-gate for the self-hosted CI (.github/workflows/preflight.yml): gofmt +
# vet + build + the FULL DB-backed suite. Uses `make test-db` (an ephemeral
# throwaway Postgres), NOT a bare `go test`, so the telegram/fsm Postgres
# guard tests — including the #155 regression (VARCHAR-too-narrow → 0 rows
# persisted) — actually RUN instead of t.Skip-ing (skip-to-green is forbidden).
preflight:
> @echo "==> gofmt -l"
> @out=$$(GOWORK=off gofmt -l .); \
> if [ -n "$$out" ]; then \
> echo "FAIL: gofmt drift in the following files (run: gofmt -w <file>):"; \
> echo "$$out"; \
> exit 1; \
> fi
> @echo "==> go vet ./..."
> @GOWORK=off go vet ./...
> @echo "==> go build ./..."
> @GOWORK=off go build ./...
> @echo "==> make test-db (ephemeral Postgres; FSM guard tests run for real)"
> @$(MAKE) test-db
