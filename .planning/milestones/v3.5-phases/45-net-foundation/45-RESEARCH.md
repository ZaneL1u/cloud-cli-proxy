# Phase 45: 网络配置基础与数据模型 - Research

**Researched:** 2026-05-12
**Domain:** sing-box 配置渲染 / 容器 DNS 入口锁 / Postgres schema 与 Repository
**Confidence:** HIGH（关键决策已在 SUMMARY.md 锁定；本研究主要做现状勘察与落地路径细化）

## Summary

Phase 45 是 v3.5 milestone 的第一阶段，目标是把三个基础设施层铺到位，给 Phase 46（API/UI）和 Phase 47（热更新+验证）腾出干净接口。研究确认：所有关键技术选型（两段式 sing-box 配置、拆分 DNS、bind mount resolv.conf、五张表 + 系统预设 seed）已在前置 4 个并行研究 agent 中确定并被 CONTEXT.md 收录，本阶段不再讨论替代方案。

本次勘察的核心增量信息：

- **当前 `gateway_singbox_config.go` 距 Phase 45 目标差距巨大但都是增量改造**：只有 2 条 route.rules、单 DNS server、strict_route=false、auto_detect_interface（非显式 eth0），没有 rule_set 字段。需要新增的字段全部是 sing-box 1.10+ 已支持的稳定 API，没有未知风险。
- **容器 `/etc/resolv.conf` 改造是个跨多文件的改动**：当前 `container_proxy_provider.go:323` 用 `docker exec sh -c 'echo nameserver 8.8.8.8 > /etc/resolv.conf'` 写盘；同 package 的 `dns.go::ConfigureContainerDNS` 是旧路径（由 `singbox_provider_linux.go` 调用，**不在当前主流程**），改造时需注意只动 `ContainerProxyProvider`。`container_proxy_provider_test.go:246/283` 还断言 `'nameserver 8.8.8.8'` 字符串，必须同步改测试。`verify.go:79 verifyDNS` 拿 `expected.Proxy.DNSServer` 比对 resolv.conf 第一行——bind mount 改 `172.19.0.1` 后，`EgressConfig.Proxy.DNSServer` 也必须随之改为 `172.19.0.1`（或 verify 比对逻辑收成常量），否则 verify 会失败。
- **Repository 测试模式被 CONTEXT.md 误描述为 "testcontainer + 真实 Postgres"**：实际仓库里 `internal/store/repository/*_test.go`（migration_0014_test、queries_contract_test、queries_claude_account_*）全部是 **SQL 文本与反射签名断言**，没有 testcontainer 依赖，也没有 `setupTestDB` helper。Phase 45 应继续沿用这个模式：迁移文件做 `os.ReadFile` + 字符串包含断言；新 Repository 方法做反射签名断言 + SQL 常量提升为包级变量供测试比对。
- **migration 序号有缺号**（0010、0011、0016 缺失，但 migrator 用 `filepath.Glob + sort.Strings` 按文件名排序，缺号不影响），0019 是合规的下一编号。
- **sing-box rule-set source format version 3 对应 sing-box 1.11.0**（不是 SUMMARY 说的 1.10），但 `type:"local" + format:"source"` + 文件 watch 在 1.10 就已经支持。Phase 45 的 placeholder `{"version":3,"rules":[]}` 需要 gateway 镜像至少跑 1.11+；本仓库 `cloud-cli-proxy-sing-gateway:local` 镜像版本需要在 plan 里勘察并明确。

**Primary recommendation:** 把 Phase 45 切成 3 个独立、可并发实现的 plan：(1) sing-box 渲染层扩展 + 单元测试；(2) 容器 bind mount 改造（resolv.conf + nsswitch.conf + 同步更新 EgressConfig.DNSServer 默认值与对应测试）；(3) migration 0019 + Repository CRUD + SQL 文本断言测试。三者之间唯一耦合点是 `gatewayConfigDir(hostID)` 这个 DATA_DIR 子目录约定 —— rule-set placeholder 文件与 resolv.conf/nsswitch.conf 都建议复用该目录，避免引入第二个 host_state 路径常量。

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| sing-box config.json 渲染 | 控制面 Go（`internal/network/`） | gateway 容器（消费 ro mount） | 渲染纯在控制面，容器只挂载结果；保持渲染逻辑可单元测试 |
| Rule-set placeholder 文件生成 | 控制面 Go（`gatewayConfigDir`） | — | Phase 45 仅生成空文件占位，动态写入归 Phase 47 host-agent |
| `/etc/resolv.conf` ro 挂载 | 控制面 Go（写源文件 + docker run -v 参数） | worker 容器（消费） | 源文件落 DATA_DIR，bind mount 由 docker create 时声明 |
| `/etc/nsswitch.conf` ro 挂载 | 控制面 Go（同上） | worker 容器 | 同 resolv.conf |
| 数据模型 schema | Postgres（migration） | Repository Go（接口） | 表结构落 SQL，类型与方法落 Go |
| 系统预设 seed | migration SQL `INSERT ... ON CONFLICT DO NOTHING` | — | 单文件幂等 seed，符合 0001 的初始 admin 模式 |
| 不可删除 `is_system` 约束 | 应用层（Phase 46 handler） | 数据库 CHECK 约束（可选加固） | Phase 45 只准备 `is_system` 列；约束执行归 API 层 |
| Repository CRUD | 控制面 Go（`internal/store/repository/`） | — | 沿用 pgx 单 Repository 结构体聚合方法的现有模式 |

## User Constraints (from CONTEXT.md)

### Locked Decisions

**sing-box 配置形态（两段式）：**

- 静态 `config.json` 每 host 一份渲染模板（变更需重启），动态 rule-set 文件由 sing-box 文件 watch 自动 reload（type=`local`，format=`source`）。
- Phase 45 只产出渲染层骨架 + 默认空 rule-set 文件占位；rule-set 文件内容的动态写入与 reload 在 Phase 47。
- `route.rules` 顺序固定：`sniff(tls/http/quic/dns)` → `protocol:"dns" → hijack-dns` → `ip_cidr=proxy_ip/32 → direct-out`（避免回环）→ `ip_is_private:true → direct-out` → `rule_set=whitelist-cidrs → direct-out` → `rule_set=whitelist-domains → direct-out`，`final="proxy-out"`。
- `tun` inbound 启用 `strict_route:true` + `auto_route:true` + `endpoint_independent_nat:true`，`route.default_interface="eth0"`。

**DNS 拆分模型：**

- `dns.servers`：`dns-local`（type=`local`）+ `dns-proxy`（type=`https`，server=`1.1.1.1`，detour=`proxy-out`，domain_resolver=`dns-local`）。
- `dns.rules`：`.lan/.local/.internal` 后缀 → `dns-local`；命中 `whitelist-domains` rule_set → `dns-proxy`。
- `dns.final="dns-proxy"`、`strategy="ipv4_only"`（v3.5 容器内全禁 IPv6）。

**容器 DNS 入口锁：**

- `/etc/resolv.conf` 从 `nameserver 8.8.8.8` 占位改为 bind mount，内容固定：
  ```
  nameserver 172.19.0.1
  options ndots:0 single-request-reopen
  ```
- `/etc/nsswitch.conf` bind mount 为 `hosts: files dns`（禁用 mdns/myhostname/wins）。
- 源文件建议放在 `<host_state_dir>/resolv.conf` 与 `<host_state_dir>/nsswitch.conf`，与 sing-box config 同根。

**数据模型：**

- 五张表全部 UUID 主键（`gen_random_uuid()`）+ `TIMESTAMPTZ DEFAULT NOW()`，命名对齐现有 `host_egress_bindings` 风格（snake_case + `host_` 前缀）。
- `host_bypass_presets`：`slug`（unique）、`name`、`is_system`、`is_force_on`、`is_active`、`description`、`rules JSONB`。
- `host_bypass_rules`：`scope`（`global`/`host`）、`rule_type`（`ip` / `cidr` / `domain` / `domain_suffix` / `domain_keyword` / `port`）、`value`、`note`、`is_risky`。
- `host_bypass_bindings`：`host_id` ↔ `preset_id` 或 `rule_id`（XOR），`enabled`、`source`（`admin`/`system`）。
- `host_bypass_snapshots`：`host_id`、`version`、`config_hash`（unique per host）、`whitelist_cidrs_json`、`whitelist_domains_json`、`applied_status`（`pending`/`applied`/`failed`/`rolled_back`，默认 `pending`）、`created_by`。
- `host_bypass_audit_log`：`actor_id`、`actor_ip`、`action`、`target_kind`、`target_id`、`before` JSONB、`after` JSONB、`note`、`created_at`；默认保留 90 天（保留策略不在本阶段实现）。

