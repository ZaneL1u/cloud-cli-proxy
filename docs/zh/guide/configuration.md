# 配置参考

## 环境变量

创建 `/etc/cloud-cli-proxy/env`（systemd 部署）或 `.env`（Docker Compose 部署）。

使用 `setup-env.sh` 可以交互式生成配置：

```bash
bash deploy/scripts/setup-env.sh
```

### 控制面

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | 是 | — | PostgreSQL 连接字符串，格式：`postgres://user:pass@host:5432/db?sslmode=disable` |
| `CONTROL_PLANE_ADDR` | 否 | `:8080` | 控制面 HTTP API 监听地址 |
| `ADMIN_USERNAME` | 否 | `admin` | 管理员用户名 |
| `ADMIN_PASSWORD` | 是 | — | 管理员密码，首次启动时作为种子密码 |
| `ADMIN_JWT_SECRET` | 是 | — | JWT 签名密钥（至少 32 字符），未设置则禁用管理后台 API |
| `HOST_AGENT_MODE` | 否 | `socket` | host-agent 模式。`socket` = 通过 Unix socket 连接独立进程，`embedded` = 嵌入控制面进程内运行 |
| `HOST_AGENT_SOCKET` | 否 | `/run/cloud-cli-proxy/host-agent.sock` | host-agent Unix socket 路径（仅 socket 模式） |
| `DATA_DIR` | 否 | `/var/lib/cloud-cli-proxy` | 数据目录，存放运行时文件 |
| `SSH_PROXY_ADDR` | 否 | `:2222` | SSH 代理监听地址 |
| `LOG_FORMAT` | 否 | `json` | 日志格式，`json` 或 `text` |
| `LOG_LEVEL` | 否 | `info` | 日志级别，`debug` / `info` / `warn` / `error` |

### 数据库（Docker Compose 内置 PostgreSQL）

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DB_MODE` | 否 | `docker` | 数据库模式，`docker` = 内置，`external` = 外部 |
| `POSTGRES_DB` | 否 | `cloudproxy` | 数据库名 |
| `POSTGRES_USER` | 否 | `cloudproxy` | 数据库用户名 |
| `POSTGRES_PASSWORD` | 是（docker 模式） | — | 数据库密码 |

### 管理后台

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `ADMIN_PORT` | 否 | `3000` | 管理后台前端端口（映射到容器 80 端口） |

### Docker Compose 端口映射

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SSH_PROXY_PORT` | `2222` | 宿主机上的 SSH 代理端口 |
| `ADMIN_PORT` | `3000` | 宿主机上的管理后台端口 |

## 代理协议配置

对于 `proxy` 类型的出口 IP，需要提供 `proxy_config` JSON 字段。该字段遵循 [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/) 格式。

### 支持的协议

#### SOCKS5

```json
{
  "type": "socks",
  "server": "192.0.2.50",
  "server_port": 1080,
  "username": "user",
  "password": "pass"
}
```

#### Shadowsocks

```json
{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}
```

支持的加密方法：`aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305` 等。

#### VMess

```json
{
  "type": "vmess",
  "server": "203.0.113.20",
  "server_port": 443,
  "uuid": "your-uuid",
  "security": "auto",
  "alter_id": 0
}
```

#### Trojan

```json
{
  "type": "trojan",
  "server": "203.0.113.30",
  "server_port": 443,
  "password": "your-password",
  "tls": {
    "enabled": true,
    "server_name": "your-domain.com"
  }
}
```

#### HTTP

```json
{
  "type": "http",
  "server": "192.0.2.100",
  "server_port": 8080,
  "username": "user",
  "password": "pass"
}
```

### 在管理后台中配置

管理后台的出口 IP 创建/编辑表单提供协议选择器和对应的配置字段，也支持直接编辑 JSON。

## 防火墙规则

### 容器级别

host-agent 使用 nftables 为每个容器的网络命名空间设置默认拒绝策略：

- **Proxy 模式**：仅允许到代理服务器的连接，禁止其他所有出站流量

规则由 host-agent 自动管理，无需手动配置。

### 宿主机级别

建议在宿主机上配置基本防火墙：

```bash
nft add table inet filter
nft add chain inet filter input '{ type filter hook input priority 0; policy drop; }'
nft add rule inet filter input ct state established,related accept
nft add rule inet filter input iif lo accept
nft add rule inet filter input tcp dport 22 accept     # 宿主机 SSH
nft add rule inet filter input tcp dport 8080 accept   # API
nft add rule inet filter input tcp dport 3000 accept   # 管理后台
nft add rule inet filter input tcp dport 2222 accept   # SSH 代理
```

## Docker 镜像

所有镜像通过 GitHub Actions 自动构建，支持 `linux/amd64` 和 `linux/arm64`。

| 镜像 | 地址 | 说明 |
|------|------|------|
| control-plane | `ghcr.io/zanel1u/cloud-cli-proxy/control-plane` | 控制面 API 服务 |
| admin | `ghcr.io/zanel1u/cloud-cli-proxy/admin` | 管理后台前端（Nginx） |
| managed-user | `ghcr.io/zanel1u/cloud-cli-proxy/managed-user` | 用户容器镜像 |
| sing-box-gateway | `ghcr.io/zanel1u/cloud-cli-proxy/sing-box-gateway` | sing-box 网关 sidecar |

**镜像标签规则：**

| 标签 | 说明 |
|------|------|
| `latest` | main 分支最新构建 |
| `1.2.3` | 发布版本，对应 GitHub Release |
| `1.2` | 自动跟随最新 patch |
| `1` | 自动跟随最新 minor |
| `a1b2c3d` | 精确到提交 |

**生产环境建议锁定版本：**

```bash
docker pull ghcr.io/zanel1u/cloud-cli-proxy/control-plane:1.2.3
```

## 用户容器预装软件

受管用户镜像基于 Ubuntu 24.04，预装：

| 软件 | 版本 | 说明 |
|------|------|------|
| OpenSSH Server | 10.2p1 | SSH 接入 |
| Claude Code | 最新 | AI 编程助手 |
| KasmVNC | 1.4.0 | 远程桌面服务 |
| Chromium | 最新 | 浏览器（配合 KasmVNC） |
| Fluxbox | — | 轻量窗口管理器 |
| sing-box | 1.13.3 | 代理模式隧道客户端 |
| Git, tmux, zsh | — | 常用开发工具 |
| Node.js | LTS | JavaScript 运行时 |
