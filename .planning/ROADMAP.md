# Roadmap: Cloud CLI Proxy

## Milestones

- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- 🗃️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (archived, capabilities merged into v3.4)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- ✅ **v3.0 远端开发体验升级** — Phases 29-35 (shipped 2026-04-23) — [Archive](milestones/v3.0-ROADMAP.md)
- ✅ **v3.1 映射语义补齐与懒加载** — Phases 36-37 (shipped 2026-04-24) — [Archive](milestones/v3.1-ROADMAP.md)
- ✅ **v3.4 多形态容器接入** — Phases 38-44 (shipped 2026-05-08) — [Archive](milestones/v3.4-ROADMAP.md)
- 🚧 **v3.5 网络白名单与 DNS 拆分解析** — Phases 45-47 (planning, started 2026-05-12)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-6) — SHIPPED 2026-03-28</summary>

- [x] Phase 1: 基础控制面与主机代理 (3/3 plans) — completed 2026-03-26
- [x] Phase 2: 隧道出网强制层 (3/3 plans) — completed 2026-03-27
- [x] Phase 3: 启动入口与 SSH 接入 (3/3 plans) — completed 2026-03-27
- [x] Phase 4: 后台管理界面 (3/3 plans) — completed 2026-03-27
- [x] Phase 5: 到期、审计与清理 (3/3 plans) — completed 2026-03-27
- [x] Phase 6: 加固与 MVP 就绪 (4/4 plans) — completed 2026-03-28

</details>

<details>
<summary>✅ v1.1 支持代理协议出网 (Phases 7-10) — SHIPPED 2026-03-28</summary>

- [x] Phase 7: 数据层与类型化 (3/3 plans) — completed 2026-03-28
- [x] Phase 8: SingBoxProvider 与受管镜像 (3/3 plans) — completed 2026-03-28
- [x] Phase 9: 前端适配与代理测试 (3/3 plans) — completed 2026-03-28
- [x] Phase 10: 技术债务清理 (2/2 plans) — completed 2026-03-28

</details>

<details>
<summary>⏸️ v1.2 用户自助面板与 Bootstrap 重设计 (Phases 11-16) — PARTIALLY SHIPPED, remaining deferred</summary>

- [x] Phase 11: 认证基础设施与数据迁移 (3/3 plans) — completed 2026-03-29
- [x] Phase 12: 用户自助 API 与前端路由 (2/2 plans) — completed 2026-03-29
- [ ] Phase 13: 账号管理与用户资源视图 (deferred)
- [ ] Phase 14: KasmVNC 用户面 (deferred)
- [ ] Phase 15: Bootstrap 重设计 (deferred)
- [ ] Phase 16: 级联禁用与到期治理 (deferred)

</details>

<details>
<summary>🗃️ v1.3 claude-shell 本地透明代理 (Phases 17-23) — ARCHIVED, capabilities merged into v3.4</summary>

**Archive reason:** v1.3 的"本地容器"和"指纹伪装"能力与 v3.4 的 `cloud-claude local` 目标重叠。为避免维护两套本地 Docker 编排逻辑，v1.3 能力拆分并入 v3.4：
- Phase 17 镜像基线 → 已并入 managed-user 镜像（v3.0+）
- Phase 18 网络隔离 → 并入 v3.4 Phase 39（本地版 sing-box sidecar + veth 注入）
- Phase 19 CLI 骨架 → 并入 v3.4 Phase 39（`cloud-claude local` 子命令）
- Phase 20 TTY 透传 → 不再需要（本地 publish 端口，SSH 用系统 OpenSSH）
- Phase 21 指纹伪造 → entrypoint `MODE=local` 深度伪装分支（快速任务级别）
- Phase 22 验证自检 → `cloud-claude local verify` / `doctor --local`
- Phase 23 混淆构建 → Makefile 构建脚本，不单独成里程碑

