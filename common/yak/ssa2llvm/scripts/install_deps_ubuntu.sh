#!/usr/bin/env bash
set -euo pipefail

if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  source /etc/os-release
else
  echo "Cannot detect OS: /etc/os-release not found" >&2
  exit 1
fi

if [[ "${ID:-}" != "ubuntu" && "${ID_LIKE:-}" != *"debian"* ]]; then
  echo "This script is intended for Ubuntu/Debian systems. Detected: ${PRETTY_NAME:-unknown}" >&2
  exit 1
fi

APT_CMD=(apt-get)
if [[ "${EUID}" -ne 0 ]]; then
  APT_CMD=(sudo apt-get)
fi

# go-llvm embeds the LLVM 18.1.3 C/C++ headers and static archives, so the
# system LLVM installation is not used at all. Only the toolchain and the
# static-link system deps go-llvm's linux LDFLAGS reference are installed.
PACKAGES=(
  gcc
  g++
  zlib1g-dev
  libzstd-dev
  libxml2-dev
  libffi-dev
  libncurses-dev
  libgc-dev
)

echo "Installing ssa2llvm build/test dependencies on ${PRETTY_NAME:-Ubuntu}..."
echo "Packages: ${PACKAGES[*]}"

"${APT_CMD[@]}" update
"${APT_CMD[@]}" install -y "${PACKAGES[@]}"

echo "Dependency installation completed."
