# Phase 36: 映射前置约束 + sshfs 内核缓存 - Research

**Researched:** 2026-04-23
**Domain:** Go CLI / FUSE sshfs / hot_sync / doctor / errcodes
**Confidence:** HIGH（全部通过代码阅读验证；无 [ASSUMED] 标注）

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01**：git 检测点在 `cmd/cloud-claude/main.go::runRoot`，位置在 `os.Getwd()` 之后、`AuthenticateAndWait` 之前；使用 `exec.Command("git", "rev-parse", "--show-toplevel").Run()`；退出码恒为 `exitConfigError`(=4)
- **D-02**：封装为 `cmd/cloud-claude/git_check.go`，导出 `requireGitRepo(cwd string) error`
- **D-03**：判定包含 git worktree / submodule / detached HEAD；git 不可用按"非 git 仓库"处理
- **D-04**：`Config` 新增 `HotSyncMaxFileMB int`（yaml tag `hot_sync_max_file_mb,omitempty`）；accessor `EffectiveHotSyncMaxFileMB() int { if <=0 { return 50 }; return c.HotSyncMaxFileMB }`
- **D-05**：`HotSyncConfig` 新增 `MaxFileBytes int64`（由 mount_strategy 注入 `cfg.HotSyncMaxFileMB * 1024 * 1024`）；零值不熔断
- **D-06**：ignore 过滤优先于 size 检查（第一层 ignore → 第二层 size）
- **D-07**：`HotSyncStatus.OversizedFiles []OversizedFile`；`StartHotSync` 返回携带，由 `MountWorkspace` 塞进 `LastSessionSnapshot.OversizedFiles`
- **D-08**：stderr 一次性提示由 `MountWorkspace` 在 `StartHotSync` 返回后输出；N≤5 全列；N>5 只列前 5 条
- **D-09**：`LastSessionSnapshot.OversizedFiles []OversizedFile`（omitempty）；schema_version=1 不变；`OversizedFile{Path string, SizeBytes int64}`
- **D-10**：`mountSSHFS` 追加 `,cache=yes,kernel_cache,auto_cache,cache_timeout=300`（字面量顺序锁死）
- **D-11**：`pkg/sftp` + 真实 sshfs 的 fixture SFTP 计数器测试；skip 模式与 v3.0 mount_test.go 同
- **D-13**：5 项新 check 全加到 `internal/cloudclaude/doctor/mount.go` 同文件
- **D-15**：5 项新 check 不提供 --fix，仅 NextAction
- **D-16/17/18**：只新增 2 条 code；`git_proxy_enabled` 和 `default_ignore_loaded` 复用 `AUTH_CONFIG_MISSING`
- **D-19**：`cmd/cloud-claude/explain.go` **不改动**
- **D-21**：`make ci-gate` 自动覆盖新增 code + 说明
- **D-22**：真机 e2e 签字留 Phase 37

### Claude's Discretion

- `OversizedFile` struct 的包位置（`cloudclaude` 包 or `last_session.go`，executor 选最自然位置）
- `scanLocalSyncFiles` 函数签名变更方式（第二返回值 vs 后处理 vs engine 内过滤，executor 选最小改动）
- `doctor/mount.go` 中 5 项新 check 的函数名、具体 Details map key
- `git_check_test.go` 中 mock `exec.Command` 的方式

### Deferred Ideas (OUT OF SCOPE)

Phase 37 全部内容（cold-promoter / PromotionEngine / 4 项晋升 doctor check / runbook / e2e UAT 脚本），详见 CONTEXT.md。
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| REQ-MOUNT-V31-01 | cwd 非 git 仓库立即拒绝，MOUNT_REQUIRE_GIT_REPO + exitConfigError | F1 锚点：main.go:332 前插 os.Getwd() + git_check.go |
| REQ-MOUNT-V31-02 | 单文件 ≥ MaxFileBytes 且未 ignore → 跳过 hot，由 cold 兜底 | F2 锚点：scanLocalSyncFiles + HotSyncEngine.initialSync |
| REQ-MOUNT-V31-03 | last-session.json 新增 oversized_files + schema_version=1 不变 | F2 锚点：LastSessionSnapshot + WriteLastSession |
| REQ-MOUNT-V31-04 | sshfs 命令追加 cache 参数；同文件 cat 2 次 → server read=1 | F3 锚点：mount_sshfs.go:41 sshfsCmd 字面量 |
| REQ-MOUNT-V31-05 | doctor mount 新增 5 项 check，JSON check count 比 v3.0 多 5 | F4 锚点：doctor.go:226-234 mount domain + mount.go |
| REQ-MOUNT-V31-06 | 2 条 errcodes 注册 + 各 ≥200 字 ExtendedExplanations + explain 子进程 exit 0 | F5 锚点：codes.go / mount.go / explanations.go |
</phase_requirements>

---

## Executive Summary

