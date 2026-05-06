---
gsd_state_version: 1.0
milestone: v3.1
milestone_name: "映射语义补齐与懒加载"
status: complete
last_updated: "2026-05-04T12:42:00.000Z"
last_activity: 2026-05-04
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 18
  completed_plans: 18
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-24 — v3.1 milestone shipped)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Planning next milestone

## Current Position

Milestone: v3.1 映射语义补齐与懒加载 — ✅ SHIPPED (2026-04-24)
Phase: 37 (e2e-uat) — COMPLETE
Plan: 5 of 5 (全部完成)
Status: Phase complete — ready for verification
Last activity: 2026-05-06 - 完成 quick task 260506-ty7: 修复种子 admin 凭据缺口

Progress: [██████████] 100% (11/11 plans across v3.1 phases)

下一步选项：

- `/gsd:new-milestone` — 开始下一里程碑规划

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
- Phase 36-01: MOUNT_REQUIRE_GIT_REPO 与 MOUNT_OVERSIZED_FILE_SKIPPED 不加入 ExplainExempt，必须提供完整长说明
- Phase 36-01: explain 子进程测试改为每个 go test 进程编译独立临时二进制，避免陈旧 /tmp 缓存
- [Phase 36-02]: Config.HotSyncMaxFileMB int yaml hot_sync_max_file_mb,omitempty + EffectiveHotSyncMaxFileMB() 默认 50MB（D-04 不在 Validate 强校验上限）；LastSessionSnapshot 末尾追加 OversizedFiles []OversizedFile omitempty + 新增 OversizedFile struct (Path string + SizeBytes int64) schema_version=1 不变；3 条序列化测试 PASS（Roundtrip/OmitemptyEmpty/OmitemptyNil），既有 Phase 32 D-27 测试零回归；Plan 03 可直接 cfg.EffectiveHotSyncMaxFileMB()*1024*1024 注入 HotSyncConfig.MaxFileBytes，并把扫描结果以 cwd 相对路径形式赋给 snapshot.OversizedFiles（T-36-02-02 Path 相对路径 mitigate 落在 Plan 03 写端）；commits a8c3cb5 (feat config) + cdeebb5 (test RED) + b1bdbdd (feat GREEN)
- [Phase 36-05]: mount_sshfs.go::mountSSHFS sshfsCmd 字面量在 ConnectTimeout=10 之后追加 cache=yes,kernel_cache,auto_cache,cache_timeout=300（4 个 FUSE page cache 参数，顺序锁死，便于 Plan 06 doctor sshfs_cache_args check 字符串匹配）；新增 internal/cloudclaude/mount_sshfs_test.go 含 TestSSHFSCacheHitsKernelPageCache（fixture SSH+SFTP server + countingFileReader atomic counter + 真实 sshfs 进程挂载，sshfs/fusermount 缺失自动 Skip，本机 macOS PASS=SKIP 符合验收）；Rule 1 修订 PLAN 引用 pkg/sftp v1.13.10 实际 API：Handlers.FileLister→FileList，sftp.ReadWriteAt 不存在→拆 FileReader/FileWriter；commits b1d9208(feat) + bd467d0(test)
- [Phase 36-03]: hot_sync 单文件熔断（D-05/D-06/D-07/L3）落地：HotSyncStatus 追加 OversizedFiles + HotSyncConfig 追加 MaxFileBytes int64 (零值不熔断) + HotSyncEngine 追加 maxFileBytes/oversized + 抽 applyOversizedFilter(localFiles, recordOversized bool) 私有方法 (initialSync 用 true 写 e.oversized, syncOnce 用 false 静默跳过, D-22 不刷屏)；StartHotSync 返回 HotSyncStatus{OversizedFiles: engine.oversized}；Phase 31 D-11 整目录级 SkipDir 完全保留与本 plan 单文件级互补；mount_strategy: MountConfig 追加 HotSyncMaxFileMB int + 包级 mountDefaultHotSyncMaxFileMB=50 + 私有 effectiveHotSyncMaxFileMB() accessor (Rule 2 兜底防止 main.go 未注入字段时静默关闭熔断)；tryModeReal 在 HotOnly/Full 双路径都注入 MaxFileBytes = effectiveHotSyncMaxFileMB() * 1024 * 1024；MountWorkspace 成功分支 snapshot.OversizedFiles = hotStatus.OversizedFiles (D-09) + D-08 一次性 stderr 提示 [!] 跳过大文件 N 个（>NMB），由 cold 兜底 (前 5 条 + ... 还有 N 个引导查 last-session.json)；3 条 TestHotSyncOversized_* 契约测试 PASS (60MB 未 ignore / ignore 命中 / 30MB 未超阈)；commits e554f68 (feat hot_sync) + 4268396 (test RED) + 22b4982 (feat GREEN mount_strategy)
- [Phase 36-04] runRoot 中 git 检测固定在 LoadConfig 之后、NewEntryClient 之前；git 不可用与非 git 仓库共用 MOUNT_REQUIRE_GIT_REPO（D-03）；测试用 t.Setenv(PATH="") 而非包级 var 注入
- [Phase 37-02]: ColdPromoter 集成到 tryModeReal Full 路径：mergerfs ready 后启动，cleanup LIFO（promoterCancel → promoter.Wait → cancel watcher → merge → sshfs → hot_sync），stats 在 writeLastSessionWarn 前刷入 snapshot；LastSessionSnapshot 新增 PromotionCount/PromotionBytes/PromotionFailedCount 三个 omitempty 字段；CLOUD_CLAUDE_NO_PROMOTION=1 时 promoter 保持 nil 完全跳过
- [Phase 37-e2e-uat]: promotion stats 在 tryModeReal 返回前刷入 snapshot（writeLastSessionWarn 之前），此时 promoter 刚启动统计为 0——plan 明确接受此为 mount 就绪时的快照语义
- [Phase 37-05]: uat-v31-promotion.sh (619 行) 6 场景全覆盖（git_reject / oversized_skip / fuse_cache_hit / cold_promotion / no_promotion / json_report），--dry-run 默认安全 + --confirm-destructive 触发实际操作；JSON 报告 schema_version=1 退出码 0/1/2；make ci-gate 追加 UAT dry-run；场景 3/4/5 在非 Linux 平台自动 SKIP；commits 2511a33 / bd06353
- [Phase 17]: 删除 Ubuntu 24.04 预置 ubuntu 用户/组释放 UID/GID 1000，groupadd 使用 --force 标志（与 managed-user 模式一致）
- [Phase 17-02]: Claude Code 安装用 curl -o + bash 分离替代 curl | bash 管道，因 Docker RUN 默认 /bin/sh 是 dash 不支持 pipefail，管道失败无法被 set -e 捕获

