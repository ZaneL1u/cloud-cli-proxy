---
phase: 36-sshfs
plan: "05"
subsystem: cli
tags: [sshfs, fuse, page-cache, mount, sftp-fixture, kernel_cache]

# Dependency graph
requires:
  - phase: 31-cli
    provides: "mount_sshfs.go::mountSSHFS sshfsCmd 字面量基线（4 个抗抖参数）"
provides:
  - "mount_sshfs.go sshfsCmd 字面量含 4 个 FUSE page cache 参数 cache=yes,kernel_cache,auto_cache,cache_timeout=300"
  - "internal/cloudclaude/mount_sshfs_test.go 含 TestSSHFSCacheHitsKernelPageCache（fixture SFTP server-side counter 端到端验证 FUSE page cache 命中）"
  - "countingFileReader（atomic.Int64 + sync.Mutex 计数 sftp.FileReader）+ noopFileWriter 占位 sftp.Handlers.FilePut"
affects: [36-06-PLAN, doctor/mount.go::checkSSHFSCacheArgs]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "fixture SFTP counter pattern：golang.org/x/crypto/ssh + pkg/sftp.NewRequestServer + atomic 计数器，端到端验证 FUSE 透明缓存"
    - "测试卫士 t.Skip + exec.LookPath 顺序检查：sshfs / fusermount / fusermount3 任一缺失自动 Skip 不阻断 CI"

key-files:
  created:
    - internal/cloudclaude/mount_sshfs_test.go
  modified:
    - internal/cloudclaude/mount_sshfs.go

key-decisions:
  - "sshfsCmd 字面量在 ConnectTimeout=10 之后追加 4 参数（顺序锁死），便于 doctor sshfs_cache_args 字符串匹配"
  - "测试不 mock pkg/sftp，走真实 SSH+SFTP fixture server + 真实 sshfs 进程，避免 cache-hit 行为偏差"
  - "Plan 示例代码字面量 (sftp.ReadWriteAt / sftp.Handlers.FileLister / inMem.FileLister) 与 v1.13.10 实际 API 不符，按实际 API 修订（Rule 1）"

patterns-established:
  - "pkg/sftp v1.13.10 Handlers 四字段实际名为 FileGet/FilePut/FileCmd/FileList（不是 FileLister）"
  - "FileGet 类型为 FileReader（仅 Fileread），FilePut 类型为 FileWriter（仅 Filewrite），不存在 ReadWriteAt 组合接口"
  - "FUSE page cache 端到端验证套路：fixture SFTP server 内 atomic counter + 同文件 ReadFile 2 次 → 期望 server-side read=1"

requirements-completed: [REQ-MOUNT-V31-04]

# Metrics
duration: 4min
completed: 2026-04-23
---

# Phase 36 Plan 05: sshfs 内核缓存参数 + fixture SFTP 验证测试 Summary

**mount_sshfs.go 注入 4 个 FUSE page cache 参数（cache=yes,kernel_cache,auto_cache,cache_timeout=300），并新增 fixture SFTP counter 测试端到端验证「同会话同文件 ReadFile 2 次 → server-side Fileread = 1」。**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-23T11:29:52Z
- **Completed:** 2026-04-23T11:33:26Z
- **Tasks:** 2
- **Files modified:** 2（1 创建 + 1 修改）

## Accomplishments

- `mount_sshfs.go::mountSSHFS` 的 sshfsCmd 字面量在 `ConnectTimeout=10` 之后、`-f` 之前追加 `cache=yes,kernel_cache,auto_cache,cache_timeout=300`（字面量顺序锁死）。函数签名 / cleanup / error 包装零变更，`go build ./...` 干净。
- `internal/cloudclaude/mount_sshfs_test.go` 新文件含：
  - `countingFileReader`：实现 `sftp.FileReader`（FileGet），用 `atomic.Int64 + sync.Mutex` 统计每路径 `Fileread` 调用次数。
  - `noopFileWriter`：占位 `sftp.Handlers.FilePut`，返回 `ErrSSHFxOpUnsupported`，规避 L5 nil 地雷。
  - `mustSignerFromKey`：测试专用 RSA host key 生成 helper。
  - `TestSSHFSCacheHitsKernelPageCache`：完整 fixture SSH+SFTP server（`golang.org/x/crypto/ssh` + `sftp.NewRequestServer`）+ 真实 `sshfs` 进程挂载 + 同文件 `os.ReadFile` ×2 + 断言 `ReadCount("/fixture.bin") == 1`。
