# Phase 36: 映射前置约束 + sshfs 内核缓存 - Context

**Gathered:** 2026-04-23
**Status:** Ready for planning
**Mode:** `--auto`（所有 gray area 自动选定推荐项；决策来源标注于 `<decisions>` 各条尾注）

<domain>
## Phase Boundary

把 v3.0 已交付的「无约束 mount + 全透传 sshfs」升级为「**git 仓库强约束 + 单文件 50MB 熔断 + FUSE page cache 命中**」三条配置/校验/参数级硬约束，**零新增依赖、零跨进程协议变更、零 host-agent endpoint 扩展**，独立可发。

本阶段交付（围绕 6 条 REQ-MOUNT-V31-01..06 与 ROADMAP §Phase 36 6 条 Success Criteria）：

1. **F1 · Git 仓库前置约束**（REQ-01 / SC#1）
   - 在 `cmd/cloud-claude/main.go::startSession`（cwd 解析后、Entry API auth 之前）执行本地 `git rev-parse --show-toplevel`；非 git 仓库立即拒绝挂载、不发起任何 SSH/SFTP/Mutagen 子进程
   - 错误码 `MOUNT_REQUIRE_GIT_REPO`（severity=error）+ 中文 next_action（`cd` 到 git 仓库 / `git init` 当前目录）
   - 退出码恒为 `exitConfigError`（=4），不可走任何降级路径，与 `--mount-mode=full|hot-only|sshfs-only` 正交（任何模式都先过这道闸）
2. **F2 · 单文件大小熔断**（REQ-02 / REQ-03 / SC#2）
   - `~/.cloud-claude/config.yaml` 新增字段 `hot_sync_max_file_mb: int`（默认 50；零值/缺失走默认）
   - `HotSyncEngine` 初始化扫描阶段：**ignore 命中优先于大小检查**（默认黑名单 + .gitignore 命中的 50MB 视频不计入熔断列表，避免重复刷屏）；**未被 ignore 命中且 size ≥ 阈值** 的文件 → 不进入热同步（不写 `/workspace-hot`、不阻断 mount），由 cold sshfs 兜底
   - 首次扫描结束后**一次性**输出 `[!] 跳过大文件: <rel1> (XX MB), <rel2> (YY MB)... 共 N 个，由 cold 兜底`（不刷屏 N 行）
   - `last-session.json` 新增 `oversized_files: [{path, size_bytes}]` 数组（schema_version=1 不变，omitempty）
3. **F3 · sshfs FUSE page cache**（REQ-04 / SC#3）
   - `mount_sshfs.go::mountSSHFS` 的 sshfs 命令在现有 4 个抗抖参数后追加 4 个缓存参数：`cache=yes,kernel_cache,auto_cache,cache_timeout=300`
   - 单测覆盖：fixture SFTP server + per-path read 计数器，断言"同会话同文件 cat 2 次 → server-side read = 1"（首次 read 后 FUSE page cache 接管，无额外 RTT）
4. **F4 · doctor mount 扩展 5 项 check**（REQ-05 / SC#4）
   - `internal/cloudclaude/doctor/mount.go` 新增 5 项检查（沿用 `Checker interface{ Run; Fix }` + `runWithTimeout` + `RemoteRunner` 既有骨架，**不新建文件**）：
     a. `require_git_repo` — 本地 `git rev-parse --show-toplevel`，命中 fail → `MOUNT_REQUIRE_GIT_REPO`
     b. `oversized_files_count` — 读 `last-session.json::oversized_files`，N>0 → warn + 错误码 `MOUNT_OVERSIZED_FILE_SKIPPED`
     c. `sshfs_cache_args` — 远端 `mount | grep sshfs` 输出断言含全部 4 个 `cache=yes,kernel_cache,auto_cache,cache_timeout=300`
     d. `git_proxy_enabled` — 本地 config `proxy_commands` 是否含 `git`（默认值已含；显式覆盖时校验）
     e. `default_ignore_loaded` — 检查 `CLOUD_CLAUDE_NO_DEFAULT_IGNORE` 环境变量是否禁用了默认黑名单（启用 → warn 提示该环境变量含义）
   - JSON `schema_version=1` 不变，新增 5 个 check 节点；总检查数从 v3.0 的 ≥17 提升到 ≥22（SC#4 字面达标）
