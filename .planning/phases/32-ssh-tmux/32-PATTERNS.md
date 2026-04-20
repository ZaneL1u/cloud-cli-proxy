# Phase 32: SSH 会话可靠性 + tmux 包装 + 多端 - Pattern Map

**Mapped:** 2026-04-20
**Files analyzed:** 13（8 新建 + 5 改造）
**Analogs found:** 13 / 13（每个新文件都有可直接 mirror 的同包 analog；`keepalive_*.go` 三平台分发文件无既有 build-tag analog，复用 `mutagen_bin.go::extractMutagenFor` 的 `runtime.GOOS` switch 思路新建）

---

## File Classification

| 新建 / 改造文件 | Role | Data Flow | Closest Analog | Match Quality |
|--|--|--|--|--|
| `internal/cloudclaude/session.go`（新建） | service | request-response | `internal/cloudclaude/mount_strategy.go` + `internal/cloudclaude/oauth_check.go` | role-match（state machine + remote probe 复用 oauth_check 模式） |
| `internal/cloudclaude/reconnect.go`（新建） | service | event-driven | `internal/cloudclaude/sshfs_watcher.go` | exact（ctx + ticker + 失败计数 模式 1:1） |
| `internal/cloudclaude/input_buffer.go`（新建） | utility | streaming | `internal/cloudclaude/mount_sshfs.go`（StdinPipe + channelRWC） + `internal/cloudclaude/colors.go`（ANSI 包装） | role-match（无完全相同的 byte-stream wrapper，但 io.Pipe 模式与 mount_sshfs 一致） |
| `internal/cloudclaude/keepalive.go`（新建） | service | event-driven | `internal/cloudclaude/sshfs_watcher.go` | exact（同样 ctx + ticker + 失败计数 + return on threshold） |
| `internal/cloudclaude/keepalive_linux.go`（新建） | utility | request-response | `internal/cloudclaude/mutagen_bin.go::extractMutagenFor`（runtime.GOOS 分发） | role-match（codebase 暂无 build-tag 文件分发先例，按 RESEARCH §2 创建） |
| `internal/cloudclaude/keepalive_darwin.go`（新建） | utility | request-response | 同上 | role-match |
| `internal/cloudclaude/keepalive_other.go`（新建） | utility | request-response | 同上 | role-match |
| `internal/cloudclaude/sync_lock.go`（新建） | service | request-response | `internal/cloudclaude/oauth_check.go` + `internal/cloudclaude/mount_sshfs.go`（shellescape + 远程命令） | exact（exit-code 解析 → 状态枚举模式 1:1） |
| `internal/cloudclaude/errcodes/session.go`（新建） | config | static | `internal/cloudclaude/errcodes/mount.go` | exact（init() + MustRegister 模板逐字符复刻） |
| `cmd/cloud-claude/sessions.go`（新建） | cli | request-response | `cmd/cloud-claude/sync.go` | exact（cobra subcommand 树 + `LoadConfig + AuthenticateAndWait + sshConnect` 模板） |
| `internal/cloudclaude/ssh.go`（改造） | controller | request-response | 自身（在 `sshConnect` 与 `ConnectAndRunClaudeV3` 内部局部插入） | exact |
| `internal/cloudclaude/last_session.go`（改造） | model | static | 自身（追加 omitempty 字段） | exact |
| `internal/cloudclaude/errcodes/codes.go`（改造） | config | static | 自身（追加 10 条 `Code` 常量） | exact |
| `internal/cloudclaude/errcodes/net.go`（改造） | config | static | 自身 + `errcodes/mount.go` | exact |
| `cmd/cloud-claude/main.go`（改造） | cli | request-response | 自身 `runRoot` 内 `--mount-mode` 剥离段 | exact |
| `internal/cloudclaude/colors.go`（改造） | utility | static | 自身（追加 `ansiGray` 常量；`ansiRed` 已存在 line 12 无需新增） | exact |

---

## Pattern Assignments

### `internal/cloudclaude/reconnect.go` — service / event-driven

**Analog:** `internal/cloudclaude/sshfs_watcher.go`（CONTEXT D-05 / RESEARCH §3.1 / RESEARCH §Patterns to Reuse P-06）

**导入与 struct 模式**（lines 1-12 + 25-33）：

```1:33:internal/cloudclaude/sshfs_watcher.go
package cloudclaude

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)
```

```25:33:internal/cloudclaude/sshfs_watcher.go
type SSHFSWatcher struct {
	conn         *ssh.Client
	coldPath     string
	interval     time.Duration
	failureLimit int
	logger       io.Writer
	onDisconnect func() error
	check        func() bool
}
```

**Run 主循环**（lines 51-76）：

```51:76:internal/cloudclaude/sshfs_watcher.go
func (w *SSHFSWatcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if w.check() {
				failures = 0
				continue
			}
			failures++
			if failures >= w.failureLimit {
				if w.logger != nil {
					fmt.Fprintln(w.logger, errcodes.Format(errcodes.MOUNT_SSHFS_DISCONNECTED))
				}
				if w.onDisconnect != nil {
					_ = w.onDisconnect()
				}
				return
			}
		}
	}
}
```

**应用到 reconnect.go**：

- `Reconnector` struct 与 `SSHFSWatcher` 同样为「conn + interval + 失败计数 + 回调」组合，把 `failureLimit` 改成 `backoffSeq[]time.Duration`，`check()` 改成 `sshConnect()` 真正去重连
- `Run(ctx)` 主循环必须用 `select{ ctx.Done | timer.C | triggerCh }` 三路；`triggerCh` 由 `Trigger()` 写入（size=1 + drop，参考 RESEARCH §3.1 暗示）
- 记录 `disconnectStart atomic.Int64` 用于三态 UX 渲染（D-22）
- `renderStatus` 是另一个 100ms ticker goroutine，**不持有** state machine 数据，只读 `disconnectStart`；输出走 `\r\x1b[K<text>` 行内覆盖（RESEARCH §3.4）
- `exceededFastRetryBudget` 用 60s 滑动窗口 + 5 次计数（CONTEXT D-05 第 5 条）

**渲染必须复用 colors.go 的 colorize + colorEnabled**（与 mount_strategy.go printBanner 一致策略，详见下方 colors.go 改造段）：

```382:393:internal/cloudclaude/mount_strategy.go
func printBanner(w io.Writer, mode Mode, noColor bool) {
	enabled := false
	if fh, ok := w.(fdHolder); ok {
		enabled = colorEnabled(noColor, fh)
	}
	color := ansiYellow
	if mode == ModeFull {
		color = ansiGreen
	}
	text := fmt.Sprintf("✓ 文件映射就绪 [%s]", mode.String())
	fmt.Fprintln(w, colorize(text, color, enabled))
}
```

依赖决策：D-05 / D-22 / D-23 + RESEARCH §3.1-3.4。

---

