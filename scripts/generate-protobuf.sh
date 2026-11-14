#!/usr/bin/env bash
# Install protoc plus protoc-gen-go v1.36.11 and protoc-gen-go-grpc v1.6.1 on PATH.
set -euo pipefail
cd "$(dirname "$0")/.."
for service in ads pms; do
  directory="pkg/svcs/${service}grpc/pb"
  protoc -I . --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative,require_unimplemented_servers=false \
    "${directory}/messages.proto" "${directory}/service.proto"
done
