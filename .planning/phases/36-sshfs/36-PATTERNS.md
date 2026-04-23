# Phase 36: 映射前置约束 + sshfs 内核缓存 - Pattern Map

**Mapped:** 2026-04-23
**Files analyzed:** 18 (10 modified + 4 new + 4 extended)
**Analogs found:** 18 / 18

---

## File Classification

| 新增/修改文件 | Role | Data Flow | 最近 Analog | Match Quality |
|---|---|---|---|---|
| `internal/cloudclaude/config.go` | config | accessor-default | `config.go::EffectiveProxyCommands` | exact |
| `internal/cloudclaude/mount_sshfs.go` | param-literal | string-literal | `mount_sshfs.go:48-51 sshfsCmd` | exact line |
| `internal/cloudclaude/hot_sync.go` | service | CRUD+batch | `hot_sync.go` `HotSyncEngine`/`initialSync`/`scanLocalSyncFiles` | exact |
| `internal/cloudclaude/last_session.go` | model | JSON-serialize | `last_session.go` Phase 32 D-27 omitempty 追加模式 | exact |
| `internal/cloudclaude/mount_strategy.go` | orchestrator | request-response | `mount_strategy.go:222-230 snapshot.ConflictCount` 赋值 + `writeLastSessionWarn` | exact |
| `internal/cloudclaude/doctor/mount.go` | checker | request-response | `checkMergerfsBranches`（remote grep） + `checkFUSEResidual`（Details map） | exact |
| `internal/cloudclaude/errcodes/codes.go` | registry | none | `MOUNT_APFS_CASE_INSENSITIVE` const 末尾插入位置 | exact |
| `internal/cloudclaude/errcodes/mount.go` | registry | none | `MustRegister(Entry{...})` init() 模式 | exact |
| `internal/cloudclaude/errcodes/explanations.go` | docs | none | `registerExplanation(MOUNT_MUTAGEN_*)` 五段模板 | exact |
| `cmd/cloud-claude/main.go` | CLI entry | request-response | `runRoot` L287-295 LoadConfig 后、L362-367 keepalive 校验后 exit 模式 | exact |
| `cmd/cloud-claude/git_check.go` (**new**) | utility | request-response | `main.go` exec.Command + `errcodes.Format` 用法 | role-match |
| `cmd/cloud-claude/git_check_test.go` (**new**) | test | unit | `explain_test.go` 真实二进制构建模式 / 临时目录真实测试 | role-match |
| `internal/cloudclaude/hot_sync_oversized_test.go` (**new**) | test | batch | `hot_sync.go` `scanLocalSyncFiles` + `IgnoreMatcher` 用法 | role-match |
| `internal/cloudclaude/mount_sshfs_test.go` (**new**, F3) | test | request-response | `explain_test.go::buildOnceExplainBin` t.Skip 模式 + pkg/sftp Handlers | role-match |
| `internal/cloudclaude/last_session_test.go` (**extended**) | test | JSON | 现有序列化断言模式 | exact |
| `internal/cloudclaude/doctor/mount_test.go` (**extended**) | test | request-response | 现有 mock RemoteRunner 矩阵 | exact |
| `cmd/cloud-claude/explain_test.go` (**extended**) | test | subprocess | `buildOnceExplainBin + runExplainBin` 模式 | exact |
| `internal/cloudclaude/doctor/doctor.go` (**extended**) | orchestrator | none | `report.Checks = append(...)` mount domain block L226-234 | exact |

---

## Pattern Assignments

### `internal/cloudclaude/config.go` (config, accessor-default)

**Analog:** `internal/cloudclaude/config.go::EffectiveProxyCommands`

**当前 Config struct（lines 20-25）：**
```go
type Config struct {
    Gateway       string   `yaml:"gateway"`
    ShortID       string   `yaml:"short_id"`
    Password      string   `yaml:"password"`
    ProxyCommands []string `yaml:"proxy_commands,omitempty"`
}
```

**Accessor 默认值兜底模式（lines 27-33）：**
```go
func (c *Config) EffectiveProxyCommands() []string {
    if len(c.ProxyCommands) > 0 {
        return c.ProxyCommands
    }
    return DefaultProxyCommands
}
```

**复制方式（D-04）：**
```go
// 在 Config struct 追加字段（yaml tag 带 omitempty）：
HotSyncMaxFileMB int `yaml:"hot_sync_max_file_mb,omitempty"`

// 新增 accessor（紧跟 EffectiveProxyCommands 之后）：
const defaultHotSyncMaxFileMB = 50

func (c *Config) EffectiveHotSyncMaxFileMB() int {
    if c.HotSyncMaxFileMB <= 0 {
        return defaultHotSyncMaxFileMB
    }
    return c.HotSyncMaxFileMB
}
```