### Pending Todos

v3.1 milestone 已启动；等待 ROADMAP.md 写入后进入 Phase 36 执行：

- Phase 36 — 映射前置约束 + sshfs 内核缓存（MOUNT-V31-01..05）
- Phase 37 — 冷文件读触发晋升 + e2e UAT（MOUNT-V31-06..12）

### Blockers/Concerns

无。前置调研已确认 Mutagen v0.18.1 / mergerfs 2.41.x / sshfs 容器配置全部可行。

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260416-wvu | injectSSHKeys 幂等化，保留用户手加密钥 | 2026-04-16 | cc18acf | [260416-wvu-make-injectsshkeys-idempotent-so-user-ge](./quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/) |
| 260417-0w4 | 新增 cloud-claude ssh doctor 子命令（owner/mode/PEM 尾换行自检与修复） | 2026-04-16 | d716b14, 3f0567c, 7836821 | [260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss](./quick/260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss/) |
| 260418 | 三层 panic recovery（Worker/Dispatcher/HTTP handler）修复启动排队卡住 | 2026-04-24 | e2bdc38, 19b098f, 15b5f1b | [260418-running-panic-recovery](./quick/260418-running-panic-recovery/) |
| 260420 | 后台添加 claude 配置和 chrome 配置的导出导入功能 | 2026-04-28 | f224a9b | [260420-claude-chrome](./quick/260420-claude-chrome/) |
| 260421 | 宿主机路径挂载管理（DB + API + Worker + 前端全链路） | 2026-04-29 | 2e910f3, ee98afa | [260421-host-bind-mounts](./quick/260421-host-bind-mounts/) |
| 260422 | 研究 cac 项目并评估 Claude 伪装增强方案 | 2026-04-29 | — | [260422-cac-claude](./quick/260422-cac-claude/) |
| 260423 | 容器反检测 + 遥测阻断 + machine-id 唯一化 | 2026-04-29 | 986a18d, 458e1cd, 9b0e660 | [260423-machine-id](./quick/260423-machine-id/) |
| 260424 | 为 cloud-claude 添加外层信息面板（出口IP、账号、文件状态等） | 2026-04-30 | 77aee4d, 64cfcc9, 224bb08 | [260424-cloud-claude-ip](./quick/260424-cloud-claude-ip/) |
| 260425 | 修复 AuthResponse.Status 字段无法解析 JSON 数字类型的问题 | 2026-04-30 | 7b49af3 | [260425-authresponse-status-json-json-cannot-unm](./quick/260425-authresponse-status-json-json-cannot-unm/) |
| 260502-ni0 | 用户中心化凭据重构（DB/后端/前端）+ 容器自启动 + 删除 claude wrapper + 修复 IP 探测镜像 | 2026-05-02 | 6ed9cdc, 9a5db98, b0e6791, f24e3d1, 1b23a18, f744ec0 | [260502-ni0-user-centric-creds](./quick/260502-ni0-user-centric-creds/) |
| 260504-2n4 | 修复挂载路径在长路径下撑爆创建主机对话框与挂载管理卡片 | 2026-05-03 | e208501, bf1c526 | [260504-2n4-fix-mount-path-overflow](./quick/260504-2n4-fix-mount-path-overflow/) |
| 260506 | 修复管理后台 stop→start 后 IP 泄漏 + list/detail 状态不一致 | 2026-05-06 | d43b2c8, 5ac4163 | [260506-stop-start-ip-list-detail](./quick/260506-stop-start-ip-list-detail/) |
| 260504-414 | 为新建主机对话框的挂载路径添加宿主机目录联想功能 | 2026-05-03 | 9754c3e, da36a2b, 4c482a1 | [260504-414-1-get-v1-admin-host-files-path-xxx-api-2](./quick/260504-414-1-get-v1-admin-host-files-path-xxx-api-2/) |
| 260504-dtd | 修复 IP 测试 macOS 报 No such container + 让 docker compose pull 默认拉 sing-box-gateway | 2026-05-04 | a309ab5 | [260504-dtd-ip-macos-no-such-container-docker-compos](./quick/260504-dtd-ip-macos-no-such-container-docker-compos/) |
| 260504-elo | 修复 image.lock v3.3.0 tag 在 ghcr.io 不存在 + 修复 rejoinHostNetworks macOS 误报 | 2026-05-04 | d1cbdf9, 5596353 | [260504-elo-image-lock-v3-3-0-tag-ghcr-io-app-go-rej](./quick/260504-elo-image-lock-v3-3-0-tag-ghcr-io-app-go-rej/) |
| 260505-fjq | Dockerfile 添加 tzdata + 前端时区选择改为固定标准偏移 | 2026-05-05 | c794309, ed0f63d, 8ff12a0 | [260505-fjq-1-dockerfile-tzdata-tz-2](./quick/260505-fjq-1-dockerfile-tzdata-tz-2/) |
| 260505-gjs | 出口 IP 探测改造为 SSE 实时推送（后端 SSE endpoint + 前端 EventSource） | 2026-05-05 | 4ae4948, a1b8654, aa458b4 | [260505-gjs-ip-sse-sse-endpoint-eventsource-sse-post](./quick/260505-gjs-ip-sse-sse-endpoint-eventsource-sse-post/) |
| 260506-ty7 | 修复种子 admin 凭据缺口（主机创建卡在创建中） | 2026-05-06 | 5b0105b, 7bfc626, 55dec86 | [260506-ty7-admin](./quick/260506-ty7-admin/) |

