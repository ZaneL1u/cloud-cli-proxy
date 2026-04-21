---
phase: 33-claude-code-cli-admin-gc
verified: 2026-04-21T16:45:00Z
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
re_verification: null
human_verification_completed:
  - test: "D-26 端到端 UAT 5 步 + SC3 容器属主 + D-22 admin host detail"
    completed_at: "2026-04-21T15:30:00Z"
    approved_by: "user"
    signal: "成了"
    location: "33-02-SUMMARY.md §SC1-SC6 闭环状态"
follow_ups: # 非阻塞性技术债 — 建议在 Phase 34 / v3.1 backlog 跟进
  - source: "33-REVIEW.md HR-01"
    severity: high
    summary: "actionToHostStatus 对 ActionVolumeRemove 走 default → \"stopped\"，仅靠 admin handler 留空 HostID 维持安全"
    blocks_goal: false
    rationale: "当前 admin DELETE handler HostID=\"\"，UPDATE WHERE id='' 0-row no-op；隐式正确性，未来若新 caller 填 HostID 会静默把 host 翻成 stopped"
    recommendation: "Phase 34 收口时为 ActionVolumeRemove 加 explicit no-op marker"
  - source: "33-REVIEW.md HR-02"
    severity: high
    summary: "embedded 模式 admin DELETE 触发的 UpdateTaskStatus 在 TaskID=\"\" 时落 ERROR 日志"
    blocks_goal: false
    rationale: "功能不受影响（pgx.ErrNoRows 被吞），仅日志噪声；UAT 已通过证实端到端无感"
    recommendation: "EmbeddedDispatcher.Dispatch 内对 update.TaskID=\"\" 短路，避免污染告警"
  - source: "33-REVIEW.md MR-04"
    severity: medium
    summary: "strict 路径 host-agent 删 volume 成功后 Commit 失败 → 数据漂移（DB 行还在但 volume 已不存在），无 audit"
    blocks_goal: false
    rationale: "罕见 DB 抖动场景；当前已 logger.Error 但缺 audit event 追溯"
    recommendation: "补 claude_account.delete_drift 审计事件 + 对账 job"
  - source: "33-REVIEW.md MR-02 / IR-01"
    severity: medium
    summary: "pullImage 错误未落 audit event；ctx-cancellation 真实行为缺测试"
    blocks_goal: false
    recommendation: "RecordEvent 补 runtime.image_pull_failed + dockerPullCmd 抽 var 加超时取消单测"
  - source: "33-REVIEW.md MR-01 / MR-03 / MR-05 / LR-01..03 / IR-02..05"
    severity: low_medium
    summary: "rollback ctx 一致性、force 路径少 logger.Error、agentClient nil 兜底、parseForceFlag 大小写、removeVolumes 多 volume 短路、recordEvent ctx 取消、EmbeddedDispatcher 适配器无直接单测、SSH label 转义白名单、audit metadata 缺 host_id"
    blocks_goal: false
    recommendation: "v3.1 backlog 批次清理"
---

# Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）— Verification Report

**Phase Goal（ROADMAP §Phase 33）：** 让 OAuth credentials 与 Claude Code 缓存跨容器重建持久化——单 Docker named volume 按 `claude_account` 隔离、entrypoint symlink 兜底权限、admin DELETE 事务联动 `volume rm` 防止 orphan 撑爆磁盘。交付 F7 完整闭环（除 F7-C 已落 Phase 31）。

**Verified:** 2026-04-21T16:45:00Z
**Status:** passed
**Re-verification:** No — initial verification
**Human UAT:** Approved by user "成了" on 2026-04-21T15:30:00Z（Task 2.5 checkpoint）

---

## Goal Achievement

### Observable Truths（=ROADMAP §Phase 33 Success Criteria 6 条 + PLAN frontmatter 合并）