---

### `internal/cloudclaude/mount_sshfs.go` (param-literal, string-literal)

**Analog:** `mount_sshfs.go::mountSSHFS`

**待修改的字面量（lines 48-51）：**
```go
sshfsCmd := fmt.Sprintf(
    "sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10 -f",
    shellQuote(remotePath),
)
```

**目标（D-10，追加 4 个缓存参数，字面量顺序锁死）：**
```go
sshfsCmd := fmt.Sprintf(
    "sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300 -f",
    shellQuote(remotePath),
)
```

> **注意**：这是唯一修改点。函数签名、cleanup 结构、error 包装模式均不改动。

---

### `internal/cloudclaude/hot_sync.go` (service, CRUD+batch)

**Analog:** 文件本身的既有结构

**HotSyncEngine struct 扩展（lines 66-83，追加 2 个字段）：**
```go
type HotSyncEngine struct {
    // ...既有字段保留...
    maxFileBytes int64         // D-05：零值不熔断
    oversized    []OversizedFile // D-06/07：initialSync 填充，run() 只读
}
```

**HotSyncConfig struct 扩展（lines 27-36，追加 1 个字段）：**
```go
type HotSyncConfig struct {
    // ...既有字段保留...
    MaxFileBytes int64 // D-05：由 mount_strategy 注入 cfg.EffectiveHotSyncMaxFileMB()*1024*1024
}
```

**HotSyncStatus struct 扩展（lines 22-25，追加 1 个字段）：**
```go
type HotSyncStatus struct {
    ConflictCount  int
    OversizedFiles []OversizedFile // D-07：initialSync 结束后携带
}
```

**initialSync() 后处理注入点（RESEARCH 方案 A，lines 145-200 之间）：**
```go
// 在 scanLocalSyncFiles 返回后、resetRemote 分支之前插入：
if e.maxFileBytes > 0 {
    for rel, state := range localFiles {
        if state.Size >= e.maxFileBytes {
            e.oversized = append(e.oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
            delete(localFiles, rel)
        }
    }
}
```

**StartHotSync 返回 OversizedFiles（line 142，当前返回 `HotSyncStatus{}`）：**
```go
// 修改前：
return cleanup, HotSyncStatus{}, nil
// 修改后：
return cleanup, HotSyncStatus{OversizedFiles: engine.oversized}, nil
```

**syncOnce() 也需要过滤（L3 地雷，lines 219-220）：**
```go
// syncOnce 开头，scanLocalSyncFiles 之后追加：
if e.maxFileBytes > 0 {
    for rel, state := range localFiles {
        if state.Size >= e.maxFileBytes {
            delete(localFiles, rel) // 仅静默跳过，不更新 e.oversized（按 D-22 不刷屏）
        }
    }
}
```

> **两层熔断互补注记**：Phase 31 D-11 已有整目录级 >50MB 拒绝逻辑（在 `scanLocalSyncFiles` 的 `isHardcodedSkipDir` 之后、目录层面的 SkipDir 判断）；Phase 36 D-06 是单文件级熔断（在 walk 返回的 `localFiles` 上做后处理过滤）。两者互补不冲突：目录级保护「巨型 mono-repo」，文件级保护「正常 repo 内超大资源文件」。**executor 不得删除 D-11 的目录级过滤逻辑**。

---

### `internal/cloudclaude/last_session.go` (model, JSON-serialize)

**Analog:** `last_session.go` Phase 32 D-27 omitempty 追加模式（lines 26-30）

**既有 Phase 32 追加方式（lines 26-30）：**
```go
// [Phase 32 D-27 新增] 全部 omitempty + schema_version 保持 1（向后兼容）
TmuxSession    string `json:"tmux_session,omitempty"`
ClientRole     string `json:"client_role,omitempty"`
ReconnectCount int    `json:"reconnect_count,omitempty"`
```

**Phase 36 D-09 复制模式（追加到 LastSessionSnapshot struct 末尾）：**
```go
// [Phase 36 D-09 新增] omitempty + schema_version=1 不变（向后兼容）
OversizedFiles []OversizedFile `json:"oversized_files,omitempty"`
```

**包级新增 struct（与 DowngradeStep 同位置，lines 32-38 后）：**
```go
// OversizedFile 描述热同步阶段跳过的单个超大文件。
type OversizedFile struct {
    Path      string `json:"path"`        // cwd 相对路径
    SizeBytes int64  `json:"size_bytes"`
}
```

> `WriteLastSession` 和 `LoadLastSession` 无需改动，json.Marshal 自动处理新字段。

---

