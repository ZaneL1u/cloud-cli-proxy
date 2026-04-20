---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: 远端开发体验升级
status: executing
stopped_at: Completed 29.1-02-PLAN.md (worker / runtime_service fail-fast + RecordEvent + 单测)
last_updated: "2026-04-20T18:04:28.353Z"
last_activity: 2026-04-20
progress:
  total_phases: 8
  completed_phases: 0
  total_plans: 4
  completed_plans: 2
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 29.1 — 修复 GetHost 缺失 entry_password 字段导致容器密码退化为 workspace

## Current Position

Milestone: v3.0 远端开发体验升级
Phase: 29.1 (修复 GetHost 缺失 entry_password 字段导致容器密码退化为 workspace) — EXECUTING
Plan: 3 of 4
Status: Ready to execute
Last activity: 2026-04-20

Progress: [░░░░░░░░░░░░░░░░░░░░] 0%（v3.0；Phase 32 全部 plan 完成，待 verifier 通过后进 Phase 33）

下一步：`/gsd-verify-phase 32-ssh-tmux` 验证 Phase 32 全部 12 条 SC 闭环；其中 mount_strategy.MountWorkspace 未实际调用 SyncSessionLock 闭包（Phase 31 D-31 落地缺口）+ Plan 02 BufferedStdin 未真接入 Reconnector 共享状态（REQ-F3-B 端到端集成缺失）两项 carry-over 由 verifier 识别后走 gap-closure。完成后进入 Phase 33。

## Accumulated Context

### Decisions

Full decision log in PROJECT.md Key Decisions table.

v3.0 关键方向已定：

