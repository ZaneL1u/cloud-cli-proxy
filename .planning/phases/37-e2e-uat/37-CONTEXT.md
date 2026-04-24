# Phase 37: 冷文件读触发晋升 + e2e UAT - Context

**Gathered:** 2026-04-24
**Status:** Ready for planning

<domain>
## Phase Boundary

在容器内 cold 分支常驻 inotify watcher，命中读事件后异步把文件经 SFTP 拉到 hot 分支，让"二进制按需读"语义从"sshfs 每次回源"升级为"读一次后变热"。同时补齐 doctor 可观测（4 项新 check）、runbook 运维手册（Pattern G）、e2e UAT 验收脚本三块。

**范围锚点：** 本 phase 只做"冷文件读触发 → 异步晋升 hot"这一条链路，不涉及跨会话持久缓存、hot_sync 主路径改造、rename/move 检测等 deferred 项目。
</domain>

<decisions>
## Implementation Decisions

### Watcher 策略
- inotify 监听整个 cold 分支根目录（`IN_OPEN` / `IN_ACCESS`），不按文件类型过滤
- watcher 启动失败 stderr 输出 `MOUNT_PROMOTER_FAILED`，降级为"无晋升"模式（cold 仍可读），不阻断 mount
- mount cleanup（LIFO）时回收 watcher 进程：cancel ctx → wait 进程退出 → fusermount → rmdirChain
- 异常退出残留进程由下次 mount 启动前 `pkill -f cold-promoter` 清理

### PromotionEngine 行为
- 5s 防抖去重：相同 path 在窗口内重复入队只触发 1 次实际 SFTP 拉取
- 失败按 1/2/4s 指数退避重试，第 3 次仍失败时 stderr 输出 `[!] 晋升失败 <path>: <reason>` 并加入熔断列表，本次会话不再尝试该文件
- 无文件大小上限（用户主动读 = 意图明确），500MB 文件晋升期间 cold 仍可读、不阻塞用户操作
- 复用 connB SFTP client，异步入队不阻塞 inotify 事件循环

### mergerfs 命中策略
- 依赖现有 `category.create=ff` 行为：晋升完成后文件在 hot 分支（RW，分支顺序第一位），mergerfs 天然命中 hot
- 无需修改 mergerfs 配置或分支顺序

### last-session.json 扩展
- 新增 `promotion_count` / `promotion_bytes` / `promotion_failed_count` 三字段（omitempty）
- schema_version 保持 1（向后兼容）

### 环境变量控制
- `CLOUD_CLAUDE_NO_PROMOTION=1` 完全关闭晋升机制：watcher 不启动、PromotionEngine 不构造
- 用户主动触发的晋升不被 ignore 二次过滤（已主动读 = 用户意图明确）

### Doctor 可观测
- 4 项新 check：`promoter_alive`（pgrep cold-promoter）/ `promotion_queue_depth`（PromotionEngine 内部队列深度）/ `promotion_total`（last-session.json 累计）/ `promotion_failed_total`（last-session.json 累计）
- 遵循 Phase 36 的 check 注册模式：plain functions，在 RunDoctor 的 mount 域直接调用
- JSON 输出可被 `make ci-doctor-grep` 锁定

### 错误码
- 新增 `MOUNT_PROMOTER_FAILED`（severity=warn）：watcher 启动失败时输出，附 ≥200 字 ExtendedExplanation
- `cloud-claude explain MOUNT_PROMOTER_FAILED` 子进程测试通过

### Runbook 运维手册
- 文件：`docs/runbooks/v31-cold-promotion.md`
- 遵循 PATTERNS Pattern G：头部 + ≥5 章节 + 快速诊断命令小节
- 覆盖：原理图（cold sshfs → inotify → SFTP → hot → mergerfs）、`CLOUD_CLAUDE_NO_PROMOTION` 关闭场景、晋升失败排障、与 mergerfs / hot_sync 协同的边界、5 个相关错误码反查

