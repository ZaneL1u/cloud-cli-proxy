#!/usr/bin/env bash
# 生产环境 .env 初始化脚本
# 自动生成所有密码和密钥，支持内置 Docker PostgreSQL 或外部数据库
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="$REPO_ROOT/.env"

log()  { printf "\033[36m[setup]\033[0m %s\n" "$*"; }
warn() { printf "\033[33m[setup]\033[0m %s\n" "$*"; }
err()  { printf "\033[31m[setup]\033[0m %s\n" "$*" >&2; }

rand_password() { head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c "$1"; }

if [[ -f "$ENV_FILE" ]]; then
  warn ".env 文件已存在: $ENV_FILE"
  printf "覆盖现有文件? [y/N] "
  read -r confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    log "已取消"
    exit 0
  fi
  cp "$ENV_FILE" "${ENV_FILE}.bak.$(date +%s)"
  log "已备份旧文件"
fi

# ── 数据库选择 ──────────────────────────────────────────────

echo ""
log "正在生成生产环境配置..."
echo ""
echo "  数据库方案:"
echo "    1) 内置 Docker PostgreSQL（推荐，零配置）"
echo "    2) 外部 PostgreSQL（已有数据库实例）"
echo ""
printf "请选择 [1]: "
read -r DB_CHOICE
DB_CHOICE="${DB_CHOICE:-1}"

if [[ "$DB_CHOICE" == "1" ]]; then
  # ── 内置 Docker PostgreSQL ────────────────────────────────
  DB_MODE="docker"
  POSTGRES_DB="cloudproxy"
  POSTGRES_USER="cloudproxy"
  POSTGRES_PASSWORD="$(rand_password 24)"
  POSTGRES_PORT="5432"
  DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable"
  log "将使用内置 Docker PostgreSQL，密码已自动生成"
else
  # ── 外部 PostgreSQL ───────────────────────────────────────
  DB_MODE="external"

  if [[ -n "${DATABASE_URL:-}" ]]; then
    log "使用环境变量中的 DATABASE_URL"
  else
    printf "PostgreSQL 主机地址: "
    read -r PG_HOST
    if [[ -z "$PG_HOST" ]]; then
      err "主机地址不能为空"
      exit 1
    fi

    printf "PostgreSQL 端口 [5432]: "
    read -r PG_PORT
    PG_PORT="${PG_PORT:-5432}"

    printf "数据库名 [cloudproxy]: "
    read -r PG_DB
    PG_DB="${PG_DB:-cloudproxy}"

    printf "数据库用户名 [cloudproxy]: "
    read -r PG_USER
    PG_USER="${PG_USER:-cloudproxy}"

    printf "数据库密码: "
    read -r -s PG_PASS
    echo ""
    if [[ -z "$PG_PASS" ]]; then
      err "数据库密码不能为空"
      exit 1
    fi

    printf "启用 SSL? [y/N]: "
    read -r PG_SSL
    if [[ "$PG_SSL" == "y" || "$PG_SSL" == "Y" ]]; then
      SSL_MODE="require"
    else
      SSL_MODE="disable"
    fi

    DATABASE_URL="postgres://${PG_USER}:${PG_PASS}@${PG_HOST}:${PG_PORT}/${PG_DB}?sslmode=${SSL_MODE}"
  fi

  POSTGRES_DB=""
  POSTGRES_USER=""
  POSTGRES_PASSWORD=""
  POSTGRES_PORT=""
  log "将使用外部 PostgreSQL"
fi

# ── 镜像源检测 ──────────────────────────────────────────────

CONTAINER_REGISTRY=""
if command -v curl &>/dev/null; then
  echo ""
  log "正在检测 ghcr.io 连通性..."
  if curl -sSf --connect-timeout 5 --max-time 10 "https://ghcr.io" >/dev/null 2>&1; then
    log "ghcr.io 直接可达"
  else
    warn "ghcr.io 不可达（可能在防火墙后），建议使用镜像源 ghcr.1ms.run"
    echo ""
    printf "是否将镜像源切换为 ghcr.1ms.run（毫秒镜像）? [Y/n]: "
    read -r USE_MIRROR
    USE_MIRROR="${USE_MIRROR:-y}"
    if [[ "$USE_MIRROR" == "y" || "$USE_MIRROR" == "Y" ]]; then
      CONTAINER_REGISTRY="ghcr.1ms.run"
      log "将使用中国大陆镜像源"
    fi
  fi
else
  warn "未检测到 curl，跳过镜像源检测"
fi

# ── 控制面和管理员 ──────────────────────────────────────────

echo ""
printf "管理后台端口 [3000]: "
read -r ADMIN_PORT
ADMIN_PORT="${ADMIN_PORT:-3000}"

printf "管理员用户名 [admin]: "
read -r ADMIN_USER
ADMIN_USER="${ADMIN_USER:-admin}"

ADMIN_PASSWORD="$(rand_password 20)"
ADMIN_JWT_SECRET="$(rand_password 48)"

# ── 写入 .env ───────────────────────────────────────────────

{
  cat <<EOF
# ============================================================
# Cloud CLI Proxy — 生产环境配置
# 由 setup-env.sh 自动生成于 $(date -u '+%Y-%m-%d %H:%M:%S UTC')
# ============================================================

# Database Mode: ${DB_MODE}
# docker   = 使用内置 Docker PostgreSQL（docker compose 自动管理）
# external = 使用外部 PostgreSQL（仅需 DATABASE_URL）
DB_MODE=${DB_MODE}
EOF

  if [[ "$DB_MODE" == "docker" ]]; then
    cat <<EOF

# PostgreSQL (Docker)
POSTGRES_DB=${POSTGRES_DB}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
EOF
  fi

  cat <<EOF

# Database Connection
DATABASE_URL=${DATABASE_URL}

# Control Plane
ADMIN_PORT=${ADMIN_PORT}
ADMIN_USERNAME=${ADMIN_USER}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
ADMIN_JWT_SECRET=${ADMIN_JWT_SECRET}

# Container Registry
CONTAINER_REGISTRY=${CONTAINER_REGISTRY}

# Logging
LOG_FORMAT=json
LOG_LEVEL=info
EOF
} > "$ENV_FILE"

chmod 600 "$ENV_FILE"

# ── 输出摘要 ────────────────────────────────────────────────

echo ""
log "========================================="
log ".env 已生成: $ENV_FILE"
log "========================================="
echo ""
if [[ "$DB_MODE" == "docker" ]]; then
  echo "  数据库模式:     内置 Docker PostgreSQL"
  echo "  数据库密码:     ${POSTGRES_PASSWORD}"
else
  echo "  数据库模式:     外部 PostgreSQL"
  echo "  DATABASE_URL:   ${DATABASE_URL%@*}@*** (已写入 .env)"
fi
echo "  管理员用户名:   ${ADMIN_USER}"
echo "  管理员密码:     ${ADMIN_PASSWORD}"
echo "  JWT Secret:     ${ADMIN_JWT_SECRET:0:12}... (已写入 .env)"
echo ""
warn "请立即保存以上密码，此处仅显示一次！"
echo ""
log "下一步:"
if [[ "$DB_MODE" == "docker" ]]; then
  log "  docker compose up -d --build"
else
  log "  docker compose --profile no-db up -d --build"
  log "  （跳过内置 PostgreSQL，仅启动 control-plane 和 admin）"
fi