**系统预设种子（migration 内置）：**

- `loopback`：`127.0.0.0/8`、`169.254.0.0/16`，`is_system=true`、`is_force_on=true`、`is_active=true`。
- `lan`：RFC1918（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`）+ CGNAT `100.64.0.0/10` + ULA `fc00::/7`，`is_system=true`、`is_force_on=false`、`is_active=true`。

**Repository CRUD：**

- 放在 `internal/store/repository/`，跟随 `queries.go` 现有风格（pgx + 单 Repository 接口聚合）。
- 提供 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` 四组 CRUD（`audit_log` 仅 `Insert` + `List by target` 两个方法）。
- 错误处理沿用项目现有 `NetworkError` 风格（参见 `internal/network/errors.go`，本研究下方 Section §Code Examples 已引用）。

### Claude's Discretion

- 表结构 SQL 的列顺序、index 选择、约束写法（CHECK / UNIQUE）。
- bind mount 源文件的具体路径（推荐复用 `gatewayConfigDir(hostID)` 即 `<DATA_DIR>/gateway/<host_id>/`，但 planner 可改）。
- `whitelist_cidrs_json` / `whitelist_domains_json` 在 snapshot 表里的存储形式（TEXT vs JSONB）—— 倾向 JSONB 便于查询；具体由 planner 决定。
- 是否拆分 sing-box config 渲染为多个 Go 文件（如 `dns_config.go` / `route_rules.go`）—— 优先保留单文件 + 内部辅助函数；如果超过 300 行可考虑拆分。
- `0019_host_bypass_rules.sql` 是否拆成多个 migration（如先建表后 seed）—— 倾向单文件，参考 0014 模式。

### Deferred Ideas (OUT OF SCOPE)

- 管理员 API / 后台 UI（Phase 46）
- 热更新链路（`ActionReloadHostBypass`、nft set 原子更新）、安全不变量 CI、E2E 验证（Phase 47）
- `cn-dev` / `oss-dev` / `ai-api` 预设（v3.5 P1 backlog）
- 用户自助配置（v3.5 仅管理员）
- 远程 rule-set 拉取（MetaCubeX/meta-rules-dat 镜像）→ P1
- 命中统计、流量 dashboard → P1
- `domain_regex` 高级规则 → 延后
- FakeIP / IPv6 双栈 → 永不引入 / v1 后再开

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| BYPASS-NET-01 | sing-box 两段式配置（静态 config.json + 动态 local rule-set 文件） | §Standard Stack §Architecture Patterns Pattern 1 / Code Example 1 |
| BYPASS-NET-02 | `route.rules` 引入 `ip_is_private:true` + `whitelist-cidrs` / `whitelist-domains` rule_set + `final:"proxy-out"` | §Code Example 1（route.rules 七条顺序） |
| BYPASS-NET-03 | `tun` inbound 启用 `strict_route` / `auto_route` / `endpoint_independent_nat`；`route.default_interface="eth0"` | §Code Example 1 tun 段；§Common Pitfalls Pitfall 1 |
| BYPASS-NET-04 | `route.rules` 首条 `action:"sniff"` + `sniffer:["tls","http","quic","dns"]`；`protocol:"dns"` 用 `action:"hijack-dns"`（替代旧 `outbound:"dns"`） | §Code Example 1；sing-box 官方文档已确认 `hijack-dns` 是最终 action（来源链接见 §Sources） |
| BYPASS-DNS-01 | `dns.servers` 含 `dns-local`+`dns-proxy(DoH)`；`dns.final="dns-proxy"`、`strategy:"ipv4_only"` | §Code Example 1 dns 段 |
| BYPASS-DNS-02 | `.lan/.local/.internal` → `dns-local`；`whitelist-domains` 命中 → `dns-proxy` | §Code Example 1 dns.rules |
| BYPASS-DNS-03 | 容器 `/etc/resolv.conf` 改 ro bind mount → `nameserver 172.19.0.1` + `options ndots:0 single-request-reopen` | §Architecture Patterns Pattern 2；§Common Pitfalls Pitfall 2、3 |
| BYPASS-DNS-04 | 容器 `/etc/nsswitch.conf` 改 ro bind mount → `hosts: files dns` | §Architecture Patterns Pattern 2 |
| BYPASS-DATA-01 | migration 0019 建五张表 | §Code Example 2 migration 骨架 |
| BYPASS-DATA-02 | Repository 层四组 CRUD + audit_log 双方法，沿用 `NetworkError` 风格 | §Code Example 3 Repository 方法骨架 |
| BYPASS-DATA-03 | migration seed `loopback`（force on）+ `lan`（默认关闭），`is_system=true` 不可删 | §Code Example 2 seed 段 |
| BYPASS-DATA-04 | snapshot 表记录 version / config_hash / 两个 JSON 字段 / applied_status / created_by | §Code Example 2 snapshot 段；§Common Pitfalls Pitfall 4 |

## Standard Stack

### Core（项目已锁定，无需重新评估）
| 组件 | 版本（仓库实际） | 用途 | 来源 |
|------|------|------|------|
| Go | 1.25.7（go.mod；CLAUDE.md 规划 1.26.1，存在差距但不影响 Phase 45） | 渲染、Repository、handler | `go.mod:3` |
| PostgreSQL | 18.x | 五张表持久化 | CLAUDE.md 技术栈 |
| pgx | v5 | DB 驱动 | `internal/store/repository/queries.go:10-11` |
| sing-box | 镜像 `cloud-cli-proxy-sing-gateway:local`（具体版本需 plan 阶段勘察） | gateway 容器运行时；必须 ≥ 1.11（rule-set source format v3） | `internal/network/container_proxy_provider.go:180` |
| Docker Engine | 28.x | bind mount via `docker run -v src:dst:ro` | CLAUDE.md 技术栈 |
| pgcrypto | Postgres extension | `gen_random_uuid()` | `0001_initial.sql:1`（已启用） |

### Supporting（本阶段无新增依赖）
| 组件 | 用途 | 何时使用 |
|------|------|----------|
| `encoding/json` 标准库 | sing-box 配置渲染、JSONB 字段 marshal | 现有 `gateway_singbox_config.go` 已用，沿用即可 |
| `os` / `os.WriteFile` | 写 rule-set placeholder、resolv.conf、nsswitch.conf 源文件 | 沿用 `container_proxy_provider.go:67` 同款写盘逻辑 |

### Alternatives Considered（本阶段 NOT 考虑）
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Go map+json.Marshal | text/template 渲染 sing-box config | 现有代码已用 map 风格，引入 template 增加心智负担且不解决任何问题 |
| 手写 pgx | sqlc 代码生成 | 仓库当前完全手写 pgx，引入 sqlc 是跨项目级改造，不属于 Phase 45 |
| JSONB 内嵌 preset.rules | preset_rules join 表 | CONTEXT 已锁定 JSONB；join 表会让 seed 与 v3.5 P1 远程 rule-set 复用变复杂 |

**Installation:** 无新增依赖，本阶段不需要 `go get`。

**Version verification:** 关键组件版本均已在仓库内固化：

```bash
# 验证 go.mod Go 版本
grep "^go " /Users/.../cloud-cli-proxy/go.mod  # 1.25.7
# 验证 pgcrypto 已启用
grep pgcrypto internal/store/migrations/0001_initial.sql
# 验证 gateway 镜像名（plan 阶段需进一步确认镜像内 sing-box 版本 ≥ 1.11）
grep -n "sing-gateway" internal/network/container_proxy_provider.go
```

`[VERIFIED: 仓库 grep]` Go 1.25.7、pgcrypto 已启用、镜像名 `cloud-cli-proxy-sing-gateway:local` 通过 env `CLOUD_CLI_PROXY_GATEWAY_IMAGE` 可覆盖。

`[ASSUMED]` 镜像内 sing-box 版本 ≥ 1.11 —— 本研究未运行 `docker inspect`，plan 阶段第一个任务建议核实。如果当前镜像还是 1.10.x，Phase 45 需要把镜像升级也纳入范围（影响 Phase 46/47）。

