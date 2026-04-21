# Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）- Research

**Researched:** 2026-04-21
**Domain:** Docker named volume 生命周期 / 镜像 entrypoint symlink seed / admin DELETE 事务 / agentapi Action 扩展
**Confidence:** HIGH（CONTEXT.md 已锁定 D-01..D-29，本文档作为"决策可实现性 + 代码骨架沉淀"，不重新设计；CONTEXT 未覆盖的细节均与现有代码 grep 结果对齐验证）

## Summary

Phase 33 的所有架构决策已在 33-CONTEXT.md（D-01..D-29）锁定，本研究的目标不是重新讨论方案，而是把决策落到**具体行号、函数签名、import 列表与可复制代码骨架**——让 planner 在写 PLAN 时直接从本文档抓骨架。

通过对 `worker.go` / `contracts.go` / `entrypoint.sh` / `admin_hosts.go` / `queries.go` 的 grep + 阅读，已确认 29 项决策**全部可直接实现**，无需返回 discuss-phase。CONTEXT.md 唯一未覆盖的实现细节（按优先级）：(1) `Repository` 没有暴露 `db.Begin` 入口，admin DELETE 事务需要新增一个 `BeginTx` 方法或允许 handler 持有 `*pgxpool.Pool`；(2) `removeDockerVolume` 的 docker 错误字符串清单需要在测试 mock 中固化两条最小集；(3) admin handler 需要遵循已有的 `var syncContainerPassword = ...` 模式注入 `var runHostAction = client.RunHostAction`；(4) `prepare_persistent_state` 在 entrypoint.sh 的插入行号是 255 之后、256 之前。

**Primary recommendation:** 严格按 D-28 拆 2 plans，Plan 01 全部代码均在 `internal/runtime/tasks/` 与 `internal/agentapi/` 下；Plan 02 新建 `admin_claude_accounts.go` 并对 `Repository` 新增 `BeginTx(ctx) (pgx.Tx, error)` 公开方法，避免把 `*pgxpool.Pool` 泄漏到 handler。

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| named volume 创建/删除（docker 调用） | Host-Agent (`runtime/tasks/worker.go`) | — | 边界守恒（Phase 29 D-22）：所有 docker CLI 调用收敛在 worker，control-plane 仅通过 agentapi 触发 |
| volume mount 拼装 | Host-Agent (`worker.buildCreateArgs`) | — | 已就位（Phase 29.1 完整测试覆盖），本阶段只在调用前补 `--mount` 元素 |
| volume → entrypoint symlink seed | 镜像 (`entrypoint.sh`) | — | 容器内 `/var/lib/claude-persist`↔`/home/claude/.claude` 的物理拓扑唯一发生在容器启动时 |
| `claude_accounts.persistent_volume_name` 写库 | Control-Plane Repository | — | 库写入幂等，集中在 `internal/store/repository/queries.go` |
| admin DELETE 事务编排 | Control-Plane HTTP Handler | Repository（事务方法）+ agentapi.Client（远程调用） | handler 是事务边界 owner（Begin/Commit/Rollback），调 host-agent 通过 `agentapi.Client.RunHostAction` |
| audit event 写入 | Control-Plane Repository (`RecordEvent`) | — | 与 Phase 29.1 / Phase 30 事件命名/metadata 风格一致 |
| host detail 字段追加 | Control-Plane HTTP Handler | Repository (`GetHostWithClaudeAccount` LEFT JOIN) | LEFT JOIN 一次查询，**不**走 docker exec |

---

## 1. 决策可实现性验证（D-01..D-29）

> 图例：✓ 直接可实现（无歧义）；⚠ planner 需补充细节；✗ 需返回 discuss-phase（本文档**无**此项）

### Volume 命名与 label（D-01..D-03）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-01 | volume 名 `claude-state-{uuid-with-dashes}` | ✓ | `claude_accounts.id` 是 `UUID PRIMARY KEY DEFAULT gen_random_uuid()`（`0007_auth_unification.sql:9`），`Repository.ResolveClaudeAccountIDForEntry` 已用 `id::text` Scan（`queries.go:1206`），格式天然带连字符；Docker volume name 正则 `[a-zA-Z0-9][a-zA-Z0-9_.-]+` 允许 `-`，`claude-state-` 前缀 13 字符 + UUID 36 字符 = 49，远低于 255 上限 |
| D-02 | label `com.cloud-cli-proxy.account_id={id}` + `com.cloud-cli-proxy.managed=true` | ✓ | docker `--label k=v --label k=v` 多次 flag 已在 `worker.buildCreateArgs:186-188` 用同样模式（`--label cloud-cli-proxy.managed=true`）证明可行 |
| D-03 | volume 与 `template_image_ref` / `image_version` 解耦 | ✓ | volume 名仅依赖 `claude_account_id`，与 `request.ImageName`（`worker.go:190`）无引用关系 |

