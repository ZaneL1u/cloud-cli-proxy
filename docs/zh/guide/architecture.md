# 架构说明

## 系统架构

```
┌─────────────┐   curl    ┌────────────────┐    Docker     ┌──────────────────────┐
│   终端用户   │ ────────> │  Control Plane │ ───────────>  │  用户容器             │
│  (SSH 客户端)│ <──────── │  (Go API :8080)│              │  SSH + Claude + VNC   │
└─────────────┘   SSH     └───────┬────────┘              └────────┬─────────────┘
       │                          │                                │
       │                   ┌──────┴───────┐               ┌───────┴─────────┐
       │                   │  PostgreSQL   │               │  sing-box tun   │
       │                   │  (状态持久化) │               │  (全隧道出口)   │
       │                   └──────────────┘               └───────┬─────────┘
       │                          │                                │
       │                   ┌──────┴───────┐               ┌───────┴─────────┐
       │                   │  Admin SPA   │               │  指定出口 IP     │
       └──── SSH ────────> │  (:80/nginx) │               │  (受控路由)      │
           代理 :2222      └──────────────┘               └─────────────────┘
```

## 核心组件

### Control Plane（控制面）

Go 编写的 API 服务，是系统的中央协调器：

- **HTTP API** — 提供管理员和用户面板的 RESTful 接口
- **认证** — JWT Token 签发，支持管理员和普通用户两种角色
- **任务编排** — 主机创建、启动、停止、重建等操作通过异步任务队列执行
- **到期扫描** — 定期检查用户到期状态，自动停机和禁用
- **SSH 代理** — 监听 `:2222`，将 SSH 会话代理到目标容器内部
- **状态协调** — 对账运行中的容器与数据库记录，修正不一致状态

### Host Agent（宿主机代理）

执行需要特权的宿主机操作，与控制面通过 Unix socket 通信：

- **Docker 管理** — 创建、启动、停止和删除用户容器
- **网络配置** — 创建网络命名空间、配置 sing-box tun
- **防火墙管理** — 为每个容器设置 nftables 默认拒绝规则
- **网络校验** — 三重校验：连通性、出口 IP 匹配、DNS 泄漏检测

两种运行模式：
- `socket` — 独立进程，通过 Unix socket 接收控制面指令（生产推荐）
- `embedded` — 嵌入控制面进程内运行（开发和 Docker Compose 部署使用）

### 用户容器

基于 Ubuntu 24.04 的受管镜像，使用 `--network=none` 创建以彻底隔离默认网络：

- OpenSSH Server — SSH 接入
- Claude Code — AI 编程助手
- KasmVNC + Fluxbox + Chromium — 远程桌面
- sing-box — 代理模式隧道客户端
- 常用开发工具 — Git、tmux、zsh、Node.js 等

### cloud-claude CLI

用户在本机安装的 Go CLI，是本地终端与远端容器之间的透明桥梁：

- **本地透明代理** — `alias claude=cloud-claude`，在本地终端直接运行远端 Claude Code
- **三层文件映射** — Auto（HotSync 优先，失败降级 SSHFS）/ Full（双轨并行）/ SSHFS-Only（纯 SSHFS）
- **tmux 会话管理** — 多端 attach 同一会话，支持 `--new-session` 独占、`--take-over` 接管
- **网络韧性** — Reconnector 1/2/4/8/30s 退避重连，BufferedStdin 输入缓冲不丢失
- **自检排障** — `doctor` 五维度自检 + `--fix` 自动修复，`explain` 错误码查询
- **命令代理** — `proxy_commands` 将 `git` 等命令代理到本机执行

### PostgreSQL

持久化所有系统状态：

- 用户账号、密码（bcrypt）和到期时间
- 主机记录、短 ID、SSH 密码和状态
- 出口 IP 配置（代理配置 JSONB）
- 主机与出口 IP 绑定关系
- 异步任务记录
- 审计事件（13 种事件类型）

## 网络模型

### 容器网络隔离

每个用户容器使用 `--network=none` 创建，创建后没有任何网络接口（除 loopback），无法直连任何外部网络。

### sing-box tun 代理隧道

```
用户容器 namespace
├── lo (loopback)
├── veth (连接到宿主机的虚拟网卡)
├── tun0 (sing-box tun 设备)
│   └── 路由：0.0.0.0/0 → tun0
└── nftables：默认拒绝，仅允许到代理服务器的连接
```

sing-box 以 tun 模式运行，捕获所有出站流量并通过指定代理协议转发。

### 三重网络校验

每次主机启动后执行三重校验，任一失败则主机不可用：

1. **连通性测试** — 从容器 namespace 访问外部 HTTP 端点
2. **出口 IP 匹配** — 检查实际出口 IP 是否与预期一致
3. **DNS 泄漏检测** — 确保 DNS 请求也走隧道，不直连

## 安全边界

### 特权分离

```
┌─────────────────────────────────┐
│  Control Plane（低特权）         │
│  - HTTP API + 业务逻辑          │
│  - 不直接接触 Docker / 网络     │
│  - 通过 Unix socket 委托操作    │
└─────────────┬───────────────────┘
              │ Unix socket
┌─────────────┴───────────────────┐
│  Host Agent（高特权）            │
│  - Docker 容器管理              │
│  - 网络 namespace 操作          │
│  - nftables 防火墙配置          │
│  - sing-box 管理                │
└─────────────────────────────────┘
```

Web / API 层不直接持有过宽的宿主机特权。所有 Docker 和网络命名空间操作都集中在 host-agent 中执行，通过 Unix socket 接口暴露给控制面。

