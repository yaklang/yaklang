#!/usr/bin/env bash
# Acquire an exclusive lock on the SSA database for write operations.
#
# GitHub Actions `concurrency.group` is a soft, single-runner assumption and
# does not protect the SQLite file when multiple self-hosted runners share the
# `ssa-ci` label. This helper wraps the critical DB-writing section in flock so
# weekly full compile and post-merge promote never write the same DB file at
# once.
#
# Usage:
#   source ./scripts/ci-ssa/acquire-db-lock.sh   # exports ACQUIRE_DB_LOCK_FD
#   exec {ACQUIRE_DB_LOCK_FD}>&-                 # release (close fd)
#
# Or as a guard at the top of a script that already `source`s export-ssa-db-env.sh:
#   ./scripts/ci-ssa/acquire-db-lock.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=export-ssa-db-env.sh
source "$SCRIPT_DIR/export-ssa-db-env.sh"

LOCK_FILE="${SSA_DATABASE_RAW}.lock"

if ! command -v flock >/dev/null 2>&1; then
  echo "::warning::flock not found on this runner; falling back to GitHub concurrency group only."
  echo "::warning::Install util-linux (flock) on self-hosted runners to harden DB writes."
  return 0 2>/dev/null || exit 0
fi

# Use a high file descriptor so it stays open for the lifetime of the caller.
exec 9>"$LOCK_FILE"

# Block up to CI_SSA_DB_LOCK_TIMEOUT_SEC (default 3600s = 1h, covers a long
# weekly full compile) waiting for exclusive access.
LOCK_TIMEOUT="${CI_SSA_DB_LOCK_TIMEOUT_SEC:-3600}"
if ! flock --exclusive --timeout "$LOCK_TIMEOUT" 9; then
  echo "::error::Could not acquire DB lock $LOCK_FILE within ${LOCK_TIMEOUT}s."
  echo "::error::Another weekly/promote write is likely in progress and did not release the lock."
  exit 1
fi

export ACQUIRE_DB_LOCK_FD=9
echo "Acquired DB write lock: $LOCK_FILE (fd 9, timeout ${LOCK_TIMEOUT}s)"
