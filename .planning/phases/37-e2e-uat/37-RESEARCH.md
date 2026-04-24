# Phase 37: 冷文件读触发晋升 + e2e UAT - Research

**Researched:** 2026-04-24
**Status:** Ready for planning

## 1. 技术选型：inotify 库

**推荐：`golang.org/x/sys/unix` + 手写 epoll/inotify 循环**

理由：
- 容器内 watcher 运行在 Linux，无需跨平台（macOS 侧不跑 watcher）
- `golang.org/x/sys/unix` 已是项目间接依赖（通过 `golang.org/x/crypto` 链），零新依赖
- `fsnotify` 在容器内 inotify 场景无额外优势，且引入缓冲 channel 不如直接 `unix.InotifyRead` 控制粒度
- watcher 只监听 `IN_OPEN | IN_ACCESS | IN_CLOSE_NOWRITE` 三个事件，逻辑简单

**备选：`fsnotify`** -- 若发现 `x/sys/unix` 的 epoll 循环代码量过大（>150行），可切换到 `fsnotify` 简化代码。两者都支持 `AddRecursive` 语义（需自行递归或借助 `fsnotify` v1.8+ 的 `WithRecursive`）。

**不推荐：`inotify` 纯 C 封装** -- 增加 CGO 依赖和跨编译器问题，与项目现有全 Go 策略冲突。

**决策：手写 `golang.org/x/sys/unix` inotify 循环**，复刻 `SSHFSWatcher` 的 `Run(ctx)` + `time.Ticker` 模式，文件变更事件走 buffered channel (depth 1024) 入队到 PromotionEngine。

## 2. PromotionEngine 设计

### 2.1 队列与去重

- 内部数据结构：`map[string]*promotionEntry` + `sync.Mutex`
- `promotionEntry` 含 `lastEnqueued time.Time`，Enqueue(path) 调用时检查：同 path 在 5s 内再次入队 → 仅更新 `lastEnqueued`，不触发新拉取
- 事件通道：`chan string`（path only），buffered=1024，drain 后做 5s debounce ticker 批量处理
- 去重窗口 5s 按 CONTEXT.md 锁死

### 2.2 SFTP 拉取

- 复用 `connB *ssh.Client`，通过 `sftp.NewClient(connB)` 创建独立 SFTP session（不共享 HotSyncEngine 的 client，避免锁竞争）
- 拉取逻辑复用 `HotSyncEngine.copyRemoteToLocal` 的模式：`sftpClient.Open(remotePath)` → `os.Create(localPath)` → `io.Copy`
- 目标路径：`<hotRoot>/<relPath>`（cold 分支路径映射到 hot staging 目录）
- 写完后不调 Chtimes（晋升时间用当前时间，保持 hot/last 一致性由 mergerfs 管理）

### 2.3 失败重试与熔断

- 退避序列：1s / 2s / 4s（三次尝试）
- 第 3 次仍失败：stderr 输出 `[!] 晋升失败 <path>: <reason>` + 加入熔断集合 `map[string]struct{}{}`
- 熔断集合存活整个会话生命期；后续同 path 的 inotify 事件直接丢弃（`Enqueue` 时检查）
- 熔断不影响其他文件晋升，不阻塞 inotify 事件循环

### 2.4 生命周期

```
mount ready → NewPromotionEngine(connB, hotRoot, coldRoot, logger) → go engine.Run(ctx)
cleanup → cancel ctx → engine.waitDone() → fusermount → rmdirChain
```

- `Run(ctx)` 内：`select { case <-ctx.Done(): return; case path := <-events: handlePath(path) }`
- `handlePath` 内异步 goroutine（每文件一个），不阻塞事件循环
- 优雅退出：ctx cancel → drain 剩余事件队列（最多 5s，不等待新事件）→ 返回

## 3. watcher 进程生命周期

### 3.1 进程命名

