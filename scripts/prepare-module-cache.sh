#!/usr/bin/env bash
set -euo pipefail

export HOME=/work/home
export GOCACHE=/work/go-build-cache
export GOMODCACHE=/work/go-module-cache
export GOPATH=/work/gopath
export GOENV=off
export TMPDIR=/work/tmp

mkdir -p "${HOME}" "${GOCACHE}" "${GOMODCACHE}" "${GOPATH}" "${TMPDIR}"
cd /usr/local/go/src/runtime/_mkmalloc
/usr/local/go/bin/go mod download golang.org/x/tools@v0.33.0
/usr/local/go/bin/go mod verify

