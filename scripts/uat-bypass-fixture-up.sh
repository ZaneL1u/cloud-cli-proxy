#!/usr/bin/env bash
# scripts/uat-bypass-fixture-up.sh — Phase 47 TD-01 fixture builder
#
# 为 scripts/uat-bypass.sh 与 .github/workflows/uat-bypass.yml 准备 e2e
# 运行环境。fixture 契约（与 uat-bypass.yml 对齐）：
#
#   1. 启动 PostgreSQL（Docker，端口 5433，专用 fixture 卷）
#   2. 启动 control-plane（go run ./cmd/control-plane），自动跑 migrations
#      到含 0019 host_bypass_rules + 0020 host_bypass_snapshot_source
#   3. POST /v1/auth/login 拿 admin JWT
#   4. 通过 admin API 准备 fixture user / egress IP / host 三件套
#   5. 在 Linux runner 上额外起 mock worker / gateway 容器
#   6. 输出 /tmp/uat-fixture.json（含 host_id + admin_token）+ .uat-fixture.env
#      让 uat-bypass.sh 与 CI workflow 都能消费
#
# 使用：
#   scripts/uat-bypass-fixture-up.sh             # 真实启动（CI / Linux）
#   scripts/uat-bypass-fixture-up.sh --dry-run   # 仅打印计划，本地烟测可用
#   scripts/uat-bypass-fixture-up.sh --help      # 显示帮助
#
# 退出码：
#   0  fixture 就绪（或 --dry-run 成功打印）
#   1  失败（缺工具 / API 不通 / 容器启动超时等）
#
# 失败时 trap 自动调用 scripts/uat-bypass-fixture-down.sh 清理。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ─── 配置（环境变量可覆盖）─────────────────────────────────────────────────
PG_CONTAINER="${PG_CONTAINER:-uat-bypass-pg}"
PG_PORT="${PG_PORT:-5433}"
PG_USER="${PG_USER:-cloud}"
PG_PASSWORD="${PG_PASSWORD:-cloudtest}"
PG_DB="${PG_DB:-cloud_cli_proxy}"
PG_IMAGE="${PG_IMAGE:-postgres:18}"

CONTROL_PLANE_ADDR="${CONTROL_PLANE_ADDR:-:8080}"
API_BASE="${API_BASE:-http://localhost:8080}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
# 默认 admin 密码仅用于 fixture；真实部署一律通过 ADMIN_PASSWORD 覆盖
ADMIN_PASSWORD="${ADMIN_PASSWORD:-uat-bypass-admin-pw}"
# JWT secret 同上：fixture 默认值不要进真实运行环境
ADMIN_JWT_SECRET="${ADMIN_JWT_SECRET:-uat-bypass-jwt-secret-do-not-use-in-prod}"

FIXTURE_USER_NAME="${FIXTURE_USER_NAME:-uatbypassuser}"
FIXTURE_EGRESS_LABEL="${FIXTURE_EGRESS_LABEL:-uat-bypass-egress}"
FIXTURE_EGRESS_IP="${FIXTURE_EGRESS_IP:-203.0.113.10}"
FIXTURE_PROXY_SERVER="${FIXTURE_PROXY_SERVER:-127.0.0.1}"
FIXTURE_PROXY_PORT="${FIXTURE_PROXY_PORT:-1080}"

# uat-bypass.sh 使用 cloudproxy-${host_id} 和 cloudproxy-gw-${host_id} 作为容器
# 名前缀；这里给定一个稳定 short_id（便于 uat-bypass.sh 预测）
FIXTURE_HOST_SHORT="${FIXTURE_HOST_SHORT:-uatbypasshost}"

CONTROL_PLANE_PIDFILE="${CONTROL_PLANE_PIDFILE:-/tmp/uat-bypass-control-plane.pid}"
CONTROL_PLANE_LOG="${CONTROL_PLANE_LOG:-/tmp/uat-bypass-control-plane.log}"
FIXTURE_OUTPUT_JSON="${FIXTURE_OUTPUT_JSON:-/tmp/uat-fixture.json}"
FIXTURE_ENV_FILE="${FIXTURE_ENV_FILE:-${PROJECT_ROOT}/.uat-fixture.env}"

