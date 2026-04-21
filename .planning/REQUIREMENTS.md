# Requirements: Cloud CLI Proxy

**Defined:** 2026-04-18
**Milestone:** v3.0 远端开发体验升级
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

> v3.0 在已 ship 的 v2.0 cloud-claude 基础上做体验升级。需求清单基于 `.planning/research/SUMMARY.md` §3，每条对应可观察的用户行为或可断言的系统行为。

---

## v3.0 Requirements

按三条主线（A · 文件映射、B · 会话可靠性、C · 运维与体验配套）组织。每条 REQ 在后续 ROADMAP.md 中映射到唯一一个 phase。

### A1 · 三层文件系统架构（F1）

- [x] **REQ-F1-A**：容器内仅暴露单一 `/workspace` 路径；用户与 Claude Code 不感知 hot/cold 分层（mergerfs 把 Mutagen 同步分支与 sshfs 兜底分支合并为单一视图）
- [x] **REQ-F1-B**：cloud-claude 启动后，从命令执行到出现可输入 prompt 的总耗时 ≤ 8s（含首轮 Mutagen 同步），过程中输出三段式中文进度：`初始化文件映射 (1/3) 热同步源码中…`
- [x] **REQ-F1-C**：在 10k 文件源码树执行 `rg .` / `ls -R` 的延迟 ≤ 等价本地操作的 1.5×
- [x] **REQ-F1-D**：候选同步目录大小 > 50MB 时，cloud-claude 拒绝热同步并自动改用 sshfs 兜底，同时给出明确中文提示与 ignore 配置建议
- [x] **REQ-F1-E**：Mutagen 同步出现 conflict 时，下次回车前在 prompt 上方插入中文警告：`⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`

### A2 · 降级路径与 `--mount-mode` 手动切换（F2）

- [x] **REQ-F2-A**：CLI 支持 `--mount-mode=auto|full|mutagen-only|sshfs-only` 四档切换，默认 `auto`
- [x] **REQ-F2-B**：三层 mount 任一失败时，cloud-claude 必须在 2 秒内降级到下一档；禁止静默降级（stderr 必须输出当前生效模式 + 错误码）
- [x] **REQ-F2-C**：每次连接成功的 banner 必须用彩色标签显示当前 mount 模式（尊重 `NO_COLOR` 环境变量）

### B1 · SSH 会话稳定性与自动重连（F3）

- [ ] **REQ-F3-A**：客户端默认 `ServerAliveInterval=15s` / `ServerAliveCountMax=4`；服务端默认 `ClientAliveInterval=15s` / `ClientAliveCountMax=8`；二者均不允许配置低于 15 秒
- [x] **REQ-F3-B**：网络中断期间用户键入字符在客户端本地缓冲并以"未确认"灰色样式显示，重连成功后按序提交（对标 Mosh 本地 echo）
- [ ] **REQ-F3-C**：cloud-claude 自动重连失败时，prompt 必须显示具体失败原因 + 下一步操作（按 Enter 重试 / 运行 `cloud-claude doctor`）
- [ ] **REQ-F3-D**：重连过程使用退避策略 `1s → 2s → 4s → 8s → 30s 上限`，复用本地缓存的 Entry API token，不重新弹出密码

### B2 · 会话恢复（tmux 默认包装，F4）

- [ ] **REQ-F4-A**：容器内 SSH 会话默认用 tmux 包装（`exec tmux new-session -A -s claude`），网络中断后重连必须 attach 到同一会话且运行中的 Claude Code 进程不丢失
- [ ] **REQ-F4-B**：用户可通过 `cloud-claude sessions ls` / `cloud-claude sessions attach <name>` 管理多个并行会话
- [ ] **REQ-F4-C**：当容器内 tmux 不可用时 cloud-claude 不得阻塞启动，但必须在 banner 明确提示 `[!] 容器内 tmux 不可用，会话恢复已禁用`

### B3 · 多端同账号 attach 同一会话（F5）