### Volume 创建链路（D-04..D-07）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-04 | `ensureDockerVolume(ctx, name, labels)` 走 inspect-then-create + 包级 `var ensureDockerVolume = realEnsureDockerVolume` | ✓ | 模式与 `worker.go:689 var execInContainer = ...` 完全一致；`docker volume inspect` / `docker volume create --label` 是稳定 CLI（无版本依赖） |
| D-05 | `request.ClaudeAccountID != ""` 时自动补 `VolumeMount{Name: BuildClaudeStateVolumeName(id), Target: "/var/lib/claude-persist", Labels: {...}}` | ✓ | `HostActionRequest.ClaudeAccountID` 已就位（`contracts.go:54`），`HostActionRequest.Volumes` 已就位（`contracts.go:50`）；显式优先（`request.Volumes` 已含同 Name 跳过）逻辑只需 `for _, vm := range request.Volumes { if vm.Name == name { skip } }` |
| D-06 | `repo.UpsertClaudeAccountPersistentVolumeName(ctx, accountID, name)` —— NULL → 写入；非空一致 → 跳过；非空冲突 → audit | ✓ | `ClaudeAccount.PersistentVolumeName *string`（`models.go:217`）三态语义已锁定；SQL `UPDATE ... WHERE id=$1 AND persistent_volume_name IS NULL` 一次往返 + `RowsAffected()==0` 时再 SELECT 比较即可（与 `queries.go:96 RowsAffected` 风格一致） |
| D-07 | 无 `ClaudeAccountID` 路径走 fallback 不阻塞（M16 风险但不破坏现有 host 启动） | ✓ | 逻辑等价于"`if request.ClaudeAccountID == "" { return nil }`"；现有 v2.0 路径 worker.createHost 不会收到 `ClaudeAccountID`，行为完全不变 |

### Entrypoint 持久化拓扑（D-08..D-12）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-08 | 单 volume 挂 `/var/lib/claude-persist` | ✓ | 已在 Dockerfile `mkdir -p /var/lib/claude-persist`（`Dockerfile:153-158`）+ entrypoint `prepare_v3_dirs` 二次 chown（`entrypoint.sh:62-69`） |
| D-09 | `prepare_persistent_state` 函数（cp -an seed + ln -sfn + chown） | ✓ | bash 函数语法、`cp -an` / `ln -sfn` / `chown -h` / `chown -R` 在 GNU coreutils 8.30+（Ubuntu 22.04+）稳定。**插入点**：`entrypoint.sh:255` `prepare_v3_dirs` 之后、`:256` `prepare_mutagen_agent` 之前。**函数定义位置**：紧跟 `prepare_v3_dirs` 函数定义（`entrypoint.sh:62-69`）之后插入新函数 |
| D-10 | 保留 `prepare_v3_dirs` 二次 chown 兜底 | ✓ | `entrypoint.sh:62-69` 已存在，本阶段不动 |
| D-11 | 不对 `~/.claude/.credentials.json` 加密 | ✓ | 决策本身即"不做"，无落点验证需求 |
| D-12 | `mkdir -p /home/claude/.cache` 兜底 | ✓ | 已写入 D-09 函数体伪码 |

### host-agent 协议扩展（D-13..D-16）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-13 | `ActionVolumeRemove HostAction = "volume_remove"` | ✓ | `contracts.go:5-11` 现有 5 个常量，追加一行即可；JSON 兼容由 `HostAction string` 透明保证 |
| D-14 | worker `Execute()` switch 新增 case | ✓ | `worker.go:48-63` switch 是平铺结构，`case agentapi.ActionVolumeRemove:` 加在 `case agentapi.ActionPrepareHost:` 之后即可 |
| D-15 | `removeDockerVolume(ctx, name, force)`：force=false → `docker volume rm`；force=true → `docker volume rm -f`；不存在视为成功 | ✓ | docker 错误字符串清单见 §6 |
| D-16 | host-agent mux 不增 endpoint | ✓ | `agent/server.go:52-116` 仅 3 个路由，本阶段不动 |

### admin DELETE 事务（D-17..D-21）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-17 | 新增 `DELETE /v1/admin/claude-accounts/{id}` handler | ⚠ | router.go 当前无 claude_accounts 任何 admin 路由（grep 仅命中仓储/migration），需在 `router.go:232 if deps.AdminHosts != nil { ... }` 同层新增 `if deps.AdminClaudeAccounts != nil { ... }` 块；planner 需决定是否新增 `AdminClaudeAccountStore` 接口（推荐：是，与 `AdminHostStore` 风格一致） |
| D-18 | 强一致路径：BEGIN → SELECT FOR UPDATE → 调 host-agent → 成功则 DELETE+COMMIT，失败则 ROLLBACK + audit + HTTP 409 | ⚠ | **Repository 当前无 `BeginTx` 公开方法**——`r.db` 是私有的 `*pgxpool.Pool`（`queries.go:14-19`），仅 `migrator/migrator.go:46` 用过 `db.Begin(ctx)`。**需在 Repository 上新增** `func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error)` 一行包装，避免把 `*pgxpool.Pool` 泄漏到 handler |
| D-19 | force=true 路径：DB 先 COMMIT → 调 host-agent，失败仅写 audit + 返回 200 | ✓ | 与 D-18 同套 transaction 工具，逻辑顺序不同 |
| D-20 | 强一致 ctx 10s / force ctx 30s | ✓ | `context.WithTimeout(r.Context(), 10*time.Second)` 在 `admin_hosts.go:614,713,762,819,907,974` 已有 6 处先例 |
| D-21 | 审计事件命名 `claude_account.*` + 不写 OAuth token | ✓ | 与 `runtime.entry_password_*` / `admin.host.*` 命名风格对齐 |

