---
phase: 33-claude-code-cli-admin-gc
reviewed: 2026-04-21T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - deploy/docker/managed-user/entrypoint.sh
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker.go
  - internal/runtime/tasks/embedded_dispatcher.go
  - internal/runtime/runtime_service.go
  - internal/store/repository/queries.go
  - internal/store/repository/models.go
  - internal/controlplane/http/admin_claude_accounts.go
  - internal/controlplane/http/admin_hosts.go
  - internal/controlplane/http/router.go
  - internal/controlplane/app/app.go
findings:
  blocking: 0
  high: 2
  medium: 5
  low: 3
  info: 5
  total: 15
status: issues_found
---

# Phase 33：Claude Code CLI Admin GC — 代码审查报告

**审查时间：** 2026-04-21
**深度：** standard（按文件读全文 + 跨文件接口/调用关系核对）
**文件数：** 11（不含纯文档与测试；测试单独抽样核对覆盖度）
**结论：** issues_found（无 blocking；2 high / 5 medium / 3 low / 5 info）

## 摘要

Phase 33 的核心改动 — Volume 生命周期（Plan 01）+ 强一致 / force 双路径 DELETE（Plan 02）+ 三处后置补丁 — **整体设计严谨**：

- SQL 全部走 `$N` 参数化，未发现注入面（含动态 `ListEvents`，仅占位符走 `fmt.Sprintf`，参数仍是 args）。
- 审计 `Metadata` 在新增的 5 处 `RecordEvent` 调用里**无 email / entry_password / credentials / oauth_token 泄漏**。
- `pullImage` 5 分钟 timeout、admin handler 10s/30s timeout 都用了 `context.WithTimeout` + `defer cancel()`，主路径 ctx 传播正确。
- `HostActionRunner` 接口被 `*agentapi.Client.RunHostAction`（远端模式）和 `*EmbeddedDispatcher.RunHostAction`（embedded 模式）双向满足；`QueueHostActionRepo` 6 个方法 `*Repository` 全部实现。

但仍有 **2 个 High** 级别的"可工作但脆弱"耦合需要在下次维护时收紧；以及若干 Medium / Low 级别的健壮性 / 一致性差距。详情见下。

---

## 高（High）

### HR-01：`actionToHostStatus` 对 `ActionVolumeRemove` 走 default 分支返回 `"stopped"`，是隐式正确性

**文件：** `internal/runtime/tasks/worker.go:131-144` + `internal/runtime/tasks/worker.go:110-111`
**问题：**
当前 admin DELETE handler 构造的 `HostActionRequest` 里 `HostID` 是空串（`admin_claude_accounts.go:94-97`），所以 `_ = w.repo.UpdateHostStatus(ctx, request.HostID, "stopped")` 退化成 `UPDATE hosts WHERE id=''` 的 0-row no-op，不会破坏数据。这个"安全"完全靠调用方留空 HostID 维持。

```131:144:internal/runtime/tasks/worker.go
func actionToHostStatus(action agentapi.HostAction) string {
	switch action {
	case agentapi.ActionCreateHost:
		return "running"
	case agentapi.ActionStartHost:
		return "running"
	case agentapi.ActionStopHost:
		return "stopped"
	case agentapi.ActionRebuildHost:
		return "running"
	default:
		return "stopped"
	}
}
```

**风险：** 任何后续 caller（例如把 VolumeRemove 接到带 HostID 的清理流程）只要把 `request.HostID` 填上，就会**静默把 host 状态翻成 stopped**，且无审计事件。
**修复建议：**

```go
case agentapi.ActionVolumeRemove:
    return "" // explicit no-op marker
```

并在 `Execute` 的成功分支加：

```go
hostStatus := actionToHostStatus(request.Action)
if hostStatus != "" {
    _ = w.repo.UpdateHostStatus(ctx, request.HostID, hostStatus)
}
```

### HR-02：embedded 模式下 admin DELETE 触发的 `UpdateTaskStatus` 必然失败并被吞掉，污染日志