### `internal/cloudclaude/keepalive.go` — service / event-driven

**Analog:** `internal/cloudclaude/sshfs_watcher.go`（同 reconnect.go）+ `internal/cloudclaude/oauth_check.go::CheckOAuthCredentials`（用 `conn.NewSession()` + `CombinedOutput` 短命令的 boilerplate）

**SSH SendRequest 在 stalled network 上无限阻塞**（RESEARCH §1.2 [ASSUMED]）— 因此**必须**走 `goroutine + select <-time.After` 包 timeout，不能直接调 `conn.SendRequest`：

```go
// 推荐实现（来自 RESEARCH §1.2）：
func sendKeepaliveWithTimeout(conn ssh.Conn, timeout time.Duration) (bool, error) {
    ch := make(chan struct{ ok bool; err error }, 1)
    go func() {
        _, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
        ch <- struct{ ok bool; err error }{ok: err == nil, err: err}
    }()
    select {
    case <-time.After(timeout):
        return false, errors.New("keepalive timeout")
    case r := <-ch:
        return r.ok, r.err
    }
}
```

**Run 主循环骨架与 sshfs_watcher 完全同构**（同样 ticker + 失败计数 + return on threshold）— 参考 sshfs_watcher.go:51-76 模板。

**与 ssh.go 的接入点**：在 `sshConnect` 返回 `*ssh.Client` 后，由 `ConnectAndRunClaudeV3` 启动 `go RunKeepAlive(ctx, conn, mountCfg.KeepAliveInterval, mountCfg.KeepAliveCountMax)` — 与 `sshfs_watcher` 在 `mount_strategy.go::tryModeReal` 内 `go watcher.Run(ctx)` 同模式：

```346:351:internal/cloudclaude/mount_strategy.go
	// 启动 sshfs_watcher：cold 抖动 → 摘除 cold branch
	ctx, cancel := context.WithCancel(context.Background())
	watcher := NewSSHFSWatcher(connA, "/workspace-cold", cfg.Logger, func() error {
		return RemoveBranch(connA, "/workspace-cold", "/workspace")
	})
	go watcher.Run(ctx)
```

依赖决策：D-03 + RESEARCH §1.1-1.3。

---

### `internal/cloudclaude/keepalive_linux.go` / `keepalive_darwin.go` / `keepalive_other.go` — utility / 平台分发

**Analog:** 没有完全对应的 build-tag 文件分发先例（codebase 仅 `integration_test.go:1` 有 `//go:build integration` 标签）；最接近的语义模式是 `internal/cloudclaude/mutagen_bin.go::extractMutagenFor` 的 runtime switch + 平台 unsupported fallthrough：

```40:44:internal/cloudclaude/mutagen_bin.go
	switch plat {
	case "darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64":
	default:
		return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "unsupported platform "+plat))
	}
```

**应用到本阶段**：

- `keepalive.go`（公共部分）暴露 `ConfigureTCPKeepAlive(tcpConn *net.TCPConn, period time.Duration) error`，内部调 `tcpConn.SetKeepAlive(true)` + `tcpConn.SetKeepAlivePeriod(period)` + `configurePlatformSpecific(tcpConn)`（包内未导出函数，由三个 build-tag 文件分别提供实现）
- `keepalive_linux.go` 加 `//go:build linux`，`configurePlatformSpecific` 内 `setsockopt(IPPROTO_TCP, 18 /* TCP_USER_TIMEOUT */, 30000)`
- `keepalive_darwin.go` 加 `//go:build darwin`，`configurePlatformSpecific` 内 `setsockopt(IPPROTO_TCP, 0x10 /* TCP_KEEPALIVE */, 15)`（实际是 noop 占位 — Go stdlib 已设过；保留 hook 方便日后加 `TCP_KEEPCNT`）
- `keepalive_other.go` 加 `//go:build !linux && !darwin`，`configurePlatformSpecific` 直接 `fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, runtime.GOOS))` 后 `return nil`（best-effort）
- 失败仅 warning 不阻塞（CONTEXT D-04 第 4 条）

**setsockopt 的 syscall 模板**（参考 RESEARCH §2.2 / §2.3）：

```go
rawConn, err := tcpConn.SyscallConn()
if err != nil { return err }
var sockErr error
err = rawConn.Control(func(fd uintptr) {
    sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, 30000)
})
if err != nil { return err }
return sockErr
```

依赖决策：D-04 + RESEARCH §2.1-2.5 + RESEARCH §Patterns to Reuse P-05。

---

### `internal/cloudclaude/input_buffer.go` — utility / streaming

**Analog:** `internal/cloudclaude/mount_sshfs.go::mountSSHFS`（StdinPipe + io 包装）+ `internal/cloudclaude/colors.go`（ANSI 灰色包装）+ `internal/cloudclaude/mount.go::channelRWC`（io.Reader + io.WriteCloser 适配器结构）

**StdinPipe + 包装为中间 io 层**（mount_sshfs.go 借助 SSH session 的 StdinPipe 把外部 SFTP 协议包成可写 channel）：

```30:46:internal/cloudclaude/mount_sshfs.go
	sshfsSession, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "创建 sshfs session 失败"), err)
	}

	stdin, err := sshfsSession.StdinPipe()
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "获取 sshfs stdin pipe 失败"), err)
	}

	stdout, err := sshfsSession.StdoutPipe()
	if err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "获取 sshfs stdout pipe 失败"), err)
	}
```

**channelRWC 适配器**（与本阶段 `BufferedStdin` 同样是 io 适配器思路）：

```39:46:internal/cloudclaude/mount.go
type channelRWC struct {
	io.Reader
	io.WriteCloser
}

func (c *channelRWC) Close() error {
	return c.WriteCloser.Close()
}
```

**ANSI 灰色包装**（input_buffer 在 Reconnecting 状态进入时打 `\x1b[90m`、退出时打 `\x1b[0m`，复用 colors.go enabled 判定）：

```28:47:internal/cloudclaude/colors.go
func colorEnabled(noColor bool, w fdHolder) bool {
	if noColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if w == nil {
		return false
	}
	return term.IsTerminal(int(w.Fd()))
}

// colorize 包装文本为 ANSI 着色。enabled=false 时返回原文。
func colorize(s, ansi string, enabled bool) string {
	if !enabled {
		return s
	}
	return ansi + s + ansiReset
}
```

**应用到 input_buffer.go**：

- `BufferedStdin{ src io.Reader, pipeW io.WriteCloser, ringBuf []byte, ringMu sync.Mutex, state atomic.Int32, localEcho io.Writer, noColor bool, onEnter func() }`
- `NewBufferedStdin` 用 `io.Pipe()` 拿到 `pipeR, pipeW`；`pipeR` 给 SSH `session.Stdin = pipeR` 用，与 mount_sshfs 把 channelRWC 给 sftp.NewServer 用同构
- `Run(ctx)` 阻塞读 `src`，按 `state` 分发：
  - `Connected` → 直写 `pipeW`（与 mount_sshfs 把 SFTP 字节透传同构）
  - `Reconnecting` → append ringBuf + 灰色 echo `localEcho` + 检测 `\r`/`\n` 触发 `onEnter()`
  - `GaveUp` → 丢弃
