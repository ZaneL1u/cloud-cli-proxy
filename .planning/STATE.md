---
gsd_state_version: 1.0
milestone: v1.2
milestone_name: 用户自助面板与 Bootstrap 重设计
status: planning
stopped_at: Completed 260405-hai quick task
last_updated: "2026-04-05T04:23:13.097Z"
last_activity: 2026-04-05
progress:
  total_phases: 6
  completed_phases: 2
  total_plans: 5
  completed_plans: 5
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-28)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持"一条命令启动"的体验足够顺滑。
**Current focus:** Phase 11 — 认证基础设施与数据迁移

## Current Position

Phase: 13 of 15 (claude 账号管理)
Plan: Not started
Status: Ready to plan
Last activity: 2026-03-29

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
| Phase 12-api P01 | 4min | 2 tasks | 6 files |
| Phase 12-api P02 | 5min | 3 tasks | 6 files |

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

- Phase 15: macOS 默认 bash 3.x 对 SSE 消费的兼容性需实测
- Phase 14: Nginx WebSocket 升级头配置和超时设置需实测验证

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260328-trs | 为用户容器添加资源限制功能（内存、CPU、磁盘） | 2026-03-29 | e55a4e4 | [260328-trs-cpu](./quick/260328-trs-cpu/) |
| 260328-u4q | 重写README并创建VitePress中英文文档站部署到GitHub Pages | 2026-03-29 | d23e1b2 | [260328-u4q-readme-vitepress-github-pages](./quick/260328-u4q-readme-vitepress-github-pages/) |
| 260405-h13 | 容器管理 API (root 密码/Claude settings/状态/更新) + 前端集成 | 2026-04-05 | 6affd42 | [260405-h13-root-claude-settings-claude-claude-code](./quick/260405-h13-root-claude-settings-claude-claude-code/) |
| 260405-hai | 增强 Claude 状态 API：返回每个进程的 PID、工作目录和运行时间 | 2026-04-05 | 2ae9592 | [260405-hai-claude-api-pid](./quick/260405-hai-claude-api-pid/) |
| 260405-hio | Claude Code 深度配置集成：结构化settings编辑面板+系统指纹查看 | 2026-04-05 | bea7886 | [260405-hio-claude-code-settings](./quick/260405-hio-claude-code-settings/) |
| 260405-jji | 镜像版本管理：自动拉取最新镜像+版本展示+一键升级 | 2026-04-05 | 6aad37a | [260405-jji-image-version-mgmt](./quick/260405-jji-image-version-mgmt/) |
| 260405-qk2 | SSH密钥体系改造：入站免密登录+出站外部鉴权+多密钥管理 | 2026-04-05 | d297fbb | [260405-qk2-ssh](./quick/260405-qk2-ssh/) |

## Session Continuity

Last session: 2026-04-05T11:15:00.000Z
Stopped at: Completed 260405-qk2 SSH 密钥体系改造
Resume file: None