**文件：** `internal/runtime/tasks/embedded_dispatcher.go:25-29` + `internal/runtime/tasks/worker.go:119-129`
**问题：**
`AdminClaudeAccountsHandler.deleteStrict / deleteForce` 构造的请求**没有 TaskID**（`admin_claude_accounts.go:94-97 / 174-178`）。在 embedded 模式下：

1. `EmbeddedDispatcher.RunHostAction` → `Dispatch` → `worker.Execute` 返回 `update.TaskID == ""`。
2. `worker.UpdateTaskStatus(ctx, update)` 执行 `UPDATE tasks SET ... WHERE id = ''`，0 行匹配，pgx 不报错；但若 worker 修了 SQL 变成 `RETURNING ...`，立即返回 `ErrNoRows`。
3. `EmbeddedDispatcher.Dispatch` 现已用 `slog.Error("embedded: failed to update task status", "error", err)` 吞掉。

```25:29:internal/runtime/tasks/embedded_dispatcher.go
	update := d.worker.Execute(ctx, request)

	if err := d.worker.UpdateTaskStatus(ctx, update); err != nil {
		slog.Error("embedded: failed to update task status", "error", err)
	}
```

实际 worker.go:734-761 的 UpdateTaskStatus 用了 `RETURNING`，对空 taskID 必然走到 `pgx.ErrNoRows`，每次 admin DELETE 都会落一条 `level=ERROR` 的日志。

**风险：** 误导排障；被 SLO 告警当成真错。
**修复建议：** 在 `EmbeddedDispatcher.Dispatch` 中加一道短路：

```go
if update.TaskID == "" {
    // 例如 ActionVolumeRemove 来自 admin handler，不是 task 排队产物
    return agentapi.HostActionResponse{Update: update}, nil
}
```

或反过来 — 在 `runHostAction` 之前由 caller 显式注入合成 TaskID（更符合"调用 host-agent 必有 task 上下文"的不变量）。

---

## 中（Medium）

### MR-01：`deleteForce` 的 `tx.Rollback(ctx)` 用了已可能超时的 ctx；`deleteStrict` 改用 `context.Background()`，两者不一致

**文件：** `internal/controlplane/http/admin_claude_accounts.go:78,152,161`
**问题：**
strict 路径里把 rollback 显式包到 background 上（保证即使请求被客户端取消也能回滚），force 路径却没做：

```76:80:internal/controlplane/http/admin_claude_accounts.go
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback(context.Background())
		}
	}()
```

```151:165:internal/controlplane/http/admin_claude_accounts.go
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			...
		}
	}
	if err := repository.DeleteClaudeAccountTx(ctx, tx, id); err != nil {
		_ = tx.Rollback(ctx)
		...
	}
```

**风险：** 30s 超时触发后或客户端断开后，rollback 用 cancelled ctx 在 pgx 内部仍会发 `ROLLBACK`（内部会换个 conn），但语义不一致；再叠加 lock 行为，未来若改成 SAVEPOINT / RETURNING 路径会更脆。
**修复建议：** 与 strict 对齐，统一 `_ = tx.Rollback(context.Background())`。

### MR-02：`pullImage` 静默吞掉 docker pull 错误（含超时），只落 `slog.Warn`，不落 audit event

**文件：** `internal/runtime/tasks/worker.go:874-890`

```874:890:internal/runtime/tasks/worker.go
func (w *Worker) pullImage(ctx context.Context, imageName string) {
	pullCtx, cancel := context.WithTimeout(ctx, pullImageTimeout)
	defer cancel()
	cmd := exec.CommandContext(pullCtx, "docker", "pull", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		timedOut := errors.Is(pullCtx.Err(), context.DeadlineExceeded)
		slog.Warn("docker pull failed, will use local image if available",
			"image", imageName,
			"error", err,
			"timed_out", timedOut,
			...
		return
	}
```

