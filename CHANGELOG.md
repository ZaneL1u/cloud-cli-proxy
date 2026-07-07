# Changelog

All notable changes to this project are documented in this file.

<!-- release-entries -->

## v4.2.15 - 2026-07-07
## What's Changed

### Backend (Go / API)
- feat(vnc): add status controls and auto recovery (64f539f)

### Frontend (Admin Web)
- feat(vnc): add status controls and auto recovery (64f539f)

### Runtime & Deployment
- feat(vnc): add status controls and auto recovery (64f539f)

### Docs
- feat(vnc): add status controls and auto recovery (64f539f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.14...v4.2.15


## v4.2.14 - 2026-07-06
## What's Changed

### Frontend (Admin Web)
- fix(desktop): restore english desktop locale defaults (cbd4a72)

### Runtime & Deployment
- fix(desktop): restore english desktop locale defaults (cbd4a72)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.13...v4.2.14


## v4.2.13 - 2026-07-05
## What's Changed

### Frontend (Admin Web)
- fix(desktop): localize VNC desktop session (d832638)

### Runtime & Deployment
- fix(desktop): localize VNC desktop session (d832638)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.12...v4.2.13


## v4.2.12 - 2026-07-05
## What's Changed

### Backend (Go / API)
- fix(runtime): start SSH before desktop stack (c38189f)

### Runtime & Deployment
- fix(runtime): start SSH before desktop stack (c38189f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.11...v4.2.12


## v4.2.11 - 2026-07-05
## What's Changed

### Backend (Go / API)
- fix(runtime): re-lock DNS after network attach (c7785c3)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.10...v4.2.11


## v4.2.10 - 2026-07-03
## What's Changed

### Backend (Go / API)
- fix(hosts): harden provisioning and resource limits (acad581)

### Frontend (Admin Web)
- fix(hosts): harden provisioning and resource limits (acad581)

### Runtime & Deployment
- fix(hosts): harden provisioning and resource limits (acad581)

### Docs
- fix(hosts): harden provisioning and resource limits (acad581)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.9...v4.2.10


## v4.2.9 - 2026-06-18
## What's Changed

### Backend (Go / API)
- fix(runtime): keep prepare host status unchanged (546a323)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.8...v4.2.9


## v4.2.8 - 2026-06-18
## What's Changed

### Runtime & Deployment
- fix(network): 将 DNS hijack 限定在本地 DNS stub inbound，并对其他 DNS 协议流量执行 reject，避免公网 DNS 探针被误接管后导致 `net.dns_leak`。

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.7...v4.2.8


## v4.2.7 - 2026-06-18
## What's Changed

### Runtime & Deployment
- fix(network): 恢复容器 DNS stub 为 sing-box 支持的 direct inbound，避免新建主机因 `unknown inbound type: dns` 进入出口连通性校验失败。
- fix(network): 为容器直连分支显式绑定 `eth0`，确保代理服务器与 bypass 直连流量不被 tun 默认路由回环。

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.6...v4.2.7


## v4.2.6 - 2026-06-15
## What's Changed

### Backend (Go / API)
- fix(network): 修正容器 DNS 劫持与网络校验日志 (bd97e0b)
- Update internal/controlplane/http/admin_hosts.go (698acdf)
- feat(hosts): 支持配置主机 PIDs 限制 (1f07e46)

### Frontend (Admin Web)
- feat(hosts): 支持配置主机 PIDs 限制 (1f07e46)

### Runtime & Deployment
- fix(network): 修正容器 DNS 劫持与网络校验日志 (bd97e0b)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.5...v4.2.6


## v4.2.5 - 2026-06-02
## What's Changed

### Runtime & Deployment
- fix: update chown command to prevent recursion into bind mount points (1d8a4ff)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.4...v4.2.5


## v4.2.4 - 2026-06-02
## What's Changed

### Runtime & Deployment
- fix: entrypoint chown -R /workspace 遇 bind mount 时无容错保护，导致容器重启循环 (36f8bcb)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.3...v4.2.4


## v4.2.3 - 2026-06-01
## What's Changed

### Backend (Go / API)
- fix: strip leading slash in SPA handler and remove standalone managed-user service (e21a6ea)

### Runtime & Deployment
- fix: strip leading slash in SPA handler and remove standalone managed-user service (e21a6ea)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.2...v4.2.3


## v4.2.2 - 2026-06-01
## What's Changed

### Backend (Go / API)
- fix: correct migration 0019 test to expect DATETIME not TEXT for CURRENT_TIMESTAMP (6bfda7b)
- refactor: remove disk limit from host resource management (904bd03)

### Frontend (Admin Web)
- refactor: remove disk limit from host resource management (904bd03)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.2.1...v4.2.2


## v4.2.0 - 2026-06-01
## What's Changed

### Backend (Go / API)
- feat: PostgreSQL → SQLite 迁移 — 全项目移除 pgx/v5，改用 database/sql + modernc.org/sqlite（纯 Go，WAL 模式）
- feat: 21 个迁移文件从 PG 语法改写为 SQLite 语法
- feat: queries.go + queries_bypass.go 从 pgx 重写为 database/sql（1690 + 652 行）
- feat: App 初始化改为 sql.Open + PRAGMA WAL/foreign_keys/busy_timeout
- feat: Admin 前端 dist 通过 //go:embed 嵌入 Go 二进制，统一端口 :8080
- feat: sing-box 探针二进制内嵌至 control-plane，优先使用 native 模式

### Runtime & Deployment
- feat: Docker Compose 从 5 服务精简至 2 服务（control-plane + managed-user）
- feat: 统一环境变量 — 移除 POSTGRES_*，DATABASE_URL 改为 SQLite 路径
- feat: Makefile 移除 db/db-stop/db-reset 目标，dev 不再检测 PostgreSQL
- feat: deploy/scripts 移除 PostgreSQL 交互和检查

### Docs
- docs: README/文档站架构图、环境变量表、访问地址全面更新
- docs: FAQ 中 PG 相关排障条目更新为 SQLite

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.1.4...v4.2.0


## v4.1.4 - 2026-06-01
## What's Changed

### Backend (Go / API)
- fix: 创建主机选择"无限制"时不再转换为 NULL，与 PATCH 路径保持一致 (b86ff11)
- fix: restart 策略改为 unless-stopped，启动时立即协调恢复容器 (ddbd50a)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.1.3...v4.1.4


## v4.1.3 - 2026-05-31
## What's Changed

### Backend (Go / API)
- fix: SSH 代理选择容器最后加入的 Docker 网络 IP (3881929)
- feat: CONTAINER_REGISTRY 环境变量统一控制运行时镜像源 (666c928)

### Runtime & Deployment
- fix: image.lock 默认 image_name 改为 ghcr.io 完整路径 (588ae5b)
- feat: docker-compose.yml 镜像引用变量化，setup-env.sh 自动检测中国大陆并换源 (26c31f9)

### Docs
- feat: docker-compose.yml 镜像引用变量化，setup-env.sh 自动检测中国大陆并换源 (26c31f9)
- feat: CONTAINER_REGISTRY 环境变量统一控制运行时镜像源 (666c928)
- docs: 新增 docker-compose.cn.yml，修正毫秒镜像地址 (f9bbb4f)
- docs: 精简部署指南，突出 Docker Compose 一键部署 (8039644)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.1.2...v4.1.3


## v4.1.2 - 2026-05-31
## What's Changed

### Backend (Go / API)
- feat: 主机文件浏览支持 Windows 路径与目录选择器 (0a41582)

### Frontend (Admin Web)
- feat: 主机文件浏览支持 Windows 路径与目录选择器 (0a41582)

### Docs
- docs: 重新梳理 README 和文档站，精简表述 (492782a)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.1.1...v4.1.2


## v4.1.1 - 2026-05-31
## What's Changed

### Backend (Go / API)
- feat: v4.1.1 - 出口 IP 检测迁移、bypass 审计面板与前端多项改进 (111ccc6)

### Frontend (Admin Web)
- feat: v4.1.1 - 出口 IP 检测迁移、bypass 审计面板与前端多项改进 (111ccc6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.1.0...v4.1.1


## v4.1.0 - 2026-05-31
## What's Changed

### Backend (Go / API)
- fix: skip TestWorker_CreateHost_CleanupOnFailure when docker unavailable (da698f7)
- fix: add missing UpdateEgressIPAddress to mockWorkerRepo in agent tests (2f6d174)
- feat: v4.0 managed-user image, egress IP auto-correction, and FK fix (bba9090)
- feat(57-02): register PATCH resources route and add --storage-opt to worker (f21b6ea)
- feat(57-02): add three-state pointer semantics to Create endpoint and PatchResources handler (e0b99fe)
- feat(57-02): add DiskLimitGB to HostActionRequest, remove defaultIfZero fallbacks (bc9dd66)
- feat(57-01): change resource limit fields to pointer types, remove defaultIfZero, add UpdateHostResources (a51e055)
- feat(57-01): create migration 0022 to make host resource limit columns nullable (6050b3f)
- refactor: firewall tests, enhance worker container configuration, and add E2E test suites (a2912a0)

### Frontend (Admin Web)
- feat: v4.0 managed-user image, egress IP auto-correction, and FK fix (bba9090)
- docs(57-03): update config description to include resource limits (704fe46)
- feat(57-03): add resource limits display and edit in host detail page (2693b6d)
- feat(57-03): extend use-hosts types and integrate ResourceLimitsSelector in create dialog (227b1da)
- feat(57-03): add ResourceLimitsSelector component with preset dropdown and custom input (21f5efb)

### Runtime & Deployment
- feat: v4.0 managed-user image, egress IP auto-correction, and FK fix (bba9090)
- refactor: firewall tests, enhance worker container configuration, and add E2E test suites (a2912a0)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.0.3...v4.1.0


## v4.0.3 - 2026-05-29
## What's Changed

### Backend (Go / API)
- feat(bypass): 域名白名单全链路 + macOS 跨平台 + UI 重构 (2716e0d)

### Frontend (Admin Web)
- feat(bypass): 域名白名单全链路 + macOS 跨平台 + UI 重构 (2716e0d)

### Runtime & Deployment
- feat(bypass): 域名白名单全链路 + macOS 跨平台 + UI 重构 (2716e0d)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.0.2...v4.0.3


## v4.0.1 - 2026-05-29
## What's Changed

### Backend (Go / API)
- fix(test): withFakeNsenterRunner 加 mutex 消除 DATA RACE (fad8ec3)
- fix(v4.0.1): macOS dev 环境兼容 + bypass reload 平台拆分 + sing-box DNS 修复 (06d2a6d)
- chore(v4.0): tech debt cleanup + milestone close artifacts (92867a7)

### Runtime & Deployment
- fix(ci): ci.yml L31 YAML 语法错误 — step name 冒号需加引号 (6e8f2a1)
- fix(ci): 移除不完整的 bypass fixture 脚本，让 uat job 正确 SKIP (1aaec90)
- fix(ci): 修复 uat-bypass lint 失败 + build 多包 -o 报错 (26cb5c7)
- fix(v4.0.1): macOS dev 环境兼容 + bypass reload 平台拆分 + sing-box DNS 修复 (06d2a6d)
- fix(entrypoint): macOS dev 环境 sing-box config 权限修复 (2d8fedf)

### Docs
- chore(v4.0): tech debt cleanup + milestone close artifacts (92867a7)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v4.0...v4.0.1


## v3.5.4 - 2026-05-13
## What's Changed

### Backend (Go / API)
- fix(260513-kru): 增强 GetContainerNetNS 健壮性，修复 worker netns 获取失败 (d99d0b6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.5.3...v3.5.4


## v3.5.3 - 2026-05-13
## What's Changed

### Backend (Go / API)
- fix(260513-gii): align UpsertHost VALUES placeholder count with column list (3bdc4f0)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.5.2...v3.5.3


## v3.5.2 - 2026-05-13
## What's Changed

### Backend (Go / API)
- test(260513-fjd-01): 调整 SubnetThirdOctet 碰撞抗性阈值至 40 (0def841)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.5.1...v3.5.2


## v3.4.5 - 2026-05-11
## What's Changed

### Backend (Go / API)
- fix(network): 恢复端口映射 + 策略路由保证所有流量走代理隧道 (39322d9)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.4.4...v3.4.5


## v3.4.4 - 2026-05-11
## What's Changed

### Backend (Go / API)
- fix(network): 恢复 worker 默认路由指向 gateway 容器，修复 DNS 不通 (1d2f921)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.4.3...v3.4.4


## v3.4.3 - 2026-05-11
## What's Changed

### Backend (Go / API)
- fix(network): 移除 PREROUTING 中的非法 MASQUERADE 规则 (2633d61)
- feat(reconciler): 扩展对账器自动恢复 failed 状态主机 (3de3d6a)
- fix(rejoin): 修复 rejoin-networks 日志误导 (88d223c)

### Runtime & Deployment
- fixup! fix(systemd): 给控制面服务添加 CAP_NET_ADMIN + CAP_SYS_ADMIN capabilities (f6fdd70)
- fix(systemd): 给控制面服务添加 CAP_NET_ADMIN + CAP_SYS_ADMIN capabilities (9dc5cae)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.4.2...v3.4.3


## v3.4.2 - 2026-05-10
## What's Changed

### Backend (Go / API)
- feat: 升级镜像支持实时 pull 进度展示（SSE + 层详情） (e3404a1)

### Frontend (Admin Web)
- feat: 升级镜像支持实时 pull 进度展示（SSE + 层详情） (e3404a1)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.4.1...v3.4.2


## v3.4.1 - 2026-05-10
## What's Changed

### Backend (Go / API)
- feat(sse): add server-sent events broadcast system for real-time updates (e32b004)
- feat(network): Linux iptables 端口转发 + docker compose 预拉取镜像 + Admin 端口 2568 (195584d)
- feat(cli): 认证失败时支持交互式重试用户名密码 (c50ab42)

### Frontend (Admin Web)
- chore: 全面迁移到 pnpm，修复 CI 构建失败 (5f54dd6)
- feat(admin): integrate SSE live updates and overhaul host detail page (a5e56f1)
- style(admin): unify list page status colors via tokens and StatusDot (900dae9)
- style(admin): elevate empty state and use TableSkeleton across list pages (ac85d0b)
- feat(admin): add TableSkeleton component for differentiated loading state (c68f1a5)
- style(admin): emphasize table headers and relax row density (83f6cbb)
- feat(admin): add StatusDot component using semantic tokens (6e3e0f0)
- feat(admin): add success/warning/info semantic tokens (f0fa9ee)
- feat(network): Linux iptables 端口转发 + docker compose 预拉取镜像 + Admin 端口 2568 (195584d)

### Runtime & Deployment
- chore: 全面迁移到 pnpm，修复 CI 构建失败 (5f54dd6)
- feat(network): Linux iptables 端口转发 + docker compose 预拉取镜像 + Admin 端口 2568 (195584d)

### Docs
- chore: 全面迁移到 pnpm，修复 CI 构建失败 (5f54dd6)
- docs: sync all docs for v3.4 — SSE, local Dev Containers, error codes, architecture (54c3e62)
- feat(network): Linux iptables 端口转发 + docker compose 预拉取镜像 + Admin 端口 2568 (195584d)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.4.0...v3.4.1


## v3.4.0 - 2026-05-08
## What's Changed

### Backend (Go / API)
- fix(local): remove platform-gated SSH port mapping (95773ba)
- feat(44-01): 注册 checkSSHDForwarding 到 doctor ssh 维度 (484e623)
- feat(44-01): 新增 3 个 sshd 转发检查错误码 + parseSSHDForwarding + checkSSHDForwarding + 13 个单元测试 (bc00c7f)
- test(41-02): remote-ssh dimension unit tests + error code coverage (45a73a5)
- feat(41-01): add remote-ssh doctor dimension with 5 checks (a4de2eb)
- feat(41-01): register remote-ssh error codes and explain coverage (72d3b6a)
- fix(40): Docker error matching + SSH key injection for VS Code Remote-SSH (1f5afd6)
- feat(39-02): egress config injection + protocol detection (bdb6904)
- feat(39-01): local CLI subcommand + internal/local package + entrypoint MODE=local (c41735a)
- feat(038-02): pre-dial shared targetClient in handleConnection (2d53dc5)
- feat(038-02): implement handleGlobalRequests + proxyForwardedChannels (2b3efb8)
- test(038-02): add failing tests for handleGlobalRequests and proxyForwardedChannels (474a377)
- feat(038-01): add direct-tcpip channel dispatch and refactor dialContainer (d70c53c)
- test(038-01): add failing tests for direct-tcpip channel dispatch in proxy (d559cd7)
- feat(038-01): implement direct-tcpip forwarding with security validation (8ca26e9)
- test(038-01): add failing tests for direct-tcpip payload parsing and security validation (1346714)

### Runtime & Deployment
- fix(40): Docker error matching + SSH key injection for VS Code Remote-SSH (1f5afd6)
- feat(39-03): devcontainer.json MODE=local + sing-box entrypoint startup (295d558)
- feat(39-01): local CLI subcommand + internal/local package + entrypoint MODE=local (c41735a)
- feat(v3.2): sshd_config forwarding + devcontainer template (a218dcc)
- docs(quick-260508): 基于当前代码全面更新 README、docs 与 deploy 文档 (c71e048)

### Docs
- fix(docs): 修复 VitePress 构建失败 (58a28ce)
- docs(quick-260508): 基于当前代码全面更新 README、docs 与 deploy 文档 (c71e048)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.7...v3.4.0


## v3.3.7 - 2026-05-07
## What's Changed

### Backend (Go / API)
- fix(app): embedded 模式下也启用 Reconciler 自动恢复 — 新增 dockerInspector 直接调用 docker inspect (16fd02f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.6...v3.3.7


## v3.3.6 - 2026-05-07
## What's Changed

### Backend (Go / API)
- fix(quick-260507): user / gateway 容器 restart 策略改为 no，防止 Docker 重启后自动重建错误路由导致 IP 泄漏 (67e8d21)
- feat(quick-260507): Reconciler 对账新增自动恢复逻辑 — DB running 但 docker 缺失时自动排队启动 (03e7dcb)

### Runtime & Deployment
- fix(quick-260507): user / gateway 容器 restart 策略改为 no (67e8d21)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.5...v3.3.6


## v3.3.5 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(runtime): 镜像刷新时从 label 读取实际版本号 (c22d3bc)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.4...v3.3.5


## v3.3.4 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(network): 移除创建主机时的 egress IP smoke check (dc61287)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.3...v3.3.4


## v3.3.3 - 2026-05-06
## What's Changed

### Backend (Go / API)
- fix(quick-260506-ty7): seed admin generates entry_password + ed25519 keys + ssh_keys row, idempotent backfill (7bfc626)
- refactor(quick-260506-ty7): extract credential generators to internal/controlplane/credgen (5b0105b)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)
- fix(quick-260506): 修复状态机分裂 — list 去 DockerStatus + worker 失败路径补 stop + 前端统一 DB status (c4a164c)
- fix(quick-260506): 修复 IP 泄漏 — 统一 disconnect bridge + 反竞态 verify+retry (23a4a02)
- fix: 修复若干问题 (bd147c6)

### Frontend (Admin Web)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)
- fix(quick-260506): 修复状态机分裂 — list 去 DockerStatus + worker 失败路径补 stop + 前端统一 DB status (c4a164c)
- fix: 修复若干问题 (bd147c6)
- fix(ssh-keys): create/delete 后立即 refetch 同步状态 (f061126)

### Runtime & Deployment
- fix(quick-260506-urq): install.sh 路径改用 cp -fL 复制真实 claude 二进制 (28d18e7)
- feat(quick-260506): 容器日志查看 + egress IP 修复 + X11 lock 清理 (1fbf88f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.2...v3.3.3


## v3.3.2 - 2026-05-05
## What's Changed

### Backend (Go / API)
- refactor(admin): 重构挂载和端口映射输入交互 (ff1a8b4)
- fix(probe): docker pull 超时从 30s 放宽到 3 分钟 (e3ff822)
- fix(probe): docker pull 单独限 30 秒，超时给明确引导 (8a848fa)
- fix(probe): 保留 ghcr.io 镜像，本地存在时跳过 docker pull (6819211)
- fix(probe): 探针改回本地 gateway 镜像，去掉 ghcr.io pull (d33c835)
- fix(probe): SSE 探测超时延长至 5 分钟，增加心跳保活 (1211155)
- feat(quick-260505-gjs-01): 后端 SSE 流式探测 endpoint (4ae4948)
- fix(probe): 探针 sing-box 镜像固定为 v1.13.3 版本 (e82a8d8)
- feat(probe): 探针启动前显式拉取 sing-box 镜像，失败报错更清晰 (b89d44d)
- revert(probe): 换回 ghcr.io/sagernet/sing-box 作为 IP 探测镜像 (a1b625e)
- fix(quick-260504-elo-02): rejoinHostNetworks 加容器存在性探测，避免 macOS 误报 (5596353)
- feat(quick-260504-dtd-01): add resolveProbeNetworking helper for host-binary mode (07c4a36)
- test(quick-260504-dtd-01): add failing tests for resolveProbeNetworking (de904f7)

### Frontend (Admin Web)
- refactor(admin): 重构挂载和端口映射输入交互 (ff1a8b4)
- feat(quick-260505-gjs): 探测阶段移到状态列，去掉检测中自动弹窗 (a75e8eb)
- feat(quick-260505-gjs-02): 前端 SSE hook 与阶段性弹窗 (a1b8654)
- fix(quick-260505-fjq-02): 前端时区选择改为固定标准偏移 (ed0f63d)

### Runtime & Deployment
- fix(quick-260504-elo-01): image.lock 中 managed-user 镜像 tag 改为 latest (d1cbdf9)
- feat(quick-260504-dtd-02): default-pull sing-box-gateway and avoid restart loop (97d59a6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.1...v3.3.2


## v3.3.1 - 2026-05-03
## What's Changed

### Backend (Go / API)
- fix(quick-260504-414-01): 修复路径联想——按父目录查询+前缀过滤 (335bead)
- feat(quick-260504-414-01): implement GET /v1/admin/host-files API with security filters (9754c3e)

### Frontend (Admin Web)
- feat(quick-260504-414-02): 新建主机对话框 UX 优化 (02ef622)
- fix(quick-260504-414-01): 修复路径联想——按父目录查询+前缀过滤 (335bead)
- feat(quick-260504-414-03): replace host path inputs with PathAutocomplete in dialogs (4c482a1)
- feat(quick-260504-414-02): add PathAutocomplete component and useHostFiles hook (da36a2b)
- fix(quick-260504-2n4-02): 修复挂载管理目标路径行潜在溢出 (bf1c526)
- fix(quick-260504-2n4-01): 修复创建主机对话框挂载路径溢出 (e208501)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.3.0...v3.3.1


## v3.3.0 - 2026-05-04
## What's Changed

### Database / Migrations
- feat(creds): 0018 用户中心化凭据迁移 — 反向回填 users.entry_password、一用户一活跃 host 硬约束、DROP hosts.entry_password (e232473)

### Backend (Go / API)
- feat(creds): A2 用户中心化凭据后端重构 — admin_users.Create 生成 entry_password + ed25519 SSH 密钥对，新增 POST /admin/users/{id}/credentials/regenerate；admin_hosts.Create 加入用户已有活跃主机 → 409 拒绝；删除 GetHostByShortID/GetUserByShortID 死代码 (38fad3c)
- fix(probe): B3 IP 探测改用本地 sing-box gateway 镜像（CLOUD_CLI_PROXY_GATEWAY_IMAGE 默认 cloud-cli-proxy-sing-gateway:local），移除冗余 run -c 参数 (7ab8da7)

### Frontend (Admin Web)
- feat(creds): A3 用户中心化凭据 UI — 创建用户对话框一次性展示 SSH 公私钥 / 密码 / 指纹；用户详情页新增"重新生成 SSH 凭据"按钮 + 二次确认 + 凭据展示双对话框；移除主机创建对话框中的 SSH 凭据块与重置主机 SSH 密码按钮 (2805c51)

### Runtime & Deployment
- feat(runtime): B1 工作容器 + sing-box gateway 容器统一加 --restart=unless-stopped，宿主机重启自动恢复 (94b79b7)
- feat(image): B2 删除 claude-wrapper.sh，managed-user 镜像 binary 直接落到 /usr/local/bin/claude，顺带修复 fallback 路径 CLAUDE_BIN 找不到的潜在问题 (060edae)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.6...v3.3.0

## v3.2.6 - 2026-05-02
## What's Changed

### Frontend (Admin Web)
- feat: 前端端口映射管理 UI (a54fb0f)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.5...v3.2.6


## v3.2.4 - 2026-05-02
## What's Changed


**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.4...v3.2.4


## v3.2.5 - 2026-05-02
## What's Changed

### Backend (Go / API)
- feat: 镜像缓存状态检测与手动刷新 (2acb198)

### Frontend (Admin Web)
- feat: 镜像缓存状态检测与手动刷新 (2acb198)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.4...v3.2.5


## v3.1 - 2026-04-30
## What's Changed

### Backend (Go / API)
- fix(hot-sync): 退出不删文件 + 远程删除不再同步到本地 (64cfcc9)
- feat(quick-260424): 为 cloud-claude 添加外层会话信息面板 (77aee4d)
- feat(ui): 紧凑现代 CLI 输出 + 连接阶段状态刷新 (01b1a57)
- feat(ui): 极客风进度条 + hot-sync 安全修复 (ed8d93e)

### Runtime & Deployment
- feat(security): 容器反检测 + per-container machine-id + 遥测环境变量 (08f0a02)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.2...v3.1


## v3.2.3 - 2026-04-30
## What's Changed

### Backend (Go / API)
- feat(ui): 紧凑现代 CLI 输出 + 连接阶段状态刷新 (01b1a57)
- feat(ui): 极客风进度条 + hot-sync 安全修复 (ed8d93e)

### Runtime & Deployment
- feat(security): 容器反检测 + per-container machine-id + 遥测环境变量 (08f0a02)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.2...v3.2.3


## v3.2.2 - 2026-04-29
## What's Changed

### Runtime & Deployment
- fix(image): 修复 Claude Code 二进制路径查找 (045f50d)
- perf(image): 镜像瘦身 — 去 nodejs/npm，用官方二进制安装 Claude Code，清理 apt 缓存 (9739fe0)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v3.2.1...v3.2.2


## v2.1.2 - 2026-04-16
## What's Changed

### Frontend (Admin Web)
- fix(admin): 修复新建主机弹窗中 taskStatus 在声明前被引用 (1213659)

### Docs
- docs: Homebrew 安装与 cloud-claude 建机后连接说明 (3e2a7a5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v2.1.1...v2.1.2


## v2.1.1 - 2026-04-16
## What's Changed

### Backend (Go / API)
- fix(cloudclaude): return remote mount path in timeout error (85502d8)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v2.1.0...v2.1.1


## v2.0 - 2026-04-15
## What's Changed

### Backend (Go / API)
- feat(28-01): add --security-opt apparmor=unconfined to createHost() (fa90fbe)
- feat(27-02): pass current working directory to ConnectAndRunClaude (6614104)
- refactor(27-02): split ConnectAndRunClaude into sshConnect/mountWorkspace/runClaude three-phase architecture (5bcc546)
- test(27-01): waitForMount 单元测试 (aa29d87)
- feat(27-01): 添加 pkg/sftp 依赖并创建 mount.go (30d691a)
- feat(26-01): cobra 根命令透传与退出码统一 (125e773)
- feat(26-01): SSH 模块重构——参数接收、安全命令构建、非 TTY 分支与退出码上浮 (3a7f666)
- feat(25-01): SSH 会话增加连接超时与根命令退出码完善 (8550bfe)
- feat(25-01): 完善 Entry 认证响应处理与就绪轮询错误分类 (5d18243)
- feat(25-01): cloud-claude CLI 骨架——cobra 入口、配置模块、init 子命令 (6fd35c3)
- feat(24-01): Worker 创建容器附加 FUSE 设备和 SYS_ADMIN 能力 (07a7b06)

### Runtime & Deployment
- feat(28-02): add FUSE kernel module detection to host-preflight.sh (bf22560)
- feat(24-01): 受管镜像预装 sshfs/fuse3 并配置 FUSE 权限 (d853b50)

### Docs
- docs(28-02): add FUSE prerequisites and AppArmor compatibility to deployment guides (fca403e)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.10...v2.0


## v1.6.10 - 2026-04-10
## What's Changed

### Backend (Go / API)
- fix(ssh): generate valid outbound keys (0daecbd)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.9...v1.6.10


## v1.6.9 - 2026-04-08
## What's Changed

### Runtime & Deployment
- fix(runtime): remove tmux wrapper from claude (da09ba9)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.8...v1.6.9


## v1.6.8 - 2026-04-08
## What's Changed

### Backend (Go / API)
- fix(ssh): 代理连容器认证顺序、create 后同步凭据与稳定解析容器 IP (05f763b)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.7...v1.6.8


## v1.6.7 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

### Frontend (Admin Web)
- fix: curl入口变量遮蔽bug + 增强日志 + SSH快捷复制按钮 (ef07560)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.6...v1.6.7


## v1.6.6 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: curl 入口改用主机 short_id 替代用户 short_id (82a9ef6)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.5...v1.6.6


## v1.6.5 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理用自己的密钥连容器，不再依赖密码 (069f2ac)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.4...v1.6.5


## v1.6.4 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH代理支持公钥认证（入站密钥免密登录） (4169f85)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.3...v1.6.4


## v1.6.3 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

### Frontend (Admin Web)
- feat: SSH密钥与容器双向实时同步 (5eb1477)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.2...v1.6.3


## v1.6.2 - 2026-04-05
## What's Changed

### Backend (Go / API)
- fix: SSH密钥API响应格式与前端对齐 (195aefe)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.1...v1.6.2


## v1.6.1 - 2026-04-05
## What's Changed

### Runtime & Deployment
- fix(ci): 镜像构建仅由打 tag 触发，移除 push to main 触发 (b3353d0)
- fix: control-plane 镜像缺少 migrations 目录导致数据库迁移不执行 (4f33cd5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.6.0...v1.6.1


## v1.6.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

### Frontend (Admin Web)
- feat(260405-qk2): SSH密钥体系改造：拆分为入站免密登录和出站外部鉴权 (8b994dc)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.1...v1.6.0


## v1.5.1 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

### Frontend (Admin Web)
- feat(260405-jji): 镜像版本管理：自动拉取最新镜像 + 版本展示 + 一键升级 (507b288)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.5.0...v1.5.1


## v1.5.0 - 2026-04-05
## What's Changed

### Backend (Go / API)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add 5 container management API endpoints (e8de4a8)

### Frontend (Admin Web)
- feat(260405-hio): structured Claude Code settings panel and system fingerprint (bea7886)
- feat(260405-hai): enhance Claude status API with per-process details (2ae9592)
- feat(260405-h13): add frontend hooks, dialogs and Claude status card (6affd42)

### Docs
- docs: add UI screenshots and local development setup guide (bae69a5)
- docs: refine hero glow to vitest-like large halo (6720691)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.6...v1.5.0


## v1.4.6 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): lowercase ghcr image prefix for registry cache (6f420d8)

### Docs
- docs: add hero logo and glow effect on homepage (cd689f5)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.5...v1.4.6


## v1.4.5 - 2026-04-02
## What's Changed

### Runtime & Deployment
- perf(ci): improve docker layer cache hit rate (6e410d4)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.4...v1.4.5


## v1.4.4 - 2026-04-02
## What's Changed

### Runtime & Deployment
- fix(ci): source pnpm version from root package manager (6cce3f8)

**Full Changelog:** https://github.com/ZaneL1u/cloud-cli-proxy/compare/v1.4.3...v1.4.4
