# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- ⏸️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (paused)
- 🚧 **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (in progress)

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

**Milestone Goal:** 建立用户认证与自助面板体系，让用户可以独立查看和管理自己的资源；同时重设计 Bootstrap 入口，提升首次接入体验。

- [x] **Phase 11: 认证基础设施与数据迁移** — 用户登录认证体系与 Claude 账号数据模型 (completed 2026-03-29)
- [x] **Phase 12: 用户自助 API 与前端路由** — 用户自助面板骨架与角色路由 (completed 2026-03-29)
- [ ] **Phase 13: 账号管理与用户资源视图** — 账号 CRUD、有效期、售后换号、用户资源汇总 (deferred)
- [ ] **Phase 14: KasmVNC 用户面** — 用户通过面板直接访问远程桌面 (deferred)
- [ ] **Phase 15: Bootstrap 重设计** — 短 URL 入口与实时状态推送 (deferred)
- [ ] **Phase 16: 级联禁用与到期治理** — 用户/账号/主机到期联动与自动关机 (deferred)

### Phase 11: 认证基础设施与数据迁移
**Goal**: 用户可以使用自己的凭证登录系统，系统能区分管理员和普通用户角色，Claude 账号数据模型就绪
**Depends on**: Phase 10 (v1.1 基线)
**Requirements**: AUTH-01, AUTH-02, AUTH-03, CLAUDE-01
**Success Criteria** (what must be TRUE):
  1. 用户使用 short_id + 密码登录后获取 JWT，JWT 中包含 role claim 区分管理员和普通用户
  2. 管理员和用户共用同一登录页面，登录后根据角色自动跳转到对应的面板
  3. 用户 API 请求只能访问自己的资源，尝试访问他人资源返回 403
  4. claude_accounts 表已创建，支持一个用户拥有多个 Claude 账号且每个账号关联一台主机
**Plans**: 3/3 complete

### Phase 12: 用户自助 API 与前端路由
**Goal**: 用户可以在自助面板中查看自己的主机状态、出口 IP 并执行主机重建
**Depends on**: Phase 11
**Requirements**: PANEL-01, PANEL-02, PANEL-03
**Success Criteria** (what must be TRUE):
  1. 用户登录后看到自己的主机列表，包含运行状态和基本信息
  2. 用户可以查看每台主机绑定的出口 IP 信息
  3. 用户可以对自己的主机触发重建操作，重建过程有状态反馈
  4. 用户面板与管理员面板共存于同一 React 应用，通过角色路由守卫隔离
**Plans**: 2/2 complete
**UI hint**: yes

### Phase 13: 账号管理与用户资源视图 (Deferred)
**Goal**: 建立完整的账号生命周期管理体系，每个账号与主机一一绑定，支持备注、有效期、售后换号；用户管理页能直观展示用户拥有的所有资源
**Depends on**: Phase 12
**Requirements**: ACCT-01, ACCT-02, ACCT-03, ACCT-04, ACCT-05

### Phase 14: KasmVNC 用户面 (Deferred)
**Goal**: 用户可以在自助面板中直接通过浏览器访问容器内的远程桌面
**Depends on**: Phase 12
**Requirements**: PANEL-04

### Phase 15: Bootstrap 重设计 (Deferred)
**Goal**: 用户通过更短的 URL 和更流畅的终端交互完成首次接入，全程有实时状态反馈
**Depends on**: Phase 11
**Requirements**: BOOT-01, BOOT-02, BOOT-03, BOOT-04

### Phase 16: 级联禁用与到期治理 (Deferred)
**Goal**: 用户、账号、主机三个维度的到期和禁用实现联动——任一上游实体被禁用或过期时，自动级联关闭其下游资源，保证不留"孤儿"运行容器
**Depends on**: Phase 13
**Requirements**: CASCADE-01, CASCADE-02, CASCADE-03

</details>

<details>
<summary>⏸️ v1.3 claude-shell 本地透明代理 (Phases 17-23) — PAUSED</summary>

- [ ] **Phase 17: 镜像与 Entrypoint 基线** — 容器镜像和启动编排就绪，Claude Code 通过官方安装脚本运行
- [ ] **Phase 18: 网络隔离与分流** — sing-box tun + nftables 全流量代理，公网走出口，私网回连宿主机
- [ ] **Phase 19: CLI 骨架与 Docker 编排** — Go 二进制 `claude` 命令的基础框架、配置和 Docker 编排
- [ ] **Phase 20: TTY 透传与交互体验** — 终端尺寸、信号、退出码完全透传，交互体验无差异
- [ ] **Phase 21: 指纹伪造与反检测** — 系统级设备指纹伪装和容器标记清除
- [ ] **Phase 22: 验证与自检** — verify 子命令一键检测出口 IP、DNS、指纹和容器标记
- [ ] **Phase 23: 混淆构建与交付** — garble 混淆产出可直接替换的单一二进制

</details>

### 🚧 v2.0 cloud-claude 透明远程 CLI

**Milestone Goal:** 交付一个可替代原生 `claude` 命令的 Go 二进制文件 `cloud-claude`，用户 `alias claude=cloud-claude` 后输入 `claude` 的体验与本地完全一致——实际运行在远端配好代理出口的 Docker 容器里，本地目录通过 sshfs slave 实时映射到容器内。

