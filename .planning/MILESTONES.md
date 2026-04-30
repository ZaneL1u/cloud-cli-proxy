# Milestones

## v3.1 映射语义补齐与懒加载 (Shipped: 2026-04-30)

**Phases completed:** 5 phases, 18 plans, 32 tasks

**Key accomplishments:**

- 0007 迁移引入 users.role 列和 claude_accounts 表，扩展 Go 模型支持角色和 Claude 账号，定义统一 AuthClaims JWT claims 结构体和 GenerateAuthToken 工具函数
- 统一登录端点 /v1/auth/login 接受 short_id + password bcrypt 认证返回带 role 的 JWT，通用 AuthMiddleware 替代旧 AdminAuthMiddleware，entry.go 废弃明文密码，启动时自动创建种子管理员
- 统一登录页改用 short_id 发送到 /v1/auth/login，按 JWT role 字段跳转 dashboard 或 portal，管理员路由守卫增加角色校验
- 用户自助 API 三端点（主机列表/详情/重建）+ 归属校验 + 敏感字段过滤 + JWT 认证中间件
- Portal 主机列表卡片页 + 主机详情页（含出口 IP/隧道类型展示和重建确认对话框）+ Topbar 路由标题和角色标签适配
- claude-shell 专用 Docker 镜像（Ubuntu 24.04 + sing-box 1.13.3 + Claude Code）与 step-function entrypoint 编排脚本
- Dockerfile Claude Code 安装步骤增加 3 次重试 + GitHub release binary 回退，解决 CDN 返回 HTTP 403 导致 docker build 失败的问题
- 两条 Phase 36 mount 错误码、对应中文长说明和 explain 子进程回归测试已接入统一 errcodes 注册表。
- Config 新增 HotSyncMaxFileMB(默认 50) accessor，LastSessionSnapshot 新增 OversizedFiles 数组与 OversizedFile struct，schema_version=1 不变，3 条序列化测试 PASS。
- HotSyncEngine 在 initialSync / syncOnce 注入单文件大小熔断，超阈文件经 HotSyncStatus 透传到 mount_strategy，写入 last-session.json 并通过 stderr 一次性提示，三场景测试 PASS。
- runRoot 工作目录获取与 git 仓库前置检测前移到 AuthenticateAndWait 之前，非 git 目录立即 exit 4 且不发起任何 SSH 连接（修复 RESEARCH §L1 时序地雷）。
- mount_sshfs.go 注入 4 个 FUSE page cache 参数（cache=yes,kernel_cache,auto_cache,cache_timeout=300），并新增 fixture SFTP counter 测试端到端验证「同会话同文件 ReadFile 2 次 → server-side Fileread = 1」。
- doctor mount 维度从 v3.0 的 4 项 check 扩展到 9 项（+5 项 require_git_repo / oversized_files_count / sshfs_cache_args / git_proxy_enabled / default_ignore_loaded），覆盖 git 仓库前置约束、上次会话大文件熔断记录、sshfs 内核缓存参数完整性、代理配置与默认 ignore 加载状态；13 条矩阵测试全 PASS，schema_version=1 不变，CI 三段 grep gate 继续通过。
- 1. [Rule 3 - 缺少依赖] MOUNT_PROMOTER_FAILED 错误码不存在
- ColdPromoter 挂入 tryModeReal Full 路径：mergerfs 就绪后启动，cleanup LIFO 回收，LastSessionSnapshot 新增 3 个 promotion 字段，CLOUD_CLAUDE_NO_PROMOTION=1 完全跳过
- MOUNT_PROMOTER_FAILED 错误码全链路（37-01 ship）+ 4 项 promotion doctor check + explain 子进程测试

---

## v3.1 映射语义补齐与懒加载 (Shipped: 2026-04-24)

**Phases completed:** 2 phases, 11 plans, ~35 tasks
**Git range:** 088b95f (2026-03-29) → 2511a33 (2026-04-24) — 14+ feat commits, 46 files (+6,984/-99 lines)
**Codebase LOC:** Go 32,103 / TS+TSX 11,772 / Shell 5,078 = 48,953 total

**Key accomplishments:**

