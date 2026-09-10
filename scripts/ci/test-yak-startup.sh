#!/usr/bin/env bash
set -euo pipefail

# Run against the already-built artifact, not a second build or a system yak.
startup_binary="${YAK_BINARY_PATH:?Set YAK_BINARY_PATH to the compiled yak executable}"
if [[ "$startup_binary" != /* || ! -f "$startup_binary" || ! -x "$startup_binary" ]]; then
  echo "YAK_BINARY_PATH must be an absolute path to an executable file" >&2
  exit 1
fi
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

# Isolate package init as well as child processes. Never use a developer's DBs
# or the DBs that later prepared suites share. Test children have bounded
# deadlines and cleanup targets only the PIDs they started.
startup_home=$(mktemp -d "${TMPDIR:-/tmp}/yak-startup.XXXXXX")
trap 'rm -rf "$startup_home"' EXIT
export YAKIT_HOME="$startup_home"
export YAK_DEFAULT_PROJECT_DATABASE_NAME="$startup_home/project.db"
export YAK_DEFAULT_PROFILE_DATABASE_NAME="$startup_home/profile.db"
export SSA_DATABASE_RAW="$startup_home/ssa.db"
export SKIP_SYNC_EMBED_RULE_IN_GITHUB=1
export YAK_STARTUP_TEST_BINARY="$startup_binary"

go test -count=1 -timeout=90s ./common/utils/engineendpoint
go test -count=1 -timeout=4m -v -run '^TestCLIStartup' ./common/yak/cmd
