---
status: partial
phase: 26-arg-passthrough-tty
source: [26-VERIFICATION.md]
started: "2026-04-15T04:35:00.000Z"
updated: "2026-04-15T04:35:00.000Z"
---

## Current Test

[awaiting human testing]

## Tests

### 1. 参数与退出码联调
expected: 在可用环境中对比 claude 与 cloud-claude 相同参数时的行为和退出码一致
result: [pending]

### 2. 窗口 Resize
expected: TTY 下改变窗口尺寸，远端 Claude Code 界面跟随调整
result: [pending]

### 3. Ctrl+C / Ctrl+\ 与终端恢复
expected: 中断信号正确转发，会话结束后本地终端无 raw 模式残留
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
