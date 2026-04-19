---
phase: 31-cli
plan: 02-mount-three-layer
subsystem: internal/cloudclaude
tags: [mount, mutagen, mergerfs, sshfs, strategy, askpass, watcher, last-session, exitcodes, wave-2]
requires:
  - "Plan 01 errcodes 包（MOUNT_* / NET_OAUTH_* 15 条 Code）"
  - "Plan 01 cloudclaude.ExtractMutagenBinary（占位 stub，CI 替换为真二进制）"
  - "Plan 01 cloudclaude.IsCaseInsensitiveFS（APFS 探测）"
  - "Phase 30 AuthResponse.{ClaudeAccountID,ImageVersion,SupportsMutagen,SupportsMergerfs}"
provides:
  - "MountWorkspace(connA, connB, cfg) — 顶层入口 + Mode 状态机 + 三段式中文进度 + banner + last-session.json"
  - "Mode 枚举 + ParseMode（cobra flag 字面值 ↔ Mode）"
  - "MountConfig（Mode/KeepAlive*/AccountID/ImageVersion/SupportsMutagen/SupportsMergerfs/Cwd/NoColor/Logger/LastSessionPath/SyncSessionLock + 测试 hooks）"
  - "ConnectAndRunClaudeV3(cfg, args, cwd, proxyCmds, mountCfg, authResp) — Phase 31 主入口"
  - "ConnectAndRunClaude（v2.0 兼容入口）签名不变 + 内部转 V3 + Mode=ModeSSHFSOnly"
  - "mountMutagen / mountMerge / RemoveBranch / MutagenSyncConfig / MutagenStatus / MutagenHealthCheck"
  - "ExitOK..ExitMountForceFailed 9 个命名退出码常量（与 v2.0 0-5 对齐 + 新增 OAuth/mount 6-8）"
affects:
  - "Wave 3 Plan 03 OAuth 检查：在 ssh.go ConnectAndRunClaudeV3 已留 TODO(plan-03) hook 点（mount ready 后、runClaude 前）"
  - "Wave 3 Plan 03 cmd/cloud-claude/sync.go：banner 后 ⚠ N 个文件同步冲突 ... 已就位，conflict 计数由 Plan 03 mount_mutagen.go sync list --template 解析"
  - "Phase 32 多端冲突：MountConfig.SyncSessionLock 已是 hook 字段（本阶段 noop），Phase 32 注入真实锁实现"
  - "Phase 34 doctor：复用 MutagenHealthCheck + 读 ~/.cloud-claude/last-session.json 展示降级链；exitcodes.go 表供 explain 子命令引用"
tech-stack:
  added: []
  patterns:
    - "依赖注入 deps struct（mountMutagenDeps）让 9 个外部调用点全部可 mock，单测无需真实 ssh/exec"
    - "hooks 字段（strategyHooks）让 mount_strategy 三层 mount 在测试中替换为函数注入 → 12 降级矩阵纯单测覆盖"
    - "codedError interface（Code() + Reason()）让 mount_strategy 通过 errors.As 识别子层 sentinel error，避免字符串比对"
    - "tmp+rename 原子写 last-session.json"
    - "ANSI 着色 + 非 TTY/NO_COLOR 自动关色（colorize helper）"
    - "manual scan os.Args 剥离 --mount-mode（DisableFlagParsing=true 下绕过 cobra）"
    - "TDD RED→GREEN 双 commit：exitcodes 单测先证伪再实现"
key-files:
  created:
    - "internal/cloudclaude/mount_mutagen.go"
    - "internal/cloudclaude/mount_mutagen_test.go"
    - "internal/cloudclaude/mount_merge.go"
    - "internal/cloudclaude/mount_strategy.go"
    - "internal/cloudclaude/mount_strategy_test.go"
    - "internal/cloudclaude/exitcodes.go"
    - "internal/cloudclaude/exitcodes_test.go"
  modified:
    - "internal/cloudclaude/ssh.go (新增 ConnectAndRunClaudeV3；ConnectAndRunClaude 转 V3 + sshfs-only)"
    - "cmd/cloud-claude/main.go (新增 --mount-mode flag 注册 + 手工解析 + 切到 ConnectAndRunClaudeV3)"