- 进程名（`/proc/self/comm`）设为 `cold-promoter`，确保 `pgrep -f cold-promoter` 可发现
- Go 中通过 `os.Args[0]` 不可改，实际方案：启动时写 PID file `~/.cloud-claude/cold-promoter.pid`，`pgrep` 检查命令改为 `pgrep -F ~/.cloud-claude/cold-promoter.pid` 或直接 `pgrep -f "cold-promoter"` 通过 goroutine 内嵌标签（不适用）。**实际方案：新增独立的轻量子进程 `cold-promoter` 或使用 goroutine + PID file。**
- **推荐方案：goroutine 内运行 inotify 循环，同时写 PID file。doctor `promoter_alive` 检查 `kill -0 $(cat pidfile)`**。pgrep 语义在 Go 单进程中不可靠（goroutine 不是进程），用 PID file + kill -0 替代。

### 3.2 启动时机

在 `tryModeReal` 的 Full 路径中，mergerfs 就绪后、cleanup 闭包构造前：

```go
// 启动 cold-promoter: inotify watcher + PromotionEngine
promoterCtx, promoterCancel := context.WithCancel(context.Background())
promoter := NewColdPromoter(connB, coldRoot, hotRoot, cfg.Logger)
go promoter.Run(promoterCtx)
// PID file 写入 promoter.pid
```

cleanup LIFO 顺序：`promoterCancel → promoter.Wait() → mergeCleanup → sCleanup → hCleanup → rmdirChain`

### 3.3 异常退出清理

- 下次 mount 启动前（`tryModeReal` 入口），执行 `kill -0 $(cat pidfile) 2>/dev/null && kill $(cat pidfile) 2>/dev/null || true`
- 残留 PID file 被新 PID 覆盖（原子替换）
- 不做 `pkill -f cold-promoter`（可能误杀其他 cloud-claude 实例，单宿主机多用户场景不安全）

### 3.4 启动失败降级

- watcher 启动失败（inotify init 失败 / PID file 写失败）→ stderr 输出 `MOUNT_PROMOTER_FAILED` + 降级为"无晋升"模式
- cold 仍可读（sshfs 路径不依赖 watcher）
- 不阻断 mount 主路径

## 4. 错误码新增

### MOUNT_PROMOTER_FAILED

- Severity: Warn
- Message: "cold-promoter 进程启动失败: %s，降级为无晋升模式（cold 分支仍可读）"
- NextAction: "检查容器内 /proc/sys/fs/inotify/max_user_watches 限制，或设置 CLOUD_CLAUDE_NO_PROMOTION=1 显式关闭"
- ExtendedExplanation: >= 200 中文字符，覆盖：inotify watch 数量不足、PID file 目录不可写、手动关闭途径、降级模式影响范围（每次读都回源 sshfs，无性能优化但功能完整）

### 注册位置

`errcodes/mount.go::init()` 内追加一条 `MustRegister`，`codes.go` 常量区追加 `MOUNT_PROMOTER_FAILED`。

## 5. last-session.json 扩展

在 `LastSessionSnapshot` 末尾追加三个 omitempty 字段：

```go
PromotionCount      int   `json:"promotion_count,omitempty"`
PromotionBytes      int64 `json:"promotion_bytes,omitempty"`
PromotionFailedCount int  `json:"promotion_failed_count,omitempty"`
```

- PromotionEngine 在每文件晋升成功后 `atomic.AddInt64` 更新内部计数器
- mount cleanup 时 flush 到 snapshot（`WriteLastSession` 前赋值）
- schema_version 保持 1（omitempty 向后兼容）

## 6. Doctor 新增 4 项 Check

### 6.1 promoter_alive

- 读取 PID file `~/.cloud-claude/cold-promoter.pid`
- `kill -0 <pid>` 判断进程存活
- 文件不存在 → `newSkip`
- 进程存活 → `newPass`
- 进程已死 → `newWarn(MOUNT_PROMOTER_FAILED)` 

### 6.2 promotion_queue_depth

