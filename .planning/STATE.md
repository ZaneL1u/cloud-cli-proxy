---
gsd_state_version: 1.0
milestone: v3.2
milestone_name: 多形态容器接入
status: unknown
last_updated: "2026-05-07T12:10:08.841Z"
progress:
  total_phases: 6
  completed_phases: 6
  total_plans: 21
  completed_plans: 21
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-07 — v3.2 milestone started)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 39 — 本地 Dev Containers 支持 (ready to plan)

## Current Position

Milestone: v3.2 多形态容器接入
Phase: 38 of 41 (SSH Proxy 端口转发支持)
Plan: 03 of 03 (sshd_config 验证 + forwarding 测试覆盖 — COMPLETE)
Status: Executing Phase 38 — Plan 03 complete, Phase 38 COMPLETE
Last activity: 2026-05-07 — Completed 038-03: sshd_config 端口转发配置验证 + 全部测试无回归

Progress: [██░░░░░░░░] 23%

## Performance Metrics

**Velocity:**
- Total plans completed: 3 (v3.2)
- Average duration: 9min 24s
- Total execution time: 28min 15s

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 038 | 3/3 | 28min 15s | 9min 24s |

*Updated after each plan completion*

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.2 初始决策：

- Cloud 版与本地版 **并行推进**，不冲突
- 本地版也强制 sing-box tun 全隧道，保持产品一致性
- 架构方向（一套代码 vs 两套入口）待研究后决策

Phase 38 Plan 01 决策：

- `channelOpenDirectMsg` 字段使用导出名（Raddr/Rport/Laddr/Lport），因为 `ssh.Marshal` 通过反射读取字段，未导出字段会导致 panic
- `dialContainer` 在 forward.go 中提取（而非 proxy.go），因为 `handleDirectTCPIP` 需要调用它
- `isForbiddenTarget` 设计为纯函数，不依赖 Server 结构体，便于单元测试

Phase 38 Plan 02 决策：

- `handleConnection` 改为预 dial 共享 targetClient，避免 per-channel dial 开销，且与 forwarded-tcpip HandleChannelOpen API 一致（每个 client 只能注册一次）
- `handleGlobalRequests` 使用 `ssh.Conn` 接口（而非 `*ssh.Client`），保持函数签名通用
- `proxyForwardedChannels` 测试通过 server-side ssh.Conn.OpenChannel 验证 SSH mux channel relay 路径

Phase 38 Plan 03 决策：

- Plan 038-03 的所有测试已在 038-01 和 038-02 中完整实现，验证确认无回归即可，无需新增代码
- sshd_config 配置在 managed-user 镜像中已就绪，38-RESEARCH.md 已确认

### Pending Todos

- Phase 39: Cloud/Local 两版架构边界分析
- Phase 39: Dev Containers 配置设计
- Phase 40: VS Code Remote-SSH E2E 验证
- Phase 41: Doctor 扩展与收尾

### Blockers/Concerns

无。

### Quick Tasks Completed

v3.1 quick tasks 见归档 STATE。

### Roadmap Evolution

v3.2 roadmap 已创建：
- Phase 38: SSH-01..04 (端口转发 + 安全校验 + 测试验证) — COMPLETE
- Phase 39: LOCAL-01..04 + UX-02 (本地 Dev Containers)
- Phase 40: SSH-05 + SEC-01..02 (E2E 验证 + 安全)
- Phase 41: UX-01 (doctor 扩展)

## Session Continuity

Last session: 2026-05-07T12:08:00Z
Stopped at: Completed 038-03-PLAN.md (sshd_config verification + forwarding test coverage)
Resume file: None

## Deferred Items

v3.1 遗留 deferred items 保持原状态，见 MILESTONES.md。

---
*State updated: 2026-05-07 after Phase 38 Plan 03 completion*
