---
phase: 50-killswitch-stress
title: Phase 50 Kill-switch 压力测试 VERIFICATION
status: passed
created: 2026-05-14
darwin_gates: passed
linux_runner: deferred-to-ci
kill_covered: [KILL-01, KILL-02, KILL-03, KILL-04]
human_verification:
  - operator: TBD
  - linux_runner: ubuntu-24.04 hosted
  - performed_at: TBD
  - sidecar_images:
      tcpdump: nicolaka/netshoot:v0.13
      pumba: gaiaadm/pumba:0.10.0
      pumba_tc: gaiadocker/iproute2
---

# Phase 50 Kill-switch 压力测试 VERIFICATION

## 总体结论

**status: passed**（darwin 闸维度）

darwin 上 4 plan 全部 implementer 验证通过：

- `go build ./tests/e2e/...` PASS。
- `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` PASS。
- `go test ./tests/e2e/ -run "Helpers|Killswitch" -count=1`：**121 PASS**（含本 phase 新增 29 个纯函数单测；累计 92 + 29 = 121）。
- `bash scripts/lint-no-bare-sleep.sh` PASS（用例与 helper 内无任何 `time.Sleep`，KILL-03 收敛窗口走 `harness.WaitFor`）。

Linux runner 真机 e2e（4 个 `TestKillSwitch_NN_*` + Phase 45 Step 2..7 真实落地 + Pumba sidecar + host eth0 tcpdump）→ 列 `human_verification: deferred-to-CI`，待 ubuntu-24.04 hosted runner 真实跑通。

**潜在 backend 流转 Phase 51**：KILL-04 用例已内置兜底分支，若 gateway 未接 cloudproxy-net-* 自定义 bridge（如 backend 改用 macvlan / host network），用例 t.Skipf 并把现状落到 VERIFICATION 备注 + ROADMAP `Phase 51` 备注列。

## 每条 KILL 覆盖证据

| KILL | e2e 用例 | 主断言 | 单测覆盖 | fixture | 预期 |
|------|----------|--------|----------|---------|------|
| KILL-01 | `TestKillSwitch_01_SigkillTiming` | `docker kill --signal=KILL <gw>` → ≤ 3000ms 内 worker curl 非 0 + host eth0 抓包 0 包 | `ClassifyStressResult KILL-01_{Pass,FailOnProbeSucceeded,FailOnLeakPackets,FailOnLatency}` × 4 + `KillswitchStressContract_Locked` | 复用 Phase 48 tcpdump 计数 fixture（行为路径相同） | PASS（darwin 单测） |
| KILL-02 | `TestKillSwitch_02_TunDevDown` | `docker exec gw ip link set tun0 down` → ≤ 3000ms 断网 + 0 包 + cleanup 内 `tun0 up` 恢复 | 复用 `ClassifyStressResult KILL-02_Pass` + contract 锁定 | — | PASS（darwin 单测） |
| KILL-03 | `TestKillSwitch_03_NetemDelay` | Pumba `netem delay 1000ms --duration 30s` → SSH banner 必须存活 + 出口 IP 投票 ≥2 源同 winner == expected 或全弃权（contract `AllowInconclusive=true`） | `ClassifyStressResult KILL-03_{PassWithVote,FailOnSSHDead,FailOnWrongIP,InconclusiveOnAbstain}` × 4 + `BuildPumbaNetemArgs` × 5 + `ParsePumbaOutput` × 5 + `PumbaOutcome_String` | `tests/e2e/testdata/killswitch/pumba_applied.txt` / `pumba_image_missing.txt` / `pumba_daemon_down.txt` / `pumba_failed.txt` | PASS（darwin 单测） |
| KILL-04 | `TestKillSwitch_04_NetworkDisconnect` | `docker network disconnect <cloudproxy-net-X> <gw>` → ≤ 3000ms 断网 + 0 包 + cleanup `network connect --ip <savedIP>` 恢复 | `ClassifyStressResult KILL-04_{Pass,FailOnPacketLeak}` × 2 + `PickGatewayBridgeNetwork` × 5 | — | PASS（darwin 单测） |

## 新增统计

- **GoldenPath 探测方法（`tests/e2e/helpers_linux.go`）**：6 个
  - `SetTunDevDown(ctx) error`
  - `SetTunDevUp(ctx) error`
  - `DisconnectGatewayFromBridge(ctx) (savedNet, savedIP string, err error)`
  - `ReconnectGatewayToBridge(ctx, netName, staticIP string) error`
  - `InjectPumbaNetem(ctx, target string, params PumbaNetemParams) (cleanup func(), err error)`
  - `ProbeSSHBanner(ctx, timeout) error`