- PromotionEngine 暴露 `QueueDepth() int` 方法
- doctor 通过读取共享状态（如 PID file 旁的状态文件 `~/.cloud-claude/promoter-state.json`）来获取
- **实际方案：last-session.json 里追加 `promotion_queue_depth` 字段（临终快照），doctor 读 last-session.json**
- 深度 0 → `newPass`，深度 > 10 → `newWarn`

### 6.3 promotion_total

- 读 `last-session.json::promotion_count`
- 缺失 → `newSkip`（无晋升记录）
- 存在 → `newPass("mount", "promotion_total", "累计晋升 N 个文件")`

### 6.4 promotion_failed_total

- 读 `last-session.json::promotion_failed_count`
- 缺失 → `newSkip`
- > 0 → `newWarn(MOUNT_PROMOTER_FAILED, ...)`
- == 0 → `newPass`

### 集成方式

在 `doctor.go::RunDoctor` 的 mount domain check 列表中追加 4 个函数调用，与 Phase 36 的 5 项 check 相邻。

## 7. CLOUD_CLAUDE_NO_PROMOTION 环境变量

- `tryModeReal` 入口检查 `os.Getenv("CLOUD_CLAUDE_NO_PROMOTION") == "1"`
- 为 "1" 时：跳过 watcher 构造 + PromotionEngine 构造，直接返回不含 promoter 的 cleanup
- last-session.json 的 `promotion_count` / `promotion_bytes` / `promotion_failed_count` 保持默认零值（omitempty → JSON 不出现）
- 用户主动读（inotify 事件）不与 ignore 规则交互：晋升逻辑在 watcher 命中后触发，ignore 规则仅用于 hot_sync 初始扫描

## 8. mergerfs 自然命中策略

无需额外实现。现有 `category.create=ff` + 分支顺序 `hot=RW:cold=RO` 已确保：
1. 晋升完成 → 文件出现在 hot 分支（RW 层）
2. 下次 `cat` → mergerfs 按分支顺序先查 hot → 命中 → 不穿透到 cold
3. `category.create=ff`（first-found）策略天然命中 hot

e2e 验证时断言 SFTP read count：首次读 +N（穿透到 cold/sshfs），第二次读 0（hot 命中）。

## 9. Runbook 结构 (Pattern G)

文件：`docs/runbooks/v31-cold-promotion.md`

章节规划（>= 5 章节 + 快速诊断命令小节）：

1. **概述与原理** -- cold sshfs → inotify → SFTP → hot → mergerfs 完整数据流图，ASCII 时序图
2. **启动与关闭** -- Full 模式自动启动、cleanup 自动回收、`CLOUD_CLAUDE_NO_PROMOTION=1` 关闭
3. **晋升失败排障** -- 常见失败模式（SFTP 断连、磁盘满、权限不足）、熔断列表含义、手动触发晋升
4. **与 mergerfs / hot_sync 协同** -- 三层文件系统的边界、晋升文件在 hot 分支的生命周期、不与 hot_sync 轮询冲突
5. **错误码反查** -- MOUNT_PROMOTER_FAILED + MOUNT_HOT_SYNC_FAILED + MOUNT_SSHFS_FAILED + MOUNT_SSHFS_DISCONNECTED + MOUNT_MERGERFS_FAILED 五码快速索引
6. **快速诊断命令** -- `pgrep -f cold-promoter`、`cat ~/.cloud-claude/last-session.json | jq '.promotion_count'`、`docker exec <ctr> ls /tmp/.cloud-claude-mounts/<hash>/hot/`、`cloud-claude doctor mount --json | jq '.checks[] | select(.name | startswith("promotion"))'`

## 10. UAT 脚本设计

文件：`tests/scripts/uat-v31-promotion.sh`

参照 `uat-network-resilience.sh` 风格：
- `set -euo pipefail`
- `pass()` / `fail()` / `skip()` / `info()` helper 函数
- `usage()` 帮助文本
- `--dry-run` 默认安全（只打印操作描述）
- `--confirm-destructive` 触发实际 mount + read 操作
- 退出码 0=PASS / 1=FAIL / 2=SKIP

### 场景矩阵