- [ ] **REQ-F5-A**：默认行为为多端共享 attach 同一 session，不踢人不报错
- [ ] **REQ-F5-B**：第二端 attach 成功后必须在 banner 显示其它 client 的来源与活跃时间（中文），例：`✓ 已 attach 到会话 claude-proj（另 1 个会话正在共享：mac-home / 5 分钟前活跃）`
- [ ] **REQ-F5-C**：CLI 提供 `--new-session` 创建独立 session（命名 `claude-<short_id>`）；提供 `--take-over` 强制独占并通知其它端；冲突时返回明确中文提示
- [x] **REQ-F5-D**：同一 claude_account 在任意时刻最多只允许 1 个活跃 Mutagen sync session；后连端只 attach tmux 与观察文件，不参与文件同步

### C1 · `cloud-claude doctor` 全面升级（F6）

- [x] **REQ-F6-A**：`cloud-claude doctor` 必须覆盖 5 个维度：**network / auth / ssh / mount（mutagen + sshfs + mergerfs 三层）/ disk**
- [x] **REQ-F6-B**：每项检查输出包含 4 个要素：状态符号 `[✓][!][✗]` + 简短中文原因 + 中文修复建议 + 错误码（缺一不可）
- [x] **REQ-F6-C**：`cloud-claude doctor --fix` 能自动修复至少 5 类常见失败：mutagen agent 无响应、FUSE 残留挂载、known_hosts 冲突、token 过期、DNS 缓存污染
- [x] **REQ-F6-D**：doctor 支持 `--verbose`（展开探测细节）、`--json`（脚本消费）、`NO_COLOR`（关闭颜色）；退出码 `0/1/2` 与 `brew doctor` 语义对齐

### C2 · Claude Code 状态持久化（F7）

- [ ] **REQ-F7-A**：`~/.claude/` 与 `~/.cache/claude` 通过独立 Docker named volume 持久化；命名粒度 = 单个 `claude_account`（建议 `claude-state-{claude_account_id}`，带 label `com.cloud-cli-proxy.account_id`）
- [ ] **REQ-F7-B**：容器重建后未过期的 OAuth credentials 必须保留，用户无需重新执行 `claude login`
- [x] **REQ-F7-C**：credentials 即将过期或已过期时，cloud-claude 必须在连接建立前给出明确的中文提示，不能让 claude 进程进入报错后才发现
- [ ] **REQ-F7-D**：通过 admin API 删除 claude_account 时，事务性联动删除对应的 Docker named volume

### C3 · 错误码与中文提示统一升级（F8）

- [x] **REQ-F8-A**：v3.0 新引入的所有错误路径纳入统一错误码体系，code 格式 `<DOMAIN>_<KIND>_<NUM>`；新增分类前缀 `MOUNT_*` / `SESSION_*` / `NET_*` / `STATE_*`
- [x] **REQ-F8-B**：每条错误输出包含三要素：错误码 + 中文原因 + 中文下一步建议（缺一不可）
- [x] **REQ-F8-C**：`cloud-claude explain <code>` 子命令对每个错误码给出详细中文说明与常见修复步骤（对标 `rustc --explain`）

---

## 性能与体验验收基线

下列基线作为对应 phase 的 verification 依据（详见 PROJECT.md "Current Milestone" 段）：

| # | 维度 | 基线 |
|---|------|------|
| BASE-01 | 元数据响应 | 10k 文件源码树 `rg .` / `ls -R` ≤ 等价本地操作 1.5× |
| BASE-02 | 首次连接 | 命令执行 → 可输入 prompt ≤ 8s（含首轮 Mutagen 同步） |
| BASE-03 | 弱网容忍 | 30s 内的网络抖动对会话无感知（运行进程不掉、tmux 自动重连） |
| BASE-04 | 镜像体积 | v3 受管镜像未压缩 ≤ 700MB（CI gate） |

---

## v3.x Requirements（已识别但 v3.0 不交付）

### 增量优化（拟 v3.1）

- **ENH-V3.1-01**：容器预热与空闲回收策略（避免每次冷启动 5–10s 等待）
- **ENH-V3.1-02**：性能 metrics 实时上报到 admin 后台（首连耗时、mount 模式、抖动事件分布）
- **ENH-V3.1-03**：admin 后台 host 详情页展示 mount 模式 / session 数 / persistent volume 列表
- **ENH-V3.1-04**：Mutagen 首轮全量同步针对 mono-repo 的优化策略（lazy probe / progressive scan）

