---
status: needs_attention
batch: 1
scope: internal/network
files_reviewed: 21
date: 2026-05-10
findings:
  blocker: 1
  critical: 3
  warning: 6
  info: 4
  total: 14
---

# 批次 1 — `internal/network` Review

## 0. 总评

`internal/network` 是项目最关键的安全边界,也是问题最集中的模块。一句话总结:**承载着两套并行实现 — 一套(更安全的 SingBoxProvider)是死代码,一套(实际生效的 ContainerProxyProvider)在边界处偷工减料**。

| 维度 | 评价 |
|---|---|
| 架构 | 端口适配器抽象方向正确,但 Provider interface 只剩 1 个真实现,工厂层多余且死代码占比高 (~35%) |
| 健壮性 | shell 脚本生成出网配置、字符串匹配判断 sing-box 健康、`time.Sleep` 等待 — 多处脆弱 |
| 可拓展性 | netlink/nftables 的低层抽象做得不错,但配置硬编码 `10.99.x.x` 等让"换网段"几乎不可能 |
| 安全 | 当前活路径下,DNS 控制依赖 sing-box hijack 而非显式注入,有启动时序窗口风险 |
| 优雅 | 同一算法两份(`mgmtSubnetIndex` × 2)、字符串拼接 shell、错误号优先级耦合在 `verify.go` 中 |

---

## 1. 🔴 BLOCKER — 多租户场景下 `teardownPortForwarding` 会清空所有 host 的端口规则

`internal/network/host_forwarding_linux.go:156`

```go
func teardownPortForwarding(ctx context.Context) {
    // 直接 flush + delete 整个 CLOUDPROXY-PORTMAP chain
    exec.CommandContext(ctx, "iptables", "-t", "nat", "-F", "CLOUDPROXY-PORTMAP").Run()
    exec.CommandContext(ctx, "iptables", "-t", "nat", "-X", "CLOUDPROXY-PORTMAP").Run()
    exec.CommandContext(ctx, "iptables", "-F", "CLOUDPROXY-PORTMAP").Run()
    exec.CommandContext(ctx, "iptables", "-X", "CLOUDPROXY-PORTMAP").Run()
}
```

`ContainerProxyProvider.teardownGateway` (container_proxy_provider.go:166) 调用此函数时**不传 hostID**,直接整链 flush。

**实际后果**:任意一个 host 重启/删除,会**整体抹掉所有其他 host 的 DNAT 与 FORWARD 规则**。多租户下端口映射会随机失效。

**应当**: 按 hostID 精确删除该 host 的 rules (用注释或 mark 标记),或为每个 host 建独立子链。

---

## 2. 🔴 CRITICAL — DNS 路径与设计文档严重脱节,有启动时序泄漏窗口

文档承诺:"全流量必须走指定出口 IP,不能出现 DNS、WebRTC 或其他类型的直接泄漏。"

实际活路径 (`ContainerProxyProvider.tryConfigureWorkerEgress` line 329):

```go
echo 'nameserver 8.8.8.8' > /etc/resolv.conf
```

**忽略**了 `EgressConfig.Proxy.DNSServer` 字段,硬编码 `8.8.8.8`。

此处虽然由 sing-box gateway 的 DNS hijack rule 拦截 (gateway_singbox_config.go:43 `{"port": 53, "action": "hijack-dns"}`),所以**正常运行时**没有真泄漏。但:

- **启动时序**: container_proxy_provider.go 的流程是 disconnect bridge → sleep 1s → configureWorkerEgress → setupPortForwarding。如果 worker 进程在网络就绪前发起 DNS,且 gateway 的 sing-box 拦截还未完全启动,DNS 直发 8.8.8.8 会经隧道之外的路径 (理论上 bridge 已断,但路由切换有竞态)。
- **配置-实际不一致**: 死代码 `SingBoxProvider` + `ConfigureContainerDNS` (dns.go) 是**显式写隧道 DNS**的正确实现 — 但没人调用。
- **代码可读性误导**: 8.8.8.8 在 worker 视角是真 DNS,实际靠 hijack。一旦 hijack 配置丢失或被误改,全员立即向 8.8.8.8 暴露 hostname。

