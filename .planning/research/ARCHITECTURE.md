# Architecture Research

**Domain:** cloud-claude 透明远程 CLI（客户端二进制 + 本地目录 ↔ 远端容器实时映射）  
**Researched:** 2026-04-15  
**Confidence:** HIGH（与现有 `internal/sshproxy`、`internal/runtime/tasks/worker.go`、`tools/cloud-dev/main.go` 对照）/ MEDIUM（Mutagen 具体拓扑以选型验证为准）

## Standard Architecture

### System Overview

在 **v1.x 已交付** 的分层上，v2.0 仅增加 **用户侧 `cloud-claude` 二进制** 与 **目录同步/挂载策略**；控制面、host-agent、Docker 创建、网络 Provider、React 后台 **保持不变**，除非单独列出「可选修改」。

```
┌────────────────────────────────────────────────────────────────────────────┐
│ 用户工作站（开发者笔记本）                                                    │
├────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐     ┌──────────────────────────────────────────────┐ │
│  │ cloud-claude     │     │ 目录映射层（二选一或组合）                         │ │
│  │ (Go 单二进制)    │     │  A) sshfs slave + 本机 SFTP（与 tools/cloud-dev   │ │
│  │ init / 透传 claude│     │     同模式）                                       │ │
│  └────────┬─────────┘     │  B) Mutagen 同步进程（见下文分层说明）              │ │
│           │               └──────────────────────────────────────────────┘ │
│           │ SSH :22 (password 或 pubkey)                                    │
└───────────┼────────────────────────────────────────────────────────────────┘
            ▼
┌───────────────────────────────────────────────────────────────────────────┐
│ 宿主机：SSH Proxy (internal/sshproxy)                                      │
│  PasswordCallback / PublicKeyCallback → RepoResolver → 容器 IP:22           │
│  每收到一个客户端 session 通道 → 独立 ssh.Dial(容器) + OpenChannel(session)   │
│  pty-req / shell / exec / window-change / exit-status 双向原样转发            │
└───────────┬───────────────────────────────────────────────────────────────┘
            ▼
┌───────────────────────────────────────────────────────────────────────────┐
│ 容器：OpenSSH + 受管镜像                                                    │
│  - 已有：-v 宿主机 homeDir → /workspace（持久化「云盘」）                     │
│  - v2 增量：FUSE + sshfs（create 已带 --device /dev/fuse）                    │
│  - 用户态：在 /workspace 或单独 mountpoint 上跑 claude / Claude Code          │
└───────────────────────────────────────────────────────────────────────────┘
            ▲
┌───────────┴───────────────────────────────────────────────────────────────┐
│ 网络：ContainerProxyProvider（bridge + sing-box 侧车）等 — 与 v1.1 一致      │
└───────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|------------------|-------------------------|
| **cloud-claude** | 读 `~/.cloud-claude/config`；建立到网关 `SSH Proxy` 的连接；编排「挂载 → exec claude」；TTY/信号/退出码 | Go，`golang.org/x/crypto/ssh`，可参考 `tools/cloud-dev/main.go` |
| **SSH Proxy** | 认证后把每个 session 通道桥到容器 SSH；不解析 exec 内容 | 现有 `internal/sshproxy/proxy.go` |
| **RepoResolver** | `short_id` + 凭证 → 容器 IP、容器用户、入口密码 | 现有 `internal/sshproxy/resolver.go` |
| **host-agent Worker** | `docker create/start`、`-v` 持久 home、`--device /dev/fuse`、网络 PrepareHost | 现有 `internal/runtime/tasks/worker.go` |
| **目录映射（sshfs 路径）** | 在**客户端**跑 SFTP 服务端；在**容器**内 `sshfs -o slave` 挂到选定 mountpoint | 与 SSH 会话正交，仅占用额外 `session` 通道 |
| **目录映射（Mutagen 路径）** | 独立同步守护进程；不经过 SSH Proxy 解析 | 通常落在「宿主机持久目录」或「容器 SSH」一侧，见下节 |

## Recommended Project Structure（与仓库对齐）

```
cmd/cloud-claude/          # 正式交付的单一二进制入口（待建或从 tools 提升）
tools/cloud-dev/           # 已存在的 PoC：sshfs slave + shell（研发/对照用）
internal/sshproxy/         # 服务端：保持「透明转发」契约
internal/runtime/tasks/    # 容器创建与挂载宿主机目录
```

### Structure Rationale

- **`cloud-claude` 放在 `cmd/`：** 与「可安装 CLI」定位一致，便于版本发布与 `go install`。  
- **保留 `tools/cloud-dev`：** 已验证多 `session` 与 sshfs/slave 数据路径，可作为集成测试夹具。  
- **不强行把映射逻辑塞进 `internal/sshproxy`：** 映射是客户端 + 容器内命令的组合，代理层保持协议无关。

## Architectural Patterns

### Pattern 1: SSH Session 作为唯一数据平面（推荐主路径）

**What:** 用户只打开到 `SSH Proxy` 的 SSH 连接；`cloud-claude` 在其上创建多个 `session`——其一跑容器内 `sshfs -o slave`，本地对接 SFTP；其二请求 `pty` + `exec`/`shell` 运行 `claude` 及子进程。  
**When to use:** 与产品目标「透明替代 `claude`」、PROJECT 中「SSH Proxy 零改造」一致。  
**Trade-offs:** 延迟与吞吐受 SSH 加密与单流复用影响；实现复杂度集中在客户端。

**Evidence（仓库内）：**

```97:120:tools/cloud-dev/main.go
// startSSHFSMount opens an SSH exec channel running `sshfs -o slave` in the
// container, then starts a local SFTP server that feeds file data back through
// the same channel.  Returns a cleanup function.
func startSSHFSMount(client *ssh.Client, localDir, mountPoint string) (func(), error) {
	// ...
	cmd := fmt.Sprintf(
		"mkdir -p %s && exec sshfs -o slave -o allow_other -o reconnect -o cache=yes -o kernel_cache :%s %s",
		mountPoint, localDir, mountPoint,
	)
```

### Pattern 2: 宿主机 bind mount + Mutagen（可选）

**What:** 持久数据仍在 `worker` 创建的宿主机 `homeDir` ↔ 容器 `/workspace`；Mutagen 在**用户机**与**宿主机目录**或**经 SSH 的远端路径**之间做双向同步。  
**When to use:** 需要更强离线/冲突处理、或 sshfs 性能不足时。  
**Trade-offs:** 组件多、需明确同步根与权限；不一定与「当前 shell 目录」天然一致，需在 `cloud-claude` 内做路径约定或一次性配置。

### Pattern 3: 每通道独立 Dial 容器（当前 Proxy 实现）

**What:** 每个客户端 `session` 通道对应一次新的 `ssh.Dial` 到容器（见 `handleChannel`）。  
**When to use:** 现状即如此；多会话（sshfs + exec）**无需**改 Proxy 即可工作。  
**Trade-offs:** 容器侧 SSH 连接数 = 客户端并发 session 数；高并发时可再优化为「单连接多通道复用」（**非 v2 必选**）。

## Data Flow

### 端到端：`claude`（实为 cloud-claude）→ 容器内 Claude Code

以下覆盖 **非交互一次执行** 与 **交互 TTY** 两种场景；差别仅在第二个 session 请求 `exec` 还是 `pty-req`+`shell`。

```
用户终端输入: cloud-claude [与原生 claude 相同参数]
    ↓
cloud-claude 解析参数、读取 ~/.cloud-claude/config（网关地址、short_id、凭证）
    ↓
TCP 连接 gateway:SSH_PROXY_PORT，SSH 握手
    ↓
认证：用户名 = host short_id，密码或 inbound 公钥（与现有 Resolver 一致）
    ↓
┌─ Session A（目录映射，若采用 sshfs 方案）───────────────────────────────┐
│ cloud-claude: NewSession → Start("sshfs -o slave ... <mount>")            │
│ SSH Proxy: 新 channel → 新 ssh.Dial(容器) → OpenChannel(session)         │
│ 容器: sshfs 进程；用户机: SFTP server 绑定 session 的 stdin/stdout          │
│ 结果: <mount> 上可见用户机当前工程目录内容                                   │
└──────────────────────────────────────────────────────────────────────────┘
    ↓
┌─ Session B（Claude Code）─────────────────────────────────────────────────┐
│ cloud-claude: NewSession → RequestPty（若交互）或 exec                    │
│              → 远端命令如: cd <mount> && claude <args>  （具体与镜像 PATH 一致）│
│ SSH Proxy: 同上透明转发 pty-req / window-change / exec / env              │
│ 容器: node/claude 子进程；stdio 经 SSH 回到用户终端                          │
│ 退出: exit-status / exit-signal 经 Proxy 回到客户端 → 进程退出码            │
└──────────────────────────────────────────────────────────────────────────┘
```

### 与「仅 SSH 登录云主机」路径的关系

| 路径 | 是否经过 cloud-claude | 目录来源 |
|------|----------------------|----------|
| `curl` bootstrap 后 `ssh short_id@gateway` | 否 | 主要为宿主机持久化的 `/workspace` |
| `cloud-claude` | 是 | 用户机当前目录经 sshfs/Mutagen 映射到约定 mountpoint |

两条路径可并存；**不要求**合并为同一挂载实现。

### Key Data Flows

1. **凭证与路由：** 仍由控制面 DB + `RepoResolver` 解析，**无新协议**。  
2. **文件字节：** sshfs 方案走 **Session A** 的 stdin/stdout（SFTP）；与 **Session B** 上 claude 的终端流 **相互独立**。  
3. **出网流量：** 仍由现有 `network.Provider` + 容器内 tun/代理链保证，**不**因 cloud-claude 改变。

## 目录映射层应放在哪一层？

| 方案 | 位置 | 与 SSH Proxy 关系 | 说明 |
|------|------|-------------------|------|
| **sshfs slave + 本机 SFTP** | **客户端二进制内部** + **容器内 sshfs** | **仅占用额外 SSH session**，Proxy 不感知 | 与 `tools/cloud-dev` 一致，最贴合「零改造 Proxy」 |
| **Mutagen → 宿主机 homeDir** | **用户机 Mutagen** ↔ **宿主机** `/var/lib/.../hosts/<id>/` | Proxy **不参与**；Worker 已有 `-v` | 「实时」由 Mutagen 保证；需处理与 sshfs 方案二选一或不同 mountpoint |
| **Mutagen → 容器 SSH** | Mutagen 插件连容器文件系统 | 经 SSH，但 **不是** Proxy 特殊逻辑 | 运维与权限模型需单独设计 |

**结论：** 目录映射 **不属于** `internal/sshproxy` 的一层服务；应落在 **cloud-claude（客户端）** 与 **镜像/容器内工具（sshfs/fuse）** 的组合；Mutagen 若采用，属于 **同步侧车进程**，架构上平行于 SSH 会话转发。

## 是否需要新增服务端 API 或修改 SSH Proxy？

| 项 | 建议 | 理由 |
|----|------|------|
| **SSH Proxy** | **默认无需改** | 已支持多 `session`、exec/pty/window-change/exit 转发；`tools/cloud-dev` 已验证双 session。 |
| **RepoResolver / 认证** | **默认无需改** | short_id + 密码/公钥 已满足 `cloud-claude` 非交互登录。 |
| **控制面 HTTP API** | **按需** | 「启动主机后再执行」可复用现有任务/主机状态接口；若仅依赖用户先 bootstrap 使主机 `running`，可无新 API。 |
| **镜像** | **已有 FUSE 设备** | `docker create` 已含 `--device /dev/fuse`；需保证镜像内 **sshfs** 与用户空间工具齐全（属镜像/打包，非 Proxy 代码）。 |

**可选增强（非阻塞）：**

- 为 `cloud-claude` 提供 **轻量健康检查**（例如公开 `GET /healthz` 或已有入口）仅用于预检网关可达性。  
- 若产品要求「一条命令从停机的 host 拉到 running」，则在客户端调用 **现有** 主机启动 API，**不**必为映射单独造 API。

## Integration Points（集成点清单）

### 与现有组件

| 集成点 | 方向 | 契约 |
|--------|------|------|
| **SSH Proxy :22** | cloud-claude → Proxy → 容器 | SSH 协议；用户名为 host short_id |
| **RepoResolver** | Proxy → Postgres | `GetHostByShortID`、主机 `running`、容器 IP |
| **容器 OpenSSH** | Proxy → 容器 | 多 session、exec、PTY |
| **镜像** | Worker 已 create | `fuse` + sshfs、Claude Code 在 PATH |
| **出站网络** | 容器内 | 现有 sing-box Provider，与 v1.1 一致 |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| cloud-claude ↔ SSH Proxy | TCP + SSH | 映射与 claude 执行均为普通 session |
| SSH Proxy ↔ 容器 | 每 session 一次 `ssh.Dial` | 见 Pattern 3 |
| cloud-claude ↔ 控制面 | 可选 HTTPS | 仅主机启动/状态，非映射数据面 |

## 新增 vs 修改组件

| 类型 | 组件 | 说明 |
|------|------|------|
| **新增** | `cloud-claude` 二进制（`cmd/cloud-claude` 或等价） | init、SSH 客户端、sshfs 编排、TTY/信号、参数透传 |
| **新增（可选）** | 安装/打包脚本、shell 补全 | 产品化 |
| **修改（小）** | 受管 Dockerfile / 镜像 lock | 确保 `sshfs`、fuse 依赖完整 |
| **修改（可选）** | SSH Proxy 连接复用 | 性能优化，非功能必需 |
| **不改** | `handleConnection` 转发语义、Resolver 规则、Worker 创建主流程 | 除非后续实测需收紧 |

## 建议构建顺序（依赖优先）

1. **镜像与容器：** 确认 `sshfs` + FUSE 在受管镜像内可用（与现有 `--device /dev/fuse` 对齐）。  
2. **cloud-claude MVP：** 仅 SSH + **单 session** `exec` 跑 `claude`（映射到容器已有 `/workspace` 用于冒烟）— 验证透传。  
3. **双 session：** 接入 `tools/cloud-dev` 同类 sshfs slave + SFTP，再 `cd` 到 mountpoint 执行 `claude`。  
4. **体验对齐：** TTY、SIGWINCH、`exit code`、非交互/交互分支。  
5. **Mutagen（若选型）：** 与 sshfs 方案对比基准测试后再并行支持。  
6. **（可选）** Proxy 侧连接池/复用 — 在监控到连接数或延迟问题后做。

## Anti-Patterns

### Anti-Pattern 1: 在 SSH Proxy 内解析 `exec` 注入挂载

**Why it's wrong:** 破坏协议透明性，且与「多会话独立 Dial」模型纠缠。  
**Do this instead:** 挂载全部由 `cloud-claude` 发普通 exec/session。

### Anti-Pattern 2: 假设 Mutagen 与 sshfs 同时写同一目录

**Why it's wrong:** 冲突与缓存不一致。  
**Do this instead:** 产品只支持一种主映射方案，或明确互斥 mountpoint。

### Anti-Pattern 3: 为映射引入第二条非 SSH 数据面（未经评审）

**Why it's wrong:** 防火墙、审计与「全隧道出网」叙事变复杂。  
**Do this instead:** 优先 SSH 承载；若加 WebSocket/FTP 需单独安全评审。

## Scaling Considerations

| Scale | 要点 |
|-------|------|
| 单用户单机 | 当前每 session 独立 Dial 足够 |
| 多会话自动化 | 关注容器 `sshd` 的 `MaxSessions` 与文件句柄 |
|  many hosts | 控制面与 Resolver 已是 DB 驱动；客户端仅增配置项 |

## Sources

- 仓库：`internal/sshproxy/proxy.go`（会话转发与每通道 Dial）  
- 仓库：`internal/runtime/tasks/worker.go`（`docker create`、`/dev/fuse`、`-v` homeDir）  
- 仓库：`tools/cloud-dev/main.go`（sshfs slave + SFTP + PTY shell PoC）  
- 产品：`.planning/PROJECT.md`（v2.0 cloud-claude 目标与「SSH Proxy 零改造」）

---
*Architecture research for: cloud-claude 透明远程 CLI 与现有 SSH Proxy 集成*  
*Researched: 2026-04-15*
