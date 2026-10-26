#!/usr/bin/env bash
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/speedle-hugo.XXXXXXXX")
trap 'rm -rf "$WORK"' EXIT
HUGO_BIN="${HUGO_BIN:-}"
if [[ -z "$HUGO_BIN" ]]; then
  # The existing site theme pins Hugo 0.48. Never replace the host's toolchain.
  [[ "$(uname -s)/$(uname -m)" == Linux/x86_64 ]] || {
    echo 'Set HUGO_BIN to a compatible Hugo 0.48 binary, or run this check in Linux CI.' >&2
    exit 1
  }
  curl -fsSL --retry 3 --retry-all-errors https://github.com/gohugoio/hugo/releases/download/v0.48/hugo_0.48_Linux-64bit.tar.gz -o "$WORK/hugo.tar.gz"
  echo "1d26dab6445fc40aa23ecd8d49cd6fdbe8b06d722907bc97b3d32e385555b2df  $WORK/hugo.tar.gz" | sha256sum -c -
  tar -xzf "$WORK/hugo.tar.gz" -C "$WORK"
  HUGO_BIN="$WORK/hugo"
fi
"$HUGO_BIN" version | grep -E 'v0\.48([ /-]|$)'
"$HUGO_BIN" --source "$ROOT/docs/hugo" --destination "$WORK/site"
node "$ROOT/website/playground-engine/tests/hugo-output.test.cjs" "$WORK/site"
