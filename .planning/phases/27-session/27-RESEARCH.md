# Phase 27: 双 session 目录映射 - Research

**Researched:** 2026-04-15
**Domain:** sshfs passive mode + embedded SFTP server over SSH multiplexed sessions
**Confidence:** HIGH

## Summary

本阶段的核心挑战是在同一条 SSH 连接上复用两个 session channel：一个承载 sshfs passive 模式的 SFTP 协议流（实现本地目录到容器 /workspace 的映射），另一个承载 claude 的交互式 PTY 会话。Go 侧通过 `pkg/sftp` 嵌入 SFTP server，读写对接 sshfs session 的 stdin/stdout；容器侧通过 `sshfs : /workspace -o passive -f` 启动 FUSE 挂载。

技术栈成熟且路径清晰：`pkg/sftp` v1.13.10 的 `NewServer` 直接接受 `io.ReadWriteCloser`，与 Go `x/crypto/ssh` 的 `Session.StdinPipe()`/`StdoutPipe()` 天然对接；SSH 多 session 能力已在 Phase 24 经代码审查确认；容器内 sshfs 3.7.3 + fuse3 已在 Phase 24 预装。关键落地点是 I/O 管道组装、启动时序控制和退出清理。

**Primary recommendation:** 将 `ConnectAndRunClaude` 拆分为 `connect → mountWorkspace → runClaude` 三阶段，使用 `pkg/sftp` v1.13.10 + `WithServerWorkingDirectory(cwd)` 嵌入 SFTP server，通过 `-o passive -f` 在容器内运行 sshfs。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 使用 `github.com/pkg/sftp` 在 cloud-claude 进程内嵌入 SFTP server。该库是 Go 生态中最成熟的 SFTP 实现，支持 `sftp.NewServer()` 接受 io.ReadWriter 作为传输层，可直接对接 SSH session channel。
- **D-02:** SFTP server 的根目录为用户当前工作目录（`os.Getwd()`），与 MAP-01 需求定义一致。不做 chroot 或虚拟文件系统层，直接服务真实文件。
- **D-03:** 重构 `ConnectAndRunClaude` 为三个阶段：`connect`（建立 SSH 连接）→ `mountWorkspace`（开启 sshfs session + SFTP server）→ `runClaude`（开启 claude PTY session）。复用同一 SSH 连接（`ssh.Client`），不建立第二条 TCP 连接。
- **D-04:** 启动顺序为先挂载后运行 claude：先在 session 1 上 exec `sshfs -o slave /workspace`，启动 SFTP server goroutine，等待挂载就绪确认后，再开启 session 2 申请 PTY 执行 `cd /workspace && claude <args>`。
- **D-05:** 容器内 claude 的工作目录通过 `cd /workspace && claude ...` 设定，与 Phase 26 的 `shellescape.QuoteCommand` 模式一致。
- **D-06:** 挂载就绪通过短生命周期 session exec `mountpoint -q /workspace` 验证。在 sshfs session 启动后，间隔轮询（建议 200ms，上限 10s），成功（exit 0）后继续；超时则报错退出。
- **D-07:** claude session 退出后，关闭 sshfs session 的 channel（触发 stdin EOF），sshfs slave 进程收到 EOF 后自动退出并卸载挂载点。
- **D-08:** 作为防御性补充，在 sshfs channel 关闭后，尝试通过短生命周期 session exec `fusermount -u /workspace 2>/dev/null || true` 确保挂载点清理干净。该步骤失败不影响整体退出码。
- **D-09:** MAP-03 要求会话正常或异常退出时自动清理。使用 Go defer 链确保 cleanup 顺序：defer fusermount → defer close sshfs session → defer close connection。

### Claude's Discretion
- sshfs 的额外挂载参数（如 `-o reconnect`、`-o cache`、`-o ServerAliveInterval` 等性能调优）。
- `pkg/sftp` server 的可选配置（如 ReadOnly 模式是否需要、MaxPacket 大小等）。
- 挂载就绪轮询的具体间隔和超时参数。
- 是否在挂载映射阶段向用户显示进度提示文字。

