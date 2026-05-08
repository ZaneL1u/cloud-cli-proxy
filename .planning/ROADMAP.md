# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- 🗃️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (archived, capabilities merged into v3.2)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- ✅ **v3.0 远端开发体验升级** — Phases 29-35 (shipped 2026-04-23) — [Archive](milestones/v3.0-ROADMAP.md)
- ✅ **v3.1 映射语义补齐与懒加载** — Phases 36-37 (shipped 2026-04-24) — [Archive](milestones/v3.1-ROADMAP.md)
- 🚧 **v3.2 多形态容器接入** — Phases 38-41 (in progress)

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
<summary>🗃️ v1.3 claude-shell 本地透明代理 (Phases 17-23) — ARCHIVED, capabilities merged into v3.2</summary>

**Archive reason:** v1.3 的"本地容器"和"指纹伪装"能力与 v3.2 的 `cloud-claude local` 目标重叠。为避免维护两套本地 Docker 编排逻辑，v1.3 能力拆分并入 v3.2：
- Phase 17 镜像基线 → 已并入 managed-user 镜像（v3.0+）
- Phase 18 网络隔离 → 并入 v3.2 Phase 39（本地版 sing-box sidecar + veth 注入）
- Phase 19 CLI 骨架 → 并入 v3.2 Phase 39（`cloud-claude local` 子命令）
- Phase 20 TTY 透传 → 不再需要（本地 publish 端口，SSH 用系统 OpenSSH）
- Phase 21 指纹伪造 → entrypoint `MODE=local` 深度伪装分支（快速任务级别）
- Phase 22 验证自检 → `cloud-claude local verify` / `doctor --local`
- Phase 23 混淆构建 → Makefile 构建脚本，不单独成里程碑

- [x] Phase 17: 镜像与 Entrypoint 基线 (17-01 + 17-02 gap closure 完成，已并入 v3.0+ 镜像)
- [ ] Phase 18: 网络隔离与分流 → **merged into v3.2 Phase 39**
- [ ] Phase 19: CLI 骨架与 Docker 编排 → **merged into v3.2 Phase 39**
- [ ] Phase 20: TTY 透传与交互体验 → **obsolete**（本地直连端口，无需 TTY 透传）
- [ ] Phase 21: 指纹伪造与反检测 → **merged into v3.2 entrypoint `MODE=local` 分支**
- [ ] Phase 22: 验证与自检 → **merged into v3.2 doctor 扩展**
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

### 🚧 v3.2 多形态容器接入 (In Progress)

**Milestone Goal:** 扩展容器接入方式，让 Cloud 版支持 VS Code Remote SSH，让本地版支持 VS Code Dev Containers，同时研究两套产品形态的最佳拆分/复用边界。

- [x] **Phase 38: SSH Proxy 端口转发支持** — 3/3 plans complete: direct-tcpip + 安全校验 + tcpip-forward/forwarded-tcpip 透传 + sshd_config 验证 — completed 2026-05-07
- [x] **Phase 39: 本地 Dev Containers 支持** — cloud-claude local 子命令 + devcontainer.json + entrypoint MODE 分支 + local down/status (completed 2026-05-07)
- [x] **Phase 40: VS Code Remote-SSH E2E 验证** — VS Code 端到端连接验证 + 安全流量校验 (completed 2026-05-08)
- [x] **Phase 41: Doctor 扩展与收尾** — doctor remote-ssh 诊断维度 + 里程碑收尾 (completed 2026-05-08)
- [x] **Phase 42: Phase 39 验证补齐** — 为 LOCAL-01~04、UX-02 补充 VERIFICATION.md（gap closure）(completed 2026-05-08)
- [x] **Phase 43: VS Code Remote-SSH 端口转发 E2E 补齐** — 端口转发 + egress + 安全验证 + VERIFICATION.md（gap closure） (completed 2026-05-08)
- [ ] **Phase 44: doctor sshd_config 主动验证** — doctor 主动检查 AllowTcpForwarding + 集成测试（gap closure）

## Phase Details

