+++
title = "Deployment"
description = "Recommended deployment topology for Speedle"
weight = 60
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
categories = ["docs"]
bref = "Deploy Speedle in embedded mode or as a service"
+++

## Choose a deployment path

You can try basic policies immediately in the [Playground](/playground/). For a native process, start with the [quick start](/quick-start/).

This guide covers a local Docker demonstration, a single-Pod Kubernetes file-store demonstration, and separate PMS/ADS Pods backed by etcd. Follow one deployment path, then run the authorization checks below. Run commands in Bash from the repository root.

### Prerequisites

- A working Docker engine with BuildKit for image builds. Images compile Go 1.26.10 internally; a native Go 1.26+ installation is needed for the certificate example.
- For Kubernetes: a reachable test cluster, kubectl and permission to create a new namespace. The examples use Kubernetes/kubectl 1.36.4, Helm 4.3.1 and etcd 3.5.35. The etcd example needs a default StorageClass for its 1 GiB PVC.
- curl and free local ports 6733/6734, or override `PMS_PORT`/`ADS_PORT` below.

The automated deployment check uses an isolated [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) 0.33.0 cluster. It does not require a paid cloud cluster.

## Build and distribute images

Initialize these variables once in your terminal. `IMAGE_PREFIX` defaults to `speedle`; for another node to pull your images, set it to your own registry/repository prefix before running this block. `IMAGE_TAG` selects an immutable release or a local development tag. `DEMO` and `NAMESPACE` identify resources created by this guide; use a new namespace.

<!-- deployment-check: setup -->
```bash
export IMAGE_PREFIX="${IMAGE_PREFIX:-speedle}"
export IMAGE_TAG="${IMAGE_TAG:-dev}"
export PMS_IMAGE="${IMAGE_PREFIX}-pms:${IMAGE_TAG}"
export ADS_IMAGE="${IMAGE_PREFIX}-ads:${IMAGE_TAG}"
export PMS_PORT="${PMS_PORT:-6733}"
export ADS_PORT="${ADS_PORT:-6734}"
export DEMO="${DEMO:-speedle-demo-$(date +%s)}"
export NAMESPACE="${NAMESPACE:-$DEMO}"
export DOCS_WORKDIR="${DOCS_WORKDIR:-$(mktemp -d)}"
export TLS_DIR="$DOCS_WORKDIR/tls"
```

Build both images from the repository root. No native binary needs to be copied into the Docker build context.

<!-- deployment-check: build -->
```bash
make image pmsImageRepo="${IMAGE_PREFIX}-pms" pmsImageTag="$IMAGE_TAG" \
  adsImageRepo="${IMAGE_PREFIX}-ads" adsImageTag="$IMAGE_TAG"
```

For a registry-backed deployment, authenticate to your chosen registry and push the exact same repository names and tags. A locally built image is not automatically available to Kubernetes nodes.

<!-- deployment-check: push -->
```bash
make push pmsImageRepo="${IMAGE_PREFIX}-pms" pmsImageTag="$IMAGE_TAG" \
  adsImageRepo="${IMAGE_PREFIX}-ads" adsImageTag="$IMAGE_TAG"
```

## Local Docker demonstration

This file-store demonstration publishes APIs only on the host loopback interface and explicitly enables plaintext inside the demonstration network. Both containers use the same policy volume. Keep this network private. Production connections need the TLS/mTLS configuration described in [security](/docs/security/).

<!-- deployment-check: docker -->
```bash
docker network create "$DEMO"
docker volume create "$DEMO-policies"
docker run -d --name "$DEMO-pms" --network "$DEMO" \
  --cpus=0.5 --memory=256m -p "127.0.0.1:$PMS_PORT:6733" \
  --mount "type=volume,source=$DEMO-policies,target=/var/lib/speedle" \
  "$PMS_IMAGE" --insecure=true
docker run -d --name "$DEMO-ads" --network "$DEMO" \
  --cpus=0.5 --memory=256m -p "127.0.0.1:$ADS_PORT:6734" \
  --mount "type=volume,source=$DEMO-policies,target=/var/lib/speedle" \
  "$ADS_IMAGE" --insecure=true
```

Wait until both containers report `healthy` before running the authorization checks.

