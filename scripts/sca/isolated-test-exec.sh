#!/bin/sh
# macOS test launcher, never a production SCA dependency.
# Denies network, filesystem writes, and exec of any program except this test
# binary. BOM cycle regressions re-exec the same test binary as a child; that
# path is allowed here and must stay enabled in the regular (non-isolated) suite.
set -eu
sca_test_path="$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")"
exec /usr/bin/sandbox-exec -D TEST_EXECUTABLE="$sca_test_path" -p '(version 1)(allow default)(deny network*)(deny file-write*)(deny process-exec)(allow process-exec (literal (param "TEST_EXECUTABLE")))' "$@"