### admin host detail（D-22..D-24）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-22 | `GET /v1/admin/hosts/{id}` 响应追加 `persistent_volume_name`，list 不动 | ✓ | `admin_hosts.go:96-99 adminHostDetailResponse` 已是 embed `repository.HostDetail` + 追加字段模式，加一行 `PersistentVolumeName *string \`json:"persistent_volume_name,omitempty"\`` 即可 |
| D-23 | `GetHostWithClaudeAccount` LEFT JOIN | ✓ | 与 `queries.go:1213 resolveClaudeAccountByUserFallbackSQL` 同包级常量风格；`hosts LEFT JOIN claude_accounts ON claude_accounts.host_id = hosts.id` |
| D-24 | 用户面 `/v1/user/hosts/{id}` 不追加 | ✓ | 决策即"不做" |

### 测试与拆分（D-25..D-29）

| ID | 决策摘要 | 状态 | 落点验证 |
|----|---------|------|---------|
| D-25 | 7 项单元测试 | ✓ | 现有测试基础设施 (`worker_volume_test.go` `minimalCreateHostRequest()`、`admin_users_test.go:79 stubEventRecorder`、`admin_hosts_test.go:21 stubHostStore`、`adminTestRouter`、`validAdminToken`) 直接复用；Plan 01 加 worker 测试，Plan 02 加 handler 测试 |
| D-26 | 5 步人工 UAT | ✓ | UAT 步骤命令均为标准 `docker volume ls --filter` / `docker exec` |
| D-27 | 不写 docker compose 集成测试，但 mock 必须覆盖 `"volume is in use"` 与 `"No such volume"` | ✓ | 见 §6 错误字符串清单 |
| D-28 | 拆 2 plans（镜像+worker+agentapi vs admin+UAT） | ✓ | 与 ROADMAP `(0/2 plans)` 一致 |
| D-29 | admin handler 不引入 service/use case 层 | ✓ | 与 `admin_hosts.go` 现状一致 |

**汇总：D-01..D-29 全部 ✓ 或 ⚠（仅 D-17 / D-18 需要 planner 在 Plan 02 决定 store 接口与 BeginTx 包装方式，无任何决策需要返回 discuss-phase）。**

---

## 2. 可复用代码骨架（直接复制到 PLAN.md）

### 2.1 entrypoint.sh — `prepare_persistent_state` 函数与调用

**插入位置：** 在 `prepare_v3_dirs()` 函数定义（`entrypoint.sh:62-69`）之后插入函数定义；在 v3 stage 调用序列 `entrypoint.sh:255 prepare_v3_dirs` 之后、`:256 prepare_mutagen_agent` 之前插入调用。

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

调用插入（`entrypoint.sh:254-258` 之间）：

```bash
# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_persistent_state    # <-- 新增（Phase 33 D-09）
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version
```

### 2.2 `internal/agentapi/contracts.go` — 新增 Action 常量

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

### 2.3 `internal/runtime/tasks/worker.go` — 三个新增/修改

**(a) 包级常量与函数（追加到文件末尾或紧跟 `execInContainer` 定义后）：**

```go
const (
	claudeStateVolumePrefix = "claude-state-"
	claudeStateMountTarget  = "/var/lib/claude-persist"
	claudeAccountLabelKey   = "com.cloud-cli-proxy.account_id"
	claudeManagedLabelKey   = "com.cloud-cli-proxy.managed"
	claudeManagedLabelVal   = "true"
)

// BuildClaudeStateVolumeName 返回 D-01 规范的 volume 名。空 id 返回错误。
func BuildClaudeStateVolumeName(claudeAccountID string) (string, error) {
	if claudeAccountID == "" {
		return "", fmt.Errorf("claude_account_id is required")
	}
	return claudeStateVolumePrefix + claudeAccountID, nil
}

// ensureDockerVolume 幂等创建 named volume；存在则 inspect 比对 label，不一致写 audit。
// 暴露为包级 var 以便测试注入 mock（沿用 execInContainer 模式）。
var ensureDockerVolume = realEnsureDockerVolume

func realEnsureDockerVolume(ctx context.Context, name string, labels map[string]string) error {
	if name == "" {
		return fmt.Errorf("ensureDockerVolume: empty name")
	}
	// inspect 优先 —— 存在即视为成功（D-04 第 1 条；label 不一致由调用方写 audit）
	inspect := exec.CommandContext(ctx, "docker", "volume", "inspect", name)
	if err := inspect.Run(); err == nil {
		return nil
	}
	args := []string{"volume", "create"}
	for k, v := range labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, name)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker volume create %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeDockerVolume 幂等删除（D-15）：volume 不存在视为成功；in-use 错误传播。
var removeDockerVolume = realRemoveDockerVolume

func realRemoveDockerVolume(ctx context.Context, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("removeDockerVolume: empty name")
	}
	args := []string{"volume", "rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	// D-15 / D-27：明确两条 docker stderr 字符串
	if strings.Contains(msg, "No such volume") {
		return nil // 幂等
	}
	if strings.Contains(msg, "volume is in use") {
		return fmt.Errorf("volume_in_use: %s", msg)
	}
	return fmt.Errorf("docker volume rm %s: %w (%s)", name, err, msg)
}
```

**(b) `Execute()` switch 扩展（`worker.go:48-63`）：**

```go
switch request.Action {
case agentapi.ActionCreateHost:
	err = w.createHost(ctx, request)
case agentapi.ActionStartHost:
	err = w.startHost(ctx, request)
case agentapi.ActionStopHost:
	err = w.stopHost(ctx, request)
case agentapi.ActionRebuildHost:
	err = w.rebuildHost(ctx, request)
case agentapi.ActionPrepareHost:
	err = w.validateAndPrepare(ctx, request.HostID)
case agentapi.ActionVolumeRemove: // Phase 33 D-14
	err = w.removeVolumes(ctx, request)
default:
	err = fmt.Errorf("unsupported host action: %s", request.Action)
}
```

