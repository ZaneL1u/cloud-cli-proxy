# 参数参考

## 命令



### doctor — 五维度自检

诊断五个维度的问题，可聚焦单项，也可自动修复。

| 维度 | 检查内容 |
|------|----------|
| network | 容器网络连通性，出口 IP 可达性 |
| auth | 认证凭据有效性，Token 可用性 |
| ssh | SSH 连接稳定性，密钥配置 |
| mount | sshfs 挂载状态，FUSE 兼容性 |
| disk | 磁盘空间，inode 使用率 |

```bash
cloud-claude doctor                  # 五项全检
cloud-claude doctor network          # 仅检查网络
cloud-claude doctor mount --fix      # 检查挂载并自动修复
```

### env check — 远端环境检查

```bash
cloud-claude env check
```

输出远端容器的时区、语言环境、出口 IP、FUSE 状态、工具链版本等信息。

### explain — 错误码解释

```bash
cloud-claude explain MOUNT_SSHFS_DISCONNECTED
```

输出该错误码的含义、可能原因和修复建议。



## 配置参考

### 环境变量

创建 `/etc/cloud-cli-proxy/env`（systemd 部署）或 `.env`（Docker Compose 部署）。推荐使用 `setup-env.sh` 交互式生成。

#### 控制面

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | 否 | `file:/data/cloud-cli-proxy.db` | SQLite 数据库文件路径 |
| `CONTROL_PLANE_ADDR` | 否 | `:8080` | HTTP API 监听地址 |
| `ADMIN_USERNAME` | 否 | `admin` | 管理员用户名 |
| `ADMIN_PASSWORD` | 是 | — | 管理员密码（首次启动种子） |
| `ADMIN_JWT_SECRET` | 是 | — | JWT 签名密钥（至少 32 字符） |
| `HOST_AGENT_MODE` | 否 | `socket` | `socket` 独立进程 / `embedded` 嵌入控制面 |
| `HOST_AGENT_SOCKET` | 否 | `/run/cloud-cli-proxy/host-agent.sock` | Agent socket 路径 |
| `DATA_DIR` | 否 | `/var/lib/cloud-cli-proxy` | 数据目录 |
| `SSH_PROXY_ADDR` | 否 | `:2222` | SSH 代理监听地址 |
| `LOG_FORMAT` | 否 | `json` | 日志格式：`json` / `text` |
| `LOG_LEVEL` | 否 | `info` | 日志级别：`debug` / `info` / `warn` / `error` |

#### 数据库（SQLite）

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | 否 | `file:/data/cloud-cli-proxy.db` | SQLite 数据库文件路径 |

#### 服务端口

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CONTROL_PLANE_ADDR` | `:8080` | 控制面监听地址（API + 管理后台 + SSH 代理） |
| `SSH_PROXY_PORT` | `2222` | SSH 代理端口 |

### cloud-claude 配置

`~/.cloud-claude/config.yaml`：

```yaml
gateway: https://gw.example.com
short_id: abc123
proxy_commands:
  - git
hot_sync_max_file_mb: 50
```

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `gateway` | 控制面 HTTPS 地址 | — |
| `short_id` | 主机短 ID | — |
| `proxy_commands` | 在本机执行的命令列表 | `["git"]` |
| `hot_sync_max_file_mb` | 单文件熔断阈值 | `50` |

环境变量：

- `CLOUD_CLAUDE_GATEWAY` — 同 `gateway`
- `CLOUD_CLAUDE_SHORT_ID` — 同 `short_id`
- `CLOUD_CLAUDE_PASSWORD` — 登录密码
- `CLOUD_CLAUDE_NO_PROMOTION=1` — 禁用冷文件晋升

## 代理协议

`proxy` 类型的出口 IP 需要填写 `proxy_config`，格式遵循 [sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/)。

支持 SOCKS5、Shadowsocks、VMess、VLESS、Trojan、HTTP 六种协议。

### SOCKS5

```json
{
  "type": "socks",
  "server": "192.0.2.50",
  "server_port": 1080,
  "username": "user",
  "password": "pass"
}
```

### Shadowsocks

```json
{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}
```

支持的加密方法：`aes-128-gcm`、`aes-256-gcm`、`chacha20-ietf-poly1305`。

### VMess

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

### Trojan

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

### HTTP

```json
{
  "type": "http",
  "server": "192.0.2.100",
  "server_port": 8080,
  "username": "user",
  "password": "pass"
}
```

管理后台的出口 IP 表单提供协议选择器和对应字段，也支持直接编辑 JSON。

## 防火墙

### 容器级别

Host Agent 用 nftables 为每个容器的 netns 设置默认拒绝策略。规则由 agent 自动管理，无需手动配置。

### 宿主机级别

建议在宿主机上配置基本防火墙：

```bash
nft add table inet filter
nft add chain inet filter input '{ type filter hook input priority 0; policy drop; }'
nft add rule inet filter input ct state established,related accept
nft add rule inet filter input iif lo accept
nft add rule inet filter input tcp dport 22 accept
nft add rule inet filter input tcp dport 8080 accept
# 管理后台已内嵌至 control-plane，无需单独端口
nft add rule inet filter input tcp dport 2222 accept
```

## Docker 镜像

所有镜像通过 GitHub Actions 构建，支持 `linux/amd64` 和 `linux/arm64`。

| 镜像 | 地址 |
|------|------|
| control-plane | `ghcr.io/zanel1u/cloud-cli-proxy/control-plane` |
| managed-user | `ghcr.io/zanel1u/cloud-cli-proxy/managed-user` |

标签规则：

| 标签 | 说明 |
|------|------|
| `latest` | main 分支最新 |
| `1.2.3` | 发布版本 |
| `1.2` | 跟随最新 patch |
| `1` | 跟随最新 minor |

生产环境建议锁定版本号。

### 用户容器预装软件

| 软件 | 说明 |
|------|------|
| OpenSSH Server | SSH 接入 |
| Claude Code | AI 编程助手 |
| KasmVNC + Chromium | 远程桌面 |
| sing-box | 隧道客户端 |
| Git, tmux, zsh | 开发工具 |
| Node.js | JavaScript 运行时 |