- **shared 纯函数 / 类型 / 锁定表（`tests/e2e/helpers.go`）**：
  - `StressVerdict` + `String()` + 4 个枚举值
  - `StressEvidence` 聚合结构
  - `KillswitchStressContract` 锁定表（KILL-01..04 各自 timing / 行为契约）
  - `ClassifyStressResult(kill, evidence) (StressVerdict, string)` 纯函数
  - `PumbaNetemParams` + `BuildPumbaNetemArgs(target, params) []string`
  - `PumbaOutcome` + `String()` + `ParsePumbaOutput(stdout, stderr) PumbaOutcome`
  - `PickGatewayBridgeNetwork(raw string) (net, ip string)`
- **darwin 新增单测（`tests/e2e/helpers_test.go`）**：**29 个**全绿，覆盖：
  - `StressVerdict_String` × 1
  - `KillswitchStressContract_Locked` × 1（4 条 KILL 整体锁定）
  - `ClassifyStressResult` 行为分支 × 12（unknown / KILL-01 4 分支 / KILL-02 1 分支 / KILL-04 1 分支 / KILL-03 4 分支 + 1 wrong-IP 边角）
  - `BuildPumbaNetemArgs` × 5（defaults / custom-image / empty-target / loss / unknown-mode）
  - `ParsePumbaOutput` × 5（Applied / ImageMissing / DaemonDown / Failed / Unknown）
  - `PumbaOutcome_String` × 1
  - `PickGatewayBridgeNetwork` × 5（cloudproxy-preferred / only-bridge / custom-fallback / empty / multi-cloudproxy）
- **e2e 用例（`tests/e2e/killswitch_stress/`）**：4 个 `TestKillSwitch_NN_*`（darwin 上 t.Skip，Linux runner 真实跑）。
- **fixture（`tests/e2e/testdata/killswitch/`）**：4 份 Pumba stdout / stderr 样本。

总测试数（darwin）：**121 PASS**（既有 92 + 本 phase 新增 29）。

## ROADMAP / CONTEXT 偏差汇总

| ID | ROADMAP / CONTEXT 草案 | 源码 / 实现真相 | 处置位置 |
|----|-----------------------|----------------|----------|
| D-50-1 | CONTEXT §Area 3 写「`docker network disconnect <bridge-net> <gateway>` 把 gateway 从 docker bridge 摘走」 | grep 源码确认 gateway 实际接的是专属自定义 bridge `cloudproxy-net-<HostID>`（`internal/network/container_proxy_provider.go:323 networkName`），**不是** docker 默认 `bridge`；worker 同时挂 default `bridge` + `cloudproxy-net-*` 两网。KILL-04 disconnect 的是前者，验证 worker 不允许 fallback 到 default bridge 直连。 | `PickGatewayBridgeNetwork` 纯函数按 `cloudproxy-net-` 前缀优先匹配；用例兜底分支识别 `has no cloudproxy-net` → t.Skipf 流转 Phase 51。详见 50-04-SUMMARY.md。 |
| D-50-2 | CONTEXT §Area 1 写「KILL-02 `ip link set tun0 down`」 | grep 源码确认 sing-box auto_route 在 gateway 容器内创建 `tun0` 设备（`internal/network/singbox_provider_linux.go:165 / waitForTun0`）；worker netns 内 nft 用的 `sb-tun0` 是接口名标识，与 KILL-02 关掉的设备不同。 | `SetTunDevDown` 锁 `tun0` 设备名；SUMMARY / PLAN 明确两者语义区别。 |
| D-50-3 | CONTEXT §Specifics 写 Pumba `--duration 30s delay --time 1000` | 实际拼成完整 `docker run --rm gaiaadm/pumba:0.10.0 netem --duration 30s --tc-image gaiadocker/iproute2 delay --time 1000 <target>`；`--tc-image` 必须显式传，否则 Pumba 默认用 `gaiadocker/iproute2` 也行但 0.10.0 部分构建产物路径漂移。 | `BuildPumbaNetemArgs` 默认值锁 `gaiaadm/pumba:0.10.0` + `gaiadocker/iproute2`，固定 tag 避免 latest 漂移。 |
| D-50-4 | CONTEXT §Specifics 写「KILL-03 SSH 存活探测用 nc -z」 | 实现选 `bash -c 'exec 3<>/dev/tcp/localhost/22 && head -c 6 <&3 \| grep -q "^SSH-"'`：不依赖 worker 镜像内有无 netcat（bash 内建 `/dev/tcp` 在 debian-slim / ubuntu 默认可用），且能拿到 banner 前缀做更强语义校验。 | `ProbeSSHBanner` 实现层；详见 50-03-SUMMARY.md。 |
| D-50-5 | CONTEXT §Area 4 写「KILL 用例可串行可并行」 | 落地串行：4 个用例独立 `StartGoldenPath`，每用例独立 GoldenPath（与 Phase 49 LEAK 套件一致），故障注入互相隔离；并行属未来 phase 优化点（CONTEXT §Deferred 已记）。 | 用例文件结构（`tests/e2e/killswitch_stress/` 子目录）+ 每用例独立 ctx + t.Cleanup 恢复。 |