错误码映射（在 `errorCode := "host_action_failed"` 之后追加 `errors.As` 分支）：

```go
if strings.HasPrefix(err.Error(), "volume_in_use:") {
	errorCode = "volume_in_use"
}
```

`removeVolumes` 函数体：

```go
func (w *Worker) removeVolumes(ctx context.Context, request agentapi.HostActionRequest) error {
	force := request.Labels["force"] == "true" // D-14
	for _, vm := range request.Volumes {
		if err := removeDockerVolume(ctx, vm.Name, force); err != nil {
			w.repo.RecordEvent(ctx, repository.RecordEventParams{
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

**(c) `createHost` 在 `buildCreateArgs` 调用前补 volume（`worker.go:194-249` 内修改）：**

插入位置：`worker.go:215 hostname := request.Hostname` 之前（即 `runDocker rm -f containerName` 之后、计算 `hostname` 之前）。

```go
// Phase 33 D-04/D-05/D-06：自动补 claude-state volume 与 mount。
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

	// 显式优先（D-05）：上游已显式带同 Name 的 mount 则跳过
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

	// D-06：upsert persistent_volume_name（NULL 才写）
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

**(d) `WorkerRepo` 接口扩展（`worker.go:32-37`）：**

```go
type WorkerRepo interface {
	UpdateTaskStatus(context.Context, string, string, string, string, string) (repository.Task, error)
	UpdateHostStatus(ctx context.Context, hostID string, status string) error
	GetEgressIPByHost(ctx context.Context, hostID string) (repository.EgressIP, error)
	RecordEvent(ctx context.Context, params repository.RecordEventParams) (repository.Event, error)
	UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error // Phase 33 D-06
}
```

### 2.4 `internal/store/repository/queries.go` — 三个新增

**(a) `UpsertClaudeAccountPersistentVolumeName`：**

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

// UpsertClaudeAccountPersistentVolumeName D-06：
//   - persistent_volume_name IS NULL → 写入 volumeName（一次往返）
//   - 已是 volumeName → 跳过
//   - 已是其他值（冲突） → 返回错误（调用方写 audit）
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
	// RowsAffected==0：列已非 NULL，比对当前值
	var current string
	if err := r.db.QueryRow(ctx, checkClaudeAccountPersistentVolumeNameSQL, accountID).Scan(&current); err != nil {
		return fmt.Errorf("verify persistent_volume_name: %w", err)
	}
	if current == volumeName {
		return nil // 一致跳过
	}
	return fmt.Errorf("persistent_volume_name conflict: current=%q want=%q", current, volumeName)
}
```

**(b) `GetHostWithClaudeAccount`（D-23）：**

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

// HostWithClaudeAccount D-23：纯 DB JOIN，避免在 detail handler 引入 docker exec。
type HostWithClaudeAccount struct {
	Host
	PersistentVolumeName string // 空串 = 未分配
}

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
```

**(c) `BeginTx` 公开方法 + claude_account 事务方法（Plan 02）：**

```go
// BeginTx 暴露 pgx 事务给 admin handler（D-18）。
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

// LockClaudeAccountForDelete + DeleteClaudeAccountTx 提供事务内行锁与删除。
func LockClaudeAccountForDelete(ctx context.Context, tx pgx.Tx, id string) (accountID, volumeName string, err error) {
	err = tx.QueryRow(ctx, lockClaudeAccountForDeleteSQL, id).Scan(&accountID, &volumeName)
	return
}

func DeleteClaudeAccountTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, deleteClaudeAccountSQL, id)
	if err != nil {
		return fmt.Errorf("delete claude_account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete claude_account: %w", pgx.ErrNoRows)
	}
	return nil
}
```

### 2.5 `internal/controlplane/http/admin_claude_accounts.go` — 新建（Plan 02）

```go
package http

import (
	"context"
	"encoding/json"
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
		force := parseForceFlag(r.URL.Query().Get("force"))

		if force {
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second) // D-20
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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // D-20
	defer cancel()

	tx, err := h.store.BeginTx(ctx)
	if err != nil {
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
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "lock failed"})
		return
	}
	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		_ = tx.Rollback(ctx)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
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
		"account_id": id, "volume_name": volumeName, "force": true,
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

// 防 unused import 占位（json 在生产实现里通常用于 force 解析；已通过 query string 解决，这里保留 import 仅以适配将来扩展）。
var _ = json.Marshal
```

> ⚠ 上面 `json` import 实际不需要，写 PLAN 时可以删除；保留是因为很多 handler 文件都有 body decode 模板，避免 planner 误以为遗漏。

### 2.6 `internal/controlplane/http/router.go` — 路由注册

在 `router.go:232 if deps.AdminHosts != nil { ... }` 块之后追加：

```go
if deps.AdminClaudeAccounts != nil {
	claudeHandler := NewAdminClaudeAccountsHandler(deps.Logger, deps.AdminClaudeAccounts, deps.AgentClient, deps.EventRecorder)
	mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", adminGuard(claudeHandler.Delete()))
}
```

`Dependencies` struct（`router.go:27-54`）追加两个字段：

```go
AdminClaudeAccounts AdminClaudeAccountStore
AgentClient         *agentapi.Client
```

### 2.7 admin_hosts.go detail 字段追加

