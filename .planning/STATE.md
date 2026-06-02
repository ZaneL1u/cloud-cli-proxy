---
gsd_state_version: 1.0
milestone: v4.2.0
milestone_name: 容器合并 · SQLite 迁移 · 配置统一
status: Awaiting next milestone
stopped_at: Phase 58 context gathered
last_updated: "2026-06-02T00:00:00.000Z"
last_activity: 2026-06-02 — Completed quick task 260602: 规则导入导出按钮
progress:
  total_phases: 5
  completed_phases: 1
  total_plans: 3
  completed_plans: 3
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-14)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 57 — resource-limits

## Current Position

Phase: Milestone v4.2.0 complete
Plan: —
Status: Awaiting next milestone
Last activity: 2026-06-02 — Completed quick task 260602: 规则导入导出按钮

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.6 关键技术决策（已落地，可作为后续里程碑参考）：

- e2e 测试体系基于 testcontainers-go + testify/suite + Scenario builder API
- 测试 harness 禁止裸 `time.Sleep`，统一走 `waitFor(ctx, predicate, timeout)` + `lint-no-bare-sleep.sh` 双层守护
- GoldenPath 抽象 + 纯函数 + 锁定常量作为跨 phase e2e 共享契约
- 出口 IP 三源 + Vote 多数派裁决；DNS / resolv.conf 篡改用 OR 语义二选一即 PASS
- host eth0 tcpdump 改走 netshoot privileged sidecar（`nicolaka/netshoot:v0.13`）
- Pumba 固定 tag `gaiaadm/pumba:0.10.0` 避免 latest 漂移
- nft 全规则 `expr.Counter` + 显式 `169.254.0.0/16 counter drop`
- worker `--cap-drop NET_RAW` + 删 SYS_ADMIN；NET_ADMIN 按 sing-box tun 依赖保留（折中，列 TD-1）
- `go test -race -shuffle=on -count=1` 默认化 + goleak.VerifyTestMain 三包接入
- 双绑互斥 API pre-check 用 409 + 中英双语 message + host_id/egress_ip_id 字段回显
- CI e2e 走 hosted ubuntu-24.04（与 v3.5 uat-bypass.yml 同款 runner 池）
- 「darwin 编译 + 纯函数单测 PASS = passed；Linux 真机断言 deferred-to-CI 非阻塞 ship」

### Pending Todos

- `/gsd-new-milestone` 进入下一里程碑（v3.7）的需求收敛与路线图规划

候选方向（参见 PROJECT.md Backlog）：

- **v3.6 起手收尾（建议作为 v3.7 第一周 P0 收尾）**
  - V36-TD-3 P1 — Scenario.Start Step 2..7 真实接入（Linux runner 真机跑通的共同前置）
  - V36-TD-1 P1 — Phase 49 LEAK-08 fixture NET_ADMIN 期望校准
  - V36-Linux-Signoff P0 — 9 项 deferred-to-CI 签字（MVS-06/07/08 + MVS-09/10 + LEAK-01..08 + KILL-01..04 + VERIFY-1/2）
- **v3.6 P2 增强**：TD-7 `DATABASE_URL` 透传 / TD-2 ContainerHandle 正式化 / TD-5 fixture bcrypt 动态生成 / TD-6 `/dev/net/tun` preflight 兜底 / TD-4 host-agent per-host health API
- **v3.5 P1 增强**：cn-dev / oss-dev / ai-api 预设 + 远程 rule-set 拉取 + 灰度按钮 + 用户自助配置 + 命中统计 + 流量 dashboard
- **v3.5 P2 tech-debt**：TD-02 I9 严格化 / TD-03 detectHostEth0IPFallback 真实化 / TD-04 I3 切 nft counter / TD-05 verify.go Linux runner 集成测试
- **ENH-NEXT 系列**：容器预热与空闲回收 / 性能 metrics 可视化 / mount 模式可观测 / 跨会话持久缓存 / 热同步 inotify 改造

### Blockers/Concerns

无。

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
| --- | --- | --- | --- | --- |
| 260513-ezu | 修复 worker firewall 测试 ApplyWorkerFirewallRules 参数错误 | 2026-05-13 | 73deb3c | [260513-ezu-worker-firewall-applyworkerfirewallrules](./quick/260513-ezu-worker-firewall-applyworkerfirewallrules/) |
| 260513-fjd | 修复 SubnetThirdOctet 碰撞测试阈值（10 → 40，匹配生日悖论期望） | 2026-05-13 | 0def841 | [260513-fjd-subnetthirdoctet](./quick/260513-fjd-subnetthirdoctet/) |
| 260513-gii | 修复 UpsertHost SQL 占位符不匹配（移除孤立的 $13，POST /v1/admin/hosts 500） | 2026-05-13 | 04636fd | [260513-gii-upserthost-sql](./quick/260513-gii-upserthost-sql/) |
| 260513-kru | 修复 worker netns 获取失败（增加重试 + 容器状态检查 + 延迟） | 2026-05-13 | f1c3a35 | [260513-kru-worker-netns](./quick/260513-kru-worker-netns/) |
| 260602 | 规则导入导出按钮 | 2026-06-02 | pending | [260602-rules-import-export](./quick/260602-rules-import-export/) |