| #   | Truth (Success Criterion)                                                                                   | Status     | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --- | ----------------------------------------------------------------------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SC1 | 同一 claude_account 容器删除并重建后，`~/.claude/.credentials.json` OAuth token 保留，无需 `claude login`（REQ-F7-B）          | ✓ VERIFIED | 代码路径完整：(a) `entrypoint.sh:71` `prepare_persistent_state` 在 v3 stage 序列内调用（`entrypoint.sh:279`），`ln -sfn /var/lib/claude-persist/.claude /home/claude/.claude` (`entrypoint.sh:84`)；(b) post-fix `27ab2d7` `runtime_service.go:105+161` 注入 `ClaudeAccountID` 到 `HostActionRequest`（闭合 Plan 01 dispatcher 缺口）；(c) `worker.go:225-262` createHost 自动 `ensureDockerVolume` + 追加 mount。**人工 UAT Step 4+5 由用户确认通过 ("成了")** — UAT 详见 33-02-SUMMARY.md §SC1。                                                          |
| SC2 | `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 返回唯一 volume `claude-state-<id>`（REQ-F7-A） | ✓ VERIFIED | `worker.go:949` `BuildClaudeStateVolumeName` 强制前缀 `claude-state-`；`worker.go:230-231` 写入双 label `com.cloud-cli-proxy.account_id=<id>` + `com.cloud-cli-proxy.managed=true`；单测 `TestBuildClaudeStateVolumeName_NonEmptyID_ReturnsPrefixedName` PASS；UAT Step 1 实测 `docker volume ls --filter label=...` 返回 `claude-state-<id>` 单值。                                                                                                                                                                                |
| SC3 | 容器内 `/home/claude/.claude` `/home/claude/.cache/claude` 属主始终为 `1000:1000`，无权限错误（C5/M17）                          | ✓ VERIFIED | `entrypoint.sh:84` `chown -R 1000:1000 "$root"`（volume 内容物）+ `entrypoint.sh:89` `chown -h 1000:1000 /home/claude/.claude /home/claude/.cache/claude`（symlink 本身）；保留 `prepare_v3_dirs` 既有 chown 兜底（D-10）。UAT Step 7 用户实测 `docker exec -u root <ctr> stat -c "%U:%G"` 返回 `claude:claude`。                                                                                                                                                                                                                       |
| SC4 | admin DELETE claude_account 后，事务结束时对应 volume 已被 `volume rm`（REQ-F7-D / M16）                                  | ✓ VERIFIED | `admin_claude_accounts.go:62-137` `deleteStrict` 在事务内同步调 `runHostAction(ActionVolumeRemove)`，rm 失败 ROLLBACK + audit `claude_account.delete_volume_rm_failed` + HTTP 409 + 中文提示；rm 成功 → DELETE row + COMMIT + audit `claude_account.deleted` + HTTP 200。8 条 handler 单测全 PASS（成功 / 409+rollback / force / 404 / no-volume / 10s+30s timeout / parseForceFlag）。UAT Step 1 用户实测 DELETE → 200 → volume 消失。                                                                                                              |
| SC5 | host-agent 重复收到同一 Volumes 创建请求时 `docker volume create` 幂等返回成功，不报 `volume exists`                              | ✓ VERIFIED | `worker.go:969-988` `realEnsureDockerVolume` 先 `docker volume inspect`，存在则直接 `return nil`，不存在才 create；单测 `TestEnsureDockerVolume_AlreadyExists_SkipsCreate` PASS（仅 1 次 docker call）+ `TestEnsureDockerVolume_NotExists_RunsCreate` PASS（2 次 call: inspect+create）。                                                                                                                                                                                                                                                  |
| SC6 | 删除 account 时 host-agent `volume rm` 失败（容器仍持有），事务回滚且明确日志记录，不留半成品                                            | ✓ VERIFIED | `worker.go:990-1018` `realRemoveDockerVolume` "volume is in use" → `volume_in_use:` 前缀错误；`worker.go:75-77` Execute 错误码映射 `errorCode = "volume_in_use"`；`admin_claude_accounts.go:98-114` handler 收到 error → 写 audit `claude_account.delete_volume_rm_failed` + return 409 + tx defer rollback 触发；单测 `TestAdminClaudeAccountsDelete_StrictHostAgentFailure_RollbackAnd409WithChineseMessage` PASS（断言 `tx.rolledback=true` + 中文 message + audit hasType）。UAT Step 2 用户实测 → 409 + DB 行仍在 + audit 落库。 |

**Score: 6/6 truths verified（含 1 项依赖人工 UAT，已由用户 "成了" 确认）**

---

### Required Artifacts（PLAN 01 + PLAN 02 frontmatter must_haves.artifacts 全集）

| Artifact                                                       | Expected                                                                                            | Status     | Details                                                                                                                                                            |
| -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `deploy/docker/managed-user/entrypoint.sh`                     | `prepare_persistent_state` 函数 + v3 stage 调用                                                        | ✓ VERIFIED | 函数定义 line 71；v3 stage 调用 line 279（位置：`prepare_v3_dirs → prepare_persistent_state → prepare_mutagen_agent`，符合 D-09）                                          |
| `internal/agentapi/contracts.go`                               | `ActionVolumeRemove HostAction = "volume_remove"`                                                   | ✓ VERIFIED | line 11，含 `// Phase 33 D-13` 注释；`TestActionVolumeRemove_StringValue` + `TestHostActionRequest_VolumeRemove_RoundTrip` PASS                                       |
| `internal/runtime/tasks/worker.go`                             | `BuildClaudeStateVolumeName` / `ensureDockerVolume` / `removeDockerVolume` / `removeVolumes` / `createHost` 自动补 volume / `WorkerRepo` 接口扩展 | ✓ VERIFIED | 包级常量 line 943-947；`BuildClaudeStateVolumeName` line 950；`ensureDockerVolume` line 969；`removeDockerVolume` line 994；`removeVolumes` line 1022；`createHost` 自动补 volume line 225-262；`WorkerRepo` 接口扩展 line 42 |
| `internal/store/repository/queries.go`                         | `UpsertClaudeAccountPersistentVolumeName` / `GetHostWithClaudeAccount` / `BeginTx` / `LockClaudeAccountForDelete` / `DeleteClaudeAccountTx` / `ResolveClaudeAccountIDForEntry`（post-fix） | ✓ VERIFIED | 6 个新方法全部就位 line 1230 / 1439 / 1476 / 1493 / 1508 / 1514；3 个 SQL const 提升                                                                                       |
| `internal/store/repository/models.go`                          | `HostWithClaudeAccount` 类型（embed Host + PersistentVolumeName string）                               | ✓ VERIFIED | line 167-170；`PersistentVolumeName string \`json:"persistent_volume_name,omitempty"\``                                                                            |
| `internal/controlplane/http/admin_claude_accounts.go`          | DELETE handler + force flag + 强/最终一致两条路径                                                            | ✓ VERIFIED | `HostActionRunner` interface line 25；`AdminClaudeAccountStore` + `AdminClaudeAccountsHandler`；`Delete()` / `parseForceFlag` / `deleteStrict`(10s) / `deleteForce`(30s) / `recordEvent` 全部就位；STATE_VOLUME_IN_USE_001 + 中文消息 line 107-108；3 类 audit event types 命中 |
| `internal/controlplane/http/admin_hosts.go`                    | `adminHostDetailResponse` 追加 `persistent_volume_name`                                              | ✓ VERIFIED | line 100 字段就位；line 119 enrich 块（失败仅 Warn 不 5xx）；接口扩展 line 35                                                                                                  |
| `internal/controlplane/http/router.go`                         | DELETE 路由注册 + Dependencies 字段扩展                                                                    | ✓ VERIFIED | Dependencies 字段 line 48-49（`AdminClaudeAccounts` / `AgentClient`）；路由注册 + adminGuard line 260-262                                                               |
| `internal/controlplane/app/app.go`                             | post-fix `c09a4d0`：按 mode wire HostActionRunner                                                    | ✓ VERIFIED | line 96 `agentRunner cphttp.HostActionRunner`；line 102-104 embedded 模式注入 `EmbeddedDispatcher`；line 154-155 注入 Dependencies                                       |
| `internal/runtime/tasks/embedded_dispatcher.go`                | post-fix `c09a4d0`：`RunHostAction` 适配 `HostActionRunner` 接口                                       | ✓ VERIFIED | line 40-42                                                                                                                                                          |
| `internal/runtime/runtime_service.go`                          | post-fix `27ab2d7`：注入 `ClaudeAccountID` via `ResolveClaudeAccountIDForEntry`                       | ✓ VERIFIED | `QueueHostActionRepo` interface line 32-44；调用 line 105；`ClaudeAccountID:` 字段填充 line 161                                                                          |
| `docs/runbooks/v3-claude-state-volumes.md`                     | 运维手册：命名规范 + GC 路径 + 孤儿审计                                                                          | ✓ VERIFIED | 240 行；29 个关键 verbatim token grep 命中（命名前缀 / 双 label / mount target / 错误码 / 6 类事件类型 / endpoint + force flag / 孤儿/orphan）                                       |

