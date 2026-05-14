---
phase: 51-qual-harden
plan: 51-01
title: QUAL-01 verifyEgressIP 多源轮询
status: completed
completed: 2026-05-14
---

# 51-01 QUAL-01 — `verifyEgressIP` 多源轮询 — SUMMARY

## 变更

- `internal/network/verify.go`：
  - 新增包级 `egressIPSources`（3 源：`ip.me / ifconfig.io / ipinfo.io/ip`）。
  - 新增 `verifyEgressIPMulti(ctx, prefix, expectedIP, sources, result)`，3 源并发 curl、每源 8s 子 timeout、汇总后调用 `voteEgressIP`。
  - 新增 `voteEgressIP(results []string) (winner, ok)`：production 端等价 `tests/e2e/helpers.go::Vote`（不能 import tests/e2e）。语义：≥2 一致 = winner；单元素结果也接受；tie / 全空 / 全 fail → `ok=false`。
  - `verifyEgressIP` 旧签名保留，内部以单源 `https://ip.me` 调用 `verifyEgressIPMulti`，对外契约零破坏。
  - `VerifyNetworkIntegrity` 切到 `verifyEgressIPMulti(ctx, prefix, expected.ExpectedIP, egressIPSources, &result)`。
- `internal/network/verify_test.go`：
  - 新增 5 个 `TestVoteEgressIP_*`：3-of-3 / 2-of-3 / 1-1-1 tie / 全空 / nil。
  - 新增 3 个 `TestVerifyEgressIPMulti_*`：多数派 match / 多数派 mismatch / 全 timeout（用既有 `withFakeNsenterRunner` + 按 url 关键字分支返回）。

## 闸

- `go build ./...` PASS。
- `GOOS=linux go build ./...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go vet ./...` PASS。
- `go test ./... -count=1`：全 PASS（19 个包，含 `internal/network 1.163s`）。

## 偏差

- CONTEXT §Area 1 写「直接复用」Phase 46 `Vote`，落地为 production 端复刻同语义函数 `voteEgressIP`（原因：production 代码不能 import `tests/e2e` 包）。语义与 tests/e2e Vote 完全等价。
- CONTEXT §Specific Ideas 写新增 `VerifyResult.VoteResult` 字段；落地为不破坏既有 `VerifyResult` 结构（沿用 `ActualEgressIP` 写多数派 winner，`EgressIPMatch` 写比对结果），既有 e2e / json 字段契约零破坏。

## 风险闭环

- 既有 4 个 `TestVerifyNetworkIntegrity_*`：fake 桩返回 err / 空响应时，全 source 失败 → `voteEgressIP` 返回 `ok=false` → 与旧路径行为等价（`EgressIPMatch=false`、`ActualEgressIP=""`），全 PASS。
- 行为契约：`ActualEgressIP` / `EgressIPMatch` 字段语义不变；`firstNetworkError` 优先级不动。
