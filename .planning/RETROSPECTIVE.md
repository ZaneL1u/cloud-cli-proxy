# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-03-28
**Phases:** 6 | **Plans:** 19 | **Timeline:** 3 days (2026-03-25 → 2026-03-28)

### What Was Built
- Go 控制面 + Unix socket host-agent + PostgreSQL 持久化的单宿主机 SSH 云主机平台
- WireGuard 命名空间注入 + nftables 默认拒绝 + 三重校验的全隧道出网强制层
- `curl → 认证 → 进度展示 → exec ssh` 一条命令接入流，含 7 个错误码映射
- JWT 认证的 React 19 管理后台，涵盖用户/出口 IP/绑定/主机生命周期/到期/事件全套 CRUD
- 到期自动扫描 + 运行时对账 + 13 种事件类型记录
- 76 个自动化测试 + 完整部署/运维/故障排查文档

### What Worked
- **阶段化切分清晰**：6 个阶段按依赖关系线性推进（基础→网络→接入→管理→运营→加固），没有出现返工
- **接口预留有效**：Phase 1 预留的 `network.Provider` 接口在 Phase 2 顺利替换为 TunnelProvider
- **标准库优先策略**：Go `net/http` + `slog` + `crypto/rand` 等标准库满足了全部需求，没有引入不必要的框架
- **三重校验门禁**：出口 IP + DNS + 泄漏检测的三重校验在 Phase 6 审计中证明零绕过风险
- **Unix socket 特权边界**：控制面与 host-agent 分离设计使安全审查只需关注一个边界

### What Was Inefficient
- **Phase 1 缺少 VERIFICATION.md**：最早的阶段没有运行验证流程，导致审计时需要依赖 SUMMARY 前置数据推断
- **SUMMARY frontmatter 不一致**：部分 SUMMARY 未在 frontmatter 中列出已完成的需求 ID，增加了审计交叉验证的复杂度
- **出口 IP handler 事件遗漏**：在 Phase 5 注入事件记录时遗漏了 egress IP CRUD，直到里程碑审计才发现
- **error_code 粗粒度**：worker 的网络错误统一返回 `host_action_failed`，与 bootstrap 脚本定义的细粒度退出码不一致，在审计时补修

### Patterns Established
- **控制面 ↔ agent 分离**：HTTP API 不直接操作 Docker 或网络，通过 Unix socket 委托给具有 Linux 特权的 agent
- **--network=none + 命名空间注入**：容器完全隔离 Docker 默认网络，手动注入 WireGuard + 管理 veth
- **EventRecorder 接口注入**：所有 admin handler 通过依赖注入接入事件记录，nil 安全（`if h.events != nil`）
- **stub + table-driven + httptest 测试模式**：Admin API 测试统一使用 stub store + 表驱动 + httptest.NewServer
- **检查清单式运维文档**：部署指南、运维手册和故障排查均采用结构化检查清单格式

### Key Lessons
1. **验证流程应从第一个阶段开始**：Phase 1 缺少 VERIFICATION.md 导致审计需要额外推断，后续里程碑应确保每个阶段都运行验证
2. **事件记录注入应在模式建立时立即覆盖所有 handler**：Phase 5 注入事件时遗漏了 egress IP handler，说明新增横切关注点时应立即全量扫描
3. **error_code 应在定义时就建立从错误类型到错误码的完整映射**：bootstrap_errors.go 定义了细粒度码但 worker 未使用，应在定义端和消费端同时建立映射
4. **单宿主机约束极大降低了复杂度**：没有服务发现、负载均衡或跨节点通信的干扰，网络正确性成为唯一聚焦点

### Cost Observations
- Model mix: 以 Sonnet/Opus 为主的多模型组合，用于研究、规划、执行和验证
- Sessions: 约 20+ 次会话完成全部 6 个阶段
- Notable: Phase 4 (admin UI) 是最大的阶段（35 files touched in Plan 01），前后端同时交付提升了集成质量

---

## Milestone: v1.1 — 支持代理协议出网

**Shipped:** 2026-03-28
**Phases:** 4 | **Plans:** 11 | **Timeline:** 3 days (2026-03-25 → 2026-03-28)

