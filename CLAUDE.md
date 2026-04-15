<!-- GSD:project-start source:PROJECT.md -->
## 项目

**Cloud CLI Proxy**

Cloud CLI Proxy 是一个面向单宿主机的容器化 SSH 云主机平台，既供自己使用，也面向出海团队和开发团队销售。用户从一个很短的 `curl` 入口开始，在终端里输入用户名和密码，等待专属 Docker “云主机”启动完成后，直接进入该容器内的 SSH 会话。

平台包含一个管理后台，用于管理用户、容器生命周期、出口 IP 分配和到期时间。每个容器都预装 `claude code`，并且所有网络流量都必须通过指定出口 IP 的全局隧道路由发送，不能出现 DNS、WebRTC 或其他类型的直接泄漏。

**核心价值：** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持“一条命令启动”的体验足够顺滑。

### 约束

- **部署方式**：v1 仅支持单台 Linux 宿主机，先把可用性和运维复杂度收住。
- **访问模型**：v1 只做 SSH 会话接入，不分散到多种远程交互形态。
- **运行时**：每个用户环境都由 Docker 容器承载，容器创建、启动和接入是产品主线。
- **网络安全**：必须通过虚拟网卡 / tun 风格的全局隧道路由实现全流量强制出网，不能允许直连外网。
- **IP 分配**：每个容器都必须至少绑定一个出口 IP，没有绑定就视为非法状态。
- **产品范围**：v1 只做后台管理、生命周期和到期治理，不做计费和商业化流程。
- **沟通语言**：所有助手面对用户的回复、计划、状态更新和总结，默认必须全部使用中文；除非用户明确要求，否则不要改回英文。
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## 技术栈

## 推荐技术栈
### 核心技术
| 技术 | 版本 | 用途 | 推荐原因 |
|------|------|------|----------|
| Go | 1.26.1 | 控制面 API、启动流程、宿主机代理、任务编排 | 官方当前稳定补丁版本，适合系统编排、网络控制、进程管理和单二进制交付。 |
| PostgreSQL | 18.3 | 持久化保存用户、容器、出口 IP 绑定、会话、到期时间和审计事件 | 当前支持中的主版本，事务能力成熟，适合从 MVP 一路扩到后续版本。 |
| Docker Engine | 28.5.x | 容器运行时、镜像管理、网络和宿主机集成 | 当前主线版本，网络控制能力持续增强，适合单机产品形态。 |
| OpenSSH | 10.2p1 | 容器内 SSH Server 和标准 SSH 兼容性 | 远程接入场景最稳妥的标准方案，远比自造终端传输层可靠。 |
| sing-box + Linux netns | 协议稳定 / 跟随宿主机发行版 | 为每个容器提供全隧道出网能力 | 通过 sing-box tun 设备在容器 namespace 内实现全流量代理，支持 SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP 等协议。 |
| React | 19.2 | 后台管理前端 | 当前官方稳定版本，适合构建响应快、交互明确的管理界面。 |
| Vite | 8.x | 后台前端构建工具 | 当前稳定主版本，开发与构建速度都很适合后台项目。 |
| Node.js | 24 LTS | 前端构建工具链运行时 | 官方生产可用的 LTS 线，适合作为前端工具链基础。 |
### 辅助库 / 组件
| 组件 | 版本 | 用途 | 何时使用 |
|------|------|------|----------|
| Go `net/http` + 标准库 | Go 1.26.1 | HTTP API、启动入口、健康检查 | 默认起步方案，减少不必要的框架依赖。 |
| `pgx` | v5.x | PostgreSQL 驱动和查询执行 | 当控制面开始落地用户、任务、绑定和事件表时直接使用。 |
| `systemd` | 宿主机稳定版 | 监管控制面进程和到期 / 清理任务 | 单宿主机方案下很合适。 |
| `nftables` 或 `iptables` | 宿主机稳定版 | 做默认拒绝的出站策略和命名空间级别例外规则 | 当网络管理模块开始接管全隧道流量时必须引入。 |
### 开发工具
| 工具 | 用途 | 备注 |
|------|------|------|
| Docker Compose | 本地多服务开发 | 足够承载后台、Postgres 和控制面联调。 |
| `air` 或同类 Go 热重载工具 | 后端开发循环 | 可选，但会提升控制面开发效率。 |
| Vitest | 前端 / 单元测试 | 与 Vite 后台项目天然契合。 |
| Playwright | 端到端验证 | 适合覆盖后台 CRUD 和启动流程。 |
## 安装 / 运行基线
# 后端运行基线
# - Go 1.26.1
# - PostgreSQL 18.x
# - Docker Engine 28.x
# - 用户镜像内的 OpenSSH 10.2p1
# 前端工具链
# - Node.js 24 LTS
# - Vite 8.x
# - React 19.2
## 备选方案
| 推荐方案 | 备选方案 | 什么时候才考虑备选 |
|----------|----------|--------------------|
| Go 1.26.1 | Node.js 24 作为后端 | 只有当团队必须统一语言栈，且愿意接受系统编排体验下降时才考虑。 |
| PostgreSQL 18.3 | SQLite | 仅适合一次性本地原型，不适合带审计和运营状态的正式产品。 |
| OpenSSH 10.2p1 | Web Terminal / 自定义 PTY 代理 | 只有在 SSH 主路径稳定后，才考虑补浏览器终端。 |
| sing-box tun + netns 全隧道 | 单纯 SOCKS / HTTP 代理 | 只适合非强约束实验，无法满足“所有流量不可泄漏”的要求。 |
## 明确不推荐
| 不要用 | 原因 | 替代方案 |
|--------|------|----------|
| Docker 默认 bridge 网络作为主生产路径 | 官方文档把它视为偏遗留细节，不适合作为严肃生产基线 | 使用用户自定义网络和显式 namespace 路由 |
| 用户容器使用 `host` 网络 | 会破坏网络隔离并削弱出口 IP 强约束 | 使用独立 namespace / 隧道模型 |
| 只做应用层代理控制出网 | 容易留下 DNS 和非代理感知流量的绕过空间 | 用全隧道 namespace 路由加默认拒绝策略 |
| v1 就上多宿主机编排 | 在产品主承诺尚未验证前，会显著放大复杂度 | 先做单宿主机，保留未来扩展边界 |
## 分场景模式
- 控制面可以保持为一个 Go 服务。
- Postgres 可以与应用部署在同一台机器上，但要做好备份。
- 因为当前真正复杂的是网络正确性，而不是微服务拆分。
- 当前设计可自然拆成控制面 + 每台宿主机一个 agent。
- 用户、容器、出口 IP 绑定的 API 契约尽量保持不变。
- 这样 v1 的大部分工作都能沿用到后续扩展阶段。
## 版本兼容性
| 组件 A | 兼容组件 | 备注 |
|--------|----------|------|
| Go 1.26.1 | PostgreSQL 18.3 | 正常驱动支持路径，没有明显兼容风险。 |
| Node.js 24 LTS | Vite 8.x | Vite 8 需要现代 Node 版本，Node 24 属于官方支持范围。 |
| React 19.2 | Vite 8.x | 是非常常规的现代前端组合。 |
| Docker Engine 28.x | 用户自定义 bridge / namespace 路由 | 官方支持此类显式网络方案，不应依赖隐式默认路由行为。 |
## 来源
- https://go.dev/doc/devel/release — 确认 Go 1.26.1 为当前补丁版本
- https://go.dev/blog/go1.26 — 确认 Go 1.26 发布状态
- https://www.postgresql.org/docs/current/index.htm — 确认 PostgreSQL 18.3 为当前文档线
- https://nodejs.org/en/about/previous-releases — 确认 Node.js 24 为 Active LTS
- https://react.dev/blog/2025/10/01/react-19-2 — 确认 React 19.2 发布状态
- https://vite.dev/blog/announcing-vite8 — 确认 Vite 8 为当前稳定主线
- https://docs.docker.com/engine/release-notes/28/ — 确认 Docker Engine 28 当前版本线
- https://docs.docker.com/engine/network/drivers/bridge/ — 确认官方更推荐用户自定义 bridge
- https://docs.docker.com/engine/network/packet-filtering-firewalls/ — 确认 Docker 与防火墙规则的交互方式
- https://www.openssh.org/releasenotes.html — 确认 OpenSSH 10.2p1 为当前便携版发布线
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## 约定