### `internal/cloudclaude/mount_strategy.go` (orchestrator, request-response)

**Analog:** `mount_strategy.go::tryModeReal` lines 222-230

**现有 hotStatus 赋值模式（lines 222-230）：**
```go
snapshot.ActualMode = mode.String()
snapshot.ConflictCount = hotStatus.ConflictCount

printBanner(cfg.Logger, mode, cfg.NoColor)
if hotStatus.ConflictCount > 0 {
    fmt.Fprintf(cfg.Logger, "⚠ 有 %d 个文件同步冲突，运行 cloud-claude sync conflicts 查看\n", hotStatus.ConflictCount)
}

writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
```

**Phase 36 修改（在 `snapshot.ConflictCount` 之后追加 D-08/D-09）：**
```go
snapshot.ActualMode = mode.String()
snapshot.ConflictCount = hotStatus.ConflictCount
snapshot.OversizedFiles = hotStatus.OversizedFiles // D-09：写入 last-session.json

// D-08：一次性 stderr 提示（在 printBanner 之前，不刷屏）
if n := len(hotStatus.OversizedFiles); n > 0 {
    limit := n
    if limit > 5 {
        limit = 5
    }
    fmt.Fprintf(cfg.Logger, "[!] 跳过大文件 %d 个（>%dMB），由 cold 兜底:\n",
        n, cfg.HotSyncMaxFileMB) // cfg.HotSyncMaxFileMB 需通过 MountConfig 透传
    for _, f := range hotStatus.OversizedFiles[:limit] {
        fmt.Fprintf(cfg.Logger, "  %s (%dMB)\n", f.Path, f.SizeBytes/1024/1024)
    }
    if n > 5 {
        fmt.Fprintf(cfg.Logger, "  ... 还有 %d 个，见 ~/.cloud-claude/last-session.json\n", n-5)
    }
}

printBanner(cfg.Logger, mode, cfg.NoColor)
writeLastSessionWarn(cfg.LastSessionPath, snapshot, cfg.Logger)
```

> `MountConfig` 需追加 `HotSyncMaxFileMB int` 字段，由 main.go 从 `cfg.EffectiveHotSyncMaxFileMB()` 注入。

---

### `internal/cloudclaude/doctor/mount.go` (checker, request-response)

#### F4 新增 5 项 check

**Analog 1 — `checkMergerfsBranches`（lines 27-59）→ `checkSSHFSCacheArgs` 精确镜像**

远端 runner + want 列表 + missing join 模式（lines 27-59）：
```go
func checkMergerfsBranches(ctx context.Context, runner RemoteRunner) Check {
    if runner == nil {
        return newSkip("mount", "mergerfs_branches", "未能连接远端容器，跳过")
    }
    mountOut, _, _ := runner.RunScript("mergerfs_mount", "mount | grep mergerfs | head -1")
    want := []string{
        "func.readdir=cor:4", "cache.attr=30", "cache.entry=30",
        "cache.readdir=true", "cache.files=off", "category.create=ff",
    }
    var missing []string
    for _, w := range want {
        if !strings.Contains(mountOut, w) {
            missing = append(missing, w)
        }
    }
    if len(missing) > 0 {
        return newFail("mount", "mergerfs_branches", errcodes.MOUNT_MERGERFS_FAILED,
            "mount 参数缺少 "+strings.Join(missing, ","))
    }
    return Check{
        Domain: "mount", Name: "mergerfs_branches", Status: StatusPass,
        Message: "...",
        Details: map[string]any{"mount": strings.TrimSpace(mountOut)},
    }
}
```

**复制方式 — `checkSSHFSCacheArgs`（几乎镜像，替换 mergerfs→sshfs）：**
```go
func checkSSHFSCacheArgs(ctx context.Context, runner RemoteRunner) Check {
    if runner == nil {
        return newSkip("mount", "sshfs_cache_args", "未能连接远端容器，跳过")
    }
    mountOut, _, _ := runner.RunScript("sshfs_mount", "mount | grep sshfs | head -1")
    want := []string{"cache=yes", "kernel_cache", "auto_cache", "cache_timeout=300"}
    var missing []string
    for _, w := range want {
        if !strings.Contains(mountOut, w) {
            missing = append(missing, w)
        }
    }
    if len(missing) > 0 {
        return newFail("mount", "sshfs_cache_args", errcodes.MOUNT_SSHFS_FAILED,
            "sshfs cache 参数缺少: "+strings.Join(missing, ","))
    }
    return Check{
        Domain: "mount", Name: "sshfs_cache_args", Status: StatusPass,
        Message: "sshfs cache 参数完整（cache=yes,kernel_cache,auto_cache,cache_timeout=300）",
        Details: map[string]any{"mount": strings.TrimSpace(mountOut)},
    }
}
```

