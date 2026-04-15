---
phase: 26-arg-passthrough-tty
reviewed: 2026-04-15T12:00:00Z
depth: standard
files_reviewed: 4
files_reviewed_list:
  - internal/cloudclaude/ssh.go
  - cmd/cloud-claude/main.go
  - go.mod
  - go.sum
findings:
  critical: 0
  warning: 1
  info: 2
  total: 3
status: issues_found
---

# Phase 26：代码审查报告

**审查时间：** 2026-04-15T12:00:00Z  
**深度：** standard  
**审查文件数：** 4  
**结论：** issues_found  

## Summary

本次审查覆盖 Phase 26（arg-passthrough-tty）修改的 `ssh.go`、`main.go` 及 `go.mod`/`go.sum`。实现与计划一致：`shellescape.QuoteCommand` 构建远端命令、`ConnectAndRunClaude` 通过 `(int, error)` 上浮退出码并移除 `ssh.go` 内 `os.Exit`、TTY/非 TTY 分支与 cobra 透传配置均到位。发现 **1 条 Warning**：TTY 路径下 SIGWINCH 监听协程在 `signal.Stop` 后仍会因 `for range` 永久阻塞而泄漏；另有 **2 条 Info**（`os.Exit` 后不可达代码、主机密钥校验策略说明）。未发现可导致命令注入或绕过转义的严重问题。

## 与计划目标对齐

| 计划目标 | 结论 |
|----------|------|
| 用户参数经 `shellescape` 安全拼接到 `session.Start` | 已满足：`QuoteCommand(append([]string{"claude"}, claudeArgs...))` |
| 非 TTY 跳过 raw/PTY/SIGWINCH | 已满足：`term.IsTerminal(fd)` 分支 |
| 退出码返回值上浮、修复 HI-01（defer `term.Restore`） | 已满足：`ssh.ExitError` 返回状态码，`main` 中 `os.Exit` |
| cobra `DisableFlagParsing` + 前导 `--` 剥离 | 已满足 |
| `init` 子命令仍可路由 | 结构符合 cobra 惯例（根命令禁用解析、子命令独立） |

## Warnings

### WR-01：SIGWINCH 监听协程在会话结束后无法退出（协程泄漏）

**File:** `internal/cloudclaude/ssh.go:82-91`  
**Issue:** `go func()` 中使用 `for range sigCh` 从信号通道接收。`defer signal.Stop(sigCh)` 会停止向该通道投递信号，但**不会关闭通道**。`for range` 在通道未关闭时会一直阻塞，因此每次 TTY 会话结束仍会残留一个永久阻塞的 goroutine；频繁使用 `cloud-claude` 会累积泄漏。  
**Fix:** 在会话生命周期上增加显式结束信号，例如：

```go
done := make(chan struct{})
go func() {
    for {
        select {
        case <-sigCh:
            if w, h, err := term.GetSize(fd); err == nil {
                _ = session.WindowChange(h, w)
            }
        case <-done:
            return
        }
    }
}()
defer close(done)
defer signal.Stop(sigCh)
```

（实现时需避免与 `signal.Notify` 的 channel 关闭语义冲突：通常用 `done` 关闭即可退出循环，保留 `signal.Stop` 在 defer 中。）

## Info

### IN-01：`os.Exit` 之后的不可达 `return nil`

**File:** `cmd/cloud-claude/main.go:119-123`、`145-155`  
**Issue:** 若干分支在 `os.Exit(...)` 之后仍有 `return nil`，编译器可能报 unreachable（视工具链而定），且增加阅读噪音。  
**Fix:** 删除 `os.Exit` 之后的 `return nil`，或改为 `return` 前集中处理退出（例如抽成小函数统一 `os.Exit`）。

### IN-02：SSH 主机密钥未校验（既有行为）

**File:** `internal/cloudclaude/ssh.go:33`  
**Issue:** `ssh.InsecureIgnoreHostKey()` 使连接易受 MITM 攻击；若为本阶段可接受的开发/内网假设，建议在后续阶段或文档中明确。非 Phase 26 新引入逻辑，仅作备案。  
**Fix:** 后续可改为 `KnownHosts` 或固定 host key fingerprint 校验。

---

_Reviewed: 2026-04-15T12:00:00Z_  
_Reviewer: Claude (gsd-code-reviewer)_  
_Depth: standard_