### What Was Built
- 出口 IP 数据模型类型化（wireguard/proxy），DB migration + 校验分支 + API 适配
- SingBoxProvider 15 步 PrepareHost 流水线：sing-box tun 模式全流量代理 + RoutingProvider 工厂按 tunnel_type 自动路由
- 受管镜像预装 sing-box v1.13.3，proxy 模式 nftables 防火墙 + 宿主机 IP 转发/masquerade
- 代理测试 API：SOCKS5/HTTP/vmess/ss/trojan 五种协议三项检测（连通性、出口 IP 匹配、DNS 泄漏）
- 前端出口 IP 表单动态切换 + 5 种协议字段渲染 + JSON 编辑模式 + 后端密码合并
- 列表页隧道类型 Badge + 测试状态圆点 + TestResultDialog 详情
- 4 项审计技术债务修复（stopHost CleanupHost、LookPath 预检、localStorage 持久化、WireGuard 测试拦截）

### What Worked
- **Provider 工厂模式**：RoutingProvider 按 TunnelType 委托到具体实现，WireGuard 路径零影响，扩展新类型只需新增 case
- **SingBoxProvider 对称设计**：与 TunnelProvider 15 步流水线高度对称，降低认知负担和维护成本
- **审计驱动的技术债务清理**：v1.1-MILESTONE-AUDIT 发现 4 项技术债务后立即创建 Phase 10 修复，确保交付时无已知遗留问题
- **proxy 防火墙解耦**：独立 ApplyProxyFirewallRules 函数，不修改现有 WireGuard 防火墙逻辑，两条路径完全隔离
- **前端 superRefine 条件校验**：tunnel_type + edit_mode 双重条件校验比 discriminatedUnion 更直观

### What Was Inefficient
- **Phase 8 SUMMARY 重复条目**：部分提交记录和 SUMMARY 有重复版本（如 08-01 两版 SUMMARY），可能是执行中的重试导致
- **vmess/ss/trojan 测试依赖控制面 sing-box**：设计上选择在控制面进程内探测，但控制面容器化部署时不一定预装 sing-box，只能降级返回错误而非真正测试
- **测试结果仅前端持久化**：localStorage 方案可用但不理想，跨设备不同步；后端持久化被推迟

### Patterns Established
- **RoutingProvider 工厂委托**：单一 Provider 接口注入，内部 switch 委托到具体实现，default 走 WireGuard
- **sing-box tun 模式部署模式**：nsenter 后台启动 + waitForTun0 轮询就绪 + pkill 兜底清理
- **proxy_config JSONB 存储**：代理配置以 sing-box outbound JSON 原格式存储，白名单校验协议类型和必需字段
- **form↔JSON 双向转换**：ProxyFields 组件内 formValuesToProxyConfig / proxyConfigToFormValues 导出函数
- **停机路径清理对称性**：所有停止容器的路径都必须调用 CleanupHost，与启动路径对称

### Key Lessons
1. **审计应在里程碑末期立即执行**：v1.1 审计在 Phase 9 完成后立即运行，发现的技术债务在同一天通过 Phase 10 修复完毕，避免了跨里程碑遗留
2. **JSONB 存储比多列更灵活**：proxy_config 用 JSONB 存储 outbound 配置，比为每种协议建独立列更易扩展，但需配合白名单校验确保安全
3. **对称设计大幅降低实现成本**：SingBoxProvider 与 TunnelProvider 保持对称结构，11 个计划在 3 天内全部完成
4. **前端条件校验用 superRefine 比 discriminatedUnion 更可控**：当表单有多个互斥条件维度时，superRefine 的命令式风格比声明式更清晰

### Cost Observations
- Model mix: Sonnet/Opus 组合用于研究、规划、执行和验证
- Sessions: 约 10+ 次会话完成全部 4 个阶段
- Notable: Phase 7-9 各 3 个计划，Phase 10 仅 2 个计划（技术债务修复），整体效率高于 v1.0

---

## Milestone: v2.0 — cloud-claude 透明远程 CLI

**Shipped:** 2026-04-15
**Phases:** 5 | **Plans:** 7 | **Timeline:** 1 天 (2026-04-15，Phase 24 前一天开始)

### What Was Built
- Go 单一二进制 `cloud-claude`：cobra 入口 + init 配置持久化 + Entry API 认证轮询 + SSH PTY 远程 claude 会话
- sshfs slave + 嵌入式 SFTP server 实现本地目录到容器 /workspace 实时双向映射
- shellescape 安全命令构建 + 非 TTY 管道模式 + claude 参数原样透传
- TTY/信号/窗口大小/退出码完全透传，体验与本地 claude 无差异
- 受管镜像预装 sshfs/fuse3 + AppArmor unconfined + FUSE 兼容性验证脚本 + 部署文档补充