- 测试 sshfs / fusermount(3) 任一缺失自动 `t.Skip`（D-11 / D-22）；本机（macOS 无 sshfs）跑出 `--- SKIP`，符合 PASS|SKIP 验收闸门。
- 全包 `go test ./internal/cloudclaude/...` 46s 内全 PASS，无回归。

## Task Commits

Each task was committed atomically:

1. **Task 1: mount_sshfs.go 追加 4 个缓存参数（D-10 字面量修改）** — `b1d9208` (feat)
2. **Task 2: 新建 mount_sshfs_test.go fixture SFTP counting 测试（D-11）** — `bd467d0` (test)

**Plan metadata:** 本 SUMMARY + STATE/ROADMAP/REQUIREMENTS 更新随后续 docs commit 一并提交。

## Files Created/Modified

- `internal/cloudclaude/mount_sshfs.go` — sshfsCmd 字面量追加 `cache=yes,kernel_cache,auto_cache,cache_timeout=300`（D-10）。
- `internal/cloudclaude/mount_sshfs_test.go`（新建，236 行）— 含 `countingFileReader` / `noopFileWriter` / `mustSignerFromKey` / `TestSSHFSCacheHitsKernelPageCache`。

## Decisions Made

- **fixture 文件大小 512KB**：足以触发多个 SFTP packet（默认 32KB/packet 即 16 packet），同时控制 5s 挂载等待 + 测试总耗时 ≤ 1s 端到端可达。
- **sshfs 端 `password_stdin` + 空 stdin**：fixture server 端 `NoClientAuth=true` 实际放行；sshfs 不会卡在交互密码提示上。
- **`mustSignerFromKey` 不复用 host hostKeyCheck=true 的全局 fixture**：每个测试新生成 2048-bit RSA，避免 Plan 06 doctor 测试若引入相似 fixture 时 host key 冲突。
- **写入 SUMMARY 完整 sshfs 字面量**（Plan 06 doctor `sshfs_cache_args` check 比对锚点）：
  ```
  passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300
  ```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] pkg/sftp v1.13.10 Handlers.FileLister 字段名错误**
- **Found during:** Task 2（写测试时 `go doc github.com/pkg/sftp Handlers` 验证）
- **Issue:** PLAN `<action>` 字面量与 RESEARCH §F3 / PATTERNS 均使用 `sftp.Handlers{... FileLister: inMem.FileLister}`。实际 v1.13.10 字段名为 `FileList`（类型为 `FileLister` 接口），且 `InMemHandler()` 返回的 struct 同样是 `inMem.FileList`。
- **Fix:** 测试代码使用正确字段名 `FileList: inMem.FileList`。
- **Files modified:** `internal/cloudclaude/mount_sshfs_test.go`
- **Verification:** `go build` 通过；若按 PLAN 字面量则编译失败 `unknown field FileLister`。
- **Committed in:** `bd467d0`

**2. [Rule 1 - Bug] PLAN 引用的 `sftp.ReadWriteAt` 接口不存在**
- **Found during:** Task 2（同上）
- **Issue:** PLAN `<interfaces>` 块定义虚构接口 `type ReadWriteAt interface { Fileread + Filewrite }`，并让 `countingFileReader` 与 `noopReadWriteAt` 都实现。pkg/sftp v1.13.10 实际：`FileGet` 类型为 `FileReader`（仅 `Fileread(*Request) (io.ReaderAt, error)`），`FilePut` 类型为 `FileWriter`（仅 `Filewrite(*Request) (io.WriterAt, error)`）。
- **Fix:** `countingFileReader` 仅实现 `Fileread`；新增 `noopFileWriter` 仅实现 `Filewrite`（不再实现两个方法的合并接口）。
- **Files modified:** `internal/cloudclaude/mount_sshfs_test.go`
- **Verification:** `go build`/`go vet` 通过；测试 SKIP 通过（fixture server handlers 字段类型匹配）。
- **Committed in:** `bd467d0`