## Architecture Patterns

### System Architecture Diagram

```
                        ┌────────────────────────────┐
                        │  控制面 Go (Phase 45 改造点) │
                        │                            │
       host create  ──▶ │  ContainerProxyProvider    │
                        │  .PrepareHost()            │
                        │  ├─ 渲染 config.json       │ ──┐
                        │  ├─ 生成空 rule-set 文件   │   │
                        │  ├─ 生成 resolv.conf       │   │
                        │  ├─ 生成 nsswitch.conf     │   │
                        │  └─ docker run -v ...:ro   │   │
                        │                            │   │
                        │  Repository                │   │  写盘到
                        │  ├─ BypassPreset CRUD      │   │  <DATA_DIR>/gateway/<host_id>/
                        │  ├─ BypassRule CRUD        │   │
                        │  ├─ BypassBinding CRUD     │   │
                        │  ├─ BypassSnapshot CRUD    │   │
                        │  └─ BypassAuditLog 双方法  │   │
                        └────────────────────────────┘   │
                                  │ pgx                  │
                                  ▼                      │
                        ┌────────────────────────────┐   │
                        │  Postgres 18                │   │
                        │  - host_bypass_presets     │   │
                        │  - host_bypass_rules       │   │
                        │  - host_bypass_bindings    │   │
                        │  - host_bypass_snapshots   │   │
                        │  - host_bypass_audit_log   │   │
                        │  seed: loopback / lan      │   │
                        └────────────────────────────┘   │
                                                         ▼
                                              ┌──────────────────┐
                                              │  gateway 容器     │
                                              │  /etc/sing-box/   │
                                              │  ├─ config.json   │ (ro)
                                              │  ├─ whitelist-    │
                                              │  │   cidrs.json   │ (ro placeholder)
                                              │  └─ whitelist-    │
                                              │      domains.json │ (ro placeholder)
                                              │  sing-box tun0:   │
                                              │   172.19.0.1      │
                                              └──────────────────┘
                                                       ▲
                                                       │  bind mount + 默认路由
                                                       │
                                              ┌──────────────────┐
                                              │  worker 容器       │
                                              │  /etc/resolv.conf │ (ro: nameserver 172.19.0.1)
                                              │  /etc/nsswitch.conf│ (ro: hosts: files dns)
                                              │  /etc/hosts        │ (默认值，不动)
                                              └──────────────────┘
```

### Component Responsibilities

| 组件 | 文件 | 责任（Phase 45 范围） |
|------|------|----------------------|
| sing-box 配置渲染 | `internal/network/gateway_singbox_config.go` | 扩展 route.rules（2 → 7 条）、rule_set、dns 拆分、tun 加固 |
| sing-box 配置测试 | `internal/network/gateway_singbox_config_test.go` | 扩展 table-driven 覆盖白名单 / DNS 拆分 / strict_route 等用例 |
| 容器 provider | `internal/network/container_proxy_provider.go` | 改造 resolv.conf 写盘逻辑为 bind mount；新增 nsswitch.conf 与 rule-set placeholder |
| Provider 测试 | `internal/network/container_proxy_provider_test.go` | 同步更新硬编码字符串断言（`'nameserver 8.8.8.8'` → 新挂载校验） |
| DNS server 默认值 | `internal/network/container_proxy_provider.go:53` 或 `EgressConfig` 默认值源 | 把 `spec.Egress.Proxy.DNSServer` 默认值改成 `172.19.0.1`（影响 verify.go） |
| Migration | `internal/store/migrations/0019_host_bypass_rules.sql` | 五张表 + 两条 seed |
| Migration 测试 | `internal/store/repository/migration_0019_test.go`（新增） | SQL 文本断言（参考 0014_test 模式） |
| Repository 类型 | `internal/store/repository/models.go` | 新增 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` / `BypassAuditLog` 与 `*Params` 结构 |
| Repository 方法 | `internal/store/repository/queries.go`（或新文件 `queries_bypass.go`） | 四组 CRUD + 审计日志双方法 |
| Repository 测试 | `internal/store/repository/queries_bypass_test.go`（新增） | SQL 常量提升 + 反射签名断言（参考 `queries_contract_test.go`） |

### Recommended Project Structure

```
internal/
├── network/
│   ├── gateway_singbox_config.go        # 扩展（保留单文件，加内部 helper）
│   ├── gateway_singbox_config_test.go   # 扩展用例
│   ├── container_proxy_provider.go      # 改造 resolv.conf 占位 + 加 bind mount
│   ├── container_proxy_provider_test.go # 同步更新硬编码断言
│   └── (不要新增 bypass.go —— rule-set 渲染归 Phase 47)
└── store/
    ├── migrations/
    │   └── 0019_host_bypass_rules.sql   # 新增（建表 + seed 单文件）
    └── repository/
        ├── models.go                    # 新增五组类型
        ├── queries.go                   # 可扩展现有大文件，或：
        ├── queries_bypass.go            # 新文件，仅放 bypass 相关方法（推荐）
        ├── migration_0019_test.go       # 新增（SQL 文本断言）
        └── queries_bypass_test.go       # 新增（SQL 常量 + 签名断言）
```

### Pattern 1: sing-box 两段式配置（静态 config + 动态 rule-set）

**What:** `config.json` 引用 `route.rule_set` 数组中的 `type:"local"` 文件路径；sing-box 1.10+ 监听文件 mtime，文件被原子替换（tmpfile + rename）后自动 reload，**进程不重启、现有连接不断**。

**When to use:** 任何「频繁变化的白名单/黑名单 + 静态进程拓扑」场景。Phase 45 只产出静态侧 + 空 placeholder；Phase 47 才接管动态侧的写入。

**Example:** 见下方 §Code Examples Example 1。

### Pattern 2: 容器配置文件只读 bind mount

**What:** 控制面在 `<DATA_DIR>/gateway/<host_id>/` 写源文件，docker create 时通过 `-v src:dst:ro` 挂入容器；容器内进程（含 root）无法修改。

**When to use:** 任何「容器需要确定性配置且不能被容器内修改」的场景。本阶段用于 resolv.conf + nsswitch.conf。

**Example:**

```go
// 写源文件
hostStateDir := gatewayConfigDir(hostID) // 已有 helper：<DATA_DIR>/gateway/<host_id>/
resolvPath := filepath.Join(hostStateDir, "resolv.conf")
nsswitchPath := filepath.Join(hostStateDir, "nsswitch.conf")

if err := os.WriteFile(resolvPath, []byte("nameserver 172.19.0.1\noptions ndots:0 single-request-reopen\n"), 0o644); err != nil {
    return fmt.Errorf("write resolv.conf: %w", err)
}
if err := os.WriteFile(nsswitchPath, []byte("hosts: files dns\n"), 0o644); err != nil {
    return fmt.Errorf("write nsswitch.conf: %w", err)
}

