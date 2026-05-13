# Phase 45: 网络配置基础与数据模型 - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** Auto-generated (smart_discuss 判定为 infrastructure-only，所有关键技术决策已在 `.planning/research/SUMMARY.md` 锁定)

<domain>
## Phase Boundary

为 v3.5 白名单/DNS 拆分能力打通三个基础设施层：

1. **sing-box 静态配置改造为两段式** —— 渲染层（`gateway_singbox_config.go`）新增 `route.rule_set` 引用、`route.rules` 排序、`tun` inbound 加固、`dns` 拆分配置。但 rule-set 文件本身的动态下发链路属于 Phase 47。
2. **容器 DNS 入口锁** —— `container_proxy_provider.go` 的 `/etc/resolv.conf` 占位改为只读 bind mount 指向 sing-box tun IP（172.19.0.1）；`/etc/nsswitch.conf` 改为 `files dns`。
3. **数据模型与系统预设** —— 新增 migration `0019_host_bypass_rules.sql` 建五张表；Repository CRUD；seed `loopback`（强制）+ `lan`（可选）两个 system 预设。

**不在本阶段范围：**
- 管理员 API / 后台 UI（Phase 46）
- 热更新链路（`ActionReloadHostBypass`、nft set 原子更新）、安全不变量 CI、E2E 验证（Phase 47）
- `cn-dev` / `oss-dev` / `ai-api` 预设（v3.5 P1 backlog）
- 用户自助配置（v3.5 仅管理员）

</domain>

<decisions>
## Implementation Decisions

所有关键决策已在前置 4 个并行研究 agent 输出中确定（参见 `.planning/research/SUMMARY.md` §1 / §3 / §4 / §5），下面把直接影响 Phase 45 实现的部分原文引用过来：

### sing-box 配置形态（两段式）
- 静态 `config.json` 每 host 一份渲染模板（变更需重启），动态 rule-set 文件由 sing-box 1.10+ 文件 watch 自动 reload（type=`local`，format=`source`）。
- Phase 45 只产出渲染层骨架 + 默认空 rule-set 文件占位；rule-set 文件内容的动态写入与 reload 在 Phase 47。
- `route.rules` 顺序固定：`sniff(tls/http/quic/dns)` → `protocol:"dns" → hijack-dns` → `ip_cidr=proxy_ip/32 → direct-out`（避免回环）→ `ip_is_private:true → direct-out` → `rule_set=whitelist-cidrs → direct-out` → `rule_set=whitelist-domains → direct-out`，`final="proxy-out"`。
- `tun` inbound 启用 `strict_route:true` + `auto_route:true` + `endpoint_independent_nat:true`，`route.default_interface="eth0"`。

### DNS 拆分模型
- `dns.servers`：`dns-local`（type=`local`）+ `dns-proxy`（type=`https`，server=`1.1.1.1`，detour=`proxy-out`，domain_resolver=`dns-local`）。
- `dns.rules`：`.lan/.local/.internal` 后缀 → `dns-local`；命中 `whitelist-domains` rule_set → `dns-proxy`。
- `dns.final="dns-proxy"`、`strategy="ipv4_only"`（v3.5 容器内全禁 IPv6）。

### 容器 DNS 入口锁（Phase 45 决定但实现也归 Phase 45）
- `/etc/resolv.conf` 从 `nameserver 8.8.8.8` 占位改为 bind mount `<host_state_dir>/resolv.conf:/etc/resolv.conf:ro`，内容：
  ```
  nameserver 172.19.0.1
  options ndots:0 single-request-reopen
  ```
- `/etc/nsswitch.conf` bind mount 为 `hosts: files dns`（禁用 mdns/myhostname/wins）。
- 现有 `container_proxy_provider.go:323` 写文件逻辑改造为只读挂载；考虑挂载源文件位置（建议 `/var/lib/cloud-cli-proxy/host/<host_id>/`）。

