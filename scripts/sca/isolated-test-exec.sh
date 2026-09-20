#!/bin/sh
# macOS test launcher, never a production SCA dependency. Only the test binary
# may be executed; network and all filesystem writes are denied.
set -eu
sca_test_path="$(cd "$(dirname "$1")" && pwd -P)/$(basename "$1")"
exec /usr/bin/sandbox-exec -D TEST_EXECUTABLE="$sca_test_path" -p '(version 1)(allow default)(deny network*)(deny file-write*)(deny process-exec)(allow process-exec (literal (param "TEST_EXECUTABLE")))' "$@"
