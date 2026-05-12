# Phase 47: 热更新链路与流量验证 - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** smart_discuss auto-optimized (infrastructure phase，所有关键决策已在 `.planning/research/SUMMARY.md` 与 Phase 45/46 实现中锁定)

<domain>
## Phase Boundary

让管理员的 apply 动作通过 host-agent 真正落到 sing-box rule-set 文件和容器 netns nftables set 上，配置变更不重启 sing-box / 不断 SSH，且 10 条安全不变量在 CI 中持续可验证。

**本阶段交付：**
- **agent 热更新链路**：worker dispatcher 真实实现 `ActionReloadHostBypass`，覆盖 Phase 46 `slog.Info("Phase 46 placeholder...")` 占位
- **nft 加固**：扩展 `worker_firewall_linux.go` 的 output 链规则（@whitelist_v4 set + uid 锁 + mDNS/LLMNR/NetBIOS drop + IPv6 全禁）
- **失败 fail-closed**：sing-box 启动失败时 gateway unhealthy + worker SSH 不放行（已在 PrepareGateway 健康检查，Phase 47 补 fail-closed e2e 验证）
- **10 条安全不变量 CI**：scripts/uat-bypass.sh + verify.go 扩展 3 项检查（白名单走 eth0 / 非白名单走代理 / dig @8.8.8.8 必超时）

**不在本阶段范围：**
- 远程 rule-set 拉取（v3.5 P1，BYPASS-RULESET-REMOTE）
- 灰度 / 命中统计 / 流量 dashboard（v3.5 P1）
- 用户自助（v3.6+）

</domain>

<decisions>
## Implementation Decisions

### worker dispatch 真实实现（覆盖 Phase 46 placeholder）

- `internal/runtime/tasks/worker.go::handleReloadHostBypass` 改为：
  1. 从 task payload 取 `host_id` + `snapshot_id`
  2. 从 Repository `GetBypassSnapshotByID(snapshot_id)` 读 `whitelist_cidrs_json` / `whitelist_domains_json`
  3. 调 host-agent API（沿用现有 agentapi `ActionXXX` 风格）下发到 host-agent
  4. host-agent 端写 rule-set 文件 `tmpfile + rename` 原子，路径 `<host_state_dir>/whitelist-{cidrs,domains}.json`（与 Phase 45 GatewayConfigDir 一致）
  5. host-agent 端用 `nft -f` 事务批量更新 `@whitelist_v4` set
  6. 等 1s 让 sing-box 文件 watch 自动 reload（sing-box 1.10+ type=local format=source 自动 reload，无需 SIGHUP）
  7. 健康检查（nsenter + curl 验证白名单走 eth0）
  8. 写 snapshot `applied_status='applied'` + audit
- 失败路径：3 次健康检查失败 → 用上一个 `applied` snapshot 重新下发（自动 rollback）→ 当前 snapshot 标 `rolled_back` + 事件日志告警

### nft 加固（扩展 `internal/network/worker_firewall_linux.go`）

- 现有规则集已在 v3.4：`output policy drop` + lo / gateway / 22 / 53。Phase 47 扩展：
  - `oifname "sb-tun0" accept`（白名单流量从 tun 出，统一走 sing-box 路由判断）
  - `meta skuid singbox ip daddr <proxy_ip> tcp dport 443 accept`（uid 锁定，仅 sing-box 进程能连代理服务器）
  - `oifname "eth0" ip daddr @whitelist_v4 accept`（白名单逃逸通道）
  - 默认末尾 `counter log prefix "sbfw-drop " drop`（计数 + 日志）
- mDNS / LLMNR / NetBIOS UDP 出向显式 drop（端口 5353 / 5355 / 137 / 138）
- IPv6：容器启动参数 `--sysctl net.ipv6.conf.all.disable_ipv6=1` + `--sysctl net.ipv6.conf.default.disable_ipv6=1`；ip6tables 默认 drop
- nft set `@whitelist_v4` 通过 `nft -f` 事务批量更新（add / del element），保证原子性

### nft 与 rule-set 文件一致性

- `internal/network/verify.go::VerifyNetworkIntegrity` 新增对账逻辑：
  - 读 `<host_state_dir>/whitelist-cidrs.json` SHA-256
  - 读 nft `@whitelist_v4` set 当前 IP 列表（`nft -j list set inet sbfw whitelist_v4`），按字典序拼接后 SHA-256
  - 两 hash 必须一致
- 控制面新增 `GET /v1/admin/hosts/{hostID}/bypass/consistency` 接口，调用 agent 验证 hash 一致性（**本阶段实现 agent 端校验 + 控制面 endpoint**）

### verify.go 三项新检查（BYPASS-VERIFY-01）

1. 白名单 IP（RFC1918 `192.168.0.1`）`nsenter + curl` 流量从 eth0 出（源 IP = host eth0），非代理出口
2. 非白名单域名（`api.example.com`）流量走代理出口（源 IP = egress IP）
3. `nsenter + dig @8.8.8.8 example.com` 必超时（DNS 不能直连公网 DNS）

