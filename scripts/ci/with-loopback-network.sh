#!/usr/bin/env bash
set -euo pipefail

# Give the complete test process tree its own network namespace. There is no
# external interface or route; TCP, UDP and IPv6 loopback fixtures still work.
# A namespace setup failure must fail the suite instead of running it online.
if [[ $# -eq 0 ]]; then
  echo "Usage: $0 command [args...]" >&2
  exit 2
fi
exec unshare --user --map-root-user --net -- bash -c '
  set -euo pipefail
  ip link set lo up
  echo "Test network: loopback only (isolated network namespace)"
  exec "$@"
' loopback-tests "$@"
