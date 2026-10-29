#!/usr/bin/env bash
# Run the Go test suites for one CI mode.
#
#   gate     every package once with the file store, the store-dependent
#            packages again with etcd, the coverage gate, and both bilingual
#            executable guides from docs/ (merge gating)
#   nightly  race-detector run of every package with both stores, the
#            quick start with every supported Go release, govulncheck and the
#            CI script tests
#   weekly   every package with the newest supported Go release
#            (the gate uses the go.mod/CI release) and staticcheck
#
# nightly and weekly never stop at the first failure: each suite writes its
# results to RESULTS_DIR (go test -json streams plus checks.txt lines of
# name=exit_status:log_file) for scripts/ci/test_report.py, and the script
# exits non-zero at the end if anything failed.
set -euo pipefail
cd "$(dirname "$0")/../.."

mode=${1:?usage: run-go-tests.sh gate|nightly|weekly}
results=${RESULTS_DIR:-test-results}

# Only these packages read STORE_TYPE; every other package tests its own
# store or none, so running them twice would repeat identical work.
store_dependent=(./pkg/eval ./cmd/spctl/command)
# Supported Go releases besides the CI default (see the quick start).
other_toolchains=(go1.27.3)
all_toolchains=(go1.26.10 go1.27.3)

packages() {
  # MongoDB runs in the weekly hosted job; generated protobuf packages,
  # samples and demos are not product code.
  local listed
  # Checked explicitly: errexit does not reach into command substitutions.
  listed=$(go list ./pkg/... ./cmd/... ./api/... | grep -v -e '/samples' -e '/pkg/store/mongodb' -e '/pkg/logging/demo' -e '/pb$') || return 1
  [[ -n "${listed}" ]] || { echo 'No Go packages found.' >&2; return 1; }
  printf '%s\n' "${listed}"
}

failed=0
check() {
  # check NAME COMMAND...: run COMMAND, record its status and log.
  local name=$1 status=0
  shift
  "$@" > "${results}/${name}.log" 2>&1 || status=$?
  echo "${name}=${status}:${name}.log" >> "${results}/checks.txt"
  (( status == 0 )) || failed=1
  echo "${name}: exit ${status}"
}
gotest_json() {
  # gotest_json NAME [ENV=VALUE...] -- go test args...: run go test -json.
  local name=$1 status=0
  shift
  local -a envs=()
  while [[ $1 != -- ]]; do envs+=("$1"); shift; done
  shift
  env "${envs[@]}" go test -json "$@" > "${results}/${name}.json" 2> "${results}/${name}.stderr" || status=$?
  (( status == 0 )) || failed=1
  echo "${name}: exit ${status}"
}

case "${mode}" in
  gate)
    listed=$(packages); mapfile -t pkgs <<< "${listed}"
    # The CLI tests run spctl as a subprocess; its profile is merged in.
    rm -f coverage-cli.out
    SPEEDLE_CLI_COVERPROFILE="${PWD}/coverage-cli.out" STORE_TYPE=file \
      go test -p 1 -count=1 -timeout 300s -coverprofile=coverage.out "${pkgs[@]}"
    STORE_TYPE=etcd go test -p 1 -count=1 -timeout 300s "${store_dependent[@]}"
    python3 scripts/ci/coverage.py coverage.out --merge coverage-cli.out \
      --min-file scripts/ci/coverage-min.txt "${pkgs[@]}"
    # Read the candidate's documentation on every run. These execute the actual
    # fenced commands/program/policy blocks; failures block the same test job.
    toolchain=$(go env GOVERSION)
    python3 website/tests/check_quick_start.py --toolchain "${toolchain}"
    python3 website/tests/check_embedded.py --toolchain "${toolchain}" --candidate
    ;;
  nightly)
    mkdir -p "${results}"; : > "${results}/checks.txt"
    listed=$(packages); mapfile -t pkgs <<< "${listed}"
    race=(-race)
    if ! CGO_ENABLED=1 go env CC >/dev/null 2>&1 || ! command -v "$(go env CC)" >/dev/null 2>&1; then
      race=()
      echo "race=1:race.log" >> "${results}/checks.txt"
      echo 'The race detector needs a C compiler; tests ran without -race.' > "${results}/race.log"
      failed=1
    fi
    gotest_json go-file STORE_TYPE=file -- -p 1 -count=1 -timeout 600s "${race[@]}" "${pkgs[@]}"
    gotest_json go-etcd STORE_TYPE=etcd -- -p 1 -count=1 -timeout 600s "${race[@]}" "${store_dependent[@]}"
    for toolchain in "${all_toolchains[@]}"; do
      check "quick-start-${toolchain}" python3 website/tests/check_quick_start.py --toolchain "${toolchain}"
    done
    check govulncheck go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./pkg/... ./cmd/... ./api/...
    check ci-script-tests bash -c 'python3 -m unittest discover -s scripts/tests -p "test_*.py" && node --test scripts/tests/*.cjs'
    exit "${failed}"
    ;;
  weekly)
    mkdir -p "${results}"; : > "${results}/checks.txt"
    listed=$(packages); mapfile -t pkgs <<< "${listed}"
    for toolchain in "${other_toolchains[@]}"; do
      gotest_json "go-${toolchain}" GOTOOLCHAIN="${toolchain}" STORE_TYPE=file -- -p 1 -count=1 -timeout 600s "${pkgs[@]}"
    done
    check staticcheck go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./pkg/... ./cmd/... ./api/...
    exit "${failed}"
    ;;
  *)
    echo "Unknown mode ${mode}." >&2
    exit 2
    ;;
esac
