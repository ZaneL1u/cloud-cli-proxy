---
phase: 33-claude-code-cli-admin-gc
plan: 01
type: execute
wave: 1
depends_on: []
autonomous: true
requirements: [REQ-F7-A, REQ-F7-B]
requirements_addressed: [REQ-F7-A, REQ-F7-B]
files_modified:
  - deploy/docker/managed-user/entrypoint.sh
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker.go
  - internal/store/repository/queries.go
  - internal/runtime/tasks/worker_volume_test.go
  - internal/runtime/tasks/worker_volume_lifecycle_test.go
  - internal/store/repository/queries_claude_account_volume_test.go

must_haves:
  truths:
    - "同一 claude_account 容器删除并重建后，~/.claude/.credentials.json OAuth token 保留，无需 claude login（SC1 / REQ-F7-B）"
    - "docker volume ls --filter label=com.cloud-cli-proxy.account_id=<id> 返回唯一 volume claude-state-<id>（SC2 / REQ-F7-A）"
    - "容器内 /home/claude/.claude 与 /home/claude/.cache/claude 属主始终为 1000:1000，无权限错误（SC3 / PITFALLS C5+M17）"
    - "host-agent 重复收到同一 Volumes 创建请求时 docker volume create 幂等返回成功，不报 volume exists（SC5）"
  artifacts:
    - path: deploy/docker/managed-user/entrypoint.sh
      provides: "prepare_persistent_state 函数 + v3 stage 调用插入"
      contains: "prepare_persistent_state"
    - path: internal/agentapi/contracts.go
      provides: "ActionVolumeRemove 协议常量"
      contains: "ActionVolumeRemove HostAction = \"volume_remove\""
    - path: internal/runtime/tasks/worker.go
      provides: "BuildClaudeStateVolumeName / ensureDockerVolume / removeDockerVolume / removeVolumes / createHost 自动补 volume / WorkerRepo 接口扩展"
      contains: "BuildClaudeStateVolumeName"
    - path: internal/store/repository/queries.go
      provides: "UpsertClaudeAccountPersistentVolumeName 三态语义仓储方法"
      contains: "UpsertClaudeAccountPersistentVolumeName"
  key_links:
    - from: "internal/runtime/tasks/worker.go::createHost"
      to: "ensureDockerVolume + UpsertClaudeAccountPersistentVolumeName"
      via: "request.ClaudeAccountID != \"\" 触发自动补 VolumeMount{Name: claude-state-<id>, Target: /var/lib/claude-persist}"
      pattern: "ensureDockerVolume\\(ctx, volumeName, labels\\)"
    - from: "internal/runtime/tasks/worker.go::Execute switch"
      to: "removeVolumes(ctx, request)"
      via: "case agentapi.ActionVolumeRemove"
      pattern: "case agentapi.ActionVolumeRemove"
    - from: "deploy/docker/managed-user/entrypoint.sh"
      to: "/home/claude/.claude → /var/lib/claude-persist/.claude (symlink)"
      via: "ln -sfn 在 prepare_v3_dirs 之后、prepare_mutagen_agent 之前执行"
      pattern: "ln -sfn .*\\$root/.claude /home/claude/.claude"
---

<objective>
交付 Phase 33 镜像 + worker + agentapi 三处基础设施，让 OAuth credentials 与 Claude Code 缓存能跨容器重建持久化。具体实现：(1) entrypoint 在 v3 stage 链路插入 `prepare_persistent_state`，把 `/var/lib/claude-persist` 下的 `.claude` / `.cache/claude` symlink 到 `/home/claude/`，并完成 cp -an seed + chown 兜底；(2) worker `createHost` 在 `request.ClaudeAccountID != ""` 时自动调 `ensureDockerVolume` 幂等创建 named volume + 自动追加 `VolumeMount` + upsert `claude_accounts.persistent_volume_name`；(3) `agentapi.ActionVolumeRemove` 协议常量 + worker switch 新增 case + `removeDockerVolume(force)` 实现幂等删除（`No such volume` 视为成功，`volume is in use` 传播 `volume_in_use` 错误码）；(4) 仓储新增 `UpsertClaudeAccountPersistentVolumeName` 三态语义方法（NULL→写入 / 一致跳过 / 冲突错误）。

Purpose: 为 Plan 02 admin DELETE 事务联动提供 `ActionVolumeRemove` 协议入口与 `ensureDockerVolume` / `BuildClaudeStateVolumeName` 工具链，并通过 entrypoint symlink 让用户感知到的 `~/.claude` 始终落在 named volume 上（REQ-F7-A / REQ-F7-B / PITFALLS C5+M16+M17 防御）。

Output:
- 1 个修改的镜像 entrypoint
- 1 个修改的 agentapi 协议
- 1 个修改的 worker（新增 ~140 行）
- 1 个修改的仓储 SQL（新增 ~30 行）
- 3 个新增/修改的单测文件（D-25 第 1-4 + 第 7 项）
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
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md

# 直接修改对象
@deploy/docker/managed-user/entrypoint.sh
@internal/agentapi/contracts.go
@internal/runtime/tasks/worker.go
@internal/store/repository/queries.go
@internal/store/repository/models.go

<interfaces>
<!-- 从既有代码提取的关键契约。executor 直接复用，无需 grep 探索。 -->

From internal/agentapi/contracts.go:
```go
type HostAction string

const (
    ActionCreateHost  HostAction = "create_host"
    ActionStartHost   HostAction = "start_host"
    ActionStopHost    HostAction = "stop_host"
    ActionRebuildHost HostAction = "rebuild_host"
    ActionPrepareHost HostAction = "prepare_host"
)

type VolumeMount struct {
    Name     string            `json:"name"`
    Target   string            `json:"target"`
    ReadOnly bool              `json:"read_only,omitempty"`
    Labels   map[string]string `json:"labels,omitempty"`
}

type HostActionRequest struct {
    // ... existing fields
    Volumes         []VolumeMount `json:"volumes,omitempty"`         // Phase 29 D-18
    ClaudeAccountID string        `json:"claude_account_id,omitempty"` // Phase 30 D-09
    Labels          map[string]string `json:"labels"`
}
```

From internal/runtime/tasks/worker.go (line 32-37, 48-63, 687-695):
```go
type WorkerRepo interface {
    UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
    UpdateHostStatus(ctx context.Context, hostID string, status string) error
    GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
    RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
}

// Execute switch 在 line 50-63
// var execInContainer mock 注入模式 在 line 687-695（沿用此模式新增 var ensureDockerVolume / removeDockerVolume）
```

From internal/store/repository/models.go (line 209-222):
```go
type ClaudeAccount struct {
    ID                   string
    UserID               string
    HostID               *string
    Email                string
    PersistentVolumeName *string  // Phase 30 D-02 三态语义：NULL = 未分配 / 非空 = 已分配
    // ...
}
```