**应当**: 把 `spec.Egress.Proxy.DNSServer` 真正写入 worker 的 `/etc/resolv.conf`(走 dns.go::ConfigureContainerDNS 的方式),让"配置即真相"。

---

## 3. 🔴 CRITICAL — 大量死代码假装是核心架构

模块里同时存在三个 Provider 实现:

| Provider | 位置 | 调用方 |
|---|---|---|
| `ContainerProxyProvider` | container_proxy_provider.go | ✅ provider_linux.go / provider_other.go |
| `SingBoxProvider` | singbox_provider_linux.go | ❌ 仅自身和 routing_provider |
| `RoutingProvider` | routing_provider_linux.go | ❌ 完全无外部调用 |

`SingBoxProvider` 自称是"更严格的"实现 (容器内 nftables default-deny + tun + 显式 DNS),自带 ~430 行 + 依赖 `namespace.go` (200 行) + `firewall_proxy.go` (130 行) + `firewall_helpers.go` (155 行) + `dns.go` (30 行) + `verify.go` (170 行 — 它的几个验证函数也只在 SingBoxProvider 中调用)。

**估算**:这一整套死路径占 `internal/network` 约 1100 行 (~32%)。

**问题**:
- 维护者要在两套并行模型间做心智切换 — 容易把"应该是这样"和"实际是那样"搞混
- review 一开始我就被这套结构误导,以为系统真的有 nftables default-deny
- 删除时阻力大,因为它"看起来很对"

**应当**: 决定一条路。要么 (a) 把 SingBoxProvider 设为唯一路径(更安全,推荐),retire ContainerProxyProvider;要么 (b) 删除 SingBoxProvider/RoutingProvider/namespace.go/firewall_proxy.go/firewall_helpers.go/dns.go/verify.go 全套死代码。

---

## 4. 🔴 CRITICAL — `ssh_host` 元数据可能引用一个不存在的 IP

`internal/runtime/tasks/ssh_handoff.go:9`

```go
access := network.DeriveManagementSSHAccess(hostID)  // 返回 10.99.{block+1}.{offset+2}
return map[string]any{"ssh_host": access.Host, ...}
```

`DeriveManagementSSHAccess` (ssh_access.go) 用与 `InjectManagementVeth` 相同的算法计算 IP。但当前活路径 `ContainerProxyProvider` 不会注入 mgmt veth — 容器接口 IP 是 `10.99.{third}.3` 的 docker network 地址,**和 mgmt 算法的网段完全不一样**(mgmt 用 `block+1` 而 third 是 fnv hash 出来的 20-219)。

**结果**: `BuildSSHHandoffMetadata` 输出的 `ssh_host` 可能是一个**没有任何接口绑定的 IP**。

下游 (entry / bootstrap / sshproxy) 用这个值作为连接目标会失败 — 也可能下游另有逻辑从 docker inspect 拿真实 IP,这套 metadata 只是表面装饰。无论哪种,都是**契约错位**。

**应当**: 立即追踪 `ssh_host` 实际消费方 (entry handoff、sshproxy、cloud-claude CLI),决定是从 docker inspect 拿真实 IP,还是把这个函数一并删掉。

---

## 5. 🟡 WARNING — `mgmtSubnetIndex` 算法的运算符优先级很可能不是作者本意

`namespace.go:175-183` 与 `ssh_access.go:37-45`(同一算法两份):

```go
return binary.BigEndian.Uint16(b[:2]) ^ binary.BigEndian.Uint16(b[2:4])%16382
```

Go 中 `%` 比 `^` 优先级高,实际计算是:

```
A XOR (B % 16382)
```

而看起来作者想表达的可能是 `(A XOR B) % 16382`(为了把结果限制在子网地址空间)。当前形式下 XOR 的高位可能溢出 uint16 不会发生,但**结果不会被 16382 限制**,可能落到合法范围之外的子网偏移上。

**应当**: 加括号显式表达意图;一个测试就能判定哪个是真意。

---

## 6. 🟡 WARNING — 同一个算法同时存在两份实现

