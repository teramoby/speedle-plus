# Bilingual documentation validation

Run these checks from the repository root:

```sh
go test -count=1 ./pkg/cmd/flags ./pkg/svcs ./website/tests/docscheck
python3 website/tests/check_docs.py
```

The contract tests compare every documented PMS/ADS method with the actual router
and validate real native responses against both OpenAPI schemas. They also test
the webhook response contract, translated-schema parity, and condition examples
extracted from both SPDL reference pages. The transport suite uses real generated
clients over TCP, including plaintext development, TLS, optional mTLS, mandatory
mTLS, invalid trust/key configuration and both server namespaces.

`check_docs.py` extracts `docs-check` shell blocks and `docs-source` Go code from
the English and Chinese guides. It verifies pairing and runs 112 command blocks
against owned PMS/ADS, mock asserter and custom-function processes. On Linux,
`unshare` and `iproute2` provide private user/network/PID namespaces so that the
exact documented ports do not conflict with other runner services. A namespace
setup failure is a failure, never a silently skipped check. macOS remaps only
owned test ports. No user CLI configuration is written. All fixtures are stopped
and temporary files removed.

The secure guide is also tested through the standalone binaries: trusted clients
work, while plaintext and missing-client-certificate gRPC calls fail for PMS and
ADS. The ordinary guides exercise the explicit insecure local mode. The generated
certificate helper creates fresh short-lived SAN certificates for each run.

`website/playground-engine/verify.sh` includes these checks in the existing
self-hosted build pipeline. The existing quick-start checks still exercise their
exact documented steps on Go 1.26.10 and Go 1.27.3. HTML, language routes, SEO,
Playground and pinned Hugo checks remain in that pipeline.

## Docker and Kubernetes deployment validation

`python3 deployment/tests/verify.py` executes the paired deployment guide blocks
against task-owned Docker resources and a temporary kind cluster. It requires a
Linux Docker engine accessible to the runner account, BuildKit, Bash, curl,
Python 3 and Go. It downloads checksum-verified Helm 4.3.1, kind 0.33.0 and
kubectl 1.36.4 into its disposable directory, without replacing host tools.

It covers repository-root image builds, tags, registry push/pull, the Docker
file-store example, Helm and independently applied YAML, both locales,
server-side dry runs, rollout, policy CRUD, Allow/Deny and real TLS/mTLS clients.
The production chart rejects an unspecified backend and requires HTTPS plus
client credentials by default. The etcd fixture requires mTLS, uses a separate
CA from the APIs, and rejects missing or foreign client certificates on a real
write request. `deployment/tests/check-charts.py` checks these defaults and the
deliberate legacy opt-in for externally protected CA-only/plaintext backends.
Fault checks distinguish backend readiness from process liveness and verify
policy persistence across etcd replacement. A process freeze tests liveness
restart behavior. The cluster uses a pinned node image, its own kubeconfig and
namespaces; API/registry forwards bind only to loopback and avoid fixed host
ports. A host lock serializes container acceptance across the two runners.
Cleanup touches only resources created by this invocation.
The full container gate runs for pull requests, master pushes, manual CI and
local verification. Feature-branch pushes keep the build/native/website checks;
their PR run owns container acceptance, avoiding a duplicate kind cluster on
the shared host. Run `python3 deployment/tests/verify.py` directly for an
unconditional container check.

`deployment/render-manifests.sh` regenerates standalone manifests from the same
charts. Runtime acceptance verifies that those committed copies match. The
single-member etcd manifest is a test example with a PVC; production deployments
must supply their required etcd availability, backups, trust and access control.
Website styling, templates and image assets remain unchanged.

## Protobuf generation

`scripts/generate-protobuf.sh` pins protoc 36.2 and protoc-gen-go v1.0.0 to retain
the existing generated Go API. It regenerates distinct file descriptors and uses
the server namespaces `pms_pb.PolicyManager` and `ads_pb.Evaluator`, then copies
those same contracts into the current website's downloadable proto files.
