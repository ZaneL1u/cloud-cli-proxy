---
phase: 31-cli
plan: 01-errcodes-mutagen-embed
subsystem: internal/cloudclaude
tags: [errcodes, mutagen-embed, envcheck-fs, wave-1]
requires:
  - "Go 1.25.7（仓库 go.mod 锁定版本，//go:embed 需要 ≥ 1.16）"
provides:
  - "internal/cloudclaude/errcodes 包：Code/Severity/Entry/MustRegister/Lookup/Registry/Format 七类对外 API + 15 条 MOUNT_*/NET_OAUTH_* 预注册错误码"
  - "cloudclaude.ExtractMutagenBinary(dst string) error：go:embed 4 平台 v0.18.1 mutagen + 幂等抽取"
  - "cloudclaude.MutagenBinaryVersion 常量（Plan 02 版本握手用）"
  - "cloudclaude.IsCaseInsensitiveFS(dir string) bool：跨平台大小写敏感探测，不依赖 macOS diskutil"
  - "scripts/fetch-mutagen-bins.sh：CI/本地拉取脚本，支持 --check-only sha256 复核"
affects:
  - "Wave 2 Plan 02 mount_mutagen.go 直接 import errcodes + 调 ExtractMutagenBinary"
  - "Wave 2 Plan 02 mount_strategy.go 调 IsCaseInsensitiveFS 决定 macOS APFS 强制 two-way-resolved"
  - "Wave 3 Plan 03 oauth_check.go 复用 NET_OAUTH_* 三态注册码"
  - "Phase 34 doctor / explain 子命令复用 errcodes.Registry()"
tech-stack:
  added: []
  patterns:
    - "init() MustRegister 模式（编译时 fail-fast，错误码冲突 / 命名违规直接 panic）"
    - "go:embed FS + extractFor(plat,dst) 内部 helper（使 unsupported-platform 用例可在任意 GOOS 跑）"
    - "tmp-then-rename 原子写文件"
    - "os.CreateTemp + 大写变体 Stat 的 case-insensitive probe"
key-files:
  created:
    - "internal/cloudclaude/errcodes/codes.go"
    - "internal/cloudclaude/errcodes/mount.go"
    - "internal/cloudclaude/errcodes/net.go"
    - "internal/cloudclaude/errcodes/codes_test.go"
    - "internal/cloudclaude/mutagen_bin.go"
    - "internal/cloudclaude/mutagen_bin_test.go"
    - "internal/cloudclaude/envcheck_fs.go"
    - "internal/cloudclaude/envcheck_fs_test.go"
    - "internal/cloudclaude/mutagen_bin/.gitattributes"
    - "internal/cloudclaude/mutagen_bin/SHA256SUMS"
    - "internal/cloudclaude/mutagen_bin/{darwin,linux}_{amd64,arm64}/mutagen（占位 stub）"
    - "scripts/fetch-mutagen-bins.sh"
  modified: []
decisions:
  - "errcodes 命名正则放宽为 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$（PLAN 原 3 段表达式与实际 4 段 code 冲突；Rule 1 修订）"
  - "mutagen_bin/.gitattributes 关闭 LFS 行（仓库未装 git-lfs；裸提交 ~49MB 在 v3.0 终态预期内）"
  - "mutagen 二进制本次提交为占位 stub，SHA256SUMS = PENDING-FETCH × 4，由 CI 在 build-images workflow 中补齐（PLAN Task 1.2 第 3 步明确允许此路径）"
  - "ExtractMutagenBinary 提供内部 extractMutagenFor(plat,dst) helper，让 UnsupportedPlatform 测试在任意 GOOS 都可跑"
metrics:
  duration_seconds: 429
  duration_human: "~7 分钟"
  tasks_completed: 3
  files_created: 13
  files_modified: 0
  commits: 4
  completed_at: "2026-04-19T08:35:36Z"
---

# Phase 31 Plan 01: errcodes + mutagen embed + envcheck_fs Summary

## One-Liner

