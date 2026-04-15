# Cloud CLI Proxy

## What This Is

Cloud CLI Proxy 是一个面向单宿主机的容器化 SSH 云主机平台，既供自己使用，也面向出海团队和开发团队销售。用户从一个很短的 `curl` 入口开始，在终端里输入用户名和密码，等待专属 Docker "云主机"启动完成后，直接进入该容器内的 SSH 会话。

平台包含一个管理后台，用于管理用户、容器生命周期、出口 IP 分配和到期时间。每个容器都预装 `claude code`，并且所有网络流量都必须通过指定出口 IP 的全局隧道路由发送（支持 WireGuard 和 sing-box tun 两种模式），不能出现 DNS、WebRTC 或其他类型的直接泄漏。

## Core Value

给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持"一条命令启动"的体验足够顺滑。

## Current State

**Shipped:** v2.0 cloud-claude 透明远程 CLI (2026-04-15)

v2.0 交付了 `cloud-claude` Go 二进制文件，用户 `alias claude=cloud-claude` 后输入 `claude` 的体验与本地完全一致——实际运行在远端配好代理出口的 Docker 容器里，本地目录通过 sshfs slave 实时映射到容器内。

## Context

**Latest shipped:** v2.0 cloud-claude 透明远程 CLI (2026-04-15)
**Codebase:** ~28,877 LOC (15,490 Go + 11,976 TypeScript + 1,411 Shell)
**Tech stack:** Go 1.26.1 + PostgreSQL + Docker + WireGuard + sing-box + React 19 + Vite + cobra + sshfs/SFTP

v1.0 MVP + v1.1 代理协议出网 已交付，涵盖：
- 单宿主机控制面 + Unix socket host-agent + 受管用户镜像
- WireGuard + sing-box 双通道全隧道出网 + nftables 默认拒绝 + 三重网络校验
- `curl → 认证 → 启动提示 → SSH` 一条命令接入流
- JWT 管理后台 (React SPA) + 用户/出口 IP/绑定/主机生命周期 CRUD
- 5 种代理协议（SOCKS5/vmess/shadowsocks/trojan/HTTP）+ 一键测试
- 到期治理、事件记录、运行时对账、部署文档

v1.2 部分交付（Phase 11-12），新增用户认证体系和自助面板骨架，Phase 13-16 延后。

v2.0 已交付，新增：
- Go 单一二进制 `cloud-claude`，cobra 入口 + init 配置管理 + Entry API 认证轮询
- sshfs slave + 嵌入式 SFTP server 实现本地目录到容器 /workspace 实时双向映射
- shellescape 安全命令构建 + 非 TTY 管道模式 + claude 参数原样透传
- TTY/信号/窗口大小/退出码完全透传
- 受管镜像预装 sshfs/fuse3 + AppArmor unconfined + 宿主机 FUSE 前置检查
- 238 行 FUSE 兼容性验证脚本 + 中英文部署文档补充

## Requirements

### Validated

- ✓ 每个运行中的容器都必须绑定至少一个出口 IP，并且所有出站流量都必须强制走该路径，不能出现 DNS、WebRTC 等流量绕行或泄漏 — v1.0
- ✓ 用户可以执行一条简短的 `curl` 命令，在终端中完成认证，等待容器启动，并无须手工配置主机信息就进入 SSH 会话 — v1.0
- ✓ 管理员可以在单宿主机环境下管理用户、登录凭证、到期时间和容器生命周期 — v1.0
- ✓ 凭证错误、账号过期、未绑定出口 IP 或启动失败时返回清晰的终端错误提示 — v1.0
- ✓ 管理员操作和启动结果被记录为运维事件，并可在事件日志页面查看 — v1.0
- ✓ 已过期用户无法开启新会话，运行中主机按策略停止 — v1.0
- ✓ 出口 IP 类型化，支持 wireguard 和 proxy 两种隧道类型 — v1.1
- ✓ SingBoxProvider tun 模式全流量代理实现 — v1.1
- ✓ 受管镜像预装 sing-box 二进制 — v1.1
- ✓ Provider 工厂按 tunnel_type 自动选择 WireGuard 或 sing-box — v1.1
- ✓ 前端出口 IP 表单按隧道类型动态切换字段 — v1.1
- ✓ 后台一键代理测试 API 及前端展示 — v1.1
- ✓ Go 单一二进制 cloud-claude，用户可 alias claude=cloud-claude 透明替代原生 claude 命令 — v2.0
- ✓ cloud-claude init 配置网关地址、用户凭证，持久化到 ~/.cloud-claude/config.yaml — v2.0
- ✓ 执行时自动获取当前目录，sshfs slave 将当前目录实时映射到容器 /workspace — v2.0
- ✓ 在容器内启动 Claude Code，所有参数原样透传（shellescape 防注入）— v2.0
- ✓ TTY/信号/窗口大小/退出码完全透传，用户体验与本地 claude 无差异 — v2.0
- ✓ 容器镜像预装 sshfs + FUSE，创建时带 --device /dev/fuse + --cap-add SYS_ADMIN — v2.0
- ✓ 支持私有部署：用户可配置自有网关地址 — v2.0
- ✓ 生产环境 FUSE + AppArmor 兼容性验证通过 — v2.0

