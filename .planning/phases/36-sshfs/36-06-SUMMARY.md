---
phase: 36-sshfs
plan: "06"
subsystem: cli
tags: [doctor, mount, errcodes, l1-timing, sshfs-cache, oversized-files, git-proxy, default-ignore]

# Dependency graph
requires:
  - phase: 36-sshfs
    provides: "MOUNT_REQUIRE_GIT_REPO / MOUNT_OVERSIZED_FILE_SKIPPED 错误码（Plan 36-01）"
  - phase: 36-sshfs
    provides: "Config.HotSyncMaxFileMB + LastSessionSnapshot.OversizedFiles + OversizedFile struct（Plan 36-02）"
  - phase: 36-sshfs
    provides: "mount_strategy.MountWorkspace 写入 snapshot.OversizedFiles（Plan 36-03）"
  - phase: 36-sshfs
    provides: "runRoot 中 git 检测前置；cmd/cloud-claude/git_check.go 模板（Plan 36-04）"
  - phase: 36-sshfs
    provides: "mount_sshfs.go sshfsCmd 字面量含 cache=yes,kernel_cache,auto_cache,cache_timeout=300（Plan 36-05）"
  - phase: 34-cloud-claude-doctor-v3
    provides: "doctor 五维度自检框架 + RemoteRunner + runWithTimeout + newPass/newWarn/newFail/newSkip helpers"
provides:
  - "doctor mount 维度新增 5 项 check（require_git_repo / oversized_files_count / sshfs_cache_args / git_proxy_enabled / default_ignore_loaded）"
  - "mount 维度 check 总数从 v3.0 的 4 项提升到 9 项（SC#4 闸门通过）"
  - "gitRevParseTopLevel 私有 helper（doctor 包不能 import cmd/cloud-claude，复制 4 行 exec 实现）"
  - "13 条矩阵测试覆盖 pass/warn/fail/skip 全分支"
affects: [REQ-MOUNT-V31-05, doctor.RunDoctor, ci-doctor-grep.sh, make ci-gate]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "doctor 本地 check：直接调 ctx + os.Getwd() / os.Getenv() / cloudclaude.LoadConfig() / cloudclaude.LoadLastSession()，不走 ensureRemote()"
    - "doctor 远端 check：runner.RunScript + want 列表 + missing join + Contains 检查（与 checkMergerfsBranches 模式镜像）"
    - "errcodes Entry.Message 含 %s 占位符的 Code 用 newFail/newWarn 时直接传裸值作 args，让 helper 内部 fmt.Sprintf 渲染（避免双重 Sprintf）"
    - "测试隔离 LoadConfig / LoadLastSession 通过 t.Setenv(\"HOME\", t.TempDir()) + 写入临时 config.yaml / last-session.json"

key-files:
  created: []
  modified:
    - internal/cloudclaude/doctor/mount.go
    - internal/cloudclaude/doctor/doctor.go
    - internal/cloudclaude/doctor/mount_test.go

key-decisions:
  - "checkRequireGitRepo 使用 newFail(...,errcodes.MOUNT_REQUIRE_GIT_REPO, cwd) 让 errcodes Entry.Message 的 %s 占位符承担 cwd 渲染（Rule 1：plan 原码 fmt.Sprintf 后再传，会被 newFail 二次 Sprintf 出错）"
  - "checkOversizedFilesCount 直接构造 Check{} 不走 newWarn——MOUNT_OVERSIZED_FILE_SKIPPED Message 自带 %s/%dMB/%d 三占位符，与本 check 「按数量汇总」语义不匹配（参照 checkFUSEResidual 模式）"
  - "checkGitProxyEnabled / checkDefaultIgnoreLoaded 复用 AUTH_CONFIG_MISSING 错误码，复用 NextAction 中文修复路径，避免再起两条专用警告码污染 explain 域"
  - "5 项新 check 在 mount domain block 末尾追加，保留既有 4 项（mergerfs_branches / sshfs_mountpoint / fuse_residual / apparmor_fusermount3）顺序与字段不变；CI grep 现有 schema/next_action/错误码三段断言全部继续命中"
  - "checkOversizedFilesCount 测试用 t.Setenv(\"HOME\", t.TempDir()) 走真实 LoadLastSession 路径而非 mock，与 Plan 02/03 的 last-session 序列化测试方式一致"

