# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- ⏸️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (paused)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- 🚧 **v3.0 远端开发体验升级** — Phases 29-35 (in progress)

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

<details open>
<summary>🚧 v3.0 远端开发体验升级 (Phases 29-35) — IN PROGRESS</summary>

**Milestone Goal:** 在已 ship 的 v2.0 cloud-claude 基础上做体验升级——三层文件系统（Mutagen + sshfs + mergerfs）、SSH 会话弱网容忍 + tmux 默认包装与多端共享、Claude Code OAuth 状态持久化、`cloud-claude doctor` 与统一错误码体系——把 CLI 从"能跑"升级到"能成为日常开发主战场"。零增量特权，复用 v2.0 已开放的 FUSE / SYS_ADMIN / AppArmor unconfined 通道。

- [ ] **Phase 29: 受管镜像 v3 + Worker 容器参数扩展** — Dockerfile 加 mergerfs/mutagen-agent、entrypoint 串行编排、Worker `Volumes` 字段、`image.lock` 凸 v3.0.0、AppArmor 部署文档 (0/3 plans)
- [ ] **Phase 30: 控制面数据模型 + Entry API 扩展** — `claude_accounts.persistent_volume_name` migration、`HostActionRequest` 扩展、Entry API 返回 `image_version` / `supports_*` / `claude_account_id`（向后兼容） (0/2 plans，已规划见下)
- [ ] **Phase 31: CLI 三层文件映射重构** — cloud-claude 拆分 `mount_strategy`/`mount_mutagen`/`mount_sshfs`/`mount_merge`，实现 `--mount-mode` 降级状态机、Mutagen 白名单 + ignore、Mutagen ‖ sshfs 并发 (0/3 plans，已规划见下)
  Plans:
  - [ ] `.planning/phases/31-cli/plans/01-errcodes-mutagen-embed/PLAN.md` — errcodes 注册表雏形（15 条）+ Mutagen v0.18.1 4 平台 go:embed + 跨平台 case-insensitive Go probe（Wave 1）
  - [ ] `.planning/phases/31-cli/plans/02-mount-three-layer/PLAN.md` — mount.go 拆 4 文件 + `--mount-mode` 状态机 + 安全门 + 50MB 拒绝 + askpass / sshfs 抖动 watcher / last-session.json + `ConnectAndRunClaudeV3` + cobra flag（Wave 2）
  - [ ] `.planning/phases/31-cli/plans/03-oauth-conflicts-integration/PLAN.md` — OAuth 三态检查 + Mutagen conflict 冒泡（--template）+ `cloud-claude sync conflicts` 子命令 + 6 个集成测试 + docker compose fixture（Wave 3）
- [x] **Phase 32: SSH 会话可靠性 + tmux 包装 + 多端** — `session.go` tmux 决策、KeepAlive + 退避重连、`--new-session`/`--take-over`、多端 banner、账号级 Mutagen 单例锁 (3/5 plans 完成 + 2 个 gap-closure 待执行) (completed 2026-04-20)
  Plans:
  - [x] `.planning/phases/32-ssh-tmux/plans/01-net-resilience/PLAN.md` — KeepAlive 应用层 + TCP 平台特化 + reconnect 退避状态机 + input_buffer 灰色未确认 + 10 条新错误码 + colors/last_session 字段（Wave 1）
  - [x] `.planning/phases/32-ssh-tmux/plans/02-tmux-multiclient/PLAN.md` — session.go tmux 包装 + take-over + 多端 banner + sessions ls/attach 子命令 + ConnectAndRunClaudeV3 路由 + cmd flag 剥离与 KeepAlive 校验（Wave 2）
  - [x] `.planning/phases/32-ssh-tmux/plans/03-sync-lock-integration/PLAN.md` — 账号级 flock 单例锁 + ssh.go 注入 + secondary 标志 + 6 个 TestIntegration_Phase32_* + C3/C7 回归（Wave 3）
  - [ ] `.planning/phases/32-ssh-tmux/plans/04-mount-strategy-sync-lock-invoke/PLAN.md` — gap_closure #2 闭合 SC11 / REQ-F5-D：MountWorkspace 真实 invoke mountCfg.SyncSessionLock + ErrSyncLocked 降级 ModeSSHFSOnly + DowngradeChain 追加 sync_locked（Wave 1 of gap batch）
  - [ ] `.planning/phases/32-ssh-tmux/plans/05-bufferedstdin-reconnect-wiring/PLAN.md` — gap_closure #1 闭合 SC5 / REQ-F3-B：Reconnector + BufferedStdin 单例提升到 runClaudePTYWithReconnect 外层通过 reconnector.StateAddr() 共享 atomic.Int32 + onReconnected 回调 bs.Flush() + WR-03/WR-04 co-fix（Wave 1 of gap batch）
