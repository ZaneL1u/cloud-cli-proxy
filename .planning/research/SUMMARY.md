# v3.5 网络白名单与 DNS 拆分解析 · 研究综述（SUMMARY）

**Project:** Cloud CLI Proxy
**Milestone:** v3.5
**研究方式:** 4 个并行专题 agent（sing-box 技术 / 项目代码现状 / DNS 泄漏防护 / UX 数据模型）输出，已在前置对话完成；本文档为浓缩版供 roadmapper 直接消费。

---

## 0. 核心问题

当前 sing-box tun 全隧道导致用户容器访问本地宿主机、局域网、特定白名单服务（GitHub 镜像、AI API 等）不通。需要让管理员能在后台配置白名单（IP/CIDR/域名/端口），让特定流量不走代理直连本地或互联网，**同时绝对避免 DNS 泄漏**。

---

## 1. 技术选型（来自 sing-box 1.x 官方文档研究）

### 1.1 关键技术决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 白名单作用层 | sing-box `route.rules` 主导 + 容器 netns nftables 兜底（双层一致） | sing-box 表达力强；nft 兜底保证 sing-box 崩溃时 fail-closed |
| 本地/局域网识别 | sing-box `ip_is_private: true`（1.8+ 内置） | 替代旧 `geoip:private`，无需额外 rule-set |
| DNS 安全模型 | **拆分 DNS** | 内网域名 → `dns-local`；公网白名单域名 → 代理 DoH（保护查询隐私） |
| 热更新机制 | sing-box `type:"local"` rule-set 文件 watch | **唯一可行的无重启进程方案**（Clash API 不能改 route 规则本体，sing-box 无 SIGHUP） |
| 配置形态 | **两段式** | 静态 config.json（变更需重启）+ 动态 rule-set 文件（实时 watch） |
| DNS 入口锁 | 容器 `/etc/resolv.conf` 只读挂载指向 sing-box tun IP（172.19.0.1） | 应用层 DNS 全部被 sing-box 接管 |
| IPv6 策略 | v1 容器内全禁 | 最简、最稳，未来再开 |
| FakeIP | **不用** | 主流量 SSH 不会被 sniff，FakeIP 会让 SSH 锁死在假 IP |

### 1.2 核心 sing-box 配置骨架

```json
{
  "dns": {
    "servers": [
      { "tag": "dns-local", "type": "local" },
      { "tag": "dns-proxy", "type": "https", "server": "1.1.1.1",
        "domain_resolver": "dns-local", "detour": "proxy-out" }
    ],
    "rules": [
      { "domain_suffix": [".lan",".local",".internal"], "action": "route", "server": "dns-local" },
      { "rule_set": ["whitelist-domains"], "action": "route", "server": "dns-proxy" }
    ],
    "final": "dns-proxy",
    "strategy": "ipv4_only"
  },
  "inbounds": [{
    "type": "tun", "tag": "tun-in",
    "interface_name": "sb-tun0",
    "address": ["172.19.0.1/30"],
    "auto_route": true, "strict_route": true,
    "stack": "system"
  }],
  "outbounds": [
    { "type": "direct", "tag": "direct-out", "bind_interface": "eth0" },
    { "type": "vless", "tag": "proxy-out", /* from egress_ips.proxy_config */ }
  ],
  "route": {
    "default_interface": "eth0",
    "rule_set": [
      { "type": "local", "tag": "whitelist-cidrs", "format": "source",
        "path": "/etc/sing-box/whitelist-cidrs.json" },
      { "type": "local", "tag": "whitelist-domains", "format": "source",
        "path": "/etc/sing-box/whitelist-domains.json" }
    ],
    "rules": [
      { "action": "sniff", "sniffer": ["tls","http","quic","dns"] },
      { "protocol": "dns", "action": "hijack-dns" },
      { "ip_cidr": ["<proxy_ip>/32"], "action": "route", "outbound": "direct-out" },
      { "ip_is_private": true, "action": "route", "outbound": "direct-out" },
      { "rule_set": ["whitelist-cidrs"], "action": "route", "outbound": "direct-out" },
      { "rule_set": ["whitelist-domains"], "action": "route", "outbound": "direct-out" }
    ],
    "final": "proxy-out"
  }
}
```

