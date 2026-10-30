#!/usr/bin/env bash
# Full regression against etcd: own a temporary etcd on random loopback ports,
# never 2379/2380 or a resident service, then run the shared engine.
set -euo pipefail
cd "$(dirname "$0")/../.."
validation_dir=$(mktemp -d "${RUNNER_TEMP:-/tmp}/speedle-etcd.XXXXXXXX")
etcd_pid=''
cleanup() {
  status=$?
  trap - EXIT
  if [[ -n "${etcd_pid}" ]]; then
    kill "${etcd_pid}" 2>/dev/null || true
    wait "${etcd_pid}" || true
  fi
  if (( status != 0 )); then tail -100 "${validation_dir}/etcd.log" 2>/dev/null || true; fi
  rm -rf "${validation_dir}"
  exit "${status}"
}
trap cleanup EXIT
go build -o "${validation_dir}/etcdserver" ./testutil/storetest/etcdserver
"${validation_dir}/etcdserver" "${validation_dir}/data" "${validation_dir}/endpoint" > "${validation_dir}/etcd.log" 2>&1 &
etcd_pid=$!
for _ in $(seq 1 300); do
  [[ -s "${validation_dir}/endpoint" ]] && break
  kill -0 "${etcd_pid}" 2>/dev/null || { echo 'etcd exited before readiness' >&2; exit 1; }
  sleep 0.1
done
[[ -s "${validation_dir}/endpoint" ]] || { echo 'etcd did not become ready' >&2; exit 1; }
SPEEDLE_ETCD_ENDPOINT=$(cat "${validation_dir}/endpoint")
printf 'etcd fixture: %s\n' "${SPEEDLE_ETCD_ENDPOINT}"
export SPEEDLE_ETCD_ENDPOINT
export STORE_TYPE=etcd
unset SPEEDLE_TEST_STORE_PROPS
python3 scripts/ci/run-store-tests.py