Phase 36 是六项纯配置/校验/参数级改动（F1–F6），零新依赖、零协议变更、独立可发。**最关键的结构性地雷**：`main.go` 中 `os.Getwd()` 当前位于 `AuthenticateAndWait` **之后**（line 332 vs line 302），而 D-01 要求 git 闸门在 `AuthenticateAndWait` **之前**——executor 必须先把 `os.Getwd()` 前移到 line 287 附近（LoadConfig 之后），再在此处插入 `requireGitRepo` 调用。其余五项改动均为加法（不改函数签名、不改序列化格式），风险面极低。`pkg/sftp v1.13.10` 已在 go.mod，`sftp.NewRequestServer`+自定义 `Handlers` 是 D-11 counting test 的标准路径。go 1.25.7 可用 `errors.Join`、`slices` 等新标准库。`make ci-gate` 脚本不需要改动：新增 2 条 errcode + 2 条 ExtendedExplanations + 5 项 check 全部自动通过现有断言集。

**主要建议**：先做 F5（errcode 注册），再 F1（git_check.go + main.go 改动），其余 F2/F3/F4/F6 顺序无强依赖。

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| F1 git 仓库前置校验 | CLI 本地（cmd/cloud-claude/） | — | 必须在网络操作前在本机执行 |
| F2 单文件大小熔断 | 本地热同步引擎（HotSyncEngine） | mount_strategy（写 last-session） | 熔断逻辑属于文件扫描层，结果由编排层持久化 |
| F3 sshfs FUSE cache 参数 | SFTP server 进程（mount_sshfs.go） | FUSE 内核 page cache（透明） | 参数在命令拼接处注入，缓存由内核接管 |
| F4 doctor mount 5 项 check | doctor 包本地/远端双模 | RemoteRunner（远端 shell） | 沿用既有 check 框架，无需新层 |
| F5 errcodes 注册 + explain | errcodes 包（启动期 init()） | explain.go（Registry 自动注入） | 注册表是全局单例，explain 消费不需改动 |
| F6 测试与 CI | 各包单元测试 + make ci-gate | — | 闸门已固化，只需代码实现通过 |

---

## Existing Architecture Map

### F1 锚点：`main.go::runRoot` 调用链（精确行号）

```
cmd/cloud-claude/main.go::runRoot

line 281: mode, err := cloudclaude.ParseMode(mountMode)
           ↓
line 287: cfg, err := cloudclaude.LoadConfig()        ← D-01 建议将 os.Getwd() 移到这里之后
           ↓
line 302: authResp, err := client.AuthenticateAndWait(...)  ← ⚠ 当前 Getwd 在此 AFTER
           ↓
line 332: cwd, err := os.Getwd()    ← 🔴 必须前移到 line 302 之前
           ↓
line 369: exitCode, err := cloudclaude.ConnectAndRunClaudeV3(sshCfg, args, cwd, ...)
```

**关键地雷**：CONTEXT D-01 说「cwd 解析后、AuthenticateAndWait 之前」，但当前代码 `os.Getwd()`（line 332）在 `AuthenticateAndWait`（line 302）之后。Executor **必须**将 `os.Getwd()` 前移，否则 git 闸门时序不满足 REQ-MOUNT-V31-01 的「不发起任何 SSH 文件操作」。

**修改方案（最小改动）**：
```go
// 在 LoadConfig（line 287）之后、NewEntryClient（line 299）之前插入：
cwd, err := os.Getwd()
if err != nil {
    fmt.Fprintln(os.Stderr, "错误: 无法获取当前工作目录: "+err.Error())
    os.Exit(exitInternalError)
}
if err := requireGitRepo(cwd); err != nil {
    fmt.Fprintln(os.Stderr, err.Error())
    os.Exit(exitConfigError)
}
// 删除原 line 332 的 os.Getwd() 块
```

**exit codes**（`main.go` constants，line 22-28）：
```go
exitOK            = 0
exitAuthFailed    = 1
exitNetworkError  = 2
exitTimeout       = 3
exitConfigError   = 4  ← MOUNT_REQUIRE_GIT_REPO 使用此值
exitInternalError = 5
```

### F2 锚点：`HotSyncEngine` 初始化扫描链

```
StartHotSync(connA, connB, HotSyncConfig{MaxFileBytes: ...})  [hot_sync.go:88]
  ↓
engine.initialSync()  [hot_sync.go:145]
  ↓
scanLocalSyncFiles(e.localDir, e.matcher)  [hot_sync.go:388]
  ← 当前返回 map[string]syncFileState，无大小过滤
  ← 第二层 size 检查需在 initialSync() 内、调用 scanLocalSyncFiles 之后注入
```

**`scanLocalSyncFiles` 现状**（hot_sync.go:388-428）：

- 第一层 ignore：`matcher.IsIgnoredRel(rel, false)`（line 416）→ `continue/return nil`
- info.Size() 已取到（line 421）但未做阈值判断
- 返回 `map[string]syncFileState`，**不含**跳过原因

**推荐注入方式**（executor 自由选最小改动）：

**方案 A（推荐）**：在 `initialSync()` 的 `localFiles, err := scanLocalSyncFiles(...)` 之后，直接遍历 `localFiles` 过滤：

