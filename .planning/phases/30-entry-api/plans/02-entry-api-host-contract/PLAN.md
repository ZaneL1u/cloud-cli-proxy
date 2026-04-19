---
phase: 30-entry-api
plan: 02-entry-api-host-contract
type: execute
wave: 2
depends_on:
  - 01-migration-entry-store
files_modified:
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker_volume_test.go
  - internal/controlplane/http/entry.go
  - internal/controlplane/http/entry_auth_test.go
  - internal/controlplane/http/entry_caps_test.go
  - internal/cloudclaude/entry.go
  - internal/cloudclaude/entry_compat_test.go
autonomous: true
requirements:
  - REQ-F7-A
must_haves:
  truths:
    - "在不新增 endpoint 的前提下，`/v1/entry/{id}/auth` 返回 v3 能力字段并保持 v2 客户端兼容（D-03）"
    - "`HostActionRequest` 可携带 `claude_account_id`，供 Phase 33 volume 编排使用（D-09）"
    - "能力字段仅由控制面按 template image tag 推导，不引入 host-agent label 查询（D-04,D-06,D-07）"
  artifacts:
    - path: internal/agentapi/contracts.go
      provides: "HostActionRequest.ClaudeAccountID JSON 契约"
      contains: claude_account_id
    - path: internal/controlplane/http/entry.go
      provides: "ready 响应扩展 image_version/supports_*/claude_account_id"
      contains: supports_mutagen
    - path: internal/cloudclaude/entry.go
      provides: "AuthResponse 新字段与向后兼容解析"
      contains: SupportsMergerfs
  key_links:
    - from: Entry Auth handler
      to: ResolveClaudeAccountIDForEntry
      via: "Wave 1 repository 接口"
      pattern: claude_account_id
    - from: template_image_ref
      to: image_version / supports_mutagen / supports_mergerfs
      via: "tag 解析与 v3.0.0 等值判断"
      pattern: v3.0.0
---

<objective>
完成 Phase 30 的 API/契约层扩展，形成与 Phase 31/33 对接所需的握手字段，不引入重复数据逻辑。

Purpose: 让 CLI 能在一次 auth 握手里感知镜像能力并拿到账号维度标识，同时保持旧版本兼容。
Output: Entry 响应扩展、agentapi 契约扩展、cloudclaude 响应结构扩展与兼容测试。
</objective>

