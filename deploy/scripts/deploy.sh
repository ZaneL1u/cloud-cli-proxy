#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

log() { printf "[deploy] %s\n" "$*"; }
err() { printf "[deploy] ERROR: %s\n" "$*" >&2; }
die() { err "$@"; exit 1; }

if [[ $EUID -ne 0 ]]; then
  die "此脚本必须以 root 权限运行 (sudo bash $0)"
fi

if [[ ! -f "$REPO_ROOT/go.mod" ]]; then
  die "请在仓库根目录执行此脚本"
fi

cd "$REPO_ROOT"

# ── 1. 依赖检查 ──────────────────────────────────────────────
log "检查宿主机依赖..."
bash deploy/scripts/host-preflight.sh
log "依赖检查通过"

for cmd in go psql; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    die "缺少必需命令: $cmd — 请参考 docs/deployment-guide.md 安装"
  fi
done

# ── 2. 系统用户和目录 ────────────────────────────────────────
log "创建系统用户和目录..."
if ! id cloudproxy >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin cloudproxy
  usermod -aG docker cloudproxy
  log "创建系统用户 cloudproxy"
else
  log "系统用户 cloudproxy 已存在"
fi

mkdir -p /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy /opt/cloud-cli-proxy/bin
chown cloudproxy:cloudproxy /var/lib/cloud-cli-proxy /run/cloud-cli-proxy /etc/cloud-cli-proxy

# ── 3. 环境变量配置 ──────────────────────────────────────────
ENV_FILE="/etc/cloud-cli-proxy/env"
if [[ ! -f "$ENV_FILE" ]]; then
  log "生成环境变量配置文件..."

  if [[ -z "${DATABASE_URL:-}" ]]; then
    read -rp "PostgreSQL 连接字符串 (DATABASE_URL): " DATABASE_URL
  fi

  if [[ -z "${ADMIN_PASSWORD:-}" ]]; then
    read -rp "管理员密码 (ADMIN_PASSWORD): " ADMIN_PASSWORD
  fi

  JWT_SECRET="${ADMIN_JWT_SECRET:-$(head -c 48 /dev/urandom | base64 | tr -d '=+/' | head -c 48)}"

  cat > "$ENV_FILE" <<EOF
DATABASE_URL=${DATABASE_URL}
CONTROL_PLANE_ADDR=${CONTROL_PLANE_ADDR:-:8080}
ADMIN_USERNAME=${ADMIN_USERNAME:-admin}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
ADMIN_JWT_SECRET=${JWT_SECRET}
EOF

  chmod 600 "$ENV_FILE"
  chown cloudproxy:cloudproxy "$ENV_FILE"
  log "环境变量写入 $ENV_FILE"
  log "ADMIN_JWT_SECRET 已自动生成，请妥善保管"
else
  log "环境变量文件已存在: $ENV_FILE"
fi

# shellcheck source=/dev/null
source "$ENV_FILE"

# ── 4. 数据库初始化 ──────────────────────────────────────────
log "检查数据库连接..."
if ! psql "$DATABASE_URL" -c "SELECT 1" >/dev/null 2>&1; then
  die "无法连接数据库，请检查 DATABASE_URL 配置"
fi
log "数据库连接正常"

# ── 5. 构建二进制 ────────────────────────────────────────────
log "构建控制面..."
go build -o /opt/cloud-cli-proxy/bin/control-plane ./cmd/control-plane
log "控制面构建完成"

log "构建 host-agent..."
go build -o /opt/cloud-cli-proxy/bin/host-agent ./cmd/host-agent
log "host-agent 构建完成"

# ── 6. 构建受管镜像 ──────────────────────────────────────────
log "构建受管用户镜像..."
bash deploy/docker/managed-user/build-managed-image.sh
log "受管用户镜像构建完成"

# ── 7. 安装 systemd 服务 ─────────────────────────────────────
log "安装 systemd 服务..."
cp deploy/systemd/cloud-cli-proxy-control-plane.service /etc/systemd/system/
cp deploy/systemd/cloud-cli-proxy-host-agent.service /etc/systemd/system/

