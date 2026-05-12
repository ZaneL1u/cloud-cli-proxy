---
phase: 47-hotreload
plan: 01
subsystem: control-plane / network / runtime
tags: [bypass, hot-reload, snapshot, rollback, nft, sing-box, atomic-write, consistency]
requires:
  - phase-46-api-ui (snapshot 表 + apply/preview/rollback HTTP 端点已落地)
  - phase-47-02-bypass-firewall (worker netns inet sbfw whitelist_v4 set 已就位)
  - phase-45-net-foundation (GatewayConfigDir + Container Proxy Provider)
provides:
  - "agentapi.HostActionRequest.BypassSnapshotID 字段（5 参 QueueHostAction 契约）"
  - "internal/network/bypass_reload.go::ApplyBypassRuleSet（atomic write + nft -f stdin 事务）"
  - "internal/network/bypass_reload.go::VerifyBypassConsistency + ConsistencyResult"
  - "internal/runtime/tasks/worker_bypass_reload.go::handleReloadHostBypass（pending→applied / rolled_back / failed 三态）"
  - "GET /v1/admin/hosts/{hostID}/bypass/consistency HTTP 端点"
affects:
  - "HostActionQueuer.QueueHostAction 签名从 4 参 → 5 参（新增 bypassSnapshotID）"
  - "WorkerRepo interface 新增 GetBypassSnapshotByID / UpdateBypassSnapshotStatus / GetLatestAppliedBypassSnapshot 三件套"
  - "worker.go case ActionReloadHostBypass 占位字面量删除，改 dispatch 真实 handler"
  - "AdminBypassSnapshotsHandler 加 Consistency() 方法 + verifyConsistencyHook 注入点"
tech-stack:
  added:
    - "sing-box rule-set source v3 schema（{\"version\":3,\"rules\":[{\"ip_cidr\":[…]}]}）"
    - "nft -f stdin 单事务：flush set + 批量 add element"
    - "nft -j list set JSON 解析（{\"nftables\":[{\"set\":{\"elem\":[{\"prefix\":{\"addr\",\"len\"}}]}}]}）"
    - "sha256 归一化对账：去重 + sort + join(\"\\n\")"
  patterns:
    - "包级 var hook 注入：applyBypassRuleSetHook / verifyBypassHook / verifyConsistencyHook / nftRunner / nftJSONLister / sleepHook —— 测试用 t.Cleanup 还原原值"
    - "atomic write：os.CreateTemp → Sync → Chmod → os.Rename，失败 os.Remove(tmp)"
    - "健康检查 3 次循环（间隔 200ms）+ 自动 rollback 上一 applied snapshot：worker 不把 rollback 视为失败（Execute return nil）"
    - "fake repo 继承式叠加：bypassReloadFakeRepo embeds fakeWorkerRepo，仅覆盖 Bypass 三件套，避免污染原有最小 no-op"
key-files:
  created:
    - internal/network/bypass_reload.go
    - internal/network/bypass_reload_test.go
    - internal/runtime/tasks/worker_bypass_reload.go
    - internal/runtime/tasks/worker_bypass_reload_test.go
  modified:
    - internal/agentapi/contracts.go
    - internal/runtime/tasks/worker.go
    - internal/runtime/tasks/ssh_inject_test.go
    - internal/agent/server_test.go
    - internal/runtime/runtime_service.go
    - internal/runtime/runtime_service_test.go
    - internal/controlplane/scheduler/expiry.go
    - internal/controlplane/http/admin_bypass_snapshots.go
    - internal/controlplane/http/admin_bypass_snapshots_test.go
    - internal/controlplane/http/router.go
