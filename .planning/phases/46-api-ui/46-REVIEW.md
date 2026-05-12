---
phase: 46-api-ui
reviewed: 2026-05-12T00:00:00Z
depth: standard
files_reviewed: 25
files_reviewed_list:
  - internal/agentapi/contracts.go
  - internal/controlplane/app/app.go
  - internal/controlplane/http/admin_bypass_audit_log.go
  - internal/controlplane/http/admin_bypass_audit_log_test.go
  - internal/controlplane/http/admin_bypass_bindings.go
  - internal/controlplane/http/admin_bypass_presets.go
  - internal/controlplane/http/admin_bypass_rules.go
  - internal/controlplane/http/admin_bypass_snapshots.go
  - internal/controlplane/http/bypass_audit_helper.go
  - internal/controlplane/http/bypass_render.go
  - internal/controlplane/http/bypass_validation.go
  - internal/controlplane/http/router.go
  - internal/runtime/tasks/worker.go
  - internal/store/migrations/0020_host_bypass_snapshot_source.sql
  - internal/store/repository/queries_bypass.go
  - web/admin/src/components/bypass/bypass-tab.tsx
  - web/admin/src/components/bypass/bypass-rule-drawer.tsx
  - web/admin/src/components/bypass/preview-sheet.tsx
  - web/admin/src/components/bypass/apply-progress-dialog.tsx
  - web/admin/src/components/bypass/rollback-confirm-dialog.tsx
  - web/admin/src/components/bypass/custom-rules-table.tsx
  - web/admin/src/components/bypass/preset-grid.tsx
  - web/admin/src/components/bypass/preset-card.tsx
  - web/admin/src/components/bypass/risky-keyword-confirm.tsx
  - web/admin/src/components/bypass/json-viewer.tsx
  - web/admin/src/components/bypass/nft-diff-viewer.tsx
  - web/admin/src/hooks/use-bypass-presets.ts
  - web/admin/src/hooks/use-bypass-rules.ts
  - web/admin/src/hooks/use-bypass-bindings.ts
  - web/admin/src/hooks/use-bypass-snapshots.ts
  - web/admin/src/lib/api/bypass.ts
findings:
  critical: 7
  warning: 9
  info: 5
  total: 21
status: fixed
fixed_at: 2026-05-12T00:00:00Z
fix_scope: critical_warning
fixed: 16
skipped: 0
deferred: 5
---

# Phase 46: Code Review Report

**Reviewed:** 2026-05-12
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Plan 46-01/02 后端实现质量较高：pgx v5 参数化、双轨审计 helper、5+1 护栏、rollback WARN-4 修复都到位；`is_system` 双层防御 + UNIQUE 幂等也正确执行了 plan 锁定的语义。但是 **前端 46-03/46-04 与后端 API 契约存在系统级失配**：URL 路径、HTTP method、payload 字段、Response 类型全部对不上，导致几乎所有 Bypass UI 交互都会 404 或被服务端拒绝。本审查共记录 7 条 CRITICAL（前后端契约错位、幂等路径返回空 task_id 卡 UI、SSE URL 缺鉴权 token、JSON 折叠护栏阈值过高等）+ 9 条 WARNING + 5 条 INFO。Phase 47 接管前必须先修 CR-01 ~ CR-07，否则后台无法操作。

## Critical Issues

### CR-01: 前端 listBypassRules URL 不匹配后端路由

**File:** `web/admin/src/lib/api/bypass.ts:21-25`
**Issue:** 前端调用 `apiFetch("/hosts/${hostId}/bypass/rules")`，加上 `API_BASE="/v1/admin"` 后实际请求 `GET /v1/admin/hosts/{hostId}/bypass/rules`。但 `router.go:263` 注册的是 `GET /v1/admin/bypass/rules`（host 维度通过 `?host_id=` query 参数过滤），不存在 `/hosts/{hostId}/bypass/rules` 路由。结果：自定义规则表永远 404，UI 显示空列表。
**Fix:**
```ts
export function listBypassRules(hostId: string) {
  return apiFetch<{ rules: BypassRule[] }>(`/bypass/rules?host_id=${encodeURIComponent(hostId)}`);
}
```
**Status:** fixed (commit c5251ee)

