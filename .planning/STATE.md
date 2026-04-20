---
gsd_state_version: 1.0
milestone: v3.0
milestone_name: 远端开发体验升级
status: verifying
stopped_at: Phase 32 context gathered
last_updated: "2026-04-20T07:12:43.189Z"
last_activity: 2026-04-19
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP
**Current focus:** Phase 31 — cli

## Current Position

Milestone: v3.0 远端开发体验升级
Phase: 31 (cli) — EXECUTING
Plan: 3 of 3
Status: Phase complete — ready for verification
Last activity: 2026-04-19

Progress: [░░░░░░░░░░░░░░░░░░░░] 0%（v3.0；Phase 29 待执行）

下一步：继续 `/gsd:execute-phase 29-v3-worker`（Wave 建议不变）。Phase 30 可先 `/gsd-plan-phase 30 --skip-research` 跑 plan-checker 修订手写 PLAN，再 `/gsd-execute-phase 30-entry-api`。

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

### Pending Todos

None — 等待 REQUIREMENTS.md 与 ROADMAP.md 产出后进入 phase 执行。

### Blockers/Concerns

无。前置调研已确认 Mutagen v0.18.1 / mergerfs 2.41.x / sshfs 容器配置全部可行。

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260416-wvu | injectSSHKeys 幂等化，保留用户手加密钥 | 2026-04-16 | cc18acf | [260416-wvu-make-injectsshkeys-idempotent-so-user-ge](./quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/) |
| 260417-0w4 | 新增 cloud-claude ssh doctor 子命令（owner/mode/PEM 尾换行自检与修复） | 2026-04-16 | d716b14, 3f0567c, 7836821 | [260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss](./quick/260417-0w4-cloud-claude-cli-ssh-doctor-workspace-ss/) |

### Phase 29 关键修订记录

- **2026-04-18 D-23 路径双修正**：(1) AppArmor override 路径由 `docker-default` → `fusermount3`（Launchpad bug #2111105 + moby#50013 + sysbox#947 + stargz-snapshotter#2144 一致证据，防御 C6）；(2) 脚本路径由"新增 `deploy/host-preflight.sh`" → "扩展现有 `deploy/scripts/host-preflight.sh`"（PATTERNS AP9 发现）。两处修正均在 29-CONTEXT.md 与 29-DISCUSSION-LOG.md 留下完整审计链。
- **2026-04-18 plan-checker 修订**：R1 关键（Plan 05 Task 5.1 弃用不存在的 log_* helper，改用现网 `echo >&2` + `return 0/1` + `|| true` advisory）+ R2..R6 五项小修订全部完成；round 2 复核 APPROVED。

## Session Continuity

Last session: 2026-04-20T07:12:43.181Z
Stopped at: Phase 32 context gathered
Resume file: .planning/phases/32-ssh-tmux/32-CONTEXT.md