<!-- deployment-check: docker-status -->
```bash
docker inspect --format '{{.Name}}: {{.State.Health.Status}}' "$DEMO-pms" "$DEMO-ads"
```

## Kubernetes file-store demonstration

Create a fresh namespace; stop if that name already exists. Ensure the selected images have been pushed to a registry accessible by the nodes, or loaded into your local kind cluster.

<!-- deployment-check: namespace -->
```bash
kubectl create namespace "$NAMESPACE"
```

Install with Helm:

<!-- deployment-check: helm-dev -->
```bash
helm upgrade --install speedle-dev deployment/helm/speedle-dev \
  --namespace "$NAMESPACE" --set-string image.pms="$PMS_IMAGE" \
  --set-string image.ads="$ADS_IMAGE" --wait --timeout 180s
```

Alternatively, apply the independent YAML. This installs the same resources; set the images explicitly and wait for the new rollout. Use this path in a fresh namespace instead of installing the same resources with Helm.

<!-- deployment-check: raw-dev -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/speedle-dev.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/speedle-dev.yaml
kubectl --namespace "$NAMESPACE" set image deployment/speedle-dev pms="$PMS_IMAGE" ads="$ADS_IMAGE"
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-dev --timeout=180s
```

The development chart uses one Pod containing PMS and ADS and one shared file volume. Its default `emptyDir` loses policies when the Pod is replaced. For retained demonstration data, provision a PVC and set `storage.existingClaim`. The chart requires one replica; separate Pods should use the etcd chart.

Forward the local ports in this terminal, then run the authorization checks. Keep the forwarding process running.

<!-- deployment-check: forward-dev -->
```bash
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-dev \
  "$PMS_PORT:6733" "$ADS_PORT:6734" > "$DOCS_WORKDIR/forward.log" 2>&1 &
DEV_FORWARD_PID=$!
```

## Separate PMS/ADS Pods with etcd and mTLS

Use another fresh namespace (set `NAMESPACE` and run the namespace-creation command above). A production deployment should use an etcd cluster with the required durability, availability, backup and access controls. The following single-member PVC-backed example is for reproducible testing; its client connection is HTTP within the test namespace.

<!-- deployment-check: etcd -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/etcd-example.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/etcd-example.yaml
kubectl --namespace "$NAMESPACE" rollout status statefulset/speedle-etcd --timeout=180s
```

Generate 24-hour demonstration certificates with SANs for localhost/loopback, then create the server secret. Client private keys stay on your workstation. For production, supply certificates issued for the actual service DNS names by your trusted CA. Regeneration requires a new output directory.

<!-- deployment-check: certificates -->
```bash
go run ./samples/docs/certificates --out "$TLS_DIR"
kubectl --namespace "$NAMESPACE" create secret generic speedle-server-tls \
  --from-file=server.crt="$TLS_DIR/server.crt" \
  --from-file=server.key="$TLS_DIR/server.key" \
  --from-file=ca.crt="$TLS_DIR/ca.crt"
```

Install the etcd-backed chart. REST and gRPC both require TLS and a trusted client certificate by default.

<!-- deployment-check: helm-prod -->
```bash
helm upgrade --install speedle-prod deployment/helm/speedle-prod \
  --namespace "$NAMESPACE" --set-string image.pms="$PMS_IMAGE" \
  --set-string image.ads="$ADS_IMAGE" \
  --set-string store.etcd.endpoint=http://speedle-etcd:2379 \
  --wait --timeout 180s
```

The independent YAML alternative uses the same default Secret and etcd endpoint:

<!-- deployment-check: raw-prod -->
```bash
kubectl --namespace "$NAMESPACE" apply --dry-run=server -f deployment/k8s/speedle-prod.yaml
kubectl --namespace "$NAMESPACE" apply -f deployment/k8s/speedle-prod.yaml
kubectl --namespace "$NAMESPACE" set image deployment/speedle-prod-pms pms="$PMS_IMAGE"
kubectl --namespace "$NAMESPACE" set image deployment/speedle-prod-ads ads="$ADS_IMAGE"
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-prod-pms --timeout=180s
kubectl --namespace "$NAMESPACE" rollout status deployment/speedle-prod-ads --timeout=180s
```

For an existing TLS-enabled etcd cluster, create a Secret containing the etcd client certificate/key and CA, then configure these values. This is separate from the certificates protecting Speedle's APIs:

