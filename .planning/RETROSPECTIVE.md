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

## Milestone: v3.0 — 远端开发体验升级

**Shipped:** 2026-04-23
**Phases:** 8 (含 1 P0 hotfix decimal Phase 29.1) | **Plans:** 30 | **Timeline:** 5 active days (2026-04-18 → 2026-04-23)

### What Was Built

- 三层文件系统架构：Mutagen 热同步白名单（≤50MB + ignore） + sshfs 冷兜底全量懒拉 + mergerfs 单一 `/workspace` 视图，替换 v2.0 sshfs 性能天花板
- `--mount-mode=auto|full|mutagen-only|sshfs-only` 四档降级状态机：12 降级矩阵单测覆盖 + last-session.json downgrade_chain 留痕 + banner 彩色 mount 模式标签
- SSH 弱网容忍：KeepAlive 15s/4 强制下限校验 + Reconnector 退避 1/2/4/8/30s + token 复用 + BufferedStdin 灰色未确认本地 echo + ringBuf 按序回放（Plan 04/05 gap-closure 闭合 SC5/SC11）
- tmux 默认包装 + 多端共享 attach + `cloud-claude sessions ls/attach` + `--new-session`/`--take-over` + 账号级 Mutagen 单例锁（远程 flock + ErrSyncLocked 降级 sshfs-only + IsSecondaryClient=true）
- Claude Code OAuth 持久化：单 Docker named volume `claude-state-{claude_account_id}` + label + entrypoint symlink + chown 1000:1000 兜底；admin DELETE 双路径（强一致 10s + force 30s）+ 错误码 STATE_VOLUME_IN_USE_001 + 6 类 audit 事件
- `cloud-claude doctor` 5 维度 18 项 check + 6 类自动 fix + JSON schema_v1 + 退出码 0/1/2 + 第一屏降级历史 banner + scripts/ci-doctor-grep.sh M14 闸门
- 错误码统一 `<DOMAIN>_<KIND>_<NUM>` 4 段：42 条 8 域 Registry + 38 条 ≥200 字符 ExtendedExplanations + `cloud-claude explain <code>` rustc 风格
- 受管镜像 v3.0.0：mergerfs 2.41.1（GitHub static `.deb`） + mutagen-agent v0.18.1 tarball + tmux 3.6a + libfuse3 3.18.x + image.lock + CI ≤ 700MB gate
- 5 章 docs/runbooks/v3-* 升级手册（升级 / AppArmor / doctor 排障 / 持久卷 / 错误码索引）+ scripts/v3-acceptance-checklist.sh 聚合脚本
- v2.0 GetHost entry_password P0 hotfix（Phase 29.1 INSERTED）：仓储 6 个 Host 读 SQL 全补 entry_password + runtime fail-fast + entrypoint passwd -S 自检 + admin batch resync 端点

### What Worked

- **Critical Pitfalls 前置研究 + phase-level 防御任务**：研究阶段识别 C1-C8 + M13/M14 共 10 个陷阱，每个 phase PLAN.md 显式列出"防御任务"+ verification 段验证手段；最终全部覆盖且无生产事故
- **Mutagen 二进制 go:embed 集成（Q1 (a)）**：用户体验"一条命令" — 4 平台二进制 ~49MB 嵌入 cloud-claude，CI build-images workflow 拉取真实文件；首次冷启动 ≤8s 验收（除真机签字外）
- **后置 gap-closure plans（Phase 32 Plan 04/05）**：第一轮 verify 发现 Gap #1 (SC5) + Gap #2 (SC11) 后通过追加 Plan 04/05 精准补漏，避免重写整个 mount/session 模块；TestPTYReconnect_BufferedInputFlush + TestMountWorkspace_SyncLocked 三测试 PASS
- **Phase 33 post-execution patches**：UAT 过程发现 dispatcher 漏注 ClaudeAccountID + EmbeddedDispatcher 漏 wire 后追加 3 个 commit (3e2ba6b/27ab2d7/c09a4d0) 闭合，未触发新一轮 phase；Plan 02 ship + 用户 "成了" 双闸门
- **gsd-integration-checker 在 audit 阶段补强**：4 条 E2E flow 全闭环验证（cloud-claude 启动 / 网络抖动重连 / admin DELETE volume rm / doctor + ApplyFixes），暴露 SupportsMutagen 字段 spec 漂移
- **Plan-level wave 化 + 依赖注入测试 hooks**：mountMutagenDeps + strategyHooks + EmbeddedDispatcher 接口适配让 12 降级矩阵 + 6 admin handler 用例全部纯单测覆盖
- **Phase 35 skip-real-hardware 路径**：自动化 PASS 即可走 ship 闸门，3 项真机签字降级为 ship 前补签，避免被物理环境阻塞
- **TDD RED→GREEN 双 commit 模式**：Phase 31 exitcodes / Phase 34 errcodes / Phase 32 reconnect 退避序列均先证伪后实现，commit 历史清晰