- 进入 `Reconnecting` 时一次性 echo `\x1b[90m`（前缀），退出时 `\x1b[0m`，**不**逐字节包裹（避免中文 cursor 跳；RESEARCH §4.3）
- ringBuf 满（4KB / 8KB 由 planner 拍板，CONTEXT Discretion + RESEARCH §4.5）→ 丢最早 1KB + `errcodes.Format(SESSION_BUFFER_OVERFLOW)` warning
- 非 TTY 模式：在 `NewBufferedStdin` 之前 `term.IsTerminal(fd)` 检查，false 则直接把 `os.Stdin` 给 SSH session.Stdin（不启用 buffer，CONTEXT D-06 第 5 条）

依赖决策：D-06 + RESEARCH §4.1-4.6。

---

### `internal/cloudclaude/sync_lock.go` — service / request-response

**Analog:** `internal/cloudclaude/oauth_check.go::CheckOAuthCredentials`（exit-code → 状态枚举）+ `internal/cloudclaude/mount_sshfs.go`（shellescape + 远程命令拼接）+ `internal/cloudclaude/mount_mutagen.go::defaultMutagenDeps.remoteRun`（`sess.CombinedOutput` 读 stdout）

**sess.CombinedOutput 模板**：

```116:124:internal/cloudclaude/mount_mutagen.go
		remoteRun: func(conn *ssh.Client, cmd string) (string, error) {
			sess, err := conn.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		},
```

**ExitError 解包模式**（与 ssh.go runClaude 末尾一致）：

```230:238:internal/cloudclaude/ssh.go
	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		if err == io.EOF {
			return 0, nil
		}
		return 0, fmt.Errorf("SSH 会话异常结束: %w", err)
	}
```

**应用到 sync_lock.go**（**重要：RESEARCH §6.2 修订 D-17 — lock 路径从 `/var/lock/cloud-claude/` 改到 `/tmp/cloud-claude/locks/`，原因是 ubuntu:24.04 容器内 UID 1000 对 `/var/lock` 无写权限**）：

```go
func AcquireSyncLock(conn *ssh.Client, accountID string) (release func(), err error) {
    if accountID == "" { // CONTEXT D-19 — anon 路径跳过锁
        return func() {}, nil
    }
    lockPath := fmt.Sprintf("/tmp/cloud-claude/locks/sync-%s.lock", accountID)
    cmd := fmt.Sprintf(
        "mkdir -p /tmp/cloud-claude/locks 2>/dev/null && "+
        "flock -n -E 99 -F %s -c 'echo $$; exec sleep infinity' &\necho $!",
        shellescape.Quote(lockPath),
    )
    sess, err := conn.NewSession()
    if err != nil { return nil, err }
    out, runErr := sess.CombinedOutput(cmd)
    sess.Close()
    if runErr != nil {
        if exitErr, ok := runErr.(*ssh.ExitError); ok && exitErr.ExitStatus() == 99 {
            return nil, ErrSyncLocked  // → 调用方降级 sshfs-only
        }
        return nil, fmt.Errorf("flock 启动失败: %w (output: %s)", runErr, out)
    }
    pid := parseLastInt(out)
    release = func() {
        killSess, e := conn.NewSession()
        if e != nil { return }
        defer killSess.Close()
        _ = killSess.Run(fmt.Sprintf("kill %d 2>/dev/null || true", pid))
    }
    return release, nil
}

var ErrSyncLocked = errors.New("sync session locked by another cloud-claude")
```

**flock -F 必需**（RESEARCH §6.1）：`-F` = no fork，让 `sleep infinity` 直接持有 lock fd；缺 `-F` 则 SSH session 关闭时 sleep 死了但 flock 进程仍持有 fd，锁不释放。

**注入 MountConfig 的位置（RESEARCH §6.4 修订 D-18）**：Phase 31 `MountConfig.SyncSessionLock` 接口签名是 `func(accountID string) (release func(), err error)` — 没有 conn 参数。本阶段在 `ssh.go::ConnectAndRunClaudeV3` 内部 connA 建立后**覆盖**该字段（不改 Phase 31 接口）：

```go
mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
    return AcquireSyncLock(connA, accountID)
}
```

依赖决策：D-17 / D-18 / D-19 + RESEARCH §6.1-6.4。

---

### `internal/cloudclaude/session.go` — service / request-response

**Analog:** `internal/cloudclaude/mount_strategy.go::tryModeReal`（多步 fallback chain + cleanup LIFO）+ `internal/cloudclaude/oauth_check.go::CheckOAuthCredentials`（远端探测命令 + 解析 + 状态枚举返回）

**远端探测模板（DetectTmux）**：直接复刻 `oauth_check.go` 的 `conn.NewSession() + sess.Run + 收敛错误到 NotFound` 思路：

```44:63:internal/cloudclaude/oauth_check.go
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error) {
	_ = claudeAccountID
	if connA == nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	sess, err := connA.NewSession()
	if err != nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = nil

	cmd := "timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null"
	_ = sess.Run(cmd)

	return parseExpiresAt(stdout.String(), time.Now()), nil
}
```

**应用到 session.go DetectTmux**：

```go
func DetectTmux(conn *ssh.Client) (available bool, version string, errReason string) {
    if conn == nil { return false, "", "no connection" }
    sess, err := conn.NewSession()
    if err != nil { return false, "", err.Error() }
    defer sess.Close()
    var buf bytes.Buffer
    sess.Stdout = &buf; sess.Stderr = &buf
    runErr := sess.Run("command -v tmux >/dev/null 2>&1 && tmux -V 2>&1")
    if runErr != nil { return false, "", strings.TrimSpace(buf.String()) }
    return true, strings.TrimSpace(buf.String()), ""
}
```

**SessionConfig 结构**（CONTEXT D-29，由 main.go 构造、ConnectAndRunClaudeV3 透传）：

```go
type SessionConfig struct {
    AccountID         string
    ShortID           string
    TakeOver          bool
    TmuxAvailable     bool
    KeepAliveInterval time.Duration
    KeepAliveCountMax int
    ReconnectEnabled  bool
}
```

**远程命令构造（D-10 tmux 包装模板）** — 复用 ssh.go runClaude 的 shellescape 模式：

```216:224:internal/cloudclaude/ssh.go
	claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
	var remoteCmd string
	if hasProxy {
		binDir := remoteCwd + "/.cloud-claude/bin"
		remoteCmd = fmt.Sprintf("export PATH=%s:$PATH && cd %s && %s",
			shellescape.Quote(binDir), shellescape.Quote(remoteCwd), claudeCmd)
	} else {
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(remoteCwd), claudeCmd)
	}
```

