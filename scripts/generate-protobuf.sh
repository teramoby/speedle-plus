#!/usr/bin/env bash
set -euo pipefail
ROOT=$(git rev-parse --show-toplevel)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/speedle-protobuf.XXXXXXXX")
trap 'rm -rf "$WORK"' EXIT
# Keep the legacy Go API while regenerating its descriptors and wire names.
# protoc 36.2 + protoc-gen-go v1.0.0 are the reviewed generation toolchain.
test "$(protoc --version)" = 'libprotoc 36.2'
GOBIN="$WORK/bin" go install github.com/golang/protobuf/protoc-gen-go@v1.0.0
cd "$ROOT"
for service in pms ads; do
  source="pkg/svcs/${service}grpc/pb/service.proto"
  protoc -I . --plugin="protoc-gen-go=$WORK/bin/protoc-gen-go" --go_out="plugins=grpc:$WORK" "$source"
  install -m 0644 "$WORK/github.com/teramoby/speedle-plus/pkg/svcs/${service}grpc/pb/service.pb.go" "pkg/svcs/${service}grpc/pb/service.pb.go"
  install -m 0644 "$source" "docs/hugo/static/protobuf/${service}.proto"
done
