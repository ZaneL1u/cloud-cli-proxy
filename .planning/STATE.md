---
gsd_state_version: 1.0
milestone: v3.5
milestone_name: 网络白名单与 DNS 拆分解析
status: milestone_shipped
stopped_at: v3.5 archived to .planning/milestones/ + tag v3.5
last_updated: "2026-05-13T12:30:00.000Z"
last_activity: 2026-05-13 -- v3.5 milestone complete and archived
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 10
  completed_plans: 10
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-13)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v3.5 ✅ shipped — 等待 `/gsd-new-milestone` 进入下一里程碑

## Current Position

Phase: —（v3.5 已 ship，待下一里程碑）
Plan: —
Status: Milestone shipped
Last activity: 2026-05-13

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.5 关键技术决策（已落地，可作为后续里程碑参考）：

- 两段式 sing-box 配置：静态 config.json（每 host 模板渲染）+ 动态 local rule-set 文件（sing-box 1.10+ 文件 watch 自动 reload，不重启进程）
- 拆分 DNS：内网 `.lan/.local/.internal` 走 `dns-local`，公网白名单走代理 DoH 1.1.1.1（保护查询隐私）
- 容器 `/etc/resolv.conf` 与 `/etc/nsswitch.conf` `:ro` bind mount（唯一 nameserver 172.19.0.1 + `hosts: files dns`）
- `ContainerProxyProvider` 拆分为 `PrepareGateway` + `PrepareHost`，消除 entrypoint 启动时 tun0 未监听的竞争
- `is_system` 预设双层防御（Go sentinel + SQL `WHERE is_system = FALSE`）
- 双轨审计（DB `host_bypass_audit_log` + `EventRecorder.RecordEvent`）
- `QueueHostAction` 第 5 参用专属 `bypassSnapshotID` 形参，闭死「借 requestedBy 传 ID」hack
- `ApplyBypassRuleSet` 严格顺序「先 nft 事务 → 后 atomic write」
- 健康检查用 `/dev/tcp/192.168.0.1/53` TCP 半握手，不发真实 DNS 报文
- nft 表族 = `ip cloudproxy`，`nftRunner` 自带 `nsenter -t <pid> -n --` 包装（单点事实源）
- uat-bypass.yml fixture 自适应 preflight（非 `if:false`）

### Pending Todos

- `/gsd-new-milestone` 进入下一里程碑的需求收敛与路线图规划

候选方向（参见 PROJECT.md Backlog）：

- v3.5 P1：cn-dev / oss-dev / ai-api 预设 + 远程 rule-set 拉取 + 灰度按钮 + 用户自助配置 + 命中统计 + 流量 dashboard
- v3.5 P2 tech-debt：TD-02 I9 严格化 / TD-03 detectHostEth0IPFallback 真实化 / TD-04 I3 切 nft counter / TD-05 verify.go Linux runner 集成测试
- ENH-NEXT 系列：容器预热与空闲回收 / 性能 metrics 可视化 / mount 模式可观测 / 跨会话持久缓存 / 热同步 inotify 改造

### Blockers/Concerns

无。

### Roadmap Evolution

v3.5 已 ship（2026-05-13），Phases 45-47 / 10 plans / 34 REQ satisfied 全部归档到 `milestones/v3.5-ROADMAP.md` 和 `milestones/v3.5-REQUIREMENTS.md`。MILESTONE-AUDIT 在 `milestones/v3.5-MILESTONE-AUDIT.md`。

历史归档：v1.0 / v1.1 / v1.2(partial) / v1.3(archived) / v2.0 / v3.0 / v3.1 / v3.4 / v3.5 均已 ship，详情见 `.planning/MILESTONES.md` 和 `.planning/milestones/`。

## Session Continuity

Last session: 2026-05-13
Stopped at: v3.5 archived to .planning/milestones/ + tag v3.5
Resume: `/clear` 后 `/gsd-new-milestone` 进入下一里程碑

## Deferred Items

v3.5 deferred-to-follow-up（不阻塞 ship，已记录在 `milestones/v3.5-MILESTONE-AUDIT.md` Tech Debt 表）：

- TD-02 I9 严格化（mDNS/LLMNR/NetBIOS 注入 + counter 断言）
- TD-03 `detectHostEth0IPFallback` 真实化（EgressConfig.LANBypassProbeIP 扩展）
- TD-04 I3 切到 nft counter 持续观测窗口
- TD-05 `verify.go` Linux runner 真实容器 + nsenter 集成测试

历史 deferred：

- v3.4 deferred-to-ship: 11 项人工验证场景（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）
- v3.0/v3.1 deferred-to-ship: 3 项真机签字（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）

---

<!-- State updated: 2026-05-13 — v3.5 milestone shipped & archived (Phases 45-47, 10/10 plans, 34/34 REQ, tag v3.5) -->
