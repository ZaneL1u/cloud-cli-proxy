---
phase: 46-api-ui
plan: 01
subsystem: controlplane/bypass-api
tags: [backend, http-api, bypass, audit, guardrails]
requires:
  - Phase 45 Repository（host_bypass_presets / host_bypass_rules / host_bypass_bindings / host_bypass_audit_log）
  - EventRecorder.RecordEvent（双轨制 Track 2）
  - AdminEgressIPStore.ListEgressIPs（护栏的代理 IP 冲突检查）
provides:
  - 11 个 Admin Bypass HTTP 端点（presets / rules / bindings / validate）
  - 5 硬 + 1 软护栏校验模块（包级 ValidateBypassRule）
  - 12 个 BYPASS_* 错误码常量（统一 `{"code","message"}` 响应）
  - writeBypassAuditLog 双轨审计 helper（audit_log + events.RecordEvent）
  - GetBypassRuleByID Repository 扩展（WARN-5 修复：Update / Delete 前 before 快照）
  - AdminBypassProxyIPProvider 接口（让规则 handler 复用 AdminEgressIPStore）
affects:
  - internal/controlplane/http/router.go（新增 11 路由 + 4 个 Dependencies 字段）
  - internal/controlplane/app/app.go（注入 Bypass 三个 Store 字段）
  - internal/store/repository/queries_bypass.go（+ GetBypassRuleByID 方法 + SQL 常量）
tech-stack:
  added: []
  patterns:
    - Store 接口子集 + stub 单测（不依赖 pgx 真实连接）
    - context.WithValue(ctxKeyUserID) 注入 actor + X-Forwarded-First-Token 取 actor_ip
    - 双轨审计（DB audit_log + EventRecorder）失败仅 Warn 不阻断主请求
    - Dry-run 端点（Validate）共享 ValidateBypassRule，绝不落库、绝不写审计
key-files:
  created:
    - internal/controlplane/http/bypass_validation.go
    - internal/controlplane/http/bypass_validation_test.go
    - internal/controlplane/http/bypass_audit_helper.go
    - internal/controlplane/http/admin_bypass_presets.go
    - internal/controlplane/http/admin_bypass_presets_test.go
    - internal/controlplane/http/admin_bypass_rules.go
    - internal/controlplane/http/admin_bypass_rules_test.go
    - internal/controlplane/http/admin_bypass_bindings.go
    - internal/controlplane/http/admin_bypass_bindings_test.go
  modified:
    - internal/controlplane/http/router.go
    - internal/controlplane/app/app.go
    - internal/store/repository/queries_bypass.go
    - internal/store/repository/queries_bypass_test.go
decisions:
  - WARN-1：当 domain_keyword 短词 + confirm_risky=true 通过时，audit_log.note 写入 `confirm_risky_accepted`，便于事后回查谁选择了接受风险
  - WARN-5：Update / Delete 之前必须先 GetBypassRuleByID 拿到 before 快照，audit_log.before 字段不允许为 nil；为此扩展 Phase 45 Repository 增加 GetBypassRuleByID 方法 + SQL 常量
  - is_system preset 双重防御：Repository 层 sentinel ErrSystemBypassPresetImmutable + SQL `WHERE is_system = FALSE`，handler 用 errors.Is 翻译成 HTTP 403 + BYPASS_PRESET_IMMUTABLE
  - 双轨制（CONTEXT.md L107）：写操作成功后串行调用 (1) InsertBypassAuditLog → (2) events.RecordEvent("bypass.<action>", metadata)；任一失败仅 Warn 日志，不阻断 HTTP 响应
  - Validate 端点是 dry-run：复用 ValidateBypassRule，但绝不调用 CreateBypassRule、绝不写 audit_log，只返回 `{valid, is_risky, code, message}`
  - AdminBypassProxyIPProvider 仅暴露 ListEgressIPs，让 rule handler 不依赖完整的 EgressIPStore；router 自动回退到 AdminEgressIPs，避免重复注入
metrics:
  duration_minutes: ~150
  tasks_completed: 5
  files_created: 9
  files_modified: 4
  test_subcases: 61  # validation 25 + presets 11 + rules 16 + bindings 11 + repository signatures 3 (含 GetBypassRuleByID)
  guardrails: 6  # 5 hard + 1 soft
  endpoints: 11
  error_codes: 12
  completed: 2026-05-12
---

# Phase 46 Plan 01: Bypass API 后端落地 Summary

实现 Cloud CLI Proxy v3.5 后台 Bypass 网络白名单的完整 HTTP 控制面：覆盖 Preset / Rule / Binding 三套 CRUD + Validate dry-run，落地 5 硬 1 软护栏 + 12 个统一错误码 + 双轨审计（host_bypass_audit_log 行级 diff + events.RecordEvent 异步事件流），并接通 Phase 45 Repository。

