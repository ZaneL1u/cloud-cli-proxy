# Phase 27: 双 session 目录映射 - Pattern Map

**Mapped:** 2026-04-15
**Files analyzed:** 5 (2 new, 3 modified)
**Analogs found:** 5 / 5

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/cloudclaude/ssh.go` (modify) | service | request-response | 自身（当前实现） | exact |
| `internal/cloudclaude/mount.go` (new) | service | streaming / I/O pipe | `internal/sshproxy/proxy.go` + `internal/runtime/tasks/ssh_ready.go` | role-match |
| `cmd/cloud-claude/main.go` (modify) | controller | request-response | 自身（当前实现） | exact |
| `internal/cloudclaude/mount_test.go` (new) | test | — | `internal/runtime/tasks/ssh_ready_test.go` | exact |
| `go.mod` (modify) | config | — | 自身 | exact |

## Pattern Assignments

### `internal/cloudclaude/ssh.go` (service, request-response) — MODIFY

**Analog:** 自身（当前实现 `ConnectAndRunClaude`）

**重构方向：** 将现有单函数拆分为三个阶段函数，但保留现有的 import 风格、错误处理和资源管理模式。

**Imports pattern** (lines 1-15):
```go
package cloudclaude

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)
```

**SSH 连接建立 pattern** (lines 28-49) — 提取为独立 `sshConnect` 函数:
```go
clientCfg := &ssh.ClientConfig{
	User: cfg.User,
	Auth: []ssh.AuthMethod{
		ssh.Password(cfg.Password),
	},
	HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	Timeout:         10 * time.Second,
}

addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
if err != nil {
	return 0, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
}

sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
if err != nil {
	tcpConn.Close()
	return 0, fmt.Errorf("SSH 握手失败: %w", err)
}
conn := ssh.NewClient(sshConn, chans, reqs)
defer conn.Close()
```

**PTY + raw mode + SIGWINCH pattern** (lines 57-92) — 保留到 `runClaude` 阶段:
```go
fd := int(os.Stdin.Fd())
isTTY := term.IsTerminal(fd)

if isTTY {
	width, height := 80, 24
	if w, h, err := term.GetSize(fd); err == nil {
		width, height = w, h
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, fmt.Errorf("设置终端 raw 模式失败: %w", err)
	}
	defer term.Restore(fd, oldState)

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return 0, fmt.Errorf("申请 PTY 失败: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			if w, h, err := term.GetSize(fd); err == nil {
				_ = session.WindowChange(h, w)
			}
		}
	}()
	defer signal.Stop(sigCh)
}
```

**远程命令构建 pattern** (lines 98-99) — claude session 需改为 `cd /workspace && claude <args>`:
```go
remoteCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
```

**退出码处理 pattern** (lines 104-114):
```go
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

**错误信息风格：** 全部使用中文描述 + `%w` 包装：
```go
return 0, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
return 0, fmt.Errorf("SSH 握手失败: %w", err)
return 0, fmt.Errorf("创建 SSH 会话失败: %w", err)
```

---

### `internal/cloudclaude/mount.go` (service, streaming / I/O pipe) — NEW

**Analog 1 (I/O 管道 + goroutine 生命周期):** `internal/sshproxy/proxy.go` — `handleChannel` 函数

**双向 I/O 复制 + WaitGroup pattern** (proxy.go lines 286-303):
```go
var wg sync.WaitGroup
wg.Add(2)

go func() {
	defer wg.Done()
	io.Copy(targetChan, clientChan)
	targetChan.CloseWrite()
}()

go func() {
	defer wg.Done()
	io.Copy(clientChan, targetChan)
	clientChan.CloseWrite()
}()

go io.Copy(clientChan.Stderr(), targetChan.Stderr())

wg.Wait()
```

**关键启发：** `mount.go` 的 SFTP server goroutine 同样需要一个 `done` channel 来同步生命周期，类似 proxy.go 使用 `sync.WaitGroup` 管理 I/O goroutine。区别在于 SFTP server 是单个阻塞调用 `server.Serve()`，用 `chan error` 而非 `WaitGroup` 更合适。

**SSH channel 创建 pattern** (proxy.go lines 252-258):
```go
targetChan, targetReqs, err := targetClient.OpenChannel("session", nil)
if err != nil {
	s.logger.Error("open target session failed", "addr", targetAddr, "error", err)
	fmt.Fprintf(clientChan.Stderr(), "Failed to open session on container\r\n")
	return
}
defer targetChan.Close()
```

