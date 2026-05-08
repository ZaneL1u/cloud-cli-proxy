# v3.4 多形态容器接入 · 研究综述（SUMMARY）

**Project:** Cloud CLI Proxy
**Milestone:** v3.4 多形态容器接入
**Domain:** 扩展容器接入方式 — Cloud 版 VS Code Remote SSH + 本地版 VS Code Dev Containers
**Researched:** 2026-05-08
**Confidence:** HIGH（PITFALLS 基于官方 VS Code 文档、gliderlabs/ssh 源码、sing-box issue tracker 交叉验证）

> 本文档是 STACK.md / FEATURES.md / ARCHITECTURE.md / PITFALLS.md 的"决策提炼版"，
> 下游 agent（roadmapper / REQUIREMENTS drafter / planner）应直接消费本文。

---

## 1. Executive Summary

v3.4 在 v3.0/v3.1 已交付的三层文件系统 + 会话可靠性基础上，**扩展容器接入方式**，让同一套受管镜像同时支持两种新形态：**Cloud 版 VS Code Remote SSH**（通过 SSH Proxy 直接连接）和**本地版 VS Code Dev Containers**（通过 `.devcontainer.json` 配置本地 Docker 启动）。核心目标不是替换现有 `cloud-claude` CLI 体验，而是让同一容器平台兼容开发者已有的 IDE 工作流。

**唯一关键的技术决策：SSH Proxy 必须扩展 `direct-tcpip` 通道支持。** 当前 `internal/sshproxy/proxy.go:206-210` 硬编码拒绝所有非 `session` 通道，这导致 VS Code Remote SSH 完全无法工作（其架构依赖 `direct-tcpip` 做端口转发）。扩展方案推荐在现有 proxy 上直接增加 handler（约 100-150 行），而非引入新组件。扩展时必须配套**严格的目的地校验**（只放行容器自身管理 veth IP 和 127.0.0.1，阻断管理子网、Docker 网桥、云元数据端点），否则等同于打开内网穿透后门。

**Cloud 版与本地版共享同一受管镜像**，通过 `MODE=cloud|local` 环境变量在 entrypoint.sh 分支。本地版独立入口 `cloud-claude local`（或新子命令）不连接 control-plane/PostgreSQL，直接通过本地 Docker 启动容器。这种设计让 bug 修复和特性更新只需改一处镜像，同时保持本地版的零外部依赖。

**v3.4 不引入任何新运行时依赖。** 不需要新数据库、新网络组件、新前端框架。改动集中在：Go SSH Proxy 扩展、Docker 镜像 entrypoint 分支、Shell 本地启动器、`.devcontainer.json` 模板。

最大风险是 `direct-tcpip` 与 sing-box tun 全隧道的交互：如果 sing-box `strict_route: true` 把 127.0.0.0/8 也路由进隧道，VS Code Server 在容器内的内部 HTTP 服务将不可达。必须在 sing-box 配置中显式排除 127.0.0.0/8 和管理 veth 子网。

---

## 2. 关键技术决策表

### 2.1 新增 / 修改组件

| 组件 | 决策 | 范围 | 与 v3.1 兼容性 |
|------|------|------|----------------|
| SSH Proxy | **扩展 `direct-tcpip` + `tcpip-forward` + `forwarded-tcpip` handler** | `internal/sshproxy/proxy.go` 新增 ~100-150 行 | 向后兼容：原有 `session` 通道行为不变 |
| 目的地校验器 | **强制 allowlist：仅容器自身 veth IP + 127.0.0.1** | 同文件内嵌 `validateForwardDestination` | 无性能影响，每连接一次校验 |
| 受管镜像 entrypoint | **增加 `MODE=cloud\|local` 分支** | `deploy/docker/managed-user/entrypoint.sh` | Cloud 模式行为与 v3.1 完全一致 |
| 本地启动器 | **新增 `cloud-claude local` 子命令** | `cmd/cloud-claude/main.go` 新增 subcommand | 不干扰现有命令 |
| `.devcontainer.json` | **提供模板配置** | `deploy/devcontainer/devcontainer.json` | 纯模板文件，无运行时影响 |

### 2.2 明确不引入