decisions:
  - "task 2.2 production code 由前次中断会话遗留为 untracked（mount_merge/mutagen/strategy.go），本次审阅确认与 PLAN 完全一致后直接补测试 + 一并提交（5cee2a1），避免重写 ~950 行"
  - "Auto/SSHFS-fail+Merge-ok → SSHFSOnly（实际下挫 2 档，因 Auto 试 [Full,MutagenOnly,SSHFSOnly]，MutagenOnly 在当前 hook mock 无 mergerfs 依赖故 trySSHFS 是其前提，方案整合见 12 降级矩阵第 1 行）"
  - "Auto/SSHFS-ok+Merge-fail → MutagenOnly（mergerfs 失败但 mutagen 已起来 → 单层 mutagen 兜底，不退回纯 sshfs）"
  - "Mutagen 安全门为 Fatal 不可降级（D-13）：alpha 空 + 远端 /workspace-hot 非空 → 直接返回错误，调用方退出非 0；防御 v2.0 升级用户首次跑空仓库会被覆盖远端代码"
  - "ConnectAndRunClaude（v2.0 入口）保留签名 + 内部默认 ModeSSHFSOnly：旧调用零回归，新调用走 V3"
  - "rootCmd 因 DisableFlagParsing=true 仍保留：runRoot 顶部手工解析 --mount-mode 后从 args 剥离，剩余 args 透传给远端 claude"
  - "exitcodes.go 新增不动 cmd/cloud-claude/main.go 现有 exit* 常量（rename 留 Phase 34），仅 Plan 03 新代码引用 cloudclaude.Exit*"
metrics:
  duration_seconds: 480
  duration_human: "~8 分钟"
  tasks_completed: 4
  files_created: 7
  files_modified: 2
  commits: 5
  completed_at: "2026-04-19T09:21:00Z"
---

# Phase 31 Plan 02: 三层 mount + ConnectAndRunClaudeV3 + exitcodes Summary

## One-Liner

把 v2.0 单层 sshfs 升级为「Mutagen 热同步 + sshfs 冷兜底 + mergerfs 联合视图」三层架构，落地 `--mount-mode={auto,full,mutagen-only,sshfs-only}` 显式降级状态机；M13 防御「禁止静默降级」由 stderr banner + last-session.json downgrade_chain 两处留痕；12 个降级矩阵 + 9 个 mutagen 专项 + 5 个 exitcodes 测试全部 PASS。

## Goal Achievement

| 维度 | 目标 | 实际 | 状态 |
|------|------|------|------|
| 4 个 mount_*.go 拆分 | sshfs/mutagen/merge/strategy 各一文件 | 全部就位 | ✅ |
| 4 个辅助文件 | colors/askpass/last_session/sshfs_watcher | 全部就位（Task 2.1） | ✅ |
| 12 降级矩阵单测 | Mode × failure 注入 | TestMountStrategy_DowngradeMatrix 12 子用例 PASS | ✅ |
| Mutagen 专项测试 | safety guard / 50MB / 版本握手 / banner / APFS / downgrade banner | 9 个用例全 PASS（含 cleanup / nil-conn 防回归） | ✅ |
| ConnectAndRunClaudeV3 | 新主入口 + AuthResponse 字段补全 | 实现完成 | ✅ |
| ConnectAndRunClaude v2.0 兼容 | 签名不变 + 内部 sshfs-only | 实现完成 | ✅ |
| --mount-mode flag | cobra 注册 + 手工解析 + ParseMode | 实现完成（非法值退出 4） | ✅ |
| exitcodes 9 常量 | v2.0 0-5 对齐 + OAuth/mount 6-8 | 实现完成（5 单测 PASS） | ✅ |
| 关键命令字符级一致 | mergerfs/setfattr/mutagen 全部硬编码 | grep 全部命中（详见 Verification） | ✅ |
| 整仓 go test ./... | 不破坏 v2.0 回归 | 全 PASS | ✅ |

