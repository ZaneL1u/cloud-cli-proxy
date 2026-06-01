# Components

**Analysis Date:** 2026-06-01

## cmd/ — Entry Points

### `cmd/control-plane/main.go`
- 控制面 HTTP API 服务入口
- 从环境变量读取配置（DATABASE_URL, ADMIN_JWT_SECRET, CONTROL_PLANE_ADDR 等）
- 组装 `app.App` 并运行
- 支持 graceful shutdown（SIGINT/SIGTERM）

### `cmd/host-agent/main.go`
- 宿主机代理服务入口
- 连接 SQLite（用于 worker 的 repository 操作）
- 创建 Unix socket 监听（默认 `/run/cloud-cli-proxy/host-agent.sock`）
- 在 Linux 上使用默认 socket，非 Linux 使用 `~/.cloud-cli-proxy/host-agent.sock`

### `cmd/cloud-claude/main.go`
- 终端用户 CLI 入口
- 使用 Cobra 框架
- 子命令: `init`, `env check`, `ssh doctor`, `sync`, `sessions`, `explain`, `doctor`, `local`
- 根命令: 认证 → 等待容器就绪 → SSH 连接 → 启动 claude code
- 支持 `--mount-mode`, `--new-session`, `--take-over` 等 flag

---

## internal/controlplane/ — HTTP API 与业务编排

### `internal/controlplane/app/app.go`
- **职责:** 控制面的依赖注入和生命周期管理
- **关键行为:**
  - 创建 `*sql.DB` 连接（WAL 模式，SetMaxOpenConns(1)）
  - 运行数据库 migrations
  - 根据 `HOST_AGENT_MODE` 环境变量选择 embedded 或独立 host-agent 模式
  - 组装 Router、SSHProxy、Scheduler（expiry + reconcile + image-cache-refresh）
  - 控制面若运行在 Docker 容器内，自动重新加入 host networks

### `internal/controlplane/http/router.go`
- **职责:** 路由定义和 handler 组装
- **关键端点:**
  - `GET /healthz` — 健康检查（database + agent）
  - `GET /v1/users`, `GET /v1/hosts` — 公开列表
  - `POST /v1/bootstrap/sessions` — bootstrap 认证
  - `GET /entry/{username}` — entry short-link 脚本
  - `POST /v1/entry/{username}/auth` — entry 认证
  - `GET /v1/admin/sse`, `GET /v1/user/sse` — SSE 实时推送
  - `/v1/admin/*` — 管理员 CRUD（JWT + admin role）
  - `/v1/user/*` — 用户自助服务（JWT + user/admin role）

### `internal/controlplane/http/admin_*.go`
- 各资源的管理 API handler
- `admin_users.go` — 用户 CRUD、密码重置、过期时间
- `admin_hosts.go` — 主机生命周期、VNC 代理、Claude 设置
- `admin_egress_ips.go` — 出口 IP CRUD、代理测试
- `admin_bindings.go` — 主机与出口 IP 绑定
- `admin_events.go` — 审计事件列表
- `admin_claude_accounts.go` — Claude 账号管理

### `internal/controlplane/http/auth.go`
- JWT 认证中间件 `AuthMiddleware`
- Role 检查中间件 `RequireRole`
- 统一登录 handler `NewUnifiedLoginHandler`

### `internal/controlplane/scheduler/scheduler.go`
- 通用定时任务调度器
- 每个 job 一个 goroutine + ticker
- 当前 jobs: expiry-scan, reconcile, image-cache-refresh

### `internal/controlplane/scheduler/expiry.go`
- **职责:** 扫描过期用户并自动停用
- **流程:**
  1. `ListExpiredActiveUsers` 查询 `expires_at < NOW()` 的用户
  2. 更新用户状态为 `expired`
  3. 停止该用户所有运行中主机（queue `ActionStopHost`）
  4. 记录 audit event

### `internal/controlplane/scheduler/reconciler.go`
- **职责:** DB 状态与 Docker 实际状态对账
- **流程:**
  1. `ListRunningHosts` 获取 DB 中状态为 running 的主机
  2. `InspectContainer` 检查容器实际状态
  3. 若容器不存在/未运行，自动 queue `ActionStartHost` 恢复，或标记为 stopped
  4. `MarkStaleTasks` 将长时间 pending/running 的任务标记为失败

