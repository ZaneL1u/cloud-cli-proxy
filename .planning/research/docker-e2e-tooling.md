# Docker 专用 e2e 工具链调研

> 调研范围：社区中"专门针对 Docker 容器产出 e2e 方案"的现成工具、平台或框架。
> 调研日期：2026-05-14
> 调研者：e2e-research 团队
> 前置研究：`.planning/research/e2e-infrastructure.md` / `network-leak-testing.md` / `oss-e2e-patterns.md` / `e2e-scenarios.md`

---

## 1. 执行摘要

**社区没有"开箱即用的 Docker e2e 银弹"，但有几个零件可以直接拼进现有栈。**

本项目要测的是：Docker 容器生命周期、Linux netns、nftables、sing-box tun 隧道、SSH 接入、防泄漏强约束。这些场景横跨"容器编排 + 网络模拟 + 安全观测 + 故障注入"四个领域，没有任何单一工具能覆盖全部。

**推荐引入的 3 个零件：**

1. **Pumba** — 唯一能直接在 Docker 容器上注入 netem 网络故障的工具，与 testcontainers-go 天然兼容，可编排 kill-switch 压力测试。
2. **Tetragon** — 唯一能从内核级实时拦截 syscall 和网络连接的 eBPF 工具，可作为"测试 oracle"：断言容器是否走了预期出口 IP、是否发生了旁路连接。
3. **Hurl** — 轻量级 HTTP 断言工具，适合验证控制面 API 和管理后台接口，比写 Go 测试代码快一个数量级。

**明确不要引入的：** Earthly（已死）、Sysbox（不支持 TUN，与本项目核心场景冲突）、Garden/Skaffold/Tilt（k8s 开发循环，不是 e2e 工具）、Chaos Mesh（纯 k8s 生态，无原生 Docker 模式）。

---

## 2. 按类别逐个评估

### A. 容器化 e2e 编排平台

#### A1. Testcontainers Cloud（商业 SaaS）

| 维度 | 评估 |
|------|------|
| 用途 | 在云端运行 Testcontainers 测试，把资源密集型测试从本地/CI runner 移到托管环境 |
| 能否解决本项目痛点 | 部分。它解决的是"CI runner 资源不够"的问题，不解决网络正确性验证、不管理 netns、不验证防泄漏 |
| 推荐度 | 2/5 |
| 一句话理由 | 2023 年底被 Docker 收购，现已并入 Docker Pro/Team/Business 订阅（含 100-1500 分钟/月）。无自托管版本，锁定 Docker 生态。本项目已有 self-hosted runner 方案，引入它只会增加账单而不增加测试能力。 |