# 颜色 / 计数（与 uat-bypass.sh 风格对齐）
STEP=0
DRY_RUN=false

if [[ -t 1 ]]; then
  C_GREEN=$'\033[32m'
  C_RED=$'\033[31m'
  C_YELLOW=$'\033[33m'
  C_CYAN=$'\033[36m'
  C_RESET=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_YELLOW=""; C_CYAN=""; C_RESET=""
fi

step() { STEP=$((STEP + 1)); echo "${C_CYAN}[STEP ${STEP}]${C_RESET} $1"; }
info() { echo "${C_CYAN}[INFO]${C_RESET}  $1"; }
ok()   { echo "${C_GREEN}[OK]${C_RESET}    $1"; }
warn() { echo "${C_YELLOW}[WARN]${C_RESET}  $1"; }
err()  { echo "${C_RED}[ERR]${C_RESET}   $1" >&2; }

usage() {
  cat <<'EOF'
uat-bypass-fixture-up.sh — 为 uat-bypass.sh 准备完整 e2e fixture

用途:
  - 起 PostgreSQL（Docker）+ control-plane（go run），等待健康
  - 通过 admin API 创建测试 user / egress IP / host 三件套
  - 在 Linux runner 上额外起 mock worker / gateway 容器
  - 输出 /tmp/uat-fixture.json（CI workflow 契约）+ .uat-fixture.env

用法:
  scripts/uat-bypass-fixture-up.sh [--dry-run] [--help]

选项:
  --dry-run   只打印步骤计划，不真起容器 / 不下任何写操作
              （macOS 本地烟测：缺 nft / docker / sudo 都能跑通）
  --help, -h  显示本帮助

环境变量（按需覆盖）:
  PG_CONTAINER / PG_PORT / PG_USER / PG_PASSWORD / PG_DB / PG_IMAGE
  CONTROL_PLANE_ADDR / API_BASE
  ADMIN_USERNAME / ADMIN_PASSWORD / ADMIN_JWT_SECRET
  FIXTURE_USER_NAME / FIXTURE_EGRESS_LABEL / FIXTURE_EGRESS_IP
  FIXTURE_PROXY_SERVER / FIXTURE_PROXY_PORT / FIXTURE_HOST_SHORT
  FIXTURE_OUTPUT_JSON / FIXTURE_ENV_FILE

输出:
  /tmp/uat-fixture.json            CI workflow 期望路径：含 host_id + admin_token
  .uat-fixture.env（项目根目录）   本地 source 用，导出所有 fixture env
  $GITHUB_OUTPUT（若存在）         CI 标准输出：host_id / admin_token / api_base

退出码：0=就绪 / 1=失败（已 best-effort 清理）
EOF
}

# ─── 参数解析 ───────────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --help|-h) usage; exit 0 ;;
    *) err "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

# ─── 失败兜底：trap EXIT 调 fixture-down ────────────────────────────────────
FIXTURE_READY=false
on_exit() {
  local rc=$?
  if [[ "$DRY_RUN" == "true" ]]; then
    return 0
  fi
  if [[ "$FIXTURE_READY" == "true" && "$rc" -eq 0 ]]; then
    return 0
  fi
  warn "fixture 启动失败或被中断（rc=${rc}），调用 fixture-down 清理..."
  if [[ -x "${SCRIPT_DIR}/uat-bypass-fixture-down.sh" ]]; then
    "${SCRIPT_DIR}/uat-bypass-fixture-down.sh" >/dev/null 2>&1 || true
  fi
}
trap on_exit EXIT INT TERM

# ─── 工具：run / dry-run 等价封装 ───────────────────────────────────────────
run() {
  # 真模式直接 exec；dry-run 只打印
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  ${C_YELLOW}[DRY]${C_RESET} $*"
  else
    "$@"
  fi
}

# ─── Step 1: preflight ─────────────────────────────────────────────────────
step "preflight：检查依赖工具"
NEED_TOOLS=(curl jq)
if [[ "$DRY_RUN" != "true" ]]; then
  NEED_TOOLS+=(docker go)
