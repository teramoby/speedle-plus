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
policies=$(grep -c 'kind: NetworkPolicy' "${work}/file.yaml")
[[ "${policies}" == "1" ]]
grep -F 'ingress: []' "${work}/file.yaml" >/dev/null
if grep -F 'from:' "${work}/file.yaml" >/dev/null; then
  echo "default PMS policy must deny all pod ingress" >&2
  exit 1
fi

open=$(render --set networkPolicy.pms.enabled=false)
printf '%s\n' "${open}" > "${work}/open.yaml"
if grep -F 'kind: NetworkPolicy' "${work}/open.yaml" >/dev/null; then
  echo "networkPolicy.pms.enabled=false still rendered a NetworkPolicy" >&2
  exit 1
fi

allowed=$(render --set-json 'networkPolicy.pms.ingress=[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"policy-admin"}}}]')
printf '%s\n' "${allowed}" > "${work}/allowed.yaml"
grep -F 'policy-admin' "${work}/allowed.yaml" >/dev/null
grep -F 'namespaceSelector:' "${work}/allowed.yaml" >/dev/null

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
grep -F 'store.mongodb.uri or store.mongodb.existingSecret is required' "${work}/mongo-empty.err" >/dev/null

if render --set store.type=mongodb \
  --set store.mongodb.uri='mongodb://user:secret@mongo.example:27017/?replicaSet=rs0' \
  >"${work}/mongo-secret.yaml" 2>"${work}/mongo-secret.err"; then
  echo "mongodb store accepted a URI with credentials" >&2
  exit 1
fi
grep -F 'must not include credentials' "${work}/mongo-secret.err" >/dev/null

if render --set store.type=mongodb \
  --set store.mongodb.uri='mongodb://mongo.example:27017/?replicaSet=rs0' \
  --set store.mongodb.existingSecret=mongo-uri \
  >"${work}/mongo-both.yaml" 2>"${work}/mongo-both.err"; then
  echo "mongodb store accepted both a URI and a Secret" >&2
  exit 1
fi
grep -F 'not both' "${work}/mongo-both.err" >/dev/null

mongo_secret=$(render \
  --set store.type=mongodb \
  --set store.mongodb.existingSecret=mongo-uri \
  --set store.mongodb.database=speedle_demo)
printf '%s\n' "${mongo_secret}" > "${work}/mongo-secret-ref.yaml"
grep -F 'SPDL_MONGOSTORE_URI' "${work}/mongo-secret-ref.yaml" >/dev/null
grep -F 'secretKeyRef:' "${work}/mongo-secret-ref.yaml" >/dev/null
grep -F 'mongo-uri' "${work}/mongo-secret-ref.yaml" >/dev/null
grep -F 'mongostore_database' "${work}/mongo-secret-ref.yaml" >/dev/null
if grep -F 'mongostore_uri' "${work}/mongo-secret-ref.yaml" >/dev/null; then
  echo "credentialed MongoDB URI was passed as a flag" >&2
  exit 1
fi
if grep -F 'kind: PersistentVolumeClaim' "${work}/mongo-secret-ref.yaml" >/dev/null; then
  echo "mongodb secret render must not create a policy volume claim" >&2
  exit 1
fi

if render --set store.file.persistence.accessMode=ReadOnlyMany \
  >"${work}/rom.yaml" 2>"${work}/rom.err"; then
  echo "file store accepted ReadOnlyMany" >&2
  exit 1
fi
grep -E 'ReadOnlyMany|ReadWriteOnce' "${work}/rom.err" >/dev/null

https=$(render --set tls.enabled=true --set tls.secretName=speedle-tls)
printf '%s\n' "${https}" > "${work}/https.yaml"
grep -F 'scheme: HTTPS' "${work}/https.yaml" >/dev/null
grep -F 'httpGet:' "${work}/https.yaml" >/dev/null
if grep -F 'force-client-cert=true' "${work}/https.yaml" >/dev/null; then
  echo "TLS without forceClientCert rendered client-certificate enforcement" >&2
  exit 1
fi

mtls=$(render \
  --set tls.enabled=true \
  --set tls.secretName=speedle-tls \
  --set tls.clientCAFile=ca.crt \
  --set tls.forceClientCert=true)
printf '%s\n' "${mtls}" > "${work}/mtls.yaml"
grep -F 'force-client-cert=true' "${work}/mtls.yaml" >/dev/null
grep -F -- '--client-cert' "${work}/mtls.yaml" >/dev/null
tcp=$(grep -c 'tcpSocket:' "${work}/mtls.yaml")
[[ "${tcp}" == "6" ]]
if grep -F 'httpGet:' "${work}/mtls.yaml" >/dev/null; then
  echo "mutual TLS render used an HTTP probe" >&2
  exit 1
fi

long=$(python3 -c 'print("a" * 63)')
long_render=$(render --set "fullnameOverride=${long}")
printf '%s\n' "${long_render}" > "${work}/long.yaml"
deploy_names=()
while IFS= read -r name; do
  deploy_names+=("${name}")
done < <(awk '$1 == "kind:" && $2 == "Deployment" { want = 1; next } want && $1 == "name:" { print $2; want = 0 }' "${work}/long.yaml")
[[ "${#deploy_names[@]}" == "2" ]]
[[ "${deploy_names[0]}" != "${deploy_names[1]}" ]]
[[ "${deploy_names[0]}" == *-pms && "${deploy_names[1]}" == *-ads ]]
[[ "${#deploy_names[0]}" -le 63 && "${#deploy_names[1]}" -le 63 ]]
claim_name=$(awk '$1 == "kind:" && $2 == "PersistentVolumeClaim" { want = 1; next } want && $1 == "name:" { print $2; exit }' "${work}/long.yaml")
[[ "${claim_name}" == *-policies ]]
[[ "${#claim_name}" -le 63 ]]

version=$(awk '/^version:/ { print $2; exit }' "${chart}/Chart.yaml")
app=$(awk '/^appVersion:/ { gsub(/"/, "", $2); print $2; exit }' "${chart}/Chart.yaml")
helm package "${chart}" --version "${version}" --app-version "${app}" --destination "${work}"
helm lint "${work}/speedle-${version}.tgz"

echo "helm lint and template checks passed"