### 1.3 sing-box 常见陷阱（必须避免）

1. 没写 `route.final` → 落到 outbound 列表第一项，可能就是 direct，全网泄漏
2. 没写 `hijack-dns` → tun 抓到的 53 端口 UDP 被代理走，DNS 模块完全没看到
3. 没启用 sniff，又用 `domain_suffix` → IP literal 连接根本拿不到域名，规则永不命中
4. `auto_route=true` 但没 `default_interface` → direct outbound 又被 tun 抢回，回环
5. `strict_route: false` → 未支持网络用系统默认路由出去，等同泄漏
6. 容器 `/etc/resolv.conf` 仍指向 8.8.8.8 → 应用层 DNS 绕过 sing-box

---

## 2. 当前代码现状（来自项目代码探索）

### 2.1 关键文件与扩展点

| 维度 | 现状 | 扩展点 |
|------|------|--------|
| sing-box 配置生成 | `internal/network/gateway_singbox_config.go:10` Go map 拼装；当前 route.rules 仅 2 条（proxy IP/32 + port 53 hijack-dns） | 加 rule_set 数组 + `ip_is_private` 规则 + 白名单 rule_set 引用 |
| 容器架构 | sidecar gateway 模式：`cloudproxy-net-<hostID>` bridge + gateway 容器（sing-box）+ worker 容器（用户 shell） | sing-box 跑在 gateway 容器，白名单 rule-set 文件挂入 gateway |
| 防火墙 | `internal/network/worker_firewall_linux.go:25` 已 `output policy drop` + allowlist（lo / gateway / 22 / 53） | 加 `@whitelist_v4` set + nftables 与 rule-set 文件 hash 一致校验 |
| 热更新 | **无** —— 改 `proxy_config` 必须 stop/start/rebuild 容器（teardownGateway） | 新增 `ActionReloadHostBypass`，agent 端写文件 + 更新 nft set + 健康检查 |
| 数据模型 | pgx + UUID 主键（`gen_random_uuid()`）+ `TIMESTAMPTZ DEFAULT NOW()` + migration 顺序号 0001–0018 | 新增 0019_host_bypass_rules.sql |
| `/etc/resolv.conf` | `container_proxy_provider.go:323` 写死 `nameserver 8.8.8.8` 占位 | **改为只读挂载 → 172.19.0.1（sing-box tun IP）** |
| 控制面 API | 标准库 `net/http` + Go 1.22 mux + JWT；`internal/controlplane/http/admin_egress_ips.go` 是模仿模板 | 新增 `internal/controlplane/http/admin_bypass.go` |
| React 后台 | TanStack Router + TanStack Query + Radix UI + Tailwind v4，shadcn 风格自建组件 | host 详情页加 Bypass Tab，复用 `egress-ip-drawer` 模式 |
| 测试 | `internal/network/verify.go` 已用 nsenter + curl 真实流量验证 | 扩展验证白名单流量走向 + 10 条安全不变量 |

### 2.2 当前网络拓扑

```
用户进程 (worker container)
  ↓ (worker 默认路由 → gateway 容器 IP)
gateway container (运行 sing-box, tun0)
  ↓ (sing-box route.rules)
  ├─ proxy 流量 → proxy-out → 出口代理服务器
  └─ 白名单(新增) → direct-out → eth0 → 宿主机/局域网
```

---

## 3. 安全模型（来自 DNS 泄漏防护研究）

### 3.1 四层防御

| 层 | 内容 | 必要性 |
|----|------|--------|
| L4 容器 `/etc/resolv.conf` | 唯一 nameserver = sing-box tun IP，只读挂载 | 必需（DNS 入口锁） |
| L1 sing-box route + DNS | 拆分 DNS + 白名单 route 规则 | 必需（业务逻辑） |
| L2 容器 netns nftables | `output policy drop` + 仅放行 `oifname sb-tun0` + sing-box uid 锁定到代理 IP + 白名单 `@whitelist_v4` set | 必需（fail-closed 关键） |
| L3 宿主 nftables | FORWARD 链按 netns 出口 IP 严校验 | 推荐（防容器内 root 篡改） |