fi
MISSING=()
for t in "${NEED_TOOLS[@]}"; do
  if ! command -v "$t" >/dev/null 2>&1; then
    MISSING+=("$t")
  fi
done
if [[ ${#MISSING[@]} -gt 0 ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    warn "缺少工具（dry-run 放行）：${MISSING[*]}"
  else
    err "缺少必要工具：${MISSING[*]}"
    exit 1
  fi
else
  ok "依赖齐备：${NEED_TOOLS[*]}"
fi

# Linux 才需要 nft / sudo，用于真实跑 uat-bypass.sh
IS_LINUX=false
if [[ "$(uname -s)" == "Linux" ]]; then
  IS_LINUX=true
fi

# ─── Step 2: 启动 PostgreSQL 容器 ───────────────────────────────────────────
step "启动 PostgreSQL 容器（${PG_IMAGE} @${PG_PORT}）"
if [[ "$DRY_RUN" == "true" ]]; then
  run docker run -d --rm --name "$PG_CONTAINER" \
    -e POSTGRES_USER="$PG_USER" \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -e POSTGRES_DB="$PG_DB" \
    -p "${PG_PORT}:5432" \
    "$PG_IMAGE"
else
  if docker ps -a --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
    info "复用已存在的 ${PG_CONTAINER}（停止旧实例）"
    docker rm -f "$PG_CONTAINER" >/dev/null 2>&1 || true
  fi
  docker run -d --rm --name "$PG_CONTAINER" \
    -e POSTGRES_USER="$PG_USER" \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -e POSTGRES_DB="$PG_DB" \
    -p "${PG_PORT}:5432" \
    "$PG_IMAGE" >/dev/null
  ok "PostgreSQL 容器已启动：$PG_CONTAINER"
fi

# ─── Step 3: 等待 PG 就绪 ───────────────────────────────────────────────────
step "等待 PostgreSQL 就绪（最多 30s）"
if [[ "$DRY_RUN" == "true" ]]; then
  info "dry-run：跳过 pg_isready 探测"
else
  for i in $(seq 1 30); do
    if docker exec "$PG_CONTAINER" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; then
      ok "PG 就绪（${i}s）"
      break
    fi
    if [[ "$i" == "30" ]]; then
      err "PG 30s 内未就绪"
      docker logs "$PG_CONTAINER" 2>&1 | tail -20
      exit 1
    fi
    sleep 1
  done
fi

DATABASE_URL="postgres://${PG_USER}:${PG_PASSWORD}@localhost:${PG_PORT}/${PG_DB}?sslmode=disable"

# ─── Step 4: 启动 control-plane（go run，迁移自动跑）────────────────────────
step "启动 control-plane（go run ./cmd/control-plane）"
if [[ "$DRY_RUN" == "true" ]]; then
  run "DATABASE_URL=*** ADMIN_USERNAME=$ADMIN_USERNAME ADMIN_PASSWORD=*** \
      ADMIN_JWT_SECRET=*** CONTROL_PLANE_ADDR=$CONTROL_PLANE_ADDR \
      go run ./cmd/control-plane >> $CONTROL_PLANE_LOG 2>&1 &"
else
  # 用 nohup + setsid 让 control-plane 脱离当前 shell；pid 写到 file 便于 down.sh
  (
    cd "$PROJECT_ROOT"
    DATABASE_URL="$DATABASE_URL" \
    ADMIN_USERNAME="$ADMIN_USERNAME" \
    ADMIN_PASSWORD="$ADMIN_PASSWORD" \
    ADMIN_JWT_SECRET="$ADMIN_JWT_SECRET" \
    CONTROL_PLANE_ADDR="$CONTROL_PLANE_ADDR" \
    nohup go run ./cmd/control-plane > "$CONTROL_PLANE_LOG" 2>&1 &
    echo $! > "$CONTROL_PLANE_PIDFILE"
  )
  ok "control-plane PID=$(cat "$CONTROL_PLANE_PIDFILE")，日志：$CONTROL_PLANE_LOG"
fi

# ─── Step 5: 等待 control-plane HTTP 健康 ───────────────────────────────────
step "等待 control-plane HTTP 健康（最多 60s）"
if [[ "$DRY_RUN" == "true" ]]; then
  info "dry-run：跳过 health 探测"
else
  for i in $(seq 1 60); do
    if curl -fsS "${API_BASE}/healthz" >/dev/null 2>&1 \
       || curl -fsS "${API_BASE}/v1/healthz" >/dev/null 2>&1 \
       || curl -fsS "${API_BASE}/" >/dev/null 2>&1; then
      ok "control-plane 健康（${i}s）"
      break
    fi
    if [[ "$i" == "60" ]]; then
      err "control-plane 60s 内未健康，日志末尾："
      tail -50 "$CONTROL_PLANE_LOG" >&2 || true
      exit 1
    fi
    sleep 1
  done
fi

# ─── Step 6: admin 登录拿 JWT ──────────────────────────────────────────────
step "POST /v1/auth/login 取 admin JWT"
if [[ "$DRY_RUN" == "true" ]]; then
  ADMIN_TOKEN="dryrun-admin-jwt"
  info "dry-run：使用占位 admin token"
else
  LOGIN_RESP=$(curl -fsS -X POST "${API_BASE}/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${ADMIN_USERNAME}\",\"password\":\"${ADMIN_PASSWORD}\"}") || {
    err "admin 登录失败"
    exit 1
  }
  ADMIN_TOKEN=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
  if [[ -z "$ADMIN_TOKEN" ]]; then
    err "登录响应未含 token：$LOGIN_RESP"
    exit 1
  fi
  ok "admin token 获取成功（长度=${#ADMIN_TOKEN}）"
fi

# ─── 工具：admin API 调用 ──────────────────────────────────────────────────
api() {
  local method=$1 path=$2 body=${3:-}
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "[DRY] curl -X $method ${API_BASE}${path} ..." >&2
    case "$path" in
      */users)       echo '{"user":{"id":"dryrun-user-id","username":"'"${FIXTURE_USER_NAME}"'"}}' ;;
      */egress-ips)  echo '{"egress_ip":{"id":"dryrun-egress-id","ip_address":"'"${FIXTURE_EGRESS_IP}"'"}}' ;;
      */hosts)       echo '{"host":{"id":"dryrun-host-id","short_id":"'"${FIXTURE_HOST_SHORT}"'","hostname":"uat-bypass-host"}}' ;;
      *)             echo '{}' ;;
    esac
    return 0
  fi
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" "${API_BASE}${path}" \
      -H "Authorization: Bearer $ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$body"
  else
    curl -fsS -X "$method" "${API_BASE}${path}" \
      -H "Authorization: Bearer $ADMIN_TOKEN"
  fi
}