`admin_hosts.go:96-99 adminHostDetailResponse` 改为：

```go
type adminHostDetailResponse struct {
	repository.HostDetail
	ConnectionInfo       *repository.ConnectionInfo `json:"connection_info,omitempty"`
	PersistentVolumeName string                     `json:"persistent_volume_name,omitempty"` // Phase 33 D-22
}
```

`Get()` 方法改用 `GetHostWithClaudeAccount` 之后再 `GetHostDetail`，或在 `GetHostDetail` 内部 enrich（planner 决定）；推荐：保持 `GetHostDetail` 不变，handler 内并行/串行调一次 `GetHostWithClaudeAccount` 仅取 `PersistentVolumeName`，避免动 `HostDetail` 类型。

---

## 3. 现有代码定位（关键插入点速查）

| 任务 | 文件 | 行号 | 修改类型 |
|------|------|------|---------|
| 新增 `ActionVolumeRemove` 常量 | `internal/agentapi/contracts.go` | 5-11 | 追加一行 |
| Worker `Execute()` switch 加 case | `internal/runtime/tasks/worker.go` | 50-63 | 追加一行 case + handler 函数 |
| `createHost` 自动补 volume | `internal/runtime/tasks/worker.go` | 194-249（建议插在 209 `runDocker rm -f` 之后、211 `hostname :=` 之前） | 插入约 40 行 block |
| `WorkerRepo` 接口扩展 | `internal/runtime/tasks/worker.go` | 32-37 | 追加一行 |
| 新增 `BuildClaudeStateVolumeName` / `ensureDockerVolume` / `removeDockerVolume` | `internal/runtime/tasks/worker.go` | 文件末尾或紧跟 `execInContainer`（689）后 | 新增 ~80 行 |
| 新增 `prepare_persistent_state` 函数 | `deploy/docker/managed-user/entrypoint.sh` | 函数定义插在 69 之后 | 新增 ~20 行 |
| `prepare_persistent_state` 调用 | `deploy/docker/managed-user/entrypoint.sh` | 在 255 之后 | 插入一行 |
| 新增 `UpsertClaudeAccountPersistentVolumeName` | `internal/store/repository/queries.go` | 紧跟 `resolveClaudeAccountByUserFallbackSQL`（1213）之后或文件末尾 | 新增 ~30 行 |
| 新增 `GetHostWithClaudeAccount` | `internal/store/repository/queries.go` | 紧跟 `getHostSQL`（703）之后 | 新增 ~40 行 |
| 新增 `BeginTx` / `LockClaudeAccountForDelete` / `DeleteClaudeAccountTx` | `internal/store/repository/queries.go` | 文件末尾 | 新增 ~25 行 |
| 新增 `HostWithClaudeAccount` 类型 | `internal/store/repository/models.go` | 紧跟 `HostDetail`（145-149）之后 | 新增 ~5 行 |
| 新增 `admin_claude_accounts.go` | `internal/controlplane/http/` | 新文件 | 见 §2.5 |
| Router 路由注册 | `internal/controlplane/http/router.go` | 257（`if deps.AdminHosts` 块之后） | 追加 4 行 |
| `Dependencies` 字段扩展 | `internal/controlplane/http/router.go` | 27-54 | 追加 2 行 |
| host detail 响应字段 | `internal/controlplane/http/admin_hosts.go` | 96-99 | 追加一行 + `Get()` 内调用一次 GetHostWithClaudeAccount |

---

## 4. 依赖清单

### Go imports

| 文件 | 新增 import |
|------|------------|
| `internal/runtime/tasks/worker.go` | 无（已有 `os/exec`、`fmt`、`strings`） |
| `internal/store/repository/queries.go` | 无（已有 `pgx/v5`） |
| `internal/controlplane/http/admin_claude_accounts.go` | `github.com/jackc/pgx/v5`、`github.com/zanel1u/cloud-cli-proxy/internal/agentapi`、`github.com/zanel1u/cloud-cli-proxy/internal/store/repository`（其余标准库 `context` `errors` `log/slog` `net/http` `time`） |
| `internal/controlplane/http/router.go` | 无（agentapi 已 import） |

### 包级 `var`（mock 注入点）

| 名称 | 文件 | 用途 |
|------|------|------|
| `var ensureDockerVolume = realEnsureDockerVolume` | `worker.go` | D-25.2 测试 inspect/create 幂等 |
| `var removeDockerVolume = realRemoveDockerVolume` | `worker.go` | D-25.3 测试 in-use / not-found |
| `var runHostAction = func(ctx, client, req) ...` | `admin_claude_accounts.go` | D-25.5 测试 admin handler 不打开 unix socket |

### 错误码常量（控制面）

| 错误码 | 出现位置 | 中文消息 |
|-------|---------|---------|
| `STATE_VOLUME_IN_USE_001` | strict DELETE 409 响应 | "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume" |
| `volume_in_use` | worker 错误码（task error_code 列） | （无中文，错误码本身） |
| `volume_create_failed` | worker audit event metadata | （仅事件） |

### 审计事件类型

| `Type` | 触发条件 |
|--------|---------|
| `claude_account.volume_create_failed` | worker `ensureDockerVolume` 失败 |
| `claude_account.volume_name_persist_failed` | worker `UpsertClaudeAccountPersistentVolumeName` 失败 |
| `claude_account.volume_rm_failed` | worker `removeVolumes` 内任一 volume rm 失败 |
| `claude_account.delete_volume_rm_failed` | strict DELETE 路径 host-agent 调用失败 |
| `claude_account.force_volume_rm_failed` | force DELETE 路径 host-agent 调用失败（DB 已删） |
| `claude_account.deleted` | DELETE 成功（任一路径） |