- [x] Phase 17: 镜像与 Entrypoint 基线 (17-01 + 17-02 gap closure 完成，已并入 v3.0+ 镜像)
- [ ] Phase 18: 网络隔离与分流 → **merged into v3.4 Phase 39**
- [ ] Phase 19: CLI 骨架与 Docker 编排 → **merged into v3.4 Phase 39**
- [ ] Phase 20: TTY 透传与交互体验 → **obsolete**（本地直连端口，无需 TTY 透传）
- [ ] Phase 21: 指纹伪造与反检测 → **merged into v3.4 entrypoint `MODE=local` 分支**
- [ ] Phase 22: 验证与自检 → **merged into v3.4 doctor 扩展**
- [ ] Phase 23: 混淆构建与交付 → **deferred to build script**

</details>

<details>
<summary>✅ v2.0 cloud-claude 透明远程 CLI (Phases 24-28) — SHIPPED 2026-04-15</summary>

- [x] Phase 24: 受管镜像 FUSE 硬化与容器参数 (1/1 plans) — completed 2026-04-14
- [x] Phase 25: cloud-claude CLI 骨架与连接 (1/1 plans) — completed 2026-04-15
- [x] Phase 26: 参数透传与终端体验 (1/1 plans) — completed 2026-04-15
- [x] Phase 27: 双 session 目录映射 (2/2 plans) — completed 2026-04-15
- [x] Phase 28: 生产环境 FUSE 兼容性验证 (2/2 plans) — completed 2026-04-15

</details>

<details>
<summary>✅ v3.0 远端开发体验升级 (Phases 29-35) — SHIPPED 2026-04-23</summary>

- [x] Phase 29: 受管镜像 v3 + Worker 容器参数扩展 (6/6 plans) — completed 2026-04-18
- [x] Phase 29.1: GetHost entry_password 修复 (P0 HOTFIX, INSERTED, 4/4 plans) — completed 2026-04-20
- [x] Phase 30: 控制面数据模型 + Entry API 扩展 (2/2 plans) — completed 2026-04-18
- [x] Phase 31: CLI 三层文件映射重构 (3/3 plans) — completed 2026-04-19
- [x] Phase 32: SSH 会话可靠性 + tmux + 多端 (5/5 plans，含 2 gap-closure) — completed 2026-04-20
- [x] Phase 33: Claude Code 状态持久化（CLI + 镜像 + admin GC）(2/2 plans + 3 post-fix) — completed 2026-04-21
- [x] Phase 34: cloud-claude doctor v3 + 错误码统一 (3/3 plans) — completed 2026-04-21
- [x] Phase 35: E2E 稳定化 + 性能验收 (5/5 plans) — completed 2026-04-23

3 项真机签字 deferred-to-ship（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04），跟踪在 `milestones/v3.0-phases/35-e2e/35-HUMAN-UAT.md`。

</details>

<details>
<summary>✅ v3.1 映射语义补齐与懒加载 (Phases 36-37) — SHIPPED 2026-04-24</summary>

- [x] Phase 36: 映射前置约束 + sshfs 内核缓存 (6/6 plans) — completed 2026-04-23
- [x] Phase 37: 冷文件读触发晋升 + e2e UAT (5/5 plans) — completed 2026-04-24

5 项人工验证 deferred-to-ship（Linux 真机 UAT / pgrep 存活 / 端到端晋升 / 手册可读性 / 双平台签字），跟踪在 `milestones/v3.1-MILESTONE-AUDIT.md`。

</details>

<details>
<summary>✅ v3.4 多形态容器接入 (Phases 38-44) — SHIPPED 2026-05-08</summary>