---

### Key Link Verification

| From                                                       | To                                                                                                                                  | Via                                                                                  | Status     | Details                                                                                                                                              |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `worker.go::createHost`                                    | `ensureDockerVolume` + `UpsertClaudeAccountPersistentVolumeName`                                                                    | `request.ClaudeAccountID != ""` 触发自动补 VolumeMount                                 | ✓ WIRED    | worker.go line 225 `if request.ClaudeAccountID != ""` → line 233 `ensureDockerVolume` → line 262 `UpsertClaudeAccountPersistentVolumeName`            |
| `worker.go::Execute switch`                                | `removeVolumes(ctx, request)`                                                                                                       | `case agentapi.ActionVolumeRemove`                                                    | ✓ WIRED    | worker.go line 67-68                                                                                                                                |
| `entrypoint.sh`                                            | `/home/claude/.claude → /var/lib/claude-persist/.claude`                                                                            | `ln -sfn` 在 prepare_v3_dirs 之后、prepare_mutagen_agent 之前                          | ✓ WIRED    | entrypoint.sh line 71（function）+ line 279（call site between prepare_v3_dirs and prepare_mutagen_agent）+ line 84 `ln -sfn ... /home/claude/.claude` |
| `DELETE /v1/admin/claude-accounts/{accountID}`             | `agentapi.ActionVolumeRemove` via `runHostAction`                                                                                   | 事务内 SELECT FOR UPDATE → host-agent 调用 → 成功 DELETE+COMMIT；失败 ROLLBACK + 409 | ✓ WIRED    | admin_claude_accounts.go line 62-137（strict 路径完整链路）；router.go line 260-262 注册 + adminGuard                                                       |
| `GET /v1/admin/hosts/{hostID}`                             | `Repository.GetHostWithClaudeAccount` LEFT JOIN                                                                                     | 纯 DB JOIN（不引入 docker exec）                                                       | ✓ WIRED    | admin_hosts.go line 119 enrich block；queries.go line 1474-1488 LEFT JOIN + LIMIT 1                                                                 |
| handler `force=true` 路径                                   | `agentapi.HostActionRequest{Labels: {"force": "true"}}`                                                                            | worker.removeVolumes 读 `request.Labels["force"]`                                    | ✓ WIRED    | admin_claude_accounts.go line 177 `Labels: map[string]string{"force": "true"}`；worker.go line 1023 `force := request.Labels["force"] == "true"`     |
| `runtime_service.QueueHostAction`                          | `request.ClaudeAccountID` 注入                                                                                                       | post-fix `27ab2d7` `ResolveClaudeAccountIDForEntry`                                  | ✓ WIRED    | runtime_service.go line 105（resolve）+ line 161（assign to request）                                                                                  |
| `app.go` embedded mode                                     | `cphttp.HostActionRunner` via `EmbeddedDispatcher.RunHostAction`                                                                   | 接口适配                                                                                | ✓ WIRED    | app.go line 102-104 + embedded_dispatcher.go line 40-42                                                                                              |

