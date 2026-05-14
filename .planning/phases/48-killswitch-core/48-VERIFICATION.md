---
phase: 48-killswitch-core
status: passed
verified_at: 2026-05-14
verified_by: claude-opus-4-7
mvs_covered: [MVS-09, MVS-10]
---

# Phase 48 — VERIFICATION

## 验证结论

**status: passed** —— 遵循 CONTEXT.md §Area 4 决策：「darwin 编译 + 纯函数单测 PASS = `passed`；Linux 真机 + tcpdump 断言 deferred-to-CI（hosted ubuntu-24.04 runner 需 `sudo tcpdump` / privileged docker，Phase 45 CI workflow 已就绪）。」

darwin 本地 4 道闸全部通过：

| 闸 | 命令 | 结果 |
|----|------|------|
| 1 | `go build ./tests/e2e/...` | PASS |
| 2 | `GOOS=linux go build -tags='e2e linux' ./tests/e2e/...` | PASS |
| 3 | `go test ./tests/e2e/ -run "Helpers" -count=1` | PASS（Phase 48 新增 20 个纯函数单测全部 ok，累计本 phase + 既有共 60+ 个） |
| 4 | `bash scripts/lint-no-bare-sleep.sh` | `[ok] tests/e2e 内无裸 time.Sleep` |

## MVS 覆盖证据矩阵

| MVS | 主用例（Linux runner） | 纯函数单测（darwin） | 锁定常量 / 表 | 落地文件 |
|-----|-----------------------|---------------------|---------------|----------|
| **MVS-09** sing-box 崩溃后用户容器立即断网 | `TestKillSwitch_SingboxCrash_GoldenPath` @ `tests/e2e/killswitch_singbox_crash_test.go` | `TestHelpersParseTcpdumpCount_{FivePackets,ZeroPackets,SingularPacket,EmptyStderr,NoMatchSubstring}`、`TestHelpersClassifyKillswitch_{OK,ProbeUnexpectedlySucceeded,PacketLeak,Both}`、`TestHelpersKillswitchVerdict_String`、`TestHelpersKillswitchTimingContract_Locked` @ `tests/e2e/helpers_test.go` | `KillswitchVerdict` 枚举、`KillswitchTimingContract{ProbeMaxLatency:3s, TcpdumpWindow:5s}`、`ErrTcpdumpCountNotFound` @ `tests/e2e/helpers.go` | `tests/e2e/killswitch_singbox_crash_test.go`、`tests/e2e/helpers_linux.go::(*GoldenPath).{KillGateway,ProbeOutboundFromUser,TcpdumpOnHostEth0,InspectContainerIPv4}` |
| **MVS-10** 容器内 resolv.conf 篡改免疫 | `TestKillSwitch_ResolvConfTamper_GoldenPath` @ `tests/e2e/killswitch_resolvconf_tamper_test.go` | `TestHelpersResolvConfTamperResult_String`、`TestHelpersResolvConfTamperContract_Locked`、`TestHelpersClassifyResolvConfDNS_{RejectedAlwaysOK,AppliedTunneledNoLeak,AppliedDeniedNoLeak,AppliedTunneledWithLeak,AppliedLeaked,AppliedUnknown,TamperUnknownIsFail}` @ `tests/e2e/helpers_test.go` | `ResolvConfTamperResult` 枚举、`ResolvConfTamperContract{Nameserver:"8.8.8.8"}` @ `tests/e2e/helpers.go` | `tests/e2e/killswitch_resolvconf_tamper_test.go`、`tests/e2e/helpers_linux.go::(*GoldenPath).{TamperResolvConf,ProbeDNSFromUser}`，复用 Plan 01 `TcpdumpOnHostEth0` |

## ROADMAP / CONTEXT 偏差汇总