## Public Interfaces 兑现清单

### `internal/cloudclaude/mount_strategy.go`

```go
type Mode int  // ModeAuto/Full/MutagenOnly/SSHFSOnly/Failed
func (m Mode) String() string
func ParseMode(s string) (Mode, error)

type MountConfig struct {
    Mode              Mode
    KeepAliveInterval time.Duration   // Phase 32 注入
    KeepAliveCountMax int             // Phase 32 注入
    ClaudeAccountID   string          // 来自 AuthResponse
    ImageVersion      string          // 来自 AuthResponse
    SupportsMutagen   bool            // 来自 AuthResponse
    SupportsMergerfs  bool            // 来自 AuthResponse
    Cwd               string
    NoColor           bool
    Logger            io.Writer
    LastSessionPath   string
    SyncSessionLock   func(accountID string) (release func(), err error)
}

func MountWorkspace(connA, connB *ssh.Client, cfg MountConfig) (cleanup func(), mode Mode, err error)
```

### `internal/cloudclaude/mount_mutagen.go`

```go
type MutagenSyncConfig struct {
    AlphaCwd, BetaPath, SSHUser, SSHHost   string
    SSHPort                                int
    Password, ClaudeAccountID, SessionName string
}

type MutagenStatus struct { DaemonReady, AgentReady, SyncReady bool; Conflicts int; Reason string }
func MutagenHealthCheck(daemonReady, agentReady, syncReady bool, conflicts int) MutagenStatus
```

（mountMutagen 保持 unexported，由 mount_strategy 内部调度）

### `internal/cloudclaude/mount_merge.go`

```go
// mountMerge 内部调用；公开仅 RemoveBranch（cold 抖动 watcher 触发）
func RemoveBranch(connA *ssh.Client, branchPath, target string) error
```

### `internal/cloudclaude/ssh.go`

```go
func ConnectAndRunClaudeV3(cfg SSHConfig, claudeArgs []string, cwd string,
    proxyCommands []string, mountCfg MountConfig, authResp *AuthResponse) (int, error)
func ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string) (int, error)
```

### `internal/cloudclaude/exitcodes.go`

```go
const (
    ExitOK            = 0  // v2.0 对齐
    ExitAuthFailed    = 1
    ExitNetworkError  = 2
    ExitTimeout       = 3
    ExitConfigError   = 4
    ExitInternalError = 5
    ExitOAuthNotFound    = 6  // Phase 31 新增
    ExitOAuthExpired     = 7
    ExitMountForceFailed = 8
)
```

## 12 降级矩阵最终决策表

| # | cfg.Mode | mutagen | sshfs | merge | finalMode | 备注 |
|---|----------|---------|-------|-------|-----------|------|
| 1 | Auto | fail | ok | ok | SSHFSOnly | mutagen 失败 → MutagenOnly（无 mutagen 故无法）→ SSHFSOnly |
| 2 | Auto | ok | fail | ok | MutagenOnly | sshfs 失败 → mergerfs 起不来 → 单层 mutagen 兜底 |
| 3 | Auto | ok | ok | fail | MutagenOnly | mergerfs 失败 → 单层 mutagen 兜底 |
| 4 | Auto | ok | ok | ok | Full | happy path |
| 5 | Auto | fail | fail | fail | Failed | 三档全失败 + err |
| 6 | Full | fail | * | * | Failed | force mode 不允许降级 → MOUNT_FORCE_MODE_FAILED |
| 7 | Full | ok | fail | * | Failed | 同上 |
| 8 | Full | ok | ok | fail | Failed | 同上 |
| 9 | MutagenOnly | ok | * | * | MutagenOnly | mutagen 单档跑 |
| 10 | MutagenOnly | fail | * | * | Failed | force fail |
| 11 | SSHFSOnly | * | ok | * | SSHFSOnly | sshfs 单档跑（v2.0 路径） |
| 12 | SSHFSOnly | * | fail | * | Failed | force fail |