**改造为 D-10 tmux 包装**（在 session.go 提供 `buildTmuxRemoteCmd(sessionName, wrapCmd, fallbackCmd) string`）：

```go
remoteCmd := fmt.Sprintf(
    "cd %s && command -v tmux >/dev/null 2>&1 && "+
    "exec tmux new-session -A -d -s %s %s \\; attach-session -t %s "+
    "|| exec %s",
    shellescape.Quote(remoteCwd),
    shellescape.Quote(sessionName), shellescape.Quote(wrapCmd),
    shellescape.Quote(sessionName),
    fallbackCmd,
)
```

**Session 命名（D-07 / D-08 / D-09）** — 与 mount_strategy.go::buildSessionName 拓扑一致但前缀改 `claude-` 而非 `cloud-claude-`：

```435:459:internal/cloudclaude/mount_strategy.go
func buildSessionName(accountID, cwd string) string {
	owner := accountID
	if owner == "" {
		owner = "anon"
	}
	h := simpleHash8(cwd)
	return fmt.Sprintf("cloud-claude-%s-%s", owner, h)
}

// simpleHash8 返回 cwd 的 8 字节 fnv64a hex 摘要（不要求加密强度）。
func simpleHash8(s string) string {
	const (
		offset64 = uint64(14695981039346656037)
		prime64  = uint64(1099511628211)
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return fmt.Sprintf("%08x", uint32(h))
}
```

**应用到 session.go**：

- `buildTmuxSessionName(accountID string) string` → `claude-<account_id_short8>`（accountID 前 8 字符）
- `buildAnonTmuxSessionName(cwd string) string` → `claude-anon-<simpleHash8(cwd)>`（直接复用 `simpleHash8`）
- `buildShortIDSessionName() string`（`--new-session` 用）→ `claude-<base64url(crypto/rand:6)>`（8 字符 base64url）
- 命名长度 ≤ 32，非法字符 `[^a-zA-Z0-9_-]` 替换为 `_` + warning（D-09）

**第二端 banner 数据源（RESEARCH §5.2 修订 D-12）** — tmux 没有 per-client 自定义名 API；改用文件注册表 `/workspace/.cloud-claude/clients/<tmux_client_pid>.json`：

```bash
# attach 时（cloud-claude → SSH conn-A 写入）
mkdir -p /workspace/.cloud-claude/clients && \
echo '{"hostname":"<local>","attach_at":<unix>,"claude_account_id":"<id>","tmux_pid":<pid>}' \
  > /workspace/.cloud-claude/clients/<tmux_client_pid>.json

# banner 渲染时（attach 之前先做）
tmux list-clients -t <session> -F '#{client_pid}|#{client_activity}|#{client_tty}'
# 然后对每个 pid 读 /workspace/.cloud-claude/clients/<pid>.json，缺失时 hostname=<unknown>

# detach / cloud-claude 退出时
rm -f /workspace/.cloud-claude/clients/<tmux_client_pid>.json
```

**注意**：`/workspace` 是 UID 1000 可写目录；不要用环境变量注入 client_name（CONTEXT D-12 设想方案不可行 — RESEARCH §5.2 [VERIFIED]）。

**fallback 路径（D-15 / D-16 tmux 不可用降级）** — 与 mount_strategy.go::MountWorkspace 的「降级不阻塞」逻辑同构：探测失败仅打 banner、走原 `runClaude`，不退出非 0：

```go
available, version, reason := DetectTmux(connA)
sessionCfg.TmuxAvailable = available
if !available {
    fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, reason))
    return runClaude(connA, claudeArgs, cwd, hasProxy)  // v2.0 路径
}
return runClaudeWithSession(ctx, connA, cfg, claudeArgs, cwd, hasProxy, sessionCfg)
```

**`sessions ls / attach` 子命令**（CONTEXT D-13 / D-14） — session.go 提供 `RunSessionsLs(conn) error` / `RunSessionsAttach(conn, name) (int, error)` 两个纯 helper，由 `cmd/cloud-claude/sessions.go` 调用（cobra 路由层不直接做 SSH 业务）。

依赖决策：D-07 / D-08 / D-09 / D-10 / D-11 / D-12 / D-13 / D-14 / D-15 / D-16 / D-29 + RESEARCH §5.1-5.5（特别是 §5.2 文件注册表修订）。

---

### `internal/cloudclaude/errcodes/session.go` — config / static

**Analog:** `internal/cloudclaude/errcodes/mount.go` — **完全 mirror，逐字符对齐**

```1:13:internal/cloudclaude/errcodes/mount.go
package errcodes

// MOUNT_* 错误码注册。文案与 Phase 31 PLAN.md <errcode_registry> 表逐字符对齐。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_VERSION_SKEW,
		Severity:   SeverityError,
		Message:    "Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only",
		NextAction: "升级容器镜像到 v3.0.0+ 或重装 cloud-claude",
	})
```

**应用到 errcodes/session.go**：

```go
package errcodes

// SESSION_* 错误码注册（Phase 32）。文案与 Phase 32 PLAN.md / RESEARCH §8 表对齐。
//nolint:lll

func init() {
    MustRegister(Entry{
        Code: SESSION_KEEPALIVE_TOO_AGGRESSIVE, Severity: SeverityFatal,
        Message:    "SSH KeepAlive 间隔 %s 低于 15s 下限",
        NextAction: "调整 keepalive_interval 至 >= 15s，或移除该配置使用默认值",
    })
    // ... 其余 6 条 SESSION_* — 见 RESEARCH §8 完整 Message / NextAction
}
```

**7 条 SESSION_* 全文 Message + NextAction 见 RESEARCH §8 行 1056-1092**（planner 直接复制粘贴，禁止自创文案）。

**严格约束**（来自 errcodes/codes.go 的 MustRegister panic + codes_test.go 的 NextAction ≤ 80 runes 检查）：

```43:46:internal/cloudclaude/errcodes/codes_test.go
		if n := utf8.RuneCountInString(e.NextAction); n > 80 {
			t.Errorf("code %q NextAction 长度 %d > 80 runes: %q", code, n, e.NextAction)
		}
```

依赖决策：D-20 / D-21 + RESEARCH §8 + RESEARCH §Patterns to Reuse P-01。

---

### `internal/cloudclaude/errcodes/net.go` — config / static（**改造**：追加 3 条 NET_*）

**Analog:** 自身现有 3 条 NET_OAUTH_* — 直接在文件末尾追加 3 条新 init register 即可。

现有内容：