### What Worked
- **SSH Proxy 零改造**：Phase 24 代码审查确认现有多 session channel + 全类型转发天然支持 cloud-claude，服务端无需任何修改
- **三阶段生命周期**：sshConnect → mountWorkspace → runClaude 的拆分使每个阶段可独立测试和替换
- **可注入轮询模式**：waitForMount 与 WaitForSSHReady 共享 timer+ticker+select 结构，纯单元测试无需真实 SSH 连接
- **退出码返回值上浮**：禁止 os.Exit 确保 defer term.Restore 始终执行，从架构层面修复终端恢复缺陷
- **极快的交付节奏**：5 个阶段 7 个计划 16 个任务在一天内全部完成，每个计划 1-5 分钟

### What Was Inefficient
- **Go module proxy 不稳定**：proxy.golang.org 多次 connection reset，需要手动切换到 goproxy.cn，增加了执行中的等待
- **人工验证项较多**：Phase 25-28 均为 human_needed 状态，代码层面完备但端到端验证依赖运行环境，无法在开发阶段全部覆盖

### Patterns Established
- **internal/cloudclaude 包结构**：所有客户端逻辑集中在一个包内（config/entry/ssh/mount），cmd 仅做入口编排
- **cobra-flag-passthrough**：DisableFlagParsing + ArbitraryArgs 透传远程 CLI 参数，init 子命令路由不受影响
- **SSH session pipe 适配模式**：channelRWC 将 session stdin/stdout 包装为 io.ReadWriteCloser 供协议库使用
- **verify-*.sh 结构化输出**：[PASS]/[FAIL]/[WARN] 前缀 + 汇总计数 + 非零退出码

### Key Lessons
1. **零改造验证应尽早做**：Phase 24 确认 SSH Proxy 零改造为后续 4 个阶段消除了最大风险，如果验证失败整个架构需要重新设计
2. **sshfs slave 模式比预期更简洁**：SFTP server 嵌入 Go 进程 + sshfs passive 模式，不需要额外的端口或进程管理
3. **shellescape 比手写转义更安全**：POSIX 单引号规则库成熟可靠，手写转义容易遗漏边界情况
4. **AppArmor 与 FUSE 的冲突是隐蔽陷阱**：docker-default profile 的 deny mount 规则会阻断 FUSE 挂载，但错误信息不明显

### Cost Observations
- Model mix: 以 fast model 为主，research/planning 使用默认模型
- Sessions: 约 5 次会话完成全部 5 个阶段
- Notable: 最高效的里程碑——平均每个计划 2.1 分钟，得益于 v1.0/v1.1 建立的模式和基础设施复用

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Timeline | Phases | Plans | Key Change |
|-----------|----------|--------|-------|------------|
| v1.0 | 3 days | 6 | 19 | 首个里程碑 — 建立了完整的 GSD 工作流 |
| v1.1 | 3 days | 4 | 11 | 审计驱动的技术债务修复 — 审计发现问题后立即创建修复阶段 |
| v2.0 | 1 day | 5 | 7 | 最高效里程碑 — 平均每个计划 2.1 分钟，复用现有基础设施 |

### Cumulative Quality

| Milestone | Tests | LOC | Plans | Avg Plan Duration |
|-----------|-------|-----|-------|-------------------|
| v1.0 | 76 | ~14,272 | 19 | — |
| v1.1 | 76+ | ~16,958 | 11 | — |
| v2.0 | 76+ | ~28,877 | 7 | ~2.1 min |

### Top Lessons (Verified Across Milestones)

1. 验证流程应覆盖所有阶段，不跳过早期阶段
2. 横切关注点（如事件记录）引入时应立即全量扫描覆盖范围
3. 错误码的定义端和消费端应同步建立映射
4. 对称设计（如 Provider 工厂 + 流水线对称结构）能大幅降低新功能的实现成本
5. 审计应在里程碑末期立即执行，发现的问题当天修复，避免跨里程碑遗留
6. 零改造验证应尽早做——确认现有组件能否复用，是后续阶段最大的风险消除
7. 基础设施复用是提速关键——v2.0 复用了 SSH Proxy、Entry API、受管镜像等 v1.x 基础设施，开发效率倍增
