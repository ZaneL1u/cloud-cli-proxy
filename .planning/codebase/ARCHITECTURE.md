# Architecture

**Analysis Date:** 2026-06-01

## System Overview

Cloud CLI Proxy 是一个面向单宿主机的容器化 SSH 云主机平台。用户从一个很短的 `curl` 入口开始，在终端里输入用户名和密码，等待专属 Docker 容器启动完成后，直接进入该容器内的 SSH 会话。所有网络流量都必须通过指定出口 IP 的全局隧道路由发送。

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        cloud-claude CLI (终端用户)                           │
│                    `cmd/cloud-claude/` — Go + Cobra                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                              Entry Short-Link                                │
│                    `GET /entry/{username}` → bootstrap script               │
│                    `POST /v1/entry/{username}/auth` → SSH 凭证              │
├─────────────────────────────────────────────────────────────────────────────┤
│                         Control Plane (HTTP API)                             │
│              `internal/controlplane/` — Go net/http, JWT, SSE               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ Admin API   │  │ Bootstrap   │  │ Entry API   │  │ Scheduler           │ │
│  │ `/v1/admin` │  │ `/v1/boot`  │  │ `/entry`    │  │ expiry + reconcile  │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────────┤
│                         Host Agent (特权操作边界)                            │
│              `internal/agent/` + `internal/agentapi/`                        │
│              Unix socket 通信 (`/run/cloud-cli-proxy/host-agent.sock`)      │
├─────────────────────────────────────────────────────────────────────────────┤
│                         Docker + Network Layer                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ Runtime     │  │ Network     │  │ SSH Proxy   │  │ Broadcast           │ │
│  │ `internal/  │  │ `internal/  │  │ `internal/  │  │ `internal/          │ │
│  │  runtime/`   │  │  network/`   │  │  sshproxy/`  │  │  broadcast/`        │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────────┤
│                         Data Persistence                                     │
│              `internal/store/` — SQLite (modernc.org/sqlite, WAL 模式)       │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Admin Dashboard (Web)                                │
│              `web/admin/` — React 19.2 + Vite + TanStack Router             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Key File |
|-----------|----------------|----------|
| Control Plane | HTTP API 服务、认证、任务编排、SSE 推送、调度器 | `internal/controlplane/app/app.go` |
| Host Agent | Docker 特权操作、容器生命周期、网络配置 | `internal/agent/server.go` |
| Agent API Client | 通过 Unix socket 与 Host Agent 通信 | `internal/agentapi/client.go` |
| Runtime Service | 将 host action 转换为 Docker 命令队列 | `internal/runtime/runtime_service.go` |
| Worker | 实际执行 docker create/start/stop/rebuild | `internal/runtime/tasks/worker.go` |
| Network Provider | sing-box + nftables/iptables 全隧道出网 | `internal/network/container_proxy_provider.go` |
| SSH Proxy | 用户 SSH 接入代理，转发到容器内部 SSH | `internal/sshproxy/proxy.go` |
| Broadcast | SSE 实时事件广播中心 | `internal/broadcast/sse.go` |
| Store / Repository | SQLite 数据访问层 | `internal/store/repository/queries.go` |
| cloud-claude CLI | 终端用户入口，认证、挂载、SSH 会话 | `cmd/cloud-claude/main.go` |
| Admin Web | 管理后台前端 | `web/admin/src/main.tsx` |

## Pattern Overview

**Overall:** 分层架构 + 端口适配器模式（Ports and Adapters）

**Key Characteristics:**
- 控制面（Control Plane）不直接持有 Docker 特权，所有特权操作通过 Host Agent 或 Embedded Worker 执行
- 数据流以 Task（任务）为核心：每个 host action 创建一条 task 记录，异步执行，SSE 广播状态变更
- 网络隔离通过 per-host Docker network + sing-box gateway sidecar 实现
- 单宿主机优先，但架构预留了多宿主机扩展边界（host-agent socket 模型）

## Layers

