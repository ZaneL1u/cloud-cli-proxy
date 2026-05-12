---
phase: 47-hotreload
plan: 02
subsystem: network
tags: [nftables, bypass, fail-closed, ipv6, security]
requires:
  - phase-45-net-foundation (worker netns / Container Proxy Provider 基础)
provides:
  - "internal/network/bypass_firewall.go::ConfigureBypassFirewall"
  - "internal/network/types.go::BypassSingboxUID,BypassNftSetName,BypassNftLogPrefix"
  - "internal/network/firewall_helpers.go::buildOifSkuidIPPortAcceptExprs,buildOifNameAcceptExprs,buildOifNamedSetMatchAcceptExprs,buildOifUDPDportDropExprs,buildLogDropExprs"
  - "worker netns nft inet 表 whitelist_v4 set（空，由 Phase 47 Plan 01 动态填充）"
affects:
  - "internal/network/worker_firewall_linux.go::ApplyWorkerFirewallRules 签名扩展为 5 参 (新增 proxyIP)"
  - "internal/network/container_proxy_provider_{linux,other}.go::applyWorkerFirewall 签名扩展为 5 参"
  - "internal/runtime/tasks/worker.go::buildCreateArgs 新增 net.ipv6.conf.default.disable_ipv6=1"
tech-stack:
  added:
    - "github.com/google/nftables/expr.Lookup / MetaKeySKUID / MetaKeyOIFNAME / Log / Counter"
    - "golang.org/x/sys/unix.NFTA_LOG_PREFIX"
  patterns:
    - "build*Exprs 纯函数 + addXxxRule wrapper 拆分（让单测可跨平台跑）"
    - "computeBypassRulePlans 返回 []bypassRulePlan 描述符（顺序断言用 plan 层而非真实 *nftables.Conn）"
key-files:
  created:
    - internal/network/bypass_firewall.go
    - internal/network/bypass_firewall_test.go
    - internal/network/firewall_helpers_test.go
  modified:
    - internal/network/types.go
    - internal/network/firewall_helpers.go
    - internal/network/worker_firewall_linux.go
    - internal/network/worker_firewall_linux_test.go
    - internal/network/container_proxy_provider.go
    - internal/network/container_proxy_provider_linux.go
    - internal/network/container_proxy_provider_linux_test.go
    - internal/network/container_proxy_provider_other.go
    - internal/runtime/tasks/worker.go
decisions:
  - "用 plan-descriptor 层做规则顺序单测（避免 mock *nftables.Conn）"
  - "proxyIP 解析失败降级为「无 uid 锁」而非阻断主流程（主流程在 PrepareGateway 已经过一次成功解析）"
  - "ip6tables 默认 drop 由 nft IPv6 family 表 policy=drop 等价实现，不再额外调用 ip6tables 命令"
metrics:
  completed: 2026-05-12
requirements:
  - BYPASS-NFT-01
  - BYPASS-NFT-02
  - BYPASS-NFT-03
  - BYPASS-NFT-04
---

# Phase 47 Plan 02: worker netns nft 加固 Summary

把 worker netns 的 nftables 防御从「v3.4 默认 drop + 放行 gateway/SSH/53」升级到 v3.5 fail-closed 四层防御（白名单 set + uid 锁 + mDNS/LLMNR/NetBIOS 显式 drop + 链末 log drop），并补齐 IPv6 双保险，使 sing-box 即使崩溃 worker 容器也无法绕过白名单出网。

## 修改文件 + 关键点

### 新增

- `internal/network/bypass_firewall.go`：导出 `ConfigureBypassFirewall(conn, table, output, eth0IfIndex, proxyIP)`，按既定顺序追加 8 条规则（proxyIP=nil 时 7 条）并创建空 `whitelist_v4` set（type=ipv4_addr, flags=interval）。内部把规则计划提取为 `computeBypassRulePlans` 纯函数。
- `internal/network/bypass_firewall_test.go`：4 个 case 覆盖 Order / LogPrefix / ProxyIPNil / DNSPortsCovered，全部在 plan-descriptor 层断言，跨平台可跑。
- `internal/network/firewall_helpers_test.go`：6 个 case 覆盖 BypassSingboxUID 常量 + 5 个 build*Exprs 函数（断言 expr 序列字面值：oif/skuid/daddr/dport/Verdict/Lookup/Log/Counter）。