**Analog 2 (轮询等待 pattern):** `internal/runtime/tasks/ssh_ready.go` — `WaitForSSHReady` 函数

**轮询 pattern** (ssh_ready.go lines 43-84):
```go
func WaitForSSHReady(ctx context.Context, containerName string, cfg SSHReadyConfig) error {
	if cfg.Check == nil {
		cfg.Check = DockerExecSSHCheck
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	var lastErr error
	if err := cfg.Check(ctx, containerName); err == nil {
		return nil
	} else {
		lastErr = err
	}

	deadline := time.NewTimer(cfg.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return &SSHNotReadyError{
				Container: containerName,
				Timeout:   cfg.Timeout,
				LastErr:   lastErr,
			}
		case <-ticker.C:
			if err := cfg.Check(ctx, containerName); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
}
```

**关键启发：** `waitForMount` 应复用完全相同的轮询结构（`time.NewTimer` + `time.NewTicker` + `select`），但使用 SSH session exec 代替 docker exec 进行检测。注意 `ssh_ready.go` 支持 `context.Context` 取消——`waitForMount` 不一定需要 context（cloud-claude CLI 进程），但保持风格一致性可以考虑加上。

**Config struct + 默认值 pattern** (ssh_ready.go lines 24-34):
```go
type SSHReadyConfig struct {
	PollInterval time.Duration
	Timeout      time.Duration
	Check        func(ctx context.Context, containerName string) error
}

var DefaultSSHReadyConfig = SSHReadyConfig{
	PollInterval: 1 * time.Second,
	Timeout:      30 * time.Second,
	Check:        DockerExecSSHCheck,
}
```

**错误类型 pattern** (ssh_ready.go lines 10-18):
```go
type SSHNotReadyError struct {
	Container string
	Timeout   time.Duration
	LastErr   error
}

func (e *SSHNotReadyError) Error() string {
	return fmt.Sprintf("ssh not ready on container %s after %s: %v", e.Container, e.Timeout, e.LastErr)
}

func (e *SSHNotReadyError) Unwrap() error {
	return e.LastErr
}
```

**Analog 3 (cleanup 模式):** `internal/cloudclaude/ssh.go` 的 defer 链

**资源清理 pattern** (ssh.go lines 49-92):
```go
conn := ssh.NewClient(sshConn, chans, reqs)
defer conn.Close()

session, err := conn.NewSession()
// ...
defer session.Close()

// ...
oldState, err := term.MakeRaw(fd)
// ...
defer term.Restore(fd, oldState)

// ...
defer signal.Stop(sigCh)
```

**关键启发：** 项目统一使用 Go defer 管理资源释放。`mountWorkspace` 返回 `cleanup func()` 而非直接 defer，是因为调用方需要控制 mount cleanup 和 claude session 之间的顺序关系。

---

### `cmd/cloud-claude/main.go` (controller, request-response) — MODIFY

**Analog:** 自身

**进度提示 pattern** (main.go lines 128-158):
```go
fmt.Println("正在连接云主机...")

authResp, err := client.AuthenticateAndWait(
	cmd.Context(),
	cfg.ShortID,
	cfg.Password,
	func(msg string) {
		fmt.Printf("\r%s", msg)
	},
)
// ... error handling ...

fmt.Println("\r正在进入 Claude Code 会话...")
```

**关键启发：** mount 阶段可在 `ConnectAndRunClaude` 调用前后插入类似的用户提示，例如 `"正在映射工作目录..."` 和 `"工作目录映射完成"`。

**错误退出 pattern** (main.go lines 167-174):
```go
exitCode, err := cloudclaude.ConnectAndRunClaude(sshCfg, args)
if err != nil {
	fmt.Fprintln(os.Stderr, "错误: "+err.Error())
	os.Exit(exitInternalError)
}
if exitCode != 0 {
	os.Exit(exitCode)
}
```

---

### `internal/cloudclaude/mount_test.go` (test) — NEW

**Analog:** `internal/runtime/tasks/ssh_ready_test.go`

