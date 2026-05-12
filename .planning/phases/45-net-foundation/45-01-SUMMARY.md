---
phase: 45-net-foundation
plan: 01
subsystem: network
tags: [sing-box, rule-set, dns-split, tun-hardening, gateway]
provides:
  - sing-box-v3.5-render-skeleton
  - rule-set-placeholder-mount
  - split-dns-config
requires:
  - sing-box>=1.11 (镜像内实测 1.13.3)
affects:
  - internal/network/gateway_singbox_config.go
  - internal/network/gateway_singbox_config_test.go
  - internal/network/container_proxy_provider.go
tech-stack:
  added: []
  patterns: ["v3.5 两段式渲染：静态 config.json + 动态 rule-set placeholder", "拆分 DNS（dns-local + dns-proxy DoH）"]
key-files:
  created: []
  modified:
    - internal/network/gateway_singbox_config.go
    - internal/network/gateway_singbox_config_test.go
    - internal/network/container_proxy_provider.go
decisions:
  - "tag 保留 'direct'（不改 'direct-out'）以兼容现有断言"
  - "dnsServer 形参保留（Plan 45-02 才动签名），函数体改用固定 1.1.1.1 + DoH"
  - "stack 从 mixed 升级为 system（配合 strict_route/EIN）"
metrics:
  duration: 约 35 分钟
  tasks_completed: 3/3
  files_modified: 3
  commits: 3
  completed_at: 2026-05-12
requirements_satisfied:
  - BYPASS-NET-01
  - BYPASS-NET-02
  - BYPASS-NET-03
  - BYPASS-NET-04
  - BYPASS-DNS-01
  - BYPASS-DNS-02
---

# Phase 45 Plan 01: 网络配置基础与数据模型 — 渲染层与 placeholder Summary

## One-liner

把 sing-box gateway 渲染层升级到 v3.5 两段式骨架（rule_set + 7 条 route.rules + 拆分 DNS + tun 三参加固 + default_interface=eth0），并在 PrepareHost 主流程里生成空 rule-set placeholder 文件且 `:ro` 挂入 gateway 容器，为 Phase 47 的动态白名单热更新备好挂点。

## 镜像勘察

| 维度 | 实测值 | 备注 |
|------|--------|------|
| 镜像 | `cloud-cli-proxy-sing-gateway:local` (ID `56f57e5ae5c7`) | 通过 `docker images` 确认存在 |
| sing-box 版本 | **1.13.3** | `docker run --rm --entrypoint=sing-box <image> version` 输出 |
| 编译环境 | go1.25.8 linux/arm64 | sing-box 自带 |
| 关键 tag | `with_gvisor / with_quic / with_dhcp / with_wireguard / with_utls / with_clash_api / with_tailscale` | 满足 tun + DoH + rule-set 全部需要 |

结论：版本 1.13.3 ≥ 1.11，`route.rule_set[type=local, format=source]` 与 v3 schema placeholder 全部原生支持。Open Question Q1 关闭。

## 渲染输出关键字段 diff 摘要

| 维度 | 旧（Phase 45 前） | 新（Plan 45-01 后） |
|------|------------------|---------------------|
| `route.rules` 条数 | 2（proxy_ip/32 + port=53 hijack-dns） | **6**（sniff + hijack-dns + proxy_ip/32 + ip_is_private + 两个 rule_set） |
| `route.rule_set` | 不存在 | **2 条** local 引用（whitelist-cidrs / whitelist-domains，format=source） |
| `route.default_interface` | 缺失，依赖 `auto_detect_interface=true` | **显式 `"eth0"`**；`auto_detect_interface` 已移除 |
| `route.final` | `proxy-out` | `proxy-out`（不变） |
| `dns.servers` | 1 条 `dns-remote`（type=tcp, server=$dnsServer, detour=proxy-out） | **2 条**：`dns-local`（type=local）+ `dns-proxy`（type=https, server=1.1.1.1, detour=proxy-out, domain_resolver=dns-local） |
| `dns.rules` | 不存在 | 2 条：`.lan/.local/.internal → dns-local`；`whitelist-domains → dns-proxy` |
| `dns.final` | 不存在 | `dns-proxy` |
| `dns.strategy` | `ipv4_only` | `ipv4_only`（不变） |
| `inbounds[0].strict_route` | `false` | **`true`** |
| `inbounds[0].endpoint_independent_nat` | 不存在 | **`true`** |
| `inbounds[0].stack` | `mixed` | **`system`** |
| `inbounds[0].sniff_override_destination` | `true`（已删除） | 由 `route.rules[0] action=sniff` 替代 |
| `outbounds[direct].bind_interface` | 不存在 | **`eth0`**（双重保险防 auto_route 回环） |
| `outbounds[direct].tag` | `direct` | `direct`（不变，明确不改为 `direct-out`） |
| Sniff/Hijack 形态 | `port=53 + action=hijack-dns` | `action=sniff (tls/http/quic/dns)` + `protocol=dns + action=hijack-dns` |