| ID | ROADMAP / CONTEXT 草案 | 源码 / 实现真相 | 处置位置 |
|----|-----------------------|----------------|----------|
| D-48-1 | ROADMAP §Phase 48 §Details 1 写「`curl https://ifconfig.me`」 | Phase 46 已锁定 3 源（`ip.me / ifconfig.io / ipinfo.io/ip`）；本 phase 用例选 `ifconfig.io` 作为单点 kill-switch 探测，避免与 MVS-02 Vote 协议混淆 | 用例代码内 `probeURL = "https://ifconfig.io"`；详见 `48-01-SUMMARY.md` |
| D-48-2 | CONTEXT §Area 3「`host.Exec` 复用 testcontainer host」 | Scenario 当前未暴露 host.Exec；改走 `docker run --rm --network host --cap-add NET_RAW --cap-add NET_ADMIN nicolaka/netshoot:v0.13 tcpdump ...` 的 sidecar 路径（CONTEXT §Area 3 「Claude's Discretion」节允许的备选方案） | `TcpdumpOnHostEth0` 实现层，支持 `E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP` 环境变量覆盖；详见 `48-01-SUMMARY.md` |
| D-48-3 | CONTEXT §Area 2「`cat > /etc/resolv.conf`」 | 落地用 `bash -c 'echo nameserver X > /etc/resolv.conf'` 单条管线（避免依赖容器内有无 `cat`，且 sh redirect 行为更稳）；带 `grep -q` 校验确保「文件确实被覆盖」 | `TamperResolvConf` 实现层；详见 `48-02-SUMMARY.md` |
| D-48-4 | CONTEXT §Specifics「workerIP via `hostname -i`」 | 改用 `docker inspect -f '{{.NetworkSettings.IPAddress}}'` —— 不依赖容器内有无 `hostname` 工具，且更稳；新增 `(*GoldenPath).InspectContainerIPv4` 作为下游可复用接口 | 实现层 + 接口约定节；详见 `48-01-SUMMARY.md` |
| D-48-5 | CONTEXT §Specifics「dig +short 单点判定」 | `dig exit 0 + stdout 空` 实现侧归 `DNSResultDenied`（更稳：避免「静默吞错」被假阳为 Tunneled） | `ProbeDNSFromUser` 实现层；详见 `48-02-SUMMARY.md` |

## human_verification_needed（deferred-to-CI）

以下 2 项必须在 Linux runner（hosted ubuntu-24.04，含 docker daemon + privileged sidecar + 真实 Scenario Step 2..7）跑通：

1. **MVS-09 `TestKillSwitch_SingboxCrash_GoldenPath`**
   - 前置：Phase 45 Step 4..6 真实落地后填充 `GatewayHandle.GatewayIP` / `ContainerID`；Phase 46 Plan 01 Step 7 真实启 worker 容器并填充 `HostHandle.ContainerName`。
   - 验证：基线 worker `curl https://ifconfig.io` exit 0 → 启 host eth0 tcpdump → `docker kill --signal=KILL <gateway>` → 容器内 `curl --max-time 3` 必须非 0 退出 + tcpdump 窗口内（BPF `src worker and not dst gateway`）零包 → verdict=KillswitchOK。
   - CI workflow 前置：`docker pull nicolaka/netshoot:v0.13`（避免 sidecar 启动慢于 KillGateway）；`tcpdump` / `nftables` 由 host network sidecar 自带，无需额外安装。

2. **MVS-10 `TestKillSwitch_ResolvConfTamper_GoldenPath`**
   - 前置：同 MVS-09；worker base image 含 `dig`（Phase 45 base image 选定后确认）。
   - 验证：基线 `dig example.com` Tunneled → 启 host eth0 tcpdump → 容器内尝试改写 `/etc/resolv.conf`（Applied 或 Rejected 都合法） → 再次 `dig` → tcpdump 窗口内（BPF `src worker and udp port 53 and dst 8.8.8.8`）零包 → ClassifyResolvConfDNSOutcome 返回 ok=true。
   - 预期 ro bind mount 内核行为：两条分支都视作合法（CONTEXT §Area 2 显式声明），用例不依赖具体内核版本。

CI runner 模板可参考 `.github/workflows/uat-bypass.yml`（已就绪，含 `sudo apt-get install -y nftables tcpdump jq dnsutils curl`）。本 phase 不要求新增 workflow 文件 —— `tests/e2e` 套件由 Phase 45 Plan 05 落地的 `e2e.yml` 统一驱动，`go test -tags='e2e linux' ./tests/e2e/...` 自然消费本 phase 的 2 个新用例。

