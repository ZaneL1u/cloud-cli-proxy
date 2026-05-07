# Phase 39: 本地 Dev Containers 支持 - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

用户通过 `cloud-claude local` 在本地机器上一键启动独立 managed-user 容器，支持 VS Code Dev Containers 工作流，无需连接 control-plane 或 Entry API。本地容器可选启用 sing-box 全隧道出网。`local down` 和 `local status` 提供生命周期管理。

</domain>

<decisions>
## Implementation Decisions

### CLI 子命令结构
- `cloud-claude local` 作为子命令组，采用 cobra 模式（与现有 init/env/ssh 一致）
- `local` 本身默认行为 = `local up`（启动容器）
- `local down` 停止并移除容器
- `local status` 显示容器运行状态、端口映射
- 容器使用固定标签 `cloud-claude-local` 便于 `down`/`status` 识别

### Entrypoint MODE 分支
- 入口 `MODE` 环境变量控制行为：`MODE=remote`（默认，现有行为）vs `MODE=local`
- `MODE=local` 跳过：KasmVNC 配置/启动、Xvnc/fluxbox/pcmanfm/Chromium 桌面栈、prepare_v3_dirs 等远程专有阶段
- `MODE=local` 保留：sshd 启动、sing-box 启动（如有 egress 配置）、用户密码设置、SSH keygen
- KasmVNC 跳过逻辑放在 entrypoint 前段，用 `if [ "$MODE" != "local" ]; then` 包裹整个桌面栈

### 容器启动方式
- 直接调用本地 Docker API（Go docker client SDK），不连接 control-plane
- 自动检测并拉取/使用 managed-user 镜像（复用现有镜像名约定）
- SSH 端口随机分配或通过 `--port` flag 指定，publish 到宿主机 127.0.0.1
- 启动完成后输出连接信息：host, port, user, password

### Egress 配置注入
- `cloud-claude local --egress-config <file>` 接受 sing-box outbound JSON 文件路径
- 文件通过 docker cp 或 bind mount 注入到容器内固定路径 `/etc/cloud-claude/sing-box-outbound.json`
- 容器 entrypoint 检测到该文件时自动启动 sing-box tun 模式
- 未提供 `--egress-config` 时容器不启动 sing-box（纯本地开发场景，无隧道开销）

### macOS 代理兜底
- macOS 宿主机无 root 权限做 tun 设备，使用 SOCKS/HTTP 代理模式
- `--egress-config` 文件中若指定 socks/http 出站协议，entrypoint 自动切换为代理模式而非 tun
- 容器内设置 `ALL_PROXY` / `HTTP_PROXY` 环境变量指向本地代理端口

### Devcontainer.json 适配
- 已有 `.devcontainer/devcontainer.json` 模板可复用
- 传入 `MODE=local` 环境变量到容器（通过 `containerEnv` 或 `runArgs --env`）
- SSH 端口通过 `forwardPorts` 暴露给 VS Code Remote-SSH

### Claude's Discretion
- 容器命名和标签策略（`cloud-claude-local` + 项目路径哈希区分多项目）
- `local status` 输出格式（table 或 key-value）
- SSH 密码自动生成方式（随机密码 vs 固定默认密码）
- 容器资源限制（CPU/memory 默认值）
- 错误提示文案和连接信息输出格式

</decisions>

<specifics>
## Specific Ideas

- 入口体验应尽量接近 `cloud-claude` 主命令的顺滑感：一条命令启动，输出连接信息即可使用
- VS Code Dev Containers 打开项目时应自动识别 `.devcontainer/devcontainer.json`，无需额外配置
- 本地容器不需要 KasmVNC 桌面环境，节省资源

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `cmd/cloud-claude/main.go`: cobra CLI 骨架，已有 root/init/env/ssh/sync/sessions 子命令，新增 `local` 子命令组直接复用
- `internal/cloudclaude/`: SSH 连接、session 管理、mount 等逻辑，local 模式可复用部分 session 和 mount 策略
- `deploy/docker/managed-user/entrypoint.sh`: 容器入口脚本，在此基础上加 MODE 分支
- `.devcontainer/devcontainer.json`: 已有模板，需微调支持 MODE=local
- `internal/network/`: sing-box 配置和 outbound 解析逻辑可复用

### Established Patterns
- cobra 子命令注册模式（main.go AddCommand 链）
- Docker SDK 使用模式（runtime_service.go 中已有容器操作）
- sing-box outbound JSON 格式（gateway_singbox_config.go）

### Integration Points
- `cmd/cloud-claude/main.go`: 注册 `local` 子命令组
- `deploy/docker/managed-user/entrypoint.sh`: MODE 分支改造
- Docker API: 本地容器创建/启动/停止/状态查询
- `.devcontainer/devcontainer.json`: 配置更新

</code_context>

<deferred>
## Deferred Ideas

- LOCAL-05: `--sync-config` 从云端拉取 egress IP 配置 — 后续版本
- LOCAL-06: 本地容器预热镜像 — 后续版本
- LOCAL-07: Windows Docker Desktop 支持验证 — 后续版本
- UX-03: doctor 本地模式适配 — Phase 41 Doctor 扩展

</deferred>

---

*Phase: 39-dev-containers*
*Context gathered: 2026-05-07*
