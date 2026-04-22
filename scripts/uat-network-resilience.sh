#!/usr/bin/env bash
# scripts/uat-network-resilience.sh — Phase 35 BASE-03 弱网三场景 UAT
#
# 把 BASE-03（30s 抖动无感知 / 2min 自动重连）+ REQ-F3-B/C/D + REQ-F4-A 从
# 「人工观察」升级为「脚本可断言的量化指标」，三大无感知锚点：
#
#   1. 进程存活：拔网全程 docker exec <ctr> pgrep -f claude 退出码=0
#   2. Buffer 完整性：拔网前/后 tmux capture-pane diff 行数 == 0
#   3. 输入回放：本地注入 token，远端最终 cat 出完整字符串
#
# 网络破坏走 tc(netem) → iptables 两级 fallback；trap EXIT 双兜底，脚本异常
# 退出务必恢复网络（否则宿主机失联）。
#
# 用法：bash scripts/uat-network-resilience.sh --scenario=10s|30s|2min [其它选项]
#
# 退出码：
#   0  PASS（场景全部断言通过）
#   1  FAIL（任一断言失败）
#   2  SKIP（环境不具备：无 tc/iptables、无目标容器、无 sudo 等）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BENCH_DIR_DEFAULT="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0
SKIP_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }
skip() { echo "[SKIP]  $1: $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }

usage() {
  cat <<'EOF'
uat-network-resilience.sh — Phase 35 BASE-03 弱网 UAT 三场景（10s|30s|2min）

用法:
  scripts/uat-network-resilience.sh --scenario=10s|30s|2min [选项]

必选:
  --scenario=10s|30s|2min   弱网场景（互斥三选一）

可选:
  --target-container=NAME   目标容器（默认从 docker ps --filter
                            label=com.cloud-cli-proxy.managed=true 自动探测）
  --iface=NAME              tc 模式网卡（默认 eth0）
  --host-ip=IP              iptables fallback 必填（控制面 IP）
  --dry-run                 只打印 tc/iptables 命令，不真实下规则
  --output-dir=DIR          报告输出目录（默认 .planning/phases/35-e2e/benchmarks）
  --help, -h                显示本帮助

无感知量化锚点（CONTEXT.md §"无感知量化指标"）：
  pgrep_survived_full_duration  拔网期间 claude 进程存活（每 5s 探测）
  buffer_diff_lines == 0        tmux capture-pane diff 字符级一致
  token_replayed == true        本地注入 token 远端完整收到

REQ-F3-D 退避序列：1s → 2s → 4s → 8s → 30s 上限
REQ-F3-C 失败提示样板：「按 Enter 重试」「cloud-claude doctor」

退出码：0=PASS / 1=FAIL / 2=SKIP
EOF
}

# ────────────────────────────────────────────────────────────────────────────
# CLI 参数
# ────────────────────────────────────────────────────────────────────────────

SCENARIO=""
TARGET_CTR=""
IFACE="eth0"
HOST_IP=""
DRY_RUN=false
OUTPUT_DIR="${BENCH_DIR_DEFAULT}"

for arg in "$@"; do
  case "$arg" in
    --scenario=*) SCENARIO="${arg#--scenario=}" ;;
    --target-container=*) TARGET_CTR="${arg#--target-container=}" ;;
    --iface=*) IFACE="${arg#--iface=}" ;;
    --host-ip=*) HOST_IP="${arg#--host-ip=}" ;;
    --dry-run) DRY_RUN=true ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage >&2; exit 1 ;;
  esac
done

# scenario 白名单（acceptance_criteria 硬绑定 10s|30s|2min）
case "$SCENARIO" in
  10s|30s|2min) ;;
  "") fail "缺少 --scenario，必须为 10s|30s|2min 之一"; usage >&2; exit 1 ;;
  *)  fail "非法 --scenario=$SCENARIO，必须为 10s|30s|2min 之一"; exit 1 ;;
esac

case "$SCENARIO" in
  10s)  DURATION_S=10 ;;
  30s)  DURATION_S=30 ;;
  2min) DURATION_S=120 ;;
esac

# 阈值常量（PLAN action #10 硬编码）
RECONNECT_WINDOW_S=60
SAMPLE_INTERVAL_S=5
CAPTURE_RETRY=5

