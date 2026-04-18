---
phase: 30-entry-api
plan: 02-entry-api-host-contract
subsystem: api-contract
tags: [entry-api, cloudclaude, agentapi, capability-handshake, omitempty]

requires:
  - phase: 30-entry-api
    plan: 01-migration-entry-store
    provides: "HostSSHAuth.TemplateImageRef 字段 + ResolveClaudeAccountIDForEntry(ctx, userID, hostID) 方法"

provides:
  - "HostActionRequest.ClaudeAccountID (json:claude_account_id,omitempty) —— Phase 33 volume/worker 直接消费"
  - "EntryHandler ready 响应扩展 image_version / supports_mutagen / supports_mergerfs / claude_account_id（全部 omitempty）"
  - "deriveEntryCapabilities(templateImageRef) 纯函数：封装 D-06/D-07 的 tag 解析 + v3.0.0 能力门"
  - "EntryStore 接口扩展 ResolveClaudeAccountIDForEntry —— 与 repository 现成方法同签名"
  - "cloudclaude.AuthResponse 新字段 ImageVersion/SupportsMutagen/SupportsMergerfs/ClaudeAccountID，向后兼容旧 gateway"

affects:
  - Phase 31 Entry HTTP/CLI 客户端流程（可直接消费 AuthResponse v3 字段）
  - Phase 33 Docker volume/worker 编排（HostActionRequest.ClaudeAccountID 作为 volume 命名入参）
  - sshproxy / admin handlers（未受影响：GetHostByShortID 仅扩展 Scan 目标，无破坏性改动）

tech-stack:
  added: []
  patterns:
    - "能力推导统一走纯函数 deriveEntryCapabilities + 常量 v3CapBaseline=v3.0.0，避免在 handler 里散落字符串"
    - "HTTP 响应继续沿用 map[string]any + omitempty 语义的键控组合：未命中 -> 省略键；ok=true -> 写入键"
    - "跨模块（agentapi / controlplane / cloudclaude）JSON 键名同字面，测试以字符串断言锁定"

key-files:
  created:
    - internal/controlplane/http/entry_auth_test.go
    - internal/controlplane/http/entry_caps_test.go
    - internal/cloudclaude/entry_compat_test.go
  modified:
    - internal/agentapi/contracts.go
    - internal/controlplane/http/entry.go
    - internal/cloudclaude/entry.go
    - internal/runtime/tasks/worker_volume_test.go

key-decisions:
  - "deriveEntryCapabilities 作为 handler 外层的纯函数：便于单测覆盖所有 tag 形状（D-06/D-07），并守护『本阶段不引入多 tag 对照表』"
  - "Resolver 错误不降级：ResolveClaudeAccountIDForEntry 返回 err 时 handler 直接 500，避免把数据库错误静默变成『没有账号』"
  - "not_ready 分支保持原 4 字段响应：D-08 不强制带扩展字段；为了锁住契约，测试显式断言 not_ready 里 4 个扩展键缺席"
  - "EntryStore 接口扩展与 repository 现成方法签名完全一致；无需修改 wire（app.go 的 EntryStore: repo 自动满足新接口）"
  - "AuthResponse v3 字段全部 omitempty：对 bool 类型 omitempty 意味着 false 会被省略——这与 D-08『增量能力，非强制条件』一致，客户端读到 SupportsMutagen=false 默认即视为不具备该能力"

requirements-completed: [REQ-F7-A]

duration: ~20min
completed: 2026-04-18
---

# Phase 30 Plan 02: Entry API Host Contract Summary

**在不新增 endpoint 的前提下，扩展 `/v1/entry/{shortId}/auth`、`HostActionRequest`、`cloudclaude.AuthResponse` 三处契约，把 Phase 30 Wave 1 的数据层产物接线到握手路径上，同时严格保持 v2 客户端兼容。**

## Performance

- **Duration:** ~20 分钟
- **Started:** 2026-04-18（Wave 2 执行）
- **Completed:** 2026-04-18
- **Tasks:** 3（全部 TDD RED→GREEN）
- **Commits:** 6（3 test + 3 feat）

## Accomplishments

