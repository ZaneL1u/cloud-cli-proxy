#!/usr/bin/env bash
# scripts/ci-doctor-grep.sh — Phase 34 Plan 03 Task 3.6
#
# M14 终验脚本（ROADMAP §Phase 34 SC#3）：
#   (1) cloud-claude doctor --json → schema_version=1 + 所有 warn/fail check 含 next_action
#   (2) cloud-claude doctor （文本）→ 所有 [!]/[✗] 行含 "建议:" 子串
#   (3) 所有 [!]/[✗] 行含错误码 `[XXX_YYY_ZZZ]` 格式
#
# 用法：bash scripts/ci-doctor-grep.sh [path/to/cloud-claude-binary]
#
# 退出码：
#   0  → M14 + SC#3 全通过
#   1  → 任一断言失败（stderr 输出失败项）

set -euo pipefail

BIN="${1:-./cloud-claude}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

command -v jq >/dev/null || { echo "需要 jq (brew install jq / apt install jq)"; exit 1; }
test -x "$BIN" || { echo "二进制不存在或不可执行: $BIN"; exit 1; }

# ---------- (1) JSON 模式：schema_version=1 + 所有 warn/fail 必有 next_action ----------
# doctor 子命令可能退出 0/1/2；| jq 可能吞掉非零，用 || true 托底
"$BIN" doctor --json > "$WORK/report.json" || true

# 检查 JSON 合法
jq empty "$WORK/report.json" >/dev/null 2>&1 \
  || { echo "FAIL: doctor --json 输出非合法 JSON" >&2; cat "$WORK/report.json" >&2; exit 1; }

# 检查 schema_version
jq -e '.schema_version == 1' "$WORK/report.json" >/dev/null \
  || { echo "FAIL: schema_version ≠ 1" >&2; exit 1; }

# 所有 status ∈ {warn,fail} 的 check 必须有非空 next_action
MISSING=$(jq -r '.checks[] | select(.status=="warn" or .status=="fail")
                 | select((.next_action // "") == "")
                 | "\(.domain).\(.name)"' "$WORK/report.json")
if [ -n "$MISSING" ]; then
  echo "FAIL: 以下 warn/fail check 缺 next_action:" >&2
  echo "$MISSING" >&2
  exit 1
fi

# ---------- (2) 文本模式：所有 [!]/[✗] 行必须含 "建议:" ----------
NO_COLOR=1 "$BIN" doctor > "$WORK/report.txt" || true

# 降级 banner 的 [!] 行（含 "未找到上次会话快照"）不走 warn/fail check 渲染，放过；
# 仅检查 "── <domain> ──" 之后的 check 行。
BAD=$(awk '
  /^── (network|auth|ssh|mount|disk) ──$/ { in_section=1; next }
  /^$/                                    { in_section=0 }
  in_section && /^\s*\[[!✗]\]/            { print $0 }
' "$WORK/report.txt" | grep -v "建议:" || true)
if [ -n "$BAD" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺 '建议:' 子串:" >&2
  echo "$BAD" >&2
  exit 1
fi

# ---------- (3) 每条 warn/fail 必须含错误码：[XXX_YYY_ZZZ] ----------
BAD_CODE=$(awk '
  /^── (network|auth|ssh|mount|disk) ──$/ { in_section=1; next }
  /^$/                                    { in_section=0 }
  in_section && /^\s*\[[!✗]\]/            { print $0 }
' "$WORK/report.txt" | grep -vE '错误码:\s*[A-Z]+_[A-Z]+_[A-Z0-9]+' || true)
if [ -n "$BAD_CODE" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺错误码:" >&2
  echo "$BAD_CODE" >&2
  exit 1
fi

echo "OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码)."