- [ ] **Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）** — entrypoint symlink `/var/lib/claude-persist`、Worker `docker volume create` 幂等、admin DELETE 事务联动 `volume rm` (0/2 plans)
- [ ] **Phase 34: cloud-claude doctor v3 + 错误码统一** — `doctor` 5 维度子命令 + `--fix`/`--json`、统一错误码 `<DOMAIN>_<KIND>_<NUM>`、`cloud-claude explain` (0/3 plans)
- [ ] **Phase 35: E2E 稳定化 + 性能验收** — `rg`/`ls -R` 10k 文件基准、拔网 10s/30s/2min UAT、首连 ≤ 8s 验收、APFS + Ubuntu 25.04 真机、image ≤ 700MB CI gate、运维手册更新 (0/2 plans)

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
| 29. 受管镜像 v3 + Worker 容器参数 | v3.0 | 6/6 | Complete    | 2026-04-18 |
| 30. 控制面数据模型 + Entry API | v3.0 | 2/2 | Complete    | 2026-04-18 |
| 31. CLI 三层文件映射重构 | v3.0 | 3/3 | Complete   | 2026-04-19 |
| 32. SSH 会话可靠性 + tmux + 多端 | v3.0 | 0/0 | Complete    | 2026-04-20 |
| 33. Claude Code 状态持久化 | v3.0 | 0/2 | Pending | — |
| 34. cloud-claude doctor v3 + 错误码统一 | v3.0 | 0/3 | Pending | — |
| 35. E2E 稳定化 + 性能验收 | v3.0 | 0/2 | Pending | — |

## v3.0 Phase Details

> 本节由 `gsd-roadmapper` 写入，下游 `gsd-plan-phase` 读取。每个 phase 的 Goal 引用 REQ-ID 与 Critical Pitfall 编号，Success criteria 给出可观察 / 可断言的验证手段，Open questions 列出 `discuss-phase` 必须 surface 的议题（来源：REQUIREMENTS.md §Open Questions）。

