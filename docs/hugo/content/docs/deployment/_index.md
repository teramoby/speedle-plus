+++
title = "Deployment"
description = "Run Speedle embedded in a Go application, from the published container image, or as standalone PMS and ADS services."
weight = 304
draft = false
toc = true
linksection = true
tocheading = "h2"
categories = ["docs"]
bref = "Embedded mode, the Docker image, source builds, and standalone PMS and ADS services."
+++

Speedle runs inside a Go application, from one published container image, or as standalone PMS and ADS services.

- [Embedded mode](/docs/deployment/embedded/) runs the evaluator in your Go process.
- [Docker image](/docs/deployment/docker/) runs `speedle-pms`, `speedle-ads` and `spctl` from `docker.io/speedle/speedle` or `ghcr.io/teramoby/speedle`.

## Choose a deployment mode

Use [embedded mode](/docs/deployment/embedded/) to run the evaluator in your Go process. Use the [Docker image](/docs/deployment/docker/) or standalone PMS and ADS when applications call Speedle over REST or gRPC. The [Quick Start](../../quick-start/) provides a complete local policy-management and authorization walkthrough.

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

## Release binaries and container images

Each release published by the release workflow attaches prebuilt `spctl`, `speedle-pms` and `speedle-ads` binaries for macOS and Linux on amd64 and arm64, named `<command>_<os>_<arch>`, plus a `checksums.txt` file. Download them from the [Releases page](https://github.com/teramoby/speedle-plus/releases) and verify each downloaded file against its SHA-256 checksum before running it; the checksum file is attached to the same release, so it detects incomplete or corrupted downloads rather than authenticating the release. Binaries and container images are produced by the release workflow, so they are available starting with the first release created after the workflow was added.

The release workflow also publishes two multi-architecture container images for Linux amd64 and arm64:

- `ghcr.io/teramoby/speedle-pms`
- `ghcr.io/teramoby/speedle-ads`

Anonymous pulls require the packages to be public; if a package is private, authenticate first with `docker login ghcr.io`.

Pull the release you deploy; `latest` follows the newest release:

<!-- deployment-template: image-pull -->
```bash
docker pull ghcr.io/teramoby/speedle-pms:latest
docker pull ghcr.io/teramoby/speedle-ads:latest
```

To pin an image by digest, read the released digest with `docker buildx imagetools inspect ghcr.io/teramoby/speedle-pms:<tag>`.

Both images run the service as UID 65532 and include a writable `/data` directory for the file store. Pass the server flags after the image name, and run each command in its own terminal:

<!-- deployment-template: image-run -->
```bash
docker run --rm -p 127.0.0.1:6733:6733 -v speedle-policies:/data ghcr.io/teramoby/speedle-pms:latest --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6733 --insecure=true
docker run --rm -p 127.0.0.1:6734:6734 -v speedle-policies:/data ghcr.io/teramoby/speedle-ads:latest --store-type file --filestore-loc /data/policies.json --endpoint 0.0.0.0:6734 --insecure=true
```

PMS has no built-in caller authentication; the `127.0.0.1` binding keeps the unauthenticated management API on this machine. Never publish these ports on a host interface or expose them to an untrusted network.

The images do not include CA certificates; provide the CA file for TLS endpoints. `--insecure=true` is for this local example only; see [Security](../security/).

## Kubernetes development example

Use a current Kubernetes cluster and compatible `kubectl`. Save the following as `speedle-dev.yaml`. It uses `apps/v1`, one pod with PMS/ADS sharing one file, a non-root application UID and an internal ClusterIP service. This is an isolated development example: `emptyDir` policies are lost when the pod is replaced, and HTTP endpoints need cluster access controls. The manifest uses a `<release-tag>` placeholder; replace it with the release tag you deploy, or a digest for immutable pinning, and add an image pull secret if your cluster requires one.

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
          image: ghcr.io/teramoby/speedle-pms:<release-tag>
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
          image: ghcr.io/teramoby/speedle-ads:<release-tag>
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

Set `PMS_IMAGE` and `ADS_IMAGE` to the exact images you deploy, for example a release tag or a digest. Substitute them locally before applying; keep port-forward running while using Quick Start clients:

<!-- deployment-template: apply -->
```bash
kubectl set image --local -f speedle-dev.yaml pms="$PMS_IMAGE" ads="$ADS_IMAGE" -o yaml | kubectl apply -f -
kubectl rollout status deployment/speedle-dev
kubectl port-forward service/speedle-dev 6733:6733 6734:6734
```

## Production deployments

Use durable policy storage, pin container image digests to the release version you deploy (a release retry can republish a version tag), protect management and decision endpoints, and configure TLS/mTLS for clients and the policy backend. [Pluggable Storage](../store/) documents the current store configuration; [Security](../security/) covers current transport flags. Size and operate the etcd deployment using the current etcd documentation, not the retired etcd-operator example.

The repository contains historical Helm/Kubernetes examples. The old Helm 2 `helm install -n ...`, `stable/etcd-operator`, `apps/v1beta2`, and `r.authz.fun` image instructions are not the deployment flow above. Container pulls, cluster admission, registry access, storage and rollout depend on your environment; validate them in your target cluster before production use.