---

### Data-Flow Trace (Level 4)

| Artifact                                                          | Data Variable                       | Source                                                                                                | Produces Real Data | Status     |
| ----------------------------------------------------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------- | ------------------ | ---------- |
| `adminHostDetailResponse.PersistentVolumeName`                    | `resp.PersistentVolumeName`         | `h.store.GetHostWithClaudeAccount` LEFT JOIN → `COALESCE(ca.persistent_volume_name, '')`              | Yes                | ✓ FLOWING  |
| `worker.createHost` `request.Volumes`                             | `request.Volumes` (mutate in place) | `ensureDockerVolume` + 自动 append `VolumeMount` 后传给 `buildCreateArgs(request, ...)`               | Yes                | ✓ FLOWING  |
| `claude_accounts.persistent_volume_name`                          | DB column                           | `UpsertClaudeAccountPersistentVolumeName` 三态 SQL（NULL→写入 / 一致跳过 / 冲突 error）                 | Yes                | ✓ FLOWING  |
| handler 响应 body `{"deleted":true,"volume_rm":...}` / `error.code` | response map                        | strict / force 路径分支结合 `runHostAction` 真实返回值；UAT Step 1+2+3 验证不同分支 body 形态正确              | Yes                | ✓ FLOWING  |

---

### Behavioral Spot-Checks（runnable bounded checks）