```go
// initialSync() 内，scanLocalSyncFiles 返回后
var oversized []OversizedFile
if e.maxFileBytes > 0 {
    for rel, state := range localFiles {
        if state.Size >= e.maxFileBytes {
            oversized = append(oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
            delete(localFiles, rel)
        }
    }
}
e.oversized = oversized
```

方案 A 不改 `scanLocalSyncFiles` 签名，改动最小，且在 `run()` goroutine 启动之前完成（无并发安全问题）。

**`HotSyncStatus` 返回链**（需新增 `OversizedFiles`）：

```
StartHotSync → return cleanup, HotSyncStatus{OversizedFiles: engine.oversized}, nil

MountWorkspace::tryModeReal → hCleanup, hStatus, hErr := StartHotSync(...)
                             → snapshot.OversizedFiles = hStatus.OversizedFiles  [mount_strategy.go:~305]
                             → writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
```

**`MountWorkspace` 中写 last-session 的行**（mount_strategy.go）：

现有 `writeLastSessionWarn` 调用位于 `tryModeReal` 成功路径：mount_strategy.go line ~305（`printBanner` 之后，`writeLastSessionWarn` 之前）。Executor 在 `snapshot.ConflictCount = hotStatus.ConflictCount` 那行后追加 `snapshot.OversizedFiles = hotStatus.OversizedFiles`。

**D-08 stderr 一次性输出**（mount_strategy.go，`StartHotSync` 成功返回后）：
```go
if n := len(hStatus.OversizedFiles); n > 0 {
    limit := n; if limit > 5 { limit = 5 }
    fmt.Fprintf(cfg.Logger, "[!] 跳过大文件 %d 个（>%dMB），由 cold 兜底:\n", n, cfg.HotSyncMaxFileMB)
    for _, f := range hStatus.OversizedFiles[:limit] {
        fmt.Fprintf(cfg.Logger, "  %s (%dMB)\n", f.Path, f.SizeBytes/1024/1024)
    }
    if n > 5 {
        fmt.Fprintf(cfg.Logger, "  ... 还有 %d 个，见 ~/.cloud-claude/last-session.json\n", n-5)
    }
}
```

`cfg.HotSyncMaxFileMB` 需要从 `MountConfig` 透传过来（D-04 的 `Config.EffectiveHotSyncMaxFileMB()`）。

### F2 并发安全分析

`engine.oversized` 在 `initialSync()` 内填充（hot_sync.go:145-187），此时 `go engine.run()` 尚未启动（`StartHotSync` 在 `initialSync()` 完成后才 `go engine.run()`，hot_sync.go:~125）。轮询阶段 `syncOnce()` 读取 `engine.oversized` 是只读操作（不 append），**无需新 sync primitive**。[VERIFIED: 代码阅读]

### F3 锚点：`mountSSHFS` sshfs 命令字面量（mount_sshfs.go:41）

```go
// 当前（mount_sshfs.go line 41）：
sshfsCmd := fmt.Sprintf(
    "sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10 -f",
    shellQuote(remotePath),
)

// Phase 36 目标（追加在 ConnectTimeout=10 之后，-f 之前）：
sshfsCmd := fmt.Sprintf(
    "sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300 -f",
    shellQuote(remotePath),
)
```

**`mountSSHFS` 函数位置**：`internal/cloudclaude/mount_sshfs.go:19`，被 `tryModeReal`（mount_strategy.go:~290）和 `mountWorkspace`（mount_sshfs.go:~90）调用。

### F4 锚点：`doctor.go` mount 维度注册点（lines 226-234）

```go
// doctor.go:226-234（当前 4 项，Phase 36 追加 5 项）：
if want("mount") {
    ensureRemote()
    report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "mergerfs_branches", timeout,
        func(c context.Context) Check { return checkMergerfsBranches(c, remoteRunner) }))
    report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_mountpoint", timeout,
        func(c context.Context) Check { return checkSSHFSMountpoint(c, remoteRunner) }))
    report.Checks = append(report.Checks, checkFUSEResidual(ctx))
    report.Checks = append(report.Checks, checkAppArmorFusermount3(ctx))
    // ↑ v3.0: 4 checks（SC#4 基线）

    // Phase 36 追加（executor 在此段末尾添加）：
    report.Checks = append(report.Checks, checkRequireGitRepo(ctx))
    report.Checks = append(report.Checks, checkOversizedFilesCount(ctx))
    report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_cache_args", timeout,
        func(c context.Context) Check { return checkSSHFSCacheArgs(c, remoteRunner) }))
    report.Checks = append(report.Checks, checkGitProxyEnabled(ctx))
    report.Checks = append(report.Checks, checkDefaultIgnoreLoaded(ctx))
    // ↑ Phase 36: +5 → 9 checks total in mount domain（SC#4 ✓）
}
```

`checkMergerfsBranches` 远端 runner 模式（mount.go:13-55）是 `sshfs_cache_args` 的精确模板：
- `runner.RunScript("sshfs_mount", "mount | grep sshfs | head -1")` 拿输出
- 遍历 `want` 切片检查各参数是否 `strings.Contains(mountOut, w)`
- missing 列表 join 后作为 fail details

