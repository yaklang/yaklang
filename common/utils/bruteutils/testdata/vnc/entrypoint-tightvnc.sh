#!/bin/sh
set -eu
export USER="${USER:-root}"
export HOME="${HOME:-/root}"
mkdir -p /tmp/.X11-unix /root/.vnc
chmod 1777 /tmp/.X11-unix
# TightVNC binary is Xtightvnc; it speaks Tight (16) and/or VNC-Auth (2).
exec Xtightvnc :1 -geometry 640x480 -depth 16 -rfbport 5900 \
  -rfbauth /root/.vnc/passwd -interface 0.0.0.0 -alwaysshared