### Deferred Ideas (OUT OF SCOPE)
- Mutagen 备选目录映射路径 — v2.x ENH-01，当 sshfs 性能不足时再评估
- 大目录 ignore 策略（node_modules 排除等） — v2.x ENH-04
- 端口转发支持 — v2.x ENH-02
- FUSE + AppArmor/seccomp 兼容性验证 — Phase 28
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MAP-01 | 用户当前目录自动映射到容器 /workspace，通过 sshfs slave 实现 | pkg/sftp `WithServerWorkingDirectory(os.Getwd())` + sshfs `-o passive` 实现。SFTP server 以 CWD 为根，sshfs 挂载空目录（`:` = 服务端 CWD）到 /workspace |
| MAP-02 | 映射为双向实时读写，本地改动容器内即时可见，反之亦然 | SFTP 协议本身是请求/响应式，每次文件操作直接透传到对端。无中间缓存层（pkg/sftp server 直接操作本地 FS）。sshfs 客户端缓存可通过参数调低 |
| MAP-03 | 会话结束时自动清理容器内挂载点和相关资源 | 关闭 sshfs session channel → EOF → sshfs 退出自动卸载 + fusermount -u 防御性兜底。Go defer 链保证清理顺序 |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| SFTP server（服务本地文件） | cloud-claude CLI 进程 | — | SFTP server 必须运行在能访问用户本地文件系统的进程中 |
| sshfs FUSE 挂载 | 用户容器（容器内核态 FUSE） | — | FUSE 挂载是容器内操作，需要 SYS_ADMIN + /dev/fuse |
| SSH session 复用 | cloud-claude CLI 进程 | SSH Proxy（透传） | 客户端发起多 session，Proxy 逐个转发到容器 |
| 挂载就绪检测 | cloud-claude CLI 进程 | 容器内 mountpoint 命令 | CLI 通过短生命周期 session 执行远程 mountpoint 命令 |
| 退出清理 | cloud-claude CLI 进程 | 容器内 fusermount | CLI 关闭 channel 触发主清理，fusermount 做兜底 |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/pkg/sftp` | v1.13.10 | 嵌入式 SFTP server | Go 生态唯一成熟的 SFTP 实现，2K+ stars，BSD-2 协议，`NewServer` 接受 `io.ReadWriteCloser` [VERIFIED: pkg.go.dev] |
| `golang.org/x/crypto/ssh` | v0.37.0 | SSH 连接、多 session 管理 | 已在项目中使用，`Session.StdinPipe()`/`StdoutPipe()` 提供管道接口 [VERIFIED: go.mod] |
| `sshfs` (容器内) | 3.7.3 | FUSE 文件系统客户端 | Ubuntu 24.04 apt 仓库版本，支持 `-o passive` 模式 [VERIFIED: Ubuntu noble manpage] |
| `fuse3` (容器内) | Ubuntu 24.04 默认 | FUSE 内核模块用户空间库 | 已在 Phase 24 的 Dockerfile 中预装 [VERIFIED: Dockerfile] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `al.essio.dev/pkg/shellescape` | v1.6.0 | 远程命令行转义 | 构建 `cd /workspace && claude <args>` 命令时使用 [VERIFIED: go.mod] |
| `golang.org/x/term` | v0.42.0 | 终端 raw mode 和尺寸检测 | 复用现有 PTY 会话管理逻辑 [VERIFIED: go.mod] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| pkg/sftp server | OpenSSH sftp-server binary | 需要在 CLI 侧也安装 openssh-server，增加用户机器依赖 |
| sshfs passive | Mutagen | 功能更强但架构更重，已明确推迟到 v2.x ENH-01 |
| sshfs passive | rsync polling | 非实时，无法满足 MAP-02 的即时可见要求 |

**Installation:**

```bash
go get github.com/pkg/sftp@v1.13.10
```

**Version verification:** pkg/sftp v1.13.10 发布于 2025-10-22，为当前最新稳定版 [VERIFIED: pkg.go.dev]。

## Architecture Patterns

### System Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│  用户机器 (cloud-claude CLI)                              │
│                                                          │
│  ┌──────────┐    io.ReadWriteCloser    ┌───────────────┐ │
│  │ pkg/sftp │◄────────────────────────►│ SSH Session 1 │─┼──┐
│  │  Server  │  (stdin/stdout pipes)    │  (sshfs用)    │ │  │
│  │ (CWD根)  │                          └───────────────┘ │  │
│  └──────────┘                                            │  │  同一 ssh.Client
│                                                          │  │
│  ┌──────────┐    stdin/stdout/PTY      ┌───────────────┐ │  │
│  │ 用户终端 │◄────────────────────────►│ SSH Session 2 │─┼──┘
│  │          │                          │  (claude用)   │ │
│  └──────────┘                          └───────────────┘ │
└──────────────────────────────────────────────────────────┘
                          │ TCP (SSH)
                          ▼
                  ┌───────────────┐
                  │  SSH Proxy    │  (多 session 透传,
                  │  (零改造)     │   每个 channel 独立)
                  └───────┬───────┘
                          │ TCP (SSH)
                          ▼
┌─────────────────────────────────────────────────────────┐
│  用户容器                                                │
│                                                          │
│  ┌─────────────────┐         ┌─────────────────────────┐ │
│  │ sshfs           │  FUSE   │ /workspace (mount)      │ │
│  │ -o passive -f   │────────►│   ← 本地 CWD 的镜像     │ │
│  │ (stdin/stdout   │         └────────────┬────────────┘ │
│  │  ↔ SFTP协议)    │                      │              │
│  └─────────────────┘                      ▼              │
│                              ┌─────────────────────────┐ │
│                              │ claude (PTY session)    │ │
│                              │ cd /workspace && claude  │ │
│                              └─────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

**数据流路径：**

1. **文件读取（容器→本地）：** claude 读 /workspace/file → FUSE 拦截 → sshfs 发 SFTP read 请求 → SSH session 1 stdout → cloud-claude 的 pkg/sftp server 收到请求 → 读本地 CWD/file → SFTP response → SSH session 1 stdin → sshfs 返回数据给 FUSE
2. **文件写入（容器→本地）：** claude 写 /workspace/file → FUSE → sshfs SFTP write → pkg/sftp server → 写本地文件
3. **文件写入（本地→容器可见）：** 用户直接编辑本地文件 → 容器内下次 read 时 sshfs 通过 SFTP 获取最新内容

### Pattern 1: I/O 管道组装

**What:** 将 SSH session 的 stdin/stdout pipe 组合为 `io.ReadWriteCloser`，供 `sftp.NewServer` 使用。

**When to use:** 任何需要在 SSH session 上透传二进制协议的场景。

**Example:**

```go
// Source: golang.org/x/crypto/ssh Session API + pkg/sftp NewServer API