### F5 锚点：errcodes 注册表

**codes.go**：在 `MOUNT_APFS_CASE_INSENSITIVE` 之后（按字母序 M-O-R 排列）：
```go
MOUNT_OVERSIZED_FILE_SKIPPED Code = "MOUNT_OVERSIZED_FILE_SKIPPED"  // O 在 A 之后
MOUNT_REQUIRE_GIT_REPO       Code = "MOUNT_REQUIRE_GIT_REPO"        // R 在 O 之后
```

**mount.go**（在 `MOUNT_APFS_CASE_INSENSITIVE` 的 `MustRegister` 之后 `init()` 末尾追加）。

**codes_test.go 自动覆盖逻辑**（codes_test.go:TestErrcodesRegistry）：
- 遍历 `Registry()` → 检查命名正则 + Message/NextAction 非空 + NextAction ≤80 runes
- 下限 `len(reg) < 30`（当前 ~42 条，+2 后 ~44 条，自动通过）
- **MOUNT_REQUIRE_GIT_REPO** 和 **MOUNT_OVERSIZED_FILE_SKIPPED** 命名均符合 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`

**explanations_test.go 自动覆盖逻辑**（TestAllCodesHaveExplanations）：
- 遍历 `Registry()` → 跳过 `ExplainExempt` → 检查 `ExtendedExplanations[code]` 存在 + ≥200 rune
- 新增 2 条均不在 `ExplainExempt`（D-18 明确）→ 必须在 `explanations.go::init()` 注册 ≥200 字说明
- `TestAllDomainsClosed` 检查 DOMAIN ∈ 8 域：MOUNT 已在允许列表

---

## Technical Approach（F1–F6 实现锚点）

### F1 · Git 仓库前置约束

**新文件**：`cmd/cloud-claude/git_check.go`（`package main`）

```go
package main

import (
    "fmt"
    "os/exec"
    "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// requireGitRepo 在 cwd 执行 git rev-parse --show-toplevel；
// 非 git 仓库（包括 git 未安装）返回包含 MOUNT_REQUIRE_GIT_REPO 错误码的 error。
func requireGitRepo(cwd string) error {
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    cmd.Dir = cwd
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_REQUIRE_GIT_REPO, cwd))
    }
    return nil
}
```

**main.go 改动**：在 `LoadConfig` 成功后、`NewEntryClient` 前：
1. 将 `os.Getwd()` 块从 line 332 前移至此处
2. 调用 `requireGitRepo(cwd)` → 失败时 `os.Exit(exitConfigError)`

**`git_check_test.go`** 三场景：git 仓库（pass）、非 git（fail + exitConfigError）、git 命令不可用（fail + exitConfigError）。注入方式：函数接受 `execCommandFunc` 参数，或用临时目录真实测试（无需 mock）。

### F2 · 单文件大小熔断

**修改文件列表**（最小集）：

| 文件 | 改动 |
|------|------|
| `internal/cloudclaude/config.go` | `Config` 新增 `HotSyncMaxFileMB int` + `EffectiveHotSyncMaxFileMB()` accessor |
| `internal/cloudclaude/hot_sync.go` | `HotSyncConfig.MaxFileBytes int64`、`HotSyncEngine.maxFileBytes/oversized`、`HotSyncStatus.OversizedFiles`、`initialSync()` 后处理 |
| `internal/cloudclaude/last_session.go` | `LastSessionSnapshot.OversizedFiles []OversizedFile`、包级 `OversizedFile` struct |
| `internal/cloudclaude/mount_strategy.go` | `MountConfig` 注入 `MaxFileBytes`、`snapshot.OversizedFiles` 赋值、D-08 stderr 输出 |

**`OversizedFile` struct 放置**：建议放在 `last_session.go`（与 `LastSessionSnapshot` 同文件，减少跨包依赖）。`hot_sync.go` 直接用 `cloudclaude.OversizedFile`（同包）。

**持续同步阶段处理**：`syncOnce()` 中 `scanLocalSyncFiles` 会再次扫描本地文件，返回的 `localFiles` 包含所有未 ignore 文件（包括大文件）。大文件进入 `localFiles` 后会触发 `localChanged = true` → `applyLocal` → `copyLocalToRemote`。因此 `syncOnce()` 也需要跳过大文件。推荐在 `syncOnce()` 开头同样过滤 `localFiles`（与 `initialSync` 同逻辑，可提取为私有方法 `filterOversized(files map[string]syncFileState) []OversizedFile`）。

### F3 · sshfs FUSE page cache 参数

**唯一改动**：`mount_sshfs.go:41` sshfsCmd 字面量追加 4 个缓存参数（如 F3 锚点所示）。

**D-11 fixture SFTP 测试**（`internal/cloudclaude/mount_sshfs_test.go` 新增）：

```go
// pkg/sftp v1.13.10 已在 go.mod [VERIFIED]
// sftp.NewRequestServer 接受 sftp.Handlers{FileGet: FileReader}
// FileReader 接口：Fileread(*Request) (io.ReaderAt, error)

