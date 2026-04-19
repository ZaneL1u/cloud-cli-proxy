#!/usr/bin/env bash
# Phase 31 Plan 03 集成测试 fixture：
# 起 Phase 29 镜像容器作为 cloud-claude integration test fixture。
#
# 用法：scripts/test-fixture-up.sh
#
# 依赖：
#   - docker
#   - docker compose plugin (>= v2)
#   - netcat (nc)；缺失时用 /dev/tcp fallback
#
# 退出码：
#   0  → fixture 已起且 sshd 就绪
#   1  → 缺少依赖、镜像未构建、或 30s 内 sshd 未就绪
#
# 幂等：重复执行会先 docker compose up -d；docker compose 自身保证 idempotent。
set -euo pipefail

FIXTURE_DIR="/tmp/cloud-claude-fixture"
IMAGE="local/managed-user:v3.0.0"

command -v docker >/dev/null || { echo "需要 docker"; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "需要 docker compose plugin"; exit 1; }

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "镜像 $IMAGE 不存在。请先构建："
  echo "  docker build -f deploy/docker/managed-user/Dockerfile -t $IMAGE ."
  exit 1
fi

mkdir -p "$FIXTURE_DIR"
cat > "$FIXTURE_DIR/docker-compose.yml" <<'YAML'
services:
  cc-fixture:
    image: local/managed-user:v3.0.0
    container_name: cc-fixture
    cap_add: [SYS_ADMIN]
    devices: ["/dev/fuse:/dev/fuse"]
    security_opt: ["apparmor=unconfined"]
    ports: ["12222:2222"]
    environment:
      - CLOUD_CLAUDE_TEST_FIXTURE=1
YAML

echo "=== 启动 fixture 容器"
(cd "$FIXTURE_DIR" && docker compose up -d)

echo "=== 等待 sshd ready (port 12222)"
for i in $(seq 1 30); do
  if command -v nc >/dev/null 2>&1; then
    if nc -z 127.0.0.1 12222 2>/dev/null; then
      echo "=== sshd ready"
      exit 0
    fi
  else
    if (echo > /dev/tcp/127.0.0.1/12222) 2>/dev/null; then
      echo "=== sshd ready"
      exit 0
    fi
  fi
  sleep 1
done
echo "=== sshd 启动超时（30s）"
(cd "$FIXTURE_DIR" && docker compose logs)
exit 1
