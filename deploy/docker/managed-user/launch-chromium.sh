#!/usr/bin/env bash
set -euo pipefail

export DISPLAY="${DISPLAY:-:99}"
export HOME="${HOME:-/workspace}"
export LANG="${DESKTOP_LANG:-${LANG:-en_US.UTF-8}}"
export LANGUAGE="${DESKTOP_LANGUAGE:-${LANGUAGE:-en_US:en}}"
export LC_ALL="${DESKTOP_LC_ALL:-${LC_ALL:-$LANG}}"

CHROMIUM_LANG="${CHROMIUM_LANG:-en-US}"
CHROMIUM_ACCEPT_LANG="${CHROMIUM_ACCEPT_LANG:-en-US,en;q=0.9}"
CHROMIUM_WINDOW_SIZE="${CHROMIUM_WINDOW_SIZE:-1880,1000}"
CHROMIUM_RENDERER_PROCESS_LIMIT="${CHROMIUM_RENDERER_PROCESS_LIMIT:-6}"

browser_cmd=""
for candidate in chromium chromium-browser google-chrome; do
  if command -v "${candidate}" >/dev/null 2>&1; then
    browser_cmd="${candidate}"
    break
  fi
done

if [[ -z "${browser_cmd}" ]]; then
  exec xterm -fa Monospace -fs 12 -geometry 120x30+60+60 -title "cloud-cli-proxy desktop" \
    -e bash -lc "echo Chromium is not installed.; exec bash"
fi

# 带参数（URL / flags）时同样附加防泄漏与固定 profile 参数，保证
# OAuth 等场景新开的标签与桌面会话一致（chromium 自动复用已有实例）。
if [[ $# -gt 0 ]]; then
  exec "${browser_cmd}" \
    --no-sandbox \
    --disable-dev-shm-usage \
    --user-data-dir=/workspace/.chrome-data \
    --lang="${CHROMIUM_LANG}" \
    --accept-lang="${CHROMIUM_ACCEPT_LANG}" \
    --no-first-run \
    --disable-gpu \
    --disable-features=WebRtcHideLocalIpsWithMdns \
    --enforce-webrtc-ip-permission-check \
    --force-webrtc-ip-handling-policy=disable_non_proxied_udp \
    "$@"
fi

exec "${browser_cmd}" \
  --no-sandbox \
  --disable-dev-shm-usage \
  --user-data-dir=/workspace/.chrome-data \
  --lang="${CHROMIUM_LANG}" \
  --accept-lang="${CHROMIUM_ACCEPT_LANG}" \
  --start-maximized \
  --renderer-process-limit="${CHROMIUM_RENDERER_PROCESS_LIMIT}" \
  --no-first-run \
  --disable-gpu \
  --disable-features=WebRtcHideLocalIpsWithMdns \
  --enforce-webrtc-ip-permission-check \
  --force-webrtc-ip-handling-policy=disable_non_proxied_udp \
  --window-position=0,0 \
  --window-size="${CHROMIUM_WINDOW_SIZE}" \
  "https://www.google.com"