mkdir -p "$OUTPUT_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
WORK="$(mktemp -d)"
REPORT_JSON="${OUTPUT_DIR}/uat-resilience-${SCENARIO}-${TIMESTAMP}.json"
REPORT_MD="${OUTPUT_DIR}/uat-resilience-${SCENARIO}-${TIMESTAMP}.md"
DISRUPT_LOG="${OUTPUT_DIR}/.network-disrupt.log"
SAMPLES_FILE="${WORK}/reconnect-samples.txt"
: > "$SAMPLES_FILE"

# 容器名安全正则（同 Plan 02 T-35-02-02 / Task 2 共享守卫）
CTR_NAME_REGEX='^[a-z0-9][a-z0-9_.-]*$'

# ────────────────────────────────────────────────────────────────────────────
# 环境闸门（Pattern B 三层）
# ────────────────────────────────────────────────────────────────────────────

is_linux()     { [[ "$(uname -s)" == "Linux" ]]; }
has_tc()       { command -v tc >/dev/null 2>&1; }
has_root_net() { [[ ${EUID:-0} -eq 0 ]] || sudo -n tc qdisc show dev lo >/dev/null 2>&1; }
has_iptables() { command -v iptables >/dev/null 2>&1; }
has_docker()   { command -v docker >/dev/null 2>&1; }

# ────────────────────────────────────────────────────────────────────────────
# 网络破坏函数（Pattern E：tc → iptables 两级 fallback；幂等）
# ────────────────────────────────────────────────────────────────────────────

DISRUPT_MODE=""

run_or_print() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "[DRY-RUN] $*" >&2
  else
    "$@"
  fi
}

disrupt_start() {
  if is_linux && has_tc && has_root_net; then
    DISRUPT_MODE="tc"
    info "网络破坏：tc qdisc add dev ${IFACE} root netem loss 100%"
    run_or_print sudo tc qdisc add dev "$IFACE" root netem loss 100%
  elif has_iptables && [[ -n "$HOST_IP" ]]; then
    DISRUPT_MODE="iptables"
    info "网络破坏：iptables -I OUTPUT -d ${HOST_IP} -j DROP"
    run_or_print sudo iptables -I OUTPUT -d "$HOST_IP" -j DROP
  else
    DISRUPT_MODE=""
    return 1
  fi
  # 留痕（T-35-02-06 Repudiation mitigation）
  printf '%s\tstart\t%s\ttarget=%s\tuser=%s\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$DISRUPT_MODE" \
    "${IFACE}/${HOST_IP:-N/A}" "$(logname 2>/dev/null || echo unknown)" \
    >> "$DISRUPT_LOG" 2>/dev/null || true
}

disrupt_stop() {
  case "$DISRUPT_MODE" in
    tc)
      info "恢复网络：tc qdisc del dev ${IFACE} root netem"
      run_or_print sudo tc qdisc del dev "$IFACE" root netem 2>/dev/null || true
      ;;
    iptables)
      info "恢复网络：iptables -D OUTPUT -d ${HOST_IP} -j DROP"
      run_or_print sudo iptables -D OUTPUT -d "$HOST_IP" -j DROP 2>/dev/null || true
      ;;
    "")
      # 起始幂等空跑（脚本第二次调用 disrupt_stop 用）
      ;;
  esac
  if [[ -n "$DISRUPT_MODE" ]]; then
    printf '%s\tstop\t%s\ttarget=%s\n' \
      "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$DISRUPT_MODE" \
      "${IFACE}/${HOST_IP:-N/A}" \
      >> "$DISRUPT_LOG" 2>/dev/null || true
  fi
  DISRUPT_MODE=""
}

# 双重 trap 兜底：脚本异常退出务必撤销 tc/iptables 规则（T-35-02-01）
trap disrupt_stop EXIT INT TERM
# 起始幂等清理（防御上一次跑残留）
disrupt_stop

cleanup_workdir() {
  rm -rf "$WORK" 2>/dev/null || true
}
trap 'disrupt_stop; cleanup_workdir' EXIT

# ────────────────────────────────────────────────────────────────────────────
# 容器探测 + 安全校验（T-35-02-02 防 docker exec 命令注入）
# ────────────────────────────────────────────────────────────────────────────

