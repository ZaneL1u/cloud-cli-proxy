---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: cloud-claude 透明远程 CLI
status: verifying
stopped_at: Completed 28-02-PLAN.md
last_updated: "2026-04-15T07:31:44.813Z"
last_activity: 2026-04-15
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 7
  completed_plans: 7
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-15)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 28 — 生产环境 FUSE 兼容性验证

## Current Position

Phase: 28 (生产环境 FUSE 兼容性验证) — VERIFIED (human_needed)
Plan: 2 of 2
Status: Code verified — awaiting production E2E confirmation
Last activity: 2026-04-15

Progress: [██████████████████░░] 88% (v2.0)

## Performance Metrics

**Velocity:**

- Total plans completed: 4 (v2.0)
- Average duration: -
- Total execution time: -

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 25 | 1 | - | - |
| 26 | 1 | - | - |
| 27 | 2 | - | - |

*Updated after each plan completion*
| Phase 24-fuse P01 | 1min | 3 tasks | 3 files |
| Phase 25 P01 | 5min | 3 tasks | 6 files |
| Phase 26 P01 | 2min | 2 tasks | 4 files |
| Phase 27 P01 | 2min | 2 tasks | 4 files |
| Phase 27 P02 | 2min | 2 tasks | 2 files |
| Phase 28 P01 | 2min | 2 tasks | 2 files |
| Phase 28 P02 | 1min | 2 tasks | 3 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v2.0 roadmap]: 目录映射主路径为 sshfs slave + SFTP，Mutagen 作为 v2.x 备选
- [v2.0 roadmap]: SSH Proxy 保持零改造，cloud-claude 通过现有多 session channel 连接
- [Phase 24-fuse]: SYS_ADMIN 和 /dev/fuse 对所有容器统一附加，不做条件区分
- [Phase 24-fuse]: SSH Proxy 确认零改造，多 session channel 天然支持 sshfs slave 模式
- [Phase 25-cli]: Entry API 为认证与 SSH 参数唯一契约；配置无硬编码默认网关；单 PTY session 进入远程 claude（argv 全量透传属 Phase 26）
- [Phase 25]: Entry API 为唯一认证契约，不新增专用 cloud-claude API
- [Phase 25]: SSH HostKeyCallback 使用 InsecureIgnoreHostKey（与 Entry 脚本一致）
- [Phase 25]: 轮询间隔 3s / 总超时 120s 作为默认值，远程命令 claude
- [Phase 26]: shellescape.QuoteCommand 构建安全远程命令行，退出码返回值上浮修复 HI-01
- [Phase 27]: channelRWC Reader=StdoutPipe / WriteCloser=StdinPipe，反接会导致协议死锁
- [Phase 27]: waitForMount 使用可注入 check 函数，与 ssh_ready.go 的轮询结构一致
- [Phase 27]: 远程命令使用 cd /workspace && claude 前缀，硬编码路径不含用户输入
- [Phase 27]: sshConnect 和 runClaude 为 unexported 函数，仅 ConnectAndRunClaude 对外暴露
- [Phase 28]: 选择 apparmor=unconfined 而非自定义 AppArmor profile（运维复杂度 vs 安全边界已由 nftables+namespace 覆盖）
- [Phase 28]: FUSE 检测采用 modprobe + /dev/fuse 双重检查，兼容内置模块和可加载模块

### Pending Todos

None yet.

### Blockers/Concerns

- FUSE + AppArmor/seccomp 兼容性需在目标 Linux 宿主上验证（Phase 28 专项）
- [Phase 25] 代码审查发现 SSH 退出时 TTY raw 模式未恢复（HI-01），建议在 Phase 26 终端体验工作中一并修复

## Session Continuity

Last session: 2026-04-15T06:43:29.489Z
Stopped at: Completed 28-02-PLAN.md
Resume file: None