- git 仓库强约束挂载：非 git 目录立即拒绝挂载，stderr 输出 `MOUNT_REQUIRE_GIT_REPO` + 中文 next_action，退出码恒定 `exitConfigError`
- 单文件 50MB 熔断（可配置 `hot_sync_max_file_mb`）：`HotSyncEngine` 超阈文件不进热同步，走 cold sshfs 兜底；`last-session.json` 记录熔断清单，stderr 一次性提示
- sshfs FUSE page cache 命中：`cache=yes,kernel_cache,auto_cache,cache_timeout=300` 默认开启，同会话重复读零额外 RTT
- doctor mount 9 项 check：从 v3.0 的 4 项扩展到 9 项（+git 仓库 / 大文件熔断 / sshfs 缓存 / git proxy / ignore 加载状态），13 条矩阵测试全 PASS
- `cloud-claude explain` 新增 2 条错误码长说明：`MOUNT_REQUIRE_GIT_REPO` / `MOUNT_OVERSIZED_FILE_SKIPPED`，rustc 风格 ≥200 字中文说明
- ColdPromoter 冷文件晋升引擎：容器内 inotify `IN_OPEN/IN_ACCESS` 监听 + 异步 SFTP 拉取到 hot 分支，5s 防抖去重 + 1/2/4s 指数退避 + 3 次熔断
- 晋升机制完整集成：`tryModeReal` Full 路径 mergerfs ready 后启动，cleanup LIFO 回收；`CLOUD_CLAUDE_NO_PROMOTION=1` 全量关闭
- 晋升可观测性：`last-session.json` 新增 `promotion_count/bytes/failed_count`；doctor 新增 4 项晋升指标（promoter_alive / queue_depth / total / failed）
- 运维手册 + e2e UAT：`docs/runbooks/v31-cold-promotion.md` Pattern G 5 章手册；619 行 `uat-v31-promotion.sh` 覆盖 6 大场景，接入 `make ci-gate`

**Coverage:**

- Requirements: **16/16 satisfied** — Phase 36 全部 6 条 + Phase 37 全部 10 条
- Cross-phase integration: Phase 36→37 4 条依赖链路全 VERIFIED
- CI: `make ci-gate` PASS（go test + ci-doctor-grep + uat dry-run）

**Known deferred items at close:** 5 项 Phase 37 人工验证（Linux 真机 UAT / pgrep 存活 / 端到端晋升 / 手册可读性 / 双平台签字），跟踪在 `v3.1-MILESTONE-AUDIT.md`

**Audit:** `.planning/milestones/v3.1-MILESTONE-AUDIT.md` (status: tech_debt — 0 实现 gap)
**Tag:** v3.1
**Archive:**

- `.planning/milestones/v3.1-ROADMAP.md`
- `.planning/milestones/v3.1-REQUIREMENTS.md`
- `.planning/milestones/v3.1-MILESTONE-AUDIT.md`

---

## v3.0 远端开发体验升级 (Shipped: 2026-04-23)

**Phases completed:** 8 phases (含 1 P0 hotfix decimal Phase 29.1), 30 plans, ~75 tasks
**Git range:** 2f6c041 (2026-04-18) → 3a86bad (2026-04-23) — 208 commits, 255 files (+72,879/-335 lines, 含 Mutagen 4 平台 ~49MB go:embed 二进制)
**Codebase LOC:** Go 29,535 / TS+TSX 11,772 / Shell 4,459 = 45,766 total

**Key accomplishments:**

