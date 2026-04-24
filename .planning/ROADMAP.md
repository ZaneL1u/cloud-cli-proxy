# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- ⏸️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (paused)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- ✅ **v3.0 远端开发体验升级** — Phases 29-35 (shipped 2026-04-23) — [Archive](milestones/v3.0-ROADMAP.md)
- 🟡 **v3.1 映射语义补齐与懒加载** — Phases 36-37 (in progress, defining requirements)

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

## v3.1 映射语义补齐与懒加载 (Phases 36-37) — IN PROGRESS

### Phase 36: 映射前置约束 + sshfs 内核缓存

**Goal:** 把"无约束 mount + 全透传 sshfs"升级为"git 仓库强约束 + 单文件 50MB 熔断 + FUSE page cache 命中"，纯配置/校验/参数级改动，零新增依赖、零跨进程协议变更，独立可发。

**Requirements:** REQ-MOUNT-V31-01 / 02 / 03 / 04 / 05 / 06

**Plans:** 6 plans

Plans:
- [x] 36-01-PLAN.md — errcodes 注册 2 条 + explain 长说明（REQ-06，Wave 1）
- [x] 36-02-PLAN.md — Config.HotSyncMaxFileMB + LastSessionSnapshot.OversizedFiles schema（REQ-02/03，Wave 1）
- [x] 36-03-PLAN.md — hot_sync 单文件熔断 + mount_strategy 持久化（REQ-02/03，Wave 2）
- [x] 36-04-PLAN.md — git 仓库前置约束 + main.go os.Getwd() 时序修正（REQ-01，Wave 2）
- [x] 36-05-PLAN.md — sshfs FUSE page cache 参数 + counting SFTP 单测（REQ-04，Wave 2）
- [ ] 36-06-PLAN.md — doctor mount +5 项 check + CI 闸门验证（REQ-05，Wave 3）

**Success Criteria:**
1. `cd /tmp && cloud-claude` 立即拒绝挂载，stderr 含 `MOUNT_REQUIRE_GIT_REPO` + 中文 next_action，退出码 = `exitConfigError`
2. 60MB fixture 不出现在 hot tree（ssh `find` 验证），cold 视图能 `stat`，`last-session.json::oversized_files` 命中
3. 同会话 `cat` 同一冷文件 2 次，本机 SFTP server read count = 1（fixture 计数器单测）
4. `cloud-claude doctor mount --json | jq '.checks | length'` 比 v3.0 多 5
5. `cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 与 `MOUNT_OVERSIZED_FILE_SKIPPED` 子进程退出 0、长说明 ≥200 字
6. `go test ./...` 全 PASS + `make ci-gate` PASS

### Phase 37: 冷文件读触发晋升 + e2e UAT

**Goal:** 在容器内 cold 分支常驻 inotify watcher，命中读事件后异步把文件经 SFTP 拉到 hot 分支，让"二进制按需读"语义从"sshfs 每次回源"升级为"读一次后变热"；同时落 doctor 可观测、runbook、e2e UAT 三块验收。

**Requirements:** REQ-MOUNT-V31-07 / 08 / 09 / 10 / 11 / 12 / 13 / 14 / 15 / 16

**Success Criteria:**
1. Full 模式 mount 就绪后 `pgrep -f cold-promoter` 非空；mount cleanup 后该进程消失
2. PromotionEngine 单测：相同 path 100ms 内 50 次 enqueue → 实际 SFTP 拉取 1 次（5s 防抖去重）
3. SFTP 拉取失败按 1/2/4s 退避重试，第 3 次失败写 stderr + 加入熔断列表，本次会话不再尝试
4. e2e fixture：首次 `cat fixture.png` → SFTP read count +N；第二次 → SFTP read count 不变
5. `CLOUD_CLAUDE_NO_PROMOTION=1` 启动 → watcher 不启动、`promotion_count = 0`
6. `cloud-claude doctor mount --json` 含 `promoter_alive` / `promotion_queue_depth` / `promotion_total` / `promotion_failed_total` 4 个新 check
7. `docs/runbooks/v31-cold-promotion.md` 满足 PATTERNS Pattern G（头部 + ≥5 章节 + 快速诊断命令小节）
8. `tests/scripts/uat-v31-promotion.sh --dry-run` 默认安全；`--confirm-destructive` 全场景 PASS；CI 接入 `make ci-gate`

**Phase 37 真机签字（PR 合并前）:** macOS（Apple Silicon）+ Linux（Ubuntu 24.04）双平台跑一遍 UAT 脚本 + 录屏，参考 `35-HUMAN-UAT.md` 流程。

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
| 24-28. v2.0 cloud-claude 全部 | v2.0 | 7/7 | Complete | 2026-04-15 |
| 29-35. v3.0 远端开发体验升级 | v3.0 | 30/30 | Complete | 2026-04-23 |
| 36. 映射前置约束 + sshfs 内核缓存 | v3.1 | 6/6 | Complete | 2026-04-23 |
| 37. 冷文件读触发晋升 + e2e UAT | 5/5 | Complete    | 2026-04-24 | — |

---

*Last updated: 2026-04-24 — Phase 37 全部 5/5 plans 完成 (37-01 ColdPromoter 核心引擎 / 37-02 tryModeReal 集成 / 37-03 error code + doctor 可观测 / 37-04 runbook / 37-05 e2e UAT 脚本 + CI 接入)。v3.1 冷文件晋升 + e2e UAT 里程碑达成。*
