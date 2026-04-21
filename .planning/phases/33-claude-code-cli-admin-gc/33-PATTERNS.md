# Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）- Pattern Map

**Mapped:** 2026-04-21
**Files analyzed:** 12（Plan 01 = 7、Plan 02 = 5；含 3 个新建测试文件）
**Analogs found:** 12 / 12（全部能在仓库内找到 1:1 形似文件）
**Sources:** `33-CONTEXT.md` D-01..D-29 + `33-RESEARCH.md` §2 / §3 / §4

---

## File Classification

| 目标文件 | 角色 | 数据流 | 最近 analog | 匹配度 |
|---------|------|--------|------------|-------|
| `deploy/docker/managed-user/entrypoint.sh`（modify） | image entrypoint | bash 串行 stage | 同文件 `prepare_v3_dirs` / `prepare_mutagen_agent`（行 62-100） | exact |
| `internal/agentapi/contracts.go`（modify） | 协议常量 | 字面量追加 | 同文件 `ActionPrepareHost`（行 5-11） | exact |
| `internal/runtime/tasks/worker.go`（modify） | worker 任务编排 | docker CLI 包装 + switch 分派 | 同文件 `execInContainer` / `runDocker` / `containerExists`（行 689-830）+ Execute switch（行 48-63）+ createHost（行 194-249） | exact |
| `internal/store/repository/queries.go`（modify - Upsert + Lock + Delete + GetHostWith…） | 仓储 SQL | `UPDATE … WHERE … IS NULL` 一次往返 + LEFT JOIN | 同文件 `UpdateHostEntryPassword`（行 1260-1271）+ `resolveClaudeAccountByUserFallbackSQL`（行 1213-1219）+ migrator `db.Begin`（`migrator/migrator.go:46`） | exact |
| `internal/store/repository/models.go`（modify - HostWithClaudeAccount 类型） | 仓储数据模型 | 嵌入 + 新字段 | `HostWithUsername`（行 157-163）embed `Host` + 追加列 | exact |
| `internal/controlplane/http/admin_claude_accounts.go`（new） | admin handler | request-response + 事务编排 + agentapi 调用 | `admin_hosts.go` 整体形态 + `ResyncPasswords`（行 421-510）+ `RotateSSHPassword`（行 350-419）+ 包级 `var syncContainerPassword`（行 1014） | exact |
| `internal/controlplane/http/admin_hosts.go`（modify - detail 字段追加） | admin handler | 响应 JSON 字段追加 | 同文件 `adminHostDetailResponse` / `Get()`（行 96-148） | exact |
| `internal/controlplane/http/router.go`（modify） | 路由注册 + Dependencies | adminGuard 注册 | 同文件 `if deps.AdminHosts != nil { ... }`（行 232-256）+ `Dependencies` struct（行 27-54） | exact |
| `internal/runtime/tasks/worker_volume_test.go`（modify - 追加 Action round-trip + Build…name） | 单测 | json marshal/unmarshal 断言 | 同文件 `TestHostActionRequest_ClaudeAccountID_RoundTrip`（行 109-131） | exact |
| `internal/runtime/tasks/worker_volume_lifecycle_test.go`（new） | 单测 | 包级 var monkey-patch + exec 模拟 | `worker_password_test.go`（已有，沿用 `var execInContainer = ...` 替换模式）+ §RESEARCH 5.1 `dockerVolumeRunner` 抽象 | role-match（同测试风格、新增一层 `dockerVolumeRunner` 抽象） |
| `internal/store/repository/queries_claude_account_volume_test.go`（new） | 仓储单测 | SQL 字符串断言（推荐）+ skip-on-no-db 集成（可选） | `queries.go:1180+` 全部 `const xxxSQL` 提升模式（Phase 29.1 强制约定） | role-match（仓库当前 unit test 文件偏少；以"测试 SQL 常量字符串包含关键 token"为最小集） |
| `internal/controlplane/http/admin_claude_accounts_test.go`（new） | handler 单测 | `adminTestRouter` + `validAdminToken` + `stubEventRecorder` + 自建 `stubAdminClaudeAccountStore` + 替换 `var runHostAction = ...` | `admin_users_test.go`（行 79-117 stubEventRecorder + validAdminToken）+ `admin_hosts_test.go`（行 21-66 stubHostStore 模板） | exact |

---

## Pattern Assignments

> 每条 pattern 给出"形似 analog 路径 + 行号"和"verbatim 复制起点"。planner 在 PLAN 的 actions 段直接引用本节即可。

---

### `deploy/docker/managed-user/entrypoint.sh`（image entrypoint / bash 串行 stage）

**Analog:** 同文件 `prepare_v3_dirs` 函数（`entrypoint.sh:62-69`）+ v3 stage 调用序列（`entrypoint.sh:254-258`）。

**风格代码片段（必须沿用）：**

```62:69:deploy/docker/managed-user/entrypoint.sh
prepare_v3_dirs() {
  echo "[entrypoint] v3: chown /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist"
  chown -R 1000:1000 \
    /home/claude \
    /workspace-hot \
    /workspace-cold \
    /var/lib/claude-persist 2>/dev/null || true
}
```