---

**Analog 2 — `checkFUSEResidual`（lines 73-106）→ `checkOversizedFilesCount` Details map 模式**

Details map + 直接构造 Check{} 绕过 newWarn 占位符问题（lines 97-106）：
```go
entry, _ := errcodes.Lookup(errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT)
return Check{
    Domain: "mount", Name: "fuse_residual",
    Status:     StatusWarn,
    Code:       errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT,
    Message:    fmt.Sprintf(entry.Message, len(points), strings.Join(points, ",")),
    NextAction: entry.NextAction,
    Details:    map[string]any{"mountpoints": points},
}
```

**复制方式 — `checkOversizedFilesCount`（L8 地雷：不走 newWarn，直接构造）：**
```go
func checkOversizedFilesCount(ctx context.Context) Check {
    snap, err := cloudclaude.LoadLastSession()
    if err != nil || snap == nil {
        return newSkip("mount", "oversized_files_count",
            "last-session.json 不存在，跳过（STATE_LAST_SESSION_MISSING）")
    }
    n := len(snap.OversizedFiles)
    if n == 0 {
        return newPass("mount", "oversized_files_count", "上次会话无超大文件跳过记录")
    }
    top5 := make([]string, 0, 5)
    for i, f := range snap.OversizedFiles {
        if i >= 5 {
            break
        }
        top5 = append(top5, fmt.Sprintf("%s (%dMB)", f.Path, f.SizeBytes/1024/1024))
    }
    entry, _ := errcodes.Lookup(errcodes.MOUNT_OVERSIZED_FILE_SKIPPED)
    return Check{
        Domain:     "mount",
        Name:       "oversized_files_count",
        Status:     StatusWarn,
        Code:       errcodes.MOUNT_OVERSIZED_FILE_SKIPPED,
        Message:    fmt.Sprintf("上次会话跳过了 %d 个超大文件，由 cold sshfs 兜底", n),
        NextAction: entry.NextAction,
        Details:    map[string]any{"oversized_count": n, "top5_files": top5},
    }
}
```

---

**Analog 3 — `newPass/newWarn/newFail/newSkip` helpers（check.go lines 67-91）→ 本地 check 的简洁路径**

```go
// 本地 check 直接用 helpers（不需要 runner nil 判断）：
func checkRequireGitRepo(ctx context.Context) Check {
    cwd, err := os.Getwd()
    if err != nil {
        return newSkip("mount", "require_git_repo", "无法获取 cwd: "+err.Error())
    }
    if err := gitRevParseTopLevel(cwd); err != nil {
        return newFail("mount", "require_git_repo", errcodes.MOUNT_REQUIRE_GIT_REPO, cwd)
    }
    return newPass("mount", "require_git_repo", "当前目录位于 git 仓库内: "+cwd)
}

// doctor 包私有 helper（不能 import cmd/cloud-claude main package）：
func gitRevParseTopLevel(cwd string) error {
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    cmd.Dir = cwd
    return cmd.Run()
}
```

```go
func checkGitProxyEnabled(ctx context.Context) Check {
    cfg, err := cloudclaude.LoadConfig()
    if err != nil {
        return newSkip("mount", "git_proxy_enabled", "配置未 init，跳过: "+err.Error())
    }
    for _, cmd := range cfg.EffectiveProxyCommands() {
        if cmd == "git" {
            return newPass("mount", "git_proxy_enabled", "proxy_commands 包含 git")
        }
    }
    return newWarn("mount", "git_proxy_enabled", errcodes.AUTH_CONFIG_MISSING)
}

func checkDefaultIgnoreLoaded(ctx context.Context) Check {
    if os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE") == "1" {
        return newWarn("mount", "default_ignore_loaded", errcodes.AUTH_CONFIG_MISSING)
    }
    return newPass("mount", "default_ignore_loaded", "默认二进制黑名单已加载")
}
```

---

**doctor.go 注册点（lines 219-234，mount domain block 末尾追加 5 项）：**
```go
if want("mount") {
    ensureRemote()
    // ...既有 4 项保留...
    report.Checks = append(report.Checks, checkRequireGitRepo(ctx))
    report.Checks = append(report.Checks, checkOversizedFilesCount(ctx))
    report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_cache_args", timeout,
        func(c context.Context) Check { return checkSSHFSCacheArgs(c, remoteRunner) }))
    report.Checks = append(report.Checks, checkGitProxyEnabled(ctx))
    report.Checks = append(report.Checks, checkDefaultIgnoreLoaded(ctx))
}
```

