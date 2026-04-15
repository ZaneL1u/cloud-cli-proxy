---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: cloud-claude 透明远程 CLI
status: planning
stopped_at: Phase 25 context gathered — 25-01-PLAN.md drafted
last_updated: "2026-04-15T12:00:00.000Z"
last_activity: 2026-04-15
progress:
  total_phases: 5
  completed_phases: 1
  total_plans: 2
  completed_plans: 1
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-14)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 25 — cloud-claude CLI 骨架与连接

## Current Position

Phase: 25
Plan: 25-01-PLAN.md（待执行）
Status: Context 已采集，计划已撰写
Last activity: 2026-04-15

Progress: [░░░░░░░░░░] 0% (v2.0)

## Performance Metrics

**Velocity:**

- Total plans completed: 0 (v2.0)
- Average duration: -
- Total execution time: -

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

*Updated after each plan completion*
| Phase 24-fuse P01 | 1min | 3 tasks | 3 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v2.0 roadmap]: 目录映射主路径为 sshfs slave + SFTP，Mutagen 作为 v2.x 备选
- [v2.0 roadmap]: SSH Proxy 保持零改造，cloud-claude 通过现有多 session channel 连接
- [Phase 24-fuse]: SYS_ADMIN 和 /dev/fuse 对所有容器统一附加，不做条件区分
- [Phase 24-fuse]: SSH Proxy 确认零改造，多 session channel 天然支持 sshfs slave 模式
- [Phase 25-cli]: Entry API 为认证与 SSH 参数唯一契约；配置无硬编码默认网关；单 PTY session 进入远程 claude（argv 全量透传属 Phase 26）

### Pending Todos

None yet.

### Blockers/Concerns

- FUSE + AppArmor/seccomp 兼容性需在目标 Linux 宿主上验证（Phase 28 专项）
- golang.org/x/crypto 全仓版本统一需在 Phase 25 开发前完成

## Session Continuity

Last session: 2026-04-15T12:00:00.000Z
Stopped at: Phase 25 discuss-phase --auto（含规划产物）
Resume file: .planning/phases/25-cloud-claude-cli/25-CONTEXT.md
