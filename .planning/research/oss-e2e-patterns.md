# 同类开源项目 e2e 测试做法调研

> 时间：2026-05-14
> 目标：为 Cloud CLI Proxy 构建一套能在 CI 上跑、覆盖「SSH 接入 + 网络强约束」的 e2e 套件，先看同类项目是怎么做的。

---

## 1. 执行摘要

**最值得抄的两个项目：**

1. **`juanfont/headscale` 的 `integration/` 包**——它把「控制面 + 多个隔离的容器节点 + 受测的连通性场景」抽象成一个 `Scenario` 对象，靠 `ory/dockertest` 在 `go test` 内部直接拉容器、组网、断言 SSH 与策略行为。这种形态几乎等于本项目「控制面 + N 个用户容器 + 出口绑定 + SSH 接入」的同构问题，可以最大限度复用 [`integration/scenario.go`](https://github.com/juanfont/headscale/blob/main/integration/scenario.go) 和 [`integration/ssh_test.go`](https://github.com/juanfont/headscale/blob/main/integration/ssh_test.go) 的代码骨架。
   - 一句话理由：它是「Go 测试驱动真实 Docker 拓扑 + 跨容器 SSH 断言」的最完整开源样本，与我们目标一致。

2. **`qdm12/gluetun` 的 `ci/` 工具 + `verify-private` GitHub Actions job**——它定义了一个独立的 Go 二进制 `ci/cmd/main.go`，CI 调用它启动真容器、注入真凭据、订阅容器日志做正则断言，并显式校验「公网 IP 是否切换到 VPN 出口」。这种「自建 runner 二进制 + 通过日志和探测验证流量已走指定出口」的做法可以直接搬到我们的「校验 sing-box 出网走指定 IP」用例上。
   - 一句话理由：它是同类项目里唯一公开的「在 CI 里跑真 VPN + 真出口探测」流水线，做法清楚且与我们的强约束诉求一致。

**研究覆盖的项目（共 9 个，分四类）：**

| 类别 | 项目 | 代表性 |
| --- | --- | --- |
| 容器化代理 / VPN 网关 | gluetun、innernet、headscale、netbird | 强 |
| 协议代理引擎 | sing-box、mihomo（资料不足，未深入） | 中 |
| 容器化远程开发 / SSH 沙箱 | coder、devpod、daytona（无公开 e2e） | 强 |
| Docker 防火墙 / 多租户隔离 | ufw-docker、k3s、nomad | 中 |

---

## 2. 项目逐个分析

### 2.1 `juanfont/headscale`（控制面 + 受控节点 + 跨容器 SSH，最对位）

1. **测试目录：** [`integration/`](https://github.com/juanfont/headscale/tree/main/integration)
   - 入口：[`scenario.go`](https://github.com/juanfont/headscale/blob/main/integration/scenario.go)、[`scenario_test.go`](https://github.com/juanfont/headscale/blob/main/integration/scenario_test.go)
   - SSH 场景：[`ssh_test.go`](https://github.com/juanfont/headscale/blob/main/integration/ssh_test.go)
   - 节点抽象：[`hsic/`](https://github.com/juanfont/headscale/tree/main/integration/hsic)、[`tsic/`](https://github.com/juanfont/headscale/tree/main/integration/tsic)
   - Dockerfile：[`Dockerfile.integration`](https://github.com/juanfont/headscale/blob/main/Dockerfile.integration)、[`Dockerfile.tailscale-HEAD`](https://github.com/juanfont/headscale/blob/main/Dockerfile.tailscale-HEAD)

2. **框架：** 原生 `go test` + [`ory/dockertest`](https://github.com/ory/dockertest) + `testify`。不引入 Ginkgo。

3. **如何起栈：** `Scenario` 内部持有 `*dockertest.Pool` 和 `map[string]*dockertest.Network`，按 `ScenarioSpec{Users, NodesPerUser, Networks, Versions}` 声明式启动。每个 user 用 `errgroup` 并发拉自己的 tailscale 节点容器，注入对 headscale 容器的 hosts 映射后调用 `Login`，最后用 `WaitForTailscaleSync()` 等所有节点 netmap 收敛。teardown 用 `ShutdownAssertNoPanics(t)` 顺便扫所有容器日志检测 panic。

4. **网络断言怎么写：** SSH 断言走「在源容器里 exec ssh，远端执行 `hostname`，比对返回的容器 ID」这条路。代码片段（来自 `ssh_test.go`）：

```go
command := []string{
    "/usr/bin/ssh", "-o StrictHostKeyChecking=no", "-o ConnectTimeout=1",
    fmt.Sprintf("%s@%s", sshUser, peerFQDN),
    "'hostname'",
}
result, _, err := doSSH(t, client, peer)
require.NoError(t, err)
require.Contains(t, peer.ContainerID(), strings.ReplaceAll(result, "\n", ""))
```

拒绝侧用 `assertSSHPermissionDenied` 检 stderr 是否包含 `"Permission denied (tailscale)"` 等已知错误串；超时侧用 `assertSSHTimeout` 检 `"Connection timed out"`。

5. **CI：** GitHub Actions，hosted runner。仓库根目录有 [`integration/run.sh`](https://github.com/juanfont/headscale/blob/main/integration/run.sh) 作为统一入口。同一个 Go 测试既能本地跑（要求本机有 Docker daemon），也能 CI 跑。

6. **我们可以直接抄的：**
   - `Scenario` + `ScenarioSpec` 这套「声明式拓扑」抽象。
   - `ShutdownAssertNoPanics` 这种把容器日志当 oracle 的扫尾。
   - 「在源容器里 exec ssh peer 跑 hostname 比对」的 SSH 验证模式。
   - 把网络/节点工厂拆成 `hsic/`（headscale-in-container）、`tsic/`（tailscale-in-container）两个小 package，让 `_test.go` 文件保持「描述场景」而不是「干脏活」。

---

### 2.2 `qdm12/gluetun`（CI 内拉真 VPN + 验证出口 IP 切换）

1. **测试位置：** 单元测试散落在 `internal/**/*_test.go`（同包同目录）；e2e 等价物在 [`ci/`](https://github.com/qdm12/gluetun/tree/master/ci) 子模块——自带 `go.mod`，独立编译成 runner 二进制。`Dockerfile` 多 stage 构建里有 `lint`、`mocks`、`test`、`xcompile` 等阶段。

2. **框架：** `go test` 跑单元测试；e2e 用自写的 Go 二进制 + 真 VPN 凭据（CI 的 secrets）。没有用 BDD 框架。

3. **如何起栈：** [`.github/workflows/ci.yml`](https://github.com/qdm12/gluetun/blob/master/.github/workflows/ci.yml) 中 `verify-private` job 调用 `./ci/runner mullvad`、`./ci/runner protonvpn-wireguard-port-forwarding` 等子命令，每个子命令通过 `internal/simple.go` 的 `runContainerTest` 拉一个 `qmcgaw/gluetun` 容器，挂 `/dev/net/tun`、加 `NET_ADMIN`、`NET_RAW` 能力。

4. **网络断言怎么写：** 关键是「订阅容器 stdout，按正则匹配几条关键日志」。比如：

```go
var (
    successRegexp        = regexp.MustCompile(`^.+Public IP address is .+$`)
    portForwardingRegexp = regexp.MustCompile(`port forwarded is \d`)
)
```

`waitForLogLines` 顺序匹配一组正则，全部命中算通过；中间还会主动 `ContainerInspect` 看进程是否还活着，避免因容器崩了但日志没刷新而误判超时。这是「拿应用自己的日志做 oracle」的一种朴素但很有效的做法。

5. **CI：** GitHub Actions 的 hosted runner（`ubuntu-latest`）。`verify-private` job 用 `environment: secrets` 来注入 Mullvad、ProtonVPN、PIA 等真凭据，所以只在 push/release 或可信 PR 上跑。

6. **我们可以直接抄的：**
   - 把 e2e runner 做成独立 Go 二进制（独立 `go.mod`），与主仓库共用类型但单独编译，CI 直接调用。
   - 「正则匹配容器日志 + 周期性 ContainerInspect 防卡死」这套等待逻辑可以直接抄进 host-agent 启动校验。
   - CI 上跑真出口（我们叫「真 sing-box outbound」）的时候用 `environment: secrets` 隔离凭据。

---

### 2.3 `SagerNet/sing-box`（go test 内启 Docker 协议端 + 协议级吞吐验证）

1. **测试目录：** [`test/`](https://github.com/SagerNet/sing-box/tree/dev-next/test)，独立 `go.mod`。
   - 公共 fixture：[`box_test.go`](https://github.com/SagerNet/sing-box/blob/dev-next/test/box_test.go)（`startInstance`、`testSuit`、`testTCP`、`testQUIC` 等）
   - Docker 桩：[`docker_test.go`](https://github.com/SagerNet/sing-box/blob/dev-next/test/docker_test.go)（`startDockerContainer`、`cleanContainer`）
   - 协议用例：`shadowsocks_test.go`、`trojan_test.go`、`vmess_test.go`、`socks_test.go` 等

2. **框架：** 原生 `go test` + Docker SDK（`github.com/docker/docker/client`）+ `testify/require`。配合 `go.uber.org/goleak` 在 `TestMain` 里检 goroutine 泄漏。

3. **如何起栈：** `startDockerContainer` 直接调 Docker API，挂 host network（`hostOptions.NetworkMode = "host"`），按测试需要的端口范围（10000+ 起步）映射端口、注入 entrypoint。`startInstance` 启的是被测的 sing-box 本身（Go in-process）。Docker 容器只承载「真实对端协议实现」（例如 shadowsocks-rust 客户端）做协议互操作。

4. **网络断言怎么写：** 协议测试统一走 `testSuit(t, clientPort, testPort)`：

```go
require.NoError(t, testPingPongWithConn(t, testPort, dialTCP))
require.NoError(t, testPingPongWithPacketConn(t, testPort, dialUDP))
require.NoError(t, testLargeDataWithConn(t, testPort, dialTCP))
require.NoError(t, testLargeDataWithPacketConn(t, testPort, dialUDP))
```

即「通过被测代理 dial 一个回显端口，断言 ping-pong 与大块数据双向成功」。UDP timeout 用例还会等超过 timeout 再发包，断言 session 已经被清掉（[`socks_test.go`](https://github.com/SagerNet/sing-box/blob/dev-next/test/socks_test.go) 的 `testUDPSessionIdleTimeout`）。

5. **CI：** 由 [`.github/workflows`](https://github.com/SagerNet/sing-box/tree/dev-next/.github/workflows) 中 Actions 跑，需要 hosted runner 上有 Docker（Linux runner 自带）。

6. **我们可以直接抄的：**
   - 「测试桩 = Docker 容器（真实第三方实现）+ 被测组件 = 进程内启」的混合模式。我们已有 sing-box 子进程，可以用这种结构「容器内只跑用户镜像 / 流量靶子，控制面进程在测试进程里启」。
   - 端口分配靠 const 块按需偏移（`serverPort uint16 = 10000 + iota`）——简单可靠，避免端口冲突。
   - `TestMain` 里挂 `goleak.VerifyTestMain` 防止网络相关代码 goroutine 泄漏。

---

### 2.4 `loft-sh/devpod`（Ginkgo + 真 Docker provider + SSH echo 断言）

1. **测试目录：** [`e2e/`](https://github.com/loft-sh/devpod/tree/main/e2e)
   - 入口：[`e2e/e2e_suite_test.go`](https://github.com/loft-sh/devpod/blob/main/e2e/e2e_suite_test.go)
   - 框架辅助：[`e2e/framework/`](https://github.com/loft-sh/devpod/tree/main/e2e/framework)
   - 各场景：`e2e/tests/{build,context,ide,integration,machine,machineprovider,provider,ssh,up}/`
   - SSH 场景：[`e2e/tests/ssh/ssh.go`](https://github.com/loft-sh/devpod/blob/main/e2e/tests/ssh/ssh.go)

2. **框架：** [Ginkgo v2 + Gomega](https://onsi.github.io/ginkgo/)。`framework/` 内有 `ExpectNoError`、`ExpectEqual` 等 Gomega 包装。

3. **如何起栈：** 每个 `It` 用 `framework.CopyToTempDir` 复制一个 testdata 工作区到临时目录，然后调用 `f.DevPodProviderAdd(ctx, "docker")`、`f.DevPodUp(devpodUpCtx, tempDir)`——实际就是把待测 CLI 当 subprocess 调起来，让它真去拉 Docker 容器。

4. **网络断言怎么写：** SSH 用例就是 `f.DevPodSSHEchoTestString(devpodSSHCtx, tempDir)`——往容器里 `ssh` 一个固定字符串再读回。端口转发用例的写法是生成随机端口 → `devpod ssh --forward port` → 在宿主机用 netcat 起服务 → 远端 curl 回来：

```go
port := rng.Intn(1000) + 50000
fmt.Println("Running netcat server on port", port)
devpodSSHDeadline := time.Now().Add(20 * time.Second)
// ... f.DevPodUp + SSH port forward 后做连通性检查
```

5. **CI：** GitHub Actions hosted runner。`runtime.GOOS != "linux"` 时还要起个 fake agent server（见 `e2e_suite_test.go` 中的 `framework.ServeAgent()`）。

6. **我们可以直接抄的：**
   - 「testdata 目录 + CopyToTempDir」的隔离方式：让每个用例有自己的临时工作目录，不污染主仓。
   - `f.DevPodUp` / `f.DevPodSSHEchoTestString` 这种把 CLI 全流程包成框架方法、`_test.go` 里就剩业务断言的拆法。
   - 用 deadline context 控制每一步超时，比 `time.Sleep` + 轮询更工程化。

---

### 2.5 `coder/coder`（Playwright + 真 coderd + 真 workspace agent）

1. **测试目录：** [`site/e2e/`](https://github.com/coder/coder/tree/main/site/e2e)
   - playwright 配置：[`site/e2e/playwright.config.ts`](https://github.com/coder/coder/blob/main/site/e2e/playwright.config.ts)
   - 用例：[`site/e2e/tests/`](https://github.com/coder/coder/tree/main/site/e2e/tests)（`webTerminal.spec.ts`、`workspaces/`、`templates/` 等）
   - 辅助：`helpers.ts`、`api.ts`、`hooks.ts`
   - 负载/大规模：[`scaletest/`](https://github.com/coder/coder/tree/main/scaletest)

2. **框架：** Playwright（TS），断言走 `expect`、`waitForSelector` 这些 web 原生 API。

3. **如何起栈：** Playwright 启动前先在本地把 coderd 跑起来，测试里用 `createTemplate` / `createWorkspace` / `startAgent`（来自 `helpers.ts`）通过真 API 创建工作区，`startAgent` 直接以子进程拉一个 workspace agent 与 coderd 建立连接，再去操作 UI。

4. **网络断言怎么写：** Web terminal 场景就是 UI 端验证：

```ts
await terminal.keyboard.type("echo he${justabreak}llo123456");
await terminal.keyboard.press("Enter");
await terminal.waitForSelector(
    'div.xterm-rows span:text-matches("hello123456")',
    { state: "visible", timeout: 10 * 1000 },
);
```

也就是「在 web 终端里输命令，再从 xterm 渲染的 DOM 中读输出」。

5. **CI：** GitHub Actions hosted runner，跟前端构建串在一起。

6. **我们可以直接抄的：**
   - 我们有后台 UI，可以用 Playwright 同样验证「在后台创建用户 → 拿到 curl 入口 → 用户登入容器 → SSH 流程跑通」。但 web 终端那一段不强求，重点借「`helpers.ts` 提供 createTemplate/createWorkspace 等等业务原语」的组织方式。
   - `scaletest/` 那种长测、`smtpmock/` `llmmock/` 这类 mock 子目录的命名也值得借鉴。

---

### 2.6 `chaifeng/ufw-docker`（Bach 单元 + Vagrant VM 真 iptables 全栈）

1. **测试目录：** [`test/`](https://github.com/chaifeng/ufw-docker/tree/master/test) + 根目录 [`test.sh`](https://github.com/chaifeng/ufw-docker/blob/master/test.sh)、[`test-vagrant.sh`](https://github.com/chaifeng/ufw-docker/blob/master/test-vagrant.sh)、[`Vagrantfile`](https://github.com/chaifeng/ufw-docker/blob/master/Vagrantfile)、`trace-iptables.sh`、`print-iptables.sh`。

2. **框架：** 单元层用 [`bach`](https://github.com/bach-sh/bach)（一个纯 Bash 测试框架，能 mock 外部命令），全栈层用 Vagrant + Virtualbox/Parallels 拉真 VM 跑真 iptables。

3. **如何起栈：** 单元层就是 `bash test/ufw-docker.test.sh` 跑 `bach` 用例（mock 掉 `iptables` / `ufw` / `docker`）。VM 层 `test-vagrant.sh` 用嵌套循环：

```bash
for ENABLE_DOCKER_IPV6 in true false; do
  for USE_IPTABLES_LEGACY in false true; do
    vagrant destroy --force && env "${env_list[@]}" vagrant up
    ...
  done
done
```

每组矩阵都 destroy + provision 一次，并额外测一次 reload。

4. **网络断言怎么写：** 在 Vagrantfile 里编排 `master` 节点和 `external` 节点，`vagrant provision external` 内部会从 external 节点 curl/ping master 节点的容器服务，依此判断规则是否生效（具体在 Vagrantfile 的 provision shell 段中，篇幅大未全展开）。单元层断言形如：

```bash
test-script-fails-if-ufw-is-disabled() {
    @mockfalse grep -Fq "Status: active"
    @mock iptables --version === @stdout 'iptables v1.8.4 (legacy)'
    ufw-docker
}
test-script-fails-if-ufw-is-disabled-assert() {
    die "UFW is disabled or you are not root user, ..."
    ufw-docker--help
}
```

5. **CI：** Drone CI（`.drone.yml`），单元层在容器里跑 Bach；VM 层一般是开发者本地手动跑。

6. **我们可以直接抄的：**
   - 「外部探针 VM/容器」模式——在测试拓扑里专门留一个不受被测策略保护的「外部」容器，作为对 master 节点容器发起连通性测试的发起方，比在被测容器里自测更可信。
   - iptables/nftables 规则验证不靠「读规则文本」，靠「真的发包看能不能过」。

---

### 2.7 `hashicorp/nomad`（Terraform 拉真集群 + 自写测试框架 + alloc exec 探针）

1. **测试目录：** [`e2e/`](https://github.com/hashicorp/nomad/tree/main/e2e)
   - 框架：[`e2e/framework/`](https://github.com/hashicorp/nomad/tree/main/e2e/framework)、[`e2e/e2eutil/`](https://github.com/hashicorp/nomad/tree/main/e2e/e2eutil)、[`e2e/v3/`](https://github.com/hashicorp/nomad/tree/main/e2e/v3)
   - 拓扑：[`e2e/terraform/`](https://github.com/hashicorp/nomad/tree/main/e2e/terraform)
   - 网络/隔离：[`e2e/networking/networking.go`](https://github.com/hashicorp/nomad/blob/main/e2e/networking/networking.go)、[`e2e/isolation/`](https://github.com/hashicorp/nomad/tree/main/e2e/isolation)、[`e2e/cni/`](https://github.com/hashicorp/nomad/tree/main/e2e/cni)

2. **框架：** 原生 `go test` + 自写 `framework`（声明 `TestSuite{Component, CanRunLocal, Cases}`）。新写的用例直接用 stdlib testing 风格。

3. **如何起栈：** Terraform 在 AWS 起多节点集群（servers + clients），Go 测试通过 `NOMAD_ADDR` / `CONSUL_HTTP_ADDR` 连到现有集群跑，不会在 Go 进程内拉容器。

4. **网络断言怎么写：** 用 `nomad alloc exec` 进容器读 `/proc`、`/etc/hosts`、`env`、`ip addr` 等：

```go
hostnameOutput, err := e2eutil.AllocExec(allocs[0].ID, "sleep", "hostname", "default", nil)
f.Equal("mylittlepony", strings.TrimSpace(hostnameOutput))

hostsOutput, err := e2eutil.AllocExec(allocs[0].ID, "sleep", "cat /etc/hosts", "default", nil)
f.Contains(hostsOutput, "mylittlepony")

envOutput, err := e2eutil.AllocExec(allocs[0].ID, "sleep", "env", "default", nil)
f.Contains(envOutput, "NOMAD_ALLOC_INTERFACE_dummy")
```

5. **CI：** 主要不在 hosted runner 上跑；要求 `NOMAD_E2E=1` 和现成集群，nightly/手动触发。

6. **我们可以直接抄的：**
   - 「测试不负责起集群、只负责验证」这种解耦——我们的 e2e 套件可以预期 `docker compose up -d` 已经把控制面 + Postgres + host-agent 起好，测试通过 HTTP 与 docker exec 探针验证。
   - 「在受测容器里 exec 命令做探针」的模式，对我们检查 `/etc/resolv.conf`、`ip route`、`curl ifconfig.me` 都直接适用。

---

### 2.8 `k3s-io/k3s`（Vagrant + Ginkgo + 跨节点 curl）

1. **测试目录：** [`tests/`](https://github.com/k3s-io/k3s/tree/master/tests)，e2e 在 [`tests/e2e/`](https://github.com/k3s-io/k3s/tree/master/tests/e2e)（按场景分 `validatecluster/`、`dualstack/`、`tailscale/`、`secretsencryption/` 等）。
   - 公共节点抽象：[`tests/e2e/testutils.go`](https://github.com/k3s-io/k3s/blob/master/tests/e2e/testutils.go)（`VagrantNode`、`RunCmdOnNode`、`CreateCluster`、`DumpNodes` 等）
   - 验证用例样例：[`tests/e2e/validatecluster/validatecluster_test.go`](https://github.com/k3s-io/k3s/blob/master/tests/e2e/validatecluster/validatecluster_test.go)

2. **框架：** Ginkgo v2 + Gomega + Vagrant（Libvirt/Virtualbox）。

3. **如何起栈：** `CreateCluster(nodeOS, serverCount, agentCount)` 内部 `vagrant up` 起多个 VM，`VagrantNode.RunCmdOnNode` 通过 `vagrant ssh --no-tty $name -c "sudo $cmd"` 在 VM 里执行命令。每个测试包都有自己的 `Vagrantfile`。

4. **网络断言怎么写：** 服务连通性的写法很朴素——挨个节点 `curl` ClusterIP 或 NodePort：

```go
clusterip, _ := e2e.FetchClusterIP(tc.KubeconfigFile, "nginx-clusterip-svc", false)
cmd := "curl -m 5 -s -f http://" + clusterip + "/name.html"
for _, node := range tc.Servers {
    Eventually(func(g Gomega) {
        res, err := node.RunCmdOnNode(cmd)
        g.Expect(err).NotTo(HaveOccurred())
        Expect(res).Should(ContainSubstring("test-clusterip"))
    }, "120s", "10s").Should(Succeed())
}
```

`Eventually(...).Should(Succeed())` 是 Gomega 标准的重试断言模式。

5. **CI：** GitHub Actions，但用 self-hosted runner（VM 编排在 hosted runner 上太重）。

6. **我们可以直接抄的：**
   - `Eventually(...).Should(Succeed())` 重试模板，比手写 backoff 强很多。
   - `VagrantNode` 这种「节点对象 + RunCmdOnNode」抽象——我们可以做 `Container{ID}.Exec(cmd)`。
   - 启动失败把 vagrant log 直接附到错误里（`e2e.GetVagrantLog(err)`），调试体验好。

---

### 2.9 `tonarino/innernet`（纯 Shell + Docker bridge + 命令式拓扑）

1. **测试目录：** [`docker-tests/`](https://github.com/tonarino/innernet/tree/main/docker-tests)
   - 入口：[`run-docker-tests.sh`](https://github.com/tonarino/innernet/blob/main/docker-tests/run-docker-tests.sh)
   - 镜像：[`Dockerfile.innernet`](https://github.com/tonarino/innernet/blob/main/docker-tests/Dockerfile.innernet)
   - 进程脚本：`start-server.sh`、`start-client.sh`

2. **框架：** 纯 Bash + `docker create/start/exec`。不依赖任何测试库。

3. **如何起栈：** 创建一个 `bridge` 网络 `innernet`（`docker network create -d bridge --subnet=172.18.0.0/16 innernet`），server 容器固定 `172.18.1.1`，peer 容器分别 `172.18.1.2/3`，每个容器都挂 `/dev/net/tun` 并 `--cap-add NET_ADMIN`。trap EXIT 时 stop 所有容器并删除网络。

4. **网络断言怎么写：** 主要靠每一步 `innernet add-cidr`、`add-association`、`add-peer` 命令的退出码；用 `--interactive` 给开发者留人工 ping/curl 入口。这是「拓扑搭起来 + 命令链跑过 = 算通」的低成本方案，缺点是没有显式断言连通性。

5. **CI：** GitHub Actions hosted runner，但脚本可直接本机跑。

6. **我们可以直接抄的：**
   - 「`trap cleanup EXIT` 一定要 stop 容器 + 删网络」——保证多次跑不会残留。
   - 固定子网 + 固定 IP，避免 Docker 自动分配引起的端口/路由漂移。
   - 把「构建测试镜像」拆成 `build-docker-images.sh` 单独一步，CI 可以缓存。

---

### 2.10 简单调研但素材不足的项目

- **`netbirdio/netbird`**：测试散落在各 module 的 `*_test.go`，没有集中的 `e2e/` 目录。WebFetch 没拿到关键文件，缺乏可借鉴样本。
- **`daytonaio/daytona`**：仓库根目录没有 `e2e/`，README 没提 e2e 流程；测试主要在 `apps/*` 内部，多为单元。
- **`MetaCubeX/mihomo`**：调研路径取到了同名但不同业务的仓库，未深入。
- **Firecracker / kasm**：kasm core-images 仓库无 e2e；firecracker 走 `tools/devtool` + Python rust-VM 集成，与本项目形态相距较远，未深入。

---

## 3. 模式提炼

把上述项目的实践拆开重组，归纳出 5 个反复出现的模式：

### 模式 A：声明式拓扑 + Go 测试驱动 Docker

**长这样：** 一个 `Scenario` / `TestConfig` 对象持有 `dockertest.Pool` 或 Docker SDK 客户端；`ScenarioSpec` 声明节点数、网络数、版本矩阵；`go test` 内部并发拉容器，统一 teardown。

**代表：** headscale `integration/scenario.go`、sing-box `test/docker_test.go`。

**适用我们：** 控制面 + 多用户容器 + 出口绑定的拓扑可以这样描述。

### 模式 B：CI runner 二进制 + 真凭据 + 日志正则做 oracle

**长这样：** 仓库里有一个独立 `ci/` 子模块（独立 `go.mod`），里面是一个 CLI runner；GitHub Actions 的 secrets 注入真凭据，runner 在 CI 上启真容器，按正则吃日志判断成败。

**代表：** gluetun `ci/internal/simple.go` + `verify-private` job。

**适用我们：** 验证「sing-box 连到指定上游 + 出口 IP 切到指定地址」就是这个模式。

### 模式 C：探针容器 / 探针节点

**长这样：** 在测试拓扑里加一个不受被测策略保护的容器（external/probe），让它对受测目标主动发起连通性请求。

**代表：** ufw-docker 的 `external` Vagrant 节点、innernet 的 `peer1` 给 `peer2` 发邀请、headscale 的 `tsic`。

**适用我们：**
- 验证「容器只能从 sing-box 出网」时，在同一 docker bridge 上放个 probe 容器，断言被测容器直连它的某些直连路径必须不可达，而绕 sing-box 的路径可达。
- 验证「DNS / WebRTC 不泄漏」时，可以在宿主机 + probe 容器上同时跑 tcpdump，等 e2e 跑完比对。

### 模式 D：在被测容器里 exec 命令做断言

**长这样：** 通过 `docker exec` 或平台原生 `alloc exec` / `vagrant ssh -c` 拿容器内的视角（`/proc`、`/etc/resolv.conf`、`ip route`、`curl ifconfig.me`、`hostname`）。

**代表：** nomad `e2eutil.AllocExec`、headscale `doSSH`、k3s `VagrantNode.RunCmdOnNode`。

**适用我们：** 直接对应「容器内看到的默认网关 / DNS / 公网 IP 必须是预期值」的断言。

### 模式 E：testdata + tempdir 隔离

**长这样：** 每个用例在 `testdata/` 下有自己的最小配置目录，运行时 `CopyToTempDir` 复制一份再启栈，避免污染主仓和并发用例互相干扰。

**代表：** devpod `e2e/tests/ssh/testdata/local-test`。

**适用我们：** 多用户、多出口、多到期策略组合验证时直接铺 `testdata/<case>/`。

---

## 4. 借鉴清单（给本项目）

按「能不能直接照搬」分两组。

### 4.1 可立即抄（无需大改）

| 做法 | 直接抄什么 | 落地建议 |
| --- | --- | --- |
| Go 测试内拉 Docker 拓扑 | headscale [`integration/scenario.go`](https://github.com/juanfont/headscale/blob/main/integration/scenario.go) 的 `Scenario` + `ScenarioSpec` 骨架 | 新建 `e2e/scenario.go`，定义 `Scenario{pool, ctrl, hostAgent, containers, networks}` 与 `ScenarioSpec{Users, EgressIPs, Networks}`。 |
| 跨容器 SSH 断言模板 | [`integration/ssh_test.go`](https://github.com/juanfont/headscale/blob/main/integration/ssh_test.go) 的 `doSSH` + `assertSSHHostname` | 直接照搬「ssh -o ConnectTimeout=1 ... 'hostname'，比对 ContainerID」。 |
| 容器日志正则做 oracle | gluetun [`ci/internal/simple.go`](https://github.com/qdm12/gluetun/blob/master/ci/internal/simple.go) 的 `waitForLogLines` | 包装成 `WaitForLog(container, regexps...)`，给「等待 sing-box 路由生效」「等待容器 SSH banner」复用。 |
| `Eventually(...).Should(Succeed())` 重试断言 | k3s `Eventually` + Gomega | 如果不引 Ginkgo，可以写一个轻量 `Eventually(t, fn, timeout, interval)`；引 testify 也可以用 `assert.Eventually`。 |
| `trap cleanup EXIT` 兜底清理 | innernet [`run-docker-tests.sh`](https://github.com/tonarino/innernet/blob/main/docker-tests/run-docker-tests.sh) | 给我们已有的 `scripts/test-fixture-up.sh` / `down.sh` 加上 `trap`，CI 中断时也能清干净。 |
| 独立 `ci/` 子模块跑真凭据 e2e | gluetun [`ci/`](https://github.com/qdm12/gluetun/tree/master/ci) | 把「带真 sing-box 上游」「带真 ifconfig.me 探测」的用例放进 `e2e/ci-runner/`，与主 `go.mod` 解耦，`environment: secrets` 注凭据。 |
| testdata 目录 + CopyToTempDir | devpod [`e2e/framework/util.go`](https://github.com/loft-sh/devpod/tree/main/e2e/framework) | `e2e/testdata/<case>/` 放最小 control-plane 配置、出口列表、Docker compose override，跑用例时复制到 tempdir。 |
| 端口分配靠 `const` 偏移 | sing-box `serverPort uint16 = 10000 + iota` | 简单粗暴够用，不必引入端口池库。 |
| `goleak.VerifyTestMain` | sing-box [`box_test.go`](https://github.com/SagerNet/sing-box/blob/dev-next/test/box_test.go) | 测控制面长连接 / sing-box 子进程时帮我们抓 goroutine 泄漏。 |

### 4.2 需要改造（思路抄但实现要适配）

| 做法 | 改造方向 |
| --- | --- |
| nomad 「测试不起集群、只验证」 | 我们也可以让 `make e2e` 先 `docker compose up -d` 起栈，Go 测试只通过 HTTP/docker exec 验证。但因为 host-agent 要操作 nftables，需要在 CI 上准备好特权 daemon——hosted runner 可以做到，但要加 `--privileged` 之类。 |
| headscale 多版本矩阵 (`Versions`) | 我们暂时不需要多控制面版本，但需要「多用户镜像版本（含/不含 sing-box 客户端）」的矩阵，沿用 `Versions` 字段思路即可。 |
| coder Playwright web 终端断言 | 我们的后台是 React，可以用 Playwright 但**只**覆盖「后台 CRUD → 拿到 curl 入口」那部分，SSH 流程不要绕 web 终端走，仍然走真 SSH。 |
| ufw-docker bach 单元 | 我们已有 BATS 烟雾，不引 bach；但「mock 掉 iptables/docker 让脚本可单测」的思路可以用到 `scripts/` 里的 shell。 |
| k3s Vagrant 多 OS 矩阵 | v1 只支持 Linux 单机，不做 Vagrant；但 `VagrantNode.RunCmdOnNode` 这层封装可以化作 `Container.Exec(cmd)`，把多种「在节点上跑命令」的差异隐藏掉。 |
| nomad e2e/framework 自定义 suite | 不建议自己写 framework；优先用原生 `go test` + 一个 `e2e/scenario.go`，复杂度不够回报。 |

---

## 5. 后续动作建议（仅供 team-lead 参考，不直接执行）

1. 在 `e2e/` 下落地一个最小骨架（`scenario.go` + 一个 `basic_ssh_test.go` + 一个 `egress_ip_test.go`），先验证「能在 GitHub Actions hosted runner 上跑通」。
2. 把目前 `scripts/smoke/` 的 BATS 用例保留为 fast smoke，e2e 走 Go 测试做 deeper coverage，两套互补。
3. 「sing-box 真上游」「真 ifconfig.me 出口验证」做成独立 `e2e/ci-runner/`，PR 不跑，main 推送或 release 才跑。
4. 网络隔离用模式 C：在 `e2e/` 拓扑里加一个 `probe` 容器，对受测用户容器做主动连通性探测。

---

## 6. 主要来源

- https://github.com/juanfont/headscale/tree/main/integration
- https://github.com/juanfont/headscale/blob/main/integration/scenario.go
- https://github.com/juanfont/headscale/blob/main/integration/ssh_test.go
- https://github.com/qdm12/gluetun/tree/master/ci
- https://github.com/qdm12/gluetun/blob/master/ci/internal/simple.go
- https://github.com/qdm12/gluetun/blob/master/.github/workflows/ci.yml
- https://github.com/SagerNet/sing-box/tree/dev-next/test
- https://github.com/SagerNet/sing-box/blob/dev-next/test/box_test.go
- https://github.com/SagerNet/sing-box/blob/dev-next/test/docker_test.go
- https://github.com/SagerNet/sing-box/blob/dev-next/test/shadowsocks_test.go
- https://github.com/loft-sh/devpod/tree/main/e2e
- https://github.com/loft-sh/devpod/blob/main/e2e/e2e_suite_test.go
- https://github.com/loft-sh/devpod/blob/main/e2e/tests/ssh/ssh.go
- https://github.com/coder/coder/tree/main/site/e2e
- https://github.com/coder/coder/blob/main/site/e2e/tests/webTerminal.spec.ts
- https://github.com/chaifeng/ufw-docker/blob/master/test/ufw-docker.test.sh
- https://github.com/chaifeng/ufw-docker/blob/master/Vagrantfile
- https://github.com/chaifeng/ufw-docker/blob/master/test-vagrant.sh
- https://github.com/hashicorp/nomad/tree/main/e2e
- https://github.com/hashicorp/nomad/blob/main/e2e/networking/networking.go
- https://github.com/k3s-io/k3s/tree/master/tests/e2e
- https://github.com/k3s-io/k3s/blob/master/tests/e2e/testutils.go
- https://github.com/k3s-io/k3s/blob/master/tests/e2e/validatecluster/validatecluster_test.go
- https://github.com/tonarino/innernet/tree/main/docker-tests
- https://github.com/tonarino/innernet/blob/main/docker-tests/run-docker-tests.sh