> `require_git_repo`、`oversized_files_count`、`git_proxy_enabled`、`default_ignore_loaded` 是本地 check，**不**需要 `ensureRemote()`。只有 `sshfs_cache_args` 走 `runWithTimeout` + `remoteRunner`。

---

### `internal/cloudclaude/errcodes/codes.go` (registry, none)

**Analog:** `MOUNT_APFS_CASE_INSENSITIVE` const 定义（lines 133）

**插入位置（按字母序，O 在 A 之后，R 在 O 之后）：**
```go
// 当前末尾（line 133）：
MOUNT_APFS_CASE_INSENSITIVE      Code = "MOUNT_APFS_CASE_INSENSITIVE"

// Phase 36 D-16 在此行之后插入（字母序 A < O < R）：
MOUNT_OVERSIZED_FILE_SKIPPED Code = "MOUNT_OVERSIZED_FILE_SKIPPED"
MOUNT_REQUIRE_GIT_REPO       Code = "MOUNT_REQUIRE_GIT_REPO"
```

> 两个 code 都满足 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`，`codes_test.go::TestErrcodesRegistry` 自动覆盖。

---

### `internal/cloudclaude/errcodes/mount.go` (registry, none)

**Analog:** `MOUNT_APFS_CASE_INSENSITIVE` MustRegister 模式（lines 92-97）

**现有最后一条注册（lines 92-97）：**
```go
MustRegister(Entry{
    Code:       MOUNT_APFS_CASE_INSENSITIVE,
    Severity:   SeverityInfo,
    Message:    "检测到 macOS APFS case-insensitive 文件系统，已强制启用 two-way-resolved 同步模式",
    NextAction: "无需操作；如需 case-sensitive 行为请创建 case-sensitive APFS 卷",
})
```

**Phase 36 D-17 追加（init() 末尾）：**
```go
MustRegister(Entry{
    Code:       MOUNT_REQUIRE_GIT_REPO,
    Severity:   SeverityError,
    Message:    "当前目录 %s 不在 git 仓库内，cloud-claude 拒绝挂载以避免误同步整个家目录",
    NextAction: "cd 到 git 仓库根目录后重试，或在当前目录运行 git init 后再启动 cloud-claude",
})

MustRegister(Entry{
    Code:       MOUNT_OVERSIZED_FILE_SKIPPED,
    Severity:   SeverityWarn,
    Message:    "%s (%dMB) 超过 hot_sync_max_file_mb=%d 阈值，已跳过热同步，由 cold sshfs 兜底",
    NextAction: "如需提高阈值，编辑 ~/.cloud-claude/config.yaml::hot_sync_max_file_mb；或在 .gitignore 加入该路径以避免警告",
})
```

> `NextAction` ≤ 80 runes 是 `codes_test.go` 断言要求，写前需 `utf8.RuneCountInString` 数一下。

---

### `internal/cloudclaude/errcodes/explanations.go` (docs, none)

**Analog:** 任意 MOUNT_MUTAGEN_* 条目（五段模板）

**五段格式参考（`MOUNT_MUTAGEN_VERSION_SKEW`，lines 48-52）：**
```
触发场景：...
根本原因：...
复现方式：...（可选）
修复路径：...
关联文档：...
```

**Phase 36 D-18 复制方式（init() 末尾，MOUNT_* 域注释块内追加）：**
```go
// ── Phase 36 新增 ──────────────────────────────────────────────────────────
registerExplanation(MOUNT_REQUIRE_GIT_REPO, `触发场景：...（≥200 中文字符五段模板）
关联文档：.planning/REQUIREMENTS.md REQ-MOUNT-V31-01 / .planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md §D-11`)