// docker run 时挂入（worker 容器 create 路径需要新增；Phase 45 需要明确这是 worker 还是 gateway）
// Phase 45 范围：worker 容器拿到这两个 ro mount
// 注意：当前 dockerRunGateway 在 :223-244，但 worker 容器创建在 runtime/tasks/worker.go
// Plan 阶段需明确改 worker create 路径而不是 gateway
```

### Pattern 3: pgx Repository 单结构体聚合

**What:** 所有数据库方法挂在 `*Repository` 上（`internal/store/repository/queries.go:14`），方法签名 `func (r *Repository) DoX(ctx, params) (T, error)`；SQL 字符串提升为包级 `const xxxSQL = ...` 供测试断言。

**When to use:** 全仓库统一风格，本阶段必须沿用。

**Example:** 见 §Code Examples Example 3。

### Anti-Patterns to Avoid

- **不要新建 `BypassRepository` 子类型或单独的 store struct**：现有项目把所有 CRUD 挂在单个 `*Repository`，分裂会破坏 handler 注入路径（参见 `internal/controlplane/app/app.go` 单点 Repository 注入）。
- **不要在 Phase 45 给 `is_system=true` 行加 DB trigger / RULE 拦截删除**：CONTEXT 把不可删除约束归到 Phase 46 应用层；DB 级 trigger 是后续加固选项，本阶段加进去会扩大测试矩阵。
- **不要把 rule-set 文件路径常量散落在多个 .go 文件**：建议在 `internal/network/gateway_singbox_config.go` 顶部定义 `const ruleSetWhitelistCIDRsPath = "/etc/sing-box/whitelist-cidrs.json"` 等常量，供渲染与 placeholder 写盘共享。
- **不要在改 `container_proxy_provider.go:323` 时偷偷调用旧的 `dns.go::ConfigureContainerDNS`**：那是 `SingBoxProvider` 路径，本阶段不动；混淆会污染主流程。
- **不要在 migration 0019 里用 ENUM 类型**：项目历史没有使用 PG ENUM 的先例，全部用 TEXT + CHECK 约束（参考 0001 `egress_ips.status`）。

## Don't Hand-Roll

| 问题 | 不要手写 | 用什么替代 | 原因 |
|------|---------|-----------|------|
| sing-box 配置文件 watch / reload | 自己写 inotify + 信号 | sing-box 1.10+ `route.rule_set[].type:"local"` | sing-box 已实现，自己再封一层只会带来一致性 bug |
| UUID 生成 | `uuid.New()` 应用层生成 | Postgres `gen_random_uuid()` | 项目所有表统一在 DB 端生成，0001 已启用 pgcrypto |
| 时间戳 | Go side `time.Now()` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | 同上，保持与现有表一致 |
| 私有网段判定 | 手写 `127.0.0.0/8`+`10.0.0.0/8`+... 的 IP 比较 | sing-box `ip_is_private: true` | sing-box 1.8+ 内置，覆盖 RFC1918/CGNAT/链路本地/ULA |
| Migration 序号管理 | 自己跑脚本拼 schema_migrations | `internal/store/migrator.go:14-70` 现有 RunMigrations | 已有完整实现，按 filename glob+sort，加 0019 即可 |
| Schema 文本断言测试 | testcontainer 启动真实 PG | `os.ReadFile + strings.Contains`（见 `migration_0014_test.go`） | 项目历史就是这个模式；testcontainer 引入会让 CI 慢 1 分钟 |

**Key insight:** 这个阶段的所有"复杂逻辑"都已经在 sing-box / pgcrypto / migrator.go / pgx 里实现过了。Phase 45 的工作是**正确组装**这些原语，而不是重新发明它们。

## Runtime State Inventory

> 本阶段是新增表 + 改造容器启动流程，不涉及对老 host 的数据迁移；但容器侧 bind mount 改造会影响存量 running host。

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| 存储数据 | Postgres 五张表是新增，无存量数据 | 仅建表，不需要数据迁移 |
| 活的服务配置 | 现有 running host 容器内 `/etc/resolv.conf` 仍指向 `8.8.8.8`（由 `container_proxy_provider.go:323` 设置） | Phase 45 改造后**新建/重建**的 host 才会拿到 bind mount；存量 running host 无影响。文档需明确「白名单功能仅对新 host 生效，存量 host 须重启」。 |
| OS 注册状态 | 无（Phase 45 不动 systemd / Task Scheduler） | 无 |
| 密钥/环境变量 | `CLOUD_CLI_PROXY_GATEWAY_IMAGE` env（gateway 镜像覆盖）；`DATA_DIR` env（gateway 配置根目录） | 无新增，沿用 |
| 构建工件 / 已安装包 | gateway 容器镜像 `cloud-cli-proxy-sing-gateway:local` 内的 sing-box 二进制版本 | **必须 ≥ 1.11**（rule-set source format v3 才支持）。Plan 阶段第一步勘察镜像内版本，若 < 1.11 需在 Phase 45 内升级镜像或退到 v2 schema |

**The canonical question:** 「现有所有 running 容器在 Phase 45 merge 后是否还能正常工作？」

答案：**是**（前提）。Phase 45 改的是**新 host 启动流程**：

- migration 0019 只新增表，不动旧表。
- sing-box config 改造只影响**新启动**的 gateway 容器（旧的不重启就不重渲染）。
- bind mount 只在**新创建**的 worker 容器生效（重启 docker container 不会自动加 mount，必须 recreate）。
- 文档侧应记一条「存量 host 升级路径：Phase 45 merge 后，需要 admin 重建（先 archive 再新建）才能拿到白名单能力」—— Phase 46 UI 文案要体现。

## Common Pitfalls

### Pitfall 1: sing-box `auto_route` 没配 `default_interface` → direct outbound 回环

**What goes wrong:** `tun.auto_route=true` 会把 default route 切到 tun0；如果 `direct-out` 没有 `bind_interface`/`default_interface`，sing-box 自己回环到 tun，白名单流量永远绕回代理。

**Why it happens:** auto_route 改全局路由，所有不显式指 interface 的 outbound 都会被抢。

**How to avoid:** `route.default_interface="eth0"` 显式声明；同时 `direct-out` 显式 `bind_interface:"eth0"`（双重保险，参考现有 `internal/network/singbox_config.go:62` 给 worker 容器的 `bind_interface:"mgmt0"` 模式）。

**Warning signs:** 渲染后的 config.json grep 不到 `"default_interface"`，或 direct outbound 缺 `bind_interface`。

### Pitfall 2: bind mount resolv.conf 在 sing-box 启动前不可达 → 容器 DNS 全断

**What goes wrong:** worker 容器创建时 resolv.conf 就被锁死指向 `172.19.0.1`，但此时 gateway 容器（跑 sing-box）可能还没启动到能监听 tun0 的状态；worker 内的任何 DNS 查询（包括 entrypoint 脚本里 `apt-get` / `curl` 域名）都会失败。

**Why it happens:** 当前 `ContainerProxyProvider.PrepareHost` 顺序（参见 `container_proxy_provider.go:60-114`）：teardown → 写 config → 创建网络 → run gateway → `waitGatewayHealthy` → connect worker → configure worker egress（含 resolv.conf）。所以**gateway 在 worker DNS 锁死之前已经 healthy**，理论上不会回退。

**How to avoid:** 严格保持启动顺序（gateway healthy → worker connect → 配 DNS）；plan 阶段验证 `waitGatewayHealthy` 至少检查到 tun0 接口存在（当前只检查容器 Running + logs 无 FATAL，可能需要加 nsenter 看 tun0）。

**Warning signs:** worker 容器 first-start 日志出现 `Could not resolve host` 类型错误；多次 host 创建偶发失败。

### Pitfall 3: `verify.go::verifyDNS` 比对的是 `Proxy.DNSServer` —— 不改它会让所有 host 创建 verify 失败

**What goes wrong:** `verify.go:79-105` 读 `/etc/resolv.conf` 第一条 nameserver，与 `expected.Proxy.DNSServer` 比对；bind mount 改成 `172.19.0.1` 后，如果 `EgressConfig.Proxy.DNSServer` 还是 `8.8.8.8`/`1.1.1.1`/原值，verify 立刻报 `net.dns_leak` 错误，host 全部进 failed。

**Why it happens:** verify 是真实流量验证，期望值来自调用方传入的 EgressConfig；当前调用链 `ContainerProxyProvider.PrepareHost:114` 直接传 `*spec.Egress`，未经改写。

**How to avoid:** Phase 45 必须同时改三处：
1. bind mount 内容固定 `nameserver 172.19.0.1`；
2. `EgressConfig.Proxy.DNSServer`（或 verify 的 expected 值）改为 `172.19.0.1`（推荐做法：在 verify 内常量化，不依赖 egress 配置；或 PrepareHost 调 verify 前把 expected.Proxy.DNSServer 改写为 `172.19.0.1`）；
3. 同步更新 `container_proxy_provider_test.go:246/283` 硬编码字符串。

**Warning signs:** Phase 45 merge 后 UAT 新建 host 卡在 `net.dns_leak` 事件。

### Pitfall 4: snapshot 表 unique 索引设计错误导致 apply 不幂等

**What goes wrong:** CONTEXT 说 `config_hash` 作为幂等键 `unique per host`；如果索引写成 `UNIQUE(config_hash)`（不带 host_id），跨 host 相同规则集会冲突；如果写成 `UNIQUE(host_id, config_hash)`，apply 同 hash 两次会失败（应该是返回已有 snapshot）。

**How to avoid:** 索引 `UNIQUE(host_id, config_hash)`；apply 路径用 `INSERT ... ON CONFLICT (host_id, config_hash) DO NOTHING RETURNING id`，未冲突走新建，冲突回查既有行（Phase 46 实现）。Phase 45 只建好索引即可。

**Warning signs:** 同一份 effective config 第二次 apply 报 23505 violation 而不是返回 200。

### Pitfall 5: migration 序号缺失误以为可填补

**What goes wrong:** 看见 0010/0011/0016 缺失，可能误以为 0019 应该补 0016；实际上 migrator 用文件名字符串排序，已 applied 的版本号记录在 schema_migrations 表里，不能"补号"。

**How to avoid:** 必须用 0019，不能用 0010/0011/0016（即使空缺）。

**Warning signs:** 误填 0016 后部署，migrator 跳过（已在 schema_migrations 表里）或报 hash mismatch。

### Pitfall 6: `pg_isvalid` 类型选择错误导致 IP 段查询失败

**What goes wrong:** `host_bypass_rules.value` 同时存 `192.168.1.1` / `192.168.0.0/24` / `example.com`，类型选 `TEXT` 是最简，但失去了 `inet`/`cidr` 类型的内置校验。

**How to avoid:** 用 `TEXT` + 应用层校验（Phase 46 实现护栏）；不要混用 `inet` + `TEXT` 列。

## Code Examples

### Example 1: sing-box config.json 七条 route.rules + DNS 拆分（目标渲染输出）

```go
// 来源参考：sing-box 官方文档 https://sing-box.sagernet.org/configuration/route/rule_action/
// 与 https://sing-box.sagernet.org/configuration/dns/server/local/
//
// 注意：这是渲染后的 JSON 形态示意；实际由 buildGatewaySingBoxConfig 用 map[string]any 拼装。
{
  "log": { "level": "info" },
  "dns": {
    "servers": [
      { "tag": "dns-local", "type": "local" },
      { "tag": "dns-proxy", "type": "https", "server": "1.1.1.1",
        "domain_resolver": "dns-local", "detour": "proxy-out" }
    ],
    "rules": [
      { "domain_suffix": [".lan", ".local", ".internal"], "action": "route", "server": "dns-local" },
      { "rule_set": ["whitelist-domains"], "action": "route", "server": "dns-proxy" }
    ],
    "final": "dns-proxy",
    "strategy": "ipv4_only"
  },
  "inbounds": [{
    "type": "tun", "tag": "tun-in",
    "interface_name": "sb-tun0",
    "address": ["172.19.0.1/30"],
    "auto_route": true,
    "strict_route": true,
    "endpoint_independent_nat": true,
    "stack": "system",
    "sniff": true
  }],
  "outbounds": [
    { "type": "<from outbound_config>", "tag": "proxy-out", "server": "<resolved_proxy_ip>" },
    { "type": "direct", "tag": "direct-out", "bind_interface": "eth0" }
  ],
  "route": {
    "default_interface": "eth0",
    "rule_set": [
      { "type": "local", "tag": "whitelist-cidrs",   "format": "source",
        "path": "/etc/sing-box/whitelist-cidrs.json" },
      { "type": "local", "tag": "whitelist-domains", "format": "source",
        "path": "/etc/sing-box/whitelist-domains.json" }
    ],
    "rules": [
      { "action": "sniff", "sniffer": ["tls", "http", "quic", "dns"] },
      { "protocol": "dns", "action": "hijack-dns" },
      { "ip_cidr": ["<proxy_ip>/32"], "action": "route", "outbound": "direct-out" },
      { "ip_is_private": true,        "action": "route", "outbound": "direct-out" },
      { "rule_set": ["whitelist-cidrs"],   "action": "route", "outbound": "direct-out" },
      { "rule_set": ["whitelist-domains"], "action": "route", "outbound": "direct-out" }
    ],
    "final": "proxy-out"
  }
}
```

```json
// Rule-set placeholder（whitelist-cidrs.json / whitelist-domains.json 初始内容）
// 来源：https://sing-box.sagernet.org/configuration/rule-set/source-format/
// version 3 = sing-box 1.11.0+ schema
{
  "version": 3,
  "rules": []
}
```

`[VERIFIED: sing-box 官方文档]` `version:3` 是 sing-box 1.11.0 引入的 schema；`rules:[]` 是空数组合法形式（文档未明示空数组运行时行为，但官方语法允许；plan 阶段如果担心可加非空 stub rule 兜底）。

### Example 2: migration 0019 骨架（建表 + seed）

```sql
-- 0019_host_bypass_rules.sql
-- v3.5 Phase 45：网络白名单/绕过规则的数据基础设施
-- 对齐 BYPASS-DATA-01..04：五张表 + 两条系统预设 seed
-- 命名风格沿用现有 host_egress_bindings；UUID 主键 + TIMESTAMPTZ + JSONB 内嵌结构
--
-- 回滚路径（运维手工执行；migrator 仅 up）：
--   DROP TABLE IF EXISTS host_bypass_audit_log;
--   DROP TABLE IF EXISTS host_bypass_snapshots;
--   DROP TABLE IF EXISTS host_bypass_bindings;
--   DROP TABLE IF EXISTS host_bypass_rules;
--   DROP TABLE IF EXISTS host_bypass_presets;