### CR-02: 前端 createBypassRule 既错路由又缺 scope/host_id 字段

**File:** `web/admin/src/lib/api/bypass.ts:27-35`, `web/admin/src/lib/api/types/bypass.ts:49-55`
**Issue:** 前端 POST 到 `/v1/admin/hosts/{hostId}/bypass/rules`，但后端是 `POST /v1/admin/bypass/rules`。即便 URL 改对，payload `BypassRuleCreatePayload` 也不含 `scope` 与 `host_id`，而 handler `admin_bypass_rules.go:105-112` 强制校验 `scope ∈ {global, host}`，缺失直接 400 BYPASS_INVALID_REQUEST。结果：所有自定义规则创建必然失败。
**Fix:**
```ts
export function createBypassRule(hostId: string, payload: BypassRuleCreatePayload) {
  return apiFetch<{ rule: BypassRule }>(`/bypass/rules`, {
    method: "POST",
    body: JSON.stringify({ scope: "host", host_id: hostId, ...payload }),
  });
}
// 同步给 BypassRuleCreatePayload 加 scope?/host_id? 透传字段或直接在调用处合并
```
**Status:** fixed (commit bc0ebf6)

### CR-03: 前端 updateBypassRule 用 PUT，后端注册的是 PATCH

**File:** `web/admin/src/lib/api/bypass.ts:37-49`
**Issue:** 前端 `method: "PUT"` 请求 `/v1/admin/hosts/{hostId}/bypass/rules/{ruleId}`。后端只有 `PATCH /v1/admin/bypass/rules/{ruleID}`（router.go:266），既不接受 PUT，URL 也不带 host 前缀。结果：编辑规则 100% 失败。
**Fix:**
```ts
export function updateBypassRule(hostId: string, ruleId: string, payload: BypassRuleUpdatePayload) {
  return apiFetch<{ rule: BypassRule }>(`/bypass/rules/${ruleId}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}