5. **F5 · 错误码注册表 + explain 长说明**（REQ-06 / SC#5）
   - `errcodes/mount.go` + `errcodes/codes.go::const` 新增 2 条 Code：
     - `MOUNT_REQUIRE_GIT_REPO`（severity=error，message 含中文 next_action 模板）
     - `MOUNT_OVERSIZED_FILE_SKIPPED`（severity=warn，message 含 `<path>` + `<size_mb>` 占位）
   - `errcodes/explanations.go` 注册 2 条 ≥200 中文字符的 ExtendedExplanations（沿用 v3.0 五段模板：触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档）
   - `cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 与 `cloud-claude explain MOUNT_OVERSIZED_FILE_SKIPPED` 子进程退出 0、长说明 ≥ 200 字（自动通过 v3.0 已实现的 `cmd/cloud-claude/explain.go` + Registry 闭环）
6. **F6 · CI 闸门**（SC#6）
   - `go test ./...` 全 PASS（含新增 unit / fixture 测试）
   - `make ci-gate` PASS（doctor JSON grep 闸门 + errcodes 注册表闭包测试 + ≥200 字长说明断言）

**不在本阶段交付**（推迟到 Phase 37）：
- 容器内 cold 分支 inotify watcher (`cold-promoter`)、SFTP 异步晋升、PromotionEngine 防抖熔断、`CLOUD_CLAUDE_NO_PROMOTION` 开关、`promoter_alive` / `promotion_*` 4 个新 check、`docs/runbooks/v31-cold-promotion.md` 运维手册、`tests/scripts/uat-v31-promotion.sh` e2e UAT 脚本 — 全部 REQ-MOUNT-V31-07..16
- 任何新的 host-agent endpoint（沿用 v3.0 边界，OOS-A20 永久禁）
- 错误码 i18n（中文硬编码，与 Phase 31 D-22 / Phase 34 D-22 一致）
- 真机签字（macOS APFS 命中 50MB 熔断 + Linux Ubuntu 25.04 sshfs cache，留 Phase 37 e2e UAT 脚本附带 + ship 闸门）

</domain>

<decisions>
## Implementation Decisions

### F1 · Git 仓库前置约束 落点

- **D-01**：检测点选定 `cmd/cloud-claude/main.go::startSession`，**位置在 cwd 解析（`os.Getwd()`）之后、Entry API `AuthenticateAndWait` 之前**：
  1. 不发起任何 SSH/SFTP 子进程，符合 SC#1「立即拒绝挂载，不发起任何 SSH 文件操作」的字面语义
  2. 与 `--mount-mode` flag 正交：四档模式（auto/full/hot-only/sshfs-only）任何一档都先过这道闸；REQ-01 明确「不可走任何降级路径」
  3. 与 v3.0 现有 ParseMode 错误处理同位置（`os.Exit(exitConfigError)`），代码路径一致便于复读
  4. 实现：`exec.Command("git", "rev-parse", "--show-toplevel").Run()`；退出码非 0 即非 git 仓库（包括"未安装 git"场景，统一按"非 git 仓库"处理，避免分支爆炸）
  5. stderr 输出走 `errcodes.Format(errcodes.MOUNT_REQUIRE_GIT_REPO)`，两段格式与 v3.0 保持一致
  6. 不复用 mount_strategy 内部检查 — 那里已经发起了 SSH 连接，违反 REQ-01 字面要求

  *[auto] G1 推荐项：main.go 入口闸门（替代：mount_strategy 内部检查 — 已晚一步；替代：cobra PreRunE — 与 ParseMode 等其它入口校验风格不一致）*

- **D-02**：检测函数封装在 `cmd/cloud-claude/git_check.go` 新文件（与 main.go 同包），导出 `requireGitRepo(cwd string) error`：
  1. 单一职责，便于 unit test mock `exec.Command`
  2. 错误返回类型为 `*errcodes.CodedError`（如已有；否则用 `errors.New(errcodes.Format(...))`），方便 main.go 统一 `os.Exit(exitConfigError)` 分支
  3. 不依赖 viper / config — 在 LoadConfig 之前也能跑（虽然 D-01 选定在 LoadConfig 之后，留给后续 v3.4 如果想前移依然能用）
- **D-03**：「git 仓库」判定**包含 git worktree / submodule / detached HEAD**（`git rev-parse --show-toplevel` 在所有这些场景都返回 0）；不接受 `.git` 文件存在但 git 命令不可用的场景（环境破损按非 git 处理，引导用户 `git init`）

### F2 · 单文件大小熔断 实现

- **D-04**：`Config` struct 新增字段 `HotSyncMaxFileMB int` （yaml tag `hot_sync_max_file_mb,omitempty`）：
  1. 默认值兜底：`Config.EffectiveHotSyncMaxFileMB() int { if c.HotSyncMaxFileMB <= 0 { return 50 }; return c.HotSyncMaxFileMB }`，与 `EffectiveProxyCommands()` 同模式
  2. `Validate()` 不强校验（允许零值走默认）；上限不设硬编码（用户配 1000 也允许，自负风险）
  3. yaml 注释由 `cloud-claude init` 生成（非本阶段交付，留 v3.4 init 文案优化；本阶段直接编辑 config.yaml 即可）
- **D-05**：`HotSyncConfig` 新增字段 `MaxFileBytes int64`（由 mount_strategy 注入 `cfg.HotSyncMaxFileMB * 1024 * 1024`）：
  1. 选用 byte 而非 MB —— 与 `syncFileState.Size`（int64 byte）对齐，避免运行时换算
  2. 零值表示不熔断（向后兼容已有测试）
- **D-06**：`HotSyncEngine` 在初始化扫描阶段（`StartHotSync` 全量推送 / 现有 walk 阶段）按以下顺序判定：
  1. **第一层 ignore 过滤**（既有 `IgnoreMatcher`）：命中 → 完全跳过（既不进 hot 也不计入 oversized 列表）
  2. **第二层 size 检查**（新增）：未被 ignore 命中且 `info.Size() >= MaxFileBytes` → 跳过 + 加入 `engine.oversized []OversizedFile{Path, SizeBytes}`
  3. 其余文件正常推到 hot
  4. 顺序固定，不开放配置（避免"50MB 视频被默认黑名单忽略 + 又被 oversized 记一次"双重提示）
- **D-07**：`HotSyncStatus` 新增字段 `OversizedFiles []OversizedFile`（与 `ConflictCount` 同位置）；`StartHotSync` 返回值携带，由 `mount_strategy.MountWorkspace` 在写 `last-session.json` 时塞进 `LastSessionSnapshot.OversizedFiles`
- **D-08**：stderr 一次性提示由 `mount_strategy.MountWorkspace` 在 `StartHotSync` 返回后输出（不在 hot_sync 内部边扫边打）：
  1. 格式：`[!] 跳过大文件 N 个（>%dMB），由 cold 兜底:\n  <rel1> (%dMB)\n  <rel2> (%dMB)\n  ... 仅显示前 5 条，完整列表见 ~/.cloud-claude/last-session.json`
  2. N≤5 时全部列出；N>5 时只列前 5 条 + `... 还有 (N-5) 个见 last-session.json`
  3. 不重复输出错误码 `[MOUNT_OVERSIZED_FILE_SKIPPED]`（warn 级提示足够；doctor 维度的 `oversized_files_count` 检查会引用错误码）
- **D-09**：`LastSessionSnapshot` 新增字段：
  ```go
  OversizedFiles []OversizedFile `json:"oversized_files,omitempty"`
  ```
  + 包级新 struct：
  ```go
  type OversizedFile struct {
      Path      string `json:"path"`        // cwd 相对路径
      SizeBytes int64  `json:"size_bytes"`
  }
  ```
  - schema_version 保持 1（omitempty 字段追加，向后兼容，与 Phase 32 D-27 同策略）
  - 既有 `LoadLastSession` / `WriteLastSession` 无需改动（json.Marshal 自动序列化）

### F3 · sshfs FUSE page cache 参数

- **D-10**：`mount_sshfs.go::mountSSHFS` 中 sshfs 命令字面量直接追加：
  ```go
  sshfsCmd := fmt.Sprintf(
      "sshfs : %s -o passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300 -f",
      shellQuote(remotePath),
  )
  ```
  - 4 个缓存参数追加在抗抖参数之后，**字面量顺序锁死**便于 grep / doctor `sshfs_cache_args` check 字符串匹配
  - `cache=yes` + `kernel_cache` 让 FUSE 用内核 page cache（核心收益）；`auto_cache` 按 mtime 自动失效；`cache_timeout=300`（5 分钟）—— 与 mergerfs `cache.attr=30` 不冲突（mergerfs 是元数据缓存，sshfs cache 是数据缓存）
  - 不暴露为可配置项（用户场景一致，配置面增加一倍但收益极低；硬编码可被单元测试锁死）
- **D-11**：单元测试方案（实现在 `internal/cloudclaude/mount_sshfs_test.go`）：
  1. 用 `pkg/sftp` 构造 fixture SFTP server，包装一层 `*countingHandler` 在 `Read` / `Open` 拦截 `sftp.Request` 计数
  2. 用 `os/exec` 启动真实 sshfs（CI 已装，与 v3.0 一致）挂到临时 mountpoint
  3. 同会话 `os.Open + io.ReadAll` 同一 fixture 文件 2 次 → 断言 server-side read count == 1
  4. 失败模式（无 sshfs / FUSE 不可用）→ 单测 `t.Skip()`（与 v3.0 mount_test.go::TestMountWorkspace_RealSSHFS_Skip 同模式）
- **D-12**：参数选型论据（写入 explanations 关联文档段，便于 ship runbook 复用）：
  - `cache=yes` 是 sshfs 默认值，但显式写出便于 grep / doctor 锁定
  - `kernel_cache` 让内核接管 page cache，避免 sshfs 自己的 fragment 缓存
  - `auto_cache` 防御「外部修改文件」场景（容器内 claude code 改了 cold 分支的文件 → mtime 变 → cache 自动失效）
  - `cache_timeout=300` 在「读多写少」（典型场景：开发期间反复 cat 同一二进制资源）和「外部修改感知延迟」之间取折中

### F4 · doctor mount 5 项 check

- **D-13**：5 项 check 全部加到 `internal/cloudclaude/doctor/mount.go` 同文件（**不新建文件**），延续 v3.0 D-01 单文件多 Checker 模式：

  | check name | 远端/本地 | 错误码（命中时） | 实现要点 |
  |-----------|----------|------------------|----------|
  | `require_git_repo` | 本地（`exec.Command("git", "rev-parse", "--show-toplevel")`） | `MOUNT_REQUIRE_GIT_REPO` (fail) | 复用 D-02 的 `requireGitRepo`；无法 import cmd/ 包时复制 12 行实现到 doctor/mount.go 内部 helper |
  | `oversized_files_count` | 本地（读 `~/.cloud-claude/last-session.json`） | `MOUNT_OVERSIZED_FILE_SKIPPED` (warn) | 复用 `cloudclaude.LoadLastSession()`；nil snapshot 走 `STATE_LAST_SESSION_MISSING` skip；`OversizedFiles` 长度 0 → pass；>0 → warn + Details 列前 5 条 |
  | `sshfs_cache_args` | 远端（`runner.RunScript("sshfs_mount", "mount \| grep sshfs \| head -1")`） | `MOUNT_SSHFS_FAILED` (fail) | 复用既有 `RemoteRunner`；缺任一 cache 参数即 fail，Details 列出 missing 列表（参照既有 `checkMergerfsBranches` 实现） |
  | `git_proxy_enabled` | 本地（解析 LoadConfig） | `AUTH_CONFIG_MISSING` (warn) | 缺 `proxy_commands` 或不含 `git` → warn + 提示 `cloud-claude init` 重新生成；与 `DefaultProxyCommands` 比对 |
  | `default_ignore_loaded` | 本地（`os.Getenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE")`） | `AUTH_CONFIG_MISSING` (warn) | 设为 "1" → warn 提示「已禁用默认二进制黑名单，热同步可能引入大文件」；未设 → pass |

  - **复用错误码不新建** `git_proxy_enabled` 和 `default_ignore_loaded` 用 `AUTH_CONFIG_MISSING`（既有 v3.0 错误码）：避免错误码注册表膨胀，本阶段只新增严格必要的 2 条（REQ-06 字面要求）