type countingFileReader struct {
    root   string
    mu     sync.Mutex
    reads  map[string]*atomic.Int64  // path → read call count
}

func (c *countingFileReader) Fileread(req *sftp.Request) (io.ReaderAt, error) {
    // 打开文件 + 递增计数器
    f, err := os.Open(filepath.Join(c.root, req.Filepath))
    if err != nil { return nil, err }
    c.mu.Lock()
    if _, ok := c.reads[req.Filepath]; !ok {
        c.reads[req.Filepath] = &atomic.Int64{}
    }
    ctr := c.reads[req.Filepath]
    c.mu.Unlock()
    ctr.Add(1)
    return f, nil
}
```

测试流程：
1. `t.Skip()` if `exec.LookPath("sshfs") != nil`（与 v3.0 mount_test.go 同模式）
2. 启动 `net.Listen("tcp", ":0")` + `ssh.NewServerConn` 包装 `sftp.NewRequestServer(..., sftp.Handlers{FileGet: counting})`
3. `exec.Command("sshfs", "user@127.0.0.1:/ tmpDir -o port=N,...")` 真实挂载
4. 两次 `os.ReadFile(filepath.Join(tmpDir, "fixture.bin"))`
5. 断言 `counting.reads["/fixture.bin"].Load() == 1`（page cache 命中，第二次 read 不到 server）

**注意**：`sftp.NewRequestServer` 需要实现完整 `Handlers`（FileLister 等），可用 `sftp.InMemHandler` 作为其他接口的 no-op 底座，仅 `FileGet` 覆盖计数逻辑。或者直接全部实现（fixture 只读，其他返回 `ErrSSHFxOpUnsupported`）。

### F4 · doctor mount 5 项 check 实现

**`internal/cloudclaude/doctor/mount.go` 追加 5 个函数**：

| 函数名 | 本地/远端 | 核心逻辑 | 命中时 Status/Code |
|--------|----------|---------|------------------|
| `checkRequireGitRepo(ctx)` | 本地 | `exec.Command("git", "rev-parse", "--show-toplevel").Run()` | Fail / MOUNT_REQUIRE_GIT_REPO |
| `checkOversizedFilesCount(ctx)` | 本地 | `cloudclaude.LoadLastSession()` → `len(snap.OversizedFiles)` | Pass(0) / Warn(>0) / Skip(nil) MOUNT_OVERSIZED_FILE_SKIPPED |
| `checkSSHFSCacheArgs(ctx, runner)` | 远端 | `runner.RunScript("sshfs_mount", "mount \| grep sshfs \| head -1")` + 4 参数 Contains 检查 | Pass / Fail MOUNT_SSHFS_FAILED |
| `checkGitProxyEnabled(ctx)` | 本地 | `cloudclaude.LoadConfig()` → `cfg.EffectiveProxyCommands()` → contains("git") | Pass / Warn AUTH_CONFIG_MISSING |
| `checkDefaultIgnoreLoaded(ctx)` | 本地 | `os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE") == "1"` | Pass(未设) / Warn(已设) AUTH_CONFIG_MISSING |

**`checkRequireGitRepo` 不能 import cmd/ 包**（cmd/cloud-claude 是 main package）。在 `mount.go` 内写一个 12 行私有 helper：
```go
func gitRevParseTopLevel(cwd string) error {
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    cmd.Dir = cwd
    return cmd.Run()
}
```

**`checkOversizedFilesCount` Details**（参照 `checkFUSEResidual` 的 Details map 模式）：
```go
Details: map[string]any{
    "oversized_count": n,
    "top5_files":      top5RelPaths,  // []string
}
```

**`checkSSHFSCacheArgs` want 切片**：
```go
want := []string{"cache=yes", "kernel_cache", "auto_cache", "cache_timeout=300"}
```
模板完全复用 `checkMergerfsBranches` 的 missing-list 逻辑（mount.go:45-55）。

**`checkGitProxyEnabled` 注意**：`LoadConfig()` 可能失败（config 未 init），此时 `newSkip(...)` 处理，与 `checkOversizedFilesCount` 中 nil snapshot 走 skip 同模式。

**`doctor.go` 中这 5 项的 `ensureRemote()` 依赖**：
- `require_git_repo`、`oversized_files_count`、`git_proxy_enabled`、`default_ignore_loaded` 都是本地 check，**不需要** ensureRemote
- `sshfs_cache_args` 需要 remoteRunner，放在 `runWithTimeout` 中（与 `sshfs_mountpoint` 同模式）

**关键**：`checkRequireGitRepo` 在 doctor 场景需要一个 cwd 来 `cd` 执行 git。doctor mount 调用时没有 cwd 参数。推荐：直接用 `os.Getwd()`（doctor 运行时 cwd 即为用户工程目录），与 main.go 语义一致。

---

## Test Strategy

### D-20 测试矩阵（对应测试文件）