```5:27:internal/cloudclaude/errcodes/net.go
func init() {
	MustRegister(Entry{
		Code:       NET_OAUTH_EXPIRED,
		Severity:   SeverityFatal,
		Message:    "Claude OAuth 凭证已过期（账号: %s）",
		NextAction: "在容器内运行 cloud-claude exec claude login 重新登录",
	})
    // ... NET_OAUTH_EXPIRING_SOON / NET_OAUTH_NOT_FOUND
}
```

**应用方式**：在文件末尾**新增第二个 `func init()`**（Go 允许同包同文件多个 init），追加：

```go
// 同文件追加（与现有 init 并列；避免在 NET_OAUTH_* 注册块中插入混淆）
func init() {
    MustRegister(Entry{
        Code: NET_RECONNECT_BACKOFF, Severity: SeverityInfo,
        Message:    "网络中断，正在重连（已等待 %s）",
        NextAction: "按 Enter 立即重试，或等待自动重连",
    })
    MustRegister(Entry{
        Code: NET_RECONNECT_GAVE_UP, Severity: SeverityFatal,
        Message:    "重连失败（已重试 %d 次，耗时 %s）",
        NextAction: "请检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor 诊断",
    })
    MustRegister(Entry{
        Code: NET_TCP_KEEPALIVE_UNSUPPORTED, Severity: SeverityWarn,
        Message:    "TCP keepalive 平台特化失败：%s",
        NextAction: "无需操作；SSH 应用层 keepalive 仍生效，弱网检测可能略慢",
    })
}
```

依赖决策：D-20 + RESEARCH §8 行 1095-1111。

---

### `internal/cloudclaude/errcodes/codes.go` — config / static（**改造**：追加 10 条常量）

**Analog:** 自身现有 const 块 line 119-136。

```119:136:internal/cloudclaude/errcodes/codes.go
const (
	MOUNT_MUTAGEN_VERSION_SKEW       Code = "MOUNT_MUTAGEN_VERSION_SKEW"
	MOUNT_MUTAGEN_WHITELIST_REJECT   Code = "MOUNT_MUTAGEN_WHITELIST_REJECT"
	// ... 其它 13 条
	NET_OAUTH_EXPIRED                Code = "NET_OAUTH_EXPIRED"
	NET_OAUTH_EXPIRING_SOON          Code = "NET_OAUTH_EXPIRING_SOON"
	NET_OAUTH_NOT_FOUND              Code = "NET_OAUTH_NOT_FOUND"
)
```

**改造方式**：在 `NET_OAUTH_NOT_FOUND` 后**追加 10 条**（保持对齐宽度，每行变量名与字面值完全相同）— 完整列表见 RESEARCH §8 行 1117-1130。命名正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`（codes.go:56 + codes_test.go:20）允许 4 段命名（最长 `SESSION_KEEPALIVE_TOO_AGGRESSIVE`）。

依赖决策：D-20 + RESEARCH §8。

---

### `cmd/cloud-claude/sessions.go` — cli / request-response

**Analog:** `cmd/cloud-claude/sync.go` — **整体结构 mirror（newSyncCmd → newSessionsCmd）**，但 sessions 子命令需要 SSH 连接（sync conflicts 是纯本地）

**newSyncCmd 模板**：

```18:37:cmd/cloud-claude/sync.go
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "Mutagen 同步管理（v3.0 三层文件映射）",
		Long:          "查看本地 Mutagen 客户端管理的 cloud-claude 同步会话与冲突文件清单。\n注：当前仅实现 sync conflicts 子命令；sync resolve / sync resume 留 v3.1。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	conflictsCmd := &cobra.Command{
		Use:           "conflicts",
		Short:         "查看当前 Mutagen 同步会话的冲突文件清单",
		Long:          "调用本地 Mutagen 客户端 sync list --long 渲染所有 cloud-claude 创建的 sync session 的冲突文件（path / alpha / beta / mtime）。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSyncConflicts,
	}
	cmd.AddCommand(conflictsCmd)
	return cmd
}
```

**SSH 连接模板**（runEnvCheck / runSSHDoctor 在 main.go:163-242 已建立）：

```163:201:cmd/cloud-claude/main.go
func runEnvCheck(cmd *cobra.Command, args []string) error {
	cfg, err := cloudclaude.LoadConfig()
	if err != nil { return err }

	client := cloudclaude.NewEntryClient(cfg.Gateway)
	fmt.Println("正在连接云主机...")
	authResp, err := client.AuthenticateAndWait(
		cmd.Context(), cfg.ShortID, cfg.Password,
		func(msg string) { fmt.Printf("\r%s", msg) },
	)
	if err != nil { return fmt.Errorf("认证失败: %w", err) }

	fmt.Println("\r正在检测远端环境...")
	sshCfg := cloudclaude.SSHConfig{
		Host: authResp.SSHHost, Port: authResp.SSHPort,
		User: authResp.SSHUser, Password: authResp.SSHPass,
	}
	result, err := cloudclaude.RunEnvCheck(sshCfg)
    // ...
```

**应用到 sessions.go**：

```go
func newSessionsCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use: "sessions", Short: "tmux 会话管理（v3.0 SSH 会话可靠性）",
        Long: "查看 / attach 容器内由 cloud-claude 创建的 tmux 会话。",
        SilenceUsage: true, SilenceErrors: true,
    }
    lsCmd := &cobra.Command{Use: "ls", Short: "列出当前 tmux 会话", RunE: runSessionsLs, SilenceUsage: true, SilenceErrors: true}
    attachCmd := &cobra.Command{Use: "attach <name>", Args: cobra.ExactArgs(1), Short: "attach 到指定 tmux 会话", RunE: runSessionsAttach, SilenceUsage: true, SilenceErrors: true}
    cmd.AddCommand(lsCmd, attachCmd)
    return cmd
}

func runSessionsLs(cmd *cobra.Command, args []string) error {
    // 复用 runEnvCheck 模板：LoadConfig → AuthenticateAndWait → sshConnect → cloudclaude.RunSessionsLs(conn)
    // 业务逻辑封装在 internal/cloudclaude/session.go::RunSessionsLs，cobra 层只做路由
}

func runSessionsAttach(cmd *cobra.Command, args []string) error {
    // 同样模板；调 cloudclaude.RunSessionsAttach(conn, args[0])
    // 内部远程：tmux has-session -t name → 失败返回 SESSION_NOT_FOUND
    //           tmux attach-session -t name → 复用 runClaude PTY 逻辑（D-14）
}
```

**main.go 注册**（同 line 92 `newSyncCmd()` 模式）：`rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())` + 在 line 98 的 switch 追加 `"sessions"` 关键字。

依赖决策：D-13 / D-14 + RESEARCH §5.4 + RESEARCH §Patterns to Reuse P-03。

---

### `internal/cloudclaude/ssh.go` — 改造（在 sshConnect / ConnectAndRunClaudeV3 内插入）

**Analog:** 自身

#### 改造点 A：sshConnect 叠加 KeepAlive（line 144-166 之间）

现状 line 155-165：

```155:165:internal/cloudclaude/ssh.go
	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