- [x] Phase 38: SSH Proxy 端口转发支持 (3/3 plans) — completed 2026-05-07
- [x] Phase 39: 本地 Dev Containers 支持 (3/3 plans) — completed 2026-05-07
- [x] Phase 40: VS Code Remote-SSH E2E 验证 (2/2 plans) — completed 2026-05-08
- [x] Phase 41: Doctor 扩展与收尾 (2/2 plans) — completed 2026-05-08
- [x] Phase 42: Phase 39 验证补齐 (1/1 plan, gap closure) — completed 2026-05-08
- [x] Phase 43: VS Code 端口转发 E2E 补齐 (2/2 plans, gap closure) — completed 2026-05-08
- [x] Phase 44: doctor sshd_config 验证 (1/1 plan, gap closure) — completed 2026-05-08

11 项人工验证 deferred-to-ship（Phase 38 x3 / Phase 39 x5 / Phase 43 x3），跟踪在 `milestones/v3.4-MILESTONE-AUDIT.md`。

</details>

<details open>
<summary>🚧 v3.5 网络白名单与 DNS 拆分解析 (Phases 45-47) — PLANNING, started 2026-05-12</summary>

- [ ] **Phase 45: 网络配置基础与数据模型** — sing-box 两段式配置 + 拆分 DNS + resolv.conf 接管 + 五张白名单表 + loopback/lan seed
- [ ] **Phase 46: 控制面 API 与后台 UI** — admin Bypass CRUD/preview/apply/rollback + 护栏校验 + React Bypass Tab + diff 预览
- [ ] **Phase 47: 热更新链路与流量验证** — ActionReloadHostBypass + 原子 rule-set / nft set 更新 + fail-closed 加固 + 10 条安全不变量 CI

</details>

## Phase Details

### Phase 45: 网络配置基础与数据模型

**Goal**: 让 sing-box 静态配置改造为两段式 + 拆分 DNS 模型 + 容器 resolv.conf 接管 + 白名单数据模型与系统预设全部就绪，为后续 API 和热更新打下可渲染、可入库、可挂载的基础。

**Depends on**: Phase 44 (v3.4 多形态容器接入 已交付的 sing-box gateway 基线)

**Requirements**: BYPASS-NET-01, BYPASS-NET-02, BYPASS-NET-03, BYPASS-NET-04, BYPASS-DNS-01, BYPASS-DNS-02, BYPASS-DNS-03, BYPASS-DNS-04, BYPASS-DATA-01, BYPASS-DATA-02, BYPASS-DATA-03, BYPASS-DATA-04

**Success Criteria** (what must be TRUE):

  1. 新建 host 渲染出的 `config.json` 包含 `route.rule_set` 数组（`whitelist-cidrs` / `whitelist-domains`，type=local，format=source），`route.rules` 第一条为 `sniff`（sniffer 至少含 `tls/http/quic/dns`），含 `protocol:"dns"` → `hijack-dns` 与 `ip_is_private:true` → `direct-out` 兜底；`route.final = "proxy-out"`、`route.default_interface = "eth0"`，`tun` inbound 同时启用 `strict_route` / `auto_route` / `endpoint_independent_nat`
  2. sing-box `dns` 段同时存在 `dns-local`（type=local）和 `dns-proxy`（type=https，server=1.1.1.1，detour=proxy-out，domain_resolver=dns-local），`dns.final = "dns-proxy"`、`strategy = "ipv4_only"`；`.lan` / `.local` / `.internal` 后缀走 `dns-local`，命中 `whitelist-domains` rule_set 的域名走 `dns-proxy`
  3. worker 容器启动后，`nsenter -t <PID> -m cat /etc/resolv.conf` 唯一 nameserver 为 `172.19.0.1`（含 `options ndots:0 single-request-reopen`），并且文件以只读 bind mount 挂载（容器内 `mount | grep resolv.conf` 标记 `ro`）；`/etc/nsswitch.conf` 的 `hosts:` 行为 `files dns`，无 mdns/myhostname/wins
  4. 数据库迁移 `0019_host_bypass_rules.sql` 应用后，`\d host_bypass_presets` / `host_bypass_rules` / `host_bypass_bindings` / `host_bypass_snapshots` / `host_bypass_audit_log` 五张表全部存在；`SELECT slug, is_system, is_force_on FROM host_bypass_presets` 至少返回 `loopback`（is_system=true、is_force_on=true）和 `lan`（is_system=true、is_force_on=false）两行
  5. Repository 层 `BypassPreset` / `BypassRule` / `BypassBinding` / `BypassSnapshot` 的 CRUD 单元测试全部通过，且 `host_bypass_snapshots.applied_status` 默认值为 `pending`，可被写入 `applied` / `failed` / `rolled_back` 四态