### 3.2 13 种 DNS 泄漏场景（必须全部封堵）

L1 resolv.conf 指向公网 DNS · L2 Docker embedded DNS · L3 IP literal 直连 · L4 白名单域名用容器外 resolver · L5 Chrome 内建 DoH · L6 Go cgo resolver 行为不一致 · L7 mDNS/LLMNR/NetBIOS · L8 sing-box 重启窗口期 · L9 sing-box 启动失败 fail-open · L10 IPv6 未禁用 · L11 ICMP/raw socket · L12 /etc/hosts 注入 · L13 系统服务自带 resolver

### 3.3 安全不变量（10 条，CI 必须验证）

| # | 不变量 | 验证 |
|---|--------|------|
| I1 | 容器 `/etc/resolv.conf` 唯一 nameserver = sing-box tun IP | `nsenter -t $PID -m cat /etc/resolv.conf` |
| I2 | 容器 netns `output` policy = drop | `nsenter ... nft list chain inet sbfw output` |
| I3 | 出 eth0 的包只能去白名单或代理 IP | `tcpdump -ni eth0 'not (host PROXY or net WHITELIST)'` 计数为 0 |
| I4 | 容器内 dig 8.8.8.8 必失败 | `nsenter ... dig @8.8.8.8 +time=2` 必超时 |
| I5 | sing-box 停止 → 白名单也断（fail-closed） | `pkill sing-box; curl WHITELIST_IP` 必失败 |
| I6 | IPv6 全禁 | `nsenter ... ip -6 addr` 仅 `::1` |
| I7 | nft set 与 rule-set 文件 hash 一致 | `/health/bypass-consistency` 接口 |
| I8 | rule-set 文件存在且有效 JSON | sing-box 启动健康检查 |
| I9 | mDNS/LLMNR/NetBIOS 外向流量为 0 | `nft list counter inet sbfw mdns-drop` |
| I10 | 白名单变更后 SSH 连接不断 | `ssh ... 'while true; do echo .; sleep 1; done'` 跨越 reload 不中断 |

---

## 4. 数据模型与 API 设计（来自 UX 研究）

### 4.1 五张表（命名遵循项目约定：snake_case、UUID 主键）

- `host_bypass_presets` — 预设方案（系统内置 + 平台维护）
- `host_bypass_rules` — 自定义规则（scope: global / host）
- `host_bypass_bindings` — host ↔ 预设/规则绑定
- `host_bypass_snapshots` — 配置版本快照（apply / rollback 用）
- `host_bypass_audit_log` — 审计日志（actor、action、before、after）

### 4.2 系统内置预设（v3.5 范围）

| 预设 | 内容 | 强制开启 |
|------|------|----------|
| `loopback` | 127.0.0.0/8、169.254.0.0/16 | 是（不可关闭） |
| `lan` | + 10/8、172.16/12、192.168/16、100.64/10、ULA | 默认关闭 |

P1 预设（`cn-dev` / `oss-dev` / `ai-api` / `custom`）不在 v3.5 范围。

### 4.3 关键 API（管理员）

```
GET/POST/PATCH/DELETE /v1/admin/bypass/presets
GET/POST/PATCH/DELETE /v1/admin/bypass/rules
POST                  /v1/admin/bypass/rules/validate
GET                   /v1/admin/hosts/{id}/bypass
POST                  /v1/admin/hosts/{id}/bypass/bind
POST                  /v1/admin/hosts/{id}/bypass/unbind
GET                   /v1/admin/hosts/{id}/bypass/effective
POST                  /v1/admin/hosts/{id}/bypass/preview
POST                  /v1/admin/hosts/{id}/bypass/apply
POST                  /v1/admin/hosts/{id}/bypass/rollback
```

### 4.4 配置下发流程

```
管理员保存
  → validateRule()（护栏校验）
  → renderSnapshot(host)（合并 preset + rule，渲染 rule-set 文件 + nft set）
  → previewIfRequested()（用户可中断）
  → writeSnapshotToDB(applied_status='pending')
  → dispatchTask(ActionReloadHostBypass)
  → agent: 写 rule-set 文件 (tmpfile + rename) + nft -f atomic 更新 + 等 1s sing-box watch reload + 健康检查
  → 成功 → applied_status='applied'
  → 失败 → 自动 rollback 上一 snapshot + 告警
```