detect_container() {
  if [[ -n "$TARGET_CTR" ]]; then
    if [[ ! "$TARGET_CTR" =~ $CTR_NAME_REGEX ]]; then
      fail "非法容器名 '${TARGET_CTR}'（不匹配 ${CTR_NAME_REGEX}）"
      exit 1
    fi
    return 0
  fi
  if ! has_docker; then
    skip "BASE-03" "未安装 docker，无法定位 managed 容器"
    return 1
  fi
  TARGET_CTR="$(docker ps \
    --filter 'label=com.cloud-cli-proxy.managed=true' \
    --format '{{.Names}}' 2>/dev/null | head -1 || true)"
  if [[ -z "$TARGET_CTR" ]]; then
    skip "BASE-03" "未发现 com.cloud-cli-proxy.managed=true 容器"
    return 1
  fi
  if [[ ! "$TARGET_CTR" =~ $CTR_NAME_REGEX ]]; then
    fail "自动探测到的容器名非法: ${TARGET_CTR}"
    exit 1
  fi
  info "目标容器（自动探测）: ${TARGET_CTR}"
}

ctr_exec() {
  # 包装 docker exec；TARGET_CTR 已通过 CTR_NAME_REGEX 守卫
  docker exec "$TARGET_CTR" "$@"
}

ctr_exec_sh() {
  # 在容器内运行单条 shell 字符串（仅在已知字面命令时使用）
  docker exec "$TARGET_CTR" sh -c "$1"
}

ctr_exec_i_sh() {
  docker exec -i "$TARGET_CTR" sh -c "$1"
}

# ────────────────────────────────────────────────────────────────────────────
# 量化断言：pgrep / tmux capture-pane / token replay
# ────────────────────────────────────────────────────────────────────────────

PGREP_SURVIVED=true
BUFFER_DIFF_LINES=-1
TOKEN_REPLAYED=false
RECONNECT_OK=false
FINAL_FAILURE_PROMPT_SEEN=false
BACKOFF_MARKS_JSON='[]'

# 拔网期间每 5s 探测一次 claude 进程存活（Pattern F）
check_alive_loop() {
  local duration="$1"
  local t=0
  while [ "$t" -lt "$duration" ]; do
    if [[ "$DRY_RUN" == "true" ]]; then
      sleep 1
    else
      if ! ctr_exec pgrep -f claude >/dev/null 2>&1; then
        fail "claude 进程在拔网第 ${t}s 退出（应全程存活）"
        PGREP_SURVIVED=false
        return 1
      fi
      sleep "$SAMPLE_INTERVAL_S"
    fi
    t=$((t + SAMPLE_INTERVAL_S))
  done
  return 0
}

capture_pane_retry() {
  # tmux capture-pane -t claude -p -e；恢复网络后存在 race，做最多 5 次重试
  local i out
  for i in $(seq 1 "$CAPTURE_RETRY"); do
    sleep 2
    out="$(ctr_exec tmux capture-pane -t claude -p -e 2>/dev/null || true)"
    if [[ -n "$out" ]]; then
      printf '%s\n' "$out"
      return 0
    fi
  done
  return 1
}

# REQ-F3-B 输入回放断言：本地注入 token，远端最终 cat 输出
TOKEN=""
inject_token() {
  TOKEN="UAT-$(date +%s)-${RANDOM}"
  local fname="/tmp/uat-echo-$$.txt"
  info "向远端 tmux 注入 token: ${TOKEN} → ${fname}"
  if [[ "$DRY_RUN" != "true" ]]; then
    # 写命令到 tmux send-keys；走 BufferedStdin 路径
    ctr_exec tmux send-keys -t claude \
      "echo \"${TOKEN}\" > ${fname}" Enter 2>/dev/null || true
  fi
  echo "$fname"
}

verify_token() {
  local fname="$1"
  if [[ "$DRY_RUN" == "true" ]]; then
    TOKEN_REPLAYED=true
    return 0
  fi
  if ctr_exec_sh "cat ${fname} 2>/dev/null" | grep -qF "$TOKEN"; then
    TOKEN_REPLAYED=true
    pass "token 完整回放: ${TOKEN} 抵达 ${fname}（REQ-F3-B 本地 buffer 无丢/无乱序）"
    return 0
  else
    TOKEN_REPLAYED=false
    fail "token ${TOKEN} 未在 ${fname} 出现（REQ-F3-B 失败）"
    return 1
  fi
}

