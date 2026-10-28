#!/usr/bin/env bash
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
OUTPUT=${1:-"$ROOT/deployment/k8s"}
HELM_BIN=${HELM_BIN:-helm}
mkdir -p "$OUTPUT"
for mode in dev prod; do
  {
    printf '# Generated from deployment/helm/speedle-%s; run deployment/render-manifests.sh to update.\n' "$mode"
    "$HELM_BIN" template "speedle-$mode" "$ROOT/deployment/helm/speedle-$mode"
  } > "$OUTPUT/speedle-$mode.yaml"
done
