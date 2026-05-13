---
phase: 46-api-ui
verified: 2026-05-12T19:50:00Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
requirements_satisfied: 10/10
re_verification:
  previous_status: null  # 初次验证（46-REVIEW.md 已闭环 16 fixes）
---

# Phase 46: 控制面 API 与后台 UI 验证报告

**Phase Goal:** 让管理员能在后台对每个 host 完成「勾预设 → 加规则 → 预览 diff → 一键 apply / rollback」闭环，所有写操作经过护栏校验和审计日志，前端体验清晰且高风险动作有二次确认。

**Verified:** 2026-05-12T19:50:00Z
**Status:** passed
**Re-verification:** 否（初次目标倒推验证；46-REVIEW.md 已记录并修复 7 CR + 9 WR）

## Goal Achievement

### Observable Truths（来自 ROADMAP Success Criteria 5 条）

| # | Truth | Status | Evidence |
|---|------|--------|----------|
| 1 | JWT admin 可访问 `bypass/presets`、`bypass/rules`、`bypass/rules/validate`、`hosts/{hostID}/bypass` 全套 CRUD/绑定接口；`is_system=true` 预设 PATCH/DELETE 返回 403 | VERIFIED | `router.go:249-274` 注册 13 条 CRUD/binding 路由；`admin_bypass_presets.go` 在 Update/Delete 中 `errors.Is(err, repository.ErrSystemBypassPresetImmutable)` → 403 + `BYPASS_PRESET_IMMUTABLE`；测试 `admin_bypass_presets_test.go:400 行` 8 个子用例覆盖（含 update_immutable / delete_immutable） |
| 2 | preview 返回 cidrs/domains JSON + nft diff + 风险报告且不落库；apply 写 snapshot（含 config_hash 幂等）；rollback 能回滚到 applied snapshot；effective 返回当前生效全集 | VERIFIED | `admin_bypass_snapshots.go:636 行` 4 个端点全实现；`router.go:280-283` 注册；config_hash 通过 sha256(cidrsJSON+"\n"+domainsJSON) 计算（bypass_render.go）；UNIQUE 冲突走 isUniqueViolation(pgconn 23505) 幂等返回 200 且补 dispatch（CR-06 修复，line 326-346）；rollback 走 `crypto/rand` 8 字节 hex 后缀（WR-05）且 audit note 含 `rollback_target_snapshot_id`（line 538）；rollback 内不调 UpdateBypassSnapshotStatus（target 状态不变，WARN-4） |
| 3 | 5 硬 1 软护栏全部触发对应错误码：`BYPASS_RULE_TOO_BROAD` / `BYPASS_RULE_CONFLICT_PROXY` / `BYPASS_LIMIT_EXCEEDED` / `BYPASS_KEYWORD_TOO_SHORT`；HTTP 422/400 | VERIFIED | `bypass_validation.go:231 行` 注册 **12 个 BYPASS_\* 错误码**（11 个 must + 1 个 INVALID_REQUEST 补充）；`bypass_validation_test.go` ≥ 18 个 case；`admin_bypass_rules.go` 在 Create/Update/Validate 三处调 ValidateBypassRule 并翻译 HTTP 422/400；中文 i18n 在 `web/admin/src/lib/i18n/bypass-error-codes.ts:59 行` 提供（含 8 条 BYPASS_\* 中文文案 + `parseBypassError` helper） |
| 4 | host 详情页存在「代理白名单」Tab：loopback 强制锁定 + lan 可勾选；自定义规则 CRUD（5 类型）+ 高风险黄色徽章 + 二次确认；预览面板可切 sing-box JSON / nft diff 双视图 | VERIFIED | `$hostId.tsx:426` 渲染 `<BypassTab>`；`preset-card.tsx:75 forced={preset.is_force_on}`（WR-08 字段名修复 from `is_forced`）+ Lock icon + Tooltip 文案；`custom-rules-table.tsx` 5 类型 Select + IsRisky 行 `border-l-warning` + "高风险" Badge + 删除 AlertDialog；`bypass-rule-drawer.tsx` 480px Sheet + 5 类型 zod + RiskyKeywordConfirm（CR-05 移除 port 字段后 5 列）；`preview-sheet.tsx` 640px Sheet + Tabs 双视图（sing-box JSON / nft set diff） |
| 5 | apply 按钮分 5 阶段反馈（生成快照 / 下发到 agent / Reload / 健康检查 / 完成）；成功 toast 中文文案；失败显示错误码；所有写操作 audit_log 留 actor_id/actor_ip/before/after | VERIFIED | `apply-progress-dialog.tsx:20-26` 5 阶段中文常量；stageStatuses 按 `task.progress_percent` 0-25-50-75-100 映射（WARN-7 修复，进度对齐阶段失败位置）；成功 toast `"已应用 · 白名单变更不影响现有 TCP 连接，新连接才用新规则"`（line 152）；失败保持开启 + 错误码 + 关闭按钮；audit 双轨：`bypass_audit_helper.go` writeBypassAuditLog 同时落 InsertBypassAuditLog（actor_id / actor_ip / before / after / note JSONB）+ events.RecordEvent（CONTEXT.md L107 双轨制）—— 全 5 个 handler 文件累计 **10 处 writeBypassAuditLog 调用** |