- 三层文件系统架构：Mutagen 热同步白名单（≤50MB + ignore）+ sshfs 冷兜底全量懒拉 + mergerfs 单一 `/workspace` 视图，替换 v2.0 纯 sshfs 性能天花板
- `--mount-mode=auto|full|mutagen-only|sshfs-only` 四档降级状态机：任一层失败 ≤2s 降级 + 禁止静默降级 + last-session.json downgrade_chain 留痕 + banner 彩色 mount 模式标签（NO_COLOR 尊重）
- SSH 弱网容忍：KeepAlive 15s/4 强制下限 + Reconnector 退避 1/2/4/8/30s + token 复用不弹密码 + BufferedStdin 灰色未确认本地 echo + ringBuf 按序回放
- tmux 默认包装 + 多端共享 attach：`exec tmux new-session -A -s claude-<account_id>` + `cloud-claude sessions ls/attach` + `--new-session`/`--take-over` + 第二端 banner 显示其它会话来源 + 活跃时间
- 账号级 Mutagen 单例锁：远程 flock + ErrSyncLocked 降级 ModeSSHFSOnly + IsSecondaryClient=true + last-session.json client_role=secondary
- Claude Code OAuth 持久化：单 Docker named volume `claude-state-{claude_account_id}` + label `com.cloud-cli-proxy.account_id` + entrypoint symlink + chown 1000:1000 兜底；admin DELETE claude_account 事务性联动 `volume rm`（强一致 10s + force 30s 双路径，错误码 STATE_VOLUME_IN_USE_001 + 6 类 audit 事件）
- `cloud-claude doctor` 5 维度 18 项 check（network/auth/ssh/mount=mutagen+sshfs+mergerfs/disk）+ 6 类自动 fix（mutagen agent / FUSE 残留 / known_hosts / token / OAuth refresh / DNS）+ JSON schema_v1 + 退出码 0/1/2 brew 对齐 + 第一屏降级历史 banner + scripts/ci-doctor-grep.sh M14 闸门
- 错误码统一：42 条 Code 8 域闭合（MOUNT/SESSION/NET/STATE/SYSTEM/SSH/AUTH/DISK + 既有），格式 `<DOMAIN>_<KIND>_<NUM>`，三要素强制（Code + 中文 message + 中文 next_action），CI 单测遍历 + `cloud-claude explain <code>` 子命令对每个码给 ≥200 字符长说明（rustc 风格）
- 镜像 v3 基线：mergerfs 2.41.1（GitHub static `.deb` 反 PITFALLS M3）+ mutagen-agent v0.18.1 tarball 预放 + tmux 3.6a 核对 + libfuse3 3.18.x；entrypoint 串行 prepare-fuse → chown → mutagen-agent → mergerfs → wait → exec sshd（防 PITFALLS M4）；image.lock v3.0.0 + CI 镜像 ≤ 700MB gate
- Worker 容器参数扩展：`HostActionRequest.Volumes []VolumeMount` + `ClaudeAccountID` 字段；`createHost` 在 ClaudeAccountID 非空时自动 `ensureDockerVolume` 幂等创建 + 追加 mount + Upsert 写库
- 控制面数据模型：migration 0014 `claude_accounts.persistent_volume_name`；Entry API `/v1/entry/{id}/auth` 追加 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id`（向后兼容，旧 v2 client 不破）
- v3 受管镜像 + 部署文档：Ubuntu 25.04 AppArmor `local override`（防御 PITFALLS C6）+ `host-preflight.sh` 检测脚本 + 5 章 docs/runbooks/v3-* 升级手册（升级 / AppArmor / doctor 排障 / 持久卷 / 错误码索引）
- E2E 验收脚本：scripts/perf-benchmark.sh + cold-start-benchmark.sh + uat-network-resilience.sh + degradation-regression.sh + v3-acceptance-checklist.sh 聚合；CI ci.yml 加 perf-benchmark + image-size-regression jobs
- v2.0 GetHost entry_password 全链路密码退化 P0 hotfix（Phase 29.1 INSERTED）：仓储 6 个 Host 读 SQL 全补 entry_password + runtime/worker fail-fast + entrypoint passwd -S 自检 + admin batch resync 端点

**Coverage:**

- Requirements: **33/34 satisfied** — 30 functional + 3 baselines（BASE-01/03/04）satisfied；BASE-02 自动化 PASS / 真机签字 deferred-to-ship
- Cross-phase integration: 4 条核心 E2E flow 全闭环（cloud-claude 启动 / 网络抖动重连 / admin DELETE volume rm / doctor + ApplyFixes）；零 orphan export，零 broken wiring
- Critical Pitfalls 防御：C1/C2/C3/C4/C5/C6/C7/C8 + M13/M14 全部覆盖（验证手段见 v3.0-MILESTONE-AUDIT.md）

**Known deferred items at close:** 23 (see STATE.md `## Deferred Items`)