- **D-14**：5 项 check 全部接入 `RunDoctor` 顶层 mount domain；总 check 数：v3.0 mount domain 4 项（`mutagen_version_match` / `mergerfs_branches` / `sshfs_mountpoint` / `fuse_residual` / `apparmor_fusermount3`）→ Phase 36 +5 项 = 9 项 mount 维度；total check 数 17 → ≥22（SC#4「比 v3.0 多 5」字面达标）
- **D-15**：`--fix` 不为本阶段 5 项新 check 提供自动修复：
  - `require_git_repo` — 用户必须自己 `git init` 或 `cd`，自动修复语义不存在
  - `oversized_files_count` — 用户必须自己加 .gitignore 规则，自动修复=猜测用户意图
  - `sshfs_cache_args` — 缺参数说明 cloud-claude 二进制本身有 bug 或被覆盖，自动修复=重启 cloud-claude
  - `git_proxy_enabled` / `default_ignore_loaded` — 都涉及用户配置，自动修复违反 PROJECT.md「优雅好用运维清晰」
  - 5 项全部走 NextAction 提示路径，不增加 FixerRegistry 入口

### F5 · 错误码 + explain 长说明

- **D-16**：`errcodes/codes.go` 在 MOUNT_* 域下新增 2 个常量（按字母序插入）：
  ```go
  MOUNT_OVERSIZED_FILE_SKIPPED Code = "MOUNT_OVERSIZED_FILE_SKIPPED"
  MOUNT_REQUIRE_GIT_REPO       Code = "MOUNT_REQUIRE_GIT_REPO"
  ```
  - 命名遵循 v3.0 D-02 正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`，单元测试自动覆盖
- **D-17**：`errcodes/mount.go` 注册 2 条 Entry（init() 内追加）：
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
- **D-18**：`errcodes/explanations.go::init()` 追加 2 条 ExtendedExplanations，遵循 v3.0 D-18 五段模板（触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档），每条 ≥ 200 中文字符。**不**追加到 `ExplainExempt`（这两条都属于实质性错误，必须有长说明）
- **D-19**：`cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 与 `cloud-claude explain MOUNT_OVERSIZED_FILE_SKIPPED` 自动通过 v3.0 已实现的 `cmd/cloud-claude/explain.go::runExplain` 路径生效；**不需要改动** explain.go 本身（依赖 Registry + ExtendedExplanations 自动注入），单测仅断言「子进程 exit 0 + stdout 含错误码 + stdout 字符数 ≥200」