### E2E UAT 脚本
- 文件：`tests/scripts/uat-v31-promotion.sh`
- Bash 脚本，与现有 `uat-network-resilience.sh` 风格一致
- 构造 fixture（10 个二进制 + 1 个 60MB + git 仓库 / 非 git 目录）→ 全场景断言（拒绝挂载 / 大文件熔断 / FUSE cache 命中 / 冷文件晋升）→ 输出 JSON 报告（schema_version=1）
- `--dry-run` 默认安全（只打印将执行的操作），`--confirm-destructive` 触发实际操作
- CI 接入 `make ci-gate`

### Claude's Discretion
- inotify 库选择（`golang.org/x/sys/unix` 或 `fsnotify`）
- PromotionEngine 内部队列实现细节（channel / ring buffer / 其他）
- PromotionEngine 单测的具体 mock 策略（fake SFTP client / counting proxy）
- Doctor check 的具体实现细节（超时值、脚本内容）
- Runbook 的具体章节组织和措辞
- UAT 脚本的具体 fixture 构造方式和断言写法
- 熔断列表数据结构（map[string]struct{} / sync.Map / 其他）
</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- **MountStrategy 框架**（`mount_strategy.go`）：Full 模式的 tryModeReal 已建立 hot staging / cold staging / merge 三步流程，PromotionEngine 在 Full 模式 mount 就绪后启动
- **connB SFTP client**（`hot_sync.go`）：已有 SFTP 文件读写模式（`copyRemoteToLocal` / `copyLocalToRemote`），PromotionEngine 复用同一 client
- **mergerfs 配置**（`mount_merge.go`）：`category.create=ff` + 分支顺序 `hot=RW:cold=RO` 已确保 hot 优先命中，无需修改
- **last-session.json**（`last_session.go`）：已有 `WriteLastSession` / `LoadLastSession` 和 omitempty 扩展模式（oversized_files 先例）
- **Doctor check 模式**（`doctor/mount.go`）：Phase 36 新增 5 项 check 已建立注册模式（plain function + 直接调用），4 项晋升 check 遵循相同模式
- **错误码注册**（`errcodes/mount.go`）：init() + MustRegister 模式，Phase 36 新增 4 条先例
- **LIFO cleanup**（`mount_strategy.go`）：cancel watcher ctx 已预留 hook 点，PromotionEngine watcher 挂入相同链路
- **ci-doctor-grep.sh**（`scripts/ci-doctor-grep.sh`）：M14 contract gate，新增 doctor check 自动纳入验证

### Established Patterns
- Go 单测使用标准 `testing` 包 + testify assert
- 错误码命名：`MOUNT_<COMPONENT>_<FAILURE_MODE>`（大写 + 下划线）
- 配置从 `~/.cloud-claude/config.yaml` 读取，环境变量覆盖
- stderr 输出格式：`[!] <描述>`（warn）/ `[CODE] <描述>`（带错误码）
- 中文 UI 约定：doctor check 的 next_action 以"建议:"开头

### Integration Points
- **mount_strategy.go:tryModeReal** — PromotionEngine 在 Full 模式 merge 就绪后启动，在 cleanup 时回收
- **hot_sync.go:copyRemoteToLocal** — PromotionEngine 复用 SFTP 拉取逻辑
- **doctor/mount.go:RunDoctor mount 域** — 4 项新 check 插入此处
- **errcodes/mount.go:init()** — 新错误码在此注册
- **last_session.go:LastSessionSnapshot** — 新增 3 个 promotion 字段
- **Makefile:ci-gate** — UAT 脚本接入 `make ci-gate`
</code_context>

<specifics>
## Specific Ideas

- REQ-MOUNT-V31-07 到 16 对 PromotionEngine 行为、doctor check、runbook 结构、UAT 脚本场景均有精确约束，无需额外补充
- Phase 36 的代码模式（oversized_files、doctor check 注册、错误码注册）是 Phase 37 的直接模板
</specifics>

<deferred>
## Deferred Ideas

- 跨会话持久缓存（hot 分支退出后保留）— v3.2 评估
- hot_sync 主路径改 inotify/fsevents 替代轮询 — v3.2 评估
- rename / move 检测优化 — v3.2 评估
- LRU 驱逐策略（hot 分支无限增长保护）— v3.2 评估
</deferred>

---

*Phase: 37-e2e-uat*
*Context gathered: 2026-04-24*