```yaml
store:
  type: etcd
  etcd:
    endpoint: https://your-etcd-service:2379
    etcdClientCertSecret: etcd-client-tls
    certFile: client.crt
    keyFile: client.key
    trustedCAFile: ca.crt
```

Forward both services to localhost:

<!-- deployment-check: forward-prod -->
```bash
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-prod-pms \
  "$PMS_PORT:6733" > "$DOCS_WORKDIR/pms-forward.log" 2>&1 &
PMS_FORWARD_PID=$!
kubectl --namespace "$NAMESPACE" port-forward --address 127.0.0.1 svc/speedle-prod-ads \
  "$ADS_PORT:6734" > "$DOCS_WORKDIR/ads-forward.log" 2>&1 &
ADS_FORWARD_PID=$!
```

## Verify authorization

For the Docker/file-store demonstration, select HTTP:

<!-- deployment-check: http-client -->
```bash
TLS_FLAGS=()
export PMS_URL="http://127.0.0.1:$PMS_PORT"
export ADS_URL="http://127.0.0.1:$ADS_PORT"
```

For the mTLS deployment, select HTTPS with certificate verification and a trusted client certificate:

<!-- deployment-check: https-client -->
```bash
TLS_FLAGS=(--cacert "$TLS_DIR/ca.crt" --cert "$TLS_DIR/client.crt" --key "$TLS_DIR/client.key")
export PMS_URL="https://127.0.0.1:$PMS_PORT"
export ADS_URL="https://127.0.0.1:$ADS_PORT"
```

Run only the client configuration matching your deployment. Wait for the forwarding log to show the local listener before sending requests. Load the bookstore fixture, which grants Alan permission to download the book:

<!-- deployment-check: create -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  --data-binary @samples/docs/bookstore.json "$PMS_URL/policy-mgmt/v1/service"
```

Watch propagation is asynchronous. Once ADS has received the update, Alan returns `allowed=true, reason=0`:

<!-- deployment-check: allow -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Alan"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  "$ADS_URL/authz-check/v1/is-allowed"
```

Bob returns `allowed=false, reason=3`:

<!-- deployment-check: deny -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -H 'Content-Type: application/json' \
  -d '{"subject":{"principals":[{"type":"user","name":"Bob"}]},"serviceName":"onlineBookStore","resource":"/books/HarryPotter","action":"download"}' \
  "$ADS_URL/authz-check/v1/is-allowed"
```

Read back and delete the test service:

<!-- deployment-check: read -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" "$PMS_URL/policy-mgmt/v1/service/onlineBookStore"
```

<!-- deployment-check: delete -->
```bash
curl --fail --silent --show-error "${TLS_FLAGS[@]}" -X DELETE "$PMS_URL/policy-mgmt/v1/service/onlineBookStore"
```

## Health checks and cleanup

Container probes use a separate loopback-only HTTP listener: PMS `127.0.0.1:8081`, ADS `127.0.0.1:8082`. `/livez` indicates that the process is responsive; `/readyz` checks backend availability with a bounded timeout. Backend failure makes readiness fail without immediately restarting a live process. The business API ports retain TLS/mTLS. These health ports are not published by Services. See the [Kubernetes probe reference](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/).

Stop and remove the Docker demonstration and its policy data when finished:

<!-- deployment-check: docker-cleanup -->
```bash
docker stop --time 15 "$DEMO-pms" "$DEMO-ads"
docker rm "$DEMO-pms" "$DEMO-ads"
docker volume rm "$DEMO-policies"
docker network rm "$DEMO"
```

For Kubernetes, stop the forwarding process(es) used by your chosen path:

<!-- deployment-check: forward-dev-cleanup -->
```bash
kill "$DEV_FORWARD_PID"
wait "$DEV_FORWARD_PID" || true
```

<!-- deployment-check: forward-prod-cleanup -->
```bash
kill "$PMS_FORWARD_PID" "$ADS_FORWARD_PID"
wait "$PMS_FORWARD_PID" "$ADS_FORWARD_PID" || true
```

Delete the namespace created for this example; this also deletes its PVC-backed test data.

<!-- deployment-check: namespace-cleanup -->
```bash
kubectl delete namespace "$NAMESPACE" --wait=true
```