- 文件映射改为 Mutagen + sshfs + mergerfs 三层（替代纯 sshfs）
- 容器内默认包一层 tmux/dtach 实现会话可恢复
- 多端连接默认 attach 同一 session，`--new-session` 独占
- doctor 升级为五维度自检
- Claude Code 登录态以 claude_account 为粒度持久化
- 性能基线：rg/ls ≤ 本地 1.5×、首连 ≤ 8s、30s 抖动无感
- [Phase 31-cli]: errcodes 命名正则放宽为 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$（PLAN 原 3 段表达式与实际 4 段 code 冲突；Plan 31-01 Rule 1 修订）
- [Phase 31-cli]: mutagen_bin/.gitattributes 关闭 LFS 行 + 占位 stub 提交，由 CI build-images workflow 拉取真实 v0.18.1 二进制（Plan 31-01 Task 1.2 自决）
- [Phase 31-cli]: Plan 02：三层 mount 状态机落地，Mode={Auto,Full,MutagenOnly,SSHFSOnly}，Auto 任一档失败 ≤2s 降级到下一档；exitcodes.go 9 个常量与 v2.0 0-5 对齐 + OAuth/mount 6-8
- [Phase 31-cli]: Plan 02：M13 防御「禁止静默降级」由 stderr MOUNT_AUTO_DOWNGRADED + last-session.json downgrade_chain 双留痕；ConnectAndRunClaudeV3 留 TODO(plan-03) OAuth hook 点
- [Phase 31-cli]: Plan 03：OAuth 三态检查接入 ConnectAndRunClaudeV3 (mount ready 后、claude 前)，Expired→ExitOAuthExpired(7) / NotFound→ExitOAuthNotFound(6)，ExpiringSoon 仅警告
- [Phase 31-cli]: Plan 03：MutagenSyncStatus{SessionName,ConflictCount,LastError} 引入，mountMutagen 第二返回值 int→struct，sync list --template 解析 conflict count（v0.18.1 不支持 --json）
- [Phase 31-cli]: Plan 03：6 个 TestIntegration_* + docker compose fixture 脚本就位，未引入 testcontainers-go；C3 netem 场景 t.Skip 留 Phase 35 真机
- [Phase 32-ssh-tmux]: Plan 01 net-resilience：keepalive (RunKeepAlive + ConfigureTCPKeepAlive 跨 linux/darwin/other build tag) + reconnect (Reconnector 退避 1/2/4/8/30s + Trigger drop + fastRetry 60s 5 次封顶 + 三态 UX) + input_buffer (BufferedStdin 4KB ringBuf + 灰色 echo + Flush) + 10 条 SESSION_*/NET_* 错误码 + last_session 三新字段 (TmuxSession/ClientRole/ReconnectCount, omitempty schema_version 仍 1) + colors.ansiGray 全部就位；ssh.go::sshConnect 仅 4 行 best-effort TCP keepalive 接入（未碰 ConnectAndRunClaudeV3 / runClaude，留 Plan 02）；windows build 因既有 syscall.SIGWINCH 失败为 out-of-scope 入 deferred
- [Phase 32-ssh-tmux]: Plan 02 tmux-multiclient：DetectTmux + buildTmuxSessionName(per-account_id) + buildTmuxRemoteCmd(D-10 tmux new-session -A) + 文件注册表 (`/workspace/.cloud-claude/clients/<pid>.json` schema_version=1) + performTakeOver(D-11 list-clients/display-message/sleep/detach -a) + printAttachBanner(D-12 完整方案 hostname 走 readClientHostnames) + RunSessionsLs/RunSessionsAttach + runClaudeWithSession 主入口（pTYAttachOnce 单次 + runClaudePTYWithReconnect 外层 + 三 goroutine: RunKeepAlive / Reconnector / BufferedStdin 占位） + ConnectAndRunClaudeV3 OAuth 后插入 DetectTmux 路由 + MountConfig 追加 SessionShortID/SessionTakeOver/LocalHostname + cmd/cloud-claude/sessions.go 新建 + main.go 4 处改造（newSessionsCmd 注册 + DisableFlagParsing switch 追加 + --new-session/--take-over flag 剥离 + KeepAliveInterval < 15s 启动校验）+ SSHConnect export 包装 + 22 条 session_test.go PASS；BufferedStdin 共享 atomic.Int32 状态（RegisterStateListener 接口）推迟 v3.1 — 本 plan 降级为占位独立 atomic（reconnect 期间用户键入暂时丢失，远端 tmux 进程不丢）
- [Phase 32-ssh-tmux]: Plan 02 tmux-multiclient：session.go 全新文件（867 行）含 DetectTmux + 命名 helpers + D-10 远程命令模板 + 文件注册表 (writeClientFile/removeClientFile/readClientHostnames，D-12 完整方案 schema_version=1) + runClaudeWithSession (PTY/RunKeepAlive/Reconnector 三协同) + RunSessionsLs/Attach；ssh.go ConnectAndRunClaudeV3 OAuth 后插入 SessionConfig 构造 + DetectTmux 路由（runClaude 函数体 zero diff 保留作 fallback）；MountConfig 末尾追加 SessionShortID/SessionTakeOver/LocalHostname；cmd 层新增 sessions.go cobra 子命令 + main.go 4 处改造 (AddCommand/DisableFlagParsing/runRoot 剥离 --new-session/--take-over/KeepAlive < 15s 启动校验)；21 个新单测全 PASS；shellescape.Quote 11 处防注入 + TestBuildTmuxRemoteCmd_SpecialCharsQuoted 显式覆盖；BufferedStdin 跨 attach 周期持久化留 v3.1 (Plan 01 已 ship StateAddr 模式，PLAN 提到的 RegisterStateListener 接口未落地，本 plan 走每轮 attach 本地 atomic.Int32 简化方案不影响 30s 抖动恢复)
- [Phase 32-ssh-tmux]: Plan 03 sync-lock-integration：sync_lock.go 新建（122 行）实现 AcquireSyncLock(conn, accountID) 通过远程 `flock -n -E 99 -F /tmp/cloud-claude/locks/sync-<id>.lock -c 'echo $$; exec sleep infinity' & echo $!`（D-17 路径 / D-18 注入位置 / D-19 anon noop）；ErrSyncLocked sentinel + parseLastInt 容错纯函数 + 9 个单测；ssh.go ConnectAndRunClaudeV3 把 SyncSessionLock 默认 noop 替换为真实 AcquireSyncLock 包装（errors.Is(ErrSyncLocked) 时 mountCfg.IsSecondaryClient=true + stderr [SESSION_SYNC_LOCKED]）；MountConfig 末尾追加 IsSecondaryClient bool；session.go SessionConfig 同字段 + writeClientFile 签名追加 role 形参 + runClaudeWithSession + pTYAttachOnce 4 处 role 三元（依 sessionCfg.IsSecondaryClient 决定 primary/secondary）；integration_test.go 末尾追加 6 个 TestIntegration_Phase32_*（PgrepNoSystemdLogind / TmuxSurvivesSighupSshd / SyncLockMutexes / SyncLockAnonNoop / DetectTmuxAvailable / NetworkDisconnect30s 框架短模式跳过）+ defaultFixtureSSHConfig helper；test-fixture-up/down.sh zero diff；go.mod 无新依赖；carry-over：(a) Phase 31 mount_strategy.MountWorkspace 未实际调用 SyncSessionLock 闭包（D-31 落地缺口）— 本 plan 严遵 user "不重写 mount_strategy" 指令，闭包安装到位但调用链留 verifier；(b) Plan 02 BufferedStdin 未真接入 Reconnector 共享状态（REQ-F3-B 端到端集成缺失）保持原样不修
- [Phase 32]: Plan 04 闭合 Gap #2 / SC11：MountWorkspace 真实调用 cfg.SyncSessionLock(cfg.ClaudeAccountID)，ErrSyncLocked 强制 ModeSSHFSOnly + DowngradeChain 追加 sync_locked + IsSecondaryClient=true；其它 lockErr 透传 ModeFailed（M13 防御）；成功拿锁挂入 finalCleanup LIFO 末尾。
- [Phase 32]: Plan 05 闭合 Gap #1 / SC5：Reconnector + BufferedStdin 单例提升到 runClaudePTYWithReconnect 外层；pTYAttachOnce 删除局部 atomic.Int32 并新增 bufferedPipeR io.Reader 参数共享外层 atomic；onReconnected 闭包内 bs.Flush() 按序回放 ringBuf；input_buffer.go 新增 echoMu sync.Mutex co-fix WR-04；bs.Run 单 goroutine co-fix WR-03；新增 TestPTYReconnect_BufferedInputFlush 6 断言覆盖 SC5 端到端；公开 API zero diff（Plan 01/02/03/04 不影响）；race mode 全 PASS
- [Phase 29.1]: Plan 01：仓储层 6 个 Host 读 SQL 一次性补齐 entry_password 列 + 提升为包级 const（getHostSQL/listHostsSQL/listHostsByUserIDSQL/listHostsWithUsernameSQL/listRunningHostsSQL/listRunningHostsByUserIDSQL），新增 TestAllHostReadQueriesIncludeEntryPassword 契约测试锁回归；commits 2af9919 (fix) + 677fe47 (test)
- [Phase 29.1]: Plan 02：runtime_service.QueueHostAction + worker.buildCreateArgs + worker.syncContainerCredentials 三处空 EntryPassword 静默 "workspace" fallback 改造为 fail-fast (return error / RecordEvent runtime.entry_password_missing)；worker.go 中 "workspace" 字面量仅剩 4 处 firstNonEmpty(request.DefaultUser, ...) 用户名 fallback；新增 worker_password_test.go 2 条 PASS 单测；syncContainerCredentials chpasswd 调用改用 execInContainer 包级 var 以可测；commits 62e4455 (2.1) + 317c94f (2.2) + a222a2e (2.3)