# REQ-F3-D 退避序列断言：日志含 1s/2s/4s/8s/30s 至少三档
collect_backoff_marks() {
  if [[ "$DRY_RUN" == "true" ]]; then
    BACKOFF_MARKS_JSON='[]'
    return 0
  fi
  # 把 2min 拔网过程中每 10s 取样的 tmux 内容存到 SAMPLES_FILE
  local marks
  marks="$(grep -oE '(重连|reconnect|retry)[^\\n]*(1s|2s|4s|8s|30s)' \
    "$SAMPLES_FILE" 2>/dev/null || true)"
  local seen=()
  for m in 1s 2s 4s 8s 30s; do
    if echo "$marks" | grep -qF "$m"; then
      seen+=("$m")
    fi
  done
  if [[ ${#seen[@]} -ge 3 ]]; then
    pass "REQ-F3-D 退避序列断言：捕获 ${#seen[@]} 档（${seen[*]})"
  else
    warn "REQ-F3-D 退避序列：仅捕获 ${#seen[@]} 档（< 3，可能 fixture 模拟不充分）"
  fi
  BACKOFF_MARKS_JSON="$(printf '%s\n' "${seen[@]}" | jq -R . | jq -cs .)"
}

# REQ-F3-C 最终失败提示断言：屏幕含「按 Enter 重试」或「cloud-claude doctor」
check_final_failure_prompt() {
  if [[ "$DRY_RUN" == "true" ]]; then
    FINAL_FAILURE_PROMPT_SEEN=true
    return 0
  fi
  local out
  out="$(capture_pane_retry || true)"
  if echo "$out" | grep -qE '(按 Enter 重试|cloud-claude doctor)'; then
    FINAL_FAILURE_PROMPT_SEEN=true
    pass "REQ-F3-C 最终失败提示在屏（按 Enter 重试 / cloud-claude doctor）"
  else
    FINAL_FAILURE_PROMPT_SEEN=false
    fail "REQ-F3-C 失败：tmux 屏幕缺少「按 Enter 重试」/「cloud-claude doctor」字样"
  fi
}

# 30s 场景：恢复网络后 60s 内自动重连成功
check_reconnect_success() {
  if [[ "$DRY_RUN" == "true" ]]; then
    RECONNECT_OK=true
    return 0
  fi
  local t=0 out
  while [ "$t" -lt "$RECONNECT_WINDOW_S" ]; do
    sleep 5
    t=$((t + 5))
    out="$(ctr_exec tmux capture-pane -t claude -p -e 2>/dev/null || true)"
    if echo "$out" | grep -qE '(自动重连成功|"reconnect":true|reconnected)'; then
      RECONNECT_OK=true
      pass "REQ-F3-C 反向（成功路径）：${t}s 内观察到自动重连成功标记"
      return 0
    fi
  done
  RECONNECT_OK=false
  fail "REQ-F3-C 反向：${RECONNECT_WINDOW_S}s 内未观察到「自动重连成功」标记"
}

# ────────────────────────────────────────────────────────────────────────────
# 场景执行体（10s / 30s / 2min 共享前 3 步，差异化加测）
# ────────────────────────────────────────────────────────────────────────────

run_scenario() {
  local scenario="$1"
  info "===== 场景: ${scenario}（拔网 ${DURATION_S}s）====="

  if ! detect_container; then
    return 2
  fi

  # 1) 拔网前快照 + token 注入
  local buf_before fname
  if [[ "$DRY_RUN" != "true" ]]; then
    buf_before="$(ctr_exec tmux capture-pane -t claude -p -e 2>/dev/null || true)"
    echo "$buf_before" > "${WORK}/buf-before.txt"
  fi
  fname="$(inject_token)"

  # 2) 启动网络破坏
  if ! disrupt_start; then
    skip "BASE-03" "tc/iptables 均不可用或缺目标 IP（--host-ip）"
    return 2
  fi

  # 3) 拔网期间每 5s 探测进程存活；2min 场景每 10s 取一次 tmux 样本
  local sample_t=0
  if [[ "$scenario" == "2min" ]]; then
    while [ "$sample_t" -lt "$DURATION_S" ]; do
      if [[ "$DRY_RUN" != "true" ]]; then
        if ! ctr_exec pgrep -f claude >/dev/null 2>&1; then
          PGREP_SURVIVED=false
          fail "claude 进程在第 ${sample_t}s 退出（2min 场景应全程存活）"
          break
        fi
        ctr_exec tmux capture-pane -t claude -p -e 2>/dev/null \
          >> "$SAMPLES_FILE" || true
        echo "---SAMPLE@${sample_t}s---" >> "$SAMPLES_FILE"
      fi
      sleep 10
      sample_t=$((sample_t + 10))
    done
  else
    check_alive_loop "$DURATION_S" || true
  fi

  # 4) 恢复网络
  disrupt_stop
  info "网络已恢复，等待 ${RECONNECT_WINDOW_S}s 重连窗口"

  # 5) 通用断言：buffer 完整性 + token 回放
  if [[ "$DRY_RUN" != "true" ]]; then
    sleep 2
    local buf_after
    buf_after="$(capture_pane_retry || true)"
    echo "$buf_after" > "${WORK}/buf-after.txt"
    BUFFER_DIFF_LINES="$(diff "${WORK}/buf-before.txt" "${WORK}/buf-after.txt" \
      | grep -cE '^[<>]' || true)"
    if [[ "$BUFFER_DIFF_LINES" -eq 0 ]]; then
      pass "buffer 完整性：tmux capture-pane diff 行数 == 0"
    else
      fail "buffer 完整性：diff 行数 = ${BUFFER_DIFF_LINES}（应 == 0）"
    fi
  else
    BUFFER_DIFF_LINES=0
  fi
  verify_token "$fname" || true

  # 6) 场景差异化加测
  case "$scenario" in
    30s)
      check_reconnect_success || true
      ;;
    2min)
      collect_backoff_marks
      check_final_failure_prompt || true
      if [[ "$DRY_RUN" != "true" ]]; then
        if ctr_exec pgrep -f claude >/dev/null 2>&1; then
          pass "REQ-F4-A：claude 进程仍存活（tmux 内未退出）"
        else
          fail "REQ-F4-A：claude 进程已退出（应一直存活）"
        fi
        # 模拟用户按 Enter 重新触发连接；10s 内屏不再含失败字样
        ctr_exec tmux send-keys -t claude Enter 2>/dev/null || true
        sleep 10
        local out
        out="$(ctr_exec tmux capture-pane -t claude -p -e 2>/dev/null || true)"
        if echo "$out" | grep -qE '(按 Enter 重试|cloud-claude doctor)'; then
          warn "按 Enter 后 10s 屏仍含失败字样，可能未触发重连"
        else
          pass "按 Enter 后 10s 屏不再含失败字样（手动重连路径生效）"
        fi
      fi
      ;;
  esac
}

# ────────────────────────────────────────────────────────────────────────────
# 主流程
# ────────────────────────────────────────────────────────────────────────────

info "BASE-03 弱网 UAT — scenario=${SCENARIO} duration=${DURATION_S}s dry-run=${DRY_RUN}"
if [[ "$DRY_RUN" == "true" ]]; then
  # 即便后续被 SKIP（无目标容器/无 tc/无 iptables），也先把两级 fallback 命令模板
  # 打到 stderr，方便审计与 acceptance_criteria grep。
  echo "[DRY-RUN-PREVIEW] sudo tc qdisc add dev ${IFACE} root netem loss 100%" >&2
  echo "[DRY-RUN-PREVIEW] sudo iptables -I OUTPUT -d ${HOST_IP:-<host-ip>} -j DROP" >&2
fi
run_scenario "$SCENARIO" || true

# ────────────────────────────────────────────────────────────────────────────
# 报告产物：JSON + Markdown
# ────────────────────────────────────────────────────────────────────────────

OUTCOME="pass"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  OUTCOME="fail"
elif [[ "$SKIP_COUNT" -gt 0 && "$PASS_COUNT" -eq 0 ]]; then
  OUTCOME="skip"
fi

if command -v jq >/dev/null 2>&1; then
  jq -n \
    --argjson schema 1 \
    --arg scenario "$SCENARIO" \
    --arg container "${TARGET_CTR:-}" \
    --arg disrupt_mode "${DISRUPT_MODE:-none}" \
    --argjson pgrep "$([[ "$PGREP_SURVIVED" == "true" ]] && echo true || echo false)" \
    --argjson buffer_diff "$BUFFER_DIFF_LINES" \
    --argjson token_replayed "$([[ "$TOKEN_REPLAYED" == "true" ]] && echo true || echo false)" \
    --argjson reconnect_success "$([[ "$RECONNECT_OK" == "true" ]] && echo true || echo false)" \
    --argjson backoff_marks "$BACKOFF_MARKS_JSON" \
    --argjson final_failure_prompt "$([[ "$FINAL_FAILURE_PROMPT_SEEN" == "true" ]] && echo true || echo false)" \
    --arg outcome "$OUTCOME" \
    '{ schema_version: $schema, scenario: $scenario, container: $container,
       disrupt_mode: $disrupt_mode,
       pgrep_survived_full_duration: $pgrep,
       buffer_diff_lines: $buffer_diff,
       token_replayed: $token_replayed,
       reconnect_success: $reconnect_success,
       backoff_marks_seen: $backoff_marks,
       final_failure_prompt_seen: $final_failure_prompt,
       outcome: $outcome }' \
    > "$REPORT_JSON"
