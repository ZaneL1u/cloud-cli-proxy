# Phase 36: 映射前置约束 + sshfs 内核缓存 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-23
**Phase:** 36-sshfs（映射前置约束 + sshfs 内核缓存）
**Mode:** `--auto` — 所有 gray area 自动选定推荐项
**Areas discussed:** F1 Git 仓库前置约束 / F2 单文件大小熔断 / F3 sshfs FUSE page cache / F4 doctor mount 5 项 check / F5 错误码 + explain 长说明 / F6 测试与 CI

---

## F1 · Git 仓库前置约束 落点

| Option | Description | Selected |
|--------|-------------|----------|
| `cmd/cloud-claude/main.go::startSession` 入口闸门（cwd 解析后、auth 之前） | 与 ParseMode 同位置；不发起任何 SSH/SFTP；与 `--mount-mode` 正交 | ✓ |
| `mount_strategy.MountWorkspace` 内部检查 | 太晚——已建立 SSH 连接，违反 REQ-01「不发起任何 SSH 文件操作」字面要求 | |
| cobra `PreRunE` hook | 与 ParseMode 等其它入口校验风格不一致；测试 mock 难度高 | |

**[auto] 选择：** main.go 入口闸门（D-01 / D-02）
**理由：** REQ-MOUNT-V31-01 明确「不发起任何 SSH 文件操作」+「不可走任何降级路径」，必须在 SSH 连接前 fail-fast。

---

## F2 · 单文件大小熔断 实现

### Q2-1：阈值配置入口

| Option | Description | Selected |
|--------|-------------|----------|
| `~/.cloud-claude/config.yaml` 新增 `hot_sync_max_file_mb` 字段 | 与 `proxy_commands` 一致的 Effective<Field>() 默认值兜底模式 | ✓ |
| 命令行 flag `--hot-sync-max-mb` | 增加 cobra flag 表面积，与 ROADMAP「配置/校验/参数级改动」叙述偏离 | |
| 环境变量 `CLOUD_CLAUDE_HOT_SYNC_MAX_MB` | 不便于持久化，多端不一致 | |

**[auto] 选择：** config.yaml 字段（D-04）

### Q2-2：ignore vs size 检查顺序

| Option | Description | Selected |
|--------|-------------|----------|
| ignore 优先（命中即跳过，不计 oversized） | 避免「50MB 视频被默认黑名单忽略 + 又被 oversized 记一次」双重提示 | ✓ |
| size 优先（命中即记 oversized） | 用户能看到所有大文件清单；但 ignore 命中本身已意味着「明确不想同步」 | |
| 并行（同时记两次） | UX 噩梦，错误信息冲突 | |

**[auto] 选择：** ignore 优先（D-06）

### Q2-3：stderr 输出形式

| Option | Description | Selected |
|--------|-------------|----------|
| 一次性输出（前 5 条 + 总数 + 引用 last-session.json） | 不刷屏；信息完整可在 last-session.json 查全 | ✓ |
| 每个文件一行 | N=20 时 20 行刷屏，影响首屏 banner 体验 | |
| 全部静默（仅写 last-session.json） | 用户首次发现「文件没同步」需要主动 doctor，UX 差 | |

**[auto] 选择：** 一次性提示（D-08）

### Q2-4：last-session.json 字段命名

| Option | Description | Selected |
|--------|-------------|----------|
| `oversized_files: [{path, size_bytes}]` | 与 REQ-03 字面 schema 完全一致 | ✓ |
| `skipped_files: [...]` | 字段名更通用但与 REQ 文字偏离 | |
| `large_files: [...]` | 「大」是相对的，less precise | |

**[auto] 选择：** `oversized_files`（D-09）

---

## F3 · sshfs FUSE page cache 参数

### Q3-1：参数集选型

| Option | Description | Selected |
|--------|-------------|----------|
| `cache=yes,kernel_cache,auto_cache,cache_timeout=300`（REQ-04 字面 4 参数） | 与 REQ 字面一致；kernel_cache 直接利用 page cache；auto_cache 防御外部修改 | ✓ |
| 仅 `kernel_cache,cache_timeout=300` | 缺 auto_cache → 容器内 claude 修改 cold 文件后客户端读到 stale | |
| `cache=no` + 自研 LRU | 复杂、引依赖、无收益 | |

**[auto] 选择：** REQ-04 字面 4 参数（D-10）

### Q3-2：参数暴露方式

| Option | Description | Selected |
|--------|-------------|----------|
| 字面量硬编码（mount_sshfs.go） | 便于 grep / doctor `sshfs_cache_args` check 字符串匹配；用户无需调 | ✓ |
| 配置文件可调 | 用户场景一致，配置面增加无收益；硬编码可被单元测试锁死 | |

**[auto] 选择：** 硬编码（D-10）

### Q3-3：单测策略

| Option | Description | Selected |
|--------|-------------|----------|
| fixture SFTP server + per-path read 计数器 + 真实 sshfs 挂载 | 直接验证 SC#3「server-side read = 1」；与 v3.0 mount_test.go 真实 sshfs 测试同模式 | ✓ |
| Mock SSH conn + assert sshfs 命令字符串包含 cache 参数 | 不能验证「实际命中 page cache」语义，只验证「参数被传」 | |
| 仅 e2e（依赖 Phase 37 UAT 脚本） | 推迟到 Phase 37 不符合 SC#3「本阶段 PASS」要求 | |

**[auto] 选择：** 真实 sshfs + counting fixture（D-11）

---

## F4 · doctor mount 5 项 check

### Q4-1：5 项 check 文件结构

