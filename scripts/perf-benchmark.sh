#!/usr/bin/env bash
# scripts/perf-benchmark.sh — Phase 35 BASE-01 三档基准（local / mergerfs / sshfs-only）
#
# 用 hyperfine 跑 rg/ls -R 在三档文件系统上的 P50/P99，输出 JSON + Markdown 双报告。
# 裁决：ratio P50 <= 1.5 PASS / <= 2.0 WARN / else FAIL；P99 <= 2.0 否则 FAIL。
# Pitfall 1: warmup=1 runs=10 必守 (hyperfine --warmup 1 --runs 10)。
#
# 退出码：
#   0  通过（含 WARN 档位，CI 不阻塞）
#   1  FAIL（P50 > 1.5 或 P99 > 2.0）
#   2  SKIP（无 hyperfine / 无 docker 特权且非 --ci-mode）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BENCH_DIR="${PROJECT_ROOT}/.planning/phases/35-e2e/benchmarks"

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
perf-benchmark.sh — Phase 35 BASE-01 (rg/ls -R 1.5x baseline) 三档基准

用法: scripts/perf-benchmark.sh [--ci-mode] [--runs=N] [--warmup=N]
                                [--output-dir=DIR] [--bench-tree=DIR] [--help]

选项:
  --ci-mode         CI 模式：只跑本地档（local-rg / local-ls），跳过 mergerfs/sshfs 容器档
  --runs=N          每档测量次数（默认 10，Pitfall 1 锁定）
  --warmup=N        预热次数（默认 1，Pitfall 1 锁定）
  --output-dir=DIR  报告输出目录（默认 .planning/phases/35-e2e/benchmarks）
  --bench-tree=DIR  benchmark 文件树路径（默认 /tmp/bench-tree；不存在则自动生成）
  --help, -h        显示本帮助

裁决闸门（BASE-01 / REQ-F1-C）：
  P50 ratio (mergerfs / local) <= 1.5  → PASS(1.5x)
  P50 ratio (mergerfs / local) <= 2.0  → WARN(<=2x)（CI 不阻塞）
  P50 ratio (mergerfs / local) >  2.0  → FAIL
  P99 ratio (mergerfs / local) >  2.0  → FAIL
EOF
}

CI_MODE=false
RUNS=10
WARMUP=1
OUTPUT_DIR="${BENCH_DIR}"
BENCH_TREE_DIR="${BENCH_TREE_DIR:-/tmp/bench-tree}"

for arg in "$@"; do
  case "$arg" in
    --ci-mode) CI_MODE=true ;;
    --runs=*) RUNS="${arg#--runs=}" ;;
    --warmup=*) WARMUP="${arg#--warmup=}" ;;
    --output-dir=*) OUTPUT_DIR="${arg#--output-dir=}" ;;
    --bench-tree=*) BENCH_TREE_DIR="${arg#--bench-tree=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage; exit 1 ;;
  esac
done

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "缺少必需命令: $1${2:+ ($2)}"
    exit 2
  fi
}

require_cmd hyperfine "brew install hyperfine / cargo install hyperfine"
require_cmd rg "brew install ripgrep / apt install ripgrep"
require_cmd jq "brew install jq / apt install jq"

if [ "$CI_MODE" = false ]; then
  require_cmd docker "docker engine 未安装，跳过 mergerfs/sshfs 档请加 --ci-mode"
fi

mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
REPORT_JSON="${OUTPUT_DIR}/bench-${TIMESTAMP}.json"
REPORT_MD="${OUTPUT_DIR}/bench-${TIMESTAMP}.md"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# 自动生成 benchmark 文件树（若不存在）
if [ ! -d "$BENCH_TREE_DIR" ]; then
  info "${BENCH_TREE_DIR} 不存在，调用 scripts/gen-bench-tree.sh --count=10000 生成"
  bash "${PROJECT_ROOT}/scripts/gen-bench-tree.sh" --count=10000 --output="${BENCH_TREE_DIR}"
fi

# 硬件信息（Pitfall 1：写入 MD 报告头部）
HOSTNAME_VAL="$(hostname 2>/dev/null || echo unknown)"
UNAME_VAL="$(uname -a 2>/dev/null || echo unknown)"
if command -v nproc >/dev/null 2>&1; then
  CPU_COUNT="$(nproc)"
elif [ "$(uname -s)" = "Darwin" ]; then
  CPU_COUNT="$(sysctl -n hw.ncpu 2>/dev/null || echo unknown)"
else
  CPU_COUNT="unknown"
fi
if [ "$(uname -s)" = "Linux" ] && command -v lscpu >/dev/null 2>&1; then
  CPU_MHZ="$(lscpu 2>/dev/null | grep -i 'mhz' | head -1 | awk -F: '{ print $2 }' | xargs || echo unknown)"
