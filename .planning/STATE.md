---
gsd_state_version: 1.0
milestone: v4.0
milestone_name: sing-box 同容器化
status: planning
last_updated: "2026-05-15T17:47:25.229Z"
last_activity: 2026-05-15
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-14)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** v3.6 已 shipped，等待 `/gsd-new-milestone` 进入下一里程碑收敛

## Current Position

Phase: Not started (defining requirements)
Plan: —
Status: Defining requirements
Last activity: 2026-05-15 — Milestone v4.0 started

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

### Roadmap Evolution

v3.6 已 ship（2026-05-14），Phases 45-52 / 39 plans / 38 REQ satisfied 全部归档到 `milestones/v3.6-ROADMAP.md`、`milestones/v3.6-REQUIREMENTS.md`、`milestones/v3.6-MILESTONE-AUDIT.md`，8 个 phase 目录归档到 `milestones/v3.6-phases/`。

历史归档：v1.0 / v1.1 / v1.2(partial) / v1.3(archived) / v2.0 / v3.0 / v3.1 / v3.4 / v3.5 / v3.6 均已 ship，详情见 `.planning/MILESTONES.md` 和 `.planning/milestones/`。

## Session Continuity

Last session: 2026-05-14
Stopped at: v3.6 archived to .planning/milestones/ + tag v3.6
Resume: `/clear` 后 `/gsd-new-milestone` 进入下一里程碑

## Deferred Items

### v3.6 close 时 acknowledged deferred items（共 40 项，2026-05-14）

`gsd-sdk query audit-open` 在 v3.6 close 时检出 40 项历史积压，全部为 v3.5 及之前里程碑的历史遗留（2 项 debug_session + 37 项 quick_task + 1 项 verification_gap）。用户在 milestone close 时选择 `[A] Acknowledge all` 继续，逐项不阻塞 v3.6 ship 决策。

**Debug sessions（2 项，均开口未关）：**

| Slug | Status | 来源 |
|------|--------|------|
| host-start-pending-stuck | investigating | 2026-04-24（v3.4 期间 task pending → running 状态转换排查未完结）|
| ip-leak-after-restart | diagnosed | 2026-05-06（v3.4 期间 user 容器 default 路由仍指向 bridge gw 192.168.215.1，已 diagnosed 但未 closed）|

**Quick tasks（37 项，全部 `status: missing` —— 表示快速任务目录存在但缺关闭标记）：**

| # | Slug |
|---|------|
| 1 | 260328-trs-cpu |
| 2 | 260328-u4q-readme-vitepress-github-pages |
| 3 | 260405-h13-root-claude-settings-claude-claude-code |
| 4 | 260405-hai-claude-api-pid |
| 5 | 260405-hio-claude-code-settings |
| 6 | 260405-jji-image-version-mgmt |
| 7 | 260405-qk2-ssh |
| 8 | 260416-wvu-make-injectsshkeys-idempotent-so-user-ge |
| 9 | 260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss |
| 10 | 260418-running-panic-recovery |
| 11 | 260419-short-id-username |
| 12 | 260420-claude-chrome |
| 13 | 260421-host-bind-mounts |
| 14 | 260422-cac-claude |
| 15 | 260423-machine-id |
| 16 | 260424-cloud-claude-ip |
| 17 | 260425-authresponse-status-json-json-cannot-unm |
| 18 | 260502-ni0-user-centric-creds |
| 19 | 260504-2n4-fix-mount-path-overflow |
| 20 | 260504-414-1-get-v1-admin-host-files-path-xxx-api-2 |
| 21 | 260504-dtd-ip-macos-no-such-container-docker-compos |
| 22 | 260504-elo-image-lock-v3-3-0-tag-ghcr-io-app-go-rej |
| 23 | 260505-fjq-1-dockerfile-tzdata-tz-2 |
| 24 | 260505-gjs-ip-sse-sse-endpoint-eventsource-sse-post |
| 25 | 260506-stop-start-ip-list-detail |
| 26 | 260506-ty7-admin |
| 27 | 260506-urq-dockerfile-claude |
| 28 | 260507-3o9-a |
| 29 | 260507-3zk-a |
| 30 | 260507-docker-ip-user-gw-restart-no-db-running |
| 31 | 260508-readme-md |
| 32 | 260510-full-review |
| 33 | 260511-o15-api |
| 34 | 260513-ezu-worker-firewall-applyworkerfirewallrules |
| 35 | 260513-fjd-subnetthirdoctet |
| 36 | 260513-gii-upserthost-sql |
| 37 | 260513-kru-worker-netns |

> 注：上表第 34-37 项的 quick_task 实际已在 v3.5 期间完成并落 commit（73deb3c / 0def841 / 04636fd / f1c3a35，详见 `## Quick Tasks Completed` 节），audit-open 索引滞后于 commit 提交，不构成真实开口。

**Verification gaps（1 项）：**

| Phase | File | Status | 备注 |
|-------|------|--------|------|
| 49 | 49-VERIFICATION.md | gaps_found | Phase 49 初次验证以 `gaps_found` 结案的 3 条 backend gap（LEAK-06 raw socket / LEAK-07 link-local 显式 drop / LEAK-08 capability 审计）已在 Phase 51 QUAL-05/06 同里程碑内闭环（详见 `milestones/v3.6-MILESTONE-AUDIT.md` §2 / §7.2），但 49-VERIFICATION.md 内部 `status: gaps_found` 字段未回写为 `passed`，audit-open 索引仍计为开口。属文档同步 lag，**不构成功能 gap**。 |

### v3.6 deferred-to-CI（9 项 human verification，不阻塞 ship）

详见 `milestones/v3.6-MILESTONE-AUDIT.md` §5 表，全部为 hosted ubuntu-24.04 真机签字项，前置 V36-TD-3。

### 历史 deferred（v3.5 及之前）

- **v3.5 deferred-to-follow-up**（详见 `milestones/v3.5-MILESTONE-AUDIT.md` Tech Debt 表）：TD-02 I9 严格化 / TD-03 detectHostEth0IPFallback 真实化 / TD-04 I3 切 nft counter / TD-05 verify.go Linux runner 集成测试
- **v3.4 deferred-to-ship**：11 项人工验证场景（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）
- **v3.0/v3.1 deferred-to-ship**：3 项真机签字（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）

---

<!-- State updated: 2026-05-14 — v3.6 milestone shipped & archived (Phases 45-52, 39/39 plans, 38/38 REQ, 8 tech debt, 40 deferred items acknowledged at close, tag v3.6) -->