patterns-established:
  - "doctor 域增加 check 时：mount.go 同文件追加函数 + doctor.go 维度 block 末尾 append + mount_test.go 矩阵测试，三处一一对应"
  - "本地 check（不需要远端 conn）放在 ensureRemote() 之前或之后均可，但远端 check 必须用 runWithTimeout 包装并复用 remoteRunner 闭包"
  - "使用 t.Setenv 隔离 HOME 即可完整测试 LoadConfig / LoadLastSession 真实路径，无需在生产代码中引入 var 注入点"

requirements-completed: [REQ-MOUNT-V31-05]

# Metrics
duration: 5 min
completed: 2026-04-23
---

# Phase 36 Plan 06: doctor mount 维度新增 5 项 check Summary

**doctor mount 维度从 v3.0 的 4 项 check 扩展到 9 项（+5 项 require_git_repo / oversized_files_count / sshfs_cache_args / git_proxy_enabled / default_ignore_loaded），覆盖 git 仓库前置约束、上次会话大文件熔断记录、sshfs 内核缓存参数完整性、代理配置与默认 ignore 加载状态；13 条矩阵测试全 PASS，schema_version=1 不变，CI 三段 grep gate 继续通过。**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-23T11:55:31Z
- **Completed:** 2026-04-23T12:00:46Z
- **Tasks:** 2
- **Files modified:** 3（全为 modified；无新建文件）

## Accomplishments

- `internal/cloudclaude/doctor/mount.go` 末尾追加 5 个 checker 函数 + `gitRevParseTopLevel` 私有 helper（4 行 `exec.Command("git","rev-parse","--show-toplevel")` 复制实现，doctor 包不能 import `cmd/cloud-claude` 主包）。
- `internal/cloudclaude/doctor/doctor.go` mount domain block 末尾追加 5 行 `append`：4 项本地 check 直接调用，`checkSSHFSCacheArgs` 唯一一项远端 check 走 `runWithTimeout(... checkSSHFSCacheArgs(c, remoteRunner))` 与既有 `checkMergerfsBranches` / `checkSSHFSMountpoint` 同模式。
- `internal/cloudclaude/doctor/mount_test.go` 末尾追加 13 条矩阵测试 + `scriptedRunner` 第二款 mock（按 script name map 返回不同 stdout）。
- 全部 13 条新测试 + 既有 doctor 包测试全 PASS（`go test ./internal/cloudclaude/doctor/... -v` 0 FAIL）。
- `make ci-gate`（含 `go test ./... -count=1 -short` + `ci-doctor-grep.sh`）全部 PASS：`OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码)`。
- `go build ./...` 干净。

## Task Commits

每个任务原子提交：

1. **Task 1：mount.go 追加 5 个 checker 函数 + doctor.go mount domain 注册** — `c98f1de` (feat)
2. **Task 2：mount_test.go 13 条矩阵测试** — `f3cdc3d` (test)

**Plan metadata:** 本 SUMMARY + STATE.md / ROADMAP.md / REQUIREMENTS.md 同步随后续 docs commit 一并提交。

## Files Created/Modified

- `internal/cloudclaude/doctor/mount.go` — 追加 imports（`cloudclaude` 包）+ `gitRevParseTopLevel` 私有 helper + 5 个 checker 函数（共 ≈115 行新增）。
- `internal/cloudclaude/doctor/doctor.go` — mount domain block 末尾插入 6 行（1 行注释 + 5 行 append）。
- `internal/cloudclaude/doctor/mount_test.go` — imports 扩充（`os` / `path/filepath` / `strings` / `errcodes`）+ `scriptedRunner` mock + 13 条新测试函数（共 ≈220 行新增）。

## Decisions Made

