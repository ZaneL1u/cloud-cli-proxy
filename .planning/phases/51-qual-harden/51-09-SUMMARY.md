---
phase: 51-qual-harden
plan: 51-09
status: completed
completed_at: 2026-05-14
gap_closure:
  - phase-47-gap-d-47-3
---

# 51-09 SUMMARY — 双绑互斥 API pre-check + 稳定 error code

## 落地清单

- `internal/store/repository/queries.go`：新增
  ```go
  func (r *Repository) GetBindingHostIDByEgressIP(ctx context.Context, egressIPID string) (string, error)
  ```
  SQL：`SELECT host_id::text FROM host_egress_bindings WHERE egress_ip_id = $1 LIMIT 1`，
  row 不存在返回 `pgx.ErrNoRows`。
- `internal/controlplane/http/admin_bindings.go`：
  - `AdminBindingStore` interface 新增 `GetBindingHostIDByEgressIP(ctx, egressIPID) (string, error)` 方法。
  - 新增常量 `ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"`。
  - `Bind` 处理函数在通过 host running 闸后、调 `BindEgressIPToHost` 前：
    - 先查 `GetBindingHostIDByEgressIP(req.EgressIPID)`；
    - 若已绑定到不同 host → 409 + JSON 含 `error_code` / `error`（中英双语
      子串：「出口 IP 已绑定到其它宿主机 (egress IP already bound to another host)」）
      / `host_id` / `egress_ip_id`；
    - 若已绑定到同 host 或未绑定 → 走原 INSERT 路径（幂等性由 UNIQUE
      (host_id, egress_ip_id) 复合键兜底）；
    - DB error（非 ErrNoRows）→ 500 with `"check existing binding failed"`。
- `internal/controlplane/http/admin_bindings_test.go`：
  - `stubBindingStore` 新增 `existingEgressHostID` / `existingEgressErr` 字段 +
    `GetBindingHostIDByEgressIP` 方法实现（默认返回 `pgx.ErrNoRows`，向后兼容
    原 8 个 case）。
  - table-driven 套件新增 2 case：双绑另一 host 409 + 同 host re-bind 201
    幂等。
  - 新增独立 `TestAdminBindings_DoubleBind_ErrorCode` 锁定 error_code +
    中英文 message 子串 + host_id / egress_ip_id 回显。

## 验证

- `go build ./...` + `GOOS=linux go build ./...` PASS。
- `go test ./internal/controlplane/http/... -run 'Admin.*Binding|DoubleBind'`
  PASS（既有 8 case 全通过 + 新增 3 case 全通过）。
- `go test ./... -count=1` 全绿。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。

## 与 Phase 47 D-47-3 闭环

- Phase 47 helpers `ParseBindEgressIPResponse` 已定义解析 `error_code` 字段
  与 `ErrorMessage` 字段；本 plan 落地后实际响应 body 同时含：
  - `error_code = "egress_ip_already_bound"`（机器可读，命中契约 +
    `EgressIPDoubleBindContract.WantStatus = 409`）
  - `error` 中含 `"already bound"` 子串（命中
    `EgressIPDoubleBindContract.WantErrSubstring = "already bound"`）。
- 因此 Phase 47 e2e 用例 `TestEgressIPBinding_DoubleBindExcluded`（Linux runner
  deferred-to-CI）在 backend 修复后 backend GAP 分支会直接命中 PASS，无需修
  e2e 用例代码。

## 偏差

- 无。