### What Was Inefficient

- **Phase 31/35 缺 phase-level VERIFICATION.md**：plan-level SUMMARY 完整 + 测试覆盖足够 ship 信心，但 audit 阶段 3-source 交叉对账多花了 30+ 分钟；应在每个 phase 完成时强制走 `/gsd-verify-phase`
- **REQUIREMENTS.md traceability 表 9 条 REQ drift**：phase VERIFICATION.md 已 ✓ SATISFIED 但 traceability 表仍标 Pending，audit 时手动修订；应在 verifier 内置 REQUIREMENTS.md 同步动作
- **SUMMARY frontmatter `requirements-completed:` 字段缺失**：Phase 31 三个 plan 的 SUMMARY 完全缺这个 field；其它 phase 也部分缺；audit 阶段 3-source 交叉对账时变成空白行
- **Phase 32 第一轮 verifier 漏 Gap #1 / #2**：BufferedStdin 与 Reconnector 共享 atomic.Int32 在 PLAN 中标了"占位"但执行时被忽略，第二轮 verify 才发现；verifier 应对每个 PLAN 中标"占位/postpone"的 hook 显式询问"已闭合？"
- **mutagen 二进制 stub 提交占位**：Plan 31-01 提交了 PENDING-FETCH × 4 的占位 stub，依赖 CI build-images workflow 拉真实二进制；本地 test fixture 可能跑不起来，但通过 //go:build integration 隔离规避
- **spec/code 数字漂移**：Registry 43 vs spec 42 / ExtendedExplanations 39 vs 38 / FixerRegistry 6 vs 5 — 实现超出最小值无害，但 release notes 一致性需要后期对齐
- **Phase 35 自动化基准跑 PASS / 真机签字 deferred**：BASE-02 真机签字依赖物理拔网与多种 OS，CI 无法替代；ship 流程必须包含真机签字闸门

### Patterns Established

- **Critical Pitfalls 前置 + phase-level 防御任务**：研究阶段识别 N 个陷阱，每个 phase PLAN.md 显式列出防御任务 + verification 段验证手段
- **Gap-closure plan**：verifier 发现 SC 不达标时追加 plan-level closure（Plan 04/05）而非整体重写
- **Post-execution patch 三件套**：UAT 发现 wiring 缺口时追加 ≤3 commit 闭合，不触发新一轮 phase；以 Plan SUMMARY 的 deviation 章节追溯
- **错误码 4 段统一 + 4 域闭合**：`<DOMAIN>_<KIND>_<NUM>` 格式 + CI 单测遍历 + ExtendedExplanations 长说明 + `explain <code>` 子命令；适用于多 phase 横切关注点
- **doctor 5 维度 18 项 check + 4 要素输出**：每条 `[符号] 原因（建议: ... | 错误码: ...）` + JSON schema_v1 锁死 + `--fix` 串行 60s timeout + Status 不降级；CI grep 闸门防 PASS/FAIL only 退化
- **HUMAN-UAT 跟踪文件三段式**：status (partial/resolved) + 多 test 块（expected/how-to-verify/result/why_human）+ Summary 计数 + Resolution Path 闭环
- **gsd-integration-checker 在 audit 阶段补强**：通过 cross-phase exports/consumers 映射 + E2E flow 验证暴露 spec/code 漂移与文档侧 gap

### Key Lessons

