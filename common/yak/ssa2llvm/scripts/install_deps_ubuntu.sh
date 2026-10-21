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
SUDO=""
if [[ "${EUID}" -ne 0 ]]; then
  APT_CMD=(sudo apt-get)
  SUDO="sudo"
fi

# Bootstrap tools needed to add the LLVM repository (present on GitHub
# runners, not on a bare Ubuntu image).
"${APT_CMD[@]}" update
"${APT_CMD[@]}" install -y ca-certificates curl gnupg

# go-llvm vendors the LLVM 18.1.3 static libraries inside the module, but the
# module zip does not carry the LLVM C/C++ headers (they are symlinks that Go's
# module cache drops). CI therefore installs the matching system headers from
# the official LLVM apt repository. Ubuntu 22.04 (jammy) and 24.04 (noble) do
# not ship llvm-18 in their own archives, so the repo has to be added first.
CODENAME="${VERSION_CODENAME:-}"
case "${CODENAME}" in
  focal|jammy|noble|bookworm|trixie)
    if ! grep -qr "apt.llvm.org" /etc/apt/sources.list /etc/apt/sources.list.d/ 2>/dev/null; then
      echo "Adding official LLVM 18 apt repository for ${CODENAME}..."
      curl -fsSL https://apt.llvm.org/llvm-snapshot.gpg.key | ${SUDO} tee /etc/apt/trusted.gpg.d/apt.llvm.org.asc >/dev/null
      echo "deb https://apt.llvm.org/${CODENAME}/ llvm-toolchain-${CODENAME}-18 main" | ${SUDO} tee /etc/apt/sources.list.d/llvm-18.list >/dev/null
    fi
    ;;
  *)
    echo "Unsupported codename '${CODENAME}': LLVM 18 packages must come from another source" >&2
    ;;
esac

# Versioned LLVM 18 packages: the unversioned llvm-dev/libclang-dev on Ubuntu
# point at the distro default (14 on jammy), which would shadow the headers
# go-llvm expects. libxml2-dev/libffi-dev/libncurses-dev are the static-link
# system deps go-llvm's linux LDFLAGS reference (-lxml2 -lffi -ltinfo).
PACKAGES=(
  gcc
  g++
  llvm-18-dev
  libclang-18-dev
  liblld-18-dev
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
