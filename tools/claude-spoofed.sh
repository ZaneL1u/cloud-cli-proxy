#!/usr/bin/env bash
#
# 以伪装身份启动 Claude Code
#
# 注入通道实测结论（claude 2.1.304，bun 编译二进制）：
#   - NODE_OPTIONS="--require"：对 bun 编译版无效，仅适用于 node 直跑旧版 cli.js
#   - BUN_OPTIONS="--preload"：对 bun 编译版有效，是当前唯一有效的注入通道
#
# 用法：
#   ./claude-spoofed.sh                           # 默认伪装
#   SPOOF_HOSTNAME=my-vm ./claude-spoofed.sh      # 自定义主机名
#   SPOOF_DEBUG=1 ./claude-spoofed.sh             # 打印伪装信息
#
# 可配合代理使用（推荐）：
#   HTTPS_PROXY=http://127.0.0.1:8888 ./claude-spoofed.sh
#   （claude 经 undici EnvHttpProxyAgent 读取该变量；遥测事件中不含
#    "使用代理"字段，不会因此被额外标记。全流量隧道场景无需设置。）
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SPOOF_SCRIPT="$SCRIPT_DIR/spoof-fingerprint.js"

if [ ! -f "$SPOOF_SCRIPT" ]; then
  echo "Error: spoof-fingerprint.js not found at $SPOOF_SCRIPT"
  exit 1
fi

# 如果没有设置伪装 hostname，生成一个稳定的
export SPOOF_HOSTNAME="${SPOOF_HOSTNAME:-cloud-vm-$(echo -n "$(whoami)-$(date +%Y%m)" | shasum | cut -c1-6)}"

# 注入伪装模块（BUN_OPTIONS 对 bun 编译版生效；NODE_OPTIONS 兼容 node 旧版）
export NODE_OPTIONS="--require $SPOOF_SCRIPT ${NODE_OPTIONS:-}"
export BUN_OPTIONS="--preload $SPOOF_SCRIPT ${BUN_OPTIONS:-}"

# ── 遥测阻断环境变量 ─────────────────────────────────────────
export DISABLE_TELEMETRY=1
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=
export DO_NOT_TRACK=1
export OTEL_SDK_DISABLED=true
export OTEL_TRACES_EXPORTER=none
export OTEL_METRICS_EXPORTER=none
export OTEL_LOGS_EXPORTER=none
export SENTRY_DSN=
export DISABLE_ERROR_REPORTING=1
export TELEMETRY_DISABLED=1

exec claude "$@"