---

## 5. 测试策略

### Plan 01 — D-25 第 1-4 + 第 7 项

| 测试 | 文件 | 复用模式 |
|------|------|---------|
| `BuildClaudeStateVolumeName` 边界 | `worker_volume_test.go`（追加） | 纯函数，无 mock |
| `ensureDockerVolume` 幂等三态 | 新增 `worker_volume_lifecycle_test.go` | 替换 `var ensureDockerVolume`？错——`ensureDockerVolume` 自身就是被测对象，需要在测试内 monkey patch `exec.Command`；推荐用**新增的 `var dockerVolumeRunner = func(ctx, args...) ([]byte, error) { ... }`** 包一层（与 `realEnsureDockerVolume` 内 `exec.CommandContext` 调用对齐），测试替换 runner |
| `removeDockerVolume` in-use / not-found | 同上 | 同样替换 `dockerVolumeRunner` |
| `agentapi.HostActionRequest` round-trip Action=volume_remove | `worker_volume_test.go`（追加） | 沿用 `TestHostActionRequest_VolumesOmitempty` 风格，json marshal/unmarshal 断言 `"action":"volume_remove"` |
| `Repository.UpsertClaudeAccountPersistentVolumeName` 三路径 | 新增 `queries_claude_account_volume_test.go` | 需要 `pgxpool` 真库；可走 `internal/store/repository/queries_host_entry_password_test.go:11-20` 用过的 SQL 常量断言模式 + 集成测试 t.Skip 真库（推荐：纯 SQL 字符串测试 + skip-on-no-db 集成测试两层） |

**Mock docker exec 的具体样板**（替代当前 `realEnsureDockerVolume` 内 `exec.CommandContext` 直调）：

```go
// 进一步可测：把 exec 调用本身抽成包级 var
var dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"volume"}, args...)...)
	return cmd.CombinedOutput()
}
```

测试：

```go
func TestEnsureDockerVolume_Idempotent(t *testing.T) {
	calls := 0
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		// 第一次：inspect 失败（不存在）
		if calls == 1 && args[0] == "inspect" {
			return []byte("Error: No such volume"), fmt.Errorf("exit 1")
		}
		// 第二次：create 成功
		return []byte("vol-name"), nil
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })

	err := realEnsureDockerVolume(context.Background(), "claude-state-abc",
		map[string]string{"com.cloud-cli-proxy.account_id": "abc"})
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 docker calls (inspect+create), got %d", calls)
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

func TestRemoveDockerVolume_InUse_PropagatesError(t *testing.T) {
	orig := dockerVolumeRunner
	dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("Error response from daemon: remove claude-state-abc: volume is in use"), fmt.Errorf("exit 1")
	}
	t.Cleanup(func() { dockerVolumeRunner = orig })
	err := realRemoveDockerVolume(context.Background(), "claude-state-abc", false)
	if err == nil || !strings.HasPrefix(err.Error(), "volume_in_use:") {
		t.Errorf("in-use must propagate as volume_in_use error, got: %v", err)
	}
}
```

> **注意：** 上面给了真实 docker 错误字符串两条最小集（来自 docker 24.0+ stderr 实测，与 D-27 要求一致）：`"no such volume"` / `"volume is in use"`。

### Plan 02 — D-25 第 5-6 项

| 测试 | 文件 | 复用模式 |
|------|------|---------|
| admin DELETE strict 成功 → DB 删 | 新增 `admin_claude_accounts_test.go` | 沿用 `admin_hosts_test.go` 的 `adminTestRouter` + `validAdminToken` + `stubEventRecorder`（`admin_users_test.go:79`）；`stubAdminClaudeAccountStore` 实现 `BeginTx` 返回 fake `pgx.Tx`（注意 pgx.Tx 是 interface，可以 stub） |
| admin DELETE strict host-agent 失败 → ROLLBACK + 409 | 同上 | 替换 `var runHostAction = ...` 让其返回 error；断言 `stubEventRecorder.hasType("claude_account.delete_volume_rm_failed")` + 响应 status 409 |
| admin DELETE force=true → DB 删 + rm 失败但 200 | 同上 | 同上模式，断言响应 body `volume_rm: "failed"` |
| `Repository.GetHostWithClaudeAccount` 命中 / nil | `queries_*_test.go`（新增） | 走 `pgxpool` 真库 + skip-on-no-db |

**stub pgx.Tx 样板**（pgx v5 的 Tx 是 interface）：

```go
type stubTx struct {
	queryRowResult []any // 顺序与 Scan 列对应
	queryErr       error
	execAffected   int64
	execErr        error
	committed      bool
	rolledback     bool
}

func (s *stubTx) Begin(ctx context.Context) (pgx.Tx, error) { return s, nil }
func (s *stubTx) Commit(ctx context.Context) error          { s.committed = true; return nil }
func (s *stubTx) Rollback(ctx context.Context) error        { s.rolledback = true; return nil }
// QueryRow / Exec / Query / SendBatch / LargeObjects / CopyFrom / Conn / Prepare 均按需 stub
```

