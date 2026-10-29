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

## Choose a deployment mode

Use [embedded mode](../deployment-embeded/) to run the evaluator in your Go process. Use standalone PMS and ADS when applications call Speedle over REST or gRPC. The [Quick Start](../../quick-start/) provides a complete local policy-management and authorization walkthrough.

## Build with current Go

Use Go 1.26.10 or Go 1.27.3 (or a newer patch on the same release branch) (Go 1.27.3 is recommended), Git and a shell on Linux or macOS. Use Go modules; the checkout can be anywhere writable. No custom `GOROOT`, `GOPATH`, `dep` or GOPATH source layout is needed.

<!-- deployment-check: source-build -->
```bash
git clone --depth 1 https://github.com/teramoby/speedle-plus.git
cd speedle-plus
mkdir -p bin
go build -o ./bin/ ./cmd/spctl ./cmd/speedle-pms ./cmd/speedle-ads
```

The executables are `bin/spctl`, `bin/speedle-pms` and `bin/speedle-ads`. From the checkout root, use one shared file for the local example:

<!-- deployment-check: pms -->
```bash
./bin/speedle-pms --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6733 --insecure=true
```

Keep PMS running; start ADS in another terminal in the same checkout:

<!-- deployment-check: ads -->
```bash
./bin/speedle-ads --store-type file --filestore-loc ./docs-policies.json --endpoint 127.0.0.1:6734 --insecure=true
```

Use a fresh policy file; PMS creates it if absent. Follow [Quick Start](../../quick-start/) for creation and Allow/Deny requests, then stop both services with Ctrl+C. gRPC is disabled unless explicitly enabled with `--grpc-endpoint`. Plain HTTP is for this loopback example; see [Security](../security/) for TLS and client authentication.

## Build container images

For an optional Linux/amd64 container deployment, first build static Linux binaries from the checkout root:

<!-- deployment-check: linux-build -->
```bash
mkdir -p ./bin/linux-amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/linux-amd64/ ./cmd/speedle-pms ./cmd/speedle-ads
```

Save this as `bin/linux-amd64/Dockerfile.pms`. Create `Dockerfile.ads` alongside it by replacing both occurrences of `speedle-pms` with `speedle-ads`. The binaries run directly without the historical image entrypoint scripts:

<!-- deployment-template: image -->
```dockerfile
FROM scratch
COPY speedle-pms /speedle-pms
USER 65532:65532
ENTRYPOINT ["/speedle-pms"]
```

Use a current Docker Engine with BuildKit and a registry you can push to. Set `PMS_IMAGE` and `ADS_IMAGE` to your own complete image names/tags, authenticate to that registry, then build and publish:

<!-- deployment-template: image-build -->
```bash
docker build --platform linux/amd64 -f ./bin/linux-amd64/Dockerfile.pms -t "$PMS_IMAGE" ./bin/linux-amd64
docker build --platform linux/amd64 -f ./bin/linux-amd64/Dockerfile.ads -t "$ADS_IMAGE" ./bin/linux-amd64
docker push "$PMS_IMAGE"
docker push "$ADS_IMAGE"
```

These images target Linux/amd64 nodes. Build an architecture matching your nodes if different. Provide CA files explicitly for TLS endpoints when using this minimal image.

## Kubernetes development example

Use a current Kubernetes cluster and compatible `kubectl`. Save the following as `speedle-dev.yaml`. It uses `apps/v1`, one pod with PMS/ADS sharing one file, a non-root application UID and an internal ClusterIP service. This is an isolated development example: `emptyDir` policies are lost when the pod is replaced, and HTTP endpoints need cluster access controls. Use a private-registry pull secret if required by your registry.

<!-- deployment-template: kubernetes -->
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: speedle-dev
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: speedle-dev
  template:
    metadata:
      labels:
        app: speedle-dev
    spec:
      initContainers:
        - name: policy-file
          image: busybox:1.37.0
          command: ["sh", "-c", "printf '{}\n' > /data/policies.json; chown 65532:65532 /data/policies.json; chmod 600 /data/policies.json"]
          volumeMounts:
            - name: policies
              mountPath: /data
      containers:
        - name: pms
          image: registry.example.com/speedle-pms:demo
          args: ["--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6733", "--insecure=true"]
          ports:
            - containerPort: 6733
          readinessProbe:
            tcpSocket:
              port: 6733
          volumeMounts:
            - name: policies
              mountPath: /data
        - name: ads
          image: registry.example.com/speedle-ads:demo
          args: ["--store-type", "file", "--filestore-loc", "/data/policies.json", "--endpoint", "0.0.0.0:6734", "--insecure=true"]
          ports:
            - containerPort: 6734
          readinessProbe:
            tcpSocket:
              port: 6734
          volumeMounts:
            - name: policies
              mountPath: /data
      volumes:
        - name: policies
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: speedle-dev
spec:
  type: ClusterIP
  selector:
    app: speedle-dev
  ports:
    - name: pms
      port: 6733
      targetPort: 6733
    - name: ads
      port: 6734
      targetPort: 6734
```

Keep `PMS_IMAGE` and `ADS_IMAGE` set to the images you published. Substitute them locally before applying; keep port-forward running while using Quick Start clients:

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$PMS_IMAGE" ads="$ADS_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## Production deployments

Use durable policy storage, protect management and decision endpoints, and configure TLS/mTLS for clients and the policy backend. [Pluggable Storage](../store/) documents the current store configuration; [Security](../security/) covers current transport flags. Size and operate the etcd deployment using the current etcd documentation, not the retired etcd-operator example.

The repository contains historical Helm/Kubernetes examples. The old Helm 2 `helm install -n ...`, `stable/etcd-operator`, `apps/v1beta2`, and `r.authz.fun` image instructions are not the deployment flow above. Container build/push, cluster admission, registry access, storage and rollout depend on your environment; validate them in your target cluster before production use.