### F6 · 测试与 CI

- **D-20**：单元测试矩阵：
  - `cmd/cloud-claude/git_check_test.go` — git_check 在 git 仓库 / 非 git 目录 / git 不存在三种场景的退出码与 stderr 输出
  - `internal/cloudclaude/hot_sync_oversized_test.go` — fixture 目录含「ignore 命中的 50MB 文件」+「未 ignore 的 60MB 文件」+「未 ignore 的 30MB 文件」，断言只有 60MB 文件进 OversizedFiles 列表
  - `internal/cloudclaude/last_session_test.go` 扩展 — 序列化/反序列化含 `OversizedFiles` 的 snapshot，断言 schema_version=1 + 字段顺序
  - `internal/cloudclaude/mount_sshfs_test.go` 新增 — D-11 的 fixture SFTP 计数测试（CI 装 sshfs 才跑，否则 skip）
  - `internal/cloudclaude/doctor/mount_test.go` 扩展 — 5 项新 check 的 pass / warn / fail / skip 矩阵
  - `internal/cloudclaude/errcodes/codes_test.go` 现有遍历测试自动覆盖 2 条新 code（命名 + Message + NextAction 非空）
  - `internal/cloudclaude/errcodes/explanations_test.go` 现有遍历测试自动覆盖 2 条长说明（≥200 字符 + 五段模板）
  - `cmd/cloud-claude/explain_test.go` 扩展 — 子进程 exit 0 + stdout 字符数 ≥200 断言新增 2 条