### Roadmap Evolution

- Phase 29.1 inserted after Phase 29: 修复 GetHost 缺失 entry_password 字段导致容器密码退化为 workspace（URGENT — 线上 P0）

### Phase 29 关键修订记录

- **2026-04-18 D-23 路径双修正**：(1) AppArmor override 路径由 `docker-default` → `fusermount3`（Launchpad bug #2111105 + moby#50013 + sysbox#947 + stargz-snapshotter#2144 一致证据，防御 C6）；(2) 脚本路径由"新增 `deploy/host-preflight.sh`" → "扩展现有 `deploy/scripts/host-preflight.sh`"（PATTERNS AP9 发现）。两处修正均在 29-CONTEXT.md 与 29-DISCUSSION-LOG.md 留下完整审计链。
- **2026-04-18 plan-checker 修订**：R1 关键（Plan 05 Task 5.1 弃用不存在的 log_* helper，改用现网 `echo >&2` + `return 0/1` + `|| true` advisory）+ R2..R6 五项小修订全部完成；round 2 复核 APPROVED。

## Session Continuity

Last session: 2026-05-04T12:42:00.000Z

## Deferred Items

Items acknowledged and deferred at v3.0 milestone close on 2026-04-23:

| Category | Item | Status | 处置路径 |
|----------|------|--------|----------|
| uat_gap | Phase 32 — 5 项 docker UAT (30s 抖动 / pkill -SIGHUP sshd / 多端 banner / 账号级单例锁 / sshfs 抖动 30s) | partial | 已转入 Phase 35 真机签字队列；跟踪在 32-HUMAN-UAT.md |
| uat_gap | Phase 35 — 3 项真机签字 (M5 APFS / BASE-03 2min / C6 Ubuntu 25.04) | partial | deferred-to-ship；跟踪在 35-HUMAN-UAT.md，ship 前补签 |
| verification_gap | Phase 11 — 11-VERIFICATION.md (gaps_found) | gaps_found | v1.2 历史残留（v1.2 partial close），与 v3.0 无关 |
| verification_gap | Phase 12 — 12-VERIFICATION.md (human_needed) | human_needed | v1.2 历史残留；用户自助面板 pending human verification |
| verification_gap | Phase 31 — 缺 31-VERIFICATION.md | doc_only | 代码 + 测试齐全（mount_strategy_test.go / oauth_check_test.go / integration_test.go），仅缺结构化档案；可选 /gsd-verify-phase 31 补 |
| verification_gap | Phase 35 — 缺 35-VERIFICATION.md | doc_only | 5 plan SUMMARY 完整 + 35-HUMAN-UAT.md 跟踪；可选 /gsd-verify-phase 35 补 |
| verification_gap | Phase 29.1 — 缺 29.1-VERIFICATION.md | doc_only | P0 hotfix；4 plan SUMMARY 完整；线上修复已生效 |
| quick_task | 260328-trs-cpu | missing | 与 v3.0 milestone goal 无直接绑定 |
| quick_task | 260328-u4q-readme-vitepress-github-pages | missing | 同上 |
| quick_task | 260405-h13-root-claude-settings-claude-claude-code | missing | 同上 |
| quick_task | 260405-hai-claude-api-pid | missing | 同上 |
| quick_task | 260405-hio-claude-code-settings | missing | 同上 |
| quick_task | 260405-jji-image-version-mgmt | missing | 同上 |
| quick_task | 260405-qk2-ssh | missing | 同上 |
| quick_task | 260416-wvu-make-injectsshkeys-idempotent-so-user-ge | missing | 同上 |
| quick_task | 260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss | missing | 同上 |
| tech_debt | Phase 32 — WR-01 SIGWINCH goroutine 泄漏 | warning | v3.1 backlog 批次处理 |
| tech_debt | Phase 32 — WR-02 *registryPid data race | warning | v3.1 backlog |
| tech_debt | Phase 32 — WR-05 cd builtin fallback 路径不可用 | warning | v3.1 backlog |
| tech_debt | Phase 33 — HR-01 actionToHostStatus default → "stopped" 隐式正确性 | warning | Phase 34 / v3.1 backlog 加 explicit no-op marker |
| tech_debt | Phase 33 — HR-02 EmbeddedDispatcher UpdateTaskStatus TaskID="" 落 ERROR 日志 | warning | v3.1 backlog 加短路 |
| tech_debt | Phase 33 — MR-04 strict 路径 Commit 失败数据漂移无 audit | warning | v3.1 backlog |
| tech_debt | Phase 33 — MR-02 / IR-01 pullImage 错误未落 audit | info | v3.1 backlog |
| tech_debt | Phase 33 — MR-01/03/05 + LR-01..03 + IR-02..05 (rollback ctx / force 路径 logger / agentClient nil 等) | low_medium | v3.1 backlog 批次清理 |
| tech_debt | Phase 34 — integration_test.go CI workflow 接入 | info | make ci-gate 已就绪，待 CI 配置 |
| tech_debt | Phase 35 — M13 destructive 测试需 --confirm-destructive 默认 SKIP | accepted | T-35-05-03 mitigation |
| tech_debt | cross-cutting — Spec/code 数字漂移 (Registry 43 vs spec 42, ExtendedExplanations 39 vs 38, FixerRegistry 6 vs 5) | doc_only | 均 ≥ 需求最小值；建议 ship 前对齐 spec |
| tech_debt | cross-cutting — ROADMAP 未记 SupportsMutagen 字段省略的设计变更 | doc_only | Phase 31 用自研 hot-sync 替换 Mutagen，留 SupportsMergerfs 等价；v3.1 spec 修订 |