```

**改造为**（在 `tcpConn` 创建后、`ssh.NewClientConn` 之前插入 TCP KeepAlive；不阻塞失败）：

```go
tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
if err != nil { return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err) }

// [新增 — D-04] TCP keepalive — best-effort，失败仅 warning
if tc, ok := tcpConn.(*net.TCPConn); ok {
    if e := ConfigureTCPKeepAlive(tc, 15*time.Second); e != nil {
        fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, e.Error()))
    }
}

sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
// ...（保持不变）
```

注意：sshConnect **不**直接启动 SSH SendRequest 心跳 goroutine（避免与重连状态机重复管理 ctx）；心跳 goroutine 由 `ConnectAndRunClaudeV3` 在拿到 connA 后启动。

#### 改造点 B：ConnectAndRunClaudeV3 在 OAuth 检查后插入 tmux 探测 + runClaudeWithSession（line 139-141）

现状 line 139-141：

```139:141:internal/cloudclaude/ssh.go
		}
	}

	return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
```

**改造为**（D-15 探测 + D-29 fork runClaudeWithSession，**保留 runClaude 原函数不动**用于 fallback / sessions attach 复用）：

```go
// [新增 — D-15] tmux 可用性探测
sessionCfg := SessionConfig{
    AccountID:         mountCfg.ClaudeAccountID,
    ShortID:           "" /* 由 main.go 通过 mountCfg 透传或新增字段 */,
    TakeOver:          false /* 同上 */,
    KeepAliveInterval: mountCfg.KeepAliveInterval,
    KeepAliveCountMax: mountCfg.KeepAliveCountMax,
    ReconnectEnabled:  true,
}
available, _, reason := DetectTmux(connA)
sessionCfg.TmuxAvailable = available
if !available {
    fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, reason))
    return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)  // v2.0 路径
}
return runClaudeWithSession(cmd.Context(), connA, cfg, claudeArgs, cwd, len(proxyCommands) > 0, sessionCfg)
```

**注意**：`SessionConfig` 字段（`ShortID` / `TakeOver`）需要从 main.go 透传 — 推荐**新增 `MountConfig` 字段**或**新增 `ConnectAndRunClaudeV3` 参数**之一；planner 决定具体方式（CONTEXT Discretion）。

#### 改造点 C：在 ConnectAndRunClaudeV3 内覆盖 SyncSessionLock（RESEARCH §6.4）

**位置**：在 line 85-89（`if mountCfg.SyncSessionLock == nil { ...noop... }`）之后**强制覆盖**为真实实现：

```go
// [新增 — D-18 / RESEARCH §6.4] 用真实 flock 包装替换 noop 默认
mountCfg.SyncSessionLock = func(accountID string) (func(), error) {
    return AcquireSyncLock(connA, accountID)
}
```

依赖决策：D-03 / D-04 / D-15 / D-18 / D-28 / D-29 + RESEARCH §1.3 / §2.5 / §5.5 / §6.4。

---

### `internal/cloudclaude/last_session.go` — 改造（追加 omitempty 字段）

**Analog:** 自身（line 15-25 现有 struct）

```15:25:internal/cloudclaude/last_session.go
type LastSessionSnapshot struct {
	SchemaVersion       int             `json:"schema_version"`
	Timestamp           time.Time       `json:"timestamp"`
	IntendedMode        string          `json:"intended_mode"`
	ActualMode          string          `json:"actual_mode"`
	DowngradeChain      []DowngradeStep `json:"downgrade_chain"`
	ConflictCount       int             `json:"conflict_count"`
	ClaudeAccountID     string          `json:"claude_account_id,omitempty"`
	ImageVersion        string          `json:"image_version,omitempty"`
	APFSCaseInsensitive bool            `json:"apfs_case_insensitive"`
}
```

**改造为**（在 `APFSCaseInsensitive` 后追加 3 个字段，全部 omitempty，schema_version 保持 1）：

```go
type LastSessionSnapshot struct {
    // ... 既有 9 个字段不变

    // [Phase 32 D-27 新增] 全部 omitempty + schema_version 保持 1
    TmuxSession    string `json:"tmux_session,omitempty"`
    ClientRole     string `json:"client_role,omitempty"`     // "primary" | "secondary"
    ReconnectCount int    `json:"reconnect_count,omitempty"`
}
```

**写入时机**：`runClaudeWithSession` 完成 attach 后写一次（含 `TmuxSession` / `ClientRole`）；`Reconnector` 每次成功重连 +1 `ReconnectCount` + 重写一次。

依赖决策：D-27 + RESEARCH §9 + RESEARCH §Patterns to Reuse P-07。

---

### `cmd/cloud-claude/main.go` — 改造（注册 flag + 子命令 + KeepAlive 字段）

**Analog:** 自身（runRoot 内 `--mount-mode` 剥离段，line 244-269）

```253:275:cmd/cloud-claude/main.go
	// 因 DisableFlagParsing=true，cobra 不会自动解析 PersistentFlags；
	// 这里手工扫描 --mount-mode 并从 args 中剥离，剩余 args 透传给远端 claude。
	mountMode := "auto"
	filtered := args[:0]
	for i := 0; i < len(args); i++ {
		if args[i] == "--mount-mode" && i+1 < len(args) {
			mountMode = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--mount-mode=") {
			mountMode = strings.TrimPrefix(args[i], "--mount-mode=")
			continue
		}
		filtered = append(filtered, args[i])
	}
	args = filtered

	mode, err := cloudclaude.ParseMode(mountMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: --mount-mode 必须是 auto / full / mutagen-only / sshfs-only 之一")
		os.Exit(exitConfigError)
	}
```

**改造点**（4 处）：

#### A. AddCommand 注册 sessions（line 92）

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())  // 新增 newSessionsCmd()
```

#### B. DisableFlagParsing switch 追加 "sessions"（line 96-101）

```go
if len(os.Args) > 1 {
    switch os.Args[1] {
    case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":  // 新增 "sessions"
        rootCmd.DisableFlagParsing = false
    }
}
```

#### C. runRoot 内剥离 --new-session / --take-over（在现有 --mount-mode 剥离循环里追加 case，**注意 `--new-session` 是布尔无值 flag**）

参考现有循环结构，在 line 257-268 改造：

```go
mountMode := "auto"
newSession := false
takeOver := false
filtered := args[:0]
for i := 0; i < len(args); i++ {
    switch {
    case args[i] == "--mount-mode" && i+1 < len(args):
        mountMode = args[i+1]; i++; continue
    case strings.HasPrefix(args[i], "--mount-mode="):
        mountMode = strings.TrimPrefix(args[i], "--mount-mode="); continue
    case args[i] == "--new-session":
        newSession = true; continue
    case args[i] == "--take-over":
        takeOver = true; continue
    }
    filtered = append(filtered, args[i])
}
args = filtered
```