## 实现概览

| 模块 | 文件 | 行数 | 测试 |
| --- | --- | ---: | ---: |
| 护栏校验 + 错误码 | `bypass_validation.go` + `_test.go` | 231 + 109 | 24 + 1 = 25 |
| 双轨审计 helper | `bypass_audit_helper.go` | 135 | 复用各 handler 测试 |
| Preset CRUD | `admin_bypass_presets.go` + `_test.go` | 252 + 400 | 11 |
| Rule CRUD + Validate | `admin_bypass_rules.go` + `_test.go` | 333 + 468 | 16 |
| Binding CRUD | `admin_bypass_bindings.go` + `_test.go` | 149 + 307 | 11 |
| Repository GetBypassRuleByID | `queries_bypass.go` + `_test.go`（扩展） | +12 | +3 reflection lock |

合计新增/修改 ~2400 行（含测试），61 个独立子用例，全部 PASS。

## HTTP 端点矩阵

所有端点都挂在 `adminGuard = AuthMiddleware → RequireRole("admin")` 后；未带 Token 一律 401（由独立 `TestAdminBypassRouter401Unauthenticated` 锁定）。

### Preset（5 个）

| 方法 | 路径 | 成功状态 | 失败状态 |
| --- | --- | ---: | --- |
| GET | `/v1/admin/bypass/presets` | 200 | 500 |
| POST | `/v1/admin/bypass/presets` | 201 | 400 / 422（护栏）|
| GET | `/v1/admin/bypass/presets/{presetID}` | 200 | 404 `BYPASS_PRESET_NOT_FOUND` |
| PATCH | `/v1/admin/bypass/presets/{presetID}` | 200 | 403 `BYPASS_PRESET_IMMUTABLE` / 404 |
| DELETE | `/v1/admin/bypass/presets/{presetID}` | 204 | 403 / 404 |

### Rule（5 个）

| 方法 | 路径 | 成功状态 | 失败状态 |
| --- | --- | ---: | --- |
| GET | `/v1/admin/bypass/rules?host_id=` | 200 | 500 |
| POST | `/v1/admin/bypass/rules` | 201 | 400 / 422（5 硬 1 软护栏）|
| POST | `/v1/admin/bypass/rules/validate` | 200 / 422 | dry-run，不落库不审计 |
| PATCH | `/v1/admin/bypass/rules/{ruleID}` | 200 | 404 / 422 |
| DELETE | `/v1/admin/bypass/rules/{ruleID}` | 204 | 404 `BYPASS_RULE_NOT_FOUND` |

### Binding（3 个）

| 方法 | 路径 | 成功状态 | 失败状态 |
| --- | --- | ---: | --- |
| GET | `/v1/admin/hosts/{hostID}/bypass` | 200 | 400（缺 hostID）|
| POST | `/v1/admin/hosts/{hostID}/bypass` | 201 | 400（preset/rule 互斥或都缺）|
| DELETE | `/v1/admin/bypass/bindings/{bindingID}` | 204 | 404 `BYPASS_BINDING_NOT_FOUND` |

## 错误码常量（12 个）

定义于 `bypass_validation.go`，前端 / Plan 02 UI 通过 `code` 字段 i18n：

```
BYPASS_RULE_TOO_BROAD       // 0.0.0.0/0 / TLD / 过宽 CIDR
BYPASS_RULE_CONFLICT_PROXY  // 规则 IP / CIDR 覆盖代理 EgressIP
BYPASS_LIMIT_EXCEEDED       // 单 host 规则 ≥1000
BYPASS_KEYWORD_TOO_SHORT    // domain_keyword < 4 且无 confirm_risky
BYPASS_PRESET_IMMUTABLE     // is_system=true 预设禁改禁删
BYPASS_SNAPSHOT_CONFLICT
BYPASS_SNAPSHOT_NOT_FOUND
BYPASS_HOST_NOT_FOUND
BYPASS_PRESET_NOT_FOUND
BYPASS_RULE_NOT_FOUND
BYPASS_BINDING_NOT_FOUND
BYPASS_INVALID_REQUEST      // 通用 400
```

## 护栏（5 硬 + 1 软）

`ValidateBypassRule(ruleType, value, port, confirmRisky, proxyIPs, currentHostRuleCount)`：