| 测试文件 | 覆盖场景 | CI 自动性 |
|---------|---------|---------|
| `cmd/cloud-claude/git_check_test.go`（新建） | git 仓库→pass；非 git→fail+错误码；无 git 命令→fail | `go test ./cmd/cloud-claude/...` |
| `internal/cloudclaude/hot_sync_oversized_test.go`（新建） | ignore 命中的大文件→不进 OversizedFiles；未 ignore 的 60MB→进；30MB→不进 | `go test ./internal/cloudclaude/...` |
| `internal/cloudclaude/last_session_test.go`（扩展） | 含 OversizedFiles 的 snapshot 序列化/反序列化；omitempty 空数组不出现 | `go test ./internal/cloudclaude/...` |
| `internal/cloudclaude/mount_sshfs_test.go`（新增函数） | D-11 fixture SFTP counting（sshfs 未安装 → Skip） | `go test -short` skip，CI 上 sshfs 可用时运行 |
| `internal/cloudclaude/doctor/mount_test.go`（扩展） | 5 项新 check 各 pass/warn/fail/skip 矩阵（mock runner + mock exec） | `go test ./internal/cloudclaude/doctor/...` |
| `internal/cloudclaude/errcodes/codes_test.go`（自动） | TestErrcodesRegistry 遍历覆盖 2 条新 code | 无需改动 |
| `internal/cloudclaude/errcodes/explanations_test.go`（自动） | TestAllCodesHaveExplanations 遍历覆盖 2 条新说明 | 无需改动 |
| `cmd/cloud-claude/explain_test.go`（扩展） | 子进程 exit 0 + stdout ≥200 字 — 2 条新 code | `go test ./cmd/cloud-claude/...` |

### explain_test.go 扩展模板（D-21 已声明自动覆盖，单测仅做端到端断言）

```go
// 复用 buildOnceExplainBin + runExplainBin pattern
func TestExplain_MountRequireGitRepo_Exit0_MinLen(t *testing.T) {
    bin := buildOnceExplainBin(t)
    code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_REQUIRE_GIT_REPO")
    if code != 0 {
        t.Fatalf("known code exit %d; stderr=%q", code, stderr)
    }
    if n := utf8.RuneCountInString(stdout); n < 200 {
        t.Errorf("stdout 字符数 %d < 200", n)
    }
}
// MOUNT_OVERSIZED_FILE_SKIPPED 同模式
```

### CI Gate 自动覆盖清单（无需改动 ci-gate 或 ci-doctor-grep.sh）