- [x] **Phase 24: 受管镜像 FUSE 硬化与容器参数** — 镜像预装 sshfs/fuse3，Worker 附加 FUSE 设备权限，SSH Proxy 零改造验证 (completed 2026-04-14)
- [x] **Phase 25: cloud-claude CLI 骨架与连接** — Go 二进制 cloud-claude 的配置、认证和远端容器连接闭环（1 plan） (completed 2026-04-15)
- [x] **Phase 26: 参数透传与终端体验** — claude 参数原样透传，TTY/信号/退出码与本地一致 (completed 2026-04-15)
- [ ] **Phase 27: 双 session 目录映射** — sshfs slave + SFTP 实现当前目录到容器 /workspace 的实时双向映射
- [ ] **Phase 28: 生产环境 FUSE 兼容性验证** — 在目标 Linux 环境验证 FUSE + AppArmor/seccomp 完整兼容性

### Phase 24: 受管镜像 FUSE 硬化与容器参数
**Goal**: 容器侧 FUSE/sshfs 前置条件和运行参数就绪，SSH Proxy 零改造验证通过
**Depends on**: Nothing (v2.0 首个阶段，基于 v1.1 受管镜像)
**Requirements**: SRV-01, SRV-02, SRV-03
**Success Criteria** (what must be TRUE):
  1. 受管镜像 docker build 产出的镜像包含 sshfs 和 fuse3，/etc/fuse.conf 中 user_allow_other 已启用
  2. Worker 创建容器时附加 --device /dev/fuse 和 --cap-add SYS_ADMIN，容器内非 root 用户可成功执行 sshfs 挂载
  3. SSH Proxy 现有多 session channel 和 exec 转发能力无需代码改动即可支持 cloud-claude 的连接模式
**Plans**: 1 plan
Plans:
- [x] 24-01-PLAN.md — 镜像 FUSE 预装、容器参数附加与 SSH Proxy 零改造验证

### Phase 25: cloud-claude CLI 骨架与连接
**Goal**: 用户可以运行 cloud-claude 命令完成配置、认证和远端容器连接
**Depends on**: Phase 24
**Requirements**: CLI-01, CLI-02, CLI-04, CLI-05
**Success Criteria** (what must be TRUE):
  1. 用户运行 `cloud-claude`（无参数）后，CLI 自动连接网关、认证、等待容器就绪，并进入远端 Claude Code 会话
  2. 用户运行 `cloud-claude init` 后，网关地址和凭证持久化到 `~/.cloud-claude/config.yaml`，后续运行自动读取
  3. 网关不可达、认证失败、容器未就绪时，CLI 输出清晰的中文错误提示并返回合适的退出码
  4. 用户可以在 config.yaml 中配置自有网关地址，CLI 连接到该地址而非默认地址
**Plans**: 1 plan
Plans:
- [x] 25-01-PLAN.md — cloud-claude 配置/init、Entry 认证与 SSH+PTY 闭环

### Phase 26: 参数透传与终端体验
**Goal**: cloud-claude 的参数透传和终端交互与本地 claude 完全一致
**Depends on**: Phase 25
**Requirements**: CLI-03, TTY-01, TTY-02, TTY-03
**Success Criteria** (what must be TRUE):
  1. 用户传入的所有 claude 参数（如 `-p "prompt"`, `--model`, `--allowedTools` 等）原样传递到容器内 Claude Code，行为与本地一致
  2. 终端窗口 resize 时 SIGWINCH 正确传递到容器内 Claude Code 进程，界面跟随调整
  3. Ctrl+C / Ctrl+\ 等信号正确转发到容器内进程，Claude Code 正常响应中断
  4. 容器内 Claude Code 退出码透传给本地 cloud-claude 进程，脚本可基于退出码判断结果
**Plans**: 1 plan
Plans:
- [x] 26-01-PLAN.md — 参数透传、安全命令构建、非 TTY 分支与退出码修复

### Phase 27: 双 session 目录映射
**Goal**: 用户当前目录通过 sshfs slave 实时映射到容器 /workspace，双向读写可靠
**Depends on**: Phase 26, Phase 24
**Requirements**: MAP-01, MAP-02, MAP-03
**Success Criteria** (what must be TRUE):
  1. 用户运行 cloud-claude 时，CLI 自动在第二个 SSH session 上通过 sshfs slave 将当前目录映射到容器 /workspace
  2. 本地文件修改在容器内即时可见，容器内文件修改在本地即时可见（双向实时读写）
  3. Claude Code 以 /workspace 为工作目录运行，可正常读写项目文件
  4. 会话正常或异常退出时，容器内 sshfs 挂载点和相关资源自动清理
**Plans**: TBD

### Phase 28: 生产环境 FUSE 兼容性验证
**Goal**: 在 Linux 生产环境验证 FUSE + 安全模块兼容性，确保全栈端到端可用
**Depends on**: Phase 27
**Requirements**: SRV-04
**Success Criteria** (what must be TRUE):
  1. 在目标 Linux 宿主机（含 AppArmor 或 seccomp）上，容器内 sshfs 挂载成功且读写正常
  2. FUSE 挂载与 sing-box tun / nftables 默认拒绝策略共存，映射通道不被防火墙阻断
  3. 完整流程（cloud-claude → SSH Proxy → 目录映射 → Claude Code 运行）在生产环境端到端通过
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 24 → 25 → 26 → 27 → 28

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 24. 受管镜像 FUSE 硬化与容器参数 | 1/1 | Complete    | 2026-04-14 |
| 25. cloud-claude CLI 骨架与连接 | 1/1 | Complete    | 2026-04-15 |
| 26. 参数透传与终端体验 | 1/1 | Complete   | 2026-04-15 |
| 27. 双 session 目录映射 | 0/0 | Not started | - |
| 28. 生产环境 FUSE 兼容性验证 | 0/0 | Not started | - |

---
*Last updated: 2026-04-15 — v1.2 deferred, v2.0 active*
