#!/usr/bin/env bash
# Build gostall at a pinned version against a pinned, current golang.org/x/tools.
#
# gostall v1.0.0 is the only release and its go.mod requires x/tools v0.44.0,
# whose go/packages cannot type-check against the Go 1.27 standard library
# ("package ... without types was imported from ..."). The source stays at
# v1.0.0; only x/tools is raised. Bump XTOOLS_VERSION to the latest stable
# (`go list -m -versions golang.org/x/tools`) when the Go toolchain moves.
#
# Always rebuilds: a binary left in $GOBIN by an earlier run (self-hosted
# runner, cache) may have been linked against an older x/tools or toolchain.
set -euo pipefail

GOSTALL_VERSION="${GOSTALL_VERSION:-v1.0.0}"
XTOOLS_VERSION="${XTOOLS_VERSION:-v0.52.0}"
GOBIN="${GOBIN:-$(go env GOPATH)/bin}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

(
  cd "$tmp"
  export GOWORK=off GOFLAGS=
  go mod init gostall-build >/dev/null 2>&1
  go get "github.com/erfanmomeniii/gostall/cmd/gostall@${GOSTALL_VERSION}" "golang.org/x/tools@${XTOOLS_VERSION}"
  mkdir -p "$GOBIN"
  go build -o "$GOBIN/gostall" github.com/erfanmomeniii/gostall/cmd/gostall
)

info="$(go version -m "$GOBIN/gostall")"
echo "$info" | grep -q "erfanmomeniii/gostall.*${GOSTALL_VERSION}" || { echo "gostall: wrong version built" >&2; exit 1; }
echo "$info" | grep -q "golang.org/x/tools.*${XTOOLS_VERSION}" || { echo "gostall: wrong x/tools linked" >&2; exit 1; }
echo "gostall ${GOSTALL_VERSION} (x/tools ${XTOOLS_VERSION}) installed to $GOBIN"