### Web / API Layer
- **Purpose:** HTTP API 入口、认证、路由分发
- **Location:** `internal/controlplane/http/`
- **Contains:** Handler 函数、JWT 中间件、路由表
- **Depends on:** store/repository, runtime, broadcast, scheduler
- **Used by:** Admin Dashboard, cloud-claude CLI, curl bootstrap

### Application / Service Layer
- **Purpose:** 业务逻辑编排、任务调度、生命周期管理
- **Location:** `internal/controlplane/app/`, `internal/controlplane/scheduler/`, `internal/runtime/`
- **Contains:** App 组装器、ExpiryScanner、Reconciler、RuntimeService
- **Depends on:** repository, agentapi, network
- **Used by:** HTTP handlers

### Domain / Worker Layer
- **Purpose:** 实际执行 Docker 和网络操作
- **Location:** `internal/runtime/tasks/`, `internal/agent/`, `internal/network/`
- **Contains:** Worker, Host Agent Server, ContainerProxyProvider
- **Depends on:** Docker CLI, sing-box, nftables/iptables
- **Used by:** RuntimeService (embedded mode) or Host Agent (独立模式)

### Data Layer
- **Purpose:** 持久化存储
- **Location:** `internal/store/repository/`, `internal/store/migrations/`
- **Contains:** Repository (raw SQL), Migrator
- **Depends on:** SQLite + database/sql
- **Used by:** 所有上层服务

## Data Flow

### Primary: Bootstrap Entry Flow

1. 用户执行 `curl https://gw.example.com/entry/alice | bash`
   - Handler: `internal/controlplane/http/entry.go` → `Script()`
2. 脚本运行后调用 `POST /v1/entry/alice/auth`
   - Handler: `internal/controlplane/http/entry.go` → `Auth()`
3. 控制面检查用户/主机状态，如未创建则 queue `create_host` task
   - `internal/runtime/runtime_service.go` → `QueueHostAction()`
4. Task 通过 dispatcher 发往 Host Agent（或 embedded worker）
   - `internal/agent/server.go` 或 `internal/runtime/tasks/worker.go`
5. Worker 执行 docker create/start，配置 sing-box 网络
   - `internal/runtime/tasks/worker.go` → `createHost()`
   - `internal/network/container_proxy_provider.go` → `PrepareHost()`
6. 容器就绪后，auth 返回 SSH 四元组（host/port/user/pass）
7. cloud-claude CLI 建立 SSH 连接并启动 claude code
   - `cmd/cloud-claude/main.go` → `ConnectAndRunClaudeV3()`

### Secondary: Admin Dashboard CRUD Flow

1. 管理员登录 → `POST /v1/auth/login` → JWT token
   - `internal/controlplane/http/auth.go`
2. 前端 API 调用带 `Authorization: Bearer <token>`
   - `internal/controlplane/http/admin_*.go`
3. 写操作创建 task 或更新 DB，SSE 广播变更
   - `internal/broadcast/sse.go` → `Broadcast()`
4. 前端 `use-sse.ts` 接收事件，触发 React Query 刷新

### Tertiary: Expiry & Reconcile Flow

1. `ExpiryScanner` 每 60s 扫描过期用户
   - `internal/controlplane/scheduler/expiry.go`
2. 过期用户状态改为 `expired`，自动停止其运行中主机
3. `Reconciler` 每 60s 对比 DB 状态与 Docker 实际状态
   - `internal/controlplane/scheduler/reconciler.go`
4. 发现漂移（DB=running 但容器不存在）则自动恢复或标记 stopped

## Key Abstractions

### Task-Driven Async Execution
- **Purpose:** 所有耗时的 host action（create/start/stop/rebuild）都异步执行
- **Pattern:** 创建 Task 记录 → dispatch 到 worker → worker 更新 task 状态 → SSE 广播
- **Files:** `internal/store/repository/models.go` (Task 模型), `internal/runtime/tasks/worker.go`