- **D-21**：`make ci-gate`（v3.0 已固化）自动覆盖：
  - errcodes 注册表无重复 + 无空字段 + 命名匹配（D-16 / D-17 自动通过）
  - ExtendedExplanations ≥200 字符（D-18 自动通过）
  - doctor JSON schema_version=1 + check 列表非空（D-13 / D-14 自动通过）
  - **不需要新增 CI job 或修改 ci-gate 脚本**
- **D-22**：性能/真机回归留 Phase 37 e2e UAT 脚本（`tests/scripts/uat-v31-promotion.sh`）附带覆盖：
  - 60MB fixture 不出现在 hot tree（ssh `find /workspace-hot` 验证）+ cold 视图能 stat
  - 同会话 cat 同一冷文件 2 次 SFTP read count = 1（macOS / Linux 双平台真机签字）
  - 本阶段单测覆盖到「Phase 36 代码内部行为」即可；e2e + 真机签字与 Phase 37 ship 闸门绑定

### Claude's Discretion

以下细节由 planner / executor 按实现便利性决定：

- 单元测试 fixture 文件大小选型（30/60/100MB 都可，与 `defaultHotSyncMaxFileMB=50` 形成测试边界即可）
- `OversizedFile.Path` 用绝对路径还是 cwd 相对路径（推荐相对，与 `IgnoreMatcher` 一致）
- stderr 一次性提示中「前 5 条」的 N 值（5 是合理默认，planner 可调整为 3 或 10）
- doctor `oversized_files_count` 检查在 last-session.json 缺失时是否走 skip（推荐 skip + 引用 `STATE_LAST_SESSION_MISSING`，与 v3.0 D-13 第二段相符）
- D-02 `requireGitRepo` 函数是否同时记录 cwd 到错误信息（推荐记录，便于排错）
- ExtendedExplanations 长说明的「关联文档」段引用哪些路径（推荐引用 REQUIREMENTS.md REQ-MOUNT-V31-01..06 + Phase 31 31-CONTEXT.md D-11 大目录熔断逻辑）

### Folded Todos

无（`gsd-sdk query todo.match-phase 36` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 规划与需求

- `.planning/PROJECT.md` — v3.1 milestone 总目标 / 5 大映射 gap / 性能验收基线 / Out of Scope 清单
- `.planning/REQUIREMENTS.md` §A1 / A2 / A3 / A4 — REQ-MOUNT-V31-01 / 02 / 03 / 04 / 05 / 06 字面要求（本阶段交付的 6 条 REQ 完整定义）
- `.planning/ROADMAP.md` §Phase 36 — Goal / Requirements / Success Criteria 6 条（plan-checker 验收基线，**任一不达标即视为缺陷**）
- `.planning/STATE.md` — v3.1 milestone 当前进度
- `.planning/MILESTONES.md` — v3.1 与历史版本的关系（v3.0 已 ship 的 errcodes / mount / doctor 基线）

### 前置阶段上下文（必读，避免重复决策）

