---
phase: 24-fuse
plan: 01
subsystem: infra
tags: [sshfs, fuse3, docker, ssh-proxy]

requires:
  - phase: none
    provides: "首个 v2.0 阶段，无前置依赖"
provides:
  - "受管镜像预装 sshfs + fuse3，fuse.conf 配置 user_allow_other"
  - "entrypoint.sh 运行时修复 /dev/fuse 权限"
  - "Worker createHost() 统一附加 --device /dev/fuse 和 --cap-add SYS_ADMIN"
  - "SSH Proxy 零改造验证结论（多 session channel + 全类型转发）"
affects: [25-cli-connect, 27-dir-mapping]

tech-stack:
  added: [sshfs, fuse3]
  patterns: ["条件设备权限修复（entrypoint 中 -c /dev/fuse 判断兼容旧容器）"]

key-files:
  created: []
  modified:
    - deploy/docker/managed-user/Dockerfile
    - deploy/docker/managed-user/entrypoint.sh
    - internal/runtime/tasks/worker.go

key-decisions:
  - "sshfs + fuse3 通过 apt-get 安装，与现有包管理块合并"
  - "entrypoint.sh 使用 [ -c /dev/fuse ] 条件判断，兼容无 FUSE 设备的旧容器"
  - "SYS_ADMIN 和 /dev/fuse 对所有容器统一附加，不做条件区分"
  - "SSH Proxy 确认零改造，多 session channel 天然支持 sshfs slave 模式"

patterns-established:
  - "FUSE 设备权限修复模式：entrypoint 以 root 身份运行时条件修复设备权限"

requirements-completed: [SRV-01, SRV-02, SRV-03]

duration: 1min
completed: 2026-04-14
---

# Phase 24 Plan 01: FUSE/sshfs 容器前置条件 Summary

**受管镜像预装 sshfs + fuse3 并配置 FUSE 权限，Worker 附加 --device /dev/fuse 和 --cap-add SYS_ADMIN，SSH Proxy 确认零改造支持多 session channel**

## Performance

- **Duration:** 1 min
- **Started:** 2026-04-14T19:04:54Z
- **Completed:** 2026-04-14T19:05:49Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- 受管镜像 Dockerfile 预装 sshfs + fuse3，fuse.conf 启用 user_allow_other
- entrypoint.sh 在容器启动时自动修复 /dev/fuse 权限为 666（兼容无 FUSE 的旧容器）
- Worker createHost() 对所有新建容器统一附加 --device /dev/fuse 和 --cap-add SYS_ADMIN
- SSH Proxy 代码审查确认：handleConnection() 循环接受所有 session channel，handleChannel() 双向转发所有请求类型，零改造即可支持 cloud-claude 多 session 连接模式

## Task Commits

Each task was committed atomically:

1. **Task 1: 受管镜像预装 sshfs/fuse3 并配置 FUSE 权限** - `d853b50` (feat)
2. **Task 2: Worker 容器创建附加 FUSE 设备和 SYS_ADMIN 能力** - `07a7b06` (feat)
3. **Task 3: SSH Proxy 零改造验证与记录** - 无代码修改（只读代码审查）

## Files Created/Modified
- `deploy/docker/managed-user/Dockerfile` - 追加 sshfs/fuse3 安装，新增 fuse.conf 配置 RUN 指令
- `deploy/docker/managed-user/entrypoint.sh` - 插入 /dev/fuse 条件权限修复代码块
- `internal/runtime/tasks/worker.go` - createHost() args 追加 --cap-add SYS_ADMIN 和 --device /dev/fuse

## SSH Proxy 验证

对 `internal/sshproxy/proxy.go` 进行了只读代码审查，验证以下三个事实：

### 1. 多 session channel 支持
`handleConnection()` 第 203 行 `for newChan := range chans` 循环接受所有 session channel，每个 channel 通过 `go s.handleChannel(newChan, targetAddr, targetUser, targetPassword)` 启动独立 goroutine。单个 SSH 连接可承载任意数量的并发 session channel。

### 2. 全类型请求转发
`handleChannel()` 第 260-271 行（client→target）和第 273-284 行（target→client）双向转发所有请求类型，包括 pty-req、shell、exec、env、window-change（客户端→目标）和 exit-status、exit-signal（目标→客户端）。转发逻辑不过滤任何请求类型。

### 3. sshfs slave/passive 模式兼容性
cloud-claude 将通过现有 SSH 连接开启第二个 session channel，在其中执行 sftp-server 子系统。`handleChannel()` 第 252 行 `targetClient.OpenChannel("session", nil)` 为每个新 channel 建立独立的到目标容器的 session，第 260-271 行完整转发 exec 请求（包括 subsystem 请求）。

**结论：SSH Proxy 无需任何代码改动即可支持 cloud-claude 的多 session 连接模式。**

## Decisions Made
- sshfs + fuse3 通过 apt-get 安装，与现有包管理块合并，保持单次 apt-get install 减少镜像层数
- entrypoint.sh 使用 `[ -c /dev/fuse ]` 条件判断，兼容没有 `--device /dev/fuse` 的旧容器
- SYS_ADMIN 和 /dev/fuse 对所有容器统一附加，不做条件区分——SYS_ADMIN 是 mount 系统调用的前提
- SSH Proxy 确认零改造：handleConnection() 天然支持多 session channel，handleChannel() 全类型转发

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 容器侧 FUSE/sshfs 前置条件已就绪，Phase 25（CLI 连接）和 Phase 27（目录映射）可直接使用
- SSH Proxy 零改造已确认，cloud-claude 可通过现有多 session channel 机制执行 sshfs slave 模式
- FUSE + AppArmor/seccomp 兼容性仍需在目标 Linux 宿主上验证（Phase 28 专项）

---
*Phase: 24-fuse*
*Completed: 2026-04-14*
