# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- 🗃️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (archived, capabilities merged into v3.4)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- ✅ **v3.0 远端开发体验升级** — Phases 29-35 (shipped 2026-04-23) — [Archive](milestones/v3.0-ROADMAP.md)
- ✅ **v3.1 映射语义补齐与懒加载** — Phases 36-37 (shipped 2026-04-24) — [Archive](milestones/v3.1-ROADMAP.md)
- ✅ **v3.4 多形态容器接入** — Phases 38-44 (shipped 2026-05-08) — [Archive](milestones/v3.4-ROADMAP.md)
- ✅ **v3.5 网络白名单与 DNS 拆分解析** — Phases 45-47 (shipped 2026-05-13) — [Archive](milestones/v3.5-ROADMAP.md)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-6) — SHIPPED 2026-03-28</summary>

- [x] Phase 1: 基础控制面与主机代理 (3/3 plans) — completed 2026-03-26
- [x] Phase 2: 隧道出网强制层 (3/3 plans) — completed 2026-03-27
- [x] Phase 3: 启动入口与 SSH 接入 (3/3 plans) — completed 2026-03-27
- [x] Phase 4: 后台管理界面 (3/3 plans) — completed 2026-03-27
- [x] Phase 5: 到期、审计与清理 (3/3 plans) — completed 2026-03-27
- [x] Phase 6: 加固与 MVP 就绪 (4/4 plans) — completed 2026-03-28

</details>

<details>
<summary>✅ v1.1 支持代理协议出网 (Phases 7-10) — SHIPPED 2026-03-28</summary>

- [x] Phase 7: 数据层与类型化 (3/3 plans) — completed 2026-03-28
- [x] Phase 8: SingBoxProvider 与受管镜像 (3/3 plans) — completed 2026-03-28
- [x] Phase 9: 前端适配与代理测试 (3/3 plans) — completed 2026-03-28
- [x] Phase 10: 技术债务清理 (2/2 plans) — completed 2026-03-28

</details>

<details>
<summary>⏸️ v1.2 用户自助面板与 Bootstrap 重设计 (Phases 11-16) — PARTIALLY SHIPPED, remaining deferred</summary>

- [x] Phase 11: 认证基础设施与数据迁移 (3/3 plans) — completed 2026-03-29
- [x] Phase 12: 用户自助 API 与前端路由 (2/2 plans) — completed 2026-03-29
- [ ] Phase 13: 账号管理与用户资源视图 (deferred)
- [ ] Phase 14: KasmVNC 用户面 (deferred)
- [ ] Phase 15: Bootstrap 重设计 (deferred)
- [ ] Phase 16: 级联禁用与到期治理 (deferred)

</details>

<details>
<summary>🗃️ v1.3 claude-shell 本地透明代理 (Phases 17-23) — ARCHIVED, capabilities merged into v3.4</summary>

**Archive reason:** v1.3 的"本地容器"和"指纹伪装"能力与 v3.4 的 `cloud-claude local` 目标重叠。为避免维护两套本地 Docker 编排逻辑，v1.3 能力拆分并入 v3.4：
- Phase 17 镜像基线 → 已并入 managed-user 镜像（v3.0+）
- Phase 18 网络隔离 → 并入 v3.4 Phase 39（本地版 sing-box sidecar + veth 注入）
- Phase 19 CLI 骨架 → 并入 v3.4 Phase 39（`cloud-claude local` 子命令）
- Phase 20 TTY 透传 → 不再需要（本地 publish 端口，SSH 用系统 OpenSSH）
- Phase 21 指纹伪造 → entrypoint `MODE=local` 深度伪装分支（快速任务级别）
- Phase 22 验证自检 → `cloud-claude local verify` / `doctor --local`
- Phase 23 混淆构建 → Makefile 构建脚本，不单独成里程碑

- [x] Phase 17: 镜像与 Entrypoint 基线 (17-01 + 17-02 gap closure 完成，已并入 v3.0+ 镜像)
- [ ] Phase 18: 网络隔离与分流 → **merged into v3.4 Phase 39**
- [ ] Phase 19: CLI 骨架与 Docker 编排 → **merged into v3.4 Phase 39**
- [ ] Phase 20: TTY 透传与交互体验 → **obsolete**（本地直连端口，无需 TTY 透传）
- [ ] Phase 21: 指纹伪造与反检测 → **merged into v3.4 entrypoint `MODE=local` 分支**
- [ ] Phase 22: 验证与自检 → **merged into v3.4 doctor 扩展**
- [ ] Phase 23: 混淆构建与交付 → **deferred to build script**

</details>

<details>
<summary>✅ v2.0 cloud-claude 透明远程 CLI (Phases 24-28) — SHIPPED 2026-04-15</summary>

- [x] Phase 24: 受管镜像 FUSE 硬化与容器参数 (1/1 plans) — completed 2026-04-14
- [x] Phase 25: cloud-claude CLI 骨架与连接 (1/1 plans) — completed 2026-04-15
- [x] Phase 26: 参数透传与终端体验 (1/1 plans) — completed 2026-04-15
- [x] Phase 27: 双 session 目录映射 (2/2 plans) — completed 2026-04-15
- [x] Phase 28: 生产环境 FUSE 兼容性验证 (2/2 plans) — completed 2026-04-15