- 来源：[Docker acquires AtomicJar](https://www.docker.com/blog/docker-whale-comes-atomicjar-maker-of-testcontainers/) / [Testcontainers Cloud Pricing](https://testcontainers.com/cloud/pricing/)

#### A2. Sysbox（nestybox/sysbox）

| 维度 | 评估 |
|------|------|
| 用途 | 让 rootless 容器能跑 systemd、Docker-in-Docker、Kubernetes，像 VM 一样但不用 VM |
| 能否解决本项目痛点 | **不能**。本项目核心场景是 sing-box tun 隧道，而 Sysbox 至今不支持在容器内创建 TUN/TAP 设备（`mknod` 返回 Operation not permitted，status: WIP）。 |
| 推荐度 | 1/5 |
| 一句话理由 | 2022 年被 Docker 收购，2025 年 8 月 EE 归档、CE 继续维护。但 TUN 设备缺失是致命伤——没有 TUN 就无法跑 sing-box tun 模式。如果未来 Issue #534 解决，可重新评估。 |

- 来源：[Sysbox limitations — TUN/TAP WIP](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/limitations.md) / [Issue #534](https://github.com/nestybox/sysbox/issues/534) / [Sysbox EE archived Aug 2025](https://github.com/docker-archive/nestybox.sysbox-ee)

#### A3. Earthly

| 维度 | 评估 |
|------|------|
| 用途 | 类 Dockerfile 语法的构建 + 测试一体化框架 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 0/5 |
| 一句话理由 | **已死**。Earthly Cloud 和 Satellites 于 2025 年 7 月 16 日停止服务，开源 CLI 进入维护模式（仅修关键 bug，不接受新 PR）。公司 pivot 到 Earthly Lunar（AI guardrails）。不要在新项目中引入。 |

- 来源：[Earthly shutting down](https://earthly.dev/blog/shutting-down-earthfiles-cloud/) / [Dagger migration guide](https://dagger.io/blog/earthly-to-dagger-migration)

#### A4. Dagger（Go SDK）

| 维度 | 评估 |
|------|------|
| 用途 | 用 Go/Python/TS 写可移植的 CI/CD pipeline，每个步骤在容器中执行，基于 BuildKit |
| 能否解决本项目痛点 | 部分。它能标准化"构建镜像 → 启动容器 → 运行测试"的流程，但不提供网络模拟、不管理 netns、不做防泄漏断言。 |
| 推荐度 | 3/5 |
| 一句话理由 | 活跃开发中（2025 持续发版），Go SDK 成熟，缓存机制优秀。但它本质是 CI 编排层，不是测试框架。如果本项目未来需要把 e2e 测试流程标准化为可复用的 Go 代码（而非 shell 脚本），Dagger 是合理选择。当前阶段，testcontainers-go 已覆盖容器编排，Dagger 属于"锦上添花"而非"雪中送炭"。 |

- 来源：[Dagger Go SDK](https://dagger.io/blog/go-sdk/) / [Dagger GitHub](https://github.com/dagger/dagger)

#### A5. Garden.io

| 维度 | 评估 |
|------|------|
| 用途 | Kubernetes 开发/测试环境编排，自动起依赖服务、共享缓存、选择性测试 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 纯 k8s 工具。本项目 v1 明确不做 k8s，Garden 的编排模型（Helm chart、Ingress、Service）与本项目单宿主机 Docker 架构完全不匹配。 |

- 来源：[Garden.io docs](https://docs.garden.io/)

#### A6. Tilt / Skaffold

| 维度 | 评估 |
|------|------|
| 用途 | Kubernetes 开发循环工具：代码变更 → 自动构建 → 热重载部署 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 两者都是"inner dev loop"工具，目标是让开发者在本地 k8s 上快速迭代。Skaffold 的测试阶段只支持 container-structure-test（镜像结构验证），不支持运行时网络断言。Tilt 有 UI 但无测试能力。它们不是 e2e 测试工具。 |

- 来源：[Skaffold vs Tilt](https://www.wallarm.com/cloud-native-products-101/skaffold-vs-tilt-local-kubernetes-development) / [CNCF comparison](https://www.cncf.io/online-programs/container-native-development-tools-compared-draft-skaffold-and-tilt/)

---

### B. Docker 专用测试工具

#### B1. goss / dgoss

| 维度 | 评估 |
|------|------|
| 用途 | YAML 定义的服务器/容器配置断言工具。dgoss 是 goss 的 Docker 包装器，在容器内执行断言 |
| 能否解决本项目痛点 | 部分。适合验证"容器内是否安装了 openssh-server""sshd 是否在监听 22 端口""某个文件是否存在"等结构/服务断言。 |
| 推荐度 | 3/5 |
| 一句话理由 | 活跃维护（goss-org 组织，v0.4.8），学习曲线极低。但 goss 的断言模型是"容器内的静态检查"，无法验证网络路由正确性、无法检测流量泄漏、无法注入故障。适合作为 e2e 测试的补充层（验证容器内环境），但不能作为主力。 |

- 来源：[goss-org/goss](https://github.com/goss-org/goss) / [dgoss Docker docs](https://goss.readthedocs.io/en/stable/containers/docker/)

#### B2. container-structure-test（Google）

| 维度 | 评估 |
|------|------|
| 用途 | 验证 Docker 镜像结构：文件存在性、元数据、命令输出 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 2/5 |
| 一句话理由 | 维护模式（2025 主要是 dependabot  bump 依赖，社区 PR 积压）。它验证的是"镜像构建产物"，不是"容器运行时行为"。本项目 e2e 的核心是运行时网络正确性，CST 完全无法覆盖。 |

- 来源：[GoogleContainerTools/container-structure-test](https://github.com/GoogleContainerTools/container-structure-test)

#### B3. InSpec（Chef / Progress）

| 维度 | 评估 |
|------|------|
| 用途 | 合规与配置审计框架，支持通过 Docker API 扫描运行中容器 |
| 能否解决本项目痛点 | 部分。可验证容器配置是否符合 CIS 基线，但不涉及网络流量断言。 |
| 推荐度 | 2/5 |
| 一句话理由 | 活跃维护（Progress Software 旗下，2025 版权），有 CIS Docker Benchmark 预置 profile。但 InSpec 的强项是"合规扫描"，不是"功能测试"。它的 Ruby-based DSL 对本项目 Go 栈来说是个异类。 |

- 来源：[Chef InSpec with Docker](https://www.chef.io/blog/chef-inspec-with-docker) / [inspec/inspec GitHub](https://github.com/inspec/inspec)

#### B4. BATS-core（bash automated testing）

| 维度 | 评估 |
|------|------|
| 用途 | Bash 测试框架，TAP 兼容，可运行 shell 脚本测试 |
| 能否解决本项目痛点 | 部分。适合编排 shell 命令做 e2e（启动容器、curl API、检查路由表）。 |
| 推荐度 | 3/5 |
| 一句话理由 | 社区维护活跃，有官方 Docker 镜像。但 BATS 是"测试编排器"不是"测试断言库"——网络正确性、泄漏检测等复杂断言仍需自己写 shell。对于已有 testcontainers-go 的 Go 测试栈，BATS 是平行方案而非互补方案，引入会增加语言异构。 |

- 来源：[bats-core/bats-core](https://github.com/bats-core/bats-core) / [Docker usage](https://github.com/bats-core/bats-core/wiki/Docker-Usage-Examples)

#### B5. Hurl

| 维度 | 评估 |
|------|------|
| 用途 | 用纯文本定义 HTTP 请求和断言，基于 libcurl，Rust 实现 |
| 能否解决本项目痛点 | 部分。适合验证控制面 API（用户 CRUD、容器生命周期 API、IP 绑定 API）。 |
| 推荐度 | 4/5 |
| 一句话理由 | Orange 开源，活跃维护，有官方 Docker 镜像。文本格式比写 Go HTTP 测试快得多，支持链式请求、变量捕获、JSON 断言、性能检查。与 testcontainers-go 配合：Go 测试负责起容器/网络，Hurl 负责 API 断言。这是"现在就值得引入"的工具。 |

- 来源：[Orange-OpenSource/hurl](https://github.com/Orange-OpenSource/hurl) / [Hurl docs](https://hurl.dev/)

---

### C. 容器安全 / 防泄漏专用

#### C1. Docker Bench Security

| 维度 | 评估 |
|------|------|
| 用途 | 对照 CIS Docker Benchmark 自动扫描 Docker 安装和配置 |
| 能否解决本项目痛点 | 不能。它扫描的是"Docker daemon 配置是否安全"，不是"我的容器网络是否正确"。 |
| 推荐度 | 2/5 |
| 一句话理由 | 适合作为宿主机安全基线检查加入 CI，但不产生任何与本项目 e2e 场景相关的功能测试价值。105 项检查中，与本项目核心诉求（出口 IP 绑定、隧道完整性、防泄漏）直接相关的几乎没有。 |

- 来源：[Docker Bench Security](https://github.com/docker/docker-bench-security) / [OneUptime guide](https://oneuptime.com/blog/post/2026-02-08-how-to-use-docker-bench-security-to-harden-your-installation/view)

#### C2. Falco

| 维度 | 评估 |
|------|------|
| 用途 | eBPF 运行时安全监控，检测异常 syscall、网络连接、文件访问 |
| 能否解决本项目痛点 | **部分，且方向正确**。Falco 可以监控容器内的所有网络连接，理论上可以作为"泄漏检测 oracle"——如果容器尝试通过非隧道接口连接外网，Falco 规则可以触发告警。 |
| 推荐度 | 3/5 |
| 一句话理由 | CNCF Graduated 项目，活跃维护。但 Falco 是**检测-only**工具（不拦截），且规则引擎基于已知模式。对于 e2e 测试，我们需要的是"断言"而非"告警"。Falco 更适合生产环境监控，测试环境中用它做 oracle 需要额外封装（把告警转成测试失败）。 |

- 来源：[falcosecurity/falco](https://github.com/falcosecurity/falco) / [Falco docs](https://falco.org/docs/)

#### C3. Tetragon（Cilium）

| 维度 | 评估 |
|------|------|
| 用途 | eBPF 运行时安全观测 **+ 实时策略执行**（可 kill 进程、阻断连接） |
| 能否解决本项目痛点 | **能，强烈推荐**。Tetragon 可以在内核级拦截网络连接和 syscall，并实时执行策略（SIGKILL、阻断 egress）。这意味着：可以在测试中定义"只允许通过 tun0 接口的连接"策略，如果容器尝试旁路，Tetragon 立即阻断并生成事件日志——这个日志就是测试的 oracle。 |
| 推荐度 | 5/5 |
| 一句话理由 | Cilium 子项目，CNCF 项目，2025 活跃开发。与 Falco 的关键差异：**Tetragon 能执行**（不只是检测）。对于本项目的防泄漏测试，这是唯一能"从内核验证网络行为"的工具。配合 Go 测试代码读取 Tetragon 的 JSON/gRPC 输出，可构建自动化泄漏检测断言。 |

- 来源：[cilium/tetragon](https://github.com/cilium/tetragon) / [Tetragon enforcement docs](https://tetragon.io/docs/getting-started/enforcement/) / [InfoWorld Feb 2025](https://www.infoworld.com/article/3810607/tetragon-extending-ebpf-and-cilium-to-runtime-security.html)

#### C4. Tracee（Aqua Security）

| 维度 | 评估 |
|------|------|
| 用途 | eBPF 运行时安全与取证，覆盖 330+ syscall |
| 能否解决本项目痛点 | 部分。与 Falco 类似，是检测-only 工具。 |
| 推荐度 | 3/5 |
| 一句话理由 | Aqua Security 维护，活跃开发（2025 年 11 月仍有 Go package 发布）。功能与 Falco 高度重叠，但社区生态和文档成熟度略逊于 Falco。如果已经决定用 eBPF 做测试 oracle，Tetragon 的执行能力比 Tracee 的纯检测更有价值。 |

- 来源：[aquasecurity/tracee](https://github.com/aquasecurity/tracee)

#### C5. Sysdig Open Source / Inspector

| 维度 | 评估 |
|------|------|
| 用途 | 系统调用捕获与分析 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | Sysdig 开源版功能有限，核心能力在商用版。且它是事后分析工具（类似 tcpdump for syscalls），不适合实时 e2e 断言。 |

#### C6. kube-hunter / kube-bench

| 维度 | 评估 |
|------|------|
| 用途 | k8s 安全扫描 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 0/5 |
| 一句话理由 | 纯 k8s 工具，本项目 v1 不做 k8s。 |

#### C7. Trivy / Grype / Clair（镜像扫描）

| 维度 | 评估 |
|------|------|
| 用途 | 扫描容器镜像中的 CVE 漏洞 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 2/5 |
| 一句话理由 | 镜像扫描是安全左移的一部分，但与本项目 e2e 测试（验证运行时网络行为）完全无关。可作为 CI 流水线中的独立步骤，不应混入 e2e 测试栈。 |

---

### D. Chaos Engineering for Docker

#### D1. Pumba

| 维度 | 评估 |
|------|------|
| 用途 | 专门针对 Docker/containerd/Podman 的 chaos 工具：kill/pause/stop 容器、netem 延迟/丢包/限速、iptables 丢包、资源压力 |
| 能否解决本项目痛点 | **能，强烈推荐**。Pumba 的 `netem delay`/`loss`/`rate` 和 `iptables loss` 可直接作用于目标容器的网络命名空间，是测试"隧道中断时 kill-switch 是否生效"的理想工具。 |
| 推荐度 | 5/5 |
| 一句话理由 | 唯一一个**原生面向 Docker**且能操作 netem/iptables 的 chaos 工具。2025 年新增 inbound traffic chaos（v0.11.0+），支持 Docker/containerd/Podman 三种运行时。与 testcontainers-go 配合：Go 测试代码启动容器后，调用 Pumba CLI/API 注入故障，然后断言容器网络行为。这是 kill-switch 压力测试的核心组件。 |

- 来源：[alexei-led/pumba](https://github.com/alexei-led/pumba) / [Pumba netem docs](https://github.com/alexei-led/pumba/blob/master/docs/network.md) / [Servicelab inbound chaos](https://servicelab.org/2025/03/16/chaos-testing-docker-containers-with-iptables-and-pumba/)

#### D2. Chaos Mesh

| 维度 | 评估 |
|------|------|
| 用途 | k8s 混沌工程平台，支持 PodChaos/NetworkChaos/IOChaos/TimeChaos 等 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 纯 k8s 生态，虽有 Chaosd（物理机模式）但标记为 experimental。NetworkChaos 需要 k8s CRD 和 chaos-daemon，无法直接操作 Docker 容器。本项目 v1 不做 k8s，无需引入。 |

- 来源：[chaos-mesh/chaos-mesh](https://github.com/chaos-mesh/chaos-mesh) / [Chaosd docs](https://chaos-mesh.org/docs/chaosd-overview/)

#### D3. toxiproxy

| 维度 | 评估 |
|------|------|
| 用途 | TCP 代理，通过 REST API 注入网络故障（延迟、丢包、带宽限制、超时） |
| 能否解决本项目痛点 | 部分。适合测试"出口 IP 上游服务故障时系统的行为"，但不适合测试容器本身的网络命名空间。 |
| 推荐度 | 3/5 |
| 一句话理由 | Shopify 维护，活跃（2025 年 3 月 v2.12.0），有 Testcontainers Java 模块。但它是一个**代理**——需要把流量路由经过它才能注入故障。对于本项目的"容器内 sing-box 隧道中断"场景，Pumba 直接操作容器的 netns 更直接。toxiproxy 更适合测试"出口 IP 上游代理故障"的边界情况。 |

- 来源：[Shopify/toxiproxy](https://github.com/Shopify/toxiproxy) / [Testcontainers Toxiproxy](https://java.testcontainers.org/modules/toxiproxy/)

#### D4. tc-netem 直接用

| 维度 | 评估 |
|------|------|
| 用途 | Linux 内核原生的流量控制工具，直接操作网络接口的 qdisc |
| 能否解决本项目痛点 | 能，但需要封装 |
| 推荐度 | 3/5 |
| 一句话理由 | Pumba 底层就是调用 tc-netem。如果追求零依赖，可以直接用 `tc` 命令操作容器 veth 接口。但这需要手动解析容器 netns 路径、找到对应的 veth pair，脚本脆弱。Pumba 把这些封装好了，除非有特殊需求，否则没必要绕过 Pumba 直接用 tc。 |

---

### E. 容器化的网络 e2e 工具

#### E1. Mininet / Containernet

| 维度 | 评估 |
|------|------|
| 用途 | Mininet 是 SDN 网络模拟器；Containernet 是其 Docker 容器扩展，允许用 Docker 容器作为模拟网络中的主机 |
| 能否解决本项目痛点 | 部分。Containernet 可以构建多容器网络拓扑，模拟路由器/交换机，适合验证复杂网络场景。 |
| 推荐度 | 2/5 |
| 一句话理由 | 学术工具（~500 stars），2025 仍有社区 issue 但维护力度有限。它的模型是"在模拟网络中运行容器"，而本项目是"在真实 Docker 网络中运行真实容器"。Containernet 的抽象层（Mininet CLI、拓扑定义）对本项目来说是额外复杂度，没有直接收益。 |

- 来源：[containernet/containernet](https://github.com/containernet/containernet)

#### E2. Kathara

| 维度 | 评估 |
|------|------|
| 用途 | 容器化网络实验平台，用 Docker 容器模拟网络设备（路由器、DNS、OVS 等） |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 学术工具（意大利 Roma Tre 大学，~547 stars），2025-2026 活跃维护。但它是为"网络教学/协议研究"设计的，预置了大量网络设备镜像（Quagga、FRR、Bind）。本项目的网络拓扑很简单（容器 → tun → 出口 IP），不需要模拟多路由器拓扑。引入 Kathara 是过度设计。 |

- 来源：[KatharaFramework/Kathara](https://github.com/KatharaFramework/Kathara) / [kathara.org](https://www.kathara.org/)

#### E3. Containerlab

| 维度 | 评估 |
|------|------|
| 用途 | 用 Docker 容器编排网络设备实验室（Nokia SR Linux、Arista cEOS、Cisco XRd 等） |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 网络工程师工具，目标是模拟多厂商网络拓扑。2025 年功能丰富（rootless、VSCode 插件、netem CLI），但本项目的网络模型不涉及多跳路由或厂商 NOS。Containerlab 的复杂度对本项目无收益。 |

- 来源：[srl-labs/containerlab](https://github.com/srl-labs/containerlab) / [containerlab.dev](https://containerlab.dev/)

#### E4. FRRouting test framework

| 维度 | 评估 |
|------|------|
| 用途 | FRR 路由套件自带测试框架，基于 topotest（Python + Mininet 风格） |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 1/5 |
| 一句话理由 | 专门测试 FRR 的 BGP/OSPF/IS-IS 实现，与本项目无关。 |

#### E5. Tailscale natlab

| 维度 | 评估 |
|------|------|
| 用途 | Tailscale 内部使用的纯内存网络模拟库，模拟 NAT 类型、防火墙、UDP hole punching |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 2/5 |
| 一句话理由 | Go package（`tailscale.com/tstest/natlab`），设计目标是测试 NAT 穿越逻辑。本项目的网络模型不涉及 NAT 穿越（sing-box tun 是三层隧道，不是 P2P VPN）。但 natlab 的"纯内存网络模拟"思路值得借鉴——如果未来需要模拟多种出口 IP 环境，可参考其 Packet/PacketHandler 抽象。 |

- 来源：[tailscale.com/tstest/natlab](https://pkg.go.dev/tailscale.com/tstest/natlab)

---

### F. SaaS / 托管 e2e 平台

#### F1. Sauce Labs / BrowserStack

| 维度 | 评估 |
|------|------|
| 用途 | 浏览器/前端自动化测试 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 0/5 |
| 一句话理由 | 前端 e2e 工具（Selenium/Playwright 云执行），与本项目的后端/网络 e2e 完全无关。 |

#### F2. CircleCI / Buildkite 的 Docker e2e 能力

| 维度 | 评估 |
|------|------|
| 用途 | CI 平台，支持在 pipeline 中运行 Docker 容器 |
| 能否解决本项目痛点 | 部分。它们提供运行环境，但不提供测试工具。 |
| 推荐度 | 2/5 |
| 一句话理由 | CircleCI 和 Buildkite 都能跑 Docker-in-Docker，但本项目已选定 GitHub Actions + self-hosted runner 方案。切换 CI 平台没有技术收益，只有迁移成本。 |

#### F3. Depot / Namespace（namespace.so）

| 维度 | 评估 |
|------|------|
| 用途 | 云 CI runner，原生支持 Docker，优化构建缓存 |
| 能否解决本项目痛点 | 不能 |
| 推荐度 | 2/5 |
| 一句话理由 | Depot 主打 Docker 构建加速（BuildKit 共置），Namespace 主打快速可调试 runner（SSH/VNC）。两者都是"让 CI 更快"的工具，不是"让测试更正确"的工具。本项目 e2e 的瓶颈是网络正确性验证，不是构建速度。 |

- 来源：[Depot docs](https://depot.dev/docs/github-actions/overview) / [Namespace pricing](https://namespace.so/pricing)

#### F4. Actuated

| 维度 | 评估 |
|------|------|
| 用途 | Firecracker MicroVM 作为 GitHub Actions runner，支持嵌套虚拟化（KVM） |
| 能否解决本项目痛点 | 部分。已在 `.planning/research/e2e-infrastructure.md` 中评估过。 |
| 推荐度 | 3/5 |
| 一句话理由 | 已在前期研究中覆盖。它解决的是"self-hosted runner 隔离性"问题（每个 job 跑在独立 MicroVM 中），但不提供任何测试工具。如果本项目未来需要更强的 runner 隔离（防止 e2e 测试之间的状态污染），Actuated 是合理选择。当前阶段，已有方案足够。 |

- 来源：[Actuated — KVM in GitHub Actions](https://actuated.com/blog/kvm-in-github-actions)

---

### G. 防泄漏专门工具

#### G1. 有没有专门为"VPN/代理产品"做 e2e 验证的开源套件？

**答案：没有。**

社区中不存在一个"开箱即用的 VPN e2e 测试框架"。最接近的是：

- **Gluetun**（qdm12/gluetun）：一个 Docker VPN 客户端，内置 kill-switch 和防火墙。它的 wiki 有[测试指南](https://github.com/qdm12/gluetun-wiki/blob/main/setup/test-your-setup.md)，但那是"用户手动验证"，不是自动化测试框架。
- **Vopono**（jamesmcm/vopono）：Rust 工具，在 Linux netns 中运行应用通过 VPN。支持多 provider，但无内置测试框架。
- **vpnkillswitchtest.com**：网页工具，手动检查 kill-switch。不是开源，不能自动化。
- **Privacy Guides 社区泄漏测试器**：开源（GitHub + Netlify），但前端为主，无 CI 集成能力。

#### G2. Mullvad 内部测试套件是否开源？

**没有。** Mullvad 的客户端应用（[mullvadvpn-app](https://github.com/mullvad/mullvadvpn-app)）是 GPLv3 开源，但：

- 代码库中没有独立的"e2e 测试框架"目录。
- 2025 年 5 月宣布 Android 构建可复现（reproducible builds），但这是构建验证，不是运行时测试。
- 安全审计由第三方（Assured Security Consultants）执行，不是开源自动化测试。

#### G3. 有没有"压力测试 VPN kill-switch"的专门工具？

**没有现成的开源工具。** 但社区有公认的测试方法：

1. **IP/DNS 泄漏检测**：用 `curl ifconfig.me` / `curl icanhazip.com` 检查出口 IP；用 `dig` 检查 DNS 服务器。
2. **kill-switch 压力测试**：在 VPN 连接状态下 kill 隧道进程，立即检查是否有流量泄漏（Page Refresher + ip-api.com 每秒刷新）。
3. **自动化封装**：将上述步骤写成脚本，在 CI 中循环执行（连接 → 验证 IP → kill 隧道 → 验证无泄漏 → 重连 → 重复）。

**结论**：防泄漏 e2e 测试没有现成框架，必须自己拼装。但拼装的零件都很成熟：Pumba（注入故障）+ Tetragon（内核级行为验证）+ 自定义 Go 测试（编排 + 断言）。

---

## 3. 可以引入的「零件清单」

以下 4 个工具建议**现在就加入**本项目的 e2e 测试栈，说明它们如何与已选栈（Go + testcontainers-go + self-hosted runner）拼接：

### 零件 1：Pumba — 网络故障注入

**拼接方式：**
- testcontainers-go 启动目标容器（用户环境容器 + sing-box 旁路容器）。
- Go 测试代码通过 `exec.Command("pumba", "netem", "delay", ...)` 或 Pumba 的 Docker API 直接对目标容器注入延迟/丢包/限速。
- 注入后立即执行泄漏检测断言（curl 外网 IP、DNS 查询、WebRTC 探测）。
- 测试结束后，Pumba 自动恢复网络（`--duration` 参数）或显式清除 tc 规则。

**解决的核心场景：**
- MVS-7：隧道中断时 kill-switch 是否生效
- MVS-8：高延迟/丢包环境下的 SSH 稳定性
- MVS-9：出口 IP 切换时的会话保持

### 零件 2：Tetragon — 内核级行为验证（测试 oracle）

**拼接方式：**
- 在 self-hosted runner（Linux VM）上安装 Tetragon（`docker run` 或 systemd 服务）。
- 定义 TracingPolicy：监控目标容器的所有 `connect()` syscall 和 netns 中的网络事件，只允许目标出口 IP 和隧道接口。
- Go 测试代码在测试开始前启动 Tetragon 事件监听（gRPC 或读取 JSON 日志）。
- 测试执行期间，任何违反策略的网络连接都会被 Tetragon 记录（或阻断）。
- 测试结束后，Go 代码解析 Tetragon 输出，断言"零违规事件"。

**解决的核心场景：**
- 所有防泄漏测试的"最终 oracle"——不管测试代码怎么写，Tetragon 从内核层面验证"没有旁路流量"。
- 补充 `.planning/research/network-leak-testing.md` 中的 20+ 泄漏向量检测。

### 零件 3：Hurl — HTTP API 断言

**拼接方式：**
- 在 testcontainers-go 测试中启动控制面服务容器。
- 用 Go 的 `os/exec` 调用 `hurl --test` 执行 `.hurl` 测试文件，验证 API 响应。
- Hurl 文件放在 `e2e/hurl/` 目录，与 Go 测试代码分离，方便非 Go 开发者编写和维护。

**解决的核心场景：**
- MVS-1：用户注册/登录 API
- MVS-2：容器创建/启动/停止生命周期 API
- MVS-3：出口 IP 绑定/解绑 API
- MVS-4：到期时间治理 API

### 零件 4：dgoss — 容器内环境断言（可选，低优先级）

**拼接方式：**
- 在 Go e2e 测试中，容器启动后执行 `dgoss run` 验证容器内环境。
- `goss.yaml` 定义断言：openssh-server 已安装、sshd 监听 22、`claude code` 可执行、`/dev/net/tun` 存在。

**解决的核心场景：**
- 镜像构建验证的补充层（确保运行时环境符合预期）。
- 优先级低于前三个零件，可在 e2e 框架稳定后引入。

---

## 4. 明确"不要引入"的清单

| 工具 | 不要引入的理由 |
|------|---------------|
| **Earthly** | 已死。2025 年 7 月停止服务，开源版维护模式。 |
| **Sysbox** | 不支持 TUN 设备创建（Issue #534，status: WIP），与本项目 sing-box tun 核心场景直接冲突。 |
| **Garden.io** | 纯 k8s 工具，本项目 v1 不做 k8s。 |
| **Tilt / Skaffold** | 开发循环工具，不是 e2e 测试工具。 |
| **Chaos Mesh** | 纯 k8s 生态，Chaosd（物理机模式）为 experimental，无法直接操作 Docker 容器。 |
| **Containerlab / Kathara / Containernet** | 网络拓扑模拟工具，适合多路由器/SDN 场景。本项目的网络模型（容器 → tun → 出口 IP）不需要这些抽象。 |
| **Sauce Labs / BrowserStack** | 前端浏览器测试，与本项目无关。 |
| **Trivy / Grype / Clair** | 镜像 CVE 扫描，属于安全左移，不是 e2e 功能测试。 |
| **kube-hunter / kube-bench** | 纯 k8s 安全扫描。 |

---

## 5. 思考：为什么社区没有银弹？

### 5.1 Docker e2e 的"领域碎片化"

Docker 容器生态的 e2e 测试被拆成了至少四个不相交的子领域，每个子领域有自己的工具链，但几乎没有交叉：

1. **容器生命周期测试** → testcontainers 家族（Java/.NET/Go/Node）
2. **网络故障注入** → Pumba、tc-netem、toxiproxy（但 Pumba 是唯一原生 Docker 的）
3. **运行时安全观测** → Falco、Tetragon、Tracee（面向生产监控，不是测试）
4. **服务/API 断言** → Hurl、REST Assured、Playwright（不关心容器底层）

这四个领域的工具各自解决各自的问题，没有"统一编排层"把它们串起来。testcontainers-go 最接近统一编排，但它只管"起容器"，不管"验证网络正确性"和"注入故障"。

### 5.2 "防泄漏"是极小众需求

VPN/代理产品的防泄漏测试是一个**极窄的垂直领域**。Mullvad、ProtonVPN、NordVPN 等厂商都有自己的内部测试流程，但：

- 它们不开源测试框架（测试框架是竞争优势的一部分）。
- 社区中的泄漏测试工具（vpnkillswitchtest.com、Privacy Guides tester）都是**交互式网页工具**，不是自动化 CI 工具。
- 没有通用框架的原因是：每个 VPN 产品的架构不同（tun vs tap vs SOCKS vs WireGuard vs OpenVPN），泄漏向量也不同（DNS、WebRTC、IPv6、时间同步、STUN 等），很难抽象出通用测试 DSL。

### 5.3 "自己拼装"是必然路径

基于以上两点，本项目的 e2e 测试栈必然是一个**拼装方案**：

- **testcontainers-go** 负责容器编排（已有）
- **Pumba** 负责网络故障注入（新增）
- **Tetragon** 负责内核级行为验证（新增）
- **Hurl** 负责 API 断言（新增）
- **自定义 Go 测试代码** 负责把这些零件串成场景（已有 headscale Scenario 抽象 + gluetun verify-private 模式）

这个拼装方案的优势是**每个零件都是各自领域的最佳实践**，劣势是需要自己写"胶水代码"。但考虑到本项目的特殊性（单宿主机、强网络约束、sing-box tun），没有任何现成框架能直接套用，胶水代码是不可避免的。

---

## 参考来源

- [Sysbox GitHub](https://github.com/nestybox/sysbox) / [Sysbox limitations](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/limitations.md) / [Issue #534](https://github.com/nestybox/sysbox/issues/534)
- [Earthly shutdown announcement](https://earthly.dev/blog/shutting-down-earthfiles-cloud/)
- [Dagger GitHub](https://github.com/dagger/dagger) / [Dagger Go SDK](https://dagger.io/blog/go-sdk/)
- [Garden.io GitHub](https://github.com/garden-io/garden)
- [goss-org/goss](https://github.com/goss-org/goss) / [dgoss docs](https://goss.readthedocs.io/en/stable/containers/docker/)
- [GoogleContainerTools/container-structure-test](https://github.com/GoogleContainerTools/container-structure-test)
- [Chef InSpec](https://github.com/inspec/inspec) / [InSpec Docker blog](https://www.chef.io/blog/chef-inspec-with-docker)
- [bats-core/bats-core](https://github.com/bats-core/bats-core)
- [Orange-OpenSource/hurl](https://github.com/Orange-OpenSource/hurl) / [Hurl docs](https://hurl.dev/)
- [falcosecurity/falco](https://github.com/falcosecurity/falco) / [Falco docs](https://falco.org/docs/)
- [cilium/tetragon](https://github.com/cilium/tetragon) / [Tetragon enforcement](https://tetragon.io/docs/getting-started/enforcement/)
- [aquasecurity/tracee](https://github.com/aquasecurity/tracee)
- [alexei-led/pumba](https://github.com/alexei-led/pumba) / [Pumba inbound chaos](https://servicelab.org/2025/03/16/chaos-testing-docker-containers-with-iptables-and-pumba/)
- [Shopify/toxiproxy](https://github.com/Shopify/toxiproxy)
- [chaos-mesh/chaos-mesh](https://github.com/chaos-mesh/chaos-mesh)
- [containernet/containernet](https://github.com/containernet/containernet)
- [KatharaFramework/Kathara](https://github.com/KatharaFramework/Kathara)
- [srl-labs/containerlab](https://github.com/srl-labs/containerlab)
- [tailscale.com/tstest/natlab](https://pkg.go.dev/tailscale.com/tstest/natlab)
- [qdm12/gluetun](https://github.com/qdm12/gluetun) / [Gluetun test guide](https://github.com/qdm12/gluetun-wiki/blob/main/setup/test-your-setup.md)
- [mullvad/mullvadvpn-app](https://github.com/mullvad/mullvadvpn-app)
- [Testcontainers Cloud](https://testcontainers.com/cloud/) / [Docker acquires AtomicJar](https://www.docker.com/blog/docker-whale-comes-atomicjar-maker-of-testcontainers/)
- [Actuated](https://actuated.com/blog/kvm-in-github-actions)
- [Depot.dev](https://depot.dev/docs/github-actions/overview)
- [Namespace.so](https://namespace.so/)
