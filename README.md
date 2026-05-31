<div align="center">

<img src="docs/public/logo.svg" width="88" height="88" alt="Cloud CLI Proxy" />

# Cloud CLI Proxy

在单台宿主机上，为每个用户提供一个独立的 Docker 容器作为云开发环境。容器预装 Claude Code 和常用工具，所有出网流量通过 sing-box 全隧道强制走你指定的出口 IP。

[![CI](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/ci.yml)
[![Images](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml/badge.svg)](https://github.com/ZaneL1u/cloud-cli-proxy/actions/workflows/build-images.yml)
[![Release](https://img.shields.io/github/v/release/ZaneL1u/cloud-cli-proxy)](https://github.com/ZaneL1u/cloud-cli-proxy/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.en.md) | [文档](https://zanel1u.github.io/cloud-cli-proxy/)

</div>

---

## 这是什么？

管理员在后台创建用户、分配容器、绑定出口 IP。用户拿到 `curl` 命令，在终端里跑一下，输入密码，等容器启动后直接 SSH 进去。每个容器是独立的 Ubuntu 24.04 环境，里面已经装好 Claude Code、OpenSSH、KasmVNC 远程桌面。容器的所有出网流量走 sing-box tun 隧道，从指定的出口 IP 出去，DNS 和 WebRTC 也不会漏。

做这个项目主要解决几个实际问题：

- 团队共用 Claude Code，API 请求统一走公司出口 IP，不需要每人自己配代理
- 出海场景需要特定地区的出口 IP，容器绑上去就行
- 外包或临时人员需要隔离的开发机，用完可以回收，操作有审计记录
- 合规要求所有研发流量必须从指定 IP 出去，不能有旁路

---

## 核心能力

### 网络与安全

- **全流量强制隧道** — sing-box tun + Linux netns 接管容器所有出网流量，nftables 默认拒绝直连，不允许 DNS / WebRTC 泄漏
- **6 种代理协议** — 出口 IP 支持 SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP
- **bypass 防火墙** — 按域名、CIDR、端口配置白名单直连，预设规则集（loopback 强制启用、LAN 可选），快照版本化管理，预览→应用→回滚，完整审计日志
- **出口 IP 自动修正** — 定时探测容器实际出口 IP，与配置不一致时自动更新数据库记录
- **容器安全加固** — 仅授予 NET_ADMIN，移除 NET_RAW，IPv6 内核级禁用，PID 限制，日志轮转

### 环境伪装

让容器内的 Claude Code 看起来像在真实物理机上运行，而非云环境。默认伪装成 macOS 或 Windows 桌面环境：

- **系统指纹** — 替换 Node.js 读取到的 CPU 型号（伪装为 AMD EPYC）、MAC 地址、`/etc/machine-id` 等硬件标识；拦截 `ioreg`、`system_profiler`、`sysctl` 等探测命令的输出
- **主机名伪装** — 自动生成 `DESKTOP-XXXXXXX` 或 `LAPTOP-XXXXXXX` 风格的主机名
- **容器检测绕过** — 隐藏 `/.dockerenv`、过滤 cgroup 中的 docker/containerd 字符串，避免被识别为容器环境
- **时区与语言环境** — 创建容器时可指定时区和 locale，默认 `America/Los_Angeles` / `en_US.UTF-8`
- **TLS 指纹** — sing-box 出口连接自动启用 uTLS，指纹设为 Chrome，握手特征与普通浏览器一致
- **遥测屏蔽** — 容器内预置 DNS 级别拦截，阻止 Claude Code 向 `statsig.anthropic.com`、`sentry.io`、`cdn.growthbook.io` 等遥测端点上报数据

### 容器与运行环境

- **独立 Docker 容器** — 每用户独立 Ubuntu 24.04 容器，CPU / 内存 / 磁盘均可限制
- **Claude Code 预装** — 进入即用，API 请求自动走出口 IP，无需额外配置
- **KasmVNC 远程桌面** — 内置 Chromium 浏览器，管理后台一键打开容器桌面；支持通过 VNC 代理在浏览器中直接操作
- **持久化存储** — Claude 状态经命名卷持久化，容器重建不丢失工作数据
- **宿主机目录挂载** — 支持将宿主机路径绑定挂载到容器内

### 接入体验

- **一条命令接入** — 用户只需 `curl | bash`，自动完成认证、容器创建和 SSH 接入
- **cloud-claude 本地 CLI** — 在本地终端透明运行远端 Claude Code，当前目录经 sshfs 挂载到容器内同名路径；支持 Auto / Full / SSHFS-Only 三层映射模式，单文件超过阈值自动走冷路径
- **tmux 多端会话** — 多客户端 attach 同一 tmux 会话，断线工作区不丢失；支持 `--new-session` 独占、`--take-over` 接管
- **断线自动恢复** — 内置 Reconnector，30s 内自动重连，输入缓冲不丢
- **doctor 五维度自检** — `cloud-claude doctor [network|auth|ssh|mount|disk]`，带 `--fix` 自动修复
- **错误码自解释** — `cloud-claude explain <CODE>` 查看详细说明和修复建议

### 管理后台与治理

- **仪表盘** — 活跃用户、运行主机、可用出口 IP、最近事件总览
- **用户生命周期** — 创建、暂停、过期自动停机、禁止登录、密码轮换
- **主机生命周期** — 创建、启动、停止、重建（保留或清空 /workspace）、删除
- **出口 IP 管理** — 增删改查、连通性测试（流式输出）、绑定到主机
- **事件审计** — 所有操作写入 events 表，可追溯
- **SSE 实时推送** — 任务进度、主机状态、事件通过 Server-Sent Events 实时更新
- **用户自助门户** — 用户可查看自己的主机、重建、重启 VNC、管理 SSH 密钥

---

## 快速开始

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh

docker compose pull
docker compose up -d

curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

启动后：

- 管理后台：`http://YOUR_HOST:3000`
- API：`http://YOUR_HOST:8080`
- SSH 代理：`YOUR_HOST:2222`

首次使用：登录管理后台 → 添加出口 IP → 创建用户 → 创建主机 → 将接入命令分发给用户。

---

## 安装与部署

### 环境要求

- Docker Engine 28.x+
- Docker Compose v2
- PostgreSQL 18.x（也可用内置 Docker PostgreSQL）

### Docker Compose（推荐）

```bash
bash deploy/scripts/setup-env.sh  # 交互式生成密码和密钥
docker compose pull               # 拉取预构建镜像
docker compose up -d              # 启动
```

`setup-env.sh` 自动生成 JWT 密钥、管理员密码，支持内置 Docker PostgreSQL 或外部数据库。

本地源码构建（预构建镜像不可用时的兜底）：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### 宿主机直接部署

```bash
sudo bash deploy/scripts/deploy.sh
```

创建 `cloudproxy` 系统用户，构建二进制和镜像，安装 systemd 单元并启动。

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DATABASE_URL` | PostgreSQL 连接字符串 | 必填 |
| `ADMIN_USERNAME` | 管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 管理员密码（bcrypt） | 必填 |
| `ADMIN_JWT_SECRET` | JWT 签名密钥 | 必填 |
| `ADMIN_PORT` | 管理后台端口 | `3000` |
| `SSH_PROXY_PORT` | SSH 代理端口 | `2222` |
| `LOG_FORMAT` | 日志格式 `json` / `text` | `json` |
| `LOG_LEVEL` | 日志级别 | `info` |

---

## 使用指南

### 管理员操作

1. **添加出口 IP** — 支持 6 种代理协议，可一键测试连通性
2. **创建用户** — 设置用户名、密码、到期时间
3. **创建主机** — 为用户创建容器，绑定出口 IP
4. **分发接入信息** — 复制主机详情页的 `curl` 命令给用户

### 用户接入（curl 方式）

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
# 输入密码 → 等待容器就绪 → 自动 SSH 进入云主机
```

进入后直接使用 Claude Code：

```bash
claude
```

### cloud-claude CLI（推荐）

安装 CLI 后在本地终端透明使用远端 Claude Code，当前目录自动挂载到容器内同名路径。

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

也可从 [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases) 下载或源码构建：

```bash
go build -ldflags "-s -w" -trimpath -o cloud-claude ./cmd/cloud-claude
```

#### 初始化

管理员提供三样信息：**网关地址**、**主机 Short ID**、**密码**。

```bash
cloud-claude init
# 交互式输入 → 写入 ~/.cloud-claude/config.yaml
```

或使用 flag / 环境变量：

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password
```

#### 日常使用

```bash
cd ~/your-project
alias claude=cloud-claude

cloud-claude            # 默认 attach 已有 tmux 会话
cloud-claude --new-session    # 新建独立会话
cloud-claude --take-over      # 接管主会话，踢掉其他客户端
cloud-claude sessions         # 列出当前会话
```

**自检与排障：**

```bash
cloud-claude doctor                     # 五维度全面自检
cloud-claude doctor mount --fix         # 检查挂载并自动修复
cloud-claude explain MOUNT_SSHFS_DISCONNECTED  # 错误码解释
cloud-claude env check                  # 检查远端时区、出口 IP、FUSE 等
```

**配置参考：**

- `proxy_commands` — 在本机执行的命令列表（默认仅 `git`），设空数组关闭代理
- `hot_sync_max_file_mb` — 单文件熔断阈值（默认 50MB）
- `CLOUD_CLAUDE_NO_PROMOTION=1` — 禁用冷文件读触发晋升

### KasmVNC 远程桌面

管理后台可直接打开容器的浏览器桌面环境，无需本地安装 GUI。

---

## 架构

```
                                                    ┌───────────────────────────────────┐
用户 ──curl──> Control Plane (:8080) ──Docker──>    │ 用户容器                          │
                    │                                │  SSH + Claude Code + VNC          │
               PostgreSQL                            │  sshfs ← 本地 CWD 同名路径映射    │
                    │                                │  sing-box tun 隧道                │
              Admin SPA (:3000)                      │       ↓                           │
                    │                                │  指定出口 IP                      │
              SSH Proxy (:2222)                      └───────────────────────────────────┘
                    ↑                                           ↑
                    │                                           │
用户 ──cloud-claude──> 认证 + SSH + sshfs ──────────────────────┘
```

| 组件 | 说明 |
|------|------|
| **Control Plane** | Go API，认证、用户管理、任务编排、SSH 代理 |
| **Host Agent** | 特权代理，管理 Docker 容器、网络命名空间和隧道 |
| **用户容器** | Ubuntu 24.04，预装 OpenSSH + Claude Code + sshfs + KasmVNC + Chromium |
| **cloud-claude** | Go CLI，透明替代本地 claude；sshfs 同名路径映射；Auto/Full/SSHFS-Only 三层映射、tmux 多端会话、断线自动重连、doctor 五维度自检与错误码解释 |
| **PostgreSQL** | 持久化用户、主机、出口 IP、任务、事件、审计日志 |
| **Admin SPA** | React 19 + TypeScript + Vite + Tailwind CSS |

---

## 参与贡献

Bug 报告和功能建议请提交 [Issue](https://github.com/ZaneL1u/cloud-cli-proxy/issues)。

Pull Request 流程：

1. Fork 仓库，从 `main` 分支创建 feature 分支
2. 修改代码，确保 `make test` 通过
3. 提交 PR，描述改了什么、为什么改

本地开发环境搭建：

```bash
make setup    # 安装依赖
make db       # 启动 PostgreSQL
make dev      # 后端 + 前端热重载（API :8090，前端 localhost:2568）
make test     # 运行测试
```

更多命令见 `make help`。

---

## 文档

完整文档见 [GitHub Pages](https://zanel1u.github.io/cloud-cli-proxy/)：快速开始、部署指南、配置参考、架构说明、API 参考、故障排查。

---

## 许可证

[MIT](LICENSE)
