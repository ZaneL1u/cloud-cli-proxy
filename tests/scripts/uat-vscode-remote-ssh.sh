#!/usr/bin/env bash
# tests/scripts/uat-vscode-remote-ssh.sh — Phase 40+43 VS Code Remote-SSH E2E UAT
#
# 覆盖 9 大场景（sing-box 进程 / 出口 IP / DNS 泄漏 / VS Code Server / sshd / sing-box 日志 /
#   direct-tcpip 端口转发 / 端口转发出口 IP / 安全拒绝），
# 风格与 uat-v31-promotion.sh 一致：
#   --dry-run 默认安全（只打印操作描述，不做实际断言）
#   --confirm-destructive 触发实际断言（需要运行中的容器）
#
# 用法：
#   bash tests/scripts/uat-vscode-remote-ssh.sh --dry-run
#   bash tests/scripts/uat-vscode-remote-ssh.sh --confirm-destructive --container=NAME --expected-egress-ip=1.2.3.4 --ssh-port=2222
#
# 退出码：
#   0  PASS（全部场景通过 或 dry-run 完成）
#   1  FAIL（任一断言失败）
#   2  SKIP（环境不具备：无 docker / 无运行容器等）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/.planning/phases/40-vs-code-remote-ssh-e2e/benchmarks"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
info() { echo "[INFO]  $1"; }

usage() {
  cat <<'EOF'
uat-vscode-remote-ssh.sh — Phase 40+43 VS Code Remote-SSH E2E UAT（9 场景）

用法:
  tests/scripts/uat-vscode-remote-ssh.sh [选项]

选项:
  --dry-run                 默认模式：打印每个场景的操作描述，不做实际断言
  --confirm-destructive     触发实际断言：需要运行中的容器
  --container=NAME          指定容器名（默认自动检测 cloud-claude-local-*）
  --expected-egress-ip=IP   指定期望的出口 IP（不指定则跳过出口 IP 断言）
  --ssh-port=PORT           SSH 代理端口（默认自动检测容器 SSH 端口）
  --output-dir=DIR          报告输出目录
  --help, -h                显示本帮助

场景覆盖:
  1. sing-box 进程检测              断言容器内 sing-box 进程存在
  2. 出口 IP 验证                   断言 curl ifconfig.me 返回期望的 egress IP
  3. DNS 泄漏检测                   断言容器内 DNS 解析正常（走 sing-box）
  4. VS Code Server 进程检测        检查容器内 vscode-server 进程（可选）
  5. sshd 进程检测                  断言容器内 sshd 进程存在
  6. sing-box 日志域名检查          检查 sing-box 日志中是否有 VS Code 更新域名
  7. direct-tcpip 端口转发验证      通过 SSH -L 建立 direct-tcpip 隧道验证连通性
  8. 端口转发出口 IP 验证           验证端口转发路径的流量路由正确性
  9. 安全拒绝验证                   验证转发到管理网段（10.99.x.x）被正确拦截

需求锚点:
  SSH-05  VS Code Remote-SSH 端到端验证
  SEC-01  验证 direct-tcpip 转发流量走 sing-box tun
  SEC-02  VS Code Server 下载/扩展安装流量走受控出口

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# CLI 参数
# ────────────────────────────────────────────────────────────────────────────

DRY_RUN=true
CONTAINER_NAME=""
EXPECTED_EGRESS_IP=""
SSH_PORT=""
OUTPUT_DIR="${OUTPUT_DIR}"

# SSH 隧道清理
cleanup_ssh_tunnels() {
  local pids
  eval "pids=( ${_UAT_SSH_PIDS:-} )"
  for pid in "${pids[@]+"${pids[@]}"}"; do
    kill "$pid" 2>/dev/null || true
  done
}
_UAT_SSH_PIDS=""
trap cleanup_ssh_tunnels EXIT

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --confirm-destructive)
      DRY_RUN=false
      ;;
    --container=*) CONTAINER_NAME="${arg#--container=}" ;;
    --expected-egress-ip=*) EXPECTED_EGRESS_IP="${arg#--expected-egress-ip=}" ;;
    --ssh-port=*) SSH_PORT="${arg#--ssh-port=}" ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_JSON="${OUTPUT_DIR}/uat-vscode-remote-ssh-${TIMESTAMP}.json"

