#!/usr/bin/env bash
# Verify that offline js/wasm engines do not retain network transports.
set -euo pipefail
cd "$(dirname "$0")/../.."

for package in ./pkg/eval ./pkg/cfg; do
  dependencies=$(GOOS=js GOARCH=wasm go list -deps -f '{{.ImportPath}}' "${package}")
  while IFS= read -r dependency; do
    case "${dependency}" in
      net/http|net/http/*|crypto/tls|crypto/x509|github.com/teramoby/speedle-plus/pkg/assertion)
        printf '%s unexpectedly depends on %s for js/wasm\n' "${package}" "${dependency}" >&2
        exit 1
        ;;
    esac
  done <<< "${dependencies}"
  count=$(printf '%s\n' "${dependencies}" | wc -l | tr -d ' ')
  printf '%s: %s js/wasm dependencies; HTTP/TLS transports absent\n' "${package}" "${count}"
done