**Score:** 5/5 truths verified（含 ROADMAP 5 条 success criteria）

### Required Artifacts

| Artifact | Expected | Lines | Status | Notes |
|----------|---------|-------|--------|-------|
| `internal/controlplane/http/bypass_validation.go` | 护栏 + 错误码 ≥ 200 行 | 231 | VERIFIED | 12 个 BYPASS_\* 错误码 + ValidateBypassRule + TLD 黑名单 + 私有段白名单 |
| `internal/controlplane/http/bypass_audit_helper.go` | 双轨 helper ≥ 80 行 | 139 | VERIFIED | extractActorIP / actorIDPtr / writeBypassAuditLog（audit_log + RecordEvent 同步双轨；WR-03 修正了注释中"异步"误导描述） |
| `internal/controlplane/http/admin_bypass_presets.go` | List/Get/Create/Update/Delete ≥ 200 行 | 252 | VERIFIED | 5 端点 + is_system 双层防御 + 测试 8 个子用例 |
| `internal/controlplane/http/admin_bypass_rules.go` | CRUD + Validate ≥ 200 行 | 333 | VERIFIED | 5 端点 + ValidateBypassRule + ListEgressIPs（proxy 冲突）+ GetBypassRuleByID（WR-5 audit before）+ confirm_risky_accepted note（WR-1） |
| `internal/controlplane/http/admin_bypass_bindings.go` | ListByHost/Bind/Unbind ≥ 130 行 | 149 | VERIFIED | 3 端点 + preset/rule XOR 校验 |
| `internal/controlplane/http/admin_bypass_snapshots.go` | preview/apply/rollback/effective ≥ 280 行 | 636 | VERIFIED | 4 端点 + rollback 不破坏 target（WARN-4）+ SQLSTATE 23505（WR-4）+ rollback hash suffix `crypto/rand` 8 字节 hex（WR-5）+ dispatch_failed audit（WR-2）+ 幂等路径补 dispatch（CR-6） |
| `internal/controlplane/http/admin_bypass_audit_log.go` | cursor 分页 ≥ 80 行 | 140 | VERIFIED | ListByHost + limit clamp（default 20, max 200） + before 过滤 |
| `internal/controlplane/http/bypass_render.go` | 渲染层 ≥ 200 行 | 275 | VERIFIED | RenderBypassConfig 纯函数 + sha256 config_hash + nft diff unified |
| `internal/agentapi/contracts.go` | ActionReloadHostBypass | - | VERIFIED | line 14 `ActionReloadHostBypass HostAction = "reload_host_bypass"` |
| `internal/runtime/tasks/worker.go` | dispatch 占位 | line 101-109 | VERIFIED | case 命中后写日志含字面 `"Phase 46 placeholder; no-op until Phase 47"` 且返回 nil |
| `internal/store/migrations/0019_host_bypass_rules.sql` | 5 表 + seed | 104 | VERIFIED | preset/rule/binding/snapshot/audit_log + loopback / lan seed |
| `internal/store/migrations/0020_host_bypass_snapshot_source.sql` | source 列 | 27 | VERIFIED | ALTER TABLE ADD source TEXT + CHECK ('apply','rollback') |
| `web/admin/src/components/bypass/*` | 11 个 .tsx + 6 个 __tests__ | 11+6 | VERIFIED | bypass-tab / preset-card / preset-grid / custom-rules-table / bypass-rule-drawer / risky-keyword-confirm / preview-sheet / apply-progress-dialog / rollback-confirm-dialog / json-viewer / nft-diff-viewer + 6 test files |
| `web/admin/src/hooks/use-bypass-*.ts` | 4 个 hooks | 178 行 | VERIFIED | presets / rules / bindings / snapshots（含 preview/apply/rollback/effective/auditLog） |
| `web/admin/src/lib/api/bypass.ts` | API client | 126 | VERIFIED | 14+ 个 endpoint 调用 |
| `web/admin/src/lib/api/types/bypass.ts` | 类型对齐 | 130 | VERIFIED | port 字段已移除（CR-05）|
| `web/admin/src/lib/i18n/bypass-error-codes.ts` | 错误码 → 中文 | 59 | VERIFIED | 8 个核心 BYPASS_\* 中文映射 + parseBypassError helper（实际命名 `bypass-error-codes.ts` 而非 plan 中的 `error-codes.ts`，命名差异不影响功能） |