type channelRWC struct {
	io.Reader      // 来自 session.StdoutPipe() — 读取 sshfs 的 SFTP 请求
	io.WriteCloser // 来自 session.StdinPipe()  — 发送 SFTP 响应给 sshfs
}

func (c *channelRWC) Close() error {
	return c.WriteCloser.Close() // 关闭 stdin → sshfs 收到 EOF → 自动卸载
}
```

**关键约束：** `StdinPipe()` 和 `StdoutPipe()` 必须在 `session.Start()` 之前调用，否则返回错误 `"ssh: StdinPipe after process started"` [VERIFIED: golang/crypto ssh/session.go 源码]。

### Pattern 2: 三阶段启动时序

**What:** connect → mountWorkspace → runClaude 的严格顺序。

**When to use:** 本阶段的核心重构模式。

**Example:**

```go
// Phase 1: 建立 SSH 连接（复用现有逻辑）
conn, err := sshConnect(cfg)
defer conn.Close()

// Phase 2: 挂载工作目录
cleanupMount, err := mountWorkspace(conn, cwd)
defer cleanupMount()

// Phase 3: 运行 claude（复用现有 PTY / raw mode 逻辑）
exitCode, err := runClaude(conn, claudeArgs)
```

### Pattern 3: 挂载就绪轮询

**What:** 使用短生命周期 session 执行 `mountpoint -q /workspace` 验证挂载状态。

**Example:**

```go
func waitForMount(conn *ssh.Client, mountPath string, interval, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("挂载 %s 超时（%v）", mountPath, timeout)
		case <-ticker.C:
			sess, err := conn.NewSession()
			if err != nil {
				continue
			}
			err = sess.Run("mountpoint -q " + mountPath)
			sess.Close()
			if err == nil {
				return nil // 挂载就绪
			}
		}
	}
}
```

### Anti-Patterns to Avoid

- **在 Start() 之后调用 StdinPipe()/StdoutPipe()：** Go SSH 库会直接返回错误。必须先获取 pipe 再 Start。
- **sshfs 不加 `-f` 参数：** 不加 `-f` 时 sshfs 默认 daemonize，父进程退出后 session channel 关闭，stdin/stdout 管道断裂，SFTP 通信中断。
- **使用 `-o slave` 而非 `-o passive`：** Ubuntu 24.04 的 sshfs 3.7.3 已将 `-o slave` 重命名为 `-o passive`。使用旧名称可能触发弃用警告或未来版本不兼容 [VERIFIED: Ubuntu noble sshfs manpage]。
- **在 sshfs session 上请求 PTY：** sshfs 需要干净的 stdin/stdout 传输 SFTP 二进制协议，PTY 会引入终端转义序列和行缓冲，破坏协议流。
- **sftp.Server.Serve() 在主 goroutine 阻塞：** `Serve()` 是阻塞调用，必须放在 goroutine 中运行，否则会阻塞后续的 claude session 启动。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SFTP 协议实现 | 手写 SFTP 请求/响应解析 | `github.com/pkg/sftp` NewServer | SFTP 协议有 30+ 种请求类型，涉及文件锁、属性、符号链接等，手写极易出错 |
| SSH session 管道 | 自建 TCP 管道或 Unix socket | `ssh.Session.StdinPipe()`/`StdoutPipe()` | 复用已有 SSH 连接，无需额外端口或认证 |
| 远程命令转义 | 手写引号转义 | `shellescape.QuoteCommand` | 已在 Phase 26 验证过的模式 |
| FUSE 挂载 | 在 Go 中通过 FUSE 库直接挂载 | 容器内 `sshfs` 命令 | sshfs 是成熟的用户态工具，处理了所有 FUSE 细节（权限、缓存、异常恢复） |
| 挂载点检测 | 解析 /proc/mounts | `mountpoint -q` 命令 | 标准 util-linux 工具，处理了所有边界情况 |

## Common Pitfalls

### Pitfall 1: sshfs 静默 daemonize 导致管道断裂

**What goes wrong:** sshfs 启动后 fork 到后台，父进程退出。SSH session 的 exit-status 触发，cloud-claude 误以为 sshfs 已结束。子进程虽仍运行但继承的 stdin/stdout fd 可能不再连接到 SSH channel。
**Why it happens:** sshfs 默认行为是 daemonize（与 mount 命令一致）。
**How to avoid:** 始终使用 `-f` 标志保持前台运行。
**Warning signs:** `session.Wait()` 在 sshfs 启动后立即返回，`mountpoint -q` 检测失败。

### Pitfall 2: StdinPipe/StdoutPipe 调用时序错误

**What goes wrong:** 在 `session.Start()` 之后调用 `StdinPipe()` 或 `StdoutPipe()`，返回 `"ssh: StdinPipe after process started"` 错误。
**Why it happens:** Go SSH 库的设计要求 pipe 在 session 启动前配置。
**How to avoid:** 严格按 StdinPipe → StdoutPipe → Start 顺序调用。
**Warning signs:** 函数直接返回 error。

### Pitfall 3: I/O 方向搞反

**What goes wrong:** 将 session.StdinPipe 接到 sftp.Server 的 Reader 上，或将 StdoutPipe 接到 Writer 上，导致 SFTP 协议死锁。
**Why it happens:** 从 cloud-claude 的视角看，StdinPipe 是**写入**到远程 stdin（即 sshfs 的输入），StdoutPipe 是**读取**远程 stdout（即 sshfs 的输出）。而 sftp.Server 需要从 Reader 读取 SFTP 请求、向 Writer 写入 SFTP 响应。sshfs 通过 stdout 发送请求、通过 stdin 接收响应。
**How to avoid:** 明确映射：`sftp.Server Reader = session.StdoutPipe()`（读 sshfs 请求），`sftp.Server Writer = session.StdinPipe()`（写 SFTP 响应给 sshfs）。
**Warning signs:** SFTP 协议握手超时或死锁。

### Pitfall 4: `-o slave` vs `-o passive` 混淆

**What goes wrong:** 使用旧名称 `-o slave` 在较新版本 sshfs 上可能触发警告或未来不兼容。
**Why it happens:** sshfs 3.7.x 将 `-o slave` 重命名为 `-o passive`，旧名称目前仍可用但标记为弃用。
**How to avoid:** 始终使用 `-o passive`。CONTEXT.md D-04 中提到 "sshfs -o slave" 需更新为 `-o passive`。
**Warning signs:** stderr 出现 deprecation warning。

### Pitfall 5: 清理顺序错误导致挂载残留

**What goes wrong:** 先关闭 SSH 连接再尝试 fusermount 清理，fusermount session 无法建立。
**Why it happens:** Go defer 是 LIFO 顺序，声明顺序与执行顺序相反。
**How to avoid:** defer 声明顺序：`conn.Close`（最先声明→最后执行）→ `fusermountCleanup`（中间）→ `sshfsSession.Close`（最后声明→最先执行）。
**Warning signs:** 容器重启后 /workspace 变成 stale mount point。

### Pitfall 6: sftp.Server.Serve() 返回后未 Close

**What goes wrong:** `Serve()` 返回 `io.EOF` 后不调用 `server.Close()` 导致内部资源泄漏。
**Why it happens:** `Serve()` 在连接断开时返回 `io.EOF`，这是正常退出，但仍需 Close。
**How to avoid:** 使用 `defer server.Close()` 在 `Serve()` 之后。
**Warning signs:** goroutine 泄漏（可通过 runtime.NumGoroutine 检测）。

## Code Examples

### 完整的 mountWorkspace 函数骨架

```go
// Source: 基于 pkg/sftp examples/go-sftp-server + golang.org/x/crypto/ssh Session API

