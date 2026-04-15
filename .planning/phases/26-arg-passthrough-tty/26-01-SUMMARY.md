---
phase: 26-arg-passthrough-tty
plan: 01
subsystem: cli
tags: [cobra, ssh, shellescape, tty, exit-code, signal]

requires:
  - phase: 25-cloud-claude-cli
    provides: "cobra 入口、SSHConfig、ConnectAndRunClaude、PTY/SIGWINCH 基础实现"
provides:
  - "ConnectAndRunClaude(cfg, claudeArgs) 接受用户参数并返回退出码"
  - "shellescape 安全远程命令构建（防注入）"
  - "非 TTY 管道模式（跳过 PTY/raw/SIGWINCH）"
  - "cobra DisableFlagParsing 透传用户 claude 参数"
  - "退出码通过返回值上浮，修复 HI-01 终端恢复"
affects: [27-sshfs-dir-mapping]

tech-stack:
  added: [al.essio.dev/pkg/shellescape v1.6.0]
  patterns: [cobra-flag-passthrough, exit-code-return-not-os-exit, tty-branch-guard]

key-files:
  created: []
  modified:
    - internal/cloudclaude/ssh.go
    - cmd/cloud-claude/main.go
    - go.mod
    - go.sum

key-decisions:
  - "shellescape.QuoteCommand 用于安全拼接远程命令行，防止 shell 注入"
  - "退出码通过 (int, error) 返回值上浮到 main，修复 HI-01"
  - "非 TTY 路径跳过 MakeRaw/RequestPty/SIGWINCH，支持管道模式"

patterns-established:
  - "cobra-flag-passthrough: 根命令 DisableFlagParsing+ArbitraryArgs 透传远程 CLI 参数"
  - "exit-code-return: SSH 模块禁止 os.Exit，退出码由 main 统一处理"
  - "tty-branch-guard: term.IsTerminal 判断后分离 TTY 与非 TTY 路径"

requirements-completed: [CLI-03, TTY-01, TTY-02, TTY-03]

duration: 2min
completed: 2026-04-15
---

# Phase 26 Plan 01: 参数透传与终端体验 Summary

**shellescape 安全命令构建 + cobra 透传用户 claude 参数 + 非 TTY 管道模式 + 退出码返回值上浮修复 HI-01**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-15T04:34:10Z
- **Completed:** 2026-04-15T04:36:32Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- ConnectAndRunClaude 接受 claudeArgs 参数，用 shellescape.QuoteCommand 构建 POSIX 安全远程命令行，缓解 T-26-01 命令注入威胁
- 非 TTY 路径（管道/脚本调用）跳过 MakeRaw、RequestPty、SIGWINCH 监听，`echo "query" | cloud-claude -p -` 可正常工作
- 退出码通过 (int, error) 返回值上浮到 main，defer term.Restore 始终执行，修复 Phase 25 HI-01 终端恢复缺陷（T-26-02 缓解）
- cobra 根命令 DisableFlagParsing + ArbitraryArgs 使 `-p`、`--model` 等 flag 不被拦截，init 子命令路由不受影响

## Task Commits

Each task was committed atomically:

1. **Task 1: SSH 模块重构——参数接收、安全命令构建、非 TTY 分支与退出码上浮** - `3a7f666` (feat)
2. **Task 2: cobra 根命令透传与退出码统一** - `125e773` (feat)

## Files Created/Modified

- `internal/cloudclaude/ssh.go` - ConnectAndRunClaude 签名重构、shellescape 命令构建、TTY/非 TTY 分支、退出码返回值
- `cmd/cloud-claude/main.go` - DisableFlagParsing 透传、前导 -- 剥离、退出码统一处理
- `go.mod` - 新增 shellescape v1.6.0 依赖
- `go.sum` - 依赖校验和更新

## Decisions Made

- 使用 `al.essio.dev/pkg/shellescape` v1.6.0 而非手写转义——POSIX 单引号规则成熟可靠，审计成本低
- 退出码以 `(int, error)` 返回而非 os.Exit——确保 defer term.Restore 始终执行
- 非 TTY 模式完全跳过 PTY 相关逻辑——保持管道/脚本调用行为一致

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- Go module proxy (`proxy.golang.org`) 首次连接 connection reset，切换到 `goproxy.cn` 后正常拉取

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 参数透传和终端体验完成，`cloud-claude` 可作为 `claude` 的透明替代
- Phase 27（sshfs 目录映射）可在此基础上添加本地目录同步能力

## Self-Check: PASSED

---
*Phase: 26-arg-passthrough-tty*
*Completed: 2026-04-15*
