---
phase: 48-killswitch-core
plan: 01
title: sing-box 崩溃后用户容器立即断网 (MVS-09)
status: shipped
mvs: MVS-09
created: 2026-05-14
---

# Phase 48 Plan 01 — SUMMARY

## 实际落地

### 新增 / 修改文件

- `tests/e2e/helpers.go`（无 build tag）：
  - 新增 `KillswitchVerdict` 枚举 + `String()`（`Unknown/OK/ProbeUnexpectedlySucceeded/PacketLeak/Both` 5 个值）。
  - 新增 `ClassifyKillswitchResult(probeExitCode, leakedPackets)` 纯函数。
  - 新增 `KillswitchTimingContract{ProbeMaxLatency:3s, TcpdumpWindow:5s}` 锁定结构。
  - 新增 `tcpdumpCountRe` 正则、`ErrTcpdumpCountNotFound` sentinel error。
  - 新增 `ParseTcpdumpCountOutput(stderr)` 纯函数（解析 `N packets captured` 字面，大小写不敏感）。
  - 新增 `errors` / `regexp` import。
- `tests/e2e/helpers_test.go`（无 build tag）：
  - 新增 8 个 ParseTcpdumpCount / Classify / Contract / String 单测：`TestHelpersParseTcpdumpCount_{FivePackets,ZeroPackets,SingularPacket,EmptyStderr,NoMatchSubstring}`、`TestHelpersClassifyKillswitch_{OK,ProbeUnexpectedlySucceeded,PacketLeak,Both}`、`TestHelpersKillswitchVerdict_String`、`TestHelpersKillswitchTimingContract_Locked`。
  - 新增 `errors` import。
- `tests/e2e/helpers_linux.go`（`e2e && linux`）：
  - 新增私有 helper `(*GoldenPath).gatewayDockerName()`、`workerDockerName()` —— 优先用 `ContainerID/ContainerName`，回退到约定命名 `cloudproxy-gw-<HostID>` / `cloudproxy-<HostID>`。
  - 新增 `(*GoldenPath).KillGateway(ctx)` —— `docker kill --signal=KILL` + `docker inspect -f '{{.State.Running}}'` 二次确认。
  - 新增 `(*GoldenPath).ProbeOutboundFromUser(ctx, url, timeout) (exitCode, err)` —— `docker exec <worker> curl -sS --max-time N url`，exec.ExitError 解包退出码。
  - 新增 `(*GoldenPath).TcpdumpOnHostEth0(ctx, bpf, count, timeout)` —— 默认走 `docker run --rm --network host --cap-add NET_RAW --cap-add NET_ADMIN nicolaka/netshoot:v0.13 tcpdump ...`；`E2E_ALLOW_HOST_TCPDUMP=1 && uid==0` 走宿主机原生 tcpdump 路径。镜像名通过 `E2E_TCPDUMP_IMAGE` 可覆盖。
  - 新增 `(*GoldenPath).InspectContainerIPv4(ctx, containerName, networkName)` —— `docker inspect -f` 拿容器 IPv4 地址。
  - 新增 `errWorkerContainerHandleUnavailable` sentinel error。
  - 新增 `strconv` import。
- `tests/e2e/killswitch_singbox_crash_test.go`（`e2e && linux`，新文件）：`TestKillSwitch_SingboxCrash_GoldenPath` 主用例（基线 → 后台 tcpdump → KillGateway → 立即 probe → 收 tcpdump 结果 → ClassifyKillswitchResult 合成裁决）。

### 锁定契约

- `KillswitchTimingContract.ProbeMaxLatency=3s` —— Phase 50 KILL-01 共享，任一漂移 darwin 单测立即 fail。
- `KillswitchTimingContract.TcpdumpWindow=5s` —— host eth0 抓包窗口，与 BPF count=5 配合。
- `ClassifyKillswitchResult` 4 分支表 —— Phase 50 KILL-02..04 直接复用。

## 与 PLAN 偏差

