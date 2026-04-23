---
phase: 36-sshfs
plan: "04"
subsystem: cli
tags: [mount, git, errcodes, l1-timing, requireGitRepo]

# Dependency graph
requires:
  - phase: 36-sshfs
    provides: "MOUNT_REQUIRE_GIT_REPO 错误码注册（Plan 36-01）"
  - phase: 31-cli
    provides: "errcodes.Format 两段输出 + exitConfigError=4 退出语义"
  - phase: 34-cloud-claude-doctor-v3
    provides: "explain 子命令 + 长说明（自动覆盖新错误码）"
provides:
  - "cmd/cloud-claude/git_check.go::requireGitRepo(cwd) 工具函数"
  - "runRoot 中 cwd 获取与 git 仓库前置检测，先于 AuthenticateAndWait"
  - "三场景 git_check_test.go 单测（git 仓库 / 非 git 目录 / git 不可用）"
affects: [36-06-PLAN, doctor.checkRequireGitRepo, REQ-MOUNT-V31-01]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "本地 CLI 前置校验：exec.Command(\"git\", \"rev-parse\", \"--show-toplevel\") + cmd.Dir = cwd + cmd.Run() 退出码即布尔结论"
    - "main.go::runRoot 把『发起网络/认证之前的本地校验』集中放在 LoadConfig 成功后、NewEntryClient 之前的固定窗口"
    - "测试通过 t.Setenv(\"PATH\", \"\") 模拟二进制不可用，复用真实 exec.Command 路径而非 mock"

key-files:
  created:
    - cmd/cloud-claude/git_check.go
    - cmd/cloud-claude/git_check_test.go
  modified:
    - cmd/cloud-claude/main.go

key-decisions:
  - "git 二进制不可用与非 git 仓库共用 MOUNT_REQUIRE_GIT_REPO 一条错误码（D-03），避免再起一条只为区分 git not installed 的码"
  - "git 检测点固定在 LoadConfig 成功后、NewEntryClient 之前；不挪到 ParseMode 之前，确保配置加载错误仍优先暴露"
  - "TestRequireGitRepo_GitUnavailableHandled 用 t.Setenv(\"PATH\", \"\") 而非新增包级 var execCommand 注入，源代码零侵入"

patterns-established:
  - "新增本地前置校验时，封装在 cmd/cloud-claude/<feature>_check.go，单测真实运行不 mock"
  - "main.go::runRoot 内『前置校验段』(LoadConfig 后 → NewEntryClient 前) 是后续 Phase 增加同类拒绝项的标准插入位"

requirements-completed: [REQ-MOUNT-V31-01]

# Metrics
duration: 3 min
completed: 2026-04-23
---

# Phase 36 Plan 04: sshfs Summary

**runRoot 工作目录获取与 git 仓库前置检测前移到 AuthenticateAndWait 之前，非 git 目录立即 exit 4 且不发起任何 SSH 连接（修复 RESEARCH §L1 时序地雷）。**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-23T11:49:18Z
- **Completed:** 2026-04-23T11:51:58Z
- **Tasks:** 2
- **Files modified:** 3（2 新增 + 1 修改）

## Accomplishments
- 新建 `cmd/cloud-claude/git_check.go::requireGitRepo(cwd)`，复用 Plan 36-01 注册的 `MOUNT_REQUIRE_GIT_REPO` + `errcodes.Format` 两段输出；git 不可用按 D-03 与非 git 仓库走同一错误码。
- 在 `runRoot` 把 `os.Getwd()` 从旧 line 332（`AuthenticateAndWait` 之后）前移到新 line 301（`LoadConfig` 成功后、`NewEntryClient` 之前），并紧接调用 `requireGitRepo(cwd)` 失败 stderr + `os.Exit(exitConfigError)`。
- `git_check_test.go` 三场景单测（git 仓库 / 临时空目录 / `t.Setenv("PATH","")` 模拟 git 不可用）全部 PASS；`go test ./cmd/cloud-claude/... -v` 含原 5 条 explain + 3 条新单测共 8 条全部 PASS；`go test ./internal/cloudclaude/...` 全部 PASS（无下游回归）。

## Task Commits

Each task was committed atomically:

1. **Task 1: 新建 git_check.go + git_check_test.go** - `425e891` (feat)
2. **Task 2: main.go os.Getwd() 前移 + requireGitRepo 调用** - `214d53b` (fix)

**Plan metadata:** 本文件随当前 docs commit 一并提交。

