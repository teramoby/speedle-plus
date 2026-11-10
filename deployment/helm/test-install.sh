#!/usr/bin/env bash
# Install the chart on the current cluster, write one policy with spctl from
# the image, and require ADS to allow it. The cluster must already be able to
# provision the default PersistentVolumeClaim, and the chart image must already
# be present on the nodes.
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
chart="${root}/deployment/helm/speedle"
release=${RELEASE:-speedle}
ns=${NAMESPACE:-speedle}
image=${IMAGE:-}

if [[ -z "${image}" ]]; then
  image=$(helm template "${release}" "${chart}" --namespace "${ns}" | awk '/^[[:space:]]*image:/ { gsub(/"/, "", $2); print $2; exit }')
fi
[[ -n "${image}" ]] || { echo "Could not determine the chart image." >&2; exit 1; }

# The chart denies ingress to PMS from other pods. This client pod is the
# policy administrator for the install, so the check opts out of that policy.
helm upgrade --install "${release}" "${chart}" \
  --namespace "${ns}" --create-namespace \
  --set networkPolicy.pms.enabled=false \
  --wait --timeout 5m

mapfile -t pods < <(kubectl get pods -n "${ns}" -l "app.kubernetes.io/instance=${release}" -o name)
[[ "${#pods[@]}" -eq 2 ]] || { echo "Expected PMS and ADS pods, found ${#pods[@]}." >&2; kubectl get pods -n "${ns}"; exit 1; }
kubectl wait -n "${ns}" --for=condition=Ready pod -l "app.kubernetes.io/instance=${release}" --timeout=180s

pms_svc=$(kubectl get svc -n "${ns}" -l "app.kubernetes.io/instance=${release},app.kubernetes.io/component=pms" -o jsonpath='{.items[0].metadata.name}')
ads_svc=$(kubectl get svc -n "${ns}" -l "app.kubernetes.io/instance=${release},app.kubernetes.io/component=ads" -o jsonpath='{.items[0].metadata.name}')
[[ -n "${pms_svc}" && -n "${ads_svc}" ]] || { echo "PMS or ADS Service is missing." >&2; exit 1; }
pms_port=$(kubectl get svc -n "${ns}" "${pms_svc}" -o jsonpath='{.spec.ports[0].port}')
ads_port=$(kubectl get svc -n "${ns}" "${ads_svc}" -o jsonpath='{.spec.ports[0].port}')
endpoint="http://${pms_svc}:${pms_port}/policy-mgmt/v1/"

run_spctl() {
  local name=$1
  shift
  kubectl delete pod "${name}" -n "${ns}" --ignore-not-found >/dev/null
  kubectl run "${name}" -n "${ns}" --restart=Never \
    --image="${image}" --image-pull-policy=IfNotPresent \
    --command -- /dispatch "$@"
  if ! kubectl wait -n "${ns}" --for=jsonpath='{.status.phase}'=Succeeded "pod/${name}" --timeout=90s; then
    kubectl describe pod -n "${ns}" "${name}" >&2 || true
    kubectl logs -n "${ns}" "${name}" >&2 || true
    return 1
  fi
  kubectl logs -n "${ns}" "${name}"
}

run_spctl spctl-service spctl --pms-endpoint "${endpoint}" create service smoke
run_spctl spctl-policy spctl --pms-endpoint "${endpoint}" create policy \
  -c "grant user smoke-user read smoke-resource" --service-name=smoke

pf_log=$(mktemp)
kubectl port-forward -n "${ns}" "svc/${ads_svc}" "${ads_port}:${ads_port}" >"${pf_log}" 2>&1 &
pf_pid=$!
cleanup() {
  kill "${pf_pid}" >/dev/null 2>&1 || true
  wait "${pf_pid}" 2>/dev/null || true
  rm -f "${pf_log}"
}
trap cleanup EXIT

deadline=$((SECONDS + 30))
until curl --fail --silent --show-error --connect-timeout 1 --max-time 2 \
  "http://127.0.0.1:${ads_port}/readyz" >/dev/null; do
  if [[ "${SECONDS}" -ge "${deadline}" ]]; then
    echo "ADS port-forward did not become ready." >&2
    cat "${pf_log}" >&2 || true
    exit 1
  fi
  sleep 1
done

body=$(mktemp)
decision_deadline=$((SECONDS + 30))
while true; do
  if curl --fail --silent --show-error --connect-timeout 1 --max-time 5 \
    --header 'Content-Type: application/json' \
    --data '{"subject":{"principals":[{"type":"user","name":"smoke-user"}]},"serviceName":"smoke","resource":"smoke-resource","action":"read"}' \
    "http://127.0.0.1:${ads_port}/authz-check/v1/is-allowed" >"${body}"; then
    if python3 -c 'import json, sys; result = json.load(sys.stdin); assert result.get("allowed") is True and result.get("reason") == 0, result' <"${body}"; then
      cat "${body}"
      rm -f "${body}"
      exit 0
    fi
  fi
  if [[ "${SECONDS}" -ge "${decision_deadline}" ]]; then
    echo "ADS did not return allowed: true." >&2
    cat "${body}" >&2 || true
    kubectl logs -n "${ns}" -l "app.kubernetes.io/instance=${release}" --all-containers --tail=200 >&2 || true
    rm -f "${body}"
    exit 1
  fi
  sleep 1
done