```254:258:deploy/docker/managed-user/entrypoint.sh
# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version
```

**verbatim 复制起点（来自 RESEARCH §2.1，已含全部 D-09 决策）：**

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

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 函数名 `prepare_persistent_state` | **verbatim**（CONTEXT D-09 字面，测试脚本依赖此名） |
| `root=/var/lib/claude-persist` | **verbatim**（CONTEXT D-08，与 Dockerfile + worker mount target 三处必须一致） |
| `cp -an` / `ln -sfn` / `chown -h` 标志组合 | **verbatim**（D-09 不变量 1+2+3） |
| `[ -d ... ] && [ -z "$(ls -A ...)" ]` 守卫 | **verbatim**（D-09 幂等条件，省略将导致重启时覆盖用户数据） |
| `|| true` 兜底 | **verbatim**（D-09 不阻塞条款） |
| `echo "[entrypoint] ..."` 日志格式 | 可微调措辞，前缀 `[entrypoint] v3:` 必须保留（与 `prepare_v3_dirs` 风格一致，便于 grep） |
| 调用插入位置 | **verbatim**：必须在 `prepare_v3_dirs` 之后、`prepare_mutagen_agent` 之前（D-09 + RESEARCH §2.1） |

---

### `internal/agentapi/contracts.go`（协议常量 / 字面量追加）

**Analog:** 同文件常量块（`contracts.go:5-11`）。

```5:11:internal/agentapi/contracts.go
const (
	ActionCreateHost  HostAction = "create_host"
	ActionStartHost   HostAction = "start_host"
	ActionStopHost    HostAction = "stop_host"
	ActionRebuildHost HostAction = "rebuild_host"
	ActionPrepareHost HostAction = "prepare_host"
)
```

**verbatim 复制：** 在 `ActionPrepareHost` 之后追加一行：

```go
ActionVolumeRemove HostAction = "volume_remove" // Phase 33 D-13
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 常量名 `ActionVolumeRemove` | **verbatim**（D-13 字面） |
| 字符串值 `"volume_remove"` | **verbatim**（D-13 + JSON 协议契约，host-agent 端 switch 字符串比较） |
| 行内注释 `// Phase 33 D-13` | 可微调，但**必须保留 Phase 编号 + 决策号**（仓库其他常量同样标注，便于回溯） |
| `HostActionRequest` 结构 | **不动**（D-13 显式复用 `Volumes`/`Labels` 字段，禁止扩字段） |

---

### `internal/runtime/tasks/worker.go`（worker 任务编排 / docker CLI 包装 + switch 分派）

**Analog 1（包级 var 注入 + exec 包装）:** 同文件 `execInContainer`（`worker.go:687-695`）+ `runDocker`（`worker.go:832-840`）。

```687:695:internal/runtime/tasks/worker.go
// execInContainer 在目标容器中以 `docker exec -i <container> bash -c <script>` 执行，
// 支持可选 stdin。暴露为 package-level 变量以便单元测试注入 fake。
var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}
```

```832:840:internal/runtime/tasks/worker.go
func (w *Worker) runDocker(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
```

**Analog 2（Execute switch 扩展）:** 同文件 `Execute`（`worker.go:48-63`）。

```48:63:internal/runtime/tasks/worker.go
func (w *Worker) Execute(ctx context.Context, request agentapi.HostActionRequest) agentapi.TaskStatusUpdate {
	var err error
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
	default:
		err = fmt.Errorf("unsupported host action: %s", request.Action)
	}
```

**Analog 3（createHost 内部调用顺序）:** 同文件 `createHost`（`worker.go:194-249`）—— 关键插入点：行 209 `runDocker rm -f` 之后、行 211 `hostname :=` 之前。

```194:223:internal/runtime/tasks/worker.go
func (w *Worker) createHost(ctx context.Context, request agentapi.HostActionRequest) error {
	homeDir := firstNonEmpty(request.HomeDir, hostHomeDir(request.HostID))
	containerName := firstNonEmpty(request.ContainerName, containerNameForHost(request.HostID))
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return fmt.Errorf("prepare host home dir %s: %w", homeDir, err)
	}

	w.pullImage(ctx, request.ImageName)

	if exists, err := w.containerExists(ctx, containerName); err != nil {
		return err
	} else if exists {
		if err := w.runDocker(ctx, "rm", "-f", containerName); err != nil {
			return err
		}
	}

	hostname := request.Hostname
```

**Analog 4（label 拼接 / `--label k=v` 多次 flag）:** 同文件 `buildCreateArgs`（`worker.go:146-147` + `185-188`）—— 同样的 `for k, v := range labels { args = append(args, "--label", fmt.Sprintf("%s=%s", k, v)) }` 模板。

**Analog 5（错误码映射）:** 同文件 `Execute` 错误码分支（`worker.go:65-88`）—— `errors.As` + 字符串前缀匹配模式。

