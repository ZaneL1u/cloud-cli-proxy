---
phase: 55-e2e
plan: "03"
subsystem: test-infra
tags: [e2e, killswitch, migration, single-container, v4.0]

# Dependency graph
requires:
  - plan: 55-01 (Scenario builder 单容器化 + helpers 更新)
provides:
  - KILL 核心用例 + 压力用例迁移到 v4.0 单容器 API
affects: []

# Completed tasks
- [x] T1 — 迁移 KILL 核心用例（killswitch_singbox_crash_test.go: docker kill gw → docker exec kill sing-box + fail-closed）
- [x] T2 — 迁移 resolv.conf 篡改用例（通过 55-01 helpers 更新间接覆盖）
- [x] T3 — 迁移 KILL 压力用例 01..04（killswitch_stress: gatewayInspectName → workerInspectName, KillGateway → KillSingBox, BPF 简化）

# Key decisions
- KillSingBox: docker exec kill sing-box → 3s fail-closed 等待
- KILL-02 (tun0 down): 语义改为验证 user 无 NET_ADMIN 不能关 tun0
- KILL-03 (netem): 注入目标改为 user 容器名
- KILL-04 (disconnect): v4.0 无 cloudproxy-net-* bridge，用例标记为 backend gap

# Dev notes
- 实际 e2e 执行需要 Linux + Docker + Scenario.Start Step 2..7 实现
- 无法在 macOS 编译 linux-tagged 文件，CI 验证

# Self-Check: PASSED (structural)
- [x] killswitch 文件 gateway API 引用已清除
- [x] go build -tags=e2e ./tests/e2e/... 通过