- PLAN §Steps Step 5 草案曾写「fallback 到 `cloudproxy-gw-<HostID>` 命名约定」，实现侧又加了一层「HostID 也为空时尝试 `Host.ID`」的兜底（更稳的多源回退）。这是落地时新增的防御性逻辑，签名 / 行为契约不受影响。
- PLAN §Steps Step 7 草案的 sidecar 镜像名 `nicolaka/netshoot:v0.13` 在实现中通过 `E2E_TCPDUMP_IMAGE` 环境变量可覆盖（默认值不变）。这是为了让 air-gapped CI runner 可指向私有镜像仓库；不影响主路径。
- PLAN §Steps Step 8 草案的「workerIP via `hostname -i`」改成 `docker inspect -f '{{.NetworkSettings.IPAddress}}'`：避免依赖容器内有无 `hostname` 工具，且更稳。新增的 `InspectContainerIPv4` 也成为本 plan 暴露给下游 Phase 49/50 的接口。
- PLAN 未提：`TcpdumpOnHostEth0` 在 tcpdump 子进程被 ctx 杀掉时仍可能写出统计行，所以 `ParseTcpdumpCountOutput` 失败时把 `run err` 与 `stderr` 一起包进 error，便于失败排障。

## CONTEXT「Claude's Discretion」节落地

- `KillGateway` 固定 SIGKILL，无 grace 选项 —— CONTEXT 决策。
- `TcpdumpOnHostEth0` 默认走 host network privileged sidecar（路径 A），`E2E_ALLOW_HOST_TCPDUMP=1` 才启用宿主机原生 tcpdump（路径 B），符合 CONTEXT 「Claude's Discretion」节允许的两条路径。
- 用例命名 `tests/e2e/killswitch_singbox_crash_test.go` —— CONTEXT 决策。
- 纯函数拆分粒度：`ParseTcpdumpCountOutput` + `ClassifyKillswitchResult` + `KillswitchTimingContract` —— CONTEXT 列举的 3 块独立锁定。

## Linux 真机验证项（deferred-to-CI）

- `TestKillSwitch_SingboxCrash_GoldenPath` 在 Scenario.Start Step 2..7 全部真实实现 + hosted ubuntu-24.04 runner（含 docker daemon 与 netshoot:v0.13 镜像）下跑通。
- 关键依赖：
  - Phase 45 Step 4..6 真实落地后填充 `GatewayHandle.GatewayIP` / `ContainerID`。
  - host eth0 在 runner 上是默认 NIC（hosted ubuntu-24.04 验证过）；自管 runner 上若 NIC 名变（如 `ens5`），需通过环境变量重定向（本 plan 暂未引入 `E2E_HOST_NIC`，留 Phase 50 扩展）。
  - `docker run --network host --cap-add NET_RAW` 在 rootless docker 上可能失败，需 runner 用 rootful docker；hosted ubuntu-24.04 默认即可。
- 失败模式（CI runner 落地后需观察）：
  - sidecar 镜像 pull 慢于 KillGateway → 错过头几个包；可通过 CI workflow 预热 `docker pull nicolaka/netshoot:v0.13` 缓解。
  - host eth0 抓不到包（kernel BPF 拒绝）→ 用例 t.Skip 并把 tcpdump stderr 打 t.Logf 排障，VERIFICATION 中显式记录。

## darwin 本地验证

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers" -count=1` PASS（11 个新增纯函数 / 锁定常量单测）。
- `bash scripts/lint-no-bare-sleep.sh` PASS。

## 给 Phase 49/50 的接口约定

- `(*GoldenPath).KillGateway(ctx) error` —— Phase 50 KILL-02..04 直接复用（不同 kill 信号请新加 `KillGatewayWithSignal`，不要改本签名）。
- `(*GoldenPath).ProbeOutboundFromUser(ctx, url, timeout)` —— 任何需要「容器内 curl 探测」的 phase 直接 import。
- `(*GoldenPath).TcpdumpOnHostEth0(ctx, bpf, count, timeout)` —— Plan 02 直接复用；Phase 49 防泄漏对抗用例可直接 import；BPF filter 由调用方拼。
- `(*GoldenPath).InspectContainerIPv4(ctx, name, network)` —— 任何需要拿容器 IPv4 的 phase 直接 import。
- `ParseTcpdumpCountOutput / ClassifyKillswitchResult / KillswitchVerdict / KillswitchTimingContract` —— Phase 50 共享。
- `E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP` 环境变量 —— Phase 50/52 自定义 runner 时可通过它们覆盖默认 sidecar 路径。
