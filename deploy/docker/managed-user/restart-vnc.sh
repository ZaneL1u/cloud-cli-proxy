#!/usr/bin/env bash
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo -n /usr/local/bin/restart-vnc "$@"
fi

ACTION="${1:-restart}"
case "${ACTION}" in
  start|restart) ;;
  *)
    echo "usage: restart-vnc [start|restart]" >&2
    exit 2
    ;;
esac

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
DISPLAY_VALUE="${DISPLAY:-:99}"
WEBSOCKET_PORT="${VNC_WEBSOCKET_PORT:-6080}"
STATE_FILE="${VNC_WATCH_STATE:-/run/cloud-cli-proxy-vnc-watch.state}"

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

write_kasmvnc_config() {
  mkdir -p /workspace/.vnc
  cat > /workspace/.vnc/kasmvnc.yaml <<'YAML'
network:
  protocol: http
  websocket_port: 6080
  ssl:
    require_ssl: false
    pem_certificate:
    pem_key:
  udp:
    public_ip: 127.0.0.1
    stun_server:
desktop:
  resolution:
    width: 1920
    height: 1080
  allow_resize: true
  pixel_depth: 24
keyboard:
  remap_keys:
  ignore_numlock: false
  raw_keyboard: false
pointer:
  enabled: true
runtime_configuration:
  allow_client_to_override_kasm_server_settings: true
  allow_override_standard_vnc_server_settings: true
  allow_override_list:
    - pointer.enabled
    - desktop.allow_resize
    - desktop.resolution
encoding:
  max_frame_rate: 30
  rect_encoding_mode:
    min_quality: 7
    max_quality: 8
    consider_lossless_quality: 10
    rectangle_compress_threads: 2
YAML
  echo -e "kasmpass\nkasmpass\n" | kasmvncpasswd -u "${RUN_USER}" -w /workspace/.vnc/passwd 2>/dev/null || true
  chown -R "${RUN_USER}:${RUN_USER}" /workspace/.vnc
}

run_desktop_process() {
  runuser -u "${RUN_USER}" -- env \
    DISPLAY="${DISPLAY_VALUE}" \
    HOME=/workspace \
    LANG="${DESKTOP_LANG}" \
    LANGUAGE="${DESKTOP_LANGUAGE}" \
    LC_ALL="${DESKTOP_LC_ALL}" \
    XDG_CURRENT_DESKTOP=cloud-cli-proxy \
    "$@"
}

is_vnc_running() {
  /usr/local/bin/vnc-status --json | grep -q '"running":true'
}

wait_for_x_display() {
  local deadline=$((SECONDS + 30))
  while (( SECONDS < deadline )); do
    if DISPLAY="${DISPLAY_VALUE}" xdpyinfo >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

reset_vnc_watch_state() {
  mkdir -p "$(dirname "${STATE_FILE}")"
  cat > "${STATE_FILE}" <<STATE
window_start=0
attempts=0
auto_restart_limited=0
last_success=$(date +%s)
STATE
}

mkdir -p "${LOG_DIR}" /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix
touch "${XVNC_LOG}" "${FLUXBOX_LOG}" "${DESKTOP_LOG}"
chown -R "${RUN_USER}:${RUN_USER}" "${LOG_DIR}"
write_kasmvnc_config
write_desktop_config

if [ "${ACTION}" = "start" ] && is_vnc_running; then
  if [ "${VNC_AUTO_START:-0}" != "1" ]; then
    reset_vnc_watch_state
  fi
  echo "VNC already running (display=${DISPLAY_VALUE} websocket=${WEBSOCKET_PORT} user=${RUN_USER})"
  exit 0
fi

pkill -f "Xvnc ${DISPLAY_VALUE}" || true
pkill -u "${RUN_USER}" -x fluxbox || true
pkill -u "${RUN_USER}" -f 'pcmanfm --desktop --profile default' || true

rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
export DISPLAY="${DISPLAY_VALUE}"

if ! is_vnc_running; then
  run_desktop_process Xvnc "${DISPLAY_VALUE}" \
    -geometry 1920x1080 \
    -depth 24 \
    -websocketPort "${WEBSOCKET_PORT}" \
    -SecurityTypes None \
    -interface 0.0.0.0 \
    -BlacklistThreshold 0 \
    -FreeKeyMappings \
    -disableBasicAuth \
    -publicIP 127.0.0.1 \
    -httpd /usr/share/kasmvnc/www >>"${XVNC_LOG}" 2>&1 &
fi

if ! wait_for_x_display; then
  echo "Xvnc did not become ready on DISPLAY ${DISPLAY_VALUE} within 30 seconds" >>"${XVNC_LOG}"
  tail -n 80 "${XVNC_LOG}" >&2 || true
  exit 1
fi

run_desktop_process xsetroot -solid "#17324d" >/dev/null 2>&1 || true
run_desktop_process fluxbox >>"${FLUXBOX_LOG}" 2>&1 &
run_desktop_process pcmanfm --desktop --profile default >>"${DESKTOP_LOG}" 2>&1 &

if [ "${VNC_AUTO_START:-0}" != "1" ]; then
  reset_vnc_watch_state
fi
echo "VNC ${ACTION}ed (display=${DISPLAY_VALUE} websocket=${WEBSOCKET_PORT} user=${RUN_USER})"