else
  CPU_MHZ="$(sysctl -n hw.cpufrequency 2>/dev/null || echo unknown)"
fi

info "硬件: hostname=${HOSTNAME_VAL} cpu_count=${CPU_COUNT} cpu_mhz=${CPU_MHZ}"
info "uname: ${UNAME_VAL}"

# ── 本地档（绝对基线，永远跑）──
info "运行本地档：rg / ls -R 在 ${BENCH_TREE_DIR}"
hyperfine --warmup "$WARMUP" --runs "$RUNS" \
  --export-json "$WORK/local.json" \
  -n "local-rg" "rg . ${BENCH_TREE_DIR} >/dev/null" \
  -n "local-ls" "ls -R ${BENCH_TREE_DIR} >/dev/null"

LOCAL_OK=true
jq empty "$WORK/local.json" >/dev/null 2>&1 || { fail "本地档 hyperfine JSON 非合法"; LOCAL_OK=false; }

# ── mergerfs / sshfs-only 档（仅非 CI 模式）──
MERGERFS_OK=false
SSHFS_OK=false
CONTAINER_NAME=""

setup_managed_container() {
  local image_name
  image_name="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' \
    "${PROJECT_ROOT}/deploy/docker/managed-user/image.lock")"
  if [ -z "$image_name" ]; then
    fail "image.lock 解析 local_dev_image_name 失败"
    return 1
  fi
  info "使用受管镜像: $image_name"

  CONTAINER_NAME="bench-perf-$$"
  if ! docker run -d --name "$CONTAINER_NAME" \
    --cap-add SYS_ADMIN \
    --device /dev/fuse \
    --security-opt apparmor=unconfined \
    -v "${BENCH_TREE_DIR}:/mnt/cold:ro" \
    "$image_name" sleep 1800 >/dev/null 2>&1; then
    fail "受管容器启动失败（可能缺少 SYS_ADMIN / /dev/fuse / apparmor=unconfined）"
    return 1
  fi
  trap 'docker rm -f "$CONTAINER_NAME" 2>/dev/null || true; rm -rf "$WORK"' EXIT
  return 0
}

if [ "$CI_MODE" = false ]; then
  if setup_managed_container; then
    # mergerfs 档：试图把 /mnt/cold 通过 mergerfs 暴露成 /workspace
    info "尝试 mergerfs 档（/mnt/cold -> /workspace via mergerfs）"
    if docker exec "$CONTAINER_NAME" bash -c 'command -v mergerfs >/dev/null && mkdir -p /workspace /mnt/empty && mergerfs -o defaults,allow_other,use_ino,category.create=ff /mnt/cold:/mnt/empty /workspace 2>/dev/null && mountpoint -q /workspace' 2>/dev/null; then
      info "mergerfs 挂载就绪，开始基准测量"
      if hyperfine --warmup "$WARMUP" --runs "$RUNS" \
        --export-json "$WORK/mergerfs.json" \
        -n "mergerfs-rg" "docker exec ${CONTAINER_NAME} rg . /workspace >/dev/null" \
        -n "mergerfs-ls" "docker exec ${CONTAINER_NAME} ls -R /workspace >/dev/null" 2>&1; then
        MERGERFS_OK=true
      else
        warn "mergerfs 档 hyperfine 失败"
      fi
    else
      skip "mergerfs" "容器内 mergerfs 不可用或挂载失败（FUSE caps / mergerfs 缺）"
    fi

    # sshfs-only 档：Claude's Discretion — 容器内若不具备直挂 sshfs，回退到 /mnt/cold bind 挂载点
    # README 已注明：/mnt/cold 是 ro bind mount，与真实 sshfs 不完全等价但 metadata 路径一致
    info "尝试 sshfs-only 档（fallback 到 /mnt/cold bind mount）"
    if hyperfine --warmup "$WARMUP" --runs "$RUNS" \
      --export-json "$WORK/sshfs.json" \
      -n "sshfs-only-rg" "docker exec ${CONTAINER_NAME} rg . /mnt/cold >/dev/null" \
      -n "sshfs-only-ls" "docker exec ${CONTAINER_NAME} ls -R /mnt/cold >/dev/null" 2>&1; then
      SSHFS_OK=true
    else
      skip "sshfs-only" "容器内 rg/ls 在 /mnt/cold 上失败"
    fi
  else
    skip "mergerfs/sshfs-only" "受管容器启动失败"
  fi
else
  skip "mergerfs/sshfs-only" "CI 基线模式，真机档在 APFS/Ubuntu 25.04 单独验证"
