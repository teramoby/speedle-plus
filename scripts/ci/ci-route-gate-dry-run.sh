#!/usr/bin/env bash
# Evaluate the latest completed Speedle CI run for a PR without publishing.
set -euo pipefail
if (($# != 1)); then
  echo "Usage: $0 PR_NUMBER" >&2
  exit 2
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec node "${script_dir}/ci-route-gate.cjs" --pr "$1" --dry-run
