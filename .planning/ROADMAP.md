# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- ⏸️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (paused)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)

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
<summary>⏸️ v1.3 claude-shell 本地透明代理 (Phases 17-23) — PAUSED</summary>

- [ ] Phase 17: 镜像与 Entrypoint 基线
- [ ] Phase 18: 网络隔离与分流
- [ ] Phase 19: CLI 骨架与 Docker 编排
- [ ] Phase 20: TTY 透传与交互体验
- [ ] Phase 21: 指纹伪造与反检测
- [ ] Phase 22: 验证与自检
- [ ] Phase 23: 混淆构建与交付

</details>

<details>
<summary>✅ v2.0 cloud-claude 透明远程 CLI (Phases 24-28) — SHIPPED 2026-04-15</summary>

- [x] Phase 24: 受管镜像 FUSE 硬化与容器参数 (1/1 plans) — completed 2026-04-14
- [x] Phase 25: cloud-claude CLI 骨架与连接 (1/1 plans) — completed 2026-04-15
- [x] Phase 26: 参数透传与终端体验 (1/1 plans) — completed 2026-04-15
- [x] Phase 27: 双 session 目录映射 (2/2 plans) — completed 2026-04-15
- [x] Phase 28: 生产环境 FUSE 兼容性验证 (2/2 plans) — completed 2026-04-15

</details>

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. 基础控制面与主机代理 | v1.0 | 3/3 | Complete | 2026-03-26 |
| 2. 隧道出网强制层 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 3. 启动入口与 SSH 接入 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 4. 后台管理界面 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 5. 到期、审计与清理 | v1.0 | 3/3 | Complete | 2026-03-27 |
| 6. 加固与 MVP 就绪 | v1.0 | 4/4 | Complete | 2026-03-28 |
| 7. 数据层与类型化 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 8. SingBoxProvider 与受管镜像 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 9. 前端适配与代理测试 | v1.1 | 3/3 | Complete | 2026-03-28 |
| 10. 技术债务清理 | v1.1 | 2/2 | Complete | 2026-03-28 |
| 11. 认证基础设施与数据迁移 | v1.2 | 3/3 | Complete | 2026-03-29 |
| 12. 用户自助 API 与前端路由 | v1.2 | 2/2 | Complete | 2026-03-29 |
| 13-16. v1.2 剩余阶段 | v1.2 | — | Deferred | — |
| 17-23. claude-shell 本地代理 | v1.3 | — | Paused | — |
| 24. 受管镜像 FUSE 硬化 | v2.0 | 1/1 | Complete | 2026-04-14 |
| 25. cloud-claude CLI 骨架 | v2.0 | 1/1 | Complete | 2026-04-15 |
| 26. 参数透传与终端体验 | v2.0 | 1/1 | Complete | 2026-04-15 |
| 27. 双 session 目录映射 | v2.0 | 2/2 | Complete | 2026-04-15 |
| 28. 生产 FUSE 兼容性验证 | v2.0 | 2/2 | Complete | 2026-04-15 |

---
*Last updated: 2026-04-15 — v2.0 shipped*
