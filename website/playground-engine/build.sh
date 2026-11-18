#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(git rev-parse --show-toplevel)
REVISION=7d2f08e5fd25a58c6e62983c4a9ac2797048a18c
GO_BIN="${GO_BIN:-go}"
# Select the pinned toolchain even when the trusted worker preinstalls an
# older Playground Go. This does not alter the worker or host configuration.
export GOTOOLCHAIN=go1.27.4
# The build tree is a git archive; never read VCS state from an enclosing checkout.
export GOFLAGS=-buildvcs=false
if [[ "$("$GO_BIN" env GOVERSION)" != go1.27.4 ]]; then
  echo 'Use Go 1.27.4 for a reproducible build (set GO_BIN=/path/to/go).' >&2
  exit 1
fi
# Build from the checked-in, checksum-pinned Git source pack. It is independent
# of Git commit availability after a squash merge or branch deletion.
grep -Fq "const Revision = \"$REVISION\"" playground/evaluate.go
BUILD_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/speedle-playground.XXXXXXXX")
trap 'rm -rf "$BUILD_ROOT"' EXIT
python3 "$ROOT/website/playground-engine/engine-source.py" verify "$REVISION" "$BUILD_ROOT"
mkdir -p "$BUILD_ROOT/website/playground-engine"
cp -R cmd playground go.mod go.sum "$BUILD_ROOT/website/playground-engine/"
OUT="$ROOT/docs/hugo/static/playground"
# License files are generated from the actual dependency graph. Remove stale
# notices when a dependency disappears from the WASM build.
rm -rf "$OUT/licenses"
mkdir -p "$OUT/licenses"
cd "$BUILD_ROOT/website/playground-engine"
"$GO_BIN" test -count=1 -mod=readonly ./playground
GOROOT=$("$GO_BIN" env GOROOT)
PLAYGROUND_GOROOT="$GOROOT" GOOS=js GOARCH=wasm "$GO_BIN" test -count=1 -mod=readonly -exec="\"$ROOT/website/playground-engine/wasm-test-exec.sh\"" ./playground
GOOS=js GOARCH=wasm "$GO_BIN" build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o "$OUT/speedle.wasm" ./cmd/wasm
chmod 0644 "$OUT/speedle.wasm"
install -m 0644 "$GOROOT/lib/wasm/wasm_exec.js" "$OUT/wasm_exec.js"
install -m 0644 "$GOROOT/LICENSE" "$OUT/licenses/Go-BSD-3-Clause.txt"
install -m 0644 "$BUILD_ROOT/LICENSE" "$OUT/licenses/Speedle-UPL-1.0.txt"
install -m 0644 "$BUILD_ROOT/pkg/condexpr/LICENSE" "$OUT/licenses/govaluate-MIT.txt"
install -m 0644 "$BUILD_ROOT/THIRD_PARTY_LICENSES.txt" "$OUT/licenses/Speedle-upstream-third-party.txt"
GOOS=js GOARCH=wasm "$GO_BIN" list -mod=readonly -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' ./cmd/wasm | sort -u | while IFS='|' read -r module directory; do
  [[ -n "$module" && "$module" != speedle-playground && "$module" != github.com/teramoby/speedle-plus ]] || continue
  safe_name=${module//\//_}
  for license in LICENSE LICENSE.txt LICENSE.md COPYING NOTICE; do
    [[ ! -f "$directory/$license" ]] || install -m 0644 "$directory/$license" "$OUT/licenses/${safe_name}_${license}.txt"
  done
done
# Hugo source is authoritative; GitHub Pages serves the checked-in docs copy.
rm -rf "$ROOT/docs/playground/licenses"
cp -R "$OUT/." "$ROOT/docs/playground/"
mkdir -p "$ROOT/docs/zh/playground"
cp "$ROOT/docs/hugo/static/zh/playground/index.html" "$ROOT/docs/zh/playground/index.html"
cp "$ROOT/docs/hugo/static/index.html" "$ROOT/docs/index.html"
cp "$ROOT/docs/hugo/static/zh/index.html" "$ROOT/docs/zh/index.html"
for stylesheet in site.css navigation.css; do
  cp "$ROOT/docs/hugo/static/css/$stylesheet" "$ROOT/docs/css/$stylesheet"
done
cp "$ROOT/docs/hugo/static/js/navigation.js" "$ROOT/docs/js/navigation.js"
sha256sum "$OUT/speedle.wasm"