decisions:
  - "QueueHostAction 第 5 参用专属 bypassSnapshotID 形参，而非通用 payload string —— 类型签名层面把 Phase 46 旧 hack「借 requestedBy 传 snapshot ID」彻底闭死，调用点 grep 可见即正确"
  - "ApplyBypassRuleSet 严格顺序「先 nft 事务 → 后 atomic write」：nft -f 事务失败不动文件，避免「文件已变但 nft 未变」漂移；nft 成功后两个文件按各自 tmp+rename 写盘（任一失败可由 VerifyBypassConsistency 检出）"
  - "健康检查复用容器 namespace 内 `/dev/tcp/192.168.0.1/53` TCP 半握手 —— 既不发真实 DNS 报文（避免污染 audit），也能验证 sb-tun0 路由是否生效"
  - "rollback 路径不走 admin API 重入，直接在 worker 内复用 ApplyBypassRuleSet 重下发 prev.WhitelistCIDRsJSON / prev.WhitelistDomainsJSON，并把 current snapshot 标 rolled_back —— prev snapshot 状态保持 applied 不动（WARN-4 不变式延续 Phase 46）"
  - "Consistency endpoint 包裹 3s context.WithTimeout 防 nft 卡死 DoS（T-47-05），DeadlineExceeded 显式映射 504 BYPASS_CONSISTENCY_TIMEOUT，与一般 500 BYPASS_CONSISTENCY_ERROR 区分以便前端 UI 显示重试还是排障"
  - "Bypass 三件套加进 WorkerRepo interface 而非新建子 interface：避免 worker_bypass_reload.go 再注入第二层依赖；代价是 fakeWorkerRepo / mockWorkerRepo 需补 no-op，但每个测试可自由叠加 bypassReloadFakeRepo override"
metrics:
  duration_minutes: 16
  completed: 2026-05-12
  tasks_completed: 4
  files_created: 4
  files_modified: 9
requirements:
  - BYPASS-RELOAD-01
  - BYPASS-RELOAD-02
  - BYPASS-RELOAD-03
  - BYPASS-RELOAD-04
---

# Phase 47 Plan 01: 热更新链路与流量验证 Summary

把 Phase 46 留在 worker 的 `Phase 46 placeholder; no-op until Phase 47` 占位字面量替换为真实的 reload 链路——
从 admin API 触发 apply / rollback 起，经 QueueHostAction(5 参) 落到 worker，再经
`内存读 snapshot → 原子写两个 rule-set 文件 → nft -f 事务批量更新 @whitelist_v4 set → 等 1s sing-box watch reload →
健康检查 3 次 → 自动 rollback` 全链路落地，并提供 `/bypass/consistency` 端点供运维侧主动校验文件 / nft / DB 三处一致性。

## 任务清单与 commit

| Task | 描述 | Commit |
|---|---|---|
| 1 | `agentapi.HostActionRequest` 加 `BypassSnapshotID` 字段；`HostActionQueuer.QueueHostAction` 签名从 4 参 → 5 参，所有 caller 同步 | `c9bf709` |
| 2 | 新建 `internal/network/bypass_reload.go` 提供 `ApplyBypassRuleSet` / `VerifyBypassConsistency` / `ConsistencyResult`，含 atomic write + nft -f 事务 + sha256 对账（4 个单测） | `1385c95` |
| 3 | 新建 `internal/runtime/tasks/worker_bypass_reload.go`，实现 `handleReloadHostBypass` + 健康检查 3 次 + 自动 rollback；删除 worker.go 中 Phase 46 占位字面量（5 个单测） | `5c596d9` |
| 4 | 新建 `GET /v1/admin/hosts/{hostID}/bypass/consistency` endpoint + `verifyConsistencyHook` 注入点（4 个单测） | `e887afb` |

## 修改文件 + 关键点

### 新增