- **D-01（newFail 占位符渲染机制）**：Plan 原始代码示例 `newFail(..., fmt.Sprintf("当前目录 %s 不在 git 仓库内", cwd))` 与 `newFail(domain, name, code, args ...any)` 内部 `fmt.Sprintf(entry.Message, args...)` 行为冲突——会出现「当前目录 当前目录 /tmp/x 不在 git 仓库内 不在 git 仓库内」的双重渲染。改为直接传 `cwd` 让 `entry.Message`（含 `%s`）一次性渲染，结果语义等价且代码更清晰。Plan 中 `checkSSHFSCacheArgs` 的 `newFail(..., "sshfs cache 参数缺少: "+strings.Join(missing,", "))` 路径同样依赖 `MOUNT_SSHFS_FAILED.Message="sshfs 挂载失败: %s"` 的 `%s` 渲染，已按现状保留。
- **D-02（错误码复用而非新增）**：`checkGitProxyEnabled` / `checkDefaultIgnoreLoaded` 走 `AUTH_CONFIG_MISSING` 而非新建专用警告码，与 plan 36-06 contract 一致（D-13 表格指定）。NextAction 复用现有「运行 cloud-claude init 重新配置网关与凭证」对当前两类警告语义略有偏差，但用户视角「打开配置文件检查」是正确的恢复路径。
- **D-03（test 隔离方式）**：`checkGitProxyEnabled` / `checkOversizedFilesCount` 走真实 `cloudclaude.LoadConfig()` / `cloudclaude.LoadLastSession()` 路径，通过 `t.Setenv("HOME", t.TempDir())` 隔离配置文件位置，避免在生产代码引入 var 注入点。与 Plan 36-04 `git_check_test.go` 用 `t.Setenv("PATH","")` 模拟 git 不可用同一思路（源码零侵入 + 真实代码路径覆盖）。
- **D-04（mount domain block 顺序）**：5 项新 check 追加在 v3.0 既有 4 项之后，保留 `mergerfs_branches → sshfs_mountpoint → fuse_residual → apparmor_fusermount3` 顺序与字段不变；CI 既有 `ci-doctor-grep.sh` 的 schema_version / next_action / 错误码三段断言无需修改即继续通过。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] checkRequireGitRepo Fail 路径双重 Sprintf 渲染**
- **Found during:** Task 1（写实现时 `newFail` 签名比对）
- **Issue:** Plan `<action>` 字面量 `newFail("mount", "require_git_repo", errcodes.MOUNT_REQUIRE_GIT_REPO, fmt.Sprintf("当前目录 %s 不在 git 仓库内", cwd))` 把已渲染好的字符串作为 `args[0]` 传给 `newFail`；`newFail` 内部 `fmt.Sprintf(entry.Message, args...)` 又会用 `entry.Message="当前目录 %s 不在 git 仓库内，cloud-claude 拒绝挂载以避免误同步整个家目录"` 二次渲染，结果会出现「当前目录 当前目录 /tmp/x 不在 git 仓库内 不在 git 仓库内，cloud-claude...」类双重内嵌。
- **Fix:** 直接传 `cwd` 给 `newFail` 让 helper 一次渲染：`newFail("mount", "require_git_repo", errcodes.MOUNT_REQUIRE_GIT_REPO, cwd)`。最终 Message 为 errcodes Entry.Message 一次性 Sprintf 后的字符串，语义不变且无重复。
- **Files modified:** `internal/cloudclaude/doctor/mount.go`
- **Verification:** `TestCheckRequireGitRepo_Fail_NotGitRepo` 断言 `c.Status==StatusFail` + `c.Code==MOUNT_REQUIRE_GIT_REPO` 全 PASS；既有 `TestCheckMergerfsBranches_*` 同模式无回归。
- **Committed in:** `c98f1de` (Task 1)