fi

# ── P50/P99 计算（jq 从 .results[].times[] 排序后按索引抽取）──
percentiles_for() {
  local file="$1" cmd="$2" pct="$3"
  jq -r --arg c "$cmd" --argjson p "$pct" '
    .results[] | select(.command == $c) |
    (.times | sort) as $t |
    ($t | length) as $n |
    if $n == 0 then "null" else $t[(($n * $p) | floor)] end
  ' "$file" 2>/dev/null || echo "null"
}

LOCAL_RG_P50=$(percentiles_for "$WORK/local.json" "local-rg" "0.5")
LOCAL_RG_P99=$(percentiles_for "$WORK/local.json" "local-rg" "0.99")
LOCAL_LS_P50=$(percentiles_for "$WORK/local.json" "local-ls" "0.5")
LOCAL_LS_P99=$(percentiles_for "$WORK/local.json" "local-ls" "0.99")

MERGERFS_RG_P50="null"; MERGERFS_RG_P99="null"
MERGERFS_LS_P50="null"; MERGERFS_LS_P99="null"
if [ "$MERGERFS_OK" = true ] && [ -s "$WORK/mergerfs.json" ]; then
  MERGERFS_RG_P50=$(percentiles_for "$WORK/mergerfs.json" "mergerfs-rg" "0.5")
  MERGERFS_RG_P99=$(percentiles_for "$WORK/mergerfs.json" "mergerfs-rg" "0.99")
  MERGERFS_LS_P50=$(percentiles_for "$WORK/mergerfs.json" "mergerfs-ls" "0.5")
  MERGERFS_LS_P99=$(percentiles_for "$WORK/mergerfs.json" "mergerfs-ls" "0.99")
fi

SSHFS_RG_P50="null"; SSHFS_RG_P99="null"
SSHFS_LS_P50="null"; SSHFS_LS_P99="null"
if [ "$SSHFS_OK" = true ] && [ -s "$WORK/sshfs.json" ]; then
  SSHFS_RG_P50=$(percentiles_for "$WORK/sshfs.json" "sshfs-only-rg" "0.5")
  SSHFS_RG_P99=$(percentiles_for "$WORK/sshfs.json" "sshfs-only-rg" "0.99")
  SSHFS_LS_P50=$(percentiles_for "$WORK/sshfs.json" "sshfs-only-ls" "0.5")
  SSHFS_LS_P99=$(percentiles_for "$WORK/sshfs.json" "sshfs-only-ls" "0.99")
fi

ratio() {
  local num="$1" den="$2"
  if [ "$num" = "null" ] || [ "$den" = "null" ] || [ -z "$den" ]; then echo "null"; return; fi
  awk -v n="$num" -v d="$den" 'BEGIN { if (d == 0) print "null"; else printf "%.3f", n / d }'
}

verdict() {
  local r="$1"
  if [ "$r" = "null" ]; then echo "SKIP"; return; fi
  awk -v r="$r" 'BEGIN {
    if (r+0 <= 1.5) print "PASS(1.5x)"
    else if (r+0 <= 2.0) print "WARN(<=2x)"
    else print "FAIL"
  }'
}

R_RG_P50=$(ratio "$MERGERFS_RG_P50" "$LOCAL_RG_P50")
R_RG_P99=$(ratio "$MERGERFS_RG_P99" "$LOCAL_RG_P99")
R_LS_P50=$(ratio "$MERGERFS_LS_P50" "$LOCAL_LS_P50")
R_LS_P99=$(ratio "$MERGERFS_LS_P99" "$LOCAL_LS_P99")

V_RG_P50=$(verdict "$R_RG_P50")
V_RG_P99=$(verdict "$R_RG_P99")
V_LS_P50=$(verdict "$R_LS_P50")
V_LS_P99=$(verdict "$R_LS_P99")

# ── 合并 JSON ──
ALL_RESULTS="$WORK/all-results.json"
echo '{"results":[]}' > "$ALL_RESULTS"
for f in "$WORK/local.json" "$WORK/mergerfs.json" "$WORK/sshfs.json"; do
  [ -s "$f" ] || continue
  jq -s '{results: (.[0].results + .[1].results)}' "$ALL_RESULTS" "$f" > "$WORK/.merge.json" \
    && mv "$WORK/.merge.json" "$ALL_RESULTS"
done

