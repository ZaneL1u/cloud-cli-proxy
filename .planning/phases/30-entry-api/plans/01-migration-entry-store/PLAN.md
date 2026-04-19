---
phase: 30-entry-api
plan: 01-migration-entry-store
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/store/migrations/0014_claude_account_persistent_volume.sql
  - internal/store/repository/models.go
  - internal/store/repository/queries.go
  - internal/store/repository/migration_0014_test.go
autonomous: true
requirements:
  - REQ-F7-A
must_haves:
  truths:
    - "数据层具备 `persistent_volume_name` 列，且 `NULL` 语义可表达“未分配”状态（D-01, D-02, D-10）"
    - "仓储层可按 D-05 规则稳定解析 `claude_account_id`，并返回 Entry API 所需 `template_image_ref` 输入"
    - "本计划不触碰 HTTP/客户端协议，避免与 Wave 2 计划重叠"
  artifacts:
    - path: internal/store/migrations/0014_claude_account_persistent_volume.sql
      provides: "claude_accounts.persistent_volume_name schema 变更与安全回滚注释"
      contains: persistent_volume_name
    - path: internal/store/repository/models.go
      provides: "ClaudeAccount 可空字段映射"
      contains: PersistentVolumeName
    - path: internal/store/repository/queries.go
      provides: "ResolveClaudeAccountIDForEntry + template_image_ref 查询输出"
      contains: ResolveClaudeAccountIDForEntry
  key_links:
    - from: ResolveClaudeAccountIDForEntry
      to: claude_accounts
      via: "host_id 优先，fallback 到 user_id + host_id IS NULL（D-05）"
      pattern: ORDER BY created_at ASC LIMIT 1
    - from: GetHostByShortID
      to: hosts.template_image_ref
      via: "SELECT 列扩展供 Wave 2 推导能力字段"
      pattern: template_image_ref
---

<objective>
完成 Phase 30 的数据层基础：数据库迁移、仓储模型与账号解析查询，给 Phase 31/33 提供握手和 volume 命名所需的统一数据来源。

Purpose: 先锁定数据真值，确保 API 层只做协议拼装而不再重复定义规则。
Output: `0014` migration + repository 查询/测试，形成 Wave 2 的稳定依赖。
</objective>

<execution_context>
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/30-entry-api/30-RESEARCH.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@internal/store/migrations/0007_auth_unification.sql
@internal/store/repository/models.go
@internal/store/repository/queries.go
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: 落地 0014 migration 与可空字段语义</name>
  <files>internal/store/migrations/0014_claude_account_persistent_volume.sql, internal/store/repository/models.go, internal/store/repository/migration_0014_test.go</files>
  <read_first>.planning/phases/30-entry-api/30-CONTEXT.md (D-01,D-02,D-10), internal/store/migrations/0007_auth_unification.sql</read_first>
  <behavior>
    - migration 使用 `ADD COLUMN IF NOT EXISTS persistent_volume_name TEXT`，不设置空字符串默认值。
    - down 注释包含 `DROP COLUMN IF EXISTS persistent_volume_name`，确保回滚路径明确。
  </behavior>
  <action>按 D-01/D-02/D-10 新增 migration，并在 `ClaudeAccount` 增加可空字段映射；保持向后兼容，不修改任何 HTTP/agent 契约。</action>
  <verify>
    <automated>go test ./internal/store/repository/... -count=1 -short</automated>
  </verify>
  <acceptance_criteria>空库与升级库均可执行 migration；模型层可区分 `NULL` 与非空 volume 名。</acceptance_criteria>
  <done>schema 与模型完成并有自动化测试覆盖。</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: 实现 D-05 账号选择查询与 template_image_ref 输出</name>
  <files>internal/store/repository/queries.go, internal/store/repository/migration_0014_test.go</files>
  <read_first>.planning/phases/30-entry-api/30-CONTEXT.md (D-05,D-06,D-11), internal/store/repository/queries.go</read_first>
  <behavior>
    - host 绑定账号优先：`host_id = ? ORDER BY created_at ASC LIMIT 1`
    - fallback 账号：`user_id = ? AND host_id IS NULL ORDER BY created_at ASC LIMIT 1`
    - 无命中返回 `ok=false`，不是错误。
  </behavior>
  <action>新增 `ResolveClaudeAccountIDForEntry`（或等价语义命名），并扩展 `GetHostByShortID` 查询输出 `template_image_ref`，为 Wave 2 的能力字段推导提供输入。</action>
  <verify>
    <automated>go test ./internal/store/repository/... -count=1 -short</automated>
  </verify>
  <acceptance_criteria>解析顺序与 D-05 完全一致；`template_image_ref` 可被上层读取且不破坏现有调用。</acceptance_criteria>
  <done>仓储层提供稳定 API，Wave 2 不再需要改 SQL。</done>
</task>

<task type="auto">
  <name>Task 3: 数据层完整性回归（防止与 Wave 2 重叠）</name>
  <files>internal/store/repository/migration_0014_test.go</files>
  <read_first>.planning/ROADMAP.md (Phase 30 Scope/Success Criteria)</read_first>
  <action>补充回归断言：本计划仅覆盖 migration/repository，不引入 `internal/controlplane/http` 与 `internal/cloudclaude` 变更，确保数据层与 API 层职责边界清晰。</action>
  <verify>
    <automated>go test ./internal/store/repository/... -count=1</automated>
  </verify>
  <acceptance_criteria>测试通过且无 HTTP/客户端文件变更，满足“数据层先于 API 层”的波次隔离。</acceptance_criteria>
  <done>Wave 1 可独立执行并产出可被 Wave 2 直接消费的数据接口。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| controlplane → postgres | schema 与查询变更直接影响认证链路数据正确性 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-30-01 | T | repository SQL | mitigate | 全部使用参数化查询，不拼接输入字符串 |
| T-30-02 | D | migration 0014 | mitigate | 使用 IF NOT EXISTS / IF EXISTS 与自动化回归测试 |
| T-30-03 | I | claude_account 解析 | accept | 仅在控制面内部使用，不扩大外部暴露面 |
</threat_model>

<verification>
- `go test ./internal/store/repository/... -count=1 -short`
- `go test ./internal/store/repository/... -count=1`
</verification>

<success_criteria>
- D-01/D-02/D-10 与 D-05 在数据层全部落地且有自动化断言。
- Wave 1 输出可直接被 Wave 2 的 Entry API 计划消费，不需要重复迁移或重复查询实现。
</success_criteria>

<output>
After completion, create `.planning/phases/30-entry-api/plans/01-migration-entry-store/SUMMARY.md`
</output>

## Source Audit

| Source | Item | Coverage |
|--------|------|----------|
| GOAL | Phase 30 数据模型与握手前置 | COVERED |
| REQ | REQ-F7-A（命名约定的数据模型落地） | COVERED |
| RESEARCH | migration + repository + D-05 查询策略 | COVERED |
| CONTEXT | D-01,D-02,D-05,D-06,D-10,D-11,D-12 | COVERED |
