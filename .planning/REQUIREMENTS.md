# Requirements: Cloud CLI Proxy

**Defined:** 2026-04-23
**Milestone:** v3.1 映射语义补齐与懒加载
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

> v3.1 在已 ship 的 v3.0 三层文件系统（hot_sync + sshfs + mergerfs）基础上做映射语义补齐：把"代码热同步 + 二进制全量 sshfs 透传"升级为"代码强约束 + 二进制懒加载（首次读触发晋升 hot）"。需求基于对话中识别出的 5 大映射 gap（git 仓库约束 / 文件大小熔断 / FUSE 缓存 / 冷文件晋升 / 可观测性），每条对应可观察的用户行为或可断言的系统行为。

---

## v3.1 Requirements

按两条主线（A · 映射前置约束与 FUSE 缓存、B · 冷文件读触发晋升）组织。每条 REQ 在 ROADMAP.md 中映射到唯一一个 phase。

### A1 · 非 git 仓库拒绝挂载（F1）

- [ ] **REQ-MOUNT-V31-01**：`cloud-claude` 启动时若当前 cwd 不在 git 仓库内（`git rev-parse --show-toplevel` 失败），必须立即拒绝挂载、不发起任何 SSH 文件操作，并在 stderr 输出错误码 `MOUNT_REQUIRE_GIT_REPO` + 中文 next_action（建议 `cd` 到 git 仓库 / `git init` 当前目录）；退出码恒为 `exitConfigError`，不可走任何降级路径

### A2 · 单文件大小熔断（F2）

- [x] **REQ-MOUNT-V31-02**：单文件大小 ≥ `hot_sync_max_file_mb`（默认 50MB，可在 `~/.cloud-claude/config.yaml` 配置）且未被 ignore 命中时，**不进入热同步**，由 cold sshfs 兜底；首次扫描时 stderr 一次性输出 `[!] 跳过大文件 <rel> (XX MB)，由 cold 兜底`；不阻断 mount、不写入 hot 分支
- [x] **REQ-MOUNT-V31-03**：`last-session.json` 新增 `oversized_files: [{path, size_bytes}]` 数组，记录本次会话被熔断的所有文件；schema_version 不变（保持向后兼容，新字段 omitempty）

### A3 · sshfs FUSE page cache（F3）

- [x] **REQ-MOUNT-V31-04
**：`mountSSHFS` 生成的 sshfs 命令默认追加 4 个缓存参数：`cache=yes,kernel_cache,auto_cache,cache_timeout=300`；同会话内对同一冷文件二次 `cat` 时本机 SFTP server 接收的 read 数 = 1（首次 read 后由 FUSE page cache 接管，无额外 RTT）

### A4 · doctor mount 扩展与错误码注册（F4）

- [ ] **REQ-MOUNT-V31-05**：`cloud-claude doctor mount` 新增 5 项 check：`require_git_repo`（当前 cwd 是否 git 仓库）/ `oversized_files_count`（本次会话熔断文件数，从 last-session.json 读）/ `sshfs_cache_args`（确认 sshfs 命令含 4 个缓存参数）/ `git_proxy_enabled`（`proxy_commands` 是否含 `git`）/ `default_ignore_loaded`（默认二进制黑名单是否生效）；JSON schema_version 不变，新增 5 个 check 节点
- [x] **REQ-MOUNT-V31-06**：错误码注册表新增 `MOUNT_REQUIRE_GIT_REPO`（severity=error）/ `MOUNT_OVERSIZED_FILE_SKIPPED`（severity=warn），各附 ≥200 字 ExtendedExplanation；`cloud-claude explain MOUNT_REQUIRE_GIT_REPO` 与 `cloud-claude explain MOUNT_OVERSIZED_FILE_SKIPPED` 子进程测试通过

### B1 · 冷分支 inotify watcher（F5）

- [ ] **REQ-MOUNT-V31-07**：Full 模式 mount 就绪后，在容器内 cold 分支根目录长驻 `cold-promoter` 进程，使用 inotify（`IN_OPEN` / `IN_ACCESS`）监听冷文件读事件；启动失败必须 stderr 输出 `MOUNT_PROMOTER_FAILED` 但**不阻断**主路径（降级为"无晋升"模式，cold 仍可读）
- [ ] **REQ-MOUNT-V31-08**：watcher 进程在 mount cleanup（LIFO）时被回收：`mountStrategy.cleanup` 触发 → cancel watcher ctx → wait 进程退出 → fusermount → rmdirChain；异常退出场景（panic / SSH 断连）下次 mount 启动前必须能清理上一次残留进程（`pkill -f cold-promoter --pidfile`）

### B2 · 异步 SFTP 晋升 + 防抖熔断（F6）

- [ ] **REQ-MOUNT-V31-09**：watcher 命中文件路径后异步入队（不阻塞 inotify 事件循环），由 `PromotionEngine` 在独立 goroutine 中复用 `connB` SFTP client 把文件从本机拉到 hot 分支；同一文件 5s 内重复入队**只触发 1 次实际拉取**（去重）
- [ ] **REQ-MOUNT-V31-10**：单文件晋升失败按指数退避重试（1/2/4s），第 3 次仍失败时 stderr 输出 `[!] 晋升失败 <path>: <reason>` 并加入熔断列表，本次会话不再尝试该文件；500MB 文件晋升期间 cold 仍可读、不阻塞用户操作

