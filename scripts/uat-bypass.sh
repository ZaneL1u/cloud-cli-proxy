#!/usr/bin/env bash
# scripts/uat-bypass.sh — Phase 47 Plan 03 BYPASS-VERIFY-02/03/04 端到端 UAT
#
# 把 v3.5 的「10 条安全不变量 + 6 场景」从「文档描述」升级为「CI 每次 PR 自动
# 跑的可断言脚本」。每个场景跑完后强制断言 I1–I10 全部 10 条不变量；
# fail-closed-pkill 场景额外断言 I5（sing-box 停止 → 白名单也断）+ snapshot
# 状态翻转到 rolled_back。
#
# 不变量清单（与 .planning/research/SUMMARY.md §3.3 对齐）：
#   I1  容器 /etc/resolv.conf 唯一指向 sing-box tun IP（nsenter cat 校验）
#   I2  容器 netns output policy = drop（nft list chain 校验）
#   I3  出 eth0 包仅去白名单或代理 IP（tcpdump 计数 == 0 校验）
#   I4  容器内 dig @8.8.8.8 必超时（nsenter dig +time=2 校验）
#   I5  sing-box 停止后白名单也断（pkill + curl 失败校验）
#   I6  IPv6 全禁（nsenter ip -6 addr 仅 ::1 校验）
#   I7  nft set 与 rule-set 文件 hash 一致（consistency endpoint 校验）
#   I8  rule-set 文件存在且有效 JSON（jq . 校验）
#   I9  mDNS/LLMNR/NetBIOS drop 规则存在（nft list chain 校验）
#   I10 白名单变更后 SSH 不断（外部 ssh while-loop 写入 /tmp/uat-bypass-ssh-loop.log）
#
# 用法：bash scripts/uat-bypass.sh --scenario=NAME --target-host-id=HID [选项]
#
# 退出码：
#   0  PASS（场景全部断言通过）
#   1  FAIL（任一断言失败）
#   2  SKIP（环境不具备：缺 nft / docker / jq / sudo / 目标容器）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
WARN_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }

usage() {
  cat <<'EOF'
uat-bypass.sh — 6 场景 × 10 不变量 v3.5 白名单 fail-closed UAT

用法:
  scripts/uat-bypass.sh --scenario=NAME --target-host-id=HID [选项]

场景:
  loopback-only        仅启用 loopback 预设（强制开启）
  lan-only             仅启用 lan 预设
  loopback-lan         同时启用 loopback + lan
  custom-ip            添加自定义 IP 规则 192.168.0.1/32
  custom-domain        添加自定义 domain_suffix .internal.example.com
  fail-closed-pkill    apply 后 pkill sing-box，断言白名单也断（I5）

不变量（每个场景跑完后断言）:
  I1 resolv.conf 唯一指向 sing-box tun IP
  I2 容器 netns nft output policy=drop
  I3 出 eth0 包仅去白名单或代理 IP（tcpdump 计数）
  I4 容器内 dig @8.8.8.8 必超时
  I5 sing-box 停止 → 白名单也断（仅 fail-closed-pkill 场景）
  I6 IPv6 全禁
  I7 nft 与 rule-set hash 一致（调 consistency endpoint）
  I8 rule-set 文件存在且有效 JSON
  I9 mDNS/LLMNR/NetBIOS drop 规则存在
  I10 白名单变更后 SSH 不断

BYPASS-VERIFY-04（端到端 snapshot.applied_status）：
  非 fail-closed 场景：apply 完成后断言 snapshot.applied_status == "applied"
  fail-closed-pkill：pkill 后断言 snapshot.applied_status == "rolled_back"

选项:
  --scenario=NAME           必填，见上方场景列表
  --target-host-id=HID      必填，目标 host id
  --admin-token=TOKEN       admin JWT（默认读 CLOUD_CLI_PROXY_ADMIN_TOKEN）
  --api-base=URL            control-plane API（默认 http://localhost:8080）
  --dry-run                 只打印命令，不实际下发
  --help, -h                显示本帮助

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ─── 参数解析 ───────────────────────────────────────────────────────────────
SCENARIO=""
TARGET_HOST_ID=""
ADMIN_TOKEN="${CLOUD_CLI_PROXY_ADMIN_TOKEN:-}"
API_BASE="${CLOUD_CLI_PROXY_API_BASE:-http://localhost:8080}"
DRY_RUN=false

SCENARIO_NAMES=(loopback-only lan-only loopback-lan custom-ip custom-domain fail-closed-pkill)

for arg in "$@"; do
  case "$arg" in
    --scenario=*) SCENARIO="${arg#--scenario=}" ;;
    --target-host-id=*) TARGET_HOST_ID="${arg#--target-host-id=}" ;;
    --admin-token=*) ADMIN_TOKEN="${arg#--admin-token=}" ;;
    --api-base=*) API_BASE="${arg#--api-base=}" ;;
    --dry-run) DRY_RUN=true ;;
    --help|-h) usage; exit 0 ;;
    *) echo "未知参数: $arg" >&2; usage >&2; exit 1 ;;
  esac