- `.planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md` §D-11 / D-12 — Phase 31 已实现的「候选目录 >50MB 拒绝热同步」整目录级熔断逻辑（本阶段升级为**单文件级**熔断，机制不同，需保留 D-11 的目录级整体过滤）
- `.planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md` §D-19 — Phase 31 已注册的 14 个 MOUNT_* / NET_* 错误码命名空间（本阶段在此基础上 +2 条）
- `.planning/milestones/v3.0-phases/31-cli/31-CONTEXT.md` §D-32 — Phase 31 给 Phase 34 留的 errcodes / last-session.json / MutagenHealthCheck 接口（本阶段沿用同 schema）
- `.planning/milestones/v3.0-phases/34-cloud-claude-doctor-v3/34-CONTEXT.md` §D-01 / D-02 / D-13 — Phase 34 doctor 包结构 / errcodes/explanations.go 长说明模板 / 第一屏布局；本阶段新增 5 项 check 必须延续同骨架
- `.planning/milestones/v3.0-phases/34-cloud-claude-doctor-v3/34-CONTEXT.md` §D-15 / D-19 — JSON schema_version=1 + 5 维度 check 分布；本阶段保持 schema_version=1
- `.planning/milestones/v3.0-phases/34-cloud-claude-doctor-v3/34-CONTEXT.md` §D-17 / D-18 — `cloud-claude explain` 子命令的 Registry + ExtendedExplanations 双层数据源；本阶段直接复用，不改动 explain.go

### 既有代码（直接改造对象）

- `internal/cloudclaude/config.go` — `Config` struct 增 `HotSyncMaxFileMB`；新增 `EffectiveHotSyncMaxFileMB()` 方法（D-04）
- `internal/cloudclaude/mount_sshfs.go` — `mountSSHFS` sshfs 命令追加 4 个缓存参数（D-10）
- `internal/cloudclaude/hot_sync.go` — `HotSyncConfig` 加 `MaxFileBytes`；`HotSyncEngine` walk 阶段加 size 过滤；`HotSyncStatus` 加 `OversizedFiles`（D-05 / D-06 / D-07）
- `internal/cloudclaude/last_session.go` — `LastSessionSnapshot` 新增 `OversizedFiles []OversizedFile` 字段；新建 `OversizedFile` struct（D-09）
- `internal/cloudclaude/mount_strategy.go` — `MountWorkspace` 注入 `MaxFileBytes` + 写 `last-session.json` 时塞 `OversizedFiles` + 输出 stderr 一次性提示（D-08 / D-09）
- `internal/cloudclaude/doctor/mount.go` — 5 项新 check 同文件追加（D-13）
- `internal/cloudclaude/errcodes/codes.go` — 2 个新 Code 常量（D-16）
- `internal/cloudclaude/errcodes/mount.go` — 2 条 MustRegister 注册（D-17）
- `internal/cloudclaude/errcodes/explanations.go` — 2 条 ExtendedExplanations 注册（D-18）
- `cmd/cloud-claude/main.go` — `startSession` 入口增 `requireGitRepo(cwd)` 调用，失败 `os.Exit(exitConfigError)`（D-01）
- `cmd/cloud-claude/git_check.go`（**新增**） — `requireGitRepo` 实现 + 单元测试 fixture（D-02）
- `cmd/cloud-claude/explain.go` — **不改动**（D-19，依赖 Registry 自动覆盖新 code）

### 既有代码（参考样板，不改动）

- `internal/cloudclaude/doctor/mount.go::checkMergerfsBranches` — D-13 `sshfs_cache_args` 远端 mount 参数 grep 实现的样板
- `internal/cloudclaude/doctor/mount.go::checkFUSEResidual` — D-13 `oversized_files_count` 用 Details map 列具体路径的样板
- `internal/cloudclaude/errcodes/explanations.go::init()` MOUNT_MUTAGEN_* 系列 — D-18 五段模板（触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档）的格式参考
- `internal/cloudclaude/errcodes/codes_test.go` — D-21 注册表闭包测试（无需扩展，自动覆盖新 code）
- `internal/cloudclaude/last_session_test.go` — D-09 序列化测试扩展点
- `internal/cloudclaude/ignore.go::DefaultBinaryIgnorePatterns` + `IgnoreMatcher` — D-06 第一层过滤的语义边界
- `cmd/cloud-claude/explain_test.go::TestExplainUnknownCode_Exit4` — D-21 子进程 explain 测试扩展样板

### 研究基线（v3.0 沿用）

- `.planning/research/PITFALLS.md` C2 / C3 / C4 / C5 / C8 — Phase 31 / 34 防御重点；本阶段不新增 PITFALLS 但需复读 C5（本地 cwd 误同步覆盖远端）确认 git 仓库前置约束 + size 熔断在该场景下的协同语义
- `.planning/research/STACK.md` — sshfs / mergerfs / FUSE 版本与已知行为
- `.planning/milestones/v3.0-phases/35-e2e/35-PATTERNS.md` Pattern G — 运维手册头部 + ≥5 章节骨架（本阶段不交付 runbook，但 Phase 37 v31-cold-promotion.md 必须遵循；这里仅作为下一阶段衔接参考）

### 不在本阶段交付（明确边界）

