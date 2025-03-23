#!/bin/bash

set -e

# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1 && export PATH="$PATH:$(go env GOPATH)/bin"

#protoc \
#  --plugin="protoc-gen-ts=./yaki/node_modules/.bin/protoc-gen-ts" \
#  --js_out="import_style=commonjs,binary:./yaki/app/gen-pb/" \
#  --ts_out="service=grpc-web:./yaki/app/gen-pb/" \
#  --go-grpc_out=common/yakgrpc/ypb \
#  --go_out=common/yakgrpc/ypb \
#  --proto_path=common/yakgrpc/ yakgrpc.proto

# Check if protoc is installed
echo "Checking if protoc is installed..."
if ! command -v protoc &> /dev/null; then
  echo "protoc command not found"
  NEED_INSTALL=true
else
  echo "protoc found, checking version..."
  PROTOC_CURRENT_VERSION=$(protoc --version)
  echo "Current protoc version: ${PROTOC_CURRENT_VERSION}"
  if [[ "${PROTOC_CURRENT_VERSION}" != *"29.4"* ]]; then
    echo "Current protoc version is not 29.4"
    echo "Removing old protoc version..."
    sudo rm -f $(which protoc)
    NEED_INSTALL=true
  else
    echo "Correct protoc version 29.4 is already installed"
    NEED_INSTALL=false
  fi
fi

if [[ "$NEED_INSTALL" == "true" ]]; then
  echo "libprotoc 29.4 not found, installing..."
  
  PROTOC_VERSION="29.4"
  PROTOC_ZIP=""
  # Detect OS
  echo "Detecting operating system..."
  if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    echo "Linux OS detected"
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
    echo "Installing dependencies: unzip and wget..."
    apt-get update && apt-get install -y unzip wget
    echo "Dependencies installed successfully"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    echo "macOS detected"
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-osx-x86_64.zip"
    echo "Installing dependencies: wget and unzip using brew..."
    brew install wget unzip || true
    echo "Dependencies installation completed"
  else
    echo "Unsupported OS for automatic installation. Please install protoc 4.24.0 manually."
    exit 1
  fi
  
  # Download and install protoc
  echo "Downloading protoc ${PROTOC_VERSION}..."
  echo "Download URL: https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"
  read -p "Continue with download? (y/n): " CONFIRM
  if [[ "$CONFIRM" != "y" ]]; then
    echo "Download cancelled by user"
    exit 1
  fi
  wget -P /tmp "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"
  echo "Download completed"
  # Extract to /tmp directory
  echo "Extracting protoc to /tmp directory..."
  mkdir -p /tmp/protoc_temp
  unzip -o "/tmp/${PROTOC_ZIP}" -d /tmp/protoc_temp
  echo "Extraction completed"
  
  # 将 protoc 移动到 /usr/local/bin
  echo "Installing protoc to /usr/local/bin..."
  sudo cp -R /tmp/protoc_temp/bin/* /usr/local/bin/
  sudo cp -R /tmp/protoc_temp/include/* /usr/local/include/
  
  echo "Cleaning up temporary directory..."
  rm -rf /tmp/protoc_temp
  
  echo "Cleaning up downloaded zip file..."
  rm -f "/tmp/${PROTOC_ZIP}"
  echo "Cleanup completed"
  
  echo "protoc ${PROTOC_VERSION} installed successfully"
else
  echo "Skipping protoc installation as correct version is already installed"
fi

echo "Checking if Yakit repository exists..."
if [ -f "./../yakit/app/protos/grpc.proto" ]; then
  echo "YAKIT GRPC PROTO exists, starting file replacement..."
  cp common/yakgrpc/yakgrpc.proto ./../yakit/app/protos/grpc.proto
  echo "YAKIT GRPC PROTO replacement completed"
else
  echo "YAKIT repository not found, skipping replacement step"
fi

echo "Starting generation of yakgrpc Go code..."
protoc \
  --go-grpc_out=common/yakgrpc/ypb \
  --go_out=common/yakgrpc/ypb \
  --proto_path=common/yakgrpc/ yakgrpc.proto
echo "yakgrpc Go code generation completed"

echo "Starting generation of cybertunnel Go code..."
protoc \
  --go-grpc_out=common/cybertunnel/tpb \
  --go_out=common/cybertunnel/tpb \
  --proto_path=common/cybertunnel/ tunnel.proto
echo "cybertunnel Go code generation completed"