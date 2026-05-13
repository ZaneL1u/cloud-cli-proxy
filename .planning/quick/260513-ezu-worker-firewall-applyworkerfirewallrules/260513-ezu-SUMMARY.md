---
phase: quick-260513-ezu
plan: 01
subsystem: internal/network
tags: [ci-fix, test, compile-error]
dependency_graph:
  requires: []
  provides:
    - "worker_firewall_linux_test.go 调用签名与 ApplyWorkerFirewallRules 的 4 参数定义一致"
  affects:
    - internal/network/worker_firewall_linux_test.go
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - internal/network/worker_firewall_linux_test.go
decisions:
  - "保持函数签名为 4 参数 (containerNS, gwIP, bridgeGW, sshPort)，因为这与 worker_firewall_linux.go:25 实际定义一致，也与全表中其余 6 处调用形式一致"
metrics:
  duration_minutes: 3
  tasks_completed: 1
  files_changed: 1
  completed_date: 2026-05-13
---

# Quick 260513-ezu Plan 01: Worker Firewall 测试参数修复 Summary

> 删除 worker_firewall_linux_test.go 第 204、371 行调用 ApplyWorkerFirewallRules 时多余的 nil 参数，恢复 Linux 平台 CI 编译。

## 修改概述

CI 在 Linux runner 上报告 `internal/network/worker_firewall_linux_test.go:204:50: too many arguments in call to ApplyWorkerFirewallRules`，因为这两处调用传了 5 个参数，而 `ApplyWorkerFirewallRules` 实际签名只有 4 个参数（`containerNS, gwIP, bridgeGW, sshPort`）。修复方式是删除两处调用末尾多余的 `, nil`。

## 修改的文件与行号

| 文件 | 行号 | 修改类型 |
| ---- | ---- | -------- |
| `internal/network/worker_firewall_linux_test.go` | 204 | 删除末尾 `, nil` |
| `internal/network/worker_firewall_linux_test.go` | 371 | 删除末尾 `, nil` |

## 修复前后对比

**第 204 行：**

```diff
- err := ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22, nil)
+ err := ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22)
```

**第 371 行：**

```diff
- err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort, nil)
+ err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort)
```

## 验证结果

| 验证项 | 命令 | 结果 |
| ------ | ---- | ---- |
| 全部调用为 4 参数 | `grep -n "ApplyWorkerFirewallRules(" internal/network/worker_firewall_linux_test.go` | 8 处调用全部为 4 参数 (126/204/228/274/306/371/413/425) |
| macOS 本地 build | `go build ./...` | 通过（macOS 平台跳过 `_linux.go` 文件） |
| macOS 本地 vet | `go vet ./internal/network/...` | 通过（exit 0） |
| Linux 目标 vet（关键） | `GOOS=linux go vet ./internal/network/...` | **通过（exit 0）**，证明 Linux CI 上 "too many arguments" 错误已消除 |

## Deviations from Plan

无 — 计划按字面执行，仅修改了第 204、371 两行的 `, nil` 部分，未触及其他代码、注释、空白。

## 提交记录

| Commit | 描述 | 范围 |
| ------ | ---- | ---- |
| `73deb3c` | fix(quick-260513-ezu-01): 修正 worker firewall 测试中多余的 nil 参数 | `internal/network/worker_firewall_linux_test.go` |

## Self-Check: PASSED

- [x] `internal/network/worker_firewall_linux_test.go` 第 204 行：调用为 4 参数 (`ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22)`)
- [x] `internal/network/worker_firewall_linux_test.go` 第 371 行：调用为 4 参数 (`ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort)`)
- [x] Commit `73deb3c` 存在于 `git log`
- [x] `GOOS=linux go vet ./internal/network/...` 通过，Linux CI 编译阻塞解除
- [x] 无其他无关改动（diff 仅 2 行新增 / 2 行删除）