func mountWorkspace(conn *ssh.Client, localDir string) (cleanup func(), err error) {
	sshfsSession, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 sshfs session 失败: %w", err)
	}

	// 必须在 Start 之前获取 pipe
	stdin, err := sshfsSession.StdinPipe()
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("获取 sshfs stdin pipe 失败: %w", err)
	}
	stdout, err := sshfsSession.StdoutPipe()
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("获取 sshfs stdout pipe 失败: %w", err)
	}

	// 启动容器内 sshfs（passive 模式 + 前台）
	sshfsCmd := "sshfs : /workspace -o passive -f"
	if err := sshfsSession.Start(sshfsCmd); err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("启动 sshfs 失败: %w", err)
	}

	// 组装 io.ReadWriteCloser 供 SFTP server 使用
	rwc := &channelRWC{Reader: stdout, WriteCloser: stdin}

	// 创建 SFTP server，以用户 CWD 为工作目录
	server, err := sftp.NewServer(rwc,
		sftp.WithServerWorkingDirectory(localDir),
	)
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("创建 SFTP server 失败: %w", err)
	}

	// 在 goroutine 中运行 SFTP server（阻塞直到连接断开）
	sftpDone := make(chan error, 1)
	go func() {
		sftpDone <- server.Serve()
		server.Close()
	}()

	// 等待挂载就绪
	if err := waitForMount(conn, "/workspace", 200*time.Millisecond, 10*time.Second); err != nil {
		sshfsSession.Close()
		return nil, err
	}

	// 返回清理函数
	cleanup = func() {
		// 1. 关闭 sshfs session → EOF → sshfs 自动卸载
		sshfsSession.Close()
		// 2. 等待 SFTP server goroutine 结束（可加短超时）
		<-sftpDone
		// 3. 防御性 fusermount
		fusermountCleanup(conn)
	}
	return cleanup, nil
}
```

### channelRWC 适配器

```go
// 将 SSH session 的 stdin/stdout pipe 适配为 sftp.NewServer 需要的 io.ReadWriteCloser
type channelRWC struct {
	io.Reader      // session.StdoutPipe() — 读 sshfs 发出的 SFTP 请求
	io.WriteCloser // session.StdinPipe()  — 写 SFTP 响应给 sshfs
}