- 任何新 host-agent endpoint（OOS-A20 永久禁，沿用 v3.0 边界）
- 错误码英文 i18n（沿用 Phase 31 D-22 / Phase 34 D-22 决策，留 v3.4 评估）
- `--fix` 自动修复 5 项新 check（D-15，全部走 NextAction 提示）
- inotify watcher / PromotionEngine / cold-promoter / e2e UAT 脚本 / runbook → 全部 Phase 37（REQ-MOUNT-V31-07..16）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/cloudclaude/errcodes/codes.go` 注册机制（`MustRegister` + `Format` + `Lookup`）— 直接增 2 条；包级 `Registry()` / `ExtendedExplanations` 自动闭包到 `cloud-claude explain` / `doctor` / `cloud-claude doctor --json`
- `internal/cloudclaude/last_session.go::LastSessionSnapshot` schema_version=1 + omitempty 字段追加策略（Phase 32 D-27 已示范）— 本阶段 OversizedFiles 字段沿用同模式，**不破坏 v3.0 已上线脚本**
- `internal/cloudclaude/doctor/mount.go::checkMergerfsBranches` 远端 mount 输出参数 grep 模式 — D-13 `sshfs_cache_args` check 几乎是它的镜像复制（替换 mergerfs → sshfs，wantList 替换为 4 个 cache 参数）
- `internal/cloudclaude/doctor/check.go::newPass / newWarn / newFail / newSkip` helper — 5 项新 check 直接调用，零模板代码
- `internal/cloudclaude/hot_sync.go::syncFileState{Size, ModTime}` — 已有 size 字段；D-06 size 过滤直接读 `info.Size()` 即可，无需新结构
- `internal/cloudclaude/ignore.go::IgnoreMatcher` — D-06 第一层过滤直接复用，确保「ignore 命中的 50MB 视频不被记两遍」
- `cmd/cloud-claude/explain.go` 已实现 Registry + ExtendedExplanations 双层数据源 — D-19 新增 2 条 code 自动覆盖，**零改动 explain.go**
- `cmd/cloud-claude/explain_test.go::TestExplainUnknownCode_Exit4` — D-21 新增 2 个子进程测试的样板（exit 0 + stdout 字符数断言）

### Established Patterns

- **错误码注册**：v3.0 已固化「`Code` 常量声明 → `MustRegister(Entry{...})` 注册 → `registerExplanation(Code, "...")` 长说明 → 单测遍历闭包」四步法（codes_test.go + explanations_test.go 自动覆盖）；本阶段 D-16 / D-17 / D-18 严格沿用
- **doctor check**：`Checker interface{ Run(ctx) Check; Fix(ctx, opts) Check }` + `runWithTimeout` 5s 默认超时 + Details map[string]any 开放字段 + Status enum {Pass, Warn, Fail, Skip}（v3.0 D-01 / D-08）
- **配置默认值**：`Config.Effective<Field>() T { if c.<Field> == zero { return defaultValue }; return c.<Field> }`（`EffectiveProxyCommands` 为样板）；D-04 `EffectiveHotSyncMaxFileMB` 沿用
- **退出码**：`exitConfigError=4` 用于「用户输入/环境错误」（如 ParseMode 失败、配置缺失），D-01 git 仓库检查归此类；与 v2.0 / v3.0 退出码语义一致
- **stderr 输出格式**：`errcodes.Format(Code, args...)` 统一两段 `[<CODE>] <Message>\n  建议: <NextAction>`；D-01 / D-08 / D-13 全部走此 helper
- **文件结构**：errcodes 域文件单 init()（mount.go / net.go / system.go / ...）；doctor 维度文件单 file（mount.go / network.go / ...）；本阶段坚持「同域追加，不新建文件」原则（D-13 / D-17）

### Integration Points

- **D-01 git_check** 与 main.go::startSession 现有 ParseMode + LoadConfig + AuthenticateAndWait 链路串联：在 LoadConfig 之后、auth 之前；与 `--mount-mode` 正交
- **D-04 hot_sync_max_file_mb** 流向：config.yaml → `LoadConfig` → `cfg.EffectiveHotSyncMaxFileMB()` → `mount_strategy.MountWorkspace` 注入 `HotSyncConfig.MaxFileBytes` → `HotSyncEngine` walk 阶段过滤 → `HotSyncStatus.OversizedFiles` 返回 → `LastSessionSnapshot.OversizedFiles` 落盘 → doctor `oversized_files_count` 读取
- **D-10 sshfs cache 参数** 与 mergerfs `cache.attr=30/cache.entry=30/cache.readdir=true` 协同：sshfs 是底层文件 IO 缓存，mergerfs 是上层目录元数据缓存，**两层缓存同时启用，不冲突**（C2 验证基线已锁住 mergerfs 端，本阶段为 sshfs 端补齐）
- **D-13 doctor mount 5 项** 与 v3.0 已有 4 项（`mutagen_version_match` / `mergerfs_branches` / `sshfs_mountpoint` / `fuse_residual` / `apparmor_fusermount3`）并列；总 mount 维度 9 项，全部走 `runWithTimeout` 5s（与 v3.0 一致）
- **D-19 explain 子命令** 自动覆盖：`cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 等价于 `Registry()[MOUNT_REQUIRE_GIT_REPO]` + `ExtendedExplanations[MOUNT_REQUIRE_GIT_REPO]` 渲染，零代码改动
- **不集成的接口**：本阶段**不**触发 host-agent / Entry API / 任何 SSH 子进程的协议变更（与 PROJECT.md「零增量特权 / 零跨进程协议变更」强一致）

