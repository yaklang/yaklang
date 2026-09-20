#!/bin/sh
set -eu
export USER="${USER:-root}"
export HOME="${HOME:-/root}"
mkdir -p /tmp/.X11-unix /tmp/.ICE-unix
chmod 1777 /tmp/.X11-unix /tmp/.ICE-unix

PASS="${VNC_PASSWORD:-VncPass123!}"
SEC="${VNC_SECURITY:-VncAuth}"
PORT="${VNC_PORT:-5900}"

if [ "$SEC" = "None" ]; then
  exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
    -SecurityTypes None -localhost no -AlwaysShared \
    -DisconnectClients=0
fi

printf '%s\n' "$PASS" | vncpasswd -f > /tmp/vncpasswd
chmod 600 /tmp/vncpasswd

# Default TigerVNC is TLSVnc,VncAuth — a common real-world mix.
# TLSVnc-only is a negative fixture (probe has no TLS security type).
sec_args="-SecurityTypes VncAuth"
if [ "$SEC" = "Default" ]; then
  sec_args=""
elif [ -n "$SEC" ]; then
  sec_args="-SecurityTypes $SEC"
fi

exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
  -rfbauth /tmp/vncpasswd $sec_args \
  -localhost no -AlwaysShared -DisconnectClients=0