```go
errorCode := "host_action_failed"
var sshErr *SSHNotReadyError
if errors.As(err, &sshErr) {
    errorCode = "ssh_not_ready"
}
```

**Analog 6（RecordEvent 调用 / metadata 风格）:** 同文件 `recordNetworkError`（`worker.go:657-668`）+ `startHost.RecordEvent`（行 285-291）。

```657:668:internal/runtime/tasks/worker.go
func (w *Worker) recordNetworkError(ctx context.Context, hostID string, err error) {
	var netErr *network.NetworkError
	if errors.As(err, &netErr) {
		_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
			HostID:   &hostID,
			Level:    "error",
			Type:     netErr.EventType(),
			Message:  netErr.Error(),
			Metadata: netErr.EventMetadata(),
		})
	}
}
```

**verbatim 复制起点：** 见 RESEARCH §2.3 (a)/(b)/(c)/(d) 完整代码（包级常量 + `BuildClaudeStateVolumeName` + `var ensureDockerVolume = realEnsureDockerVolume` + `var removeDockerVolume = realRemoveDockerVolume` + `removeVolumes` + `createHost` 内 ~40 行 block + `WorkerRepo` 接口扩展 1 行）。

**planner 提示：**

| 字段 | 处理 |
|------|------|
| `claudeStateVolumePrefix = "claude-state-"` | **verbatim**（D-01） |
| `claudeStateMountTarget = "/var/lib/claude-persist"` | **verbatim**（D-08，与 entrypoint.sh / Dockerfile 三处一致） |
| `claudeAccountLabelKey = "com.cloud-cli-proxy.account_id"` | **verbatim**（D-02 / Phase 30 D-01 锁定字符串） |
| `claudeManagedLabelKey = "com.cloud-cli-proxy.managed"` / `claudeManagedLabelVal = "true"` | **verbatim**（D-02） |
| `BuildClaudeStateVolumeName(id)` 函数名/签名 | **verbatim**（D-25.1 测试断言此名） |
| `var ensureDockerVolume = realEnsureDockerVolume` 模式 | **verbatim**（与 `var execInContainer = ...` 风格强一致；测试依赖此包级 var 注入） |
| `var removeDockerVolume = realRemoveDockerVolume` | 同上 |
| docker 错误字符串 `"No such volume"` / `"volume is in use"` | **verbatim**（D-15 + D-27 + RESEARCH §6.1，跨版本兼容用 `strings.Contains(strings.ToLower(msg), "no such volume")`） |
| 错误码 `"volume_in_use"` / `"volume_create_failed"` | **verbatim**（D-15 + RESEARCH §4 错误码表） |
| audit event Type `claude_account.volume_create_failed` / `volume_name_persist_failed` / `volume_rm_failed` | **verbatim**（D-21 命名规范 + RESEARCH §4） |
| metadata key `account_id` / `volume_name` / `error_code` / `error_message` | **verbatim**（D-21 + Discretion 第 3 条字段顺序） |
| `metadata` **禁止** 含 OAuth token / `EntryPassword` 等任何凭据 | **守恒**（CONTEXT D-21 + Phase 29.1 Plan 02 模式） |
| `request.ClaudeAccountID == ""` 跳过分支 | **verbatim**（D-07 fallback，禁报错） |
| createHost 插入位置（runDocker rm -f 之后、hostname := 之前） | **verbatim**（RESEARCH §3 插入点表 + §2.3 (c) 注释） |
| 是否实现 `inspect` JSON label 比对 | 可微调（RESEARCH §6.6 推荐方案 a：仅做存在性检查；标 v3.1 backlog） |

---

### `internal/store/repository/queries.go`（仓储 SQL / `UPDATE … WHERE … IS NULL` 一次往返 + LEFT JOIN + 事务工具）

**Analog 1（条件 UPDATE + RowsAffected）:** 同文件 `UpdateHostEntryPassword`（`queries.go:1260-1271`）。

