#!/usr/bin/env bash
# Fail if a Linux binary requires glibc symbols newer than 2.17 (manylinux2014 / RHEL 7).
set -euo pipefail

BIN="${1:?binary path required}"
MAX_MINOR=17

if [ ! -f "$BIN" ]; then
  echo "Error: binary not found: $BIN" >&2
  exit 1
fi

GLIBC_SYMS="$(objdump -T "$BIN" 2>/dev/null | grep -oE 'GLIBC_[0-9]+(\.[0-9]+)?' || true)"
if [ -z "$GLIBC_SYMS" ]; then
  GLIBC_SYMS="$(strings "$BIN" | grep -oE '^GLIBC_[0-9]+(\.[0-9]+)?' || true)"
fi

echo "$GLIBC_SYMS" | sort -Vu

while IFS= read -r sym; do
  [ -z "$sym" ] && continue
  ver="${sym#GLIBC_}"
  major="${ver%%.*}"
  minor="${ver#*.}"
  if [ "$minor" = "$ver" ]; then
    minor=0
  fi
  # Only compare GLIBC_2.x; ignore GLIBC_2.2.5-style three-part versions via integer minor
  minor="${minor%%.*}"
  if [ "$major" -gt 2 ] || { [ "$major" -eq 2 ] && [ "$minor" -gt "$MAX_MINOR" ]; }; then
    echo "Error: $BIN requires $sym; max allowed is GLIBC_2.${MAX_MINOR}" >&2
    exit 1
  fi
done <<< "$GLIBC_SYMS"

echo "glibc symbol check passed (<= GLIBC_2.${MAX_MINOR})"
ldd "$BIN" || true