---

## internal/agent/ + internal/agentapi/ — Host Agent

### `internal/agent/server.go`
- **职责:** Host Agent HTTP server，监听 Unix socket
- **端点:**
  - `GET /healthz` — 健康检查
  - `GET /v1/containers/{name}/status` — 容器状态查询（docker inspect）
  - `POST /v1/host-actions` — 执行 host action（委托给 worker）
- **特点:** handler 内建 panic recovery，panic 时更新 task 状态为 failed 并广播 SSE

### `internal/agentapi/client.go`
- **职责:** Host Agent 的 HTTP 客户端
- **通信方式:** Unix domain socket HTTP
- **方法:** `Ping`, `InspectContainer`, `RunHostAction`
- **超时:** 30 秒

### `internal/agentapi/contracts.go`
- **职责:** 控制面与 host-agent 之间的共享契约
- **核心类型:**
  - `HostAction` 枚举: `create_host`, `start_host`, `stop_host`, `rebuild_host`, `prepare_host`, `volume_remove`
  - `HostActionRequest` — 操作请求（含 task_id, host_id, image, mounts, volumes, ssh keys 等）
  - `HostActionResponse` — 操作响应
  - `TaskStatusUpdate` — 任务状态更新
  - `ContainerStatusResponse` — 容器状态

---

## internal/broadcast/ — SSE 实时事件广播

### `internal/broadcast/sse.go`
- **职责:** Server-Sent Events 广播中心
- **特点:**
  - 包级单例 `defaultHub`
  - 支持多 topic 订阅（`?topics=hosts,tasks,events`）
  - 连接上限: 500（可配置 `SSE_MAX_CONNECTIONS`）
  - 每 IP 上限: 10（可配置 `SSE_MAX_CONNECTIONS_PER_IP`）
  - 心跳: 30 秒
  - 超时清理: 30 分钟
  - 连续 5 次连接失败后前端进入降级模式（fallback to polling）
- **便捷函数:** `Subscribe`, `Broadcast`, `BroadcastJSON`, `Stats`

---

## internal/cloudclaude/ — CLI 内部库

### `internal/cloudclaude/entry.go`
- **职责:** Entry API 客户端
- **核心类型:** `EntryClient`, `AuthResponse`
- **方法:** `CheckGateway`, `Authenticate`, `AuthenticateAndWait`
- **轮询:** 默认 3 秒间隔，120 秒超时

### `internal/cloudclaude/config.go`
- **职责:** CLI 配置管理（`~/.cloud-claude/config.yaml`）
- **配置项:** gateway, username, password, proxy_commands, hot_sync_max_file_mb

### `internal/cloudclaude/mount*.go`
- **职责:** 本地工作目录挂载到远程容器
- **模式:** auto / full / hot-only / sshfs-only
- **技术:** SSHFS + mergerfs（可选）+ hot sync（文件变更实时同步）

### `internal/cloudclaude/doctor/` — 诊断工具
- `doctor.go` — 主诊断流程编排
- `check.go` — 各检查项定义
- `auth.go` — 认证相关检查
- `disk.go` — 磁盘空间检查
- `mount.go` — 挂载相关检查
- `network.go` — 网络/出口 IP 检查
- `ssh.go` — SSH 配置检查
- `remote_ssh.go` — 远程 SSH 环境检查
- `fix.go` — 自动修复逻辑
- `render.go` — 诊断结果渲染输出

### `internal/cloudclaude/errcodes/` — 统一错误码注册表
- `codes.go` — 注册表核心（`MustRegister`, `Lookup`, `Format`）
- 命名规范: `^[A-Z]+_[A-Z]+_[A-Z0-9]+$`（DOMAIN_KIND_NAME）
- 各域文件: `auth.go`, `disk.go`, `mount.go`, `net.go`, `remote_ssh.go`, `session.go`, `ssh.go`, `state.go`, `system.go`
- 使用方式: `errcodes.Format(errcodes.MOUNT_HOT_SYNC_FAILED, args...)`

---

## internal/local/ — Dev Containers 本地工作流

