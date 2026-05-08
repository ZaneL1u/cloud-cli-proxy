---
gsd_state_version: 1.0
milestone: v3.2
milestone_name: 多形态容器接入
status: in-progress
stopped_at: Completed 44-01-PLAN.md
last_updated: "2026-05-08T08:45:25Z"
last_activity: 2026-05-08
progress:
  total_phases: 7
  completed_phases: 6
  total_plans: 14
  completed_plans: 14
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-07 — v3.2 milestone started)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v3.2 里程碑全部阶段完成，待归档

## Current Position

Milestone: v3.2 多形态容器接入
Phase: 44 of 42 (doctor sshd config)
Plan: 01 complete (2/2)
Status: Phase 44 Plan 01 complete — checkSSHDForwarding 已实现并注册
Last activity: 2026-05-08

Progress: [██████████] 100%

## Performance Metrics

**Velocity:**

- Total plans completed: 6 (v3.2)
- Average duration: 8min 12s
- Total execution time: 30min 49s

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 038 | 3/3 | 28min 15s | 9min 24s |
| 43 | 2 | - | - |
| 44 | 1 | 2min 34s | 2min 34s |

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

Phase 44 Plan 01 决策：

- runner 错误时默认报 `SSH_SSHD_FORWARDING_DISABLED`（第一个发现问题的错误码），保持与 `checkSSHDKeepaliveDrift` 一致的错误处理策略
- parseSSHDForwarding 使用 `strings.ToLower` 做大小写防御性处理，sshd -T 输出全小写但做防护

### Pending Todos

- v3.2 里程碑归档（audit + complete-milestone）

### Blockers/Concerns

无。

### Quick Tasks Completed

v3.1 quick tasks 见归档 STATE。

### Roadmap Evolution

v3.2 roadmap 全部完成：

- Phase 38: SSH-01..04 (端口转发 + 安全校验 + 测试验证) — COMPLETE
- Phase 39: LOCAL-01..04 + UX-02 (本地 Dev Containers) — COMPLETE
- Phase 40: SSH-05 + SEC-01..02 (E2E 验证 + 安全) — COMPLETE
- Phase 41: UX-01 (doctor 扩展) — COMPLETE
- Phase 42: Phase 39 验证补齐 (gap closure) — COMPLETE
- Phase 44-01: doctor sshd 转发检查 (SSH-03 gap closure) — COMPLETE

## Session Continuity

Last session: 2026-05-08T08:45:25Z
Stopped at: Completed 44-01-PLAN.md
Resume file: .planning/phases/44-doctor-sshd-config/44-01-SUMMARY.md

## Deferred Items

v3.1 遗留 deferred items 保持原状态，见 MILESTONES.md。

---
*State updated: 2026-05-08 after Phase 44 Plan 01 completion*
