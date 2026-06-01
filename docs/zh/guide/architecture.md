# 架构说明

## 概览

Cloud CLI Proxy 由四个核心组件组成：Control Plane（控制面）、Host Agent（宿主机代理）、用户容器和 cloud-claude CLI。控制面负责 API、认证和任务编排，宿主机代理执行 Docker 和网络操作，两者通过 Unix socket 通信。

```
用户 ──curl──> Control Plane (:8080) ──Docker──> │ 用户容器                 │
                    │                            │  SSH + Claude + VNC      │
               PostgreSQL                        │  sing-box tun 隧道       │
                    │                            │       ↓                  │
              Admin UI (embed)                  │  指定出口 IP              │
                    │                            └──────────────────────────┘
              SSH Proxy (:2222)
```

## 核心组件

### Control Plane

Go 编写的 API 服务，是系统的中央调度器：

- **HTTP API** — 管理员和用户面板的 RESTful 接口
- **认证** — JWT Token 签发，区分管理员和用户角色
- **任务编排** — 主机创建、启动、停止、重建等操作通过异步任务队列执行
- **到期扫描** — 定时检查用户到期状态，自动停机和禁用
- **SSH 代理** — 监听 `:2222`，将 SSH 会话代理到目标容器
- **状态协调** — 对账运行中的容器与数据库记录，修正不一致

### Host Agent

执行需要特权的宿主机操作，与控制面通过 Unix socket 通信：

- **Docker 管理** — 创建、启动、停止、删除用户容器
- **网络配置** — 创建 netns、配置 sing-box tun
- **防火墙管理** — 为每个容器设置 nftables 默认拒绝规则
- **网络校验** — 连通性、出口 IP 匹配、DNS 泄漏三重检测

两种运行模式：`socket`（独立进程，生产推荐）和 `embedded`（嵌入控制面，开发使用）。

### 用户容器

基于 Ubuntu 24.04 的受管镜像，以 `--network=none` 创建，彻底隔离默认网络。预装 OpenSSH、Claude Code、KasmVNC + Chromium、sing-box 以及 Git、tmux、zsh、Node.js 等开发工具。

### cloud-claude CLI

用户在本地安装的 Go CLI，作为本地终端与远端容器之间的桥梁：

- 本地目录通过 sshfs 挂载到容器内同名路径
- 支持 Auto / Full / SSHFS-Only 三层映射模式
- tmux 多端会话、断线自动重连
- `doctor` 五维度自检 + `--fix` 自动修复
- `explain` 错误码查询
- `proxy_commands` 将 git 等命令代理到本机执行

### PostgreSQL

持久化所有系统状态：用户、主机、出口 IP 配置、绑定关系、异步任务、审计事件。

## 网络模型

### 容器网络隔离

每个容器以 `--network=none` 创建，创建后只有 loopback 接口，无法直连外部网络。

### sing-box tun 隧道

```
用户容器 namespace
├── lo (loopback)
├── tun0 (sing-box tun 设备)
│   └── 路由：0.0.0.0/0 → tun0
└── nftables：默认拒绝，仅允许代理服务器连接
```

sing-box 以 tun 模式运行，捕获所有出站流量并通过指定代理协议转发。

### 三重网络校验

主机每次启动后执行：

1. **连通性** — 从容器 netns 访问外部 HTTP 端点
2. **出口 IP 匹配** — 实际出口 IP 是否与配置一致
3. **DNS 泄漏** — DNS 请求是否走隧道

任一失败则主机不可用。

## 安全边界

### 特权分离

控制面不直接接触 Docker 或网络，所有特权操作集中在 Host Agent 中，通过 Unix socket 暴露给控制面。

### 用户隔离

- 每用户独立容器，`--network=none` 创建
- 容器间无网络互通
- JWT Token 区分角色，用户只能访问自己的资源

### 凭证管理

- 用户密码 bcrypt 哈希存储
- JWT Token 认证，密钥可轮换
- 容器 SSH 密码独立于用户登录密码

## 数据流

### 用户接入（curl 方式）

```
用户 → curl /entry/{shortId} → 获取入口脚本
     → 输入密码
     → POST /v1/entry/{shortId}/auth → 认证
     → 返回 SSH 参数 → SSH 代理接入容器
```

### cloud-claude CLI 方式

```
用户 → cloud-claude init → 写入配置
     → cd 项目目录 → cloud-claude
     → 认证 + 容器就绪等待
     → sshfs 挂载本地目录到容器同名路径
     → attach 或新建 tmux 会话
     → 远端启动 Claude Code
```

### 主机启动任务流

```
控制面创建任务 → Host Agent 接收
  → 拉取受管镜像
  → 创建容器（--network=none）
  → 配置 netns + sing-box tun
  → 配置 nftables
  → 启动容器
  → 三重网络校验
  → 标记任务成功
```

## 架构原则

- **单宿主机优先** — v1 不引入多节点调度复杂度
- **网络强约束** — 所有流量走指定出口，不允许旁路
- **特权最小化** — API 层与特权操作严格分离

## 项目结构

```
cloud-cli-proxy/
├── cmd/
│   ├── cloud-claude/           # cloud-claude CLI
│   ├── control-plane/          # 控制面 API
│   └── host-agent/             # 宿主机代理
├── internal/
│   ├── controlplane/           # HTTP 路由、业务逻辑、调度
│   │   ├── http/               # 路由和中间件
│   │   ├── app/                # 应用生命周期
│   │   ├── scheduler/          # 到期扫描和定时任务
│   │   └── credgen/            # 凭证生成
│   ├── agent/                  # Host Agent 服务端
│   ├── agentapi/               # Agent API 客户端
│   ├── broadcast/              # SSE 实时广播
│   ├── cloudclaude/            # cloud-claude CLI 库
│   ├── local/                  # 本地 Dev Containers
│   ├── network/                # nftables / sing-box 配置
│   ├── runtime/                # 任务运行时、容器生命周期
│   ├── sshproxy/               # SSH 代理
│   └── store/                  # 数据库迁移和查询（pgx）
├── web/admin/                  # React 管理后台
├── deploy/                     # Dockerfile、Compose、脚本
├── docs/                       # VitePress 文档
├── docker-compose.yml          # 生产 Compose
└── Makefile
```

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.26, net/http, pgx v5 |
| 前端 | React 19, TypeScript, Vite, Tailwind CSS |
| 数据库 | PostgreSQL 18 |
| 容器 | Docker Engine 28, Ubuntu 24.04 |
| 网络 | sing-box tun + Linux netns, nftables |
| 桌面 | KasmVNC + Chromium |
