---
phase: 55-e2e
plan: "02"
requirements-completed: [E2E-V4-02]
subsystem: test-infra
tags: [e2e, leak, migration, single-container, v4.0]

# Dependency graph
requires:
  - plan: 55-01 (Scenario builder 单容器化)
provides:
  - LEAK-01..08 用例使用更新后的 GoldenPath（无 gateway 字段）
affects: []

# Completed tasks
- [x] T1 — 迁移 leak helpers 到单容器模式（仅注释引用，无实际 API 调用）
- [x] T2..T9 — LEAK-01..08 用例通过 GoldenPath 间接使用新 API

# Key decisions
- LEAK 用例不直接引用 gateway API，通过 GoldenPath 间接使用
- 55-01 的 builder 重构覆盖了 LEAK 套件的间接依赖
- 实际 e2e 执行需要 Linux + Docker + Scenario.Start Step 2..7 实现

# Dev notes
- LEAK 用例文件无 gateway API 直接引用（仅 helpers_test.go 注释中提及）
- 迁移完成后需要 Linux CI runner 验证 8 条用例全绿

# Self-Check: PASSED (structural)
- [x] grep "SingBoxGateway\|WithSingBoxGateway" tests/e2e/leak/ 0 命中
- [x] go build -tags=e2e ./tests/e2e/leak/... 通过