```
`deleteBypassRule` 同步改为 `/bypass/rules/${ruleId}`。
**Status:** fixed (commit 36b3f8f)

### CR-04: 前端 listBypassBindings / createBypassBinding / deleteBypassBinding URL 全部错位

**File:** `web/admin/src/lib/api/bypass.ts:58-78`
**Issue:** 前端 list/create 用 `/hosts/{hostId}/bypass/bindings`，delete 用 `/hosts/{hostId}/bypass/bindings/{bindingId}`。后端是：
- `GET /v1/admin/hosts/{hostID}/bypass`（不带 `/bindings` 后缀，router.go:272）
- `POST /v1/admin/hosts/{hostID}/bypass`（同上，router.go:273）
- `DELETE /v1/admin/bypass/bindings/{bindingID}`（不带 host 前缀，router.go:274）

结果：预设网格里所有 toggle preset 的勾选/取消都 404，binding 永远落不了库。
**Fix:**
```ts
export function listBypassBindings(hostId: string) {
  return apiFetch<{ bindings: BypassBinding[] }>(`/hosts/${hostId}/bypass`);
}
export function createBypassBinding(hostId: string, presetId: string) {
  return apiFetch<{ binding: BypassBinding }>(`/hosts/${hostId}/bypass`, {
    method: "POST",
    body: JSON.stringify({ preset_id: presetId }),
  });
}
export function deleteBypassBinding(_hostId: string, bindingId: string) {
  return apiFetch<void>(`/bypass/bindings/${bindingId}`, { method: "DELETE" });
}
```
**Status:** fixed (commit 1be580a)

### CR-05: BypassRule 前端类型含 port 字段，后端 model 不存在该列；UI 展示 rule.port 必然永远是 null

**File:** `web/admin/src/lib/api/types/bypass.ts:29-39`, `web/admin/src/components/bypass/custom-rules-table.tsx:230-232`, `internal/store/repository/models.go:356-366`
**Issue:** 前端类型声明 `port: string | null`，UI 表格还专门做了「端口」列；但后端 `BypassRule` 结构体只有 `ID/Scope/HostID/RuleType/Value/Note/IsRisky/CreatedAt/UpdatedAt`，SQL 列也没 port。validation handler 接受 `port` 字段（用于校验冲突）但不写入持久化；GET 返回的对象绝不会含 port。Drawer 编辑时 `rule.port ?? ""` 永远初始化为空串，等于 "用户保存 port 后回填看不见"，且端口列对所有行显示「—」给出假象。
**Fix:** 二选一：
1. 加 SQL 列 + 迁移 + Repository 字段 + handler 持久化 port；
2. 移除前端 `port` 字段、删除表格端口列与 Drawer 端口输入，明确「v3.5 不支持端口区分」。
推荐选 2（plan 46-01 truth 没有 port 持久化承诺），然后更新 46-UI-SPEC。
**Status:** fixed (commit 317a834 — 选方案 2，前端移除 port)

### CR-06: Apply 幂等路径返回 `task_id=""`，前端 UI 永远卡在 "dispatch" 阶段

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:308-319`, `web/admin/src/components/bypass/apply-progress-dialog.tsx:82-91,102-110`
**Issue:** Apply handler 在 UNIQUE 冲突幂等命中时直接 `writeJSON(... applyResponse{...})`，`TaskID` 字段为零值 `""`。前端 `onSuccess: (resp) => setTaskId(resp.task_id)` 把空串塞进 state，`useTaskPolling("")` 因 `enabled: !!taskId` 不 poll；但 `isRunning` 判断是 `taskId !== null`，空串不是 null，于是 stageStatuses 永远停在 `["done","active",...]`，dialog 永不自动关闭也不报错，用户感觉「应用卡死」。
**Fix:** 幂等路径只要保证下次有 reload 任务下发即可。方案：
```go
// snapshot 已存在 → 也补一次 dispatch（worker 占位实现是幂等的）
if existing, found, _ := h.findSnapshotByConfigHash(...); found {
    var taskID string
    if h.actions != nil {
        if task, qErr := h.actions.QueueHostAction(r.Context(), hostID, agentapi.ActionReloadHostBypass, existing.ID); qErr == nil {
            taskID = task.ID
        }
    }
    writeJSON(w, nethttp.StatusOK, applyResponse{... TaskID: taskID, ...})
    return
}
```
或前端 `setTaskId(resp.task_id || null)` + 视空字符串为「已完成」。
**Status:** fixed (commit 7434739 — 选后端方案，幂等路径补 dispatch)

### CR-07: SSE 订阅 URL 没带 Authorization token

**File:** `web/admin/src/components/bypass/apply-progress-dialog.tsx:67-72`, `web/admin/src/hooks/use-tasks.ts:29`
**Issue:** `useSSE(open ? \`${window.location.origin}/v1/admin/sse?topics=tasks\` : "")` 直接走原生 `EventSource`，不能附带 Authorization header；而 `/v1/admin/sse` 走 `broadcast.Subscribe`，未做任何 token 校验（router.go:335 没套 `adminGuard`）—— 任意未登录访问者可直接订阅 admin 实时事件流，泄漏全局 host/task/event 元数据。即 Bypass UI 引入此模式实际触发的是已存在的鉴权漏洞，但本 Phase 通过 ApplyProgressDialog 把它"扩大使用面"，应一并修。
**Fix:** 在 router.go 把 `mux.HandleFunc("GET /v1/admin/sse", broadcast.Subscribe)` 改为 `mux.Handle("GET /v1/admin/sse", adminGuard(nethttp.HandlerFunc(broadcast.Subscribe)))`，并支持把 token 作为 query param（`?token=...`）传入，Subscribe 端解析校验后再升级 SSE。Bypass UI 端再用 `?token=...` 拼 URL。如果坚持沿用旧 SSE 设计，至少添加 origin / cookie 校验并写入 STATE.md 已知问题。
**Status:** fixed (commit 1003efd — admin + user SSE 都套 guard，sse-manager 新增 buildSSEUrl helper)