## human_verification_needed（deferred-to-CI）

以下 4 项必须在 Linux runner（hosted ubuntu-24.04，含 docker daemon + privileged sidecar + 真实 Scenario Step 2..7）跑通：

1. **KILL-01 `TestKillSwitch_01_SigkillTiming`**
   - 前置：Phase 45 Step 4..6 真实落地后填充 `GatewayHandle.GatewayIP` / `ContainerID`；Phase 46 Plan 01 Step 7 真实启 worker 容器并填充 `HostHandle.ContainerName`。
   - 验证：基线 worker curl exit 0 → 启 host eth0 tcpdump sidecar → `docker kill --signal=KILL <gw>` → ≤ 3000ms 内 worker curl 必须非 0 + tcpdump 窗口 0 包 → verdict=Pass。
   - CI workflow 前置：`docker pull nicolaka/netshoot:v0.13`。

2. **KILL-02 `TestKillSwitch_02_TunDevDown`**
   - 前置：同 KILL-01。
   - 验证：基线 worker curl exit 0 → `docker exec <gw> ip link set tun0 down` → ≤ 3000ms 内断网 + 0 包 → cleanup 内 `ip link set tun0 up` 恢复。
   - 关键观察点：sing-box 进程未死、tun0 已 down 的窗口期 worker nft policy=drop 是否真生效。

3. **KILL-03 `TestKillSwitch_03_NetemDelay`**
   - 前置：同 KILL-01；CI workflow 必须 `docker pull gaiaadm/pumba:0.10.0` + `docker pull gaiadocker/iproute2`（hosted runner 网络白名单覆盖 Docker Hub 公网 mirror）。
   - 验证：基线 SSH banner + 出口 IP 投票稳定 → 注入 Pumba netem 1000ms delay → SSH banner 仍存活（10s 超时内） + 出口 IP 投票全弃权可 Inconclusive（t.Skipf）。
   - 兜底：Pumba image pull 失败 → 用例 t.Skipf "deferred-to-self-managed-runner"，VERIFICATION 记录。

4. **KILL-04 `TestKillSwitch_04_NetworkDisconnect`**
   - 前置：同 KILL-01。
   - 验证：基线 worker curl exit 0 → `docker network disconnect cloudproxy-net-<HostID> <gw>` → ≤ 3000ms 内断网 + 0 包 → cleanup 内 `docker network connect --ip <savedIP>` 接回。
   - 关键观察点：worker 同时挂 default `bridge` + `cloudproxy-net-*`，本测就是要证「disconnect cloudproxy-net 后 worker 不会回落到 default bridge 默认路由」。

CI runner 模板可参考 `.github/workflows/uat-bypass.yml`（已就绪，含 `sudo apt-get install -y nftables tcpdump jq dnsutils curl`）。本 phase 不要求新增 workflow 文件 —— `tests/e2e/killswitch_stress/...` 套件由 Phase 45 Plan 05 落地的 `e2e.yml` 统一驱动，`go test -tags='e2e linux' ./tests/e2e/killswitch_stress/...` 自然消费本 phase 的 4 个新用例。

## 接口约定（给 Phase 51 / 52 的下游消费）

新增 GoldenPath 方法（`tests/e2e/helpers_linux.go`，`e2e && linux`）：

