---
phase: 55-e2e
plan: "01"
requirements-completed: [E2E-V4-01, E2E-V4-05]
subsystem: test-infra
tags: [e2e, refactor, single-container, v4.0, scenario-builder]

# Dependency graph
requires:
  - phase: 53-entrypoint (managed-user v4 镜像)
  - phase: 54-control-plane (单容器路径)
provides:
  - Scenario builder 单容器化 API（WithOutboundConfig / WithHost / WithUser）
  - 删除 gatewaySpec/GatewayHandle/WithSingBoxGateway/SingBoxGateway
  - GoldenPath 去掉 Gateway 字段
  - helpers_linux.go: singBoxContainerName / KillSingBox（替代 gatewayDockerName / KillGateway）
affects: [55-02, 55-03, 55-04, 55-05]

# Completed tasks
- [x] T1 — 删除 gatewaySpec + hostSpec.GatewayName
- [x] T2 — 替换 WithSingBoxGateway 为 WithOutboundConfig + userSpec.OutboundConfig
- [x] T3 — 删除 SingBoxGateway 访问器 + GatewayHandle
- [x] T4 — 迁移所有调用点（helpers_linux/smoke/killswitch）
- [x] T5 — 删除 cross-container 协调辅助代码（净删 271 行，目标 ≥ 150）

# Key decisions
- v4.0 API: WithOutboundConfig(outbound) → WithHost(name) → WithUser(name)
- singBoxContainerName() 直接返回 Host.ContainerName（单容器）
- KillSingBox 用 `docker exec <container> kill -9 $(pidof sing-box)` + 3s fail-closed 等待
- BPF filter 简化（无 "not dst gateway" 子句）

# Dev notes
- go build -tags=e2e ./tests/e2e/... 通过
- linux-only 文件（helpers_linux.go/killswitch_stress）在 macOS 不编译，CI 验证

# Self-Check: PASSED
- [x] gatewaySpec/GatewayHandle/WithSingBoxGateway/SingBoxGateway — 0 命中
- [x] go build -tags=e2e ./tests/e2e/... — 通过
- [x] 净删 ≥ 150 行 — 实际 271 行