| # | 场景 | 关键断言 |
|---|------|---------|
| 1 | 非 git 目录拒绝挂载 | stderr 含 MOUNT_REQUIRE_GIT_REPO |
| 2 | 大文件熔断 | stderr 含 "跳过大文件"；hot 分支无该文件 |
| 3 | FUSE cache 命中 | 首次 cat → SFTP read N 次；二次 cat → SFTP read 不变 |
| 4 | 冷文件晋升 | 首次 cat fixture.png → wait 5s → hot 分支出现同 path → 二次 cat → SFTP read 不变 |
| 5 | CLOUD_CLAUDE_NO_PROMOTION=1 | watcher 不启动；promotion_count=0 |
| 6 | JSON 报告 | schema_version=1 |

### Fixture 构造

- 10 个小二进制文件（1KB-100KB，png/jpg/pdf/bin）
- 1 个 60MB 大文件（触发熔断）
- git 仓库 / 非 git 目录两种场景
- 使用 `mktemp -d` 创建临时目录，trap EXIT 清理

### CI 接入

在 Makefile `ci-gate` target 追加一行：

```makefile
bash tests/scripts/uat-v31-promotion.sh --dry-run
```

## 11. 单测策略

### PromotionEngine 单测

- `TestPromotionDedup`：相同 path 100ms 内 50 次 enqueue → 实际拉取 1 次（用 counting proxy 验证）
- `TestPromotionRetryBackoff`：fake SFTP client 前 2 次失败、第 3 次成功 → 验证退避序列 1/2/4s
- `TestPromotionCircuitBreaker`：3 次全失败 → 熔断标记置位 → 后续 enqueue 不触发拉取
- `TestPromotionNoBlock`：500MB 文件拉取期间 cold 仍可读（goroutine 不阻塞主循环）

### Watcher 单测

- `TestWatcherStartStop`：ctx cancel → Run 返回
- `TestWatcherPIDFile`：启动后 pidfile 存在、kill -0 成功
- `TestPromoterAliveCheck`：模拟 doctor check promoter_alive 读 pidfile

### explain 子进程测试

- `TestExplain_MOUNT_PROMOTER_FAILED`：`cloud-claude explain MOUNT_PROMOTER_FAILED` 输出含 ≥200 中文字符

## 12. 文件清单

新增文件：
- `internal/cloudclaude/cold_promoter.go` — ColdPromoter（inotify watcher）+ PromotionEngine
- `internal/cloudclaude/cold_promoter_test.go` — 单测
- `docs/runbooks/v31-cold-promotion.md` — 运维手册
- `tests/scripts/uat-v31-promotion.sh` — e2e UAT 脚本

修改文件：
- `internal/cloudclaude/mount_strategy.go` — tryModeReal Full 路径集成 ColdPromoter
- `internal/cloudclaude/last_session.go` — 新增 3 个 promotion 字段
- `internal/cloudclaude/errcodes/mount.go` — 注册 MOUNT_PROMOTER_FAILED
- `internal/cloudclaude/errcodes/codes.go` — 声明 MOUNT_PROMOTER_FAILED 常量
- `internal/cloudclaude/doctor/mount.go` — 新增 4 项 promotion check
- `Makefile` — ci-gate 追加 UAT dry-run

## 13. 风险与缓解

| 风险 | 缓解 |
|------|------|
| inotify watch 数量不足（`/proc/sys/fs/inotify/max_user_watches` 默认 8192） | watcher 监听 cold 分支根目录递归，大仓库可能超限；启动时检查当前 watch 数量并 warn |
| goroutine 泄漏 | ctx cancel → select 退出 → Run 返回；cleanup 有 Wait() 同步点 |
| PID file 残留 | 启动前 `kill -0` + cleanup 时 `os.Remove`；trap EXIT 兜底 |
| 大文件晋升阻塞 SFTP | 单 goroutine 异步，不阻塞事件循环；500MB 期间 cold 仍可读 |

---

*Phase: 37-e2e-uat*
*Research completed: 2026-04-24*
