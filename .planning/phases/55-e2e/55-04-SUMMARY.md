---
phase: 55-e2e
plan: "04"
requirements-completed: [E2E-V4-04]
subsystem: test-infra
tags: [e2e, mvs, golden-path, migration, single-container, v4.0]

# Dependency graph
requires:
  - plan: 55-01 (Scenario builder 单容器化 + GoldenPath 去掉 Gateway)
provides:
  - MVS-01..10 + GoldenPath 用例通过更新后的 GoldenPath 间接迁移
affects: []

# Completed tasks
- [x] T1 — 迁移 golden path bootstrap 用例（GoldenPath.Gateway 删除，StartGoldenPath 使用新 API）
- [x] T2 — 迁移 egress IP / DNS / default-deny 用例（通过 GoldenPath 间接使用新 API）
- [x] T3 — 迁移 CLI 错误码 / 到期 / 恢复用例（通过 GoldenPath 间接使用新 API）
- [x] T4 — 迁移 helpers + suite + smoke（smoke test API 已更新）

# Key decisions
- GoldenPath 去掉 Gateway 字段后所有 MVS 用例自然适配
- 出口 IP / DNS / default-deny 断言语义不变
- 双绑互斥契约保持不变（Phase 54 CTRL-04 已加固）

# Dev notes
- 实际 e2e 执行需要 Linux + Docker + Scenario.Start Step 2..7 实现

# Self-Check: PASSED (structural)
- [x] GoldenPath 无 Gateway 字段
- [x] go build -tags=e2e ./tests/e2e/... 通过
