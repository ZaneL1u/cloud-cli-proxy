#!/usr/bin/env bash
# OAuth 浏览器入口 — claude /login 等"需要打开浏览器"的认证流程统一走这里。
#
# 背景：claude 的 OAuth 授权会打印 https://claude.ai/oauth/authorize?...&redirect_uri=http://localhost:PORT
# 并经 $BROWSER 打开浏览器。若用户用本机浏览器完成认证：
#   1. 认证流量走用户本地网络（出口 IP 与容器 API 会话不一致，触发风控关联）
#   2. redirect_uri 指向容器内 localhost，本机浏览器根本回调不到
# 因此浏览器必须与 claude 同容器：本脚本在 VNC 桌面（DISPLAY=:99）的 Chromium
# 中打开授权页，浏览器全流量经 sing-box tun 出口，回调走容器内 localhost 直达。
#
# 用户通过产品的 VNC 入口（websocket 6080）看到并操作该浏览器。

set -euo pipefail

URL="${1:-}"
DISPLAY="${DISPLAY:-:99}"
export DISPLAY

if [[ -z "${URL}" ]]; then
  echo "[browser-open] no URL given" >&2
  exit 1
fi

if ! xdpyinfo >/dev/null 2>&1; then
  # 桌面未就绪（MODE=local 或桌面栈故障）：不阻塞 claude，给出人工路径
  echo "[browser-open] desktop not ready — connect via VNC (ws 6080) and open:" >&2
  echo "${URL}" >&2
  exit 0
fi

browser_cmd=""
for candidate in chromium chromium-browser google-chrome; do
  if command -v "${candidate}" >/dev/null 2>&1; then
    browser_cmd="${candidate}"
    break
  fi
done
if [[ -z "${browser_cmd}" ]]; then
  echo "[browser-open] no chromium binary found" >&2
  exit 1
fi

# 与桌面 Chromium 共用同一 user-data-dir：已有实例时自动开新标签并复用会话。
# WebRTC 防泄漏 flags 与 launch-chromium.sh 保持一致（UDP 不走非代理路径）。
nohup "${browser_cmd}" \
  --no-sandbox \
  --disable-dev-shm-usage \
  --user-data-dir=/workspace/.chrome-data \
  --no-first-run \
  --disable-gpu \
  --disable-features=WebRtcHideLocalIpsWithMdns \
  --enforce-webrtc-ip-permission-check \
  --force-webrtc-ip-handling-policy=disable_non_proxied_udp \
  "${URL}" >/dev/null 2>&1 &

# 立即返回，避免调用方（claude 的 open 调用）等待浏览器退出
exit 0