| 断言 | 触发文件 | 自动通过原因 |
|-----|---------|------------|
| errcodes 命名正则 | codes_test.go | MOUNT_REQUIRE_GIT_REPO / MOUNT_OVERSIZED_FILE_SKIPPED 符合 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$` |
| Message/NextAction 非空 + NextAction ≤80 rune | codes_test.go | D-17 明确要求 |
| ExtendedExplanations ≥200 字符 | explanations_test.go | D-18 写入 ≥200 字 |
| 8 域闭合（MOUNT ∈ allowed） | explanations_test.go::TestAllDomainsClosed | MOUNT 已在允许列表 |
| schema_version=1 | ci-doctor-grep.sh | doctor Report.SchemaVersion 硬编码 1 |
| warn/fail check 含 next_action | ci-doctor-grep.sh | `newWarn`/`newFail` helper 自动从 Entry 取 NextAction |
| warn/fail 行含错误码 `[XXX_YYY]` | ci-doctor-grep.sh | `newWarn`/`newFail` 设 `Check.Code`，`render.go` 负责格式化 |

---

## Landmines & Mitigations

### L1 ⚠️ `os.Getwd()` 位置地雷（最高优先级）

**问题**：main.go line 332 的 `os.Getwd()` 在 `AuthenticateAndWait`（line 302）之后。D-01 要求 git 检测在 `AuthenticateAndWait` 之前。

**规避**：Executor 必须将 `os.Getwd()` 块（line 332-337）整体前移到 `LoadConfig` 成功后（line 295）、`NewEntryClient` 前。同时删掉 line 332 的原始代码（避免重复调用和 `cwd` 重声明 shadowing 问题）。

### L2 `doctor/mount.go` 中 `checkRequireGitRepo` 的 cwd 来源

**问题**：doctor mount 运行时没有"会话 cwd"参数，只有运行时进程的 cwd。

**规避**：使用 `os.Getwd()`——doctor 命令约定在工程目录下运行，与 cloud-claude main 语义一致。若 cwd 不是 git repo，doctor 输出 Fail 提示用户 cd 到 git 仓库再检查，符合预期。

### L3 `scanLocalSyncFiles` 改动对 `syncOnce()` 的遗漏

**问题**：若只在 `initialSync()` 过滤大文件，`syncOnce()` 轮询时 `scanLocalSyncFiles` 仍会返回大文件，导致后续 `copyLocalToRemote` 上传超大文件。

**规避**：在 `syncOnce()` 内部同样应用 `filterOversized`，或在 `copyLocalToRemote` 入口检查 `state.Size >= e.maxFileBytes`。推荐将过滤逻辑提取为 `engine.filterOversizedFromLocalFiles(files map[string]syncFileState)` 供 `initialSync` 和 `syncOnce` 复用。

### L4 `HotSyncStatus.OversizedFiles` 仅来自 `initialSync` vs 持续同步新增

**问题**：用户在会话中间新增了一个 80MB 文件，该文件不在 `snapshot.OversizedFiles` 里（initial scan 之后出现），但会被 syncOnce 跳过，无提示。

**规避**：本阶段（D-22）只需 initial scan 列表写入 last-session.json（SC#2 的验证锚是 60MB fixture 在 last-session.json 中）。持续同步新增的大文件被静默跳过视为 acceptable（Phase 37 e2e UAT 场景）。

### L5 `pkg/sftp` 版本：go.mod v1.13.10 vs 本地缓存 v1.13.9

**问题**：本地 GOPATH 缓存为 v1.13.9，但 go.mod 声明 v1.13.10。API 兼容（minor patch），`sftp.NewRequestServer` + `sftp.Handlers` 接口在两个版本均稳定。

**规避**：Executor 运行 `go mod download` 确保 v1.13.10 下载，或直接 `go test` 时自动下载。`sftp.Handlers{FileGet, FilePut, FileCmd, FileLister}` 四接口全部需要实现（不能为 nil）；fixture test 对不需要的接口返回 `ErrSSHFxOpUnsupported`。

### L6 `checkGitProxyEnabled` 在 doctor 中 LoadConfig 失败

**问题**：用户未 init 时 `cloudclaude.LoadConfig()` 返回错误，check 无法继续。

**规避**：错误时走 `newSkip("mount", "git_proxy_enabled", "配置未 init，跳过")`，不影响其他 check。与 `checkOversizedFilesCount` 中 nil snapshot → skip 同模式。

### L7 Phase 31 C5 防御在 Phase 36 场景下的有效性

**C5 背景**：v3.0 讨论过「local cwd 误同步覆盖远端」场景（Mutagen 反向清空）。

**Phase 36 影响**：D-01 git 闸门拦截了非 git 目录（最高风险场景），git 仓库内的 hot_sync 仍走 `chooseConflictWinner` 逻辑（`local.ModTime.After(remote.ModTime)` 决策）。C5 的核心防御（safety guard）是 `MOUNT_MUTAGEN_SAFETY_GUARD`，已在 Mutagen 路径实现，自研 hot_sync 路径通过 `resetRemote=false`（`ModeHotOnly`）/ `resetRemote=true`（`ModeFull` 先 reset staging）避免了反向清空。Phase 36 不改变此逻辑，**无需新增防御**。

### L8 `MOUNT_OVERSIZED_FILE_SKIPPED` 的 Message 占位符 vs `newWarn` 渲染

**问题**：`newWarn(domain, name, MOUNT_OVERSIZED_FILE_SKIPPED)` 调用 `fmt.Sprintf(entry.Message, args...)` 但 doctor check 没有具体 path/size。

**规避**：`checkOversizedFilesCount` 是 warn 级的聚合 check（N个文件），不走 `newWarn` helper，而是直接构造 `Check{}` 结构体（参照 `checkFUSEResidual` 的 `return Check{...}` 模式，直接设 `Message` 和 `NextAction`）。或者在 `newWarn` 调用时传递聚合描述字符串——这依赖 Message 模板可适配，executor 自由选择。

---

## Open Questions for Planner

无强约束未解决问题。以下为两个细节可由 executor 自由决策：

1. **`filterOversized` 函数位置**：private method on `HotSyncEngine` vs package-level function。private method 更内聚，推荐前者。

2. **`mount_sshfs_test.go` 中 fixture SFTP server 的 SSH 握手方式**：需要一个最小 SSH server 包装 sftp RequestServer。可参考 `golang.org/x/crypto/ssh` 的 `NewServerConn` API；testdata 证书用 `ssh.GenerateKey` 临时生成（与 `internal/cloudclaude/integration_test.go` 同模式）。

---

## Reading List for Executor（精确行号范围）

| 文件 | 必读行范围 | 目的 |
|------|----------|------|
| `cmd/cloud-claude/main.go` | 280–375（runRoot 全体） | F1 改动点：Getwd 前移位置、exit 常量定义 |
| `internal/cloudclaude/hot_sync.go` | 88–130（StartHotSync）、145–187（initialSync）、388–428（scanLocalSyncFiles） | F2 注入点 |
| `internal/cloudclaude/hot_sync.go` | 195–280（syncOnce）| L3 遗漏点——syncOnce 也需要过滤 |
| `internal/cloudclaude/last_session.go` | 全文（125 行） | F2 OversizedFile struct 落点 |
| `internal/cloudclaude/mount_strategy.go` | 280–320（tryModeReal，Full 路径） | F2 OversizedFiles 注入 + D-08 stderr 输出 |
| `internal/cloudclaude/mount_sshfs.go` | 19–50（mountSSHFS + sshfsCmd） | F3 字面量修改位置 |
| `internal/cloudclaude/config.go` | 全文（103 行） | F2 Config.HotSyncMaxFileMB + accessor 落点 |
| `internal/cloudclaude/ignore.go` | 100–115（LoadMountIgnorePatterns） | 确认 CLOUD_CLAUDE_NO_DEFAULT_IGNORE 行为（F4 doctor check） |
| `internal/cloudclaude/doctor/mount.go` | 全文（约 140 行） | F4 新增 5 check 的精确模板 |
| `internal/cloudclaude/doctor/doctor.go` | 226–234（mount domain block） | F4 新增 5 check 注册位置 |
| `internal/cloudclaude/doctor/check.go` | 全文（约 80 行） | F4 newPass/newWarn/newFail/newSkip/runWithTimeout API |
| `internal/cloudclaude/errcodes/codes.go` | 全文（最后 20 行 const 块） | F5 新增 2 const 插入位置 |
| `internal/cloudclaude/errcodes/mount.go` | 全文（约 70 行） | F5 MustRegister 模板 |
| `internal/cloudclaude/errcodes/explanations.go` | init() 函数（约 40–600 行） | F5 registerExplanation 调用模板（五段格式） |
| `cmd/cloud-claude/explain_test.go` | 全文（约 100 行） | F6 buildOnceExplainBin + runExplainBin 复用 |
| `internal/cloudclaude/errcodes/codes_test.go` | 全文（约 80 行） | F6 确认无需改动 + 命名限制（NextAction ≤80 rune） |

---

## Sources

### Primary（HIGH confidence — 代码直接阅读，VERIFIED）
- `internal/cloudclaude/hot_sync.go`（583 行全文阅读）
- `internal/cloudclaude/mount_sshfs.go`（95 行全文阅读）
- `internal/cloudclaude/mount_strategy.go`（400 行全文阅读）
- `internal/cloudclaude/last_session.go`（125 行全文阅读）
- `internal/cloudclaude/config.go`（103 行全文阅读）
- `internal/cloudclaude/doctor/mount.go`、`check.go`、`doctor.go` 相关段落
- `internal/cloudclaude/errcodes/codes.go`、`mount.go`、`explanations.go`、`codes_test.go`、`explanations_test.go`
- `cmd/cloud-claude/main.go`（runRoot 全文）
- `cmd/cloud-claude/explain.go`、`explain_test.go`
- `internal/cloudclaude/ignore.go`（LoadMountIgnorePatterns）
- `Makefile`（ci-gate target + ci-doctor-grep.sh）
- `go.mod`（go 1.25.7，pkg/sftp v1.13.10）
- `$(go env GOPATH)/pkg/mod/github.com/pkg/sftp@v1.13.9/request-interfaces.go`（FileReader interface）

---

## Metadata

**Confidence breakdown：**
- Standard stack：HIGH（go.mod 直接读取，pkg/sftp API 验证）
- Architecture：HIGH（全部代码直接阅读）
- Pitfalls：HIGH（代码结构验证，os.Getwd 位置地雷直接发现）

**Research date：** 2026-04-23
**Valid until：** 2026-05-23（代码稳定，30 天内有效）

---

## RESEARCH COMPLETE

**Phase：** 36 - 映射前置约束 + sshfs 内核缓存
**Confidence：** HIGH

### Key Findings

1. **最高优先级地雷**：`main.go::runRoot` 中 `os.Getwd()`（line 332）在 `AuthenticateAndWait`（line 302）之后，违反 D-01 时序要求；executor 必须前移 `os.Getwd()` 到 LoadConfig 之后（line ~295）。
2. **热同步改动最小路径**：`scanLocalSyncFiles` 无需改签名；在 `initialSync()` 拿到 `localFiles` 后后处理过滤大文件，并在 `syncOnce()` 应用同样过滤（L3 地雷）。
3. **SFTP counting test 路径**：`sftp.NewRequestServer` + 自定义 `Handlers{FileGet: countingFileReader{...}}`；pkg/sftp v1.13.10 已在 go.mod，无需新依赖。
4. **doctor mount 新增注册**：5 项 check 全加 `mount.go` + 在 `doctor.go` mount domain block（line 226-234）追加；`require_git_repo`、`oversized_files_count`、`git_proxy_enabled`、`default_ignore_loaded` 是本地 check，不需要 ensureRemote；只有 `sshfs_cache_args` 走 RemoteRunner。
5. **ci-gate 零改动**：所有断言（命名正则、≥200 字、schema_version=1、next_action）对新增 2 条 code + 5 项 check 全部自动覆盖。

### File Created
`.planning/phases/36-sshfs/36-RESEARCH.md`

### Confidence Assessment
| Area | Level | Reason |
|------|-------|--------|
| Standard Stack | HIGH | go.mod 直接读取，pkg/sftp API 通过 request-interfaces.go 验证 |
| Architecture | HIGH | 全部改动文件逐行阅读，行号精确到个位 |
| Pitfalls | HIGH | os.Getwd 位置地雷通过直接读代码发现，非推断 |

### Open Questions
无 CONTEXT.md 未覆盖的歧义。

### Ready for Planning
Research complete. Planner 可以直接创建 PLAN.md files。