- `(*GoldenPath).SetTunDevDown(ctx) error` —— gateway 容器内关闭 tun0；KILL-02 主接口，未来「软故障 + 设备级关停」场景直接复用。
- `(*GoldenPath).SetTunDevUp(ctx) error` —— 对称 cleanup 接口。
- `(*GoldenPath).DisconnectGatewayFromBridge(ctx) (savedNet, savedIP, err)` —— 自动挑选 `cloudproxy-net-*` 摘走 + 返回元数据供 reconnect；KILL-04 主接口。
- `(*GoldenPath).ReconnectGatewayToBridge(ctx, netName, staticIP) error` —— 对称 cleanup 接口；支持 staticIP 空走 docker 自动分配。
- `(*GoldenPath).InjectPumbaNetem(ctx, target, params) (cleanup, err)` —— Pumba sidecar 注入；KILL-03 主接口，未来 loss / duplicate / corrupt 等其它 netem 模式可扩 `PumbaNetemParams.Mode`。
- `(*GoldenPath).ProbeSSHBanner(ctx, timeout) error` —— 容器内 SSH 22 banner 探测；任何「SSH 控制流存活」用例直接 import。

新增包级函数 / 类型 / 常量（`tests/e2e/helpers.go`，无 build tag）：

- `StressVerdict` 三值枚举 + `String()`、`StressEvidence` 聚合结构。
- `KillswitchStressContract` 锁定表（KILL-01..04 每条 KILL 的 `MaxDisconnectMs / SSHAlive / AllowInconclusive`）；任一漂移立即 darwin 单测 fail。
- `ClassifyStressResult(kill, evidence) (StressVerdict, string)` —— 4 条 KILL 的合成裁决；下游用例直接 import。
- `PumbaNetemParams` + `BuildPumbaNetemArgs(target, params) []string`、`PumbaOutcome` 枚举 + `ParsePumbaOutput(stdout, stderr) PumbaOutcome` —— 任何「Pumba 故障注入」场景的 argv / 输出解析。
- `PickGatewayBridgeNetwork(raw string) (net, ip string)` —— docker inspect 输出 `<name>=<ip>;` 字面量解析；下游所有「网络选择」场景可复用。

新增 / 变更 Scenario / harness：

- **无**。本 phase 不动 `harness/scenario.go` / `harness/dump.go` / `harness/waitfor.go` / `harness/artifacts.go`。tcpdump pcap 持久化属 Phase 52 OBS-02 范围。

新增环境变量（CI / 本地）：

- 无新增。复用 Phase 48 引入的 `E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP`。Pumba image 通过 `PumbaNetemParams.Image` 字段（默认 `gaiaadm/pumba:0.10.0`）覆盖；CI workflow 如需私有 mirror 可在用例侧覆盖。

## 给 Phase 51 / 52 的接口约定

### Phase 51（代码层质量加固）

- **潜在 backend GAP（条件性）**：KILL-04 用例内置兜底分支，若 ubuntu-24.04 真机 + Step 2..7 落地后 gateway 仍未接 `cloudproxy-net-*` 自定义 bridge（grep 当前 v3.6 源码已就绪，但分支预留），用例自动 t.Skipf + reason 提示，待 Phase 51 QUAL-* 收紧 → 转 PASS。当前 grep 显示源码已实现专属 bridge，本 phase 不主动列 GAP。
- **不引入新 source-of-truth 漂移检测**：本 phase 所有 contract（KillswitchStressContract / KillswitchTimingContract）均锁纯函数 + darwin 单测，无 backend 源码常量交叉断言（与 Phase 46 `BootstrapExitCodeContract` 模式不同），因为 KILL-* timing 属测试侧 SLA，不是 backend API。

### Phase 52（可观察性）

- `InjectPumbaNetem` 当前不收集 Pumba sidecar 自身的 stdout/stderr 到 artifact；如 OBS-02 引入 PcapArtifactHook，可在 cleanup 内调 `cmd.Wait + io.ReadAll(stderr buffer)` → 喂 `ParsePumbaOutput` 写出 `pumba.txt` artifact。
- KILL-03 `dockerExecHandle` 是 ContainerHandle 接口最小实现；OBS-* 引入正式 testcontainers Container 后可移除。

## 提交记录

```
9441873 docs(50): 拆出 Phase 50 四个 PLAN.md (KILL-01..04)
037544d feat(50-shared): KILL-* 压力测试共享类型与 Pumba/网络 helpers
536780d feat(50-01): KILL-01 SIGKILL gateway timing 严格化 e2e 用例
de45027 feat(50-02): KILL-02 ip link set tun0 down e2e 用例
88d78ee feat(50-03): KILL-03 Pumba netem delay e2e 用例
b1c0db6 feat(50-04): KILL-04 docker network disconnect e2e 用例
```

（VERIFICATION 自身提交在本文件落定后追加。）