**Plans**: TBD

### Phase 46: 控制面 API 与后台 UI

**Goal**: 让管理员能在后台对每个 host 完成「勾预设 → 加规则 → 预览 diff → 一键 apply / rollback」的闭环，所有写操作经过护栏校验和审计日志，前端体验清晰且高风险动作有二次确认。

**Depends on**: Phase 45

**Requirements**: BYPASS-API-01, BYPASS-API-02, BYPASS-API-03, BYPASS-API-04, BYPASS-API-05, BYPASS-UI-01, BYPASS-UI-02, BYPASS-UI-03, BYPASS-UI-04, BYPASS-UI-05

**Success Criteria** (what must be TRUE):

  1. 管理员使用 JWT 访问 `GET/POST/PATCH/DELETE /v1/admin/bypass/presets` 和 `/v1/admin/bypass/rules`、`POST /v1/admin/bypass/rules/validate`、`GET/POST /v1/admin/hosts/{hostID}/bypass` 全套 CRUD/绑定接口可用，且 `is_system=true` 的预设 PATCH/DELETE 返回 403
  2. `POST /v1/admin/hosts/{hostID}/bypass/preview` 返回渲染后的 `whitelist-cidrs.json` / `whitelist-domains.json`、nft set diff 和风险报告且不落库；`POST .../apply` 写入 `host_bypass_snapshots`（含 `config_hash` 幂等键）；`POST .../rollback` 能回到上一个 `applied` snapshot；`GET .../effective` 返回当前生效规则全集
  3. 护栏触发样本（`0.0.0.0/0` / v4 < /16 公网段 / `.com` 等顶级 TLD / 覆盖代理服务器 IP / 单 host > 1000 条）全部返回 HTTP 422，错误码分别为 `BYPASS_RULE_TOO_BROAD` / `BYPASS_RULE_CONFLICT_PROXY` / `BYPASS_LIMIT_EXCEEDED`；`domain_keyword` < 4 字符返回 400 并要求 `confirm_risky:true` 才能保存
  4. host 详情页存在「代理白名单」Tab：预设卡片中 `loopback` 强制锁定不可取消，`lan` 可勾选；自定义规则支持 IP/CIDR/域名/域名后缀/端口五种类型 CRUD，高风险规则显示黄色徽章并弹出二次确认；「预览生效配置」面板可切换 sing-box JSON 视图和 nft set diff 视图
  5. 应用按钮按 `生成快照 → 下发到 agent → reload → 健康检查 → 完成` 分阶段反馈进度，成功后 toast 显示「白名单变更不影响现有 TCP 连接，新连接才用新规则」；失败时显示具体错误码并标注自动回滚状态；所有写操作（create/update/delete/bind/unbind/apply/rollback）在 `host_bypass_audit_log` 留下 actor_id / actor_ip / action / before / after 行

**Plans**: TBD

**UI hint**: yes

### Phase 47: 热更新链路与流量验证

**Goal**: 让管理员的 apply 动作通过 host-agent 真正落到 sing-box rule-set 文件和容器 netns nftables set 上，配置变更不重启进程、不断 SSH，且 10 条安全不变量在 CI 中持续可验证。

**Depends on**: Phase 45, Phase 46

**Requirements**: BYPASS-RELOAD-01, BYPASS-RELOAD-02, BYPASS-RELOAD-03, BYPASS-RELOAD-04, BYPASS-NFT-01, BYPASS-NFT-02, BYPASS-NFT-03, BYPASS-NFT-04, BYPASS-VERIFY-01, BYPASS-VERIFY-02, BYPASS-VERIFY-03, BYPASS-VERIFY-04

