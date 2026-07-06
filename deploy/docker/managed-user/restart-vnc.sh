#!/usr/bin/env bash
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo -n /usr/local/bin/restart-vnc "$@"
fi

RUN_USER="${CONTAINER_USER:-workspace}"
if ! id "${RUN_USER}" >/dev/null 2>&1; then
  RUN_USER="workspace"
fi

LOG_DIR=/workspace/.vnc
XVNC_LOG="${LOG_DIR}/xvnc.log"
FLUXBOX_LOG="${LOG_DIR}/fluxbox.log"
DESKTOP_LOG="${LOG_DIR}/desktop.log"
DESKTOP_DIR=/workspace/Desktop
PCMANFM_PROFILE_DIR=/workspace/.config/pcmanfm/default
DESKTOP_LANG="${DESKTOP_LANG:-en_US.UTF-8}"
DESKTOP_LANGUAGE="${DESKTOP_LANGUAGE:-en_US:en}"
DESKTOP_LC_ALL="${DESKTOP_LC_ALL:-$DESKTOP_LANG}"

write_desktop_config() {
  mkdir -p "${DESKTOP_DIR}" "${PCMANFM_PROFILE_DIR}" /workspace/.chrome-data

  cat > "${DESKTOP_DIR}/Chrome.desktop" <<'DESKTOP'
[Desktop Entry]
Version=1.0
Type=Application
Name=Browser
Comment=Open the browser
Exec=/usr/local/bin/launch-chromium.sh
Icon=chromium
Terminal=false
StartupNotify=true
Categories=Network;WebBrowser;
DESKTOP

  cat > "${PCMANFM_PROFILE_DIR}/desktop-items-0.conf" <<'CONF'
[*]
desktop_bg=#17324d
desktop_fg=#f5f7ff
desktop_shadow=#1b1f2a
show_wm_menu=0
wallpaper_mode=color
sort=name;ascending;
show_documents=0
show_trash=0
show_mounts=0
CONF

  chmod 0755 "${DESKTOP_DIR}/Chrome.desktop"
  chown -R "${RUN_USER}:${RUN_USER}" "${DESKTOP_DIR}" /workspace/.config /workspace/.chrome-data
}

run_desktop_process() {
  runuser -u "${RUN_USER}" -- env \
    DISPLAY="${DISPLAY:-:99}" \
    HOME=/workspace \
    LANG="${DESKTOP_LANG}" \
    LANGUAGE="${DESKTOP_LANGUAGE}" \
    LC_ALL="${DESKTOP_LC_ALL}" \
    XDG_CURRENT_DESKTOP=cloud-cli-proxy \
    "$@"
}

mkdir -p "${LOG_DIR}" /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix
touch "${XVNC_LOG}" "${FLUXBOX_LOG}" "${DESKTOP_LOG}"
chown -R "${RUN_USER}:${RUN_USER}" "${LOG_DIR}"
write_desktop_config

pkill -f 'Xvnc :99' || true
pkill -u "${RUN_USER}" -x fluxbox || true
pkill -u "${RUN_USER}" -f 'pcmanfm --desktop --profile default' || true

export DISPLAY=:99
run_desktop_process Xvnc :99 \
  -geometry 1920x1080 \
  -depth 24 \
  -websocketPort 6080 \
  -SecurityTypes None \
  -interface 0.0.0.0 \
  -BlacklistThreshold 0 \
  -FreeKeyMappings \
  -disableBasicAuth \
  -publicIP 127.0.0.1 \
  -httpd /usr/share/kasmvnc/www >>"${XVNC_LOG}" 2>&1 &

ready=0
for _ in $(seq 1 30); do
  if DISPLAY=:99 xdpyinfo >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [ "${ready}" -ne 1 ]; then
  echo "Xvnc did not become ready on DISPLAY :99 within 30 seconds" >>"${XVNC_LOG}"
  tail -n 80 "${XVNC_LOG}" >&2 || true
  exit 1
fi

run_desktop_process xsetroot -solid "#17324d" >/dev/null 2>&1 || true
run_desktop_process fluxbox >>"${FLUXBOX_LOG}" 2>&1 &
run_desktop_process pcmanfm --desktop --profile default >>"${DESKTOP_LOG}" 2>&1 &

echo "VNC restarted (display=:99 websocket=6080 user=${RUN_USER})"
