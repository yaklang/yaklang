#!/bin/bash

PROTO_DIR="common/yakgrpc/protos"
OUT_DIR="common/yakgrpc/ypb"

echo "Starting to process .proto files in $PROTO_DIR..."

find $PROTO_DIR -name '*.proto' -exec sh -c '
    echo "Processing file: $(basename $1)..."
    protoc --go-grpc_out='$OUT_DIR' --go_out='$OUT_DIR' --proto_path='$PROTO_DIR' $1
' sh {} \;

echo "Finished processing .proto files."
