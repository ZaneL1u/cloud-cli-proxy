---
phase: 26-arg-passthrough-tty
verified: 2026-04-15T12:00:00Z
status: human_needed
score: 6/6
overrides_applied: 0
human_verification:
  - test: "在已配置网关且容器可用的环境中，对比 `claude` 与 `cloud-claude` 传入相同参数（如 `-p`、`--model`）时远端行为与退出码是否一致"
    expected: "远端 Claude Code 接收到的参数语义与本地等价，退出码一致"
    why_human: "需真实 SSH/容器与镜像侧 `claude`，本环境无法对远端进程做自动化断言"
  - test: "交互式 TTY 会话中调整终端行/列数，观察远端 Claude Code 界面是否随窗口变化"
    expected: "SIGWINCH 触发后远端 PTY 尺寸更新，界面排版跟随"
    why_human: "属实时终端与 UI 表现，静态代码无法等价验证"
  - test: "TTY 下在长时间任务中按 Ctrl+C / Ctrl+\\，确认中断符合预期；会话结束后本地 `stty` 正常（无 raw 残留）"
    expected: "中断由远端处理；本地 shell 可正常回显与编辑"
    why_human: "信号与终端状态需实机交互验证"
---

# Phase 26：参数透传与终端体验 — Verification Report

**Phase Goal：** cloud-claude 的参数透传和终端交互与本地 claude 完全一致  
**Verified：** 2026-04-15T12:00:00Z  
**Status：** human_needed  
**Re-verification：** 否 — 首次验证  

## Goal Achievement

### 与 ROADMAP / PLAN 对齐的 Must-Haves（Observable Truths）

| # | Truth | Status | Evidence |
|---|--------|--------|----------|
| 1 | 用户传入的 claude 参数到达容器内进程（经安全拼接） | ✓ VERIFIED | `main.go`：`DisableFlagParsing` + `args` 传入 `ConnectAndRunClaude`；`ssh.go`：`shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))` 后 `session.Start(remoteCmd)` |
| 2 | 终端 resize 时远端界面跟随（SIGWINCH → WindowChange） | ✓ VERIFIED | `ssh.go` TTY 分支：`signal.Notify(..., SIGWINCH)` + 循环中 `term.GetSize` + `session.WindowChange(h, w)` |
| 3 | Ctrl+C / Ctrl+\\ 在远端响应（raw 字节透传） | ✓ VERIFIED | TTY 路径：`term.MakeRaw` + `RequestPty`，stdin 与 session 直连；控制字符作为字节流送达远端 PTY（与常见本地 claude TTY 语义一致） |
| 4 | 远端 claude 退出码原样为 cloud-claude 退出码 | ✓ VERIFIED | `ssh.go`：`*ssh.ExitError` 时 `return exitErr.ExitStatus(), nil`；`main.go`：`if exitCode != 0 { os.Exit(exitCode) }` |
| 5 | 管道模式不申请 PTY | ✓ VERIFIED | `term.IsTerminal(fd)==false` 时跳过 `MakeRaw`、`RequestPty`、`SIGWINCH` |
| 6 | SSH 结束后本地终端不残留 raw 模式 | ✓ VERIFIED | TTY 路径 `defer term.Restore`；`ssh.go` 内无 `os.Exit`（仅注释提及），避免 HI-01 跳过 defer |

**Score：** 6/6 must-have truths 在代码层已验证  

### ROADMAP Success Criteria（契约）

| # | Success Criterion | Status | 对应实现 |
|---|-------------------|--------|----------|
| 1 | 用户参数原样传递到容器内 Claude Code | ✓ VERIFIED | 同 Truth 1 |
| 2 | resize 时 SIGWINCH 传递到远端进程 | ✓ VERIFIED | 同 Truth 2 |
| 3 | Ctrl+C 等正确转发 | ✓ VERIFIED | 同 Truth 3 |
| 4 | 退出码透传 | ✓ VERIFIED | 同 Truth 4 |