**Planned Phase:** 37 (冷文件读触发晋升 + e2e UAT) — 5 plans — 2026-04-24

### Phase 37 执行记录

- **37-04** (2026-04-24): 创建 docs/runbooks/v31-cold-promotion.md Pattern G 运维手册 — commit fc9d3ca
- 覆盖原理图、启停、排障、协同、错误码反查 5 大方面，6 个章节，5 个错误码全部覆盖
- **37-05** (2026-04-24): 编写 619 行 uat-v31-promotion.sh + CI 接入 make ci-gate — commits 2511a33 / bd06353
- 6 大场景全覆盖（git_reject / oversized_skip / fuse_cache_hit / cold_promotion / no_promotion / json_report）
- --dry-run 默认安全 + --confirm-destructive 触发实际操作；JSON 报告 schema_version=1 退出码 0/1/2
- **37-01** (2026-04-24): 实现 ColdPromoter 核心引擎（inotify watcher + PromotionEngine）— commits dc0c86a / b9786af
- ColdPromoter 完整实现：5s 去重窗口 + 1/2/4s 退避重试 + 熔断集合 + QueueDepth/Stats/Wait 可观测 API
- 4 条核心单测全 PASS（含 -race）：dedup / retry-backoff / circuit-breaker / start-stop
- 平台兼容：Linux（真实 inotify）/ macOS（stub）通过 build tag 分离
- 新增 MOUNT_PROMOTER_FAILED 错误码（Warn 级 + >=200 字 ExtendedExplanation）
- 剩余计划：37-02 / 37-03 / 37-05
