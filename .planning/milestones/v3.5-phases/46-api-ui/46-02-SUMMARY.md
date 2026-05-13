---
phase: 46
plan: 02
subsystem: bypass-api
tags: [bypass, snapshots, rollback, audit-log, sing-box, render]
requires:
  - Phase 45 sing-box rule-set placeholder (version=3 schema)
  - Phase 46 Plan 01 bypass binding/preset/rule CRUD
  - Phase 46 Plan 01 writeBypassAuditLog 双轨审计辅助
provides:
  - POST /v1/admin/hosts/{hostID}/bypass/preview
  - POST /v1/admin/hosts/{hostID}/bypass/apply
  - POST /v1/admin/hosts/{hostID}/bypass/rollback
  - GET  /v1/admin/hosts/{hostID}/bypass/effective
  - GET  /v1/admin/hosts/{hostID}/bypass/audit-log
  - RenderBypassConfig(纯函数渲染层)
  - ActionReloadHostBypass(worker dispatch 占位)
  - host_bypass_snapshots.source 列 + GetBypassSnapshotByID 仓库方法
affects:
  - Phase 47 host-agent 将接管 ActionReloadHostBypass 的真实 reload
  - 后续 UI Tab 「白名单/快照/历史」可直接调用上述 5 个端点
tech-stack:
  added:
    - 无新依赖（沿用 Go 标准库 net/http + crypto/sha256 + encoding/json）
  patterns:
    - sing-box rule-set source format v3 envelope（{"version":3,"rules":[...]}）
    - sha256(cidrsJSON + "\n" + domainsJSON) hex = ConfigHash 幂等键
    - rollback 创建新 snapshot 行（source='rollback'）而非修改 target 状态
    - cursor 分页（before=RFC3339 + limit clamp 默认 20 / 最大 200）
    - 双轨审计（host_bypass_audit_log 行 + EventRecorder.RecordEvent 事件流）
key-files:
  created:
    - internal/controlplane/http/bypass_render.go
    - internal/controlplane/http/bypass_render_test.go
    - internal/controlplane/http/admin_bypass_snapshots.go
    - internal/controlplane/http/admin_bypass_snapshots_test.go
    - internal/controlplane/http/admin_bypass_audit_log.go
    - internal/controlplane/http/admin_bypass_audit_log_test.go
    - internal/store/migrations/0020_host_bypass_snapshot_source.sql
  modified:
    - internal/agentapi/contracts.go（ActionReloadHostBypass 常量）
    - internal/runtime/tasks/worker.go（dispatch case 占位）
    - internal/store/repository/models.go（BypassSnapshot.Source 字段）
    - internal/store/repository/queries_bypass.go（source 列 + GetBypassSnapshotByID）
    - internal/controlplane/http/router.go（Dependencies 字段 + 5 路由）
    - internal/controlplane/app/app.go（wire snapshot/audit-log store）
decisions:
  - WARN-4：rollback 不修改 target snapshot 的 applied_status；新建 source='rollback' pending 行；Phase 47 worker 决定状态切换
  - WARN-3：ActionReloadHostBypass 占位只写日志 "Phase 46 placeholder; no-op until Phase 47"，不做任何系统调用
  - 跨 host 访问 target snapshot 返回 404 而非 403，避免存在性泄露
  - audit-log 在 handler 内存做 before cursor 过滤（Repository 已 created_at DESC 排序）
  - schema gap 修复优先：补 migration 0020 增加 source 列，而非沿用计划里 config_hash 后缀 hack
metrics:
  duration: "约 1 个执行 wave"
  completed: "2026-05-12T10:38:21Z"
---

# Phase 46 Plan 02: Snapshot / Apply / Rollback / Effective / Audit-Log 后台 API Summary

落地 v3.5 后台 API 「快照与动作半区」5 个 admin 端点 + 纯函数渲染层 + worker dispatch 占位；
Apply/Rollback 双轨审计，Rollback 通过新建 `source='rollback'` snapshot 行保留历史，全部
满足 Plan 中 WARN-3/WARN-4 验收要求。

## 一句话总结

为单 host bypass 白名单提供 preview/apply/rollback/effective/audit-log 五端点，渲染层产
出 byte-identical sing-box rule-set 与 sha256 ConfigHash，rollback 不破坏 target 历史。

## 实现要点

### Task 1: Worker dispatch 占位（commit 77fe637）