- 3 真机签字（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）— ship 前补签
- 2 v1.2 历史 verification gap（Phase 11/12，与 v3.0 无关）
- 5 docker UAT（Phase 32 → Phase 35 真机 UAT 队列）
- 9 quick_tasks（与 v3.0 milestone goal 无直接绑定）
- 14 项 tech debt（WR/HR/MR 系列 + spec/code 数字漂移）

**Audit:** `.planning/milestones/v3.0-MILESTONE-AUDIT.md` (status: tech_debt — 0 实现 gap，4 E2E flow WIRED)
**Tag:** v3.0
**Archive:**

- `.planning/milestones/v3.0-ROADMAP.md`
- `.planning/milestones/v3.0-REQUIREMENTS.md`
- `.planning/milestones/v3.0-MILESTONE-AUDIT.md`
- `.planning/milestones/v3.0-phases/` (8 phase directories)

---

## v2.0 cloud-claude 透明远程 CLI (Shipped: 2026-04-15)

**Phases completed:** 5 phases, 7 plans, 16 tasks

**Key accomplishments:**

- 受管镜像预装 sshfs + fuse3 并配置 FUSE 权限，Worker 附加 --device /dev/fuse 和 --cap-add SYS_ADMIN，SSH Proxy 确认零改造支持多 session channel
- cobra 入口 + init 配置持久化 + Entry API 认证轮询 + SSH PTY 远程 claude 会话的完整 CLI 闭环
- shellescape 安全命令构建 + cobra 透传用户 claude 参数 + 非 TTY 管道模式 + 退出码返回值上浮修复 HI-01
- pkg/sftp 嵌入式 SFTP server + sshfs passive 模式启动 + mountpoint 轮询检测 + fusermount 防御性清理
- 重构 ConnectAndRunClaude 为 sshConnect→mountWorkspace→runClaude 三阶段架构，main.go 传递 os.Getwd() 实现端到端目录映射
- worker.go 添加 apparmor=unconfined 解除 FUSE 阻断，238 行验证脚本覆盖 sshfs 真实挂载 + 网络策略共存 + E2E 流程
- host-preflight.sh 添加 FUSE 内核模块双重检测，中英文部署文档补充 FUSE/AppArmor 兼容性章节和已知限制表

---

## v1.2 用户自助面板与 Bootstrap 重设计 (Partial: 2026-03-29, remaining deferred)

**Phases completed:** 2 of 6 phases (Phase 11-12), remaining (Phase 13-16) deferred to future milestone
**Plans completed:** 5 plans

**Key accomplishments:**

- 用户登录认证体系（区别于管理员 JWT），统一登录页按角色自动跳转，用户 API 资源隔离（403）
- claude_accounts 数据模型（一个用户多个 Claude 账号，每个账号关联一台主机）
- 用户自助面板骨架：TanStack Router 角色路由守卫，用户面板与管理员面板共存于同一 React 应用
- 用户自助 API（UserHostsHandler + 主机列表/详情/重建 + 出口 IP 查看）
- auth_middleware.go（AuthMiddleware / RequireRole / UserIDFromContext / RoleFromContext）

**Deferred to future:**

- Phase 13: 账号管理与用户资源视图（账号 CRUD、有效期、售后换号）
- Phase 14: KasmVNC 用户面（浏览器远程桌面）
- Phase 15: Bootstrap 重设计（短 URL 入口与实时状态推送）
- Phase 16: 级联禁用与到期治理（用户/账号/主机到期联动）

---

## v1.1 支持代理协议出网 (Shipped: 2026-03-28)

**Phases completed:** 4 phases, 11 plans, 21 tasks

**Key accomplishments:**

