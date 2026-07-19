#!/usr/bin/env bash
# Build a complete Linux yak binary from <git-ref> without leaving the current
# checkout. Mirrors the CI manylinux build path so the resulting binary has the
# same CGO/pcap and gzip_embed coverage as a release build.
#
# Usage:
#   build_yak_at_ref.sh <git-ref> <output-binary>
#
# Env overrides:
#   YAK_BENCHMARK_MANYLINUX_IMAGE   manylinux image (default: manylinux2014_aarch64)
#   YAK_BENCHMARK_GOARCH            target go arch (default: arm64)
#   YAK_BENCHMARK_SKIP_SELF_CHECK   "1" to skip the in-container smoke exec
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <git-ref> <output-binary>" >&2
  exit 2
fi

ref="$1"
output="$2"
repo_root="$(git rev-parse --show-toplevel)"
worktree="$(mktemp -d "${TMPDIR:-/tmp}/yak-benchmark-worktree.XXXXXX")"

cleanup() {
  git -C "$repo_root" worktree remove --force "$worktree" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$(dirname "$output")"
git -C "$repo_root" worktree add --detach "$worktree" "$ref"

manylinux_image="${YAK_BENCHMARK_MANYLINUX_IMAGE:-quay.io/pypa/manylinux2014_aarch64}"
goarch="${YAK_BENCHMARK_GOARCH:-arm64}"

echo "[build] ref=$ref worktree=$worktree image=$manylinux_image goarch=$goarch"

# --- 1. Generate the gzip_embed .tar.gz resources ---------------------------
# Must run before `go build -tags gzip_embed`, otherwise the `//go:embed *.tar.gz`
# directives fail. This is the same 6-command set CI runs
# (.github/workflows/exp-cross-build.yml "Generate tar.gz resources"). The
# generator tool is host-arch; only its output (the archives) matters.
echo "[build] installing gzip-embed tool on host..."
(
  cd "$worktree"
  export GOCACHE="${GOCACHE:-/tmp/yak-benchmark-go-cache}"
  go install ./common/utils/gzip_embed/gzip-embed
)

GZIP_EMBED_BIN="$(go env GOPATH 2>/dev/null)/bin/gzip-embed"
if [[ ! -x "$GZIP_EMBED_BIN" ]]; then
  echo "[build] gzip-embed not found at $GZIP_EMBED_BIN after install" >&2
  exit 1
fi

echo "[build] generating gzip_embed resources..."
(
  cd "$worktree"
  "$GZIP_EMBED_BIN" -cache --source ./common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai --gz ./common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai.tar.gz --no-embed
  "$GZIP_EMBED_BIN" -cache --source ./common/ai/aid/aireact/skills --gz ./common/ai/aid/aireact/skills.tar.gz --root-path --no-embed
  "$GZIP_EMBED_BIN" -cache --source ./common/yso/resources/static --gz ./common/yso/resources/static.tar.gz --no-embed --root-path
  "$GZIP_EMBED_BIN" -cache --source ./common/coreplugin/base-yak-plugin --gz ./common/coreplugin/base-yak-plugin.tar.gz --root-path --no-embed
  "$GZIP_EMBED_BIN" -cache --source ./common/syntaxflow/sfbuildin/buildin --gz ./common/syntaxflow/sfbuildin/buildin.tar.gz --root-path --no-embed
  "$GZIP_EMBED_BIN" -cache --source ./common/aiforge/buildinforge --gz ./common/aiforge/buildinforge.tar.gz --root-path --no-embed
)

# --- 2. Build inside manylinux (CGO on, gzip_embed tag) ---------------------
# Reuse the verified CI script. It mounts the worktree at /work and writes the
# binary to ./${OUTPUT_BINARY_REL} inside the container (= host worktree).
# We pass a *relative* name because the script does `./${OUTPUT_BINARY}`.
output_abs="$(cd "$(dirname "$output")" && pwd)/$(basename "$output")"
output_rel="yak-linux-${goarch}"

yak_tag="$(git -C "$worktree" describe --tags 2>/dev/null || git -C "$worktree" rev-parse --short HEAD)"
build_tags="gzip_embed"

echo "[build] building yak with manylinux (tags=$build_tags tag=$yak_tag)..."
(
  cd "$worktree"
  bash .github/scripts/build-linux-manylinux.sh \
    "$manylinux_image" \
    "$output_rel" \
    "$build_tags" \
    "$yak_tag"
)

mv "$worktree/$output_rel" "$output_abs"
chmod 0755 "$output_abs"

# --- 3. Self-check: binary must execute on the target Linux arch ------------
if [[ "${YAK_BENCHMARK_SKIP_SELF_CHECK:-0}" != "1" ]]; then
  echo "[build] self-check: executing binary in $manylinux_image..."
  if ! docker run --rm \
      -v "$output_abs:/yak:ro" \
      --platform "linux/${goarch}" \
      "$manylinux_image" \
      /bin/bash -c 'source /opt/rh/devtoolset-10/enable 2>/dev/null; /yak version >/dev/null 2>&1 || /yak --help >/dev/null 2>&1'; then
    echo "[build] self-check FAILED: binary did not execute cleanly in the target container" >&2
    exit 1
  fi
fi

echo "[build] ok: $(git -C "$worktree" rev-parse HEAD) -> $output_abs"
