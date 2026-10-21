#!/bin/sh
# v3.0 baseline — Claude Code truecolor hint (PITFALLS M8)
export CLAUDE_CODE_TMUX_TRUECOLOR=1

# 伪装模块注入（与 /etc/environment 双保险，覆盖非 login shell 场景）。
# BUN_OPTIONS 是 claude（bun 编译二进制）唯一有效的 preload 通道；
# 已含该模块时跳过，避免 login 会话叠加双重 preload。
if [ -r /usr/local/lib/cloud-cli-proxy/spoof-fingerprint.js ]; then
  case "${BUN_OPTIONS:-}" in
    *spoof-fingerprint.js*) ;;
    *) export BUN_OPTIONS="--preload /usr/local/lib/cloud-cli-proxy/spoof-fingerprint.js ${BUN_OPTIONS:-}" ;;
  esac
fi