1. **每个 phase 完成时必走 `/gsd-verify-phase`** — Phase 31/35/29.1 缺 VERIFICATION.md 让 audit 阶段 3-source 交叉对账多花 30+ 分钟；plan-level SUMMARY 不能完全替代 phase-level verifier 报告
2. **PLAN 中"占位/postpone"的 hook 必须在 verifier 中显式追问** — Phase 32 Gap #1/#2 都是 PLAN 标占位但执行时未追加 carry-over plan；verifier checklist 应包含"PLAN 中标占位的 hook 是否已闭合"
3. **REQUIREMENTS.md traceability 表应由 verifier 自动维护** — 9 条 drift REQ 在 audit 阶段才发现，verifier 跑完后应自动 PR 同步 traceability 状态
4. **Critical Pitfalls 在研究阶段识别 + phase-level 防御任务是 v3.0 最大收益** — C1-C8 + M13/M14 全部覆盖且无生产事故，证明 ROADMAP 中显式列出 pitfall 防御 phase 的做法值得复用
5. **Gap-closure plan + post-execution patch 比整体重做更经济** — Phase 32 Plan 04/05 + Phase 33 三个 post-fix 都是"小改 + 精准 verify"模式，避免触发新一轮 phase
6. **真机签字闸门必须独立于 phase 完成** — Phase 35 选择 skip-real-hardware 让 phase 不被物理环境阻塞，但 ship 流程必须强制 3 项真机签字（M5/BASE-03 2min/C6）
7. **gsd-integration-checker 在 audit 阶段补强 cross-phase wiring** — 单 phase verification 看不到 export/consumer 链路，integration check 是 audit 阶段必跑步骤
8. **spec/code 数字漂移要在 ship 前对齐** — Registry 43 vs 42 / FixerRegistry 6 vs 5 等差异虽不影响功能，但 release notes 与运维手册的一致性需要 ship 前统一修订

### Cost Observations

- **Model mix**: balanced profile（model_profile = "balanced"）；planning/research 用默认 + Opus 多模型组合，execution 用 fast model
- **Sessions**: 约 30+ 次会话完成全部 8 个 phase + audit；Phase 32/33 各占 ~6-8 次（gap-closure + post-fix 多轮迭代）
- **Notable**: Phase 33 是最复杂 phase（control plane + image + admin GC 三层联动 + UAT 用户 "成了" 双闸门），耗时 2 天；Phase 35 自动化部分 5 plan 在 1 天内完成
- **Mutagen go:embed 增加 binary size**：cloud-claude 二进制从 ~15MB 涨到 ~64MB（4 平台 ~49MB），用户分发体积较大但消除 brew install 依赖

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Timeline | Phases | Plans | Key Change |
|-----------|----------|--------|-------|------------|
| v1.0 | 3 days | 6 | 19 | 首个里程碑 — 建立了完整的 GSD 工作流 |
| v1.1 | 3 days | 4 | 11 | 审计驱动的技术债务修复 — 审计发现问题后立即创建修复阶段 |
| v2.0 | 1 day | 5 | 7 | 最高效里程碑 — 平均每个计划 2.1 分钟，复用现有基础设施 |
| v3.0 | 5 days | 8 (含 1 hotfix) | 30 | 最复杂里程碑 — Critical Pitfalls 前置研究 + Gap-closure plan + post-execution patch + integration-checker audit |

### Cumulative Quality

| Milestone | Tests | LOC | Plans | Avg Plan Duration |
|-----------|-------|-----|-------|-------------------|
| v1.0 | 76 | ~14,272 | 19 | — |
| v1.1 | 76+ | ~16,958 | 11 | — |
| v2.0 | 76+ | ~28,877 | 7 | ~2.1 min |
| v3.0 | 200+ | ~45,766 | 30 | ~10-30 min（含 gap-closure / post-fix）|

### Top Lessons (Verified Across Milestones)

1. 验证流程应覆盖所有阶段，不跳过早期阶段（v1.0 → v3.0 反复印证：Phase 1 / 11/12 / 31/35/29.1 都因缺 VERIFICATION.md 在 audit 阶段付额外成本）
2. 横切关注点（如事件记录、错误码）引入时应立即全量扫描覆盖范围
3. 错误码的定义端和消费端应同步建立映射（v1.0 worker bootstrap_errors 漏映射 → v3.0 Phase 34 用 4 段 Registry 闭合）
4. 对称设计（Provider 工厂 / 流水线对称 / Wave 化依赖注入 hooks）能大幅降低新功能的实现成本
5. 审计应在里程碑末期立即执行，发现的问题当天修复，避免跨里程碑遗留
6. 零改造验证应尽早做（v2.0 Phase 24 SSH Proxy 零改造 = 后续 4 phase 最大风险消除）
7. 基础设施复用是提速关键（v2.0 → v3.0 复用 Entry API / 受管镜像 / SSH Proxy / 退出码透传）
8. **Critical Pitfalls 前置研究 + phase-level 防御任务（v3.0 新）** — C1-C8 + M13/M14 全部覆盖且无生产事故
9. **Gap-closure plan + post-execution patch 比整体重做更经济（v3.0 新）** — Plan 04/05 + Phase 33 三个 post-fix 都验证了"小改 + 精准 verify"模式
10. **REQUIREMENTS.md traceability 应由 verifier 自动同步（v3.0 新发现）** — 9 条 drift REQ 在 audit 阶段才发现，verifier 跑完后应自动同步状态
11. **真机签字闸门必须独立于 phase 完成（v3.0 新）** — skip-real-hardware 路径让 phase 不被物理环境阻塞，但 ship 流程强制真机签字