#### D. 启动期校验 KeepAlive < 15s（CONTEXT D-03 第 4 条）

在 `mountCfg` 构造（line 335-340）后立即校验：

```go
mountCfg := cloudclaude.MountConfig{
    Mode:              mode,
    KeepAliveInterval: 15 * time.Second,
    KeepAliveCountMax: 4,
    NoColor:           os.Getenv("NO_COLOR") != "",
}
// [新增 — D-03 / RESEARCH §1.3] 校验 keepalive_interval >= 15s
if mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr,
        errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
    os.Exit(exitConfigError)
}
```

**注意**：本阶段默认值就是 15s，校验在「未来用户通过环境变量 / config 自定义」场景才会触发（CONTEXT Discretion 允许 planner 决定是否在本阶段同时引入 env/config 入口）。

依赖决策：D-01 / D-03 + RESEARCH §1.3 + RESEARCH §Patterns to Reuse P-03 / P-04。

---

### `internal/cloudclaude/colors.go` — 改造（追加 ansiGray 常量）

**Analog:** 自身（line 9-16 现有常量块）

```9:16:internal/cloudclaude/colors.go
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)
```

**改造**：仅追加 `ansiGray = "\033[90m"`（`ansiRed` 已存在 line 12，**无需重复添加** — CONTEXT D-23 + RESEARCH §Patterns to Reuse P-09 提到的 "新增 ansiGray=90 / ansiRed=31" 中 ansiRed 已 ship，本阶段实际只新增 ansiGray 一行）：

```go
const (
    ansiReset  = "\033[0m"
    ansiRed    = "\033[31m"
    ansiGreen  = "\033[32m"
    ansiYellow = "\033[33m"
    ansiCyan   = "\033[36m"
    ansiGray   = "\033[90m"  // [新增 — D-23] reconnect 灰色 "..." / input_buffer 未确认字符
)
```

依赖决策：D-22 / D-23 + RESEARCH §Patterns to Reuse P-09。

---

## Shared Patterns

### SP-01 错误返回 — 统一 errcodes.Format

**Source:** `internal/cloudclaude/errcodes/codes.go` Format helper（line 99-117）

```99:117:internal/cloudclaude/errcodes/codes.go
// Format 渲染统一两段输出：
//
//	[<CODE>] <Message>
//	  建议: <NextAction>
//
// args 用于填充 Message 中的 %s/%d 占位。code 未注册时返回带 "(unknown code)" 的占位字符串，不 panic。
func Format(c Code, args ...any) string {
	registryMu.RLock()
	e, ok := registry[c]
	registryMu.RUnlock()
	if !ok {
		return fmt.Sprintf("[%s] (unknown code)\n  建议: 联系维护者", c)
	}
	msg := e.Message
	if len(args) > 0 {
		msg = fmt.Sprintf(e.Message, args...)
	}
	return fmt.Sprintf("[%s] %s\n  建议: %s", c, msg, e.NextAction)
}
```

**Apply to:** session.go / reconnect.go / sync_lock.go / keepalive.go / input_buffer.go 全部错误输出 — 禁止新建错误格式 helper、禁止直接 `fmt.Fprintf(os.Stderr, "[CODE] %s", ...)`（CONTEXT D-21 + RESEARCH §Patterns to Reuse P-02）。

错误包装走 `fmt.Errorf("...: %w", err)` + `errcodes.Format(code, args...)` 拼接（参考 mount_sshfs.go 的 27-28 行）：

```26:28:internal/cloudclaude/mount_sshfs.go
	if err := sshRun(conn, mkdirCmd); err != nil {
		return nil, fmt.Errorf("%s: %w", errcodes.Format(errcodes.MOUNT_SSHFS_FAILED, "创建远端挂载目录失败"), err)
	}
```

---

### SP-02 ctx 传递 — 全部用 context.Context 不裸 time.NewTimer

**Source:** `internal/cloudclaude/sshfs_watcher.go::Run`（line 51-76）+ `internal/cloudclaude/mount.go::waitForMount`（line 49-78）

**Apply to:** reconnect.Run / keepalive.RunKeepAlive / input_buffer.Run / Reconnector.renderStatus 全部 goroutine 必须接 ctx 第一参数；任何 timer / ticker 必须 select ctx.Done。

**禁止**：`time.Sleep(...)` 裸用、`time.NewTimer(...)` 不 select ctx。

---

### SP-03 SSH 远程命令构造 — shellescape

**Source:** `internal/cloudclaude/ssh.go::runClaude`（line 216-224）+ `internal/cloudclaude/mount_sshfs.go`

```216:224:internal/cloudclaude/ssh.go
	claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
	var remoteCmd string
	if hasProxy {
		binDir := remoteCwd + "/.cloud-claude/bin"
		remoteCmd = fmt.Sprintf("export PATH=%s:$PATH && cd %s && %s",
			shellescape.Quote(binDir), shellescape.Quote(remoteCwd), claudeCmd)
	} else {
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(remoteCwd), claudeCmd)
	}
```

**Apply to:** session.go（tmux new-session / list-clients / detach-client / attach-session）/ sync_lock.go（flock 命令）/ session.go（sessions ls/attach）— 全部远程命令拼接走 `shellescape.Quote` / `shellescape.QuoteCommand`，禁止手写 `'...'` / `"..."`。

注：`mount.go::shellQuote`（line 115-117）是 v2.0 遗留的简化实现，**新代码统一用 `al.essio.dev/pkg/shellescape`**（与 ssh.go / ssh_doctor.go / 各 mount_*.go 一致）。

---

### SP-04 sshRun / sshRunWithOutput helpers

**Source:** `internal/cloudclaude/mount.go::sshRun`（line 105-112）

```105:112:internal/cloudclaude/mount.go
func sshRun(conn *ssh.Client, cmd string) error {
	sess, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.Run(cmd)
}
```

**Apply to:** session.go / sync_lock.go 远程命令的简单调用直接用 `sshRun`；需要拿 stdout 时用 `sess.CombinedOutput`（参考 mount_mutagen.go::defaultMutagenDeps.remoteRun，line 116-124）— **不要新建第 N 个 helper**。

---

### SP-05 cobra 子命令注册 + DisableFlagParsing 路由

**Source:** `cmd/cloud-claude/main.go`（line 92-101）+ `cmd/cloud-claude/sync.go`

```92:101:cmd/cloud-claude/main.go
	rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd())

	// DisableFlagParsing 会阻止 cobra 识别子命令，
	// 在检测到已知子命令时关闭它以恢复正常路由。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init", "env", "ssh", "sync", "help", "--help", "-h":
			rootCmd.DisableFlagParsing = false
		}
	}
```

**Apply to:** `cmd/cloud-claude/sessions.go::newSessionsCmd` 注册 + main.go switch 追加 `"sessions"` — 与 RESEARCH §Patterns to Reuse P-03 / P-04 一致。

