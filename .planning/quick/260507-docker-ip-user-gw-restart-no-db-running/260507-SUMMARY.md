---
phase: quick
plan: "260507"
subsystem: "runtime / network / controlplane-scheduler"
tags: [docker, restart-policy, reconciler, auto-recovery, ip-leak, sing-box]
dependency_graph:
  requires: []
  provides: ["docker-restart-no-leak", "reconciler-auto-recover"]
  affects: ["internal/runtime/tasks/worker.go", "internal/network/container_proxy_provider.go", "internal/controlplane/scheduler/reconciler.go", "internal/controlplane/app/app.go"]
tech_stack:
  added: []
  patterns: ["HostActionQueuer interface reuse", "fallback-on-error resilience", "backward-compatible nil queuer"]
key_files:
  created: []
  modified:
    - internal/runtime/tasks/worker.go
    - internal/network/container_proxy_provider.go
    - internal/controlplane/scheduler/reconciler.go
    - internal/controlplane/scheduler/reconciler_test.go
    - internal/controlplane/app/app.go
decisions:
  - "复用 expiry.go 中已定义的 HostActionQueuer 接口，避免同一包内重复声明"
  - "QueueHostAction 失败时自动回退到原有 drift 行为（UpdateHostStatus stopped + drift 事件），保证对账链路不因队列故障而中断"
  - "queuer == nil 时保持原有行为不变，确保嵌入式模式（embeddedMode）等未传入 queuer 的调用方零破坏"
metrics:
  duration: "~8 minutes"
  completed_date: "2026-05-07"
---

# Quick Task 260507: Docker 重启后出口 IP 泄漏修复

## 一句话总结

将 user/gateway 容器 restart 策略从 `unless-stopped` 改为 `no`，并在控制面对账逻辑中新增自动恢复：DB 标记 running 但 docker 实际不存在/停止的主机，自动 QueueHostAction(ActionStartHost) 重建完整网络栈。

## 执行内容

### Task 1: 修改 restart 策略为 no

- `internal/runtime/tasks/worker.go` `buildCreateArgs`：`--restart unless-stopped` -> `--restart no`
- `internal/network/container_proxy_provider.go` `dockerRunGateway`：`--restart unless-stopped` -> `--restart no`
- 两处同步修改，grep 确认无残留 `unless-stopped`

### Task 2: Reconciler 新增自动恢复逻辑

- `Reconciler` 结构体新增 `queuer HostActionQueuer` 字段（复用 `expiry.go` 中已定义的接口）
- `NewReconciler` 新增 `queuer` 参数；`queuer == nil` 时向后兼容，保持原有 drift 行为
- `reconcileHosts` 逻辑变更：
  - 容器存在且运行中：跳过（不变）
  - 容器不存在或停止 + `queuer != nil`：调用 `QueueHostAction(ActionStartHost, "system")` 自动恢复，记录 `reconcile.host.auto_recover` 事件
  - 自动恢复失败：回退到原有 drift 行为（UpdateHostStatus stopped + `reconcile.host.drift` 事件）
  - `queuer == nil`：保持原有 drift 行为
- 测试新增 3 个用例：
  - `Run_ContainerStopped_QueuesStartHost`：验证 stopped -> auto_recover
  - `Run_ContainerNotFound_QueuesStartHost`：验证 not_found -> auto_recover
  - `Run_QueuerError_FallsBackToDrift`：验证 QueueHostAction 失败回退 drift
- `internal/controlplane/app/app.go`：`!embeddedMode` 分支中，将 `runtimeService` 作为 queuer 传入 `NewReconciler`

### Task 3: 编译与回归验证

- `go build ./...`：全项目编译通过，无错误
- `go test ./internal/controlplane/scheduler/... -v`：13 个测试全部 PASS（5 ExpiryScanner + 8 Reconciler）
- `go test ./internal/runtime/tasks/... -run TestBuildCreateArgs`：5 个测试 PASS
- `go test ./internal/network/...`：18 个测试 PASS

## 偏离计划

无。计划按预期执行，无偏差。

## 已知 Stub

无。所有修改均为生产可用代码，无占位符或 TODO。

## Self-Check: PASSED

- [x] `grep -rn "unless-stopped" internal/runtime/tasks/worker.go internal/network/container_proxy_provider.go` 返回空
- [x] `go test ./internal/controlplane/scheduler/... -v` 全部 PASS（含新增 3 个测试）
- [x] `go build ./...` 无编译错误
- [x] 提交记录存在：67e8d21、03e7dcb

## 提交记录

| Commit | 类型 | 说明 |
|--------|------|------|
| 67e8d21 | fix | user/gw 容器 restart 策略改为 no |
| 03e7dcb | feat | Reconciler 自动恢复 DB-running 但 docker-stopped 的主机 |