---

## Milestone: v3.1 — 映射语义补齐与懒加载

**Shipped:** 2026-04-24
**Phases:** 2 | **Plans:** 11 | **Timeline:** ~26 days span (2026-03-29 → 2026-04-24, Phase 36-37 active execution)

### What Was Built

- git 仓库强约束：`runRoot` 中 `os.Getwd()` + `git rev-parse --show-toplevel` 前置检查，`MOUNT_REQUIRE_GIT_REPO` + 中文 next_action + `exitConfigError`
- 单文件 50MB 熔断：`Config.HotSyncMaxFileMB` / `MountConfig.effectiveHotSyncMaxFileMB()` 兜底 50MB；`HotSyncEngine.applyOversizedFilter` 双路径（initialSync 记录 + syncOnce 静默）；`last-session.json::oversized_files` + D-08 stderr 一次性提示
- sshfs FUSE page cache：`mountSSHFS` 追加 `cache=yes,kernel_cache,auto_cache,cache_timeout=300`；`TestSSHFSCacheHitsKernelPageCache` fixture 计数器单测验证同会话二次 read count = 1
- doctor mount 9 项 check：`checkRequireGitRepo` / `checkOversizedFilesCount` / `checkSSHFSCacheArgs` / `checkGitProxyEnabled` / `checkDefaultIgnoreLoaded`；13 条矩阵测试 + CI ci-doctor-grep 三段闸门继续 PASS
- 2 条新错误码 + `explain` 长说明：`MOUNT_REQUIRE_GIT_REPO` / `MOUNT_OVERSIZED_FILE_SKIPPED`，rustc 风格 ≥200 字中文 ExtendedExplanation
- ColdPromoter 核心引擎：`cold_promoter.go` inotify `IN_OPEN/IN_ACCESS` watcher + `PromotionEngine` 异步队列；5s `dedupWindow` + 1/2/4s 指数退避 + `circuitBreaker` 3 次熔断；`QueueDepth/Stats/Wait` 可观测 API；4 条核心单测（dedup/backoff/circuit/start-stop）含 -race PASS
- tryModeReal Full 路径集成：mergerfs ready → `NewColdPromoter` → `go promoter.Run(ctx)`；cleanup LIFO：`promoterCancel → promoter.Wait → cancel watcher → merge → sshfs → hot_sync`
- `CLOUD_CLAUDE_NO_PROMOTION=1` 全量关闭：promoter 保持 nil，cleanup guard 跳过，snapshot 字段零值 omitempty 不写入 JSON
- promotion 统计持久化：`LastSessionSnapshot` 新增 `PromotionCount/PromotionBytes/PromotionFailedCount`；stats 在 writeLastSession 前刷入（mount 就绪时 = 0，plan 接受语义）
- doctor 晋升 4 项可观测指标：`promoter_alive` / `promotion_queue_depth` / `promotion_total` / `promotion_failed_total`
- 运维手册：`docs/runbooks/v31-cold-promotion.md` Pattern G（头部 + 6 章节 + 快速诊断命令），覆盖原理图 / env var / 故障排查 / mergerfs 边界 / 5 错误码反查
- e2e UAT 脚本：`tests/scripts/uat-v31-promotion.sh` 619 行，6 场景（git_reject / oversized_skip / fuse_cache_hit / cold_promotion / no_promotion / json_report），`--dry-run` 默认安全 + `--confirm-destructive` 触发实际 mount；JSON schema_version=1；CI 接入 `make ci-gate`

### What Worked

- **Phase 36 零跨进程协议变更**：纯配置/校验/参数级改动，6 个 plan 全部独立可发，无新增依赖
- **effective*() accessor 兜底模式**：`MountConfig.effectiveHotSyncMaxFileMB()` 私有 accessor 防止 main.go 未注入字段时静默关闭熔断；与 Config 层同一 50MB 默认值
- **ColdPromoter 平台兼容分离**：Linux 真实 inotify / macOS stub 通过 `//go:build` 分离，`+linux` 文件 167 行 / `+darwin` stub 23 行；macOS 开发不阻塞
- **测试隔离走 `t.Setenv` 而非 var 注入**：`checkGitProxyEnabled` / `checkOversizedFilesCount` / `git_check_test.go` 全部用 `t.Setenv("HOME", tempDir)` 或 `t.Setenv("PATH", "")` 隔离，生产代码零注入点
- **PromotionEngine 单测聚焦核心语义**：dedup（100ms 内 50 次 enqueue → 1 次实际拉取）、backoff（1/2/4s 序列）、circuit（3 次失败 → 不再尝试）、start-stop（goroutine 生命周期）；不依赖真实 SFTP 服务器
- **UAT 脚本 dry-run / confirm-destructive 双闸门**：默认安全，非 Linux 平台场景 3/4/5 自动 SKIP；与 v3.0 `uat-network-resilience.sh` / `degradation-regression.sh` 同一框架

