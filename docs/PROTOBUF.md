# Generating the gRPC bindings

The checked-in bindings use `protoc` 36.2 (`libprotoc 36.2`, reported in
Go headers as v7.36.2). Install that compiler, then the pinned Go generators:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
scripts/generate-protobuf.sh
```

The generated Go packages and public message fields remain in
`pkg/svcs/{ads,pms}grpc/pb`. Services retain their existing `pb.Evaluator`
and `pb.PolicyManager` names, RPC paths and binary request/response layouts.
Message declarations live in separate `speedle.ads` and `speedle.pms`
namespaces so both sets of descriptors can coexist in the modern protobuf
registry. Descriptor file names also change from `service.proto` to
`pkg/svcs/{ads,pms}grpc/pb/service.proto` and `messages.proto` under the same
qualified directory. Reflection clients looking up files must use those paths.
Reflection clients must use these new message names; hypothetical
`Any` type URLs built from those names would also change. Speedle's RPCs do
not use `Any`. The standalone schemas in `docs/protobuf` describe the legacy
wire contract; generation uses the schemas under `pkg/svcs`. The published
standalone schemas intentionally retain `pb.*`: they are wire-compatible with
the server, but not descriptor-identical to its reflected message names and
file paths. Reflection/schema-registry consumers should use the server schemas
or reflection. The generation script verifies all three pinned tool versions
before writing files.

`testdata/legacy-wire.json` in each generated package was captured from the
old `github.com/golang/protobuf` bindings, populating all public fields in
every message. The compatibility tests compare deterministic binary encoding,
decode those fixtures, and verify preservation of unknown fields.

```sh
go test -race ./pkg/svcs/adsgrpc/... ./pkg/svcs/pmsgrpc/... ./testutil/protocompat
```
