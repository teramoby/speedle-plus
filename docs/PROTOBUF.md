# Generating the gRPC bindings

Install `protoc`, then install the pinned Go generators:

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
registry. Reflection clients must use these new message names; hypothetical
`Any` type URLs built from those names would also change. Speedle's RPCs do
not use `Any`. The standalone schemas in `docs/protobuf` describe the legacy
wire contract; generation uses the schemas under `pkg/svcs`.

`testdata/legacy-wire.json` in each generated package was captured from the
old `github.com/golang/protobuf` bindings, populating all public fields in
every message. The compatibility tests compare deterministic binary encoding,
decode those fixtures, and verify preservation of unknown fields.

```sh
go test -race ./pkg/svcs/adsgrpc/... ./pkg/svcs/pmsgrpc/... ./testutil/protocompat
```
