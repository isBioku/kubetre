#!/bin/sh
# Starts the X server (VNC on :5901, password-protected) and the XFCE session.
# Network policy admits VNC only from the access gateway's guacd pods.
set -eu

: "${VNC_PASSWORD:?VNC_PASSWORD must be set}"
mkdir -p "$XDG_RUNTIME_DIR" "$HOME/.vnc"
chmod 700 "$XDG_RUNTIME_DIR"
printf '%s\n' "$VNC_PASSWORD" | vncpasswd -f > "$HOME/.vnc/passwd"
chmod 600 "$HOME/.vnc/passwd"

Xtigervnc "$DISPLAY" -geometry "$RESOLUTION" -depth 24 -rfbport 5901 \
  -SecurityTypes VncAuth -PasswordFile "$HOME/.vnc/passwd" -AlwaysShared \
  -AcceptCutText=0 -SendCutText=0 &
xpid=$!

i=0
until [ -e "/tmp/.X11-unix/X${DISPLAY#:}" ] || [ $i -ge 50 ]; do sleep 0.2; i=$((i+1)); done

dbus-launch --exit-with-session startxfce4 >/tmp/xfce.log 2>&1 &

trap 'kill $xpid 2>/dev/null' TERM INT
wait $xpid
