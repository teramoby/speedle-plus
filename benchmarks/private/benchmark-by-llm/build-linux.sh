#!/usr/bin/env bash
# Rebuild the locked Linux package on shrek; does not start remote tests.
set -euo pipefail
cd "$(dirname "$0")"
GO_BIN=${GO_BIN:-/opt/homebrew/bin/go}
PACKAGE=${1:-dist/authz-bench-linux-20261029}
case "$PACKAGE" in ''|/|.) echo 'Choose a package directory under dist/' >&2; exit 2;; esac
if [[ $("$GO_BIN" version) != 'go version go1.27.2 '* ]]; then
  echo 'This comparison pins compiler Go 1.27.2; set GO_BIN to that toolchain.' >&2; exit 2
fi
mkdir -p "$PACKAGE/bin"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" test -c -o "$PACKAGE/bin/authzbench.test" .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build -o "$PACKAGE/bin/" ./cmd/authzd ./cmd/loadgen ./cmd/watchcheck
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build -o "$PACKAGE/bin/opa" github.com/open-policy-agent/opa
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build -o "$PACKAGE/bin/speedle-ads" github.com/teramoby/speedle-plus/cmd/speedle-ads
cp -R scenarios stress.sh run-all.sh summarize.py README.md TEST_PLAN.md "$PACKAGE/"
{
  date -u '+%Y-%m-%dT%H:%M:%SZ'; "$GO_BIN" version
  echo 'GOOS=linux GOARCH=amd64 CGO_ENABLED=0; local authzbench source snapshot in source.tar.gz'
  for program in authzbench.test authzd loadgen opa speedle-ads watchcheck; do
    "$GO_BIN" version -m "$PACKAGE/bin/$program"
    shasum -a 256 "$PACKAGE/bin/$program"
  done
} > "$PACKAGE/build-info.txt"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$PACKAGE/source.tar.gz" gen.go engines.go bench_test.go scenarios_test.go generated_test.go capabilities_test.go broken_test.go go.mod go.sum deps.go cmd scenarios stress.sh run-all.sh summarize.py build-linux.sh build_report.py plot_results.py plot-requirements.txt
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$PACKAGE.tar.gz" -C "$(dirname "$PACKAGE")" "$(basename "$PACKAGE")"
printf '%s\n' "$PACKAGE.tar.gz"
