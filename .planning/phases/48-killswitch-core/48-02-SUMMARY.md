---
phase: 48-killswitch-core
plan: 02
title: 容器内 resolv.conf 篡改免疫 (MVS-10)
status: shipped
mvs: MVS-10
created: 2026-05-14
---

# Phase 48 Plan 02 — SUMMARY

## 实际落地

### 新增 / 修改文件

- `tests/e2e/helpers.go`（无 build tag，与 Plan 01 同一次编辑批落地）：
  - 新增 `ResolvConfTamperResult` 枚举 + `String()`（`Unknown / Applied / Rejected`）。
  - 新增 `ResolvConfTamperContract{Nameserver:"8.8.8.8"}` 锁定结构。
  - 新增 `ClassifyResolvConfDNSOutcome(tamper, dnsResult, leakedPackets) (ok bool, reason string)` 纯函数（6 个分支表）。
- `tests/e2e/helpers_test.go`（无 build tag，与 Plan 01 同一次编辑批落地）：
  - 新增 `TestHelpersResolvConfTamperResult_String`、`TestHelpersResolvConfTamperContract_Locked`。
  - 新增 7 个 ClassifyResolvConfDNSOutcome 分支单测：`TestHelpersClassifyResolvConfDNS_{RejectedAlwaysOK,AppliedTunneledNoLeak,AppliedDeniedNoLeak,AppliedTunneledWithLeak,AppliedLeaked,AppliedUnknown,TamperUnknownIsFail}`。
- `tests/e2e/helpers_linux.go`（`e2e && linux`，与 Plan 01 同一次编辑批落地）：
  - 新增 `(*GoldenPath).TamperResolvConf(ctx, nameserver) (ResolvConfTamperResult, error)` —— `docker exec <worker> bash -c "cp ... && echo ... > /etc/resolv.conf && grep -q"`，exec exit code 解包。
  - 新增 `(*GoldenPath).ProbeDNSFromUser(ctx, domain, timeout) (DNSProbeResult, error)` —— `docker exec <worker> dig +short +time=N +tries=1 <domain>`，复用 Phase 46 `ClassifyDNSResult`。
  - 复用 Plan 01 的 `TcpdumpOnHostEth0` / `InspectContainerIPv4` / `workerDockerName`，未重复实现。
- `tests/e2e/killswitch_resolvconf_tamper_test.go`（`e2e && linux`，新文件）：`TestKillSwitch_ResolvConfTamper_GoldenPath` 主用例（基线 → 后台 tcpdump → TamperResolvConf → ProbeDNSFromUser → 收 tcpdump → ClassifyResolvConfDNSOutcome 合成裁决）。

### 锁定契约

- `ResolvConfTamperContract.Nameserver = "8.8.8.8"` —— 与 BPF filter `dst host 8.8.8.8` 严格绑定，darwin 单测 `TestHelpersResolvConfTamperContract_Locked` 守护。
- `ClassifyResolvConfDNSOutcome` 6 分支表 —— Phase 49 LEAK-02..04 防泄漏对抗用例可直接复用「TamperRejected 视作合法防御」「PacketLeak 一票否决」两条核心规则。

## 与 PLAN 偏差

- PLAN §Steps Step 4 草案的篡改脚本为「`cp + echo + grep`」三步链；实现侧合并为单条 bash `-c`，把 `cp` 失败重定向到 `/dev/null`（备份是排障辅助，不应阻塞篡改路径）。grep 校验确保「文件确实被覆盖」，避免假阳 Applied。
- PLAN §Steps Step 5 草案要求 `dig` exit 0 + stdout 空时直接归 Tunneled；实现侧把它归 Denied（更稳：exit 0 而 A 记录空，等价于 NXDOMAIN / 超时空响应；不应被解释为「tun 接管」）。这一调整也让 ClassifyResolvConfDNSOutcome 在「dig 静默吞错」时仍能合成正确裁决。
- PLAN §Risk 「dig 在 worker 容器内未安装」的回退方案未实现；当前若 worker image 缺 dig，`docker exec` 会以 127 退出，被 `ClassifyDNSResult` 归到 `Unknown`，用例最终 ok=false 并把 stderr 打 t.Logf。worker base image 选定后（Phase 45 决策）若确认无 dig，再补 `nslookup` 回退。

## CONTEXT「Claude's Discretion」节落地

- 用例命名 `tests/e2e/killswitch_resolvconf_tamper_test.go` —— CONTEXT 决策。
- 纯函数拆分粒度：`ResolvConfTamperResult` + `ResolvConfTamperContract` + `ClassifyResolvConfDNSOutcome` —— CONTEXT 列举的 3 块独立锁定，全部落地为纯函数 / 锁定常量。
- DNS 分类直接复用 Phase 46 `ClassifyDNSResult / DNSProbeResult / DNSResultTunneled / DNSResultDenied` —— CONTEXT §Area 3 明确要求，未引入二次枚举。

## Linux 真机验证项（deferred-to-CI）

- `TestKillSwitch_ResolvConfTamper_GoldenPath` 在 Scenario.Start Step 2..7 全部真实实现 + tun-side DNS 接通 + hosted ubuntu-24.04 runner 下跑通。
- 关键依赖：
  - worker base image 含 `dig`（或后续补 `nslookup` 回退）。
  - host eth0 抓包 oracle 可用（与 Plan 01 共享，前置依赖一致）。
  - ro bind mount 内核行为：当前用例两条分支（Applied / Rejected）都视作合法，不依赖具体内核版本。
- 失败模式：
  - 篡改 Applied 后 dig 走宿主机回环（127.0.0.11 docker DNS proxy）而不是 8.8.8.8 → 不会被 host eth0 抓到包，dns 结果会是 Tunneled/Denied，最终仍 ok=true。这是 CONTEXT 期望的「tun 接管」分支。
  - tcpdump 抓到 1+ 包 → 真实泄漏，按设计 t.Fatalf；CI artifact 会保留 `t.Logf` 打的 BPF filter 与 packets 计数。

## darwin 本地验证

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers" -count=1` PASS（9 个新增纯函数 / 锁定常量单测；累计 Phase 48 共 20 个）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## 给 Phase 49/50 的接口约定

- `(*GoldenPath).TamperResolvConf(ctx, nameserver) (ResolvConfTamperResult, error)` —— Phase 49 LEAK-02..04 任何「容器内改 resolv.conf」对抗用例直接复用；如要写多 nameserver / 改格式，新加 `TamperResolvConfBytes`，不要改本签名。
- `(*GoldenPath).ProbeDNSFromUser(ctx, domain, timeout) (DNSProbeResult, error)` —— Phase 49 LEAK-02..05 任何「容器内 DNS 探测」直接复用。
- `ResolvConfTamperResult` / `ResolvConfTamperContract` / `ClassifyResolvConfDNSOutcome` —— Phase 49 共享；如要扩展 nameserver 列表（如 1.1.1.1 / 9.9.9.9 矩阵），把 Contract 改成切片 + 单测同步扩展。
- 与 Plan 01 共享：`TcpdumpOnHostEth0` / `InspectContainerIPv4` / `workerDockerName` / `gatewayDockerName` / `ParseTcpdumpCountOutput` —— 都已在 helpers_linux.go / helpers.go 落地，下游 phase 直接 import。