### `internal/local/local.go`
- **职责:** `cloud-claude local` 子命令的后端实现
- **功能:** 在本地 Docker 启动一个开发容器（不经过控制面）
- **容器命名:** `cloud-claude-local-{md5(projectDir)[:8]}`
- **默认镜像:** `ghcr.io/zanel1u/cloud-cli-proxy/managed-user:latest`
- **方法:** `Up`（启动）, `Down`（停止删除）, `Status`（状态查询）

### `internal/local/egress.go`
- 本地容器的出口代理配置验证

### `internal/local/container.go`
- Docker 容器操作封装

### `internal/local/password.go`
- 随机密码生成

---

## internal/network/ — nftables / sing-box 网络层

### `internal/network/provider.go`
- **职责:** 网络 Provider 接口定义
- **接口:** `Provider` — `PrepareHost(ctx, HostNetworkSpec) error`, `CleanupHost(ctx, HostNetworkSpec) error`

### `internal/network/container_proxy_provider.go`
- **职责:** Linux 生产环境网络实现
- **架构:** 每个 host 一个独立的 Docker network + sing-box gateway sidecar
- **IP 分配:** 基于 host_id hash 的 /24 子网（`10.99.{third}.0/24`）
  - `.1` — bridge gateway
  - `.2` — sing-box gateway
  - `.3` — worker 容器
- **流量路径:** worker → bridge → gateway → sing-box tun → 出口代理
- **端口映射:** Linux 通过宿主机 iptables DNAT 转发（不依赖 Docker -p）

### `internal/network/singbox_config.go`
- sing-box 配置文件生成

### `internal/network/firewall_proxy.go` / `firewall_helpers.go`
- nftables/iptables 规则管理

### `internal/network/validate.go`
- 出口 IP 绑定验证逻辑

---

## internal/runtime/ — Docker 容器生命周期

### `internal/runtime/runtime_service.go`
- **职责:** 将 host action 转换为可执行的 task
- **核心方法:** `QueueHostAction(ctx, hostID, action, requestedBy)`
- **流程:**
  1. 加载 host 和 owner user
  2. 加载 `image.lock` runtime spec
  3. 解析 claude_account_id（Phase 33 自动补 volume）
  4. 创建 task 记录
  5. 组装 `HostActionRequest`
  6. 异步 dispatch 到 worker
  7. SSE 广播 task 状态变更

### `internal/runtime/image_cache.go`
- Docker 镜像状态缓存，供前端展示

### `internal/runtime/tasks/worker.go`
- **职责:** 实际执行 Docker 命令
- **Actions:**
  - `create_host` — docker create + start + 网络配置 + SSH 就绪检查
  - `start_host` — docker start + 网络配置 + SSH 就绪检查
  - `stop_host` — docker stop + 网络清理
  - `rebuild_host` — stop + rm + create_host
  - `prepare_host` — 验证出口绑定
  - `volume_remove` — 删除指定 Docker volume
- **SSH 就绪检查:** `waitForSSH` → `WaitForSSHReady` → 同步容器密码 + 注入 SSH keys
- **Panic recovery:** worker 和 agent handler 都有 panic recovery

### `internal/runtime/tasks/embedded_dispatcher.go`
- **职责:** embedded 模式下的 dispatcher 适配器
- 直接调用 worker，不经过 Unix socket

### `internal/runtime/tasks/dispatcher.go`
- **职责:** 独立 host-agent 模式下的 dispatcher
- 通过 `agentapi.Client` 发送请求到 host-agent socket

---

## internal/sshproxy/ — SSH 代理

### `internal/sshproxy/proxy.go`
- **职责:** SSH 代理服务器，将用户 SSH 连接转发到容器内部
- **监听:** 默认 `:2222`（`SSH_PROXY_ADDR`）
- **认证方式:**
  - 密码认证 — 通过 `ContainerResolver.ResolveContainer`
  - 公钥认证 — 通过 `ContainerResolver.ResolveContainerByPublicKey`
- **连接复用:** 每个用户连接预 dial 容器 SSH，所有 channel 共享该连接
- **支持 channel 类型:** `session`, `direct-tcpip`
- **全局请求转发:** `tcpip-forward` / `cancel-tcpip-forward`

### `internal/sshproxy/resolver.go`
- **职责:** 根据用户名/密码或公钥解析目标容器
- 查询 repository 获取 host 和 user 信息