From internal/store/repository/queries.go (line 1203-1219):
```go
const resolveClaudeAccountByHostSQL = `
    SELECT id::text
    FROM claude_accounts
    WHERE host_id = $1
    ORDER BY created_at ASC
    LIMIT 1
`
// 风格参考：包级 const SQL + id::text + COALESCE 处理三态
```

From deploy/docker/managed-user/entrypoint.sh (line 60-69, 254-258):
```bash
# Analog 函数（行 62-69）：
prepare_v3_dirs() {
  echo "[entrypoint] v3: chown /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist"
  chown -R 1000:1000 \
    /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist 2>/dev/null || true
}

# v3 stage 调用序列（行 254-258）：
# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1.1: entrypoint.sh 新增 prepare_persistent_state 函数 + v3 stage 调用插入（D-09 / SC3 / REQ-F7-B）</name>
  <files>deploy/docker/managed-user/entrypoint.sh</files>

  <read_first>
    - deploy/docker/managed-user/entrypoint.sh（特别是 line 60-100 函数定义区 + line 254-258 v3 stage 调用块）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-08/D-09/D-10/D-11/D-12（持久化拓扑）
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.1（verbatim 代码骨架）+ §6.2（cp -an overlay2/btrfs 边界）
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `entrypoint.sh` 节（analog + verbatim 字段清单）
    - .planning/phases/29-v3-worker/29-CONTEXT.md D-09 v3 stage 串行编排约定
  </read_first>

  <behavior>
    - 函数 prepare_persistent_state 必须幂等：volume 已有内容时**不**覆盖（cp -an + [ -z "$(ls -A ...)" ] 守卫），symlink 重复创建不报错（ln -sfn）
    - 权限不变量：volume 内容物 + symlink 链接本身都是 1000:1000（chown -R 与 chown -h 双重）
    - 不阻塞：cp -an 失败 || true（极端 immutable 文件场景仍允许容器启动）
    - 调用插入位置：必须在 prepare_v3_dirs 之后、prepare_mutagen_agent 之前（D-09 锁定顺序，让 mutagen-agent 看到稳定路径）
    - 日志前缀必须保留 `[entrypoint] v3:`（与 prepare_v3_dirs 风格一致，便于运维 grep）
  </behavior>

  <action>
在 `deploy/docker/managed-user/entrypoint.sh` 的 `prepare_v3_dirs()` 函数定义（line 62-69）之后、`prepare_mutagen_agent()` 之前插入下列函数定义（**verbatim 复制 RESEARCH §2.1**）：

```bash
prepare_persistent_state() {
  local root=/var/lib/claude-persist
  mkdir -p "$root/.claude" "$root/.cache/claude"

  if [ -d /home/claude/.claude ] && [ -z "$(ls -A "$root/.claude" 2>/dev/null)" ]; then
    cp -an /home/claude/.claude/. "$root/.claude/" 2>/dev/null || true
  fi
  if [ -d /home/claude/.cache/claude ] && [ -z "$(ls -A "$root/.cache/claude" 2>/dev/null)" ]; then
    cp -an /home/claude/.cache/claude/. "$root/.cache/claude/" 2>/dev/null || true
  fi

  chown -R 1000:1000 "$root"

  rm -rf /home/claude/.claude /home/claude/.cache/claude
  ln -sfn "$root/.claude" /home/claude/.claude
  mkdir -p /home/claude/.cache
  ln -sfn "$root/.cache/claude" /home/claude/.cache/claude

  chown -h 1000:1000 /home/claude/.claude /home/claude/.cache/claude

  echo "[entrypoint] v3: persistent state ready (volume=/var/lib/claude-persist)"
}
```

然后在 v3 stage 调用块（line 254-258）插入对应调用，最终块为：

```bash
# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_persistent_state
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version
```

**verbatim 守恒**（不允许微调）：
- 函数名 `prepare_persistent_state`（CONTEXT D-09 字面）
- 路径 `/var/lib/claude-persist`（D-08，与 worker `claudeStateMountTarget` + Dockerfile 三处必须一致）
- 标志组合 `cp -an` / `ln -sfn` / `chown -h` / `chown -R`（D-09 不变量）
- `[ -d ... ] && [ -z "$(ls -A ...)" ]` 守卫（D-09 幂等条件，省略将导致重启时覆盖用户数据）
- `|| true` 兜底（D-09 不阻塞条款）
- 调用插入位置：`prepare_v3_dirs` 之后、`prepare_mutagen_agent` 之前

**禁止**：删除 `prepare_v3_dirs` 现有 chown（D-10 兜底语义保留），修改 echo 日志前缀，引入 sed/awk 写文件（用 StrReplace）。
  </action>

  <verify>
    <automated>
bash -c 'set -e
test -f deploy/docker/managed-user/entrypoint.sh
grep -q "^prepare_persistent_state()" deploy/docker/managed-user/entrypoint.sh
grep -q "ln -sfn \"\$root/.claude\" /home/claude/.claude" deploy/docker/managed-user/entrypoint.sh
grep -q "cp -an /home/claude/.claude/. \"\$root/.claude/\"" deploy/docker/managed-user/entrypoint.sh
grep -q "chown -h 1000:1000 /home/claude/.claude /home/claude/.cache/claude" deploy/docker/managed-user/entrypoint.sh
# 调用顺序断言：prepare_v3_dirs → prepare_persistent_state → prepare_mutagen_agent
awk "/^prepare_v3_dirs\$/,/^prepare_mutagen_agent\$/" deploy/docker/managed-user/entrypoint.sh | grep -q "^prepare_persistent_state\$"
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "^prepare_persistent_state()" deploy/docker/managed-user/entrypoint.sh` 输出 = 1（函数定义存在且唯一）
    - `grep -c "^prepare_persistent_state$" deploy/docker/managed-user/entrypoint.sh` 输出 = 1（v3 stage 块内调用存在且唯一）
    - `grep -A1 "^prepare_v3_dirs$" deploy/docker/managed-user/entrypoint.sh | tail -1` = `prepare_persistent_state`（顺序正确）
    - `grep -B1 "^prepare_mutagen_agent$" deploy/docker/managed-user/entrypoint.sh | head -1` = `prepare_persistent_state`（顺序正确）
    - `grep -q "echo \"\\[entrypoint\\] v3: persistent state ready" deploy/docker/managed-user/entrypoint.sh` 命中（日志前缀守恒）
    - `bash -n deploy/docker/managed-user/entrypoint.sh` 退出码 = 0（语法正确）
  </acceptance_criteria>

  <done>
    - prepare_persistent_state 函数定义就位且通过 bash -n 语法检查
    - v3 stage 调用顺序 prepare_v3_dirs → prepare_persistent_state → prepare_mutagen_agent → prepare_mergerfs_check → assert_tmux_version
    - 所有 verbatim 字段（路径 / 标志 / 守卫 / 日志前缀）逐一通过 grep 断言
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 1.2: agentapi 新增 ActionVolumeRemove 协议常量 + worker_volume_test.go 追加 round-trip 测试（D-13 / D-25.4）</name>
  <files>internal/agentapi/contracts.go, internal/runtime/tasks/worker_volume_test.go</files>

  <read_first>
    - internal/agentapi/contracts.go（特别是 line 5-11 常量块）
    - internal/runtime/tasks/worker_volume_test.go（特别是 line 109-131 TestHostActionRequest_ClaudeAccountID_RoundTrip 风格）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-13/D-14/D-25.4
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.2 verbatim 骨架
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `internal/agentapi/contracts.go` + `worker_volume_test.go` 节
  </read_first>

  <behavior>
    - Test 1: TestHostActionRequest_VolumeRemove_RoundTrip — 构造 `HostActionRequest{Action: ActionVolumeRemove, Volumes: [{Name: "claude-state-abc"}]}` json.Marshal 必须包含 `"action":"volume_remove"` 与 `"name":"claude-state-abc"`，json.Unmarshal 还原后字段不丢
    - Test 2: TestActionVolumeRemove_StringValue — `string(agentapi.ActionVolumeRemove)` == `"volume_remove"`（保护 host-agent 端 switch 字符串比较的协议契约）
  </behavior>

  <action>
