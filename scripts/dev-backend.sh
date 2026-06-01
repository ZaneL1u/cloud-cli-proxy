#!/bin/bash
# scripts/dev-backend.sh — Go 后端热重载脚本
# 编译 control-plane 并在 .go 文件变更时自动重建重启。
set -uo pipefail

BIN="/tmp/cloud-cli-proxy-dev"
PID=""
DATA_DIR="${DATA_DIR:-$(pwd)/.data}"

cleanup() {
    [ -n "${PID:-}" ] && kill "$PID" 2>/dev/null || true
    [ -n "${PID:-}" ] && wait "$PID" 2>/dev/null || true
    rm -f "$BIN"
    exit 0
}
trap cleanup INT TERM EXIT

build() {
    echo "[dev-backend] building..."
    # 清除 Makefile 导出的 GOOS/GOARCH，使用本机平台编译。
    # Makefile 的 GOOS=linux 是给 release build 用的，开发机必须在本地平台运行。
    if GOOS= GOARCH= go build -o "$BIN" ./cmd/control-plane; then
        echo "[dev-backend] built."
        return 0
    else
        echo "[dev-backend] build failed."
        return 1
    fi
}

start() {
    HOST_AGENT_MODE=embedded DATA_DIR="$DATA_DIR" DATABASE_URL="file:${DATA_DIR}/cloud-cli-proxy.db?_texttotime=true" "$BIN" &
    PID=$!
    echo "[dev-backend] started (PID $PID)"
}

restart() {
    if [ -n "${PID:-}" ]; then
        kill "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    start
}

# Initial build and start
if build; then
    start
fi

# Watch loop
while true; do
    sleep 2

    if [ ! -f "$BIN" ]; then
        # Binary missing (initial build failed) — keep trying
        if build; then
            start
        fi
        continue
    fi

    CHANGED=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./web/*" -newer "$BIN" 2>/dev/null || true)
    if [ -n "$CHANGED" ]; then
        echo "[dev-backend] detected change, restarting..."
        if build; then
            restart
        fi
    fi
done