// Close 关闭写端，向 sshfs 发送 EOF
func (c *channelRWC) Close() error {
	return c.WriteCloser.Close()
}
```

### fusermount 防御性清理

```go
func fusermountCleanup(conn *ssh.Client) {
	sess, err := conn.NewSession()
	if err != nil {
		return // 连接已断开，无需清理
	}
	defer sess.Close()
	_ = sess.Run("fusermount -u /workspace 2>/dev/null || true")
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| sshfs `-o slave` | sshfs `-o passive` | sshfs 3.7.x (2022+) | 旧名称仍可用但标记弃用，新代码应使用 `-o passive` |
| pkg/sftp 无 WorkDir 选项 | `WithServerWorkingDirectory()` 可设定工作目录 | pkg/sftp 近期版本 | 不再需要手动 chdir 或路径前缀处理 |
| 需要 dpipe 工具连接管道 | SSH session channel 直接提供 stdin/stdout | 一直如此 | 无需安装额外工具，Go SSH 库原生支持 |

**Deprecated/outdated:**
- `-o slave`：已重命名为 `-o passive`，建议始终使用新名称。

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | sshfs 在容器内以 workspace 用户运行时有权限执行 FUSE 挂载 | Architecture Patterns | 如果权限不足，挂载会失败。Phase 24 已配置 SYS_ADMIN + /dev/fuse + user_allow_other 应足够 [ASSUMED — 基于 Phase 24 决策，未在运行时验证] |
| A2 | `mountpoint -q` 在容器内可用（util-linux 包） | Common Pitfalls | Ubuntu 24.04 基础镜像通常包含此命令。如果不可用，需改用其他检测方式 [ASSUMED] |
| A3 | sshfs 3.7.3 的 `-o passive` 接受空 host（`:` 格式）| Architecture Patterns | 如果需要非空 host，可使用 `sshfs x: /workspace -o passive -f` [ASSUMED — 基于 dpipe 文档示例] |
| A4 | SFTP server Serve() 在 channel 关闭时返回 io.EOF（干净退出） | Common Pitfalls | 如果返回其他错误类型，可能需要额外的错误处理逻辑 [ASSUMED — 基于 pkg/sftp 官方示例] |

## Open Questions

1. **sshfs `-o passive` 是否需要额外权限或配置？**
   - What we know: sshfs 3.7.3 支持 `-o passive`，Phase 24 已配置 FUSE 权限
   - What's unclear: 是否有容器级别的额外限制（如 seccomp profile 拦截 FUSE 系统调用）
   - Recommendation: Phase 28 专门处理 AppArmor/seccomp 兼容性，本阶段在开发环境验证即可

2. **sshfs 缓存对 MAP-02 "即时可见"的影响程度**
   - What we know: sshfs 默认 cache_timeout=20s、attr_timeout=1s。文件内容不缓存（每次通过 SFTP 读取）
   - What's unclear: 目录列表缓存是否影响 claude 的文件发现能力
   - Recommendation: 先用默认缓存设置。如果 claude 发现文件延迟明显，再添加 `-o cache_timeout=1` 调低

3. **SFTP server 的 MaxPacket 大小是否需要调大？**
   - What we know: 默认 32768 bytes，`WithMaxTxPacket()` 可调大
   - What's unclear: Claude Code 的典型文件操作是否受限于此包大小
   - Recommendation: 默认值足够 MVP 使用。大文件操作会自动分包，只影响吞吐量不影响正确性

## Environment Availability

> 本阶段的外部依赖均已在 Phase 24 的 Dockerfile 中预装。

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| sshfs (容器内) | FUSE 挂载 | ✓ | 3.7.3 (Ubuntu 24.04 apt) | — |
| fuse3 (容器内) | FUSE 内核模块支持 | ✓ | Ubuntu 24.04 默认 | — |
| /dev/fuse (容器内) | FUSE 挂载 | ✓ | Phase 24 --device /dev/fuse | — |
| SYS_ADMIN cap (容器内) | FUSE 挂载 | ✓ | Phase 24 --cap-add SYS_ADMIN | — |
| mountpoint (容器内) | 挂载就绪检测 | ✓ | util-linux (Ubuntu 基础镜像自带) | `stat -f /workspace` 或检查 /proc/mounts |
| fusermount (容器内) | 清理兜底 | ✓ | fuse3 包附带 | — |
| pkg/sftp (CLI 侧) | SFTP server | 需 go get | v1.13.10 | — |

**Missing dependencies with no fallback:**
- `github.com/pkg/sftp` — 需要通过 `go get` 添加到项目依赖

**Missing dependencies with fallback:**
- 无

## Sources

### Primary (HIGH confidence)
- [pkg.go.dev/github.com/pkg/sftp](https://pkg.go.dev/github.com/pkg/sftp) — v1.13.10 API 文档，发布日期 2025-10-22
- [github.com/pkg/sftp/blob/master/server.go](https://github.com/pkg/sftp/blob/master/server.go) — NewServer、WithServerWorkingDirectory、Serve 源码
- [github.com/pkg/sftp/blob/master/examples/go-sftp-server/main.go](https://github.com/pkg/sftp/blob/master/examples/go-sftp-server/main.go) — 官方 SFTP server 示例
- [github.com/golang/crypto/blob/master/ssh/session.go](https://github.com/golang/crypto/blob/master/ssh/session.go) — StdinPipe/StdoutPipe 源码，确认调用时序约束
- [Ubuntu noble sshfs manpage](https://manpages.ubuntu.com/manpages/noble/en/man1/sshfs.1.html) — sshfs 3.7.3，确认 `-o passive` 选项
- [github.com/libfuse/sshfs/blob/master/sshfs.rst](https://github.com/libfuse/sshfs/blob/master/sshfs.rst) — sshfs 官方文档，passive 模式说明

### Secondary (MEDIUM confidence)
- [man7.org sshfs(1)](https://www.man7.org/linux/man-pages/man1/SSHFS.1.html) — sshfs 选项完整参考
- [GitHub dustymabe/vagrant-sshfs#11](https://github.com/dustymabe/vagrant-sshfs/issues/11) — slave/passive 模式历史背景
- [StackOverflow: Go SSH multiple sessions](https://stackoverflow.com/questions/24440193/golang-ssh-how-to-run-multiple-commands-on-the-same-session) — 验证同一连接多 session 模式

### Tertiary (LOW confidence)
- 无

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — pkg/sftp 和 sshfs 都是各自生态中的标准选择，版本已验证
- Architecture: HIGH — SSH 多 session、SFTP 协议透传都是成熟模式，且 SSH Proxy 已确认支持
- Pitfalls: HIGH — 基于官方源码和文档确认的具体约束（pipe 时序、daemonize 行为、命名变更）

**Research date:** 2026-04-15
**Valid until:** 2026-05-15（30 天 — 涉及的技术栈均处于稳定维护状态）
