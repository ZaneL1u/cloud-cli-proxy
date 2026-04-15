---
phase: 27-session
verified: 2026-04-15T06:10:00Z
status: human_needed
score: 8/8
overrides_applied: 0
human_verification:
  - test: "在本地项目目录运行 cloud-claude，容器内 ls /workspace 可看到本地文件"
    expected: "容器 /workspace 下内容与本地 CWD 一致"
    why_human: "需要运行中的网关和容器环境，无法离线验证 sshfs 挂载"
  - test: "在容器内 /workspace 创建文件，退出会话后本地目录出现该文件"
    expected: "新文件在本地 CWD 即时可见"
    why_human: "双向实时读写需要端到端运行环境"
  - test: "正常退出（exit/Ctrl+D）后执行 mountpoint -q /workspace 返回非零"
    expected: "挂载点已被清理，无残留"
    why_human: "需要真实 FUSE 环境验证清理逻辑"
  - test: "异常退出（kill -9 cloud-claude）后容器内挂载点无残留"
    expected: "fusermountCleanup 或后续启动时防御性卸载生效"
    why_human: "异常退出场景需要真实 SSH 连接和容器环境"
---

# Phase 27: 双 session 目录映射 Verification Report

**Phase Goal:** 用户当前目录通过 sshfs slave 实时映射到容器 /workspace，双向读写可靠
**Verified:** 2026-04-15T06:10:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

**Roadmap Success Criteria (SC):**

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| SC-1 | 用户运行 cloud-claude 时，CLI 自动在第二个 SSH session 上通过 sshfs slave 将当前目录映射到容器 /workspace | ✓ VERIFIED | `mount.go:44` 通过 `conn.NewSession()` 创建独立 sshfs session，`mount.go:62` 执行 `sshfs : /workspace -o passive -f`；`ssh.go:34` 在 ConnectAndRunClaude 内调用 `mountWorkspace(conn, cwd)` |
| SC-2 | 本地文件修改在容器内即时可见，容器内文件修改在本地即时可见（双向实时读写） | ✓ VERIFIED | `mount.go:70` 使用 `sftp.NewServer(rwc, sftp.WithServerWorkingDirectory(localDir))` 创建 SFTP server，SFTP 协议天然支持双向读写；`localDir` 由 `main.go:160` 的 `os.Getwd()` 获取后经 `ssh.go:34` 传入 |
| SC-3 | Claude Code 以 /workspace 为工作目录运行，可正常读写项目文件 | ✓ VERIFIED | `ssh.go:116` 构建远程命令为 `remoteCmd := "cd /workspace && " + claudeCmd`，保证 claude 在挂载目录内运行 |
| SC-4 | 会话正常或异常退出时，容器内 sshfs 挂载点和相关资源自动清理 | ✓ VERIFIED | `mount.go:99-104` cleanup 函数按序执行：`sshfsSession.Close()` → `<-sftpDone` → `server.Close()` → `fusermountCleanup(conn)`；`ssh.go:38` 通过 `defer cleanupMount()` 保证执行 |

**Plan 01 Must-Have Truths:**

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| P1-1 | mountWorkspace 在 SSH 连接上开启 sshfs session 并启动嵌入式 SFTP server | ✓ VERIFIED | `mount.go:44` `conn.NewSession()` + `mount.go:62` `session.Start("sshfs : /workspace -o passive -f")` + `mount.go:70` `sftp.NewServer(rwc, ...)` |
| P1-2 | waitForMount 轮询 mountpoint 检测直到挂载就绪或超时 | ✓ VERIFIED | `mount.go:110-138` timer+ticker+select 轮询结构；三个单元测试全部通过 |
| P1-3 | fusermountCleanup 通过短生命周期 session 执行防御性卸载 | ✓ VERIFIED | `mount.go:143-150` 创建短 session 执行 `fusermount -u /workspace 2>/dev/null \|\| true`，错误静默 |
| P1-4 | cleanup 函数按正确顺序关闭 sshfs session、等待 SFTP goroutine、执行 fusermount | ✓ VERIFIED | `mount.go:99-104` 顺序：Close session → wait sftpDone → Close server → fusermountCleanup |

**Plan 02 Must-Have Truths (merged/deduplicated with SC above):**

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| P2-1 | ConnectAndRunClaude 内部按 connect → mountWorkspace → runClaude 三阶段执行 | ✓ VERIFIED | `ssh.go:28-40` 三阶段编排：`sshConnect(cfg)` → `mountWorkspace(conn, cwd)` → `runClaude(conn, claudeArgs)` |
| P2-2 | 两个 session（sshfs + claude PTY）共享同一个 ssh.Client | ✓ VERIFIED | `ssh.go:28` 创建单一 `conn`；`ssh.go:34` mountWorkspace 使用 `conn`；`ssh.go:68` runClaude 使用同一 `conn` |
| P2-3 | claude 在容器内以 /workspace 为工作目录运行 | ✓ VERIFIED | 同 SC-3 |
| P2-4 | defer 链保证清理顺序：fusermount → close sshfs session → close SSH connection | ✓ VERIFIED | `ssh.go:32` `defer conn.Close()` 先声明（最后执行），`ssh.go:38` `defer cleanupMount()` 后声明（先执行），LIFO 正确 |
| P2-5 | main.go 传递当前工作目录给 ConnectAndRunClaude | ✓ VERIFIED | `main.go:160` `cwd, err := os.Getwd()` + `main.go:173` `ConnectAndRunClaude(sshCfg, args, cwd)` |