1. **TOO_BROAD（硬）**：`cidr=0.0.0.0/0` / 私网 prefix < /16 / domain 命中 18 项 TLD 黑名单 → 422
2. **CONFLICT_PROXY（硬）**：rule.value 直接命中代理 EgressIP 或 CIDR 覆盖代理 IP → 422
3. **LIMIT_EXCEEDED（硬）**：单 host scope 规则数 ≥ 1000（Create 前先 ListBypassRules 计数）→ 422
4. **INVALID PORT（硬）**：port 解析失败（仅 `80` / `80-443` / 空 三种格式合法）→ 422
5. **DOMAIN_SUFFIX 太短（硬）**：`domain_suffix .com` 等 ≤ TLD 长度 → 422
6. **KEYWORD_TOO_SHORT（软）**：`domain_keyword` < 4 字符且 confirm_risky=false → 400；带 confirm_risky=true 允许保存（is_risky=true + audit note `confirm_risky_accepted`）

## 双轨审计实现（CONTEXT.md L107 锁定）

每个写端点（create / update / delete / bind / unbind）成功后串行调用 `writeBypassAuditLog`：

```go
// Track 1：落 host_bypass_audit_log（含 before/after JSONB diff、actor_id、actor_ip）
writer.InsertBypassAuditLog(ctx, repository.InsertBypassAuditLogParams{
    Action:     "create_rule",
    TargetKind: "rule",
    TargetID:   &ruleID,
    ActorID:    actorIDPtr(ctx),
    ActorIP:    extractActorIP(r), // X-Forwarded-For 首段 → X-Real-IP → RemoteAddr
    Before:     beforeJSON,
    After:      afterJSON,
    Note:       "confirm_risky_accepted", // 仅 WARN-1 场景
})

// Track 2：发 bypass.<action> 事件流（SSE 推 UI / 监控订阅）
events.RecordEvent(ctx, repository.RecordEventParams{
    Level:    "info",
    Type:     "bypass.create_rule",
    Message:  "Bypass create_rule",
    Metadata: map[string]any{"target_kind": ..., "target_id": ..., "actor_id": ...},
})
```

任一 Track 失败只记 `slog.Warn`，不阻断 HTTP 响应。

## 关键修复（保留在历史中）

| 标识 | 原始风险 | 修复方案 | 落地位置 |
| --- | --- | --- | --- |
| WARN-1 | confirm_risky 通过的短词审计无法回查谁同意了风险 | audit_log.note 写 `confirm_risky_accepted` | `admin_bypass_rules.go:169-172` |
| WARN-5 | Update / Delete 前没有 GetBypassRuleByID，audit_log.before=nil 导致 diff 不完整 | 先 GetBypassRuleByID 取 before 快照，pgx.ErrNoRows → 404 | `admin_bypass_rules.go:181-190` / `253-265` + Repository.GetBypassRuleByID |
| is_system 防御 | 仅靠 sentinel 易被绕过 | Repository SQL `WHERE is_system = FALSE` + sentinel `ErrSystemBypassPresetImmutable` + handler `errors.Is` → 403 | Phase 45 SQL 常量 + `admin_bypass_presets.go` |
| ProxyIP 注入 | rule handler 不应依赖完整 EgressIPStore | 引入 `AdminBypassProxyIPProvider`（仅 ListEgressIPs），router 自动回退到 AdminEgressIPs | `admin_bypass_rules.go:28-31` + `router.go` |

## Plan 02 / 03 衔接契约

- **错误码契约**：12 个 BYPASS_* 常量已落地，Plan 02（前端）按 `code` 字段做 i18n 文案映射；不允许新增错误码而不同步本文档
- **dry-run 端点**：`POST /v1/admin/bypass/rules/validate` 已就绪，Plan 02 UI 的「即时校验」按钮直接调它，返回结构 `{valid, is_risky, code, message}` 锁定
- **事件流**：Plan 03（SSE/监控）订阅 `bypass.*` 类型即可拿到所有写操作；payload metadata 包含 target_kind / target_id / actor_id
- **审计日志**：Plan 03 历史回放端点查询 `host_bypass_audit_log`，before/after 都是 JSONB，直接 unmarshal 即可显示 diff
- **快照 API（Plan 03）**：本 Plan 不实现 `/v1/admin/bypass/snapshots`，但 Repository 层已就绪（ListBypassSnapshotsByHost / CreateBypassSnapshot / UpdateBypassSnapshotStatus / GetLatestAppliedBypassSnapshot）

## 测试矩阵（61 个独立子用例）

| 文件 | 子用例数 | 覆盖 |
| --- | ---: | --- |
| `bypass_validation_test.go` | 24 + 1 | 5 硬护栏各 ≥3 case + 软护栏 confirm_risky 双向 + port 解析 + containsCIDR |
| `admin_bypass_presets_test.go` | 11 | List / Get404 / Create+audit / Create 422 / UpdateSysImmutable / Update404 / DeleteSysImmutable / Delete+audit / UpdateBeforeAfter / CreateNoActor / 401 |
| `admin_bypass_rules_test.go` | 16 | List 2 + Create 8（含 WARN-1 + 5 护栏）+ Update 2（含 WARN-5）+ Delete 2 + Validate 2 |
| `admin_bypass_bindings_test.go` | 11 | ListByHost 2 + Bind 6（含互斥校验）+ Unbind 2 + source 透传 + 非法 JSON |
| `queries_bypass_test.go`（扩展） | +3 | Repository 签名锁 + SQL 常量锁（含新增 getBypassRuleByIDSQL）|

