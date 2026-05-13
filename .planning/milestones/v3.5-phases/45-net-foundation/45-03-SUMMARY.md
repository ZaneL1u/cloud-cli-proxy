---
phase: 45-net-foundation
plan: 03
subsystem: store/repository
tags: [bypass, whitelist, migrations, repository, pgx, data-foundation]
requirements:
  - BYPASS-DATA-01
  - BYPASS-DATA-02
  - BYPASS-DATA-03
  - BYPASS-DATA-04
dependency_graph:
  requires: []
  provides:
    - host_bypass_presets table（含 loopback / lan 两条系统预设 seed）
    - host_bypass_rules table（global / host scope，XOR 约束）
    - host_bypass_bindings table（preset_id 与 rule_id XOR 绑定）
    - host_bypass_snapshots table（version + config_hash 幂等键、applied_status 四态枚举）
    - host_bypass_audit_log table（actor / target / before/after JSONB）
    - Repository 19 个 Bypass* 方法 + 5 个领域类型 + 9 个 *Params + ErrSystemBypassPresetImmutable sentinel
    - 20 个包级 SQL 常量（lock 在 queries_bypass_test.go 的文本断言下）
  affects:
    - Phase 46 admin API（将基于 ErrSystemBypassPresetImmutable / 19 个方法签名直接调用）
    - Phase 47 apply / rollback 链路（依赖 snapshots.version DESC + UNIQUE(host_id, config_hash)）
tech_stack:
  added:
    - PostgreSQL 18.x JSONB CHECK 约束 + UUID 主键 + TIMESTAMPTZ DEFAULT NOW
    - pgx v5 RETURNING 扫描 + json.RawMessage 透传
  patterns:
    - is_system 双层防御：Go 层先查 + SQL `AND is_system = FALSE` WHERE 子句兜底
    - W4 拆分：Task 2a 先 lock 签名 + SQL（panic stub）→ Task 3 反射 + 文本断言 → Task 2b 填方法体
    - 可空 UUID 用 `*string` + nil→any 占位，让 pgx 使用 COALESCE 不改原列
    - JSONB 默认值兜底（`[]` / `{"version":3,"rules":[]}`），避免 NOT NULL 列空写入
key_files:
  created:
    - internal/store/migrations/0019_host_bypass_rules.sql（104 行）
    - internal/store/repository/migration_0019_test.go（125 行）
    - internal/store/repository/queries_bypass.go（592 行）
    - internal/store/repository/queries_bypass_test.go（122 行）
  modified:
    - internal/store/repository/models.go（追加 Phase 45 Plan 03 类型段，约 137 行）
decisions:
  - 用 TEXT + CHECK 约束代替 PostgreSQL ENUM 类型，对齐项目历史（RESEARCH §Anti-Patterns）
  - migration 0019 不写 down 段，仅在顶部注释中给出运维参考的 DROP TABLE 串
  - 系统预设 seed 用 ON CONFLICT (slug) DO NOTHING 保证 migration 可重放
  - is_system 拦截做 Go 层 + SQL 层双重防御；不依赖前端校验
  - InsertBypassAuditLog 只返回 id，created_at 留给数据库默认 NOW()
metrics:
  duration: 单 wave 内顺序完成
  tasks_completed: 4
  files_changed: 5（4 新建 + 1 扩展）
  loc_added: ~1080
  tests_added: 6（3 migration + 3 repository）
  date: 2026-05-12
---

# Phase 45 Plan 03: 网络配置基础与数据模型 Summary

把 v3.5 网络白名单 / 绕过规则的数据基础设施落到 Postgres + Go Repository，五张表 + 两条系统预设 seed + 19 个 CRUD 方法 + is_system 双层防御一次成型，为 Phase 46 admin API 与 Phase 47 apply 链路提供稳定接口契约。

## 范围一句话

落地 `host_bypass_presets` / `host_bypass_rules` / `host_bypass_bindings` / `host_bypass_snapshots` / `host_bypass_audit_log` 五张表（含 loopback / lan 系统预设 seed），并在 Repository 层暴露 19 个 Bypass* 方法 + ErrSystemBypassPresetImmutable sentinel，所有签名与 SQL 文本被反射 + 文本断言锁定。

## 完成的任务

| Task | 描述 | Commit | 关键文件 |
| ---- | --- | ------ | -------- |
| 1 | 写入 migration 0019（5 表 + 2 seed + 索引 + CHECK + UNIQUE）+ 3 个 migration 文本断言测试 | `ca81807` | `migrations/0019_host_bypass_rules.sql`、`migration_0019_test.go` |
| 2a | 在 `models.go` 追加 5 个实体 + 9 个 *Params；新建 `queries_bypass.go` 放 20 个 SQL 常量 + 19 个 panic stub + ErrSystemBypassPresetImmutable | `597c540` | `models.go`、`queries_bypass.go` |
| 3 | 写 `queries_bypass_test.go`：反射锁定 19 个方法签名 + 21 个 SQL 常量文本断言 + sentinel 文案断言 | `fc18ea9` | `queries_bypass_test.go` |
| 2b | 把 19 个 panic stub 替换为真实 pgx v5 实现（4 个 scanBypass* helper + is_system 双层拦截 + COALESCE 部分更新） | `db42bb7` | `queries_bypass.go` |