`namespace.go::mgmtSubnetIndex` 和 `ssh_access.go::mgmtSubnetIndexFromID` **完全相同**,理由是 `namespace.go` 有 `//go:build linux` tag。

注释里也承认:"Mirrors the algorithm in namespace.go (mgmtSubnetIndex) so the two remain consistent without requiring the linux-only build tag."

**应当**: 把算法移到 `ssh_access.go` (无 build tag),让 namespace.go 调用它。维护成本立刻减半。

---

## 7. 🟡 WARNING — Gateway 健康检查靠 `docker logs` 字符串匹配

`container_proxy_provider.go:269`

```go
if strings.Contains(s, "FATAL") || strings.Contains(s, "panic:") {
    return fmt.Errorf("gateway sing-box failed: %s", ...)
}
```

把 `docker inspect .State.Running == true` + log 字符串扫描 当成 health 信号:

- sing-box 升级一次 log 格式可能就失效
- log 大量增长时 `docker logs --tail 120` 截断可能丢掉 FATAL 行
- 没有 sing-box 内部 ready 信号 (例如端口探活)

**应当**: 用 `nc -z gw_ip 7892` 或 sing-box 自身的 health 端点判断 ready,而不是 grep log。

---

## 8. 🟡 WARNING — Worker 内出网配置走 shell 脚本字符串拼接

`container_proxy_provider.go:303-330` `tryConfigureWorkerEgress`:

```go
script := fmt.Sprintf(`set -e
...
ip route add default via %s dev "$DEV" metric 0
...
echo 'nameserver 8.8.8.8' > /etc/resolv.conf
`, workerIP, workerIP, bridgeGW, bridgeGW)
cmd := exec.CommandContext(ctx, "docker", "exec", workerName, "sh", "-c", script)
```

这些 IP 由代码生成,**当前**没有注入风险。但:

- 模式上是 `fmt.Sprintf` → `sh -c` 的脚本拼接,只要将来某个字段来自 DB / 配置 / 用户输入,立即变成命令注入面
- 单个脚本既做"等接口就绪"又做"删默认路由"又做"加默认路由"又做"写 resolv.conf",失败时无法定位哪一步
- 重试机制是整段脚本重跑(configureWorkerEgress 包了 3 次),partial-state 会留垃圾

**应当**: 换成多步 `docker exec`(每步一个 ip 命令);或把脚本封装成镜像里的 `setup-egress.sh`,只传位置参数。

---

## 9. 🟡 WARNING — `CleanupHost` 完全吞掉错误

`container_proxy_provider.go:155-175`

```go
func (p *ContainerProxyProvider) CleanupHost(ctx context.Context, spec HostNetworkSpec) error {
    p.teardownGateway(ctx, spec.HostID)
    return nil  // 永远不返回错误
}

func (p *ContainerProxyProvider) teardownGateway(...) {
    teardownPortForwarding(ctx)
    _ = exec.CommandContext(...).Run()
    _ = exec.CommandContext(...).Run()
    _ = exec.CommandContext(...).Run()
    _ = exec.CommandContext(...).Run()
    _ = os.RemoveAll(gatewayConfigDir(hostID))
}
```

所有 docker / fs 操作都 `_ = ...Run()`。资源泄漏不会被察觉,孤儿 docker network、孤儿 gateway 容器、孤儿配置目录会随时间堆积。

**应当**: 至少把错误日志化(p.logger.Warn),并在 cleanup 全失败时返回汇总错误。CleanupHost 不能完全抑制 — 调用方需要知道是不是要重试。

---

## 10. 🟡 WARNING — 子网哈希分配不检测冲突

`container_proxy_provider.go:204-208`

```go
func subnetThirdOctet(hostID string) int {
    h := fnv.New32a()
    _, _ = h.Write([]byte(hostID))
    return int(h.Sum32()%200) + 20
}
```

200 个槽位,生日碰撞概率非可忽略 (n=18 时 ~50%)。两个 host 哈希到同一第三段会:
- `dockerNetworkCreate` 因 subnet 已存在而报错
- 无回退,直接 PrepareHost 失败

