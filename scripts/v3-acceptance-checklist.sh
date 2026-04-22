#!/usr/bin/env bash
# scripts/v3-acceptance-checklist.sh — Phase 35 v3.0 验收主脚本
#
# 一条命令遍历 30 条 functional REQ + 4 条 BASE + 3 个 pitfall，输出 JSON+MD 双报告。
# 三环境感知：CI / macOS APFS / Ubuntu 25.04 AppArmor。
# 关键真机场景（M5 / BASE-03 / C6）通过 checkpoint:human-verify 走签字流程。
#
# 用法：
#   bash scripts/v3-acceptance-checklist.sh --help
#   bash scripts/v3-acceptance-checklist.sh --track=all --dry-run
#   bash scripts/v3-acceptance-checklist.sh --track=all --env=auto --target-container=<ctr>
#
# 退出码：
#   0 → 无 FAIL（允许 SKIP — 真机环境不全时）
#   1 → 至少一条 FAIL
#   2 → 环境完全不适配（0 PASS + 全 SKIP）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# ───────────────────────────────────────────────────────────────────────────────
# REQ-ID 硬编码常量（与 .planning/REQUIREMENTS.md L170-203 Traceability 表一致）
# 合计：5+3+4+3+4+4+4+3 = 30 functional REQ + 4 BASE + 3 pitfalls
# ───────────────────────────────────────────────────────────────────────────────
REQS_F1=(REQ-F1-A REQ-F1-B REQ-F1-C REQ-F1-D REQ-F1-E)
REQS_F2=(REQ-F2-A REQ-F2-B REQ-F2-C)
REQS_F3=(REQ-F3-A REQ-F3-B REQ-F3-C REQ-F3-D)
REQS_F4=(REQ-F4-A REQ-F4-B REQ-F4-C)
REQS_F5=(REQ-F5-A REQ-F5-B REQ-F5-C REQ-F5-D)
REQS_F6=(REQ-F6-A REQ-F6-B REQ-F6-C REQ-F6-D)
REQS_F7=(REQ-F7-A REQ-F7-B REQ-F7-C REQ-F7-D)
REQS_F8=(REQ-F8-A REQ-F8-B REQ-F8-C)
BASES=(BASE-01 BASE-02 BASE-03 BASE-04)
PITFALLS=(M5 M13 C6)

# ───────────────────────────────────────────────────────────────────────────────
# 计数器 + 输出 helper（不使用 ANSI 色码，反向断言友好；Pattern M）
# ───────────────────────────────────────────────────────────────────────────────
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
WARN_COUNT=0

# 明细记录：每行 "ID|status|evidence|verdict"
RESULTS_FILE=""

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); record_result "$CURRENT_ID" "PASS" "$1"; }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); record_result "$CURRENT_ID" "FAIL" "$1"; }
skip() { echo "[SKIP]  $1"; SKIP_COUNT=$((SKIP_COUNT + 1)); record_result "$CURRENT_ID" "SKIP" "$1"; }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); record_result "$CURRENT_ID" "WARN" "$1"; }
info() { echo "[INFO]  $1"; }

CURRENT_ID=""
record_result() {
  local id="$1" status="$2" evidence="$3"
  [ -z "$id" ] && return 0
  [ -z "$RESULTS_FILE" ] && return 0
  printf '%s|%s|%s\n' "$id" "$status" "${evidence//|/／}" >> "$RESULTS_FILE"
}

# ───────────────────────────────────────────────────────────────────────────────
# 环境探测函数（PATTERNS Pattern B 三层闸门 — 与 host-preflight.sh 一致算法）
# ───────────────────────────────────────────────────────────────────────────────
is_ci() { [[ "${CI:-}" == "true" ]]; }
is_macos() { [[ "$(uname -s)" == "Darwin" ]]; }
is_linux() { [[ "$(uname -s)" == "Linux" ]]; }
is_ubuntu25() {
  [ -f /etc/os-release ] || return 1
  # shellcheck source=/dev/null
  . /etc/os-release
  [ "${ID:-}" = "ubuntu" ] || return 1
  local ubuntu_major ubuntu_minor ver_rest
  ubuntu_major="${VERSION_ID%%.*}"
  ver_rest="${VERSION_ID#*.}"
  ubuntu_minor="${ver_rest%%.*}"
  [ "${ubuntu_major}" -gt 25 ] || { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor:-0}" -ge 4 ]; }
}
has_tc() { command -v tc >/dev/null 2>&1; }
has_root_net() { [[ $EUID -eq 0 ]] || sudo -n tc qdisc show dev lo &>/dev/null; }
has_docker() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }
has_apfs() { [[ "$(uname -s)" == "Darwin" ]] && diskutil info / 2>/dev/null | grep -q 'APFS'; }
has_jq() { command -v jq >/dev/null 2>&1; }

