---
phase: 49-leak-defense
plan: 08
title: LEAK-08 capability 审计 (SUMMARY)
status: implemented_with_gap
leak: LEAK-08
build_tag: "e2e && linux"
created: 2026-05-14
gap: phase-51-qual-06
---

# Phase 49 Plan 08 SUMMARY: LEAK-08 capability 审计

## 实际实现

- **oracle 命令**：`*GoldenPath.GetProcCapabilities(ctx, 1)` —— `docker exec
  <worker> cat /proc/1/status`，返回完整 stdout。
- **解析**：`ParseProcCapabilities` 用正则 `(?m)^Cap(Inh|Prm|Eff|Bnd):\s+([0-9a-fA-F]+)\s*$`
  抽 4 行 hex 掩码，按 `KnownCapabilityBits`（≥10 个 cap，含 NET_RAW=13 /
  NET_ADMIN=12 / SYS_ADMIN=21）展开为 `map[capName]bool`。
- **裁决**：`caps.Eff[<dangerous>] || caps.Bnd[<dangerous>]` 任一命中
  `LeakDangerousCaps = {NET_RAW, NET_ADMIN, SYS_ADMIN}` → **t.Errorf**（不阻塞）。

## 与 Plan 偏差

无；纯函数单测覆盖 5 fixture（clean / dirty / partial / corrupt / 行数不全）+
3 显式位断言（NET_RAW only / NET_ADMIN only / SYS_ADMIN only），共 11+ 单测分支。

## 实际命令 / 工具

- `docker exec <worker> cat /proc/1/status`。
- 不依赖 `getpcaps`（避免 worker 镜像装 libcap-bin 依赖）。

## 单测覆盖（darwin）

- `KnownCapabilityBits_LocksCriticalSubset`：锁 NET_RAW=13 / NET_ADMIN=12 /
  SYS_ADMIN=21 三位 + ≥10 总数。
- `ParseProcCapabilities_*` × 7（Clean / Dirty / Partial / Corrupt / Missing /
  TabSeparator / AllZeros）。
- `ExpandCapBits_NetRawBit` / `_NetAdminBitOnly` / `_SysAdminBitOnly`。
- fixture：`proc_status_clean.txt`（hex 0xa80405fb，无危险 cap）/
  `proc_status_dirty.txt`（hex 0xa83435fb，含 NET_RAW + NET_ADMIN + SYS_ADMIN）/
  `proc_status_partial.txt`（CapBnd 全 1）/ `proc_status_corrupt.txt`（hex 错）。

## Phase 51 GAP（必须修）

**预期 fail**：grep `internal/runtime/tasks/worker.go:217-218` 当前显式
`--cap-add NET_ADMIN --cap-add SYS_ADMIN`，且未显式 `--cap-drop NET_RAW`，
docker 默认 capability 集合**包含** `cap_net_raw`，因此 worker 进程 1 的
`CapEff` / `CapBnd` 三条危险 cap 全命中。

**Phase 51 QUAL-06 修复方案**：见 49-06-SUMMARY。LEAK-08 与 LEAK-06 是同一根因：
worker.go 缺 `--cap-drop NET_RAW` + 多余 `--cap-add SYS_ADMIN`；修复后两条
LEAK 用例同时转 PASS。