| 不要引入 | 原因 | 替代方案 |
|----------|------|----------|
| 新 SSH 库替代 gliderlabs/ssh | 现有库已支持 DirectTCPIPHandler，只需配置 callback | 复用 gliderlabs/ssh，加 LocalPortForwardingCallback |
| 新容器网络模型 | `--network=none + sing-box tun` 已满足，无需改 | 保持现有网络架构 |
| 新数据库 / 新前端 | 本地版明确不依赖 control-plane + Postgres | 本地配置用文件/env var |
| Docker Compose 编排（本地版） | 增加复杂度，与"单命令启动"承诺冲突 | `docker run` 直接启动 |
| Web Terminal / 浏览器 IDE | v1 范围明确不做 | 保持 SSH-only 接入 |

---

## 3. Cloud / Local 组件复用矩阵

| 组件 | Cloud 版 | 本地版 | 复用策略 |
|------|----------|--------|----------|
| 受管 Docker 镜像 | 使用 | 使用 | **完全共享**，MODE 环境变量分支 |
| SSH Proxy | 使用（入口） | 可选（直接容器端口也可） | **共享实现**，本地版可直连 |
| sing-box tun | 使用 | 使用 | **共享配置生成**，本地版同样全隧道 |
| entrypoint.sh | 使用 | 使用 | **共享**，MODE=cloud\|local 分支 |
| control-plane API | 使用 | **不使用** | 本地版零依赖 |
| PostgreSQL | 使用 | **不使用** | 本地版零依赖 |
| Admin Dashboard | 使用 | **不使用** | 本地版无管理后台 |
| JWT 认证 | 使用 | **不使用** | 本地版用本地配置文件或 env var |
| 容器生命周期调度 | control-plane 管理 | `cloud-claude local` 管理 | **独立** |
| `.devcontainer.json` | N/A | 使用 | 本地版独有 |

**架构边界原则：**
- **共享（library packages）：** SSH Proxy、容器网络设置（namespace/veth/tun）、sing-box 配置生成、错误码体系
- **Cloud-only：** control-plane API、PostgreSQL、Admin Dashboard、JWT、生命周期调度
- **Local-only：** 独立 CLI 入口、本地配置管理、`.devcontainer.json` 模板

---

## 4. 每个 Feature 的必做行为清单

### F1 · SSH Proxy `direct-tcpip` 支持

- **REQ-F1-A：** SSH Proxy 必须接受并正确处理 `direct-tcpip` 通道（RFC 4254），解析 dest_addr/dest_port/originator_addr/originator_port。
- **REQ-F1-B：** 实现严格的目的地校验，阻断以下目标：管理 veth 子网（10.99.0.0/16）、Docker 桥接网（172.17.0.0/12）、云元数据（169.254.169.254）、Unix socket 路径。
- **REQ-F1-C：** 允许的目标仅限：容器自身管理 veth IP（来自 `DeriveManagementSSHAccess`）、127.0.0.1/::1（容器内本地服务）。
- **REQ-F1-D：** 所有 `direct-tcpip` 请求必须记审计日志（用户、源、目的地、时间戳）。

### F2 · Cloud 版 VS Code Remote SSH 完整功能

- **REQ-F2-A：** VS Code Remote SSH 扩展必须能完整连接、打开终端、浏览文件、使用端口转发面板。
- **REQ-F2-B：** sing-box TUN 配置必须排除 127.0.0.0/8 和管理 veth 子网，避免 VS Code Server 内部 HTTP 通信被路由进隧道。
- **REQ-F2-C：** 容器内 `lo` 接口必须正常 UP，即使 `--network=none`。
- **REQ-F2-D：** 可选：实现 `tcpip-forward` + `forwarded-tcpip` 以支持 VS Code Ports 面板自动转发。

### F3 · 本地版独立启动入口

- **REQ-F3-A：** `cloud-claude local`（或等效子命令）必须能在**不连接 control-plane、不依赖 PostgreSQL** 的情况下启动本地容器。
- **REQ-F3-B：** 本地版使用本地配置文件（`~/.cloud-claude/local.yaml`）或环境变量获取必要参数（镜像名、出口 IP 配置、用户凭证）。
- **REQ-F3-C：** 本地版启动的容器同样使用 sing-box tun 全隧道，保证出口 IP 强约束不变。
- **REQ-F3-D：** 本地版二进制不得引入 control-plane 的 Go package 依赖。

### F4 · 本地版 VS Code Dev Containers 支持

