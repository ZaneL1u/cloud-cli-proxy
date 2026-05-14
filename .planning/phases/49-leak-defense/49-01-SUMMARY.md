---
phase: 49-leak-defense
plan: 01
title: LEAK-01 DNS 明文 UDP/53 旁路检测 (SUMMARY)
status: implemented
leak: LEAK-01
build_tag: "e2e && linux"
created: 2026-05-14
---

# Phase 49 Plan 01 SUMMARY: LEAK-01 DNS 明文 UDP/53 旁路检测

## 实际实现

- **探测方法**：`*GoldenPath.DigPlainDNS(ctx, "8.8.8.8", "example.com")`，
  容器内执行 `dig +short +time=3 +tries=1 @8.8.8.8 example.com`，把 exit code +
  stderr 关键字（`timed out` / `servfail` / `connection refused` /
  `no servers could be reached`）映射为 `LeakProbeResult{Blocked, Reason}`。
- **独立 oracle**：`*GoldenPath.TcpdumpOnHostEth0(ctx, bpf, 5, 5s)`（复用
  Phase 48）；BPF：`udp port 53 and dst host 8.8.8.8 and src host <workerIP>`。
- **裁决**：`tdRes.packets > 0 || !digRes.Blocked` → t.Fatalf。严格不接受
  「dig 返回 A 记录但抓包 0」（区分 Phase 46 MVS-03 OR 语义）。
- **共享 fixture**：`leak/suite_test.go::StartLeakGolden` 包装 `e2e.StartGoldenPath`，
  Step 2..7 sentinel → t.Skip。

## 与 Plan 偏差

| Plan | 实际 | 原因 |
|------|------|------|
| 计划用 suite 共享 fixture（`*sync.Once`） | 改为每用例独立 `StartLeakGolden` | StartGoldenPath 内部用 t.Cleanup / t.Skipf 强绑 \*testing.T，跨用例共享需要重构 e2e.StartGoldenPath；Step 2..7 sentinel 期间 8 用例都 t.Skip，复用没意义。Phase 52 OBS-* 再考虑 fixture pool。 |
| 计划在 PLAN 中提到 nft counter 校验作为辅助 | 本 plan 实际不查 counter（counter 校验落在 Plan 05/07） | 简化职责边界：LEAK-01 只断言「明文 53 不可达 + 抓包 0」。 |

## 实际命令 / 工具

- `dig +short +time=3 +tries=1 @8.8.8.8 example.com`（worker 镜像若无 dig，
  `EnsureWorkerLeakTools` 在 SetupSuite 调 `apt-get install -y dnsutils` 兜底）。
- `tcpdump -nn -i eth0 -c 5 'udp port 53 and dst host 8.8.8.8 and src host <ip>'`
  通过 `nicolaka/netshoot:v0.13` host-network sidecar；或 host-native 路径
  （`E2E_ALLOW_HOST_TCPDUMP=1`）。

## 单测覆盖（darwin）

复用 shared commit 的 `ClassifyLeakProbe` × 5 + `LeakVerdict_String` + `LeakDangerousCaps_Locked`。
fixture：`testdata/leak/dig_timeout.txt` / `dig_servfail.txt` / `dig_ok.txt`。

## Phase 51 GAP

无（本 plan 期望 PASS；若 Linux runner 真实跑 fail，需要先排查 sing-box 路由配置是否真的接管 worker namespace 53 出口）。
