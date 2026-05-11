# cloud-claude local — 本地 Dev Containers

`cloud-claude local` 允许用户在本地 Docker 中启动与云端同构的受管容器，用于本地开发、调试和离线场景。

## 快速开始

### 1. 初始化配置

```bash
cloud-claude local init
```

生成 `~/.cloud-claude/local.yaml`，包含本地容器默认参数。

### 2. 启动本地容器

```bash
cloud-claude local up
```

该命令会：
- 拉取受管镜像（如本地不存在）
- 创建 `--network=none` 容器（与云端同构）
- 注入 sing-box outbound 配置（支持 tun/proxy 双模式）
- 跳过 KasmVNC 和心跳逻辑，仅保留 sshd + sing-box
- 自动挂载本地 SSH 密钥

### 3. 查看状态

```bash
cloud-claude local status
```

显示本地容器状态、SSH 端口和 sing-box 隧道状态。

### 4. 停止容器

```bash
cloud-claude local down
```

停止并可选删除本地容器。

## 出口 IP 配置

使用 `--egress-config` 注入 sing-box outbound JSON：

```bash
cloud-claude local up --egress-config '{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}'
```

支持协议：Shadowsocks、VMess、SOCKS5、Trojan、HTTP。

## VS Code Dev Containers 集成

项目根目录提供 `.devcontainer/devcontainer.json` 模板，支持 VS Code Remote-Containers：

```json
{
  "name": "cloud-claude-local",
  "image": "ghcr.io/your-org/managed-user:v3.4.0",
  "runArgs": ["--network=none"],
  "postCreateCommand": "sing-box run -c /etc/sing-box/outbound.json"
}
```

## 与云端容器的差异

| 特性 | 云端容器 | 本地容器 |
|------|----------|----------|
| 网络隔离 | `--network=none` + sing-box tun | `--network=none` + sing-box tun/proxy |
| 桌面环境 | KasmVNC + Fluxbox + Chromium | 无（纯终端） |
| 心跳保活 | 有 | 无 |
| 到期治理 | 管理员控制 | 用户自行管理 |
| 持久卷 | Docker named volume | Docker named volume |

## 典型场景

- **离线开发**：无网络时在本机启动容器继续工作
- **本地调试**：在本地复现云端环境问题
- **快速实验**：无需等待云端容器启动即可测试配置
- **CI/CD 基线**：在本地验证容器镜像行为后再部署

## 命令参考

```
cloud-claude local up [flags]
  --egress-config string   sing-box outbound JSON
  --image string           自定义镜像（默认受管镜像）
  --name string            容器名称
  --rm                     停止后自动删除容器

cloud-claude local down [flags]
  --name string            目标容器名称
  --volumes                同时删除关联卷

cloud-claude local status
  显示所有本地容器及其状态
```
