---
phase: 27-session
plan: 02
subsystem: cli
tags: [ssh, sshfs, sftp, go, refactor]

requires:
  - phase: 27-session-01
    provides: mountWorkspace(conn, localDir) 和 fusermountCleanup 函数
  - phase: 26-passthrough
    provides: shellescape.QuoteCommand 远程命令构建和退出码透传
provides:
  - 三阶段 ConnectAndRunClaude（sshConnect → mountWorkspace → runClaude）
  - main.go 获取 CWD 并传递给 ConnectAndRunClaude
  - claude 在容器内以 /workspace 为工作目录运行
affects: [28-verification, end-to-end-testing]

tech-stack:
  added: []
  patterns: [three-phase lifecycle with LIFO defer cleanup]

key-files:
  created: []
  modified:
    - internal/cloudclaude/ssh.go
    - cmd/cloud-claude/main.go

key-decisions:
  - "远程命令使用 cd /workspace && claude 前缀，硬编码路径不含用户输入"
  - "sshConnect 和 runClaude 为 unexported 函数，仅 ConnectAndRunClaude 对外暴露"

patterns-established:
  - "三阶段生命周期：connect → mount → run，defer 链 LIFO 保证清理顺序"

requirements-completed: [MAP-01, MAP-02, MAP-03]

duration: 2min
completed: 2026-04-15
---

# Phase 27 Plan 02: 双 session 目录映射集成 Summary

**重构 ConnectAndRunClaude 为 sshConnect→mountWorkspace→runClaude 三阶段架构，main.go 传递 os.Getwd() 实现端到端目录映射**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-15T05:48:49Z
- **Completed:** 2026-04-15T05:50:55Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- ConnectAndRunClaude 拆分为 sshConnect / mountWorkspace / runClaude 三个阶段函数
- 远程命令以 `cd /workspace && claude <args>` 运行，保证 claude 在映射目录中工作
- main.go 通过 os.Getwd() 获取当前目录并传入，完成端到端 CWD 传递链
- defer 链 LIFO 顺序保证 cleanupMount → conn.Close 正确清理

## Task Commits

Each task was committed atomically:

1. **Task 1: 重构 ssh.go 为三阶段架构** - `5bcc546` (refactor)
2. **Task 2: 更新 main.go 传递当前工作目录** - `6614104` (feat)

## Files Created/Modified
- `internal/cloudclaude/ssh.go` - 拆分为 sshConnect + runClaude 两个内部函数，ConnectAndRunClaude 编排三阶段
- `cmd/cloud-claude/main.go` - 新增 os.Getwd() 获取 CWD 并传入 ConnectAndRunClaude

## Decisions Made
- 远程命令使用 `cd /workspace && claude` 前缀，`/workspace` 为硬编码常量路径（无用户可控输入注入风险）
- sshConnect 和 runClaude 保持 unexported，对外 API 仅 ConnectAndRunClaude

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 27 全部计划已完成，sshfs 挂载 + 三阶段编排端到端就绪
- Phase 28 可进行 FUSE + AppArmor/seccomp 兼容性验证和端到端集成测试

---
*Phase: 27-session*
*Completed: 2026-04-15*