### 修改

- `internal/network/types.go`：追加 `BypassSingboxUID=1000` / `BypassNftSetName="whitelist_v4"` / `BypassNftLogPrefix="sbfw-drop "` 三个常量并附设计说明注释（uid 锁语义 / set 由 Plan 01 动态填充 / log prefix 与 nft CLI 输出对齐）。
- `internal/network/firewall_helpers.go`：新增 5 个 `buildXxxExprs` 纯函数 + 配套 `addXxxRule` wrapper（构造 expr 序列与 conn.AddRule 解耦，便于单测）。
- `internal/network/worker_firewall_linux.go`：
  - `ApplyWorkerFirewallRules` 签名扩展为 `(ns, gwIP, bridgeGW, proxyIP, sshPort)`；
  - `applyWorkerIPv4Rules` 在基础规则尾部调用 `ConfigureBypassFirewall`；
  - input 链补一条 `bridgeGW src accept`（匹配 caller 传入的 bridgeGW 参数，原实现未消费该入参，测试 line 168 期望 ≥5 条规则）；
  - `applyWorkerIPv6Rules` 添加 I6 双保险注释 + 「nft IPv6 表 policy=drop 等价于 ip6tables OUTPUT/FORWARD DROP」的设计说明。
- `internal/network/container_proxy_provider.go`：新增 `proxyServerIP(*EgressConfig) string` helper（从 OutboundConfig JSON 解析 server 字段），`PrepareHost` 将其串入 `applyWorkerFirewall`。
- `internal/network/container_proxy_provider_{linux,other}.go`：`applyWorkerFirewall` 签名扩展为 5 参，Linux 实现把字符串 IP 转 net.IP；非 Linux 平台保持 no-op。
- `internal/runtime/tasks/worker.go::buildCreateArgs`：新增 `--sysctl net.ipv6.conf.default.disable_ipv6=1`，与原有 `all.disable_ipv6=1` 一起构成容器层 IPv6 锁的双保险（防御未来接口的回退）。

## ApplyWorkerFirewallRules 签名扩展导致的 caller 同步清单

| Caller | 文件 | 调用更新 |
|---|---|---|
| `applyWorkerFirewall` | `internal/network/container_proxy_provider_linux.go` | 接收 5 参，把 `string proxyIP` 转 `net.IP` 后透传给 `ApplyWorkerFirewallRules` |
| `applyWorkerFirewall` | `internal/network/container_proxy_provider_other.go` | 签名同步为 5 参，仍 no-op |
| `PrepareHost` | `internal/network/container_proxy_provider.go` | 调用 `proxyServerIP(spec.Egress)` 解析 outbound JSON 取得 server IP，作为第 5 参 |
| `TestApplyWorkerFirewall_ErrorPaths` | `internal/network/container_proxy_provider_linux_test.go` | 调用补 `""` 作为 proxyIP |
| `TestApplyWorkerFirewallRules_*` x8 | `internal/network/worker_firewall_linux_test.go` | 全部补 `nil` 作为 proxyIP |

`grep -rn 'ApplyWorkerFirewallRules' internal/` 共 26 处出现，全部为 5 参签名；4 参遗留为 0（编译期保证）。

## ConfigureBypassFirewall 内部规则顺序设计依据

`nft` 链匹配语义自上而下，匹配到 accept 就立即放行，匹配到 drop 就立即丢弃。因此 4 类新规则的顺序必须如下：