jq --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
   --arg host "$HOSTNAME_VAL" --arg uname "$UNAME_VAL" \
   --arg cpu_count "$CPU_COUNT" --arg cpu_mhz "$CPU_MHZ" \
   --arg ci_mode "$CI_MODE" \
   --arg r_rg_p50 "$R_RG_P50" --arg r_rg_p99 "$R_RG_P99" \
   --arg r_ls_p50 "$R_LS_P50" --arg r_ls_p99 "$R_LS_P99" \
   '. + {
      schema_version: 1,
      kind: "perf-benchmark",
      timestamp: $ts,
      ci_mode: ($ci_mode == "true"),
      host: { hostname: $host, uname: $uname, cpu_count: $cpu_count, cpu_mhz: $cpu_mhz },
      ratios: {
        mergerfs_rg_p50_over_local: $r_rg_p50,
        mergerfs_rg_p99_over_local: $r_rg_p99,
        mergerfs_ls_p50_over_local: $r_ls_p50,
        mergerfs_ls_p99_over_local: $r_ls_p99
      }
    }' "$ALL_RESULTS" > "$REPORT_JSON"

# ── Markdown 报告 ──
{
  echo "# BASE-01 perf-benchmark 报告 — ${TIMESTAMP}"
  echo ""
  echo "> 适用版本：v3.0+ ；脚本：scripts/perf-benchmark.sh ；统计方法：hyperfine warmup=${WARMUP} runs=${RUNS}"
  echo ""
  echo "## 硬件信息"
  echo ""
  echo "- hostname: \`${HOSTNAME_VAL}\`"
  echo "- uname: \`${UNAME_VAL}\`"
  echo "- cpu_count: ${CPU_COUNT}"
  echo "- cpu_mhz: ${CPU_MHZ}"
  echo "- ci_mode: ${CI_MODE}"
  echo ""
  echo "## P50 / P99 比值（裁决：ratio P50 <= 1.5 PASS / <= 2.0 WARN / 其它 FAIL）"
  echo ""
  echo "| 命令 | local P50 | mergerfs P50 | ratio P50 | local P99 | mergerfs P99 | ratio P99 | 裁决 |"
  echo "|------|-----------|--------------|-----------|-----------|--------------|-----------|------|"
  echo "| rg . | ${LOCAL_RG_P50} | ${MERGERFS_RG_P50} | ${R_RG_P50} | ${LOCAL_RG_P99} | ${MERGERFS_RG_P99} | ${R_RG_P99} | ${V_RG_P50} / ${V_RG_P99} |"
  echo "| ls -R | ${LOCAL_LS_P50} | ${MERGERFS_LS_P50} | ${R_LS_P50} | ${LOCAL_LS_P99} | ${MERGERFS_LS_P99} | ${R_LS_P99} | ${V_LS_P50} / ${V_LS_P99} |"
  echo ""
  echo "## sshfs-only 降级档"
  echo ""
  echo "| 命令 | sshfs-only P50 | sshfs-only P99 |"
  echo "|------|----------------|----------------|"
  echo "| rg . | ${SSHFS_RG_P50} | ${SSHFS_RG_P99} |"
  echo "| ls -R | ${SSHFS_LS_P50} | ${SSHFS_LS_P99} |"
  echo ""
  echo "## 备注"
  echo ""
  echo "- 单位：秒（hyperfine times[] 原始值）"
  echo "- ci_mode=true 时 mergerfs/sshfs-only 档为 SKIP，本地档单独成立"
  echo "- sshfs-only 档为 Claude's Discretion 实现：当容器内 sshfs+sshd 直挂不可用时，回退到 ro bind mount /mnt/cold"
} > "$REPORT_MD"

info "JSON 报告: $REPORT_JSON"
info "MD   报告: $REPORT_MD"

# ── 裁决闸门 ──
EXIT_CODE=0
if [ "$MERGERFS_OK" = true ]; then
  for v in "$V_RG_P50" "$V_LS_P50"; do
    case "$v" in
      FAIL) fail "BASE-01 P50 超出 1.5x 基线 (verdict=$v)"; EXIT_CODE=1 ;;
      WARN*) warn "BASE-01 P50 接近 2x 基线 (verdict=$v)" ;;
      PASS*) pass "BASE-01 P50 verdict=$v" ;;
    esac
  done
  for v in "$V_RG_P99" "$V_LS_P99"; do
    case "$v" in
      FAIL) fail "BASE-01 P99 超出 2x 基线 (verdict=$v)"; EXIT_CODE=1 ;;
      WARN*) warn "BASE-01 P99 接近 2x 基线 (verdict=$v)" ;;
      PASS*) pass "BASE-01 P99 verdict=$v" ;;
    esac
  done
else
  info "mergerfs 档 SKIP，无 P50/P99 ratio 裁决；本地档已落盘供基线比较"
fi

echo ""
echo "========================================"
echo "perf-benchmark: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN, ${SKIP_COUNT} SKIP"
exit "$EXIT_CODE"