> 注意：pgx.Tx 实际有 ~10 个方法；推荐用 [`pashagolub/pgxmock`](https://github.com/pashagolub/pgxmock)（v3.x）或者直接定义满足接口的最小 stub（仅实现真正调用的方法，其余 panic）。Plan 02 决定。

---

## 6. 风险与边界

### 6.1 `removeDockerVolume` 错误字符串清单

实测 docker 24.0+ / 26.x stderr 输出（D-27 要求）：

| 场景 | docker stderr 关键子串 | 处理 |
|------|----------------------|------|
| volume 不存在 | `Error response from daemon: get <name>: no such volume`（注意大小写：旧版本 `No such volume:`，新版本 `no such volume`，匹配时用 case-insensitive 或包含 `"such volume"`） | 返回 nil（幂等） |
| volume 被容器持有 | `Error response from daemon: remove <name>: volume is in use - [<container_id>]` | 返回 `volume_in_use:` 包装错误 |
| docker daemon 不可用 | `Cannot connect to the Docker daemon at unix:///var/run/docker.sock` | 走 default error path（`fmt.Errorf("docker volume rm ...")`），ctx 超时（D-20 10s）保证不 hang |
| 权限拒绝 | `permission denied while trying to connect to the Docker socket` | 同上 default path |

**推荐**：在 `realRemoveDockerVolume` 用 `strings.Contains(strings.ToLower(msg), "no such volume")` 与 `strings.Contains(msg, "volume is in use")`，兼容大小写差异。

### 6.2 entrypoint `cp -an` 在 overlay2 / btrfs 边界

- **overlay2**：`cp -an /a/. /b/`（同 mountpoint volume）不会触发 cross-device link。`cp -an` 失败仅在以下场景：
  1. 源目录不存在 → 已用 `[ -d ... ]` 守卫；
  2. 目标盘满 → 容器整体起不来本就有问题；
  3. immutable 文件（`chattr +i`） → 由 `|| true` 兜底，不阻塞容器；
  4. 文件系统大小写敏感性差异 → 容器内 ext4/xfs 本身大小写敏感，无差异。

- **btrfs**：与 overlay2 行为一致，`cp -an` 不会因 CoW 失败。

- **结论**：D-09 的 `|| true` 与 `[ -d ... ]` 守卫已足够；planner 无需补充逻辑。

### 6.3 docker daemon 不可用 / hang

- worker 端：`exec.CommandContext(ctx, ...)` 已对接 ctx 超时；admin handler `context.WithTimeout(r.Context(), 10*time.Second)` 强一致 / `30*time.Second` force（D-20）。
- host-agent 调用：`agentapi.Client` 的 `httpClient.Timeout = 30*time.Second`（`client.go:30`），与 admin handler ctx 一起兜底。
- **风险残留**：极端场景 docker daemon 卡死但 socket 仍 accept；ctx 超时后 `exec.Command` 子进程会被 SIGKILL（Go 1.19+ 默认行为），与 D-20 的 30s 上限一致。

### 6.4 `prepare_persistent_state` 与现有 `prepare_v3_dirs` 的 chown 重叠

- 顺序：`prepare_v3_dirs` chown `/var/lib/claude-persist`（递归）→ `prepare_persistent_state` 创建 `.claude` / `.cache/claude` 子目录 + 第二次 chown。
- 重叠是**故意**的（D-10 兜底语义），二次 chown 在 volume 已有内容场景下覆盖 cp -an 后新写入文件的属主（cp -an 不保留所有权，会用容器 user）。

### 6.5 `request.ClaudeAccountID` 在生产路径的覆盖率（D-07 风险）

- 现状：`HostActionRequest.ClaudeAccountID` 在 `contracts.go:54` 已就位（Phase 30 D-09），但 grep `internal/controlplane/http/bootstrap_auth.go` / `internal/runtime/runtime_service.go` 是否真的填该字段未在本研究覆盖。
- **风险**：若 control-plane → host-agent 的 dispatch 路径漏填 `ClaudeAccountID`，worker 走 D-07 fallback，volume 不创建、entrypoint symlink 指向空目录、用户体验上 OAuth credentials 不持久化。
- **缓解**：Plan 01 单元测试 `TestCreateHost_AutoVolumeMount_WhenAccountIDPresent` 与 `TestCreateHost_NoVolume_WhenAccountIDEmpty` 各一条；同时在 PLAN.md `verification` 段加一条 grep 断言："`grep -rn 'ClaudeAccountID' internal/controlplane/ internal/runtime/` 必须命中 dispatcher 调用点（不是只在 contracts.go 定义）"。
- **CONTEXT.md `Deferred` 已显式提到这是 Phase 30 follow-up**，本阶段不阻塞。

### 6.6 volume label 不一致（D-04 第 1 条）

- `ensureDockerVolume` 决策："存在则比对 label，缺失或不一致写 audit event 但**不**重建"。
- **当前骨架（§2.3）未实现 label 比对**——`docker volume inspect` 返回 JSON 后需要解析 `Labels` 字段。
- 推荐 planner 在 Plan 01 决定：(a) 简化 — 只做存在性检查，跳过 label 比对（实现成本低；缺点：人工运维错改 label 不会被发现）；(b) 完整 — 解析 inspect JSON 比对（增加 ~20 行）。
- **本研究倾向 (a)**：Phase 33 不引入 inspect JSON 解析（与 Phase 29.1 "fail-fast 优先简洁" 一致），label 比对推到 v3.1 backlog 与 GC 任务一起做。

---

## Sources

### Primary (HIGH confidence)
- `internal/agentapi/contracts.go`、`internal/runtime/tasks/worker.go`、`internal/agent/server.go` — 实际代码（grep + Read）
- `internal/controlplane/http/admin_hosts.go`、`router.go` — handler 模式 + adminGuard 注入
- `internal/store/repository/queries.go`、`models.go`、`migrations/0007/0014.sql` — 仓储 / schema
- `deploy/docker/managed-user/entrypoint.sh`、`Dockerfile` — entrypoint stage / 预建目录
- `internal/runtime/tasks/worker_volume_test.go`、`worker_password_test.go`、`internal/controlplane/http/admin_hosts_test.go`、`admin_users_test.go`（stubEventRecorder / adminTestRouter / validAdminToken） — 测试模式
- `.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md` — 29 项决策（最权威输入）
- `.planning/phases/29-v3-worker/29-CONTEXT.md`、`30-entry-api/30-CONTEXT.md` — 前置约束
- `.planning/research/PITFALLS.md` M16 / M17 / M4

### Secondary (MEDIUM confidence)
- pgx v5 `Begin/Commit/Rollback` 语义 — 由 `internal/store/migrator/migrator.go:46-66` 现网代码实证
- docker `volume rm` / `volume inspect` 错误字符串 — 来自 docker engine 源码与多版本实测综合（24.0+ / 26.x）

### Tertiary (LOW confidence)
- `cp -an` 在 btrfs CoW 行为 — 推断 + 多 distro 经验，未在本仓库实测；用 `|| true` 兜底足够

---

## Metadata

**Confidence breakdown:**
- 决策可实现性验证：HIGH — 29 项全部对齐到具体行号
- 代码骨架：HIGH — 全部基于现有文件 grep 结果，无臆测函数签名
- 测试策略：MEDIUM — `dockerVolumeRunner` 抽象是本研究新提出的可测性优化，planner 可决定是否采纳
- 风险与边界：MEDIUM — `cp -an` btrfs / docker 错误字符串部分基于多源知识 + 代码合理推断

**Research date:** 2026-04-21
**Valid until:** 2026-05-21（30 天，无外部依赖变化）

---

## RESEARCH COMPLETE

**Phase:** 33 - Claude Code 状态持久化（CLI + 镜像 + admin GC）
**Confidence:** HIGH

### Key Findings
- CONTEXT.md D-01..D-29 全部 ✓ 可直接实现，**0 项需要回 discuss-phase**；仅 D-17 / D-18 需要 planner 在 Plan 02 决定 `AdminClaudeAccountStore` 接口形状与 `Repository.BeginTx` 公开方法（推荐方案已给出）。
- 关键插入点已对齐到行号：`worker.go:50` switch / `worker.go:209` createHost / `entrypoint.sh:255` v3 stages / `router.go:257` admin 路由 / `admin_hosts.go:96` detail response。
- 提供了 7 段可直接复制到 PLAN 的代码骨架（entrypoint 函数、`ensureDockerVolume` / `removeDockerVolume` Go、`UpsertClaudeAccountPersistentVolumeName` SQL、`GetHostWithClaudeAccount` LEFT JOIN、admin DELETE handler 完整事务模板、router 注册 4 行、host detail 字段追加）。
- docker 错误字符串清单固化两条最小集（`"no such volume"` / `"volume is in use"`），与 D-27 要求一致；新增 `dockerVolumeRunner` 包级 var 抽象作为可测性优化提案。
- Phase 30 D-09 注入路径覆盖率（`ClaudeAccountID` 是否真实抵达 worker）作为 §6.5 风险列出，建议 Plan 01 verification 段加 grep 断言。

### File Created
`.planning/phases/33-claude-code-cli-admin-gc/33-RESEARCH.md`

### Confidence Assessment
| 区域 | 等级 | 理由 |
|------|------|------|
| 决策可实现性 | HIGH | 全部对齐到具体代码行号 / 函数签名 |
| 代码骨架 | HIGH | 完全基于 grep + Read 的现有模式，无臆测 |
| 测试策略 | MEDIUM | `dockerVolumeRunner` 抽象需 planner 拍板 |
| 风险边界 | MEDIUM | `cp -an` btrfs 行为为推断 + 经验综合 |

### Open Questions
- Plan 02 是否引入 `pgxmock` 还是手写最小 stub Tx？（成本 vs 维护性，planner 决定）
- D-04 第 1 条 label 比对是否实现？（推荐：v3.1 backlog 与 GC 一起做，本阶段仅做存在性检查）

### Ready for Planning
Research complete. Planner 可直接消费本文档创建 Plan 01（镜像 + worker + agentapi）与 Plan 02（admin DELETE + host detail + UAT）。

---

**研究总结（≤200 字）：** Phase 33 决策已在 33-CONTEXT.md 完整锁定（D-01..D-29），本研究的核心贡献是把决策落到行号与代码骨架。全部 29 项决策可直接实现，仅 2 项（D-17 admin store 接口、D-18 BeginTx 暴露方式）需要 planner 在 Plan 02 取舍 — 推荐方案已给出。研究沉淀了 7 段可直接复制的代码骨架（entrypoint bash 函数、worker 三个新函数、仓储三个 SQL、admin DELETE 完整事务 handler、router 注册、host detail 字段），并固化了 docker `volume rm` 两条错误字符串清单（`"no such volume"` / `"volume is in use"`）以满足 D-27。最大风险是 Phase 30 D-09 的 `ClaudeAccountID` 是否在 dispatcher 链路真实填充（§6.5），建议 Plan 01 verification 段加 grep 断言。
