#!/usr/bin/env bash
# Run spctl --help, then start PMS and ADS, from the local image speedle:smoke
# and one shared volume. The Release workflow and the pull-request image smoke
# job both call this script. The image and dist/spctl_linux_<arch> must already
# exist.
#
# Docker accepts connections on the published port before the process listens,
# and that proxy resets the connection. Retry every error until the deadline.
# Each attempt has its own limit: a connection that is accepted and then left
# open must not wait forever, or this deadline is never reached.
# SMOKE_READY_TIMEOUT is that deadline in seconds (default 30).
set -euo pipefail

ready_timeout="${SMOKE_READY_TIMEOUT:-30}"
[[ "${ready_timeout}" =~ ^[0-9]+$ ]] || { echo "SMOKE_READY_TIMEOUT must be a number of seconds." >&2; exit 1; }

host_arch=$(go env GOHOSTARCH)
case "${host_arch}" in
  amd64|arm64) ;;
  *) echo "Unsupported release smoke host architecture: ${host_arch}" >&2; exit 1 ;;
esac

spctl="${SPEEDLE_SMOKE_SPCTL:-dist/spctl_linux_${host_arch}}"
[[ -x "${spctl}" ]] || { echo "Missing host spctl binary: ${spctl}" >&2; exit 1; }
image="${SPEEDLE_SMOKE_IMAGE:-speedle:smoke}"

on_exit() {
  status=$?
  if [[ "${status}" -ne 0 ]]; then
    echo "Image smoke failed." >&2
    docker ps -a >&2 || true
    echo "----- speedle-smoke-pms -----" >&2
    docker logs speedle-smoke-pms >&2 || true
    echo "----- speedle-smoke-ads -----" >&2
    docker logs speedle-smoke-ads >&2 || true
  fi
  docker rm -f speedle-smoke-pms speedle-smoke-ads >/dev/null 2>&1 || true
  docker volume rm speedle-smoke >/dev/null 2>&1 || true
  exit "${status}"
}
trap on_exit EXIT

wait_ready() {
  url=$1
  shift
  deadline=$((SECONDS + ready_timeout))
  body_file=$(mktemp)
  err_file=$(mktemp)
  while true; do
    remaining=$((deadline - SECONDS))
    if [[ "${remaining}" -le 0 ]]; then
      echo "Timed out after ${ready_timeout}s waiting for ${url}" >&2
      rm -f "${body_file}" "${err_file}"
      return 1
    fi
    # --max-time 0 disables the limit. Cap the attempt at 5s and at the
    # seconds still left on the deadline.
    attempt="${remaining}"
    if [[ "${attempt}" -gt 5 ]]; then
      attempt=5
    fi
    if curl --fail --silent --show-error --connect-timeout 1 --max-time "${attempt}" "$@" "${url}" >"${body_file}" 2>"${err_file}"; then
      cat "${body_file}"
      rm -f "${body_file}" "${err_file}"
      return 0
    fi
    cat "${err_file}" >&2 || true
    remaining=$((deadline - SECONDS))
    if [[ "${remaining}" -le 0 ]]; then
      echo "Timed out after ${ready_timeout}s waiting for ${url}" >&2
      rm -f "${body_file}" "${err_file}"
      return 1
    fi
    sleep 1
  done
}

# The first argument selects the binary, so this exercises the dispatch
# entrypoint and spctl before either service starts.
docker run --rm "${image}" spctl --help >/dev/null

docker rm -f speedle-smoke-pms speedle-smoke-ads >/dev/null 2>&1 || true
docker volume rm speedle-smoke >/dev/null 2>&1 || true
docker volume create speedle-smoke >/dev/null
docker run -d --name speedle-smoke-pms -p 127.0.0.1:6733:6733 -v speedle-smoke:/data \
  "${image}" speedle-pms --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
wait_ready http://127.0.0.1:6733/policy-mgmt/v1/service >/dev/null
# Write through PMS, then read the same volume through both the API and
# ADS. Starting an empty file store alone does not catch volume or
# permission failures, and ADS proves the persisted policy is usable.
"${spctl}" --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create service smoke
"${spctl}" --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ create policy \
  -c "grant user smoke-user read smoke-resource" --service-name=smoke
"${spctl}" --pms-endpoint http://127.0.0.1:6733/policy-mgmt/v1/ get service --all | \
  python3 -c 'import json, sys; services = json.load(sys.stdin); assert any(s.get("name") == "smoke" and len(s.get("policies", [])) == 1 for s in services), services'
docker stop speedle-smoke-pms >/dev/null
docker run -d --name speedle-smoke-ads -p 127.0.0.1:6734:6734 -v speedle-smoke:/data \
  "${image}" speedle-ads --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
decision=$(wait_ready \
  http://127.0.0.1:6734/authz-check/v1/is-allowed \
  --header 'Content-Type: application/json' \
  --data '{"subject":{"principals":[{"type":"user","name":"smoke-user"}]},"serviceName":"smoke","resource":"smoke-resource","action":"read"}')
python3 -c 'import json, sys; result = json.load(sys.stdin); assert result.get("allowed") is True and result.get("reason") == 0, result' <<< "${decision}"
docker stop speedle-smoke-ads >/dev/null
