---
phase: 55-e2e
plan: "05"
subsystem: test-infra
tags: [e2e, security, assertions, single-container, v4.0]

# Dependency graph
requires:
  - plan: 55-01 (Scenario builder 单容器化 + GoldenPath)
provides:
  - tests/e2e/security_assertions_test.go — SEC-01..03 三条新 e2e 用例
affects: []

# Completed tasks
- [x] T1 — SEC-01: 用户不能 kill sing-box（docker exec -u clouduser kill → Operation not permitted + 验证进程仍存）
- [x] T2 — SEC-02: 用户不能读 config（docker exec -u clouduser cat /etc/sing-box/config.json → Permission denied / No such file）
- [x] T3 — SEC-03: 用户 cap 集合为空（getpcaps + ip link down 必须失败 + unshare -n 必须失败）

# Key decisions
- SEC-01 验证路径：非 root kill → 失败 → sing-box pid 仍存在
- SEC-02 双重保证：文件权限 0600 + 启动后 rm（entrypoint remove_singbox_config）
- SEC-03 三条子断言：getpcaps 空、ip link 失败、unshare 失败

# Dev notes
- 实际 e2e 执行需要 Linux + Docker + Scenario.Start Step 2..7 实现
- go build -tags=e2e ./tests/e2e/... 通过

# Self-Check: PASSED (structural)
- [x] security_assertions_test.go 创建
- [x] go build -tags=e2e ./tests/e2e/... 通过
