---
phase: 51-qual-harden
plan: 51-02
status: completed
completed_at: 2026-05-14
---

# 51-02 SUMMARY — `verifyLeakBlocked` 多目标参数化

## 落地清单

- `internal/network/verify.go`：
  - 新增 `leakTarget{Host, Port}` 私有类型 + `String()` helper。
  - 新增 `defaultLeakTargets` 常量切片（4 个 target，与 Phase 46
    `DefaultDenyMatrix` 一一对齐）：1.1.1.1:80 / 8.8.8.8:443 / 9.9.9.9:443 /
    169.254.169.254:80。
  - 新增 `verifyLeakBlockedMulti(ctx, prefix, targets, *result)` 并发探测。
  - `verifyLeakBlocked` 旧签名保留，内部委托 `Multi(defaultLeakTargets)`，
    `LeakTarget` 默认值 "1.1.1.1:80" 保持（兼容
    `TestVerifyNetworkIntegrity_LeakTargetSet`）。
- `internal/network/verify_test.go`：新增 4 单测
  - `TestDefaultLeakTargets_LockedContract`（4 行锁定）
  - `TestVerifyLeakBlockedMulti_AllBlocked` / `_OneLeaked` / `_AllLeaked` /
    `_EmptyTargets`

## 验证

- `go build ./...` + `GOOS=linux go build ./...` PASS。
- `go test ./internal/network/... -count=1` PASS。
- `go test ./... -count=1` 全绿。
- 既有单测零修改通过。

## 偏差

- 与 51-01 类似，本地复刻 `DefaultDenyMatrix` 列表，因为 tests/e2e 包不可
  import 至 production 代码。锁定单测保证两边契约不漂移。