> 注：行 1 的 SSHFSOnly 是当前 hook mock 下的行为（mock 实现简单，不区分 mergerfs 是否依赖 sshfs）。生产路径 Auto/MutagenOnly 会先尝试单层 mutagen，本测试通过 hooks 注入的 trySSHFS 直接失败 → 转 SSHFSOnly。Plan 03 集成测试将复核生产路径。

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 2.1 | 拆分 mount.go + 4 辅助文件骨架（colors/askpass/last_session/sshfs_watcher） | 92e0feb | mount.go / mount_sshfs.go / colors.go / askpass.go(+test) / last_session.go(+test) / sshfs_watcher.go(+test) |
| 2.2 | 三层 mount 实现 + 12 降级矩阵 + 9 mutagen 专项单测 | 5cee2a1 | mount_mutagen.go(+test) / mount_merge.go / mount_strategy.go(+test) |
| 2.3 | ConnectAndRunClaudeV3 + cobra --mount-mode flag 接线 | 9a75372 | ssh.go / cmd/cloud-claude/main.go |
| 2.4 (RED) | exitcodes 5 个失败测试（编译失败证伪） | 3676bc7 | exitcodes_test.go |
| 2.4 (GREEN) | exitcodes.go 9 个命名退出码常量 | 0ed84a4 | exitcodes.go |

## Test Coverage