**应当**: 落库一张 `host_subnet_assignments` 表,或在 docker network 命名失败后线性探测下一个槽位。

---

## 11. 🟡 WARNING — 关键参数硬编码,无配置入口

| 项目 | 位置 | 影响 |
|---|---|---|
| `10.99.0.0/16` 子网 | container_proxy_provider.go, host_forwarding_linux.go | 与用户既有网络冲突时无法切换 |
| `8.8.8.8` 默认 DNS | container_proxy_provider.go:329 | 已在 #2 讨论 |
| `gatewayTPProxyPort = 7892` | container_proxy_provider.go:16 | 不可改 |
| `1 second sleep` | container_proxy_provider.go:108 | 慢机器 / 高负载可能不够 |
| `20s gateway healthy timeout` | container_proxy_provider.go:262 | 快机器浪费、慢机器不够 |
| `gateway image = "...:local"` | container_proxy_provider.go:181 | 默认值是开发镜像,生产用要靠环境变量 |

**应当**: 集中到一个 `network.Config` 结构,从配置文件 / env 读取,提供合理默认。

---

## 12. ⚪ INFO — Provider interface 抽象是过度设计

```go
type Provider interface {
    PrepareHost(context.Context, HostNetworkSpec) error
    CleanupHost(context.Context, HostNetworkSpec) error
}
```

加上一个 `provider_factory.go` 一行函数 `func NewProvider(...) Provider { return newLinuxProvider(...) }`。

实际只有一个真实现 `ContainerProxyProvider`(SingBoxProvider 是死代码,见 #3)。这个接口的存在没有给 (a) 测试 mock (b) 多实现切换 提供任何收益,反而引入了一层 indirection。

**应当**: 删除 interface,直接用 `*ContainerProxyProvider`。当真有第二个实现时再抽。

---

## 13. ⚪ INFO — `verify.go::firstNetworkError` 把错误优先级硬编码在 verify 模块

`verify.go:122` 把 "egress > DNS > leak" 的优先级写死在 verify 内部。这种业务策略应该在调用方决定;verify 应该只产出结构化结果,优先级由 controlplane / scheduler 选择。

(注:此函数仅在死代码 SingBoxProvider 中使用。如果按 #3 结论删除 SingBoxProvider 整套,这个函数也跟着走。)

---

## 14. ⚪ INFO — 端口映射 setup 不是幂等

`setupPortForwarding` 直接 `iptables -A`,没有 `-C` 检查。重复调用会创建重复规则。

**应当**: 配套的 ensureChainHook 已经做了 `-C` 检查,这里也应该一致。

---

## 15. ⚪ INFO — 模块对 `os.Hostname()` 当成 control-plane container ID

`container_proxy_provider.go:136-141`,`teardownGateway` 用 `os.Hostname()` 拿到的字符串当 docker container 名,把控制面也接到隔离网络上(为了 VNC)。

如果运行环境 hostname 不是 docker 容器的 short ID(例如在主机原生进程中),这一步默默失败(被 `_ =` 吞)。隐式依赖一个非常具体的部署假设,但代码没声明也没断言。

**应当**: 显式传入 control-plane container 名(从配置或 env 读取),不依赖 hostname 巧合相等。

---

## 后续批次的视野(已记录,不深挖)

- **批次 3 (runtime/agent)**: 验证 `BuildSSHHandoffMetadata` 的下游消费方,确认 #4 是真错位还是死装饰
- **批次 5 (sshproxy)**: 确认 SSH 代理实际怎么定位容器(docker inspect / mgmt IP / docker network IP)
- **批次 6 (前端)**: 端口映射是否真的多 host 同时使用过 — 如果生产从未触发 #1,可能症状一直没暴露

## 修复优先级建议

1. **立即**: #1 (多租户端口规则误删) — 一行函数签名修改,但要重写整个 cleanup 逻辑
2. **本里程碑内**: #2 (DNS 显式化)、#4 (ssh_host 验证)
3. **下个里程碑**: #3 (死代码清理) — 大动作,需要决策 retire 哪条路径
4. **持续**: #5-#15 — 都是局部修复,按团队节奏
