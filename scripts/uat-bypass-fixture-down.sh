#!/usr/bin/env bash
# scripts/uat-bypass-fixture-down.sh — Phase 47 TD-01 fixture 拆除脚本
#
# 与 scripts/uat-bypass-fixture-up.sh 配对，best-effort 清理：
#   - 杀掉 control-plane（按 pidfile）
#   - 停掉 PostgreSQL 容器
#   - 停掉 mock worker / gateway 容器
#   - 删除 /tmp/uat-fixture.json 与 .uat-fixture.env
#
# 设计原则：任何步骤失败都不抛错，保证 CI cleanup 步 always 成功。

set -u
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

PG_CONTAINER="${PG_CONTAINER:-uat-bypass-pg}"
CONTROL_PLANE_PIDFILE="${CONTROL_PLANE_PIDFILE:-/tmp/uat-bypass-control-plane.pid}"
CONTROL_PLANE_LOG="${CONTROL_PLANE_LOG:-/tmp/uat-bypass-control-plane.log}"
FIXTURE_OUTPUT_JSON="${FIXTURE_OUTPUT_JSON:-/tmp/uat-fixture.json}"
FIXTURE_ENV_FILE="${FIXTURE_ENV_FILE:-${PROJECT_ROOT}/.uat-fixture.env}"

info() { echo "[INFO]  $1"; }
ok()   { echo "[OK]    $1"; }
warn() { echo "[WARN]  $1"; }

# ─── 1. 停 control-plane ───────────────────────────────────────────────────
if [[ -f "$CONTROL_PLANE_PIDFILE" ]]; then
  PID="$(cat "$CONTROL_PLANE_PIDFILE" 2>/dev/null || true)"
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    info "停止 control-plane PID=$PID"
    # SIGINT 给 control-plane signal.NotifyContext 一个优雅退出窗口
    kill -INT "$PID" 2>/dev/null || true
    for i in 1 2 3 4 5; do
      if ! kill -0 "$PID" 2>/dev/null; then
        break
      fi
      sleep 1
    done
    if kill -0 "$PID" 2>/dev/null; then
      warn "control-plane 5s 内未退出，发送 SIGKILL"
      kill -KILL "$PID" 2>/dev/null || true
    fi
    ok "control-plane 已停止"
  else
    info "control-plane PID 文件存在但进程已退出"
  fi
  rm -f "$CONTROL_PLANE_PIDFILE"
fi

# go run 会产生 child；用 pkill 兜底（best-effort，匹配模式仅本仓库 cmd 路径）
if command -v pkill >/dev/null 2>&1; then
  pkill -f "go run ./cmd/control-plane" 2>/dev/null || true
  pkill -f "/cmd/control-plane/control-plane" 2>/dev/null || true
fi

# ─── 2. 停 PostgreSQL 容器 ─────────────────────────────────────────────────
if command -v docker >/dev/null 2>&1; then
  if docker ps -a --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
    info "删除 PostgreSQL 容器 $PG_CONTAINER"
    docker rm -f "$PG_CONTAINER" >/dev/null 2>&1 || true
    ok "$PG_CONTAINER 已删除"
  fi

  # ─── 3. 删 mock worker / gateway 容器（按 cloudproxy-* 前缀清扫）─────────
  # 优先从 /tmp/uat-fixture.json 读 host_id，定位精确容器名；
  # 兜底按 cloud-cli-proxy label 全扫一遍。
  if [[ -f "$FIXTURE_OUTPUT_JSON" ]] && command -v jq >/dev/null 2>&1; then
    HOST_ID="$(jq -r '.host_id // empty' "$FIXTURE_OUTPUT_JSON" 2>/dev/null || true)"
    if [[ -n "$HOST_ID" ]]; then
      for c in "cloudproxy-${HOST_ID}" "cloudproxy-gw-${HOST_ID}"; do
        if docker ps -a --format '{{.Names}}' | grep -qx "$c"; then
          info "删除容器 $c"
          docker rm -f "$c" >/dev/null 2>&1 || true
        fi
      done
    fi
  fi

  # 兜底：按 label 过滤；仅清理 uat-bypass fixture 自己打的 label，避免误伤
  CANDIDATES=$(docker ps -a \
    --filter "label=com.cloud-cli-proxy.managed=true" \
    --format '{{.Names}}' 2>/dev/null || true)
  for c in $CANDIDATES; do
    case "$c" in
      cloudproxy-*)
        info "label 兜底删除容器 $c"
        docker rm -f "$c" >/dev/null 2>&1 || true
        ;;
    esac
  done
fi

# ─── 4. 清理 fixture 输出文件 ──────────────────────────────────────────────
rm -f "$FIXTURE_OUTPUT_JSON" "$FIXTURE_ENV_FILE" "$CONTROL_PLANE_LOG" 2>/dev/null || true

ok "fixture 已清理"
exit 0