### Active

（下一里程碑定义时填充）

### Paused (v1.3 claude-shell)

- [ ] 单一 Go 二进制即 `claude` 命令（本地 Docker 模式），用户下载后直接替换本机 claude
- [ ] 系统级指纹伪造：entrypoint 预生成 /etc/machine-id、bind mount /proc/cpuinfo 和 /proc/meminfo
- [ ] sing-box tun 全流量代理 + nftables 默认拒绝（本地 Docker 容器内）
- [ ] 反容器检测（删 /.dockerenv、伪造 /proc/1/cgroup）
- [ ] verify 命令验证出口 IP、DNS、指纹和容器标记
- [ ] garble 混淆构建交付单一二进制

### Deferred from v1.2

- [ ] Bootstrap 流程改为 `curl domain/{short_id}`，展示欢迎艺术字，交互输入密码，实时状态推送，自动 SSH 接入
- [x] 用户自助面板：同一 React 应用根据角色展示不同视图，用户可查看自己的主机、重建主机、查看出口 IP — Phase 12 (2026-03-29, pending human verification)
- [ ] 用户可在自助面板查看管理员绑定的 Claude 账号信息
- [ ] 用户可在自助面板直接访问 KasmVNC 远程桌面
- [ ] 数据模型支持一个用户拥有多个 Claude 账号，每个账号对应一台独立主机
- [ ] 管理员可管理 Claude 账号（CRUD）及其与用户/主机的绑定关系
- [x] 用户登录认证体系（区别于管理员 JWT），用户只能访问自己的资源 — Phase 11+12 (2026-03-29)

### Out of Scope

- 计费、套餐、余额和自助支付流程：在核心主机生命周期和网络强约束能力验证前，不纳入 v1。
- 多宿主机编排和集群调度：v1 明确限制为单宿主机，以降低复杂度并加快落地。
- Web Terminal 和浏览器远程桌面：v1 只做 SSH 访问体验。
- 用户自定义任意镜像：会削弱就绪性、安全性和可支持性。
- 用户自选代理节点：由管理员统一配置，避免安全和支持风险。
- 代理链/多跳：延迟增加、排查困难，单跳足够。
- 实时流量监控：开发量大，先做连通性测试。
- 用户申请交接账号：流程未设计清楚，v1.2 暂不做。

## Product Context

- v1.0 + v1.1 + v2.0 已交付，首批目标用户是项目拥有者本人，随后扩展到需要受控出口 IP 工作环境的出海团队和开发团队。
- 两条产品路径已形成：Web SSH 接入（curl 入口）和本地 CLI 透明替代（cloud-claude）。
- 容器虽然基于 Docker，但对用户来说应当像一台"可管理、可复用、可回收"的云主机。
- `claude code` 已在镜像中预装，用户可通过 SSH 或 cloud-claude 直接使用。
- 网络模型已实现双通道：WireGuard 命名空间注入 + sing-box tun 模式，均配合 nftables 默认拒绝 + 三重校验门禁。
- 出口 IP 支持 5 种代理协议（SOCKS5/vmess/shadowsocks/trojan/HTTP），管理后台提供一键测试。
- cloud-claude 通过 sshfs slave 实现本地目录实时双向映射，体验与本地 claude 一致。
- 产品优先级是优雅、好用、运维清晰，而不是功能数量最多。

## Constraints

- **部署方式**：v1 仅支持单台 Linux 宿主机，先把可用性和运维复杂度收住。
- **访问模型**：v1 只做 SSH 会话接入，不分散到多种远程交互形态。
- **运行时**：每个用户环境都由 Docker 容器承载，容器创建、启动和接入是产品主线。
- **网络安全**：必须通过虚拟网卡 / tun 风格的全局隧道路由实现全流量强制出网，不能允许直连外网。
- **IP 分配**：每个容器都必须至少绑定一个出口 IP，没有绑定就视为非法状态。
- **产品范围**：v1 只做后台管理、生命周期和到期治理，不做计费和商业化流程。
- **沟通语言**：所有助手面对用户的回复、计划、状态更新和总结，默认必须全部使用中文；除非用户明确要求，否则不要改回英文。

