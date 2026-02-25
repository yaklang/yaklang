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

PACKAGES=(
  llvm-dev
  libclang-dev
  zlib1g-dev
  libzstd-dev
  libgc-dev
)

APT_CMD=(apt-get)
if [[ "${EUID}" -ne 0 ]]; then
  APT_CMD=(sudo apt-get)
fi

echo "Installing ssa2llvm build/test dependencies on ${PRETTY_NAME:-Ubuntu}..."
echo "Packages: ${PACKAGES[*]}"

"${APT_CMD[@]}" update
"${APT_CMD[@]}" install -y "${PACKAGES[@]}"

echo "Dependency installation completed."
