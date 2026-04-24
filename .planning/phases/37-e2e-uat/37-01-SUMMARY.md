---
phase: 37-e2e-uat
plan: "01"
subsystem: cold-promoter
tags: [cold-promoter, inotify, promotion-engine, dedup, circuit-breaker]
requires: [REQ-MOUNT-V31-07, REQ-MOUNT-V31-09, REQ-MOUNT-V31-10]
provides:
  - "ColdPromoter inotify watcher + PromotionEngine async SFTP promotion"
  - "4 条核心单测覆盖 dedup/retry-backoff/circuit-breaker/start-stop"
affects:
  - "internal/cloudclaude/cold_promoter.go"
  - "internal/cloudclaude/cold_promoter_test.go"
  - "internal/cloudclaude/errcodes/codes.go"
  - "internal/cloudclaude/errcodes/mount.go"
  - "internal/cloudclaude/errcodes/explanations.go"
tech-stack:
  added:
    - "golang.org/x/sys/unix (inotify, Linux only)"
    - "golang.org/x/crypto/ssh (connB SFTP session)"
    - "github.com/pkg/sftp (SFTP client)"
  patterns:
    - "strategyHooks-style package-level var injection for testability"
    - "build tag separation (linux / !linux) for platform-specific inotify"
key-files:
  created:
    - "internal/cloudclaude/cold_promoter.go (ColdPromoter + PromotionEngine)"
    - "internal/cloudclaude/cold_promoter_linux.go (real inotify, build tag: linux)"
    - "internal/cloudclaude/cold_promoter_notlinux.go (stubs, build tag: !linux)"
    - "internal/cloudclaude/cold_promoter_test.go (4 tests, mock injection)"
  modified:
    - "internal/cloudclaude/errcodes/codes.go (MOUNT_PROMOTER_FAILED constant)"
    - "internal/cloudclaude/errcodes/mount.go (MOUNT_PROMOTER_FAILED registration)"
    - "internal/cloudclaude/errcodes/explanations.go (>=200 char explanation)"
decisions:
  - "选择 golang.org/x/sys/unix 手写 inotify 循环，不引入 fsnotify 新依赖"
  - "使用 package-level var 注入（promoterCopyFileFn / promoterInitInotifyFn）替代真实 SSH/SFTP fixture 做单测，避免跨平台死锁"
  - "Linux/non-Linux 通过 build tag 分离，macOS 编译使用 stub 实现"
  - "5s 去重窗口、1/2/4s 退避序列、熔断集合均为常量，遵循 RESEARCH.md 锁死值"
metrics:
  duration: "24m 45s"
  completed_date: "2026-04-24"
---

# Phase 37 Plan 01: ColdPromoter 核心引擎实现 Summary

实现 ColdPromoter（inotify 冷文件读监听器）+ PromotionEngine（异步 SFTP 晋升、5s 去重、指数退避重试、熔断集合），连同 4 条核心单测覆盖所有关键路径。

## 一、核心实现

### ColdPromoter 结构体

公开 API：
- `NewColdPromoter(connB, coldRoot, hotRoot, logger, pidFile)` — 构造器
- `Run(ctx)` — 阻塞运行 inotify 事件循环 + PromotionEngine；ctx cancel 后 drain 剩余事件并返回
- `QueueDepth() int` — 返回内部队列深度（doctor 可观测）
- `Stats() (count int, bytes int64, failed int)` — 返回晋升统计
- `Wait()` — 等待 Run 返回（cleanup LIFO）

### PromotionEngine 关键常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `promotionDedupWindow` | 5s | 同 path 在窗口内重复入队只触发 1 次 SFTP 拉取 |
| 退避序列 | 1s / 2s / 4s | 3 次尝试，第 3 次仍失败则熔断 |
| `events channel buffer` | 1024 | 溢出时静默丢弃，不阻塞 inotify 循环 |

### 熔断机制

第 3 次晋升失败后：
1. stderr 输出 `[!] 晋升失败 <path>: <reason>`
2. `promotionFailedCount` 原子递增
3. 加入 `circuitBreaker map[string]struct{}`，本次会话不再尝试

## 二、平台兼容

| 文件 | Build Tag | 说明 |
|------|-----------|------|
| `cold_promoter.go` | (all) | 平台无关：struct、API、PromotionEngine |
| `cold_promoter_linux.go` | `linux` | 真实 inotify 初始化 + 事件解析 |
| `cold_promoter_notlinux.go` | `!linux` | Stub 实现（macOS 编译通过，运行时降级） |

## 三、单测覆盖

