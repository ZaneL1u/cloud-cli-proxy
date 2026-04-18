---
phase: 29-v3-worker
plan: 04-worker-contract
subsystem: api
tags: [go, docker, volume, host-agent]

key-files:
  created:
    - internal/runtime/tasks/worker_volume_test.go
  modified:
    - internal/agentapi/contracts.go
    - internal/runtime/tasks/worker.go

requirements-completed:
  - D-18
  - D-19
  - D-20
  - D-21
  - D-22

duration: "~20min"
completed: 2026-04-18
---

# Phase 29 Plan 04-worker-contract Summary

为 `HostActionRequest` 增加 `VolumeMount` 与 `Volumes` 字段，并在 `buildCreateArgs` 中为 `docker create` 拼接 `--mount type=volume,src=…,dst=…[,readonly]`；抽出 `buildCreateArgs` 便于单测覆盖；新增 `worker_volume_test.go` 验证 JSON omitempty、v2 兼容与参数组装。

## Task Commits

1. **Task 4.1** — `3da47bd` — `VolumeMount` + `Volumes` 于 `contracts.go`
2. **Tasks 4.2–4.3** — `9b5dadc` — `buildCreateArgs` 与 volume 循环
3. **Task 4.4** — `049392d` — 单元测试

## Verification

- `go vet ./internal/agentapi/... ./internal/runtime/tasks/...` 通过
- `go test ./...` 通过

## Deviations from Plan

无。

## Self-Check: PASSED