```
go test ./internal/controlplane/http/ ./internal/store/repository/ -count=1
ok  github.com/zanel1u/cloud-cli-proxy/internal/controlplane/http  1.298s
ok  github.com/zanel1u/cloud-cli-proxy/internal/store/repository    0.551s
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] EventRecorder 签名漂移**
- **Found during:** Task 2（writeBypassAuditLog 实现）
- **Issue:** Plan 描述 `RecordEvent(ctx, kind string, payload any)`，但运行时实际签名是 `RecordEvent(ctx, repository.RecordEventParams) (repository.Event, error)`
- **Fix:** 对照 `admin_egress_ips.go` / `admin_bindings.go` 现有调用，统一使用 `RecordEventParams{Level, Type, Message, Metadata}`
- **Files modified:** `bypass_audit_helper.go`
- **Commit:** 07e98f8

**2. [Rule 2 - Missing Critical Functionality] WARN-5 audit_log.before 字段不能为 nil**
- **Found during:** Task 4（rules handler 落地）
- **Issue:** Plan 仅指明 Update / Delete 写 audit_log，但若不先 GetBypassRuleByID 拿 before 快照，diff 会缺一半，事后无法回查
- **Fix:** Repository 扩展 `GetBypassRuleByID(ctx, id) (BypassRule, error)` + SQL 常量；rule handler 在 Update / Delete 前调用，pgx.ErrNoRows → 404
- **Files modified:** `queries_bypass.go` + `queries_bypass_test.go`（签名锁更新）
- **Commit:** 22403c7

**3. [Rule 3 - Blocking Issue] AdminBypassProxyIPProvider 解耦**
- **Found during:** Task 4（rules handler 落地）
- **Issue:** rule handler 需要 proxy IP 列表做护栏，但要求 store 接口同时实现完整 EgressIPStore 会强耦合
- **Fix:** 新增最小接口 `AdminBypassProxyIPProvider { ListEgressIPs(ctx) ([]EgressIP, error) }`；router 自动用 AdminEgressIPs 兜底
- **Files modified:** `admin_bypass_rules.go` + `router.go`
- **Commit:** a38ab36 / 6ff8614

### 无其他偏离

未发生 Rule 4 级别（架构）变更；plan 任务范围、命名空间、双轨制设计、HTTP 状态码与错误码全部与原计划一致。

## Authentication Gates

无：所有变更落在已有的 `adminGuard = AuthMiddleware → RequireRole("admin")` 链路内，未引入新认证流。

## Known Stubs

无：所有路由真实接到 Repository，handler 真实执行护栏校验 + 落库 + 双轨审计，没有写死的占位响应。

## Threat Flags

无新增风险面：本 Plan 全部在 Phase 41 admin auth + Phase 45 Bypass schema 既有 trust boundary 内增加 CRUD；未引入新公网入口、未新增 file system 访问、未跨越 namespace 边界。

## Commit 序列

| 序号 | Hash | 内容 |
| ---: | --- | --- |
| 1 | 048ea1e | 护栏校验模块 + 12 错误码常量 |
| 2 | 07e98f8 | 双轨审计 helper |
| 3 | 22403c7 | Repository.GetBypassRuleByID 扩展（WARN-5） |
| 4 | bf1affb | Preset CRUD handler + 11 子用例 |
| 5 | a38ab36 | Rule CRUD + Validate + 5+1 护栏 + 16 子用例 |
| 6 | 6ff8614 | Binding CRUD + 路由注册 + 11 子用例 |

## Self-Check: PASSED

- internal/controlplane/http/bypass_validation.go  ✓
- internal/controlplane/http/bypass_validation_test.go  ✓
- internal/controlplane/http/bypass_audit_helper.go  ✓
- internal/controlplane/http/admin_bypass_presets.go  ✓
- internal/controlplane/http/admin_bypass_presets_test.go  ✓
- internal/controlplane/http/admin_bypass_rules.go  ✓
- internal/controlplane/http/admin_bypass_rules_test.go  ✓
- internal/controlplane/http/admin_bypass_bindings.go  ✓
- internal/controlplane/http/admin_bypass_bindings_test.go  ✓
- 6 个 commit hash 全部存在于 `git log --all`：048ea1e / 07e98f8 / 22403c7 / bf1affb / a38ab36 / 6ff8614  ✓
- `go build ./...` 通过；`go test ./... -count=1 -short` 全仓通过  ✓