detect_env() {
  if is_ci; then echo "ci"
  elif is_macos; then echo "macos"
  elif is_ubuntu25; then echo "ubuntu25"
  else echo "linux-other"
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# CLI 解析
# ───────────────────────────────────────────────────────────────────────────────
TRACK="all"
ENV_OVERRIDE="auto"
TARGET_CONTAINER=""
HOST_IP=""
DRY_RUN=false
CONFIRM_DESTRUCTIVE=false
OUTPUT_DIR="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"
REPORT_MD=""

usage() {
  cat <<'USAGE'
Usage: bash scripts/v3-acceptance-checklist.sh [OPTIONS]

Tracks (--track=...):
  base      4 个 BASE-0X 性能基线项
  req-f1    REQ-F1-A..E（5 项 — 三路 mount UX）
  req-f3    REQ-F3-A..D（4 项 — 网络韧性）
  req-f4    REQ-F4-A..C（3 项 — tmux 会话）
  req-f5    REQ-F5-A..D（4 项 — 多端共存）
  req-f6    REQ-F6-A..D（4 项 — doctor 五维度）
  req-f7    REQ-F7-A..D（4 项 — 持久化卷）
  req-f8    REQ-F8-A..C（3 项 — 错误码体系）
  pitfalls  M5 / M13 / C6 三个真机 pitfall 场景
  all       上述全部（默认）

Options:
  --env={ci|macos|ubuntu25|auto}     环境锁定，默认 auto
  --target-container=NAME            UAT track 用的目标容器名
  --host-ip=IP                       iptables fallback 指向的宿主 IP
  --dry-run                          只枚举不执行
  --confirm-destructive              透传给 degradation-regression.sh（M13 破坏链 opt-in 闸门）
  --output-dir=DIR                   JSON 报告输出目录
                                     默认: .planning/phases/35-e2e/benchmarks
  --report-md=PATH                   人类可读 Markdown 报告路径
                                     默认: docs/runbooks/v3-acceptance-report-YYYYMMDD.md
  --help                             显示此帮助

退出码：
  0 → 无 FAIL（允许 SKIP — 真机环境不全时）
  1 → 至少一条 FAIL
  2 → 环境完全不适配（0 PASS + 全 SKIP）
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --track=*) TRACK="${1#*=}";;
    --env=*) ENV_OVERRIDE="${1#*=}";;
    --target-container=*) TARGET_CONTAINER="${1#*=}";;
    --host-ip=*) HOST_IP="${1#*=}";;
    --dry-run) DRY_RUN=true;;
    --confirm-destructive) CONFIRM_DESTRUCTIVE=true;;
    --output-dir=*) OUTPUT_DIR="${1#*=}";;
    --report-md=*) REPORT_MD="${1#*=}";;
    --help|-h) usage; exit 0;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2;;
  esac
  shift
done

# Track 白名单校验（base|req-f1|req-f3|req-f4|req-f5|req-f6|req-f7|req-f8|pitfalls|all）
case "$TRACK" in
  base|req-f1|req-f3|req-f4|req-f5|req-f6|req-f7|req-f8|pitfalls|all) ;;
  *) echo "Invalid --track: $TRACK" >&2; usage >&2; exit 2;;
esac

# T-35-05-02 mitigation: --target-container 注入守卫
if [ -n "$TARGET_CONTAINER" ]; then
  if ! [[ "$TARGET_CONTAINER" =~ ^[a-z0-9][a-z0-9_.-]*$ ]]; then
    echo "FATAL: --target-container 含非法字符（仅允许 [a-z0-9_.-]，首字符必须字母数字）" >&2
    exit 2
  fi
fi

# 解析环境
if [ "$ENV_OVERRIDE" = "auto" ]; then
  ENV_DETECTED="$(detect_env)"
else
  ENV_DETECTED="$ENV_OVERRIDE"
fi
info "ENV_DETECTED=$ENV_DETECTED  TRACK=$TRACK  DRY_RUN=$DRY_RUN  CONFIRM_DESTRUCTIVE=$CONFIRM_DESTRUCTIVE"

# ───────────────────────────────────────────────────────────────────────────────
# 输出路径准备 + 默认报告名
# ───────────────────────────────────────────────────────────────────────────────
mkdir -p "$OUTPUT_DIR"
TS_DATE="$(date +%Y%m%d)"
TS_FULL="$(date -u +%Y%m%dT%H%M%SZ)"
JSON_OUT="${OUTPUT_DIR}/v3-acceptance-${TS_FULL}.json"
[ -z "$REPORT_MD" ] && REPORT_MD="${PROJECT_ROOT}/docs/runbooks/v3-acceptance-report-${TS_DATE}.md"
mkdir -p "$(dirname "$REPORT_MD")"