registerExplanation(MOUNT_OVERSIZED_FILE_SKIPPED, `触发场景：...（≥200 中文字符五段模板）
关联文档：.planning/REQUIREMENTS.md REQ-MOUNT-V31-02 / REQ-MOUNT-V31-03 / .planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md §D-11（整目录级熔断，本阶段为单文件级，两层互补）`)
```

> 每条长说明必须 ≥ 200 中文字符（用 `utf8.RuneCountInString` 计数），`explanations_test.go::TestAllCodesHaveExplanations` 自动断言。两条均不得放入 `ExplainExempt`（D-18 明确）。

---

### `cmd/cloud-claude/main.go` (CLI entry, request-response)

**Analog:** `main.go::runRoot` keepalive 校验段（lines 362-367）

**现有 exitConfigError 模式（lines 362-367）：**
```go
if mountCfg.KeepAliveInterval > 0 && mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr,
        errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
    os.Exit(exitConfigError)
}
```

**os.Getwd() 前移 + requireGitRepo 注入（D-01，在 LoadConfig 成功后 L287-295 块之后、L298 NewEntryClient 之前）：**
```go
// [Phase 36 D-01] 前移 os.Getwd()：从 line 332 移到 LoadConfig 之后
cwd, err := os.Getwd()
if err != nil {
    fmt.Fprintln(os.Stderr, "错误: 无法获取当前工作目录: "+err.Error())
    os.Exit(exitInternalError)
}
if err := requireGitRepo(cwd); err != nil {
    fmt.Fprintln(os.Stderr, err.Error())
    os.Exit(exitConfigError)
}
// 删除原 line 332-336 的 os.Getwd() 块（避免重复声明 / shadowing）
```

> **L1 地雷**：原始 `os.Getwd()` 在 line 332（`AuthenticateAndWait` 之后），必须前移；若遗漏会导致已发起 SSH 连接后才做 git 检测，违反 REQ-MOUNT-V31-01 字面要求。

---

### `cmd/cloud-claude/git_check.go` (**new**, utility, request-response)

**Analog:** `main.go` 中 `exec.Command` 用法 + `errcodes.Format` 两段输出

**Imports 模式（参照 main.go 包头）：**
```go
package main

import (
    "fmt"
    "os/exec"

    "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)
```

**Core 实现（D-02）：**
```go
// requireGitRepo 在 cwd 执行 git rev-parse --show-toplevel；
// 非 git 仓库（含 git 未安装）返回包含 MOUNT_REQUIRE_GIT_REPO 格式化错误的 error。
func requireGitRepo(cwd string) error {
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    cmd.Dir = cwd
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_REQUIRE_GIT_REPO, cwd))
    }
    return nil
}
```

> 不依赖 viper/config，D-03 三种场景（worktree/submodule/detached HEAD）均由 `git rev-parse` 返回 0 自然覆盖。git 不可用时 `cmd.Run()` 返回非 nil，统一按「非 git 仓库」处理。

---

### `cmd/cloud-claude/git_check_test.go` (**new**, test, unit)

**Analog:** `explain_test.go` 临时目录真实测试风格（不 mock exec.Command，直接用真实目录）

**三种测试场景（D-20）：**
```go
package main

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func TestRequireGitRepo_InGitRepo(t *testing.T) {
    // 使用当前 workspace（cloud-cli-proxy 本身是 git 仓库）
    cwd, _ := os.Getwd()
    if err := requireGitRepo(cwd); err != nil {
        t.Errorf("git 仓库目录应 pass，got error: %v", err)
    }
}

func TestRequireGitRepo_NotAGitRepo(t *testing.T) {
    dir := t.TempDir() // 临时空目录（不是 git 仓库）
    if err := requireGitRepo(dir); err == nil {
        t.Error("非 git 仓库应返回 error")
    }
}

func TestRequireGitRepo_GitNotAvailable(t *testing.T) {
    if _, err := exec.LookPath("git"); err != nil {
        t.Skip("git 不可用，跳过")
    }
    // 通过 PATH="" 模拟 git 不存在
    dir := t.TempDir()
    // 直接用 requireGitRepo（内部 exec.Command 找不到 git）
    // 需要 mock execCommandFunc 或测试 cmd.Dir=dir + empty PATH
    // executor 自由选最简方案（e.g. PATH 环境变量截断 + subprocess 隔离）
}
```

---

### `internal/cloudclaude/hot_sync_oversized_test.go` (**new**, test, batch)

**Analog:** `hot_sync.go` `scanLocalSyncFiles` + `IgnoreMatcher` 直接调用

**Fixture 场景（D-20，边界值覆盖 D-21 discreation）：**
```go
package cloudclaude

import (
    "os"
    "path/filepath"
    "testing"
)