## 19 个 Repository 方法清单

签名顺序与 `queries_bypass_test.go::TestBypassRepository_Signatures` 中的 `expected` 列表严格对齐；Phase 46 handler 可按签名直接调用。

| # | 方法 | 签名 |
| - | ---- | ---- |
| 1 | `ListBypassPresets` | `(ctx) -> ([]BypassPreset, error)` |
| 2 | `GetBypassPresetBySlug` | `(ctx, slug string) -> (BypassPreset, error)` |
| 3 | `GetBypassPresetByID` | `(ctx, id string) -> (BypassPreset, error)` |
| 4 | `CreateBypassPreset` | `(ctx, params CreateBypassPresetParams) -> (BypassPreset, error)` |
| 5 | `UpdateBypassPreset` | `(ctx, id string, params UpdateBypassPresetParams) -> (BypassPreset, error)` |
| 6 | `DeleteBypassPreset` | `(ctx, id string) -> error` |
| 7 | `ListBypassRules` | `(ctx, hostID *string) -> ([]BypassRule, error)` |
| 8 | `CreateBypassRule` | `(ctx, params CreateBypassRuleParams) -> (BypassRule, error)` |
| 9 | `UpdateBypassRule` | `(ctx, id string, params UpdateBypassRuleParams) -> (BypassRule, error)` |
| 10 | `DeleteBypassRule` | `(ctx, id string) -> error` |
| 11 | `ListBypassBindingsByHost` | `(ctx, hostID string) -> ([]BypassBinding, error)` |
| 12 | `CreateBypassBinding` | `(ctx, params CreateBypassBindingParams) -> (BypassBinding, error)` |
| 13 | `DeleteBypassBinding` | `(ctx, id string) -> error` |
| 14 | `ListBypassSnapshotsByHost` | `(ctx, hostID string, limit int) -> ([]BypassSnapshot, error)` |
| 15 | `CreateBypassSnapshot` | `(ctx, params CreateBypassSnapshotParams) -> (BypassSnapshot, error)` |
| 16 | `UpdateBypassSnapshotStatus` | `(ctx, id string, status string) -> (BypassSnapshot, error)` |
| 17 | `GetLatestAppliedBypassSnapshot` | `(ctx, hostID string) -> (BypassSnapshot, error)` |
| 18 | `InsertBypassAuditLog` | `(ctx, params InsertBypassAuditLogParams) -> (string, error)` |
| 19 | `ListBypassAuditLogByTarget` | `(ctx, kind, id string) -> ([]BypassAuditLog, error)` |

> 计数说明：Plan 标题文案写「18 个」，但内容实际枚举 19 个（presets 6 + rules 4 + bindings 3 + snapshots 4 + audit 2）。本 plan 按内容数量 19 实现，反射测试 expected 列表也以 19 为准。Phase 46 启动前可在 Phase 45 PLAN.md 标题处把「18」修正为「19」，无功能影响。

## ErrSystemBypassPresetImmutable 使用方式

```go
// internal/store/repository/queries_bypass.go
var ErrSystemBypassPresetImmutable = errors.New("bypass preset is system preset and cannot be deleted or modified")
```

**触发点：**
- `DeleteBypassPreset(ctx, id)`：先 `SELECT is_system FROM host_bypass_presets WHERE id = $1`，命中则直接返回 sentinel；即使 Go 层漏检，DELETE SQL 也有 `AND is_system = FALSE` 兜底，永远不会真删。
- `UpdateBypassPreset(ctx, id, params)`：UPDATE WHERE 含 `AND is_system = FALSE`，影响 0 行时回查 `is_system` 区分「系统预设」与「行不存在」，分别返回 `ErrSystemBypassPresetImmutable` 与 `pgx.ErrNoRows`。

**Phase 46 handler 用法（推荐契约）：**
```go
if err := repo.DeleteBypassPreset(ctx, id); err != nil {
    if errors.Is(err, repository.ErrSystemBypassPresetImmutable) {
        return c.JSON(403, errorResp("BYPASS_PRESET_IMMUTABLE", "系统预设不可删除"))
    }
    if errors.Is(err, pgx.ErrNoRows) {
        return c.JSON(404, errorResp("BYPASS_PRESET_NOT_FOUND", "预设不存在"))
    }
    return c.JSON(500, errorResp("INTERNAL", err.Error()))
}
```

## SQL 常量清单（21 个，被 `TestBypassRepository_SQLConstants` lock）

`listBypassPresetsSQL` / `getBypassPresetBySlugSQL` / `getBypassPresetByIDSQL` / `createBypassPresetSQL` / `updateBypassPresetSQL` / `deleteBypassPresetSQL` / `checkBypassPresetIsSystemSQL` / `listBypassRulesGlobalOnlySQL` / `listBypassRulesGlobalOrHostSQL` / `createBypassRuleSQL` / `updateBypassRuleSQL` / `deleteBypassRuleSQL` / `listBypassBindingsByHostSQL` / `createBypassBindingSQL` / `deleteBypassBindingSQL` / `listBypassSnapshotsByHostSQL` / `createBypassSnapshotSQL` / `updateBypassSnapshotStatusSQL` / `getLatestAppliedBypassSnapshotSQL` / `insertBypassAuditLogSQL` / `listBypassAuditLogByTargetSQL`