```
mount_mutagen_test.go (9 用例):
  Test_SafetyGuard_AlphaEmptyBetaNonEmpty  PASS
  Test_50MBReject                          PASS
  Test_VersionSkew                         PASS
  Test_DaemonStartIdempotent               PASS  ("daemon already started" 视为 OK)
  Test_MutagenHappyPath_CleansUpOnTerminate PASS
  Test_MutagenHealthCheck_ReasonsCorrect   PASS  (5 子用例 — daemon/agent/sync/conflicts/ok)
  Test_WriteMutagenDefaultsYML             PASS  (yaml 包含 ignore 列表关键项)
  Test_VersionSkewSkippedWhenConnNil       PASS  (防 nil-deref 回归)
  Test_CleanupRunsTerminateAndAskpass      PASS  (cleanup → terminate + askpass)

mount_strategy_test.go (12 + 7 = 19 用例):
  TestMountStrategy_DowngradeMatrix        PASS  (12 子用例，详见上表)
  Test_BannerColors                        PASS  (3 子用例 — 非 TTY / NO_COLOR / 模式标签)
  Test_APFSCaseInsensitive_WritesLastSession PASS
  Test_Downgrade_BannerEachStep            PASS
  Test_Downgrade_CapabilityFromAuthResp    PASS  (2 子用例 — SupportsMutagen=false / SupportsMergerfs=false)
  Test_ParseMode                           PASS  (5 个有效值 + 1 个非法值)
  Test_ForceMode_FailureUsesForceCode      PASS
  Test_BuildSessionName                    PASS  (确定性 + 不同 cwd 不同 hash + anon fallback)
  Test_ExtractErrCode_FallbackForceFailed  PASS

exitcodes_test.go (5 用例):
  Test_ExitCodes_Unique                    PASS
  Test_ExitCodes_PosixLimit                PASS
  Test_ExitCodes_V2Compat                  PASS  (0-5 与 cmd/cloud-claude/main.go 对齐)
  Test_ExitCodes_NewCodesNotConflictV2     PASS
  Test_ExitCodes_NamesPresent              PASS

整仓回归：go test ./... -count=1 全 PASS（含 v2.0 mount_test / ssh_doctor_test）。
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 2.2 production code 已存在于 untracked 文件**

- **Found during:** 接手时 `git status` 发现 mount_merge.go / mount_mutagen.go / mount_strategy.go 三个 untracked
- **Issue:** 用户指令说 "若 previous attempt 中断，已 reset 到 56ffd6f"，但实际 HEAD 在 92e0feb（Task 2.1 已提交），且 untracked 三个文件含有约 950 行成熟实现，与 PLAN 完全契合（关键命令字符串、deps 注入模式、状态机骨架全对应）。
- **Fix:** 选择不重写。审阅三个文件 → 确认 PLAN action 全部覆盖 + grep 关键字符串 + go build 通过 → 直接补 mount_mutagen_test.go / mount_strategy_test.go 后一并提交。这避免了 ~30 分钟无意义重写。
- **Files modified:** 无，三个 untracked 文件直接 git add；新增两个测试文件。
- **Commit:** 5cee2a1
- **影响:** Task 2.2 严格 TDD（PLAN 标记 tdd="true"）实际改为「production + tests 同 commit」。Task 2.4 仍走严格 TDD（RED 3676bc7 → GREEN 0ed84a4）。

**2. [Rule 3 - Blocking] mount_merge.go 接手时 gofmt -l 有警告**

- **Found during:** Task 2.2 commit 前
- **Issue:** untracked mount_merge.go 由前次会话生成，未 gofmt。
- **Fix:** `gofmt -w internal/cloudclaude/mount_{merge,mutagen,strategy}.go`，全部消化。
- **Commit:** 5cee2a1

### Deferred Issues (Out of Scope)

- `internal/cloudclaude/envcheck.go` / `internal/cloudclaude/ssh_doctor_test.go` 仍然 gofmt -l 有警告（pre-existing，Plan 01 SUMMARY 已记录，本 plan 同样按 SCOPE BOUNDARY 不动）。
- 真实 Mutagen sync session 集成测试（带 docker-in-docker）→ 留 Plan 03 集成测试落地。
- 真实 sshfs 抖动 watcher 端到端验证（需 docker stop/start sshfs 模拟）→ Plan 03。

### No Architectural Changes

未触发 Rule 4。所有 mount 接口字段、Mode 枚举、cobra flag 等架构决策都在 PLAN 文档中明确，本 plan 仅按 PLAN 落地。

## Authentication Gates

无（本 plan 全部本地 unit test + git commit；远端 SSH/Mutagen 集成测试留 Plan 03）。

## Known Stubs

| 文件 | 行 | Stub 类型 | 由谁补齐 |
|------|----|----|----|
| `internal/cloudclaude/mutagen_bin/{darwin,linux}_{amd64,arm64}/mutagen` | 整文件 | 占位 shell stub（Plan 01 遗留） | CI build-images workflow `bash scripts/fetch-mutagen-bins.sh` |
| `internal/cloudclaude/mount_strategy.go::tryModeReal` | 311-356 | 生产路径已写但未端到端测过（仅 hooks 单测） | Plan 03 集成测试或 Phase 34 doctor 真实部署验证 |
| `cleanup` 注释 `// Plan 03 将在此插入 OAuth 检查` | ssh.go ~107 | TODO 占位 | Plan 03 Task 3.2 |

**Stub 是否阻塞 plan goal？** 不阻塞。本 plan goal 是「三层 mount 状态机 + flag 接线 + exitcodes 集中管理」，全部以单元测试覆盖；端到端集成是 Plan 03 / Phase 34 的明确分工。

## 与 Plan 03 的接口契约

1. **OAuth 检查 hook 点**：`internal/cloudclaude/ssh.go::ConnectAndRunClaudeV3` 在 mount ready 之后、`runClaude` 之前留有 `TODO(plan-03)` 注释（具体行号见 `git grep 'TODO(plan-03)' internal/cloudclaude/ssh.go`）。Plan 03 Task 3.2 在此插入 `CheckOAuthCredentials(connA, mountCfg.ClaudeAccountID)`，命中 NET_OAUTH_NOT_FOUND/EXPIRED 时返回 `cloudclaude.ExitOAuthNotFound/Expired`。
2. **conflict count 上报**：`mount_strategy.MountWorkspace` 已支持 `conflicts > 0` 时输出 `⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`。conflicts 来自 `mountMutagen` 返回的第二个返回值（当前总返回 0，因为本 plan 未集成 `mutagen sync list --template`）。Plan 03 在 mountMutagen 内部加上 sync list 调用 + parse template 即可。
3. **sync 子命令**：`cmd/cloud-claude/sync.go` 由 Plan 03 创建，复用 `cloudclaude.Mode` 与 `cloudclaude.Exit*` 常量。
4. **exitcodes 引用**：Plan 03 OAuth 检查 + mount_strategy ModeForce 失败路径必须使用 `cloudclaude.Exit*` 常量而非裸数字。

