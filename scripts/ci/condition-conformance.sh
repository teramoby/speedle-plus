#!/usr/bin/env bash
# Exercise both authorization paths and the fork's supplemental tests, then
# audit every condition-path statement without changing the repository gate.
set -euo pipefail
cd "$(dirname "$0")/../.."
conformance_results=$(mktemp -d "${TMPDIR:-/tmp}/speedle-condition-conformance.XXXXXX")
trap 'rm -rf "${conformance_results}"' EXIT

coverage_go_version=$(go env GOVERSION)
echo "Condition coverage toolchain: ${coverage_go_version}"
STORE_TYPE=file go test -p 1 -count=1 -timeout 300s \
  -coverpkg=./pkg/eval/condition,./pkg/eval/function,./3rdparty/github.com/Knetic/govaluate \
  -coverprofile="${conformance_results}/coverage.out" \
  ./pkg/eval/... ./pkg/svcs/adsrest/... ./3rdparty/github.com/Knetic/govaluate
STORE_TYPE=file go test -p 1 -count=1 -timeout 300s -v ./pkg/eval -run '^TestConditionConformanceMatrix$' \
  > "${conformance_results}/matrix.log"
python3 scripts/ci/condition-conformance-report.py \
  --go-version "${coverage_go_version}" \
  "${conformance_results}/coverage.out" "${conformance_results}/matrix.log"
python3 -m unittest discover -s scripts/tests -p 'test_condition_conformance_report.py'