BEGIN;

CREATE TABLE IF NOT EXISTS host_bypass_presets (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug          TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    description   TEXT,
    is_system     BOOLEAN NOT NULL DEFAULT FALSE,
    is_force_on   BOOLEAN NOT NULL DEFAULT FALSE,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    rules         JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS host_bypass_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope        TEXT NOT NULL CHECK (scope IN ('global', 'host')),
    host_id      UUID REFERENCES hosts(id) ON DELETE CASCADE,
    rule_type    TEXT NOT NULL CHECK (rule_type IN ('ip','cidr','domain','domain_suffix','domain_keyword','port')),
    value        TEXT NOT NULL,
    note         TEXT,
    is_risky     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- scope='host' 必须带 host_id；scope='global' 必须不带（XOR）
    CONSTRAINT chk_bypass_rule_scope CHECK (
        (scope = 'global' AND host_id IS NULL) OR
        (scope = 'host'   AND host_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_bypass_rules_host ON host_bypass_rules(host_id) WHERE host_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS host_bypass_bindings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id     UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    preset_id   UUID REFERENCES host_bypass_presets(id) ON DELETE RESTRICT,
    rule_id     UUID REFERENCES host_bypass_rules(id)   ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    source      TEXT NOT NULL DEFAULT 'admin' CHECK (source IN ('admin','system')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- preset_id / rule_id 必须 XOR（恰好一个非空）
    CONSTRAINT chk_bypass_binding_xor CHECK (
        (preset_id IS NOT NULL AND rule_id IS NULL) OR
        (preset_id IS NULL     AND rule_id IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_bypass_bindings_host ON host_bypass_bindings(host_id);

CREATE TABLE IF NOT EXISTS host_bypass_snapshots (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id                 UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    version                 BIGINT NOT NULL,
    config_hash             TEXT NOT NULL,   -- SHA-256 hex (64 chars)
    whitelist_cidrs_json    JSONB NOT NULL DEFAULT '{"version":3,"rules":[]}'::jsonb,
    whitelist_domains_json  JSONB NOT NULL DEFAULT '{"version":3,"rules":[]}'::jsonb,
    applied_status          TEXT NOT NULL DEFAULT 'pending'
                            CHECK (applied_status IN ('pending','applied','failed','rolled_back')),
    created_by              UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (host_id, config_hash)
);
CREATE INDEX IF NOT EXISTS idx_bypass_snapshots_host_version ON host_bypass_snapshots(host_id, version DESC);

CREATE TABLE IF NOT EXISTS host_bypass_audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_ip     TEXT,
    action       TEXT NOT NULL,
    target_kind  TEXT NOT NULL,
    target_id    UUID,
    before       JSONB,
    after        JSONB,
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_target ON host_bypass_audit_log(target_kind, target_id);
CREATE INDEX IF NOT EXISTS idx_bypass_audit_created ON host_bypass_audit_log(created_at DESC);

-- 系统预设 seed（is_system=true 不可删，由 Phase 46 应用层强制）
INSERT INTO host_bypass_presets (slug, name, description, is_system, is_force_on, is_active, rules)
VALUES
  ('loopback', '本机回环', '127.0.0.0/8 与 169.254.0.0/16（链路本地），强制开启不可关闭。',
   TRUE, TRUE, TRUE,
   '[{"rule_type":"cidr","value":"127.0.0.0/8"},{"rule_type":"cidr","value":"169.254.0.0/16"}]'::jsonb),
  ('lan',      '局域网',   'RFC1918（10/8、172.16/12、192.168/16）+ CGNAT 100.64/10 + ULA fc00::/7。',
   TRUE, FALSE, TRUE,
   '[{"rule_type":"cidr","value":"10.0.0.0/8"},{"rule_type":"cidr","value":"172.16.0.0/12"},{"rule_type":"cidr","value":"192.168.0.0/16"},{"rule_type":"cidr","value":"100.64.0.0/10"},{"rule_type":"cidr","value":"fc00::/7"}]'::jsonb)
ON CONFLICT (slug) DO NOTHING;

COMMIT;
```

`[VERIFIED: 仓库 grep]` `gen_random_uuid()` 与 `pgcrypto` 在 0001_initial.sql 已启用；`hosts` 与 `users` 表存在；REFERENCES ... ON DELETE CASCADE 风格与 0001/0007 一致。

### Example 3: Repository CRUD 方法骨架（沿用 queries.go 风格）

```go
// internal/store/repository/queries_bypass.go（新文件）
package repository

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/jackc/pgx/v5"
)

// 把 SQL 提升为包级 const，供 queries_bypass_test.go 做文本断言（沿用 queries.go 模式）
const listBypassPresetsSQL = `
    SELECT id::text, slug, name, COALESCE(description, ''),
           is_system, is_force_on, is_active, rules, created_at, updated_at
    FROM host_bypass_presets
    ORDER BY is_system DESC, slug ASC
`

const getBypassPresetBySlugSQL = `
    SELECT id::text, slug, name, COALESCE(description, ''),
           is_system, is_force_on, is_active, rules, created_at, updated_at
    FROM host_bypass_presets WHERE slug = $1
`

func (r *Repository) ListBypassPresets(ctx context.Context) ([]BypassPreset, error) {
    rows, err := r.db.Query(ctx, listBypassPresetsSQL)
    if err != nil {
        return nil, fmt.Errorf("query bypass presets: %w", err)
    }
    defer rows.Close()

    out := make([]BypassPreset, 0)
    for rows.Next() {
        var it BypassPreset
        var rawRules json.RawMessage
        if err := rows.Scan(&it.ID, &it.Slug, &it.Name, &it.Description,
            &it.IsSystem, &it.IsForceOn, &it.IsActive, &rawRules,
            &it.CreatedAt, &it.UpdatedAt); err != nil {
            return nil, fmt.Errorf("scan bypass preset: %w", err)
        }
        if len(rawRules) > 0 {
            _ = json.Unmarshal(rawRules, &it.Rules)
        }
        out = append(out, it)
    }
    return out, rows.Err()
}

// ... 同款风格：Get / Create / Update / Delete（is_system 的删除拦截 by Phase 46 handler）
// ... BypassRule / BypassBinding / BypassSnapshot 各自 4 个方法
// ... BypassAuditLog 仅 InsertBypassAuditLog + ListBypassAuditLogByTarget

// audit_log 错误用 fmt.Errorf 包装；上层 handler（Phase 46）转 NetworkError。
// 本层与 queries.go 其他方法一致，不自己造 NetworkError —— NetworkError 是
// internal/network 包的 type，repository 包不引入跨包依赖。
```

`[VERIFIED: 仓库阅读]` 现有 `queries.go` 的所有方法都用 `fmt.Errorf` 包装错误，**没有任何一处直接构造 `NetworkError`**；CONTEXT 中"错误处理沿用 NetworkError 风格"应理解为「上层（network 包或 handler）转 NetworkError」，不是数据层。Plan 阶段需澄清这一点。

## Validation Architecture

> 项目 `.planning/config.json` 未显式禁用 nyquist_validation，按默认启用处理。

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test`（标准库 testing）+ table-driven |
| Config file | 无独立 config（go test 默认） |
| Quick run command | `go test ./internal/network/... ./internal/store/repository/... -run TestBypass -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| BYPASS-NET-01 | 配置 JSON 含 `route.rule_set` 数组（2 个 local 引用） | unit | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig_RuleSetReferences -v` | ❌ Wave 0 |
| BYPASS-NET-02 | route.rules 含 `ip_is_private:true`、两个 rule_set 引用、`final:"proxy-out"` | unit | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig_RouteRulesOrder -v` | ❌ Wave 0 |
| BYPASS-NET-03 | tun inbound 含 `strict_route:true`、`endpoint_independent_nat:true`；route `default_interface:"eth0"` | unit | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig_TunHardening -v` | ❌ Wave 0 |
| BYPASS-NET-04 | 第一条 rule 是 `action:"sniff"` + 四协议；第二条是 `protocol:"dns"`+`action:"hijack-dns"`（不是旧 `outbound:"dns"`） | unit | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig_SniffAndHijack -v` | ❌ Wave 0 |
| BYPASS-DNS-01 | `dns.servers` 含 `dns-local` 与 `dns-proxy`，`final="dns-proxy"`，`strategy="ipv4_only"` | unit | `go test ./internal/network/ -run TestBuildGatewaySingBoxConfig_SplitDNS -v` | ❌ Wave 0 |
| BYPASS-DNS-02 | `dns.rules` 含 `.lan/.local/.internal` → dns-local；`whitelist-domains` → dns-proxy | unit | 同上 | ❌ Wave 0 |
| BYPASS-DNS-03 | `ContainerProxyProvider.PrepareHost` 生成 resolv.conf 源文件且 docker run 带 `:ro` mount 参数 | unit (脚本/参数断言) | `go test ./internal/network/ -run TestContainerProxy_ResolvConfBindMount -v` | ❌ Wave 0 |
| BYPASS-DNS-04 | 同上 nsswitch.conf | unit | `go test ./internal/network/ -run TestContainerProxy_NsswitchBindMount -v` | ❌ Wave 0 |
| BYPASS-DATA-01 | migration 0019 文件含五张表与正确列 | unit (文件文本断言) | `go test ./internal/store/repository/ -run TestMigration0019_FileContent -v` | ❌ Wave 0 |
| BYPASS-DATA-02 | Repository 暴露 `ListBypassPresets` / `GetBypassPreset` / 各 CRUD 签名 | unit (反射) | `go test ./internal/store/repository/ -run TestBypassRepository_Signatures -v` | ❌ Wave 0 |
| BYPASS-DATA-03 | migration 文件含 `loopback` 与 `lan` 两条 seed INSERT；`is_system=true`；`ON CONFLICT (slug) DO NOTHING` | unit | `go test ./internal/store/repository/ -run TestMigration0019_SystemPresetsSeed -v` | ❌ Wave 0 |
| BYPASS-DATA-04 | snapshot 表 schema 含 `version`/`config_hash`/`applied_status` CHECK | unit (文件文本断言) | `go test ./internal/store/repository/ -run TestMigration0019_SnapshotShape -v` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/network/ ./internal/store/repository/ -count=1`
- **Per wave merge:** `go test ./... -count=1`（含 `internal/controlplane`、`internal/runtime` 等下游）
- **Phase gate:** Full suite green + 手工触发一次 docker compose up 的 smoke（新 host 创建无 error 事件）

### Wave 0 Gaps

- [ ] `internal/network/gateway_singbox_config_test.go` 新增至少 5 个测试函数（rule_set / strict_route / sniff+hijack / split-dns / route-rules-order），现有用例保持兼容
- [ ] `internal/network/container_proxy_provider_test.go` 同步更新硬编码 `'nameserver 8.8.8.8'` 断言；新增 bind mount 参数断言
- [ ] `internal/store/repository/migration_0019_test.go` —— 文件不存在，参考 `migration_0014_test.go` 创建
- [ ] `internal/store/repository/queries_bypass_test.go` —— 文件不存在，覆盖签名 + SQL 常量文本断言
- [ ] `internal/store/repository/models.go` 新增 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` / `BypassAuditLog` 与对应 `*Params` 类型
- [ ] 框架安装：无需，go test 已就绪

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | 渲染层、Repository、测试 | ✓ | 1.25.7（go.mod） | — |
| pgx v5 | Repository | ✓ | go.mod 已声明 | — |
| PostgreSQL（运行时） | migration 0019 跑得起来 | 视部署环境 | 18.x | docker-compose dev 模式自带 |
| Docker Engine | gateway/worker 容器、bind mount | 视部署环境 | 28.x | dev 用 macOS Docker Desktop |
| sing-box（镜像内） | rule-set source format v3 | **未验证** | 需 ≥ 1.11.0 | 见下方"Missing"段 |
| pgcrypto extension | `gen_random_uuid()` | ✓ | 已在 0001 启用 | — |
| `github.com/google/nftables` | 不直接影响 Phase 45（worker_firewall_linux.go 已用） | ✓ | go.mod 已声明 | — |

**Missing dependencies with no fallback:**
- 无（所有强依赖都已就绪）

**Missing dependencies with fallback (需 plan 阶段验证):**
- gateway 镜像内 sing-box 版本 —— Plan 阶段第一个任务 `docker inspect cloud-cli-proxy-sing-gateway:local` + 容器内 `sing-box version`；若 < 1.11 需扩大 Phase 45 范围把镜像构建 Dockerfile 升 sing-box

## Security Domain

> security_enforcement 默认启用；Phase 45 是基础设施层但涉及关键安全不变量。

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Phase 46 才接 admin API；本阶段无认证路径 |
| V3 Session Management | no | 同上 |
| V4 Access Control | yes（schema 级） | `is_system` 列 + CHECK 约束 + scope/host_id XOR；执行归 Phase 46 |
| V5 Input Validation | yes | migration 用 `CHECK` 约束限定 enum 字符串；rule_type / scope / applied_status 都有 CHECK |
| V6 Cryptography | yes | `config_hash` 用 SHA-256 hex；不存任何明文密钥；UUID 用 pgcrypto |
| V8 Data Protection at Rest | yes | audit_log before/after 用 JSONB 不存原始凭据；保留 90 天（Phase 47 实现策略） |
| V14 Configuration | yes | bind mount 改 `/etc/resolv.conf` 是关键加固；config.json 渲染输出固定 strict_route 与 final |

### Known Threat Patterns for {Go + Postgres + sing-box}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection | Tampering | pgx 参数化（沿用，所有 SQL 用 `$1` 占位） |
| DNS 泄漏（容器绕过 sing-box） | Information Disclosure | bind mount `/etc/resolv.conf` ro + nsswitch.conf 限定 + sing-box hijack-dns（本阶段实现前两条） |
| 路由回环导致全网泄漏 | Tampering | `route.final="proxy-out"` + `default_interface="eth0"` + `direct-out.bind_interface="eth0"` |
| seed 数据被运营误删 | Tampering | Phase 46 应用层拦截；Phase 45 schema 只标记 `is_system=true` |
| audit log 注入 | Tampering | actor_ip 用 TEXT，before/after 用 JSONB（自动反序列化校验） |
| snapshot apply 非幂等 | DoS（重复 apply） | `UNIQUE (host_id, config_hash)` |
| 攻击者修改 rule-set 文件让流量直连 | Tampering | bind mount `:ro` 让 worker 容器内无写权限；rule-set 文件落在控制面 host_state_dir 不暴露给用户容器 |

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| sing-box `outbound:"dns"` | `action:"hijack-dns"` | sing-box 1.10 | Phase 45 BYPASS-NET-04 必须用新写法（来源：sing-box 官方文档） |
| sing-box `geoip:private` | `ip_is_private:true` | sing-box 1.8 | Phase 45 BYPASS-NET-02 用内置（无需引入 geoip db） |
| sing-box rule-set 远程 HTTP 拉取（仅） | `type:"local" + format:"source"` 文件 watch | sing-box 1.10 | Phase 45 用 local；远程拉取归 v3.5 P1 |
| Rule-set source format v1/v2 | v3 | sing-box 1.11 | placeholder `{"version":3,"rules":[]}` 需要镜像 ≥ 1.11 |

**Deprecated/outdated:**
- 容器内 `nameserver 8.8.8.8` 占位：Phase 45 移除（DNS 泄漏面）。
- `auto_detect_interface:true`：当前 `gateway_singbox_config.go:59` 在用，Phase 45 改为显式 `default_interface:"eth0"`。

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | gateway 镜像 `cloud-cli-proxy-sing-gateway:local` 内 sing-box 版本 ≥ 1.11.0 | §Standard Stack / §Environment Availability | rule-set version 3 schema 不被识别 → sing-box 启动失败 → gateway 不健康 → 所有新 host 创建失败。Plan 阶段必须先验证；若 < 1.11 Phase 45 范围扩大 |
| A2 | sing-box 加载 `{"version":3,"rules":[]}` 空 rule-set 文件不会启动失败 | §Code Example 1 placeholder | 若 sing-box 拒绝空 rules，Phase 45 需要在 placeholder 中加 1 条永不命中的 stub（如 `domain:"never.invalid"`） |
| A3 | `EgressConfig.Proxy.DNSServer` 字段可改成 `172.19.0.1` 而不破坏其他调用者 | §Common Pitfalls Pitfall 3 | 如果有其他模块依赖 DNSServer 原值，需要把 verify expected 改成常量而不是改 EgressConfig；Plan 阶段需 grep `Proxy.DNSServer` 全部引用 |
| A4 | worker 容器创建路径接受新 bind mount 参数（在 `internal/runtime/tasks/worker.go` 或类似位置） | §Architecture Patterns Pattern 2 | bind mount 实际需要在 docker create worker 容器的代码里加 `-v` 参数；该位置不在 `container_proxy_provider.go` 而在 worker.go。Plan 阶段需精确定位 worker create 代码 |
| A5 | gateway 镜像启动顺序保证 sing-box tun0 在 worker 容器 connect 之前 ready | §Common Pitfalls Pitfall 2 | 当前 `waitGatewayHealthy` 只校验容器 Running + logs 无 FATAL；如果 sing-box 监听 tun0 早于 healthy 信号则没问题，反之 worker DNS 全断 |
| A6 | `host_bypass_audit_log` 错误用 `fmt.Errorf` 包装而不是 `NetworkError`（数据层一致风格） | §Code Example 3 | CONTEXT 的"沿用 NetworkError 风格"实际指上层 handler；数据层自己造 NetworkError 会引入 internal/network 反向依赖 |

## Open Questions

1. **gateway 镜像内 sing-box 实际版本？**
   - 我们已知：镜像名 `cloud-cli-proxy-sing-gateway:local`，env 可覆盖
   - 不清楚：实际版本号、Dockerfile 在仓库何处
   - 建议：Plan 阶段第一个任务 `docker inspect` + `docker run --rm <image> sing-box version`；如果是镜像构建脚本在 `deploy/` 或 `Dockerfile.gateway`，把版本作为构建参数固化

2. **worker 容器创建路径在哪？bind mount 加哪里？**
   - 我们已知：`container_proxy_provider.go` 改的是 gateway 启动 + worker 路由配置，但 worker 容器本身的 `docker create/run` 在别处
   - 不清楚：worker create 在 `internal/runtime/tasks/worker.go` 还是其他位置
   - 建议：Plan 阶段 `grep -rn "docker.*run.*workerName\|--name.*workerContainerName" internal/`

3. **`is_system=true` 行不可删的强度？**
   - CONTEXT 明确：Phase 45 仅准备字段，Phase 46 应用层拦截
   - 仍可争议：Phase 45 是否同步加 DB trigger 兜底？
   - 建议：暂不加，保持本阶段最小变更面；Phase 46 同时加 handler 拦截 + 加 trigger，两层防御

4. **空 rule-set 文件是否需要包含 stub rule 兜底？**
   - 官方文档未明示 `rules: []` 运行时行为
   - 建议：Plan 阶段 dry-run sing-box 单独跑一次确认；若失败用 `[{"domain":"never.invalid.cloud-cli-proxy"}]` 兜底

5. **`EgressConfig` 的 DNSServer 字段定义在哪？谁还在用？**
   - 我们已知：`verify.go::verifyDNS` 用它做比对
   - 不清楚：是否有 admin API 让管理员配置每个 egress 的 DNS server（如果有，改成固定 `172.19.0.1` 会引入向后不兼容）
   - 建议：Plan 阶段 grep `Proxy.DNSServer` 全部引用，决定改字段默认值 vs 改 verify 比对源

## Project Constraints (from CLAUDE.md)

| 约束 | 来源 | Phase 45 影响 |
|------|------|--------------|
| 沟通语言：默认中文 | CLAUDE.md §沟通 | 所有 plan/RESEARCH/讨论/PR 描述用中文 |
| 文档：项目规划用中文 | CLAUDE.md §文档 | RESEARCH.md / PLAN.md 中文撰写；标识符保持英文 |
| 禁止写入绝对路径 | CLAUDE.md §隐私与安全 | RESEARCH.md / migration / Go 代码不得出现 `/Users/...` `/home/...` `C:\Users\...`，一律相对路径或环境变量 `DATA_DIR` |
| 禁止写入真实凭据 | CLAUDE.md §隐私与安全 | seed 数据不含任何凭据；audit_log 示例用占位符 |
| GSD 工作流：通过 GSD 命令修改文件 | CLAUDE.md §GSD 工作流 | Phase 45 通过 `/gsd-plan-phase` → `/gsd-execute-phase` 落地 |
| 单宿主机 v1，不上多节点 | CLAUDE.md §约束 | Repository 不预留 tenant_id；表设计单租户 |
| 全流量必须走指定出口 IP | CLAUDE.md §核心价值 | Phase 45 的白名单是受控例外；`final:"proxy-out"` 是默认路径 |
| 每容器至少绑定一个出口 IP | CLAUDE.md §约束 | 不冲突；bypass 是 egress 之外的旁路 |
| Go 1.26.1（CLAUDE.md 推荐） vs 1.25.7（go.mod 实际） | CLAUDE.md §技术栈 / go.mod:3 | 差距不影响 Phase 45；不要在本阶段升 Go |
| PostgreSQL 18.3 | CLAUDE.md §技术栈 | 与 0019 schema 兼容；gen_random_uuid / JSONB / CHECK 都是 PG 18 支持 |

## Sources

### Primary (HIGH confidence)
- sing-box 官方文档 `route/rule_action`：https://sing-box.sagernet.org/configuration/route/rule_action/ — 确认 `hijack-dns` 是最终 action、`route` 是默认 action、`outbound` 在 `route` 时必填
- sing-box 官方文档 `rule-set/source-format`：https://sing-box.sagernet.org/configuration/rule-set/source-format/ — 确认 version 3 = sing-box 1.11.0 引入的 schema；最小合法格式 `{"version":3,"rules":[...]}`
- 仓库代码：
  - `internal/network/gateway_singbox_config.go:1-90`（当前渲染入口与现状）
  - `internal/network/gateway_singbox_config_test.go:1-300`（现有 table-driven 模式）
  - `internal/network/container_proxy_provider.go:1-333`（PrepareHost 主流程与 resolv.conf 写盘点）
  - `internal/network/verify.go:60-118`（DNS / 泄漏校验逻辑）
  - `internal/network/errors.go:1-44`（NetworkError 类型定义）
  - `internal/store/migrations/0001_initial.sql`（pgcrypto / gen_random_uuid 基线）
  - `internal/store/migrations/0014_claude_account_persistent_volume.sql`（带注释的 ALTER 模板）
  - `internal/store/migrations/0018_user_centric_credentials.sql`（含 BEGIN/COMMIT、UPDATE、UNIQUE INDEX 的复合 migration 模板）
  - `internal/store/migrator/migrator.go:14-70`（glob+sort+schema_migrations 表的执行逻辑，确认 0019 序号合法）
  - `internal/store/repository/queries.go:14-100`（Repository 单结构体聚合模式）
  - `internal/store/repository/migration_0014_test.go:1-198`（文件文本断言测试模式）
  - `internal/store/repository/queries_contract_test.go:1-100`（SQL 常量与列断言模式）
  - `go.mod:3`（Go 1.25.7）

### Secondary (MEDIUM confidence)
- 前置研究 `.planning/research/SUMMARY.md` §1（sing-box 选型）/ §2（代码现状）/ §3（DNS 泄漏）/ §4（数据模型）——已被 4 个并行 agent 交叉验证

### Tertiary (LOW confidence)
- 无（本研究未引入未验证的第三方来源）

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — Go / pgx / sing-box 选型已锁定 milestone-level，pgcrypto 验证过
- Architecture: HIGH — 两段式 sing-box + ro bind mount + pgx Repository 全部有现成代码模式可对照
- Pitfalls: HIGH — Pitfall 3（verify DNS expected 同步改）来自实际代码勘察，不是猜测
- Code Examples: HIGH（Example 1/2/3 都基于现有渲染/migration/queries 代码改写）
- Open Questions: MEDIUM — 5 个 open Q 都是 plan 阶段可在 30 分钟内回答的具体勘察任务

**Research date:** 2026-05-12
**Valid until:** 2026-06-11（30 天；sing-box 与 Postgres 都属稳定生态，但 sing-box rule-set v4 / Postgres 19 若意外发布需重新评估）

## RESEARCH COMPLETE

**Phase:** 45 - 网络配置基础与数据模型
**Confidence:** HIGH

### Key Findings

- sing-box 现有渲染入口 `buildGatewaySingBoxConfig` 全部用 `map[string]any` 拼装，扩展 5 处字段（route.rule_set / route.rules 七条 / dns 拆分 / tun 三参加固 / default_interface）属增量改造，无未知风险
- 当前 `container_proxy_provider.go:323` 用 docker exec 写 `nameserver 8.8.8.8`，且 `verify.go:79 verifyDNS` 拿 `EgressConfig.Proxy.DNSServer` 比对 —— 改 bind mount 必须**同时**改三处（写盘逻辑 / DNSServer 期望值 / 测试硬编码字符串），否则 verify 必失败（Pitfall 3）
- migration 0019 是合规下一编号（缺号 0010/0011/0016 由 `filepath.Glob+sort.Strings` 排序处理）；CHECK 约束 + JSONB 内嵌 rules + `ON CONFLICT (slug) DO NOTHING` seed 全部有现成范式
- Repository 测试模式 = SQL 文本断言 + 反射签名断言，**没有 testcontainer**（CONTEXT 中此句需修正）；新增 `queries_bypass_test.go` 与 `migration_0019_test.go` 沿用 `migration_0014_test.go` 模板
- gateway 镜像内 sing-box 版本未在仓库勘察出来，必须 ≥ 1.11（rule-set source format v3）—— 这是 Phase 45 范围的第一风险点，Plan 阶段第一任务确认

### File Created
`.planning/phases/45-net-foundation/45-RESEARCH.md`

### Confidence Assessment
| Area | Level | Reason |
|------|-------|--------|
| Standard Stack | HIGH | 仓库实际版本已 grep 验证 |
| Architecture | HIGH | 全部依赖现有代码模式 |
| Pitfalls | HIGH | Pitfall 3 是真实代码勘察发现，非猜测 |
| Schema | HIGH | sing-box 官方文档 + 0001/0014/0018 migration 模板已验证 |
| Test 模式 | HIGH | 已直读 `migration_0014_test.go` / `queries_contract_test.go` 确认 |

### Open Questions

5 个 plan 阶段可在 30 分钟内回答的勘察任务（见 §Open Questions）：
1. 镜像内 sing-box 版本（决定 placeholder schema 与镜像升级范围）
2. worker 容器 docker create 在哪个文件（决定 bind mount 注入点）
3. `is_system` 删除拦截是否同时加 DB trigger（兼容性 vs 加固）
4. 空 rule-set 文件是否需要 stub rule 兜底（实测 1 次即可定）
5. `EgressConfig.Proxy.DNSServer` 全引用清单（决定改字段 vs 改 verify 比对）

### Ready for Planning
Research 完成。Planner 可基于本文档把 Phase 45 切成 3 个 plan：
- Plan 01：sing-box 配置渲染层扩展（BYPASS-NET-01..04 / BYPASS-DNS-01..02）
- Plan 02：容器 bind mount 改造（BYPASS-DNS-03..04，含 verify expected 同步与测试硬编码同步）
- Plan 03：migration 0019 + Repository CRUD + seed（BYPASS-DATA-01..04）

三 plan 间唯一耦合：`<DATA_DIR>/gateway/<host_id>/` 这一 host_state 子目录作为 rule-set placeholder / resolv.conf / nsswitch.conf 的共同源根；Plan 01 与 Plan 02 都需要写该目录，建议共享 helper。