**风险：** 后续 `docker create` 若本地无 image，会以一个非常通用的 `docker create ...` 错误失败，运维要回到日志去找上面那条 `slog.Warn` 才能定位是 pull 阶段超时。审计表上完全看不到。
**修复建议：** 在 `err != nil` 分支加：

```go
_, _ = w.repo.RecordEvent(ctx, repository.RecordEventParams{
    HostID:  &request.HostID, // 需要把 hostID 透传进 pullImage 签名
    Level:   "warn",
    Type:    "runtime.image_pull_failed",
    Message: err.Error(),
    Metadata: map[string]any{"image": imageName, "timed_out": timedOut},
})
```

（需要把 `hostID` 透传到 `pullImage`，签名小改即可。）

### MR-03：`deleteForce` 在 `LockClaudeAccountForDelete` 出错时少了一行 `h.logger.Error`

**文件：** `internal/controlplane/http/admin_claude_accounts.go:150-159` vs `:82-91`
**问题：** strict 路径在 lock 出错时会 `h.logger.Error("lock claude_account failed", ...)`；force 路径只 writeJSON 500，没有日志，导致出问题时排障要靠 DB 反查。
**修复建议：** 拷一份相同的 `h.logger.Error` 到 force 路径。

### MR-04：strict 路径在 host-agent 删 volume 成功后 Commit 失败 → 数据漂移（DB 行还在，volume 已不存在）

**文件：** `internal/controlplane/http/admin_claude_accounts.go:93-125`
**问题：** D-18 设计承认：
1. lock → 2. host-agent rm volume → 3. DELETE row → 4. COMMIT
若第 4 步 COMMIT 失败（DB 短暂抖动），第 2 步已经把 volume 物理删了，回滚 DB 救不回来 — 留下"DB 里 row 还在但 persistent_volume_name 指向不存在的 volume"的漂移。当前代码：

```120:125:internal/controlplane/http/admin_claude_accounts.go
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("commit failed", "error", err)
		writeJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}
```

仅记 logger.Error，没有 audit event 落库，运维侧从事件流里完全看不见。
**修复建议：** 在 `tx.Commit` 失败分支补一条 `claude_account.delete_drift` 审计事件，metadata 含 `account_id` / `volume_name` / `commit_error`。后续配合一个对账 job 扫这种事件。

### MR-05：`runHostAction` 默认实现对 `nil` agentClient 没有兜底，依赖运行期注入正确

**文件：** `internal/controlplane/http/admin_claude_accounts.go:30-32`

```30:32:internal/controlplane/http/admin_claude_accounts.go
var runHostAction = func(ctx context.Context, client HostActionRunner, req agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	return client.RunHostAction(ctx, req)
}
```

测试里把 `client` 传 `nil` 是因为同时把 `runHostAction` mock 掉了（见 `admin_claude_accounts_test.go:138`）。生产中 `app.go:104,113` 总会注入非 nil — 当前安全。
**风险：** 若未来 `Dependencies.AgentClient` 在某一段 init 顺序里被忘记设值，`volumeName != ""` 分支会直接 nil pointer panic。
**修复建议：** `Delete()` handler 入口或 `runHostAction` 内做一次显式校验：

```go
if h.agentClient == nil && volumeName != "" {
    h.logger.Error("agent client not configured for volume removal")
    writeJSON(w, nethttp.StatusServiceUnavailable, ...)
    return
}
```

---

## 低（Low）

### LR-01：`parseForceFlag` 大小写敏感，不接受 `"TRUE"` / `"True"` / `"y"`

**文件：** `internal/controlplane/http/admin_claude_accounts.go:56-62`
**问题：** 测试 `TestParseForceFlag_AcceptsTrueOneYes` 已断言 `"TRUE"` 返回 false。属于刻意收窄，不是 bug。
**风险：** 用户 / 调试侧用 `?force=True` 会误以为强删生效，实际走 strict。
**修复建议：** `strings.EqualFold` 一行升级：