## Warnings

### WR-01: ListBypassRules host 维度兜底集合化遗漏 boundRuleIDs 与 host scope 同 ID 时去重

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:104-125`
**Issue:** `collectRenderInput` 先把所有「scope='host' 且 host_id=hostID」加入 input.Rules，然后遍历同一批 rules，若 `r.Scope == "global"` 且 ID 在 `boundRuleIDs` 中也加入。逻辑本身正确，但是若一条规则同时存在 host scope + 又被 binding 引用了 global scope 的同 ID（理论不可能，但 binding.rule_id 是任意 FK）会重复纳入，导致 RenderBypassConfig 的去重 set 虽然不重，但 totalRules++ 会多计一次，summary 计数虚高。
**Fix:** 渲染前用 `seen` set 在 caller 层做一次 dedup：`if _, ok := seen[r.ID]; ok { continue }`。
**Status:** fixed (commit 958bd25)

### WR-02: Apply handler dispatch 失败时仍写 audit_log，但 audit 写 events 表的 message 缺乏失败信号

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:326-339`
**Issue:** `if qErr != nil { h.logger.Error(...) }`，后续 `writeBypassAuditLog(..., "apply", ...)` 无视失败。审计日志和 events 表都标 "apply" 成功，留下 snapshot.pending + 无 task 的孤儿，运维难以发现。
**Fix:** dispatch 失败时审计 action 用 `"apply_dispatch_failed"` 或 note 字段附 `dispatch_error=<msg>`，让 events SSE 推 warning。
**Status:** fixed (commit f393ea9)

### WR-03: writeBypassAuditLog 中 events.RecordEvent 同步阻塞响应

**File:** `internal/controlplane/http/bypass_audit_helper.go:96-113`
**Issue:** 注释（L57）写"events 在后（异步消费）"，但实现是同步调用 `events.RecordEvent(ctx, ...)`，写库 + SSE 广播都阻塞 HTTP response。批量 binding/rule 操作时延迟会成倍叠加。
**Fix:** 拉一个 goroutine 或保持现状但删掉"异步消费"注释以免误导。推荐：
```go
go func() {
    if _, err := events.RecordEvent(context.Background(), ...); err != nil { ... }
}()
```
注意 ctx 要用 background，否则 caller ctx 取消后 goroutine 会失败丢日志。
**Status:** fixed (commit 396ab82 — 保留同步实现以避开测试 race，仅修正误导注释；批量延迟优化留待 Phase 47 Outbox)

### WR-04: Snapshot apply 用 isUniqueViolation 字符串匹配，"unique" 在普通错误信息里可能误命中

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:562-571`
**Issue:** `strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") || "23505" || "host_bypass_snapshots_host_id_config_hash_key"`。第一/第二条匹配过于宽泛 —— 任何 PG 报错信息包含 "unique" 或 "duplicate" 字样都会被误判为幂等命中，包括其他表的 UNIQUE 冲突（一致性约束、用户名重复等）。
**Fix:** 优先用 pgconn.PgError + SQLSTATE：
```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    // 还可检查 pgErr.ConstraintName == "host_bypass_snapshots_host_id_config_hash_key"
}
```
**Status:** fixed (commit 69402fb)