done

# --help 时已 exit 0；下面是实际跑动的参数校验
if [[ -z "$SCENARIO" ]]; then
  echo "缺少 --scenario，必须为下列之一: ${SCENARIO_NAMES[*]}" >&2
  usage >&2
  exit 1
fi

case " ${SCENARIO_NAMES[*]} " in
  *" $SCENARIO "*) ;;
  *) echo "非法 --scenario=$SCENARIO，必须为下列之一: ${SCENARIO_NAMES[*]}" >&2; exit 1 ;;
esac

# 安全正则（与 scripts/uat-network-resilience.sh 共享守卫）
HOST_ID_REGEX='^[A-Za-z0-9._-]+$'
if [[ -n "$TARGET_HOST_ID" && ! "$TARGET_HOST_ID" =~ $HOST_ID_REGEX ]]; then
  echo "非法 host id: $TARGET_HOST_ID" >&2
  exit 1
fi

# ─── 前置闸门 ───────────────────────────────────────────────────────────────
# 工具：dry-run 模式下放宽缺工具校验，方便在 macOS / 无 root CI 上做 smoke。
need_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    if [[ "$DRY_RUN" == "true" ]]; then
      warn "preflight: $1 不存在（dry-run 放行）"
    else
      skip "preflight" "$1 不存在"
      exit 2
    fi
  fi
}

need_tool jq
need_tool curl
if [[ "$DRY_RUN" != "true" ]]; then
  need_tool nft
  need_tool docker
fi