- **Task 1 — agentapi 契约升级：** `HostActionRequest` 新增 `ClaudeAccountID string \`json:"claude_account_id,omitempty"\`` (D-09)。同时在 `worker_volume_test.go` 追加 Omitempty / RoundTrip / ForwardCompat / V2 兼容四组断言，锁死"空串不得序列化"和"v3 payload 可被旧结构解析"两条边界。未改动 `Volumes` 语义，不破坏 Phase 29 已经交付的 worker 挂载路径。
- **Task 2 — Entry API 扩展：** `EntryHandler.Auth()` 在 ready 分支返回 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id`（D-03 单 endpoint、D-06/D-07 能力推导、D-05 账号解析、D-08 非 ready 不强制扩展）。能力推导抽出纯函数 `deriveEntryCapabilities`：对 `template_image_ref` trim → 取最后一个 `:` 之后的 tag → 再 trim → 与常量 `v3CapBaseline = "v3.0.0"` 字符串相等决定 `supports_*`。`EntryStore` 接口新增 `ResolveClaudeAccountIDForEntry`，由 Wave 1 已落地的 `*repository.Repository` 方法天然满足，`app.go` 的 `EntryStore: repo` 无需任何改动即可编译通过。
- **Task 3 — cloudclaude 客户端升级：** `AuthResponse` 追加四个 omitempty 字段，新增 `entry_compat_test.go` 覆盖 v3 round-trip、v2 缺省场景、omitempty 序列化、以及通过 `httptest.Server` 的端到端 `Authenticate()` 回归。v2 gateway 响应依然能走 ready 分支并保持原 SSH 四元组校验。

## Task Commits

按 TDD 原子推进（test → feat）：

1. **Task 1 RED:** `e1ea1cb` — `test(30-02-entry-api-host-contract): add failing tests for HostActionRequest.ClaudeAccountID`
2. **Task 1 GREEN:** `358f973` — `feat(30-02-entry-api-host-contract): add ClaudeAccountID to HostActionRequest`
3. **Task 2 RED:** `d2227ae` — `test(30-02-entry-api-host-contract): add failing tests for entry auth v3 fields`
4. **Task 2 GREEN:** `6bb46fb` — `feat(30-02-entry-api-host-contract): extend entry auth with v3 capability fields`
5. **Task 3 RED:** `dfa0c3c` — `test(30-02-entry-api-host-contract): add failing tests for AuthResponse v3 fields`
6. **Task 3 GREEN:** `b50cea8` — `feat(30-02-entry-api-host-contract): extend AuthResponse with v3 capability fields`

## Files Created/Modified

- `internal/agentapi/contracts.go` — `HostActionRequest.ClaudeAccountID` 新字段（D-09）。
- `internal/controlplane/http/entry.go` — 新增常量 `v3CapBaseline`、纯函数 `deriveEntryCapabilities`，扩展 `EntryStore` 接口，`Auth()` ready 响应追加四个扩展字段；非 ready / 401 分支保持原样。
- `internal/controlplane/http/entry_auth_test.go` — 新增 HTTP 级别回归：ready via host short_id / via user short_id、claude_account_id 省略、resolver 错误 500、not_ready / 401 不暴露扩展字段。
- `internal/controlplane/http/entry_caps_test.go` — 新增 `deriveEntryCapabilities` 七个 table-driven case：v3.0.0 / v2.0.0 / pre-release / no-colon / 空串 / 含 port 的 registry / whitespace。
- `internal/cloudclaude/entry.go` — `AuthResponse` 新增 4 个 omitempty 字段。
- `internal/cloudclaude/entry_compat_test.go` — 新增结构体层面的 marshal/unmarshal 兼容测试 + 基于 `httptest.Server` 的 v2/v3 gateway 端到端兼容回归。
- `internal/runtime/tasks/worker_volume_test.go` — 新增 4 个契约断言围绕 `ClaudeAccountID`，并在既有 `V2Compat` 里补充 "缺省空串" 的守护。

## Decisions Made

- **Handler 外纯函数承载能力推导：** 在 `entry.go` 顶部定义 `deriveEntryCapabilities` 与 `v3CapBaseline`，避免把 tag 解析逻辑散落到 `Auth()` 里；这样 `entry_caps_test.go` 的 table-driven 覆盖就等价于 D-06/D-07 契约测试，后续加新能力时也只需改这一处。
- **Resolver 错误 → 500，不降级为 omit：** D-05 的第三条"未命中省略字段"只适用于 `pgx.ErrNoRows` 语义；任意其他错误应 fail-fast 而不是把数据库故障伪装成"该用户没有账号"。`stubEntryStore.resolveAccountErr` 的单测显式锁死这一点。
- **D-08『不强制扩展字段』以缺席方式表达：** 非 ready 路径（例如 host_status != "running"）保持原有 2 字段响应（`status` + `message`），测试显式断言 4 个扩展键一个都不能出现，避免后续"顺手带上 image_version"这种隐形漂移。
- **AuthResponse 的 bool 字段也走 omitempty：** 因为缺省值 false 本身就是"不具备该能力"的语义，等价于"字段缺失"；相比另起 `*bool` 三态，当前项目的其他 v3 客户端检测逻辑可以直接 `if resp.SupportsMutagen {}`，代价更低。
- **EntryStore 扩展签名与 repository 对齐，复用 app.go 现有 wire：** 不引入新的 struct 或新 wire 层，直接依赖 Go 接口的结构化匹配；`*repository.Repository` 在 Wave 1 已实现该方法，零侵入完成接线。

## Deviations from Plan

None — 全部 3 个任务按 TDD RED→GREEN 完成，未触发 Rule 1/2/3 自动修复。`EntryHandler` 中对 resolver 错误返回 500 属于 Rule 2 "auto-add missing critical functionality" 的兜底逻辑，但计划 `<behavior>` 与 `<threat_model>` 已经暗示了这条路径（T-30-04），所以不单列为 deviation。

## Issues Encountered

- 最初担心 `EntryStore` 接口扩展会打破 `internal/controlplane/app/app.go` 的 wire：实测 `*repository.Repository` 已在 Wave 1 交付 `ResolveClaudeAccountIDForEntry`，接口新增方法后依赖侧自动满足，编译与单测均无改动需求。这个经验把"数据层在前、API 层在后"的 wave 分层 ROI 体现得很直接。
- `stubEntryStore` 在 `entry_auth_test.go` 里作为 `EntryStore` 接口的实现：为了避免一个新 stub 文件额外影响 build，直接放在测试文件顶部。

## Threat Model Compliance

- **T-30-04 (T — Entry response mapping)**：`entry_auth_test.go` 以字符串键断言锁定 `image_version` / `supports_mutagen` / `supports_mergerfs` / `claude_account_id` 四个 JSON 键名；`entry_caps_test.go` 锁定值域。"键名拼写漂移"会被两处测试同时标红。
- **T-30-05 (I — capability exposure)**：ready 路径只返回非敏感能力 bool + tag 字符串；`claude_account_id` 只有在 `ok=true` 时才出现；`TestEntryAuth_InvalidCredentials_NoExtensions` 明确验证 401 响应不泄露任何扩展字段；`TestEntryAuth_NotReady_DoesNotForceExtensionFields` 锁定 not_ready 路径同样不暴露。
- **T-30-06 (E — supports 推导)**：`deriveEntryCapabilities` 的默认分支都是 `false`，符合"误判只会降级能力、不会提升权限"的 accept 语义。
- **T-30-07 (D — 兼容性回归)**：`TestHostActionRequest_V2Compat` / `TestHostActionRequest_ClaudeAccountID_ForwardCompat` / `TestAuthenticate_V2Gateway_NoExtensionsRequired` / `TestAuthResponse_MarshalOmitempty` 四条测试从 agentapi、cloudclaude 两侧共同保证 v2↔v3 payload 互操作。

## Threat Flags

无。本计划未在计划外引入新 endpoint、未改动鉴权路径、未新增数据流入点；仅在既有 `/v1/entry/{shortId}/auth` 的 ready 分支追加非敏感字段。

## User Setup Required

None — 控制面部署时无新配置；前端 / CLI 如果要消费新字段可在 Phase 31/33 再接入。

## Next Phase Readiness

- **Phase 31（Entry HTTP/CLI 流程）：** 可直接读取 `cloudclaude.AuthResponse.ImageVersion/SupportsMutagen/SupportsMergerfs/ClaudeAccountID`，无需任何控制面改动；缺省值全部安全（`false` / `""`）。
- **Phase 33（Docker worker / volume 编排）：** 可从 `HostActionRequest.ClaudeAccountID` 直接派生 `claude-state-{id}` volume 名（配合 Wave 1 的 `ClaudeAccount.PersistentVolumeName *string`），不需要再做 API 变更。
- **后续镜像 tag 扩展：** 当前 `v3CapBaseline` 仅字符串相等；若后续出现多 tag 能力矩阵，扩展点集中在 `deriveEntryCapabilities` + `entry_caps_test.go`，不会再扩散到 handler 或客户端。

---
*Phase: 30-entry-api*
*Plan: 02-entry-api-host-contract*
*Completed: 2026-04-18*

## Self-Check

Performed the following verifications before completion:

- `[ -f internal/controlplane/http/entry_auth_test.go ]` → FOUND
- `[ -f internal/controlplane/http/entry_caps_test.go ]` → FOUND
- `[ -f internal/cloudclaude/entry_compat_test.go ]` → FOUND
- `git log --oneline | grep e1ea1cb` → FOUND (Task 1 test commit)
- `git log --oneline | grep 358f973` → FOUND (Task 1 feat commit)
- `git log --oneline | grep d2227ae` → FOUND (Task 2 test commit)
- `git log --oneline | grep 6bb46fb` → FOUND (Task 2 feat commit)
- `git log --oneline | grep dfa0c3c` → FOUND (Task 3 test commit)
- `git log --oneline | grep b50cea8` → FOUND (Task 3 feat commit)
- `go test ./internal/runtime/tasks/... -count=1 -short` → PASS
- `go test ./internal/controlplane/http/... -count=1 -short` → PASS
- `go test ./internal/cloudclaude/... -count=1 -short` → PASS
- `go test ./... -count=1 -short` → PASS
- `go build ./...` → PASS
- `go vet ./...` → PASS

## Self-Check: PASSED