### Required Artifacts（gsd-tools + 人工核对）

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/cloudclaude/ssh.go` | 参数、`QuoteCommand`、TTY/非 TTY、退出码返回 | ✓ VERIFIED | `gsd-tools verify artifacts` 通过；含 `shellescape.QuoteCommand`、`IsTerminal`、`RequestPty` 仅一处 |
| `cmd/cloud-claude/main.go` | cobra 透传、`--` 剥离、退出码 | ✓ VERIFIED | `DisableFlagParsing`、`ArbitraryArgs`、`args[0]=="--"`、`ConnectAndRunClaude(sshCfg, args)` |
| `go.mod` | shellescape 依赖 | ✓ VERIFIED | `al.essio.dev/pkg/shellescape v1.6.0` |

### Key Link Verification

`gsd-tools verify key-links` 对 PLAN 中 `pattern` 报告未匹配（例如 `ConnectAndRunClaude` 在源码中带 `cloudclaude.` 前缀；`ExitStatus()` 出现在 `ssh.go` 而非 `main`）。**手动核对如下：**

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `cmd/cloud-claude/main.go` | `internal/cloudclaude/ssh.go` | `ConnectAndRunClaude` | ✓ WIRED | `exitCode, err := cloudclaude.ConnectAndRunClaude(sshCfg, args)` |
| `internal/cloudclaude/ssh.go` | `session.Start` | `QuoteCommand` 构建命令 | ✓ WIRED | `remoteCmd := ... QuoteCommand(...)` → `session.Start(remoteCmd)` |
| `internal/cloudclaude/ssh.go` | `cmd/cloud-claude/main.go` | 退出码上浮 | ✓ WIRED | 返回 `(int, error)`，`main` 中 `os.Exit(exitCode)`（非 `ExitStatus` 字面量于 main，语义等价 PLAN） |

### Data-Flow Trace（Level 4）

| Artifact | 数据 | 来源 | Produces Real Data | Status |
|----------|------|------|---------------------|--------|
| `runRoot` → SSH | `args` | `cobra` 根命令非 init 时的 argv 余量 | 是（用户真实参数） | ✓ FLOWING |
| `ConnectAndRunClaude` | `claudeArgs` | `main` 传入 | 拼入远端命令字符串 | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| 可编译 | `go build -o /dev/null ./cmd/cloud-claude/` | 成功 | ✓ PASS |
| 远端命令构建 | `grep -n 'QuoteCommand' internal/cloudclaude/ssh.go` | 存在 | ✓ PASS |
| 无 ssh 内 os.Exit | `grep 'os.Exit' internal/cloudclaude/ssh.go` | 仅注释 | ✓ PASS |

未启动网关/SSH 服务；完整 `cloud-claude -p` 端到端留待人工或集成环境（符合 Step 7b 约束）。

### Requirements Coverage（PLAN `requirements` ↔ REQUIREMENTS.md）

| ID | REQUIREMENTS.md 描述 | Source Plan | Status | Evidence |
|----|------------------------|-------------|--------|----------|
| CLI-03 | 用户传入的所有 claude 参数原样透传到容器内 Claude Code | 26-01-PLAN | ✓ SATISFIED | cobra 透传 + `QuoteCommand` + `session.Start` |
| TTY-01 | 终端 resize 时 SIGWINCH 正确传递 | 26-01-PLAN | ✓ SATISFIED | SIGWINCH + `WindowChange` |
| TTY-02 | Ctrl+C / Ctrl+\\ 等信号正确转发 | 26-01-PLAN | ✓ SATISFIED | raw + PTY + 字节直通 |
| TTY-03 | 容器退出码透传本地 CLI | 26-01-PLAN | ✓ SATISFIED | `ExitStatus` 返回 + `main` `os.Exit` |

**孤儿需求：** 无 — PLAN 声明的四个 ID 均在 REQUIREMENTS.md 中有定义且本阶段有实现证据。

### Anti-Patterns / 备注

| 来源 | 说明 | Severity |
|------|------|----------|
| `26-REVIEW.md`（若存在） | SIGWINCH 监听 goroutine 在 `signal.Stop` 后可能阻塞于 `for range` | ℹ️ Info，非本阶段目标阻断项 |

代码审查未在 `ssh.go`/`main.go` 中发现占位 `TODO`、空返回或「未实现」API 响应类桩代码。

### Deferred Items（Step 9b）

无。后续 Phase 27/28 目标不涉及用后续阶段覆盖本阶段未完成的参数/TTY/退出码契约。

### Human Verification Required

见 YAML frontmatter `human_verification` 列表。

### Gaps Summary

无代码层缺口（`gaps` 省略）。**「与本地 claude 完全一致」**在联调意义上的最终确认依赖真实环境与人工/集成测试，故总状态为 **human_needed**。

---

_Verified: 2026-04-15T12:00:00Z_  
_Verifier: Claude (gsd-verifier)_