### Phase 29: 受管镜像 v3 + Worker 容器参数扩展
**Goal**: 交付 v3.0 受管镜像基线（mergerfs 2.41.1 + mutagen-agent v0.18.1 + tmux 3.6a 核对 + 严格的 mount/启动参数），并扩展 Worker 容器创建以接受 `Volumes` 字段。本阶段是 F1（三层文件映射）/ F4（tmux 包装）/ F7（持久化 volume）/ BASE-04（镜像 ≤ 700MB CI gate）的镜像侧基础，必须一次性防御 Critical Pitfalls C1 / C2 / C3 / C5 / C6 / C7。
**Depends on**: 无（与 Phase 30 并行）
**Requirements**: BASE-04（镜像体积 CI gate）。同时为 F1/F2/F4/F7 提供镜像侧前置条件，但用户可观察的 REQ-F* 行为在 Phase 31/32/33 交付。
**Scope**:
- `deploy/docker/managed-user/Dockerfile`：mergerfs 2.41.1 GitHub static `.deb` 安装（**禁止 apt**，反 PITFALLS M3）、mutagen-agent v0.18.1 tarball 预放、tmux 版本核对、libfuse3 3.18.x、`/etc/cloud-claude/mutagen.version` 写入
- Dockerfile 预建 `/home/claude/.claude` `/home/claude/.cache/claude` `/workspace-hot` `/workspace-cold` `/workspace` 并 `chown 1000:1000`（防 PITFALLS C5 / M17）
- `entrypoint.sh`：串行 `prepare-fuse → chown → mutagen-agent → mergerfs → wait → exec sshd`（PITFALLS M4），mergerfs 显式 `category.create=ff`、`func.readdir=cor:4`、`cache.attr=30`、`cache.entry=30`、`cache.readdir=true`、`cache.files=off`、`inodecalc=path-hash`、branches `=RW:NC,RO`
- `/etc/tmux.conf`：`set -ga terminal-overrides ",*:RGB"` + `window-size latest` + `aggressive-resize on`；`/etc/profile.d/cloud-claude.sh` 暴露 `CLAUDE_CODE_TMUX_TRUECOLOR=1`（PITFALLS M7 / M8）
- `sshd_config`：确认 `ClientAliveInterval 15` + `ClientAliveCountMax 8`、`MaxSessions 30`、`MaxStartups 60:30:120`（PITFALLS M12，REQ-F3-A 的服务端基线）
- 容器不跑 systemd，PID 1 = `tini` 间接守护 tmux（PITFALLS C7）
- `image.lock` 凸至 `v3.0.0`
- `internal/agentapi/contracts.go:HostActionRequest`：新增 `Volumes []VolumeMount` 字段解析（仅类型与 host-agent 解析，volume 创建逻辑放 Phase 33）
- `internal/runtime/tasks/worker.go:createHost`：`docker create` args 拼接接受 `--mount type=volume,...` + label
- 部署文档：Ubuntu 25.04 AppArmor `local override`（`capability dac_override,`）+ `host-preflight.sh` 检测脚本（PITFALLS C6）
- BuildKit cache mount + `--no-install-recommends`，CI gate 断言镜像未压缩 ≤ 700MB（PITFALLS M18 / BASE-04）
**Success Criteria** (what must be TRUE):
  1. `docker build` 产出的镜像 `mount | grep mergerfs` 在容器内必须包含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff`（C1+C2 验证手段）
  2. `getfattr -n user.mergerfs.branches /workspace/.mergerfs` 返回明确的 `RW`（hot）+ `NC,RO`（cold）两 branch（C2 验证）
  3. 容器内 `/home/claude/.claude` 存在且属主为 `1000:1000`，`mutagen-agent --version` 等于 `/etc/cloud-claude/mutagen.version` 内容（C5 / C4 前置）
  4. 容器内 `tmux -V` ≥ 3.6a，`/etc/tmux.conf` 含 `terminal-overrides ",*:RGB"` 与 `window-size latest`（M7 / M8）
  5. 容器内 PID 1 为 `tini`，无 `systemd` / `systemd-logind`（C7）
  6. CI 镜像构建产物 `docker image inspect --format='{{.Size}}'` < 700×1024×1024（BASE-04）
  7. `host-preflight.sh` 在 Ubuntu 25.04 检测到缺失 AppArmor override 时退出码 = 1 并输出修复命令（C6）
**Open questions to resolve**: Q10（mergerfs branch 是 2 路还是 3 路 — 本阶段先实现 2 路，但 entrypoint 编排需为 3 路保留扩展点）

### Phase 29.1: 修复 GetHost 缺失 entry_password 字段导致容器密码退化为 workspace (INSERTED)

**Goal:** 修复 `GetHost` SELECT 漏 `entry_password` 导致的全链路密码退化：(1) 新建容器 `CONTAINER_SSH_PASSWORD` 与 DB `hosts.entry_password` 严格一致，杜绝 `"workspace"` 字面量 fallback；(2) 仓储层 6 个 Host 读函数整批补齐 `entry_password` 列；(3) worker / runtime 任何 `EntryPassword == ""` 路径改 fail-fast + audit event；(4) entrypoint 新增 `passwd -S` 自检，容器宁可起不来也不起"伪成功"；(5) admin 批量 resync 端点一键修复存量运行容器；(6) 仓储 / worker / handler 三层回归测试 + 端到端人工 UAT。
**Requirements**: P0-HOTFIX-29.1（无正式 REQ-ID，线上紧急修复）
**Depends on:** Phase 29
**Plans:** 2/4 plans executed

Plans:
- [ ] `.planning/phases/29.1-gethost-entry-password-workspace/29.1-01-PLAN.md` — 仓储层：6 个 Host 读函数 SELECT + Scan 补 `entry_password`，SQL 提升为包级常量，新增 `TestAllHostReadQueriesIncludeEntryPassword` 契约测试（Wave 1）
- [ ] `.planning/phases/29.1-gethost-entry-password-workspace/29.1-02-PLAN.md` — runtime/worker fail-fast：`QueueHostAction` / `buildCreateArgs` / `syncContainerCredentials` 去除 `"workspace"` 密码 fallback，改 RecordEvent + error，新增 2 条 worker 单测（Wave 1）
- [ ] `.planning/phases/29.1-gethost-entry-password-workspace/29.1-03-PLAN.md` — 镜像自检：`entrypoint.sh` 在 chpasswd 后追加 `passwd -S` 自检，非 `P|PS` 状态 `exit 1`（Wave 1）
- [ ] `.planning/phases/29.1-gethost-entry-password-workspace/29.1-04-PLAN.md` — 存量修复 + e2e：新增 `POST /v1/admin/hosts/resync-passwords` admin 端点（复用 adminGuard），`syncContainerPassword` 提升为 `var` 便于 mock，3 条 handler 单测 + 人工 UAT（Wave 2，depends_on 01+02）

### Phase 30: 控制面数据模型 + Entry API 扩展
**Goal**: 为 v3.0 体验所需的"客户端动态能力探测"打开控制面通道——`claude_accounts.persistent_volume_name` 字段就绪、`HostActionRequest` 在 API 契约层接收 `ClaudeAccountID + Volumes`、`Entry API` 在现有 `/v1/entry/{id}/auth` 响应里追加 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id` 字段（向后兼容，旧 client 不破）。本阶段不交付任何 user-facing REQ-F*，但 Phase 31/33 全部依赖它。
**Depends on**: 无（与 Phase 29 并行）
**Plans:** 2/2 plans complete
Plans:
- [ ] `.planning/phases/30-entry-api/plans/01-migration-entry-store/PLAN.md` — migration `0014` + `ClaudeAccount`/`HostSSHAuth` 仓储与 `ResolveClaudeAccountIDForEntry`（D-01/D-02/D-05/D-10）
- [ ] `.planning/phases/30-entry-api/plans/02-entry-api-host-contract/PLAN.md` — `HostActionRequest` + Entry `Auth` + `cloudclaude.AuthResponse` 与兼容测试（D-03..D-09）
**Requirements**: 为 F1/F2 (Phase 31)、F7 (Phase 33) 提供握手 / 数据模型支持。本阶段无独占 REQ；唯一直接涉及的字段是 REQ-F7-A 中的 volume 命名约定（`claude-state-{claude_account_id}` + label `com.cloud-cli-proxy.account_id`）的数据模型落地。
**Scope**:
- `internal/store/migrations/0014_claude_account_persistent_volume.sql`：新增字段 + 默认值 + 索引
- `internal/agentapi/contracts.go:HostActionRequest`：新增 `ClaudeAccountID string` 与 `Volumes []VolumeMount`（与 Phase 29 host-agent 端解析对齐）
- `internal/controlplane/http/entry.go:Auth`：在响应里 join 查询 claude_account 与 image version，追加 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id` 字段；JSON `omitempty` 保旧 client 兼容
- `internal/cloudclaude/entry.go:AuthResponse`：扩展同名字段（指针或 `omitempty`）；旧字段不动
- 单元测试：旧 v2.0 client 反序列化新响应不报错；新字段缺失时降级到默认值
**Success Criteria** (what must be TRUE):
  1. migration `0014` 在干净库 + v2.0 库上均能 up/down 幂等执行
  2. v2.0 客户端二进制（旧 `AuthResponse` 结构）调用新版 Entry API `/v1/entry/{id}/auth` 不返回错误，未知字段被忽略
  3. v3.0 客户端能正确读到 `image_version="v3.0.0"` / `supports_mutagen=true` / `supports_mergerfs=true` / `claude_account_id="<uuid>"`
  4. `HostActionRequest` 在 host-agent 端解析 `Volumes` 字段且不引入新增 endpoint（沿用现有 `/agent/host/action`）
**Open questions to resolve**: Q4（persistent volume 命名规范：单 volume `claude-state-{account_id}` vs 双 volume — 本阶段必须定稿，因为 migration 字段命名与之绑定）；Q5（在现有 endpoint 加字段 vs 新增 `/capabilities` endpoint，倾向 (a) 加字段）；Q6（host-agent 是否扩展返回 image labels，倾向不扩展）

### Phase 31: CLI 三层文件映射重构
**Goal**: 把 cloud-claude 从 v2.0 单 sshfs 升级为 **三层文件映射**（Mutagen 热同步 + sshfs 冷兜底 + mergerfs 联合视图），实现 `--mount-mode=auto|full|mutagen-only|sshfs-only` 降级状态机；本阶段是 v3.0 最大的技术风险点（**Critical Pitfalls C1 / C2 / C3 / C5 / M13 必须在此一次性防御到位**）。交付 F1（三层文件系统）+ F2（降级路径），并把 F7-C（连接前 OAuth 过期警告）织入连接握手阶段。
**Depends on**: Phase 29（镜像 mergerfs / mutagen-agent 就绪）、Phase 30（Entry API 返回 `supports_mutagen` / `supports_mergerfs`）
**Requirements**:
- F1：REQ-F1-A、REQ-F1-B、REQ-F1-C、REQ-F1-D、REQ-F1-E
- F2：REQ-F2-A、REQ-F2-B、REQ-F2-C
- F7：REQ-F7-C（连接前 credentials 过期中文提示，发生在 CLI 连接握手期间）
**Scope**:
- 拆分 `internal/cloudclaude/mount.go` 为：`mount_sshfs.go`（v2.0 逻辑保留作降级分支）、`mount_mutagen.go`（Mutagen daemon + sync session 管理 + 版本握手）、`mount_merge.go`（mergerfs 校验）、`mount_strategy.go`（`--mount-mode` 降级状态机）
- Mutagen 客户端二进制 `go:embed` 集成（Q1 倾向 (a)）；daemon 长期复用（Q2 倾向 (a)，需多 cloud-claude 并发 daemon 锁）
- 启动时 `mutagen version` 与容器内 `/etc/cloud-claude/mutagen.version` 比对，不一致直接降级 sshfs-only + 错误码 `MOUNT_MUTAGEN_VERSION_SKEW`（C4）
- Mutagen 同步前 `du -sb` 候选目录，> 50MB 拒绝热同步 + 自动降级 sshfs（REQ-F1-D）
- alpha 空 + beta 非空安全门：拒绝并报错 `MOUNT_MUTAGEN_SAFETY_GUARD`（C5 + Q7）
- 默认 ignore：`.git/`、`node_modules/`、`target/`、`dist/`、`*.pyc`
- 强制 `--default-owner-beta=id:1000 --default-group-beta=id:1000`，mode 默认值由 Q3 在 discuss 阶段定调
- 三段式中文进度输出 `初始化文件映射 (1/3) 热同步源码中…` / `(2/3) 启动冷兜底…` / `(3/3) 合并视图…`（REQ-F1-B）
- macOS APFS case-insensitive 启动检测 + 强制 `--mode=two-way-resolved`（PITFALLS M5）
- 降级状态机：任一层失败 ≤ 2s 内降级到下一档；**禁止静默降级**——stderr 中文输出当前生效模式 + 错误码（REQ-F2-B / M13）
- 每次连接成功 banner 显示彩色当前 mount 模式标签，尊重 `NO_COLOR`（REQ-F2-C）
- Mutagen conflict 冒泡：下次回车前 prompt 上方插入中文警告 `⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`（REQ-F1-E）
- 连接握手期间检查 Entry API 返回的 `claude_account_id` 对应 OAuth credentials 状态，过期或即将过期时连接前给出明确中文提示，**禁止让 claude 进程先报错**（REQ-F7-C）
- 启动并发改造：`Mutagen sync create ‖ sshfs mount` 走独立 SSH channel，对应 `/mnt/hot` vs `/mnt/cold`（SUMMARY §4.3 时序）
- 监控 sshfs 抖动：sshfs 必须含 `reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10`，CLI 监测 sshfs 异常时主动从 mergerfs branch 摘除以避免整体挂死（C3）
**Success Criteria** (what must be TRUE):
  1. 在 10k 文件源码树执行 `rg .` 与 `ls -R /workspace`，平均延迟 ≤ 同机本地等价操作的 1.5×（REQ-F1-C，对接 BASE-01）
  2. 首次 `cloud-claude` 命令执行 → 可输入 prompt 总耗时 ≤ 8s（REQ-F1-B，对接 BASE-02），过程中三段式中文进度输出可见
  3. 候选目录 > 50MB 时 CLI 拒绝热同步、自动改 sshfs 兜底，stderr 输出 `MOUNT_MUTAGEN_WHITELIST_REJECT` 错误码与中文 ignore 配置建议（REQ-F1-D）
  4. 强 kill 容器内 `mutagen-agent` 后 ≤ 2s 内 cloud-claude stderr 中文输出降级到 sshfs-only + 错误码，banner 显示 `[sshfs-only]` 标签；**任何静默降级都视为失败**（REQ-F2-B / M13 验收）
  5. `mutagen` 客户端 / 容器内 agent 版本不一致时直接降级 sshfs-only 并输出 `MOUNT_MUTAGEN_VERSION_SKEW`（C4）
  6. alpha 空 + beta 非空场景下 CLI 必须中止并输出 `MOUNT_MUTAGEN_SAFETY_GUARD`，**不允许执行 sync**（C5 验证）
  7. `--mount-mode=full|mutagen-only|sshfs-only|auto` 四档行为与文档一致，banner 标签与实际 mount 状态一致；`NO_COLOR=1` 时关色（REQ-F2-A / REQ-F2-C）
  8. Mutagen 出现 conflict 时下次回车前 prompt 上方插入中文警告，且 `cloud-claude sync conflicts` 能列出文件清单（REQ-F1-E）
  9. 容器 OAuth credentials 已过期场景下，cloud-claude 在 SSH 握手成功后、启动 claude 进程前给出明确中文提示，退出码非 0 且不会进入 claude 报错（REQ-F7-C）
  10. sshfs 抖动 30s 后 `/workspace` 不应整体 hang，`Ctrl-C` 必须可恢复（C3 UAT）
**Open questions to resolve**: Q1（Mutagen 二进制分发，倾向 `go:embed`）、Q2（Mutagen daemon 生命周期，倾向长期复用 + 并发锁）、Q3（默认同步模式 `two-way-safe` vs `two-way-resolved`，**必须本阶段 discuss 拍板**）、Q7（首次同步前 alpha/beta 安全门 vs ≤ 8s 基线权衡，倾向"是 + 但不阻塞"）、Q10（mergerfs 2 路 vs 3 路，与 Phase 29 一并定稿）

### Phase 32: SSH 会话可靠性 + tmux 包装 + 多端
**Goal**: 把 SSH 会话从"断线重来"升级为"30s 抖动无感知 + 进程不丢 + 多端共享 attach"。交付 F3（SSH 弱网容忍 + 自动重连）+ F4（tmux 默认包装 + 会话恢复）+ F5（多端同账号 attach），并在容器侧账号粒度强制 Mutagen 单例锁防止双向冲突累积。本阶段必须防御 Critical Pitfalls C3（sshfs 抖动级联）+ C7（systemd-logind 杀 tmux）。
**Depends on**: Phase 31（复用 CLI mount/降级架构与并发 channel 模型）
**Requirements**:
- F3：REQ-F3-A、REQ-F3-B、REQ-F3-C、REQ-F3-D
- F4：REQ-F4-A、REQ-F4-B、REQ-F4-C
- F5：REQ-F5-A、REQ-F5-B、REQ-F5-C、REQ-F5-D
**Scope**:
- 新增 `internal/cloudclaude/session.go`：tmux `has-session ? attach : new` 决策、`--new-session` / `--take-over` 处理、多端 attach banner 渲染、容器内 tmux 不可用降级（不阻塞 SSH 启动 + banner 提示）
- `ssh.go:runClaude` 远程命令改造：`exec tmux new-session -A -s claude-<id>` 包装，session 命名沿用 Q8 决议（倾向 per-claude_account）
- SSH `KeepAlive` 改造：客户端 `ServerAliveInterval=15` / `ServerAliveCountMax=4`；CLI 启动时校验 < 15s 直接报错（REQ-F3-A / PITFALLS M11）
- TCP 层：socket 启用 `SO_KEEPALIVE`，Linux `TCP_USER_TIMEOUT=30000` / macOS `TCP_KEEPALIVE`
- 自实现 reconnect 退避：`1s → 2s → 4s → 8s → 30s 上限`，复用本地缓存 Entry API token，**不重新弹密码**（REQ-F3-D）
- 本地输入缓冲：断网期间灰色"未确认"样式渲染键入字符，重连后按序提交（REQ-F3-B）
- UX 阈值渲染：`>1.5s` 灰色 `…` / `>8s` 黄色 `网络抖动中（N 秒未响应）` / `>30s` 红色 `网络已断 N 秒，正在自动重试…`
- 重连失败 prompt：具体失败原因 + 下一步操作（按 Enter 重试 / 运行 `cloud-claude doctor`）（REQ-F3-C）
- 多端共享 attach：默认行为不踢人不报错；第二端 banner 中文显示其它 client 来源 + 活跃时间（REQ-F5-A / REQ-F5-B）
- `--new-session` 创建独立 session 命名 `claude-<short_id>`；`--take-over` 强制独占并通知其它端，冲突时返回明确中文提示（REQ-F5-C）
- 账号级 Mutagen 单例锁：同一 claude_account 任意时刻最多 1 个 Mutagen sync session；后连端只 attach tmux + 观察文件，不参与文件同步（REQ-F5-D / PITFALLS M15）
**Success Criteria** (what must be TRUE):
  1. 客户端配置任何 `ServerAliveInterval < 15s` 必须启动失败并输出明确错误（REQ-F3-A）
  2. UAT：断网 30s 内 cloud-claude 不退出、运行中 claude 进程不丢、tmux session 可重连 attach 同一会话且历史 buffer 完整（REQ-F4-A，对接 BASE-03）
  3. UAT：`ssh container 'tmux new -d -s test; sleep 1; pkill -SIGHUP sshd'` 后重连 `tmux attach -t test` 必须成功（C7 验证）
  4. 重连过程实测退避序列 `1s → 2s → 4s → 8s → 30s 上限`，且不重新弹密码（REQ-F3-D）
  5. 断网时本地键入字符以灰色未确认样式显示，重连后按序提交，无丢字 / 无乱序（REQ-F3-B）
  6. 重连最终失败 prompt 必须显示原因 + 下一步操作两要素（REQ-F3-C）
  7. 容器内 tmux 不可用场景：cloud-claude 不阻塞启动，banner 明确输出 `[!] 容器内 tmux 不可用，会话恢复已禁用`（REQ-F4-C）
  8. `cloud-claude sessions ls` / `attach <name>` 列出并可切换多个 session（REQ-F4-B）
  9. 多端默认共享 attach：第二端 banner 显示 `✓ 已 attach 到会话 claude-<id>（另 N 个会话正在共享：<source> / <时间>）`，无强制踢人（REQ-F5-A / REQ-F5-B）
  10. `--new-session` 创建命名为 `claude-<short_id>` 的独立 session；`--take-over` 强制独占并向其它端发送通知；冲突时中文提示明确（REQ-F5-C）
  11. 同一 claude_account 第二个 cloud-claude 启动时，Mutagen sync 不重复创建（账号级单例锁），后连端只读 sshfs / mergerfs 视图（REQ-F5-D）
  12. sshfs 抖动 30s 后 mergerfs 不整体挂死（与 Phase 31 联合验收 C3）
**Open questions to resolve**: Q8（tmux session 命名 per-user vs per-claude_account，倾向 per-claude_account 与 volume 一致）

### Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）
**Goal**: 让 OAuth credentials 与 Claude Code 缓存跨容器重建持久化——单 Docker named volume 按 claude_account 隔离、entrypoint symlink 兜底权限、admin DELETE 事务联动 `volume rm` 防止 orphan 撑爆磁盘。交付 F7 完整闭环（除 F7-C 已落 Phase 31）。
**Depends on**: Phase 29（镜像 entrypoint / 预建目录就绪）、Phase 30（Entry API 返回 `claude_account_id` + `HostActionRequest.Volumes` 契约）
**Requirements**:
- F7：REQ-F7-A、REQ-F7-B、REQ-F7-D（REQ-F7-C 已在 Phase 31 交付）
**Scope**:
- `deploy/docker/managed-user/entrypoint.sh`：symlink `/var/lib/claude-persist/.claude` ↔ `/home/claude/.claude` 与 `/var/lib/claude-persist/.cache/claude` ↔ `/home/claude/.cache/claude`；二次 `chown -R 1000:1000 /home/claude` 兜底（C5 / M17）
- `internal/runtime/tasks/worker.go:createHost`：`docker volume create claude-state-{claude_account_id} --label com.cloud-cli-proxy.account_id=<id> --label managed=true`（幂等），`--mount type=volume,src=claude-state-{id},dst=/var/lib/claude-persist`
- `internal/controlplane/http/...`：admin DELETE claude_account handler 在事务内调用 host-agent `volume rm`（按 label 过滤），失败回滚（PITFALLS M16）
- 可选：admin host 详情页加 `volume_name` 一行展示（OOS-A19 边界内的"最多加一行"）
- 单元测试：volume 创建幂等、label 一致性、事务联动；UAT：删 account 后 `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 必须空
**Success Criteria** (what must be TRUE):
  1. 同一 claude_account 容器删除并重建后，`~/.claude/.credentials.json` 内 OAuth token 保留，再次运行 cloud-claude 无需重新 `claude login`（REQ-F7-B）
  2. `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 返回唯一 volume `claude-state-<id>`（REQ-F7-A 命名 + label）
  3. 容器内 `/home/claude/.claude` `/home/claude/.cache/claude` 属主始终为 `1000:1000`，无权限错误（C5 / M17）
  4. admin DELETE claude_account 后，事务结束时对应 volume 已被 `volume rm`，磁盘无 orphan（REQ-F7-D / M16）
  5. host-agent 重复收到同一 `Volumes` 创建请求时 `docker volume create` 幂等返回成功，不报 `volume exists` 错误
  6. 删除 account 时如果 host-agent `volume rm` 失败（如容器仍持有），事务回滚且明确日志记录，不留半成品状态
**Open questions to resolve**: Q4（volume 命名规范，本阶段必须最终定稿——倾向单 volume `claude-state-{account_id}` 与 Phase 30 migration 字段对齐）

### Phase 34: cloud-claude doctor v3 + 错误码统一
**Goal**: 把 v3.0 引入的所有错误路径统一到 `<DOMAIN>_<KIND>_<NUM>` 错误码体系（新增 `MOUNT_*` / `SESSION_*` / `NET_*` / `STATE_*` 前缀），同时把 `cloud-claude doctor` 升级为覆盖 5 维度的可观测工具，并提供 `cloud-claude explain <code>` 子命令。本阶段是 F8 横切关注点的"收口"phase，必须防御 Critical Pitfalls C2（doctor 断言 mergerfs 参数）+ C4（Mutagen 版本漂移）+ C6（AppArmor）+ C8（错误码命名空间冲突）+ M13（降级历史可见）+ M14（doctor 必须给修复命令）。
**Depends on**: Phase 31、Phase 32、Phase 33（所有错误路径已落码完毕，doctor 才能完整覆盖）
**Requirements**:
- F6：REQ-F6-A、REQ-F6-B、REQ-F6-C、REQ-F6-D
- F8：REQ-F8-A、REQ-F8-B、REQ-F8-C
**Scope**:
- 错误码注册表：统一 `<DOMAIN>_<KIND>_<NUM>` 格式，新增前缀 `MOUNT_*` / `SESSION_*` / `NET_*` / `STATE_*`；与 v2.0 已有 7 个错误码命名空间无冲突（C8）
- 错误输出三要素：错误码 + 中文原因 + 中文下一步建议；CI 单元测试遍历错误码注册表断言 (a) 无重复 (b) 每条有中文消息 (c) 每条有 `next_action` 字段（PITFALLS C8 / REQ-F8-B）
- `cmd/cloud-claude/main.go` 加 `doctor` 子命令，5 维度子检：`network` / `auth` / `ssh` / `mount`（mutagen + sshfs + mergerfs 三层均独立检查项）/ `disk`（REQ-F6-A）
- 每项检查输出 4 要素：状态符号 `[✓][!][✗]` + 中文原因 + 中文修复建议 + 错误码（REQ-F6-B / M14；CI `grep -L "建议:" doctor-output.txt` 应无命中）
- `doctor --fix` 自动修复至少 5 类失败：mutagen agent 无响应（restart）、FUSE 残留挂载（fusermount -u 清理）、known_hosts 冲突（ssh-keygen -R）、token 过期（refresh）、DNS 缓存污染（flush）（REQ-F6-C）
- `--verbose` / `--json` / `NO_COLOR`；退出码 `0/1/2` 对齐 `brew doctor`（REQ-F6-D）
- doctor 完全本地 + SSH 实现，**不给 host-agent 加 endpoint**（ARCHITECTURE §6 / SUMMARY §4.4）
- doctor 必须显示"降级历史"——若本次 / 上次连接发生过降级，第一屏即展示，避免"用户以为在 full 模式下跑"（M13）
- mergerfs 检查：`getfattr -n user.mergerfs.branches` 断言 RW/NC,RO 三 branch + `mount` 输出含必需参数（C2）
- Mutagen 检查：客户端与容器内 agent 版本比对（C4）
- AppArmor 检查：复用 Phase 29 `host-preflight.sh` 检测逻辑（C6）
- `cloud-claude explain <code>` 子命令：对每个错误码给出详细中文说明 + 常见修复步骤（对标 `rustc --explain`）（REQ-F8-C）
**Success Criteria** (what must be TRUE):
  1. CI 单元测试遍历错误码注册表：无重复 code、每条有非空中文 `message` 与 `next_action`、所有 `MOUNT_*` / `SESSION_*` / `NET_*` / `STATE_*` 前缀与 v2.0 无冲突（REQ-F8-A / REQ-F8-B / C8）
  2. `cloud-claude doctor` 输出 5 维度（network / auth / ssh / mount / disk），每个检查项均有 `[符号] 原因（建议: ... | 错误码: ...）` 四要素（REQ-F6-A / REQ-F6-B）
  3. CI grep `cloud-claude doctor` 输出，所有 `[!]` 与 `[✗]` 行必须含"建议:"子串（M14 验证）
  4. `cloud-claude doctor --fix` 在测试矩阵中能自动修复 ≥ 5 类失败（REQ-F6-C）
  5. `cloud-claude doctor --json` 输出可被 `jq` 直接解析；`NO_COLOR=1` 关闭颜色；退出码 0（全 pass）/ 1（warning）/ 2（fail）（REQ-F6-D）
  6. 强制让 cloud-claude 上次连接静默降级到 sshfs-only 后，`cloud-claude doctor` 第一屏必须展示降级历史（M13 验收）
  7. mergerfs 参数被人为篡改后，`doctor mount` 必须输出错误码 + 修复命令（C2 + M14 联合）
  8. `cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` 等任意错误码均能返回中文详细说明 + 常见修复步骤（REQ-F8-C）
  9. doctor 全程不调用 host-agent 任何新 endpoint（边界守恒）
**Open questions to resolve**: Q9（doctor `--fix` 自动修复幂等性边界 — 默认全幂等，需二次确认走 stdin `y/N` + CI `--yes` 跳过）

### Phase 35: E2E 稳定化 + 性能验收
**Goal**: 把 v3.0 全栈在真实环境跑通并锁定性能基线——10k 文件 `rg`/`ls -R` 1.5× 基线、首连 ≤ 8s 基线、30s 抖动无感知基线、镜像 ≤ 700MB CI gate；macOS APFS 真机 + Ubuntu 25.04 AppArmor 真机；运维手册更新。本阶段是验收 phase，承担 BASE-01 / BASE-02 / BASE-03 全部三条性能基线（BASE-04 已在 Phase 29 CI gate 落地，本阶段二次回归）。
**Depends on**: Phase 29、Phase 30、Phase 31、Phase 32、Phase 33、Phase 34
**Requirements**:
- 性能基线：BASE-01（元数据响应）、BASE-02（首次连接）、BASE-03（弱网容忍）、BASE-04（镜像体积 — 二次回归）
**Scope**:
- 性能基准脚本：10k 文件源码树 `rg .` 与 `ls -R /workspace` 在 mergerfs 与本地 ext4 / APFS 的对比基准，输出比值与 P50/P99
- 首连时序埋点：从 `cloud-claude` 命令执行到 prompt 可输入的总耗时（REQ-F1-B 三段式进度的端到端测量）
- 弱网 UAT 矩阵：拔网 10s（无感知 / mosh-style 缓冲生效）/ 30s（仍无感知 / tmux 不丢）/ 2min（自动重连失败前提示明确，运行中 claude 进程仍在 tmux 内）
- macOS APFS 真机：case-insensitive 双向 `--mode=two-way-resolved` 行为验证（PITFALLS M5）
- Ubuntu 25.04 真机：AppArmor local override 部署 + 嵌套 FUSE 三路并发（sshfs + mutagen-agent + mergerfs）（PITFALLS C6）
- 镜像 ≤ 700MB CI gate 二次回归（BASE-04）
- 静默降级回归（M13）：人为破坏每一层 mount，断言 cloud-claude 必须可见降级
- 运维手册更新：`v3.0 升级指南`、`AppArmor override 部署`、`doctor 排障手册`、`持久卷生命周期与 GC`、`错误码索引`
- v3.0 验收清单：所有 30 个 functional REQ + 4 个 BASE 在真机环境逐项验证签字
**Success Criteria** (what must be TRUE):
  1. **BASE-01**：在 10k 文件源码树（mono-repo 模拟）执行 `rg .` 与 `ls -R /workspace` 的 P50 延迟 ≤ 同机本地等价操作 1.5×，P99 ≤ 2×；测试报告留档
  2. **BASE-02**：5 次连续 `cloud-claude` 冷启动到 prompt 可输入 ≥ 4 次 ≤ 8s，且 100% 输出三段式中文进度；无 1 次进入 claude 报错
  3. **BASE-03**：拔网 30s 内 cloud-claude 与 tmux 内 claude 进程均无感知（运行进程不掉、buffer 不丢）；2min 拔网后 cloud-claude 自动重连成功且 tmux session 完整恢复
  4. **BASE-04**：CI 镜像构建产物未压缩 ≤ 700MB，二次回归通过
  5. macOS APFS case-insensitive 仓库（含 `Foo.txt` / `foo.txt` 冲突文件）双向同步无数据丢失
  6. Ubuntu 25.04 真机 AppArmor override 部署后，三路并发 FUSE mount（sshfs + mutagen-agent + mergerfs）全部成功（C6）
  7. 静默降级回归测试：人为 kill mutagen-agent / umount sshfs / 破坏 mergerfs 参数三种场景下，cloud-claude 100% 在 stderr 输出中文降级说明 + 错误码（M13 终验）
  8. 30 条 functional REQ + 4 条 BASE 在真机验收清单全部签字通过
  9. 运维手册新增 5 章（升级 / AppArmor / doctor / 持久卷 / 错误码），每章可独立 follow
**Open questions to resolve**: 无（本阶段是验收，所有 Open Question 应已在前序 phase 决策完毕；若验收发现回归，回流到对应 phase）

---
*Last updated: 2026-04-18 — v3.0 roadmap drafted (Phases 29-35)*