- egress_ips 表新增 tunnel_type（wireguard/proxy CHECK 约束）和 proxy_config JSONB 列，Go 模型和全部 6 个 SQL 查询同步扩展
- EgressConfig 扩展为 wireguard/proxy 双模式，ValidateEgressBinding 按 TunnelType 分支校验，新增 ProxySpec 和 3 个 proxy 测试用例
- Admin API 完整支持 tunnel_type/proxy_config 字段的创建、更新、白名单校验和响应脱敏，repoValidator 正确映射新字段
- sing-box 配置结构体 + JSON 生成函数（tun inbound / proxy outbound / DNS hijack）及受管镜像 v1.13.3 二进制预装
- proxy 模式 nftables 防火墙规则（tun0/proxy server 白名单）和宿主机 IP 转发 + masquerade
- SingBoxProvider 15 步 PrepareHost 流水线（tun 模式全流量代理）和 RoutingProvider 工厂按 TunnelType 自动路由到 WireGuard/sing-box
- 代理测试 API 支持 SOCKS5/HTTP/vmess/ss/trojan 五种协议，返回连通性、出口 IP 匹配、DNS 泄漏三项检测结果，前端 TestResult 类型和 mutation hook 就绪
- 动态隧道类型表单切换 + 5 种代理协议字段渲染 + 表单/JSON 双向编辑 + 后端密码合并逻辑
- 出口 IP 列表页增加隧道类型 / 测试状态两列并集成 TestResultDialog 展示连通性、出口 IP 匹配和 DNS 泄漏三项检测详情
- stopHost 追加 CleanupHost 消除 mgmt veth 残留 + vmess/ss/trojan 代理测试添加 sing-box LookPath 预检返回中文提示
- localStorage 持久化代理测试结果跨刷新恢复 + WireGuard 类型出口 IP 测试按钮禁用并显示 toast 提示

---

## v1.0 MVP (Shipped: 2026-03-28)

**Phases completed:** 6 phases, 19 plans, 42 tasks

**Key accomplishments:**

- 基于 Go 标准库的 control-plane 启动骨架、PostgreSQL 核心 schema 与单宿主机开发编排
- 固定镜像锁、SSH 工作环境和 `claude code` 预装的受管用户模板容器
- 基于 Unix socket 的 host-agent、真实 Docker 生命周期 worker 与 systemd 特权边界
- WireGuard 隧道类型建模、6 类网络错误体系、启动前绑定校验门禁和 --network=none 容器隔离
- WireGuard birthplace-namespace 隧道注入、nftables 默认拒绝防火墙、管理 veth 和隧道 DNS 配置，TunnelProvider 替换 NoopProvider
- Triple network verification (egress IP match, DNS path, leak blocking) integrated as PrepareHost pipeline gate with typed event recording and extended host preflight checks
- bcrypt 密码认证 + 异步 start_host 任务入队 + 受管 bootstrap 脚本（密码不回显 + 稳定退出码）
- SSH readiness gate 阻止假就绪接入 + GET /v1/bootstrap/tasks/{taskID} 阶段化进度轮询（D-06 固定映射）
- host-agent ssh.handoff.ready 元数据 + GET handoff API + 稳定 error_code/exit_code 映射 + bootstrap 脚本 poll→handoff→exec ssh 完整闭环
- Go 端 JWT 登录 API + 认证中间件 + 仪表板统计 API，React 19 SPA 脚手架含登录页、5 项侧边栏导航和 3 卡片仪表板概览
- 用户 CRUD API（Go bcrypt + crypto/rand 密码轮换）+ React 前端用户管理全页面（列表/详情/创建/删除确认/密码轮换）
- 出口 IP CRUD + 绑定管理（含运行中主机保护）+ 主机启停重建 + 任务列表的完整前后端实现
- DB migration with expires_at/user_id fields, generic ticker-based scheduler, expiry scanner with auto-stop, and admin expiry API endpoints
- 为所有管理 handler 注入事件记录、新增事件查询 API、实现 host-agent 容器 inspect 端点和 DB/Docker 运行时对账定时器
- 用户列表/详情页展示和管理到期时间，事件日志页面支持筛选分页和 metadata 展开，仪表板集成最近事件摘要卡片
- ExpiryScanner/Reconciler 11 个 mock 单元测试 + bootstrap 脚本 7 个 BATS 错误码契约测试，修复脚本 set -eo pipefail 下两个退出码 bug
- 部署指南、运维手册、故障排查手册和自动化部署/备份脚本，覆盖从零部署到日常运维的完整文档体系
- 结构化日志 + healthz 分组检查 + pgxpool 显式配置 + bootstrap 错误码解析 + EgressIP 敏感字段清除 + 前端表单格式校验

---