**(a) 在 `internal/agentapi/contracts.go` 的 `ActionPrepareHost` 之后追加一行：**

```go
const (
	ActionCreateHost   HostAction = "create_host"
	ActionStartHost    HostAction = "start_host"
	ActionStopHost     HostAction = "stop_host"
	ActionRebuildHost  HostAction = "rebuild_host"
	ActionPrepareHost  HostAction = "prepare_host"
	ActionVolumeRemove HostAction = "volume_remove" // Phase 33 D-13
)
```

verbatim 字段：
- 常量名 `ActionVolumeRemove`
- 字符串值 `"volume_remove"`
- 行内注释 `// Phase 33 D-13`

**(b) 在 `internal/runtime/tasks/worker_volume_test.go` 文件末尾追加两条测试**（**沿用同文件 line 109-131 的 round-trip 风格**）：

```go
// TestHostActionRequest_VolumeRemove_RoundTrip 守护 D-13/D-25.4：
// Action=volume_remove + Volumes 字段必须完整 round-trip，供 Plan 02 admin handler 触发 host-agent 删 volume。
func TestHostActionRequest_VolumeRemove_RoundTrip(t *testing.T) {
	req := agentapi.HostActionRequest{
		TaskID: "t1", HostID: "h1", Action: agentapi.ActionVolumeRemove,
		Volumes: []agentapi.VolumeMount{{Name: "claude-state-abc"}},
		Labels:  map[string]string{"force": "true"},
	}
	buf, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(buf), `"action":"volume_remove"`) {
		t.Fatalf("ActionVolumeRemove must serialize as volume_remove, got: %s", buf)
	}
	if !strings.Contains(string(buf), `"name":"claude-state-abc"`) {
		t.Fatalf("VolumeMount.Name must round-trip, got: %s", buf)
	}

	var parsed agentapi.HostActionRequest
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Action != agentapi.ActionVolumeRemove {
		t.Fatalf("Action lost: got %q, want %q", parsed.Action, agentapi.ActionVolumeRemove)
	}
	if len(parsed.Volumes) != 1 || parsed.Volumes[0].Name != "claude-state-abc" {
		t.Fatalf("Volumes lost: got %+v", parsed.Volumes)
	}
	if parsed.Labels["force"] != "true" {
		t.Fatalf("Labels[force] lost: got %q", parsed.Labels["force"])
	}
}

// TestActionVolumeRemove_StringValue 守护协议契约（host-agent 端用字符串 switch 比较）。
func TestActionVolumeRemove_StringValue(t *testing.T) {
	if string(agentapi.ActionVolumeRemove) != "volume_remove" {
		t.Fatalf("ActionVolumeRemove must equal \"volume_remove\", got %q", agentapi.ActionVolumeRemove)
	}
}
```

import 检查：测试文件已 import `agentapi` / `encoding/json` / `strings` / `testing`（grep 验证）。
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "^	ActionVolumeRemove HostAction = \"volume_remove\"" internal/agentapi/contracts.go
grep -q "// Phase 33 D-13" internal/agentapi/contracts.go
grep -q "TestHostActionRequest_VolumeRemove_RoundTrip" internal/runtime/tasks/worker_volume_test.go
grep -q "TestActionVolumeRemove_StringValue" internal/runtime/tasks/worker_volume_test.go
go build ./internal/agentapi/... ./internal/runtime/tasks/...
go test ./internal/agentapi/... -count=1
go test ./internal/runtime/tasks/ -run "TestHostActionRequest_VolumeRemove_RoundTrip|TestActionVolumeRemove_StringValue" -count=1 -v
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "ActionVolumeRemove HostAction = \"volume_remove\"" internal/agentapi/contracts.go` 输出 = 1
    - `go test ./internal/runtime/tasks/ -run TestHostActionRequest_VolumeRemove_RoundTrip -count=1` 退出码 = 0
    - `go test ./internal/runtime/tasks/ -run TestActionVolumeRemove_StringValue -count=1` 退出码 = 0
    - `go vet ./internal/agentapi/... ./internal/runtime/tasks/...` 退出码 = 0
  </acceptance_criteria>

  <done>
    - `agentapi.ActionVolumeRemove = "volume_remove"` 常量就位，注释含 `// Phase 33 D-13`
    - 两条新增 round-trip / string-value 测试 PASS
    - `go build ./...` PASS（contracts.go 修改不破坏既有依赖）
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 1.3: worker.go 新增 BuildClaudeStateVolumeName / ensureDockerVolume / removeDockerVolume + Execute switch 加 case + createHost 自动补 volume + WorkerRepo 接口扩展（D-04..D-07 / D-14/D-15 / SC2/SC4/SC5）</name>
  <files>internal/runtime/tasks/worker.go, internal/runtime/tasks/worker_volume_lifecycle_test.go</files>

  <read_first>
    - internal/runtime/tasks/worker.go（特别是 line 1-15 import / line 32-37 WorkerRepo / line 48-88 Execute switch + 错误码映射 / line 194-249 createHost / line 657-668 recordNetworkError 风格 / line 687-695 var execInContainer / line 832-840 runDocker）
    - internal/runtime/tasks/worker_password_test.go（沿用 var execInContainer 替换的测试风格）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-04/D-05/D-06/D-07/D-14/D-15
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.3 (a)(b)(c)(d) 完整代码骨架 + §5.1 dockerVolumeRunner 抽象 + §6.1 docker 错误字符串清单 + §6.5 ClaudeAccountID 覆盖率风险
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `internal/runtime/tasks/worker.go` + `worker_volume_lifecycle_test.go` 节 + Shared 1/3 节
    - .planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md Plan 02 fail-fast + audit event metadata 不写凭据原则
  </read_first>

  <behavior>
    - Test 1 (TestBuildClaudeStateVolumeName_NonEmptyID_ReturnsPrefixedName)：`BuildClaudeStateVolumeName("acct-42")` 返回 `("claude-state-acct-42", nil)`
    - Test 2 (TestBuildClaudeStateVolumeName_EmptyID_ReturnsError)：`BuildClaudeStateVolumeName("")` 返回 `("", error)` 且 error 信息含 `"required"`
    - Test 3 (TestEnsureDockerVolume_NotExists_RunsCreate)：mock `dockerVolumeRunner` 第一次（inspect）返回 error，第二次（create）成功 → `realEnsureDockerVolume` 返回 nil 且共调用 2 次 docker
    - Test 4 (TestEnsureDockerVolume_AlreadyExists_SkipsCreate)：mock 第一次（inspect）成功 → 返回 nil 且只调用 1 次 docker
    - Test 5 (TestRemoveDockerVolume_NotFound_IsSuccess)：mock 返回 stderr 含 `"no such volume"` + exit 1 → 返回 nil（幂等）
    - Test 6 (TestRemoveDockerVolume_InUse_PropagatesVolumeInUseError)：mock 返回 stderr 含 `"volume is in use"` + exit 1 → 返回 error 且 `err.Error()` 以 `"volume_in_use:"` 开头
    - Test 7 (TestRemoveDockerVolume_ForceTrue_PassesDashF)：mock 捕获 args 必须含 `"-f"` 当 force=true
  </behavior>

  <action>
