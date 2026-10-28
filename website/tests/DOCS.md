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

## Current scope

The owner deferred the container/Kubernetes batch (audit D14-D17). Do not include
changes to `deployment/` or the two service-deployment Markdown pages in this
repair. Their outstanding toolchain, Helm, image-build, YAML and probe issues
remain deferred. The deployment-page portions of D07/D22/D23 remain deferred too.
Current docs and downloadable APIs are updated; historical version archives remain
historical. Stylesheets, templates, image assets and site design are unchanged.

## Protobuf generation

`scripts/generate-protobuf.sh` pins protoc 36.2 and protoc-gen-go v1.0.0 to retain
the existing generated Go API. It regenerates distinct file descriptors and uses
the server namespaces `pms_pb.PolicyManager` and `ads_pb.Evaluator`, then copies
those same contracts into the current website's downloadable proto files.