# ─── Step 7: 创建 fixture user ─────────────────────────────────────────────
step "创建 fixture user（${FIXTURE_USER_NAME}）"
USER_RESP=$(api POST /v1/admin/users "{\"username\":\"${FIXTURE_USER_NAME}\"}") || {
  err "创建 user 失败"
  exit 1
}
USER_ID=$(echo "$USER_RESP" | jq -r '.user.id // .id // empty')
if [[ -z "$USER_ID" || "$USER_ID" == "null" ]]; then
  err "user 响应缺 id：$USER_RESP"
  exit 1
fi
ok "user_id=${USER_ID}"

# ─── Step 8: 创建 fixture egress IP ────────────────────────────────────────
step "创建 fixture egress IP（${FIXTURE_EGRESS_LABEL} → ${FIXTURE_EGRESS_IP}）"
PROXY_CFG=$(cat <<EOF
{"type":"socks","server":"${FIXTURE_PROXY_SERVER}","server_port":${FIXTURE_PROXY_PORT}}
EOF
)
EGRESS_BODY=$(jq -n \
  --arg label "$FIXTURE_EGRESS_LABEL" \
  --arg ip "$FIXTURE_EGRESS_IP" \
  --argjson proxy "$PROXY_CFG" \
  '{label:$label, ip_address:$ip, provider:"manual", proxy_config:$proxy}')
