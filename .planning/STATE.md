---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: 远端开发体验升级
status: Defining requirements
stopped_at: Phase 29 context gathered
last_updated: "2026-04-18T08:04:31.686Z"
last_activity: 2026-04-17 — Milestone v3.0 started
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v3.0 远端开发体验升级 — 定义需求中

## Current Position

Milestone: v3.0 远端开发体验升级
Phase: 未开始（定义需求中）
Plan: —
Status: Defining requirements
Last activity: 2026-04-17 — Milestone v3.0 started

Progress: [░░░░░░░░░░░░░░░░░░░░] 0% (v3.0)

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.0 关键方向已定：

- 文件映射改为 Mutagen + sshfs + mergerfs 三层（替代纯 sshfs）
- 容器内默认包一层 tmux/dtach 实现会话可恢复
- 多端连接默认 attach 同一 session，`--new-session` 独占
- doctor 升级为五维度自检
- Claude Code 登录态以 claude_account 为粒度持久化
- 性能基线：rg/ls ≤ 本地 1.5×、首连 ≤ 8s、30s 抖动无感

### Pending Todos

None — 等待 REQUIREMENTS.md 与 ROADMAP.md 产出后进入 phase 执行。

### Blockers/Concerns

无。前置调研已确认 Mutagen v0.18.1 / mergerfs 2.41.x / sshfs 容器配置全部可行。

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260416-wvu | injectSSHKeys 幂等化，保留用户手加密钥 | 2026-04-16 | cc18acf | [260416-wvu-make-injectsshkeys-idempotent-so-user-ge](./quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/) |
| 260417-0w4 | 新增 cloud-claude ssh doctor 子命令（owner/mode/PEM 尾换行自检与修复） | 2026-04-16 | d716b14, 3f0567c, 7836821 | [260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss](./quick/260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss/) |

## Session Continuity

Last session: 2026-04-18T08:04:31.674Z
Stopped at: Phase 29 context gathered
Resume file: .planning/phases/29-v3-worker/29-CONTEXT.md