**Score:** 8/8 truths verified (4 SC + 4 unique plan truths; plan truths overlapping with SC deduplicated)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/cloudclaude/mount.go` | mountWorkspace, waitForMount, fusermountCleanup, channelRWC, MountNotReadyError | ✓ VERIFIED | 151 行，全部函数/类型存在且实现完整 |
| `internal/cloudclaude/mount_test.go` | TestWaitForMount 三个子测试 | ✓ VERIFIED | 61 行，3 个子测试全部 PASS |
| `go.mod` | github.com/pkg/sftp v1.13.10 | ✓ VERIFIED | 依赖声明存在，版本匹配 |
| `internal/cloudclaude/ssh.go` | 三阶段 ConnectAndRunClaude + sshConnect + runClaude | ✓ VERIFIED | 134 行，三函数完整实现 |
| `cmd/cloud-claude/main.go` | os.Getwd() + CWD 传递到 ConnectAndRunClaude | ✓ VERIFIED | line 160 获取 CWD，line 173 传入 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `mount.go` | `github.com/pkg/sftp` | `sftp.NewServer(rwc, sftp.WithServerWorkingDirectory(localDir))` | ✓ WIRED | mount.go:70 |
| `mount.go` | `golang.org/x/crypto/ssh` | `conn.NewSession()` 创建 sshfs session 和 mountpoint 检测 session | ✓ WIRED | mount.go:44, 83, 144 — 三处调用 |
| `ssh.go` | `mount.go` | `mountWorkspace(conn, cwd)` | ✓ WIRED | ssh.go:34 |
| `ssh.go` | `shellescape.QuoteCommand` | `cd /workspace && claude <args>` | ✓ WIRED | ssh.go:115-116 |
| `main.go` | `ssh.go` | `ConnectAndRunClaude(sshCfg, args, cwd)` | ✓ WIRED | main.go:173 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `mount.go` | `localDir` | `main.go:160 os.Getwd()` → `ssh.go:34 mountWorkspace(conn, cwd)` | Yes — OS CWD | ✓ FLOWING |
| `mount.go` | `conn` | `ssh.go:28 sshConnect(cfg)` | Yes — live SSH connection | ✓ FLOWING |
| `ssh.go` | `remoteCmd` | `shellescape.QuoteCommand` + `"cd /workspace && "` | Yes — dynamic command construction | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| 全项目编译 | `go build ./...` | exit 0 | ✓ PASS |
| cloudclaude 包编译 | `go build ./internal/cloudclaude/...` | exit 0 | ✓ PASS |
| CLI 二进制编译 | `go build ./cmd/cloud-claude/...` | exit 0 | ✓ PASS |
| waitForMount 测试 | `go test ./internal/cloudclaude/ -run TestWaitForMount -v` | 3/3 PASS | ✓ PASS |
| go vet | `go vet ./internal/cloudclaude/... ./cmd/cloud-claude/...` | exit 0 | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| MAP-01 | 27-01, 27-02 | 用户当前目录自动映射到容器 /workspace，通过 sshfs slave 实现 | ✓ SATISFIED | `mount.go:62` sshfs passive 模式启动 + `mount.go:70` SFTP server + `ssh.go:34` 三阶段集成 |
| MAP-02 | 27-01, 27-02 | 映射为双向实时读写，本地改动容器内即时可见，反之亦然 | ✓ SATISFIED | SFTP 协议原生支持双向读写；`sftp.WithServerWorkingDirectory(localDir)` 将用户 CWD 作为 SFTP 根 |
| MAP-03 | 27-01, 27-02 | 会话结束时自动清理容器内挂载点和相关资源 | ✓ SATISFIED | `mount.go:99-104` cleanup 四步顺序清理 + `ssh.go:38` defer 保证执行 + `mount.go:149` fusermount 防御性卸载 |

**Orphaned Requirements:** 无 — REQUIREMENTS.md 将 MAP-01/02/03 均映射到 Phase 27，全部被两个 PLAN 覆盖。

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | — | — | — |

无 TODO、FIXME、placeholder 或空实现。所有文件干净。

### Human Verification Required

以下行为无法通过静态分析验证，需要在有运行中网关+容器的环境下进行端到端测试：

### 1. sshfs 挂载实际生效

**Test:** 在本地项目目录运行 `cloud-claude`，容器内执行 `ls /workspace` 检查文件列表
**Expected:** 容器 /workspace 下内容与本地 CWD 一致
**Why human:** 需要运行中的 SSH 网关和带 FUSE 支持的容器环境

### 2. 双向实时读写

**Test:** 在容器内 `/workspace` 创建文件，退出会话后检查本地目录
**Expected:** 新文件在本地 CWD 即时可见；反向（本地新建文件后容器内可见）同理
**Why human:** SFTP 双向读写的实时性需要端到端运行环境验证

### 3. 正常退出清理

**Test:** 正常退出 cloud-claude（exit 或 Ctrl+D），然后在容器内执行 `mountpoint -q /workspace`
**Expected:** 返回非零退出码（挂载点已清理）
**Why human:** 需要真实 FUSE 环境和 SSH 连接验证清理链

### 4. 异常退出清理

**Test:** 运行 cloud-claude 后 `kill -9` 进程，检查容器内挂载点状态
**Expected:** fusermountCleanup 兜底或下次启动时防御性卸载生效
**Why human:** 异常退出场景需要真实进程管理和容器环境

### Gaps Summary

无代码级别 gap。所有 must-have truths 在代码层面全部验证通过（8/8），所有 key links 正确连接，所有 requirements 满足。

4 项端到端行为需要人工在有网关+容器的环境中验证。这些验证属于 Phase 28（生产环境 FUSE 兼容性验证）的前置条件——Phase 28 的 SC-3 明确要求"完整流程端到端通过"。

---

_Verified: 2026-04-15T06:10:00Z_
_Verifier: Claude (gsd-verifier)_