### What Was Inefficient

- **Phase 36-06 4 个 Rule 1 / Rule 2 auto-fix**：plan 字面量与 helper 实际签名/语义对齐的局部修订（newFail 占位符双重 Sprintf / newWarn 缺 args 导致 `%s` 字面量 / mount.go 缺 cloudclaude import / mount_test.go 缺 4 个 import）；虽全部 auto-fixed 且行为契约不变，但说明 plan 中代码示例需要更严格的编译期验证
- **Phase 36-03 `effectiveHotSyncMaxFileMB()` 是 plan 范围外兜底**：plan 字面量引用 `Config.EffectiveHotSyncMaxFileMB()` 但 `MountConfig` 无此方法；main.go 不在 plan 范围，必须新增 accessor 避免 SC#2 失效；plan 应在 action 段显式标注"若 cfg 为 MountConfig 需自有 accessor"
- **Phase 37-02 promotion stats 刚启动 = 0 的语义需要 plan 显式接受**：mount 就绪时 promoter 刚启动，Stats() 返回 (0,0,0)；若用户期望"首次 mount 就有统计"会困惑；plan 明确记录此语义但文档侧未同步到 runbook
- **Phase 37 5 项人工验证 deferred-to-ship**：与 v3.0 同样模式 — 自动化 PASS 后真机签字 ship 前补签；cold-promoter 需要 Linux 容器环境，macOS 开发机无法本地验证完整链路
- **v3.1 与 v3.0 同跨度 26 天（Phase 36 实际 1 天执行）**：Phase 36 从 2026-03-29 定义到 2026-04-23 执行，中间有 v3.0 收尾和 quick task 穿插； milestones 的"跨度"不等于"专注执行时间"

### Patterns Established

- **`effective*()` 私有 accessor 兜底**：MountConfig / Config 层需要默认值的字段统一走 `(c *T) effectiveField()` 模式，防止零值静默关闭功能
- **测试隔离 `t.Setenv` 优先于 var 注入**：生产代码零注入点，测试通过环境变量隔离真实代码路径
- **ColdPromoter 5s dedup + 指数退避 + circuit 三件套**：inotify 事件风暴防护的标准模式；与 HTTP client retry 不同（非 429 感知，而是文件系统事件去重）
- **UAT 脚本框架复用**：pass/fail/skip helper + dry-run 默认安全 + JSON schema_version=1 + `--confirm-destructive` 中文 opt-in；跨里程碑一致
- **doctor 新增 check 三处对应**：mount.go 同文件追加函数 + doctor.go 维度 block append + mount_test.go 矩阵测试；三处一一对应，避免遗漏
- **errcodes Entry.Message 含 `%s` 时直接传裸值**：`newFail(domain, name, code, args...)` 内部 `fmt.Sprintf(entry.Message, args...)`；plan 中不应预渲染字符串

### Key Lessons

1. **plan 中代码示例需编译期验证** — Phase 36-06 4 个 auto-fix 全部是 plan 代码示例与 helper 实际签名不对齐；plan-checker 应在 round 2 增加"代码示例是否可编译"检查
2. **`effective*()` accessor 应在 plan 中显式标注** — Phase 36-03 的兜底 accessor 是执行时发现的，plan 应在 action 段注明"若字段可能在 main.go 未注入时调用，需新增 effective accessor"
3. **mount 就绪时的零值统计语义需文档同步** — Phase 37-02 promotion stats (0,0,0) 在 plan 中接受但 runbook 未说明；用户可见行为必须在 runbook FAQ 中解释
4. **macOS stub + Linux 真实实现的双平台模式有效** — ColdPromoter 167+23 行分离让 macOS 开发不阻塞，CI `make ci-gate` 在 macOS 上 dry-run PASS；真机验证留到 Linux 环境
5. **Phase 36 纯配置/参数级改动的"独立可发"验证** — 零新增依赖、零跨进程协议变更、零 schema 变更；6 plan 在 1 天内完成，证明前期架构设计到位后增量功能可极快交付
6. **UAT 脚本 dry-run 在 CI 中的价值** — macOS CI 跑 dry-run 可验证脚本框架/JSON schema/场景覆盖完整性，虽然无法验证真实 mount 行为，但已能 catch 脚本语法错误和场景遗漏

