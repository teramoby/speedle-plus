#!/usr/bin/env bash
# Build the Linux x86-64 package for a four-engine run (Casbin, OPA, Cedar,
# Speedle+): the test binaries, servers and load generator, Cedar's Rust
# harness, and two Speedle+ builds (go.mod: current master; go.may.mod:
# master throughout May 2026, 8fb1481d, unchanged from 2026-01-16 until
# 2026-06-24). Runs where Go and Cargo are installed
# (the CI package job); the benchmark host needs neither.
set -euo pipefail
cd "$(dirname "$0")"
PACKAGE=${1:-dist/authz-bench-linux}
case "$PACKAGE" in ''|/|.) echo 'Choose a package directory under dist/' >&2; exit 2;; esac
mkdir -p "$PACKAGE/bin"
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
go test -c -o "$PACKAGE/bin/authzbench.test" .
go test -c -modfile=go.may.mod -o "$PACKAGE/bin/authzbench-may.test" .
go build -o "$PACKAGE/bin/" ./cmd/authzd ./cmd/loadgen ./cmd/watchcheck
go build -modfile=go.may.mod -o "$PACKAGE/bin/authzd-may" ./cmd/authzd
go build -o "$PACKAGE/bin/opa" github.com/open-policy-agent/opa
go build -o "$PACKAGE/bin/speedle-ads" github.com/teramoby/speedle-plus/cmd/speedle-ads
go build -modfile=go.may.mod -o "$PACKAGE/bin/speedle-ads-may" github.com/teramoby/speedle-plus/cmd/speedle-ads
(cd cmd/cedar-rs && cargo build --release --locked)
cp cmd/cedar-rs/target/release/cedar-rs "$PACKAGE/bin/"
cp -R scenarios stress.sh run-all.sh summarize.py "$PACKAGE/"
{
  date -u '+%Y-%m-%dT%H:%M:%SZ'; go version; cargo --version; rustc --version
  echo 'GOOS=linux GOARCH=amd64 CGO_ENABLED=0; cedar-rs: cargo --release (lto=fat, codegen-units=1), system allocator'
  for program in authzbench.test authzbench-may.test authzd authzd-may loadgen opa speedle-ads speedle-ads-may watchcheck; do
    go version -m "$PACKAGE/bin/$program"
  done
  grep -A1 -E '^name = "(cedar-policy|axum|tokio)"$' cmd/cedar-rs/Cargo.lock
  (cd "$PACKAGE/bin" && sha256sum ./*)
} > "$PACKAGE/build-info.txt"
tar -czf "$PACKAGE/source.tar.gz" ./*.go go.mod go.sum go.may.mod go.may.sum cmd/authzd cmd/loadgen cmd/watchcheck \
  cmd/cedar-rs/Cargo.toml cmd/cedar-rs/Cargo.lock cmd/cedar-rs/src scenarios stress.sh run-all.sh summarize.py build-package.sh
printf '%s\n' "$PACKAGE"