| Behavior                                                      | Command                                                                                                                                                | Result                                                | Status   |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------- | -------- |
| 全仓库构建闭环                                                  | `go build ./...`                                                                                                                                       | exit 0                                                | ✓ PASS   |
| Plan 01 lifecycle 测试                                         | `go test ./internal/runtime/tasks/ -run "TestBuildClaudeStateVolumeName\|TestEnsureDockerVolume\|TestRemoveDockerVolume\|TestActionVolumeRemove" -count=1` | 9 PASS                                                | ✓ PASS   |
| Plan 02 admin handler 测试                                     | `go test ./internal/controlplane/http/ -run "TestAdminClaudeAccountsDelete\|TestParseForceFlag\|TestAdminHostDetail_\|TestAdminHostList_DoesNotInclude" -count=1` | 11 PASS                                               | ✓ PASS   |
| 全包测试无回归                                                  | `go test ./internal/agentapi/ ./internal/runtime/tasks/ ./internal/store/repository/ ./internal/controlplane/http/ -count=1`                           | 全 ok                                                 | ✓ PASS   |
| entrypoint shell 语法                                          | `bash -n deploy/docker/managed-user/entrypoint.sh`                                                                                                     | exit 0                                                | ✓ PASS   |
| audit metadata 凭据守恒                                         | `! grep -nE "Metadata:.*\"(email\|entry_password\|credentials\|oauth_token)\"" admin_claude_accounts.go worker.go`                                     | 0 命中                                                 | ✓ PASS   |
| router adminGuard 链路                                          | `grep "adminGuard(claudeHandler.Delete())" router.go`                                                                                                  | line 262 命中                                          | ✓ PASS   |
| post-fix pullImage timeout 边界                                 | `go test ./internal/runtime/tasks/ -run TestPullImageTimeout_IsBounded`                                                                                | PASS                                                  | ✓ PASS   |
| 运维手册 verbatim 字段                                          | `grep -c "claude-state-\|com.cloud-cli-proxy.\|/var/lib/claude-persist\|STATE_VOLUME_IN_USE_001\|claude_account.\|orphan\|孤儿" runbook`               | 29 命中                                                | ✓ PASS   |
| 端到端容器 rebuild → OAuth 保留                                 | UAT Step 4+5（环境依赖）                                                                                                                                  | user "成了" approved 2026-04-21T15:30:00Z              | ✓ PASS   |

---

### Requirements Coverage

