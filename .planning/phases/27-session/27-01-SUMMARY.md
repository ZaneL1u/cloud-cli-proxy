---
phase: 27-session
plan: 01
subsystem: infra
tags: [sftp, sshfs, fuse, ssh, directory-mapping]

requires:
  - phase: 24-fuse
    provides: 容器内 sshfs + FUSE 支持（SYS_ADMIN + /dev/fuse）
  - phase: 25-cli
    provides: SSH 连接建立和 PTY 会话管理（ConnectAndRunClaude）
provides:
  - mountWorkspace 函数：sshfs passive 模式 + 嵌入式 SFTP server
  - waitForMount 函数：轮询 mountpoint 检测直到挂载就绪或超时
  - fusermountCleanup 函数：防御性 fusermount 卸载
  - channelRWC 适配器：SSH session pipe → io.ReadWriteCloser
  - MountNotReadyError 错误类型
affects: [27-02, 28-compat]

tech-stack:
  added: [github.com/pkg/sftp v1.13.10]
  patterns: [SSH session pipe → io.ReadWriteCloser 适配, 可注入 check 函数的轮询模式]

key-files:
  created: [internal/cloudclaude/mount.go, internal/cloudclaude/mount_test.go]
  modified: [go.mod, go.sum]

key-decisions:
  - "channelRWC Reader=StdoutPipe / WriteCloser=StdinPipe，反接会导致协议死锁"
  - "waitForMount 接受可注入 check 函数，便于单元测试且解耦远程执行"

patterns-established:
  - "SSH session pipe 适配模式：channelRWC 将 session stdin/stdout 包装为 io.ReadWriteCloser 供协议库使用"
  - "可注入轮询检测：waitForMount 与 WaitForSSHReady 共享 timer+ticker+select 结构"

requirements-completed: [MAP-01, MAP-02, MAP-03]

duration: 2min
completed: 2026-04-15
---

# Phase 27 Plan 01: 目录映射基础设施 Summary

**pkg/sftp 嵌入式 SFTP server + sshfs passive 模式启动 + mountpoint 轮询检测 + fusermount 防御性清理**

## Performance

- **Duration:** 2 min
- **Started:** 2026-04-15T05:44:34Z
- **Completed:** 2026-04-15T05:46:42Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- 添加 `github.com/pkg/sftp v1.13.10` 依赖，创建 `mount.go` 包含完整目录映射基础设施
- 实现 `mountWorkspace`：通过 SSH session 启动容器内 sshfs passive 模式，Go 侧嵌入 SFTP server 服务本地文件
- 实现 `waitForMount`/`fusermountCleanup`/`channelRWC`/`MountNotReadyError` 辅助组件
- `waitForMount` 三个子测试全部通过：立即成功、重试后成功、超时返回 MountNotReadyError

## Task Commits

Each task was committed atomically:

1. **Task 1: 添加 pkg/sftp 依赖并创建 mount.go** - `30d691a` (feat)
2. **Task 2: waitForMount 单元测试** - `aa29d87` (test)

## Files Created/Modified

- `internal/cloudclaude/mount.go` - mountWorkspace / waitForMount / fusermountCleanup / channelRWC / MountNotReadyError
- `internal/cloudclaude/mount_test.go` - TestWaitForMount 三个子测试
- `go.mod` - 添加 github.com/pkg/sftp v1.13.10，升级 x/crypto 等间接依赖
- `go.sum` - 依赖校验和更新

## Decisions Made

- channelRWC 的 Reader 接 StdoutPipe、WriteCloser 接 StdinPipe，方向反接会导致协议死锁
- waitForMount 使用可注入 check 函数，与 ssh_ready.go 的轮询结构保持一致，便于纯单元测试

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] go.sum 缺失条目导致编译失败**
- **Found during:** Task 1（编译验证）
- **Issue:** `go get` 升级间接依赖后 go.sum 中 semaphore 包条目缺失
- **Fix:** 执行 `go mod tidy` 补齐 go.sum
- **Files modified:** go.sum
- **Verification:** `go build ./internal/cloudclaude/...` 编译通过
- **Committed in:** 30d691a (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** go mod tidy 为常规依赖管理操作，无范围蔓延。

## Issues Encountered

- Go module proxy 默认地址连接不稳定（connection reset），切换 GOPROXY=https://goproxy.cn 后正常下载

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- mountWorkspace / waitForMount / fusermountCleanup 已就绪，Plan 02 可直接导入使用
- Plan 02 将重构 ConnectAndRunClaude 为 connect → mountWorkspace → runClaude 三阶段

## Self-Check: PASSED

- [x] internal/cloudclaude/mount.go 存在
- [x] internal/cloudclaude/mount_test.go 存在
- [x] Commit 30d691a 存在
- [x] Commit aa29d87 存在
- [x] 无意外文件删除

---
*Phase: 27-session*
*Completed: 2026-04-15*