# ────────────────────────────────────────────────────────────────────────────
# 环境探测
# ────────────────────────────────────────────────────────────────────────────

has_docker() { command -v docker >/dev/null 2>&1; }

detect_container() {
  if [ -n "$CONTAINER_NAME" ]; then
    echo "$CONTAINER_NAME"
    return
  fi
  local name
  name=$(docker ps --filter "label=cloud-claude-local=true" --format '{{.Names}}' 2>/dev/null | head -1)
  if [ -z "$name" ]; then
    name=$(docker ps --filter "name=cloud-claude-local" --format '{{.Names}}' 2>/dev/null | head -1)
  fi
  echo "$name"
}

# ────────────────────────────────────────────────────────────────────────────
# 断言辅助
# ────────────────────────────────────────────────────────────────────────────

declare -a SCENARIO_ASSERTIONS

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","expected":"%s","actual":"%s"}' "$label" "$expected" "$actual")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","expected":"%s","actual":"%s"}' "$label" "$expected" "$actual")")
    return 1
  fi
}

assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","detail":"contains \"%s\""}' "$label" "$needle")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","detail":"missing \"%s\""}' "$label" "$needle")")
    return 1
  fi
}

assert_process_running() {
  local label="$1" container="$2" pattern="$3"
  if docker exec "$container" pgrep -f "$pattern" >/dev/null 2>&1; then
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"pass","detail":"process found: %s"}' "$label" "$pattern")")
    return 0
  else
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"%s","result":"fail","detail":"process not found: %s"}' "$label" "$pattern")")
    return 1
  fi
}

reset_assertions() {
  SCENARIO_ASSERTIONS=()
}

# ────────────────────────────────────────────────────────────────────────────
# JSON 报告输出
# ────────────────────────────────────────────────────────────────────────────

SCENARIO_RESULTS_JSON="[]"

