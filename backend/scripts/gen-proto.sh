#!/usr/bin/env bash
# Regenerates the Go code for the gRPC contract between the control plane and
# the agents. Generated files are committed so the project builds without protoc.
#
# Requirements:
#   brew install protobuf                       # or apt install protobuf-compiler
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROTO_DIR="$ROOT/api/proto"
OUT_DIR="$ROOT/gen"

export PATH="$PATH:$(go env GOPATH)/bin"

for tool in protoc protoc-gen-go protoc-gen-go-grpc; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing tool: $tool (see header of $0)" >&2
    exit 1
  fi
done

mkdir -p "$OUT_DIR"

# go_package in the .proto files is "backend/gen/agent/v1", so we strip the
# module prefix with --go_opt=module to land files directly under $OUT_DIR.
protoc \
  -I "$PROTO_DIR" \
  --go_out="$OUT_DIR" --go_opt=module=backend/gen \
  --go-grpc_out="$OUT_DIR" --go-grpc_opt=module=backend/gen \
  $(find "$PROTO_DIR" -name '*.proto')

echo "protobuf code generated in $OUT_DIR"
