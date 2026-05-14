---
phase: 51-qual-harden
title: Phase 51 代码层质量加固 VERIFICATION
status: passed
created: 2026-05-14
verified_at: 2026-05-14
darwin_gates: passed
linux_runner: deferred-to-ci
gap_closures:
  - phase-47-d-47-3
  - phase-49-gap-1
  - phase-49-gap-2
human_verification:
  - operator: TBD
  - linux_runner: ubuntu-24.04 hosted
  - performed_at: TBD
---

# Phase 51 代码层质量加固 VERIFICATION

## 验证结论

**status: passed**

8 条 QUAL 不变量 + 1 条 backend GAP 收口（共 9 plan）全部在 darwin 跨编译 + 单测层面落地通过；Phase 47/49 的 3 条 backend GAP 同步闭合。Linux runner 真机 e2e 列 `human_verification: deferred-to-CI`（与 Phase 47/49/50 节奏一致）。

darwin 本地闸：

| 闸 | 命令 | 结果 |
|----|------|------|
| 1 | `go build ./...` | PASS |
| 2 | `GOOS=linux go build ./...` | PASS |
| 3 | `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` | PASS |
| 4 | `go vet ./...` | PASS |
| 5 | `go test ./... -count=1`（19 包，含 e2e 包 darwin 路径） | PASS |
| 6 | `bash scripts/lint-no-bare-sleep.sh`（既有 e2e 守护） | PASS（本 phase 未触 e2e） |

## 9 plan 覆盖证据矩阵

| Plan | 不变量 | 落地文件 | 验证证据 | Commit |
|------|--------|----------|----------|--------|
| 51-01 | QUAL-01 verifyEgressIP 多源轮询 | `internal/network/verify.go` | `TestVoteEgressIP_*` × 5 + `TestVerifyEgressIPMulti_*` × 3 @ `internal/network/verify_test.go` | 2d51db3 / 290e5b5 |
| 51-02 | QUAL-02 verifyLeakBlocked 多目标 | `internal/network/verify.go` | `TestDefaultLeakTargets_LockedContract` + `TestVerifyLeakBlockedMulti_*` × 4 | c5d809b / 6ea8862 |
| 51-03 | QUAL-03 verifyDNS 遍历全 nameserver | `internal/network/verify.go` | `TestParseAllNameservers_*` × 5 + `TestVerifyDNS_ReportsAllNameservers` | dccebd3 / 974659a |
| 51-04 | QUAL-04 GetContainerNetNS 探测窗口 | `internal/network/namespace.go` | `Option` / `WithProbeWindow` / `WithMaxRetries` 接入；既有 e2e 跨编译 PASS | e146b3f |
| 51-05 | QUAL-05 nft counter + 169.254/16 显式 drop | `internal/network/firewall_helpers.go`、`worker_firewall_linux.go` | `TestBuildIPDaddrCIDRDropExprs_LinkLocal` + `TestBuildIPDaddrCIDRDropExprs_RejectsIPv6` + `TestBuildIPDaddrCIDRDropExprs_RejectsGarbage`；既有 `TestBuildLogDropExprs / TestBuildOifUDPDportDropExprs` 零回归 | 0f5228e |
| 51-06 | QUAL-06 worker cap-drop NET_RAW + 删 SYS_ADMIN（NET_ADMIN 保留） | `internal/runtime/tasks/worker.go` | `TestBuildCreateArgs_CapabilitiesLocked` @ `worker_caps_test.go` | c84c86d / 03ace12 |
| 51-07 | QUAL-07 `-race -shuffle=on -count=1` 默认 | `Makefile` / `.github/workflows/ci.yml` | 默认 test target 命令更新；CI 主 job 加 `-race -shuffle=on` | 92b5e56 |
| 51-08 | QUAL-08 goleak.VerifyTestMain | `cmd/cloud-claude/testmain_test.go`（含 control-plane / host-agent 同型注入）；`go.mod` 新增 `go.uber.org/goleak` | `go test ./cmd/cloud-claude/... -count=1` 0 goroutine leak | 1bb7831 |
| 51-09 | 双绑 API pre-check + error code | `internal/controlplane/http/admin_bindings.go`、`internal/store/repository/queries.go` | `TestAdminBindings_DoubleBind_ErrorCode` + `Bind double-bind … 409 with error_code` + `Bind same host re-bind 201 idempotent` 单测三连；既有 8 个 case 零回归 | c290810 / e59dc5a |

## 自动闭环的 backend GAP

| 来源 | GAP | 收口 plan | 闭环证据 |
|------|-----|----------|----------|
| Phase 47 D-47-3 | `host_egress_bindings` 缺 pre-check + 无 4xx error code | **51-09** | admin Bind 响应已带 `error_code = "egress_ip_already_bound"` + 中文 + 英文 `"already bound"` 子串；Phase 47 `EgressIPDoubleBindContract{WantStatus:409, WantErrSubstring:"already bound"}` 满足；`ParseBindEgressIPResponse` 解析 `ErrorMessage / ErrorCode` 双字段命中。Linux runner `TestEgressIPBinding_DoubleBindExcluded` 预期由 BACKEND GAP → PASS 分支。 |
| Phase 49 GAP-1 | worker `--cap-add NET_ADMIN/SYS_ADMIN`，docker 默认含 NET_RAW | **51-06** | worker 启动参数：删 `--cap-add SYS_ADMIN`；保留 `--cap-add NET_ADMIN`（sing-box tun 设备依赖，CONTEXT §Area 4 折中决策）；新增 `--cap-drop NET_RAW`。Linux runner LEAK-06（SOCK_RAW PermissionDenied）预期由 fail → PASS；LEAK-08 fixture `proc_status_clean.txt` 需 Phase 49 二次工作放宽 NET_ADMIN 期望（已在 51-06 SUMMARY 明确披露）。 |
| Phase 49 GAP-2 | 缺 IPv4 `169.254.0.0/16` 显式 drop nft 规则 | **51-05** | `addIPDaddrCIDRDropRule(conn, table, outputChain, "169.254.0.0/16", "linklocal-drop")` 插入 OUTPUT 链 lo / ESTABLISHED 之后、所有 accept 之前；nft list ruleset 输出含 comment `"linklocal-drop"` 行 + counter。Linux runner LEAK-07 `HasLinkLocalDropRule` 契约预期 fail → PASS。 |