---

### SP-06 errcodes 注册（包级 init + MustRegister）

**Source:** `internal/cloudclaude/errcodes/mount.go` + `internal/cloudclaude/errcodes/net.go` 的 init() pattern

**Apply to:** errcodes/session.go init() + errcodes/net.go 追加 init() — 严格 mirror 既有写法（`MustRegister(Entry{ Code, Severity, Message, NextAction })`）。

**约束**（来自 errcodes/codes.go MustRegister panic + codes_test.go 校验）：

- `Code` 必须匹配正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`
- `Message` / `NextAction` 不能为空
- `NextAction` ≤ 80 runes
- 不允许重复注册

---

### SP-07 LastSessionSnapshot 追加 omitempty 字段

**Source:** `internal/cloudclaude/last_session.go`（既有 9 字段全 `,omitempty` / 强制 schema_version=1）

**Apply to:** 本阶段 3 个新字段（`TmuxSession` / `ClientRole` / `ReconnectCount`）必须 `,omitempty`，**禁止**修改 `SchemaVersion` 常量（保持 1）— 保证向后兼容。

---

## No Analog Found

无。本阶段 13 个文件全部能找到至少 role-match 级别的同包 analog；唯一可能争议的是：

| 文件 | 形态 | 理由 |
|---|---|---|
| `internal/cloudclaude/keepalive_{linux,darwin,other}.go` | build-tag 文件分发 | codebase 暂无 `//go:build linux/darwin` 三件套先例（仅 `integration_test.go:1` 有 `//go:build integration`）；本阶段按 RESEARCH §2 + Go 标准 build constraint 创建。**最近的语义 analog** 是 `mutagen_bin.go::extractMutagenFor` 的 runtime.GOOS switch（已在 `keepalive_*` 段引用）。 |

---

## Metadata

**Analog search scope:** `internal/cloudclaude/` + `internal/cloudclaude/errcodes/` + `cmd/cloud-claude/`
**Files scanned:** 33（cloudclaude pkg）+ 4（errcodes pkg）+ 2（cmd pkg）= 39
**Pattern extraction date:** 2026-04-20
**Project conventions consulted:** `.cursor/skills/` GSD 命令系统 / `CLAUDE.md`（未直接影响 pattern 选择，沟通中文 / 错误码中文文案 / 零增量特权约束已在 RESEARCH 段反映）

---

## PATTERN MAPPING COMPLETE

**Phase:** 32 - SSH 会话可靠性 + tmux 包装 + 多端
**Files classified:** 13（8 新建 + 5 改造；不含 `colors.go` 仅 1 行常量追加）
**Analogs found:** 13 / 13

### Coverage
- Files with exact analog: 11
- Files with role-match analog: 4
- Files with no analog: 0

### Key Patterns Identified
- **errcodes 注册** 完全 mirror Phase 31 mount.go / net.go 的 `init() + MustRegister` 模板（SP-06）；session.go / net.go 的 7+3 条新码逐字符复刻 RESEARCH §8 的 Message/NextAction
- **ctx + ticker + 失败计数 + 阈值 return** 模式从 sshfs_watcher.go 直接复用到 reconnect.go 与 keepalive.go（结构 1:1）
- **远端 SSH 命令探测 → 解析 → 状态枚举返回** 模式从 oauth_check.go::CheckOAuthCredentials 复用到 session.go::DetectTmux 与 sync_lock.go::AcquireSyncLock（exit-code 99 → ErrSyncLocked 与 OAuthExpired 同构）
- **shellescape.Quote / QuoteCommand** 是全项目唯一允许的远程命令拼接方式（SP-03）；sync_lock.go 的 `flock -n -E 99 -F ... -c '...'` 必须走 shellescape，禁止手写引号
- **cobra 子命令注册** 从 sync.go::newSyncCmd 直接 mirror 到 sessions.go::newSessionsCmd；main.go 的 DisableFlagParsing switch 追加 `"sessions"` 关键字 + flag 剥离段追加 `--new-session` / `--take-over` 两个 case
- **build-tag 平台分发** codebase 首次引入；公共 `keepalive.go` + 三个 `keepalive_{linux,darwin,other}.go` 提供 `configurePlatformSpecific(*net.TCPConn) error` 的不同实现（参考 RESEARCH §2 setsockopt 模板）

### CONTEXT 修订点（已反映在 pattern 段）
- **D-12 修订**（RESEARCH §5.2）：tmux 没有 per-client 自定义名 API；session.go 的 banner 数据源改用 `/workspace/.cloud-claude/clients/<tmux_client_pid>.json` 文件注册表，**不再注入 `CLOUD_CLAUDE_CLIENT_NAME` 环境变量**
- **D-17 修订**（RESEARCH §6.2）：lock 路径从 `/var/lock/cloud-claude/` 改到 `/tmp/cloud-claude/locks/`，原因 ubuntu:24.04 `/var/lock` 是 root-only，UID 1000 写不进
- **D-18 澄清**（RESEARCH §6.4）：Phase 31 `MountConfig.SyncSessionLock` 接口签名缺 conn 参数；本阶段在 `ssh.go::ConnectAndRunClaudeV3` 内部 connA 建立后**覆盖**该字段（不改 Phase 31 接口）
- **SendRequest 必须包 timeout**（RESEARCH §1.2 / §3 修订 D-03）：`keepalive.go::sendKeepaliveWithTimeout` 必须 `goroutine + select <-time.After`，否则 dead network 上 SendRequest 永久阻塞导致失败计数永远不会触发

### File Created
`.planning/phases/32-ssh-tmux/32-PATTERNS.md`

### Ready for Planning
Pattern mapping complete. planner 可以基于本文件 + CONTEXT.md + RESEARCH.md 创建 PLAN.md。建议 plan 切分（与 RESEARCH §RESEARCH COMPLETE 一致）：

- **Plan 01 — 网络层韧性**：keepalive.go + keepalive_{linux,darwin,other}.go + reconnect.go + input_buffer.go + errcodes/session.go(部分) + errcodes/net.go(追加) + colors.go(ansiGray) + last_session.go(omitempty 字段) + ssh.go::sshConnect 改造
- **Plan 02 — 会话层 + 多端**：session.go（DetectTmux + tmux 包装 + take-over + sessions ls/attach helpers + 文件注册表） + cmd/cloud-claude/sessions.go + cmd/cloud-claude/main.go(注册 flag + sessions cmd) + ssh.go::ConnectAndRunClaudeV3 改造
- **Plan 03 — Mutagen 单例锁 + 集成测试**：sync_lock.go + ssh.go::ConnectAndRunClaudeV3 注入 SyncSessionLock + integration_test.go 双 cloud-claude / docker network disconnect 30s / pkill -SIGHUP sshd / pgrep systemd-logind 全部用例