- `internal/agentapi/contracts.go` 新增 `ActionReloadHostBypass HostAction = "reload_host_bypass"`
- `internal/runtime/tasks/worker.go` Execute switch case 仅 `slog.Info("reload_host_bypass dispatched (Phase 46 placeholder; no-op until Phase 47)", ...)`，`err = nil`
- 日志字面量严格匹配 WARN-3 acceptance：`grep -c "Phase 46 placeholder; no-op until Phase 47" internal/runtime/tasks/worker.go` ≥ 1

### Task 1.5: Schema 补救（commit 2ebf8c0 — Rule 3 Deviation）

- 发现 Phase 45 Plan 03 SUMMARY 声称已添加 `host_bypass_snapshots.source`，但实际 migration 0019 未建该列；模型也无该字段
- 解决：新建 `migrations/0020_host_bypass_snapshot_source.sql`（DO $ ... END $ 保护 re-run 幂等）
  追加 `source TEXT DEFAULT 'apply'` + CHECK 约束（'apply' / 'rollback'）
- `models.go` BypassSnapshot / CreateBypassSnapshotParams 增加 Source 字段
- `queries_bypass.go` 4 个 SQL 常量补 source 列；scanBypassSnapshot 扫描；新增 `GetBypassSnapshotByID`
- 避免了原计划「config_hash 后缀 hack」的 UNIQUE 冲突回退路径

### Task 2: RenderBypassConfig 纯函数（commit c6a57db）

- `bypass_render.go` ~290 行，主类型：BypassRenderInput / BypassRenderOutput / ruleSetCIDRBucket / ruleSetDomainBucket / ruleSetEnvelope
- ip → /32 cidr，domain/domain_suffix/domain_keyword 各占独立 rule 块
- `sortedKeys` 字典序保证 deterministic JSON 输出
- `sha256(cidrsJSON + "\n" + domainsJSON)` hex = ConfigHash
- `renderNftDiff` 输出 `+ <cidr>` / `- <cidr>` 给前端预览
- 8 个子测试覆盖：空输入、单 cidr、domain+suffix 分块、order-stable hash、prev=nil 全增、prev 删除+新增、RiskyCount、preset rules

### Task 3: AdminBypassSnapshotsHandler（commit c85304f）

- `admin_bypass_snapshots.go` ~480 行（4 个 handler + collectRenderInput + nextSnapshotVersion + findSnapshotByConfigHash + isUniqueViolation）
- Preview：渲染并返回 JSON，不落库
- Apply：CreateBypassSnapshot(status=pending) → dispatch ActionReloadHostBypass → writeBypassAuditLog；UNIQUE(host_id,config_hash) 冲突走查询既存 snapshot 的幂等路径，**不重复写审计也不重复 dispatch**
- Rollback：GetBypassSnapshotByID(target) 校验 host_id 匹配 + applied_status='applied'；与当前 latest applied 一致时返回幂等 200；否则 CreateBypassSnapshot(source='rollback', status=pending) → dispatch with **new snap.ID** → 审计 note 前缀 `rollback_target_snapshot_id=<id>`；**全程不调 UpdateBypassSnapshotStatus(target.ID, ...)**（WARN-4）
- Effective：返回 4 段（presets_active / rules_active / cidrs_rendered / domains_rendered）
- 跨 host 访问返回 404 而非 403（不暴露存在性）
- 12 个子测试覆盖：preview 200/404、apply 成功/idempotent/hash 一致、rollback 成功/404 missing/404 cross-host/409 not-applied/200 idempotent、effective 成功/404；stub.updateSnapshotStatusCalls 始终为空验证 WARN-4

### Task 4: AdminBypassAuditLogHandler（commit 2c1ea07）

- `admin_bypass_audit_log.go` ~140 行
- query 参数：`limit`（默认 20，> 200 clamp 至 200）+ `before`（RFC3339 cursor）+ `target_kind`/`target_id`（可选覆盖）
- Repository 已 `created_at DESC`，handler 在内存过滤 `created_at < before` 后按 limit 截断
- 响应：`{audit_log: [...], next_before: "<iso>"}`；不足 limit 时 `next_before=""` 表示末页
- `audit_log` 字段保证非 nil 切片，避免 JSON null
- 7 个子测试覆盖默认 limit、before 过滤、limit clamp、空结果、invalid limit/before、刚好 == limit 时 next_before

### Task 5: 路由注册 + wire deps（commit b5329a1）

