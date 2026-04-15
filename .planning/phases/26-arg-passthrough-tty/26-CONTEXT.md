# Phase 26: 参数透传与终端体验 - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

让 `cloud-claude` 的参数透传和终端交互与本地 `claude` 完全一致：用户传入的所有 claude 参数原样传递到容器内 Claude Code，终端窗口 resize / 信号 / 退出码完全透传。

本阶段**不包含**：sshfs 目录映射（Phase 27）、SSH Proxy 服务端改动、新增 cobra 子命令。

</domain>

<decisions>
## Implementation Decisions

### 参数解析与透传
- **D-01:** 根命令使用 cobra 的 `Args: cobra.ArbitraryArgs` 和 `DisableFlagParsing: true`（或 `TraverseChildren: false`），确保 `cloud-claude -p "prompt" --model opus` 的 `-p`、`--model` 等 flag 不被 cobra 拦截，而是完整传入 `args []string`。`init` 子命令仍正常解析自身 flag。
- **D-02:** 远程命令构建方式：将 `args` 拼接为 `claude <arg1> <arg2> ...`，每个 arg 使用 `shellescape` 或手写引号转义以防注入。通过 `session.Start(cmdLine)` 发送到远程 shell。
- **D-03:** 若用户 `cloud-claude -- -p "prompt"` 使用双横线分隔，cobra 在 `DisableFlagParsing` 模式下会保留 `--`，需在构建远程命令时剥离前导 `--`。

### 信号转发
- **D-04:** 当终端处于 raw mode 时，Ctrl+C 和 Ctrl+\ 生成的字节序列直接经 SSH stdin channel 发送到远程，**无需** Go 侧拦截 SIGINT/SIGQUIT 后主动转发——raw mode 下 OS 不会向本进程发送这些信号，而是作为普通字节传到 session.Stdin。
- **D-05:** SIGWINCH 已在 Phase 25 实现（`session.WindowChange`），本阶段复核其在多 OS 下的行为即可。若 macOS/Linux 差异影响窗口变化检测，补充 `SIGWINCH` fallback 或 polling。

### 退出码与 TTY 恢复
- **D-06:** 修复 Phase 25 代码审查 HI-01：`ssh.ExitError` 时 `os.Exit()` 会跳过 `defer term.Restore`，导致本地终端停留在 raw mode。方案：将退出码通过返回值传递到 `main`，由 `main` 在 `defer Restore` 完成后再 `os.Exit`。
- **D-07:** 退出码映射：远程 `claude` 的退出码直接作为 `cloud-claude` 的退出码；SSH 连接断开等异常使用 Phase 25 已有的 `exitInternalError (5)` 退出码。

### 非 TTY 模式
- **D-08:** 当 stdin 不是终端时（如管道 `echo "query" | cloud-claude -p -`），跳过 raw mode、PTY 申请和 SIGWINCH 监听，直接以非交互方式运行远程命令。这使 `cloud-claude` 可被脚本调用。

### Claude's Discretion
- shellescape 库选择（标准库方案或三方包）。
- 是否对 `claude --help` 等 flag 做本地短路（不影响 MVP，可延后）。
- 远程命令路径是 `claude` 还是绝对路径（依赖受管镜像 PATH 配置）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求与路线图
- `.planning/REQUIREMENTS.md` — CLI-03、TTY-01、TTY-02、TTY-03
- `.planning/ROADMAP.md` — Phase 26 Goal、Success Criteria、依赖 Phase 25

### 前序阶段产出
- `.planning/phases/25-cloud-claude-cli/25-CONTEXT.md` — D-01~D-12 决策（cobra 结构、Entry API 契约、SSH 连接、退出码分层）
- `.planning/phases/25-cloud-claude-cli/25-REVIEW.md` — HI-01 TTY 恢复缺陷

### 现有代码
- `cmd/cloud-claude/main.go` — cobra 入口与退出码常量
- `internal/cloudclaude/ssh.go` — SSH 连接、PTY、SIGWINCH、session.Start("claude")
- `internal/cloudclaude/entry.go` — Entry API 客户端
- `internal/cloudclaude/config.go` — 配置读写

### SSH Proxy
- `internal/sshproxy/proxy.go` — 客户端经 Proxy 接入容器，多 session 能力

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/cloudclaude/ssh.go`：已有 SIGWINCH 监听、PTY 申请、term.MakeRaw/Restore——本阶段在此基础上扩展参数透传和退出码修复。
- `cmd/cloud-claude/main.go`：退出码常量已定义，`runRoot` 函数是主流程入口。

### Established Patterns
- cobra 根命令 + `init` 子命令结构。
- `session.Start(command)` 用于远程命令执行。
- `*ssh.ExitError` 用于获取远程退出码。

### Integration Points
- `ConnectAndRunClaude` 函数签名需扩展以接收 `args []string` 参数。
- `runRoot` 需收集 cobra 未解析的参数传递给 SSH 模块。
- 退出码从 `ConnectAndRunClaude` 返回值传递到 `main`，而非在函数内部 `os.Exit`。

</code_context>

<specifics>
## Specific Ideas

无额外产品偏好 — `/gsd-discuss-phase 26 --auto` 采用推荐默认（与 Phase 25 一致的工程约定）。

</specifics>

<deferred>
## Deferred Ideas

- sshfs slave 与当前目录映射 — Phase 27
- `claude --help` 本地短路 — 可在后续 polish 阶段添加
- SSH 主机密钥钉扎 — 延续 Phase 25 决策，暂不实现

</deferred>

---

*Phase: 26-arg-passthrough-tty*
*Context gathered: 2026-04-15*