## Tasks

### Task 1: 镜像版本预检 + 常量与渲染骨架重构

**Commit:** `370b14b`

- 镜像版本预检：sing-box 1.13.3，满足 ≥ 1.11 门槛。
- 新增包级常量：`ruleSetWhitelistCIDRsName` / `ruleSetWhitelistDomainsName` / `ruleSetWhitelistCIDRsPath` / `ruleSetWhitelistDomainsPath` / `ruleSetPlaceholder`（v3 空规则集字面量）。
- 重构 `buildGatewaySingBoxConfig` 为骨架：拆出 5 个 helper（`buildGatewayTunInbound` / `buildGatewayDirectOutbound` / `buildGatewayDNS` / `buildGatewayRouteRules` / `buildGatewayRouteRuleSet`），主体只负责拼装。
- tun inbound：`strict_route=true`、`endpoint_independent_nat=true`、`stack=system`、`sniff=true`，移除 `sniff_override_destination`。
- direct outbound：`bind_interface=eth0` 显式声明；tag 保留 `direct`。
- DNS：dns-local + dns-proxy（DoH/1.1.1.1/detour=proxy-out/domain_resolver=dns-local），rules 内网后缀 → dns-local，rule_set → dns-proxy；final=dns-proxy；strategy=ipv4_only。
- route：`default_interface=eth0` 显式替代旧 `auto_detect_interface`；6 条 rules + 2 条 rule_set + final=proxy-out。
- 保留 `dnsServer` 形参（Plan 45-02 才会动签名），函数体不再使用它；删除旧 `if dnsServer == "" { dnsServer = "1.1.1.1" }` 兜底。

### Task 2: 扩展测试覆盖 6 类断言（TDD）

**Commit:** `db846e1`

新增测试函数（全部 PASS）：

| 测试函数 | 覆盖需求 | 关键断言 |
|----------|----------|----------|
| `TestBuildGatewaySingBoxConfig_RuleSetReferences` | BYPASS-NET-01 | rule_set 长度 = 2，tag/type/format/path 完全匹配 |
| `TestBuildGatewaySingBoxConfig_RouteRulesOrder` | BYPASS-NET-02 | 6 条按下标精确断言；route.final=proxy-out |
| `TestBuildGatewaySingBoxConfig_TunHardening` | BYPASS-NET-03 | tun 三参加固 + stack=system + default_interface=eth0；明确断言 auto_detect_interface 被移除 |
| `TestBuildGatewaySingBoxConfig_SniffAndHijack` | BYPASS-NET-04 | 第一条 action=sniff 且无 outbound；第二条 action=hijack-dns（不是旧 outbound:"dns"） |
| `TestBuildGatewaySingBoxConfig_SplitDNS` | BYPASS-DNS-01 / BYPASS-DNS-02 | dns-local + dns-proxy 各字段 + 2 条 rules 排序 + final/strategy |
| `TestBuildGatewaySingBoxConfig_DirectOutboundBindEth0` | BYPASS-NET-03 防回环 | direct outbound bind_interface=eth0 |

共享 helper：`renderTestConfig` / `asMap` / `asStrSlice` / `containsStr`，全部落在同一文件。

旧测试调整：
- 删除 `TestBuildGatewaySingBoxConfig_DefaultDNS`（dns-remote 断言已与新结构不兼容；新结构下 dns.servers 是 dns-local + dns-proxy，dns-local 没有 server 字段）。
- 简化 `TestBuildGatewaySingBoxConfig` 顶层断言，不再要求 servers 长度 = 1。
- `TestBuildGatewaySingBoxConfig_ProxyServerIPInRoute` / `TestBuildGatewayProxyOutbound*` 系列全部保留并继续 PASS。

### Task 3: PrepareHost 写 rule-set placeholder + dockerRunGateway 挂载

**Commit:** `5d7d9fc`

- `PrepareHost` 在写完 `configDir/config.json` 之后，紧接着用 `ruleSetPlaceholder` 常量写 `whitelist-cidrs.json` 与 `whitelist-domains.json` 到同一目录（0o644）。
- `dockerRunGateway` 签名追加 `cidrsPath` / `domainsPath` 两个参数（保持顺序：configPath 后紧跟）；唯一调用点 PrepareHost 同步更新。
- gateway 容器 docker 参数追加：
  ```
  -v <cidrsPath>:/etc/sing-box/whitelist-cidrs.json:ro
  -v <domainsPath>:/etc/sing-box/whitelist-domains.json:ro
  ```
- 不动 `EgressConfig.Proxy.DNSServer` 默认值与 `verify.go` —— 留给 Plan 45-02 同步任务。

## Deviations from Plan