// TestHotSyncOversized_IgnoreHits_NotCounted 断言 ignore 命中的 50MB+ 文件不进 OversizedFiles
func TestHotSyncOversized_IgnoreHits_NotCounted(t *testing.T) {
    dir := t.TempDir()
    // 创建 60MB 文件（未 ignore → 进 oversized）
    createFixtureFile(t, filepath.Join(dir, "bigfile.bin"), 60*1024*1024)
    // 创建 60MB 文件（已 ignore → 不进 oversized）
    createFixtureFile(t, filepath.Join(dir, "ignored.bin"), 60*1024*1024)
    // 创建 30MB 文件（未 ignore，小于阈值 → 不进 oversized）
    createFixtureFile(t, filepath.Join(dir, "smallfile.bin"), 30*1024*1024)

    matcher := NewIgnoreMatcher(dir, []string{"ignored.bin"})
    localFiles, _ := scanLocalSyncFiles(dir, matcher)

    const maxBytes = 50 * 1024 * 1024
    var oversized []OversizedFile
    for rel, state := range localFiles {
        if state.Size >= maxBytes {
            oversized = append(oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
            delete(localFiles, rel)
        }
    }
    // 只有 bigfile.bin 进 oversized（ignored.bin 被 ignore 跳过，smallfile.bin < 50MB）
    if len(oversized) != 1 || oversized[0].Path != "bigfile.bin" {
        t.Errorf("expected [bigfile.bin], got %v", oversized)
    }
    // 剩余 localFiles 只含 smallfile.bin（bigfile 被过滤，ignored 被 ignore 跳过）
    if _, ok := localFiles["smallfile.bin"]; !ok {
        t.Error("smallfile.bin 应留在 localFiles")
    }
}
```

---

### `internal/cloudclaude/mount_sshfs_test.go` (**new**, test, request-response)

**Analog:** `explain_test.go::buildOnceExplainBin` t.Skip 模式

**Skip 判断（D-11 / D-20）：**
```go
func TestSSHFSCacheHitsKernelPageCache(t *testing.T) {
    if _, err := exec.LookPath("sshfs"); err != nil {
        t.Skip("sshfs 未安装，跳过 cache 计数测试")
    }
    // 1. 启动 fixture SSH+SFTP server（pkg/sftp NewRequestServer + countingFileReader）
    // 2. 真实 sshfs 挂载到 t.TempDir()
    // 3. 同文件 os.ReadFile 2 次
    // 4. 断言 server-side read count == 1
}
```

**pkg/sftp Handlers 四接口全实现（L5 地雷规避）：**
```go
// sftp.Handlers{FileGet, FilePut, FileCmd, FileLister} 都不能为 nil
// 不需要的接口返回 ErrSSHFxOpUnsupported
handlers := sftp.Handlers{
    FileGet:  &countingFileReader{root: fixtureDir},
    FilePut:  sftp.InMemHandler().FilePut,  // no-op
    FileCmd:  sftp.InMemHandler().FileCmd,  // no-op
    FileLister: sftp.InMemHandler().FileLister,
}
```

---

### `internal/cloudclaude/last_session_test.go` (**extended**, test, JSON)

**Analog:** 现有序列化测试（json.Marshal/Unmarshal + schema_version=1 断言）

**扩展点（D-20）：**
```go
func TestLastSession_OversizedFiles_Roundtrip(t *testing.T) {
    snap := LastSessionSnapshot{
        SchemaVersion: 1,
        OversizedFiles: []OversizedFile{
            {Path: "assets/video.mp4", SizeBytes: 60 * 1024 * 1024},
        },
    }
    data, _ := json.Marshal(snap)
    var got LastSessionSnapshot
    json.Unmarshal(data, &got)
    if got.SchemaVersion != 1 {
        t.Errorf("schema_version 应为 1，got %d", got.SchemaVersion)
    }
    if len(got.OversizedFiles) != 1 || got.OversizedFiles[0].Path != "assets/video.mp4" {
        t.Errorf("OversizedFiles roundtrip 失败: %+v", got.OversizedFiles)
    }
}

func TestLastSession_OversizedFiles_OmitemptyEmpty(t *testing.T) {
    snap := LastSessionSnapshot{SchemaVersion: 1}
    data, _ := json.Marshal(snap)
    // omitempty：空切片不出现在 JSON 中
    if strings.Contains(string(data), "oversized_files") {
        t.Error("空 OversizedFiles 应被 omitempty 省略")
    }
}
```

---

### `internal/cloudclaude/doctor/mount_test.go` (**extended**, test, request-response)

**Analog:** 现有 mock RemoteRunner 矩阵模式（pass/warn/fail/skip）

**5 项新 check 矩阵扩展模式：**
```go
// checkSSHFSCacheArgs — pass（含全部 4 参数）
// checkSSHFSCacheArgs — fail（缺 kernel_cache）
// checkOversizedFilesCount — pass（OversizedFiles 为空）
// checkOversizedFilesCount — warn（OversizedFiles 含 1 条）
// checkOversizedFilesCount — skip（last-session.json 不存在）
// checkRequireGitRepo — pass（cwd = git 仓库）
// checkRequireGitRepo — fail（cwd = 临时目录）
// checkGitProxyEnabled — pass（config proxy_commands 含 git）
// checkGitProxyEnabled — warn（config 不含 git）
// checkDefaultIgnoreLoaded — pass（env 未设）
// checkDefaultIgnoreLoaded — warn（CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1）
```

---

### `cmd/cloud-claude/explain_test.go` (**extended**, test, subprocess)

**Analog:** `TestExplain_KnownCode_Exit0`（lines 52-67）+ `TestExplain_UnknownCode_Exit4`（lines 71-83）

**buildOnceExplainBin + runExplainBin 复用模式（lines 16-48）：**
```go
func buildOnceExplainBin(t *testing.T) string { ... }

func runExplainBin(t *testing.T, bin string, args ...string) (exitCode int, stdout, stderr string) { ... }
```

**Phase 36 D-20 新增 2 条测试（复制 TestExplain_KnownCode_Exit0 + 追加 ≥200 字断言）：**
```go
func TestExplain_MountRequireGitRepo_Exit0_MinLen(t *testing.T) {
    bin := buildOnceExplainBin(t)
    code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_REQUIRE_GIT_REPO")
    if code != 0 {
        t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
    }
    if !strings.Contains(stdout, "MOUNT_REQUIRE_GIT_REPO") {
        t.Errorf("stdout 未包含错误码字面量")
    }
    if n := utf8.RuneCountInString(stdout); n < 200 {
        t.Errorf("stdout 字符数 %d < 200（D-18 要求 ≥200 中文字符）", n)
    }
}

func TestExplain_MountOversizedFileSkipped_Exit0_MinLen(t *testing.T) {
    bin := buildOnceExplainBin(t)
    code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_OVERSIZED_FILE_SKIPPED")
    if code != 0 {
        t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
    }
    if n := utf8.RuneCountInString(stdout); n < 200 {
        t.Errorf("stdout 字符数 %d < 200", n)
    }
}
```

> `utf8.RuneCountInString` 计中文字符数（SC#5 字面要求），不是 `len(stdout)`（byte 数）。

---

## Shared Patterns

### 错误码格式化输出
**Source:** `internal/cloudclaude/errcodes/codes.go::Format`
**Apply to:** `git_check.go`、`main.go` 中所有 errcode 相关 fmt.Fprintln

```go
// 统一两段格式：[CODE] Message\n  建议: NextAction
errcodes.Format(errcodes.MOUNT_REQUIRE_GIT_REPO, cwd)
```

### exitConfigError=4 退出模式
**Source:** `cmd/cloud-claude/main.go` lines 282-285 / 362-367
**Apply to:** `main.go` 中 git_check 失败路径

```go
fmt.Fprintln(os.Stderr, err.Error())
os.Exit(exitConfigError) // = 4，用于「用户输入/环境错误」
```

### omitempty JSON 字段追加（schema_version 不变）
**Source:** `internal/cloudclaude/last_session.go` Phase 32 D-27 注释块（lines 26-30）
**Apply to:** `last_session.go` OversizedFiles 字段追加

```go
// [Phase 36 D-09 新增] schema_version 保持 1（omitempty 向后兼容）
OversizedFiles []OversizedFile `json:"oversized_files,omitempty"`
```

### t.Skip() FUSE 测试卫士
**Source:** `explain_test.go::buildOnceExplainBin` 的 `os.Stat(bin)` 存在性检查模式
**Apply to:** `mount_sshfs_test.go` sshfs/fuse 可用性检查

```go
if _, err := exec.LookPath("sshfs"); err != nil {
    t.Skip("sshfs 未安装，跳过")
}
```

### runWithTimeout 5s 包装
**Source:** `doctor/check.go::runWithTimeout`（lines 35-63）
**Apply to:** `checkSSHFSCacheArgs`（需要 remoteRunner 的远端 check）

```go
report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_cache_args", timeout,
    func(c context.Context) Check { return checkSSHFSCacheArgs(c, remoteRunner) }))
```

---

## No Analog Found

无（所有 18 个文件均找到精确或 role-match 级 analog）。

---

## 两层熔断互补说明

| 层 | 实现位置 | 触发粒度 | 保护场景 |
|---|---|---|---|
| 整目录级（Phase 31 D-11） | `scanLocalSyncFiles` → `isHardcodedSkipDir` / IgnoreMatcher `SkipDir` | 目录 >50MB | 巨型 mono-repo 误同步 |
| 单文件级（Phase 36 D-06） | `initialSync()` 后处理 + `syncOnce()` 后处理 | 单文件 ≥ `MaxFileBytes` | 正常 repo 内超大资源文件（video/model） |

**关键约束**：两层必须同时存在，executor 不得将 Phase 36 的单文件过滤合并到 `scanLocalSyncFiles` 内部，以免影响 Phase 31 的目录级 `SkipDir` 语义。

---

## Metadata

**Analog search scope:** `internal/cloudclaude/`, `cmd/cloud-claude/`, `internal/cloudclaude/doctor/`, `internal/cloudclaude/errcodes/`
**Files scanned:** 15 个源文件全文阅读
**Pattern extraction date:** 2026-04-23
