---
phase: 46-mvs-ip
plan: 02
subsystem: tests/e2e
tags: [mvs-02, egress-ip, majority-vote]
provides:
  - vote-majority-pure-function
  - fetch-egress-ip-container-helper
  - egress-ip-test-skeleton
requires:
  - tests/e2e/helpers.go (Vote / EgressIPSources，46-01 已落)
  - tests/e2e/helpers_linux.go (FetchEgressIPInContainer，46-01 已落)
affects:
  - tests/e2e/egress_ip_test.go (新增)
tech-stack:
  added: []
  patterns:
    - "三源并行 goroutine + WaitGroup + 单源 5s timeout"
    - "全弃权 → t.Skip 而非 t.Fail（CONTEXT §Area 2 抖动容错）"
key-files:
  created:
    - tests/e2e/egress_ip_test.go
  modified: []
decisions:
  - "Vote / FetchEgressIPInContainer / EgressIPSources 在 46-01 已经一次性落地，本 plan 只新增主用例文件，保持每个 plan commit 边界清晰"
  - "MVS-02 ground truth (绑定的公网 egress IP) 暂列入 t.Log：GatewayHandle.GatewayIP 只是 10.99.x.2 私网地址，公网 NAT IP 需 fixture 接通后再断言，与 Step 2..7 实现联动"
  - "worker 容器句柄通过 workerContainerHandle(gp) 抽象封装，当前阶段返回 nil 让用例 Skip；Linux CI runner 实现后只需补 docker exec wrapper"
metrics:
  duration: 约 10 分钟（用例骨架 + SUMMARY）
  tasks_completed: 3/3
  files_modified: 1
  completed_at: 2026-05-14
requirements_satisfied:
  - MVS-02 (Vote 多数派裁决纯函数完整 + 用例骨架就位)
requirements_partial:
  - MVS-02 真实公网 IP 等值断言（deferred-to-CI）
---

# Phase 46 Plan 02 Summary: 出口 IP 三源轮询 + 多数派裁决

## One-liner

新增 `tests/e2e/egress_ip_test.go` 主用例，复用 46-01 已就位的 `Vote` 纯函数与 `FetchEgressIPInContainer` 容器侧 helper；用例骨架在 worker 容器句柄未暴露 / 全弃权时自动 `t.Skip`，Linux CI runner 解锁后真实跑通。

## 实际产出

| 文件 | 性质 | 关键内容 |
|------|------|----------|
| `tests/e2e/egress_ip_test.go` | 新建 | `//go:build e2e && linux`；EgressIPSuite + TestEgressIP_MajorityVote；3 源并行拉取 + Vote 裁决 |

## 验证结果

- `go build ./tests/e2e/...`（darwin）exit 0 ✓
- `go test ./tests/e2e/ -run "HelpersVote|HelpersEgress" -count=1`（darwin）7 PASS ✓
- `GOOS=linux go vet -tags='e2e linux' ./tests/e2e/...` 干净 ✓

## 与 PLAN 偏差

- 三源固定 URL 与 PLAN 02 完全对齐（`https://ip.me` / `https://ifconfig.io` / `https://ipinfo.io/ip`）。
- ground truth 比对（vote.Winner == bound_egress_ip）未落地：当前 `GatewayHandle.GatewayIP` 是 10.99.x.2 私网地址，公网 NAT IP 需要 fixture 接通后从控制面 admin API 拉。本 plan 把比对降级为 `t.Logf` + Skip on `!vote.OK`，让用例骨架先合入。

## 风险与遗留

- worker 容器句柄当前返回 nil（Step 7 未实现）；CI 上接通后只需补 `workerContainerHandle(gp)` 内部的 docker exec wrapper。
- 三源全弃权时 Skip 不 Fail（CONTEXT 决策），可能掩盖真实泄漏；这是有意为之的容错设计，Phase 49 防泄漏对抗会用强对抗手段补足。

## 给后续 plan 的接口契约

- `Vote` 是公开纯函数，46-03 不复用但 Phase 49 防泄漏判定可参考相同 OR 语义。
- `FetchEgressIPInContainer` 仅 46-02 私用。