</code_context>

<specifics>
## Specific Ideas

- 用户对 v3.1 的核心承诺是「映射语义补齐 + 懒加载」——本阶段是该承诺的**前 50%**（约束 + 缓存命中），Phase 37 是后 50%（懒加载 + e2e 验收）；planner 在 Plan 切分时应把 D-01 / D-13 / D-17 / D-18 视为最高优先级（SC#1 / SC#4 / SC#5 字面对应）
- ROADMAP §Phase 36 SC#3「同会话 cat 同一冷文件 2 次，本机 SFTP server read count = 1」是 plan-checker 关键验收锚点；D-11 fixture SFTP 计数测试**必须真实启动 sshfs**（CI 已装 fuse / sshfs，与 Phase 31 一致），不能用 mock
- ROADMAP §Phase 36 SC#5「`cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 子进程退出 0、长说明 ≥200 字」对应 D-19；测试必须断言**子进程 stdout 字符数 ≥ 200**（用 `utf8.RuneCountInString` 计中文字符），不是 byte 数
- 用户已在 Phase 31 显式选择「单 volume 命名」「Entry API 加字段不另加 endpoint」「不扩展 host-agent」三条路径；本阶段 D-01 / D-04 / D-09 严格延续 — `git_check` 全程本地、`hot_sync_max_file_mb` 走现有 yaml、`OversizedFiles` 走现有 last-session.json 字段追加
- v3.0 PROJECT.md 强调「零增量特权 / 零跨进程协议变更」——本阶段所有改动都是「客户端配置 + 校验 + 参数级」，不动镜像、不动 host-agent、不引新依赖（git 是开发者机器必备，不算新增依赖）
- Phase 31 D-11 已实现「整目录 >50MB 拒绝热同步」逻辑，本阶段是**单文件级**熔断（语义不同，互补不冲突）：整目录熔断保护「巨型 mono-repo 误同步」，单文件熔断保护「正常 repo 中混入超大资源文件」；D-06 必须保留 D-11 的目录级整体过滤，不替换不删除

</specifics>

<deferred>
## Deferred Ideas

### 阶段内确认但不交付的 follow-up

- **Phase 37 配套**：cold-promoter inotify watcher / PromotionEngine 异步 SFTP 拉取 / 5s 防抖 + 1/2/4s 退避 + 3 次熔断 / `CLOUD_CLAUDE_NO_PROMOTION` 开关 / mergerfs hot 优先验证 / `promoter_alive` / `promotion_*` 4 个新 check / `docs/runbooks/v31-cold-promotion.md` 运维手册 / `tests/scripts/uat-v31-promotion.sh` e2e UAT 脚本 — 全部 REQ-MOUNT-V31-07..16，phase 37 集中交付
- **`cloud-claude init` 文案优化**：D-04 新增的 `hot_sync_max_file_mb` 字段在 init 时不出现在 prompt（仅手动编辑 yaml 可配）；交互式引导留 v3.4
- **错误码英文 i18n**：沿用 Phase 31 / 34 决策，本阶段 ExtendedExplanations 中文硬编码；i18n 框架留 v3.4（OOS 未明确禁止但 ROI 低）
- **`--fix` 自动修复**：D-15 明确 5 项新 check 不提供自动修复；如 v3.4 用户反馈强烈，可考虑为 `default_ignore_loaded` 提供「unset 环境变量」修复（极简）
- **doctor `oversized_files_count` 历史聚合**：本阶段只读 last-session.json 即「上次会话」的列表；跨会话累计 / 历史趋势留 v3.4 metrics
- **Windows 客户端支持**：sshfs 在 Windows 缺少成熟客户端栈 + git rev-parse 路径大小写敏感性差异，本阶段不涵盖；与 PROJECT.md OOS 一致
- **`hot_sync_max_file_mb` 上限校验**：D-04 不强校验上限；如用户配 1000 导致 daemon 内存爆炸，由 doctor `disk` 维度间接发现
- **per-file size 与 du 整目录的双重熔断 UX**：Phase 31 D-11 整目录 >50MB 拒绝 vs 本阶段单文件 ≥50MB 跳过，错误信息可能让用户混淆「为何同样 50MB 一个被拒一个被跳」；UX 文案统一留 v3.4 docs runbook 集中说明

### Reviewed Todos (not folded)

无（`gsd-sdk query todo.match-phase 36` 返回 0 条匹配）。

</deferred>

---

*Phase: 36-sshfs*
*Context gathered: 2026-04-23*
*Mode: --auto（所有 gray area 自动选定推荐项）*