为 Phase 31 Wave 2/3 搭好底座：落地 `internal/cloudclaude/errcodes` 错误码注册表（15 条 MOUNT_*/NET_*）、嵌入 4 平台 Mutagen v0.18.1 二进制（`go:embed` + 幂等抽取）、跨平台 case-insensitive 文件系统 probe。

## Goal Achievement

| 维度 | 目标 | 实际 | 状态 |
|------|------|------|------|
| errcodes 包 6 类对外 API | Code/Severity/Entry/Registry/Lookup/Format | 七类（追加 MustRegister）+ 15 条注册 | ✅ |
| 错误码注册数 | ≥ 15（11 MOUNT_* + 1 transport + 3 NET_*） | 12 + 3 = 15 | ✅ |
| Format 模板严格匹配 | `[<CODE>] <Message>\n  建议: <NextAction>` | TestFormat_Render 字面断言通过 | ✅ |
| Mutagen go:embed 4 平台 | darwin/linux × amd64/arm64 + ExtractTo 幂等 | 4 平台目录就位（占位 stub），CI 拉真二进制 | ⚠️ 占位 |
| ExtractMutagenBinary 幂等 | 已存在且 version 含 0.18.1 直接复用 | 实现完成；测试 SKIP 占位场景 | ⚠️ 占位 |
| IsCaseInsensitiveFS 不依赖 diskutil | 纯 Go probe | os.CreateTemp + 大写 Stat 实现 | ✅ |
| 整仓 go test ./... | 不破坏 v2.0 回归 | 全 PASS | ✅ |

## Public Interfaces 兑现清单

### `internal/cloudclaude/errcodes` 导出 API

```go
type Code string
type Severity int  // SeverityInfo / SeverityWarn / SeverityError / SeverityFatal
type Entry struct { Code Code; Severity Severity; Message string; NextAction string }

func MustRegister(e Entry)                  // panic on dup / 命名违规 / 空字段
func Lookup(c Code) (Entry, bool)
func Registry() map[Code]Entry              // 浅拷贝
func Format(c Code, args ...any) string     // [<CODE>] <Message>\n  建议: <NextAction>
```

### 15 个 Code 常量

`MOUNT_MUTAGEN_VERSION_SKEW` / `MOUNT_MUTAGEN_WHITELIST_REJECT` / `MOUNT_MUTAGEN_SAFETY_GUARD` / `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` / `MOUNT_MUTAGEN_SYNC_FAILED` / `MOUNT_MUTAGEN_TRANSPORT_FAILED` / `MOUNT_SSHFS_FAILED` / `MOUNT_SSHFS_DISCONNECTED` / `MOUNT_MERGERFS_FAILED` / `MOUNT_AUTO_DOWNGRADED` / `MOUNT_FORCE_MODE_FAILED` / `MOUNT_APFS_CASE_INSENSITIVE` / `NET_OAUTH_EXPIRED` / `NET_OAUTH_EXPIRING_SOON` / `NET_OAUTH_NOT_FOUND`

### `internal/cloudclaude` 新增导出

```go
const MutagenBinaryVersion = "v0.18.1"
func ExtractMutagenBinary(dst string) error
func IsCaseInsensitiveFS(dir string) bool
```

## Plan 02 调用契约

Plan 02 `mount_mutagen.go` 落地时按以下方式 import 与调用：

```go
import "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"

home, _ := os.UserHomeDir()
binPath := filepath.Join(home, ".cloud-claude", "bin", "mutagen")
if err := cloudclaude.ExtractMutagenBinary(binPath); err != nil {
    return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, err.Error()))
}

// macOS APFS 检测
if runtime.GOOS == "darwin" && cloudclaude.IsCaseInsensitiveFS(localCwd) {
    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.MOUNT_APFS_CASE_INSENSITIVE))
    syncMode = "two-way-resolved"  // 强制
}
```

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1.1 | errcodes 包核心 + 15 条注册 + 5 项单测 | 4c8b3f0 | `internal/cloudclaude/errcodes/{codes,mount,net,codes_test}.go` |
| 1.2 | fetch 脚本 + 4 平台占位 + .gitattributes + SHA256SUMS | c10cd53 | `scripts/fetch-mutagen-bins.sh`, `internal/cloudclaude/mutagen_bin/*` |
| 1.3-RED | 失败测试（ExtractMutagenBinary × 4 + IsCaseInsensitiveFS × 2） | 91c8fc2 | `internal/cloudclaude/{mutagen_bin,envcheck_fs}_test.go` |
| 1.3-GREEN | 实现 ExtractMutagenBinary + IsCaseInsensitiveFS | 715483a | `internal/cloudclaude/{mutagen_bin,envcheck_fs}.go` |