### 数据模型
- 五张表全部 UUID 主键（`gen_random_uuid()`）+ `TIMESTAMPTZ DEFAULT NOW()`，命名对齐现有 `host_egress_bindings` 风格（snake_case + `host_` 前缀）。
- `host_bypass_presets`：`slug`（unique）、`name`、`is_system`、`is_force_on`、`is_active`、`description`、`rules JSONB`（内嵌 IP/CIDR/域名/后缀/端口规则数组）。
- `host_bypass_rules`：`scope`（`global`/`host`）、`rule_type`（`ip` / `cidr` / `domain` / `domain_suffix` / `domain_keyword` / `port`）、`value`、`note`、`is_risky`。
- `host_bypass_bindings`：`host_id` ↔ `preset_id` 或 `rule_id`（XOR），`enabled`、`source`（`admin`/`system`）。
- `host_bypass_snapshots`：`host_id`、`version`、`config_hash`（幂等键 unique per host）、`whitelist_cidrs_json`、`whitelist_domains_json`、`applied_status`（`pending`/`applied`/`failed`/`rolled_back`，默认 `pending`）、`created_by`。
- `host_bypass_audit_log`：`actor_id`、`actor_ip`、`action`、`target_kind`、`target_id`、`before` JSONB、`after` JSONB、`note`、`created_at`；默认保留 90 天（保留策略不在本阶段实现）。

### 系统预设种子（migration 内置）
- `loopback`：`127.0.0.0/8`、`169.254.0.0/16`，`is_system=true`、`is_force_on=true`、`is_active=true`。
- `lan`：RFC1918（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`）+ CGNAT `100.64.0.0/10` + ULA `fc00::/7`，`is_system=true`、`is_force_on=false`、`is_active=true`。
- migration 写 seed；后续 API（Phase 46）禁止删除或修改 `is_system=true` 行。

### Repository CRUD
- 放在 `internal/store/repository/`，跟随 `queries.go` 现有风格（pgx + 单 Repository 接口聚合）。
- 提供 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` 四组 CRUD（`audit_log` 仅 `Insert` + `List by target` 两个方法）。
- 错误处理沿用项目现有 `NetworkError` 风格。
- 单元测试用真实 Postgres（已有 testcontainer 模式），不 mock。

### Claude's Discretion（具体实现细节由 planner / executor 决定）
- 表结构 SQL 的列顺序、index 选择、约束写法（CHECK / UNIQUE）。
- bind mount 源文件的具体路径（建议 `<host_state_dir>/resolv.conf` 与 `<host_state_dir>/nsswitch.conf`）。
- `whitelist_cidrs_json` / `whitelist_domains_json` 在 snapshot 表里的存储形式（TEXT vs JSONB）—— 倾向 JSONB 便于查询；具体由 planner 决定。
- 是否拆分 sing-box config 渲染为多个 Go 文件（如 `dns_config.go` / `route_rules.go`）—— 优先保留单文件 + 内部辅助函数；如果超过 300 行可考虑拆分。
- `0019_host_bypass_rules.sql` 是否拆成多个 migration（如先建表后 seed）—— 倾向单文件，参考 `0014_claude_account_persistent_volume.sql` 模式。

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