**(a) 在 `internal/runtime/tasks/worker.go` 文件末尾（紧跟 `runDocker` 之后）追加包级常量、`BuildClaudeStateVolumeName`、`dockerVolumeRunner` 抽象、`ensureDockerVolume` / `removeDockerVolume` 函数（**verbatim 复制 RESEARCH §2.3 (a) + §5.1 dockerVolumeRunner**）：**

```go
const (
	claudeStateVolumePrefix = "claude-state-"
	claudeStateMountTarget  = "/var/lib/claude-persist"
	claudeAccountLabelKey   = "com.cloud-cli-proxy.account_id"
	claudeManagedLabelKey   = "com.cloud-cli-proxy.managed"
	claudeManagedLabelVal   = "true"
)

// BuildClaudeStateVolumeName 返回 D-01 规范的 volume 名 `claude-state-{id}`（保留 UUID 原格式含连字符）。空 id 返回错误。
func BuildClaudeStateVolumeName(claudeAccountID string) (string, error) {
	if claudeAccountID == "" {
		return "", fmt.Errorf("BuildClaudeStateVolumeName: claude_account_id is required")
	}
	return claudeStateVolumePrefix + claudeAccountID, nil
}

// dockerVolumeRunner 抽象 docker volume 子命令的实际执行；包级 var 便于单元测试注入 mock。
// 与 var execInContainer = ... (worker.go:687) 模式一致。
var dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"volume"}, args...)...)
	return cmd.CombinedOutput()
}

// ensureDockerVolume 幂等创建 named volume（D-04）：
//   - inspect 成功：视为已存在，返回 nil（label 比对 v3.1 backlog，参 RESEARCH §6.6）
//   - inspect 失败：执行 create --label k=v --label k=v <name>
// 暴露为包级 var 以便测试注入 mock（沿用 var execInContainer 模式）。
var ensureDockerVolume = realEnsureDockerVolume

func realEnsureDockerVolume(ctx context.Context, name string, labels map[string]string) error {
	if name == "" {
		return fmt.Errorf("ensureDockerVolume: empty name")
	}
	if _, err := dockerVolumeRunner(ctx, "inspect", name); err == nil {
		return nil
	}
	args := []string{"create"}
	for k, v := range labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, name)
	out, err := dockerVolumeRunner(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker volume create %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeDockerVolume 幂等删除（D-15）：
//   - "no such volume" 视为成功（幂等）
//   - "volume is in use" 包装为 volume_in_use: 前缀错误（供 Execute 错误码映射）
//   - force=true 追加 -f 标志
var removeDockerVolume = realRemoveDockerVolume

func realRemoveDockerVolume(ctx context.Context, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("removeDockerVolume: empty name")
	}
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	out, err := dockerVolumeRunner(ctx, args...)
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	low := strings.ToLower(msg)
	if strings.Contains(low, "no such volume") {
		return nil
	}
	if strings.Contains(low, "volume is in use") {
		return fmt.Errorf("volume_in_use: %s", msg)
	}
	return fmt.Errorf("docker volume rm %s: %w (%s)", name, err, msg)
}
```

**(b) 在 `Execute()` switch（line 50-63）新增 case，在 `case agentapi.ActionPrepareHost:` 之后、`default:` 之前插入：**

```go
case agentapi.ActionVolumeRemove:
    err = w.removeVolumes(ctx, request)
```

**在错误码映射块（line 65-88）追加 `volume_in_use` 映射**，紧跟 `errorCode := "host_action_failed"` 行下方添加：

```go
if err != nil && strings.HasPrefix(err.Error(), "volume_in_use:") {
    errorCode = "volume_in_use"
}
```

**(c) 在 `runDocker`（line 832-840）之前或之后定义 `removeVolumes` 方法：**

```go
// removeVolumes 处理 ActionVolumeRemove：遍历 request.Volumes 调 removeDockerVolume，
// 任一失败立即写 audit event 并 return（D-14 + D-21 metadata 不写凭据）。
func (w *Worker) removeVolumes(ctx context.Context, request agentapi.HostActionRequest) error {
	force := request.Labels["force"] == "true"
	for _, vm := range request.Volumes {
		if err := removeDockerVolume(ctx, vm.Name, force); err != nil {
			_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
				Level:   "error",
				Type:    "claude_account.volume_rm_failed",
				Message: err.Error(),
				Metadata: map[string]any{
					"volume_name": vm.Name,
					"force":       force,
				},
			})
			return err
		}
	}
	return nil
}
```

**(d) 在 `createHost`（line 194-249）的 `runDocker rm -f` 之后（line 209）、`hostname := request.Hostname`（line 211）之前插入自动补 volume block（D-04/D-05/D-06/D-07）：**

