# 快速开始

## Docker Compose 部署

### 前置要求

- Linux 宿主机（Ubuntu 22.04+ / Debian 12+）
- Docker Engine 28+，Docker Compose v2
- 至少一个出口 IP（代理服务器）

### 1. 克隆仓库

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 2. 生成环境配置

```bash
bash deploy/scripts/setup-env.sh
```

脚本支持两种数据库方案：

- **内置 Docker PostgreSQL**：自动生成数据库密码，Docker Compose 统一管理。
- **外部 PostgreSQL**：交互式填入地址、端口、用户名、密码，支持 SSL。

两种方案都会自动生成管理员密码（20 位）和 JWT 密钥（48 位）。

::: warning 重要
脚本执行完毕后会显示管理员密码，此密码只显示一次，请立即保存。
:::

### 3. 启动服务

```bash
# 内置 PostgreSQL
docker compose pull
docker compose up -d

# 外部 PostgreSQL（跳过内置数据库）
docker compose pull control-plane admin
docker compose up -d control-plane admin
```

如果预构建镜像不可用，可以从源码构建：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 4. 验证

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

服务地址：

- **API**：`http://YOUR_HOST:8080`
- **管理后台（内嵌）**：`http://YOUR_HOST:8080`
- **SSH 代理**：`YOUR_HOST:2222`

## 给用户开机器

完整流程：**登录 → 添加出口 IP → 创建用户 → 创建主机并绑定 → 分发接入命令**。

### 1. 获取管理员 Token

可以通过管理后台登录，也可以通过 API：

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"你的管理员密码"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

### 2. 添加出口 IP

出口 IP 通过 sing-box tun 全隧道工作。`tunnel_type` 为 `proxy`，在 `proxy_config` 中填写上游代理配置。

支持 6 种协议：SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP。

```bash
# Shadowsocks
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "jp-ss-01",
    "ip_address": "198.51.100.5",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "shadowsocks",
      "server": "198.51.100.5",
      "server_port": 8388,
      "method": "aes-256-gcm",
      "password": "your-ss-password"
    }
  }'
```

```bash
# SOCKS5
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-socks-01",
    "ip_address": "192.0.2.50",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "socks",
      "server": "192.0.2.50",
      "server_port": 1080,
      "username": "user",
      "password": "pass"
    }
  }'
```

测试出口 IP 连通性：

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

测试结果包含连通性、出口 IP 匹配和 DNS 泄漏三项检测。

### 3. 创建用户

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "zhangsan",
    "password": "给用户的初始密码",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

### 4. 创建主机并绑定出口 IP

```bash
# 创建主机
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "用户UUID"}'

# 绑定出口 IP
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id": "主机UUID", "egress_ip_id": "出口IP的UUID"}'
```

::: tip
主机至少需要绑定一个出口 IP 才能正常启动。
:::

### 5. 分发接入信息

主机创建完成、任务状态显示容器已就绪后，在管理后台「主机详情」页复制接入命令。

**方式一：curl + SSH**

把下面命令发给用户（替换 `YOUR_HOST` 和 `SHORT_ID`）：

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

也可以使用 bootstrap 方式（用户输入用户名）：

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

**方式二：cloud-claude CLI（推荐）**

除上面的 `curl` 命令外，还需把以下三项一并发给用户：

| 信息 | 说明 |
|------|------|
| **网关地址** | 控制面对外的 HTTPS 地址，例如 `https://gw.example.com` |
| **Short ID** | 主机详情页的主机短 ID |
| **密码** | 用户在后台的登录密码 |

用户安装 `cloud-claude` 并执行 `init` 填入上述信息后，在项目目录下运行 `cloud-claude` 即可。当前目录会通过 sshfs 挂载到容器内同名路径。

## 用户使用方式

### cloud-claude CLI（推荐）

#### 安装

**Homebrew（macOS / Linux）：**

```bash
brew tap ZaneL1u/tap
brew install cloud-claude
```

**一行脚本：**

```bash
curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash
```