### Cost Observations

- **Model mix**: balanced profile；Phase 36 以 fast model 为主（配置/参数级改动），Phase 37 涉及并发 goroutine 和 inotify 用默认模型
- **Sessions**: 约 8-10 次会话完成 2 个 phase（Phase 36: ~3 次，Phase 37: ~5-7 次含 e2e UAT）
- **Notable**: Phase 36 是最高效的 phase 之一（6 plan ~1 天），得益于纯配置/校验级改动；Phase 37 ColdPromoter 并发逻辑 + e2e UAT 脚本耗时较多
- **Code growth**: +2,568 Go LOC（32,103 vs 29,535），主要是 ColdPromoter（~170 行）+ doctor check（~115 行）+ 测试（~220 行）+ UAT 脚本（619 行）

---

## Milestone: v3.4 — 多形态容器接入

**Shipped:** 2026-05-08 (tag v3.4.0)
**Phases:** 7 (含 3 gap closure) | **Plans:** 14 | **Timeline:** 2 days (2026-05-07 → 2026-05-08)

### What Was Built

- SSH Proxy 端口转发：direct-tcpip + tcpip-forward/forwarded-tcpip 全通道，管理网段/Docker socket/metadata 安全拦截
- `cloud-claude local up/down/status`：本地 Dev Containers 支持 + entrypoint `MODE=local` + sing-box tun/proxy 出网
- VS Code Remote-SSH E2E 验证：端到端连接 + 端口转发 + 出口 IP 强约束
- Doctor remote-ssh 维度：5 项检查 + 6 个新错误码 + 20 个单元测试
- doctor sshd_config 主动验证：parseSSHDForwarding + 3 个错误码 + 13 个单元测试

### What Worked

- **审计驱动的 gap closure 链**：第一轮审计发现 3 个缺口 → Phase 42/43/44 精准补齐 → 第二轮审计全通过。闭环效率高。
- **纯函数设计 `isForbiddenTarget`**：不依赖 Server 结构体，单测覆盖率高
- **`ssh.Conn` 接口抽象**：`handleGlobalRequests` 保持通用性，测试用 server-side conn 验证
- **Phase 38 预 dial 架构**：共享 targetClient 避免 per-channel dial，与 forwarded-tcpip API 契合
- **entrypoint MODE 分支复用**：`MODE=local` 跳过特定栈但复用 sshd + sing-box，实现简洁

### What Was Inefficient

- **Phase 39 无独立 VERIFICATION.md**：实现时未同步生成验证文件，审计时发现需要 Phase 42 补齐
- **Phase 40 UAT 脚本不完整**：首版只有 6 个场景，缺少端口转发验证，Phase 43 补到 9 个
- **UX-01 REQUIREMENTS.md 勾选遗漏**：审计时发现 UX-01 行仍为 `[ ]`，traceability 表已 Complete 但正文未同步

### Patterns Established

- **审计 → gap closure → 再审计**三步闭环：里程碑审计发现问题后，立即创建 gap closure phase 精准修复，修复后再跑一轮审计确认。
- **VERIFICATION.md 应在实现阶段同步生成**：不应等到审计时才发现缺失
- **UAT 脚本按"场景数"增长**：功能扩展时 UAT 场景应同步增加（6→9）

### Key Lessons

1. Phase 实现时就生成 VERIFICATION.md，不要推到 gap closure 阶段
2. UAT 脚本首版应覆盖所有已知场景类型（端口转发是 v3.4 核心，不应遗漏）
3. REQUIREMENTS.md 正文和 traceability 表应自动同步校验
4. `ssh.Conn` 接口比 `*ssh.Client` 更适合测试和抽象层次

### Cost Observations

- **Model mix**: balanced profile
- **Sessions**: 约 5-7 次会话完成 7 个 phase（Phase 38: ~3 次，其余各 ~1 次）
- **Notable**: 最高效的里程碑之一（2 天 14 plans），得益于 Phase 38 架构清晰 + gap closure 精准
- **Code growth**: +8,599 Go LOC（40,702 vs 32,103），主要是 SSH 端口转发 + local 子命令 + doctor checks + 测试

---

## Milestone: v4.2.0 — 容器合并 · SQLite 迁移 · 配置统一

**Shipped:** 2026-06-01
**Phases:** 5 (58-62) | **Plans:** 3 (Phase 58 waves) | **Timeline:** 1 day (2026-06-01)

### What Was Built