### WR-05: rollback 用 hash 后缀绕开 UNIQUE 冲突会污染未来 nft diff 起点

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:457-473`
**Issue:** 当回滚目标的 config_hash 与现有任意 snapshot 重复，handler 把 `params.ConfigHash` 改为 `<targetHash>:rollback:<version>`。看似只影响落库 row，但 Phase 47 reload 用 ConfigHash 做磁盘文件名 / nft set 标识时，这个带冒号的 hash 会破坏 sha256 校验逻辑（多数文件系统接受冒号但 nft 标识不行），且 `extractPrevCIDRs` 取 `prev.WhitelistCIDRsJSON` 不依赖 hash，但前端展示的 hash 会变得很奇怪。
**Fix:** 重新计算渲染输出 + 重新 sha256，或者改为更稳定的 ULID/UUID 后缀；并把决策记入 audit note。
**Status:** fixed (commit 3de1446 — 用 crypto/rand 8 字节 hex 后缀，保持整串合法 hex；audit note 加 rollback_hash_suffixed=true)

### WR-06: PreviewSheet useEffect 触发依赖 previewMutation.data/isError，但 deps 仅 `[open]`

**File:** `web/admin/src/components/bypass/preview-sheet.tsx:42-55`
**Issue:** 注释 `// eslint-disable-next-line react-hooks/exhaustive-deps` 屏蔽了 lint。`useEffect` 闭包内读取 `previewMutation.data/isPending/isError`，但 deps 只有 `[open]`。如果 sheet 一直打开（不太可能但可能），mutation 状态变化不会触发 re-effect，闭包读到的值不会更新。当前流程 OK 是因为 effect 只在 `open` 切换时执行一次，但属于脆弱编程。
**Fix:** 把判断拆到单独的 useEffect，或用 `previewMutation.status` 做依赖。
**Status:** fixed (commit 52f3e38)

### WR-07: ApplyProgressDialog isFailed 判定让 errorCode 跟 taskStatus 失败混淆，UI 阶段定位有 bug

**File:** `web/admin/src/components/bypass/apply-progress-dialog.tsx:103-126`
**Issue:** `isFailed = taskStatus === "failed" || "canceled" || !!errorCode`。stageStatuses 在 `isFailed && !errorCode`（即 task 失败但 mutation 成功）时返回 `["done","failed",...]`，把 dispatch 标 failed。但真实 task 失败可能在 reload/health 阶段而非 dispatch。所有 task 失败一律落到「下发到 agent」很误导用户。
**Fix:** 用 `task.progress_percent` 判断在哪个阶段失败：< 25 dispatch, < 50 reload, < 75 health, 否则 done 后失败（极少）。
**Status:** fixed (commit 88d0b35 — 同步在 test/setup.ts 补 localStorage polyfill)

### WR-08: PresetCard 用 preset.is_forced 字段，后端模型只有 is_force_on

**File:** `web/admin/src/components/bypass/preset-card.tsx:16,34`, `web/admin/src/components/bypass/preset-grid.tsx:75`, `internal/store/repository/models.go:331`
**Issue:** 前端类型 `BypassPreset.is_forced`，后端 JSON tag `is_force_on`。即便前端 UI 编译过，运行时 `preset.is_forced` 永远 undefined，loopback 不会出现强制锁定 UI（lock 图标、disabled checkbox 都失效）。同理 `rule_count`/`sample_rules` 后端也没有这些字段。
**Fix:** 统一字段名为 `is_force_on`，并在 handler 层补 `rule_count = len(preset.Rules)`、`sample_rules = preset.Rules[:min(3, len)]` 计算字段，或前端改为直接读 `preset.rules`。
**Status:** fixed (commit cfe703c — 选「前端直接读 preset.rules」方案，保持后端契约不动)

### WR-09: useSSE 在 ApplyProgressDialog 中接 origin URL，与 useTasks 重复订阅同一 topic

