#!/usr/bin/env bash
# test-db.sh — run the DB-backed test suites against a THROWAWAY Postgres 16.
#
# If TEST_DATABASE_URL is already set (e.g. by GitHub Actions `services:`),
# uses it directly — no Docker container is started. This lets the same
# `make preflight` work on both:
#   - GitHub-hosted runners (ubuntu-latest + services: postgres)
#   - Local/self-hosted runs (spins an ephemeral postgres:16-alpine)
#
# When TEST_DATABASE_URL is unset, spins up an ephemeral postgres:16-alpine
# on 127.0.0.1:55434 (NOT the live shared Postgres on :5432), exports
# TEST_DATABASE_URL to it, runs the full test suite, and ALWAYS tears the
# container down (trap on EXIT — even on test failure or Ctrl-C). The
# container name is unique per run so concurrent invocations and stale
# leftovers never collide.
#
# This is the ONLY way the telegram/fsm Postgres-backed guard tests actually
# run: they t.Skip when TEST_DATABASE_URL is unset, and skip-to-green is
# forbidden (a DB test that t.Skips for a missing dep gets the dep
# provisioned, not skipped).
#
# Readiness probe: pg_isready with -h 127.0.0.1 -p 5432 forces a TCP check.
# Without -h, pg_isready probes the UNIX socket which the postgres entrypoint
# bootstraps on a TEMPORARY socket-only server BEFORE the TCP listener on 5432
# is ready — so the probe can succeed while go test (TCP) still hits
# connection-refused. The TCP probe eliminates this race.
#
# Usage: make test-db   (or: bash scripts/test-db.sh)
set -euo pipefail

run_tests() {
  echo ">> running full test suite with TEST_DATABASE_URL"
  # -p 1: run packages one at a time so the FSM Postgres suite does not share
  # the throwaway DB concurrently with other packages that might also pick up
  # TEST_DATABASE_URL. The FSM tests DROP TABLE in t.Cleanup, and parallel
  # package execution could race the DROP against a concurrent test's SELECT.
  # -race only when CGO is enabled (race detector requires cgo).
  local race_flag=""
  if [ "${CGO_ENABLED:-1}" = "1" ]; then
    race_flag="-race"
  fi
  GOWORK=off go test -p 1 $race_flag -count=1 ./...
}

# If TEST_DATABASE_URL is already set (GitHub Actions services:, or manual
# env), use it directly — no Docker needed.
if [ -n "${TEST_DATABASE_URL:-}" ]; then
  echo ">> TEST_DATABASE_URL already set — using external Postgres"
  run_tests
  exit 0
fi

PORT="${TEST_DB_PORT:-55434}"
NAME="gokit-test-pg-$$"            # unique per process; $$ = PID
IMAGE="postgres:16-alpine"
PGUSER="test"
PGPASS="test"
PGDB="testdb"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo ">> starting ephemeral $IMAGE as $NAME on 127.0.0.1:$PORT"
# Reclaim a stale container still holding the fixed host port: a SIGKILL'd
# GH cancel (realistic on the loaded box) skips the EXIT trap and leaks one,
# wedging the next run on "port already allocated".
docker ps -q --filter "publish=${PORT}" | xargs -r docker rm -f
docker run -d \
  --name "$NAME" \
  -e POSTGRES_USER="$PGUSER" \
  -e POSTGRES_PASSWORD="$PGPASS" \
  -e POSTGRES_DB="$PGDB" \
  -p "127.0.0.1:${PORT}:5432" \
  "$IMAGE" >/dev/null

echo ">> waiting for postgres TCP listener on 127.0.0.1:5432 (inside container)"
for i in $(seq 1 30); do
  if docker exec "$NAME" pg_isready -h 127.0.0.1 -p 5432 -U "$PGUSER" -d "$PGDB" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "!! postgres TCP did not become ready in 30s" >&2
    docker logs "$NAME" >&2 || true
    exit 1
  fi
  sleep 1
done

export TEST_DATABASE_URL="postgres://${PGUSER}:${PGPASS}@127.0.0.1:${PORT}/${PGDB}?sslmode=disable"
echo ">> TEST_DATABASE_URL set (ephemeral; NOT live shared Postgres)"

run_tests