### 4.5 护栏（硬拦截）

| 护栏 | 触发 |
|------|------|
| 全量绕过 | `0.0.0.0/0` / `::/0` |
| CIDR 过宽 | v4 < /16 且非私有段 |
| 顶级域名后缀 | `.com` `.net` 等 |
| 关键字过短 | `domain_keyword` 长度 < 4（警告 + 二次确认） |
| 自我矛盾 | 规则覆盖代理服务器 IP |
| 数量上限 | 单 host > 1000 条 |

---

## 5. 关键决策点（已与用户对齐）

1. **公网 CIDR 限制**：默认仅允许私有段 + 用户精确 IP / 小段；禁止 `0.0.0.0/0`、v4 < /16 等过宽段
2. **DNS 路径**：公网白名单域名走代理 DoH（保护查询隐私）；内网域名走 dns-local
3. **IPv6**：v1 容器内全禁
4. **用户范围**：v3.5 仅管理员，用户自助进 P2 backlog
5. **cn-dev / oss-dev 预设**：v3.5 P0 不做；P1 引用开源（MetaCubeX/meta-rules-dat） + 自维护镜像 fallback
6. **sing-box 启动失败**：fail-closed —— 容器 unhealthy 不放行 SSH
7. **白名单变更影响**：不影响现有 TCP 连接，新连接才用新规则（UI 明确告知）
8. **审计日志保留**：默认 90 天，可配置

---

## 6. 实施路径建议（roadmapper 参考）

### Phase 45 主线 A · 基础与数据模型（建议 3-4 个 plan）

- 0019 migration（5 张表 + seed loopback/lan 预设）+ repository 层 CRUD
- `internal/network/bypass.go` —— 渲染 rule-set 文件 + nftables set 内容
- 扩展 `gateway_singbox_config.go` 加入 `rule_set` 数组 + `ip_is_private` 内置规则 + `strict_route:true`
- DNS 拆分：dns-local + dns-proxy(DoH detour proxy-out)
- 容器 `/etc/resolv.conf` 接管（从 8.8.8.8 改为 172.19.0.1，只读挂载）

### Phase 46 主线 B · 控制面 API + 后台 UI（建议 3-4 个 plan）

- HTTP handler `internal/controlplane/http/admin_bypass.go`（仿 admin_egress_ips.go）
- 护栏校验 `internal/network/bypass_validate.go`
- React hook `use-bypass.ts` + 组件 `bypass-manager.tsx`（仿 binding-manager）
- host 详情页加 Bypass Tab
- preview 接口（返回 rule-set 文件 + nft set diff + 风险报告，不落库）

### Phase 47 主线 C · 热更新与流量验证（建议 3-4 个 plan）

- 新增 `ActionReloadHostBypass`（runtime/tasks/worker.go）
- agent 实现：写 rule-set 文件（tmpfile + rename）+ nft atomic set 更新 + 健康检查 + 失败回滚
- fail-closed 加固：worker_firewall_linux.go 增加 `@whitelist_v4` set 管理 + IPv6 全禁
- 扩展 `internal/network/verify.go` —— 10 条安全不变量 CI 化
- E2E 验证脚本（仿 scripts/uat-network-resilience.sh）

---

## 7. 来源

- sing-box 官方文档：https://sing-box.sagernet.org/configuration/ （route/rule、dns/rule、rule-set、inbound/tun）
- sing-box 1.10 release notes（rule-set 文件 watch 支持）
- 项目代码探索（gateway_singbox_config.go、worker_firewall_linux.go、container_proxy_provider.go、verify.go）
- DNS leak 知识：dnsleaktest.com、Tailscale exit node、Mullvad split tunneling 设计参考
- 类似项目实现：MetaCubeX/meta-rules-dat、Loyalsoldier/clash-rules

---

*生成时间：2026-05-12（v3.5 milestone 启动时）。本文档由前置 4 个并行研究 agent 输出综合，跳过 spawn 新研究 agents 以避免重复。*