```go
// Phase 33 D-04/D-05/D-06：仅当 ClaudeAccountID 非空时自动补 claude-state volume + mount + upsert persistent_volume_name。
// 空 ClaudeAccountID 走 D-07 fallback：跳过，不报错（v2.0 旧 host 重建路径）。
if request.ClaudeAccountID != "" {
    volumeName, err := BuildClaudeStateVolumeName(request.ClaudeAccountID)
    if err != nil {
        return err
    }
    labels := map[string]string{
        claudeAccountLabelKey: request.ClaudeAccountID,
        claudeManagedLabelKey: claudeManagedLabelVal,
    }
    if err := ensureDockerVolume(ctx, volumeName, labels); err != nil {
        _, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
            HostID:  &request.HostID,
            Level:   "error",
            Type:    "claude_account.volume_create_failed",
            Message: err.Error(),
            Metadata: map[string]any{
                "account_id":  request.ClaudeAccountID,
                "volume_name": volumeName,
            },
        })
        return fmt.Errorf("ensureDockerVolume: %w", err)
    }

    // 显式优先（D-05）：上游已显式带同 Name 的 mount 则跳过补写
    already := false
    for _, vm := range request.Volumes {
        if vm.Name == volumeName {
            already = true
            break
        }
    }
    if !already {
        request.Volumes = append(request.Volumes, agentapi.VolumeMount{
            Name:   volumeName,
            Target: claudeStateMountTarget,
            Labels: labels,
        })
    }

    // D-06：upsert persistent_volume_name（仅在当前为 NULL 时写入；冲突写 audit 但不阻塞容器启动）
    if err := w.repo.UpsertClaudeAccountPersistentVolumeName(ctx, request.ClaudeAccountID, volumeName); err != nil {
        _, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
            HostID:  &request.HostID,
            Level:   "warn",
            Type:    "claude_account.volume_name_persist_failed",
            Message: err.Error(),
            Metadata: map[string]any{
                "account_id":  request.ClaudeAccountID,
                "volume_name": volumeName,
            },
        })
        // D-07：不阻塞容器启动
    }
}
```

**(e) 在 `WorkerRepo` 接口（line 32-37）追加一行：**

```go
type WorkerRepo interface {
    UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
    UpdateHostStatus(ctx context.Context, hostID string, status string) error
    GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
    RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
    UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error // Phase 33 D-06
}
```

**(f) 新建 `internal/runtime/tasks/worker_volume_lifecycle_test.go`，含 7 条单测**（**verbatim 复制 RESEARCH §5.1 + 补 Build/Idempotent/Force 三条**）：

```go
package tasks

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBuildClaudeStateVolumeName_NonEmptyID_ReturnsPrefixedName(t *testing.T) {
	got, err := BuildClaudeStateVolumeName("acct-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "claude-state-acct-42" {
		t.Fatalf("want claude-state-acct-42, got %q", got)
	}
}

func TestBuildClaudeStateVolumeName_EmptyID_ReturnsError(t *testing.T) {
	_, err := BuildClaudeStateVolumeName("")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("error must mention 'required', got %q", err.Error())
	}
}

func TestEnsureDockerVolume_NotExists_RunsCreate(t *testing.T) {
	calls := 0
	gotArgs := [][]string{}
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		gotArgs = append(gotArgs, args)
		if calls == 1 {
			return []byte("Error: No such volume"), fmt.Errorf("exit 1")
		}
		return []byte("claude-state-abc\n"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })

	if err := realEnsureDockerVolume(context.Background(), "claude-state-abc",
		map[string]string{"com.cloud-cli-proxy.account_id": "abc", "com.cloud-cli-proxy.managed": "true"}); err != nil {
		t.Fatalf("create flow should succeed: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 docker calls (inspect+create), got %d (args=%v)", calls, gotArgs)
	}
	if gotArgs[0][0] != "inspect" {
		t.Errorf("first call must be inspect, got %v", gotArgs[0])
	}
	if gotArgs[1][0] != "create" {
		t.Errorf("second call must be create, got %v", gotArgs[1])
	}
}

func TestEnsureDockerVolume_AlreadyExists_SkipsCreate(t *testing.T) {
	calls := 0
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		return []byte("[{\"Name\":\"claude-state-abc\"}]"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realEnsureDockerVolume(context.Background(), "claude-state-abc", map[string]string{}); err != nil {
		t.Fatalf("inspect-success path must be nil: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 docker call (inspect only), got %d", calls)
	}
}

func TestRemoveDockerVolume_NotFound_IsSuccess(t *testing.T) {
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("Error response from daemon: get claude-state-abc: no such volume"), fmt.Errorf("exit 1")
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realRemoveDockerVolume(context.Background(), "claude-state-abc", false); err != nil {
		t.Errorf("not-found must be treated as success, got: %v", err)
	}
}

func TestRemoveDockerVolume_InUse_PropagatesVolumeInUseError(t *testing.T) {
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("Error response from daemon: remove claude-state-abc: volume is in use - [container_xyz]"), fmt.Errorf("exit 1")
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	err := realRemoveDockerVolume(context.Background(), "claude-state-abc", false)
	if err == nil {
		t.Fatal("in-use must produce an error")
	}
	if !strings.HasPrefix(err.Error(), "volume_in_use:") {
		t.Errorf("error must start with volume_in_use:, got: %v", err)
	}
}

func TestRemoveDockerVolume_ForceTrue_PassesDashF(t *testing.T) {
	var captured []string
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		captured = args
		return []byte("claude-state-abc"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	if err := realRemoveDockerVolume(context.Background(), "claude-state-abc", true); err != nil {
		t.Fatalf("force=true success path: %v", err)
	}
	if len(captured) < 2 || captured[0] != "rm" || captured[1] != "-f" {
		t.Errorf("force=true must pass [rm -f ...], got %v", captured)
	}
}
```

**关键不变量（verbatim 守恒）：**
- 包级常量名 `claudeStateVolumePrefix` / `claudeStateMountTarget` / `claudeAccountLabelKey` / `claudeManagedLabelKey` / `claudeManagedLabelVal`
- 字符串值 `"claude-state-"` / `"/var/lib/claude-persist"` / `"com.cloud-cli-proxy.account_id"` / `"com.cloud-cli-proxy.managed"` / `"true"`
- 函数名 `BuildClaudeStateVolumeName` / `realEnsureDockerVolume` / `realRemoveDockerVolume`
- 包级 var 名 `ensureDockerVolume` / `removeDockerVolume` / `dockerVolumeRunner`
- 错误码字符串 `"volume_in_use"`（D-15 + RESEARCH §4 错误码表）
- audit event Type `claude_account.volume_create_failed` / `claude_account.volume_name_persist_failed` / `claude_account.volume_rm_failed`
- metadata key 仅 `account_id` / `volume_name` / `error_code` / `error_message` / `force` / `host_id`（**禁止** OAuth token / EntryPassword / email）
- `request.ClaudeAccountID == ""` 跳过分支（D-07 fallback，禁报错）
- createHost 插入位置：`runDocker rm -f` 之后、`hostname := request.Hostname` 之前