</details>

<details>
<summary>✅ v3.0 远端开发体验升级 (Phases 29-35) — SHIPPED 2026-04-23</summary>

- [x] Phase 29: 受管镜像 v3 + Worker 容器参数扩展 (6/6 plans) — completed 2026-04-18
- [x] Phase 29.1: GetHost entry_password 修复 (P0 HOTFIX, INSERTED, 4/4 plans) — completed 2026-04-20
- [x] Phase 30: 控制面数据模型 + Entry API 扩展 (2/2 plans) — completed 2026-04-18
- [x] Phase 31: CLI 三层文件映射重构 (3/3 plans) — completed 2026-04-19
- [x] Phase 32: SSH 会话可靠性 + tmux + 多端 (5/5 plans，含 2 gap-closure) — completed 2026-04-20
- [x] Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）(2/2 plans + 3 post-fix) — completed 2026-04-21
- [x] Phase 34: cloud-claude doctor v3 + 错误码统一 (3/3 plans) — completed 2026-04-21
- [x] Phase 35: E2E 稳定化 + 性能验收 (5/5 plans) — completed 2026-04-23

3 项真机签字 deferred-to-ship（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04），跟踪在 `milestones/v3.0-phases/35-e2e/35-HUMAN-UAT.md`。

</details>

<details>
<summary>✅ v3.1 映射语义补齐与懒加载 (Phases 36-37) — SHIPPED 2026-04-24</summary>

- [x] Phase 36: 映射前置约束 + sshfs 内核缓存 (6/6 plans) — completed 2026-04-23
- [x] Phase 37: 冷文件读触发晋升 + e2e UAT (5/5 plans) — completed 2026-04-24

5 项人工验证 deferred-to-ship（Linux 真机 UAT / pgrep 存活 / 端到端晋升 / 手册可读性 / 双平台签字），跟踪在 `milestones/v3.1-MILESTONE-AUDIT.md`。

</details>

<details>
<summary>✅ v3.4 多形态容器接入 (Phases 38-44) — SHIPPED 2026-05-08</summary>

- [x] Phase 38: SSH Proxy 端口转发支持 (3/3 plans) — completed 2026-05-07
- [x] Phase 39: 本地 Dev Containers 支持 (3/3 plans) — completed 2026-05-07
- [x] Phase 40: VS Code Remote-SSH E2E 验证 (2/2 plans) — completed 2026-05-08
- [x] Phase 41: Doctor 扩展与收尾 (2/2 plans) — completed 2026-05-08
- [x] Phase 42: Phase 39 验证补齐 (1/1 plan, gap closure) — completed 2026-05-08
- [x] Phase 43: VS Code 端口转发 E2E 补齐 (2/2 plans, gap closure) — completed 2026-05-08
- [x] Phase 44: doctor sshd_config 验证 (1/1 plan, gap closure) — completed 2026-05-08

11 项人工验证 deferred-to-ship（Phase 38 x3 / Phase 39 x5 / Phase 43 x3），跟踪在 `milestones/v3.4-MILESTONE-AUDIT.md`。

</details>

<details>
<summary>✅ v3.5 网络白名单与 DNS 拆分解析 (Phases 45-47) — SHIPPED 2026-05-13</summary>

- [x] Phase 45: 网络配置基础与数据模型 (3/3 plans) — completed 2026-05-12
- [x] Phase 46: 控制面 API 与后台 UI (4/4 plans) — completed 2026-05-12
- [x] Phase 47: 热更新链路与流量验证 (3/3 plans) — completed 2026-05-13

34/34 v3.5 REQ-ID SATISFIED；5 项 tech-debt follow-up（TD-01 fixed commit 58c0a5f；TD-02..05 P2 不阻塞 ship）；10 项 Info-level deferred 见 `milestones/v3.5-MILESTONE-AUDIT.md`。

</details>

## Progress

**Current milestone:** （TBD — 待 `/gsd-new-milestone` 启动下一个里程碑）

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-6. v1.0 MVP | v1.0 | 19/19 | Complete | 2026-03-28 |
| 7-10. v1.1 代理协议出网 | v1.1 | 11/11 | Complete | 2026-03-28 |
| 11-12. v1.2 认证与自助面板 | v1.2 | 5/5 | Partial | 2026-03-29 |
| 17-23. claude-shell 本地代理 | v1.3 | — | Archived | — |
| 24-28. v2.0 cloud-claude | v2.0 | 7/7 | Complete | 2026-04-15 |
| 29-35. v3.0 远端开发体验升级 | v3.0 | 30/30 | Complete | 2026-04-23 |
| 36-37. v3.1 映射语义补齐与懒加载 | v3.1 | 11/11 | Complete | 2026-04-24 |
| 38-44. v3.4 多形态容器接入 | v3.4 | 14/14 | Complete | 2026-05-08 |
| 45-47. v3.5 网络白名单与 DNS 拆分解析 | v3.5 | 10/10 | Complete | 2026-05-13 |

---

*Last updated: 2026-05-13 — v3.5 网络白名单与 DNS 拆分解析 shipped (Phases 45-47, 34/34 requirements satisfied, tag v3.5).*