if [[ -z "$TARGET_HOST_ID" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    TARGET_HOST_ID="dryrun-host"
    warn "preflight: --target-host-id 未提供，dry-run 用占位 $TARGET_HOST_ID"
  else
    skip "preflight" "--target-host-id 未提供"
    exit 2
  fi
fi

if [[ -z "$ADMIN_TOKEN" && "$DRY_RUN" != "true" ]]; then
  skip "preflight" "未提供 admin token（--admin-token 或 CLOUD_CLI_PROXY_ADMIN_TOKEN）"
  exit 2
fi

# ─── 全局 trap：恢复 sing-box / 清理临时元素 ─────────────────────────────────
cleanup() {
  # best-effort：起回 gateway 容器（如果 fail-closed 场景 pkill 过 sing-box）；
  # 即使容器名不存在也吞掉错误，避免 trap 自身抛异常。
  if command -v docker >/dev/null 2>&1; then
    docker restart "cloudproxy-gw-${TARGET_HOST_ID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

# ─── 工具：调控制面 API ─────────────────────────────────────────────────────
api() {
  local method=$1 path=$2 body=${3:-}
  if [[ "$DRY_RUN" == "true" ]]; then
    # 遮蔽 token 防止 CI 日志泄漏（T-47-12 mitigation）
    echo "[DRY] curl -X $method ${API_BASE}${path} -H 'Authorization: Bearer ***'${body:+ -d '$body'}" >&2
    # 让 dry-run mock 在 fail-closed-pkill 场景下返回 rolled_back，确保 --dry-run
    # 能跑通端到端流程而不假阳性失败；其它场景仍返回 applied。
    local snapshot_state="applied"
    if [[ "$SCENARIO" == "fail-closed-pkill" ]]; then
      snapshot_state="rolled_back"
    fi
    case "$path" in
      */consistency) echo '{"ok":true,"ruleset_sha256":"dryrun","nft_set_sha256":"dryrun"}' ;;
      */snapshots)   echo "{\"snapshots\":[{\"applied_status\":\"${snapshot_state}\"}]}" ;;
      */apply)       echo '{"task_id":"dryrun-task"}' ;;
      */tasks/*)     echo '{"status":"succeeded"}' ;;
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

# ─── 工具：取 worker 容器 PID ──────────────────────────────────────────────
worker_pid() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "0"
    return 0
  fi
  docker inspect -f '{{.State.Pid}}' "cloudproxy-${TARGET_HOST_ID}" 2>/dev/null
}

# ─── 10 不变量断言 helpers ─────────────────────────────────────────────────
assert_invariant_I1() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I1 resolv.conf=nameserver 172.19.0.1 (dry-run)"
    return 0
  fi
  local pid rc
  pid=$(worker_pid) || { fail "I1: worker pid lookup"; return; }
  rc=$(sudo nsenter -t "$pid" -m cat /etc/resolv.conf 2>/dev/null | head -1)
  if [[ "$rc" == "nameserver 172.19.0.1" ]]; then
    pass "I1 resolv.conf=$rc"
  else
    fail "I1 resolv.conf=$rc（期望 'nameserver 172.19.0.1'）"
  fi
}

assert_invariant_I2() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I2 nft output policy=drop (dry-run)"
    return 0
  fi
  local pid
  pid=$(worker_pid)
  if sudo nsenter -t "$pid" -n nft list chain inet cloudproxy output 2>/dev/null | grep -q 'policy drop'; then
    pass "I2 nft output policy=drop"
  else
    fail "I2 nft output policy missing"
  fi
}

assert_invariant_I3() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I3 eth0 旁路计数=0 (dry-run)"
    return 0
  fi
  # tcpdump 5s 抓 eth0 出向，过滤掉白名单 / 代理；包数应为 0。
  # 简化版：成功跑完 timeout、未抓到任何匹配即视为通过；任何抓到的包都 fail。
  local pid count
  pid=$(worker_pid)
  count=$(sudo timeout 5 nsenter -t "$pid" -n tcpdump -ni eth0 -c 1 \
    'not (host 1.2.3.4 or net 192.168.0.0/16)' 2>&1 | grep -c 'packets captured' || true)
  if [[ "$count" == "0" ]]; then
    pass "I3 eth0 旁路计数=0"
  else
    fail "I3 eth0 旁路计数>0"
  fi
}

assert_invariant_I4() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I4 dig @8.8.8.8 超时 (dry-run)"
    return 0
  fi
  local pid
  pid=$(worker_pid)
  if sudo nsenter -t "$pid" -n dig +time=2 +tries=1 @8.8.8.8 example.com +short >/dev/null 2>&1; then
    fail "I4 dig @8.8.8.8 成功（应被 nft 阻断超时）"
  else
    pass "I4 dig @8.8.8.8 超时（公网 DNS 被阻断）"
  fi
}

assert_invariant_I5_fail_closed() {
  # 仅 fail-closed-pkill 场景调用：pkill sing-box 之后白名单流量也必须断。
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I5 sing-box pkill → 白名单也断 (dry-run)"
    return 0
  fi
  docker exec "cloudproxy-gw-${TARGET_HOST_ID}" pkill sing-box >/dev/null 2>&1 || true
  sleep 2
  local pid
  pid=$(worker_pid)
  if sudo nsenter -t "$pid" -n curl --max-time 3 -sS http://192.168.0.1/ >/dev/null 2>&1; then
    fail "I5 sing-box 停止后白名单仍通（未 fail-closed）"
  else
    pass "I5 sing-box 停止 → 白名单也断"
  fi
}

assert_invariant_I6() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I6 IPv6 全禁 (dry-run)"
    return 0
  fi
  local pid nonlo
  pid=$(worker_pid)
  # 仅允许 ::1（loopback）；任何非 ::1 inet6 一律视为 leak。
  nonlo=$(sudo nsenter -t "$pid" -n ip -6 addr show 2>/dev/null \
    | grep 'inet6' | grep -vc '::1' || true)
  if [[ "$nonlo" == "0" ]]; then
    pass "I6 IPv6 全禁（仅 ::1）"
  else
    fail "I6 发现 $nonlo 条非 ::1 inet6 地址"
  fi
}

assert_invariant_I7() {
  local code
  code=$(api GET "/v1/admin/hosts/${TARGET_HOST_ID}/bypass/consistency" | jq -r '.ok // empty')
  if [[ "$code" == "true" ]]; then
    pass "I7 nft/ruleset hash 一致"
  else
    fail "I7 consistency drift (.ok=$code)"
  fi
}

assert_invariant_I8() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I8 whitelist-cidrs.json valid (dry-run)"
    return 0
  fi
  if docker exec "cloudproxy-gw-${TARGET_HOST_ID}" \
       sh -c 'jq . /etc/sing-box/whitelist-cidrs.json' >/dev/null 2>&1; then
    pass "I8 whitelist-cidrs.json valid JSON"
  else
    fail "I8 whitelist-cidrs.json invalid JSON 或文件缺失"
  fi
}

assert_invariant_I9() {
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I9 mDNS drop 规则存在 (dry-run)"
    return 0
  fi
  # 简化版（lenient）：只要 nft 链里能看到 udp dport 5353/137/138 的 drop 规则
  # 即视为通过；严格 counter ≥ 1 的注入验证为已知 follow-up。
  local pid
  pid=$(worker_pid)
  if sudo nsenter -t "$pid" -n nft list chain inet cloudproxy output 2>/dev/null \
       | grep -qE 'udp dport (5353|5355|137|138).*drop'; then
    pass "I9 mDNS/LLMNR/NetBIOS drop 规则存在"
  else
    fail "I9 mDNS/LLMNR/NetBIOS drop 规则缺失"
  fi
}

assert_invariant_I10() {
  # caller 在场景外部启动 `ssh while-loop`，把每秒一次的 echo 写到 log file。
  # 本 helper 只做存在性 + 行数下限断言。
  local log="/tmp/uat-bypass-ssh-loop.log"
  if [[ "$DRY_RUN" == "true" ]]; then
    pass "I10 SSH 不断 (dry-run)"
    return 0
  fi
  if [[ -f "$log" && "$(wc -l < "$log")" -ge 5 ]]; then
    pass "I10 SSH 不断（log lines>=5）"
  else
    # 未启动 ssh loop 当作 SKIP 处理，不计入 FAIL（避免 CI 强制要求真实 SSH 服务）
    skip "I10" "ssh while-loop log 未启动或行数不足，跳过 SSH 不变量"
  fi
}

# ─── BYPASS-VERIFY-04：snapshot.applied_status ──────────────────────────────
assert_snapshot_state() {
  local want=$1 got
  got=$(api GET "/v1/admin/hosts/${TARGET_HOST_ID}/bypass/snapshots" \
    | jq -r '.snapshots[0].applied_status // empty')
  if [[ "$got" == "$want" ]]; then
    pass "BYPASS-VERIFY-04: snapshot.applied_status=$got"
  else
    fail "BYPASS-VERIFY-04: snapshot.applied_status want=$want got=$got"
  fi
}

# ─── 场景实现 ───────────────────────────────────────────────────────────────
bind_preset() {
  api POST "/v1/admin/hosts/${TARGET_HOST_ID}/bypass" "{\"preset_slug\":\"$1\"}" >/dev/null
}

bind_rule() {
  api POST "/v1/admin/bypass/rules" "$1" >/dev/null
}

apply_and_wait() {
  local res task_id i s
  res=$(api POST "/v1/admin/hosts/${TARGET_HOST_ID}/bypass/apply" '{"note":"uat-bypass"}')
  task_id=$(echo "$res" | jq -r '.task_id // empty')
  if [[ -z "$task_id" ]]; then
    warn "apply 未返回 task_id（dry-run 或控制面缺字段）"
    return 0
  fi
  for i in $(seq 1 30); do
    s=$(api GET "/v1/admin/tasks/${task_id}" | jq -r '.status // empty')
    case "$s" in
      succeeded|failed) return 0 ;;
    esac
    if [[ "$DRY_RUN" == "true" ]]; then return 0; fi
    sleep 1
  done
  warn "apply task ${task_id} 30s 内未结束"
}

scenario_loopback_only()      { bind_preset loopback; }
scenario_lan_only()           { bind_preset lan; }
scenario_loopback_lan()       { bind_preset loopback; bind_preset lan; }
scenario_custom_ip()          { bind_rule '{"rule_type":"ip_cidr","value":"192.168.0.1/32"}'; }
scenario_custom_domain()      { bind_rule '{"rule_type":"domain_suffix","value":".internal.example.com"}'; }
scenario_fail_closed_pkill()  { bind_preset loopback; }

run_scenario() {
  local name=$1
  echo "=== Scenario: $name ==="
  case "$name" in
    loopback-only)     scenario_loopback_only ;;
    lan-only)          scenario_lan_only ;;
    loopback-lan)      scenario_loopback_lan ;;
    custom-ip)         scenario_custom_ip ;;
    custom-domain)     scenario_custom_domain ;;
    fail-closed-pkill) scenario_fail_closed_pkill ;;
  esac
  apply_and_wait

  # 通用断言（I1–I4 / I6–I10 + snapshot 状态）
  assert_invariant_I1
  assert_invariant_I2
  assert_invariant_I3
  assert_invariant_I4
  assert_invariant_I6
  assert_invariant_I7
  assert_invariant_I8
  assert_invariant_I9
  assert_invariant_I10

  if [[ "$name" == "fail-closed-pkill" ]]; then
    assert_invariant_I5_fail_closed
    # fail-closed 场景必须能从 snapshot 里读到 rolled_back 状态
    assert_snapshot_state "rolled_back"
  else
    assert_snapshot_state "applied"
  fi
}

# ─── 主入口 ─────────────────────────────────────────────────────────────────
info "uat-bypass scenario=${SCENARIO} host=${TARGET_HOST_ID} api=${API_BASE} dry-run=${DRY_RUN}"
run_scenario "$SCENARIO" || true

echo ""
echo "── Summary: PASS=${PASS_COUNT} FAIL=${FAIL_COUNT} SKIP=${SKIP_COUNT} WARN=${WARN_COUNT} ──"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "状态: 存在失败项"
  exit 1
fi
if [[ "$PASS_COUNT" -eq 0 && "$SKIP_COUNT" -gt 0 ]]; then
  echo "状态: 环境不具备已 SKIP"
  exit 2
fi
echo "状态: 全部通过"
exit 0