```go
switch strings.ToLower(s) {
case "true", "1", "yes":
    return true
}
```

并把测试的 `"TRUE": false` 改成 `true`。

### LR-02：`removeVolumes` 多 volume 时遇错短路，剩余 volume 不清理

**文件：** `internal/runtime/tasks/worker.go:1022-1039`

```1022:1039:internal/runtime/tasks/worker.go
func (w *Worker) removeVolumes(ctx context.Context, request agentapi.HostActionRequest) error {
	force := request.Labels["force"] == "true"
	for _, vm := range request.Volumes {
		if err := removeDockerVolume(ctx, vm.Name, force); err != nil {
			...
			return err
		}
	}
	return nil
}
```

**风险：** 当前 admin handler 每次只塞 1 个 volume（`admin_claude_accounts.go:96`），不会触发；但语义上"删多个 volume 第一个失败就放弃"对未来批量场景不友好。
**修复建议：** 收集所有失败到 multi-error，循环走完再决定整体返回值；或在注释里写明"v1 仅支持单 volume，多 volume 短路是契约"。

### LR-03：`recordEvent` 在 `deleteStrict` 的失败/成功分支用 `r.Context()`，可能在客户端断连后落不下事件

**文件：** `internal/controlplane/http/admin_claude_accounts.go:99,128,180,191` + 199-211
**问题：** 审计是合规需求，理论上应在 `context.Background()` 下落库。当前 `r.Context()` 在客户端取消时会一起 cancel，导致审计事件丢失。
**修复建议：** `recordEvent` 内部 `ctx = context.Background()`（或 `context.WithoutCancel(ctx)` Go 1.21+），确保审计始终落库。

---

## 信息（Info）

### IR-01：`pullImage` 行为没有 ctx-cancellation 真实测试，只锁了常量边界

**文件：** `internal/runtime/tasks/worker_pull_timeout_test.go`
当前测试只断言 `pullImageTimeout > 0` 且 `[1m, 30m]`。后置补丁的核心承诺是"父 ctx 没死，子 ctx 也能在 5 分钟把 hung registry 切掉"，没有用 fake `exec.CommandContext` / `httptest` 验证这一点。
**建议：** 用 `var dockerPullCmd = exec.CommandContext` 抽出来，模拟一个 sleep 6m 的子进程，断言 `pullImage` 在 ~5m 后退出且 `timed_out=true` 落到 slog。

### IR-02：`EmbeddedDispatcher.RunHostAction` 适配器没有直接单测

**文件：** `internal/runtime/tasks/embedded_dispatcher.go:40-42`
新增的接口适配器没有用例验证 `RunHostAction(ctx, req) == Dispatch(ctx, req)`。HR-02 修复后更需要这道守护。
**建议：** 加一个 `TestEmbeddedDispatcher_RunHostAction_DelegatesToDispatch`，验证 fake worker.Execute 被调用且返回值与 `Dispatch` 等价。

### IR-03：`app.go` embedded vs 远端模式的 wiring 没有冒烟测试

**文件：** `internal/controlplane/app/app.go:98-114`
`agentRunner = embeddedDisp` 这一行依赖"`*EmbeddedDispatcher` 满足 `cphttp.HostActionRunner` 接口"，目前靠编译期可见。建议加一个 `TestApp_EmbeddedMode_AgentRunner_NotNil` 编译期 + 运行期双校验。

### IR-04：`injectSSHKeys` 把 `key.Label` 直接拼进 `sh -c` 脚本，未做转义（**预存在，不在 Phase 33 改动范围**，但 worker.go 在审查面里）

**文件：** `internal/runtime/tasks/worker.go:590-598, 610-613, 636-640`

```590:598:internal/runtime/tasks/worker.go
		} else {
			safeName := key.Label
			if safeName == "" {
				safeName = fmt.Sprintf("id_%d", outboundIdx)
			}
			keyFile = sshDir + "/" + safeName
			pubFile = sshDir + "/" + safeName + ".pub"
		}
```

