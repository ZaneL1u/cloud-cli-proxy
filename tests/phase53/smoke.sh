#!/usr/bin/env bash
# Phase 53 烟测：本地 docker run managed-user:v4-dev 跑通 T-53-1..6。
# 仅用于开发期手测，CI 走 Phase 55 e2e（v3.6 harness 接入）。
#
# Usage:
#   bash tests/phase53/smoke.sh                       # 默认 image tag = managed-user:v4-dev
#   IMAGE=managed-user:v4-rc1 bash tests/phase53/smoke.sh
#
# Exit: 0 全绿 / 非 0 某条失败（打印失败步骤名）

set -euo pipefail

IMAGE="${IMAGE:-managed-user:v4-dev}"
CONTAINER_NAME="phase53-smoke-$$"
FIXTURE_DIR="$(cd "$(dirname "$0")" && pwd)/fixtures"
TMP_CONFIG="$(mktemp -d)/config.json"
trap 'cleanup' EXIT

# shellcheck disable=SC2329  # invoked via trap above
cleanup() {
  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
  rm -rf "$(dirname "$TMP_CONFIG")"
}

log() { printf "\033[1;36m[smoke]\033[0m %s\n" "$*"; }
fail() { printf "\033[1;31m[FAIL]\033[0m %s\n" "$*" >&2; exit 1; }

# ===== 准备 =====
log "preparing fixture (writable copy, root:0600)"
cp "$FIXTURE_DIR/test-singbox-config.json" "$TMP_CONFIG"
chmod 600 "$TMP_CONFIG"
# config 必须由 root 拥有以模拟生产 host-agent 注入语义
sudo chown root:root "$TMP_CONFIG" 2>/dev/null || true

# ===== 启动容器 =====
log "starting container $CONTAINER_NAME from $IMAGE"
docker run -d --name "$CONTAINER_NAME" \
    --device /dev/net/tun \
    --cap-drop ALL --cap-add NET_ADMIN \
    -v "$TMP_CONFIG:/etc/sing-box/config.json" \
    -e CONTAINER_USER=workspace \
    -e CONTAINER_SSH_PASSWORD=phase53-smoke-pw \
    -e MODE=local \
    --restart=on-failure:3 \
    "$IMAGE" >/dev/null

# 等容器启动序列跑完（tun0 + nft + config rm）
sleep 6

# ===== T-53-1: tun0 ready + sing-box uid=9000 =====
log "[T-53-1] tun0 ready + sing-box uid=9000"
docker exec "$CONTAINER_NAME" ip link show tun0 >/dev/null \
    || fail "T-53-1 tun0 not ready"
uid=$(docker exec "$CONTAINER_NAME" sh -c 'ps -o uid= -p $(pidof sing-box) | tr -d " "')
[ "$uid" = "9000" ] || fail "T-53-1 sing-box uid=$uid, want 9000"

# ===== T-53-2: config 已 rm =====
log "[T-53-2] config 文件已删除"
if docker exec "$CONTAINER_NAME" test -f /etc/sing-box/config.json; then
  fail "T-53-2 config 仍存在"
fi

# ===== T-53-3: curl ip.me 走 tun =====
log "[T-53-3] tun 接管默认路由"
default_dev=$(docker exec "$CONTAINER_NAME" sh -c 'ip route show table all | grep "^default" | head -1 | awk "{print \$5}"')
case "$default_dev" in
  tun0) ;;
  *) fail "T-53-3 默认出口 dev=$default_dev, want tun0" ;;
esac

# curl 能 reach 外网（证明 tun 转发链路 OK；fixture direct outbound 直出）
if ! docker exec "$CONTAINER_NAME" curl -fsS --max-time 10 https://api.ipify.org >/dev/null 2>&1; then
  fail "T-53-3 容器内 curl ipify.org 失败（tun 链路异常）"
fi

# ===== T-53-4: workspace 用户空 cap + 不能 sudo =====
log "[T-53-4] workspace cap 集合空 + 不能 sudo"
caps=$(docker exec -u workspace "$CONTAINER_NAME" sh -c 'getpcaps $$ 2>&1' | head -1)
# expected: 形如 "Capabilities for `<pid>`: =" (空集)
if echo "$caps" | grep -qE 'cap_net_admin|cap_sys_admin|cap_net_raw'; then
  fail "T-53-4 workspace 持有特权 cap: $caps"
fi

if docker exec -u workspace "$CONTAINER_NAME" sudo -n true 2>/dev/null; then
  fail "T-53-4 workspace 居然能 sudo"
fi

# ===== T-53-5: kill sing-box → 容器 ≤3s 死 =====
log "[T-53-5] sing-box 死 → 容器死 fail-closed"
start_ts=$(date +%s)
# 必须用 root 杀（sing-box 是 uid=9000，workspace=1000 杀不了）
docker exec --user 0 "$CONTAINER_NAME" sh -c 'kill -9 $(pidof sing-box)' 2>/dev/null || true

# 等容器退出
deadline=$((start_ts + 5))
while [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null)" = "true" ]; do
  if [ "$(date +%s)" -ge "$deadline" ]; then
    fail "T-53-5 sing-box 死 5s 后容器仍在跑（fail-closed 未生效）"
  fi
  sleep 0.2
done

end_ts=$(date +%s)
elapsed=$((end_ts - start_ts))
log "  container died in ${elapsed}s"
[ "$elapsed" -le 3 ] || fail "T-53-5 容器死亡耗时 ${elapsed}s > 3s"

# ===== T-53-6: docker restart on-failure 触发 =====
# 注：Phase 53 阶段控制面尚未实现 --restart=on-failure 配置（Phase 54 落地），
# 此处用 docker run 时手工设的 --restart=on-failure:3 验证 docker daemon 重启行为
log "[T-53-6] docker restart on-failure 触发"
sleep 3  # 给 docker daemon 时间触发 restart

restart_count=$(docker inspect -f '{{.RestartCount}}' "$CONTAINER_NAME" 2>/dev/null || echo 0)
[ "$restart_count" -ge 1 ] || fail "T-53-6 RestartCount=$restart_count, want >=1"

is_running=$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null)
[ "$is_running" = "true" ] || fail "T-53-6 容器未重新拉起 (Running=$is_running)"

# 重启后 sing-box 应该再次 uid=9000 跑起来
sleep 3
new_uid=$(docker exec "$CONTAINER_NAME" sh -c 'ps -o uid= -p $(pidof sing-box) | tr -d " "' 2>/dev/null || echo "missing")
[ "$new_uid" = "9000" ] || fail "T-53-6 restart 后 sing-box uid=$new_uid"

# ===== 全绿 =====
printf "\033[1;32m[OK]\033[0m Phase 53 smoke 全部通过 (T-53-1..6)\n"
exit 0
