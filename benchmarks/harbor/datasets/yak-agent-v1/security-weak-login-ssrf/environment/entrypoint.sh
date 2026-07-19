#!/bin/sh
set -eu

python3 /opt/challenge/server.pyc &
exec "$@"