**禁止：**
- 在 `removeDockerVolume` 内做 `inspect` fast-fail（CONTEXT Discretion 第 4 条：直接 rm 让 docker 自己判断）
- 在 `ensureDockerVolume` 内解析 inspect JSON 比对 label（推迟到 v3.1 backlog，参 RESEARCH §6.6）
- 添加 `// 增量注释 / TODO 描述本次改动`（按 making_code_changes 规则，注释只解释非显然意图）
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "claudeStateVolumePrefix = \"claude-state-\"" internal/runtime/tasks/worker.go
grep -q "claudeStateMountTarget  = \"/var/lib/claude-persist\"" internal/runtime/tasks/worker.go
grep -q "func BuildClaudeStateVolumeName" internal/runtime/tasks/worker.go
grep -q "var ensureDockerVolume = realEnsureDockerVolume" internal/runtime/tasks/worker.go
grep -q "var removeDockerVolume = realRemoveDockerVolume" internal/runtime/tasks/worker.go
grep -q "var dockerVolumeRunner" internal/runtime/tasks/worker.go
grep -q "case agentapi.ActionVolumeRemove:" internal/runtime/tasks/worker.go
grep -q "func (w \\*Worker) removeVolumes" internal/runtime/tasks/worker.go
grep -q "UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error" internal/runtime/tasks/worker.go
grep -q "claude_account.volume_create_failed" internal/runtime/tasks/worker.go
grep -q "claude_account.volume_name_persist_failed" internal/runtime/tasks/worker.go
grep -q "claude_account.volume_rm_failed" internal/runtime/tasks/worker.go
grep -q "errorCode = \"volume_in_use\"" internal/runtime/tasks/worker.go
test -f internal/runtime/tasks/worker_volume_lifecycle_test.go
go vet ./internal/runtime/tasks/...
go test ./internal/runtime/tasks/ -run "TestBuildClaudeStateVolumeName|TestEnsureDockerVolume|TestRemoveDockerVolume" -count=1 -v
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "case agentapi.ActionVolumeRemove:" internal/runtime/tasks/worker.go` 输出 = 1
    - `grep -c "claudeStateMountTarget" internal/runtime/tasks/worker.go` ≥ 2（常量定义 + createHost 调用）
    - `grep -c "ensureDockerVolume(ctx, volumeName, labels)" internal/runtime/tasks/worker.go` 输出 = 1（createHost 内调用就位）
    - `grep -q "UpsertClaudeAccountPersistentVolumeName" internal/runtime/tasks/worker.go` 命中（接口扩展 + createHost 调用）
    - `go test ./internal/runtime/tasks/ -run TestBuildClaudeStateVolumeName -count=1` 退出码 = 0
    - `go test ./internal/runtime/tasks/ -run TestEnsureDockerVolume -count=1` 退出码 = 0（4 条子测全 PASS）
    - `go test ./internal/runtime/tasks/ -run TestRemoveDockerVolume -count=1` 退出码 = 0（3 条子测全 PASS）
    - **接口契约断言**：`go build ./...` 全仓库 PASS（WorkerRepo 接口扩展不破坏 Repository 实现 — Repository 必须在 Task 1.4 同时实现新方法以满足接口）
    - audit event metadata 不含凭据：`grep -nE "EntryPassword|credentials|oauth_token|email" internal/runtime/tasks/worker.go` 无新增命中（仅既有引用）
  </acceptance_criteria>

  <done>
    - 5 个新增 worker 包级符号（`BuildClaudeStateVolumeName` / `dockerVolumeRunner` / `ensureDockerVolume` / `removeDockerVolume` / `removeVolumes`）就位
    - Execute switch 增 ActionVolumeRemove case + 错误码映射 `volume_in_use`
    - createHost 在 ClaudeAccountID 非空时自动补 mount + upsert + 失败写 audit
    - WorkerRepo 接口扩展 1 行（必须在 Task 1.4 同时实现仓储方法才能 build 通过）
    - 7 条 lifecycle 单测全 PASS
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 1.4: 仓储新增 UpsertClaudeAccountPersistentVolumeName SQL + 三态语义实现 + 单测（D-06 / D-25.7）</name>
  <files>internal/store/repository/queries.go, internal/store/repository/queries_claude_account_volume_test.go</files>

  <read_first>
    - internal/store/repository/queries.go（特别是 line 1180-1260 claude_account 查询群 + line 1260-1271 UpdateHostEntryPassword 模式）
    - internal/store/repository/models.go（特别是 line 209-222 ClaudeAccount.PersistentVolumeName *string 三态语义）
    - internal/store/repository/queries_host_entry_password_test.go（沿用 Phase 29.1 SQL 字符串断言风格）
    - .planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md §D-06（三态语义） + §D-25.7
    - .planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md §2.4 (a)
    - .planning/phases/33-claude-code-cli-admin-gc/33-PATTERNS.md `internal/store/repository/queries.go` 节 + Shared 2（SQL 包级 const 提升）
    - .planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md Plan 01 SQL 包级 const 强制约定
  </read_first>

  <behavior>
    - Test 1 (TestUpsertClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens)：纯 SQL 字符串断言 `upsertClaudeAccountPersistentVolumeNameSQL` 必须包含子串 `"WHERE id = $1 AND persistent_volume_name IS NULL"` 与 `"UPDATE claude_accounts"` 与 `"updated_at = NOW()"`
    - Test 2 (TestCheckClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens)：`checkClaudeAccountPersistentVolumeNameSQL` 包含 `"COALESCE(persistent_volume_name, '')"` 与 `"FROM claude_accounts"` 与 `"WHERE id = $1"`
    - Test 3 (TestUpsertClaudeAccountPersistentVolumeName_EmptyArgs_ReturnsError)：`Upsert(ctx, "", "x")` 与 `Upsert(ctx, "x", "")` 都返回 error 且不触达 db（用 nil pool 验证 fast-fail）
  </behavior>

  <action>
**(a) 在 `internal/store/repository/queries.go` 文件末尾追加（**verbatim 复制 RESEARCH §2.4 (a)**）：**

```go
const upsertClaudeAccountPersistentVolumeNameSQL = `
	UPDATE claude_accounts
	SET persistent_volume_name = $2, updated_at = NOW()
	WHERE id = $1 AND persistent_volume_name IS NULL
`

const checkClaudeAccountPersistentVolumeNameSQL = `
	SELECT COALESCE(persistent_volume_name, '')
	FROM claude_accounts
	WHERE id = $1
`