无。计划执行完全按写定流程，没有 Rule 1/2/3 自动修复，也没有 Rule 4 架构性改动。

## 已知遗留副作用（交给 Plan 45-02 处理）

1. **`EgressConfig.Proxy.DNSServer` 与 `verify.verifyDNS` 期望未同步**：本 plan 主流程依然透传 `spec.Egress.Proxy.DNSServer` 给 `buildGatewaySingBoxConfig`（函数内不再使用），但容器侧 `/etc/resolv.conf` 仍是 `nameserver 8.8.8.8`（来自 `tryConfigureWorkerEgress` 内的 `echo 'nameserver 8.8.8.8' > /etc/resolv.conf`）。
   - 单元测试不受影响（`go test ./internal/network -short` 全 PASS）。
   - 真实启动 gateway + worker 的集成 / 端到端验证可能仍按旧 DNS 路径走，需要 Plan 45-02 把 resolv.conf 改 bind mount + verify.expected 改 172.19.0.1 后才会重新对齐。
2. **gateway 容器实际启动**：未在本机做 `docker run` 真实启动验证（无法构造完整 hosts 表 + 控制面 ctx），仅做 build + unit test。Phase 47 的 E2E 流程会覆盖。

## 给 Plan 45-02 的接口约定

Plan 45-01 已经写到盘上的不变量（Plan 45-02 可以依赖）：

| 文件 | 内容 | 路径模板 |
|------|------|----------|
| `config.json` | sing-box v3.5 两段式渲染输出 | `<DATA_DIR>/gateway/<host_id>/config.json` |
| `whitelist-cidrs.json` | v3 schema 空规则集（`{"version":3,"rules":[]}\n`） | `<DATA_DIR>/gateway/<host_id>/whitelist-cidrs.json` |
| `whitelist-domains.json` | 同上 | `<DATA_DIR>/gateway/<host_id>/whitelist-domains.json` |

容器内（gateway）三个 ro mount 点：
- `/etc/sing-box/config.json:ro`
- `/etc/sing-box/whitelist-cidrs.json:ro`
- `/etc/sing-box/whitelist-domains.json:ro`

Plan 45-02 还未实现的相邻能力（worker 容器侧）：
- worker 容器内 `/etc/resolv.conf` 改为 ro bind mount → `nameserver 172.19.0.1` + `options ndots:0 single-request-reopen`
- worker 容器内 `/etc/nsswitch.conf` 改为 ro bind mount → `hosts: files dns`
- `EgressConfig.Proxy.DNSServer` 默认值 / verify.expected 同步到 `172.19.0.1`
- `container_proxy_provider_test.go:246/283` 旧 `'nameserver 8.8.8.8'` 字符串断言同步更新

## 术语对齐说明

本 plan 沿用旧 outbound tag `direct`（不改为 RESEARCH 中的 `direct-out`）。

理由：保持与现有 `gateway_singbox_config_test.go` 断言兼容，避免在本 plan 引入跨 phase 的术语漂移。RESEARCH.md 中写的 `direct-out` 仅是研究稿术语，未在代码与测试中使用；Phase 47 文档若引用 `direct-out` 需统一为 `direct`（或在 Phase 47 引入新 tag `direct-out` 时同步改测试断言，但这超出 v3.5 当前范围）。

route.rules 中所有 6 条 `outbound: "direct"` 都引用同一个 outbound tag，无歧义。

## Files Modified

| File | 增 | 删 | 说明 |
|------|-----|-----|------|
| `internal/network/gateway_singbox_config.go` | +142 | -34 | 渲染骨架重构 + 5 个 helper + 5 个常量 |
| `internal/network/gateway_singbox_config_test.go` | +335 | -31 | 6 个新增 table-driven 测试 + 共享 helper |
| `internal/network/container_proxy_provider.go` | +15 | -2 | PrepareHost 写 placeholder + dockerRunGateway 签名扩展 |

## Verification

| 检查 | 命令 | 结果 |
|------|------|------|
| Build | `go build ./internal/network/...` | exit 0 |
| Unit tests | `go test ./internal/network/ -count=1 -short` | PASS |
| `TestBuildGatewaySingBoxConfig_*` 全套 | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig -count=1 -v` | 11/11 PASS（6 新 + 5 现有） |
| grep 关键字段 | 见 plan verify 段落 | 全部命中 |
| 不应出现的字段 | `auto_detect_interface` / `sniff_override_destination` | grep 命中 0 |

## Self-Check

文件存在性核对：
- `internal/network/gateway_singbox_config.go`: FOUND
- `internal/network/gateway_singbox_config_test.go`: FOUND
- `internal/network/container_proxy_provider.go`: FOUND

提交核对：
- `370b14b` (Task 1): FOUND in `git log`
- `db846e1` (Task 2): FOUND in `git log`
- `5d7d9fc` (Task 3): FOUND in `git log`

## Self-Check: PASSED