---

## Out of Scope（v3.0 明确不做）

直接来自 `.planning/research/SUMMARY.md` §7（已与各子研究 anti-feature 一一对账）。所有条目 v3.0 不接受任何形式的"小做一点"。

| # | 不做 | 原因 |
|---|------|------|
| OOS-A1 | Web UI 显示 Mutagen 同步进度条 | cloud-claude 是 CLI 工具；引入 Web 控制面会放大鉴权复杂度，用 banner + `cloud-claude status` 替代 |
| OOS-A2 | 暴露 Mutagen 5 种冲突解决模式给用户 | 99% 场景 `two-way-safe` 够用；开放选项反而误配置（mutagen #533） |
| OOS-A3 | 用 bcachefs / 升级 OverlayFS 替代 mergerfs | OverlayFS 不支持 FUSE-on-FUSE；mergerfs 是当下唯一稳定方案 |
| OOS-A4 | `--mount-mode=none`（完全不挂载本地） | 等于回到原生 ssh，破坏 cloud-claude 核心承诺 |
| OOS-A5 | 运行时自动"升级" mount mode | 动态改挂载点会让进行中的 claude 进程看不到文件 |
| OOS-A6 | 用 UDP / Mosh 协议替代 SSH | 与 sing-box tun + nftables 默认拒绝模型不兼容 |
| OOS-A7 | 断网时自动杀掉 claude 进程 | 用户会丢失未保存工作（VS Code Remote #274774 同样的踩坑） |
| OOS-A8 | tmux 会话永不过期 | 违背容器生命周期契约，给后续资源回收埋雷 |
| OOS-A9 | 默认独占（新端踢旧端） | 违反"两个屏都想看"的用户直觉；要独占必须显式 `--take-over` |
| OOS-A10 | 实时协作光标（VS Code Live Share 风格） | 超出 CLI 能力范围，tmux 做不到 |
| OOS-A11 | doctor 自动上报"诊断报告"到服务端 | 数据脱敏与合规风险，v3.0 不开此坑 |
| OOS-A12 | doctor 自动改用户本地 SSH config | 不可逆；VS Code Remote #8910 大量抱怨 |
| OOS-A13 | 用 `ANTHROPIC_API_KEY` 替代 OAuth | 官方明确互斥（claude-code #5767），混用会强制降级到按量付费 |
| OOS-A14 | 容器内做 `claude login` 交互式 OAuth | 全隧道出网会打断 OAuth 回调 |
| OOS-A15 | 错误用 emoji 提示替代 ASCII 符号 | 终端 emoji 宽度 / CI 日志兼容性差 |
| OOS-A16 | 多宿主机编排 / 集群调度 | 沿用 v1 single-host 约束 |
| OOS-A17 | 容器预热 / 空闲回收策略 | 推迟到 v3.1（涉及控制面资源调度） |
| OOS-A18 | 性能 metrics 实时上报 admin 后台 | 推迟到 v3.1（依赖 v3.0 稳定后） |
| OOS-A19 | admin 后台新增 mount mode / session 数管理页 | v3.0 不做新页面，最多在 host 详情页加 `image_version` 一行 |
| OOS-A20 | 新增 "session 管理" REST endpoint | CLI 完全在 SSH 层解决多端协作，不需服务端介入 |

---

## Open Questions（plan-phase 必须 surface 决策）

研究阶段已识别但尚无唯一答案的事项；不要假装已经解决。每条标注涉及 phase，必须在该 phase 的 `discuss-phase` 或 `plan-phase` 中拍板。

