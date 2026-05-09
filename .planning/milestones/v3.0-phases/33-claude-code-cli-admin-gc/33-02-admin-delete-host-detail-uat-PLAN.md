---
phase: 33-claude-code-cli-admin-gc
plan: 02
type: execute
wave: 2
depends_on: [01]
autonomous: false
requirements: [REQ-F7-D]
requirements_addressed: [REQ-F7-A, REQ-F7-D]
files_modified:
  - internal/store/repository/queries.go
  - internal/store/repository/models.go
  - internal/controlplane/http/admin_claude_accounts.go
  - internal/controlplane/http/admin_claude_accounts_test.go
  - internal/controlplane/http/admin_hosts.go
  - internal/controlplane/http/router.go
  - docs/runbooks/v3-claude-state-volumes.md

must_haves:
  truths:
    - "admin DELETE claude_account 默认强一致：volume 仍被容器持有时 HTTP 409 + 中文提示 + DB 行仍在（SC4 / REQ-F7-D / D-18）"
    - "admin DELETE ?force=true 时 DB 先删；rm 失败仅写 audit + HTTP 200 with volume_rm:failed（D-19 最终一致）"
    - "GET /v1/admin/hosts/{id} 响应顶层追加 persistent_volume_name 字段（OOS-A19 边界 \"最多加一行\"，list endpoint 不动）"
    - "admin DELETE 成功后 docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id> 返回空（SC2 / REQ-F7-A 闭环）"
    - "运维手册新增 v3-claude-state-volumes 章节，含 volume 命名规范 + GC 路径 + 孤儿 volume 审计脚本（M16 兜底）"
  artifacts:
    - path: internal/controlplane/http/admin_claude_accounts.go
      provides: "DELETE /v1/admin/claude-accounts/{accountID} handler + force flag + 强/最终一致两条路径"
      contains: "AdminClaudeAccountsHandler"
    - path: internal/store/repository/queries.go
      provides: "GetHostWithClaudeAccount LEFT JOIN + BeginTx + LockClaudeAccountForDelete + DeleteClaudeAccountTx"
      contains: "GetHostWithClaudeAccount"
    - path: internal/store/repository/models.go
      provides: "HostWithClaudeAccount 类型（embed Host + PersistentVolumeName）"
      contains: "HostWithClaudeAccount"
    - path: internal/controlplane/http/admin_hosts.go
      provides: "adminHostDetailResponse 追加 persistent_volume_name"
      contains: "PersistentVolumeName string"
    - path: internal/controlplane/http/router.go
      provides: "DELETE /v1/admin/claude-accounts/{accountID} 路由注册 + Dependencies 字段扩展"
      contains: "AdminClaudeAccounts"
    - path: docs/runbooks/v3-claude-state-volumes.md
      provides: "运维手册章节：volume 命名规范 + GC 路径 + 孤儿审计"
      contains: "claude-state-"
  key_links:
    - from: "DELETE /v1/admin/claude-accounts/{accountID}"
      to: "agentapi.ActionVolumeRemove via runHostAction"
      via: "事务内 SELECT FOR UPDATE → host-agent 调用 → 成功 DELETE+COMMIT，失败 ROLLBACK + 409"
      pattern: "agentapi.HostActionRequest\\{Action: agentapi.ActionVolumeRemove"
    - from: "GET /v1/admin/hosts/{hostID}"
      to: "Repository.GetHostWithClaudeAccount LEFT JOIN"
      via: "纯 DB JOIN（不引入 docker exec）"
      pattern: "GetHostWithClaudeAccount"
    - from: "DELETE handler force=true 路径"
      to: "agentapi.HostActionRequest{Labels: {\"force\": \"true\"}}"
      via: "Plan 01 worker.removeVolumes 读 request.Labels[\"force\"]"
      pattern: "Labels:\\s*map\\[string\\]string\\{\"force\": \"true\"\\}"
---

<objective>
完成 Phase 33 控制面侧闭环：(1) 新建 `DELETE /v1/admin/claude-accounts/{accountID}` handler，强一致路径在 SQL 事务内同步调 host-agent `ActionVolumeRemove`，rm 失败 → ROLLBACK + audit `claude_account.delete_volume_rm_failed` + HTTP 409；`?force=true` 走最终一致：DB 先 COMMIT → host-agent rm，失败仅 audit + HTTP 200 with `volume_rm:failed`；(2) 仓储新增 `BeginTx` 公开方法 + `LockClaudeAccountForDelete` + `DeleteClaudeAccountTx` 事务工具 + `GetHostWithClaudeAccount` LEFT JOIN；(3) `adminHostDetailResponse` 追加 `persistent_volume_name string` 字段（OOS-A19 边界），list endpoint **不动**；(4) router 注册 + Dependencies 字段扩展 `AdminClaudeAccounts` / `AgentClient`；(5) handler 单测覆盖 D-25.5（strict 成功 / strict 409+rollback / force=true 200+rm失败）+ D-25.6（GetHostWithClaudeAccount 命中/nil）；(6) 运维手册新增 `v3-claude-state-volumes.md`（命名规范 + GC 路径 + 孤儿审计脚本）。

Purpose: 闭合 REQ-F7-D（admin DELETE 事务联动 volume rm）+ ROADMAP §Phase 33 SC4/SC6（删 account 后 volume 必空 / rm 失败 ROLLBACK）；通过 OOS-A19 边界守恒 admin host detail 字段，让运维能在不开管理页的前提下看到 volume 名（REQ-F7-A 命名规范的 UI 收口）。

Output:
- 1 个新建 handler（~180 行）
- 1 个新建 handler 单测（~250 行）
- 1 个修改的仓储 SQL（新增 ~70 行：GetHostWithClaudeAccount + BeginTx + Lock + Delete）
- 1 个修改的 models.go（新增 HostWithClaudeAccount ~5 行）
- 1 个修改的 admin_hosts.go detail 响应字段
- 1 个修改的 router.go（路由注册 + Dependencies）
- 1 个新建运维手册章节
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md
@.planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md
@.planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md
@.planning/phases/33-claude-code-cli-admin-gc/33-01-SUMMARY.md  # Plan 01 成果（含 ActionVolumeRemove / removeDockerVolume / WorkerRepo 接口契约）
@.planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md  # admin handler fail-fast + audit 模式

# 直接修改对象
@internal/store/repository/queries.go
@internal/store/repository/models.go
@internal/controlplane/http/admin_hosts.go
@internal/controlplane/http/router.go
@internal/agentapi/contracts.go
@internal/agentapi/client.go

# 单测复用对象
@internal/controlplane/http/admin_users_test.go  # stubEventRecorder + validAdminToken + adminTestRouter
@internal/controlplane/http/admin_hosts_test.go  # stubHostStore 模板

<interfaces>
<!-- Plan 01 已落地的契约（executor 直接消费） -->

From internal/agentapi/contracts.go (Plan 01 增量):
```go
const (
    ActionCreateHost   HostAction = "create_host"
    ActionStartHost    HostAction = "start_host"
    ActionStopHost     HostAction = "stop_host"
    ActionRebuildHost  HostAction = "rebuild_host"
    ActionPrepareHost  HostAction = "prepare_host"
    ActionVolumeRemove HostAction = "volume_remove" // Plan 01 Task 1.2
)

// VolumeMount.Name 用于 ActionVolumeRemove；其它字段被 worker 忽略
// HostActionRequest.Labels["force"] == "true" 时 worker 走 docker volume rm -f
```

From internal/agentapi/client.go (line 73):
```go
func (c *Client) RunHostAction(ctx context.Context, request HostActionRequest) (HostActionResponse, error)
// 默认 httpClient.Timeout = 30s（client.go:30）
```

From internal/controlplane/http/admin_hosts.go (line 25-46):
```go
type AdminHostStore interface {
    ListHostsWithUsername(context.Context) ([]repository.HostWithUsername, error)
    GetHostDetail(context.Context, string) (repository.HostDetail, error)
    GetHost(context.Context, string) (repository.Host, error)
    // ... 9 methods total
}

type AdminHostsHandler struct { ... }
func NewAdminHostsHandler(logger, store, queue, events) *AdminHostsHandler

// adminHostDetailResponse (line 96-99)：
type adminHostDetailResponse struct {
    repository.HostDetail
    ConnectionInfo *repository.ConnectionInfo `json:"connection_info,omitempty"`
}
```

From internal/controlplane/http/admin_users_test.go (line 79-117):
```go
type stubEventRecorder struct {
    called bool
    events []repository.RecordEventParams
}
func (s *stubEventRecorder) RecordEvent(...) (repository.Event, error)
func (s *stubEventRecorder) hasType(t string) bool
func validAdminToken(t *testing.T) string  // 返回 admin role JWT
// adminTestRouter 在同 package 可用
```

From internal/controlplane/http/router.go (line 27-54, 232-256):
```go
type Dependencies struct {
    Logger          *slog.Logger
    Health          HealthChecker
    // ... existing
    AdminHosts      AdminHostStore
    AdminEvents     AdminEventStore
    EventRecorder   EventRecorder
    // ...
}

// Existing pattern (line 232+):
if deps.AdminHosts != nil {
    hostsHandler := NewAdminHostsHandler(deps.Logger, deps.AdminHosts, deps.HostActions, deps.EventRecorder)
    mux.Handle("DELETE /v1/admin/hosts/{hostID}", adminGuard(hostsHandler.Delete()))
}
```