| Requirement | Source Plan(s)        | Description                                                                                                                       | Status        | Evidence                                                                                                                                              |
| ----------- | --------------------- | --------------------------------------------------------------------------------------------------------------------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| REQ-F7-A    | Plan 01 + Plan 02     | `~/.claude` 与 `~/.cache/claude` 通过独立 Docker named volume 持久化，命名 `claude-state-{id}` + label `com.cloud-cli-proxy.account_id` | ✓ SATISFIED   | `BuildClaudeStateVolumeName` + 双 label（worker.go:230-231）+ entrypoint symlink + admin host detail 显式 `persistent_volume_name` 字段（OOS-A19 边界守恒） |
| REQ-F7-B    | Plan 01               | 容器重建后未过期的 OAuth credentials 必须保留，无需重新 `claude login`                                                                   | ✓ SATISFIED   | entrypoint `prepare_persistent_state` symlink + worker 自动补 mount + post-fix `27ab2d7` dispatcher 注入 `ClaudeAccountID`；UAT Step 5 用户确认通过       |
| REQ-F7-D    | Plan 02               | 通过 admin API 删除 claude_account 时，事务性联动删除对应的 Docker named volume                                                            | ✓ SATISFIED   | `deleteStrict` 强一致路径（事务内同步 rm + 失败 ROLLBACK + 409 + 中文提示）；`deleteForce` 最终一致路径（DB 先 COMMIT + 失败 audit + next_action）；8 条 handler 单测 PASS + UAT Step 1+2+3 |

**注：** REQ-F7-C 已在 Phase 31 落地，本阶段不属于范围（ROADMAP 第 269 行已说明）。本阶段无 ORPHANED 需求。

---

### Anti-Patterns Found