## Limitations / Trade-offs

1. **conflict 计数本阶段恒为 0**：`mountMutagen` 返回 `(cleanup, conflicts=0, err)`，因为 `mutagen sync list --template` 解析逻辑由 Plan 03 落地。Banner 的 `⚠ N 个冲突` 行已在 `mount_strategy` 准备好，只是当前永远不会触发。
2. **生产路径 tryModeReal 未端到端测试**：mount_strategy_test.go 全部用 `hooks` 注入 mock 实现验证状态机；`tryModeReal` 只通过整仓 `go build` 保证编译正确性。Plan 03 集成测试 + Phase 34 doctor 的真实部署验证将兜底。
3. **exitcodes 与 cmd/cloud-claude/main.go 双套常量**：本 plan 新增 `cloudclaude.Exit*` 但未 rename `cmd/cloud-claude/main.go::exit*`，避免大范围 git diff 与潜在行为回归；rename 工作留 Phase 34。
4. **--mount-mode 解析依赖 manual scan**：因 `rootCmd.DisableFlagParsing=true`，cobra 自动解析失效，runRoot 顶部用循环手工剥离 `--mount-mode[=value]`。Phase 34 若计划重构 cobra 接线（如 disable parsing 切换为 known-flags 模式），可统一改用 cobra 原生 flag 解析。

## Threat Flags

无新增 threat surface。所有 PLAN `<threat_model>` T-31-02-01..09 的 mitigation 都已在代码中落地：
- T-31-02-01（askpass 密码不进 ps）：askpass.go::Env 仅注 CLOUD_CLAUDE_SSH_PASS 环境变量；Test_AskpassHelper_PasswordNotInPath 单测断言。
- T-31-02-03/04（mergerfs/setfattr 注入）：branchPath / target 路径在 mount_merge.go 硬编码 + shellQuote 包装。
- T-31-02-08（M13 静默降级）：applyDowngrade 强制 stderr + last-session.json 双留痕；Test_Downgrade_BannerEachStep 强制断言。

## Self-Check: PASSED

- [x] `internal/cloudclaude/mount_mutagen.go` 存在
- [x] `internal/cloudclaude/mount_mutagen_test.go` 存在
- [x] `internal/cloudclaude/mount_merge.go` 存在
- [x] `internal/cloudclaude/mount_strategy.go` 存在
- [x] `internal/cloudclaude/mount_strategy_test.go` 存在
- [x] `internal/cloudclaude/exitcodes.go` 存在
- [x] `internal/cloudclaude/exitcodes_test.go` 存在
- [x] `internal/cloudclaude/ssh.go` 包含 `ConnectAndRunClaudeV3`
- [x] `cmd/cloud-claude/main.go` 包含 `--mount-mode` + `cloudclaude.ParseMode`
- [x] commits 92e0feb / 5cee2a1 / 9a75372 / 3676bc7 / 0ed84a4 全部存在于 `git log --oneline`
- [x] `go test ./... -count=1` 全 PASS
- [x] `gofmt -l` 三个新 mount_*.go + ssh.go + main.go + exitcodes*.go 输出空
- [x] `cloud-claude --mount-mode=invalid` 输出 `错误: --mount-mode 必须是 ...` 并退出 4 (exitConfigError)
- [x] 关键命令字符级一致：mergerfs opts / setfattr / sshfs reconnect / mutagen --mode / --default-owner-beta / --ignore-vcs 全部 grep 命中
- [x] `! grep -F 'sync list --json' internal/cloudclaude/mount_mutagen.go` 命中（D-28 修订落实）