mkdir -p /etc/systemd/system/cloud-cli-proxy-control-plane.service.d
cat > /etc/systemd/system/cloud-cli-proxy-control-plane.service.d/env.conf <<'UNIT'
[Service]
EnvironmentFile=/etc/cloud-cli-proxy/env
UNIT

mkdir -p /etc/systemd/system/cloud-cli-proxy-host-agent.service.d
cat > /etc/systemd/system/cloud-cli-proxy-host-agent.service.d/env.conf <<'UNIT'
[Service]
EnvironmentFile=/etc/cloud-cli-proxy/env
UNIT

# ── 8. 部署项目文件 ──────────────────────────────────────────
log "同步项目文件到 /opt/cloud-cli-proxy/..."
rsync -a --exclude='.git' --exclude='node_modules' --exclude='.planning' \
  "$REPO_ROOT/" /opt/cloud-cli-proxy/ 2>/dev/null || \
  cp -r "$REPO_ROOT"/{deploy,internal,cmd,docs} /opt/cloud-cli-proxy/ 2>/dev/null || true

# ── 9. 启动服务 ──────────────────────────────────────────────
log "启动服务..."
systemctl daemon-reload
systemctl enable --now cloud-cli-proxy-control-plane
systemctl enable --now cloud-cli-proxy-host-agent

# ── 10. 健康检查 ─────────────────────────────────────────────
log "等待控制面就绪..."
HEALTHZ_URL="http://127.0.0.1${CONTROL_PLANE_ADDR:-:8080}/healthz"
for i in $(seq 1 15); do
  if curl -sf "$HEALTHZ_URL" >/dev/null 2>&1; then
    log "控制面健康检查通过"
    break
  fi
  if [[ $i -eq 15 ]]; then
    die "控制面健康检查超时 (15s)，请检查: journalctl -u cloud-cli-proxy-control-plane --no-pager -n 30"
  fi
  sleep 1
done

log "检查 host-agent 状态..."
if systemctl is-active --quiet cloud-cli-proxy-host-agent; then
  log "host-agent 运行正常"
else
  err "host-agent 未正常启动，请检查: journalctl -u cloud-cli-proxy-host-agent --no-pager -n 30"
fi

AUTO_REBUILD_ALL_HOSTS="${REBUILD_ALL_HOSTS_AFTER_DEPLOY:-0}"
if [[ "$AUTO_REBUILD_ALL_HOSTS" == "1" || "$AUTO_REBUILD_ALL_HOSTS" == "true" || "$AUTO_REBUILD_ALL_HOSTS" == "yes" ]]; then
  if ! command -v jq >/dev/null 2>&1; then
    die "REBUILD_ALL_HOSTS_AFTER_DEPLOY 已开启，但缺少 jq，请先安装 jq"
  fi

  log "REBUILD_ALL_HOSTS_AFTER_DEPLOY 已开启，开始批量重建所有主机 runner..."
  ADMIN_USERNAME="${ADMIN_USERNAME:-admin}" \
  ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
  bash deploy/scripts/rebuild-host-runners.sh \
    --scope all
  log "所有主机重建任务已入队"
fi

# ── 完成 ─────────────────────────────────────────────────────
log "========================================"
log "部署完成！"
log "  控制面: http://127.0.0.1${CONTROL_PLANE_ADDR:-:8080}"
log "  健康检查: curl ${HEALTHZ_URL}"
log "  管理后台: 使用 ADMIN_USERNAME / ADMIN_PASSWORD 登录"
log "  Bootstrap: curl -sSL http://YOUR_HOST:${CONTROL_PLANE_ADDR:-8080}/v1/bootstrap/script | bash"
log "========================================"
log "后续步骤:"
log "  1. 通过 Admin API 创建出口 IP（含 WireGuard 配置）"
log "  2. 创建用户并为其主机绑定出口 IP"
log "  3. 测试 bootstrap 流程"
log "  详见: docs/operations-manual.md"
