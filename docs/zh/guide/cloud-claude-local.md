# cloud-claude local — 本地 Dev Containers

`cloud-claude local` 在本地 Docker 中启动与云端同构的容器，用于开发、调试和离线场景。

## 使用

### 初始化

```bash
cloud-claude local init
```

生成 `~/.cloud-claude/local.yaml`。

### 启动

```bash
cloud-claude local up
```

自动拉取镜像、创建 `--network=none` 容器、注入 sing-box 配置。跳过 KasmVNC 和心跳，仅保留 sshd + sing-box。

### 查看状态

```bash
cloud-claude local status
```

### 停止

```bash
cloud-claude local down
```

## 出口 IP 配置

```bash
cloud-claude local up --egress-config '{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}'
```

支持 Shadowsocks、VMess、SOCKS5、Trojan、HTTP。

## VS Code Dev Containers 集成

项目根目录提供 `.devcontainer/devcontainer.json` 模板：

```json
{
  "name": "cloud-claude-local",
  "image": "ghcr.io/your-org/managed-user:v3.4.0",
  "runArgs": ["--network=none"],
  "postCreateCommand": "sing-box run -c /etc/sing-box/outbound.json"
}
```

## 与云端容器的差异

| 特性 | 云端 | 本地 |
|------|------|------|
| 网络 | `--network=none` + sing-box tun | 同左 |
| 桌面 | KasmVNC + Chromium | 无 |
| 心跳 | 有 | 无 |
| 到期治理 | 管理员控制 | 用户自管 |
| 持久卷 | Docker named volume | 同左 |

## 典型场景

- 离线开发：无网络时在本机继续工作
- 本地调试：复现云端环境问题
- 快速实验：不等云端容器启动即可测试配置
- CI/CD 基线：验证容器镜像行为后再部署

## 命令参考

```
cloud-claude local up [flags]
  --egress-config string   sing-box outbound JSON
  --image string           自定义镜像
  --name string            容器名称
  --rm                     停止后自动删除

cloud-claude local down [flags]
  --name string            目标容器
  --volumes                同时删除关联卷

cloud-claude local status
  显示所有本地容器及状态
```
