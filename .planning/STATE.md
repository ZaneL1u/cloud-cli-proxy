---
gsd_state_version: 1.0
milestone: v1.3
milestone_name: 管理后台凭据与 SSH 密钥增强
status: planning
stopped_at: Roadmap created for v1.3
last_updated: "2026-04-03T10:10:00.000Z"
last_activity: 2026-04-03
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-03)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持"一条命令启动"的体验足够顺滑。
**Current focus:** Phase 17 — 时区与出口 IP 交互重构

## Current Position

Phase: 17 of 20 (时区与出口 IP 交互重构)
Plan: —
Status: Ready to discuss
Last activity: 2026-04-03 — Roadmap for v1.3 created

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**

- Total plans completed: 30 (v1.0: 19 + v1.1: 11)
- Average duration: —
- Total execution time: —

**By Milestone:**

| Milestone | Phases | Plans | Tasks | Timeline |
|-----------|--------|-------|-------|----------|
| v1.0 MVP | 6 | 19 | 42 | 3 days |
| v1.1 支持代理协议出网 | 4 | 11 | 21 | 3 days |
| v1.2 用户自助面板与 Bootstrap 重设计 | 6 | 3 | — | paused |
| v1.3 管理后台凭据与 SSH 密钥增强 | 4 | 0 | — | planning |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.

- [Phase 12-api]: auth_middleware.go 作为 Phase 11 前置依赖提前创建，包含 AuthMiddleware/RequireRole/UserIDFromContext/RoleFromContext
- [Phase 12-api]: userGuard 中间件链放在 deps.Admin 块内复用 JWT secret，用户 API 响应过滤所有敏感字段
- [Phase 12-api]: Portal 路由文件放在 _portal/portal/ 目录下，避免 TanStack Router 与 dashboard 路径冲突
- [Phase 12-api]: useMyHostDetail refetchInterval 使用函数形式基于 query data status 判断轮询

### Pending Todos

None.

### Blockers/Concerns

- None.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260328-trs | 为用户容器添加资源限制功能（内存、CPU、磁盘） | 2026-03-29 | e55a4e4 | [260328-trs-cpu](./quick/260328-trs-cpu/) |
| 260328-u4q | 重写README并创建VitePress中英文文档站部署到GitHub Pages | 2026-03-29 | d23e1b2 | [260328-u4q-readme-vitepress-github-pages](./quick/260328-u4q-readme-vitepress-github-pages/) |

## Session Continuity

Last session: 2026-04-03T10:10:00.000Z
Stopped at: Roadmap created for milestone v1.3
Resume file: None
