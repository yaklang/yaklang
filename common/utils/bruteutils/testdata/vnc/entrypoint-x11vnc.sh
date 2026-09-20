#!/bin/sh
set -eu
PASS="${VNC_PASSWORD:-VncPass123!}"
x11vnc -storepasswd "$PASS" /tmp/vncpasswd >/dev/null
Xvfb :99 -screen 0 640x480x16 >/tmp/xvfb.log 2>&1 &
i=0
while [ "$i" -lt 50 ]; do
  if [ -e /tmp/.X11-unix/X99 ]; then
    break
  fi
  i=$((i + 1))
  sleep 0.1
done
extra=""
if [ -n "${RFB_VERSION:-}" ]; then
  extra="-rfbversion $RFB_VERSION"
fi
exec x11vnc -display :99 -rfbauth /tmp/vncpasswd -rfbport 5900 \
  -forever -shared -listen 0.0.0.0 -noxdamage $extra