### B3 · mergerfs 自然命中 hot + 关闭开关（F7）

- [ ] **REQ-MOUNT-V31-11**：晋升完成后，下次对该文件的 `cat` 必须直接走 hot 分支（mergerfs `category.create=ff` 行为天然命中 hot）；e2e 验证：首次读一个 PNG → SFTP read count +N，第二次读 → SFTP read count 不变
- [ ] **REQ-MOUNT-V31-12**：`last-session.json` 新增 `promotion_count` / `promotion_bytes` / `promotion_failed_count` 三字段；schema_version 不变（omitempty）
- [ ] **REQ-MOUNT-V31-13**：环境变量 `CLOUD_CLAUDE_NO_PROMOTION=1` 完全关闭晋升机制（watcher 不启动、PromotionEngine 不构造）；用户主动触发的晋升**不被 ignore 二次过滤**（已主动读 = 用户意图明确，即使是 `.gitignore` 命中的 `.png` 也允许晋升）

### B4 · doctor 晋升可观测 + runbook + e2e UAT（F8）

- [ ] **REQ-MOUNT-V31-14**：`cloud-claude doctor mount` 新增 4 项晋升指标 check：`promoter_alive`（pgrep cold-promoter）/ `promotion_queue_depth`（PromotionEngine 内部队列深度）/ `promotion_total`（last-session.json 累计）/ `promotion_failed_total`（last-session.json 累计）；JSON 输出可被 `make ci-gate` grep 锁定
- [ ] **REQ-MOUNT-V31-15**：新增运维手册 `docs/runbooks/v31-cold-promotion.md`，遵循 PATTERNS Pattern G（头部 + ≥5 章节 + 快速诊断命令小节），覆盖：原理图（cold sshfs → inotify → SFTP → hot → mergerfs）、`CLOUD_CLAUDE_NO_PROMOTION` 关闭场景、晋升失败排障、与 mergerfs / hot_sync 协同的边界、5 个相关错误码反查
- [ ] **REQ-MOUNT-V31-16**：新增 e2e UAT 脚本 `tests/scripts/uat-v31-promotion.sh`：构造 fixture（10 个二进制 + 1 个 60MB + git 仓库 / 非 git 目录）→ 全场景断言（拒绝挂载 / 大文件熔断 / FUSE cache 命中 / 冷文件晋升）→ 输出 JSON 报告（schema_version=1）；脚本 `--dry-run` 默认安全，`--confirm-destructive` 触发实际操作；CI 接入 `make ci-gate`

---

## Future Requirements (v3.2+ 评估)

- 跨会话持久缓存（hot 分支退出后保留，第二次 cloud-claude 直接命中已晋升文件）
- 热同步改 inotify/fsevents 替代秒级轮询（降低 CPU、避免 1 秒内同尺寸修改漏检）
- rename / move 检测优化（避免大文件 rename = 删除 + 重新上传）
- LRU 驱逐策略（hot 分支无限增长保护，会话长期运行场景）

## Out of Scope（v3.1 明确不做）

- **bash 命令整体 proxy 到本机**：会割裂容器内 Claude Code 的执行环境一致性（PATH / env / cwd 全部不一致）；现有 `proxy_commands` 默认 `git`、缓存 + 晋升后冷文件读取语义已充分接近用户预期
- **Windows 客户端支持**：路径同名映射（`/Users/x` vs `C:\Users\x`）+ NTFS 大小写敏感性差异 + Windows FUSE 客户端栈缺失，需单独立项；本里程碑维持 macOS / Linux 双平台
- **改造 hot_sync 主路径（轮询 → fsnotify）**：v3.0 实现已稳定，本里程碑不动主路径降低回归面，单独评估
- **rename / move 检测**：与 fsnotify 改造耦合度高，同上推迟
- **客户端 / 容器内 hot 分支跨会话持久化**：涉及 hot staging 路径生命周期重设计与并发清理策略，v3.2 评估

---

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| REQ-MOUNT-V31-01 | 36 | pending |
| REQ-MOUNT-V31-02 | 36 | completed |
| REQ-MOUNT-V31-03 | 36 | completed |
| REQ-MOUNT-V31-04 | 36 | pending |
| REQ-MOUNT-V31-05 | 36 | pending |
| REQ-MOUNT-V31-06 | 36 | completed |
| REQ-MOUNT-V31-07 | 37 | pending |
| REQ-MOUNT-V31-08 | 37 | pending |
| REQ-MOUNT-V31-09 | 37 | pending |
| REQ-MOUNT-V31-10 | 37 | pending |
| REQ-MOUNT-V31-11 | 37 | pending |
| REQ-MOUNT-V31-12 | 37 | pending |
| REQ-MOUNT-V31-13 | 37 | pending |
| REQ-MOUNT-V31-14 | 37 | pending |
| REQ-MOUNT-V31-15 | 37 | pending |
| REQ-MOUNT-V31-16 | 37 | pending |

> Coverage: 16/16 mapped to phases (100%)；ROADMAP.md 已写入完整映射；status 将随 phase 执行进度更新。

---

*Defined: 2026-04-23 — v3.1 milestone start*
