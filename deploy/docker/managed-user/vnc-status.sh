#!/usr/bin/env bash
set -euo pipefail

DISPLAY_VALUE="${DISPLAY:-:99}"
WEBSOCKET_PORT="${VNC_WEBSOCKET_PORT:-6080}"
STATE_FILE="${VNC_WATCH_STATE:-/run/cloud-cli-proxy-vnc-watch.state}"

state_auto_restart_limited=0
if [ -f "${STATE_FILE}" ]; then
  # shellcheck source=/dev/null
  . "${STATE_FILE}" 2>/dev/null || true
  state_auto_restart_limited="${auto_restart_limited:-0}"
fi

is_vnc_running() {
  pgrep -f "Xvnc ${DISPLAY_VALUE}" >/dev/null 2>&1 || return 1
  DISPLAY="${DISPLAY_VALUE}" xdpyinfo >/dev/null 2>&1 || return 1
  ss -H -ltn "sport = :${WEBSOCKET_PORT}" 2>/dev/null | grep -q .
}

json_bool() {
  case "${1:-0}" in
    1|true|yes) printf 'true' ;;
    *) printf 'false' ;;
  esac
}

if is_vnc_running; then
  status="running"
  running=1
  can_start=0
  can_restart=1
else
  status="stopped"
  running=0
  can_start=1
  can_restart=0
fi

if [ "${1:-}" = "--json" ]; then
  cat <<JSON
{"status":"${status}","running":$(json_bool "${running}"),"can_start":$(json_bool "${can_start}"),"can_restart":$(json_bool "${can_restart}"),"auto_restart_limited":$(json_bool "${state_auto_restart_limited}"),"display":"${DISPLAY_VALUE}","websocket_port":${WEBSOCKET_PORT}}
JSON
else
  printf 'status=%s running=%s can_start=%s can_restart=%s auto_restart_limited=%s display=%s websocket_port=%s\n' \
    "${status}" "${running}" "${can_start}" "${can_restart}" "${state_auto_restart_limited}" "${DISPLAY_VALUE}" "${WEBSOCKET_PORT}"
fi