### Key Link Verification

| From | To | Via | Status |
|------|-----|-----|--------|
| router.go | NewAdminBypassPresetsHandler / RulesHandler / BindingsHandler / SnapshotsHandler / AuditLogHandler | 5 个工厂函数 + adminGuard | VERIFIED（router.go:249-287 命中 18 条 bypass 路由） |
| handler 写操作 | writeBypassAuditLog | bypass_audit_helper.go 双轨 helper | VERIFIED（admin_bypass_*.go 累计 10 次调用） |
| Apply handler | worker dispatch | h.actions.QueueHostAction(ctx, hostID, ActionReloadHostBypass, snap.ID) | VERIFIED（apply path + idempotent path 都 dispatch） |
| rollback handler | 新 snapshot 行 | CreateBypassSnapshot(Source="rollback") + 不调 UpdateBypassSnapshotStatus(target) | VERIFIED（target.AppliedStatus 保持不变） |
| 前端 listBypassRules | GET /v1/admin/bypass/rules?host_id={id} | `web/admin/src/lib/api/bypass.ts:22` | VERIFIED（CR-01 修复） |
| 前端 createBypassRule | POST /v1/admin/bypass/rules with scope/host_id | bypass.ts:28 | VERIFIED（CR-02 修复） |
| 前端 update/deleteBypassRule | PATCH/DELETE /v1/admin/bypass/rules/{id} | bypass.ts:40/55 | VERIFIED（CR-03 修复） |
| 前端 bindings | GET/POST /hosts/{id}/bypass, DELETE /bypass/bindings/{id} | bypass.ts:66/72/82 | VERIFIED（CR-04 修复） |
| 前端 SSE | useSSE(buildSSEUrl("/v1/admin/sse", topic, token)) + adminGuard on server | sse-manager.ts + auth_middleware.go(?token=) | VERIFIED（CR-07 修复：admin + user SSE 都套 guard，token query 支持） |
| BypassTab | PreviewSheet + ApplyProgressDialog | bypass-tab.tsx 接入 | VERIFIED |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Real Data? | Status |
|----------|--------------|--------|------------|--------|
| BypassTab | presets / bindings / rules | useBypassPresets / useBypassBindings / useBypassRules → bypassApi.* → /v1/admin/bypass/* → repository.List* | 是（pg 查询） | FLOWING |
| PreviewSheet | preview | usePreviewBypass → POST /preview → admin_bypass_snapshots.go:Preview → collectRenderInput(实际查 binding/preset/rule) → RenderBypassConfig | 是 | FLOWING |
| ApplyProgressDialog | task / stageStatuses | useApplyBypass → POST /apply → admin_bypass_snapshots.go:Apply → CreateBypassSnapshot + QueueHostAction → task.progress_percent | 是（worker placeholder 会立即标 succeeded，5 阶段瞬间 done —— Phase 47 接管后产生真实进度） | FLOWING（已知占位行为，下一阶段 Phase 47 接管） |
| CustomRulesTable | rules | useBypassRules(hostId) → GET /v1/admin/bypass/rules?host_id= → ListBypassRules(ctx, &hostID) | 是 | FLOWING |
| PresetGrid | presets / bindings | useBypassPresets + useBypassBindings(hostId) | 是 | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| 后端编译 | `go build ./internal/...` | exit 0 | PASS |
| 后端 controlplane http 测试套件 | `go test ./internal/controlplane/http/ -count=1 -short` | `ok 0.824s` | PASS |
| 前端 bypass 测试套件 | `pnpm test:unit -- --run src/components/bypass/` | `Test Files 8 passed, Tests 34 passed` | PASS |
| 前端生产构建 | `pnpm build` | `built in 314ms, 840.37 kB bundle` | PASS |
| router 路由计数 | `grep -c "/v1/admin/bypass\|/v1/admin/hosts/{hostID}/bypass" router.go` | 18 | PASS |
| writeBypassAuditLog 调用计数 | 5 handler 文件累计 | 10 | PASS（≥ 7 阈值） |
| 错误码常量计数 | bypass_validation.go | 12 个 BYPASS_\* | PASS（≥ 11 阈值） |
| Worker placeholder 字面 | `grep "Phase 46 placeholder; no-op until Phase 47" worker.go` | 命中 1 次（line 109） | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| BYPASS-API-01 | 46-01 | preset/rule/binding 三套 CRUD API | SATISFIED | router.go:249-274 注册 13 条；handler 测试 ≥ 26 个子用例 |
| BYPASS-API-02 | 46-02 | preview/apply/rollback 三核心接口 | SATISFIED | admin_bypass_snapshots.go + router.go:280-283 |
| BYPASS-API-03 | 46-01 | 护栏硬拦截（422 + 3 错误码） | SATISFIED | bypass_validation.go ValidateBypassRule + rules handler 翻译 422 |
| BYPASS-API-04 | 46-01 | domain_keyword 软拦截（400 + confirm_risky） | SATISFIED | ValidateBypassRule 软拦截路径 + audit note "confirm_risky_accepted"（WR-1） |
| BYPASS-API-05 | 46-01 + 46-02 | 写操作落 audit_log（actor_id/ip/action/before/after JSON/note） | SATISFIED | writeBypassAuditLog 双轨 helper + 10 处调用点（5 handler + apply/rollback） |
| BYPASS-UI-01 | 46-03 | host 详情页「代理白名单」Tab + shadcn 风格 | SATISFIED | $hostId.tsx:426 渲染 BypassTab + Tabs primitive |
| BYPASS-UI-02 | 46-03 | 预设多选卡片（loopback 锁定 / lan 勾选）+ 悬浮规则示例 | SATISFIED | preset-card.tsx forced + Tooltip + preset-grid.tsx 3 列 |
| BYPASS-UI-03 | 46-03 | 自定义规则 CRUD（5 类型）+ 高风险徽章 + 二次确认 | SATISFIED | custom-rules-table.tsx 5 类型 Select + warning border + Badge + AlertDialog；bypass-rule-drawer.tsx + RiskyKeywordConfirm |
| BYPASS-UI-04 | 46-04 | 预览面板 + sing-box JSON / nft diff 双视图 | SATISFIED | preview-sheet.tsx 640px Sheet + Tabs 双视图 |
| BYPASS-UI-05 | 46-04 | 应用按钮分 5 阶段反馈 + 成功 toast + 失败错误码 | SATISFIED | apply-progress-dialog.tsx STAGES 5 个中文阶段 + 成功 toast 字面 + 失败错误码展示 |

**覆盖率：10/10 requirement IDs satisfied，无 orphaned。**

### Anti-Patterns Scan

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| custom-rules-table.tsx | 5 列（无 port），与 plan 03 declared 6 列存在差异 | Info | CR-05 修复决策：后端 BypassRule 模型无 port 字段，前端移除 port 显示；plan 文本未同步更新 success_criteria 文本但语义一致 |
| use-hosts.ts | `toast.success("配置导入成功")` 缺 import（typecheck error） | Info | 预先存在，commit `a892af4` 引入（与 Phase 46 无关）；不影响 Phase 46 功能 |
| 其他 egress-ips 类 typecheck error | resolver / hosts.hosts 字段缺失 | Info | 全部预先存在，与 Phase 46 无关；不影响 Phase 46 build / runtime |

未发现 Phase 46 引入的 BLOCKER 或 WARNING 级别反模式。

### Human Verification Required

（空 —— 所有 must-haves 都已通过自动化方式验证；下游 Phase 47 会自然驱动 ApplyProgressDialog 真实阶段切换的 UX 验证）

## Gap Summary

无 Gap。Phase 46 ROADMAP 5 条 success criteria 全部 VERIFIED：

1. **API 全套 CRUD/绑定接口 + is_system 403** —— 18 条路由全注册，handler 双层防御 + 12 个错误码 + 26+ 个 handler 测试子用例
2. **preview/apply/rollback/effective 闭环 + config_hash 幂等 + rollback 不破坏 target** —— config_hash 走 sha256 + UNIQUE(host_id, config_hash) DB 兜底 + SQLSTATE 23505 严格识别（WR-4）+ 幂等路径补 dispatch（CR-6）；rollback 新建 source='rollback' snapshot 行 + crypto/rand 8 字节 hex 后缀绕开 UNIQUE 冲突（WR-5）+ 不调 UpdateBypassSnapshotStatus（WARN-4）+ audit note 含 rollback_target_snapshot_id
3. **5 硬 1 软护栏 + 12 个 BYPASS_\* 错误码 + i18n 中文** —— ValidateBypassRule 18+ test cases，前端 parseBypassError + bypass-error-codes.ts 8 条核心中文文案
4. **host 详情页 BypassTab + 预设卡片 + 自定义规则表 + Drawer + 二次确认** —— preset is_force_on 字段名对齐（WR-8）+ Lock icon + Tooltip + 5 类型 zod + RiskyKeywordConfirm 复选框 ack 才能保存
5. **ApplyProgressDialog 5 阶段 + audit 双轨** —— stageStatuses 按 task.progress_percent 4 档映射（WR-7）+ 成功 toast 中文 + 失败保持开启 + 错误码展示 + writeBypassAuditLog 同时落 audit_log + events.RecordEvent

Code Review 16 个 critical+warning 全部修复（commit 1003efd / cfe703c / 7434739 / 1be580a / 36b3f8f / bc0ebf6 / c5251ee / 317a834 / 958bd25 / f393ea9 / 396ab82 / 69402fb / 3de1446 / 52f3e38 / 88d0b35 / 3fb8fec），5 个 Info 留为 deferred（IN-01 ~ IN-05），与 Phase 47 工作合并。

Worker dispatch 占位行为已对齐 truth #6（"reload_host_bypass dispatched (Phase 46 placeholder; no-op until Phase 47)"），Phase 47 在此基础上把日志替换为真实 reload 即可，不需要再改 Phase 46 契约。

---

_Verified: 2026-05-12_
_Verifier: Claude (gsd-verifier)_
