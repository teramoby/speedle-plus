#!/usr/bin/env bash
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
OUTPUT=${1:-"$ROOT/deployment/k8s"}
HELM_BIN=${HELM_BIN:-helm}
mkdir -p "$OUTPUT"
for mode in dev prod; do
  options=("$ROOT/deployment/helm/speedle-$mode")
  if [[ "$mode" == prod ]]; then
    options+=(-f "$ROOT/deployment/helm/speedle-prod/values-example.yaml")
  fi
  {
    printf '# Generated from deployment/helm/speedle-%s; run deployment/render-manifests.sh to update.\n' "$mode"
    "$HELM_BIN" template "speedle-$mode" "${options[@]}"
  } > "$OUTPUT/speedle-$mode.yaml"
done