`safeName` 之后被拼进 `mkdir -p %s && cat > %s && chmod 600 %s && chown %s:%s %s`。`key.Label` 来源是管理员通过 admin SSH 密钥 API 写入的 DB 字段，可信度较高，但仍属于"可攻击面"。
**建议（非阻断）：** 引入白名单正则（`^[A-Za-z0-9._-]+$`），label 不合规时回退到 `id_N`。

### IR-05：admin DELETE 的 audit event metadata 缺 host_id 维度

**文件：** `internal/controlplane/http/admin_claude_accounts.go:128-132,191-195`
`claude_account.deleted` 事件只有 `account_id` / `volume_name` / `force`，没有 `host_id`。事件流按 host 维度排查时拉不到。考虑在 `LockClaudeAccountForDelete` 阶段顺手把 `host_id` 一并捞出来落到 metadata。

---

## 跨文件接口 / 调用关系核对

| 关注点 | 状态 | 证据 |
| --- | --- | --- |
| SQL 注入面 | ✅ 干净 | 全部 `$N` 参数化；`ListEvents` 动态 SQL 只把 `$N` 拼进字符串，参数仍走 args |
| 审计 metadata 凭据泄漏 | ✅ 干净 | 5 处新增 `RecordEvent` 仅含 `account_id / volume_name / host_id / force / error_*` |
| `pullImage` ctx 传播 | ✅ 正确 | `context.WithTimeout(ctx, ...) + defer cancel()`，且检测 `pullCtx.Err() == DeadlineExceeded` 区分超时 |
| admin handler 10s/30s timeout | ✅ 正确 | strict 用 10s，force 用 30s；deadline 通过测试断言验证 |
| `HostActionRunner` 接口完整性 | ✅ 双实现 | `*agentapi.Client.RunHostAction`（`client.go:73`）+ `*EmbeddedDispatcher.RunHostAction`（`embedded_dispatcher.go:40`）|
| `QueueHostActionRepo` 接口完整性 | ✅ Repository 全部实现 | `GetHost / GetUser / CreateTask / ListSSHKeysByUser / RecordEvent / ResolveClaudeAccountIDForEntry` 均在 `queries.go` 已存在 |
| `AdminClaudeAccountStore` 接口 | ✅ Repository.BeginTx 实现 | `queries.go:1493-1495` |
| Embedded 模式并发竞态 | ⚠️ 见 HR-02 | Worker 自身无共享状态，但 admin DELETE 路径走 embedded 时 `UpdateTaskStatus` 必败；`runtime_service` 既有 `go func() Dispatch` 模式不变 |
| Volume label 一致性 | ✅ | `claudeAccountLabelKey / claudeManagedLabelKey` 在 createHost 与 ensureDockerVolume 之间一致 |

---

## 后置 3 提交补丁覆盖度复盘

| Commit | 修改内容 | 测试覆盖 | 评级 |
| --- | --- | --- | --- |
| `3e2ba6b` pullImage 5min timeout | `worker.go:874-890` 新增 `pullCtx` + 超时识别 | `worker_pull_timeout_test.go` 仅锁常量边界 | ⚠️ 缺 ctx 取消行为测试（IR-01）|
| `27ab2d7` runtime_service 注入 ClaudeAccountID | `runtime_service.go:33-44, 105-109, 161` + 接口提取 | `runtime_service_test.go` 有 3 个用例覆盖成功 / 未命中 / resolve 出错 | ✅ 充分 |
| `c09a4d0` AdminClaudeAccounts handler wiring | `admin_claude_accounts.go:21-32`、`embedded_dispatcher.go:40-42`、`app.go:96-114, 154-156` | `admin_claude_accounts_test.go` 覆盖 7 用例（strict 成功/409/404/无 volume、force/30s timeout/parseFlag）；`EmbeddedDispatcher.RunHostAction` 适配器无直接测试 | ⚠️ 缺 IR-02 / IR-03 |

---

_Reviewed: 2026-04-21_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