### `internal/sshproxy/forward.go`
- SSH channel 和 TCP 转发实现

---

## internal/store/ — 数据持久化

### `internal/store/repository/models.go`
- **职责:** 数据模型定义
- **核心模型:**
  - `User` — 用户（id, username, status, role, short_id, password_hash, entry_password, ssh_public_key, ssh_private_key, ssh_key_type, expires_at）
  - `Host` — 主机（id, user_id, status, short_id, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, host_ports）
  - `Task` — 任务（id, host_id, kind, status, requested_by, error_code, error_message, last_error_summary, progress_percent, progress_message）
  - `Event` — 审计事件（id, task_id, host_id, user_id, level, type, message, metadata）
  - `EgressIP` — 出口 IP（id, label, ip_address, provider, status, proxy_config）
  - `HostBinding` — 主机与出口 IP 绑定
  - `ClaudeAccount` — Claude 账号（id, user_id, host_id, email, persistent_volume_name, display_name, status）
  - `SSHKey` — SSH 密钥（id, user_id, purpose, label, public_key, private_key, key_type, fingerprint）

### `internal/store/repository/queries.go`
- **职责:** 数据访问方法
- 每个模型一组 CRUD 查询
- 使用 `database/sql` 的 `QueryRow` / `Query` / `Exec`
- 事务支持通过 `BeginTx` 手动管理

### `internal/store/migrations/`
- SQL 迁移文件（按序号命名，如 `001_create_users.sql`）

### `internal/store/migrator/migrator.go`
- 迁移执行器

---

## web/admin/ — React 管理后台

### `web/admin/src/main.tsx`
- 应用入口
- 配置 TanStack Query（retry: 1, refetchOnWindowFocus: false）
- 配置 TanStack Router

### `web/admin/src/lib/api.ts`
- Admin API 封装
- `apiFetch<T>(path, init)` — 自动附加 JWT token，处理 401 跳转登录
- `ApiError` 类封装 HTTP 错误

### `web/admin/src/lib/portal-api.ts`
- 用户门户 API 封装（`/v1/user/*` 前缀）

### `web/admin/src/lib/sse-manager.ts`
- SSE 连接管理器
- 单例模式，支持多 handler 订阅
- 自动重连（指数退避），5 次失败后进入降级模式

### `web/admin/src/hooks/`
- `use-users.ts` — 用户列表/详情/操作
- `use-hosts.ts` — 主机列表/详情/生命周期
- `use-egress-ips.ts` — 出口 IP 管理
- `use-events.ts` — 审计事件
- `use-tasks.ts` — 任务列表
- `use-sse.ts` — SSE 订阅封装
- `use-auth-sessions.ts` — 认证状态

### `web/admin/src/routes/`
- TanStack Router 文件系统路由
- `_dashboard.tsx` — 管理员布局（Sidebar + Topbar），role=admin 守卫
- `_portal.tsx` — 用户门户布局，role=user/admin 守卫
- `login.tsx` — 登录页
- `_dashboard/users/index.tsx` — 用户列表
- `_dashboard/users/$userId.tsx` — 用户详情
- `_dashboard/hosts/index.tsx` — 主机列表
- `_dashboard/hosts/$hostId.tsx` — 主机详情
- `_dashboard/egress-ips/index.tsx` — 出口 IP 列表
- `_dashboard/tasks/index.tsx` — 任务列表
- `_dashboard/events/index.tsx` — 事件列表
- `_portal/portal/index.tsx` — 用户门户首页
- `_portal/portal/hosts/$hostId.tsx` — 用户主机详情

### `web/admin/src/components/`
- `layout/` — 布局组件（sidebar, topbar, data-table-shell, empty-state, page-header）
- `ui/` — 基础 UI 组件（基于 Radix UI + Tailwind）
- `hosts/` — 主机相关组件（create-host-dialog, lifecycle-actions, binding-manager, mount-manager, port-manager, claude-settings-dialog, claude-status-card）
- `users/` — 用户相关组件（create-user-dialog, delete-user-dialog, rotate-password-dialog）
- `egress-ips/` — 出口 IP 组件（egress-ip-drawer, proxy-fields, test-result-dialog）
- `ssh-keys/` — SSH 密钥管理

---

*Components analysis: 2026-06-01*
