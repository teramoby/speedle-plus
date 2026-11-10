#!/usr/bin/env bash
# helm lint and helm template for the file, etcd and MongoDB stores.
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
chart="${root}/deployment/helm/speedle"
work=$(mktemp -d)
trap 'rm -rf "${work}"' EXIT

helm lint "${chart}"

render() {
  helm template speedle "${chart}" "$@"
}

file=$(render)
printf '%s\n' "${file}" > "${work}/file.yaml"
grep -F 'speedle/speedle:1.7.0' "${work}/file.yaml" >/dev/null
grep -F 'filestore-loc' "${work}/file.yaml" >/dev/null
grep -F '/data/policies.json' "${work}/file.yaml" >/dev/null
grep -F 'runAsNonRoot: true' "${work}/file.yaml" >/dev/null
grep -F 'runAsUser: 65532' "${work}/file.yaml" >/dev/null
grep -F 'kind: PersistentVolumeClaim' "${work}/file.yaml" >/dev/null
grep -F '/policy-mgmt/v1/service' "${work}/file.yaml" >/dev/null
grep -F '/readyz' "${work}/file.yaml" >/dev/null
grep -F '/healthz' "${work}/file.yaml" >/dev/null
grep -F 'speedle-pms' "${work}/file.yaml" >/dev/null
grep -F 'speedle-ads' "${work}/file.yaml" >/dev/null
deps=$(grep -c 'kind: Deployment' "${work}/file.yaml")
[[ "${deps}" == "2" ]]

claim=$(awk '/claimName:/ { print $2; exit }' "${work}/file.yaml")
grep -F "name: ${claim}" "${work}/file.yaml" >/dev/null

etcd=$(render \
  --set store.type=etcd \
  --set store.etcd.endpoint=http://etcd.example:2379 \
  --set store.etcd.tls.secretName=etcd-client \
  --set store.etcd.tls.certFile=tls.crt \
  --set store.etcd.tls.keyFile=tls.key \
  --set store.etcd.tls.caFile=ca.crt)
printf '%s\n' "${etcd}" > "${work}/etcd.yaml"
grep -F 'etcdstore-endpoint' "${work}/etcd.yaml" >/dev/null
grep -F 'http://etcd.example:2379' "${work}/etcd.yaml" >/dev/null
grep -F 'etcdstore-keyprefix' "${work}/etcd.yaml" >/dev/null
grep -F '/speedle_ps/' "${work}/etcd.yaml" >/dev/null
grep -F 'etcdstore-tls-cert' "${work}/etcd.yaml" >/dev/null
grep -F 'etcdstore-tls-key' "${work}/etcd.yaml" >/dev/null
grep -F 'etcdstore-tls-ca' "${work}/etcd.yaml" >/dev/null
if grep -F 'kind: PersistentVolumeClaim' "${work}/etcd.yaml" >/dev/null; then
  echo "etcd render must not create a policy volume claim" >&2
  exit 1
fi

mongo=$(render \
  --set store.type=mongodb \
  --set store.mongodb.uri='mongodb://mongo.example:27017/?replicaSet=rs0' \
  --set store.mongodb.database=speedle_demo)
printf '%s\n' "${mongo}" > "${work}/mongo.yaml"
grep -F 'mongostore_uri' "${work}/mongo.yaml" >/dev/null
grep -F 'replicaSet=rs0' "${work}/mongo.yaml" >/dev/null
grep -F 'mongostore_database' "${work}/mongo.yaml" >/dev/null
grep -F 'speedle_demo' "${work}/mongo.yaml" >/dev/null
if grep -F 'kind: PersistentVolumeClaim' "${work}/mongo.yaml" >/dev/null; then
  echo "mongodb render must not create a policy volume claim" >&2
  exit 1
fi

scaled=$(render --set ads.replicaCount=3)
printf '%s\n' "${scaled}" > "${work}/scaled.yaml"
grep -F 'replicas: 3' "${work}/scaled.yaml" >/dev/null

if render --set pms.replicaCount=2 >"${work}/two-pms.yaml" 2>"${work}/two-pms.err"; then
  echo "file store accepted two PMS replicas" >&2
  exit 1
fi
grep -F 'one PMS' "${work}/two-pms.err" >/dev/null

if render --set store.type=etcd >"${work}/etcd-empty.yaml" 2>"${work}/etcd-empty.err"; then
  echo "etcd store accepted an empty endpoint" >&2
  exit 1
fi
grep -F 'store.etcd.endpoint is required' "${work}/etcd-empty.err" >/dev/null

if render --set store.type=mongodb >"${work}/mongo-empty.yaml" 2>"${work}/mongo-empty.err"; then
  echo "mongodb store accepted an empty URI" >&2
  exit 1
fi
grep -F 'store.mongodb.uri is required' "${work}/mongo-empty.err" >/dev/null

version=$(awk '/^version:/ { print $2; exit }' "${chart}/Chart.yaml")
app=$(awk '/^appVersion:/ { gsub(/"/, "", $2); print $2; exit }' "${chart}/Chart.yaml")
helm package "${chart}" --version "${version}" --app-version "${app}" --destination "${work}"
helm lint "${work}/speedle-${version}.tgz"

echo "helm lint and template checks passed"