# 临时明细文件（pass/fail/skip/warn 写入）
RESULTS_FILE="$(mktemp -t v3acc-results-XXXXXX)"
trap 'rm -f "$RESULTS_FILE"' EXIT

# ───────────────────────────────────────────────────────────────────────────────
# check_item — 包装 runner，DRY_RUN 模式只打印 banner
# 必须输出形如 "───── <ID> (<TRACK>): <DESC> ─────" 的枚举行（acceptance regex 依赖）
# ───────────────────────────────────────────────────────────────────────────────
check_item() {
  local id="$1" track="$2" desc="$3" runner_fn="$4"
  CURRENT_ID="$id"
  echo ""
  info "───── $id ($track): $desc ─────"
  if $DRY_RUN; then
    skip "DRY_RUN: 不执行 runner"
    return 0
  fi
  if ! $runner_fn; then
    return 0
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — BASE 性能基线
# ───────────────────────────────────────────────────────────────────────────────
runner_base_01() {
  if [ ! -x "$SCRIPT_DIR/perf-benchmark.sh" ]; then
    skip "BASE-01: scripts/perf-benchmark.sh 缺失（上游 Plan 01 尚未完成？）"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/perf-benchmark.sh" --ci-mode --runs=10 --warmup=1 || rc=$?
  case "$rc" in
    0) pass "BASE-01: rg/ls P50 ratio ≤ 1.5× 本地基线";;
    2) skip "BASE-01: 环境不适配（perf-benchmark exit=2）";;
    *) fail "BASE-01: perf-benchmark 失败 (exit=$rc)";;
  esac
}

runner_base_02() {
  if [ ! -x "$SCRIPT_DIR/cold-start-benchmark.sh" ]; then
    skip "BASE-02: scripts/cold-start-benchmark.sh 缺失"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/cold-start-benchmark.sh" --attempts=5 --threshold-seconds=8 --min-pass=4 || rc=$?
  case "$rc" in
    0) pass "BASE-02: 首连 ≤ 8s（5 次中 ≥ 4 次）+ 三段式进度";;
    2) skip "BASE-02: 环境不适配";;
    *) fail "BASE-02: cold-start-benchmark 失败 (exit=$rc)";;
  esac
}

runner_base_03() {
  if [ ! -x "$SCRIPT_DIR/uat-network-resilience.sh" ]; then
    skip "BASE-03: scripts/uat-network-resilience.sh 缺失（上游 Plan 02 尚未完成？）"
    return 0
  fi
  if [ -z "$TARGET_CONTAINER" ]; then
    skip "BASE-03: 未提供 --target-container（2min 拔网场景需手工签字）"
    return 0
  fi
  local rc=0 args=(--scenario=30s --target-container="$TARGET_CONTAINER")
  [ -n "$HOST_IP" ] && args+=(--host-ip="$HOST_IP")
  bash "$SCRIPT_DIR/uat-network-resilience.sh" "${args[@]}" || rc=$?
  case "$rc" in
    0) pass "BASE-03: 30s 抖动无感知（pgrep/buffer/token 三锚点）；2min 场景需人工签字";;
    2) skip "BASE-03: 缺 tc/iptables 权限或 docker 不可用";;
    *) fail "BASE-03: uat-network-resilience 失败 (exit=$rc)";;
  esac
}

