#!/bin/bash

# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1 && export PATH="$PATH:$(go env GOPATH)/bin"

#protoc \
#  --plugin="protoc-gen-ts=./yaki/node_modules/.bin/protoc-gen-ts" \
#  --js_out="import_style=commonjs,binary:./yaki/app/gen-pb/" \
#  --ts_out="service=grpc-web:./yaki/app/gen-pb/" \
#  --go-grpc_out=common/yakgrpc/ypb \
#  --go_out=common/yakgrpc/ypb \
#  --proto_path=common/yakgrpc/ yakgrpc.proto

if [ -f "./../yakit/app/protos/grpc.proto" ]; then
  echo "YAKIT GRPC PROTO EXISTED, start to replace"
  cp common/yakgrpc/yakgrpc.proto ./../yakit/app/protos/grpc.proto
else
  echo "YAKIT REPOS NOT FOUND"
fi

protoc \
  --go-grpc_out=common/yakgrpc/ypb \
  --go_out=common/yakgrpc/ypb \
  --proto_path=common/yakgrpc/ yakgrpc.proto

protoc \
  --go-grpc_out=common/cybertunnel/tpb \
  --go_out=common/cybertunnel/tpb \
  --proto_path=common/cybertunnel/ tunnel.proto