## 给 Phase 49/50/51 的接口约定（防止下游漂移）

新增 GoldenPath 方法（`tests/e2e/helpers_linux.go`，`e2e && linux`）：

- `(*GoldenPath).KillGateway(ctx) error` —— `docker kill --signal=KILL` + `docker inspect` 二次确认；Phase 50 KILL-02..04 直接复用，不同 kill 信号请新加 `KillGatewayWithSignal`。
- `(*GoldenPath).ProbeOutboundFromUser(ctx, url string, timeout time.Duration) (exitCode int, err error)` —— 容器内 curl 探测；任何「容器内出网探测」phase 直接 import。
- `(*GoldenPath).TcpdumpOnHostEth0(ctx, bpfFilter string, count int, timeout time.Duration) (packets int, err error)` —— 默认走 host network privileged sidecar；`E2E_ALLOW_HOST_TCPDUMP=1 + uid==0` 走原生路径；`E2E_TCPDUMP_IMAGE` 可覆盖 sidecar 镜像。
- `(*GoldenPath).InspectContainerIPv4(ctx, name, network string) (string, error)` —— `docker inspect` 拿容器 IPv4；任何 BPF filter / 内网拓扑断言 phase 直接 import。
- `(*GoldenPath).TamperResolvConf(ctx, nameserver string) (ResolvConfTamperResult, error)` —— Phase 49 LEAK-02..04 任何 resolv.conf 对抗用例直接复用。
- `(*GoldenPath).ProbeDNSFromUser(ctx, domain string, timeout time.Duration) (DNSProbeResult, error)` —— Phase 49 任何「容器内 DNS 探测」直接复用。

新增包级函数 / 类型 / 常量（`tests/e2e/helpers.go`，无 build tag）：

- `KillswitchVerdict` 枚举 + `String()`、`ClassifyKillswitchResult(probeExitCode, leakedPackets) KillswitchVerdict`。
- `KillswitchTimingContract{ProbeMaxLatency:3s, TcpdumpWindow:5s}` 锁定结构。
- `ParseTcpdumpCountOutput(stderr string) (int, error)` + `ErrTcpdumpCountNotFound` sentinel。
- `ResolvConfTamperResult` 枚举 + `String()`、`ResolvConfTamperContract{Nameserver:"8.8.8.8"}` 锁定结构。
- `ClassifyResolvConfDNSOutcome(tamper, dnsResult, leakedPackets) (ok bool, reason string)` —— 6 分支表，Phase 49 LEAK-02..04 共享。

新增 / 变更 Scenario / harness：

- **无**。本 phase 不动 `harness/scenario.go` / `harness/dump.go` / `harness/waitfor.go` / `harness/artifacts.go`。失败 artifact 仍由 Phase 45 Plan 04 落地的 `ArtifactDumper.OnWaitForTimeout` 负责（写 `system/wait-timeout.txt`）；tcpdump pcap 持久化属 Phase 52 OBS-02 范围，本 phase 不引入新 hook。

新增环境变量（CI / 本地）：

- `E2E_TCPDUMP_IMAGE`（默认 `nicolaka/netshoot:v0.13`）：覆盖 sidecar tcpdump 镜像；air-gapped runner 可指向私有镜像仓库。
- `E2E_ALLOW_HOST_TCPDUMP`（默认 0）：设为 `1` 且 `uid==0` 时走宿主机原生 tcpdump，跳过 sidecar 路径；建议仅自管 runner 启用。

## 提交记录

```
fcfb6dd docs(48): 拆出 Phase 48 两个 PLAN.md
2fca349 feat(48-01): sing-box 崩溃后用户容器立即断网 e2e 用例 (MVS-09)
433bf7a feat(48-02): 容器内 resolv.conf 篡改免疫 e2e 用例 (MVS-10)
```

（VERIFICATION 自身提交在本文件落定后追加。）