所有常量遵守：
- `id::text` 把 UUID 转 string，对齐项目历史；
- 占位符一律 `$N`，禁止 fmt.Sprintf 拼接；
- preset 写入路径 (`UPDATE` / `DELETE`) 都附 `AND is_system = FALSE` 兜底。

## W4 拆分回顾

| 阶段 | 目的 | 价值 |
| ---- | ---- | ---- |
| Task 2a（panic stub） | 先把 5 个领域类型 + 9 个 *Params + 20 个 SQL 常量 + 19 个空方法签名 + sentinel 一次性写齐 | 让下游 plan 立刻能 import 类型与签名进行规划 |
| Task 3（lock 测试） | 反射断言方法签名 + 文本断言 SQL 常量 + 文案断言 sentinel | 把契约钉死，Task 2b / Phase 46 任何意外漂移立即被测试抓住 |
| Task 2b（填方法体） | 在签名与 SQL 不变的前提下只填实现 | 实现错误的爆炸半径被限制在方法体内，CI 反馈点清晰 |

收益：Task 2b 编辑量虽大（357 行新增），但因为 Task 3 在前面 lock 了所有外部可见契约，回归点窄，1 次过测试。

## 验证结果

```
go build ./internal/store/repository/...                                      # 通过
go test ./internal/store/repository/ -count=1 -short                          # ok
go test -run 'TestBypassRepository|TestMigration0019' -count=1 -v             # 6/6 PASS
  ├── TestMigration0019_FileContent
  ├── TestMigration0019_SystemPresetsSeed
  ├── TestMigration0019_SnapshotShape
  ├── TestBypassRepository_Signatures
  ├── TestBypassRepository_SQLConstants
  └── TestBypassRepository_ErrSystemPresetImmutable
```

migration `0019_host_bypass_rules.sql` 在 dev 库的实际执行验证留给 Phase 45 末尾的 `migrator` 启动脚本，本 plan 仅做静态断言。

## 跨 Plan 接口确认

- **Phase 46 admin API（消费方）：** 已确认可按 19 个方法签名 + `errors.Is(err, ErrSystemBypassPresetImmutable)` 翻译为 HTTP 403 BYPASS_PRESET_IMMUTABLE；`errors.Is(err, pgx.ErrNoRows)` 翻译 404。
- **Phase 47 apply/rollback（消费方）：** 已确认 `host_bypass_snapshots` 提供：
  - `version BIGINT NOT NULL` + `UNIQUE (host_id, config_hash)` 做幂等键；
  - `applied_status` 四态枚举（`pending` / `applied` / `failed` / `rolled_back`）；
  - `GetLatestAppliedBypassSnapshot` 定位回滚目标（version DESC LIMIT 1，filter applied_status='applied'）。
- **Phase 45 Plan 01 / 02（同 phase 兄弟）：** 本 plan 数据层独立，零横向依赖；Plan 01 控制面策略组件 / Plan 02 全隧道路由都通过 Repository 类型对齐数据契约。

## 偏差 / 自动修复

无 Rule 1-4 deviation。Plan 写「18 个方法」与实际「19 个」的偏差走文档修正路径，不影响实现（详见上面方法清单的「计数说明」）。

## 已知遗留 / 后续工作

- migration 0019 没有写 down 段，回滚靠运维手工执行（顶部注释给出 5 行 DROP TABLE 串）；这是项目历史一致策略，不视为遗留。
- BypassRule 表暂未做 `UNIQUE (scope, host_id, rule_type, value)` 去重约束；后续 Phase 46 在创建路径上做应用层去重即可，迁移层不绑死，避免阻塞历史脏数据。
- `host_bypass_audit_log` 暂未做分区；Phase 49+ 引入审计归档时再决定按月分区或冷热分层。

## Self-Check: PASSED

- [x] `internal/store/migrations/0019_host_bypass_rules.sql` 存在
- [x] `internal/store/repository/migration_0019_test.go` 存在
- [x] `internal/store/repository/queries_bypass.go` 存在（592 行）
- [x] `internal/store/repository/queries_bypass_test.go` 存在
- [x] `internal/store/repository/models.go` 已扩展含 BypassPreset/Rule/Binding/Snapshot/AuditLog 全套类型
- [x] commit `ca81807`（Task 1）/ `597c540`（Task 2a）/ `fc18ea9`（Task 3）/ `db42bb7`（Task 2b）均存在
- [x] `go build ./internal/store/repository/...` 通过
- [x] `go test ./internal/store/repository/ -short` 全包 PASS
- [x] 6 个新增测试全部 PASS
- [x] CLAUDE.md 合规：无绝对路径 / 无真实凭据 / 中文文档
