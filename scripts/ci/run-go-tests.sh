#!/usr/bin/env bash
# Run the Go test suites for one CI mode.
#
#   gate     every package once with the file store, the store-dependent
#            packages again with etcd, the coverage gate, and both bilingual
#            executable guides from docs/ (merge gating)
#   nightly  the same full regression (scripts/ci/store_regression.py: every
#            package under the race detector, then the tagged ADS
#            runtime/cache suites against native PMS/ADS) with file, etcd,
#            MySQL and MongoDB, each on a store this run owns; the etcd and
#            MongoDB setup guides, the quick start with every supported Go
#            release, govulncheck and the CI script tests
#   weekly   every package with the newest supported Go release
#            (the gate uses the go.mod/CI release) and staticcheck
#   regression
#            after every merge to master: race-detector run of every package
#            with the file store and the executable guides from docs/
#
# nightly, weekly and regression never stop at the first failure: each suite
# writes its results to RESULTS_DIR (go test -json streams plus checks.txt
# lines of name=exit_status:log_file) for scripts/ci/test_report.py, and the
# script exits non-zero at the end if anything failed.
set -euo pipefail
cd "$(dirname "$0")/../.."

mode=${1:?usage: run-go-tests.sh gate|nightly|weekly|regression}
results=${RESULTS_DIR:-test-results}

# The gate reruns only these packages with etcd; nightly reruns every
# package, whose STORE_TYPE-aware fixtures (testutil/storetest) then use etcd.
store_dependent=(./pkg/eval ./cmd/spctl/command)
# Supported Go releases besides the CI default (see the quick start).
other_toolchains=(go1.27.3)
all_toolchains=(go1.26.10 go1.27.3)