| File                                                          | Line      | Pattern                                                                                              | Severity      | Impact                                                                                                                                          |
| ------------------------------------------------------------- | --------- | ---------------------------------------------------------------------------------------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/runtime/tasks/worker.go`                            | 131-144   | `actionToHostStatus` 对 `ActionVolumeRemove` 走 default → `"stopped"`（HR-01）                          | ⚠️ Warning    | 当前 admin handler HostID="" 走 0-row no-op；未来 caller 填 HostID 会静默把 host 翻 stopped。**不阻塞** Phase 33 goal — 已纳入 follow-ups   |
| `internal/runtime/tasks/embedded_dispatcher.go`               | 25-29     | embedded 模式 `UpdateTaskStatus(ctx, update)` 在 `update.TaskID==""` 时返回 `pgx.ErrNoRows` 落 ERROR 日志（HR-02） | ⚠️ Warning    | 仅日志噪声，功能不受影响（pgx 内部不报错）；UAT 已通过证实端到端无感。**不阻塞** Phase 33 goal — 已纳入 follow-ups                                |
| `internal/controlplane/http/admin_claude_accounts.go`         | 121-125   | strict 路径 `tx.Commit` 失败时仅 logger.Error，无 audit event（MR-04）                                    | ⚠️ Warning    | 罕见 DB 抖动场景；不阻塞主路径，已纳入 follow-ups                                                                                                |
| `internal/runtime/tasks/worker.go`                            | 874-890   | `pullImage` 错误（含超时）只落 `slog.Warn`，无 audit（MR-02）                                              | ℹ️ Info       | 已通过 5min timeout 防 hang，但运维需要回到日志才能定位；不影响 Phase 33 SC                                                                       |
| `internal/runtime/tasks/worker.go`                            | 590-598   | `injectSSHKeys` 把 `key.Label` 拼进 `sh -c`，未做转义（**预存在，非本次改动**）                              | ℹ️ Info       | 来源 admin SSH 密钥 API DB 字段，可信度较高；不属 Phase 33 攻击面                                                                                  |

**Anti-pattern 总览：** 0 blocker、2 warning（=33-REVIEW.md HR-01/HR-02，已分析为非 goal-blocking）、3 info。详细分类与修复建议见 frontmatter `follow_ups`。

---

### Code Review High-Severity 决议（来自 33-REVIEW.md）

| Finding ID | 严重度 | 主题                                                                              | 是否阻塞 Phase 33 goal | 决议依据 |
| ---------- | ------ | --------------------------------------------------------------------------------- | ---------------------- | -------- |
| HR-01      | High   | `actionToHostStatus` 对 `ActionVolumeRemove` 走 default 返回 `"stopped"`，靠调用方留空 HostID 维持隐式正确性 | **不阻塞** — accept | 当前调用路径（admin DELETE）固定不填 HostID（admin_claude_accounts.go:94-97 显式只填 Action+Volumes），UPDATE WHERE id='' 为 0-row no-op；功能正确，仅是脆弱耦合。SC4/SC6 在当前实现下完全达成。建议在 Phase 34 doctor 错误码统一时一并加 explicit no-op marker。 |
| HR-02      | High   | embedded 模式 admin DELETE 触发 `UpdateTaskStatus` 在 TaskID="" 时落 ERROR 日志    | **不阻塞** — accept | pgx 内部不报错（pgx.ErrNoRows 被吞），仅日志噪声；UAT 已用户确认端到端跑通（"成了"）。SC1/SC4 等 SC 不依赖 task status 更新。建议 EmbeddedDispatcher.Dispatch 加短路；纳入 v3.1 backlog。 |

**结论：** 2 项 High 均为 "可工作但脆弱" 的耦合 / 日志污染问题，**不构成 Phase 33 goal 的失败**。已逐项记入 `follow_ups` 供 Phase 34 / v3.1 backlog 跟进。

---

### Human Verification Required

**N/A — 已完成。** Task 2.5 checkpoint:human-verify 由用户在 2026-04-21T15:30:00Z 回复 "成了" 确认通过；UAT 7 步覆盖：

- D-26.1 删除未运行 host 的 account → volume 清理 ✅
- D-26.2 删除有运行 host 的 account（默认）→ HTTP 409 + DB 行仍在 ✅
- D-26.3 加 `?force=true` 重试 → HTTP 200 + DB 删 + volume 删 ✅
- D-26.4 容器 stop/start → `~/.claude/.credentials.json` 内容不变 ✅
- D-26.5 容器 rebuild → OAuth credentials 仍可用 ✅
- 额外 D-22 admin host detail 字段验证 ✅
- 额外 SC3 容器内属主 1000:1000 ✅

UAT 实测细节见 `.planning/phases/33-claude-code-cli-admin-gc/33-02-SUMMARY.md` §SC1-SC6 闭环状态表与 §Audit Event Metadata 白名单实测样本。

---

### Gaps Summary

**No gaps.** 6/6 must-haves verified；REQ-F7-A / REQ-F7-B / REQ-F7-D 全部 SATISFIED；端到端 UAT 已由用户确认通过；代码审查 0 blocking，2 high 已决议为非 goal-blocking 的脆弱耦合。

Phase 33 goal **完全达成**：
- 镜像层：entrypoint symlink + chown 兜底就位
- worker 层：createHost 自动补 named volume + ensureDockerVolume 幂等 + ActionVolumeRemove 协议常量
- 仓储层：UpsertClaudeAccountPersistentVolumeName 三态语义 + BeginTx/Lock/DeleteTx 工具链 + GetHostWithClaudeAccount LEFT JOIN
- handler 层：DELETE 强一致 + force 双路径 + admin host detail 追加 persistent_volume_name + 运维手册章节
- post-fix 层：pullImage 5min timeout + dispatcher ClaudeAccountID 注入 + EmbeddedDispatcher.RunHostAction 适配（共 3 commit 已纳入 SUMMARY 的 deviation 章节）

---

## Recommendation

**proceed → update_roadmap**

Phase 33 已达成 goal 且 UAT 通过，建议 orchestrator：

1. **更新 ROADMAP.md** — 把 Phase 33 状态从 `[ ]` (pending) 改为 `[x]` (complete)，并把括号中的 `awaiting phase-level verification` 改为完成时间戳；同步把 REQUIREMENTS.md 中 REQ-F7-A / REQ-F7-B / REQ-F7-D 标记 Complete。
2. **进入 Phase 34** — Phase 34 (cloud-claude doctor v3 + 错误码统一) 可立即开始，依赖的 `STATE_VOLUME_IN_USE_001` 错误码 + 6 类 audit 事件类型 + 运维手册基线已就位。
3. **将 follow_ups 纳入 Phase 34 / v3.1 backlog** — 特别是 HR-01 (actionToHostStatus marker) 与 HR-02 (EmbeddedDispatcher 短路)，建议在 Phase 34 错误路径收口时一并清理；其余 medium/low 项放 v3.1 backlog 批次处理。

---

_Verified: 2026-04-21T16:45:00Z_
_Verifier: Claude (gsd-verifier)_