### Phase 38: SSH Proxy 端口转发支持
**Goal**: Cloud 版 managed-user 容器支持 VS Code Remote SSH 所需的端口转发能力，同时保证转发目标的安全性
**Depends on**: Phase 37 (v3.1 shipped)
**Requirements**: SSH-01, SSH-02, SSH-03, SSH-04
**Success Criteria** (what must be TRUE):
  1. VS Code 可以通过 SSH Proxy 的 2222 端口建立 `direct-tcpip` channel 转发到容器内任意端口
  2. 远程端口转发 (`tcpip-forward` / `forwarded-tcpip`) 在同一 SSH 连接上正常工作
  3. 容器内 sshd 显式开启 `AllowTcpForwarding yes` 和 `AllowStreamLocalForwarding yes`，同时 `GatewayPorts no` 防止外部暴露
  4. 转发到管理网段 (10.99.x.x)、Docker socket、metadata 端点的请求被明确拒绝并记录
  5. 同一 SSH 连接支持多个并发 forwarding channel 而不互相干扰
**Plans**: 3 plans
- [x] 038-01-PLAN.md — direct-tcpip channel 转发 + 安全校验（SSH-01, SSH-04） — completed 2026-05-07
- [x] 038-02-PLAN.md — tcpip-forward 全局请求透传 + forwarded-tcpip 回传（SSH-02） — completed 2026-05-07
- [x] 038-03-PLAN.md — sshd_config 验证 + forwarding 集成测试补充（SSH-03） — completed 2026-05-07

### Phase 39: 本地 Dev Containers 支持
**Goal**: 用户可以在本地机器上通过 `cloud-claude local` 一键启动独立容器，支持 VS Code Dev Containers 工作流，无需连接 control-plane
**Depends on**: Phase 38
**Requirements**: LOCAL-01, LOCAL-02, LOCAL-03, LOCAL-04, UX-02
**Success Criteria** (what must be TRUE):
  1. 用户运行 `cloud-claude local` 后，本地直接启动 managed-user 容器并输出 SSH 连接信息（host, port, user, password）
  2. 项目根目录的 `.devcontainer/devcontainer.json` 可以引用 managed-user 镜像并正确 bind mount 当前目录到 `/workspace`
  3. 本地容器 entrypoint 在 `MODE=local` 下跳过 KasmVNC 和 control-plane 心跳，但仍启动 sshd 和 sing-box
  4. 用户可以通过 `--egress-config` 注入 sing-box outbound JSON，容器内自动启动 tun 模式；macOS 宿主机上支持 SOCKS/HTTP 代理兜底
  5. `cloud-claude local down` 可以停止并清理本地容器，`cloud-claude local status` 显示容器运行状态和端口映射
**Plans**: 3 plans
- [x] 039-01-PLAN.md — CLI local up 子命令 + internal/local 包 + entrypoint MODE=local（LOCAL-01, LOCAL-04） — completed 2026-05-07
- [x] 039-02-PLAN.md — local down/status 子命令 + --egress-config 注入（UX-02, LOCAL-03） — completed 2026-05-07
- [x] 039-03-PLAN.md — devcontainer.json 更新 + sing-box 启动逻辑（LOCAL-02, LOCAL-03） — completed 2026-05-07
**UI hint**: yes

### Phase 40: VS Code Remote-SSH E2E 验证
**Goal**: VS Code Remote-SSH 可以完整连接到 Cloud 版容器，所有功能正常工作，且流量严格走 sing-box tun 出口
**Depends on**: Phase 39
**Requirements**: SSH-05, SEC-01, SEC-02
**Success Criteria** (what must be TRUE):
  1. VS Code 通过 SSH Proxy 2222 端口成功连接，VS Code Server 在容器内自动下载并启动
  2. VS Code 端口转发（语言服务器、调试器）正常工作，容器内 `claude` 命令在 VS Code terminal 中可用
  3. 通过 VS Code 端口转发访问外部服务时，出口 IP 必须是绑定的 egress IP，不能绕过 tun 直接走宿主机路由
  4. VS Code Server 下载和扩展安装流量（`update.code.visualstudio.com` 等）全部通过 sing-box 出站，不破坏出口 IP 强约束
**Plans**: TBD

### Phase 41: Doctor 扩展与收尾
**Goal**: `cloud-claude doctor` 覆盖 Remote-SSH 场景的诊断，里程碑完成时所有验收标准满足
**Depends on**: Phase 40
**Requirements**: UX-01
**Success Criteria** (what must be TRUE):
  1. `cloud-claude doctor` 新增 remote-ssh 维度，能检测 VS Code Server 进程是否存在
  2. doctor 能检测 `~/.vscode-server/` 磁盘占用并给出清理建议
  3. doctor 能检测 forwarding channel 是否被拦截或异常
  4. v3.2 所有需求对应的错误码已注册，explain 子命令覆盖新增场景
**Plans**: 2 plans
- [x] 41-01-PLAN.md — remote-ssh doctor 维度 + 5 项检查 — completed 2026-05-08
- [x] 41-02-PLAN.md — error codes + explain 覆盖 + 单元测试 — completed 2026-05-08