**Success Criteria** (what must be TRUE):

  1. `internal/agentapi/contracts.go` 暴露 `ActionReloadHostBypass = "reload_host_bypass"`，worker dispatch 命中后 host-agent 完成「`tmpfile + rename` 原子写 rule-set 文件 → `nft -f` 事务更新 `@whitelist_v4` set → 等 1s sing-box 文件 watch reload → 健康检查」全流程；reload 期间长跑 `ssh ... 'while true; do echo .; sleep 1; done'` 不中断（I10）
  2. 健康检查 3 次失败后自动用上一个 `applied` snapshot 重新下发，当前 snapshot 在数据库标记为 `rolled_back` 并产生事件日志告警；`GET /v1/admin/hosts/{hostID}/bypass/consistency` 返回 nft set 与 rule-set 文件 SHA-256 hash 一致性的对账结果（I7）
  3. 容器 netns `output` 链 policy = `drop` 且仅放行 `oifname sb-tun0` + uid=singbox 直连代理 IP:443 + `oifname eth0 ip daddr @whitelist_v4`，默认末尾 `counter log prefix "sbfw-drop " drop`；mDNS(5353) / LLMNR(5355) / NetBIOS(137) UDP 出向被显式 drop 且计数器可读（I2/I3/I9）
  4. 容器启动参数携带 `--sysctl net.ipv6.conf.all.disable_ipv6=1` 和 `default.disable_ipv6=1`，容器内 `ip -6 addr` 仅看到 `::1`，ip6tables 默认 drop（I6）；gateway 容器健康检查不通过时 `provider.PrepareHost` verify 流程不放行 worker 容器开放 SSH 端口（BYPASS-NFT-04 fail-closed）
  5. 扩展后的 `internal/network/verify.go` 增加白名单 IP 走 eth0、非白名单走代理出口、`dig @8.8.8.8` 必超时 3 项检查；`scripts/uat-bypass.sh` 覆盖 6 个场景（仅 loopback / 仅 lan / loopback+lan / 自定义 IP / 自定义域名 / pkill sing-box fail-closed），10 条安全不变量（I1–I10）全部接入 CI；`host_bypass_snapshots.applied_status` 与 `host_bypass_audit_log` 在 e2e 测试中可拉到 `applied` / `rolled_back` 两种状态行

**Plans**: TBD

## Progress

**Current milestone:** v3.5 网络白名单与 DNS 拆分解析（planning，3 phases / 0 plans）

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1-6. v1.0 MVP | v1.0 | 19/19 | Complete | 2026-03-28 |
| 7-10. v1.1 代理协议出网 | v1.1 | 11/11 | Complete | 2026-03-28 |
| 11-12. v1.2 认证与自助面板 | v1.2 | 5/5 | Partial | 2026-03-29 |
| 17-23. claude-shell 本地代理 | v1.3 | — | Archived | — |
| 24-28. v2.0 cloud-claude | v2.0 | 7/7 | Complete | 2026-04-15 |
| 29-35. v3.0 远端开发体验升级 | v3.0 | 30/30 | Complete | 2026-04-23 |
| 36-37. v3.1 映射语义补齐与懒加载 | v3.1 | 11/11 | Complete | 2026-04-24 |
| 38-44. v3.4 多形态容器接入 | v3.4 | 14/14 | Complete | 2026-05-08 |
| 45. v3.5 网络配置基础与数据模型 | v3.5 | 0/0 | Planning | — |
| 46. v3.5 控制面 API 与后台 UI | v3.5 | 0/0 | Planning | — |
| 47. v3.5 热更新链路与流量验证 | v3.5 | 0/0 | Planning | — |

---

*Last updated: 2026-05-12 — v3.5 roadmap drafted (Phases 45-47), 34/34 requirements mapped.*