- **REQ-F4-A：** 提供 `.devcontainer/devcontainer.json` 模板，配置 `postCreateCommand`（安装 sshd）和 `postStartCommand`（启动 sshd）。
- **REQ-F4-B：** 模板必须正确处理生命周期顺序：`postCreateCommand` 只跑一次，`postStartCommand` 每次启动都跑。
- **REQ-F4-C：** 端口转发统一使用 SSH `forwardPorts`，不使用 Docker `ports`（因为 `--network=none` 下 Docker 端口映射无效）。
- **REQ-F4-D：** `remoteUser` 必须与容器内 SSH 用户一致，必要时 `updateRemoteUserUID: true`。

### F5 · Cloud/Local 架构边界分析

- **REQ-F5-A：** 明确文档化共享组件 vs 独立组件的边界（见 §3 矩阵）。
- **REQ-F5-B：** 本地版代码不得导入 control-plane 的 DB model、migration、API handler。
- **REQ-F5-C：** 共享代码抽取为独立 internal package，通过 interface 隔离 Cloud/Local 差异。

---

## 5. TOP Critical Pitfalls（必须在对应 Phase 防御）

| # | Pitfall | 触发症状 | 防御 Phase | 验证手段 |
|---|---------|---------|-----------|----------|
| **1** | SSH Proxy 拒绝 `direct-tcpip` | VS Code 连接失败在 "Setting up SSH tunnel" | P1 | `ssh -L` 手动测试；VS Code 实际连接 |
| **2** | `direct-tcpip` 无目的地校验 | 用户可穿透到 Docker socket、管理 API、其他容器 | P1 | 单元测试校验器；渗透测试内网服务 |
| **3** | 缺失 `tcpip-forward`/`forwarded-tcpip` | VS Code Ports 面板不显示转发端口 | P1/P2 | `ssh -R` 测试；Ports 面板自动转发测试 |
| **4** | sing-box TUN 劫持 127.0.0.1 | VS Code Server 启动但扩展无法激活 | P2 | 容器内 `curl 127.0.0.1:PORT`；扩展激活测试 |
| **5** | Dev Containers `forwardPorts` 与 Docker `ports` 冲突 | 端口已绑定错误 | P3 | `docker-compose up` 双配置验证 |
| **6** | Dev Containers 生命周期脚本顺序错误 | 重启后 sshd 未启动 | P3 | 停/启容器验证 `sshd` 进程存在 |
| **7** | SSH Agent 转发 socket 路径冲突 | `ssh-add -l` 显示无身份 | P3 | 容器内 `ssh-add -l`；git push 测试 |
| **8** | Cloud/Local 代码耦合 | 本地版二进制体积膨胀、需要 Postgres | P0 | Code review：无 control-plane import |

---

## 6. 建议的 Phase 切分

| # | Phase 名称 | 范围 | Features | 工作量 | Depends on |
|---|-----------|------|----------|--------|-----------|
| **P0** | **架构边界分析** | 文档化 Cloud/Local 组件复用矩阵；抽取共享 package；定义 interface 边界 | F5 | **S** | — |
| **P1** | **SSH Proxy 转发支持** | 实现 `direct-tcpip` handler + 目的地校验；可选 `tcpip-forward`/`forwarded-tcpip`；审计日志 | F1 | **M** | P0 |
| **P2** | **Cloud 版 VS Code Remote SSH 验证** | sing-box TUN 排除 127.0.0.0/8；容器内 lo 验证；VS Code 完整功能 UAT | F2 | **M** | P1 |
| **P3** | **本地版 Dev Containers 支持** | `cloud-claude local` 子命令；`.devcontainer.json` 模板；生命周期脚本；SSH agent 集成 | F3, F4 | **L** | P0 |
| **P4** | **E2E 验证 + 文档** | Cloud/Local 双路径完整 UAT；架构边界文档；运维手册更新 | 验收 | **S-M** | P1-P3 |

**合并选项：** P0+P1 可合并为"Proxy 扩展 + 架构边界"。P2+P3 可部分并行（P2 验证 Cloud，P3 开发 Local，两者接触不同代码路径）。

---

## 7. Confidence Assessment

