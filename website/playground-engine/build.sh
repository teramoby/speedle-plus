#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(git rev-parse --show-toplevel)
REVISION=7f61995d3344e94ed614494e013dacdcfa491b26
GO_BIN="${GO_BIN:-go}"
export GOTOOLCHAIN=local
if [[ "$("$GO_BIN" env GOVERSION)" != go1.26.3 ]]; then
  echo 'Use Go 1.26.3 for a reproducible build (set GO_BIN=/path/to/go).' >&2
  exit 1
fi
# Build the real engine from an immutable archive. Later server/dependency edits
# must not silently change the published browser engine or break its rebuild.
git -C "$ROOT" cat-file -e "$REVISION^{commit}" || {
  echo "Missing pinned engine commit $REVISION; fetch full repository history." >&2
  exit 1
}
grep -Fq "const Revision = \"$REVISION\"" playground/evaluate.go
BUILD_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/speedle-playground.XXXXXXXX")
trap 'rm -rf "$BUILD_ROOT"' EXIT
git -C "$ROOT" archive "$REVISION" api pkg 3rdparty go.mod go.sum LICENSE THIRD_PARTY_LICENSES.txt | tar -x -C "$BUILD_ROOT"
mkdir -p "$BUILD_ROOT/website/playground-engine"
cp -R cmd playground go.mod go.sum "$BUILD_ROOT/website/playground-engine/"
OUT="$ROOT/docs/hugo/static/playground"
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
install -m 0644 "$BUILD_ROOT/3rdparty/github.com/Knetic/govaluate/LICENSE" "$OUT/licenses/govaluate-MIT.txt"
install -m 0644 "$BUILD_ROOT/THIRD_PARTY_LICENSES.txt" "$OUT/licenses/Speedle-upstream-third-party.txt"
GOOS=js GOARCH=wasm "$GO_BIN" list -mod=readonly -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' ./cmd/wasm | sort -u | while IFS='|' read -r module directory; do
  [[ -n "$module" && "$module" != speedle-playground && "$module" != github.com/teramoby/speedle-plus ]] || continue
  safe_name=${module//\//_}
  for license in LICENSE LICENSE.txt LICENSE.md COPYING NOTICE; do
    [[ ! -f "$directory/$license" ]] || install -m 0644 "$directory/$license" "$OUT/licenses/${safe_name}_${license}.txt"
  done
done
# Hugo source is authoritative; GitHub Pages serves the checked-in docs copy.
cp -R "$OUT/." "$ROOT/docs/playground/"
mkdir -p "$ROOT/docs/zh/playground"
cp "$ROOT/docs/hugo/static/zh/playground/index.html" "$ROOT/docs/zh/playground/index.html"
cp "$ROOT/docs/hugo/static/index.html" "$ROOT/docs/index.html"
cp "$ROOT/docs/hugo/static/zh/index.html" "$ROOT/docs/zh/index.html"
for stylesheet in site.css navigation.css; do
  cp "$ROOT/docs/hugo/static/css/$stylesheet" "$ROOT/docs/css/$stylesheet"
done
sha256sum "$OUT/speedle.wasm"
