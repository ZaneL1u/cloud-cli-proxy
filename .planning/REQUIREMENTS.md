# Requirements: Cloud CLI Proxy v3.5

**Defined:** 2026-05-12
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

## v3.5 Requirements

### 网络配置基础（BYPASS-NET）

- [ ] **BYPASS-NET-01**: sing-box 配置改造为两段式 —— 静态 config.json（每 host 一份模板渲染）+ 动态 local rule-set 文件（路径形如 `/etc/sing-box/whitelist-cidrs.json` 和 `whitelist-domains.json`），`type:"local" + format:"source"` 由 sing-box 文件 watch 自动 reload，不重启进程。

- [ ] **BYPASS-NET-02**: sing-box `route.rules` 引入 `ip_is_private:true` 内置规则做本地/链路本地/RFC1918/ULA 兜底直连；并按顺序加 `whitelist-cidrs` / `whitelist-domains` 两个 rule_set 引用，所有未命中规则的流量走 `final: "proxy-out"`。

- [ ] **BYPASS-NET-03**: sing-box `tun` inbound 启用 `strict_route:true` + `auto_route:true` + `endpoint_independent_nat:true`，禁止未支持网络回退到系统默认路由；`route.default_interface` 显式指向 `eth0`，避免 direct outbound 回环走 tun。

- [ ] **BYPASS-NET-04**: route.rules 第一条 `action:"sniff"` 配置 `sniffer: ["tls","http","quic","dns"]`，保证 IP literal 连接也能命中基于域名的白名单；`protocol:"dns"` 必须走 `hijack-dns` action（替代旧 `outbound:"dns"`）。

### DNS 拆分与防泄漏（BYPASS-DNS）

- [ ] **BYPASS-DNS-01**: sing-box 配置启用拆分 DNS —— `dns.servers` 至少包含 `dns-local`（type:local）和 `dns-proxy`（type:https，server:1.1.1.1，detour:proxy-out，domain_resolver:dns-local），且 `dns.final = "dns-proxy"`，`strategy:"ipv4_only"`。

- [ ] **BYPASS-DNS-02**: 内网域名后缀（`.lan` / `.local` / `.internal`）走 `dns-local` 解析；公网白名单域名（`whitelist-domains` rule_set）走 `dns-proxy` 解析以保护查询隐私（即使流量走 direct 也不让 LAN DNS 留痕）。

- [ ] **BYPASS-DNS-03**: 容器 `/etc/resolv.conf` 从当前 `nameserver 8.8.8.8` 占位改为只读挂载，唯一 nameserver 指向 sing-box tun IP（172.19.0.1），并配 `options ndots:0 single-request-reopen`；用户和应用无法在容器内修改。

- [ ] **BYPASS-DNS-04**: 容器 `/etc/nsswitch.conf` 设为 `hosts: files dns`，禁用 mdns/myhostname/wins 等本地发现解析路径，避免 mDNS/LLMNR/NetBIOS 旁路。

### 数据模型与预设（BYPASS-DATA）

- [ ] **BYPASS-DATA-01**: 新增 migration `0019_host_bypass_rules.sql`，建 5 张表：`host_bypass_presets`、`host_bypass_rules`、`host_bypass_bindings`、`host_bypass_snapshots`、`host_bypass_audit_log`；UUID 主键 + `gen_random_uuid()` + `TIMESTAMPTZ DEFAULT NOW()`，命名与现有 `host_egress_bindings` 风格一致。