| Option | Description | Selected |
|--------|-------------|----------|
| 全部加到现有 `doctor/mount.go` 同文件 | 延续 v3.0 D-01「单文件多 Checker」模式；零目录变化 | ✓ |
| 拆为 `mount_v31.go` 子文件 | 文件膨胀；与 v3.0 mount.go 5 项 check 同居一起更易复读 | |
| 5 项各拆 5 个文件 | 单职责过度拆分，违反 v3.0 既有 doctor/ 文件粒度 | |

**[auto] 选择：** 同文件追加（D-13）

### Q4-2：错误码新建 vs 复用

| Option | Description | Selected |
|--------|-------------|----------|
| 仅严格必要的 2 条新 code（REQ-06 字面要求）+ `git_proxy_enabled` / `default_ignore_loaded` 复用 `AUTH_CONFIG_MISSING` | 错误码注册表不膨胀；REQ-06 字面达标 | ✓ |
| 5 项 check 各自配套独立 code（=5 条新 code） | 错误码爆炸；REQ 未要求 | |
| 仅 `MOUNT_REQUIRE_GIT_REPO` 一条（合并 `MOUNT_OVERSIZED_FILE_SKIPPED` 进 warn 文案） | 违反 REQ-06「2 个错误码」字面 | |

**[auto] 选择：** 2 条新 code + 复用现有（D-13 / D-16）

### Q4-3：`--fix` 自动修复策略

| Option | Description | Selected |
|--------|-------------|----------|
| 5 项 check 全部不提供 `--fix`，走 NextAction 提示 | 用户意图不可猜（git init / .gitignore / 配置编辑都需用户决策） | ✓ |
| `default_ignore_loaded` 提供 unset 环境变量修复 | 涉及 shell 配置修改，副作用大 | |
| `git_proxy_enabled` 自动 `cloud-claude init` 重新生成 | 会覆盖用户已有配置 | |

**[auto] 选择：** 全部走 NextAction（D-15）

---

## F5 · 错误码 + explain 长说明

### Q5-1：长说明模板

| Option | Description | Selected |
|--------|-------------|----------|
| 沿用 Phase 34 D-18 五段模板（触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档） | 与 v3.0 38 条长说明一致；CI ≥200 字符断言已固化 | ✓ |
| 自由文本 ≥200 字 | 失去模板一致性，运维查阅时风格不一 | |
| 引用外部 markdown 文件 | 引入新文件类型管理成本，与 v3.0 D-18 否定理由一致 | |

**[auto] 选择：** 五段模板（D-18）

### Q5-2：explain.go 是否需要改动

| Option | Description | Selected |
|--------|-------------|----------|
| 不改动（依赖 Registry + ExtendedExplanations 自动覆盖） | v3.0 已实现的双层数据源天然支持新 code | ✓ |
| 增加 hardcode case 列表 | 反模式，违反 Registry 设计意图 | |

**[auto] 选择：** 不改动（D-19）

---

## F6 · 测试与 CI

### Q6-1：CI 闸门是否需扩展

| Option | Description | Selected |
|--------|-------------|----------|
| 不扩展（v3.0 已固化 ci-gate 闭包测试自动覆盖新 code / 新 check） | errcodes 注册表 + ExtendedExplanations + doctor JSON schema 全部自动覆盖 | ✓ |
| 新增 phase 36 专属 ci job | 增加 CI 复杂度无收益 | |
| 修改 ci-gate 脚本 grep 字符串 | 违反「v3.0 ci-gate 已闭合」假设 | |

**[auto] 选择：** 不扩展（D-21）

### Q6-2：60MB fixture 性能验收 vs 单元测试

| Option | Description | Selected |
|--------|-------------|----------|
| 单元测试覆盖代码内部行为（HotSyncEngine size 过滤 / OversizedFiles 序列化） + e2e 留 Phase 37 UAT 脚本 | SC#2 字面要求由本阶段单测达成；真机 60MB fixture 与 cold 视图验证更适合 Phase 37 集成 | ✓ |
| 本阶段交付完整 60MB fixture e2e | 与 Phase 37 e2e 脚本职责重叠；本阶段 scope 过载 | |

**[auto] 选择：** 单测 + Phase 37 e2e（D-22）

---

## Claude's Discretion

以下细节由 planner / executor 按实现便利性决定（CONTEXT.md `Claude's Discretion` 完整列表）：

- 单元测试 fixture 文件大小选型（30/60/100MB 都可，与 default 50MB 形成边界即可）
- `OversizedFile.Path` 用绝对路径还是 cwd 相对路径（推荐相对）
- stderr 一次性提示中「前 N 条」的 N 值（推荐 5）
- doctor `oversized_files_count` 在 last-session.json 缺失时是否走 skip（推荐 skip + STATE_LAST_SESSION_MISSING）
- D-02 `requireGitRepo` 错误信息是否记录 cwd（推荐记录）
- ExtendedExplanations「关联文档」段引用路径（推荐 REQUIREMENTS.md REQ-MOUNT-V31-01..06 + Phase 31 31-CONTEXT.md D-11）

## Deferred Ideas

- Phase 37 配套（cold-promoter / PromotionEngine / runbook / e2e UAT 脚本）— 全部 REQ-MOUNT-V31-07..16
- `cloud-claude init` 文案优化（hot_sync_max_file_mb 引导）— v3.4
- 错误码英文 i18n — v3.4
- `--fix` 自动修复 5 项新 check — v3.4 视用户反馈
- doctor `oversized_files_count` 跨会话历史聚合 — v3.4 metrics
- Windows 客户端支持 — 与 PROJECT.md OOS 一致
- `hot_sync_max_file_mb` 上限校验 — 由 doctor disk 维度间接发现
- per-file size 与 du 整目录双重熔断 UX 文案统一 — v3.4 docs runbook
