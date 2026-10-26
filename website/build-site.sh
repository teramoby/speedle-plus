#!/usr/bin/env bash
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
OUT=${1:?Pass an explicit output directory}
HUGO_BIN=${HUGO_BIN:-hugo}
"$HUGO_BIN" version | grep -E 'v0\.48([ /-]|$)'
"$HUGO_BIN" --source "$ROOT/docs/hugo" --destination "$OUT"
python3 "$ROOT/website/localize-links.py" "$OUT"
