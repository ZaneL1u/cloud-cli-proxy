#!/usr/bin/env bash
set -euo pipefail

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0
CONTAINER_NAME="fuse-verify-$$"
IMAGE_NAME=""
SSHFS_MOUNT_OK=false

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }

cleanup() {
  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT

IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' deploy/docker/managed-user/image.lock)"
if [[ -z "${IMAGE_NAME}" ]]; then
  fail "镜像名读取失败: deploy/docker/managed-user/image.lock 中未找到 local_dev_image_name"
  echo "========================================"
  echo "验证结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN"
  echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
  exit 1
fi
info "使用镜像: ${IMAGE_NAME}"

# ── 阶段 1：宿主机安全模块检测 ──

echo ""
echo "=== 阶段 1: 宿主机安全模块检测 ==="

if modprobe fuse 2>/dev/null && test -c /dev/fuse; then
  pass "FUSE 内核模块: 已加载"
else
  fail "FUSE 内核模块: 未加载，请运行 modprobe fuse"
fi

if command -v aa-enabled >/dev/null 2>&1; then
  if aa-enabled --quiet 2>/dev/null; then
    info "AppArmor 状态: 已启用 (enforcing)"
  else
    info "AppArmor 状态: 未启用"
  fi
else
  info "AppArmor 状态: 未安装"
fi

if command -v aa-status >/dev/null 2>&1; then
  if aa-status 2>/dev/null | grep -q "fusermount3"; then
    warn "fusermount3 AppArmor profile 已加载 (Ubuntu 25.04+ 可能影响容器内 FUSE 操作)"
  else
    info "fusermount3 AppArmor profile: 未加载"
  fi
fi

DOCKER_SECURITY="$(docker info --format '{{.SecurityOptions}}' 2>/dev/null || echo 'N/A')"
info "Docker 安全模块: ${DOCKER_SECURITY}"

if [[ -f /etc/os-release ]]; then
  PRETTY_NAME="$(grep '^PRETTY_NAME=' /etc/os-release | cut -d'"' -f2)"
  info "宿主机 OS: ${PRETTY_NAME}"
fi

# ── 阶段 2：容器内真实 sshfs FUSE 挂载测试 ──

echo ""
echo "=== 阶段 2: 容器内真实 sshfs FUSE 挂载测试 ==="

info "启动测试容器: ${CONTAINER_NAME}"
if ! docker run -d \
  --name "$CONTAINER_NAME" \
  --cap-add SYS_ADMIN \
  --device /dev/fuse \
  --security-opt apparmor=unconfined \
  "$IMAGE_NAME" sleep 600 >/dev/null 2>&1; then
  fail "测试容器启动失败"
  echo "========================================"
  echo "验证结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN"
  echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
  exit 1
fi

if docker exec "$CONTAINER_NAME" test -c /dev/fuse; then
  pass "/dev/fuse 设备: 可用"
else
  fail "/dev/fuse 设备: 不可用"
fi

if docker exec "$CONTAINER_NAME" grep -q "^user_allow_other" /etc/fuse.conf 2>/dev/null; then
  pass "user_allow_other 配置: 已启用"
else
  fail "user_allow_other 配置: 未启用 (/etc/fuse.conf 缺少 user_allow_other)"
fi

info "执行真实 sshfs FUSE 挂载测试..."
FUSE_OUTPUT=$(docker exec "$CONTAINER_NAME" bash -c '
  mkdir -p /tmp/fuse-src /tmp/fuse-mount
  echo "fuse-test-content" > /tmp/fuse-src/test.txt

  ssh-keygen -A 2>/dev/null || true
  ssh-keygen -t ed25519 -f /tmp/fuse-testkey -N "" -q
  mkdir -p /root/.ssh
  cat /tmp/fuse-testkey.pub >> /root/.ssh/authorized_keys
  chmod 600 /root/.ssh/authorized_keys

  /usr/sbin/sshd -p 2299 -o ListenAddress=127.0.0.1

  sshfs root@127.0.0.1:/tmp/fuse-src /tmp/fuse-mount \
    -p 2299 \
    -o StrictHostKeyChecking=no \
    -o IdentityFile=/tmp/fuse-testkey \
    -o allow_other 2>/dev/null

  if mountpoint -q /tmp/fuse-mount; then
    echo "SSHFS_MOUNT_OK"
  else
    echo "SSHFS_MOUNT_FAIL"
  fi

  if [ -f /tmp/fuse-mount/test.txt ] && [ "$(cat /tmp/fuse-mount/test.txt)" = "fuse-test-content" ]; then
    echo "READ_OK"
  else
    echo "READ_FAIL"
  fi

  echo "write-verify" > /tmp/fuse-mount/write-test.txt
  if [ -f /tmp/fuse-src/write-test.txt ] && [ "$(cat /tmp/fuse-src/write-test.txt)" = "write-verify" ]; then
    echo "WRITE_OK"
  else
    echo "WRITE_FAIL"
  fi

  fusermount -u /tmp/fuse-mount 2>/dev/null || umount /tmp/fuse-mount 2>/dev/null || true
' 2>&1)

if echo "$FUSE_OUTPUT" | grep -q "SSHFS_MOUNT_OK"; then
  pass "sshfs FUSE 挂载: 成功 (mountpoint -q 确认)"
  SSHFS_MOUNT_OK=true
else
  fail "sshfs FUSE 挂载: 失败，请检查 AppArmor 配置和 FUSE 设备权限"
fi

if echo "$FUSE_OUTPUT" | grep -q "READ_OK"; then
  pass "FUSE 挂载读取: 成功"
else
  fail "FUSE 挂载读取: 失败"
fi

if echo "$FUSE_OUTPUT" | grep -q "WRITE_OK"; then
  pass "FUSE 挂载写入: 成功"
else
  fail "FUSE 挂载写入: 失败"
fi

# ── 阶段 3：网络策略共存验证 ──

echo ""
echo "=== 阶段 3: 网络策略共存验证 ==="

info "sshfs slave 模式的 SFTP 数据走 SSH session channel (进程内 pipe)，不经过容器网络栈"

TUNNEL_ACTIVE=false
if command -v nft >/dev/null 2>&1; then
  if nft list chain inet cloud_cli_proxy forward 2>/dev/null | grep -q "drop"; then
    info "全隧道出网: nftables 默认拒绝规则已启用"
    TUNNEL_ACTIVE=true
  else
    warn "全隧道出网: nftables 默认拒绝规则未检测到 (D-07 要求在全隧道状态下验证)"
  fi
else
  warn "nft 命令不可用，无法检测 nftables 规则状态"
fi

if docker exec "$CONTAINER_NAME" command -v sshfs >/dev/null 2>&1; then
  pass "容器内 sshfs 命令: 可用"
else
  fail "容器内 sshfs 命令: 不可用"
fi

if command -v nft >/dev/null 2>&1; then
  NFT_SUMMARY="$(nft list ruleset 2>/dev/null | head -5)"
  info "nftables 规则摘要: ${NFT_SUMMARY:-'(空)'}"
fi

if [[ "$SSHFS_MOUNT_OK" == "true" ]]; then
  if [[ "$TUNNEL_ACTIVE" == "true" ]]; then
    pass "FUSE 与网络策略: 全隧道状态下共存正常 (per D-07)"
  else
    warn "FUSE 挂载成功但全隧道出网未启用，建议在生产环境重新验证 (per D-07)"
  fi
else
  fail "FUSE 与网络策略: 挂载失败，无法验证共存性"
fi

# ── 阶段 4：端到端流程验证 ──

echo ""
echo "=== 阶段 4: 端到端流程验证 ==="

info "SC-3 要求完整流程: cloud-claude → SSH Proxy → 目录映射 → Claude Code 运行"

CONTROL_PLANE_UP=false
if systemctl is-active cloud-cli-proxy >/dev/null 2>&1; then
  CONTROL_PLANE_UP=true
elif curl -sf http://localhost:8080/health >/dev/null 2>&1; then
  CONTROL_PLANE_UP=true
fi

if [[ "$CONTROL_PLANE_UP" == "true" ]]; then
  info "控制面已运行，可进行端到端验证"
  info "手工 E2E: cloud-claude connect <test-host> 并确认 /workspace 内文件可读写"
  pass "端到端前置条件: 控制面就绪"
else
  warn "控制面未运行，跳过端到端验证 (SC-3 需在完整部署后手工执行)"
  info "手工 E2E 验证步骤:"
  info "  1) 启动控制面: systemctl start cloud-cli-proxy"
  info "  2) cloud-claude connect <host>"
  info "  3) 容器内执行: mountpoint -q /workspace"
  info "  4) 读写文件确认映射正常"
  info "  5) 确认 Claude Code 可正常启动: claude --version"
fi

# ── 阶段 5：汇总输出 ──

echo ""
echo "========================================"
echo "验证结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN"
if [ "$FAIL_COUNT" -eq 0 ]; then
  echo "状态: 全部通过"
  exit 0
else
  echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
  exit 1
fi