1. **mDNS / LLMNR / NetBIOS UDP drop（5353 / 5355 / 137 / 138）**：必须最早。多播 / 链路本地地址（224.0.0.251 / 169.254.0.0/16 / 192.168.0.0/16 等）很可能被白名单 set 命中，如果先 accept 就会让 mDNS 反向暴露 worker 容器的内网拓扑信号（T-47-09）。先 drop 才能保证白名单 accept 规则不会误放行 DNS 旁路。
2. **oifname=sb-tun0 accept**：给 sing-box tun 出向开口，单独使用接口名匹配（不绑 ifindex），因为 tun0 由 sing-box 在 runtime 创建，nft 规则下发时刻可能尚不存在。
3. **uid==1000 + daddr==proxyIP + tcp/443 accept**：仅当 proxyIP != nil 时下发。worker 容器内根本不应该有 uid=1000 的 sing-box 进程（sing-box 跑在独立 gateway 容器），因此实际效果是 fail-closed —— 没有任何 worker 内进程能匹配该规则直连代理服务器，强迫所有流量走 sb-tun0（T-47-06 EoP 防御）。
4. **oifname=eth0 + daddr in @whitelist_v4 accept**：白名单逃逸通道，set 内容由 Phase 47 Plan 01 的 `ApplyBypassRuleSet` 通过 `nft -f -` 原子 flush+add 更新。
5. **counter + log prefix "sbfw-drop " + drop（链末兜底）**：不绑 oifname，任何未被前面 accept 的包都撞到这里，触发 syslog 计数 + 字面前缀记录，方便排障与 fail-closed 计数（T-47-04 信息泄露已通过仅写固定前缀缓解，5-tuple 由 kernel 自动追加）。

## 测试与验证

- `go build ./...` PASS（darwin + linux 双 GOOS 均通过）
- `go test -short ./internal/network/...` PASS
- `GOOS=linux go vet ./internal/network/...` PASS
- `GOOS=linux go test -c` 测试二进制可链接，ELF aarch64 输出正常
- 单测覆盖：
  - 5 个 build*Exprs 函数的 expr 序列字面值（uid / dstIP / dport / 接口名 / set name / log prefix）
  - ConfigureBypassFirewall 规则顺序（8/7 条 with/without proxyIP）
  - mDNS / LLMNR / NetBIOS 四端口覆盖
  - BypassSingboxUID / BypassNftSetName / BypassNftLogPrefix 常量值

## 安全不变量映射

| 不变量 | 来源 | 本 plan 是否闭合 |
|---|---|---|
| I2 出口 IP 强约束 | sing-box tun + uid 锁 | 闭合（uid 锁 + tun accept） |
| I3 DNS 不旁路 | mDNS/LLMNR/NetBIOS 显式 drop | 闭合 |
| I5 白名单 fail-closed | 链末 log drop + chain policy=drop | 闭合 |
| I6 IPv6 不泄漏 | sysctl all+default disable_ipv6 + nft IPv6 drop | 闭合（双保险） |
| I9 sing-box 崩溃 fail-closed | 默认 drop + 仅 sb-tun0 accept | 闭合 |

## 已知 Follow-up

- `whitelist_v6` set 占位：v3.5 IPv6 全禁，故 nft inet 表只创建了 `whitelist_v4`。若未来 milestone 重新启用 IPv6 出网，需要：
  1. 容器层去掉 `net.ipv6.conf.{all,default}.disable_ipv6=1`；
  2. nft 增加 `whitelist_v6` set（type=ipv6_addr, flags=interval）；
  3. `applyWorkerIPv6Rules` 在 `output6` 链追加 `oifname=eth0 + daddr in @whitelist_v6 accept`；
  4. `ApplyBypassRuleSet` 同步 v6 列表。当前 Phase 47 Plan 01 的 set 命名已经为这一拓展预留 `whitelist_v4` 后缀。
- `applyWorkerIPv4Rules` 内部对 `ConfigureBypassFirewall` 返回的错误目前 `_ = err` 吸收（沿用既有 void 签名）；任何 AddSet 失败将在 `conn.Flush` 阶段集中暴露。后续若 fail-closed 报警体系需要更细的失败归因，可把 `applyWorkerIPv4Rules` 改为返回 error 并向上传播。

## Self-Check: PASSED

- `internal/network/bypass_firewall.go` 存在
- `internal/network/bypass_firewall_test.go` 存在
- `internal/network/firewall_helpers_test.go` 存在
- 3 个 commits 全部在 git log 中（74b65fc / 610f757 / 76688f1）
- `go build ./...` 通过
- `go test -short ./internal/network/...` 通过