- `router.go` Dependencies struct 新增 `AdminBypassSnapshots AdminBypassSnapshotStore` + `AdminBypassAuditLog AdminBypassAuditLogStore`
- 注册 5 条路由：POST preview / POST apply / POST rollback / GET effective / GET audit-log
- `app/app.go` wire `*Repository` 到这两个字段（编译器担保接口满足）

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] 补救 host_bypass_snapshots.source 列缺失**

- **Found during:** Task 3 开始实现 rollback 时，发现 `BypassSnapshot` 模型无 Source 字段
- **Issue:** Phase 45 Plan 03 SUMMARY 声称 source 列存在，但 migration 0019 实际未创建；rollback 需 `source='rollback'` 否则无法满足 WARN-4 设计
- **Fix:** 新建 migration 0020（带 DO $ ... END $ 防重入），同步更新 models.go + queries_bypass.go 4 处 SQL + scanner + 新增 GetBypassSnapshotByID
- **Files modified:** internal/store/migrations/0020_host_bypass_snapshot_source.sql, internal/store/repository/models.go, internal/store/repository/queries_bypass.go
- **Commit:** 2ebf8c0

**未自动处理的：** Plan 中描述的「config_hash 后缀 hack 作为 UNIQUE 冲突回退」仍保留在代码里
作为兜底（仅极端竞态触发），主路径已不需要它。

## 验证结果

```text
go test ./internal/controlplane/http/ -count=1 -short            => ok
go test ./internal/controlplane/http/ -run 'TestRenderBypassConfig|TestAdminBypassSnapshotsHandler|TestAdminBypassAuditLogHandler' -v
  TestRenderBypassConfig             8/8 PASS
  TestAdminBypassSnapshotsHandler   12/12 PASS
  TestAdminBypassAuditLogHandler     7/7 PASS
go build ./internal/controlplane/... ./internal/agentapi/... ./internal/runtime/...  => ok
go test ./internal/agentapi/ ./internal/runtime/tasks/ -count=1 -short                 => ok
```

### Grep 接收标准

| 检查项 | 期望 | 实际 |
| ------ | ---- | ---- |
| `/v1/admin/hosts/{hostID}/bypass/` 在 router.go | ≥ 5 | 5 |
| AdminBypass* 字段 wire 在 app.go | ≥ 5 | 5 |
| ActionReloadHostBypass 出现次数 | ≥ 3 | 11 |
| WARN-3 字面量 "Phase 46 placeholder; no-op until Phase 47" | ≥ 1 | 2 |
| BypassRender/RenderBypassConfig 出现文件数 | ≥ 4 | 4 |
| audit-log next_before/before= 出现次数 | ≥ 2 | 4 |
| audit-log limit 出现次数 | ≥ 3 | 16 |

## Known Stubs

- `internal/runtime/tasks/worker.go` 的 `ActionReloadHostBypass` case 是 **Phase 46 计划内**的 dispatch 占位，仅写日志；Phase 47 host-agent 接管真实 reload。这是计划文档里明确声明的 WARN-3 设计，并非未连线 UI。
- 无前端组件 stub —— 本计划仅交付后端 API 与渲染层。

## Threat Flags

无新发现的威胁面。所有变更都在 Plan 的 `<threat_model>` 已识别范围内（T-46-01/02/04/04b/06/09/10/11 均已 mitigate）。

## 提交清单

| Task | 描述 | Commit |
| ---- | ---- | ------ |
| 1 | ActionReloadHostBypass + worker placeholder dispatch | 77fe637 |
| 1.5 (Rule 3) | host_bypass_snapshots.source 列 + GetBypassSnapshotByID | 2ebf8c0 |
| 2 | RenderBypassConfig 纯函数 + 8 子测试 | c6a57db |
| 3 | AdminBypassSnapshotsHandler + 12 子测试（含 WARN-4 验证） | c85304f |
| 4 | AdminBypassAuditLogHandler + cursor 分页 + 7 子测试 | 2c1ea07 |
| 5 | 路由注册 + wire deps | b5329a1 |

## Self-Check: PASSED

- 8 个声明文件全部存在（bypass_render.go/test, admin_bypass_snapshots.go/test, admin_bypass_audit_log.go/test, migration 0020, SUMMARY 自身）
- 6 个声明 commit hash 全部存在于 git history（77fe637 / 2ebf8c0 / c6a57db / c85304f / 2c1ea07 / b5329a1）