## 与 ROADMAP 偏差

- ROADMAP §Phase 51 写 **8 plan**；CONTEXT §Phase Boundary 扩展为 **8 + 1**（新增 51-09 双绑 API），实际落地 9 plan，与 CONTEXT 一致。
- ROADMAP §Phase 51 §Plans 写 `51-06 — worker 容器启动参数加 --cap-drop=NET_RAW --cap-drop=NET_ADMIN`。实际落地按 CONTEXT §Area 4 折中决策**保留 NET_ADMIN**（sing-box 在 worker netns 创建 tun0 设备的硬依赖）；51-06 SUMMARY 已明确披露。Phase 49 LEAK-08 fixture 需相应放宽 NET_ADMIN 期望，属 Phase 49 后续二次工作（本 phase 锁定不动 `tests/e2e/`）。
- ROADMAP §Phase 51 §Plans 暂未列 51-09；本 VERIFICATION 落定后建议 ROADMAP 同步把 51-09 加入 Plans 列表（属 ROADMAP 维护，不在本 phase commit 范围）。

## human_verification_needed（deferred-to-CI）

以下 4 项必须在 Linux runner（含 docker + 真实 worker 容器拓扑 + sing-box + nft）跑通：

1. **LEAK-06 `TestLeak_06_RawSocket_PermissionDenied`**：worker 容器内 `python3 -c 'import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, 1)'` 必须 `PermissionError`（NET_RAW 已 drop）。
2. **LEAK-07 `TestLeak_07_NftLinkLocalDrop_RuleExists`**：`nft list ruleset` 输出必含一行 `ip daddr 169.254.0.0/16 ... counter ... drop` 且 comment `"linklocal-drop"`；`HasLinkLocalDropRule(rules) == true`。
3. **LEAK-08 `TestLeak_08_WorkerCapabilities_Locked`**：worker `/proc/1/status` `CapEff` / `CapBnd` 不含 `NET_RAW` / `SYS_ADMIN`；`NET_ADMIN` 按本 phase 折中**保留**，fixture 需同步调整。
4. **MVS-07 `TestEgressIPBinding_DoubleBindExcluded`**：第二次绑定不同 host 必须 409 + `error_code = "egress_ip_already_bound"` + message 含 `"already bound"`；原绑定不破坏 + 第二绑定不被意外写入。

## 整组耗时

- darwin：`go test ./... -count=1` 实测 ≈ 75 s（含 `internal/cloudclaude` 46 s 长 case）；`internal/network` ≈ 1.2 s；`internal/runtime/tasks` ≈ 1.1 s；`internal/controlplane/http` ≈ 2.4 s。
- Linux runner CI 预期：QUAL-01..03 / QUAL-04 / 51-09 全在 unit 层，加进 `-race -shuffle=on` 后整组单测 < 3 min；e2e LEAK-06/07/08 + MVS-07 沿用 Phase 49 / 47 既有 ≤ 5 min 预算。

## 依赖增量

- 仅新增 `go.uber.org/goleak`（CONTEXT 唯一允许）；`go.mod` / `go.sum` 一并 commit。
- 未引入 sing-box / pgx / nftables 等核心库版本变化。

## 整组 commit 列表

```
dda44c2 docs(51): 拆出 Phase 51 九个 PLAN.md (QUAL-01..08 + 双绑 API)
e146b3f feat(51-04): GetContainerNetNS 探测窗口参数化
92b5e56 feat(51-07): go test 默认 -race -shuffle=on
1bb7831 feat(51-08): goleak.VerifyTestMain 接入
2d51db3 feat(51-01): verifyEgressIP 多源轮询 + 多数派投票
290e5b5 feat(51-01): verifyEgressIP 多源轮询 + 多数派投票（test 补强 + SUMMARY）
c5d809b feat(51-02): verifyLeakBlocked 多目标矩阵化
6ea8862 feat(51-02): verifyLeakBlocked 多目标参数化（SUMMARY 落定）
dccebd3 feat(51-03): verifyDNS 遍历全部 nameserver
974659a feat(51-03): verifyDNS 遍历全部 nameserver 行（test 补强 + SUMMARY）
0f5228e feat(51-05): nft 全规则加 counter + 169.254/16 显式 drop
c84c86d feat(51-06): worker cap-drop NET_RAW + 删 SYS_ADMIN
03ace12 feat(51-06): worker cap-drop NET_RAW + 删 SYS_ADMIN（SUMMARY 落定）
c290810 feat(51-09): 双绑互斥 API pre-check + 稳定 error code
e59dc5a feat(51-09): 双绑互斥 API pre-check + 稳定 error code（test 补强 + SUMMARY）
```

（VERIFICATION 自身提交在本文件落定后追加。）

> 备注：51-01/02/03/06/09 出现 2 个 commit，原因是 worktree 中已有先行落地的代码与新做的 test 补强 + SUMMARY 分别成 commit；最终代码状态与单一 commit 等价，回滚仍可按 plan 粒度操作。
