---
phase: 25-cloud-claude-cli
plan: 01
subsystem: cli
tags: [cobra, yaml, ssh, pty, go]

requires:
  - phase: 24-fuse
    provides: 容器 FUSE 权限与 SSH Proxy 多 session 能力
provides:
  - cloud-claude CLI 二进制（cobra 入口 + init 子命令 + 根命令主流程）
  - ~/.cloud-claude/config.yaml 配置读写模块
  - Entry API HTTP 客户端（认证 + 就绪轮询 + 网关预检）
  - SSH+PTY 会话模块（密码认证 + 窗口变更 + 远程 claude 启动）
affects: [phase-26-argv-passthrough, phase-27-dir-mapping]

tech-stack:
  added: [cobra v1.10.2, gopkg.in/yaml.v3, golang.org/x/term v0.42.0]
  patterns: [cobra 子命令结构, 环境变量/flag/交互式三路配置输入]

key-files:
  created:
    - cmd/cloud-claude/main.go
    - internal/cloudclaude/config.go
    - internal/cloudclaude/entry.go
    - internal/cloudclaude/ssh.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Entry API 为唯一认证契约，不新增专用 cloud-claude API"
  - "SSH HostKeyCallback 使用 InsecureIgnoreHostKey（与现有 Entry 脚本一致，Phase 25 范围）"
  - "轮询间隔 3s / 总超时 120s 作为默认值"
  - "远程命令执行 claude（假设受管镜像已在 PATH）"

patterns-established:
  - "internal/cloudclaude 包承载所有客户端逻辑，cmd/cloud-claude 仅为入口编排"
  - "退出码约定: 0 成功 / 1 认证 / 2 网络 / 3 超时 / 4 配置 / 5 其他"

requirements-completed: [CLI-01, CLI-02, CLI-04, CLI-05]

duration: 5min
completed: 2026-04-15
---

# Phase 25 Plan 01: cloud-claude CLI 骨架与连接 Summary

**cobra 入口 + init 配置持久化 + Entry API 认证轮询 + SSH PTY 远程 claude 会话的完整 CLI 闭环**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-15T04:07:01Z
- **Completed:** 2026-04-15T04:12:00Z
- **Tasks:** 3
- **Files modified:** 6

## Accomplishments

- `cloud-claude` 可构建二进制：无参数运行完成认证→轮询→SSH→PTY→远程 claude 全流程
- `cloud-claude init` 支持交互式/环境变量/flag 三种配置输入路径，写入 `~/.cloud-claude/config.yaml`（目录 0700、文件 0600）
- Entry 客户端覆盖 ready/not_ready/401/403/404/500 共 6 种 HTTP 状态码，全部中文错误提示
- SSH 连接具备 10 秒 TCP 超时和独立握手阶段，PTY 申请含窗口尺寸检测和 SIGWINCH 转发
- 退出码严格按约定（0-5）区分错误类型，gateway 完全来自配置，无硬编码 URL

## Task Commits

1. **Task 1: 依赖与配置模块** - `6fd35c3` (feat)
2. **Task 2: Entry 认证与就绪轮询** - `5d18243` (feat)
3. **Task 3: SSH 会话与根命令** - `8550bfe` (feat)

## Files Created/Modified

- `cmd/cloud-claude/main.go` — CLI 入口，cobra 根命令和 init 子命令编排
- `internal/cloudclaude/config.go` — 配置结构体、路径解析、YAML 读写
- `internal/cloudclaude/entry.go` — Entry API HTTP 客户端、认证、就绪轮询、网关预检
- `internal/cloudclaude/ssh.go` — SSH 密码认证连接、PTY 申请、SIGWINCH 转发、远程 claude 启动
- `go.mod` — 新增 cobra、yaml.v3 依赖，x/term 升级
- `go.sum` — 依赖校验和更新

## Decisions Made

- Entry API (`POST /v1/entry/{shortId}/auth`) 为唯一认证契约，与控制面现有实现完全对齐
- SSH `HostKeyCallback` 使用 `InsecureIgnoreHostKey`，与现有 bootstrap 脚本行为一致
- 轮询默认值：间隔 3 秒，总超时 120 秒
- 远程命令为 `claude`，依赖受管镜像 PATH 中已有 claude code
- 使用 `net.JoinHostPort` 格式化 SSH 地址，兼容 IPv6

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] IPv6 地址格式兼容**
- **Found during:** Task 3
- **Issue:** 使用 `fmt.Sprintf("%s:%d")` 格式化 SSH 地址不兼容 IPv6（linter 警告）
- **Fix:** 改用 `net.JoinHostPort` 生成地址字符串
- **Files modified:** internal/cloudclaude/ssh.go
- **Verification:** linter 通过，构建成功
- **Committed in:** 8550bfe (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix)
**Impact on plan:** IPv6 兼容性修复，无范围扩展

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `cloud-claude` 二进制可构建并运行，为 Phase 26（参数透传 + TTY/信号/退出码完整语义）提供客户端锚点
- `internal/cloudclaude` 包结构清晰，Phase 26 可直接在 SSH 模块上扩展信号转发和退出码透传
- Phase 27 目录映射可复用 SSH 连接基础设施

## Self-Check: PASSED

- [x] cmd/cloud-claude/main.go exists
- [x] internal/cloudclaude/config.go exists
- [x] internal/cloudclaude/entry.go exists
- [x] internal/cloudclaude/ssh.go exists
- [x] Commit 6fd35c3 exists
- [x] Commit 5d18243 exists
- [x] Commit 8550bfe exists

---
*Phase: 25-cloud-claude-cli*
*Completed: 2026-04-15*