<execution_context>
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/30-entry-api/plans/01-migration-entry-store/PLAN.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@internal/controlplane/http/entry.go
@internal/cloudclaude/entry.go
@internal/agentapi/contracts.go
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: 扩展 HostActionRequest 的 claude_account_id 契约</name>
  <files>internal/agentapi/contracts.go, internal/runtime/tasks/worker_volume_test.go</files>
  <read_first>.planning/phases/30-entry-api/30-CONTEXT.md (D-09), internal/agentapi/contracts.go</read_first>
  <behavior>
    - 旧 payload 不含 `claude_account_id` 时仍可正常反序列化。
    - 新 payload 带 `claude_account_id` 时 round-trip 保留值。
  </behavior>
  <action>在 `HostActionRequest` 新增 `ClaudeAccountID string \`json:"claude_account_id,omitempty"\``，并补足 JSON 兼容测试，禁止改动 `Volumes` 既有语义。</action>
  <verify>
    <automated>go test ./internal/runtime/tasks/... -count=1 -short</automated>
  </verify>
  <acceptance_criteria>契约升级后与 v2/v3 payload 均兼容，且为 Phase 33 提供可直接消费的字段。</acceptance_criteria>
  <done>agentapi 合约变更稳定，未引入破坏性变更。</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: 扩展 Entry Auth 响应字段并接入 Wave 1 查询</name>
  <files>internal/controlplane/http/entry.go, internal/controlplane/http/entry_auth_test.go, internal/controlplane/http/entry_caps_test.go</files>
  <read_first>.planning/phases/30-entry-api/30-CONTEXT.md (D-03,D-04,D-05,D-06,D-07,D-08), internal/controlplane/http/entry.go</read_first>
  <behavior>
    - ready 路径返回 `image_version`、`supports_mutagen`、`supports_mergerfs`。
    - 当账号解析 `ok=true` 时返回 `claude_account_id`；`ok=false` 时省略该字段。
    - 非 ready 路径保持既有行为，不强制携带扩展字段。
  </behavior>
  <action>扩展 `EntryStore` 接口并接入 Wave 1 的 `ResolveClaudeAccountIDForEntry` 与 `template_image_ref`；按 D-06/D-07 实现能力推导；坚持 D-03 单 endpoint 路径，不新增 `/capabilities`。</action>
  <verify>
    <automated>go test ./internal/controlplane/http/... -count=1 -short</automated>
  </verify>
  <acceptance_criteria>ROADMAP 的握手扩展字段在 ready 场景可稳定返回，且决策 D-03~D-08 全部可测试验证。</acceptance_criteria>
  <done>Entry API 扩展完成且不与数据层职责重叠。</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: cloudclaude AuthResponse 扩展与兼容回归</name>
  <files>internal/cloudclaude/entry.go, internal/cloudclaude/entry_compat_test.go</files>
  <read_first>.planning/phases/30-entry-api/30-CONTEXT.md (D-03,D-08), internal/cloudclaude/entry.go</read_first>
  <behavior>
    - 新响应结构能读取扩展字段。
    - 缺失扩展字段时不影响既有 SSH ready 校验。
    - 旧客户端反序列化新 JSON 不报错。
  </behavior>
  <action>扩展 `AuthResponse` 字段定义并新增兼容测试，禁止将新字段设为必填，确保旧网关/旧客户端路径可继续工作。</action>
  <verify>
    <automated>go test ./internal/cloudclaude/... -count=1 -short</automated>
  </verify>
  <acceptance_criteria>v2/v3 客户端均可使用同一 auth 接口，扩展字段是“增量能力”而不是“强制条件”。</acceptance_criteria>
  <done>客户端契约升级完成，兼容性由测试锁定。</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client → Entry Auth API | 未信任输入进入认证与握手响应路径 |
| controlplane → cloudclaude JSON contract | 响应字段变更会直接影响客户端行为 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-30-04 | T | Entry response mapping | mitigate | 通过 handler 测试锁定键名和值，避免 map 键拼写漂移 |
| T-30-05 | I | capability exposure | mitigate | 仅返回非敏感能力字段，`claude_account_id` 仅在认证成功路径输出 |
| T-30-06 | E | supports 推导 | accept | 误判只会降级能力，不会提升权限 |
| T-30-07 | D | 兼容性回归 | mitigate | 增加 v2/v3 反序列化回归测试并保持 omitempty 策略 |
</threat_model>

<verification>
- `go test ./internal/runtime/tasks/... -count=1 -short`
- `go test ./internal/controlplane/http/... -count=1 -short`
- `go test ./internal/cloudclaude/... -count=1 -short`
</verification>

<success_criteria>
- Wave 2 全面覆盖 D-03~D-09（不含已在 Wave 1 的 D-05 数据实现细节）。
- API/客户端扩展与 Wave 1 产物严格接线，无重复实现与无文件冲突。
</success_criteria>

<output>
After completion, create `.planning/phases/30-entry-api/plans/02-entry-api-host-contract/SUMMARY.md`
</output>

## Source Audit

| Source | Item | Coverage |
|--------|------|----------|
| GOAL | Entry API 扩展与握手字段 | COVERED |
| REQ | REQ-F7-A（命名约定的握手对齐） | COVERED |
| RESEARCH | entry/authresponse/contracts 测试策略 | COVERED |
| CONTEXT | D-03,D-04,D-05,D-06,D-07,D-08,D-09,D-12 | COVERED |