| # | 议题 | 涉及 Phase | 候选方案 | 倾向 |
|---|------|-----------|----------|------|
| Q1 | Mutagen 客户端二进制如何分发 | P3 | (a) `go:embed` 整个二进制 / (b) 检测本机已装提示 brew install / (c) 首次运行自动下载 | 倾向 (a) |
| Q2 | Mutagen daemon 谁负责生命周期 | P3 | (a) cloud-claude 启动时 daemon start，退出不停（长期复用） / (b) 每次 session 起停同步 daemon | 倾向 (a)；需考虑多 cloud-claude 并发的 daemon 锁 |
| Q3 | Mutagen 同步默认 mode | P3 | `two-way-safe`（保守，冲突堆积需人工） vs `two-way-resolved`（本地优先自动覆盖远端） | 子研究矛盾，必须 P3 discuss 阶段定调 |
| Q4 | persistent volume 命名规范 | P2 / P5 | 单 volume `claude-state-{account_id}` vs 双 volume（creds + cache 分开） vs 命名前缀 `ccp_` | 倾向单 volume（运维更简单） |
| Q5 | Entry API 扩展字段 vs 新增 endpoint | P2 | (a) 在现有 `/v1/entry/{id}/auth` 响应里加字段（向后兼容） / (b) 新增 `/v1/entry/{id}/capabilities` | 倾向 (a) |
| Q6 | host-agent 是否扩展返回 image labels | P2 / P5 | 扩展（doctor 不必走 SSH） vs 不扩展（doctor 全走 SSH） | 倾向不扩展，保边界 |
| Q7 | 首次同步前是否做 alpha/beta 双向 diff 安全门 | P3 | 是（防 PITFALLS C5 反向清空） vs 否（信任 Mutagen safety mode） | 倾向是，但与 ≤ 8s 基线冲突需权衡 |
| Q8 | 多端 tmux session 命名是 per-user 还是 per-claude_account | P4 / P5 | per-user vs per-claude_account（与 volume 一致） | 倾向 per-claude_account |
| Q9 | doctor `--fix` 自动修复操作的幂等性边界 | P6 | 全幂等（重启 mutagen / remount sshfs / 清 known_hosts） vs 部分需二次确认（清 mutagen 残留 session） | 默认幂等；二次确认走 stdin `y/N`，CI `--yes` 跳过 |
| Q10 | mergerfs branch 是 2 路（hot + cold）还是 3 路（hot + cold + 本地覆盖） | P1 / P3 | STACK §2 推 2 路 / PITFALLS C2 暗示 3 路 | 倾向 2 路简化，P3 discuss 阶段确认 |

---

## Critical Pitfalls（每个 phase 必须显式防御，不计为 Out of Scope）

来自 `.planning/research/SUMMARY.md` §5 TOP 10。每条都必须在指定 phase 的 PLAN.md `tasks` 里有独立"防御任务"，并在 `verification` 段给出验证手段。本节列出供 roadmapper 与 planner 直接引用，非验收清单。

| # | Pitfall | 防御 Phase |
|---|---------|-----------|
| C1 | mergerfs 默认串行 readdir 拖慢 ls 90s+ | P1 + P3 + P7 |
| C2 | mergerfs 2.41 默认 `category.create=pfrd` 让新文件随机落 branch | P1 + P6 |
| C3 | sshfs 抖动级联 mergerfs 整体挂死 | P1 + P3 + P4 |
| C4 | Mutagen client/agent patch 版本差导致 handshake 失败 | P3 + P6 |
| C5 | non-root 用户 + Mutagen root 默认导致首次同步反向清空本地目录 | P1 + P3 + P5 |
| C6 | Ubuntu 25.04 AppArmor 默认禁止嵌套 FUSE | P1 + P6 |
| C7 | systemd-logind `KillUserProcesses=yes` 杀掉 tmux server | P1 + P4 |
| C8 | v3.0 新错误码与 v2.0 已有码命名空间冲突 | P6 + 各 phase |
| M13 | F2 静默降级到 sshfs-only，用户以为在 full 模式下跑 | P3 + P6 + P7 |
| M14 | doctor 仅输出 PASS/FAIL 不给修复命令 | P6 |

---

## Traceability

由 roadmap 阶段填充。每个 REQ-ID 必须映射到唯一一个 phase。

每个 REQ 主映射唯一一个 phase；F8 错误码体系是横切关注点（每个 phase 落码时遵循 Phase 34 定义的命名规范），主交付 phase 为 Phase 34。