packages() {
  # MongoDB runs in the nightly hosted job; generated protobuf packages,
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
  if [[ "$name" == *-full ]]; then
    "$@" 2>&1 | tee "${results}/${name}.log" || status=$?
  else
    "$@" > "${results}/${name}.log" 2>&1 || status=$?
  fi
  echo "${name}=${status}:${name}.log" >> "${results}/checks.txt"
  (( status == 0 )) || failed=1
  echo "${name}: exit ${status}"
}
with_docker() {
  # with_docker COMMAND...: run COMMAND where it can reach Docker. A runner
  # service started before its account joined the docker group lacks that
  # group; like run-mysql-tests.py, use the account's existing membership.
  if docker info > /dev/null 2>&1 || ! id -nG "$(id -un)" | grep -qw docker; then
    "$@"
  else
    sg docker -c "$(printf '%q ' "$@")"
  fi
}
set_race() {
  # Sets race to (-race), or to () with a failed check when no C compiler is
  # available for the race detector.
  race=(-race)
  if ! CGO_ENABLED=1 go env CC >/dev/null 2>&1 || ! command -v "$(go env CC)" >/dev/null 2>&1; then
    race=()
    echo "race=1:race.log" >> "${results}/checks.txt"
    echo 'The race detector needs a C compiler; tests ran without -race.' > "${results}/race.log"
    failed=1
  fi
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
    listed=$(packages); pkgs=(); while IFS= read -r package; do pkgs+=("${package}"); done <<< "${listed}"
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
    go test -count=1 ./website/tests/docscheck
    python3 website/tests/check_docs.py
    ;;
  nightly)
    mkdir -p "${results}"; : > "${results}/checks.txt"
    # A branch can be tried immediately through the existing protected worker.
    # Normal scheduled runs always include all backends.
    if [[ "${GITHUB_EVENT_NAME:-}" == workflow_dispatch && -f "${GITHUB_EVENT_PATH:-}" ]] &&
       python3 -c 'import json,sys; value=json.load(open(sys.argv[1])).get("inputs",{}).get("mysql_only",False); sys.exit(0 if value is True or value == "true" else 1)' "$GITHUB_EVENT_PATH"; then
      check mysql-full python3 scripts/ci/run-mysql-tests.py
      check ci-script-tests bash -c 'python3 -m unittest discover -s scripts/tests -p "test_*.py" && node --test scripts/tests/*.cjs'
      exit "${failed}"
    fi
    if ! CGO_ENABLED=1 go env CC >/dev/null 2>&1 || ! command -v "$(go env CC)" >/dev/null 2>&1; then
      echo "race=1:race.log" >> "${results}/checks.txt"
      echo 'The race detector needs a C compiler; the full regressions cannot run.' > "${results}/race.log"
      failed=1
    fi
    # Every backend runs the identical regression; the launchers only provision
    # the store, and the engine writes <backend>-*.json into RESULTS_DIR.
    export RESULTS_DIR="${results}"
    check file-full env STORE_TYPE=file python3 scripts/ci/run-store-tests.py
    check etcd-full bash scripts/ci/validate-etcd.sh
    check mysql-full python3 scripts/ci/run-mysql-tests.py
    check mongodb-full bash scripts/ci/validate-mongodb.sh
    # Manual candidate validation can omit notification/report side effects.
    if [[ "${GITHUB_EVENT_NAME:-}" == workflow_dispatch && -f "${GITHUB_EVENT_PATH:-}" ]] &&
       python3 -c 'import json,sys; value=json.load(open(sys.argv[1])).get("inputs",{}).get("stores_only",False); sys.exit(0 if value is True or value == "true" else 1)' "$GITHUB_EVENT_PATH"; then
      check ci-script-tests bash -c 'python3 -m unittest discover -s scripts/tests -p "test_*.py" && node --test scripts/tests/*.cjs'
      exit "${failed}"
    fi
    check etcd-docs with_docker python3 website/tests/check_etcd.py
    check mongodb-docs with_docker python3 website/tests/check_mongodb.py
    for toolchain in "${all_toolchains[@]}"; do
      check "quick-start-${toolchain}" python3 website/tests/check_quick_start.py --toolchain "${toolchain}"
    done
    check govulncheck go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./pkg/... ./cmd/... ./api/...
    check ci-script-tests bash -c 'python3 -m unittest discover -s scripts/tests -p "test_*.py" && node --test scripts/tests/*.cjs'
    exit "${failed}"
    ;;
  regression)
    mkdir -p "${results}"; : > "${results}/checks.txt"
    listed=$(packages); pkgs=(); while IFS= read -r package; do pkgs+=("${package}"); done <<< "${listed}"
    set_race
    gotest_json go-file STORE_TYPE=file -- -p 1 -count=1 -timeout 600s "${race[@]}" "${pkgs[@]}"
    toolchain=$(go env GOVERSION)
    check quick-start python3 website/tests/check_quick_start.py --toolchain "${toolchain}"
    check embedded python3 website/tests/check_embedded.py --toolchain "${toolchain}" --candidate
    check docs-contract go test -count=1 ./website/tests/docscheck
    check docs python3 website/tests/check_docs.py
    exit "${failed}"
    ;;
  weekly)
    mkdir -p "${results}"; : > "${results}/checks.txt"
    listed=$(packages); pkgs=(); while IFS= read -r package; do pkgs+=("${package}"); done <<< "${listed}"
    for toolchain in "${other_toolchains[@]}"; do
      gotest_json "go-${toolchain}" GOTOOLCHAIN="${toolchain}" STORE_TYPE=file -- -p 1 -count=1 -timeout 600s "${pkgs[@]}"
    done
    gotest_json feature-large STORE_TYPE=file SPEEDLE_STRESS_POLICIES=100000 -- -race -count=1 -timeout 1200s -tags feature_stress ./pkg/eval -run '^TestFeatureLargePolicyRoleSets$'
    check staticcheck go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./pkg/... ./cmd/... ./api/...
    exit "${failed}"
    ;;
  *)
    echo "Unknown mode ${mode}." >&2
    exit 2
    ;;
esac
