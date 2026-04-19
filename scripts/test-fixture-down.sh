#!/usr/bin/env bash
# Phase 31 Plan 03 集成测试 fixture 销毁脚本。
#
# 用法：scripts/test-fixture-down.sh
#
# 行为：
#   - docker compose down -v --remove-orphans 销毁容器与卷
#   - 清理 /tmp/cloud-claude-fixture/ 临时目录
#   - 失败一律 silent（脚本不抛错，让 TestMain defer 调用安全收尾）
set -euo pipefail
FIXTURE_DIR="/tmp/cloud-claude-fixture"
if [ -d "$FIXTURE_DIR" ]; then
  (cd "$FIXTURE_DIR" && docker compose down -v --remove-orphans 2>/dev/null || true)
  rm -rf "$FIXTURE_DIR"
fi
echo "=== fixture 已销毁"