| Requirement | Phase | Status | 说明 |
|-------------|-------|--------|------|
| REQ-F1-A | Phase 31 | Complete | 三层 mount 单一 `/workspace` 视图 |
| REQ-F1-B | Phase 31 | Complete | 首连 ≤ 8s 含三段式中文进度（最终验收对接 Phase 35 / BASE-02） |
| REQ-F1-C | Phase 31 | Complete | 10k 文件 1.5× 性能（最终验收对接 Phase 35 / BASE-01） |
| REQ-F1-D | Phase 31 | Complete | > 50MB 候选目录拒绝 + 自动降级 sshfs |
| REQ-F1-E | Phase 31 | Complete | Mutagen conflict 中文冒泡 |
| REQ-F2-A | Phase 31 | Complete | `--mount-mode` 四档切换 |
| REQ-F2-B | Phase 31 | Complete | 任一层失败 ≤ 2s 降级 + 禁止静默 |
| REQ-F2-C | Phase 31 | Complete | banner 彩色 mount 模式标签 |
| REQ-F3-A | Phase 32 | Pending | KeepAlive 15s/4 与服务端 15s/8 基线（服务端 sshd_config 在 Phase 29 落地） |
| REQ-F3-B | Phase 32 | Complete | 断网本地输入缓冲 + 灰色未确认样式 |
| REQ-F3-C | Phase 32 | Pending | 重连失败 prompt 原因 + 下一步 |
| REQ-F3-D | Phase 32 | Pending | 重连退避 + token 复用不弹密码 |
| REQ-F4-A | Phase 32 | Pending | tmux 默认包装 + 重连不丢进程 |
| REQ-F4-B | Phase 32 | Pending | `cloud-claude sessions ls/attach` |
| REQ-F4-C | Phase 32 | Pending | 容器内 tmux 不可用降级提示 |
| REQ-F5-A | Phase 32 | Pending | 多端默认共享 attach |
| REQ-F5-B | Phase 32 | Pending | 第二端 banner 中文显示来源 + 活跃时间 |
| REQ-F5-C | Phase 32 | Pending | `--new-session` / `--take-over` |
| REQ-F5-D | Phase 32 | Complete | 账号级 Mutagen 单例锁 |
| REQ-F6-A | Phase 34 | Complete | doctor 5 维度覆盖 |
| REQ-F6-B | Phase 34 | Complete | doctor 输出四要素 |
| REQ-F6-C | Phase 34 | Complete | doctor `--fix` 至少 5 类 |
| REQ-F6-D | Phase 34 | Complete | doctor `--verbose` / `--json` / 退出码 0/1/2 |
| REQ-F7-A | Phase 33 | Pending | named volume `claude-state-{id}` + label（数据模型在 Phase 30） |
| REQ-F7-B | Phase 33 | Pending | 容器重建 OAuth 保留 |
| REQ-F7-C | Phase 31 | Complete | 连接握手期 OAuth 过期中文提示 |
| REQ-F7-D | Phase 33 | Pending | admin DELETE 事务联动 `volume rm` |
| REQ-F8-A | Phase 34 | Complete | 错误码 `<DOMAIN>_<KIND>_<NUM>` 体系（横切：各 phase 落码时遵循） |
| REQ-F8-B | Phase 34 | Complete | 错误三要素（横切：各 phase 落码时遵循） |
| REQ-F8-C | Phase 34 | Complete | `cloud-claude explain <code>` |
| BASE-01 | Phase 35 | Pending | 元数据响应 1.5× 真机基准 |
| BASE-02 | Phase 35 | Pending | 首连 ≤ 8s 真机基准 |
| BASE-03 | Phase 35 | Pending | 弱网 30s 无感知 UAT |
| BASE-04 | Phase 29 | Pending | 镜像 ≤ 700MB CI gate（Phase 35 二次回归） |

**Coverage:**
- v3.0 requirements: 30 functional + 4 baselines = 34 total
- Open questions: 10（已分布到 Phase 29-34 的 `Open questions to resolve` 段）
- Out of scope: 20
- Mapped to phases: 34/34 ✓ — 30 functional REQ + 4 BASE 全部映射到唯一 phase；F8 错误码 / BASE-04 在多个 phase 横切引用，但主交付 phase 唯一

---

*Requirements defined: 2026-04-18*
*Source: `.planning/research/SUMMARY.md` §3 (REQ-IDs) + §6 (Open Questions) + §7 (Out of Scope)*
*Last updated: 2026-04-18 after milestone v3.0 initialization*