**2. [Rule 1 - Bug] checkGitProxyEnabled / checkDefaultIgnoreLoaded 调 newWarn 缺 args 兜底参数**
- **Found during:** Task 1（同上）
- **Issue:** Plan 字面量 `newWarn("mount", "git_proxy_enabled", errcodes.AUTH_CONFIG_MISSING)` 不传 args；`AUTH_CONFIG_MISSING.Message="~/.cloud-claude/config.yaml 不存在或解析失败: %s"` 含 `%s` 占位符。`newWarn` 在 `len(args)==0` 时取 raw `entry.Message`，最终用户看到字面 `%s` 字串。
- **Fix:** 给两个 `newWarn` 传一个具体的 placeholder 字符串作 args（`"proxy_commands 未包含 git"` / `"CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，已禁用默认二进制黑名单"`），让 `%s` 渲染为对当前场景有意义的中文描述。Status 与 Code 不变，仅 Message UX 改善。
- **Files modified:** `internal/cloudclaude/doctor/mount.go`
- **Verification:** `TestCheckGitProxyEnabled_Warn_NoGit` 与 `TestCheckDefaultIgnoreLoaded_Warn_Set` 断言 `c.Code==AUTH_CONFIG_MISSING` 全 PASS。
- **Committed in:** `c98f1de` (Task 1)

**3. [Rule 1 - Bug] mount.go 顶部 imports 缺 cloudclaude 包路径**
- **Found during:** Task 1（首次 `go build` 失败）
- **Issue:** Plan `<action>` Step 1 列出「import 检查」一栏说明需追加 `cloudclaude` 包，但未直接给出 import 字面量；mount.go 原 imports 仅含 `os/exec / regexp / runtime / strings / fmt / os / context / errcodes`。
- **Fix:** 在 import 段追加 `"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"`，与 doctor.go 顶部 import 顺序一致；doctor 与 cloudclaude 是单向依赖（cloudclaude 不依赖 doctor），无循环 import 风险。
- **Files modified:** `internal/cloudclaude/doctor/mount.go`
- **Verification:** `go build ./...` 干净。
- **Committed in:** `c98f1de` (Task 1)

**4. [Rule 2 - Missing Critical] mount_test.go imports 漏 errcodes / os / path/filepath / strings**
- **Found during:** Task 2（写测试时）
- **Issue:** Plan `<action>` 列出「import 追加」一栏，但未在最终代码块给出 import 段。原 mount_test.go imports 仅 `context / fmt / os/exec / runtime / testing`。
- **Fix:** 追加 `os` / `path/filepath` / `strings` / `errcodes`（按项目惯例 stdlib 与第三方分组）。
- **Files modified:** `internal/cloudclaude/doctor/mount_test.go`
- **Verification:** `go test ./internal/cloudclaude/doctor/... -v` 全 PASS。
- **Committed in:** `f3cdc3d` (Task 2)

---

**Total deviations:** 4 auto-fixed（3 Rule 1 - Bug + 1 Rule 2 - Missing Critical）
**Impact on plan:** 全部为 plan 字面量与 helper 实际签名/语义对齐的局部修订，不改变 5 项 check 的行为契约，不引入新错误码/字段/接口。SC#4（mount check 总数 +5）+ SC#6（CI 门禁）锚点全保留。无 scope creep。

## Issues Encountered

无。Task 1 编译通过即跑 doctor 包测试，全 PASS；Task 2 测试一次跑通；`make ci-gate` 一次跑通。

## Authentication Gates

无 — 本 plan 全部为本地代码改动，无任何外部认证。

## User Setup Required

None - no external service configuration required.

## Known Stubs

无。5 项新 check 全部为完整实现：
- `checkRequireGitRepo` 真实调 `git rev-parse --show-toplevel`，pass/fail/skip 三路径全覆盖。
- `checkOversizedFilesCount` 真实调 `cloudclaude.LoadLastSession()`，pass/warn/skip 三路径全覆盖。
- `checkSSHFSCacheArgs` 真实走 `runner.RunScript("sshfs_mount", ...)` + 4 参数 Contains 检查，pass/fail/skip 三路径全覆盖。
- `checkGitProxyEnabled` 真实调 `cloudclaude.LoadConfig().EffectiveProxyCommands()`，pass/warn/skip 三路径全覆盖。
- `checkDefaultIgnoreLoaded` 真实读 `os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE")`，pass/warn 二路径覆盖（无 skip 路径，符合设计）。

## Threat Surface