```1260:1271:internal/store/repository/queries.go
func (r *Repository) UpdateHostEntryPassword(ctx context.Context, hostID, password string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE hosts SET entry_password = $2, updated_at = NOW() WHERE id = $1
	`, hostID, password)
	if err != nil {
		return fmt.Errorf("update host entry password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
```

**Analog 2（包级 SQL const 提升）:** 同文件 `resolveClaudeAccountByHostSQL` / `resolveClaudeAccountByUserFallbackSQL`（`queries.go:1203-1219`）。

```1203:1219:internal/store/repository/queries.go
// resolveClaudeAccountByHostSQL / resolveClaudeAccountByUserFallbackSQL 实现 D-05 的确定性解析。
// 两条语句都全部使用参数化查询（T-30-01 缓解），避免 SQL 注入。
const resolveClaudeAccountByHostSQL = `
	SELECT id::text
	FROM claude_accounts
	WHERE host_id = $1
	ORDER BY created_at ASC
	LIMIT 1
`

const resolveClaudeAccountByUserFallbackSQL = `
	SELECT id::text
	FROM claude_accounts
	WHERE user_id = $1 AND host_id IS NULL
	ORDER BY created_at ASC
	LIMIT 1
`
```

**Analog 3（LEFT JOIN + Scan 多列）:** 同文件 `getHostByShortIDSQL`（`queries.go:1180-1201`）—— `JOIN users u ON ...` + `SELECT h.col, u.col` + `Scan(&item.X, &item.Y, ...)`.

**Analog 4（pgx.Tx 事务包装 — 唯一既有调用点）:** `internal/store/migrator/migrator.go:46-66`（仓库唯一对 `db.Begin(ctx)` 的现存使用）。grep 验证：仓库内 `r.db.Begin` 调用为 0，需要新建 `BeginTx` 公开方法。

**verbatim 复制起点：** 见 RESEARCH §2.4 (a) `UpsertClaudeAccountPersistentVolumeName` + (b) `GetHostWithClaudeAccount` + (c) `BeginTx` / `LockClaudeAccountForDelete` / `DeleteClaudeAccountTx`。

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 所有 SQL 必须提升为包级 `const xxxSQL` | **verbatim**（Phase 29.1 Plan 01 强制约定；用于回归测试断言） |
| SQL 字符串内列引用 `id::text` / `COALESCE(persistent_volume_name, '')` | **verbatim**（与 `resolveClaudeAccountByHostSQL` 行 1206 一致；nil string 三态语义由 `COALESCE` 转空串处理） |
| `WHERE id = $1 AND persistent_volume_name IS NULL` 条件 | **verbatim**（D-06 三态语义；省略 `IS NULL` 将破坏"非空冲突写 audit"语义） |
| `r.db.Exec` / `r.db.QueryRow` API | **verbatim**（pgx v5 的 pgxpool 接口） |
| `tag.RowsAffected() == 1`（写入成功）/ `== 0`（NULL 校验） | **verbatim**（D-06 双返回路径） |
| `BeginTx` 函数签名 `func (r *Repository) BeginTx(ctx) (pgx.Tx, error)` | **verbatim**（RESEARCH §1 D-18 ⚠ + §2.4 (c) 推荐方案；禁止把 `*pgxpool.Pool` 泄漏到 handler） |
| `LockClaudeAccountForDelete` / `DeleteClaudeAccountTx` 是包级函数（非 method） | 可微调（RESEARCH §2.4 (c) 写成包级 func 接收 tx；planner 可改为 method on `Repository` 但需要持有 tx ref） |
| `FOR UPDATE` 行锁 | **verbatim**（D-18 强一致路径必需） |
| import `github.com/jackc/pgx/v5` | **verbatim**（已在 queries.go:10 import） |
| `LIMIT 1` + `ORDER BY ca.created_at ASC` 在 LEFT JOIN | **verbatim**（D-23 防御性，避免 1:N JOIN 返回多行；与 `resolveClaudeAccountByHostSQL` 风格一致） |

---

### `internal/store/repository/models.go`（仓储数据模型 / 嵌入 + 新字段）

**Analog:** 同文件 `HostWithUsername`（`models.go:157-163`）—— embed `Host` + 追加列模式。

```157:163:internal/store/repository/models.go
type HostWithUsername struct {
	Host
	Username       string  `json:"username"`
	EgressIPLabel  *string `json:"egress_ip_label,omitempty"`
	EgressIPAddr   *string `json:"egress_ip_address,omitempty"`
	DockerStatus   string  `json:"docker_status,omitempty"`
}
```

**verbatim 复制起点（来自 RESEARCH §2.4 (b)）：**

```go
// HostWithClaudeAccount D-23：纯 DB JOIN，避免在 detail handler 引入 docker exec。
type HostWithClaudeAccount struct {
	Host
	PersistentVolumeName string // 空串 = 未分配
}
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 类型名 `HostWithClaudeAccount` | **verbatim**（D-23 + RESEARCH §2.4 (b)） |
| embed `Host`（不重复字段） | **verbatim**（与 `HostWithUsername` 风格一致） |
| `PersistentVolumeName string`（**非指针**）+ 注释"空串 = 未分配" | 可微调：planner 也可改用 `*string` 与 `ClaudeAccount.PersistentVolumeName` 类型对齐；但当前推荐 `string` 以简化 Scan + handler 序列化（`omitempty` 仍可用） |
| 插入位置 | 紧跟 `HostDetail`（行 145-149）之后或 `HostWithUsername` 之后 |

---

### `internal/controlplane/http/admin_claude_accounts.go`（new — admin handler / request-response + 事务编排 + agentapi 调用）

**Analog 1（handler 类型 + Constructor + Method 风格）:** `admin_hosts.go:37-69`（`AdminHostsHandler` struct + `NewAdminHostsHandler` + `List() nethttp.Handler`）。

```37:46:internal/controlplane/http/admin_hosts.go
type AdminHostsHandler struct {
	logger *slog.Logger
	store  AdminHostStore
	queue  HostActionQueuer
	events EventRecorder
}

func NewAdminHostsHandler(logger *slog.Logger, store AdminHostStore, queue HostActionQueuer, events EventRecorder) *AdminHostsHandler {
	return &AdminHostsHandler{logger: logger, store: store, queue: queue, events: events}
}
```

**Analog 2（Store 接口最小集声明）:** `admin_hosts.go:25-35`（`AdminHostStore`）。

**Analog 3（包级 var 注入 mock）:** `admin_hosts.go:1012-1023`（`var syncContainerPassword = ...`）。

```1012:1023:internal/controlplane/http/admin_hosts.go
// syncContainerPassword updates the Linux user password inside a running container via docker exec.
// Exposed as a package-level var so unit tests can inject a fake implementation (Phase 29.1).
var syncContainerPassword = func(containerName, user, password string) error {
	cmd := exec.CommandContext(context.Background(), "docker", "exec", "-i", containerName,
		"chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", user, password))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker exec chpasswd: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
```

**Analog 4（admin handler 内 RecordEvent + writeJSON 错误响应）:** `admin_hosts.go:395-417`（`RotateSSHPassword` 末尾的 RecordEvent + writeJSON 风格）。

**Analog 5（PathValue + writeJSON 200/4xx/5xx）:** `admin_hosts.go:101-148`（`Get()` —— `r.PathValue("hostID")` + `errors.Is(err, pgx.ErrNoRows)` 走 404 + 默认 500）。

**verbatim 复制起点：** 见 RESEARCH §2.5 完整 handler 文件骨架（注意 §2.5 末尾 `var _ = json.Marshal` 占位需删除——RESEARCH 已声明）。

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 文件名 `admin_claude_accounts.go` | **verbatim**（D-28 Plan 02 字面） |
| `AdminClaudeAccountStore` 接口 + `BeginTx(ctx) (pgx.Tx, error)` 唯一方法 | **verbatim**（与 `AdminHostStore` 风格强一致；最小化暴露面） |
| `var runHostAction = func(ctx, client, req) ...` 包级 var | **verbatim**（沿用 `var syncContainerPassword` 模式；handler 单测必须依赖此 var 注入 mock） |
| `parseForceFlag(s)` 函数 + 接受 `"true" / "1" / "yes"` | **可微调**（CONTEXT Discretion 第 5 条，planner 决定；推荐三者全收） |
| 事务 `defer rollback` 模式（`rollback := true; defer if rollback { tx.Rollback }`） | **verbatim**（标准 Go pgx 事务 idiom） |
| `context.WithTimeout(r.Context(), 10*time.Second)` 强一致路径 | **verbatim**（D-20） |
| `context.WithTimeout(r.Context(), 30*time.Second)` force 路径 | **verbatim**（D-20） |
| HTTP 409 + 错误 body `{"error": {"code": "STATE_VOLUME_IN_USE_001", "message": "...", "next_action": "..."}}` | **verbatim**（CONTEXT `<code_context>` 行 251 错误码体系 + D-18 中文消息） |
| 错误码 `STATE_VOLUME_IN_USE_001` | **verbatim**（RESEARCH §4 错误码表；前缀 `STATE_` + 序号 `_001` 需保持） |
| 中文消息原文 "请先停止使用该账号的所有 host 后重试，或追加 ?force=true 强删 volume" | **verbatim**（D-18 决策原文） |
| audit event Type `claude_account.delete_volume_rm_failed` / `claude_account.force_volume_rm_failed` / `claude_account.deleted` | **verbatim**（D-18 + D-19 + RESEARCH §4 事件表） |
| metadata key `account_id` / `volume_name` / `error_code` / `error_message` | **verbatim**（D-21 + Discretion 第 3 条） |
| metadata **禁止** 含 OAuth token / `email` 也可写但不推荐（仅 account_id 已足够） | **守恒**（D-21 + Phase 29.1 Plan 02 mitigation） |
| 路由方法 `nethttp.Handler` 返回值 + `nethttp.HandlerFunc` 包装 | **verbatim**（与 `admin_hosts.go` 全部 handler 风格一致） |
| **不**新增 service 层 / use case 层 | **verbatim**（D-29） |

---

### `internal/controlplane/http/admin_hosts.go`（modify — host detail 字段追加）

**Analog:** 同文件 `adminHostDetailResponse`（`admin_hosts.go:96-99`）+ `Get()` handler（行 101-148）。

```96:99:internal/controlplane/http/admin_hosts.go
type adminHostDetailResponse struct {
	repository.HostDetail
	ConnectionInfo *repository.ConnectionInfo `json:"connection_info,omitempty"`
}
```

**verbatim 复制起点（追加一行）：**

```go
type adminHostDetailResponse struct {
	repository.HostDetail
	ConnectionInfo       *repository.ConnectionInfo `json:"connection_info,omitempty"`
	PersistentVolumeName string                     `json:"persistent_volume_name,omitempty"` // Phase 33 D-22
}
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| JSON tag `persistent_volume_name,omitempty` | **verbatim**（D-22 + Phase 30 D-03 兼容策略：旧前端忽略未知字段） |
| 字段类型 `string`（非指针） | **verbatim**（与 `HostWithClaudeAccount.PersistentVolumeName` 一致；`omitempty` 对空串生效） |
| `Get()` 内 enrich 方式 | **可微调**：RESEARCH §2.7 推荐 handler 内调一次 `GetHostWithClaudeAccount(hostID)` 仅取 `PersistentVolumeName`，不动 `GetHostDetail` 签名（与 OOS-A19 "最多加一行" 边界一致） |
| **不**改 `List()` / `getDockerStatuses` | **verbatim**（D-22 + RESEARCH §6 反模式：detail handler 必须**纯 DB JOIN** 不引入 docker exec） |
| `AdminHostStore` 接口要不要增方法 | **可微调**：planner 可在 `AdminHostStore` 加 `GetHostWithClaudeAccount(ctx, id) (HostWithClaudeAccount, error)`，让 handler 走同一 store；推荐采纳此方案以维持单一 store 接口 |

---

### `internal/controlplane/http/router.go`（modify — 路由注册 + Dependencies 字段扩展）

**Analog:** 同文件 `if deps.AdminHosts != nil { ... }` 块（`router.go:232-256`）+ `Dependencies` struct（行 27-54）。

```232:236:internal/controlplane/http/router.go
		if deps.AdminHosts != nil {
			hostsHandler := NewAdminHostsHandler(deps.Logger, deps.AdminHosts, deps.HostActions, deps.EventRecorder)
			mux.Handle("GET /v1/admin/hosts", adminGuard(hostsHandler.List()))
			mux.Handle("POST /v1/admin/hosts", adminGuard(hostsHandler.Create()))
			mux.Handle("POST /v1/admin/hosts/resync-passwords", adminGuard(hostsHandler.ResyncPasswords()))
```

**verbatim 复制起点（来自 RESEARCH §2.6）：**

```go
if deps.AdminClaudeAccounts != nil {
	claudeHandler := NewAdminClaudeAccountsHandler(deps.Logger, deps.AdminClaudeAccounts, deps.AgentClient, deps.EventRecorder)
	mux.Handle("DELETE /v1/admin/claude-accounts/{accountID}", adminGuard(claudeHandler.Delete()))
}
```

`Dependencies` 追加：

```go
AdminClaudeAccounts AdminClaudeAccountStore
AgentClient         *agentapi.Client
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 路由 `DELETE /v1/admin/claude-accounts/{accountID}` | **verbatim**（D-17 + REST 复数命名与 `/v1/admin/users/{userID}` 一致） |
| `adminGuard(...)` 中间件链 | **verbatim**（与 `AdminHosts` 块完全对称；admin role 强制） |
| 路径参数名 `{accountID}` | **verbatim**（与 handler `r.PathValue("accountID")` 一致） |
| Dependencies 字段名 `AdminClaudeAccounts` / `AgentClient` | **verbatim**（与 `AdminHosts` / `EventRecorder` 风格一致） |
| 字段插入位置 | 紧跟 `AdminHosts` 字段之后或文件末尾，保持 admin/agent 字段集中 |
| 注册块插入位置 | 紧跟 `if deps.AdminHosts != nil { ... }` 块之后（行 256 之后） |

---

### `internal/runtime/tasks/worker_volume_test.go`（modify — 追加 Action round-trip + BuildClaudeStateVolumeName 测试）

**Analog:** 同文件 `TestHostActionRequest_ClaudeAccountID_RoundTrip`（`worker_volume_test.go:109-131`）。

```109:131:internal/runtime/tasks/worker_volume_test.go
// TestHostActionRequest_ClaudeAccountID_RoundTrip 守护 D-09：
// 非空 claude_account_id 必须完整 round-trip，供 Phase 33 worker 组装 volume/容器使用。
func TestHostActionRequest_ClaudeAccountID_RoundTrip(t *testing.T) {
	req := agentapi.HostActionRequest{
		TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost,
		ClaudeAccountID: "acct-42",
	}
	buf, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(buf), `"claude_account_id":"acct-42"`) {
		t.Fatalf("non-empty ClaudeAccountID must serialize as claude_account_id, got: %s", buf)
	}

	var parsed agentapi.HostActionRequest
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if parsed.ClaudeAccountID != "acct-42" {
		t.Fatalf("round-trip lost ClaudeAccountID: got %q, want %q", parsed.ClaudeAccountID, "acct-42")
	}
}
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 测试名 `TestHostActionRequest_VolumeRemove_RoundTrip` | 可微调，`Test*_RoundTrip` 风格必保留 |
| 断言 `"action":"volume_remove"` | **verbatim**（D-13 + D-25.4） |
| `BuildClaudeStateVolumeName` 边界（空 id 返错） | **verbatim**（D-25.1 + RESEARCH §2.3 (a) 函数签名） |
| 测试位置 | 同文件追加（避免新建文件造成测试散乱） |

---

### `internal/runtime/tasks/worker_volume_lifecycle_test.go`（new — ensureDockerVolume / removeDockerVolume mock 测试）

**Analog:** §RESEARCH 5.1 的 `dockerVolumeRunner` 包级 var 抽象提案 + 仓库 `var execInContainer = ...` 替换模式（`worker_password_test.go` 已有同模式）。

**verbatim 复制起点：** 见 RESEARCH §5.1 完整测试样板（`TestEnsureDockerVolume_Idempotent` / `TestRemoveDockerVolume_NotFound_IsSuccess` / `TestRemoveDockerVolume_InUse_PropagatesError`）。

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 是否引入 `var dockerVolumeRunner = ...` 抽象层 | **可微调**：RESEARCH §5.1 推荐此抽象（与 `realEnsureDockerVolume` 内 `exec.CommandContext` 调用对齐）；planner 也可选择直接替换 `var ensureDockerVolume = ...` 写"行为级"mock，但 docker 错误字符串测试无法覆盖（D-27 显式要求）。**强烈推荐采纳 dockerVolumeRunner 方案** |
| docker 错误字符串两条最小集 `"no such volume"` / `"volume is in use"` | **verbatim**（D-27 + RESEARCH §6.1） |
| `t.Cleanup(func() { dockerVolumeRunner = orig })` 恢复模式 | **verbatim**（避免测试串污） |
| 文件名 `worker_volume_lifecycle_test.go` | 可微调；推荐与现有 `worker_volume_test.go` 区分（前者纯函数 + JSON，后者真 docker exec mock） |

---

### `internal/store/repository/queries_claude_account_volume_test.go`（new — Upsert SQL 单测）

**Analog:** 仓库内 `internal/store/repository/queries_host_entry_password_test.go`（grep 已确认存在；测试 SQL 常量字符串 + skip-on-no-db 集成）。

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 测试 D-25.7 三条路径（NULL→写入 / 一致跳过 / 冲突 audit） | **verbatim**（CONTEXT D-25 第 7 条字面） |
| SQL 字符串断言（最小集，无需 pgxpool） | **必做**（断言 `upsertClaudeAccountPersistentVolumeNameSQL` 包含 `"WHERE id = $1 AND persistent_volume_name IS NULL"` 关键 token） |
| pgxpool 真库集成（可选） | 可微调；推荐 `t.Skip` 兜底（沿用 Phase 31/32 风格，CONTEXT D-27 字面） |
| 文件名 | 可微调；推荐 `queries_claude_account_volume_test.go` 与功能分组一致 |

---

### `internal/controlplane/http/admin_claude_accounts_test.go`（new — handler 单测）

**Analog:** `admin_users_test.go:79-117`（`stubEventRecorder` + `validAdminToken`）+ `admin_hosts_test.go:21-66`（`stubHostStore` 模板）+ `adminTestRouter`（`admin_users_test.go:119+`）。

```79:117:internal/controlplane/http/admin_users_test.go
type stubEventRecorder struct {
	called bool
	events []repository.RecordEventParams
}

func (s *stubEventRecorder) RecordEvent(_ context.Context, p repository.RecordEventParams) (repository.Event, error) {
	s.called = true
	s.events = append(s.events, p)
	return repository.Event{}, nil
}

func (s *stubEventRecorder) hasType(t string) bool {
	for _, ev := range s.events {
		if ev.Type == t {
			return true
		}
	}
	return false
}

var testJWTSecret = []byte("test-jwt-secret-for-admin-api")

func validAdminToken(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			Issuer:    "cloud-cli-proxy",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: "admin",
		Role:   "admin",
	})
	s, err := token.SignedString(testJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
```

**planner 提示：**

| 字段 | 处理 |
|------|------|
| 复用 `stubEventRecorder` / `validAdminToken` / `adminTestRouter` | **verbatim**（同 package，直接调用） |
| 新增 `stubAdminClaudeAccountStore` 实现 `BeginTx(ctx) (pgx.Tx, error)` | **必做**（pgx.Tx 是 interface，可手写最小 stub；RESEARCH §5.2 stubTx 样板可参考） |
| stub Tx 实现方式 | **可微调**：RESEARCH §5.2 列了 (a) 手写最小 stub vs (b) 引入 `pashagolub/pgxmock` 两选项；推荐 (a)（仓库无新增依赖、与现有测试风格一致） |
| 替换 `var runHostAction = ...` 注入 mock | **verbatim**（沿用 `syncContainerPassword` 测试模式；测试结尾 `t.Cleanup` 恢复 orig） |
| 三条主测试 case：strict 成功 / strict 409+回滚 / force=true 200+rm 失败 | **verbatim**（D-25.5 字面） |
| 断言 `stubEventRecorder.hasType("claude_account.delete_volume_rm_failed")` | **verbatim**（D-21 命名 + RESEARCH §4） |
| 文件名 `admin_claude_accounts_test.go` | **verbatim**（与 handler 文件名对应） |

---

## Shared Patterns

> 跨多个 Plan 文件的横切关注点。planner 在每个 plan 的 actions 段都需引用本节。

### Shared 1：包级 `var X = realX` mock 注入模式

**Source：**`internal/runtime/tasks/worker.go:687-695`（`var execInContainer`）+ `internal/controlplane/http/admin_hosts.go:1014-1023`（`var syncContainerPassword`）

**Apply to：**
- `var ensureDockerVolume = realEnsureDockerVolume`（worker.go）
- `var removeDockerVolume = realRemoveDockerVolume`（worker.go）
- `var dockerVolumeRunner = func(ctx, args...) ([]byte, error) { ... }`（worker.go，RESEARCH §5.1 推荐新增可测性层）
- `var runHostAction = func(ctx, client, req) (HostActionResponse, error) { ... }`（admin_claude_accounts.go）

**Rule：** 所有"调用 docker CLI / 远程 host-agent"的入口必须暴露为包级 var；测试通过临时替换 + `t.Cleanup` 恢复。

---

### Shared 2：SQL 包级 const 提升

**Source：**`internal/store/repository/queries.go:1180+`（Phase 29.1 Plan 01 强制约定，仓库已有 6+ 条 const SQL）

**Apply to：** Plan 01/02 全部新增 SQL 字符串：
- `upsertClaudeAccountPersistentVolumeNameSQL`
- `checkClaudeAccountPersistentVolumeNameSQL`
- `getHostWithClaudeAccountSQL`
- `lockClaudeAccountForDeleteSQL`
- `deleteClaudeAccountSQL`

**Rule：** 所有 SQL 必须为包级 `const xxxSQL = \`...\`` 字符串；handler/method 内只允许 `r.db.Exec(ctx, xxxSQL, ...)` / `r.db.QueryRow(ctx, xxxSQL, ...)`，禁止行内拼字符串。配套：测试断言 SQL 字符串包含关键 token（`"WHERE id = $1 AND persistent_volume_name IS NULL"` 等）。

---

### Shared 3：audit event metadata 不写凭据

**Source：**Phase 29.1 Plan 02 mitigation + CONTEXT D-21 + `worker.go` 全文 `RecordEvent` 调用（如 `recordNetworkError` 行 657-668）

**Apply to：** Plan 01/02 全部 audit event：
- `claude_account.volume_create_failed` / `volume_name_persist_failed` / `volume_rm_failed`（worker）
- `claude_account.delete_volume_rm_failed` / `force_volume_rm_failed` / `deleted`（admin handler）

**Rule：** metadata 仅含 `account_id` / `volume_name` / `error_code` / `error_message` / `force` / `host_id`（如可用）。**禁止**含 `email` / OAuth token / `EntryPassword` / 任何 SSH key / 容器内文件绝对路径（`/var/lib/...` 例外，是固定字符串非用户路径）。

---

### Shared 4：HTTP 中文错误响应格式

**Source：**CONTEXT `<code_context>` 行 251 + Phase 31/32/34 错误码体系

**Apply to：** Plan 02 admin handler 全部 4xx 响应。

**Rule：** body 严格为：

```json
{"error": {"code": "<DOMAIN>_<KIND>_<NUM>", "message": "中文原因", "next_action": "中文下一步"}}
```

Phase 33 唯一新错误码 `STATE_VOLUME_IN_USE_001`（D-18 + RESEARCH §4）。中文消息禁止用 LLM 改写（决策原文已锁定字面）。

---

### Shared 5：context 超时强约束

**Source：**`admin_hosts.go:614,713,762,819,907,974` 已有 6 处 `context.WithTimeout(r.Context(), 10*time.Second)` 先例

**Apply to：** Plan 02 admin handler 全部 host-agent 调用：
- 强一致路径 → `context.WithTimeout(r.Context(), 10*time.Second)`（D-20）
- force 路径 → `context.WithTimeout(r.Context(), 30*time.Second)`（D-20）

**Rule：** 调用 `runHostAction(ctx, ...)` 之前必须已 `defer cancel()`；禁止直接传 `r.Context()`（docker daemon hang 会拖死请求）。

---

## No Analog Found

无。Phase 33 全部 12 个目标文件均能在仓库内找到 1:1 形似 analog（前置 Phase 29 / 29.1 / 30 已奠定全部基础设施）。

---

## Metadata

**Analog search scope:**
- `deploy/docker/managed-user/`（entrypoint 函数）
- `internal/agentapi/`（协议常量 + Client）
- `internal/runtime/tasks/`（worker + 测试）
- `internal/store/repository/`（queries + models + migrator）
- `internal/controlplane/http/`（admin handler + router + 测试）

**Files scanned:** 13（worker.go / contracts.go / queries.go / models.go / admin_hosts.go / admin_users_test.go / admin_hosts_test.go / router.go / entrypoint.sh / worker_volume_test.go / agent/server.go / migrator/migrator.go / agentapi/client.go）

**Pattern extraction date:** 2026-04-21

**Mapping confidence:** HIGH —— 所有 analog 行号已 verified；verbatim 起点全部来自 RESEARCH §2 已沉淀的 7 段代码骨架，无需 planner 二次发明。
