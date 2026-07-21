#!/usr/bin/env bash
# Install yak binary from aliyun-oss (same version policy as diff-code-check).
set -euo pipefail

PATTERN="${YAK_VERSION_PATTERN:-.*-alpha.*-diff-check|.*-alpha.*-code-scan|.*-beta[0-9]+$}"
REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

cd "$REPO_ROOT"

echo "::group::Finding latest Yak version"
SELECTED_VERSION=$(bash ./scripts/get-yak-version.sh --pattern "$PATTERN")
if [ -z "$SELECTED_VERSION" ]; then
  echo "::error::No suitable version found (pattern: $PATTERN)"
  exit 1
fi
echo "Selected version: $SELECTED_VERSION"
echo "::endgroup::"

echo "::group::Downloading Yak binary"
OS=""
ARCH=""
case "$(uname -s)" in
  Linux*)   OS="linux" ;;
  Darwin*)  OS="darwin" ;;
  MINGW*)   OS="windows" ;;
  *)        echo "::error::Unsupported OS: $(uname -s)"; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64)   ARCH="amd64" ;;
  aarch64)  ARCH="arm64" ;;
  arm64)    ARCH="arm64" ;;
  *)        echo "::error::Unsupported architecture: $(uname -m)"; exit 1 ;;
esac

BINARY_NAME="yak_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="${BINARY_NAME}.exe"
fi

DOWNLOAD_URL="https://aliyun-oss.yaklang.com/yak/${SELECTED_VERSION}/${BINARY_NAME}"
echo "Downloading from: $DOWNLOAD_URL"

if ! curl -sS -L "$DOWNLOAD_URL" -o yak; then
  echo "::error::Failed to download Yak binary"
  exit 1
fi
chmod +x yak
echo "::endgroup::"

echo "::group::Verifying Yak installation"
if ! ./yak version; then
  echo "::error::Yak installation verification failed"
  exit 1
fi

if command -v sudo >/dev/null 2>&1; then
  sudo mv yak /usr/local/bin/yak || sudo mv yak /usr/bin/yak || mv yak "$HOME/.local/bin/yak"
else
  mv yak /usr/local/bin/yak 2>/dev/null || mv yak /usr/bin/yak 2>/dev/null || mv yak "$HOME/.local/bin/yak"
fi

export PATH="$HOME/.local/bin:$PATH"
if ! yak version; then
  echo "::error::Yak not on PATH after install"
  exit 1
fi
echo "Yak installed: $(yak version)"
echo "::endgroup::"