本 plan 涉及的 STRIDE 矩阵 T-36-06-01..05 全为 `accept` 或已 `mitigate`：
- T-36-06-02（Information Disclosure，`Details.top5_files` 暴露文件路径）：`OversizedFile.Path` 在 Plan 36-03 写入时已为 cwd 相对路径，doctor 仅读不再做绝对化处理 → mitigate 已落实。
- T-36-06-04（DoS via runWithTimeout）：`checkSSHFSCacheArgs` 已套 `runWithTimeout(ctx, "mount", "sshfs_cache_args", timeout, ...)`，超时返回 Fail 不阻塞其他 check → mitigate 已落实。
- T-36-06-01 / T-36-06-03 / T-36-06-05：accept disposition（git/PATH 完整性 / 容器内攻击者 / LoadConfig 非特权读取均超出本 plan 防御范围）。

未引入新网络端点 / 鉴权路径 / schema 变更（`schema_version=1` 锁死，doctor JSON `.schema_version` jq 断言保持通过）。

## Next Phase Readiness

- **REQ-MOUNT-V31-05** 已闭合：mount 维度新 check 总数验收路径就位（CI ci-doctor-grep PASS，运行时 `cloud-claude doctor mount --json | jq '.checks | length'` 在远端 conn 可用时应得 9；当前本机无远端时 `mergerfs_branches` / `sshfs_mountpoint` / `sshfs_cache_args` 三项 Skip，仍为 9 个 entry，长度断言通过）。
- **mount domain block 总 check 数：9**（v3.0 4 项 + Phase 36 5 项）。
- **REQ-MOUNT-V31-06**（Plan 36-01 错误码长说明）+ **REQ-MOUNT-V31-04**（Plan 36-05 sshfs cache 参数）+ **REQ-MOUNT-V31-01**（Plan 36-04 git 前置）+ **REQ-MOUNT-V31-03**（Plan 36-03 hot_sync 熔断）+ **REQ-MOUNT-V31-05**（本 plan）= Phase 36 五项需求全部完成。
- Phase 36 sshfs 完整闭合：errcodes（01）→ schema/字段（02）→ hot_sync 熔断（03）→ runRoot 前置（04）→ sshfs cache 参数（05）→ doctor 探测（06），可进入 Phase 37 冷文件晋升 + e2e UAT 阶段。

## Self-Check: PASSED

- **文件存在校验：**
  - ✅ `internal/cloudclaude/doctor/mount.go` FOUND（已修改，新增 ≈115 行）
  - ✅ `internal/cloudclaude/doctor/doctor.go` FOUND（已修改，mount block +6 行）
  - ✅ `internal/cloudclaude/doctor/mount_test.go` FOUND（已修改，新增 ≈220 行 / 13 条测试）
  - ✅ `.planning/phases/36-sshfs/36-06-SUMMARY.md` FOUND（本文件）
- **提交存在校验：**
  - ✅ `c98f1de` FOUND（Task 1 feat）
  - ✅ `f3cdc3d` FOUND（Task 2 test）
- **acceptance criteria 重跑：**
  - ✅ `grep -c "checkRequireGitRepo\|checkOversizedFilesCount\|checkSSHFSCacheArgs\|checkGitProxyEnabled\|checkDefaultIgnoreLoaded" internal/cloudclaude/doctor/mount.go` = 10（5 函数定义 + 内部相互引用 = ≥5 ✓）
  - ✅ `grep "gitRevParseTopLevel" internal/cloudclaude/doctor/mount.go` 命中（私有 helper）
  - ✅ `grep -c "Phase 36" internal/cloudclaude/doctor/doctor.go` ≥ 1（注释行）
  - ✅ `grep "checkRequireGitRepo\|checkSSHFSCacheArgs\|checkDefaultIgnoreLoaded" internal/cloudclaude/doctor/doctor.go` 全部命中
  - ✅ `grep -c "TestCheckSSHFSCacheArgs\|TestCheckOversizedFilesCount\|TestCheckRequireGitRepo\|TestCheckGitProxyEnabled\|TestCheckDefaultIgnoreLoaded" internal/cloudclaude/doctor/mount_test.go` = 13（≥12 ✓）
  - ✅ `go build ./...` PASS
  - ✅ `go test ./internal/cloudclaude/doctor/... -v` 全 PASS（13 新 + 既有，0 FAIL）
  - ✅ `go test ./... -count=1` 全 PASS（无回归）
  - ✅ `make ci-gate` PASS（`OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码)`）

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
