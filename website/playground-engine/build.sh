#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(git rev-parse --show-toplevel)
REVISION=5cccf4c4e50a2b0ab901a9fa65acc39074ed1068
GO_BIN="${GO_BIN:-go}"
export GOTOOLCHAIN=local
if [[ "$("$GO_BIN" env GOVERSION)" != go1.26.3 ]]; then
  echo 'Use Go 1.26.3 for a reproducible build (set GO_BIN=/path/to/go).' >&2
  exit 1
fi
# The adapter imports the real source in this repository, never a JS substitute.
# Website-only changes are allowed; engine source stays pinned until deliberately
# upgraded together with Revision, notices, tests and the checked-in artifact.
SOURCE_PATHS=(api pkg 3rdparty go.mod go.sum)
git -C "$ROOT" cat-file -e "$REVISION^{commit}" || {
  echo "Missing pinned engine commit $REVISION; fetch full repository history." >&2
  exit 1
}
git -C "$ROOT" diff --exit-code "$REVISION" -- "${SOURCE_PATHS[@]}" >/dev/null || {
  echo 'Upstream engine source differs from the pinned revision; refusing to build.' >&2
  exit 1
}
if [[ -n "$(git -C "$ROOT" ls-files --others --exclude-standard -- "${SOURCE_PATHS[@]}")" ]]; then
  echo 'Untracked upstream engine source; refusing to build.' >&2
  exit 1
fi
OUT="$ROOT/docs/playground"
mkdir -p "$OUT/licenses"
"$GO_BIN" test -count=1 -mod=readonly ./playground
GOOS=js GOARCH=wasm "$GO_BIN" build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o "$OUT/speedle.wasm" ./cmd/wasm
chmod 0644 "$OUT/speedle.wasm"
GOROOT=$("$GO_BIN" env GOROOT)
install -m 0644 "$GOROOT/lib/wasm/wasm_exec.js" "$OUT/wasm_exec.js"
install -m 0644 "$GOROOT/LICENSE" "$OUT/licenses/Go-BSD-3-Clause.txt"
install -m 0644 "$ROOT/LICENSE" "$OUT/licenses/Speedle-UPL-1.0.txt"
install -m 0644 "$ROOT/3rdparty/github.com/Knetic/govaluate/LICENSE" "$OUT/licenses/govaluate-MIT.txt"
install -m 0644 "$ROOT/THIRD_PARTY_LICENSES.txt" "$OUT/licenses/Speedle-upstream-third-party.txt"
GOOS=js GOARCH=wasm "$GO_BIN" list -mod=readonly -deps -f '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}' ./cmd/wasm | sort -u | while IFS='|' read -r module directory; do
  [[ -n "$module" && "$module" != speedle-playground && "$module" != github.com/teramoby/speedle-plus ]] || continue
  safe_name=${module//\//_}
  for license in LICENSE LICENSE.txt LICENSE.md COPYING NOTICE; do
    [[ ! -f "$directory/$license" ]] || install -m 0644 "$directory/$license" "$OUT/licenses/${safe_name}_${license}.txt"
  done
done
sha256sum "$OUT/speedle.wasm"
