#!/usr/bin/env bash
# Summarize heap profiles produced by YAK_SSA_HEAP_PROFILE_DIR (phase dumps)
# and optional measure_before/after snapshots. Writes text reports under OUT_DIR.
set -euo pipefail

PROFILE_DIR="${1:-}"
OUT_DIR="${2:-}"
if [[ -z "$PROFILE_DIR" || -z "$OUT_DIR" ]]; then
  echo "usage: $0 <profile-dir> <out-dir>" >&2
  exit 2
fi
mkdir -p "$OUT_DIR"

summarize() {
  local name="$1"
  local file="$2"
  local out="$OUT_DIR/pprof-top-${name}.txt"
  echo "=== inuse_space top30: $file ===" >"$out"
  go tool pprof -top -sample_index=inuse_space -nodecount=30 "$file" >>"$out" 2>&1 || true
  echo "" >>"$out"
  echo "=== alloc_space top30: $file ===" >>"$out"
  go tool pprof -top -sample_index=alloc_space -nodecount=30 "$file" >>"$out" 2>&1 || true
}

for f in "$PROFILE_DIR"/*.heap.pb.gz; do
  [[ -f "$f" ]] || continue
  base=$(basename "$f" .heap.pb.gz)
  summarize "$base" "$f"
done

# Diff f1 vs f2 (inuse) when both exist — shows what dropped after pass1 AST release
F1="$PROFILE_DIR/f1_pre_handler.heap.pb.gz"
F2="$PROFILE_DIR/f2_after_pre.heap.pb.gz"
if [[ -f "$F1" && -f "$F2" ]]; then
  go tool pprof -base "$F1" -top -sample_index=inuse_space -nodecount=40 "$F2" \
    >"$OUT_DIR/pprof-diff-f1-to-f2-inuse.txt" 2>&1 || true
fi

F1L="$PROFILE_DIR/f1_pre_handler.heap.pb.gz"
F1S="$PROFILE_DIR/../enhance-skeleton/f1_pre_handler.heap.pb.gz"
# optional cross-run: caller may symlink

echo "wrote reports under $OUT_DIR"