| 维度 | Confidence | 依据 |
|------|------------|------|
| SSH Proxy 扩展方案 | **HIGH** | gliderlabs/ssh 源码已验证 DirectTCPIPHandler 存在；实现模式明确 |
| 目的地校验规则 | **HIGH** | 基于现有网络架构（管理 veth 子网、Docker 网桥）直接推导 |
| sing-box TUN 排除配置 | **HIGH** | sing-box 官方文档支持 `route.rules` + `ip_cidr` 排除 |
| VS Code Remote SSH 行为 | **HIGH** | 官方文档确认 `AllowTcpForwarding yes` 要求；社区 issue 验证 |
| Dev Containers 生命周期 | **HIGH** | 官方文档明确 `postCreateCommand` vs `postStartCommand` 语义 |
| Cloud/Local 架构边界 | **MEDIUM** | 需要实际代码重构验证 interface 边界是否干净 |

**Overall confidence: HIGH** — 可直接进入 roadmap → REQUIREMENTS → plan-phase。

### 7.1 需要在 plan-phase 验证的潜在 gap

1. **VS Code Server 在容器内的具体端口范围** — 影响 `direct-tcpip` allowlist 的精确度。
2. **本地版 sing-box 配置来源** — 用户如何提供出口 IP 配置（文件模板、交互式输入、环境变量）。
3. **macOS 本地版 Docker Desktop 与 sing-box tun 的兼容性** — 需要真机验证。
4. **Dev Containers Features 市场兼容性** — 是否允许用户使用官方 Features，还是完全自管。

---

## 8. Out-of-Scope 强化清单

| # | 不做的功能 | 理由 |
|---|-----------|------|
| 1 | 替换现有 `cloud-claude` CLI 主路径 | v3.4 是扩展，不是替换 |
| 2 | 多宿主机编排 | 沿用 v1 单宿主机约束 |
| 3 | Web Terminal / 浏览器 IDE | v1 范围明确不做 |
| 4 | 本地版连接 Cloud 版容器 | 本地版是独立形态，不混合 |
| 5 | 用户自定义任意镜像 | 会削弱就绪性和可支持性 |
| 6 | 计费/套餐/支付流程 | 沿用 v1 不做商业化 |

---

## 9. Sources

### Primary（HIGH，官方文档 / 源码）
- gliderlabs/ssh `tcpip.go`：<https://github.com/gliderlabs/ssh/blob/master/tcpip.go> — DirectTCPIPHandler 和 ForwardedTCPHandler 源码
- VS Code Remote Development Troubleshooting：<https://code.visualstudio.com/docs/remote/troubleshooting>
- VS Code Dev Containers Lifecycle：<https://code.visualstudio.com/docs/devcontainers/containers#_lifecycle-scripts>
- sing-box TUN 路由排除：<https://github.com/SagerNet/sing-box/issues/2700>、<https://github.com/SagerNet/sing-box/issues/1666>

### Secondary（MEDIUM，社区验证）
- Fly.io VS Code Remote SSH Discussion：<https://community.fly.io/t/how-to-connect-vscode-remote-development-to-a-fly-machine/23541>
- Tailscale SSH Issue #5295：<https://github.com/tailscale/tailscale/issues/5295>
- VS Code Remote-SSH Issue #3025：<https://github.com/microsoft/vscode-remote-release/issues/3025>
- Dev Containers Discussion #224：<https://github.com/orgs/devcontainers/discussions/224>

### 项目内部（HIGH）
- `.planning/PROJECT.md` — v3.4 milestone 目标与约束
- `internal/sshproxy/proxy.go:206-210` — 当前硬编码 channel 拒绝逻辑
- v3.0/v3.1 已交付基础设施（三层文件系统、tmux、会话可靠性）

---

## 10. 与下游 agent 的对接清单

**给 gsd-roadmapper：**
- 直接采用 §6 的 5 phase 切分；P0 是架构决策必须在编码前完成。
- 每个 phase 的 `goal` 段引用对应 §4 REQ-ID + §5 Pitfall 编号。
- P1 必须把 Pitfall 1/2/3 作为硬性 success criteria。

**给 REQUIREMENTS drafter：**
- §4 的 15 条 REQ-F*-* 直接转写为 active requirements。
- §7.1 的 4 个 gap 收录为 `### Open Questions`。
- §8 Out-of-Scope 追加到 PROJECT.md。

**给 gsd-planner：**
- P1 的 PLAN.md 必须为 Pitfall 1/2 创建独立防御任务。
- P1 验证段必须包含 `ssh -L` 测试和目的地校验单元测试。
- P0 必须在 `discuss-phase` 输出架构边界文档。

---

*Researched: 2026-05-08*
*Synthesized by: gsd-research-synthesizer*
*Confidence: HIGH*
*Ready for: roadmap → REQUIREMENTS → plan-phase*
