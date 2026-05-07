# Requirements: Cloud CLI Proxy v3.2

**Defined:** 2026-05-07
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

## v3.2 Requirements

### Remote SSH 支持（SSH-REMOTE）

- [ ] **SSH-01**: SSH Proxy 支持 `direct-tcpip` channel 转发
  - 解析 `direct-tcpip` payload（目标 host:port）
  - 通过已有 SSH 连接向容器侧请求 `direct-tcpip`
  - 双向并发拷贝数据
  - 同一 SSH 连接支持多个 forwarding channel

- [ ] **SSH-02**: SSH Proxy 支持 `tcpip-forward` 全局请求和 `forwarded-tcpip` channel
  - 处理客户端的 `tcpip-forward` 请求（远程端口转发）
  - 当远程端口有连接时，打开 `forwarded-tcpip` channel 回传客户端

- [ ] **SSH-03**: 容器内 `sshd_config` 显式开启端口转发
  - `AllowTcpForwarding yes`
  - `AllowStreamLocalForwarding yes`
  - `GatewayPorts no`（安全：只允许本地转发）

- [ ] **SSH-04**: SSH Proxy 对 forwarding 目标做安全校验
  - 拒绝转发到管理网段（10.99.x.x）
  - 拒绝转发到 Docker socket、metadata 端点
  - 只允许转发到容器 netns 内地址或用户显式白名单

- [ ] **SSH-05**: VS Code Remote-SSH 端到端验证
  - VS Code 能连接 SSH Proxy 2222 端口
  - VS Code Server 能在容器内自动下载并启动
  - 端口转发（语言服务器、调试器）正常工作
  - 容器内 `claude` 命令在 VS Code terminal 中可用

### 本地 Dev Containers 支持（LOCAL）

- [ ] **LOCAL-01**: `cloud-claude local` 子命令支持一键启动本地容器
  - 不连接 control-plane/Entry API，直接调用本地 Docker
  - 自动拉取/构建 managed-user 镜像
  - 创建容器并 publish SSH 端口到宿主机
  - 输出连接信息（host, port, user, password）

- [ ] **LOCAL-02**: 本地容器支持 Dev Containers 配置
  - 项目根目录 `.devcontainer/devcontainer.json` 模板
  - 引用 managed-user 镜像
  - `workspaceMount` bind mount 当前目录到 `/workspace`
  - `runArgs` 包含 `--cap-add SYS_ADMIN --device /dev/fuse`

- [ ] **LOCAL-03**: 本地容器支持 sing-box 全隧道（可选配置）
  - 通过 `cloud-claude local --egress-config <file>` 注入 sing-box outbound JSON
  - 容器内 sing-box 自动启动 tun 模式
  - 本地宿主机 macOS 上支持 SOCKS/HTTP 代理模式兜底

- [ ] **LOCAL-04**: entrypoint 支持 `MODE=local` 分支
  - 本地模式跳过 KasmVNC 启动（节省资源）
  - 本地模式跳过 control-plane 心跳
  - 本地模式仍启动 sshd + sing-box

### 安全与验证（SEC）

- [ ] **SEC-01**: 验证 `direct-tcpip` 转发流量走 sing-box tun
  - 通过 VS Code 端口转发访问外部服务时，出口 IP 必须是绑定的 egress IP
  - 不能出现绕过 tun 直接走宿主机路由的情况

- [ ] **SEC-02**: VS Code Server 下载/扩展安装流量也走受控出口
  - `update.code.visualstudio.com` 等域名通过 sing-box 出站
  - 不因为 VS Code 的流量破坏出口 IP 强约束

### 诊断与体验（UX）

- [ ] **UX-01**: `cloud-claude doctor` 新增 remote-ssh 诊断维度
  - 检测 VS Code Server 进程是否存在
  - 检测 `~/.vscode-server/` 磁盘占用
  - 检测 forwarding channel 是否被拦截

- [ ] **UX-02**: `cloud-claude local` 支持 `down` / `status` 子命令
  - `local down` 停止并清理本地容器
  - `local status` 显示本地容器运行状态、端口映射

## v2 Requirements (Deferred)

### Remote SSH 增强

- **SSH-06**: `~/.vscode-server` 持久化 volume（容器重建后保留扩展和设置）
- **SSH-07**: VS Code 多窗口/多工作区支持（多个 forwarding channel 并发优化）
- **SSH-08**: Remote SSH 的 `localServer` 模式支持（Windows 特殊路径）

### Local 增强

- **LOCAL-05**: `cloud-claude local --sync-config` 从云端拉取 egress IP 配置
- **LOCAL-06**: 本地容器预热镜像（减少首次启动时间）
- **LOCAL-07**: Windows Docker Desktop 支持验证

### 诊断增强

- **UX-03**: doctor 本地模式适配（跳过 auth/egress 检查，简化网络检查）

## Out of Scope

| Feature | Reason |
|---------|--------|
| VS Code Remote-Tunnels | 走微软中继服务器，完全绕过出口 IP 强约束 |
| 在容器内预装 VS Code Server | VS Code 自动下载对应版本，预装反而版本不匹配 |
| 本地容器连接 control-plane | 本地模式的核心价值是"不依赖云端" |
| 本地容器做到期治理 | 本地容器是用户自己机器上的资源 |
| 支持 Docker Compose 多服务（本地） | v3.2 聚焦单容器体验 |
| 计费/套餐/支付 | 延续 v1 约束，核心能力验证前不做 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| SSH-01 | Phase 38 | Pending |
| SSH-02 | Phase 38 | Pending |
| SSH-03 | Phase 38 | Pending |
| SSH-04 | Phase 38 | Pending |
| SSH-05 | Phase 40 | Pending |
| LOCAL-01 | Phase 39 | Pending |
| LOCAL-02 | Phase 39 | Pending |
| LOCAL-03 | Phase 39 | Pending |
| LOCAL-04 | Phase 39 | Pending |
| SEC-01 | Phase 40 | Pending |
| SEC-02 | Phase 40 | Pending |
| UX-01 | Phase 41 | Pending |
| UX-02 | Phase 39 | Pending |

**Coverage:**
- v3.2 requirements: 13 total
- Mapped to phases: 13
- Unmapped: 0

---
*Requirements defined: 2026-05-07*
*Last updated: 2026-05-07 after research synthesis*