EGRESS_RESP=$(api POST /v1/admin/egress-ips "$EGRESS_BODY") || {
  err "创建 egress IP 失败"
  exit 1
}
EGRESS_ID=$(echo "$EGRESS_RESP" | jq -r '.egress_ip.id // .id // empty')
if [[ -z "$EGRESS_ID" || "$EGRESS_ID" == "null" ]]; then
  err "egress 响应缺 id：$EGRESS_RESP"
  exit 1
fi
ok "egress_ip_id=${EGRESS_ID}"

# ─── Step 9: 创建 fixture host ─────────────────────────────────────────────
step "创建 fixture host（绑定 user + egress_ip）"
HOST_BODY=$(jq -n \
  --arg uid "$USER_ID" \
  --arg eid "$EGRESS_ID" \
  '{user_id:$uid, egress_ip_id:$eid, timezone:"UTC", memory_limit_mb:512, cpu_limit:1, disk_limit_gb:5}')
HOST_RESP=$(api POST /v1/admin/hosts "$HOST_BODY") || {
  err "创建 host 失败"
  exit 1
}
HOST_ID=$(echo "$HOST_RESP" | jq -r '.host.id // .id // empty')
HOST_SHORT=$(echo "$HOST_RESP" | jq -r '.host.short_id // .short_id // empty')
if [[ -z "$HOST_ID" || "$HOST_ID" == "null" ]]; then
  err "host 响应缺 id：$HOST_RESP"
  exit 1
fi
ok "host_id=${HOST_ID} short_id=${HOST_SHORT}"

# ─── Step 10: Linux 真实模式：拉起 mock worker + gateway 容器 ──────────────
step "拉起 mock worker / gateway 容器（仅 Linux 真实模式）"
WORKER_CONTAINER="cloudproxy-${HOST_ID}"
GATEWAY_CONTAINER="cloudproxy-gw-${HOST_ID}"
WORKER_IMAGE="${WORKER_IMAGE:-alpine:3.20}"
GATEWAY_IMAGE="${GATEWAY_IMAGE:-cloud-cli-proxy-sing-gateway:local}"

if [[ "$DRY_RUN" == "true" ]]; then
  info "dry-run：跳过容器启动（计划名：${WORKER_CONTAINER} / ${GATEWAY_CONTAINER}）"
elif [[ "$IS_LINUX" != "true" ]]; then
  warn "非 Linux 环境（$(uname -s)），跳过 worker/gateway 容器（uat-bypass.sh 真实模式仅 Linux 可跑）"
else
  # mock worker：sleep 容器，足够 nsenter -t <pid> 进 netns；
  # 真实 uat-bypass.sh 通过 docker inspect 拿 PID，所以名字与 PID 都需可解析
  if docker ps -a --format '{{.Names}}' | grep -qx "$WORKER_CONTAINER"; then
    docker rm -f "$WORKER_CONTAINER" >/dev/null 2>&1 || true
  fi
  docker run -d --rm --name "$WORKER_CONTAINER" \
    --label "com.cloud-cli-proxy.managed=true" \
    --label "com.cloud-cli-proxy.host-id=${HOST_ID}" \
    "$WORKER_IMAGE" sleep 86400 >/dev/null
  ok "worker 容器已启动：$WORKER_CONTAINER"

  # gateway：优先用项目自带 sing-box 镜像；缺失则回退 alpine 占位（uat-bypass.sh
  # 的 I5/I8 会用 docker exec/sh，所以容器内得有 sh + 能跑 jq/pkill 才算严格）
  if docker image inspect "$GATEWAY_IMAGE" >/dev/null 2>&1; then
    info "复用 gateway 镜像：$GATEWAY_IMAGE"
  else
    warn "gateway 镜像 $GATEWAY_IMAGE 不存在，退化到 alpine 占位（I5/I8 走 lenient）"
    GATEWAY_IMAGE="alpine:3.20"
  fi
  if docker ps -a --format '{{.Names}}' | grep -qx "$GATEWAY_CONTAINER"; then
    docker rm -f "$GATEWAY_CONTAINER" >/dev/null 2>&1 || true
  fi
  docker run -d --rm --name "$GATEWAY_CONTAINER" \
    --label "com.cloud-cli-proxy.managed=true" \
    --label "com.cloud-cli-proxy.host-id=${HOST_ID}" \
    --label "com.cloud-cli-proxy.role=gateway" \
    "$GATEWAY_IMAGE" sh -c "apk add --no-cache jq >/dev/null 2>&1 || true; sleep 86400" >/dev/null
  ok "gateway 容器已启动：$GATEWAY_CONTAINER"