### 形式偏差（不计入 Auto-fixed）

- Task 2 acceptance `grep -c "TestSSHFSCacheHitsKernelPageCache"` 期望 `1`，实际 `2`（函数名同时出现在文档注释 + 函数定义）。语义满足（仅 1 个测试函数）；与 Plan 02 SUMMARY 同类形式偏差一致，无需 Rule 1 修订。

---

**Total deviations:** 2 auto-fixed（均为 Rule 1 - PLAN 引用的 pkg/sftp API 字面量与 v1.13.10 实际 API 不符）
**Impact on plan:** 修订仅替换字段名/接口拆分，测试语义完全一致；不构成 scope creep。Plan 06 引用的 pkg/sftp API 同样需按实际字段名 `FileList` 编写，已在 SUMMARY decisions 章节锚定。

## Issues Encountered

- 无。本机（Darwin，无 sshfs / fusermount）测试自动 SKIP，符合 PASS|SKIP 验收闸门；后续在 Linux 容器或 macFUSE 安装的 macOS 节点上将自动激活完整断言路径。

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- **Plan 06（doctor sshfs_cache_args check）**：可直接 `grep` mount 输出比对本 SUMMARY decisions 章节锚定的完整字面量；4 个参数顺序与 mount_sshfs.go 字面量一一对应。
- **后续 Linux CI / 真机验证**：当前测试在 sshfs 可用环境（Ubuntu CI、Linux 容器、macFUSE 安装的 macOS）会激活真实端到端断言；若 `ReadCount != 1` 则定位为 sshfs cache 参数失效或 FUSE 实现差异。
- 本 plan 无 stub、无新增安全面、无外部依赖阻塞。

## Threat Surface

本 plan 涉及威胁矩阵 T-36-05-01..03 全部为 `accept` 或 `mitigate`：
- T-36-05-03（countingFileReader 并发 Fileread）：`counter()` 用 `sync.Mutex` 保护 map 读写 + `atomic.Int64` 保证计数原子，落实 mitigate。
- T-36-05-01 / T-36-05-02：accept（cache_timeout 误差与 page cache 容量受内核 LRU 管理，非新增可利用面）。

未引入新网络端点 / 鉴权路径 / schema 变更。fixture SSH server `NoClientAuth=true` 仅在 `_test.go` 文件内、`127.0.0.1:0`（OS 分配端口）+ `t.Cleanup` 关闭 listener，生产代码零暴露。无需 `Threat Flags` 章节。

## Known Stubs

无 stub。`TestSSHFSCacheHitsKernelPageCache` 在 sshfs 可用环境中将激活完整断言路径；当前 Skip 由 `exec.LookPath` 卫士保护，符合 D-11 / D-22 设计。

## Self-Check: PASSED

- 文件存在：
  - `internal/cloudclaude/mount_sshfs.go` FOUND（已修改）
  - `internal/cloudclaude/mount_sshfs_test.go` FOUND（新建）
  - `.planning/phases/36-sshfs/36-05-SUMMARY.md` FOUND（本文件）
- 提交存在：
  - `b1d9208` FOUND（Task 1 feat）
  - `bd467d0` FOUND（Task 2 test）
- 验证：
  - `go build ./...` PASS
  - `go test ./internal/cloudclaude/... -count=1` 全包 PASS（46s，无回归）
  - `go test -run TestSSHFSCacheHits -v` 输出 `--- SKIP` + `PASS`（D-22 / 验收闸门符合）
  - grep 字面量校验全部命中（4 缓存参数、ConnectTimeout=10,cache=yes 计数=1、旧字面量已替换）

---
*Phase: 36-sshfs*
*Completed: 2026-04-23*