## Test Coverage

```
errcodes 包：
  TestErrcodesRegistry        PASS  (注册表完整性：去重/命名/非空/≤80runes/≥15条)
  TestFormat_Render           PASS  (字面断言两段输出模板)
  TestFormat_UnknownCode      PASS  (未注册 code 不 panic)
  TestLookup_Hit              PASS  (NET_OAUTH_EXPIRED Severity + Message)
  TestLookup_Miss             PASS  (未注册 code 返回 false)

cloudclaude 包：
  Test_IsCaseInsensitiveFS_TempDir              PASS
  Test_IsCaseInsensitiveFS_NoWrite              PASS
  Test_ExtractMutagenBinary_UnsupportedPlatform PASS
  Test_ExtractMutagenBinary_FreshDir            SKIP (占位场景)
  Test_ExtractMutagenBinary_Idempotent          SKIP (占位场景)
  Test_ExtractMutagenBinary_OverwriteWrongVersion SKIP (占位场景)

整仓回归：go test ./... -count=1 全 PASS（含 v2.0 现有 mount_test / ssh_doctor_test 等）。
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] errcodes 命名正则与实际 code 段数不匹配**

- **Found during:** Task 1.1 撰写 codes_test.go 时
- **Issue:** PLAN action 5 与 acceptance 写明 `var codeRe = regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+$`)`，仅允许 3 段（如 `MOUNT_MUTAGEN_FOO`）。但 `<errcode_registry>` 实际全部 4 段（如 `MOUNT_MUTAGEN_VERSION_SKEW`）。如严格按 PLAN 实现，所有 init() 注册都会 panic。
- **Fix:** 把正则放宽为 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`（≥ 3 段）。codes.go 与 codes_test.go 均同步修订。
- **Files modified:** `internal/cloudclaude/errcodes/codes.go`、`internal/cloudclaude/errcodes/codes_test.go`
- **Commit:** 4c8b3f0
- **Phase 34 影响:** Phase 34 doctor `--explain` 验证表达式应同步使用本仓库 codeRe；记入 D-32 接口契约。

**2. [Rule 4 → 自决] mutagen_bin/.gitattributes 关闭 LFS 行**

- **Found during:** Task 1.2 准备提交占位 mutagen 文件
- **Issue:** PLAN 提供的 `.gitattributes` 模板默认开启 git-lfs（4 行 filter=lfs），但仓库未安装 git-lfs（`git lfs version` 报「不是一个 git 命令」）；强行 commit 会让 LFS clean filter 缺失。
- **Decision:** PLAN Task 1.2 第 2 步明确允许"如团队拒绝 LFS，可改为裸提交（4×~12MB ≈ 49MB）" → 选裸提交方案。`.gitattributes` 保留 `* binary` 行 + 4 条 LFS 行注释（注释中说明未来切换路径），未来团队装 LFS 后取消注释 + `git lfs migrate` 即可。
- **Commit message 已声明此选择。**

**3. [Rule 4 → 自决] mutagen 二进制本次落占位 stub，CI 拉真**

- **Found during:** Task 1.2 拉真二进制前体积探测
- **Issue:** Mutagen v0.18.1 单 tarball 102MB（不是 RESEARCH §1.1 旧值 ~12MB），4 个共 ~400MB；直接 commit 会让仓库膨胀且违背 SCOPE BOUNDARY「禁止把任意未校验的二进制 commit」。
- **Decision:** 按 PLAN Task 1.2 第 3 步明确允许的「占位 + CI 补齐」路径走。占位文件是可执行 shell stub，运行时直接 `exit 1` 提示用户运行 fetch 脚本；SHA256SUMS 写 `PENDING-FETCH  <plat>/mutagen` × 4。CI 在 build-images workflow 中执行 `scripts/fetch-mutagen-bins.sh` 拉真二进制并替换。
- **Commit:** c10cd53（commit message 已显式声明）

### Deferred Issues (Out of Scope)

- `internal/cloudclaude/envcheck.go` 与 `internal/cloudclaude/ssh_doctor_test.go` 存在 pre-existing gofmt 不规范（来自 quick-task 7836821），与本 plan 无关，按 SCOPE BOUNDARY 不修。建议下一次触碰这些文件的 plan 顺手 `gofmt -w`。

### No Architectural Changes

未触及 v2.0 现网代码路径（`mount.go` / `ssh.go` / `entry.go` / `main.go` 全部未改动），符合 PLAN 原则「Wave 1 只搭底座，不改现网」。

## Authentication Gates

无（本 plan 全部本地操作 + git commit）。

## Known Stubs

| 文件 | 行 | Stub 类型 | 由谁补齐 |
|------|----|----|----|
| `internal/cloudclaude/mutagen_bin/{darwin,linux}_{amd64,arm64}/mutagen` | 整文件 | 占位 shell stub（`exit 1` + 中文提示） | CI build-images workflow 运行 `scripts/fetch-mutagen-bins.sh` 替换 |
| `internal/cloudclaude/mutagen_bin/SHA256SUMS` | 4 行 | `PENDING-FETCH` 占位 | 同上（fetch 脚本写真实 sha256） |

**Stub 是否阻塞 plan goal？** 不阻塞。本 plan goal 是「Wave 2/3 起步所需的接口、Code、函数签名、占位资源」全部就位，二进制内容由 CI 注入是 PLAN 明确允许的延后分工。Plan 02/03 任务计划本就在 CI 环境跑（带真二进制）。`Test_ExtractMutagenBinary_FreshDir/Idempotent/OverwriteWrongVersion` 用例自动 SKIP 占位场景，CI 替换后会自动 PASS。

## Threat Flags

无新增 threat surface。本 plan 引入的所有外部依赖（mutagen GitHub release）已在 PLAN 的 `<threat_model>` T-31-01-01..06 中评估并设定 disposition。

## Limitations / Trade-offs

1. **二进制体积**：cloud-claude 二进制尺寸将从 ~30MB 涨到 ~80MB（`go:embed` 将 4 平台 mutagen 全打入；与 RESEARCH §1.1 已知一致）。v3.1 可考虑按 build tag 拆平台二进制。
2. **gofmt 整仓警告**：v2.0 既有的 `envcheck.go` / `ssh_doctor_test.go` pre-existing gofmt 不规范本 plan 未修，避免触发 SCOPE BOUNDARY。
3. **Mutagen 真二进制依赖 CI**：本地开发者首次跑 Plan 02/03 测试前需手动 `bash scripts/fetch-mutagen-bins.sh`（README/onboarding 文档可由 Phase 35 补充）。

## Self-Check: PASSED

- [x] `internal/cloudclaude/errcodes/codes.go` 存在
- [x] `internal/cloudclaude/errcodes/mount.go` 存在
- [x] `internal/cloudclaude/errcodes/net.go` 存在
- [x] `internal/cloudclaude/errcodes/codes_test.go` 存在
- [x] `internal/cloudclaude/mutagen_bin.go` 存在
- [x] `internal/cloudclaude/mutagen_bin_test.go` 存在
- [x] `internal/cloudclaude/envcheck_fs.go` 存在
- [x] `internal/cloudclaude/envcheck_fs_test.go` 存在
- [x] `scripts/fetch-mutagen-bins.sh` 存在且可执行
- [x] `internal/cloudclaude/mutagen_bin/SHA256SUMS` 存在
- [x] `internal/cloudclaude/mutagen_bin/{darwin,linux}_{amd64,arm64}/mutagen` 4 个占位存在
- [x] commit 4c8b3f0 / c10cd53 / 91c8fc2 / 715483a 全部存在于 `git log --oneline`
- [x] `go test ./... -count=1` 全 PASS
- [x] `go vet ./...` exit 0