## Key Decisions

| 决策 | 原因 | 结果 |
|------|------|------|
| 先从单宿主机部署开始 | 最快拿到可用产品、同时控制运维复杂度 | ✓ Good — v1.0 已验证 |
| v1 只提供 SSH 访问方式 | 最符合目标体验，减少远程接入面复杂度 | ✓ Good — bootstrap 脚本 + exec ssh 体验顺畅 |
| 使用短 `curl` 入口完成认证和启动 | 低摩擦、易传播，符合产品定位 | ✓ Good — 7 个错误码 + 中文提示完整 |
| 在镜像中预装 `claude code` | 用户进入环境后立即可用 | ✓ Good — image.lock 模板已实现 |
| 强制要求出口 IP 绑定和全隧道路由 | 出口可控不是附加功能，而是产品承诺核心 | ✓ Good — WireGuard + sing-box 双通道 + nftables + 三重校验 |
| 延后计费和多节点调度 | 保持 MVP 聚焦在主机交付和网络正确性 | ✓ Good — v1.0 + v1.1 按时交付 |
| 控制面通过 Unix socket 驱动 host-agent | 避免在 HTTP 层直接持有 Docker/网络特权 | ✓ Good — 清晰的特权边界 |
| 容器使用 --network=none 创建 | 彻底隔离 Docker 默认网络，防止旁路 | ✓ Good — 无绕过可能 |
| WireGuard birthplace-namespace 模式 | 密钥不经过宿主机网络栈 | ✓ Good — 安全性更强 |
| bcrypt 密码 + JWT 管理后台 | 标准安全实践，简单可靠 | ✓ Good — 测试覆盖完整 |
| 新增 sing-box tun 模式与 WireGuard 并行 | 支持更多代理协议，扩展出口 IP 灵活性 | ✓ Good — 5 种协议支持，WireGuard 路径不受影响 |
| 代理配置以 sing-box outbound JSON 存储 | 灵活且面向未来，不为每种协议建列 | ✓ Good — JSONB 列 + 白名单校验 |
| RoutingProvider 工厂按 tunnel_type 委托 | 单一 Provider 接口，内部按类型路由 | ✓ Good — 扩展新隧道类型只需新增 case |
| proxy 模式独立防火墙函数 | 不修改现有 WireGuard 路径 | ✓ Good — 两条路径完全解耦 |
| 宿主机 masquerade 用 iptables 而非 nftables | 避免与 Docker Engine iptables 规则冲突 | ✓ Good — 幂等且安全 |
| SYS_ADMIN + /dev/fuse 统一附加，不做条件区分 | FUSE mount 需要 SYS_ADMIN，统一避免条件分支 | ✓ Good — 简化运维 |
| Entry API 为 cloud-claude 唯一认证契约 | 复用控制面现有实现，不新增专用 API | ✓ Good — 零服务端改造 |
| sshfs slave + 嵌入式 SFTP server 实现目录映射 | 复用 SSH 连接，无需额外端口或协议 | ✓ Good — 架构简洁 |
| shellescape.QuoteCommand 构建远程命令行 | POSIX 单引号规则成熟，防止 shell 注入 | ✓ Good — 安全可靠 |
| apparmor=unconfined 而非自定义 AppArmor profile | 安全边界已由 nftables + namespace 覆盖 | ✓ Good — 降低运维复杂度 |
| 退出码通过返回值上浮，禁止 os.Exit | 保证 defer term.Restore 始终执行 | ✓ Good — 修复终端恢复 |

## Evolution

这个文档会在阶段切换和里程碑完成时持续更新。

**每次阶段切换之后**（通过 `$gsd-transition`）：

1. 如果有需求被证伪，移动到"明确不做"并说明原因
2. 如果有需求被验证，移动到"已验证"并标注阶段
3. 如果出现新需求，加入"当前活跃"
4. 如果产生重要决策，补充到"关键决策"
5. 如果"这是什么"已经不准确，就按当前现实更新

**每次里程碑完成之后**（通过 `$gsd-complete-milestone`）：

1. 全量复查所有章节
2. 检查"核心价值"是否仍然是最高优先级
3. 审视"明确不做"的理由是否还成立
4. 用当前产品状态更新"背景"

---
*Last updated: 2026-04-15 after v2.0 milestone*
