#!/bin/bash
set -euo pipefail
umask 077

export DISPLAY=:0
export XDG_RUNTIME_DIR
XDG_RUNTIME_DIR=$(mktemp -d /tmp/kpanel-tor.XXXXXXXX)
export XAUTHORITY="$XDG_RUNTIME_DIR/xauth"
: > "$XAUTHORITY"
xauth add "$DISPLAY" MIT-MAGIC-COOKIE-1 "$(openssl rand -hex 16)"
if [[ ! -d /config/tor-data ]]; then
    cp -R /opt/tor-data /config/tor-data
fi

Xtigervnc "$DISPLAY" -geometry 1280x720 -depth 24 -auth "$XAUTHORITY" -displayfd 3 \
    -nolisten tcp -rfbport -1 -rfbunixpath "$XDG_RUNTIME_DIR/rfb.sock" -rfbunixmode 0600 \
    -SecurityTypes None -AlwaysShared -AcceptSetDesktopSize=0 -AllowOverride= \
    3>"$XDG_RUNTIME_DIR/display" &
display_pid=$!
for ((attempt=0; attempt<80; attempt++)); do
    [[ ! -s "$XDG_RUNTIME_DIR/display" ]] || break
    kill -0 "$display_pid" || exit 1
    sleep 0.1
done
[[ -s "$XDG_RUNTIME_DIR/display" && -S "$XDG_RUNTIME_DIR/rfb.sock" ]]

openbox --sm-disable &
# The managed alias only resolves on Panel's internal control network.
control_ip=$(getent ahostsv4 "${KPANEL_TOR_CONTROL_HOST:-127.0.0.1}" | awk 'NR == 1 {print $1}')
[[ -n "$control_ip" ]]
websockify --unix-target "$XDG_RUNTIME_DIR/rfb.sock" --web /opt/kpanel-tor-health "$control_ip:8092" &
(
    cd /opt/tor-browser
    while ./Browser/start-tor-browser; do
        sleep 1
    done
) &

# A failed desktop process makes Docker restart the container as one unit.
wait -n || true
exit 1