// UpsertClaudeAccountPersistentVolumeName 实现 D-06 三态语义：
//   - persistent_volume_name IS NULL → 写入 volumeName（一次往返）
//   - 已是相同 volumeName → 跳过返回 nil
//   - 已是其他值（冲突） → 返回错误（调用方写 audit）
//
// 不允许从已分配回写 NULL（与 Phase 30 D-02 三态消除一致）。
func (r *Repository) UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error {
	if accountID == "" || volumeName == "" {
		return fmt.Errorf("upsert claude_account persistent_volume_name: empty arg")
	}
	tag, err := r.db.Exec(ctx, upsertClaudeAccountPersistentVolumeNameSQL, accountID, volumeName)
	if err != nil {
		return fmt.Errorf("update persistent_volume_name: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var current string
	if err := r.db.QueryRow(ctx, checkClaudeAccountPersistentVolumeNameSQL, accountID).Scan(&current); err != nil {
		return fmt.Errorf("verify persistent_volume_name: %w", err)
	}
	if current == volumeName {
		return nil
	}
	return fmt.Errorf("persistent_volume_name conflict: current=%q want=%q", current, volumeName)
}
```

**(b) 新建 `internal/store/repository/queries_claude_account_volume_test.go`：**

```go
package repository

import (
	"context"
	"strings"
	"testing"
)

func TestUpsertClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens(t *testing.T) {
	must := []string{
		"UPDATE claude_accounts",
		"SET persistent_volume_name = $2",
		"updated_at = NOW()",
		"WHERE id = $1 AND persistent_volume_name IS NULL",
	}
	for _, token := range must {
		if !strings.Contains(upsertClaudeAccountPersistentVolumeNameSQL, token) {
			t.Errorf("upsertClaudeAccountPersistentVolumeNameSQL missing %q\nfull:\n%s", token, upsertClaudeAccountPersistentVolumeNameSQL)
		}
	}
}

func TestCheckClaudeAccountPersistentVolumeNameSQL_ContainsKeyTokens(t *testing.T) {
	must := []string{
		"COALESCE(persistent_volume_name, '')",
		"FROM claude_accounts",
		"WHERE id = $1",
	}
	for _, token := range must {
		if !strings.Contains(checkClaudeAccountPersistentVolumeNameSQL, token) {
			t.Errorf("checkClaudeAccountPersistentVolumeNameSQL missing %q\nfull:\n%s", token, checkClaudeAccountPersistentVolumeNameSQL)
		}
	}
}

func TestUpsertClaudeAccountPersistentVolumeName_EmptyArgs_ReturnsError(t *testing.T) {
	r := &Repository{} // nil db is OK because empty-arg branch returns before touching db
	if err := r.UpsertClaudeAccountPersistentVolumeName(context.Background(), "", "x"); err == nil {
		t.Error("empty accountID must return error")
	}
	if err := r.UpsertClaudeAccountPersistentVolumeName(context.Background(), "x", ""); err == nil {
		t.Error("empty volumeName must return error")
	}
}
```

**verbatim 字段守恒：**
- SQL 常量名 `upsertClaudeAccountPersistentVolumeNameSQL` / `checkClaudeAccountPersistentVolumeNameSQL`
- 方法名 `UpsertClaudeAccountPersistentVolumeName`
- SQL 关键 token：`UPDATE claude_accounts` / `WHERE id = $1 AND persistent_volume_name IS NULL` / `updated_at = NOW()` / `COALESCE(persistent_volume_name, '')`
- 错误信息前缀 `"upsert claude_account persistent_volume_name: empty arg"`
- 三态分支语义不可变更（`RowsAffected()==1` 写入成功 / `==0` 走 SELECT 比较 / 不一致返回 conflict 错误）

**禁止：**
- 行内拼接 SQL 字符串（必须包级 `const xxxSQL`，Phase 29.1 强制约定）
- 实现 NULL→NULL 回写路径（违反 D-06 三态消除）
- 改用 `pgxpool` 真库集成测试（CONTEXT D-27：不引入 docker compose fixture，沿用 Phase 31/32 t.Skip 风格；本 plan 仅做 SQL 字符串断言 + 空参数边界）
  </action>

  <verify>
    <automated>
bash -c 'set -e
grep -q "const upsertClaudeAccountPersistentVolumeNameSQL" internal/store/repository/queries.go
grep -q "const checkClaudeAccountPersistentVolumeNameSQL" internal/store/repository/queries.go
grep -q "WHERE id = \\\$1 AND persistent_volume_name IS NULL" internal/store/repository/queries.go
grep -q "func (r \\*Repository) UpsertClaudeAccountPersistentVolumeName" internal/store/repository/queries.go
test -f internal/store/repository/queries_claude_account_volume_test.go
go vet ./internal/store/repository/...
go test ./internal/store/repository/ -run "TestUpsertClaudeAccountPersistentVolumeNameSQL|TestCheckClaudeAccountPersistentVolumeNameSQL|TestUpsertClaudeAccountPersistentVolumeName_EmptyArgs" -count=1 -v
# WorkerRepo 接口契约：Repository 必须实现新方法（Task 1.3 接口扩展才能 build 通过）
go build ./...
echo "OK"'
    </automated>
  </verify>

  <acceptance_criteria>
    - `grep -c "^const upsertClaudeAccountPersistentVolumeNameSQL" internal/store/repository/queries.go` 输出 = 1
    - `grep -c "^const checkClaudeAccountPersistentVolumeNameSQL" internal/store/repository/queries.go` 输出 = 1
    - `grep -c "func (r \*Repository) UpsertClaudeAccountPersistentVolumeName" internal/store/repository/queries.go` 输出 = 1
    - 3 条新增 SQL/边界单测全 PASS（`go test ./internal/store/repository/ -run "TestUpsert.*PersistentVolumeName|TestCheck.*PersistentVolumeName"`）
    - `go build ./...` 退出码 = 0（关键：Task 1.3 在 WorkerRepo 加了方法，本 task 仓储实现必须同步落地，否则 worker 测试无法编译）
    - 既有 worker 测试不回归：`go test ./internal/runtime/tasks/ -count=1` 退出码 = 0（无 hang，避免 docker 调用）
  </acceptance_criteria>

  <done>
    - `Repository.UpsertClaudeAccountPersistentVolumeName` 三态语义实现完整且单测覆盖空参数边界
    - SQL 提升为包级 const，关键 token grep 可断言
    - `go build ./...` PASS（WorkerRepo 接口契约闭环）
    - 既有 worker 单测无回归
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| control-plane → host-agent (`agentapi.Client.RunHostAction` HTTP) | 复用 Phase 1 已建立的 host-agent token 鉴权（unix socket / token header），本 plan 不引入新 endpoint |
| host-agent → docker daemon (`exec.CommandContext("docker", ...)`) | docker socket 已是 host-agent 既有信任边界 |
| 容器内 `entrypoint.sh` → docker volume `/var/lib/claude-persist` | volume 内容物 owner 1000:1000；UID 0 (root) 仅在 entrypoint chown 阶段触达 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-33-01 | Spoofing | `agentapi.ActionVolumeRemove` 通过既有 `/v1/host-actions` endpoint | accept | 沿用 Phase 1 host-agent token 鉴权（unix socket only）+ 不新增 endpoint，攻击面 0 增量；rationale：control-plane 调用方需先持有 host-agent socket 访问权 |
| T-33-02 | Tampering | `prepare_persistent_state` 的 `cp -an` 复制阶段读 `/home/claude/.claude/` | mitigate | 函数仅读取镜像预建路径（Dockerfile 时已 chown 1000:1000）+ 容器内不开启 root SSH（Phase 1 已强制），无 host 路径泄漏风险；`|| true` 兜底使 immutable 文件不阻塞但写 stderr 可见 |
| T-33-03 | Repudiation | `removeVolumes` 删除 named volume 无审计 | mitigate | 任一 volume rm 失败立即 `RecordEvent(claude_account.volume_rm_failed)` 含 `volume_name` + `force` + 错误原文；成功路径不写事件（Plan 02 admin handler 在事务结束时统一写 `claude_account.deleted` 总账） |
| T-33-04 | Information Disclosure | audit event metadata 误写 OAuth token / EntryPassword | mitigate | metadata key 白名单：`account_id` / `volume_name` / `error_code` / `error_message` / `force` / `host_id`；**禁止** `email` / `entry_password` / `credentials` / `oauth_token` 任一字符串出现（PATTERNS Shared 3 + Phase 29.1 Plan 02 mitigation） |
| T-33-05 | Information Disclosure | entrypoint `cp -an` 把 host bind-mount 路径泄漏到 volume | accept | rationale：`cp -an /home/claude/.claude/.` 源是 Dockerfile 镜像层路径（非 bind mount），不会引入 host 文件系统；`/home/claude` 在 v3 镜像里是镜像 owned 目录（Phase 29 Dockerfile 验证） |
| T-33-06 | Denial of Service | docker daemon hang 拖死 worker | mitigate | `dockerVolumeRunner` 使用 `exec.CommandContext(ctx, ...)`；上游 worker.Execute 已带 ctx 超时（既有约束）；admin handler 在 Plan 02 用 `context.WithTimeout` 10s/30s 兜底（D-20） |
| T-33-07 | Elevation of Privilege | `ensureDockerVolume` 接受任意 `name` 串可执行 docker 命令注入 | mitigate | `name` 字符串经 `BuildClaudeStateVolumeName(claudeAccountID)` 生成（前缀固定 `claude-state-` + UUID），`exec.CommandContext` 逐参数传入（无 shell 解释）；`labels` 经 `fmt.Sprintf("%s=%s", k, v)` 但同样走 `exec.Command` 参数列表，不存在 shell injection 路径 |
| T-33-08 | Tampering | worker 接收伪造 `request.ClaudeAccountID` 触达任意 volume | accept | rationale：`ClaudeAccountID` 由 control-plane EntryHandler 注入（Phase 30 D-09），上游已校验；worker 端再校验等于重复信任边界。volume 名固定 `claude-state-{id}`，最坏情况是创建多余 volume（M16 已被 Plan 02 admin DELETE 清理覆盖） |

**ASVS L1 高严重度阻塞性威胁：** 0 — T-33-04（凭据泄漏）通过 metadata key 白名单 + grep CI 断言（acceptance_criteria）已 mitigate。
</threat_model>

<verification>
## Plan-level 验证

```bash
# 1. 全仓库构建（接口契约闭环）
go build ./...

# 2. 单元测试（4 个 task 共 12+ 条新增/修改测试）
go test ./internal/agentapi/ -count=1
go test ./internal/runtime/tasks/ -count=1 -run "TestHostActionRequest_VolumeRemove_RoundTrip|TestActionVolumeRemove_StringValue|TestBuildClaudeStateVolumeName|TestEnsureDockerVolume|TestRemoveDockerVolume"
go test ./internal/store/repository/ -count=1 -run "TestUpsert.*PersistentVolumeName|TestCheck.*PersistentVolumeName"

# 3. 既有 worker / 仓储单测无回归（关键 SC：WorkerRepo 接口扩展不破坏现有实现）
go test ./internal/agentapi/... ./internal/runtime/tasks/... ./internal/store/repository/... -count=1

# 4. entrypoint 语法 + grep 守卫
bash -n deploy/docker/managed-user/entrypoint.sh
grep -q "^prepare_persistent_state()" deploy/docker/managed-user/entrypoint.sh
grep -q "^prepare_persistent_state$" deploy/docker/managed-user/entrypoint.sh

# 5. audit event metadata 白名单（禁止凭据出现）
! grep -nE "Metadata:.*\"(email|entry_password|credentials|oauth_token)\"" internal/runtime/tasks/worker.go

# 6. ClaudeAccountID dispatcher 链路覆盖率（RESEARCH §6.5 风险）
# 仅做 informational grep — 若 dispatcher 链路没填该字段，worker 走 D-07 fallback（不报错），由 Plan 02 UAT 兜底
grep -rn "ClaudeAccountID:" internal/controlplane/ internal/runtime/ | head -5
```

## SC 映射（ROADMAP §Phase 33 Success Criteria）

| SC | 验证手段（本 plan 部分） |
|----|----------------------|
| SC1 (REQ-F7-B 容器重建 OAuth 保留) | entrypoint symlink 落地（Task 1.1）+ worker 自动补 mount（Task 1.3）；端到端验证由 Plan 02 UAT D-26 完成 |
| SC2 (REQ-F7-A volume 命名 + label) | `BuildClaudeStateVolumeName` 单测断言 `claude-state-<id>` + worker 拼装两条 label（Task 1.3） |
| SC3 (容器内目录属主 1000:1000) | entrypoint `chown -R` + `chown -h` 双重（Task 1.1）；运行时验证由 Plan 02 UAT |
| SC5 (host-agent volume create 幂等) | `TestEnsureDockerVolume_AlreadyExists_SkipsCreate` 单测覆盖（Task 1.3） |
| SC4 (admin DELETE 事务联动) | 本 plan 提供 `ActionVolumeRemove` + `removeDockerVolume` 协议入口；事务编排由 Plan 02 实现 |
| SC6 (volume rm 失败事务回滚) | 本 plan 提供 `volume_in_use` 错误码 + audit event；回滚行为由 Plan 02 实现 |
</verification>

<success_criteria>
- [ ] 4 个 task 全部完成，各 task 的 acceptance_criteria 全 PASS
- [ ] `go build ./...` 退出码 = 0（WorkerRepo 接口契约闭环）
- [ ] 12+ 条新增/修改单测全 PASS
- [ ] `bash -n deploy/docker/managed-user/entrypoint.sh` 退出码 = 0
- [ ] grep 断言：所有 verbatim 字段（函数名、SQL token、协议常量字符串）逐一就位
- [ ] audit event metadata 不含凭据（grep 验证）
- [ ] 既有 worker / 仓储测试无回归
</success_criteria>

<output>
After completion, create `.planning/phases/33-claude-code-cli-admin-gc/33-01-SUMMARY.md` with:
- 4 个 task 的实际 commit SHA + 关键 diff 片段引用
- WorkerRepo 接口扩展的影响面（Repository / 任何 mock 实现）
- audit event 类型清单 + metadata 白名单实测样本
- 7 条 lifecycle 单测的 PASS 时间戳
- ClaudeAccountID dispatcher 覆盖率 grep 实测（提供给 Plan 02 admin UAT 用）
- carry-over：是否需要在 v3.1 backlog 加 `ensureDockerVolume` label 比对（RESEARCH §6.6）
</output>
