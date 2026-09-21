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

# Anonymous TLSVnc/TLSNone are started as-is (stdlib cannot speak them).
# Certificate TLS uses X509Vnc/X509None with a throwaway cert.
make_x509() {
  openssl req -x509 -newkey rsa:2048 -keyout /tmp/vnc.key -out /tmp/vnc.crt \
    -days 1 -nodes -subj "/CN=localhost" >/tmp/openssl.log 2>&1
}

if [ "$SEC" = "TLSVnc" ]; then
  exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
    -rfbauth /tmp/vncpasswd -SecurityTypes TLSVnc \
    -localhost no -AlwaysShared -DisconnectClients=0
fi

if [ "$SEC" = "TLSNone" ]; then
  exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
    -SecurityTypes TLSNone \
    -localhost no -AlwaysShared -DisconnectClients=0
fi

if [ "$SEC" = "X509Vnc" ]; then
  make_x509
  exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
    -rfbauth /tmp/vncpasswd -SecurityTypes X509Vnc \
    -X509Cert /tmp/vnc.crt -X509Key /tmp/vnc.key \
    -localhost no -AlwaysShared -DisconnectClients=0
fi

if [ "$SEC" = "X509None" ]; then
  make_x509
  exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
    -SecurityTypes X509None -X509Cert /tmp/vnc.crt -X509Key /tmp/vnc.key \
    -localhost no -AlwaysShared -DisconnectClients=0
fi

sec_args="-SecurityTypes VncAuth"
if [ "$SEC" = "Default" ]; then
  sec_args=""
elif [ -n "$SEC" ]; then
  sec_args="-SecurityTypes $SEC"
fi

exec Xvnc :1 -geometry 640x480 -depth 16 -rfbport "$PORT" \
  -rfbauth /tmp/vncpasswd $sec_args \
  -localhost no -AlwaysShared -DisconnectClients=0
