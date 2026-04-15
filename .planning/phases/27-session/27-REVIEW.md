---
phase: 27
status: issues_found
findings: 6
severity_counts:
  critical: 0
  high: 1
  medium: 1
  low: 4
---

# Phase 27: Code Review Report

**Reviewed:** 2026-04-15
**Depth:** standard
**Files Reviewed:** 4
**Status:** issues_found

## Summary

本次审查覆盖了 Phase 27（双 session 目录映射）引入的全部源文件。核心变更包括：新增嵌入式 SFTP server 实现（`mount.go`）及其单元测试（`mount_test.go`），修改 SSH 会话管理（`ssh.go`）以集成挂载流程，以及修改 CLI 入口（`main.go`）传入工作目录。

整体代码质量良好：defer 清理链的 LIFO 顺序正确保证了资源释放顺序，`waitForMount` 的轮询逻辑清晰且可测试，错误路径上的手动资源释放也考虑周全。

主要关注点集中在两个安全相关问题和一个 goroutine 生命周期问题。

## Files Reviewed

- `internal/cloudclaude/mount.go` (new)
- `internal/cloudclaude/mount_test.go` (new)
- `internal/cloudclaude/ssh.go` (modified)
- `cmd/cloud-claude/main.go` (modified)

## High

### HI-01: `ssh.InsecureIgnoreHostKey()` 禁用了主机密钥验证

**File:** `internal/cloudclaude/ssh.go:49`
**Issue:** `HostKeyCallback: ssh.InsecureIgnoreHostKey()` 使 SSH 连接容易受到中间人攻击。攻击者可以拦截连接并窃取凭证或篡改流量。虽然容器是动态创建的、没有预置信任锚，但 gateway 的 `authResp` 已经包含了连接信息，可以同时下发容器主机指纹用于验证。

**Fix:** 让 gateway 在 `authResp` 中返回容器的 SSH host key 指纹，客户端用 `ssh.FixedHostKey()` 验证：

```go
hostKey, err := ssh.ParsePublicKey(authResp.HostKeyBytes)
if err != nil {
    return nil, fmt.Errorf("解析主机公钥失败: %w", err)
}
clientCfg := &ssh.ClientConfig{
    User:            cfg.User,
    Auth:            []ssh.AuthMethod{ssh.Password(cfg.Password)},
    HostKeyCallback: ssh.FixedHostKey(hostKey),
    Timeout:         10 * time.Second,
}
```

## Medium

### ME-01: 嵌入式 SFTP server 未做文件系统沙箱限制

**File:** `internal/cloudclaude/mount.go:70`
**Issue:** `sftp.NewServer(rwc, sftp.WithServerWorkingDirectory(localDir))` 仅设置 SFTP server 的初始工作目录，但 `pkg/sftp` 库不限制客户端通过绝对路径或 `../` 访问工作目录以外的文件系统。如果容器内部被攻破，攻击者可以通过 SFTP 协议读写本地主机上当前用户权限下的任意文件（如 `~/.ssh/`、`~/.aws/`）。

**Fix:** 使用 `sftp.Handlers` 接口自定义 `FileGet`/`FilePut`/`FileCmd`/`FileList`，在路径解析层拒绝逃逸出 `localDir` 的请求。或者在 SFTP server 启动前用 `os.Chroot` / Linux namespace 限制进程可见的文件系统范围：

```go
server, err := sftp.NewServer(rwc,
    sftp.WithServerWorkingDirectory(localDir),
    sftp.ReadOnly(), // 至少先加只读限制，如果不需要写回
)
```

如果需要双向同步，则应实现路径白名单的 `Handlers`。

## Low

### LO-01: SIGWINCH 监听 goroutine 泄漏

**File:** `internal/cloudclaude/ssh.go:101-108`
**Issue:** `signal.Stop(sigCh)` 停止信号投递但不关闭 channel，`for range sigCh` 的 goroutine 将永远阻塞。虽然在本程序中 `runClaude` 返回后进程即退出，实际不造成影响，但如果该函数未来被复用或测试中多次调用，每次都会泄漏一个 goroutine。

**Fix:**

```go
defer func() {
    signal.Stop(sigCh)
    close(sigCh)
}()
```

替换当前的 `defer signal.Stop(sigCh)`，确保 goroutine 在 channel 关闭后正常退出。

### LO-02: `err == io.EOF` 应使用 `errors.Is`

**File:** `internal/cloudclaude/ssh.go:128`
**Issue:** 直接比较 `err == io.EOF` 无法匹配被 `fmt.Errorf("%w", ...)` 包装过的 `io.EOF`。项目其余代码（controlplane、store 等）统一使用 `errors.Is` 进行错误类型判断。

**Fix:**

```go
if errors.Is(err, io.EOF) {
    return 0, nil
}
```

需在文件头 import 中补充 `"errors"`。

### LO-03: `os.Exit` 在 cobra `RunE` 中导致不可达代码

**File:** `cmd/cloud-claude/main.go:119-123, 151-155`
**Issue:** `runRoot` 是 cobra 的 `RunE` 回调，但函数中多处使用 `os.Exit()` 后紧跟 `return nil`。`os.Exit` 之后的语句永远不会执行，且 `os.Exit` 会跳过所有 `defer` 清理。两处 `return nil`（第 123 行和第 155 行）是死代码。

**Fix:** 将错误分类逻辑移到 `main()` 中，让 `runRoot` 返回带有分类信息的错误类型，由 `main()` 统一处理退出码：

```go
// runRoot 中返回错误而非直接 os.Exit
return fmt.Errorf("配置不存在: %w", err)

// main() 中处理
if err := rootCmd.Execute(); err != nil {
    fmt.Fprintf(os.Stderr, "错误: %s\n", err)
    os.Exit(classifyExitCode(err))
}
```

### LO-04: 错误分类依赖中文字符串匹配

**File:** `cmd/cloud-claude/main.go:117, 142-153`
**Issue:** 使用 `strings.Contains(errMsg, "认证失败")` 等中文子串匹配来分类错误并确定退出码。如果底层错误消息措辞变化（如"认证失败"改为"鉴权失败"），分类逻辑将静默失效，所有错误都落入 `exitInternalError`。

**Fix:** 定义类型化错误或哨兵值，在产生错误的位置用自定义类型包装，在分类处用 `errors.As` / `errors.Is` 判断：

```go
type AuthError struct{ Msg string }
func (e *AuthError) Error() string { return e.Msg }

// 分类时
var authErr *AuthError
if errors.As(err, &authErr) {
    os.Exit(exitAuthFailed)
}
```

---

_Reviewed: 2026-04-15_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
