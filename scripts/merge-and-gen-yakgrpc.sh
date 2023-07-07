#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

command -v protoc >/dev/null 2>&1 || { echo -e "${RED}protoc is not installed. Please download and install it. Aborting.${NC}"; echo -e "${GREEN}https://github.com/protocolbuffers/protobuf/releases${NC}"; exit 1; }
echo "Found protoc version $(protoc --version)"

command -v protoc-gen-go >/dev/null 2>&1 || { echo -e "${RED}protoc-gen-go is not installed. Please download and install it. Aborting.${NC}"; echo -e "${GREEN}go install google.golang.org/protobuf/cmd/protoc-gen-go@latest${NC}"; exit 1; }
echo "Found protoc-gen-go version $(protoc-gen-go --version)"

command -v protoc-gen-go-grpc >/dev/null 2>&1 || { echo -e "${RED}protoc-gen-go-grpc is not installed. Please download and install it. Aborting.${NC}"; echo -e "${GREEN}go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest${NC}"; exit 1; }
echo "Found protoc-gen-go-grpc version $(protoc-gen-go-grpc --version)"



if [ -f "common/yakgrpc/yakgrpc.proto" ]; then
  echo "YAKIT GRPC PROTO EXISTED, start to replace"
  cp common/yakgrpc/yakgrpc.proto ./../yakit/app/protos/grpc.proto
else
  echo "YAKIT REPOS NOT FOUND"
fi