### Host Action Contract
- **Purpose:** 控制面与 host-agent 之间的操作契约
- **Pattern:** `agentapi.HostActionRequest` / `HostActionResponse` JSON over HTTP (Unix socket)
- **Files:** `internal/agentapi/contracts.go`

### Network Provider Interface
- **Purpose:** 抽象网络配置，支持 Linux 实现和测试桩
- **Pattern:** `network.Provider` interface: `PrepareHost` / `CleanupHost`
- **Files:** `internal/network/provider.go`, `internal/network/container_proxy_provider.go`

### Repository Pattern
- **Purpose:** 数据访问抽象，直接 SQL 无 ORM
- **Pattern:** `repository.Repository` 封装 `*sql.DB`，每个表一组 Query/Scan 方法
- **Files:** `internal/store/repository/queries.go`

## Entry Points

**Control Plane:**
- Location: `cmd/control-plane/main.go`
- Triggers: systemd service 或 `go run`
- Responsibilities: 启动 HTTP server、运行 migrations、启动 scheduler、启动 SSH proxy

**Host Agent:**
- Location: `cmd/host-agent/main.go`
- Triggers: systemd service（生产环境）或 `go run`
- Responsibilities: 监听 Unix socket，执行 Docker 和网络操作

**cloud-claude CLI:**
- Location: `cmd/cloud-claude/main.go`
- Triggers: 终端用户直接执行
- Responsibilities: 认证、等待容器就绪、SSH 连接、目录挂载、启动 claude code

**Admin Dashboard:**
- Location: `web/admin/src/main.tsx`
- Triggers: 浏览器访问
- Responsibilities: 管理用户、主机、出口 IP、查看事件和任务

## Architectural Constraints

- **Threading:** Go 协程模型。HTTP handlers 在 goroutine 中运行；SSE 每个连接一个 goroutine；scheduler 每个 job 一个 goroutine
- **Global state:** `broadcast.defaultHub` 是包级单例（SSE 连接管理）；`errcodes.registry` 是包级单例（错误码注册表）
- **Circular imports:** 未发现明显循环依赖。`internal/agentapi` 作为共享契约包，被 `controlplane` 和 `agent` 共同依赖
- **特权分离:** Web/API 层不直接执行 Docker 命令。特权操作集中在 `internal/agent/` 或 `internal/runtime/tasks/`
- **网络隔离:** 每个 host 拥有独立的 Docker network + sing-box gateway sidecar，bridge 网络断开以防止 IP 泄漏

## Anti-Patterns

### Direct Docker Exec from Control Plane

**What happens:** 在 embedded 模式下，control-plane 直接实例化 worker 执行 docker 命令
**Why it's wrong:** 模糊了特权边界，未来多宿主机扩展时需要重构
**Do this instead:** 始终通过 `agentapi.Dispatcher` 接口 dispatch，embedded 模式使用 `EmbeddedDispatcher` 适配器

### HTTP Status Code as Business Logic

**What happens:** 部分 handler 使用 HTTP 200 返回内部错误，或在 success body 中嵌入 error 字段
**Why it's wrong:** 客户端难以统一处理错误
**Do this instead:** 使用正确的 HTTP status code（4xx/5xx），body 统一为 `{error: string}` 或业务数据结构

## Error Handling

**Strategy:** 分层错误处理 + 统一错误码注册表

**Patterns:**
- 基础设施层（docker exec, database/sql）: 包装为 `fmt.Errorf("...: %w", err)`
- 业务层: 使用 `internal/cloudclaude/errcodes` 注册表定义结构化错误码
- HTTP 层: 统一 `writeJSON` 输出，status code 反映错误类别

## Cross-Cutting Concerns

**Logging:** `log/slog` 结构化日志，支持 `LOG_LEVEL` 和 `LOG_FORMAT=json` 环境变量
**Validation:** 输入校验分散在各 handler 中，无统一校验中间件
**Authentication:** JWT (HS256) for admin/user API；entry auth 使用用户名+密码
**Authorization:** Role-based (`admin` / `user`)，中间件 `RequireRole`

---

*Architecture analysis: 2026-06-01*
