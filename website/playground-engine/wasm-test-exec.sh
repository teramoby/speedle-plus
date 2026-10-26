#!/usr/bin/env bash
set -euo pipefail
# The official launcher copies process.env into Go WASM's small argv/env area.
# CI has a much larger environment than the browser or a developer shell. Keep
# build/network settings in the Go parent, and only pass test runtime settings.
NODE_BIN=$(command -v node)
exec env -i TMPDIR="${TMPDIR:-/tmp}" TZ=UTC "$NODE_BIN" --stack-size=8192 "${PLAYGROUND_GOROOT:?}/lib/wasm/wasm_exec_node.js" "$@"
