#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUNTIME_DIR="${ROOT_DIR}/runtime"

cd "${RUNTIME_DIR}/runtime_go"
go build -buildmode=c-archive -o "${RUNTIME_DIR}/libyak.a" .
rm -f "${RUNTIME_DIR}/libyak.h"

echo "Built ${RUNTIME_DIR}/libyak.a (Go Runtime)"