From internal/store/repository/models.go (line 157-163, 209-222):
```go
// HostWithUsername — embed + 追加列模式 analog
type HostWithUsername struct {
    Host
    Username       string  `json:"username"`
    EgressIPLabel  *string `json:"egress_ip_label,omitempty"`
    // ...
}

// ClaudeAccount.PersistentVolumeName *string  // 三态：NULL = 未分配 / 非空 = 已分配
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 2.1: 仓储新增 BeginTx + LockClaudeAccountForDelete + DeleteClaudeAccountTx + GetHostWithClaudeAccount + HostWithClaudeAccount 类型（D-18 / D-23 / D-25.6）</name>
  <files>internal/store/repository/queries.go, internal/store/repository/models.go, internal/store/repository/queries_claude_account_delete_test.go</files>

  <read_first>
    - internal/store/repository/queries.go（特别是 line 1180-1271：claude_account 查询群 + UpdateHostEntryPassword 风格 + 包级 const SQL 提升约定）
    - internal/store/repository/models.go（line 145-163 HostDetail/HostWithUsername embed 模式 + line 209-222 ClaudeAccount.PersistentVolumeName）
    - internal/store/migrator/migrator.go（特别是 line 46-66 唯一既有 r.db.Begin 调用点 — pgx.Tx 使用模板）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-18/D-22/D-23
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §1 D-18 ⚠ + §2.4 (b)(c) verbatim 骨架
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `internal/store/repository/queries.go` + `models.go` 节 + Shared 2
  </read_first>

  <behavior>
    - Test 1 (TestGetHostWithClaudeAccountSQL_ContainsLeftJoinTokens)：SQL 字符串包含 `"FROM hosts h"` + `"LEFT JOIN claude_accounts ca ON ca.host_id = h.id"` + `"COALESCE(ca.persistent_volume_name, '')"` + `"LIMIT 1"`
    - Test 2 (TestLockClaudeAccountForDeleteSQL_HasForUpdate)：SQL 字符串包含 `"FOR UPDATE"` 与 `"COALESCE(persistent_volume_name, '')"` 与 `"WHERE id = $1"`
    - Test 3 (TestDeleteClaudeAccountSQL_IsDelete)：SQL 字符串 = `"DELETE FROM claude_accounts WHERE id = $1"`（精确匹配）
    - Test 4 (TestHostWithClaudeAccount_EmbedsHost)：通过反射或字段访问验证 `HostWithClaudeAccount{}.Host.ID` 编译通过 + `PersistentVolumeName` 是 string 类型
  </behavior>

  <action>
**(a) 在 `internal/store/repository/models.go` 的 `HostWithUsername`（line 157-163）之后追加：**

```go
// HostWithClaudeAccount D-23：纯 DB JOIN，避免在 detail handler 引入 docker exec。
// 配合 GetHostWithClaudeAccount LEFT JOIN 使用；空 PersistentVolumeName = 该 host 关联 account 未分配 volume 或无 account。
type HostWithClaudeAccount struct {
	Host
	PersistentVolumeName string `json:"persistent_volume_name,omitempty"`
}
```

**(b) 在 `internal/store/repository/queries.go` 文件末尾追加（**verbatim 复制 RESEARCH §2.4 (b)(c)**）：**

```go
const getHostWithClaudeAccountSQL = `
	SELECT
		h.id::text, h.user_id::text, h.status, COALESCE(h.short_id, ''),
		COALESCE(h.entry_password, ''), h.template_image_ref, h.home_volume_name,
		h.slot_key, h.timezone, h.hostname, h.memory_limit_mb, h.cpu_limit,
		h.disk_limit_gb, h.created_at, h.updated_at,
		COALESCE(ca.persistent_volume_name, '')
	FROM hosts h
	LEFT JOIN claude_accounts ca ON ca.host_id = h.id
	WHERE h.id = $1
	ORDER BY ca.created_at ASC
	LIMIT 1
`

// GetHostWithClaudeAccount D-23：单次 LEFT JOIN 返回 host + 可能 NULL 的 persistent_volume_name。
// 与 GetHost / ListHostsWithUsername 等 6 个既有 SELECT 解耦，不修改 Phase 29.1 已锁定的查询。
func (r *Repository) GetHostWithClaudeAccount(ctx context.Context, hostID string) (HostWithClaudeAccount, error) {
	var item HostWithClaudeAccount
	if err := r.db.QueryRow(ctx, getHostWithClaudeAccountSQL, hostID).Scan(
		&item.ID, &item.UserID, &item.Status, &item.ShortID,
		&item.EntryPassword, &item.TemplateImageRef, &item.HomeVolumeName,
		&item.SlotKey, &item.Timezone, &item.Hostname,
		&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
		&item.CreatedAt, &item.UpdatedAt,
		&item.PersistentVolumeName,
	); err != nil {
		return HostWithClaudeAccount{}, fmt.Errorf("get host with claude_account: %w", err)
	}
	return item, nil
}

// BeginTx 暴露 pgx 事务给 admin handler（D-18），避免把 *pgxpool.Pool 泄漏到 control plane。
// 与 internal/store/migrator/migrator.go:46 唯一既有 r.db.Begin 调用点对齐。
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

const lockClaudeAccountForDeleteSQL = `
	SELECT id::text, COALESCE(persistent_volume_name, '')
	FROM claude_accounts
	WHERE id = $1
	FOR UPDATE
`

const deleteClaudeAccountSQL = `DELETE FROM claude_accounts WHERE id = $1`

// LockClaudeAccountForDelete D-18 强一致路径第 2 步：BEGIN 后行锁 + 读 volume 名。
// 包级函数（非 method）以便 handler 显式持有 tx ref。
func LockClaudeAccountForDelete(ctx context.Context, tx pgx.Tx, id string) (accountID, volumeName string, err error) {
	err = tx.QueryRow(ctx, lockClaudeAccountForDeleteSQL, id).Scan(&accountID, &volumeName)
	return
}