### 用户隔离

- 每个用户独立容器，`--network=none` 创建
- 容器间无网络互通
- 用户只能通过 SSH（直连或代理）访问自己的容器
- JWT Token 区分管理员和普通用户角色，用户只能看到自己的资源

### 凭证管理

- 用户密码使用 bcrypt 哈希存储
- 管理后台使用 JWT Token 认证，密钥可轮换
- 容器 SSH 密码独立于用户登录密码

## 数据流

### Bootstrap 接入流程

```
用户 → curl /v1/bootstrap/script → 获取引导脚本
     → 脚本提示输入用户名和密码
     → POST /v1/bootstrap/sessions → 认证 + 排队启动任务
     → 轮询 GET /v1/bootstrap/tasks/{id} → 等待任务完成
     → GET /v1/bootstrap/tasks/{id}/handoff → 获取 SSH 连接参数
     → exec ssh → 进入容器
```

### Entry 短链接接入流程

```
用户 → curl /entry/{shortId} → 获取入口脚本
     → 脚本提示输入密码
     → POST /v1/entry/{shortId}/auth → 认证
     → 返回 SSH 连接参数（host, port, user）
     → ssh -p 2222 → 通过 SSH 代理接入容器
```

### cloud-claude 本地 CLI 接入流程

```
用户 → cloud-claude init → 写入 ~/.cloud-claude/config.yaml（网关 / Short ID / 密码）
     → cd 项目目录 → cloud-claude
     → POST /v1/entry/{shortId}/auth → 认证 + 获取 SSH 参数
     → sshfs 将本地 CWD 挂到容器内同名路径（Auto/Full/SSHFS-Only 三层映射）
     → 检测 tmux 会话 → attach 已有或新建
     → 在远端启动 Claude Code
     → 终端大小、信号、退出码透传
     → 网络抖动 → Reconnector 自动重连（输入缓冲回放）
```

### 主机启动任务流

```
控制面创建任务 → host-agent 接收
  → 拉取/检查受管镜像
  → 创建容器（--network=none）
  → 创建网络命名空间
  → 配置 sing-box tun
  → 配置 nftables 防火墙规则
  → 启动容器
  → 三重网络校验
  → 标记任务成功
```

## 架构原则

- **单宿主机优先** — 不为 v1 提前引入多节点调度复杂度
- **网络强约束优先** — 先保证"所有流量都必须走指定出口"
- **启动体验优先** — 建立在真实可验证的运行时正确性之上
- **特权最小化** — API 层与特权操作严格分离

## 项目结构

```
cloud-cli-proxy/
├── cmd/
│   ├── cloud-claude/           # cloud-claude 本地 CLI 入口
│   ├── control-plane/          # 控制面 API 入口
│   └── host-agent/             # 宿主机代理入口
├── internal/
│   ├── controlplane/           # HTTP 路由、业务逻辑、到期扫描、状态协调
│   │   ├── http/               # 路由注册和中间件
│   │   ├── app/                # 应用生命周期和依赖组装
│   │   ├── admin/              # 管理员 API 处理器
│   │   ├── scheduler/          # 到期扫描和定时任务调度
│   │   └── credgen/            # 凭证生成（SSH 密码、JWT Secret）
│   ├── agent/                  # host-agent 服务端
│   ├── agentapi/               # host-agent API 客户端（控制面调用 agent）
│   ├── broadcast/              # SSE 实时事件广播（topic-based pub/sub）
│   ├── cloudclaude/            # cloud-claude CLI 内部库
│   │   ├── doctor/             # 五维度自检排障 + --fix 自动修复
│   │   └── errcodes/           # 错误码注册表（8 域 51 条）
│   ├── local/                  # cloud-claude local Dev Containers 工作流
│   ├── network/                # nftables / sing-box 网络配置
│   ├── runtime/                # 任务运行时、Docker 容器生命周期
│   │   └── tasks/              # 异步任务定义和执行器
│   ├── sshproxy/               # SSH 代理（转发到容器 22 端口）
│   └── store/                  # 数据库迁移和查询（pgx）
│       ├── migrations/         # SQL 迁移文件
│       ├── migrator/           # 迁移执行引擎
│       └── repository/         # 数据访问层
├── web/admin/                  # React 管理后台（TanStack Router + Query）
├── deploy/
│   ├── docker/                 # 4 个 Dockerfile
│   │   ├── control-plane/      # 控制面镜像
│   │   ├── admin/              # 管理后台镜像
│   │   ├── managed-user/       # 用户容器镜像
│   ├── compose/                # 开发用 Compose 文件
│   ├── bootstrap/              # 用户 curl 引导脚本
│   ├── scripts/                # setup-env.sh、deploy.sh、backup.sh
│   └── systemd/                # systemd 服务单元
├── docs/                       # VitePress 文档站
├── docker-compose.yml          # 生产 Compose
├── Makefile                    # 开发命令入口
└── .github/workflows/          # CI/CD
```

## 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go 1.26.1, net/http 标准库, pgx v5 |
| 前端 | React 19.2, TypeScript, Vite, Tailwind CSS, TanStack Router/Query |
| 数据库 | PostgreSQL 18 |
| 容器 | Docker Engine 28, Ubuntu 24.04 用户镜像 |
| 网络 | sing-box tun + Linux netns, nftables |
| 桌面 | KasmVNC 1.4.0 + Fluxbox + Chromium |
| CI/CD | GitHub Actions，多架构构建 |
