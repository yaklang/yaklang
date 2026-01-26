#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Build the runtime as a C archive from Go source
cd "$SCRIPT_DIR/runtime_go"
go build -buildmode=c-archive -o ../libyak.a yak_lib.go
cd ..

rm -f libyak.h
echo "Built libyak.a (Go Runtime)"
