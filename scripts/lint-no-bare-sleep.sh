#!/usr/bin/env bash
# Phase 45 Plan 05 — Cloud CLI Proxy e2e 套件「禁止裸 sleep」守护脚本。
#
# 设计动机（CONTEXT.md §Area 4 / E2E-05 决策）：
#   tests/e2e/ 下的所有用例必须通过 tests/e2e/harness/waitfor.go 的
#   WaitFor / WaitForLog / WaitForPort / WaitForHTTP / WaitForExec 系列
#   helper 来等条件就绪；裸 time.Sleep 会在并发 e2e / netem 抖动时假阳，
#   且 dump hook 也无从介入。
#
# 行为：
#   - 仓库根目录运行
#   - 用 grep 扫描 $TARGET_DIR（默认 tests/e2e）下所有 *.go 中匹配
#     ^\s*time\.Sleep\( 的行；正则前导是 \s*time，已天然规避 // time.Sleep( 注释
#   - 命中即打印「<file>:<line>: <content>」并 exit 1
#   - 无命中 → exit 0
#   - --help → 中文 usage + 退出码说明 → exit 0
#   - $TARGET_DIR 不存在 → 打印 [warn] 后 exit 0（Phase 45 之前的分支兼容）
#
# 不在范围：
#   - internal/ / cmd/ 下的合法 sleep（v3.5 既有 worker connect 后等 route
#     收敛、netns 暴露窗口等场景），改造它们是 Phase 51 QUAL-04 的任务。

set -euo pipefail

TARGET_DIR="${TARGET_DIR:-tests/e2e}"
SCRIPT_NAME="$(basename "$0")"

usage() {
    cat <<EOF
$SCRIPT_NAME — Cloud CLI Proxy e2e 套件「禁止裸 sleep」守护

用途：
    扫描 \$TARGET_DIR（默认 tests/e2e）下所有 *.go 中 time.Sleep( 调用。
    命中即认为违反 v3.6 Phase 45 E2E-05 决策（必须改用 harness.WaitFor 系列 helper）。

用法：
    bash $SCRIPT_NAME              # 扫描 tests/e2e/
    TARGET_DIR=tests/leak bash $SCRIPT_NAME
    bash $SCRIPT_NAME --help       # 显示本帮助

退出码：
    0  无命中（或目标目录不存在 — 打印 warn 后退出）
    1  发现裸 sleep
EOF
}

case "${1:-}" in
    -h|--help) usage; exit 0 ;;
esac

if [[ ! -d "$TARGET_DIR" ]]; then
    echo "[warn] target dir not found: $TARGET_DIR (skipped)"
    exit 0
fi

# grep -RInE：递归 + 行号 + 文件名 + ERE；--include='*.go' 仅扫 Go 源码。
# 命令本身可能 exit 1（无命中），用 || true 让 set -e 不直接终止脚本。
matches="$(grep -RInE --include='*.go' '^\s*time\.Sleep\(' "$TARGET_DIR" || true)"

if [[ -n "$matches" ]]; then
    echo "[fail] 发现裸 time.Sleep 调用（违反 Phase 45 E2E-05）："
    echo "$matches"
    echo
    echo "请改用 tests/e2e/harness/waitfor.go 提供的 WaitFor / WaitForLog / WaitForPort / WaitForHTTP / WaitForExec 系列 helper。"
    exit 1
fi

echo "[ok] $TARGET_DIR 内无裸 time.Sleep"
exit 0