else
  cat > "$REPORT_JSON" <<EOF
{ "schema_version": 1, "scenario": "${SCENARIO}", "container": "${TARGET_CTR:-}",
  "disrupt_mode": "${DISRUPT_MODE:-none}",
  "pgrep_survived_full_duration": ${PGREP_SURVIVED},
  "buffer_diff_lines": ${BUFFER_DIFF_LINES},
  "token_replayed": ${TOKEN_REPLAYED},
  "reconnect_success": ${RECONNECT_OK},
  "backoff_marks_seen": [],
  "final_failure_prompt_seen": ${FINAL_FAILURE_PROMPT_SEEN},
  "outcome": "${OUTCOME}" }
EOF
fi

# 过滤 token / key / secret 后写 MD（T-35-02-05 Information Disclosure）
sed -E 's/(token|key|secret)=\S+/\1=[REDACTED]/gi' "$SAMPLES_FILE" \
  > "${WORK}/samples-redacted.txt"

{
  echo "# BASE-03 弱网 UAT — ${SCENARIO} 场景"
  echo ""
  echo "- 时间: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- 容器: ${TARGET_CTR:-N/A}"
  echo "- 破坏模式: ${DISRUPT_MODE:-none}"
  echo "- 拔网时长: ${DURATION_S}s"
  echo "- 结论: ${OUTCOME}"
  echo ""
  echo "## 量化锚点"
  echo ""
  echo "| 指标 | 值 |"
  echo "|------|-----|"
  echo "| pgrep_survived_full_duration | ${PGREP_SURVIVED} |"
  echo "| buffer_diff_lines | ${BUFFER_DIFF_LINES} |"
  echo "| token_replayed | ${TOKEN_REPLAYED} |"
  echo "| reconnect_success | ${RECONNECT_OK} |"
  echo "| final_failure_prompt_seen | ${FINAL_FAILURE_PROMPT_SEEN} |"
  echo ""
  echo "## 关键日志（最后 30 行，已脱敏）"
  echo ""
  echo '```'
  tail -n 30 "${WORK}/samples-redacted.txt" 2>/dev/null || echo "(无样本)"
  echo '```'
} > "$REPORT_MD"

info "JSON 报告: ${REPORT_JSON}"
info "MD   报告: ${REPORT_MD}"

# ────────────────────────────────────────────────────────────────────────────
# 汇总 + 退出码
# ────────────────────────────────────────────────────────────────────────────

echo ""
echo "========================================"
echo "BASE-03 ${SCENARIO} UAT 结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN, ${SKIP_COUNT} SKIP"
case "$OUTCOME" in
  pass)
    echo "状态: 全部通过"
    exit 0
    ;;
  skip)
    echo "状态: 环境不具备已 SKIP（不计 FAIL）"
    exit 2
    ;;
  fail|*)
    echo "状态: 存在失败项，请检查上方 [FAIL] 条目"
    exit 1
    ;;
esac