- `internal/network/bypass_reload.go`：
  - `ApplyBypassRuleSet(ctx, hostID, cidrsJSON, domainsJSON) error`：解析两份 v3 schema → 抽 CIDR 并拼 `flush set` + `add element` 行 → 走包级 `nftRunner` 跑 `nft -f -` 单事务 → 通过 `atomicWrite` 把两份 JSON 落到 `<GatewayConfigDir(hostID)>/whitelist-cidrs.json` 和 `whitelist-domains.json`（mode 0o644）。
  - `VerifyBypassConsistency(ctx, hostID) (ConsistencyResult, error)`：读两份文件 hash + `nftJSONLister` 跑 `nft -j list set inet sbfw whitelist_v4` 解析所有 `prefix.addr/len` → 与文件 hash 归一化（去重 + sort + join(\"\\n\") 后 sha256）比对。
  - `nftRunner` / `nftJSONLister` 是包级 `var` —— 默认绑真实 `exec.CommandContext`，单测覆盖。
- `internal/network/bypass_reload_test.go`：4 个测试覆盖 `TestApplyBypassRuleSet_AtomicWrite`（rename 成功路径）、`TestApplyBypassRuleSet_NftFailureLeavesFilesUntouched`（nft 失败时文件不变）、`TestApplyBypassRuleSet_StdinShape`（断言 stdin 含 `flush set inet sbfw whitelist_v4` + `add element` 行序）、`TestVerifyBypassConsistency_HashMatch`（文件 vs nft set 一致 → OK=true）。`withTempGatewayBase` / `withFakeNftRunner` / `withFakeNftJSONLister` 三个 helper 全部 t.Cleanup 自动还原。
- `internal/runtime/tasks/worker_bypass_reload.go`：
  - 常量 `healthCheckRetries=3` / `healthCheckInterval=200ms` / `singboxReloadWait=1s`。
  - 错误 sentinel `ErrBypassReloadInvalidInput` / `ErrBypassReloadFailed`，由 worker.go 映射 errorCode。
  - `handleReloadHostBypass(ctx, req)` 顺序：req.BypassSnapshotID 空 → ErrBypassReloadInvalidInput；GetBypassSnapshotByID → applyBypassRuleSetHook(cidrs, domains) → sleepHook(singboxReloadWait) → 健康检查循环（最多 3 次，间隔 200ms 用 sleepHook）→ 全失败时尝试 autoRollbackToPrevious（GetLatestAppliedBypassSnapshot + applyBypassRuleSetHook with prev.JSON + UpdateBypassSnapshotStatus(current, "rolled_back") + RecordEvent("bypass.reload_rolled_back")，return nil）→ 无 prev 时 UpdateBypassSnapshotStatus(current, "failed") + RecordEvent("bypass.reload_failed") + return ErrBypassReloadFailed；成功路径 UpdateBypassSnapshotStatus(current, "applied") + RecordEvent("bypass.reload_applied")。
  - `verifyBypassHealthyDefault(ctx, hostID)` 默认实现：`docker inspect` 拿到容器 pid → `nsenter -t pid -n -- bash -c 'cat < /dev/tcp/192.168.0.1/53'` 验证 sb-tun0 路由；测试用 verifyBypassHook 替换为 fake 闭包。
- `internal/runtime/tasks/worker_bypass_reload_test.go`：5 个测试守护 acceptance：`Success`（首次健康通过 → applied + bypass.reload_applied）、`AutoRollback`（健康检查 3 次失败 + 存在 prev → rollback 用 prev.JSON + rolled_back + bypass.reload_rolled_back + err==nil）、`NoApplied_FailedTerminal`（健康检查失败 + 无 prev → failed + bypass.reload_failed + ErrBypassReloadFailed）、`MissingSnapshotID`（req.BypassSnapshotID="" → 短路 ErrBypassReloadInvalidInput）、`Dispatch_ReloadHostBypass`（Worker.Execute 触发 handler，无占位字面量）。`bypassReloadFakeRepo` embeds `fakeWorkerRepo` 叠加 Bypass 三件套可注入返回。

### 修改

- `internal/agentapi/contracts.go`：
  - `HostActionRequest` 新增 `BypassSnapshotID string \`json:"bypass_snapshot_id,omitempty"\``，专用于 `ActionReloadHostBypass`，其它 action 留空。
- `internal/controlplane/http/router.go`：
  - `HostActionQueuer.QueueHostAction` 签名扩展为 `(ctx, hostID, action, requestedBy, bypassSnapshotID string)`，第 4 参恢复语义为 actor，第 5 参专用 snapshot ID。
  - `mux.Handle("GET /v1/admin/hosts/{hostID}/bypass/consistency", adminGuard(sh.Consistency()))`。
- `internal/controlplane/http/admin_bypass_snapshots.go`：
  - import `time` + `internal/network`。
  - 包级 `var verifyConsistencyHook = network.VerifyBypassConsistency` + `const consistencyTimeout = 3 * time.Second`。
  - `Consistency() nethttp.Handler`：hostID 空 → 400 / GetHost ErrNoRows → 404 / context.WithTimeout(3s) 包裹 → DeadlineExceeded → 504 / 其它 err → 500 / OK=false → 409 + ConsistencyResult / OK=true → 200 + ConsistencyResult。adminGuard 由 router 层守好，handler 不重复鉴权。
  - apply / rollback 内部 `q.QueueHostAction(...)` 调用全部从 4 参 → 5 参，第 5 参传入新 snapshot.ID（之前 Phase 46 hack 把 snapshot.ID 借 requestedBy 形参传递，本次彻底拆开）。
- `internal/controlplane/http/admin_bypass_snapshots_test.go`：
  - 新增 4 个测试 `TestConsistency_OK` / `TestConsistency_Drift` / `TestConsistency_AdminOnly` / `TestConsistency_Timeout`。
  - `withFakeConsistencyHook` helper：直接 import `internal/network` 包（无循环依赖），把 fake `func(ctx, hostID) (network.ConsistencyResult, error)` 替换包级 hook，t.Cleanup 还原。
  - `stubHostActionQueuer.QueueHostAction` 签名同步为 5 参（第 5 参 `Payload string` 字段断言）。
- `internal/runtime/tasks/worker.go`：
  - `WorkerRepo` interface 加 `GetBypassSnapshotByID` / `UpdateBypassSnapshotStatus` / `GetLatestAppliedBypassSnapshot` 三件套。
  - `case agentapi.ActionReloadHostBypass:` 占位 log 字面量替换为 `err = w.handleReloadHostBypass(ctx, request)`。
  - `errorCode` 映射：`ErrBypassReloadInvalidInput → "bypass_reload_invalid_input"`、`ErrBypassReloadFailed → "bypass_reload_failed"`。
- `internal/runtime/tasks/ssh_inject_test.go` / `internal/agent/server_test.go`：`fakeWorkerRepo` / `mockWorkerRepo` 各补三件套的 no-op 实现，避免接口变更打破现有测试。
- `internal/runtime/runtime_service.go` / `internal/runtime/runtime_service_test.go` / `internal/controlplane/scheduler/expiry.go`：调用 `QueueHostAction` 处全部补 `""` 作为第 5 参（这些路径与 bypass 无关）。

## QueueHostAction 5 参契约同步清单

| Caller | 文件 | 第 5 参传值 |
|---|---|---|
| Apply | `internal/controlplane/http/admin_bypass_snapshots.go` | 新 snapshot.ID |
| Rollback | `internal/controlplane/http/admin_bypass_snapshots.go` | 新 snapshot.ID（不是 target.ID） |
| Apply idempotent dispatch | `internal/controlplane/http/admin_bypass_snapshots.go` | existing snapshot.ID |
| 任务调度 / 生命周期 | `internal/runtime/runtime_service.go` | `""` |
| 到期清理 | `internal/controlplane/scheduler/expiry.go` | `""` |
| HostActionsHandler.{Create,Start,Stop,Rebuild} | `internal/controlplane/http/host_actions.go` | `""` |

`grep -rn 'QueueHostAction(' internal/` 0 处 4 参遗留，编译期保证 5 参一致。

## ApplyBypassRuleSet 顺序设计依据

按「nft 事务 → atomic write」严格顺序：

1. **解析 cidrsJSON v3 schema**：拿出全部 `rules[].ip_cidr[]`，排重 + sort 后生成 nft `add element { …, … }` 单行；任何解析失败立即 return，不动文件、不动 nft。
2. **nft -f - 事务**：stdin 拼 `flush set inet sbfw whitelist_v4\nadd element inet sbfw whitelist_v4 { … }\n`，由 nft kernel 在单事务内 commit；中途任意一行解析失败整批回滚，set 内容保持上次成功值。
3. **atomic write whitelist-cidrs.json**：CreateTemp 同目录 → Write → Sync → Chmod 0o644 → Rename；失败 Remove(tmp)。先 cidrs 后 domains —— sing-box rule-set watch 对两份文件分别 inotify，落盘顺序不要紧但同目录 rename 保证可见性。
4. **atomic write whitelist-domains.json**：同上。

如果某步失败：
- 1 失败：文件 + nft 都没动 → 上层 worker 命中健康检查失败 → 自动 rollback prev。
- 2 失败：nft 内核回滚 → 文件没动 → 同上 rollback。
- 3 / 4 失败：nft 已更新但文件未对齐 → 下一次 `/bypass/consistency` 会侦测 hash 漂移返回 409 → 运维侧 apply 重新覆盖（幂等）。

## handleReloadHostBypass 状态机

```
                ┌── pending ──→ apply rule-set ──→ nft ok? ──→ sleep 1s ──→ health probe (≤3 次)
                │                                                                │
                │                                                          ┌─────┴─────┐
                │                                                          │           │
                │                                                          ✔           ✘
                │                                                          │           │
                │                                                  status=applied  prev applied?
                │                                                  event=*.applied      │
                │                                                  return nil       ┌───┴───┐
                │                                                                   ✔       ✘
                │                                                            apply prev    status=failed
                │                                                            status=rolled_back  event=*.failed
                │                                                            event=*.rolled_back  return ErrBypassReloadFailed
                │                                                            return nil
                └── BypassSnapshotID == "" ──→ return ErrBypassReloadInvalidInput
```

错误码三态彻底分离：
- `applied`：成功路径。
- `rolled_back`：本 snapshot 健康检查失败但 prev 重新生效，集群仍有可用配置 —— Execute return nil，task 视为成功。
- `failed`：连 prev 都没有 —— 集群无可用配置（典型为首次 apply 失败），Execute 把 ErrBypassReloadFailed 映射为 `bypass_reload_failed` errorCode + Status="failed"。

## Consistency endpoint 状态码语义

| 情况 | HTTP | error_code / payload |
|---|---|---|
| hostID 缺失 | 400 | `BYPASS_INVALID_REQUEST` |
| host 不存在 | 404 | `BYPASS_HOST_NOT_FOUND` |
| `ctx.DeadlineExceeded`（3s 超时） | 504 | `BYPASS_CONSISTENCY_TIMEOUT` |
| nft 命令其它错误 | 500 | `BYPASS_CONSISTENCY_ERROR` + err.Error() |
| hash 漂移（OK=false） | 409 | `ConsistencyResult{ok:false, ruleset_sha256, nft_set_sha256, detail}` |
| 一致（OK=true） | 200 | `ConsistencyResult{ok:true, ruleset_sha256, nft_set_sha256}` |

## 测试与验证

- `go vet ./...` ✅
- `go build ./...` ✅
- `go test ./internal/network/ -count=1 -run 'Bypass'` ✅ 4 个 case 全绿
- `go test ./internal/runtime/tasks/ -count=1 -run 'HandleReloadHostBypass|Execute_Dispatch_ReloadHostBypass'` ✅ 5 个 case 全绿
- `go test ./internal/controlplane/http/ -count=1 -run 'Consistency'` ✅ 4 个 case 全绿
- `go test ./internal/controlplane/http/ -count=1` ✅ 整个 http 包测试通过

## Acceptance 验收

| 条目 | 检查 | 命中 |
|---|---|---|
| 占位字面量删除 | `grep -n "Phase 46 placeholder" internal/runtime/tasks/worker.go` | ✅ 0 匹配 |
| Worker.Execute 真调 handler | `grep -n "w.handleReloadHostBypass" internal/runtime/tasks/worker.go` | ✅ 命中 |
| ApplyBypassRuleSet 暴露 | `grep -n "func ApplyBypassRuleSet" internal/network/bypass_reload.go` | ✅ 命中 |
| Consistency endpoint 注册 | `grep -n "GET /v1/admin/hosts/{hostID}/bypass/consistency" internal/controlplane/http/router.go` | ✅ line 288 |
| Consistency handler 实现 | `grep -n "func.*Consistency" internal/controlplane/http/admin_bypass_snapshots.go` | ✅ line 649 |
| BypassSnapshotID 字段 | `grep -n "BypassSnapshotID" internal/agentapi/contracts.go` | ✅ 命中 |
| QueueHostAction 5 参 | `grep -rn "QueueHostAction(.*,.*,.*,.*,.*)" internal/` 全 5 参，无 4 参遗留 | ✅ 编译期保证 |

## Deviations from Plan

无。Plan 按既定顺序 4 个 task 全部完成，每个 task 原子 commit，未触发任何 Rule 1/2/3 自动修复，未碰到 Rule 4 架构级决策。

## 后续工作

- Plan 47-03 将基于本 plan 暴露的 `verifyConsistencyHook` 注入点接入定时巡检 + 报警。
- 前端 ApplyProgressDialog 在 Phase 46 已经接好 SSE pending/applied/failed 三态推进；本 plan 新增 `bypass.reload_rolled_back` 事件类型，前端可在下一次 UI 迭代时把「自动回滚成功」展示为信息态而非失败态（暂不阻塞）。

## Self-Check: PASSED

- `[ -f internal/network/bypass_reload.go ]` ✅ FOUND
- `[ -f internal/network/bypass_reload_test.go ]` ✅ FOUND
- `[ -f internal/runtime/tasks/worker_bypass_reload.go ]` ✅ FOUND
- `[ -f internal/runtime/tasks/worker_bypass_reload_test.go ]` ✅ FOUND
- `git log | grep c9bf709` ✅ FOUND
- `git log | grep 1385c95` ✅ FOUND
- `git log | grep 5c596d9` ✅ FOUND
- `git log | grep e887afb` ✅ FOUND