### Pending Todos

None — 等待 REQUIREMENTS.md 与 ROADMAP.md 产出后进入 phase 执行。

### Blockers/Concerns

无。前置调研已确认 Mutagen v0.18.1 / mergerfs 2.41.x / sshfs 容器配置全部可行。

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260416-wvu | injectSSHKeys 幂等化，保留用户手加密钥 | 2026-04-16 | cc18acf | [260416-wvu-make-injectsshkeys-idempotent-so-user-ge](./quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/) |
| 260417-0w4 | 新增 cloud-claude ssh doctor 子命令（owner/mode/PEM 尾换行自检与修复） | 2026-04-16 | d716b14, 3f0567c, 7836821 | [260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss](./quick/260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss/) |

### Roadmap Evolution

- Phase 29.1 inserted after Phase 29: 修复 GetHost 缺失 entry_password 字段导致容器密码退化为 workspace（URGENT — 线上 P0）

### Phase 29 关键修订记录

- **2026-04-18 D-23 路径双修正**：(1) AppArmor override 路径由 `docker-default` → `fusermount3`（Launchpad bug #2111105 + moby#50013 + sysbox#947 + stargz-snapshotter#2144 一致证据，防御 C6）；(2) 脚本路径由"新增 `deploy/host-preflight.sh`" → "扩展现有 `deploy/scripts/host-preflight.sh`"（PATTERNS AP9 发现）。两处修正均在 29-CONTEXT.md 与 29-DISCUSSION-LOG.md 留下完整审计链。
- **2026-04-18 plan-checker 修订**：R1 关键（Plan 05 Task 5.1 弃用不存在的 log_* helper，改用现网 `echo >&2` + `return 0/1` + `|| true` advisory）+ R2..R6 五项小修订全部完成；round 2 复核 APPROVED。

## Session Continuity

Last session: 2026-04-20T18:04:19.815Z
Stopped at: Completed 29.1-02-PLAN.md (worker / runtime_service fail-fast + RecordEvent + 单测)
Resume file: None