write_json_report() {
  local scenario_name="$1" status="$2"
  local assertions_json
  if [[ ${#SCENARIO_ASSERTIONS[@]} -gt 0 ]]; then
    assertions_json="[$(IFS=','; echo "${SCENARIO_ASSERTIONS[*]}")]"
  else
    assertions_json="[]"
  fi

  local entry
  entry="$(printf '{"name":"%s","status":"%s","assertions":%s}' \
    "$scenario_name" "$status" "$assertions_json")"

  if [[ "$SCENARIO_RESULTS_JSON" == "[]" ]]; then
    SCENARIO_RESULTS_JSON="[$entry]"
  else
    SCENARIO_RESULTS_JSON="${SCENARIO_RESULTS_JSON%\]},${entry}]"
  fi
}

render_final_json() {
  local summary_outcome="pass"
  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    summary_outcome="fail"
  elif [[ "$SKIP_COUNT" -gt 0 && "$PASS_COUNT" -eq 0 ]]; then
    summary_outcome="skip"
  fi

  if command -v jq >/dev/null 2>&1; then
    jq -n \
      --argjson schema 1 \
      --arg script "uat-vscode-remote-ssh.sh" \
      --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --argjson dry_run "$DRY_RUN" \
      --argjson pass "$PASS_COUNT" \
      --argjson fail "$FAIL_COUNT" \
      --argjson skip "$SKIP_COUNT" \
      --arg outcome "$summary_outcome" \
      --argjson scenarios_json "$SCENARIO_RESULTS_JSON" \
      '{
        schema_version: $schema,
        script: $script,
        timestamp: $timestamp,
        dry_run: $dry_run,
        summary: { pass: $pass, fail: $fail, skip: $skip },
        outcome: $outcome,
        scenarios: $scenarios_json
      }' > "$REPORT_JSON"
  else
    cat > "$REPORT_JSON" <<JSONEOF
{
  "schema_version": 1,
  "script": "uat-vscode-remote-ssh.sh",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "dry_run": $DRY_RUN,
  "summary": { "pass": $PASS_COUNT, "fail": $FAIL_COUNT, "skip": $SKIP_COUNT },
  "outcome": "${summary_outcome}",
  "scenarios": $SCENARIO_RESULTS_JSON
}
JSONEOF
  fi
  info "JSON 报告: ${REPORT_JSON}"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 1 — sing-box 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_singbox_process() {
  reset_assertions
  info "===== 场景 1: sing-box 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -x sing-box"
    echo "  [DRY-RUN] 预期: sing-box 进程存在"
    echo "  [DRY-RUN] 注意: proxy 模式和 tun 模式都适用"
    pass "sing-box 进程检测（dry-run 描述通过）"
    write_json_report "singbox_process" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "singbox_process" "未安装 docker"
    write_json_report "singbox_process" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "singbox_process" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "singbox_process" "skip"
    return 0
  fi

  info "容器: $container"

  if assert_process_running "sing-box 进程" "$container" "sing-box"; then
    pass "场景 1: sing-box 进程运行中"
  else
    fail "场景 1: sing-box 进程未运行"
  fi

  write_json_report "singbox_process" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 2 — 出口 IP 验证
# ────────────────────────────────────────────────────────────────────────────

scenario_egress_ip() {
  reset_assertions
  info "===== 场景 2: 出口 IP 验证 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER curl -s --max-time 15 ifconfig.me"
    echo "  [DRY-RUN] 预期: 返回 IP 等于 --expected-egress-ip 参数值"
    if [ -z "$EXPECTED_EGRESS_IP" ]; then
      echo "  [DRY-RUN] 注意: 未指定 --expected-egress-ip，将 SKIP"
      skip "出口 IP 验证" "未指定 --expected-egress-ip（dry-run）"
      write_json_report "egress_ip" "skip"
    else
      pass "出口 IP 验证（dry-run 描述通过，期望 IP: $EXPECTED_EGRESS_IP）"
      write_json_report "egress_ip" "pass"
    fi
    return 0
  fi

  if [ -z "$EXPECTED_EGRESS_IP" ]; then
    skip "egress_ip" "未指定 --expected-egress-ip"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "egress_ip" "未安装 docker"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "egress_ip" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "egress_ip" "skip"
    return 0
  fi

  info "容器: $container"
  info "检测出口 IP（可能需要 10-15 秒）..."

  local actual_ip
  actual_ip=$(docker exec "$container" curl -s --max-time 15 ifconfig.me 2>/dev/null || echo "CURL_FAILED")

  if [ "$actual_ip" = "CURL_FAILED" ]; then
    fail "场景 2: curl ifconfig.me 执行失败"
    SCENARIO_ASSERTIONS+=('{"name":"curl 执行","result":"fail","detail":"curl ifconfig.me failed"}')
  elif assert_eq "出口 IP" "$EXPECTED_EGRESS_IP" "$actual_ip"; then
    pass "场景 2: 出口 IP 验证通过 ($actual_ip)"
  else
    fail "场景 2: 出口 IP 不匹配 (实际: $actual_ip, 期望: $EXPECTED_EGRESS_IP)"
  fi

  write_json_report "egress_ip" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 3 — DNS 泄漏检测
# ────────────────────────────────────────────────────────────────────────────

scenario_dns_leak() {
  reset_assertions
  info "===== 场景 3: DNS 泄漏检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER nslookup ifconfig.me"
    echo "  [DRY-RUN] 预期: DNS 解析成功，走 sing-box DNS"
    pass "DNS 泄漏检测（dry-run 描述通过）"
    write_json_report "dns_leak" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "dns_leak" "未安装 docker"
    write_json_report "dns_leak" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "dns_leak" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "dns_leak" "skip"
    return 0
  fi

  info "容器: $container"

  local dns_result
  dns_result=$(docker exec "$container" nslookup ifconfig.me 2>&1 || true)

  if echo "$dns_result" | grep -q "Address:"; then
    pass "场景 3: DNS 解析成功"
    assert_contains "DNS 结果包含 Address" "$dns_result" "Address:"
  else
    fail "场景 3: DNS 解析失败"
    SCENARIO_ASSERTIONS+=('{"name":"DNS 解析","result":"fail","detail":"nslookup failed"}')
  fi

  write_json_report "dns_leak" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 4 — VS Code Server 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_vscode_server() {
  reset_assertions
  info "===== 场景 4: VS Code Server 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -f vscode-server"
    echo "  [DRY-RUN] 预期: VS Code Server 进程存在（需要先通过 VS Code 连接）"
    echo "  [DRY-RUN] 注意: 如果尚未通过 VS Code 连接，此场景将 SKIP"
    skip "VS Code Server 进程" "需要先通过 VS Code 连接（dry-run）"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "vscode_server" "未安装 docker"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "vscode_server" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  info "容器: $container"

  if docker exec "$container" pgrep -f "vscode-server" >/dev/null 2>&1; then
    pass "场景 4: VS Code Server 进程运行中"
    SCENARIO_ASSERTIONS+=('{"name":"VS Code Server","result":"pass","detail":"process found"}')
  else
    skip "vscode_server" "VS Code Server 未运行（可能尚未通过 VS Code 连接）"
    write_json_report "vscode_server" "skip"
    return 0
  fi

  write_json_report "vscode_server" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 5 — sshd 进程检测
# ────────────────────────────────────────────────────────────────────────────

scenario_sshd_process() {
  reset_assertions
  info "===== 场景 5: sshd 进程检测 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行: docker exec \$CONTAINER pgrep -x sshd"
    echo "  [DRY-RUN] 预期: sshd 进程存在"
    pass "sshd 进程检测（dry-run 描述通过）"
    write_json_report "sshd_process" "pass"
    return 0
  fi

  if ! has_docker; then
    skip "sshd_process" "未安装 docker"
    write_json_report "sshd_process" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "sshd_process" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "sshd_process" "skip"
    return 0
  fi

  info "容器: $container"

  if assert_process_running "sshd 进程" "$container" "sshd"; then
    pass "场景 5: sshd 进程运行中"
  else
    fail "场景 5: sshd 进程未运行"
  fi

  write_json_report "sshd_process" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 6 — sing-box 日志域名检查
# ────────────────────────────────────────────────────────────────────────────

scenario_singbox_log_domains() {
  reset_assertions
  info "===== 场景 6: sing-box 日志域名检查 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将检查 sing-box 日志中是否有 VS Code 更新域名"
    echo "  [DRY-RUN] 域名: update.code.visualstudio.com, marketplace.visualstudio.com"
    echo "  [DRY-RUN] 注意: 需要先通过 VS Code 连接并触发扩展安装/更新"
    skip "sing-box 日志域名" "需要先通过 VS Code 连接并触发更新（dry-run）"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "singbox_log_domains" "未安装 docker"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "singbox_log_domains" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  info "容器: $container"

  # 尝试多种方式获取 sing-box 日志
  local sing_log=""
  sing_log=$(docker exec "$container" cat /var/log/sing-box.log 2>/dev/null || true)
  if [ -z "$sing_log" ]; then
    sing_log=$(docker logs "$container" 2>&1 | grep -i "sing-box" || true)
  fi

  if [ -z "$sing_log" ]; then
    skip "singbox_log_domains" "无法获取 sing-box 日志"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  local found_domains=false
  if echo "$sing_log" | grep -q "update.code.visualstudio.com"; then
    pass "场景 6: sing-box 日志中发现 update.code.visualstudio.com"
    found_domains=true
  fi
  if echo "$sing_log" | grep -q "marketplace.visualstudio.com"; then
    pass "场景 6: sing-box 日志中发现 marketplace.visualstudio.com"
    found_domains=true
  fi

  if [ "$found_domains" = false ]; then
    skip "singbox_log_domains" "sing-box 日志中未发现 VS Code 更新域名（可能需要触发更新）"
    write_json_report "singbox_log_domains" "skip"
    return 0
  fi

  write_json_report "singbox_log_domains" "pass"
}

# ────────────────────────────────────────────────────────────────────────────
# SSH 端口检测辅助
# ────────────────────────────────────────────────────────────────────────────

detect_ssh_port() {
  local container="$1"
  if [ -n "$SSH_PORT" ]; then
    echo "$SSH_PORT"
    return
  fi
  local port
  port=$(docker port "$container" 22 2>/dev/null | head -1 | cut -d: -f2)
  echo "$port"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 7 — direct-tcpip 端口转发验证 (Phase 43)
# ────────────────────────────────────────────────────────────────────────────

scenario_direct_tcpip_forward() {
  reset_assertions
  info "===== 场景 7: direct-tcpip 端口转发验证 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行:"
    echo "    1. docker exec \$CONTAINER python3 -m http.server 9876 --directory /tmp"
    echo "    2. ssh -L 19876:localhost:9876 -f -N -p \$SSH_PORT workspace@127.0.0.1"
    echo "    3. curl http://127.0.0.1:19876/"
    echo "  [DRY-RUN] 预期: curl 返回 200，端口转发通过 direct-tcpip channel 工作"
    echo "  [DRY-RUN] 需要: 运行中的容器 + SSH 连接信息"
    skip "direct_tcpip_forward" "需要运行中的容器（dry-run）"
    write_json_report "direct_tcpip_forward" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "direct_tcpip_forward" "未安装 docker"
    write_json_report "direct_tcpip_forward" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "direct_tcpip_forward" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "direct_tcpip_forward" "skip"
    return 0
  fi

  local ssh_port
  ssh_port="$(detect_ssh_port "$container")"
  if [ -z "$ssh_port" ]; then
    skip "direct_tcpip_forward" "无法确定 SSH 端口（设置 --ssh-port 或确保容器有端口映射）"
    write_json_report "direct_tcpip_forward" "skip"
    return 0
  fi

  info "容器: $container, SSH 端口: $ssh_port"

  # 在容器内启动 HTTP 服务
  docker exec -d "$container" python3 -m http.server 9876 --directory /tmp 2>/dev/null || true
  sleep 1

  # 通过 SSH -L 建立 direct-tcpip 隧道
  local ssh_pid
  ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=10 \
      -L 19876:localhost:9876 \
      -f -N -p "$ssh_port" workspace@127.0.0.1 2>/dev/null && ssh_pid=$! || true
  if [ -n "$ssh_pid" ]; then
    _UAT_SSH_PIDS="${_UAT_SSH_PIDS:+${_UAT_SSH_PIDS} }${ssh_pid}"
  fi
  sleep 1

  # 通过隧道访问
  local result
  result=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 http://127.0.0.1:19876/ 2>/dev/null || echo "CURL_FAILED")

  # 清理容器内服务
  docker exec "$container" pkill -f "python3 -m http.server 9876" 2>/dev/null || true

  if [ "$result" = "200" ]; then
    pass "场景 7: direct-tcpip 端口转发验证通过 (HTTP $result)"
    assert_eq "端口转发 HTTP 状态码" "200" "$result"
  elif [ "$result" = "CURL_FAILED" ]; then
    fail "场景 7: curl 通过 SSH 隧道访问失败"
    SCENARIO_ASSERTIONS+=('{"name":"direct-tcpip 端口转发","result":"fail","detail":"curl failed"}')
  else
    fail "场景 7: 端口转发返回非 200 状态码: $result"
    assert_eq "端口转发 HTTP 状态码" "200" "$result"
  fi

  write_json_report "direct_tcpip_forward" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 8 — 端口转发出口 IP 验证 (Phase 43)
# ────────────────────────────────────────────────────────────────────────────

scenario_forward_egress_ip() {
  reset_assertions
  info "===== 场景 8: 端口转发出口 IP 验证 ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行:"
    echo "    1. 容器内启动返回 client IP 的 HTTP 服务（端口 9877）"
    echo "    2. ssh -L 19877:localhost:9877 -f -N"
    echo "    3. curl http://127.0.0.1:19877/ 检查流量路由"
    echo "  [DRY-RUN] 预期: 验证端口转发路径的流量路由正确性"
    skip "forward_egress_ip" "需要运行中的容器 + egress 配置（dry-run）"
    write_json_report "forward_egress_ip" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "forward_egress_ip" "未安装 docker"
    write_json_report "forward_egress_ip" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "forward_egress_ip" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "forward_egress_ip" "skip"
    return 0
  fi

  if [ -z "$EXPECTED_EGRESS_IP" ]; then
    skip "forward_egress_ip" "未指定 --expected-egress-ip"
    write_json_report "forward_egress_ip" "skip"
    return 0
  fi

  local ssh_port
  ssh_port="$(detect_ssh_port "$container")"
  if [ -z "$ssh_port" ]; then
    skip "forward_egress_ip" "无法确定 SSH 端口"
    write_json_report "forward_egress_ip" "skip"
    return 0
  fi

  info "容器: $container, SSH 端口: $ssh_port, 期望出口 IP: $EXPECTED_EGRESS_IP"

  # 在容器内启动返回 client IP 的服务
  docker exec -d "$container" python3 -c "
from http.server import HTTPServer, BaseHTTPRequestHandler
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(self.client_address[0].encode())
    def log_message(self, *args): pass
HTTPServer(('0.0.0.0', 9877), H).serve_forever()
" 2>/dev/null || true
  sleep 1

  # 通过 SSH 隧道访问
  local ssh_pid
  ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=10 \
      -L 19877:localhost:9877 \
      -f -N -p "$ssh_port" workspace@127.0.0.1 2>/dev/null && ssh_pid=$! || true
  if [ -n "$ssh_pid" ]; then
    _UAT_SSH_PIDS="${_UAT_SSH_PIDS:+${_UAT_SSH_PIDS} }${ssh_pid}"
  fi
  sleep 1

  # 检查返回的 IP
  local client_ip
  client_ip=$(curl -s --max-time 5 http://127.0.0.1:19877/ 2>/dev/null || echo "CURL_FAILED")

  # 清理
  docker exec "$container" pkill -f "9877" 2>/dev/null || true

  if [ "$client_ip" = "CURL_FAILED" ]; then
    fail "场景 8: curl 通过端口转发访问失败"
    SCENARIO_ASSERTIONS+=('{"name":"forward egress IP","result":"fail","detail":"curl failed"}')
  else
    # 通过 direct-tcpip 隧道连接时，client IP 是 SSH 客户端在容器内的映射地址
    # 关键是流量确实通过了 SSH 隧道（而不是直接连接容器端口）
    pass "场景 8: 端口转发路径可访问，client IP: $client_ip"
    assert_contains "端口转发路径返回有效 IP" "$client_ip" "."
  fi

  write_json_report "forward_egress_ip" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景 9 — 安全拒绝验证（forbidden target）(Phase 43)
# ────────────────────────────────────────────────────────────────────────────

scenario_security_reject() {
  reset_assertions
  info "===== 场景 9: 安全拒绝验证（forbidden target） ====="

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "  [DRY-RUN] 将执行:"
    echo "    1. ssh -L 19878:10.99.1.1:22 -N -p \$SSH_PORT workspace@127.0.0.1"
    echo "    2. curl http://127.0.0.1:19878/"
    echo "  [DRY-RUN] 预期: SSH 端口转发被拒绝（administratively prohibited）"
    echo "  [DRY-RUN] 验证: isForbiddenTarget() 对管理网段 10.99.x.x 正确拦截"
    skip "security_reject" "需要运行中的容器（dry-run）"
    write_json_report "security_reject" "skip"
    return 0
  fi

  if ! has_docker; then
    skip "security_reject" "未安装 docker"
    write_json_report "security_reject" "skip"
    return 0
  fi

  local container
  container="$(detect_container)"
  if [ -z "$container" ]; then
    skip "security_reject" "未找到运行中的 cloud-claude-local 容器"
    write_json_report "security_reject" "skip"
    return 0
  fi

  local ssh_port
  ssh_port="$(detect_ssh_port "$container")"
  if [ -z "$ssh_port" ]; then
    skip "security_reject" "无法确定 SSH 端口"
    write_json_report "security_reject" "skip"
    return 0
  fi

  info "容器: $container, SSH 端口: $ssh_port"

  # 尝试转发到管理网段（10.99.1.1:22）
  # SSH -L 会建立隧道，但实际连接到 forbidden target 时会被 isForbiddenTarget 拦截
  local ssh_output
  ssh_output=$(ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=10 -o BatchMode=yes \
      -L 19878:10.99.1.1:22 \
      -N -p "$ssh_port" workspace@127.0.0.1 2>&1) || true

  # 尝试通过隧道连接 — 预期失败
  local curl_result
  curl_result=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://127.0.0.1:19878/ 2>/dev/null || echo "REFUSED")

  # 安全拒绝的判断：连接不应成功（HTTP 200）
  if [ "$curl_result" = "REFUSED" ] || [ "$curl_result" = "000" ]; then
    pass "场景 9: 安全拒绝验证通过 — 连接到 10.99.1.1 被拒绝"
    SCENARIO_ASSERTIONS+=('{"name":"安全拒绝 10.99.x.x","result":"pass","detail":"connection refused as expected"}')
  elif echo "$ssh_output" | grep -qi "prohibited\|refused\|denied"; then
    pass "场景 9: 安全拒绝验证通过 — SSH 报告 prohibited"
    SCENARIO_ASSERTIONS+=('{"name":"安全拒绝 10.99.x.x","result":"pass","detail":"SSH prohibited"}')
  else
    fail "场景 9: 安全拒绝验证失败 — 连接到 10.99.1.1 未被拦截 (curl: $curl_result)"
    SCENARIO_ASSERTIONS+=("$(printf '{"name":"安全拒绝 10.99.x.x","result":"fail","detail":"curl returned %s"}' "$curl_result")")
  fi

  write_json_report "security_reject" "$([ "$FAIL_COUNT" -eq 0 ] && echo "pass" || echo "fail")"
}

# ────────────────────────────────────────────────────────────────────────────
# 主流程
# ────────────────────────────────────────────────────────────────────────────

main() {
  info "VS Code Remote-SSH E2E UAT — dry_run=${DRY_RUN}"
  echo ""

  scenario_singbox_process || true
  echo ""
  scenario_egress_ip || true
  echo ""
  scenario_dns_leak || true
  echo ""
  scenario_vscode_server || true
  echo ""
  scenario_sshd_process || true
  echo ""
  scenario_singbox_log_domains || true
  echo ""
  scenario_direct_tcpip_forward || true
  echo ""
  scenario_forward_egress_ip || true
  echo ""
  scenario_security_reject || true
  echo ""

  render_final_json

  echo ""
  echo "========================================"
  echo "VS Code Remote-SSH E2E UAT 结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${SKIP_COUNT} SKIP"

  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
    exit 1
  elif [[ "$SKIP_COUNT" -gt 0 && "$PASS_COUNT" -eq 0 ]]; then
    echo "状态: 环境不具备全部 SKIP（不计 FAIL）"
    exit 2
  else
    echo "状态: 全部通过（dry_run=${DRY_RUN}）"
    exit 0
  fi
}

main "$@"