也可从 [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases) 下载或 `go build ./cmd/cloud-claude`。

#### 首次配置

```bash
cloud-claude init
```

按提示输入网关地址、Short ID 和密码，写入 `~/.cloud-claude/config.yaml`。

也可使用 flag 或环境变量：

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password

export CLOUD_CLAUDE_GATEWAY=https://gw.example.com
export CLOUD_CLAUDE_SHORT_ID=abc123
export CLOUD_CLAUDE_PASSWORD=your-password
cloud-claude init
```

#### 日常使用

```bash
cd ~/your-project
alias claude=cloud-claude

cloud-claude
cloud-claude -p "帮我重构这个函数"
```

**会话管理：**

```bash
cloud-claude                  # 默认 attach 已有会话
cloud-claude --new-session    # 新建独立会话
cloud-claude --take-over      # 接管主会话，踢掉其他客户端
cloud-claude sessions         # 列出当前会话
```

**映射模式：**

```bash
cloud-claude --mount-mode=auto         # 默认：HotSync 优先，失败降级 SSHFS
cloud-claude --mount-mode=full         # HotSync + SSHFS 双轨
cloud-claude --mount-mode=sshfs-only   # 纯 SSHFS
```

**自检与排障：**

```bash
cloud-claude doctor                     # 五维度全面自检
cloud-claude doctor mount --fix         # 检查挂载并自动修复
cloud-claude explain MOUNT_SSHFS_DISCONNECTED  # 错误码解释
cloud-claude env check                  # 检查远端时区、出口 IP、FUSE 等
```

**常用配置（`~/.cloud-claude/config.yaml`）：**

- `proxy_commands` — 在本机执行的命令列表（默认 `["git"]`），设为 `[]` 关闭代理
- `hot_sync_max_file_mb` — 单文件熔断阈值（默认 50MB）
- `CLOUD_CLAUDE_NO_PROMOTION=1` — 禁用冷文件读触发晋升

### curl + SSH 接入

在终端执行管理员给你的命令：

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
```

输入密码后等待容器就绪，自动 SSH 进入云主机。

### 容器内预装工具

| 工具 | 说明 |
|------|------|
| **Claude Code** | 终端运行 `claude` |
| **KasmVNC + Chromium** | 通过管理后台访问浏览器桌面 |
| **Git / tmux / zsh** | 常用开发工具 |
| **Node.js** | JavaScript 运行时 |

### 使用 Claude Code

进入容器后直接运行：

```bash
claude
```

所有 Claude API 请求自动通过出口 IP 路由，无需额外配置代理。

### 断线重连

SSH 断开后重新执行同一条 `curl` 命令即可恢复，容器不会因断开而停止。

### 重建主机

在管理后台中点击"重建"按钮可重置环境，home 目录数据保留。

## 源码开发

参与二次开发时，按以下步骤在本机搭建开发环境。

### 1. 准备依赖

- Go 1.25.7+
- Node.js 20+（建议启用 `corepack`）
- pnpm 10+
- Docker Engine + Docker Compose v2
- GNU Make

### 2. 搭建环境

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

make setup    # 安装前端依赖，生成 .env
make db       # 启动 PostgreSQL
make dev      # 后端 + 前端热重载
```

启动后：

- Admin 前端：`http://localhost:2568`
- Control Plane API：`http://127.0.0.1:8080`

### 3. 验证

```bash
curl http://127.0.0.1:8080/healthz
make test
```

### 常用命令

```bash
make dev-api   # 仅启动后端
make dev-web   # 仅启动前端
make db-stop   # 停止 PostgreSQL
make db-reset  # 重建数据库
make help      # 查看全部命令
```

## 下一步

- [部署指南](./deployment) — systemd 原生部署
- [配置参考](./configuration) — 环境变量和网络配置
- [架构说明](./architecture) — 系统设计和项目结构
- [API 参考](../reference/api) — 完整的 Admin API 文档
- [故障排查](../reference/faq) — 常见问题和恢复