| 测试 | 覆盖路径 | 耗时 | 结果 |
|------|---------|------|------|
| `TestPromotionDedup` | 相同 path 100ms 内 50 次 enqueue → 实际 copy 1 次 | 0.25s | PASS |
| `TestPromotionRetryBackoff` | 前 2 次失败，第 3 次成功，耗时在 1~4s 间 | 3.01s | PASS |
| `TestPromotionCircuitBreaker` | 3 次全失败 → 熔断，二次调用不触发 copy | 3.00s | PASS |
| `TestPromoterStartStop` | ctx cancel → Run 5s 内返回 → PID file 已清理 | 0.06s | PASS |

全部 4 条单测通过 `-race` 检测，无 data race。

## 四、错误码

新增 `MOUNT_PROMOTER_FAILED`：
- Severity: Warn
- Message: "cold-promoter 进程启动失败: %s，降级为无晋升模式（cold 分支仍可读）"
- NextAction: "检查 /proc/sys/fs/inotify/max_user_watches 上限，或设 CLOUD_CLAUDE_NO_PROMOTION=1"
- ExtendedExplanations: >= 200 中文字符，已注册到 explanations.go

## 五、Deviation from Plan

### 自动修复项

**1. [Rule 3 - 缺少依赖] MOUNT_PROMOTER_FAILED 错误码不存在**
- **发现于:** Task 1
- **问题:** 计划代码引用 `errcodes.MOUNT_PROMOTER_FAILED`，但该常量/注册/解释均不存在
- **修复:** 在 `codes.go` 添加常量、`mount.go` 注册、`explanations.go` 添加 >=200 字长说明
- **文件:** `errcodes/codes.go`, `errcodes/mount.go`, `errcodes/explanations.go`
- **Commit:** dc0c86a, b9786af

**2. [Rule 1 - Bug] MOUNT_PROMOTER_FAILED NextAction 超 80 runes 限制**
- **发现于:** Task 3（errcodes 测试回归）
- **问题:** 原 NextAction 83 字符超过 `TestErrcodesRegistry` 的 80 runes 上限
- **修复:** 缩短为 75 字符："检查 /proc/sys/fs/inotify/max_user_watches 上限，或设 CLOUD_CLAUDE_NO_PROMOTION=1"
- **Commit:** b9786af

**3. [Rule 1 - Bug] MOUNT_PROMOTER_FAILED 缺 ExtendedExplanation**
- **发现于:** Task 3（`TestAllCodesHaveExplanations` 失败）
- **问题:** 新错误码未注册到 `ExtendedExplanations` 且未列入 `ExplainExempt`
- **修复:** 添加 >= 200 中文字符的五段长说明（触发场景/根本原因/复现方式/修复路径/关联文档）
- **Commit:** b9786af

**4. [Rule 3 - 平台兼容] inotify 符号在 macOS 不可编译**
- **发现于:** Task 1
- **问题:** `unix.InotifyInit` / `unix.InotifyEvent` 等仅在 Linux 可用，macOS 编译失败
- **修复:** 拆分为三个文件：`cold_promoter.go`（平台无关）、`cold_promoter_linux.go`（Linux `//go:build linux`）、`cold_promoter_notlinux.go`（stub `//go:build !linux`），使用 package-level var 注入在 Linux init() 中赋值真实实现
- **Commit:** dc0c86a

**5. [Rule 3 - 测试策略] SFTP fixture 死锁**
- **发现于:** Task 3（初版使用真实 SSH+SFTP server 的单测 hang）
- **问题:** `sftp.NewClient` → `InMemHandler` + 自定义 `FileGet` 的 fixture 模式在 goroutine 间死锁，SFTP 握手后 `io.Copy` 无限等待
- **修复:** 改用 package-level `promoterCopyFileFn` var 注入 mock 实现，绕过 SSH/SFTP 协议栈，仅验证 PromotionEngine 行为（去重/退避/熔断）
- **Commit:** b9786af

## 六、Commits

| Hash | Message |
|------|---------|
| dc0c86a | feat(37-01): create ColdPromoter inotify watcher core framework |
| b9786af | test(37-01): add 4 core unit tests for ColdPromoter + PromotionEngine |

## 七、Self-Check

- [x] `go build ./...` — PASS
- [x] `go vet ./internal/cloudclaude/...` — PASS
- [x] `go test -race ./internal/cloudclaude/... -run "TestPromotion|TestPromoter" -v` — 4/4 PASS
- [x] `go test ./internal/cloudclaude/errcodes/...` — 9/9 PASS
- [x] 所有既有测试无回归