- PostgreSQL → SQLite 全项目迁移：移除 pgx/v5，引入 modernc.org/sqlite + google/uuid（纯 Go，零 CGO）
- migrator 重写为 database/sql + embed.FS 内嵌迁移文件
- 21 个迁移文件从 PG 语法改写为 SQLite 语法（UUID/TIMESTAMPTZ/JSONB → TEXT，BOOLEAN → INTEGER）
- queries.go (1690 行) + queries_bypass.go (652 行) 从 pgx 重写为 database/sql
- App 初始化 sql.Open + PRAGMA WAL/foreign_keys/busy_timeout，SetMaxOpenConns(1)
- Admin 前端 dist 通过 //go:embed 嵌入 Go 二进制，SPA fallback handler，统一端口 :8080
- sing-box 探针二进制内嵌至 control-plane Dockerfile，优先 native 模式
- Docker Compose 5 服务精简至 2 服务（control-plane + managed-user），移除 PostgreSQL/admin/sing-box 容器
- 环境变量统一：移除 POSTGRES_*，DATABASE_URL 改为 SQLite 路径
- Makefile 移除 db/db-stop/db-reset 目标和 PG 检测；deploy/scripts 移除 PostgreSQL 交互
- README/文档站/FAQ 全面更新
- 单元测试从 testcontainers PG 切换到内存 SQLite

### What Worked

- **Phase 58 wave 化执行**：3 波按依赖顺序推进（依赖切换 → 迁移+查询重写 → 全项目引用清除），每波独立可验证，偏差自动修复
- **embed.FS 迁移文件**：完全消除外部文件依赖，单一二进制包含所有迁移 SQL
- **database/sql 标准库**：零新依赖，与 Go 标准库无缝集成，WAL 模式性能满足单机需求
- **SPA fallback handler**：简洁的 //go:embed + http.FileServer + 自定义 404 回退，无需独立 nginx
- **sing-box native 优先**：宿主机有二进制直接用，Docker 作为回退，灵活且高效
- **atomic commits + auto-fix**：Phase 58 plan 执行中发现的 3 个 Bug（缺 embed.go、pgx.Tx 不兼容、SQLite 语法不兼容）全部由 executor 自动修复

### What Was Inefficient

- **Phase 59-62 全部 inline 执行**：没有生成正式的 PLAN.md / SUMMARY.md / VERIFICATION.md，仅 Phase 58 有完整 GSD 工件集
- **Phase 58 Plan 01 拆分不足**：Task 1（依赖切换）和 Task 2（migrator 重写）必须合并执行，因为 go mod tidy 会重新引入 pgx 依赖——plan 设计时应考虑原子性边界
- **REQUIREMENTS.md 追溯表未同步**：PRB/DEP 共 7 条需求在审计确认完成时，追溯表仍显示 Pending，audit 后才手动修正
- **codebase/ 文档未随里程碑更新**：ARCHITECTURE/DEPENDENCIES/COMPONENTS 仍然引用 PostgreSQL/pgx/pgxpool，与代码实际状态不符
- **go.mod 中 pgx/v5 残留**：因 tests/e2e/helpers_linux.go 的 e2e 构建标签而保留，虽非阻塞但属于技术债务

### Patterns Established

- **纯 Go SQLite (modernc.org/sqlite)**：零 CGO 依赖，WAL 模式，单文件数据库，适合单机部署
- **embed.FS 迁移系统**：迁移 SQL 文件嵌入 Go 二进制，消除运行时文件路径依赖
- **SPA fallback 模式**：//go:embed 嵌入前端 dist + 自定义 FileSystem wrapper + router 注册
- **sing-box native 优先模式**：探针启动时先检查宿主机二进制路径，不存在才回退 Docker 容器
- **统一端口 :8080**：所有 HTTP 流量走单一端口，简化部署和防火墙配置

### Key Lessons

1. **Plan 应显式标注原子性边界** — Phase 58 Plan 01 拆分了两个必须一起执行的任务，导致执行时自动合并偏差；plan 中应标注"以下 task 必须同波执行"的强依赖
2. **里程碑审计应自动触发文档同步** — codebase/ 文档和追溯表的过期是重复出现的问题（v1.0 → v4.2.0 反复印证），应在审计检查清单中自动提醒
3. **inline phase 至少应保留 SUMMARY** — Phase 59-62 的 inline 执行虽然快速，但缺少 SUMMARY 让后续回顾和跨里程碑趋势分析失去依据
4. **构建标签下的遗留依赖要有明确清理计划** — pgx/v5 因 e2e 构建标签保留，应在下一里程碑 E2E 适配时一并清理
5. **SQLite 迁移无痛切换的前提是标准库接口** — pgx 到 database/sql 的接口差异（没有 QueryRow direct scan、没有 Batch、没有 CopyFrom）需要在 plan 阶段充分评估影响面