## 沟通
- 所有面向用户的回复、计划、状态更新、说明、错误提示与总结，默认必须全部使用中文。
- 除非用户明确要求英文原文或双语，否则不要输出英文或中英混写的自然语言说明。
- 命令、路径、环境变量、协议名、代码标识符、第三方产品名可以保留英文原文，但解释文字必须使用中文。
## 文档
- 项目规划、实现说明、运行手册、排障记录优先使用中文撰写。
- 需求 ID、文件名、接口字段名等机器可读标识保持原格式，不做翻译。
## 隐私与安全
- 禁止在代码、注释、文档、规划文件或提交信息中写入任何本机绝对路径（如 `/Users/xxx/`、`/home/xxx/`、`C:\Users\xxx\`）。
- 禁止在任何被 git 跟踪的文件中写入真实的 API 密钥、私钥、密码、token、个人邮箱、手机号等敏感信息。
- 涉及路径引用时，一律使用项目根目录的相对路径。
- 涉及示例凭据时，使用明确的占位符（如 `your-secret-here`、`test@example.com`）。
- 每次批量生成或修改 `.planning/`、`.cursor/`、`.claude/` 等工具链文件后，必须检查是否引入了绝对路径或个人信息。
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## 架构

## 总体结构
- 控制面：负责用户、会话、出口 IP 绑定、生命周期、到期和审计状态
- 宿主机代理：负责 Docker、命名空间、隧道和防火墙等特权操作
- 用户容器：承载 OpenSSH、Shell 工具和 `claude code`
- 持久化层：使用 PostgreSQL 保存系统真实状态
## 关键边界
- Web / API 层不要直接持有过宽的宿主机特权
- Docker 和网络 namespace 的操作应集中在独立边界中执行
- 用户容器的默认出网必须被隧道网络接管，不能保留旁路
## 核心数据流
## 当前架构原则
- 单宿主机优先，不为 v1 提前引入多节点调度复杂度
- 网络强约束优先，先保证“所有流量都必须走指定出口”
- 启动体验建立在真实可验证的运行时正确性之上
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD 工作流约束

在使用 Edit、Write 或其他会修改文件的工具前，应先通过 GSD 命令进入工作流，以确保规划产物与执行上下文保持同步。

推荐入口：
- `/gsd:quick`：用于小修复、文档更新和零散任务
- `/gsd:debug`：用于排查问题和修复缺陷
- `/gsd:execute-phase`：用于执行已经规划好的阶段工作

除非用户明确要求绕过，否则不要在 GSD 工作流之外直接修改仓库文件。
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## 开发者画像

> 当前尚未配置开发者画像。可运行 `/gsd:profile-user` 生成。
> 本节由 `generate-claude-profile` 管理，请不要手工修改。
<!-- GSD:profile-end -->