fi

# ─── Step 11: 输出 fixture 契约 ────────────────────────────────────────────
step "写出 fixture 契约文件"
# 1) /tmp/uat-fixture.json（CI workflow 期望路径）
FIXTURE_JSON=$(jq -n \
  --arg host_id "$HOST_ID" \
  --arg short_id "$HOST_SHORT" \
  --arg admin_token "$ADMIN_TOKEN" \
  --arg api_base "$API_BASE" \
  --arg user_id "$USER_ID" \
  --arg egress_id "$EGRESS_ID" \
  --arg worker "$WORKER_CONTAINER" \
  --arg gateway "$GATEWAY_CONTAINER" \
  --arg database_url "$DATABASE_URL" \
  '{
    host_id:$host_id,
    short_id:$short_id,
    admin_token:$admin_token,
    api_base:$api_base,
    user_id:$user_id,
    egress_ip_id:$egress_id,
    worker_container:$worker,
    gateway_container:$gateway,
    database_url:$database_url
  }')
echo "$FIXTURE_JSON" > "$FIXTURE_OUTPUT_JSON"
ok "写出 $FIXTURE_OUTPUT_JSON"

# 2) .uat-fixture.env（本地 source 用；不写敏感字段进 git，文件在 .gitignore 之外
#    时由 fixture-down.sh 清理）
cat > "$FIXTURE_ENV_FILE" <<EOF
# Auto-generated by scripts/uat-bypass-fixture-up.sh — do not commit.
# Source 这个文件即可让本地 uat-bypass.sh 拿到所有 env vars。
export CLOUD_CLI_PROXY_API_BASE="${API_BASE}"
export CLOUD_CLI_PROXY_ADMIN_TOKEN="${ADMIN_TOKEN}"
export UAT_BYPASS_HOST_ID="${HOST_ID}"
export UAT_BYPASS_HOST_SHORT_ID="${HOST_SHORT}"
export UAT_BYPASS_USER_ID="${USER_ID}"
export UAT_BYPASS_EGRESS_IP_ID="${EGRESS_ID}"
export UAT_BYPASS_WORKER_CONTAINER="${WORKER_CONTAINER}"
export UAT_BYPASS_GATEWAY_CONTAINER="${GATEWAY_CONTAINER}"
export DATABASE_URL="${DATABASE_URL}"
EOF
ok "写出 $FIXTURE_ENV_FILE"

# 3) $GITHUB_OUTPUT（CI 标准输出）
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "host_id=${HOST_ID}"
    echo "admin_token=${ADMIN_TOKEN}"
    echo "api_base=${API_BASE}"
    echo "worker_container=${WORKER_CONTAINER}"
    echo "gateway_container=${GATEWAY_CONTAINER}"
  } >> "$GITHUB_OUTPUT"
  ok "已追加 host_id / admin_token / api_base 到 \$GITHUB_OUTPUT"
fi

FIXTURE_READY=true
echo ""
echo "── fixture 已就绪 ──"
echo "  host_id          = $HOST_ID"
echo "  short_id         = $HOST_SHORT"
echo "  api_base         = $API_BASE"
echo "  admin_token      = ${ADMIN_TOKEN:0:8}***（已脱敏）"
echo "  worker_container = $WORKER_CONTAINER"
echo "  gateway_container= $GATEWAY_CONTAINER"
echo ""
echo "下一步："
echo "  source ${FIXTURE_ENV_FILE}"
echo "  bash scripts/uat-bypass.sh --scenario=loopback-only --target-host-id=\"\$UAT_BYPASS_HOST_ID\""
echo ""
echo "清理："
echo "  bash scripts/uat-bypass-fixture-down.sh"

exit 0