### Cost Observations

- **Model mix**: balanced profile；Phase 58 使用默认模型（大规模重写），Phase 59-62 以 fast model 为主
- **Sessions**: 约 3-5 次会话完成全部 5 个 phase
- **Notable**: Phase 58 是核心复杂度所在（3 waves × 2 commits = 6 commits），Phase 59-62 均为小粒度变更（1 commit each）
- **Code change**: 约 40+ 文件被修改，核心变更是 queries.go (1690 行) + queries_bypass.go (652 行) 的完整重写

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Timeline | Phases | Plans | Key Change |
|-----------|----------|--------|-------|------------|
| v1.0 | 3 days | 6 | 19 | 首个里程碑 — 建立了完整的 GSD 工作流 |
| v1.1 | 3 days | 4 | 11 | 审计驱动的技术债务修复 — 审计发现问题后立即创建修复阶段 |
| v2.0 | 1 day | 5 | 7 | 最高效里程碑 — 平均每个计划 2.1 分钟，复用现有基础设施 |
| v3.0 | 5 days | 8 (含 1 hotfix) | 30 | Critical Pitfalls 前置 + Gap-closure + post-fix + integration-checker |
| v3.1 | ~1 day active | 2 | 11 | 纯配置/参数级改动 + 并发引擎 + e2e UAT；effective accessor 兜底模式 |
| v3.4 | 2 days | 7 (含 3 gap) | 14 | 审计驱动 gap closure 闭环；SSH 端口转发 + 本地 Dev Containers |
| v4.2.0 | 1 day | 5 | 3 | SQLite 全项目迁移 + 服务精简至 2 个 + 前端/探针内嵌至单一二进制 |

### Cumulative Quality

| Milestone | Tests | LOC | Plans | Avg Plan Duration |
|-----------|-------|-----|-------|-------------------|
| v1.0 | 76 | ~14,272 | 19 | — |
| v1.1 | 76+ | ~16,958 | 11 | — |
| v2.0 | 76+ | ~28,877 | 7 | ~2.1 min |
| v3.0 | 200+ | ~45,766 | 30 | ~10-30 min |
| v3.1 | 230+ | ~48,953 | 11 | ~5-15 min |
| v3.4 | 263+ | ~43,300 | 14 | ~2-10 min |
| v4.2.0 | 263+ | ~43,000 | 3 | ~5-18 min |

### Top Lessons (Verified Across Milestones)

1. 验证流程应覆盖所有阶段，不跳过早期阶段（v1.0 → v4.2.0 反复印证）
2. 横切关注点引入时应立即全量扫描覆盖范围
3. 错误码的定义端和消费端应同步建立映射
4. 对称设计能大幅降低新功能实现成本
5. 审计应在里程碑末期立即执行，发现的问题当天修复
6. 零改造验证应尽早做
7. 基础设施复用是提速关键
8. Critical Pitfalls 前置研究 + phase-level 防御任务
9. Gap-closure plan + post-execution patch 比整体重做更经济
10. REQUIREMENTS.md traceability 应由 verifier 自动同步
11. 真机签字闸门必须独立于 phase 完成
12. **`effective*()` accessor 兜底防止零值静默关闭功能（v3.1 新）** — MountConfig / Config 层统一模式
13. **plan 中代码示例需编译期验证（v3.1 新）** — 4 个 auto-fix 说明 plan-checker 应增加可编译性检查
14. **macOS stub + Linux 真实实现的双平台分离（v3.1 新）** — 开发不阻塞，真机验证留到目标平台
15. **审计→gap closure→再审计 三步闭环（v3.4 新）** — 精准修复比一次性修补更可靠，gap closure phase 应即开即关
16. **VERIFICATION.md 应在实现阶段同步生成（v3.4 新）** — 推到审计阶段补齐会多浪费一个 gap closure phase
17. **Plan 应显式标注原子性边界（v4.2.0 新）** — Phase 58 Plan 01 拆分导致 go mod tidy 重引入旧依赖，task 间有隐式原子依赖时应在 plan 中标注
18. **inline phase 至少应保留 SUMMARY（v4.2.0 新）** — Phase 59-62 inline 执行缺少工作总结，后续回顾失去依据
19. **里程碑审计应自动触发文档同步（v4.2.0 新）** — codebase/ 文档和 REQUIREMENTS.md 追溯表在审计后仍显示旧状态，应在审计流程中加入自动更新步骤