## Files Created/Modified
- `cmd/cloud-claude/git_check.go` — `requireGitRepo(cwd) error` 工具函数；非 git 仓库 / git 不可用统一返回 `MOUNT_REQUIRE_GIT_REPO` 两段格式化错误。
- `cmd/cloud-claude/git_check_test.go` — 三场景单测（in-repo / not-a-repo / git-unavailable）。
- `cmd/cloud-claude/main.go` — `runRoot` 中：① 删除原 line 332-336 的 `os.Getwd()` 块；② 在 `LoadConfig` 成功之后插入新的 `os.Getwd()` + `requireGitRepo(cwd)` 调用块。最终 `os.Getwd` / `requireGitRepo` 在 main.go 中各恰好出现 1 次（line 301 / line 306），`runRoot::AuthenticateAndWait` 在 line 315，时序：306 < 315 ✓。

## Decisions Made
- **D-03 共用错误码**：git 二进制不可用与非 git 仓库都映射到 `MOUNT_REQUIRE_GIT_REPO`，统一由「修复路径」给出（cd 到 git 仓库 / `git init`）。理由：用户视角两种状态后果一致——cloud-claude 拒绝挂载；增加 `GIT_NOT_INSTALLED` 等独立码会污染 explain 域且无操作差异。
- **检测点固定在 LoadConfig 成功后**：保持 `--mount-mode` 参数错误（line 282）与 `LoadConfig` 错误（line 287）的优先级不变；非 git 目录在以上两类配置错误之后立即被拒绝，但仍早于任何网络 IO。
- **测试用 `t.Setenv("PATH","")`**：避免在 `git_check.go` 引入包级 var execCommand 仅为可测试性；`exec.Command("git", ...)` 在 PATH 为空时本地立即返回 not found，对源代码完全零侵入，并与 `cmd.Run()` 失败统一走 D-03 路径。

## Deviations from Plan

None — plan executed exactly as written. acceptance_criteria 全部通过。

唯一为满足验收做的微调：注释中将「`os.Getwd()` 前移」改写为「工作目录获取前移」，「`AuthenticateAndWait` 之后」改写为「认证流程之后」，以保证 plan acceptance 的「`grep -n "os.Getwd" cmd/cloud-claude/main.go` 输出恰好 1 行」与「`grep -n "AuthenticateAndWait"` 行号 > `requireGitRepo` 行号」严格成立（仅修辞调整，语义零变化，不构成 deviation）。

## Issues Encountered
None.

## Authentication Gates
None — Plan 36-04 完全是本地 CLI 改动，无外部认证。

## Known Stubs
None — `requireGitRepo` 为完整实现；不存在硬编码空值或占位符。

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 36-06 doctor `checkRequireGitRepo` 可直接复用 Plan 36-04 的 `requireGitRepo` 语义，按 PATTERNS.md §"`internal/cloudclaude/doctor/mount.go`"  本地 check 模板补一份 `gitRevParseTopLevel` 私有 helper（doctor 包不能 import `cmd/cloud-claude` main 包，需自行复制 4 行 exec 代码）。
- L1 时序地雷已闭合，REQ-MOUNT-V31-01 字面要求（"git 检测必须在任何 SSH 连接之前"）有源代码 + 单测双重证据。
- 本 plan 不引入任何新的网络面、auth 路径、文件系统写入或 schema 变更；threat_model 4 条全部 accept/已 mitigate（exec 调用参数为字面量，cwd 来自 os.Getwd 不受用户输入污染；本地 git 命令 <100ms 不需 timeout）。

## Self-Check: PASSED

- ✅ `cmd/cloud-claude/git_check.go` 存在
- ✅ `cmd/cloud-claude/git_check_test.go` 存在
- ✅ `cmd/cloud-claude/main.go` 已修改（diff 1 file changed, 13 insertions(+), 6 deletions(-)）
- ✅ Task 1 commit `425e891` 在 git 历史中
- ✅ Task 2 commit `214d53b` 在 git 历史中
- ✅ acceptance criteria 全部通过：`grep -n "os.Getwd" cmd/cloud-claude/main.go` 输出 1 行（line 301）；`grep -n "requireGitRepo" cmd/cloud-claude/main.go` 输出 1 行（line 306）；`runRoot::AuthenticateAndWait` 在 line 315；306 < 315 ✓
- ✅ `go build ./...` PASS
- ✅ `go test ./cmd/cloud-claude/...` 全 PASS（含 3 条新 + 5 条 explain）
- ✅ `go test ./internal/cloudclaude/...` 全 PASS（无下游回归）

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
