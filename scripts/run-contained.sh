#!/usr/bin/env bash
set -euo pipefail

readonly LAB_ROOT=/lab
readonly WORK_ROOT=/work
readonly TOOLCHAIN_ROOT=/work/go-abort
readonly EVIDENCE_ROOT=/work/evidence
readonly UPSTREAM_VERSION=go1.26.4

export HOME=/work/home
export GOCACHE=/work/go-build-cache
export GOMODCACHE=/work/go-module-cache
export GOPATH=/work/gopath
export GOENV=off
export TMPDIR=/work/tmp

rm -rf "${TOOLCHAIN_ROOT}" "${EVIDENCE_ROOT}" "${TMPDIR}"
mkdir -p "${EVIDENCE_ROOT}" "${HOME}" "${GOCACHE}" "${GOMODCACHE}" "${GOPATH}" "${TMPDIR}"
cp -a /usr/local/go "${TOOLCHAIN_ROOT}"

cd "${TOOLCHAIN_ROOT}"
git apply --check "${LAB_ROOT}/patches/go1.26.4-goroutine-abort.patch"
git apply "${LAB_ROOT}/patches/go1.26.4-goroutine-abort.patch"
git apply --check "${LAB_ROOT}/patches/go1.26.4-abort-safe-boundary.patch"
git apply "${LAB_ROOT}/patches/go1.26.4-abort-safe-boundary.patch"
git apply --check "${LAB_ROOT}/patches/go1.26.4-abort-sysmon-retry.patch"
git apply "${LAB_ROOT}/patches/go1.26.4-abort-sysmon-retry.patch"
git apply --check "${LAB_ROOT}/patches/go1.26.4-abort-scheduler-livelock.patch"
git apply "${LAB_ROOT}/patches/go1.26.4-abort-scheduler-livelock.patch"
git apply --check "${LAB_ROOT}/patches/go1.26.4-abort-lifecycle-accounting.patch"
git apply "${LAB_ROOT}/patches/go1.26.4-abort-lifecycle-accounting.patch"

cd "${TOOLCHAIN_ROOT}/src"
GOROOT_BOOTSTRAP=/usr/local/go ./make.bash \
  >"${EVIDENCE_ROOT}/make.stdout.log" \
  2>"${EVIDENCE_ROOT}/make.stderr.log"

"${TOOLCHAIN_ROOT}/bin/go" version | tee "${EVIDENCE_ROOT}/go-version.txt"
cd "${LAB_ROOT}"
GOMAXPROCS=1 "${TOOLCHAIN_ROOT}/bin/go" test ./aborttest -count=1 -v \
  | tee "${EVIDENCE_ROOT}/abort-tests.txt"
GOMAXPROCS=8 "${TOOLCHAIN_ROOT}/bin/go" test ./aborttest -count=20 -run 'TestHardAbortTightLoop|TestGracefulAbortRunsDefers' \
  | tee "${EVIDENCE_ROOT}/repeat-tests.txt"
"${TOOLCHAIN_ROOT}/bin/go" run ./cmd/abort-probe \
  -cycles 1000 -timeout 250ms \
  | tee "${EVIDENCE_ROOT}/probe.json"
"${TOOLCHAIN_ROOT}/bin/go" test ./aborttest -count=1 \
  -run TestHardAbortTightLoop \
  -trace="${EVIDENCE_ROOT}/hard-abort.trace" \
  | tee "${EVIDENCE_ROOT}/trace-test.txt"
"${TOOLCHAIN_ROOT}/bin/go" tool trace -d=parsed \
  "${EVIDENCE_ROOT}/hard-abort.trace" >/dev/null
printf 'trace-parse-ok\n' >"${EVIDENCE_ROOT}/trace-parse.txt"
"${TOOLCHAIN_ROOT}/bin/go" test -race ./aborttest -count=1 \
  -run 'TestConcurrentHardAbortCallers|TestHardAbortWaitsForSyscallReturn' \
  | tee "${EVIDENCE_ROOT}/race-tests.txt"
GOOS=linux GOARCH=386 "${TOOLCHAIN_ROOT}/bin/go" test -c \
  -o "${TMPDIR}/aborttest-linux-386" ./aborttest
printf 'linux-386-compile-ok\n' >"${EVIDENCE_ROOT}/linux-386-compile.txt"
"${TOOLCHAIN_ROOT}/bin/go" vet ./... \
  | tee "${EVIDENCE_ROOT}/vet.txt"

"${TOOLCHAIN_ROOT}/bin/go" test runtime -count=1 \
  | tee "${EVIDENCE_ROOT}/runtime-tests.txt"

sha256sum "${LAB_ROOT}/patches/go1.26.4-goroutine-abort.patch" \
  "${LAB_ROOT}/patches/go1.26.4-abort-safe-boundary.patch" \
  "${LAB_ROOT}/patches/go1.26.4-abort-sysmon-retry.patch" \
  "${LAB_ROOT}/patches/go1.26.4-abort-scheduler-livelock.patch" \
  "${LAB_ROOT}/patches/go1.26.4-abort-lifecycle-accounting.patch" \
  "${EVIDENCE_ROOT}"/* >"${EVIDENCE_ROOT}/sha256sums.txt"