- [ ] **BYPASS-DATA-02**: Repository 层提供 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` 的 CRUD 方法（`internal/store/repository/`），错误处理沿用项目现有 `NetworkError` 风格。

- [ ] **BYPASS-DATA-03**: 系统内置预设种子（migration seed）：`loopback`（`127.0.0.0/8` + `169.254.0.0/16`，`is_system=true`，`is_force_on=true`，**不可关闭**）和 `lan`（含 RFC1918 + CGNAT 100.64/10 + ULA fc00::/7，默认关闭）；用户和管理员都不能删除或修改 `is_system` 预设。

- [ ] **BYPASS-DATA-04**: 配置快照（`host_bypass_snapshots`）记录每次 apply 的版本号、`config_hash`、渲染后的 `whitelist-cidrs.json` / `whitelist-domains.json` 内容、`applied_status`（pending/applied/failed/rolled_back）和 `created_by`，用于回滚和审计。

### 控制面 API 与护栏（BYPASS-API）

- [ ] **BYPASS-API-01**: 新增管理员 API（标准库 `net/http` + Go 1.22 mux + JWT 鉴权）：
  - `GET/POST/PATCH/DELETE /v1/admin/bypass/presets`（系统预设不可写）
  - `GET/POST/PATCH/DELETE /v1/admin/bypass/rules`
  - `POST /v1/admin/bypass/rules/validate`（dry-run 单条规则校验）
  - `GET/POST /v1/admin/hosts/{hostID}/bypass`（list / bind / unbind）

- [ ] **BYPASS-API-02**: 提供 preview + apply + rollback 三个核心接口：
  - `POST /v1/admin/hosts/{hostID}/bypass/preview` —— 渲染 rule-set 文件 + nft set diff + 风险报告，不落库
  - `POST /v1/admin/hosts/{hostID}/bypass/apply` —— 写 snapshot 并触发 agent reload（`config_hash` 作为幂等键）
  - `POST /v1/admin/hosts/{hostID}/bypass/rollback` —— 回到上一个 `applied` snapshot
  - `GET /v1/admin/hosts/{hostID}/bypass/effective` —— 返回当前生效的完整规则集合

- [ ] **BYPASS-API-03**: 护栏硬拦截（HTTP 422 错误码 `BYPASS_RULE_TOO_BROAD` / `BYPASS_RULE_CONFLICT_PROXY` / `BYPASS_LIMIT_EXCEEDED`）：
  - 拒绝 `0.0.0.0/0` / `::/0` 全量绕过
  - 拒绝 v4 CIDR < /16 且不属于私有段（防误绕公网大段）
  - 拒绝 `domain_suffix` 长度 < 4 或顶级 TLD（如 `.com`）
  - 拒绝覆盖代理服务器 IP 的规则（自我矛盾）
  - 单 host 有效规则数 > 1000 拒绝

- [ ] **BYPASS-API-04**: 危险关键字软拦截 —— `domain_keyword` 长度 < 4 时返回 400 警告，要求请求体携带 `confirm_risky:true` 才允许保存；审计日志记录管理员的二次确认动作。

- [ ] **BYPASS-API-05**: 所有写操作（create/update/delete/bind/unbind/apply/rollback）写入 `host_bypass_audit_log`，记录 actor_id、actor_ip、action、target_kind、target_id、before/after JSON、note，默认保留 90 天。

### React 后台 UI（BYPASS-UI）

- [ ] **BYPASS-UI-01**: host 详情页（`web/admin/src/routes/_dashboard/hosts/$hostId.tsx`）新增「代理白名单」Tab；UI 风格遵循 shadcn + Tailwind v4 + Radix UI（仿 `binding-manager.tsx` 和 `egress-ip-drawer.tsx`）。

- [ ] **BYPASS-UI-02**: Tab 内提供预设多选卡片（loopback 强制锁定，lan 可勾选），每个卡片悬浮显示包含的规则示例；选中后实时刷新预览。

- [ ] **BYPASS-UI-03**: 自定义规则列表 CRUD（IP/CIDR/域名/域名后缀/端口五种类型）；高风险规则展示黄色徽章，触发二次确认弹窗。

- [ ] **BYPASS-UI-04**: 「预览生效配置」面板，展示当前 v→v+1 的 rule-set diff 和人类可读摘要；支持「查看 sing-box JSON」和「查看 nft set diff」两种视图切换。

- [ ] **BYPASS-UI-05**: 应用按钮分阶段反馈进度 —— 生成快照 → 下发到 agent → reload → 健康检查 → 完成；失败时显示具体错误码并标注自动回滚状态；成功后 toast 提示 `白名单变更不影响现有 TCP 连接，新连接才用新规则`。

### Agent 热更新链路（BYPASS-RELOAD）

- [ ] **BYPASS-RELOAD-01**: `internal/agentapi/contracts.go` 新增 `ActionReloadHostBypass HostAction = "reload_host_bypass"`；worker dispatch（`internal/runtime/tasks/worker.go`）新增对应 case，读取最新 snapshot 并下发到 host-agent。

- [ ] **BYPASS-RELOAD-02**: host-agent 收到 reload 指令后执行：
  1. 写 rule-set 文件用 `tmpfile + rename` 原子语义，路径 `/var/lib/cloud-cli-proxy/host/<host_id>/whitelist-{cidrs,domains}.json`，并 bind mount 到 gateway 容器 `/etc/sing-box/`；
  2. 同步更新容器 netns nftables `@whitelist_v4` set（用 `nft -f` 事务批量更新）；
  3. 等 1s 让 sing-box 文件 watch reload；
  4. 健康检查（nsenter + curl 验证白名单流量从 eth0 出 + 非白名单仍走代理出口）。

- [ ] **BYPASS-RELOAD-03**: 失败自动回滚 —— reload 健康检查 3 次失败后，自动用上一个 `applied` snapshot 重新下发，并把当前 snapshot 标记为 `rolled_back`，触发事件日志告警。

- [ ] **BYPASS-RELOAD-04**: nft set 内容与 rule-set 文件的 SHA-256 必须 hash 一致；控制面提供 `GET /v1/admin/hosts/{hostID}/bypass/consistency` 健康检查接口，定期对账。

### fail-closed 加固（BYPASS-NFT）

- [ ] **BYPASS-NFT-01**: 扩展 `internal/network/worker_firewall_linux.go` 的 `output` 链规则集，增加：
  - `oifname "sb-tun0" accept`（白名单流量从 tun 出，统一走 sing-box 路由判断）
  - `meta skuid singbox ip daddr <代理服务器IP> tcp dport 443 accept`（uid 锁定）
  - `oifname "eth0" ip daddr @whitelist_v4 accept`（白名单逃逸通道）
  - 默认末尾 `counter log prefix "sbfw-drop " drop`（计数 + 日志）

- [ ] **BYPASS-NFT-02**: 阻断 mDNS（5353）/ LLMNR（5355）/ NetBIOS（137）的 UDP 出向流量，无论是否在白名单中。

- [ ] **BYPASS-NFT-03**: 容器内全局禁用 IPv6 —— 启动参数加 `--sysctl net.ipv6.conf.all.disable_ipv6=1` 和 `--sysctl net.ipv6.conf.default.disable_ipv6=1`；ip6tables 默认 drop。

- [ ] **BYPASS-NFT-04**: sing-box 启动失败 fail-closed —— gateway 容器健康检查不通过时，control-plane 不允许 worker 容器开放 SSH 端口（容器 unhealthy 状态由 `provider.PrepareHost` 中的现有 verify 流程负责）。

### E2E 验证与安全不变量（BYPASS-VERIFY）

- [ ] **BYPASS-VERIFY-01**: 扩展 `internal/network/verify.go` 的 `VerifyNetworkIntegrity`，新增 3 项检查：
  1. 白名单 IP（如 RFC1918 192.168.0.1）`nsenter + curl` 流量从 eth0 出（源 IP = host eth0），非代理出口
  2. 非白名单域名（如 `api.example.com`）流量仍走代理出口（源 IP = egress IP）
  3. `nsenter + dig @8.8.8.8 example.com` 必超时（DNS 不能直连公网 DNS）

- [ ] **BYPASS-VERIFY-02**: 实现 10 条安全不变量的 CI 化（含 fail-closed 场景 —— pkill sing-box 后白名单也必须断），完整清单见 `.planning/research/SUMMARY.md` §3.3：I1（resolv.conf 唯一指向 sing-box）/ I2（nft policy=drop）/ I3（eth0 出向 = 白名单 ∪ 代理 IP）/ I4（公网 DNS 失败）/ I5（fail-closed）/ I6（IPv6 全禁）/ I7（nft 与 rule-set hash 一致）/ I8（rule-set 文件有效）/ I9（mDNS/LLMNR/NetBIOS=0）/ I10（reload 不断 SSH）。

- [ ] **BYPASS-VERIFY-03**: 新增 `scripts/uat-bypass.sh` UAT 脚本（仿 `scripts/uat-network-resilience.sh`），覆盖 6 个场景：仅 loopback / 仅 lan / loopback+lan / 自定义 IP 规则 / 自定义域名规则 / fail-closed sing-box 崩溃。

- [ ] **BYPASS-VERIFY-04**: `host_bypass_snapshots.applied_status` 和 `host_bypass_audit_log` 在 e2e 测试中端到端可见 —— 测试脚本能拉到 `applied/rolled_back` 两种状态的 snapshot 且 audit log 行数正确。

## Future Requirements (v3.5 P1 / v3.6+)

- [ ] **BYPASS-PRESET-CN-DEV**: `cn-dev` 预设（阿里云/腾讯云 metadata、mirrors.aliyun.com、mirrors.tuna、registry.npmmirror.com、goproxy.cn 等）
- [ ] **BYPASS-PRESET-OSS-DEV**: `oss-dev` 预设（github.com、registry.npmjs.org、pypi.org、registry-1.docker.io、proxy.golang.org 等）
- [ ] **BYPASS-PRESET-AI-API**: `ai-api` 预设（api.anthropic.com、api.openai.com 等代理到不了的场景）
- [ ] **BYPASS-RULESET-REMOTE**: 远程 rule-set 拉取 worker（引用 MetaCubeX/meta-rules-dat + 自维护镜像 fallback）
- [ ] **BYPASS-CANARY**: 「先在测试 host 验证」按钮，灰度下发
- [ ] **BYPASS-USER-SELF**: 用户自助配置白名单（区分管理员/用户角色权限）
- [ ] **BYPASS-HIT-STATS**: 命中统计（轮询 sing-box Clash API `/connections`）
- [ ] **BYPASS-DASHBOARD**: 流量 dashboard（bypass / proxy 流量字节占比）

## Out of Scope (v3.5)

- 复合规则编辑器（多字段 AND 组合）—— UI 复杂度高，P1 再评估
- `domain_regex` 高级规则 —— 性能和误用风险，延后
- 多租户白名单（按 tenant_id 隔离）—— v1 单宿主机单租户，未来再加
- FakeIP 模式 —— 主流量 SSH 不会被 sniff，FakeIP 会让 SSH 锁死在假 IP 段
- IPv6 双栈支持 —— v3.5 容器内全禁 IPv6，未来在能确保 ip6tables / IPv6 路由规则对称完整时再开
- 用户层 bypass 自定义 —— v3.5 仅管理员可配置
- 跨 host 批量 apply —— v3.5 单 host 维度 apply，未来再加

## Traceability

| REQ-ID | Phase | Plans |
|--------|-------|-------|
| BYPASS-NET-01..04 | TBD | TBD |
| BYPASS-DNS-01..04 | TBD | TBD |
| BYPASS-DATA-01..04 | TBD | TBD |
| BYPASS-API-01..05 | TBD | TBD |
| BYPASS-UI-01..05 | TBD | TBD |
| BYPASS-RELOAD-01..04 | TBD | TBD |
| BYPASS-NFT-01..04 | TBD | TBD |
| BYPASS-VERIFY-01..04 | TBD | TBD |

（roadmapper 将填入对应 phase 号和 plan 号）

---

*Last updated: 2026-05-12 — milestone v3.5 requirements defined. 30 active requirements across 8 categories.*
