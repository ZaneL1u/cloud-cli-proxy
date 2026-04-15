# Phase 27: 双 session 目录映射 - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

通过 sshfs slave 模式将用户当前工作目录实时映射到容器 /workspace，实现双向读写。cloud-claude 在同一 SSH 连接上开启第二个 session channel，容器内运行 `sshfs -o slave /workspace`，cloud-claude 侧嵌入 SFTP server 服务本地目录，SFTP 协议经 SSH session channel 透传。

本阶段**不包含**：SSH Proxy 服务端改动（延续 Phase 24 零改造结论）、FUSE + AppArmor/seccomp 兼容性验证（Phase 28）、Mutagen 备选路径（v2.x ENH-01）。

</domain>

<decisions>
## Implementation Decisions

### SFTP 服务端实现
- **D-01:** 使用 `github.com/pkg/sftp` 在 cloud-claude 进程内嵌入 SFTP server。该库是 Go 生态中最成熟的 SFTP 实现，支持 `sftp.NewServer()` 接受 io.ReadWriter 作为传输层，可直接对接 SSH session channel。
- **D-02:** SFTP server 的根目录为用户当前工作目录（`os.Getwd()`），与 MAP-01 需求定义一致。不做 chroot 或虚拟文件系统层，直接服务真实文件。

### Session 生命周期与时序
- **D-03:** 重构 `ConnectAndRunClaude` 为三个阶段：`connect`（建立 SSH 连接）→ `mountWorkspace`（开启 sshfs session + SFTP server）→ `runClaude`（开启 claude PTY session）。复用同一 SSH 连接（`ssh.Client`），不建立第二条 TCP 连接。
- **D-04:** 启动顺序为先挂载后运行 claude：先在 session 1 上 exec `sshfs -o slave /workspace`，启动 SFTP server goroutine，等待挂载就绪确认后，再开启 session 2 申请 PTY 执行 `cd /workspace && claude <args>`。
- **D-05:** 容器内 claude 的工作目录通过 `cd /workspace && claude ...` 设定，与 Phase 26 的 `shellescape.QuoteCommand` 模式一致。

### 挂载就绪检测
- **D-06:** 挂载就绪通过短生命周期 session exec `mountpoint -q /workspace` 验证。在 sshfs session 启动后，间隔轮询（建议 200ms，上限 10s），成功（exit 0）后继续；超时则报错退出。

### 异常退出清理
- **D-07:** claude session 退出后，关闭 sshfs session 的 channel（触发 stdin EOF），sshfs slave 进程收到 EOF 后自动退出并卸载挂载点。
- **D-08:** 作为防御性补充，在 sshfs channel 关闭后，尝试通过短生命周期 session exec `fusermount -u /workspace 2>/dev/null || true` 确保挂载点清理干净。该步骤失败不影响整体退出码。
- **D-09:** MAP-03 要求会话正常或异常退出时自动清理。使用 Go defer 链确保 cleanup 顺序：defer fusermount → defer close sshfs session → defer close connection。

### Claude's Discretion
- sshfs 的额外挂载参数（如 `-o reconnect`、`-o cache`、`-o ServerAliveInterval` 等性能调优）。
- `pkg/sftp` server 的可选配置（如 ReadOnly 模式是否需要、MaxPacket 大小等）。
- 挂载就绪轮询的具体间隔和超时参数。
- 是否在挂载映射阶段向用户显示进度提示文字。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求与路线图
- `.planning/REQUIREMENTS.md` — MAP-01、MAP-02、MAP-03 定义了目录映射、双向读写和自动清理的具体要求
- `.planning/ROADMAP.md` — Phase 27 Goal、Success Criteria、依赖 Phase 26 和 Phase 24

### 前序阶段产出
- `.planning/phases/24-fuse/24-CONTEXT.md` — D-01~D-06：sshfs/fuse3 预装、FUSE 设备权限、SSH Proxy 多 session 确认
- `.planning/phases/25-cloud-claude-cli/25-CONTEXT.md` — D-01~D-12：cobra 结构、Entry API 契约、SSH 连接建立
- `.planning/phases/26-arg-passthrough-tty/26-CONTEXT.md` — D-01~D-08：shellescape、退出码修复、非 TTY 模式

### 现有代码
- `cmd/cloud-claude/main.go` — cobra 入口、退出码常量、runRoot 主流程
- `internal/cloudclaude/ssh.go` — ConnectAndRunClaude 当前实现（单 session），本阶段需重构为三阶段
- `internal/cloudclaude/entry.go` — Entry API 客户端（不改动）
- `internal/cloudclaude/config.go` — 配置读写（不改动）

### SSH Proxy
- `internal/sshproxy/proxy.go` — handleConnection 多 session 支持（`for newChan := range chans`），handleChannel 全类型请求转发

### 第三方库
- `github.com/pkg/sftp` — Go SFTP server/client 库，用于嵌入式 SFTP server 实现

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/cloudclaude/ssh.go`：`ConnectAndRunClaude` 已有 SSH 连接建立、PTY 申请、SIGWINCH 监听、raw mode 管理——本阶段需拆分重构但逻辑可复用。
- `cmd/cloud-claude/main.go`：退出码常量和 runRoot 流程已定义，扩展为双 session 模式。
- SSH Proxy `handleConnection`：`for newChan := range chans` 循环天然支持同一连接上的多个 session channel，无需服务端改动。

### Established Patterns
- 单一 `ssh.Client` 上调用 `conn.NewSession()` 可获取独立 session channel。
- `session.Start(command)` 用于远程命令执行，sshfs slave 命令同样使用此模式。
- `shellescape.QuoteCommand` 用于安全构建远程命令行。
- Go defer 链管理资源生命周期。

### Integration Points
- `ConnectAndRunClaude` 需要重构：将 SSH 连接建立提取出来，使两个 session 共享同一个 `ssh.Client`。
- sshfs session 的 stdin/stdout 对接 `sftp.NewServer()` 的 io.ReadWriteCloser。
- claude session 的工作目录从当前的 `claude <args>` 变为 `cd /workspace && claude <args>`。
- `runRoot` 中可能需要在认证成功和 claude 启动之间插入目录映射步骤。

</code_context>

<specifics>
## Specific Ideas

无额外产品偏好 — `/gsd-discuss-phase 27 --auto` 采用推荐默认（sshfs slave + pkg/sftp 嵌入式 SFTP server 方案）。

</specifics>

<deferred>
## Deferred Ideas

- Mutagen 备选目录映射路径 — v2.x ENH-01，当 sshfs 性能不足时再评估
- 大目录 ignore 策略（node_modules 排除等） — v2.x ENH-04
- 端口转发支持 — v2.x ENH-02
- FUSE + AppArmor/seccomp 兼容性验证 — Phase 28

</deferred>

---

*Phase: 27-session*
*Context gathered: 2026-04-15*
