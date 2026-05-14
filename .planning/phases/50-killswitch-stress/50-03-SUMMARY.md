---
phase: 50-killswitch-stress
plan: 03
title: Pumba netem delay → SSH 存活 + 出口 IP 投票允许 inconclusive (KILL-03)
status: implemented
created: 2026-05-14
---

# Phase 50 Plan 03 SUMMARY — KILL-03 Pumba netem delay

## 落地范围

- `tests/e2e/killswitch_stress/killswitch_03_netem_delay_test.go`：`TestKillSwitch_03_NetemDelay`。
- `tests/e2e/killswitch_stress/helpers_test.go` 追加 `dockerExecHandle` + `newWorkerExecHandle`，让 `e2e.FetchEgressIPInContainer` 在 KILL-03 路径下能消费纯 `docker exec` ContainerHandle（绕过 testcontainers，Scenario Step 7 sentinel 期间仍可走通编译 + Skip 路径）。
- 共享 helper `(*GoldenPath).InjectPumbaNetem` / `ProbeSSHBanner`、`BuildPumbaNetemArgs`、`ParsePumbaOutput`、`PumbaOutcome` 枚举由 `feat(50-shared)` 一笔合入。

## 关键决策

- **Pumba image 锁 `gaiaadm/pumba:0.10.0` + tc image `gaiadocker/iproute2`**：避免 latest 漂移；CI 网络白名单未覆盖时 InjectPumbaNetem 返回 err，用例区分 `image / manifest / pull / daemon` 关键字 → t.Skipf（CONTEXT §备选兜底）。
- **基线锁**：先校验 worker curl exit 0 + SSH banner 拿到 + 出口 IP 投票多数派达成；任一基线缺失 → t.Skipf，不当 Fail 算（避免外网 / runner 抖动假阴）。
- **tc 收敛窗口用 harness.WaitFor 守门**：5s 内反复试 SSH banner 当作 tc 规则就绪信号；不用裸 `time.Sleep`（`lint-no-bare-sleep.sh` PASS）。
- **行为契约**：
  - `SSHAlive == false` → `Fail`（控制流必须存活）。
  - `Vote.OK && Winner != expected` → `Fail`（不允许给错误的出口 IP）。
  - 全弃权 → `Inconclusive`（contract `AllowInconclusive=true` 命中，t.Skipf 而非 t.Fatalf）。
- **cleanup**：`defer cleanup()` 触发 `docker kill <pumba-sidecar>` + `cmd.Wait`，避免 Pumba `--duration 30s` 拖延总耗时；Pumba 自身在 SIGTERM 时清理 tc 规则。

## darwin 闸

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Killswitch" -count=1` PASS（含 `BuildPumbaNetemArgs` × 5 + `ParsePumbaOutput` × 5 + KILL-03 classify 分支 × 3）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## Linux runner 真机验收（deferred-to-CI）

VERIFICATION.md 列 human_verification（含 `gaiaadm/pumba:0.10.0` 镜像 pull 前置）。