**测试结构 pattern** (ssh_ready_test.go lines 11-96):
```go
func TestWaitForSSHReady(t *testing.T) {
	t.Run("succeeds when check passes immediately", func(t *testing.T) {
		cfg := SSHReadyConfig{
			PollInterval: 10 * time.Millisecond,
			Timeout:      100 * time.Millisecond,
			Check: func(_ context.Context, _ string) error {
				return nil
			},
		}

		err := WaitForSSHReady(context.Background(), "test-container", cfg)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		var attempts int32
		cfg := SSHReadyConfig{
			PollInterval: 10 * time.Millisecond,
			Timeout:      500 * time.Millisecond,
			Check: func(_ context.Context, _ string) error {
				n := atomic.AddInt32(&attempts, 1)
				if n < 3 {
					return errors.New("connection refused")
				}
				return nil
			},
		}

		err := WaitForSSHReady(context.Background(), "test-container", cfg)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("returns SSHNotReadyError on timeout", func(t *testing.T) {
		// ...
		var sshErr *SSHNotReadyError
		if !errors.As(err, &sshErr) {
			t.Fatalf("expected SSHNotReadyError, got %T: %v", err, err)
		}
	})

	t.Run("returns error when context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		// ...
	})
}
```

**关键启发：**
- 使用 `t.Run` 子测试组织用例
- 使用 `atomic.Int32` 跟踪重试次数
- 注入 `Check` 函数实现模拟——`waitForMount` 的测试同样可以注入一个模拟 SSH session 的检测函数
- 使用极短的 interval/timeout（10ms/100ms）加速测试执行
- 断言使用 `t.Fatalf` / `t.Errorf`，不引入第三方 assertion 库
- 使用 `errors.As` 检查具体错误类型

---

### `go.mod` (config) — MODIFY

**Pattern:** 直接 `go get` 添加依赖，遵循现有 require block 的字母排序。

```bash
go get github.com/pkg/sftp@v1.13.10
```

---

## Shared Patterns

### 错误消息风格
**Source:** `internal/cloudclaude/ssh.go` 全文
**Apply to:** `mount.go` 所有新函数

项目约定所有面向用户的错误使用中文描述 + `%w` 包装：
```go
return fmt.Errorf("创建 sshfs session 失败: %w", err)
return fmt.Errorf("挂载 %s 超时（%v）", mountPath, timeout)
```

### SSH Session 生命周期
**Source:** `internal/cloudclaude/ssh.go` lines 51-55
**Apply to:** `mount.go` 的 sshfs session 和 mountpoint 检测 session

```go
session, err := conn.NewSession()
if err != nil {
	return 0, fmt.Errorf("创建 SSH 会话失败: %w", err)
}
defer session.Close()
```

### defer 资源释放
**Source:** `internal/cloudclaude/ssh.go` lines 49-92
**Apply to:** `ssh.go` 重构后的三阶段函数和 `mount.go`

按 LIFO 顺序声明 defer，确保正确的释放顺序：
```go
conn := ssh.NewClient(...)
defer conn.Close()                // 最后释放

cleanupMount, err := mountWorkspace(conn, cwd)
defer cleanupMount()              // 中间释放

exitCode, err := runClaude(conn, args)
// runClaude 内部 defer session.Close()  // 最先释放
```

### shellescape 命令构建
**Source:** `internal/cloudclaude/ssh.go` line 98
**Apply to:** `mount.go` 或 `ssh.go` 中构建 `cd /workspace && claude <args>` 命令

```go
remoteCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
```

### 包结构
**Source:** `internal/cloudclaude/` 目录
**Apply to:** `mount.go` 新文件

项目按职责拆分同一 package 的多个文件：
- `config.go` — 配置读写
- `entry.go` — Entry API 客户端
- `ssh.go` — SSH 连接和会话管理
- `mount.go` (new) — 目录映射和 SFTP server

所有文件共享 `package cloudclaude`，不引入子 package。

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| — | — | — | 所有新文件都在现有代码中找到了 role-match 或 exact-match 的模式参考 |

**补充说明：** `channelRWC` 适配器（将 `StdinPipe`/`StdoutPipe` 组合为 `io.ReadWriteCloser`）在项目中没有直接先例，但 `sshproxy/proxy.go` 的双向 `io.Copy` 展示了类似的 I/O 管道对接风格。`channelRWC` 的实现应参考 RESEARCH.md 中的代码示例。

---

## Metadata

**Analog search scope:** `internal/`, `cmd/`
**Files scanned:** 89 Go source files
**Pattern extraction date:** 2026-04-15