// DeleteClaudeAccountTx 在事务内删除 claude_account 行；RowsAffected==0 返回 pgx.ErrNoRows。
func DeleteClaudeAccountTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, deleteClaudeAccountSQL, id)
	if err != nil {
		return fmt.Errorf("delete claude_account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
```

**(c) 新建 `internal/store/repository/queries_claude_account_delete_test.go`：**

```go
package repository

import (
	"strings"
	"testing"
)

func TestGetHostWithClaudeAccountSQL_ContainsLeftJoinTokens(t *testing.T) {
	must := []string{
		"FROM hosts h",
		"LEFT JOIN claude_accounts ca ON ca.host_id = h.id",
		"COALESCE(ca.persistent_volume_name, '')",
		"WHERE h.id = $1",
		"LIMIT 1",
	}
	for _, token := range must {
		if !strings.Contains(getHostWithClaudeAccountSQL, token) {
			t.Errorf("getHostWithClaudeAccountSQL missing %q\nfull:\n%s", token, getHostWithClaudeAccountSQL)
		}
	}
}

func TestLockClaudeAccountForDeleteSQL_HasForUpdate(t *testing.T) {
	must := []string{
		"FROM claude_accounts",
		"WHERE id = $1",
		"FOR UPDATE",
		"COALESCE(persistent_volume_name, '')",
	}
	for _, token := range must {
		if !strings.Contains(lockClaudeAccountForDeleteSQL, token) {
			t.Errorf("lockClaudeAccountForDeleteSQL missing %q\nfull:\n%s", token, lockClaudeAccountForDeleteSQL)
		}
	}
}

func TestDeleteClaudeAccountSQL_IsExactDelete(t *testing.T) {
	want := `DELETE FROM claude_accounts WHERE id = $1`
	if deleteClaudeAccountSQL != want {
		t.Errorf("deleteClaudeAccountSQL must equal %q, got %q", want, deleteClaudeAccountSQL)
	}
}

func TestHostWithClaudeAccount_EmbedsHost(t *testing.T) {
	var item HostWithClaudeAccount
	// 编译器断言 embed Host：访问 ID 字段
	item.ID = "h-1"
	item.PersistentVolumeName = "claude-state-acct-42"
	if item.ID != "h-1" {
		t.Errorf("Host.ID assignment via embed must work")
	}
	if item.PersistentVolumeName != "claude-state-acct-42" {
		t.Errorf("PersistentVolumeName field must be string-typed")
	}
}
```

**verbatim 字段守恒：**
- 类型名 `HostWithClaudeAccount`
- SQL 常量名 `getHostWithClaudeAccountSQL` / `lockClaudeAccountForDeleteSQL` / `deleteClaudeAccountSQL`
- 函数 `GetHostWithClaudeAccount` (method on Repository) / `BeginTx` (method on Repository) / `LockClaudeAccountForDelete` (package func) / `DeleteClaudeAccountTx` (package func)
- SQL 关键 token：`LEFT JOIN claude_accounts ca ON ca.host_id = h.id` / `FOR UPDATE` / `LIMIT 1` / `ORDER BY ca.created_at ASC` / `COALESCE(ca.persistent_volume_name, '')`

**禁止：**
- 修改 GetHost / ListHostsWithUsername 等 Phase 29.1 已锁定的 6 个 SELECT
- 把 `*pgxpool.Pool` 类型泄漏到非 repository 包
- 在 `LockClaudeAccountForDelete` 内做 row count 校验（pgx.ErrNoRows 由 Scan 自动返回）
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "type HostWithClaudeAccount struct" internal/store/repository/models.go
grep -q "PersistentVolumeName string" internal/store/repository/models.go
grep -q "const getHostWithClaudeAccountSQL" internal/store/repository/queries.go
grep -q "LEFT JOIN claude_accounts ca ON ca.host_id = h.id" internal/store/repository/queries.go
grep -q "func (r \\*Repository) GetHostWithClaudeAccount" internal/store/repository/queries.go
grep -q "func (r \\*Repository) BeginTx" internal/store/repository/queries.go
grep -q "func LockClaudeAccountForDelete" internal/store/repository/queries.go
grep -q "func DeleteClaudeAccountTx" internal/store/repository/queries.go
grep -q "FOR UPDATE" internal/store/repository/queries.go
test -f internal/store/repository/queries_claude_account_delete_test.go
go vet ./internal/store/repository/...
go test ./internal/store/repository/ -run "TestGetHostWithClaudeAccountSQL|TestLockClaudeAccountForDeleteSQL|TestDeleteClaudeAccountSQL|TestHostWithClaudeAccount_EmbedsHost" -count=1 -v
go build ./...
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "^const getHostWithClaudeAccountSQL\|^const lockClaudeAccountForDeleteSQL\|^const deleteClaudeAccountSQL" internal/store/repository/queries.go` 输出 = 3
    - `grep -c "^func (r \*Repository) GetHostWithClaudeAccount\|^func (r \*Repository) BeginTx\|^func LockClaudeAccountForDelete\|^func DeleteClaudeAccountTx" internal/store/repository/queries.go` 输出 = 4
    - `grep -c "type HostWithClaudeAccount struct" internal/store/repository/models.go` 输出 = 1
    - `grep -c "FOR UPDATE" internal/store/repository/queries.go` ≥ 1（行锁存在）
    - 4 条新增 SQL/类型单测全 PASS
    - `go build ./...` 退出码 = 0
    - 既有仓储测试无回归：`go test ./internal/store/repository/ -count=1` 退出码 = 0
  </acceptance_criteria>

  <done>
    - 5 个新增仓储符号（`HostWithClaudeAccount` 类型 + `GetHostWithClaudeAccount` method + `BeginTx` method + `LockClaudeAccountForDelete` func + `DeleteClaudeAccountTx` func）就位
    - 3 个新增 SQL 常量提升为包级 const，关键 token grep 可断言
    - 4 条新增单测全 PASS
    - `go build ./...` PASS
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2.2: 新建 admin_claude_accounts.go handler（强一致 + force 两条路径） + 完整 handler 单测（D-17/D-18/D-19/D-20/D-21 / D-25.5）</name>
  <files>internal/controlplane/http/admin_claude_accounts.go, internal/controlplane/http/admin_claude_accounts_test.go</files>

  <read_first>
    - internal/controlplane/http/admin_hosts.go（特别是 line 25-46 AdminHostStore + AdminHostsHandler 模板 / line 350-419 RotateSSHPassword 风格 / line 421-510 ResyncPasswords / line 1012-1023 var syncContainerPassword 包级 var 注入模式）
    - internal/controlplane/http/router.go（特别是 line 27-54 Dependencies + line 232-256 AdminHosts 注册块 + line 207-256 adminGuard 链）
    - internal/controlplane/http/admin_users_test.go（line 79-117 stubEventRecorder + validAdminToken + adminTestRouter）
    - internal/controlplane/http/admin_hosts_test.go（line 21-66 stubHostStore 模板）
    - internal/agentapi/client.go（line 15-30 Client struct + line 73 RunHostAction 签名）
    - internal/store/repository/queries.go（Plan 02 Task 2.1 落地的 BeginTx / LockClaudeAccountForDelete / DeleteClaudeAccountTx）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-17/D-18/D-19/D-20/D-21
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.5 完整 handler 骨架（注意末尾 var _ = json.Marshal 占位需删除）+ §5.2 stub Tx 样板
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `admin_claude_accounts.go` + `admin_claude_accounts_test.go` 节 + Shared 1/3/4/5
    - .planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md Plan 02 admin handler audit event 模式 + metadata 不写凭据原则
  </read_first>

  <behavior>
    - Test 1 (TestAdminClaudeAccountsDelete_StrictSuccess_DBDeletedAndAuditEventEmitted)：mock host-agent 调用成功 → DB DELETE → COMMIT → audit `claude_account.deleted` → HTTP 200 + body `{"deleted":true,"volume_rm":"succeeded"}`
    - Test 2 (TestAdminClaudeAccountsDelete_StrictHostAgentFailure_RollbackAnd409WithChineseMessage)：mock host-agent 返回 error → tx.Rollback 调用 + audit `claude_account.delete_volume_rm_failed` → HTTP 409 + body `error.code == "STATE_VOLUME_IN_USE_001"` + 中文 message 含 "请先停止使用该账号的所有 host 后重试"
    - Test 3 (TestAdminClaudeAccountsDelete_ForceTrue_DBDeletedEvenWhenRmFails)：query `?force=true` + mock host-agent 返回 error → DB COMMIT 已发生 + audit `claude_account.force_volume_rm_failed` → HTTP 200 + body `volume_rm == "failed"` + `next_action` 含 `"docker volume rm -f"`
    - Test 4 (TestAdminClaudeAccountsDelete_AccountNotFound_404)：mock LockClaudeAccountForDelete 返回 pgx.ErrNoRows → HTTP 404 + body `error == "claude_account not found"`
    - Test 5 (TestAdminClaudeAccountsDelete_NoVolumeName_SkipsHostAgentCall)：mock 返回 volumeName="" → host-agent 不被调用 → DB DELETE → HTTP 200
    - Test 6 (TestAdminClaudeAccountsDelete_StrictUsesTenSecondTimeout)：捕获 ctx.Deadline()，断言 ≤ 10.1s 且 > 9.9s（D-20 强一致 10s）
    - Test 7 (TestAdminClaudeAccountsDelete_ForceUsesThirtySecondTimeout)：同上断言 ≤ 30.1s 且 > 29.9s（D-20 force 30s）
    - Test 8 (TestParseForceFlag_AcceptsTrueOneYes)：`parseForceFlag` 对 `"true"`/`"1"`/`"yes"` 返回 true，其它返回 false
  </behavior>

  <action>
**(a) 新建 `internal/controlplane/http/admin_claude_accounts.go`**（**改造自 RESEARCH §2.5；删除占位 `var _ = json.Marshal` 与无用 `encoding/json` import**）：

```go
package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// AdminClaudeAccountStore 暴露 Plan 02 仅需的最小集（与 AdminHostStore 风格一致）。
type AdminClaudeAccountStore interface {
	BeginTx(ctx context.Context) (pgx.Tx, error)
}

// runHostAction 包级 var 便于测试注入 mock（沿用 syncContainerPassword 模式 admin_hosts.go:1014）。
var runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	return client.RunHostAction(ctx, req)
}

type AdminClaudeAccountsHandler struct {
	logger      *slog.Logger
	store       AdminClaudeAccountStore
	agentClient *agentapi.Client
	events      EventRecorder
}

func NewAdminClaudeAccountsHandler(logger *slog.Logger, store AdminClaudeAccountStore, agentClient *agentapi.Client, events EventRecorder) *AdminClaudeAccountsHandler {
	return &AdminClaudeAccountsHandler{logger: logger, store: store, agentClient: agentClient, events: events}
}

func (h *AdminClaudeAccountsHandler) Delete() nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		accountID := r.PathValue("accountID")
		if parseForceFlag(r.URL.Query().Get("force")) {
			h.deleteForce(w, r, accountID)
			return
		}
		h.deleteStrict(w, r, accountID)
	})
}

func parseForceFlag(s string) bool {
	switch s {
	case "true", "1", "yes":
		return true
	}
	return false
}

// deleteStrict D-18 强一致路径：BEGIN → SELECT FOR UPDATE → 调 host-agent → 成功 DELETE+COMMIT；失败 ROLLBACK + 409。
func (h *AdminClaudeAccountsHandler) deleteStrict(w nethttp.ResponseWriter, r *nethttp.Request, accountID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second) // D-20 强一致
	defer cancel()

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.logger.Error("begin tx failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "begin tx failed"})
		return
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback(context.Background())
		}
	}()

	id, volumeName, err := repository.LockClaudeAccountForDelete(ctx, tx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "claude_account not found"})
			return
		}
		h.logger.Error("lock claude_account failed", "id", accountID, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "lock claude_account failed"})
		return
	}

	if volumeName != "" {
		req := agentapi.HostActionRequest{
			Action:  agentapi.ActionVolumeRemove,
			Volumes: []agentapi.VolumeMount{{Name: volumeName}},
		}
		if _, err := runHostAction(ctx, h.agentClient, req); err != nil {
			h.recordEvent(r.Context(), "claude_account.delete_volume_rm_failed", map[string]any{
				"account_id":    id,
				"volume_name":   volumeName,
				"error_code":    "volume_in_use",
				"error_message": err.Error(),
			})
			writeJSON(w, nethttp.StatusConflict, map[string]any{
				"error": map[string]string{
					"code":        "STATE_VOLUME_IN_USE_001",
					"message":     "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume",
					"next_action": "停止 host → 重试 DELETE，或附加 ?force=true",
				},
			})
			return
		}
	}

	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		h.logger.Error("delete claude_account failed", "id", id, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete claude_account failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("commit failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}
	rollback = false

	h.recordEvent(r.Context(), "claude_account.deleted", map[string]any{
		"account_id":  id,
		"volume_name": volumeName,
		"force":       false,
	})
	writeJSON(w, nethttp.StatusOK, map[string]any{
		"deleted":   true,
		"volume_rm": "succeeded",
	})
}

// deleteForce D-19 最终一致路径：DB 先 COMMIT；rm 失败仅写 audit + 返回 200。
func (h *AdminClaudeAccountsHandler) deleteForce(w nethttp.ResponseWriter, r *nethttp.Request, accountID string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // D-20 force
	defer cancel()

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
		h.logger.Error("begin tx failed (force)", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "begin tx failed"})
		return
	}
	id, volumeName, err := repository.LockClaudeAccountForDelete(ctx, tx, accountID)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, nethttp.StatusNotFound, map[string]string{"error": "claude_account not found"})
			return
		}
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "lock claude_account failed"})
		return
	}
	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		_ = tx.Rollback(ctx)
		h.logger.Error("delete claude_account failed (force)", "id", id, "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete claude_account failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("commit failed (force)", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	resp := map[string]any{"deleted": true, "volume_rm": "skipped"}
	if volumeName != "" {
		req := agentapi.HostActionRequest{
			Action:  agentapi.ActionVolumeRemove,
			Volumes: []agentapi.VolumeMount{{Name: volumeName}},
			Labels:  map[string]string{"force": "true"},
		}
		if _, err := runHostAction(ctx, h.agentClient, req); err != nil {
			h.recordEvent(r.Context(), "claude_account.force_volume_rm_failed", map[string]any{
				"account_id":    id,
				"volume_name":   volumeName,
				"error_message": err.Error(),
			})
			resp["volume_rm"] = "failed"
			resp["next_action"] = "运维需手工 docker volume rm -f " + volumeName
		} else {
			resp["volume_rm"] = "succeeded"
		}
	}
	h.recordEvent(r.Context(), "claude_account.deleted", map[string]any{
		"account_id":  id,
		"volume_name": volumeName,
		"force":       true,
	})
	writeJSON(w, nethttp.StatusOK, resp)
}

func (h *AdminClaudeAccountsHandler) recordEvent(ctx context.Context, eventType string, metadata map[string]any) {
	if h.events == nil {
		return
	}
	if _, err := h.events.RecordEvent(ctx, repository.RecordEventParams{
		Level:    "info",
		Type:     eventType,
		Message:  "管理员删除 Claude 账号",
		Metadata: metadata,
	}); err != nil {
		h.logger.Error("record event failed", "type", eventType, "error", err)
	}
}
```

**(b) 新建 `internal/controlplane/http/admin_claude_accounts_test.go`**（手写最小 stub Tx，**禁止引入 pgxmock 依赖** — 与现有测试风格一致）：

```go
package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// stubTx 实现 pgx.Tx 最小接口（仅 handler 实际使用的 4 方法）；其余 panic 以提示设计偏差。
type stubTx struct {
	scanResults  []any // QueryRow.Scan 顺序填充
	queryRowErr  error
	execAffected int64
	execErr      error
	committed    bool
	rolledback   bool
}

type stubRow struct {
	results []any
	err     error
}

func (r *stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.results) {
			return errors.New("stubRow: not enough results")
		}
		switch d := d.(type) {
		case *string:
			*d = r.results[i].(string)
		default:
			return errors.New("stubRow: unsupported dest type")
		}
	}
	return nil
}

func (s *stubTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &stubRow{results: s.scanResults, err: s.queryRowErr}
}
func (s *stubTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	if s.execErr != nil {
		return pgconn.CommandTag{}, s.execErr
	}
	return pgconn.NewCommandTag("DELETE " + itoa(s.execAffected)), nil
}
func (s *stubTx) Commit(_ context.Context) error   { s.committed = true; return nil }
func (s *stubTx) Rollback(_ context.Context) error { s.rolledback = true; return nil }

// 未使用方法 — panic 以便发现新增 handler 调用
func (s *stubTx) Begin(_ context.Context) (pgx.Tx, error) { panic("stubTx.Begin not implemented") }
func (s *stubTx) BeginFunc(_ context.Context, _ func(pgx.Tx) error) error {
	panic("stubTx.BeginFunc not implemented")
}
func (s *stubTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	panic("stubTx.CopyFrom not implemented")
}
func (s *stubTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults {
	panic("stubTx.SendBatch not implemented")
}
func (s *stubTx) LargeObjects() pgx.LargeObjects { panic("stubTx.LargeObjects not implemented") }
func (s *stubTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	panic("stubTx.Prepare not implemented")
}
func (s *stubTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("stubTx.Query not implemented")
}
func (s *stubTx) Conn() *pgx.Conn { panic("stubTx.Conn not implemented") }

// itoa 轻量 helper（避免 import strconv 仅 for one int）
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type stubAdminClaudeAccountStore struct {
	tx        *stubTx
	beginErr  error
	beginCnt  int
	beginCtx  context.Context
}

func (s *stubAdminClaudeAccountStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	s.beginCnt++
	s.beginCtx = ctx
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return s.tx, nil
}

func newAdminClaudeAccountsTestRouter(t *testing.T, store AdminClaudeAccountStore, events EventRecorder) (nethttp.Handler, func()) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewAdminClaudeAccountsHandler(logger, store, nil, events)
	mux := nethttp.NewServeMux()
	authMw := func(next nethttp.Handler) nethttp.Handler { return next } // 测试场景跳过 admin auth；下游 router 注册才走 adminGuard
	mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", authMw(handler.Delete()))
	return mux, func() {}
}

func TestAdminClaudeAccountsDelete_StrictSuccess_DBDeletedAndAuditEventEmitted(t *testing.T) {
	tx := &stubTx{
		scanResults:  []any{"acct-1", "claude-state-acct-1"},
		execAffected: 1,
	}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		if req.Action != agentapi.ActionVolumeRemove {
			t.Fatalf("expected ActionVolumeRemove, got %q", req.Action)
		}
		if len(req.Volumes) != 1 || req.Volumes[0].Name != "claude-state-acct-1" {
			t.Fatalf("expected single volume claude-state-acct-1, got %+v", req.Volumes)
		}
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Errorf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.committed {
		t.Error("tx must be committed on success")
	}
	if !events.hasType("claude_account.deleted") {
		t.Error("audit event claude_account.deleted must be emitted")
	}
}

func TestAdminClaudeAccountsDelete_StrictHostAgentFailure_RollbackAnd409WithChineseMessage(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, errors.New("volume_in_use: stuck on container_xyz")
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusConflict {
		t.Fatalf("want 409, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.rolledback {
		t.Error("tx must be rolled back on host-agent failure")
	}
	if !events.hasType("claude_account.delete_volume_rm_failed") {
		t.Error("audit event claude_account.delete_volume_rm_failed must be emitted")
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body must be JSON: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "STATE_VOLUME_IN_USE_001" {
		t.Errorf("error.code must be STATE_VOLUME_IN_USE_001, got %v", errObj["code"])
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "请先停止使用该账号的所有 host 后重试") {
		t.Errorf("error.message must contain Chinese guidance, got %q", msg)
	}
}

func TestAdminClaudeAccountsDelete_ForceTrue_DBDeletedEvenWhenRmFails(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}

	origRun := runHostAction
	var capturedLabels map[string]string
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		capturedLabels = req.Labels
		return agentapi.HostActionResponse{}, errors.New("daemon connection refused")
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1?force=true", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Fatalf("force=true must return 200 even when rm fails, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !tx.committed {
		t.Error("tx must be committed (DB delete first) in force path")
	}
	if capturedLabels["force"] != "true" {
		t.Errorf("force label must be propagated to host-agent, got %v", capturedLabels)
	}
	if !events.hasType("claude_account.force_volume_rm_failed") {
		t.Error("audit event claude_account.force_volume_rm_failed must be emitted")
	}

	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["volume_rm"] != "failed" {
		t.Errorf("volume_rm must be \"failed\", got %v", body["volume_rm"])
	}
	if na, _ := body["next_action"].(string); !strings.Contains(na, "docker volume rm -f") {
		t.Errorf("next_action must hint docker volume rm -f, got %q", na)
	}
}

func TestAdminClaudeAccountsDelete_AccountNotFound_404(t *testing.T) {
	tx := &stubTx{queryRowErr: pgx.ErrNoRows}
	store := &stubAdminClaudeAccountStore{tx: tx}
	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/missing", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != nethttp.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestAdminClaudeAccountsDelete_NoVolumeName_SkipsHostAgentCall(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", ""}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	events := &stubEventRecorder{}
	called := false
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		called = true
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, events)
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != nethttp.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if called {
		t.Error("host-agent must NOT be called when volume_name is empty")
	}
}

func TestAdminClaudeAccountsDelete_StrictUsesTenSecondTimeout(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if store.beginCtx == nil {
		t.Fatal("BeginTx must be called")
	}
	deadline, ok := store.beginCtx.Deadline()
	if !ok {
		t.Fatal("strict path must have deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 10*time.Second+100*time.Millisecond || remaining < 9*time.Second {
		t.Errorf("strict timeout must be ~10s, got %v", remaining)
	}
}

func TestAdminClaudeAccountsDelete_ForceUsesThirtySecondTimeout(t *testing.T) {
	tx := &stubTx{scanResults: []any{"acct-1", "claude-state-acct-1"}, execAffected: 1}
	store := &stubAdminClaudeAccountStore{tx: tx}
	origRun := runHostAction
	runHostAction = func(ctx context.Context, client *agentapi.Client, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
		return agentapi.HostActionResponse{}, nil
	}
	t.Cleanup(func() { runHostAction = origRun })

	mux, _ := newAdminClaudeAccountsTestRouter(t, store, &stubEventRecorder{})
	req := httptest.NewRequest(nethttp.MethodDelete, "/v1/admin/claude-accounts/acct-1?force=true", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	deadline, ok := store.beginCtx.Deadline()
	if !ok {
		t.Fatal("force path must have deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 30*time.Second+100*time.Millisecond || remaining < 29*time.Second {
		t.Errorf("force timeout must be ~30s, got %v", remaining)
	}
}

func TestParseForceFlag_AcceptsTrueOneYes(t *testing.T) {
	cases := map[string]bool{"true": true, "1": true, "yes": true, "false": false, "": false, "TRUE": false}
	for s, want := range cases {
		if got := parseForceFlag(s); got != want {
			t.Errorf("parseForceFlag(%q): got %v, want %v", s, got, want)
		}
	}
}
```

> 注意 stubTx 必须 import `github.com/jackc/pgx/v5/pgconn`（CommandTag 类型来源）；该包已在仓库 indirect deps 中（pgx v5 自带），无需 go get。

**verbatim 字段守恒：**
- 文件名 `admin_claude_accounts.go` / `admin_claude_accounts_test.go`
- 接口名 `AdminClaudeAccountStore` / 唯一方法 `BeginTx(ctx context.Context) (pgx.Tx, error)`
- handler 类型 `AdminClaudeAccountsHandler` / 构造器 `NewAdminClaudeAccountsHandler` 签名 `(logger, store, agentClient, events)`
- 包级 var `runHostAction = func(ctx, client, req) (HostActionResponse, error)`（沿用 admin_hosts.go:1014 syncContainerPassword 模式）
- 路由 `DELETE /v1/admin/claude-accounts/{accountID}` 路径 + 路径参数名 `accountID`
- query 参数 `force=true|1|yes`（CONTEXT Discretion 第 5 条三种均接受）
- ctx 超时 `10*time.Second` 强一致 / `30*time.Second` force（D-20）
- 错误码 `STATE_VOLUME_IN_USE_001` + 中文消息原文 "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume"
- audit event Type `claude_account.delete_volume_rm_failed` / `claude_account.force_volume_rm_failed` / `claude_account.deleted`
- metadata key 白名单：`account_id` / `volume_name` / `error_code` / `error_message` / `force`（**禁止** `email` / OAuth token）
- 错误响应 body 形态 `{"error": {"code": ..., "message": ..., "next_action": ...}}`
- 成功响应 body：strict `{"deleted":true,"volume_rm":"succeeded"}` / force-fail `{"deleted":true,"volume_rm":"failed","next_action":"运维需手工 docker volume rm -f <name>"}` / force-succeed `{"deleted":true,"volume_rm":"succeeded"}` / no-volume `{"deleted":true,"volume_rm":"skipped"}`

**禁止：**
- 引入 `pashagolub/pgxmock` 依赖（用手写 stubTx，CONTEXT D-29 风格）
- 在 handler 内做 service / use case 抽象（D-29）
- audit metadata 写 OAuth token / EntryPassword / email（PATTERNS Shared 3）
- 直接传 `r.Context()` 给 `runHostAction`（必须先 `WithTimeout`）
- 改用 `nethttp.Error`（统一走 `writeJSON`）
  </action>

  <verify>
    <automated>
bash -c 'set -e
test -f internal/controlplane/http/admin_claude_accounts.go
test -f internal/controlplane/http/admin_claude_accounts_test.go
grep -q "type AdminClaudeAccountStore interface" internal/controlplane/http/admin_claude_accounts.go
grep -q "BeginTx(ctx context.Context) (pgx.Tx, error)" internal/controlplane/http/admin_claude_accounts.go
grep -q "var runHostAction = func" internal/controlplane/http/admin_claude_accounts.go
grep -q "STATE_VOLUME_IN_USE_001" internal/controlplane/http/admin_claude_accounts.go
grep -q "请先停止使用该账号的所有 host 后重试" internal/controlplane/http/admin_claude_accounts.go
grep -q "claude_account.delete_volume_rm_failed" internal/controlplane/http/admin_claude_accounts.go
grep -q "claude_account.force_volume_rm_failed" internal/controlplane/http/admin_claude_accounts.go
grep -q "claude_account.deleted" internal/controlplane/http/admin_claude_accounts.go
grep -q "10\\*time.Second" internal/controlplane/http/admin_claude_accounts.go
grep -q "30\\*time.Second" internal/controlplane/http/admin_claude_accounts.go
# audit metadata 白名单
! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\"" internal/controlplane/http/admin_claude_accounts.go
go vet ./internal/controlplane/http/...
go test ./internal/controlplane/http/ -run "TestAdminClaudeAccountsDelete|TestParseForceFlag" -count=1 -v
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - 8 条 handler 单测全 PASS
    - `grep -c "STATE_VOLUME_IN_USE_001" internal/controlplane/http/admin_claude_accounts.go` 输出 = 1
    - `grep -c "请先停止使用该账号的所有 host 后重试" internal/controlplane/http/admin_claude_accounts.go` 输出 = 1
    - `grep -c "claude_account\\." internal/controlplane/http/admin_claude_accounts.go` ≥ 3（三种事件类型）
    - `grep -cE "10\*time.Second|30\*time.Second" internal/controlplane/http/admin_claude_accounts.go` = 2（D-20 双超时）
    - audit metadata 白名单守恒：grep `Metadata:.*"(email|entry_password|credentials|oauth_token)"` 无命中
    - 不引入新依赖：`go mod tidy` 后 go.mod / go.sum diff 仅含 pgconn（如有）已存在条目
    - `go build ./...` 退出码 = 0
  </acceptance_criteria>

  <done>
    - `admin_claude_accounts.go` 含强一致 + force 两条路径 + parseForceFlag + recordEvent helper
    - 8 条 handler 单测覆盖：成功 / 409+rollback / force+rm失败 / 404 / 无 volume / 10s 超时 / 30s 超时 / parseForceFlag 边界
    - `go build ./...` 与 `go vet ./internal/controlplane/http/...` PASS
    - audit metadata 白名单 grep 守恒
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2.3: admin_hosts.go GET detail 追加 persistent_volume_name 字段（OOS-A19 / D-22）</name>
  <files>internal/controlplane/http/admin_hosts.go, internal/controlplane/http/admin_hosts_test.go</files>

  <read_first>
    - internal/controlplane/http/admin_hosts.go（特别是 line 25-46 AdminHostStore 接口 + line 96-99 adminHostDetailResponse + line 101-148 Get() handler）
    - internal/controlplane/http/admin_hosts_test.go（Get handler 既有测试与 stubHostStore）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-22/D-23/D-24
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.7
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `admin_hosts.go` 节
    - Plan 02 Task 2.1 落地的 `Repository.GetHostWithClaudeAccount`
  </read_first>

  <behavior>
    - Test 1 (TestAdminHostDetail_IncludesPersistentVolumeName_WhenAvailable)：mock store.GetHostWithClaudeAccount 返回 PersistentVolumeName="claude-state-acct-1" → 响应 JSON 顶层含 `"persistent_volume_name":"claude-state-acct-1"`
    - Test 2 (TestAdminHostDetail_OmitsPersistentVolumeName_WhenEmpty)：mock 返回空串 → 响应 JSON **不**含 `persistent_volume_name` 键（omitempty 生效）
    - Test 3 (TestAdminHostList_DoesNotIncludePersistentVolumeName)：list endpoint 响应 JSON **不**含 `persistent_volume_name`（OOS-A19 list 不动）
  </behavior>

  <action>
**(a) 在 `internal/controlplane/http/admin_hosts.go` 的 `AdminHostStore` 接口（line 25-35）追加一行方法：**

```go
type AdminHostStore interface {
    ListHostsWithUsername(context.Context) ([]repository.HostWithUsername, error)
    GetHostDetail(context.Context, string) (repository.HostDetail, error)
    GetHost(context.Context, string) (repository.Host, error)
    UpsertHost(context.Context, repository.UpsertHostParams) (repository.Host, error)
    GetUser(context.Context, string) (repository.User, error)
    BindEgressIPToHost(context.Context, string, string) (repository.HostBinding, error)
    DeleteHost(context.Context, string) error
    UpdateHostEntryPassword(context.Context, string, string) error
    ListRunningHosts(ctx context.Context) ([]repository.Host, error)
    GetHostWithClaudeAccount(ctx context.Context, hostID string) (repository.HostWithClaudeAccount, error) // Phase 33 D-22
}
```

**(b) 把 `adminHostDetailResponse`（line 96-99）改为：**

```go
type adminHostDetailResponse struct {
    repository.HostDetail
    ConnectionInfo       *repository.ConnectionInfo `json:"connection_info,omitempty"`
    PersistentVolumeName string                     `json:"persistent_volume_name,omitempty"` // Phase 33 D-22
}
```

**(c) 在 `Get()` handler（line 101-148）的 `resp := adminHostDetailResponse{HostDetail: detail}` 之后、`resp.Host.EntryPassword = ""` 之前追加 enrich 块：**

```go
// Phase 33 D-22：从 LEFT JOIN 取 persistent_volume_name，失败仅记日志不影响 detail 主路径。
if hostWithCA, err := h.store.GetHostWithClaudeAccount(r.Context(), hostID); err == nil {
    resp.PersistentVolumeName = hostWithCA.PersistentVolumeName
} else if !errors.Is(err, pgx.ErrNoRows) {
    h.logger.Warn("get host with claude_account failed (degraded)", "host_id", hostID, "error", err)
}
```

> 设计理由：失败容忍 — admin host detail 主路径不能因 LEFT JOIN 失败而 5xx；走 omitempty 让前端老二进制感知不到字段缺失。

**(d) 更新 `admin_hosts_test.go` 中的 `stubHostStore` 实现新方法（必须，否则接口不完整无法编译）。**

在 stubHostStore struct 内追加字段：
```go
hostWithCA    repository.HostWithClaudeAccount
hostWithCAErr error
```

并实现方法：
```go
func (s *stubHostStore) GetHostWithClaudeAccount(_ context.Context, _ string) (repository.HostWithClaudeAccount, error) {
    return s.hostWithCA, s.hostWithCAErr
}
```

**(e) 在 `admin_hosts_test.go` 末尾追加 3 条新增测试：**

```go
func TestAdminHostDetail_IncludesPersistentVolumeName_WhenAvailable(t *testing.T) {
    store := newStubHostStoreWithDefaults(t) // 复用既有 helper（若不存在则 inline 构造）
    store.hostWithCA = repository.HostWithClaudeAccount{
        Host:                 repository.Host{ID: "h-1"},
        PersistentVolumeName: "claude-state-acct-1",
    }
    // ... build router + admin token (复用 adminTestRouter / validAdminToken)
    // 发 GET /v1/admin/hosts/h-1 → 断言 body JSON 含 "persistent_volume_name":"claude-state-acct-1"
}

func TestAdminHostDetail_OmitsPersistentVolumeName_WhenEmpty(t *testing.T) {
    store := newStubHostStoreWithDefaults(t)
    store.hostWithCA = repository.HostWithClaudeAccount{Host: repository.Host{ID: "h-1"}, PersistentVolumeName: ""}
    // 断言 body **不**含 "persistent_volume_name" 键（json.Unmarshal map 后 _, ok := body["persistent_volume_name"]; ok 必须 false）
}

func TestAdminHostList_DoesNotIncludePersistentVolumeName(t *testing.T) {
    // GET /v1/admin/hosts → 断言响应 hosts[0] map 不含 "persistent_volume_name" 键（OOS-A19）
}
```

> 实际填充 helper 与 admin token 部分由 executor 按 `admin_hosts_test.go` 既有风格补全；关键是断言行为而非结构。

**verbatim 字段守恒：**
- 字段名 `PersistentVolumeName string`
- JSON tag `persistent_volume_name,omitempty`
- 行内注释 `// Phase 33 D-22`
- 接口方法签名 `GetHostWithClaudeAccount(ctx context.Context, hostID string) (repository.HostWithClaudeAccount, error)`

**禁止：**
- 改 `List()` 方法引入 GetHostWithClaudeAccount（OOS-A19 list 不动）
- 把 GetHostWithClaudeAccount 失败转成 5xx（必须降级仅记 Warn）
- 改 `repository.HostDetail` 类型（破坏其它 handler 兼容性）
- 在 Get() 内引入 docker exec（PATTERNS warning：detail handler 必须纯 DB JOIN）
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "GetHostWithClaudeAccount(ctx context.Context, hostID string) (repository.HostWithClaudeAccount, error)" internal/controlplane/http/admin_hosts.go
grep -qE "PersistentVolumeName[[:space:]]+string" internal/controlplane/http/admin_hosts.go
grep -q '`json:"persistent_volume_name,omitempty"`' internal/controlplane/http/admin_hosts.go
grep -q "h.store.GetHostWithClaudeAccount" internal/controlplane/http/admin_hosts.go
# List endpoint 响应 struct 未受影响（HostWithUsername 字段集不变）
go vet ./internal/controlplane/http/...
go test ./internal/controlplane/http/ -run "TestAdminHostDetail_Includes|TestAdminHostDetail_Omits|TestAdminHostList_DoesNot" -count=1 -v
go test ./internal/controlplane/http/ -count=1 -short  # 既有 admin_hosts_test 全 PASS（接口契约闭环）
go build ./...
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "GetHostWithClaudeAccount(ctx context.Context, hostID string)" internal/controlplane/http/admin_hosts.go` 输出 = 1
    - `grep -c "PersistentVolumeName string" internal/controlplane/http/admin_hosts.go` 输出 = 1
    - `grep -c "h.store.GetHostWithClaudeAccount" internal/controlplane/http/admin_hosts.go` 输出 = 1（仅 Get() 调用）
    - `grep -c "persistent_volume_name" internal/controlplane/http/admin_hosts.go` ≤ 2（仅 detail response struct + 注释）
    - 3 条新增测试全 PASS
    - 既有 admin_hosts_test 无回归
    - `go build ./...` PASS
  </acceptance_criteria>

  <done>
    - `AdminHostStore` 接口扩展 + `adminHostDetailResponse` 字段追加 + `Get()` handler enrich 块 + stubHostStore 同步实现
    - 3 条新增 detail/list 测试覆盖 OOS-A19 边界
    - `go build ./...` PASS（接口契约闭环：Repository 已在 Plan 02 Task 2.1 实现 GetHostWithClaudeAccount）
  </done>
</task>

<task type="auto">
  <name>Task 2.4: router.go 注册 DELETE /v1/admin/claude-accounts/{accountID} + Dependencies 字段扩展</name>
  <files>internal/controlplane/http/router.go</files>

  <read_first>
    - internal/controlplane/http/router.go（特别是 line 27-54 Dependencies + line 232-256 AdminHosts 注册块 + line 300 mux 返回点）
    - internal/agentapi/client.go（line 15-30 Client struct）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-17
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.6
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `router.go` 节
  </read_first>

  <action>
**(a) 在 `Dependencies` struct（line 27-54）追加两个字段：**

```go
type Dependencies struct {
    // ... existing fields
    AdminClaudeAccounts AdminClaudeAccountStore // Phase 33 D-17
    AgentClient         *agentapi.Client        // Phase 33 D-17
}
```

字段插入位置：紧跟 `AdminHosts AdminHostStore` 之后，保持 admin 字段集中。

**(b) 在 `if deps.AdminHosts != nil { ... }` 块（line 232-256）之后追加：**

```go
if deps.AdminClaudeAccounts != nil {
    claudeHandler := NewAdminClaudeAccountsHandler(deps.Logger, deps.AdminClaudeAccounts, deps.AgentClient, deps.EventRecorder)
    mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", adminGuard(claudeHandler.Delete()))
}
```

**verbatim 字段守恒：**
- Dependencies 字段名 `AdminClaudeAccounts` / `AgentClient`
- 路由 `DELETE /v1/admin/claude-accounts/{accountID}` 路径 + 路径参数 `accountID`
- adminGuard 中间件链（与 AdminHosts 对称）
- if-deps-nil 守卫模式（与 AdminHosts/AdminEvents 风格一致）

**禁止：**
- 不带 adminGuard 注册（必须 admin role 鉴权）
- 在 router.go 内构造 agentapi.Client（由调用方注入）
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "AdminClaudeAccounts AdminClaudeAccountStore" internal/controlplane/http/router.go
grep -q "AgentClient         \\*agentapi.Client" internal/controlplane/http/router.go
grep -q "if deps.AdminClaudeAccounts != nil {" internal/controlplane/http/router.go
grep -q "DELETE /v1/admin/claude-accounts/{accountID}" internal/controlplane/http/router.go
grep -q "adminGuard(claudeHandler.Delete())" internal/controlplane/http/router.go
go vet ./internal/controlplane/http/...
go build ./...
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "AdminClaudeAccounts AdminClaudeAccountStore" internal/controlplane/http/router.go` 输出 = 1
    - `grep -c "AgentClient         \*agentapi.Client" internal/controlplane/http/router.go` 输出 = 1
    - `grep -c "DELETE /v1/admin/claude-accounts/{accountID}" internal/controlplane/http/router.go` 输出 = 1
    - `grep -c "adminGuard(claudeHandler.Delete())" internal/controlplane/http/router.go` 输出 = 1（adminGuard 链路就位）
    - `go build ./...` 退出码 = 0
    - 既有 router 测试无回归
  </acceptance_criteria>

  <done>
    - Dependencies 扩展 2 字段、路由注册块就位
    - `go build ./...` PASS
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 2.5: 人工 UAT — D-26 五步 + ROADMAP §Phase 33 SC1/SC4 端到端验收</name>
  <files>（无文件修改 — 端到端 UAT 任务）</files>

  <action>
**checkpoint 类型：human-verify** — 全部代码已自动化落地（Task 2.1-2.4），本 task 由 executor 把下方 `<how-to-verify>` 步骤完整呈现给用户/运维，等待 `<resume-signal>` 确认后才视为完成。**禁止**自动模拟通过；**禁止**跳过任一步骤；如某步无法在当前环境复现，必须明确写"environment skip: <reason>"而不是默认 approved。
  </action>

  <verify>
    <automated>echo "checkpoint:human-verify task — verification gated by user resume-signal (no automated check)"</automated>
  </verify>

  <done>
    - 用户回复 `approved` 或 `approved with notes: ...`
    - 7 个 Step 全部明确 PASS / NOTED / SKIP（无未确认状态）
    - 实测输出（curl / docker 命令的关键 stdout 截取）已纳入 33-02-SUMMARY.md
  </done>

  <what-built>
    - Plan 01 + Plan 02 全部代码已落地（entrypoint symlink + worker 自动补 volume + agentapi ActionVolumeRemove + admin DELETE 强/最终一致两条路径 + admin host detail 字段）
    - 所有单元测试已 PASS
  </what-built>

  <how-to-verify>
**前提：** 至少 1 个测试环境运行中：control-plane + host-agent + 受管 v3 镜像（Phase 29 镜像 ≥ v3.0.0）；准备一个测试 user + claude_account（有 host 关联）。

**Step 1 (D-26.1): 删除一个未运行 host 的 account → volume 必须被清理**

1. 创建测试 account：通过既有 SQL 直插或现有 admin 接口创建一个 `claude_account`，关联 user 但不关联 host
2. 触发 worker createHost（通过 control-plane 发起 host 创建任务，传 `ClaudeAccountID`）→ 等待 host 状态 = `running` → 此时 volume `claude-state-{id}` 存在
3. 停止 host：`docker rm -f cloudproxy-<host_id>`
4. 调 `DELETE /v1/admin/claude-accounts/{accountID}`（强一致路径，无 force）
5. **断言：** HTTP 200 + body `{"deleted":true,"volume_rm":"succeeded"}`
6. **断言：** `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 输出为空（除头行外无数据行）

**Step 2 (D-26.2): 删除一个有运行 host 的 account（默认 force=false）→ HTTP 409 + DB 行仍在**

1. 同 Step 1 但**不**停 host
2. 调 `DELETE /v1/admin/claude-accounts/{accountID}`
3. **断言：** HTTP 409 + body 含 `"code":"STATE_VOLUME_IN_USE_001"` + 中文 message "请先停止使用该账号的所有 host 后重试..."
4. **断言：** DB 查询 `SELECT * FROM claude_accounts WHERE id = '<id>'` 仍返回该行
5. **断言：** 审计事件查询 `SELECT * FROM events WHERE type = 'claude_account.delete_volume_rm_failed'` 命中且 metadata 含 `volume_name` + `error_code` + `error_message` 但**不**含 `email` / OAuth token

**Step 3 (D-26.3): 加 ?force=true 重试 → HTTP 200 + DB 删 + volume 删**

1. 接 Step 2 状态（host 仍在跑，account 仍存在）
2. 调 `DELETE /v1/admin/claude-accounts/{accountID}?force=true`
3. **断言：** HTTP 200 + body `{"deleted":true,"volume_rm":"succeeded"}` 或（若 docker rm -f 也失败）`volume_rm:"failed"` + `next_action` 含 `docker volume rm -f`
4. **断言：** DB 行已删
5. **断言：** `docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id>` 为空（除非 host 仍持有，需手工清理）

**Step 4 (D-26.4): 同一 account 容器 stop → start 后 ~/.claude/.credentials.json 内容不变**

1. 创建 account → host → 进容器 `docker exec -u 1000 cloudproxy-<host_id> bash` → `echo '{"test":"oauth-token-123"}' > ~/.claude/.credentials.json`
2. `docker stop cloudproxy-<host_id>` → `docker start cloudproxy-<host_id>` → 等待 SSH 起来
3. 进容器 → `cat ~/.claude/.credentials.json`
4. **断言：** 内容仍为 `{"test":"oauth-token-123"}`（symlink → volume 持久化生效）

**Step 5 (D-26.5): 重建容器 (rebuild) 后 OAuth credentials 仍可用**

1. 接 Step 4 状态
2. 通过 control-plane 触发 host rebuild（`POST /v1/admin/hosts/{id}/rebuild`）→ 等待 host 状态 = running
3. 进新容器 → `cat ~/.claude/.credentials.json`
4. **断言：** 内容仍为 Step 4 写入的内容（容器重建但 volume 保留）

**Step 6 (额外 D-22 验证): admin host detail 字段**

1. `curl -H "Authorization: Bearer <admin-token>" GET /v1/admin/hosts/<host_id>` → JSON 顶层应含 `"persistent_volume_name":"claude-state-<account_id>"`
2. `curl -H "Authorization: Bearer <admin-token>" GET /v1/admin/hosts` → JSON `hosts[]` 各项**不**含 `persistent_volume_name` 键（OOS-A19 验证）

**Step 7 (额外 SC3 验证): 容器内权限**

1. `docker exec -u root cloudproxy-<host_id> stat -c "%U:%G %n" /home/claude/.claude /home/claude/.cache/claude`
2. **断言：** 输出含两行，owner 都是 `claude:claude`（即 1000:1000）；`-h` 模式下 symlink 本身也是 claude:claude
  </how-to-verify>

  <resume-signal>
回复以下内容之一：
- **`approved`** — 全部 7 个 Step 通过
- **`approved with notes: <notes>`** — 通过但有运维侧 follow-up（写入 SUMMARY）
- **`failed: <step-id> <observed>`** — 列出失败 step + 实测现象 + 建议修复路径，由 plan-checker 决定是否重做或回 discuss
  </resume-signal>
</task>

<task type="auto">
  <name>Task 2.6: 运维手册新增 v3-claude-state-volumes 章节（M16 兜底 + REQ-F7-A 命名规范文档化）</name>
  <files>docs/runbooks/v3-claude-state-volumes.md</files>

  <read_first>
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-01/D-02/D-22 + Deferred 段（独立 GC 任务推迟 v3.1）
    - .planning/research/PITFALLS.md M16（孤儿 volume 不被 prune 默认清理 + label-filter 审计脚本）
    - 既有运维手册风格参考：检查 `docs/` 目录下已有 runbook 文件（grep `find docs -name "*.md"`），保持章节结构一致；如果 `docs/runbooks/` 目录不存在则创建
  </read_first>

  <action>
新建 `docs/runbooks/v3-claude-state-volumes.md`（如 docs/runbooks/ 不存在则创建目录）。内容覆盖：

1. **章节标题：** `# Claude Code 状态持久化 Volume 运维手册（v3.0+）`

2. **背景：** 简述 Phase 33 落地的 named volume 机制（每个 claude_account 一个 volume `claude-state-<id>`，挂到容器 `/var/lib/claude-persist`，entrypoint symlink 到 `~/.claude` 与 `~/.cache/claude`）

3. **命名规范（REQ-F7-A / D-01 / D-02）：**
   - Volume 名格式：`claude-state-{claude_account_id}`（UUID 原格式含连字符）
   - 必带 label：
     - `com.cloud-cli-proxy.account_id=<uuid>` — 唯一性键
     - `com.cloud-cli-proxy.managed=true` — 二级保险

4. **生命周期：**
   - **创建**：worker `createHost` 自动调 `docker volume create`（幂等）
   - **挂载**：`docker create --mount type=volume,src=claude-state-<id>,dst=/var/lib/claude-persist`
   - **删除（强一致）**：`DELETE /v1/admin/claude-accounts/{id}` 默认路径，事务内 `volume rm`，host 仍持有时返回 HTTP 409
   - **删除（最终一致）**：`DELETE /v1/admin/claude-accounts/{id}?force=true`，DB 先删，rm 失败仅写 audit；运维需关注 `claude_account.force_volume_rm_failed` 事件并手工清理

5. **孤儿 volume 审计脚本（M16 兜底）：**

```bash
#!/usr/bin/env bash
# 列出所有受管 claude-state-* volume
docker volume ls --filter label=com.cloud-cli-proxy.managed=true --format '{{.Name}}'

# 与 DB 对比找出孤儿（DB 中无对应 account 但 docker 仍存在的 volume）
psql "$DATABASE_URL" -tAc "SELECT 'claude-state-' || id FROM claude_accounts" > /tmp/db-volumes.txt
docker volume ls --filter label=com.cloud-cli-proxy.managed=true --format '{{.Name}}' | grep '^claude-state-' > /tmp/docker-volumes.txt
echo "=== Orphan volumes (in docker, not in DB) ==="
comm -23 <(sort /tmp/docker-volumes.txt) <(sort /tmp/db-volumes.txt)
```

6. **故障排查：**
   - `claude_account.delete_volume_rm_failed` 事件 → 检查 `volume_name` 关联的 host 是否仍 running
   - `claude_account.volume_create_failed` 事件 → 检查 host-agent 与 docker daemon 连通性
   - `claude_account.volume_name_persist_failed` 事件 → 检查 `claude_accounts.persistent_volume_name` 字段是否被人工改过；冲突时 worker 不阻塞容器启动但 audit 留痕
   - 容器内 `~/.claude` 不持久化 → 检查 entrypoint 日志是否含 `[entrypoint] v3: persistent state ready`；symlink 验证：`docker exec <ctr> readlink /home/claude/.claude` 应返回 `/var/lib/claude-persist/.claude`

7. **deferred 项（v3.1 backlog）：**
   - 独立 GC 定时任务（cron 扫孤儿 volume，按 label + DB 反查无 account 时清理）
   - Volume 备份脚本（tar `/var/lib/docker/volumes/claude-state-*`）
   - Label 不一致检测（`ensureDockerVolume` 当前仅做存在性检查，参 RESEARCH §6.6）

8. **参考：**
   - `.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md`
   - `.planning/REQUIREMENTS.md` §F7
   - `.planning/research/PITFALLS.md` M16/M17

**verbatim 字段（必须出现在文档内，便于后续运维 grep）：**
- 字符串 `claude-state-`（命名前缀）
- label key `com.cloud-cli-proxy.account_id` / `com.cloud-cli-proxy.managed`
- mount target `/var/lib/claude-persist`
- 事件类型 `claude_account.deleted` / `claude_account.delete_volume_rm_failed` / `claude_account.force_volume_rm_failed` / `claude_account.volume_create_failed` / `claude_account.volume_name_persist_failed` / `claude_account.volume_rm_failed`
- 错误码 `STATE_VOLUME_IN_USE_001`
- HTTP endpoint `DELETE /v1/admin/claude-accounts/{id}` + query `?force=true`

**禁止：**
- 在文档内写绝对开发机路径 / 真实 OAuth token / 真实 account UUID（用 `<uuid>` / `<account_id>` 占位）
- 写"未来如何如何"的承诺；deferred 项明确标 v3.1 backlog
  </action>

  <verify>
    <automated>
bash -c 'set -e
test -f docs/runbooks/v3-claude-state-volumes.md
grep -q "claude-state-" docs/runbooks/v3-claude-state-volumes.md
grep -q "com.cloud-cli-proxy.account_id" docs/runbooks/v3-claude-state-volumes.md
grep -q "com.cloud-cli-proxy.managed" docs/runbooks/v3-claude-state-volumes.md
grep -q "/var/lib/claude-persist" docs/runbooks/v3-claude-state-volumes.md
grep -q "STATE_VOLUME_IN_USE_001" docs/runbooks/v3-claude-state-volumes.md
grep -q "DELETE /v1/admin/claude-accounts" docs/runbooks/v3-claude-state-volumes.md
grep -q "force=true" docs/runbooks/v3-claude-state-volumes.md
grep -q "claude_account.delete_volume_rm_failed" docs/runbooks/v3-claude-state-volumes.md
grep -q "claude_account.force_volume_rm_failed" docs/runbooks/v3-claude-state-volumes.md
grep -q "孤儿\\|orphan" docs/runbooks/v3-claude-state-volumes.md
# 不写真实 OAuth token / 真实 UUID（不应有 36 位连续 hex-with-dash）
! grep -E "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}" docs/runbooks/v3-claude-state-volumes.md || echo "WARNING: doc may contain real UUIDs"
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `docs/runbooks/v3-claude-state-volumes.md` 存在
    - 关键 verbatim 字段全部命中（命名前缀 / 双 label / mount target / 错误码 / 6 类事件类型 / endpoint + force flag）
    - 文档含孤儿 volume 审计脚本 + 故障排查段
    - 不含真实 UUID / OAuth token（grep 守恒）
  </acceptance_criteria>

  <done>
    - 运维手册章节就位，含命名规范 / 生命周期 / 审计脚本 / 故障排查 / deferred 项 / 引用
    - 所有 grep 守恒断言通过
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 浏览器 / curl → admin handler | adminGuard 中间件链（既有，强制 admin role JWT） |
| admin handler → control-plane Repository (pgx.Tx) | 同进程内 SQL 事务，BeginTx → COMMIT/ROLLBACK 严格配对 |
| admin handler → host-agent (`agentapi.Client.RunHostAction`) | 复用 Phase 1 已建立的 host-agent token 鉴权（unix socket / token header），本 plan **不**新增 endpoint |
| host-agent → docker daemon (Plan 01 落地) | 既有信任边界 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-33-09 | Spoofing | `DELETE /v1/admin/claude-accounts/{accountID}` 未鉴权调用删任意 account | mitigate | router 注册必须用 `adminGuard(claudeHandler.Delete())`（与 AdminHosts 全部端点一致）；adminGuard 在 router.go 既有实现含 admin role JWT 校验。Task 2.4 acceptance_criteria grep 强制断言 `adminGuard(claudeHandler.Delete())` 命中。 |
| T-33-10 | Tampering | force=true 路径 DB 已删但 volume 未删导致状态不一致 | mitigate | force 路径**必须**写 `claude_account.force_volume_rm_failed` audit + 响应体明确 `volume_rm:"failed"` + `next_action: "运维需手工 docker volume rm -f <name>"`（D-19 显式上报）。运维手册（Task 2.6）章节 6 提供故障排查脚本兜底。 |
| T-33-11 | Repudiation | 管理员删除 account 无审计 | mitigate | 任一路径成功必写 `claude_account.deleted`（含 `account_id` / `volume_name` / `force` bool）；失败路径写对应 `*_failed` 事件。Task 2.2 acceptance_criteria 含 grep 验证 3 类事件就位。 |
| T-33-12 | Information Disclosure | audit metadata 误写 OAuth token / email | mitigate | metadata key 白名单：`account_id` / `volume_name` / `error_code` / `error_message` / `force`；**禁止** `email` / `entry_password` / `credentials` / `oauth_token`。Task 2.2 verify 段含 `! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\""` 强制守恒。沿用 Phase 29.1 Plan 02 mitigation 模式。 |
| T-33-13 | Denial of Service | docker daemon hang 拖死 admin handler | mitigate | strict 路径 `context.WithTimeout(r.Context(), 10*time.Second)`、force 路径 30s（D-20）。Task 2.2 含两条 timeout 单测断言（TestAdminClaudeAccountsDelete_StrictUsesTenSecondTimeout / ForceUsesThirtySecondTimeout）。`agentapi.Client.httpClient.Timeout=30s` 既有兜底。 |
| T-33-14 | Information Disclosure | admin host detail 返回敏感字段 | accept | `adminHostDetailResponse` 已在既有代码 line 116-118 显式抹掉 `EntryPassword` / `PasswordHash`；Task 2.3 仅追加 `PersistentVolumeName`（非敏感字符串）。 |
| T-33-15 | Tampering | LEFT JOIN 1:N 返回多行误导 detail handler | mitigate | `getHostWithClaudeAccountSQL` 含 `ORDER BY ca.created_at ASC LIMIT 1`（与既有 `resolveClaudeAccountByHostSQL` 风格一致）。Task 2.1 单测 `TestGetHostWithClaudeAccountSQL_ContainsLeftJoinTokens` 断言 `LIMIT 1` 存在。 |
| T-33-16 | Elevation of Privilege | force=true query 参数被普通用户路径误传 | mitigate | adminGuard 在 router 层已强制 admin role；user-facing endpoint（`/v1/user/*`）不暴露此 handler。`parseForceFlag` 仅在 admin handler 调用。 |
| T-33-17 | Repudiation | 强一致路径 audit 写在事务外（rollback 后仍留痕） | accept | 设计意图 — `recordEvent` 用 `r.Context()` 而非 `ctx`（`tx` 已 rollback 后仍写事件），让运维能查到失败原因。rationale：审计可见性优先于事务原子性。 |

**ASVS L1 高严重度阻塞性威胁：** 0 — T-33-09 / T-33-12 / T-33-13 三项关键威胁都通过 grep CI 断言 + 单测 mitigate；其余项 disposition 已明确。
</threat_model>

<verification>
## Plan-level 验证

```bash
# 1. 全仓库构建（Repository.GetHostWithClaudeAccount 接口契约闭环）
go build ./...

# 2. Plan 02 全部新增/修改测试
go test ./internal/store/repository/ -run "TestGetHostWithClaudeAccountSQL|TestLockClaudeAccountForDeleteSQL|TestDeleteClaudeAccountSQL|TestHostWithClaudeAccount_EmbedsHost" -count=1
go test ./internal/controlplane/http/ -run "TestAdminClaudeAccountsDelete|TestParseForceFlag|TestAdminHostDetail_Includes|TestAdminHostDetail_Omits|TestAdminHostList_DoesNot" -count=1

# 3. Plan 01 单测无回归（Plan 02 修改不破坏 Plan 01）
go test ./internal/agentapi/ ./internal/runtime/tasks/ -count=1

# 4. 既有 admin / repository 测试无回归
go test ./internal/controlplane/http/ ./internal/store/repository/ -count=1 -short

# 5. 审计 metadata 白名单守恒（关键安全断言）
! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\"" internal/controlplane/http/admin_claude_accounts.go
! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\"" internal/runtime/tasks/worker.go

# 6. 路由注册闭环（grep）
grep -q "DELETE /v1/admin/claude-accounts/{accountID}" internal/controlplane/http/router.go
grep -q "adminGuard(claudeHandler.Delete())" internal/controlplane/http/router.go

# 7. 运维手册关键 token
grep -q "claude-state-" docs/runbooks/v3-claude-state-volumes.md
grep -q "STATE_VOLUME_IN_USE_001" docs/runbooks/v3-claude-state-volumes.md

# 8. 不引入新依赖
go mod tidy && git diff --exit-code go.mod go.sum
```

## SC 映射（ROADMAP §Phase 33 Success Criteria 全 6 条 — Plan 01+02 联合）

| SC | Plan 01 / Plan 02 | 验证手段 |
|----|-------------------|---------|
| SC1 (REQ-F7-B 容器重建 OAuth 保留) | Plan 01 entrypoint + Plan 02 UAT Step 5 | 人工 UAT + entrypoint grep 断言 |
| SC2 (REQ-F7-A volume 命名 + label) | Plan 01 BuildClaudeStateVolumeName + worker label | Plan 01 单测 + Plan 02 UAT Step 1 grep `docker volume ls --filter` |
| SC3 (容器内目录属主 1000:1000) | Plan 01 entrypoint chown -R + chown -h | Plan 02 UAT Step 7 |
| SC4 (admin DELETE 事务联动) | Plan 02 admin handler 强一致路径 | Plan 02 UAT Step 1 + 8 条 handler 单测 |
| SC5 (host-agent volume create 幂等) | Plan 01 ensureDockerVolume | Plan 01 单测 TestEnsureDockerVolume_AlreadyExists_SkipsCreate |
| SC6 (volume rm 失败事务回滚) | Plan 02 admin handler 409 + audit + ROLLBACK | Plan 02 UAT Step 2 + TestAdminClaudeAccountsDelete_StrictHostAgentFailure_RollbackAnd409WithChineseMessage |
</verification>

<success_criteria>
- [ ] 6 个 task 全部完成，各 task 的 acceptance_criteria 全 PASS
- [ ] `go build ./...` 退出码 = 0（接口契约闭环）
- [ ] Plan 02 新增 15+ 条单测全 PASS（4 条仓储 SQL + 8 条 admin handler + 3 条 admin host detail）
- [ ] `! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\"" internal/controlplane/http/admin_claude_accounts.go` 守恒
- [ ] router.go 注册闭环（adminGuard 链路就位）
- [ ] 运维手册 v3-claude-state-volumes.md 含命名规范 / 生命周期 / 孤儿审计 / 故障排查
- [ ] 人工 UAT D-26 五步 + 额外 SC3/D-22 验证全部 approved
- [ ] 既有 worker / 仓储 / handler 测试无回归
- [ ] 不引入新 Go 依赖（手写 stubTx 而非 pgxmock）
</success_criteria>

<output>
After completion, create `.planning/phases/33-claude-code-cli-admin-gc/33-02-SUMMARY.md` with:
- 6 个 task 的实际 commit SHA + 关键 diff 片段引用
- handler 单测 8 条 + 仓储单测 4 条 + admin_hosts 测试 3 条 PASS 时间戳
- UAT D-26 五步实测结果（每步附 curl/docker 命令实际输出截图或 paste）
- audit event 实测样本（admin handler 触发后 events 表 3 类事件 + metadata 字段实际值，验证白名单）
- 运维手册 v3-claude-state-volumes.md 章节预览（前 30 行）
- 阶段闭环：ROADMAP §Phase 33 SC1..SC6 逐条状态（Plan 01+02 联合）
- carry-over：force=true 路径 next_action 内容是否需要前端 UI 同步追加（Plan 02 自查 React admin SPA TS interface 是否强类型化）
- v3.1 backlog 候选项：(a) 独立 GC 定时任务、(b) volume 备份脚本、(c) ensureDockerVolume label 比对（RESEARCH §6.6）
</output>