| 资产 | 路径 | 复用方式 |
|------|------|----------|
| sing-box 配置渲染入口 | `internal/network/gateway_singbox_config.go:10` | 扩展 `BuildGatewaySingBoxConfig` 函数，加 `dns`、`route.rule_set`、`route.rules` 重排逻辑 |
| sing-box 配置测试 | `internal/network/gateway_singbox_config_test.go` | 沿用 table-driven test 模式，增加白名单 / DNS 拆分用例 |
| 容器 provider | `internal/network/container_proxy_provider.go:323` | 改 resolv.conf 占位生成逻辑为只读挂载 |
| 防火墙骨架 | `internal/network/worker_firewall_linux.go:25` | Phase 47 才扩展；Phase 45 不动 |
| 验证脚本基线 | `internal/network/verify.go` | Phase 47 才扩展；Phase 45 不动 |
| Repository 模式参考 | `internal/store/repository/queries.go` + `models.go` | 新增 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` 类型与方法 |
| Migration 模板 | `internal/store/migrations/0018_user_centric_credentials.sql` | 新增 `0019_host_bypass_rules.sql` 沿用相同 header / down 段 / seed insert 风格 |
| Migration 测试 | `migration_0014_test.go` | 新增 `migration_0019_test.go` 验证表 / 索引 / seed 一致 |

### Established Patterns

- **pgx + UUID 主键 + TIMESTAMPTZ**：所有表 `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`，`created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`。
- **JSONB for variable structure**：snapshot 文件内容、audit log diff、preset 内嵌规则数组用 JSONB；不另外建外键。
- **Repository 单接口聚合**：`Repository` 接口在 `internal/store/repository/queries.go` 聚合所有方法；新表实现同样模式。
- **Migration 序号递增 + seed 在同一文件**：0019 既建表又 INSERT 两条系统预设；down 段 DROP TABLE。
- **测试用真实 Postgres**：testcontainer 启动，迁移跑完再跑测试；不 mock SQL。
- **sing-box 配置 Go map 拼装**：现有 `gateway_singbox_config.go` 用 `map[string]any` + `json.Marshal`，不引入新模板引擎。

### Integration Points

- **sing-box 配置渲染** ← gateway 容器启动时调用 `BuildGatewaySingBoxConfig(host)` → 写 `/etc/sing-box/config.json`。Phase 45 需在渲染时确保 `route.rule_set` 引用的文件路径（`/etc/sing-box/whitelist-cidrs.json` 与 `whitelist-domains.json`）能由 gateway 容器读取；Phase 45 可以先生成空 rule-set 文件（`{"version":3,"rules":[]}`）作为 placeholder，确保 sing-box 启动不报错。
- **容器 spec** ← `container_proxy_provider.go:PrepareHost` → Docker create 时加 bind mount。Phase 45 改这里。
- **数据库迁移** ← 进程启动时自动跑 `migrations.RunUp()`。Phase 45 加 0019 即可，无需调用方改动。
- **Repository 注入** ← `internal/controlplane/server.go` 构造 `Repository{...}` 时把新方法暴露给 HTTP handler；Phase 45 只加方法，Phase 46 才在 server.go 接入。

### 现状 vs 目标对照（关键差异）

| 维度 | 当前 | Phase 45 目标 |
|------|------|--------------|
| `route.rules` | 仅 2 条（proxy IP/32 + port 53 hijack-dns） | 7 条（含 sniff / hijack-dns / proxy_ip / ip_is_private / 两个 rule_set / final） |
| `route.rule_set` | 不存在 | 2 个 local 文件引用（whitelist-cidrs / whitelist-domains），初始内容为空 |
| `dns` | 单一 server（依赖默认 resolv.conf） | 拆分 `dns-local` + `dns-proxy`，规则按域名后缀和 rule_set 路由 |
| `tun` inbound | 已有 `auto_route` | 加 `strict_route` + `endpoint_independent_nat` |
| `route.default_interface` | 未设置 | 显式 `eth0` |
| `/etc/resolv.conf` | 写入 `nameserver 8.8.8.8`（容器内可改） | 只读挂载 `nameserver 172.19.0.1` |
| `/etc/nsswitch.conf` | 镜像默认（含 mdns 等） | 只读挂载 `hosts: files dns` |
| DB 表 | 无 bypass 表 | 5 张表 + 2 个系统预设 seed |

</code_context>

<specifics>
## Specific Ideas

- migration 顺序号 `0019_host_bypass_rules.sql`（现有最大序号 0018）。
- rule-set 文件初始空内容采用 sing-box source 格式 `{"version":3,"rules":[]}` 作为 placeholder，避免 sing-box 启动失败。
- `host_bypass_snapshots.config_hash` 推荐用 SHA-256 hex（64 字符）；作为 unique key 实现幂等 apply。
- 测试 host 渲染配置时需要锁定 RNG / 时间戳（如有），保证 `config_hash` 在相同输入下稳定。
- 不要在 Phase 45 改动 `worker_firewall_linux.go` 或 `verify.go`，避免与 Phase 47 冲突。

</specifics>

<deferred>
## Deferred Ideas

- 远程 rule-set 拉取（MetaCubeX/meta-rules-dat 镜像）→ v3.5 P1（`BYPASS-RULESET-REMOTE`）
- 命中统计（sing-box Clash API `/connections` 轮询）→ v3.5 P1（`BYPASS-HIT-STATS`）
- 流量 dashboard → v3.5 P1（`BYPASS-DASHBOARD`）
- 用户自助白名单（区分管理员/用户权限）→ v3.6+（`BYPASS-USER-SELF`）
- `domain_regex` 高级规则 → 性能与误用风险，延后
- 多租户白名单（按 tenant_id 隔离）→ v1 单宿主机单租户，未来再加
- FakeIP 模式 → 主流量 SSH 不会被 sniff，FakeIP 会让 SSH 锁死在假 IP 段，永不引入
- IPv6 双栈 → v3.5 容器内全禁 IPv6，未来 ip6tables 对称就绪再开

</deferred>