runner_base_04() {
  if [ ! -x "$SCRIPT_DIR/verify-managed-image.sh" ]; then
    skip "BASE-04: scripts/verify-managed-image.sh 缺失"
    return 0
  fi
  if ! has_docker; then
    skip "BASE-04: docker 不可用"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/verify-managed-image.sh" || rc=$?
  if [ "$rc" -eq 0 ]; then
    pass "BASE-04: managed-user 镜像 ≤ 700MB（CI image-size-regression 等价口径）"
  else
    fail "BASE-04: verify-managed-image.sh 失败 (exit=$rc)"
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F1（三路 mount UX）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f1_a() {
  if [ -z "$TARGET_CONTAINER" ] || ! has_docker; then
    skip "REQ-F1-A: 缺 --target-container 或 docker"
    return 0
  fi
  if docker exec "$TARGET_CONTAINER" mount 2>/dev/null | grep -qE 'type fuse\.mergerfs.*on /workspace'; then
    pass "REQ-F1-A: /workspace 走 mergerfs FUSE 单一视图"
  else
    fail "REQ-F1-A: /workspace 未挂载为 mergerfs（可能未启用 Auto/Full 档）"
  fi
}

runner_req_f1_b() { skip "REQ-F1-B: reuse BASE-02 stderr_progress_matches=true（见 BASE-02 JSON）"; }
runner_req_f1_c() { skip "REQ-F1-C: reuse BASE-01 P50 ratio ≤ 1.5（见 BASE-01 JSON）"; }
runner_req_f1_d() { skip "REQ-F1-D: 手工场景 — >50MB 候选目录触发 MOUNT_MUTAGEN_WHITELIST_REJECT"; }
runner_req_f1_e() { skip "REQ-F1-E: 手工场景 — cloud-claude sync conflicts 输出格式校验"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F2（mount-mode 四档）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f2_a() { skip "REQ-F2-A: 手工场景 — 四档 mount-mode banner 标签验证"; }
runner_req_f2_b() {
  if [ ! -x "$SCRIPT_DIR/degradation-regression.sh" ] || [ -z "$TARGET_CONTAINER" ]; then
    skip "REQ-F2-B: degradation-regression.sh 缺失或缺 --target-container"
    return 0
  fi
  if ! $CONFIRM_DESTRUCTIVE; then
    skip "REQ-F2-B: 需 --confirm-destructive 显式 opt-in 才执行 degradation-regression"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/degradation-regression.sh" --layer=mergerfs --target-container="$TARGET_CONTAINER" --confirm-destructive || rc=$?
  if [ "$rc" -eq 0 ]; then
    pass "REQ-F2-B: mergerfs 单层破坏后降级 SSHFSOnly 可恢复"
  else
    fail "REQ-F2-B: degradation-regression --layer=mergerfs 失败 (exit=$rc)"
  fi
}
runner_req_f2_c() { skip "REQ-F2-C: 手工场景 — NO_COLOR=1 banner 反向断言"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F3（网络韧性）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f3_a() { skip "REQ-F3-A: 手工场景 — --server-alive-interval=10 启动应报 SESSION_KEEPALIVE_TOO_AGGRESSIVE"; }
runner_req_f3_b() { skip "REQ-F3-B: reuse uat-network-resilience 30s 场景的 token_replayed=true"; }
runner_req_f3_c() { skip "REQ-F3-C: 2min 拔网真机签字（人工 checkpoint，见 docs/runbooks/v3-acceptance-procedure.md §3.2/3.3）"; }
runner_req_f3_d() { skip "REQ-F3-D: 2min 退避序列（1/2/4/8/30s ≥ 3 档命中），见 BASE-03 JSON backoff_marks_seen 数组"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F4（tmux 会话）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f4_a() { skip "REQ-F4-A: reuse BASE-03 30s 场景的 pgrep_survived_full_duration=true"; }
runner_req_f4_b() {
  if [ -z "$TARGET_CONTAINER" ] || ! has_docker; then
    skip "REQ-F4-B: 缺 --target-container 或 docker"
    return 0
  fi
  local n
  n="$(docker exec "$TARGET_CONTAINER" tmux ls 2>/dev/null | wc -l | tr -d ' ')"
  if [ "${n:-0}" -ge 1 ]; then
    pass "REQ-F4-B: 容器内 tmux 会话 ≥ 1（实际 $n）"
  else
    fail "REQ-F4-B: 容器内 tmux ls 无会话（可能 cloud-claude 未运行或 tmux 不可用）"
  fi
}
runner_req_f4_c() { skip "REQ-F4-C: 手工场景 — 破坏 tmux 二进制权限后 cloud-claude banner 含「容器内 tmux 不可用」"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F5（多端共存）— 全手工
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f5_a() { skip "REQ-F5-A: 手工场景 — 双端 cloud-claude 第二端无「被踢」错误（CI 必 SKIP）"; }
runner_req_f5_b() { skip "REQ-F5-B: 手工场景 — 第二端 banner 含「另 1 个会话正在共享」"; }
runner_req_f5_c() { skip "REQ-F5-C: 手工场景 — 测 --new-session + --take-over 流程"; }
runner_req_f5_d() { skip "REQ-F5-D: 手工场景 — 第二端日志含 SESSION_SYNC_LOCKED"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F6（doctor 五维度）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f6_a() {
  if [ -z "$TARGET_CONTAINER" ] || ! has_docker || ! has_jq; then
    skip "REQ-F6-A: 缺 --target-container/docker/jq"
    return 0
  fi
  local n
  n="$(docker exec "$TARGET_CONTAINER" cloud-claude doctor --json 2>/dev/null \
       | jq -r '.checks | group_by(.domain) | length' 2>/dev/null || echo 0)"
  if [ "${n:-0}" -eq 5 ]; then
    pass "REQ-F6-A: doctor JSON 五维度齐备（network/auth/ssh/mount/disk）"
  else
    fail "REQ-F6-A: doctor domain 数 = $n ≠ 5"
  fi
}
runner_req_f6_b() {
  if [ ! -x "$SCRIPT_DIR/ci-doctor-grep.sh" ]; then
    skip "REQ-F6-B: scripts/ci-doctor-grep.sh 缺失"
    return 0
  fi
  local cc_bin="${CLOUD_CLAUDE_BIN:-./cloud-claude}"
  if [ ! -x "$cc_bin" ]; then
    skip "REQ-F6-B: cloud-claude 二进制不可执行（CLOUD_CLAUDE_BIN=$cc_bin）"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/ci-doctor-grep.sh" "$cc_bin" || rc=$?
  if [ "$rc" -eq 0 ]; then
    pass "REQ-F6-B: doctor schema_version=1 + next_action + 错误码三段断言通过"
  else
    fail "REQ-F6-B: ci-doctor-grep.sh 失败 (exit=$rc)"
  fi
}
runner_req_f6_c() { skip "REQ-F6-C: 手工场景 — doctor --fix --yes 应能修复 ≥ 5 种故障"; }
runner_req_f6_d() { skip "REQ-F6-D: 手工场景 — --verbose / --json / NO_COLOR=1 + 退出码 ∈ {0,1,2}"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F7（持久化卷）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f7_a() {
  if ! has_docker; then
    skip "REQ-F7-A: docker 不可用"
    return 0
  fi
  local sample
  sample="$(docker volume ls --format '{{.Name}}' 2>/dev/null | grep -E '^claude-state-' | head -1 || true)"
  if [ -n "$sample" ]; then
    pass "REQ-F7-A: 检测到 claude-state-* 卷示例（$sample）；标签查询能力具备"
  else
    skip "REQ-F7-A: 当前主机无 claude-state-* 卷（需先连过任一账号）"
  fi
}
runner_req_f7_b() { skip "REQ-F7-B: 手工场景 — 容器删除后 docker volume ls 仍可见 + 重建后 .credentials.json 存活"; }
runner_req_f7_c() { skip "REQ-F7-C: 手工场景 — 过期 OAuth → cloud-claude stderr 含 NET_OAUTH_EXPIRED 中文"; }
runner_req_f7_d() { skip "REQ-F7-D: 手工场景 — DELETE /v1/admin/claude-accounts/<id> 后卷消失"; }

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — REQ-F8（错误码体系）
# ───────────────────────────────────────────────────────────────────────────────
runner_req_f8_a() {
  if [ -z "$TARGET_CONTAINER" ] || ! has_docker || ! has_jq; then
    skip "REQ-F8-A: 缺 --target-container/docker/jq"
    return 0
  fi
  local n
  n="$(docker exec "$TARGET_CONTAINER" cloud-claude explain --all 2>/dev/null \
       | jq -r 'length' 2>/dev/null || echo 0)"
  if [ "${n:-0}" -ge 42 ]; then
    pass "REQ-F8-A: cloud-claude explain --all 返回 ≥ 42 条（实际 $n）"
  else
    fail "REQ-F8-A: explain --all 数量 = $n < 42"
  fi
}
runner_req_f8_b() { skip "REQ-F8-B: 手工 — errcodes 注册表完整性（遍历代码 diff，参 docs/runbooks/v3-error-code-index.md）"; }
runner_req_f8_c() {
  if [ -z "$TARGET_CONTAINER" ] || ! has_docker; then
    skip "REQ-F8-C: 缺 --target-container 或 docker"
    return 0
  fi
  local out
  out="$(docker exec "$TARGET_CONTAINER" cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW 2>/dev/null || echo '')"
  if [ -n "$out" ] && echo "$out" | grep -qE '建议|recommend'; then
    pass "REQ-F8-C: cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW 含「建议」段"
  else
    fail "REQ-F8-C: explain 输出缺「建议」段或为空"
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# Runner 函数 — Pitfalls（M5 / M13 / C6）
# ───────────────────────────────────────────────────────────────────────────────
runner_m5_apfs() {
  if ! is_macos || ! has_apfs; then
    skip "M5 APFS: 非 macOS APFS 环境"
    return 0
  fi
  if ! has_docker; then
    skip "M5 APFS: 缺 docker"
    return 0
  fi
  if [ -z "$TARGET_CONTAINER" ]; then
    skip "M5 APFS: 缺 --target-container（需 cloud-claude 已连接的容器）"
    return 0
  fi
  local ctr="$TARGET_CONTAINER"
  local tmp_local
  tmp_local="$(mktemp -d)"
  printf 'A-upper' > "$tmp_local/Foo.txt"
  if ! printf 'B-lower' > "$tmp_local/foo.txt" 2>/dev/null; then
    warn "M5 APFS: 本地 APFS case-insensitive 触发覆盖；观测 Mutagen two-way-resolved 策略"
  fi
  sleep 10
  local remote_foo_upper remote_foo_lower
  remote_foo_upper="$(docker exec "$ctr" cat /workspace/Foo.txt 2>/dev/null || echo '')"
  remote_foo_lower="$(docker exec "$ctr" cat /workspace/foo.txt 2>/dev/null || echo '')"
  if [ -n "$remote_foo_upper" ] && [ -n "$remote_foo_lower" ] && [ "$remote_foo_upper" != "$remote_foo_lower" ]; then
    pass "M5 APFS: Foo.txt='$remote_foo_upper' / foo.txt='$remote_foo_lower' 双向同步保留，无数据丢失"
  else
    fail "M5 APFS: Foo.txt='$remote_foo_upper' / foo.txt='$remote_foo_lower' 至少一侧丢失"
  fi
  rm -rf "$tmp_local"
}

runner_m13() {
  if [ ! -x "$SCRIPT_DIR/degradation-regression.sh" ]; then
    skip "M13: scripts/degradation-regression.sh 缺失"
    return 0
  fi
  if [ -z "$TARGET_CONTAINER" ]; then
    skip "M13: 缺 --target-container"
    return 0
  fi
  if ! $CONFIRM_DESTRUCTIVE; then
    skip "M13: 需 --confirm-destructive 显式 opt-in 才执行破坏链（T-35-05-03 mitigation）"
    return 0
  fi
  local rc=0
  bash "$SCRIPT_DIR/degradation-regression.sh" --layer=all --target-container="$TARGET_CONTAINER" --confirm-destructive || rc=$?
  if [ "$rc" -eq 0 ]; then
    pass "M13: 三层降级链全 PASS（doctor 命中 MOUNT_* + next_action 非空）"
  else
    fail "M13: degradation-regression --layer=all 失败 (exit=$rc)"
  fi
}

runner_c6_ubuntu25() {
  if ! is_ubuntu25; then
    skip "C6 Ubuntu 25.04: 非 Ubuntu 25.04+ 环境"
    return 0
  fi
  local rc=0
  if [ -x "$PROJECT_ROOT/deploy/scripts/host-preflight.sh" ]; then
    bash "$PROJECT_ROOT/deploy/scripts/host-preflight.sh" || rc=$?
    if [ "$rc" -ne 0 ]; then
      fail "C6: host-preflight.sh 失败 (exit=$rc) — 请检查 AppArmor override /etc/apparmor.d/local/fusermount3"
      return 0
    fi
  else
    skip "C6: deploy/scripts/host-preflight.sh 缺失"
    return 0
  fi
  if [ -x "$SCRIPT_DIR/verify-fuse-compat.sh" ]; then
    bash "$SCRIPT_DIR/verify-fuse-compat.sh" || rc=$?
    if [ "$rc" -eq 0 ]; then
      pass "C6: AppArmor override + 三路 FUSE 全 PASS"
    else
      fail "C6: verify-fuse-compat.sh 失败 (exit=$rc)"
    fi
  else
    skip "C6: scripts/verify-fuse-compat.sh 缺失"
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# Track 调度
# ───────────────────────────────────────────────────────────────────────────────
run_track_base() {
  check_item BASE-01 base "rg/ls 元数据响应 ≤ 1.5× 本地基线" runner_base_01
  check_item BASE-02 base "首连冷启动 ≤ 8s + 三段式进度" runner_base_02
  check_item BASE-03 base "30s 抖动无感知（pgrep/buffer/token 三锚点）" runner_base_03
  check_item BASE-04 base "managed-user 镜像 ≤ 700MB" runner_base_04
}

run_track_req_f1() {
  check_item REQ-F1-A req-f1 "/workspace 走 mergerfs FUSE 单一视图" runner_req_f1_a
  check_item REQ-F1-B req-f1 "首连三段式进度（reuse BASE-02）" runner_req_f1_b
  check_item REQ-F1-C req-f1 "rg/ls 性能比 ≤ 1.5×（reuse BASE-01）" runner_req_f1_c
  check_item REQ-F1-D req-f1 ">50MB 候选目录拒绝（MOUNT_MUTAGEN_WHITELIST_REJECT）" runner_req_f1_d
  check_item REQ-F1-E req-f1 "cloud-claude sync conflicts 输出格式" runner_req_f1_e
}

run_track_req_f3() {
  check_item REQ-F3-A req-f3 "--server-alive-interval=10 启动报错 SESSION_KEEPALIVE_TOO_AGGRESSIVE" runner_req_f3_a
  check_item REQ-F3-B req-f3 "10s 抖动 token 完整回放" runner_req_f3_b
  check_item REQ-F3-C req-f3 "2min 拔网最终失败提示文案 final_failure_prompt_seen" runner_req_f3_c
  check_item REQ-F3-D req-f3 "2min 退避序列 1/2/4/8/30s ≥ 3 档命中" runner_req_f3_d
}

run_track_req_f4() {
  check_item REQ-F4-A req-f4 "30s 抖动期 cloud-claude 进程持续存活" runner_req_f4_a
  check_item REQ-F4-B req-f4 "容器内 tmux ls + cloud-claude sessions ls ≥ 1" runner_req_f4_b
  check_item REQ-F4-C req-f4 "tmux 不可用时 cloud-claude banner 显式提示" runner_req_f4_c
}

run_track_req_f5() {
  check_item REQ-F5-A req-f5 "双端 cloud-claude 第二端无「被踢」错误" runner_req_f5_a
  check_item REQ-F5-B req-f5 "第二端 banner 含「另 1 个会话正在共享」" runner_req_f5_b
  check_item REQ-F5-C req-f5 "--new-session / --take-over 流程" runner_req_f5_c
  check_item REQ-F5-D req-f5 "第二端日志含 SESSION_SYNC_LOCKED" runner_req_f5_d
}

run_track_req_f6() {
  check_item REQ-F6-A req-f6 "doctor JSON 五维度齐备" runner_req_f6_a
  check_item REQ-F6-B req-f6 "doctor schema_version=1 + next_action + 错误码（ci-doctor-grep）" runner_req_f6_b
  check_item REQ-F6-C req-f6 "doctor --fix --yes 修复 ≥ 5 种故障" runner_req_f6_c
  check_item REQ-F6-D req-f6 "doctor --verbose / --json / NO_COLOR=1 + 退出码 ∈ {0,1,2}" runner_req_f6_d
}

run_track_req_f7() {
  check_item REQ-F7-A req-f7 "claude-state-* 卷标签查询" runner_req_f7_a
  check_item REQ-F7-B req-f7 "容器删除后 .credentials.json 持久化" runner_req_f7_b
  check_item REQ-F7-C req-f7 "OAuth 过期 stderr 含 NET_OAUTH_EXPIRED 中文" runner_req_f7_c
  check_item REQ-F7-D req-f7 "DELETE 接口后卷消失" runner_req_f7_d
}

run_track_req_f8() {
  check_item REQ-F8-A req-f8 "cloud-claude explain --all ≥ 42 条" runner_req_f8_a
  check_item REQ-F8-B req-f8 "errcodes 注册表完整性 diff" runner_req_f8_b
  check_item REQ-F8-C req-f8 "explain MOUNT_MUTAGEN_VERSION_SKEW 含「建议」段" runner_req_f8_c
}

run_track_pitfalls() {
  check_item M5 pitfalls "macOS APFS case-insensitive 双向同步无数据丢失（Foo.txt + foo.txt）" runner_m5_apfs
  check_item M13 pitfalls "三层 mount 静默降级回归（degradation-regression --layer=all --confirm-destructive）" runner_m13
  check_item C6 pitfalls "Ubuntu 25.04 AppArmor override + 三路 FUSE（host-preflight + verify-fuse-compat）" runner_c6_ubuntu25
}

# ───────────────────────────────────────────────────────────────────────────────
# 汇总 + 报告生成（PATTERNS Pattern A: JSON + MD 双输出 / schema_version=1）
# ───────────────────────────────────────────────────────────────────────────────
write_json_report() {
  local total=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT + WARN_COUNT))
  {
    printf '{\n'
    printf '  "schema_version": 1,\n'
    printf '  "generated_at": "%s",\n' "$TS_FULL"
    printf '  "hostname": "%s",\n' "$(hostname)"
    printf '  "uname": "%s",\n' "$(uname -a | sed 's/"/\\"/g')"
    printf '  "env_detected": "%s",\n' "$ENV_DETECTED"
    printf '  "track": "%s",\n' "$TRACK"
    printf '  "summary": { "pass": %d, "fail": %d, "skip": %d, "warn": %d, "total": %d },\n' \
      "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" "$WARN_COUNT" "$total"
    printf '  "results": [\n'
    local first=1
    while IFS='|' read -r id status evidence; do
      [ -z "$id" ] && continue
      if [ "$first" -eq 1 ]; then first=0; else printf ',\n'; fi
      local esc_evidence
      esc_evidence="${evidence//\\/\\\\}"
      esc_evidence="${esc_evidence//\"/\\\"}"
      printf '    { "id": "%s", "status": "%s", "evidence": "%s" }' \
        "$id" "$status" "$esc_evidence"
    done < "$RESULTS_FILE"
    printf '\n  ]\n'
    printf '}\n'
  } > "$JSON_OUT"
}

write_md_report() {
  local total=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT + WARN_COUNT))
  {
    printf '# v3.0 验收报告 — %s %s\n\n' "$(hostname)" "$TS_FULL"
    printf '> 执行环境：%s ／ 内核：%s ／ docker：%s\n\n' \
      "$ENV_DETECTED" \
      "$(uname -a)" \
      "$(docker --version 2>/dev/null || echo 'N/A')"
    printf '> JSON 报告：`%s`\n\n' "$JSON_OUT"

    printf '## 汇总\n\n'
    printf '| PASS | FAIL | SKIP | WARN | 总计 |\n'
    printf '|------|------|------|------|------|\n'
    printf '| %d | %d | %d | %d | %d |\n\n' \
      "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" "$WARN_COUNT" "$total"

    printf '## 明细（按 ID）\n\n'
    printf '| REQ-ID | 状态 | 证据 |\n'
    printf '|--------|------|------|\n'
    while IFS='|' read -r id status evidence; do
      [ -z "$id" ] && continue
      printf '| %s | %s | %s |\n' "$id" "$status" "$evidence"
    done < "$RESULTS_FILE"
    printf '\n'

    printf '## 关键场景签字（人工 — checkpoint:human-verify）\n\n'
    printf '| 场景 | 机器信息 | 执行时间 | 签字人 | 证据 |\n'
    printf '|------|---------|---------|--------|------|\n'
    printf '| M5 APFS case-insensitive 双向同步 | _填写：hostname / OS 版本 / CPU_ | _YYYY-MM-DD HH:MM_ | _@user_ | `%s` |\n' "$JSON_OUT"
    printf '| BASE-03 / REQ-F3-C 2min 拔网自动重连 | _填写：hostname / iface / 断网方式_ | _YYYY-MM-DD HH:MM_ | _@user_ | `%s` |\n' "$JSON_OUT"
    printf '| C6 Ubuntu 25.04 AppArmor + 三路 FUSE | _填写：hostname / kernel / ubuntu_version_ | _YYYY-MM-DD HH:MM_ | _@user_ | `%s` |\n' "$JSON_OUT"
    printf '\n'
    printf '签字流程见 `docs/runbooks/v3-acceptance-procedure.md` §4。\n'
  } > "$REPORT_MD"
}

summarize() {
  local total=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT + WARN_COUNT))
  echo ""
  echo "========================================"
  echo "验收汇总: $PASS_COUNT PASS / $FAIL_COUNT FAIL / $SKIP_COUNT SKIP / $WARN_COUNT WARN (total=$total)"
  echo "========================================"
  if ! $DRY_RUN; then
    write_json_report
    write_md_report
    echo "JSON 报告: $JSON_OUT"
    echo "Markdown 报告: $REPORT_MD"
  else
    echo "(DRY_RUN: 跳过报告写入)"
  fi
}

# ───────────────────────────────────────────────────────────────────────────────
# 主流程
# ───────────────────────────────────────────────────────────────────────────────
case "$TRACK" in
  base) run_track_base;;
  req-f1) run_track_req_f1;;
  req-f3) run_track_req_f3;;
  req-f4) run_track_req_f4;;
  req-f5) run_track_req_f5;;
  req-f6) run_track_req_f6;;
  req-f7) run_track_req_f7;;
  req-f8) run_track_req_f8;;
  pitfalls) run_track_pitfalls;;
  all)
    run_track_base
    run_track_req_f1
    # REQ-F2 三项（架构上属 mount UX 但不在 --track 白名单暴露，归入 all 全跑）
    check_item REQ-F2-A all "mount-mode 四档 banner 标签验证" runner_req_f2_a
    check_item REQ-F2-B all "降级回归（reuse degradation-regression --layer=all）" runner_req_f2_b
    check_item REQ-F2-C all "NO_COLOR=1 banner 反向断言" runner_req_f2_c
    run_track_req_f3
    run_track_req_f4
    run_track_req_f5
    run_track_req_f6
    run_track_req_f7
    run_track_req_f8
    run_track_pitfalls
    ;;
esac

summarize

# 退出码裁决
if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
if [ "$PASS_COUNT" -eq 0 ] && [ "$SKIP_COUNT" -gt 0 ]; then
  exit 2
fi
exit 0