### 10 条安全不变量 CI（BYPASS-VERIFY-02）

- 见 `.planning/research/SUMMARY.md` §3.3：I1~I10
- 实现方式：`scripts/uat-bypass.sh` 涵盖 6 个场景（仅 loopback / 仅 lan / loopback+lan / 自定义 IP / 自定义域名 / pkill sing-box fail-closed），每个场景跑完后用 verify.go + 直接 `nft list` / `nsenter ... ip -6 addr` / `tcpdump` 校验 I1~I10 全部满足
- 在 CI 中作为 e2e job 运行（GitHub Actions 等）

### Claude's Discretion

- agent API 字段命名（`reload_host_bypass` 已确认；payload 字段名 / 错误码 由 planner 决定）
- nft set 名称 `@whitelist_v4` 是否需要 `@whitelist_v6`（v3.5 全禁 IPv6，本阶段只做 v4，给 v6 留 placeholder 注释）
- consistency endpoint 是否做 polling 监控（推荐 30s 间隔，但实现细节由 planner 决定）
- uat-bypass.sh 是 bash + curl + jq 还是 Go test —— 建议 bash（与 scripts/uat-network-resilience.sh 一致）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

| 资产 | 路径 | 复用方式 |
|------|------|----------|
| Worker dispatcher | `internal/runtime/tasks/worker.go` (Phase 46 已注册 ActionReloadHostBypass case) | 覆盖 placeholder 实现 |
| Agent API contracts | `internal/agentapi/contracts.go` | 已有 `ActionReloadHostBypass` 常量 |
| Repository snapshot | `internal/store/repository/queries_bypass.go` | `GetBypassSnapshotByID` / `UpdateBypassSnapshotStatus` 已就绪 |
| Container 防火墙骨架 | `internal/network/worker_firewall_linux.go` | 扩展 output 链规则 |
| 验证脚本基线 | `internal/network/verify.go` (Phase 45 已加 containerExpectedDNS 常量化) | 扩展 3 项新检查 |
| host-agent 通信 | （现有 host-agent 实现） | sing-box 1.13.3 已确认（Phase 45 镜像勘察） |
| UAT 模板 | `scripts/uat-network-resilience.sh` | 仿写 scripts/uat-bypass.sh |
| GatewayConfigDir | Phase 45 引入 | rule-set 文件 / DNS 源文件 / nft set 输入文件统一根目录 |

### Established Patterns

- **Atomic file write**: `tmpfile + os.Rename`（已在 `WriteContainerDNSConfig` 用过，Phase 47 复用）
- **nft 事务**: `nft -f -` 读 stdin 批量执行
- **健康检查重试**: 3 次 / 200ms 间隔（已在 `waitGatewayHealthy` 用过）
- **agent API 调用**: 通过 HostAgent client（HTTP over Unix socket / TCP，看现有实现）
- **verify.go nsenter**: `nsenter -t $PID -n curl ...` / `nsenter -t $PID -n dig ...`

### Integration Points

- Phase 46 apply API 调用 `tasks.Dispatch(ctx, ActionReloadHostBypass, payload)` → worker dispatcher（Phase 46 placeholder）→ Phase 47 真实实现
- consistency endpoint 注册：`router.go` `/v1/admin/hosts/{hostID}/bypass/consistency`，handler 调用 agent API
- nft 扩展位置：`worker_firewall_linux.go::buildOutputChain` 或类似函数

</code_context>

<specifics>
## Specific Ideas

- worker dispatcher 健康检查 timeout：每次 5s，最多 3 次（总 15s）
- `nft -f` 命令通过 stdin 注入，避免落临时文件
- `@whitelist_v4` set 类型用 `ipv4_addr` 单 IP + `ipv4_addr_prefix` CIDR 混合（nftables 5+ 支持）
- sing-box rule-set reload watch 间隔：sing-box 默认 5s，不要短于此（避免 watch storm）
- consistency endpoint 内部超时 3s，对外 504 Gateway Timeout
- uat-bypass.sh 用 `set -euo pipefail` + `trap` 清理 + 颜色输出（仿 uat-network-resilience.sh）

</specifics>

<deferred>
## Deferred Ideas

- 远程 rule-set 拉取（MetaCubeX/meta-rules-dat 镜像）→ v3.5 P1（BYPASS-RULESET-REMOTE）
- 灰度按钮「先在测试 host 验证」→ v3.5 P1（BYPASS-CANARY）
- 命中统计 / 流量 dashboard → v3.5 P1（BYPASS-HIT-STATS / BYPASS-DASHBOARD）
- 用户自助配置 → v3.6+（BYPASS-USER-SELF）
- IPv6 双栈 → v3.5 容器内全禁 IPv6，未来 ip6tables 对称就绪再开
- audit_log 90 天保留策略 retention worker → v3.5 P1

</deferred>
