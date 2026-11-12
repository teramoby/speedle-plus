#!/usr/bin/env bash
# Rebuild the locked Linux package of the 2026-10-29 archive from its archived
# source commit; does not start remote tests. The working tree has since gained
# Cedar and a different go.mod Speedle+ pin, so it cannot reproduce the
# archived binaries.
set -euo pipefail
cd "$(dirname "$0")"
GO_BIN=${GO_BIN:-/opt/homebrew/bin/go}
PACKAGE=${1:-dist/authz-bench-linux-20261029}
case "$PACKAGE" in ''|/|.) echo 'Choose a package directory under dist/' >&2; exit 2;; esac
# The commit that archived the 2026-10-29 evidence and its source tree.
ARCHIVE_COMMIT=${ARCHIVE_COMMIT:-bca2c4e59df907c8cddd5836074ced4184b0f522}
mkdir -p "$(dirname "$PACKAGE")"
PACKAGE=$(cd "$(dirname "$PACKAGE")" && pwd)/$(basename "$PACKAGE")
TOP=$(git rev-parse --show-toplevel 2>/dev/null) || { echo 'build-linux.sh needs a git checkout to extract the archived source.' >&2; exit 2; }
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
git -C "$TOP" archive "$ARCHIVE_COMMIT" benchmarks/private/benchmark-by-llm | tar -x -C "$WORK"
(cd "$WORK/benchmarks/private/benchmark-by-llm" && GO_BIN="$GO_BIN" ./build-linux.sh "$PACKAGE")
