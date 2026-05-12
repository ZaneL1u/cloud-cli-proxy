---
gsd_state_version: 1.0
milestone: v3.5
milestone_name: 网络白名单与 DNS 拆分解析
status: planning
last_updated: "2026-05-12T05:00:00.000Z"
last_activity: 2026-05-12
progress:
  total_phases: 3
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-12)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v3.5 网络白名单与 DNS 拆分解析（roadmap 草案就绪，等待 plan-phase 分解）

## Current Position

Phase: Not started (roadmap drafted, awaiting `/gsd-plan-phase 45`)
Plan: —
Status: Planning
Last activity: 2026-05-12 — v3.5 roadmap drafted (Phases 45-47), 34/34 requirements mapped

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.5 关键技术决策（已研究确定，不再讨论）：

- 两段式 sing-box 配置：静态 config.json（变更需重启）+ 动态 local rule-set 文件（sing-box 1.10+ 文件 watch 自动 reload）
- 拆分 DNS：内网 `.lan/.local/.internal` 走 `dns-local`，公网白名单走代理 DoH（保护查询隐私）
- 容器 `/etc/resolv.conf` 改为只读挂载指向 sing-box tun IP（172.19.0.1），替代旧 8.8.8.8 占位
- IPv6：v3.5 容器内全禁（`--sysctl disable_ipv6=1` + ip6tables drop）
- fail-closed：sing-box 起不来则容器 unhealthy 不开 SSH；nft `output policy drop` + uid 锁定 sing-box 直连代理 IP:443
- v3.5 仅管理员可配置，不做用户自助
- 系统预设仅 `loopback`（强制开启）+ `lan`（默认关闭），`cn-dev` / `oss-dev` / `ai-api` 推到 P1

### Pending Todos

- 运行 `/gsd-plan-phase 45` 分解 Phase 45（网络配置基础与数据模型）
- Plan 化前对 `gateway_singbox_config.go` / `worker_firewall_linux.go` / `container_proxy_provider.go` 三个核心扩展点做现状勘察

### Blockers/Concerns

无。

### Roadmap Evolution

v3.5 roadmap 草案（基于 `.planning/research/SUMMARY.md` §6 建议路径，按需求依赖关系切分为 3 个 phase）：

- Phase 45: 网络配置基础与数据模型 — BYPASS-NET-01..04 / BYPASS-DNS-01..04 / BYPASS-DATA-01..04（12 需求）
- Phase 46: 控制面 API 与后台 UI — BYPASS-API-01..05 / BYPASS-UI-01..05（10 需求）
- Phase 47: 热更新链路与流量验证 — BYPASS-RELOAD-01..04 / BYPASS-NFT-01..04 / BYPASS-VERIFY-01..04（12 需求）

覆盖率：34/34 active v3.5 requirements mapped (100%)。

历史归档：v1.0 / v1.1 / v1.2(partial) / v1.3(archived) / v2.0 / v3.0 / v3.1 / v3.4 均已 ship，详情见 `.planning/MILESTONES.md` 和 `.planning/milestones/`。

## Session Continuity

Last session: 2026-05-12
Stopped at: v3.5 ROADMAP.md / REQUIREMENTS.md (traceability) / STATE.md 三件套全部写盘
Resume: `/gsd-plan-phase 45` 进入 Phase 45 规划

## Deferred Items

v3.5 暂无 deferred 项目（milestone 刚启动）

历史 deferred：

- v3.4 deferred-to-ship: 11 项人工验证场景（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）
- v3.0/v3.1 deferred-to-ship: 3 项真机签字（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）

---

<!-- State updated: 2026-05-12 — v3.5 milestone roadmap drafted (Phases 45-47, 34 requirements mapped) -->