**File:** `web/admin/src/components/bypass/apply-progress-dialog.tsx:67-72`, `web/admin/src/hooks/use-tasks.ts:29`
**Issue:** `useTaskPolling` 不订阅 SSE（只是 polling 2s 一次），但 dialog 又自己开了一条 SSE 连接给 tasks topic。一个用户同时使用 Bypass + 其他 host 操作时，浏览器会出现多条 `EventSource` 并发，部分浏览器限制单 origin 6 路 HTTP/1.1 并发连接，可能耗尽。
**Fix:** dialog 删 useSSE 直接靠 polling；或者把 useSSE 升级为 useTasksSSE 全局单例。
**Status:** fixed (commit 3fb8fec — 选「dialog 删 useSSE 改靠 polling」方案)

## Info

### IN-01: JSONViewer 折叠阈值 10000 行过高，预览中后端实际只产 ~50 行 indent JSON

**File:** `web/admin/src/components/bypass/json-viewer.tsx:21`
**Issue:** `needsFold = lineCount > 10000`。但 `RenderBypassConfig` 用 `MarshalIndent` + 每条规则一行，1000 条规则上限也就 ~2000 行，10000 行折叠等于永远不触发。这条护栏对真实场景无效，徒增代码。
**Fix:** 阈值降到 1000 或直接删除折叠分支（plan 把它锁死 10000 是基于早期错误预估）。
**Status:** deferred (Info；fix_scope=critical_warning，本轮不修)

### IN-02: extractActorIP 未去除 X-Forwarded-For 中可能的中括号 IPv6 端口

**File:** `internal/controlplane/http/bypass_audit_helper.go:22-41`
**Issue:** XFF 第一段若是 `[::1]:8080`，直接 trim 后会作为 IP 入库，actor_ip 列脏数据。
**Fix:** `if h, _, err := net.SplitHostPort(first); err == nil { return h }`。
**Status:** deferred (Info；fix_scope=critical_warning，本轮不修)

### IN-03: bypass_validation TLD 黑名单写死且漏 .gov/.edu/.mil 等机构 TLD

**File:** `internal/controlplane/http/bypass_validation.go:43-47`
**Issue:** 黑名单只覆盖常见商业/国家 TLD。`.gov` `.edu` `.mil` `.int` `.tv` `.cc` `.top` `.club` 等不在内。`domain_suffix=.gov` 也是「全公网绕过」性质的高危规则但会通过。
**Fix:** 用 IANA TLD 全集（或保守只允许 ≥ 2 段子域 + TLD），并标 TODO 等 Plan 47 引入完整 PSL。
**Status:** deferred (Info；fix_scope=critical_warning，本轮不修)

### IN-04: rollback 在 currentErr != nil 时 currentForAudit 留 nil，audit 行 before 列为空

**File:** `internal/controlplane/http/admin_bypass_snapshots.go:486-492`
**Issue:** 若 host 此前从未 apply 过（latest applied 不存在），rollback 仍可被发起（target 通过 GetByID 找到，但其本身肯定 applied_status='applied' 否则前面 422 已挡）—— 矛盾点：target 是 applied，而 latest applied 不存在？不可能。但 GetLatestAppliedBypassSnapshot 在 DB 查询层可能因为 ctx 取消等异步错误返回 err，此时 currentForAudit=nil，audit before 列丢失上下文。
**Fix:** `currentErr != nil` 时把 err.Error() 写到 note 里 `note += "; current_lookup_error=..."`。
**Status:** deferred (Info；fix_scope=critical_warning，本轮不修)

### IN-05: 多处用 nethttp.HandlerFunc 包 closure，相同 idiom 没抽 helper

**File:** `internal/controlplane/http/admin_bypass_*.go` 几乎所有 handler
**Issue:** 代码重复度高（每个 handler 都是 `return nethttp.HandlerFunc(func(...))` + 错误处理同质化）。可以抽出 `withJSON(body T, fn func(ctx, hostID, body) (any, error))` 之类的 middleware。非阻塞，但每加一个 endpoint 都要复制 30+ 行模板，长期维护成本高。
**Fix:** Phase 47 重构时引入 handler helper。
**Status:** deferred (Info；fix_scope=critical_warning，本轮不修)

---

_Reviewed: 2026-05-12_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