### Phase 42: Phase 39 验证补齐
**Goal**: 为 Phase 39 本地 Dev Containers 的 5 个需求补充正式 VERIFICATION.md，消除 unverified phase blocker
**Depends on**: Phase 41
**Requirements**: LOCAL-01, LOCAL-02, LOCAL-03, LOCAL-04, UX-02
**Gap Closure:** Closes Phase 39 verification gaps from v3.2 audit
**Success Criteria** (what must be TRUE):
  1. `cloud-claude local up` 一键启动容器并输出 SSH 连接信息，功能正确
  2. `cloud-claude local down` / `local status` 功能正确
  3. entrypoint `MODE=local` 跳过 KasmVNC、保留 sshd + sing-box
  4. `--egress-config` 注入 tun 和 proxy 模式均工作正常
  5. 生成 Phase 39-VERIFICATION.md，所有 5 个需求标记 SATISFIED
**Plans**: 1 plan
- [x] 42-01-PLAN.md — 运行自动化验证 + 生成 39-VERIFICATION.md + Docker 集成确认 — completed 2026-05-08

### Phase 43: VS Code Remote-SSH 端口转发 E2E 补齐
**Goal**: 完成 VS Code Remote-SSH 端口转发 + egress 流量的端到端验证，生成标准 VERIFICATION.md
**Depends on**: Phase 42
**Requirements**: SSH-05, SEC-01, SEC-02
**Gap Closure:** Closes Phase 40 verification gaps, integration gap (UAT 端口转发), and flow gap (VS Code 端口转发 E2E)
**Success Criteria** (what must be TRUE):
  1. UAT 脚本包含 direct-tcpip / tcpip-forward 端口转发验证场景
  2. VS Code 端口转发（语言服务器端口）通过 direct-tcpip channel 正常工作
  3. 通过 VS Code 端口转发访问外部服务时，出口 IP 是绑定的 egress IP
  4. VS Code Server 下载和扩展安装流量通过 sing-box 出站
  5. 生成 Phase 40-VERIFICATION.md，SSH-05 / SEC-01 / SEC-02 标记 SATISFIED
**Plans**: TBD

### Phase 44: doctor sshd_config 主动验证
**Goal**: doctor remote-ssh 维度主动检查 sshd_config 中 `AllowTcpForwarding=yes`，消除最后一个集成缺口
**Depends on**: Phase 43
**Requirements**: SSH-03（集成缺口）
**Gap Closure:** Closes integration gap (doctor → sshd_config)
**Success Criteria** (what must be TRUE):
  1. doctor remote-ssh 维度新增 sshd_config 解析检查
  2. 检测到 `AllowTcpForwarding` 缺失或为 `no` 时输出 warning
  3. 单元测试覆盖正常和异常 sshd_config 场景
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 38 → 39 → 40 → 41

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-6. v1.0 MVP | v1.0 | 19/19 | Complete | 2026-03-28 |
| 7-10. v1.1 代理协议出网 | v1.1 | 11/11 | Complete | 2026-03-28 |
| 11-12. v1.2 认证与自助面板 | v1.2 | 5/5 | Partial | 2026-03-29 |
| 17-23. claude-shell 本地代理 | v1.3 | — | Paused | — |
| 24-28. v2.0 cloud-claude | v2.0 | 7/7 | Complete | 2026-04-15 |
| 29-35. v3.0 远端开发体验升级 | v3.0 | 30/30 | Complete | 2026-04-23 |
| 36-37. v3.1 映射语义补齐与懒加载 | v3.1 | 11/11 | Complete | 2026-04-24 |
| 38. SSH Proxy 端口转发支持 | v3.2 | 3/3 | Complete | 2026-05-07 |
| 39. 本地 Dev Containers 支持 | v3.2 | Complete    | 2026-05-07 | — |
| 40. VS Code Remote-SSH E2E 验证 | v3.2 | 2/2 | Complete | 2026-05-08 |
| 41. Doctor 扩展与收尾 | v3.2 | 2/2 | Complete | 2026-05-08 |
| 42. Phase 39 验证补齐 | v3.2 | Complete    | 2026-05-08 | 2026-05-08 |
| 43. VS Code 端口转发 E2E 补齐 | v3.2 | 2/2 | Complete    | 2026-05-08 |
| 44. doctor sshd_config 验证 | v3.2 | 0/? | In Progress | — |

---

*Last updated: 2026-05-07 — v3.2 roadmap created.*