### Roadmap Evolution

v3.6 已 ship（2026-05-14），Phases 45-52 / 39 plans / 38 REQ satisfied 全部归档到 `milestones/v3.6-ROADMAP.md`、`milestones/v3.6-REQUIREMENTS.md`、`milestones/v3.6-MILESTONE-AUDIT.md`，8 个 phase 目录归档到 `milestones/v3.6-phases/`。

历史归档：v1.0 / v1.1 / v1.2(partial) / v1.3(archived) / v2.0 / v3.0 / v3.1 / v3.4 / v3.5 / v3.6 均已 ship，详情见 `.planning/MILESTONES.md` 和 `.planning/milestones/`。

## Session Continuity

Last session: 2026-06-01T06:20:49.515Z
Stopped at: Phase 58 context gathered
Resume: `/clear` 后 `/gsd-new-milestone` 进入下一里程碑

## Deferred Items

### v4.0 close 时 acknowledged deferred items（共 40 项，2026-05-28）

`gsd-sdk query audit-open` 在 v4.0 close 时检出 40 项历史积压，全部为 v3.6 及之前里程碑的历史遗留（2 项 debug_session + 37 项 quick_task + 1 项 context_question）。用户在 milestone close 时选择 `[A] Acknowledge all` 继续，逐项不阻塞 v4.0 ship 决策。

**Debug sessions（2 项）：**

| Slug | Status | 备注 |
|------|--------|------|
| host-start-pending-stuck | investigating | v3.4 期间 task pending → running 状态转换排查未完结 |
| ip-leak-after-restart | diagnosed | 根因已定位为 v3.x 架构问题，v4.0 单容器化从架构层面消除 |

**Quick tasks（37 项）：** 全部 `status: missing`，历史快速任务未标记关闭，不构成功能缺口。

**CONTEXT questions（Phase 53, 3 项）：** sing-box process.user / NET_ADMIN cap 继承 / tun cgroup 白名单，均已在 Phase 53 plan 阶段解答。

### v4.0 tech debt（已清理，2026-05-28）

v4.0 审计发现的 5 项技术债务已在 close 前全部清理：

1. ✅ 删除孤立的 `gateway_singbox_config.go` + 测试文件，`buildGatewayProxyOutbound`/`buildGatewayDirectOutbound` 迁移到 `container_singbox_config.go`
2. ✅ 删除 `app.go` 中过期的 `rejoinHostNetworks` 函数（v3.x `cloudproxy-net-*` bridge 重连逻辑）
3. ✅ 删除 `admin_hosts.go` 中过期的 `cloudproxy-gw-*`/`cloudproxy-net-*` 清理路径 + 未使用的 `dockerNetworkRm` 函数
4. ✅ 修复 6 处文档中过期的 `sing-box-gateway`/`make gateway-image` 引用（zh/en FAQ、architecture、configuration）
5. ✅ 补全 Phase 54-56 SUMMARY.md 的 `requirements-completed` frontmatter（13 条 REQ）

### v3.6 deferred-to-CI（9 项 human verification，不阻塞 ship）

详见 `milestones/v3.6-MILESTONE-AUDIT.md` §5 表，全部为 hosted ubuntu-24.04 真机签字项，前置 V36-TD-3。

### 历史 deferred（v3.5 及之前）

- **v3.5 deferred-to-follow-up**（详见 `milestones/v3.5-MILESTONE-AUDIT.md` Tech Debt 表）：TD-02 I9 严格化 / TD-03 detectHostEth0IPFallback 真实化 / TD-04 I3 切 nft counter / TD-05 verify.go Linux runner 集成测试
- **v3.4 deferred-to-ship**：11 项人工验证场景（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）
- **v3.0/v3.1 deferred-to-ship**：3 项真机签字（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）

---

<!-- State updated: 2026-05-14 — v3.6 milestone shipped & archived (Phases 45-52, 39/39 plans, 38/38 REQ, 8 tech debt, 40 deferred items acknowledged at close, tag v3.6) -->

## Operator Next Steps

- Start the next milestone with /gsd-new-milestone
