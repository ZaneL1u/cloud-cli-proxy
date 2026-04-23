---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: 远端开发体验升级
status: milestone_complete
stopped_at: "Phase 35 完成（5/5 plans）；v3.0 milestone 8/8 phases 全部完成；3 项真机签字延后到 ship 前补完，跟踪在 .planning/phases/35-e2e/35-HUMAN-UAT.md"
last_updated: "2026-04-23T07:08:19Z"
last_activity: 2026-04-23
progress:
  total_phases: 8
  completed_phases: 8
  total_plans: 30
  completed_plans: 30
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-21)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 35 — e2e

## Current Position

Milestone: v3.0 远端开发体验升级
Phase: 35
Plan: Not started
Status: Milestone complete
Last activity: 2026-04-23

Progress: [███████████████████░] 87%（7/8 phases complete；Phase 35 未开始，无 CONTEXT.md）

下一步：`/gsd-discuss-phase 35 --auto` 进入 Phase 35 上下文收集（auto-chain 已开启），或 `/gsd-progress` 查看更新后的 ROADMAP。Phase 35 承担 BASE-01/02/03 三条性能基线真机验收，外加 macOS APFS + Ubuntu 25.04 AppArmor 双真机矩阵。

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
- [Phase 29.1]: Plan 02：runtime 层三处 firstNonEmpty(..., "workspace") 密码 fallback 改为 fail-fast — Service.repo/NewService 形参 interface 各加 RecordEvent 一行（concrete *Repository 已实现，调用方零破坏），QueueHostAction 空密码写 runtime.entry_password_missing 事件后 return error 不再 Dispatch；buildCreateArgs 空密码 return error，CONTAINER_SSH_PASSWORD 直接拼真值；syncContainerCredentials 空密码写事件后 return 不再 chpasswd（顺带改用 execInContainer 包级 var 提升可测性，PLAN Task 2.3 note 1 预批）；保留 4 处 firstNonEmpty(request.DefaultUser, "workspace") Linux 用户名 fallback；新增 TestBuildCreateArgs_EmptyEntryPassword_ReturnsError + TestSyncContainerCredentials_EmptyEntryPassword_RecordsEventNoChpasswd；事件 metadata 严守 T-29.1-05-log 仅 host_id/container/action/source 不含明文密码；Rule 3：minimalCreateHostRequest 工厂补 EntryPassword 占位避免无关 volume 类用例误触发新守卫；commits 62e4455 (feat) + 317c94f (fix) + a222a2e (test) + 0a60198 (docs)
- [Phase 29.1]: Plan 03：entrypoint.sh chpasswd 之后追加 passwd -S 自检（status ∉ {P,PS} → exit 1），让密码退化故障从用户登不进去前移到容器启动失败 / restart 循环可观测
- [Phase 29.1]: Plan 03：FATAL 消息只回显 RUN_USER + passwd -S 第 2 列状态字串，绝不写入 CONTAINER_PASSWORD / CONTAINER_SSH_PASSWORD 任一密码值（T-29.1-05-log-entrypoint mitigation）
- [Phase 29.1]: Plan 03：自检失败 entrypoint 不做 retry，重启交给外层 docker restart policy / host-agent（T-29.1-04 accept disposition）；passwd -S 解析用 awk + 命令链 || echo UNSET 兜底，避免解析失败被当 P 通过
- [Phase 29.1]: Wave 1 集成门禁：`go build ./...` PASS；`go test ./internal/store/repository/... ./internal/runtime/...` 全 PASS；`go test ./internal/controlplane/http/...` 在本机 hang，根因为既有 `getDockerStatuses()` (admin_hosts.go:73) 直接 `exec.Command("docker", "ps", ...)` 而本机 docker daemon 不可用 — 已用 `git checkout ec1e841` 复跑确认是 Phase 29.1 之前就存在的测试基础设施依赖问题；不是本 phase 引入。Plan 04 自身的 3 条 `TestResyncPasswords_*` 通过 syncContainerPassword var 化绕开 docker，不会被该问题阻塞；后续应另起 backlog 修复 List handler 的 docker 调用（注入 var 或 context timeout）。
- [Phase 33]: 关键 landings 汇总（Plan 01+02 + post-execution patches 联合）：(1) **entrypoint v3 stage symlink**：`prepare_persistent_state` 通过 `cp -an` seed + `ln -sfn` 把 `/home/claude/.claude` 与 `~/.cache/claude` 重定向到 `/var/lib/claude-persist` named volume，1000:1000 双重 chown（Plan 01 / 7acf3d6）；(2) **worker 自动补 volume**：`createHost` 在 ClaudeAccountID 非空时调 `ensureDockerVolume` 幂等创建 `claude-state-{id}` + 追加 mount + Upsert 写库（Plan 01 / 2d0bc22）；(3) **admin DELETE 双一致性路径**：强一致 (10s timeout) 事务内调 host-agent rm 失败 ROLLBACK + 409 + 中文 next_action；force=true (30s timeout) DB 先 COMMIT，rm 失败仅 audit + 200 + next_action 含 `docker volume rm -f`；错误码 `STATE_VOLUME_IN_USE_001` + 3 类 audit 事件（Plan 02 / 11989dd）；(4) **dispatcher 注入 ClaudeAccountID**（post-fix 27ab2d7）：抽出 `QueueHostActionRepo.ResolveClaudeAccountIDForEntry`，在 `RuntimeService.QueueHostAction` 路径填充 `request.ClaudeAccountID`，闭合 Plan 01 SUMMARY 显式列出的 D-04 dispatcher 缺口；(5) **pullImage 5min timeout**（post-fix 3e2ba6b）：`worker.pullImage` 加 `context.WithTimeout`，根治 ghcr.io pull hang 导致的 "rebuild 卡 pending + container missing + DB running" 三态分裂；(6) **EmbeddedDispatcher RunHostAction 适配**（post-fix c09a4d0）：引入 `HostActionRunner` 接口，`EmbeddedDispatcher` 实现适配器，`cmd/control-plane/app.go` 按 mode wire 正确 runner，闭合 Plan 02 Task 2.4 router.go 改了但 app.go 漏 wire 的 deployment 缺口。SC1-SC6 全部 ✅ APPROVED；UAT D-26 五步 + SC3/D-22 通过；23 条新单测全 PASS（4 仓储 + 8 admin handler + 3 admin host detail + 4 post-fix + 4 Plan 01 carry）；运维手册 docs/runbooks/v3-claude-state-volumes.md ship。Plan 02 commits: e232d40 / 11989dd / f05cdd4 / ba5b533 / db582e8 + post-fix 3e2ba6b / 27ab2d7 / c09a4d0。
- [Phase 33-02]: 控制面侧闭环代码已落地（Tasks 2.1-2.4 + 2.6，5 commits e232d40 / 11989dd / f05cdd4 / ba5b533 / db582e8）。仓储新增 BeginTx + LockClaudeAccountForDelete + DeleteClaudeAccountTx + GetHostWithClaudeAccount + HostWithClaudeAccount 类型 (4 SQL/类型单测)；admin_claude_accounts.go 新建 AdminClaudeAccountsHandler 含强一致 (10s timeout) + force (30s timeout) 双路径，错误码 STATE_VOLUME_IN_USE_001 + 中文消息 + 3 类 audit 事件 (claude_account.deleted / delete_volume_rm_failed / force_volume_rm_failed)，metadata 白名单严守 (account_id / volume_name / error_code / error_message / force)；admin_hosts.go AdminHostStore 接口扩展 GetHostWithClaudeAccount + adminHostDetailResponse 追加 PersistentVolumeName (omitempty，list 不动 — OOS-A19 守恒)；router.go Dependencies 追加 AdminClaudeAccounts/AgentClient + DELETE /v1/admin/claude-accounts/{accountID} 注册走 adminGuard。手写 stubTx 实现 pgx.Tx 最小接口，零新依赖 (go.mod / go.sum diff 空)。8 条 handler 单测 + 3 条 admin host detail 单测 + 4 条仓储单测 = 15 条新单测全 PASS；既有 internal/runtime/tasks/ + internal/store/repository/ 全包测试无回归；go build ./... PASS。运维手册 docs/runbooks/v3-claude-state-volumes.md 新建（命名规范 / 生命周期 / 6 类 audit 事件 / 孤儿审计脚本 / 故障排查 / v3.1 backlog），关键 verbatim token 全部命中。Task 2.5 人工 UAT (D-26 五步 + SC3/D-22) → APPROVED by user "成了" 2026-04-21。
- [Phase 33-01]: 镜像层 entrypoint 追加 `prepare_persistent_state` v3 stage（位于 `prepare_v3_dirs → prepare_mutagen_agent` 之间），通过 `cp -an` 幂等 seed + `ln -sfn` 把 `/home/claude/.claude` 与 `.cache/claude` 重定向到 `/var/lib/claude-persist`；agentapi 增 `ActionVolumeRemove = "volume_remove"` 协议常量（D-13）；worker 新增 `BuildClaudeStateVolumeName / dockerVolumeRunner / ensureDockerVolume / removeDockerVolume / removeVolumes`（包级 var 注入 mock 模式），Execute switch 增 `case ActionVolumeRemove` + `volume_in_use` 错误码映射；`createHost` 在 `ClaudeAccountID != ""` 时自动 ensure volume + 追加 mount + upsert 写库 + 失败写 audit；WorkerRepo 接口扩展 `UpsertClaudeAccountPersistentVolumeName`（Repository 三态语义实现 NULL→写入 / 一致跳过 / 冲突错误，SQL 提升包级 const）。fakeWorkerRepo 同步实现新方法以闭环包测试编译。Audit event metadata 严守白名单 `account_id/volume_name/force/host_id`，`grep Metadata:.*"(email|entry_password|credentials|oauth_token)"` 命中 0。新增 12 条单测全 PASS（2 协议 round-trip + 7 lifecycle + 3 SQL/边界），既有 `internal/runtime/tasks/` + `internal/store/repository/` 全包无回归；`go build ./...` PASS。Carry-over：(a) dispatcher 链路 `ClaudeAccountID:` 字段在生产代码全无注入，目前 createHost 走 D-07 fallback 不激活自动 volume — Plan 02 admin handler 走显式 Volumes 不依赖此字段，但 SC1 端到端激活责任移交后续 phase；(b) `ensureDockerVolume` label 一致性比对推迟 v3.1 backlog。Commits: 7acf3d6 (entrypoint) + 235d969 (agentapi) + 2d0bc22 (worker) + 208df5f (repo)。
- [Phase 34-cloud-claude-doctor-v3]: [Phase 34-01]: 8 域闭合错误码 Registry (42 条) + ExtendedExplanations 38 条 ≥200 中文字符长说明 + cloud-claude explain <code> CLI 子命令 (rustc-style); ExplainExempt 4 条 Info 豁免 (Rule 1: MOUNT_AUTO_DOWNGRADED Severity=Warn 移到 ExtendedExplanations 避免与 TestExplainExemptOnlyInformational 矛盾); NET_EGRESS_IP_DRIFT 登记到 auth.go init (避免新建独立 network.go); STATE_VOLUME_IN_USE_001 字面量与 Phase 33 admin handler 守恒 (D-27); 9 errcodes test + 3 explain 子进程 test 全 PASS; commits 01d9f12 / b2fcd24 / b421445 / 2a03abe / 6ae00c6 / 6dc4037 / 75ae2d7 / ccd9317
- [Phase 34-cloud-claude-doctor-v3]: [Phase 34-02]: cloud-claude doctor 五维度自检框架 (18 项 check / 51 单测) — network×3 / auth×3 / ssh×4 / mount×5 / disk×3；M13 (降级 banner 第一屏字面量) + M14 (warn/fail 必带「建议:」+「错误码:」) + SC#5 (JSON schema_version=1 锁死 + 退出码 0/1/2 brew 对齐) 三大锚点全 PASS；Rule 3 deviations: cloudclaude.LoadLastSession 自实现 (plan 引用了不存在的读端 API) + ColorEnabled/Colorize 保留 2-arg/3-arg 签名 (避免重构既有 mount_strategy/session 调用点); Rule 1: authRespExpectedEgressIP 恒返回 '' (entry.go AuthResponse 未导出该字段，v3.1 backlog)；commits 0ddeb10 / c5f6df5 / a945598 / 511753c / aefef3d / 7fb7124 / d77aa8a / 326cdb1 / 3a95373 / 231fe17 / a34e4ce
- [Phase 34-cloud-claude-doctor-v3]: Plan 34-03 doctor-fix-integration: FixerRegistry 6 entry (5 类 + AUTH_OAUTH_REFRESH_FAILED 派生) + ApplyFixes 60s timeout + Status 不降级 (D-16); confirmDestructive 三级判定 (Yes/JSON/非TTY); 5 个 exec* 包级 var mock + isTerminalFD; integration_test build tag + cc-fixture 实际容器名 (非 plan 示例的 cloud-claude-fixture); ci-doctor-grep.sh 三段断言 (schema=1 + next_action + 错误码) + Makefile ci-gate target; JSON 模式守卫 [fix] 顶部行避免 stdout 污染 (SC#5 守恒); commits 78626e5/f4b7893/73e47ab/f38dfd8/8b63af1/dc4519d/5d4a0d8
- [Phase 35-e2e]: [Phase 35-perf-benchmarks]: Plan 01 三脚本就位 — gen-bench-tree.sh (synthetic 10k mono-repo, 80/15/5 + .git/objects/pack 3 + node_modules 200 嵌套 + 5% NUL，T-35-01-01/02 路径黑名单 + df 1GB) + perf-benchmark.sh (hyperfine warmup=1 runs=10 三档 local/mergerfs/sshfs-only + jq P50/P99 + ratio 裁决 PASS(1.5x)/WARN(<=2x)/FAIL，sshfs-only 档 Discretion 落到 ro bind /mnt/cold) + cold-start-benchmark.sh (5×attempt × 200ms tmux capture-pane 探测 prompt × 15s 硬超时 + 三段式 stderr verbatim grep + JSON schema_version=1 双闸门 pass>=4 AND progress_matches_all)；产物 README 含 schema/命名/历史对比 jq 范例；commits b0fd3ba/c388ea7/5078513
- [Phase 35-e2e]: Plan 02 双脚本就位 (35-02 / commits 1926981 / 3a2a2cd)：uat-network-resilience.sh (594 行) BASE-03 三场景 UAT — tc(netem loss 100%) → iptables(OUTPUT DROP) 两级 fallback + 双重 trap disrupt_stop EXIT/INT/TERM + 起始幂等清理 + 三大无感知锚点 (pgrep -f claude 5s 间隔存活循环 + tmux capture-pane diff==0 字符级一致 + token 完整回放) + 30s 场景 60s 内自动重连成功断言 + 2min 场景退避序列 1/2/4/8/30s ≥3 档命中 + REQ-F3-C 失败提示 grep '(按 Enter 重试|cloud-claude doctor)' + REQ-F4-A 进程存活；degradation-regression.sh (534 行) M13 三层静默降级回归 — pkill -9 mergerfs / fusermount3 -u /mnt/cold / pkill -9 mutagen-agent 三层 → docker exec cloud-claude doctor --json → jq -e select(.code==X) 命中期望 MOUNT_* (5 个码与 errcodes/mount.go 注册表交叉一致) + warn/fail check next_action 非空守恒 (M13 等价口径) + 错误码命名正则 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$；T-35-02 威胁矩阵 6 条全落地：双重 trap (T-01) + CTR_NAME_REGEX 命令注入守卫 (T-02) + sudo -n 检测无 prompt (T-03) + --dry-run + --confirm-destructive 双闸门默认全闭 + 中文 opt-in 提示 (T-04) + sed 脱敏 token/key/secret + JSON 只数值不内容 (T-05) + .network-disrupt.log/.degradation-destruct.log 留痕 (T-06)；deviations 2 条均为验证侧 Rule 1 — dry-run+SKIP 路径 stderr 补 disrupt 命令预览 + PLAN acceptance 第 16 条 grep -oE 多文件应改 -hoE，脚本侧 5 个 MOUNT_* 全在注册表语义正确
- [Phase 35-e2e]: Plan 03 runbooks: 5 章 docs/runbooks/v3-*.md 落地（升级指南 + AppArmor 部署 + doctor 5 维度排障 + 持久卷顶层导航 + 43 条错误码索引）；按 Pattern G 头部 + ≥5 ## 章节 + ### 快速诊断命令 小节统一风格；AppArmor 锁定 D-23 三条字面量与 host-preflight.sh::check_apparmor_fusermount3 L51-68 一字不差；错误码索引反向 diff (registry vs 手册) 输出空 — 注册表 43 条 Code 全在手册中；持久卷手册严守 PATTERNS Pattern J 仅作顶层导航，OAuth 持久化跳转 v3-claude-state-volumes.md 不复制
- [Phase 35]: [Phase 35-04] ci.yml 追加 perf-benchmark (本地档 only, hyperfine+ripgrep+jq, --seed=42 --runs=10 --warmup=1) + image-size-regression (image.lock 解析 → docker build → verify-managed-image.sh → 700 * 1024 * 1024 字面量阈值) 两个 job；复用顶部 on/concurrency 不加 job 级 if:；零破坏 go-test/web-build；commit 998c32d

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

Last session: 2026-04-22T11:23:59.275Z
Stopped at: Plan 35-05 in_progress at T3 (checkpoint:human-verify, gate=blocking) — T1 b4b22cc + T2 5923288 done; awaiting 真机三项签字 (M5 APFS / BASE-03 2min / C6 Ubuntu 25.04)
Resume file: None
