# Cloud CLI Proxy

## What This Is

Cloud CLI Proxy 是一个面向单宿主机的容器化 SSH 云主机平台，既供自己使用，也面向出海团队和开发团队销售。用户从一个很短的 `curl` 入口开始，在终端里输入用户名和密码，等待专属 Docker "云主机"启动完成后，直接进入该容器内的 SSH 会话。

平台包含一个管理后台，用于管理用户、容器生命周期、出口 IP 分配和到期时间。每个容器都预装 `claude code`，并且所有网络流量都必须通过指定出口 IP 的全局隧道路由发送（通过 sing-box tun 全隧道模式），不能出现 DNS、WebRTC 或其他类型的直接泄漏。

## Core Value

给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP，同时保持"一条命令启动"的体验足够顺滑。

## Current Milestone

v3.5 ✅ shipped 2026-05-13 — 网络白名单与 DNS 拆分解析（tag v3.5）

## Next Milestone Goals

TBD — 运行 `/gsd-new-milestone` 进入下一个里程碑的需求收敛与路线图规划。

候选方向（参见 Backlog）：v3.5 P1 三大白名单预设（cn-dev / oss-dev / ai-api）+ 远程 rule-set 拉取 + 灰度发布；或 ENH-NEXT 系列（容器预热与空闲回收 / 性能 metrics 可视化 / mount 模式可观测 / 跨会话持久缓存 / 热同步 inotify 改造 等）。

## Current State

**Shipped:** v3.5 网络白名单与 DNS 拆分解析 (2026-05-13, tag v3.5)

v3.5 让管理员能在后台对每个 host 完成「勾预设 → 加规则 → 预览 diff → 一键 apply / rollback」的闭环，所有流量按白名单走宿主机直连或继续走 sing-box 代理出口，零 DNS 泄漏；10 条安全不变量（I1–I10）固化为 `scripts/uat-bypass.sh` + GitHub Actions fixture 自适应 preflight CI 守护。

**v3.5 ship 总览:**

- 3 phases (45-47), 10 plans
- 127 commits, 154 files (+30,861 / -459 lines)
- Codebase: Go 92,783 + TS/TSX 16,941 + Shell 13,900 ≈ 123,600+ LOC
- 2026-05-12 → 2026-05-13 (2 days)
- 34/34 REQ satisfied；7/7 跨 phase 集成接线 WIRED；3/3 phase VERIFICATION.md passed
- 5 项 tech-debt follow-up（TD-01 fixed commit 58c0a5f，TD-02..05 P2 不阻塞 ship）
- 10 项 Info-level deferred（Phase 45/46 REVIEW.md，不影响功能 / 安全 / 契约）

### v3.5 关键交付

sing-box 两段式静态配置 + 拆分 DNS（Phase 45）

- gateway `config.json` 改造为 `rule_set` 引用 + 7 条 route.rules（sniff → hijack-dns → ip_is_private → rule_set → final proxy-out）+ tun `strict_route`/`auto_route`/`endpoint_independent_nat`
- `dns.servers` 同时提供 `dns-local`（type=local）与 `dns-proxy`（type=https，DoH 1.1.1.1，domain_resolver=dns-local）；`.lan`/`.local`/`.internal` 后缀走 dns-local，公网白名单走 DoH
- 容器 `/etc/resolv.conf` 与 `/etc/nsswitch.conf` `:ro` bind mount（唯一 nameserver 172.19.0.1 + `ndots:0 single-request-reopen` + `hosts: files dns`）
- `ContainerProxyProvider` 拆分为 `PrepareGateway` + `PrepareHost`，消除 entrypoint 启动时 tun0 未监听的竞争

白名单数据模型 + Repository（Phase 45）

- migration `0019_host_bypass_rules.sql` 五张表（presets / rules / bindings / snapshots / audit_log）+ `0020_host_bypass_snapshot_source.sql` source 列
- 19 个 Bypass* CRUD 方法 + 21 SQL 常量 + `ErrSystemBypassPresetImmutable` sentinel + Go/SQL 双层防御
- seed: `loopback`（is_system + is_force_on，不可关闭）+ `lan`（默认关闭）

控制面 API + 6 护栏 + 双轨审计（Phase 46）

- 18 条 Admin Bypass 路由（preset / rule / binding / validate / preview / apply / rollback / effective / audit-log / consistency）
- 5 硬护栏（12 个 BYPASS_* 错误码）+ 1 软护栏（`domain_keyword < 4` 要求 `confirm_risky:true`）
- 所有写操作走 `writeBypassAuditLog` 双轨（DB audit_log 行 + `EventRecorder.RecordEvent("bypass.<action>")` 事件流）
- `sha256(cidrsJSON+"\n"+domainsJSON)` 幂等键；rollback 新建 source='rollback' pending 行

React 后台 Bypass Tab（Phase 46）

- 11 个 `.tsx` 组件 + 6 个 vitest 测试 + 4 个 hooks + 8 条中文错误码 i18n（34 case PASS）
- PreviewSheet（sing-box JSON / nft set diff 双 Tab + unified diff 颜色 token）
- ApplyProgressDialog（5 阶段固定中文：生成快照 / 下发到 agent / Reload 配置 / 健康检查 / 完成，task.progress_percent 4 档映射）
- RollbackConfirmDialog（host slug 严格输入二次确认）

Agent 热更新链路 + fail-closed 加固（Phase 47）

- worker dispatcher 真实接管 `ActionReloadHostBypass`：`nft -f` 事务更新 `@whitelist_v4` set → `tmpfile + rename` 原子写 rule-set 文件 → 等 1s sing-box 文件 watch reload → 健康检查 3 次失败自动 rollback 上一 applied snapshot
- `Consistency` endpoint：3s timeout（504 BYPASS_CONSISTENCY_TIMEOUT vs. 500 BYPASS_CONSISTENCY_ERROR）+ 归一化 sha256 对账
- netns nft 四层防御：`oifname sb-tun0` accept + uid=singbox 直连代理 IP:443 + `oifname eth0 ip daddr @whitelist_v4` 逃逸通道 + 链末 `counter log prefix "sbfw-drop " drop`
- 显式 drop mDNS(5353) / LLMNR(5355) / NetBIOS(137)；`--sysctl disable_ipv6=1` + IPv6 family policy=drop 双保险

E2E 验证（Phase 47）

- `verify.go` 新增 3 项流量检查（`verifyBypassEgressMatchesEth0` / `verifyNonBypassTraffic` / `verifyPublicDNSBlocked`）+ `nsenterRunner` 包级 var 让单测旁路 exec
- `scripts/uat-bypass.sh` 6 场景 × 10 不变量（I1–I10）
- `.github/workflows/uat-bypass.yml` fixture 自适应 preflight + `scripts/uat-bypass-fixture-up.sh` 三件套（TD-01 fixed），fixture 一旦合入 PR 即触发完整 6 场景 × ubuntu-24.04 × I1–I10 CI 守护

<details>
<summary>📦 v3.4 多形态容器接入 (已归档)</summary>

**Shipped:** v3.4 多形态容器接入 (2026-05-08, tag v3.4.0)

v3.4 扩展容器接入方式：Cloud 版 SSH Proxy 支持 full port forwarding（VS Code Remote-SSH），本地版 `cloud-claude local` 支持 Dev Containers 工作流，doctor 新增 remote-ssh 维度和 sshd_config 主动验证。

**v3.4 ship 总览:**
- 7 phases (38-44, 含 3 gap closure), 14 plans
- 61 commits, 84 files (+12,724 / -968 lines)
- Codebase: Go 40,702 + Shell 2,594 ≈ 43,300+ LOC
- 2026-05-07 → 2026-05-08 (2 days)
- 13/13 REQ satisfied；13/13 集成接线 WIRED；3/3 E2E 流程 complete
- 11 项人工验证 deferred-to-ship（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）

### v3.4 关键交付

SSH Proxy 端口转发（Phase 38）
- direct-tcpip channel dispatch + 管理网段/Docker socket/metadata 安全拦截
- tcpip-forward/forwarded-tcpip 全局请求透传 + 共享 targetClient 预 dial 架构
- sshd_config 验证（AllowTcpForwarding + AllowStreamLocalForwarding + GatewayPorts）

本地 Dev Containers（Phase 39）
- `cloud-claude local up/down/status` cobra 子命令组 + `internal/local` 包
- entrypoint `MODE=local` 跳过 KasmVNC + 心跳，保留 sshd + sing-box
- `--egress-config` 注入 sing-box outbound JSON（tun/proxy 双模式）
- `.devcontainer/devcontainer.json` 模板 + SSH key 注入

Doctor remote-ssh 维度（Phase 41 + Phase 44）
- 5 项检查：VS Code Server 进程 / vscode-server 磁盘 / forwarding channel / sshd / sing-box
- 6 个新错误码 + 4 条 explain 长说明 + 20 个单元测试
- parseSSHDForwarding + checkSSHDForwarding + 3 个 sshd 错误码 + 13 个单元测试

Gap closure 链（Phase 42/43/44）
- 审计发现 3 个关键缺口 → 三阶段全关闭，13/13 需求 satisfied

</details>

## Backlog（待后续里程碑收敛）

- **BYPASS-PRESET-CN-DEV** v3.5 P1 — `cn-dev` 白名单预设（阿里云/腾讯云 metadata、mirrors.aliyun.com、registry.npmmirror.com、goproxy.cn 等）
- **BYPASS-PRESET-OSS-DEV** v3.5 P1 — `oss-dev` 预设（github.com、registry.npmjs.org、pypi.org、registry-1.docker.io、proxy.golang.org 等）
- **BYPASS-PRESET-AI-API** v3.5 P1 — `ai-api` 预设（api.anthropic.com、api.openai.com 等）
- **BYPASS-RULESET-REMOTE** v3.5 P1 — 远程 rule-set 拉取 worker（MetaCubeX/meta-rules-dat + 自维护镜像 fallback）
- **BYPASS-CANARY** v3.5 P1 — 「先在测试 host 验证」按钮，灰度下发
- **BYPASS-USER-SELF** v3.5 P1 — 用户自助配置白名单（区分管理员 / 用户角色权限）
- **BYPASS-HIT-STATS** v3.5 P1 — 命中统计（轮询 sing-box Clash API `/connections`）
- **BYPASS-DASHBOARD** v3.5 P1 — 流量 dashboard（bypass / proxy 流量字节占比）
- **v3.5 tech-debt** — TD-02 I9 严格化、TD-03 detectHostEth0IPFallback 真实化、TD-04 I3 切 nft counter、TD-05 verify.go Linux runner 集成测试（均 P2，详见 `milestones/v3.5-ROADMAP.md` Tech Debt 表）

- **ENH-NEXT-01** 容器预热与空闲回收策略（控制面资源调度）
- **✓ SSE 实时推送基础设施** — 控制面 topic-based pub/sub + `/v1/admin/sse` + `/v1/user/sse`，前端无需轮询（v3.4 已完成）
- **ENH-NEXT-02** 性能 metrics 数据上报与可视化（首连耗时、mount 模式、抖动事件分布 → 接入 SSE 通道）
- **ENH-NEXT-03** admin 后台 host 详情页展示 mount 模式 / session 数 / persistent volume 列表
- **ENH-NEXT-04** 自研 hot-sync spec doc 修订（v3.0 隐式设计变更）
- **ENH-NEXT-05** `~/.vscode-server` 持久化 volume（容器重建后保留扩展和设置）
- **ENH-NEXT-06** VS Code 多窗口/多工作区支持
- **ENH-NEXT-07** `cloud-claude local --sync-config` 从云端拉取 egress IP 配置
- **ENH-NEXT-08** 本地容器预热镜像（减少首次启动时间）
- **ENH-NEXT-09** doctor 本地模式适配（跳过 auth/egress 检查）
- 3 项真机签字 ship 闸门（M5 APFS / BASE-03 2min / C6 Ubuntu 25.04）— v3.0/v3.1 deferred-to-ship
- 11 项人工验证（Phase 38 x3 / Phase 39 x5 / Phase 43 x3）— v3.4 deferred-to-ship
- v3.0/v3.1 tech debt 批次清理（≈14 项 WR/HR/MR 系列 + spec/code 数字漂移）
- 跨会话持久缓存（hot 分支退出后保留，第二次 mount 直接命中）
- 热同步改 inotify/fsevents 替代秒级轮询
- rename / move 检测优化
- Windows 客户端支持（单独立项）

<details>
<summary>📦 v3.0 Milestone Goals (已归档)</summary>

**Goal:** 把 cloud-claude 从"能跑起来"升级到"长期可依赖的远程开发日常工具"——彻底解决 sshfs 性能天花板、网络抖动会话丢失、多端使用冲突等 v2.0 实际使用中暴露的核心体验问题。

**Target features:**

主线 A · 文件映射架构重构
- F1：三层文件系统架构（Mutagen 热同步源码白名单 + sshfs 冷兜底全量懒拉 + mergerfs 合并成单一 `/workspace` 视图），替换纯 sshfs 方案 — ✓ Phase 31
- F2：降级路径与手动切换（`--mount-mode=auto|full|mutagen-only|sshfs-only`，三层任一失败时 CLI 自动降级并明确告知用户当前模式） — ✓ Phase 31

主线 B · 会话可靠性
- F3：SSH 会话稳定性增强（长心跳、弱网容忍、网络恢复后自动重连提示） — ✓ Phase 32
- F4：会话恢复（容器内默认用 tmux/dtach 包一层，断网后回到原 shell 不丢失运行中的 claude 进程） — ✓ Phase 32
- F5：多端同时连接（同账号 mac+linux 默认 attach 同一 session 协作观察，`--new-session` 开关独占新 session，冲突时中文提示） — ✓ Phase 32

主线 C · 运维与体验配套
- F6：`cloud-claude doctor` 全面升级（network/auth/ssh/mount 三层/disk 五维度自检 + 一键修复） — ✓ Phase 34
- F7：Claude Code 状态持久化（`~/.claude/`、`~/.cache/claude` 通过独立 volume 持久化，以 claude_account 为粒度，容器重建不丢登录） — ✓ Phase 33
- F8：错误码与中文提示统一升级（新架构所有错误路径统一纳入 v2.0 错误码体系，每条带"下一步该做什么"） — ✓ Phase 34

**性能/体验验收基线（达成情况）：**
- `rg .` / `ls -R` 在 10k 文件源码树响应 ≤ 本地 1.5× — ✓ Phase 35 自动化 PASS（BASE-01）
- 首次连接到能交互时间 ≤ 8s — ⚠️ Phase 35 自动化 PASS / 真机签字 deferred-to-ship（BASE-02）
- 30s 内网络抖动对会话无感知 — ✓ Phase 35 自动化 PASS（BASE-03）；2min 真机签字 deferred-to-ship

**Out of scope（推迟到 v3.1 或更后）：**
- 容器预热与空闲回收策略（涉及控制面资源调度，单独 milestone）
- 性能 metrics 实时上报到 admin 后台（依赖 v3.0 稳定后再做）

</details>

## Context

**Latest shipped:** v3.5 网络白名单与 DNS 拆分解析 (2026-05-13, tag v3.5)
**Codebase:** ~123,600+ LOC (Go 92,783 + TS/TSX 16,941 + Shell 13,900)
**Tech stack:** Go 1.26.1 + PostgreSQL 18 + Docker 28 + sing-box 1.13.3 + nftables（`google/nftables` + `nft -f` 事务）+ React 19.2 + Vite 8 + cobra + sshfs/SFTP + hot-sync (ssh+tar) + mergerfs 2.41.1 + tmux 3.6a + inotify + vitest 4 + Radix UI

v1.0 MVP + v1.1 代理协议出网 已交付，涵盖：
- 单宿主机控制面 + Unix socket host-agent + 受管用户镜像
- sing-box tun 全隧道出网 + nftables 默认拒绝 + 三重网络校验
- `curl → 认证 → 启动提示 → SSH` 一条命令接入流
- JWT 管理后台 (React SPA) + 用户/出口 IP/绑定/主机生命周期 CRUD
- 5 种代理协议（SOCKS5/vmess/shadowsocks/trojan/HTTP）+ 一键测试
- 到期治理、事件记录、运行时对账、部署文档

v1.2 部分交付（Phase 11-12），新增用户认证体系和自助面板骨架，Phase 13-16 延后。

v2.0 已交付：
- Go 单一二进制 `cloud-claude`，cobra 入口 + init 配置管理 + Entry API 认证轮询
- sshfs slave + 嵌入式 SFTP server 实现本地目录到容器 /workspace 实时双向映射
- shellescape 安全命令构建 + 非 TTY 管道模式 + claude 参数原样透传
- TTY/信号/窗口大小/退出码完全透传
- 受管镜像预装 sshfs/fuse3 + AppArmor unconfined + 宿主机 FUSE 前置检查
- 238 行 FUSE 兼容性验证脚本 + 中英文部署文档补充

v3.0 已交付，新增：
- 三层文件系统：Mutagen 热同步白名单（≤50MB + ignore） + sshfs 冷兜底 + mergerfs 单一 /workspace 视图
- `--mount-mode=auto|full|mutagen-only|sshfs-only` 四档降级状态机（≤2s 降级 + 禁止静默降级 + last-session.json downgrade_chain 留痕 + banner 彩色标签）
- SSH 弱网容忍：KeepAlive 15s/4 强制 + Reconnector 退避 1/2/4/8/30s + token 复用 + BufferedStdin 灰色未确认本地 echo + ringBuf 按序回放
- tmux 默认包装 (`exec tmux new-session -A -s claude-<account_id>`) + 多端共享 attach + `cloud-claude sessions ls/attach` + `--new-session`/`--take-over` + 账号级 Mutagen 单例锁
- Claude Code OAuth 持久化：单 Docker named volume `claude-state-{claude_account_id}` + label + entrypoint symlink + chown 1000:1000 兜底；admin DELETE 联动 `volume rm` 双路径（强一致 10s + force 30s）
- `cloud-claude doctor` 5 维度（network/auth/ssh/mount/disk）18 项 check + 6 类自动 fix + JSON schema_v1 + 退出码 0/1/2 + 第一屏降级历史 banner + ci-doctor-grep.sh M14 闸门
- 错误码统一 `<DOMAIN>_<KIND>_<NUM>` 4 段（42 条 8 域 Registry + 38 条 ≥200 字符长说明 + `cloud-claude explain <code>` rustc 风格）
- 受管镜像 v3.0.0：mergerfs 2.41.1 + mutagen-agent v0.18.1 + tmux 3.6a + libfuse3 3.18.x + image.lock + CI ≤ 700MB gate
- 运维手册 5 章：升级 / AppArmor / doctor 排障 / 持久卷 / 错误码索引
- 性能基线：BASE-01 (10k 文件 1.5×) / BASE-03 (30s 抖动无感) / BASE-04 (镜像 700MB gate) 自动化 PASS；BASE-02 + 3 项真机签字 deferred-to-ship

## Requirements

### Validated

- ✓ 每个运行中的容器都必须绑定至少一个出口 IP，并且所有出站流量都必须强制走该路径，不能出现 DNS、WebRTC 等流量绕行或泄漏 — v1.0
- ✓ 用户可以执行一条简短的 `curl` 命令，在终端中完成认证，等待容器启动，并无须手工配置主机信息就进入 SSH 会话 — v1.0
- ✓ 管理员可以在单宿主机环境下管理用户、登录凭证、到期时间和容器生命周期 — v1.0
- ✓ 凭证错误、账号过期、未绑定出口 IP 或启动失败时返回清晰的终端错误提示 — v1.0
- ✓ 管理员操作和启动结果被记录为运维事件，并可在事件日志页面查看 — v1.0
- ✓ 已过期用户无法开启新会话，运行中主机按策略停止 — v1.0
- ✓ 出口 IP 类型化，支持代理隧道 — v1.1
- ✓ SingBoxProvider tun 模式全流量代理实现 — v1.1
- ✓ 受管镜像预装 sing-box 二进制 — v1.1
- ✓ Provider 统一使用 sing-box 处理全隧道出网 — v1.1
- ✓ 前端出口 IP 表单按隧道类型动态切换字段 — v1.1
- ✓ 后台一键代理测试 API 及前端展示 — v1.1
- ✓ Go 单一二进制 cloud-claude，用户可 alias claude=cloud-claude 透明替代原生 claude 命令 — v2.0
- ✓ cloud-claude init 配置网关地址、用户凭证，持久化到 ~/.cloud-claude/config.yaml — v2.0
- ✓ 执行时自动获取当前目录，sshfs slave 将当前目录实时映射到容器 /workspace — v2.0
- ✓ 在容器内启动 Claude Code，所有参数原样透传（shellescape 防注入）— v2.0
- ✓ TTY/信号/窗口大小/退出码完全透传，用户体验与本地 claude 无差异 — v2.0
- ✓ 容器镜像预装 sshfs + FUSE，创建时带 --device /dev/fuse + --cap-add SYS_ADMIN — v2.0
- ✓ 支持私有部署：用户可配置自有网关地址 — v2.0
- ✓ 生产环境 FUSE + AppArmor 兼容性验证通过 — v2.0
- ✓ cloud-claude 用 Mutagen 热同步源码白名单（≤50MB、按扩展名/路径过滤），替换原纯 sshfs 方案 — v3.0 (Phase 31)
- ✓ cloud-claude 同时挂载 sshfs 冷兜底，覆盖未同步文件（懒读、按需走网络）— v3.0 (Phase 31)
- ✓ 容器内用 mergerfs 把热/冷分支合并成单一 `/workspace`，对用户和 Claude Code 透明 — v3.0 (Phase 29 镜像 + Phase 31 CLI)
- ✓ CLI 提供 `--mount-mode=auto|full|mutagen-only|sshfs-only` 切换；三层任一失败时自动降级到下一档并明确告知 — v3.0 (Phase 31)
- ✓ SSH 长心跳与弱网容忍（KeepAlive 15s/4 强制下限）— v3.0 (Phase 32)
- ✓ 网络恢复后 cloud-claude 自动提示并支持重连，无需重新认证（退避 1/2/4/8/30s + token 复用）— v3.0 (Phase 32)
- ✓ 容器内 SSH 会话默认包一层 tmux（`tmux new-session -A`），断网回到原 shell 时运行中的 claude 进程不丢失 — v3.0 (Phase 32)
- ✓ 同账号多端连接默认 attach 同一 session，`--new-session` 独占新 session，`--take-over` 强制独占 — v3.0 (Phase 32)
- ✓ 多端 attach 时第二端 banner 中文显示其它会话来源 + 活跃时间 — v3.0 (Phase 32)
- ✓ 账号级 Mutagen 单例锁（同账号同时只允许 1 个 sync session，后连端只读 sshfs/mergerfs 视图）— v3.0 (Phase 32)
- ✓ `cloud-claude doctor` 覆盖 network/auth/ssh/mount（mutagen+sshfs+mergerfs 三层）/disk 五个维度 — v3.0 (Phase 34)
- ✓ `doctor` 每项检查支持一键修复或给出明确下一步指引（6 Fixer + 四要素 + M14 CI grep 闸门）— v3.0 (Phase 34)
- ✓ `doctor` 支持 `--verbose` / `--json` / `NO_COLOR`，退出码 0/1/2 与 brew doctor 对齐 — v3.0 (Phase 34)
- ✓ Claude Code 登录态/缓存目录通过独立 Docker volume 持久化（`claude-state-{account_id}` + label）— v3.0 (Phase 33)
- ✓ 持久化 volume 以 claude_account 为粒度，容器重建后用户无需重新登录 Claude — v3.0 (Phase 33)
- ✓ admin API 删除 claude_account 时事务性联动删除对应 Docker named volume — v3.0 (Phase 33)
- ✓ credentials 即将过期或已过期时连接前给出明确中文提示，不让 claude 进程进入报错 — v3.0 (Phase 31)
- ✓ 新架构所有错误路径统一纳入 `<DOMAIN>_<KIND>_<NUM>` 4 段错误码体系，每条带中文原因 + 中文 next_action — v3.0 (Phase 34)
- ✓ `cloud-claude explain <code>` 子命令对每个错误码给详细中文说明（rustc 风格）— v3.0 (Phase 34)
- ✓ 非 git 仓库拒绝挂载（`MOUNT_REQUIRE_GIT_REPO` + 中文 next_action + 退出码恒定）— v3.1 (Phase 36)
- ✓ 单文件 50MB 熔断（`hot_sync_max_file_mb` 可配置），超阈走 cold sshfs 兜底 — v3.1 (Phase 36)
- ✓ sshfs FUSE page cache 默认开启（`cache=yes,kernel_cache,auto_cache,cache_timeout=300`），同会话重复读 0 RTT — v3.1 (Phase 36)
- ✓ `doctor mount` 从 4 项扩展到 9 项 check（+git 仓库 / 大文件熔断 / sshfs 缓存 / git proxy / ignore 加载）— v3.1 (Phase 36)
- ✓ 错误码注册表新增 `MOUNT_REQUIRE_GIT_REPO` / `MOUNT_OVERSIZED_FILE_SKIPPED`，附 ≥200 字 explain 长说明 — v3.1 (Phase 36)
- ✓ 容器内 `cold-promoter` inotify watcher（`IN_OPEN/IN_ACCESS`），LIFO cleanup 回收 — v3.1 (Phase 37)
- ✓ 异步 SFTP 晋升到 hot 分支，5s 防抖 + 1/2/4s 退避 + 3 次熔断 — v3.1 (Phase 37)
- ✓ mergerfs 自然读 hot 优先；`last-session.json` 新增 promotion 统计字段 — v3.1 (Phase 37)
- ✓ `CLOUD_CLAUDE_NO_PROMOTION=1` 关闭晋升；用户主动读不二次过滤 ignore — v3.1 (Phase 37)
- ✓ `doctor mount` 新增 4 项晋升可观测指标（promoter_alive / queue_depth / total / failed）— v3.1 (Phase 37)
- ✓ runbook `docs/runbooks/v31-cold-promotion.md` Pattern G 5 章手册 — v3.1 (Phase 37)
- ✓ e2e UAT 脚本 `tests/scripts/uat-v31-promotion.sh` 6 场景全覆盖 + CI 接入 — v3.1 (Phase 37)
- ✓ SSH Proxy 支持 `direct-tcpip` channel 转发 + 管理网段/Docker socket/metadata 安全拦截 — v3.4 (Phase 38)
- ✓ SSH Proxy 支持 `tcpip-forward` + `forwarded-tcpip` 全局请求透传 — v3.4 (Phase 38)
- ✓ 容器内 sshd_config 显式开启端口转发（AllowTcpForwarding + AllowStreamLocalForwarding + GatewayPorts）— v3.4 (Phase 38 + Phase 44 doctor)
- ✓ `cloud-claude local up/down/status` 子命令，不依赖 control-plane 直接启动本地容器 — v3.4 (Phase 39)
- ✓ 本地容器支持 `.devcontainer/devcontainer.json` + VS Code Dev Containers 工作流 — v3.4 (Phase 39)
- ✓ 本地容器支持 `--egress-config` 注入 sing-box tun/proxy 双模式出网 — v3.4 (Phase 39)
- ✓ entrypoint `MODE=local` 跳过 KasmVNC + 心跳，保留 sshd + sing-box — v3.4 (Phase 39)
- ✓ VS Code Remote-SSH E2E 验证：端口转发 + 出口 IP 强约束 + 下载流量走 sing-box — v3.4 (Phase 40 + Phase 43)
- ✓ `cloud-claude doctor` 新增 remote-ssh 维度（5 项检查 + 6 个错误码 + 20 单测）— v3.4 (Phase 41)
- ✓ doctor sshd_config 主动验证（parseSSHDForwarding + 3 个错误码 + 13 单测）— v3.4 (Phase 44)
- ✓ sing-box 静态配置改造为两段式：静态 config.json + 动态 local rule-set 文件 watch — v3.5 (Phase 45)
- ✓ 拆分 DNS 模型：内网 `.lan/.local/.internal` 走 `dns-local`，公网白名单走代理 DoH 1.1.1.1（保护查询隐私）— v3.5 (Phase 45)
- ✓ 容器 `/etc/resolv.conf` + `/etc/nsswitch.conf` `:ro` bind mount，唯一 nameserver 172.19.0.1 + `hosts: files dns` — v3.5 (Phase 45)
- ✓ 白名单数据模型五张表 + 19 个 Repository CRUD + `ErrSystemBypassPresetImmutable` 双层防御 — v3.5 (Phase 45)
- ✓ 系统内置预设：`loopback`（is_system + is_force_on，强制开启）+ `lan`（默认关闭）— v3.5 (Phase 45)
- ✓ 管理员可在后台对每个 host CRUD 白名单规则（IP/CIDR/域名/域名后缀/端口五类型）— v3.5 (Phase 46)
- ✓ 管理员可在后台勾选预设组合并 PreviewSheet 双 Tab 预览 sing-box JSON 与 nft set diff — v3.5 (Phase 46)
- ✓ 6 护栏（5 硬 1 软）+ 12 个 BYPASS_* 错误码 + 双轨审计（DB audit_log + EventRecorder）— v3.5 (Phase 46)
- ✓ Agent 热更新链路：nft -f 事务 + tmpfile+rename 原子写 rule-set + sing-box 文件 watch reload + 健康检查 — v3.5 (Phase 47)
- ✓ 配置下发健康检查 3 次失败自动 rollback 上一 applied snapshot + 事件日志告警 — v3.5 (Phase 47)
- ✓ fail-closed 加固：netns nft policy=drop + `oifname sb-tun0` + uid=singbox 锁定 + `@whitelist_v4` 逃逸通道 + 链末 log drop + IPv6 双保险 — v3.5 (Phase 47)
- ✓ 显式 drop mDNS(5353) / LLMNR(5355) / NetBIOS(137) 出向流量 — v3.5 (Phase 47)
- ✓ E2E 流量验证：verify.go 3 项新检查（白名单 eth0 / 非白名单走代理 / `dig @8.8.8.8` 必超时）+ 10 条安全不变量（I1–I10）CI 守护 — v3.5 (Phase 47)
- ✓ `scripts/uat-bypass.sh` 6 场景 + `.github/workflows/uat-bypass.yml` fixture 自适应 preflight + `uat-bypass-fixture-up.sh` 三件套 — v3.5 (Phase 47)
- ✓ `GET /v1/admin/hosts/{hostID}/bypass/consistency` 归一化 sha256 对账（3s timeout，504/500 区分）— v3.5 (Phase 47)

### Active

无（v3.5 已 shipped）。下一里程碑目标由 `/gsd-new-milestone` 进入定义。

### Archived (v1.3 claude-shell) — 能力并入 v3.4

v1.3 的"本地容器"和"指纹伪装"能力与 v3.4 的 `cloud-claude local` 目标重叠。为避免维护两套本地 Docker 编排逻辑，v1.3 能力拆分并入 v3.4 Phase 39：

- Phase 17 镜像基线 → 已并入 managed-user 镜像（v3.0+）
- Phase 18 网络隔离 → 并入 v3.4 Phase 39（本地版 sing-box sidecar + veth 注入）
- Phase 19 CLI 骨架 → 并入 v3.4 Phase 39（`cloud-claude local` 子命令）
- Phase 20 TTY 透传 → 不再需要（本地 publish 端口，SSH 用系统 OpenSSH）
- Phase 21 指纹伪造 → entrypoint `MODE=local` 深度伪装分支（快速任务级别）
- Phase 22 验证自检 → `cloud-claude local verify` / `doctor --local`
- Phase 23 混淆构建 → Makefile 构建脚本，不单独成里程碑

### Deferred from v1.2

- [ ] Bootstrap 流程改为 `curl domain/{short_id}`，展示欢迎艺术字，交互输入密码，实时状态推送，自动 SSH 接入
- [x] 用户自助面板：同一 React 应用根据角色展示不同视图，用户可查看自己的主机、重建主机、查看出口 IP — Phase 12 (2026-03-29, pending human verification)
- [ ] 用户可在自助面板查看管理员绑定的 Claude 账号信息
- [ ] 用户可在自助面板直接访问 KasmVNC 远程桌面
- [ ] 数据模型支持一个用户拥有多个 Claude 账号，每个账号对应一台独立主机
- [ ] 管理员可管理 Claude 账号（CRUD）及其与用户/主机的绑定关系
- [x] 用户登录认证体系（区别于管理员 JWT），用户只能访问自己的资源 — Phase 11+12 (2026-03-29)

### Out of Scope

- 计费、套餐、余额和自助支付流程：在核心主机生命周期和网络强约束能力验证前，不纳入 v1。
- 多宿主机编排和集群调度：v1 明确限制为单宿主机，以降低复杂度并加快落地。
- Web Terminal 和浏览器远程桌面：v1 只做 SSH 访问体验。
- 用户自定义任意镜像：会削弱就绪性、安全性和可支持性。
- 用户自选代理节点：由管理员统一配置，避免安全和支持风险。
- 代理链/多跳：延迟增加、排查困难，单跳足够。
- 实时流量监控：开发量大，先做连通性测试。
- 用户申请交接账号：流程未设计清楚，v1.2 暂不做。

## Product Context

- v1.0 + v1.1 + v2.0 已交付，首批目标用户是项目拥有者本人，随后扩展到需要受控出口 IP 工作环境的出海团队和开发团队。
- 两条产品路径已形成：Web SSH 接入（curl 入口）和本地 CLI 透明替代（cloud-claude）。
- 容器虽然基于 Docker，但对用户来说应当像一台"可管理、可复用、可回收"的云主机。
- `claude code` 已在镜像中预装，用户可通过 SSH 或 cloud-claude 直接使用。
- 网络模型已实现 sing-box tun 全隧道模式，配合 nftables 默认拒绝 + 三重校验门禁。
- 出口 IP 支持 5 种代理协议（SOCKS5/vmess/shadowsocks/trojan/HTTP），管理后台提供一键测试。
- cloud-claude 通过 sshfs slave 实现本地目录实时双向映射，体验与本地 claude 一致。
- 产品优先级是优雅、好用、运维清晰，而不是功能数量最多。

## Constraints

- **部署方式**：v1 仅支持单台 Linux 宿主机，先把可用性和运维复杂度收住。
- **访问模型**：v1 只做 SSH 会话接入，不分散到多种远程交互形态。
- **运行时**：每个用户环境都由 Docker 容器承载，容器创建、启动和接入是产品主线。
- **网络安全**：必须通过虚拟网卡 / tun 风格的全局隧道路由实现全流量强制出网，不能允许直连外网。
- **IP 分配**：每个容器都必须至少绑定一个出口 IP，没有绑定就视为非法状态。
- **产品范围**：v1 只做后台管理、生命周期和到期治理，不做计费和商业化流程。
- **沟通语言**：所有助手面对用户的回复、计划、状态更新和总结，默认必须全部使用中文；除非用户明确要求，否则不要改回英文。

## Key Decisions

| 决策 | 原因 | 结果 |
|------|------|------|
| 先从单宿主机部署开始 | 最快拿到可用产品、同时控制运维复杂度 | ✓ Good — v1.0 已验证 |
| v1 只提供 SSH 访问方式 | 最符合目标体验，减少远程接入面复杂度 | ✓ Good — bootstrap 脚本 + exec ssh 体验顺畅 |
| 使用短 `curl` 入口完成认证和启动 | 低摩擦、易传播，符合产品定位 | ✓ Good — 7 个错误码 + 中文提示完整 |
| 在镜像中预装 `claude code` | 用户进入环境后立即可用 | ✓ Good — image.lock 模板已实现 |
| 强制要求出口 IP 绑定和全隧道路由 | 出口可控不是附加功能，而是产品承诺核心 | ✓ Good — sing-box tun 全隧道 + nftables + 三重校验 |
| 延后计费和多节点调度 | 保持 MVP 聚焦在主机交付和网络正确性 | ✓ Good — v1.0 + v1.1 按时交付 |
| 控制面通过 Unix socket 驱动 host-agent | 避免在 HTTP 层直接持有 Docker/网络特权 | ✓ Good — 清晰的特权边界 |
| 容器使用 --network=none 创建 | 彻底隔离 Docker 默认网络，防止旁路 | ✓ Good — 无绕过可能 |
| 容器网络命名空间隔离 | 隧道配置不经过宿主机网络栈 | ✓ Good — 安全性更强 |
| bcrypt 密码 + JWT 管理后台 | 标准安全实践，简单可靠 | ✓ Good — 测试覆盖完整 |
| sing-box tun 全隧道模式 | 支持多种代理协议，扩展出口 IP 灵活性 | ✓ Good — 6 种协议支持 |
| 代理配置以 sing-box outbound JSON 存储 | 灵活且面向未来，不为每种协议建列 | ✓ Good — JSONB 列 + 白名单校验 |
| RoutingProvider 统一委托给 SingBoxProvider | 单一 Provider 接口 | ✓ Good — 简洁可维护 |
| 宿主机 masquerade 用 iptables 而非 nftables | 避免与 Docker Engine iptables 规则冲突 | ✓ Good — 幂等且安全 |
| SYS_ADMIN + /dev/fuse 统一附加，不做条件区分 | FUSE mount 需要 SYS_ADMIN，统一避免条件分支 | ✓ Good — 简化运维 |
| Entry API 为 cloud-claude 唯一认证契约 | 复用控制面现有实现，不新增专用 API | ✓ Good — 零服务端改造 |
| sshfs slave + 嵌入式 SFTP server 实现目录映射 | 复用 SSH 连接，无需额外端口或协议 | ✓ Good — 架构简洁 |
| shellescape.QuoteCommand 构建远程命令行 | POSIX 单引号规则成熟，防止 shell 注入 | ✓ Good — 安全可靠 |
| apparmor=unconfined 而非自定义 AppArmor profile | 安全边界已由 nftables + namespace 覆盖 | ✓ Good — 降低运维复杂度 |
| 退出码通过返回值上浮，禁止 os.Exit | 保证 defer term.Restore 始终执行 | ✓ Good — 修复终端恢复 |
| doctor 纯本地 + SSH 实现，不给 host-agent 加 endpoint | 保持特权边界清晰，避免 control/data plane 混淆 | ✓ Good — Phase 34 SC#9 守恒（`rg "agentapi\." internal/cloudclaude/doctor/` 命中 0） |
| 错误码命名统一 `<DOMAIN>_<KIND>_<NUM>` 4 段，正则 `^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$` | v2.0 原 3 段正则与实际使用的 4 段 code 冲突；放宽而非裁剪 code | ✓ Good — Phase 31 起沿用，Phase 34 收口 42 条 8 域无重复 |
| `cloud-claude explain <code>` 对标 `rustc --explain` | 为非研发用户（运维、客服）提供错误码反查能力，不需要读源码 | ✓ Good — Phase 34 落 38 条 ≥200 字符长说明，exit 0/4 两态 |
| 三层文件系统 Mutagen + sshfs + mergerfs（替代 v2.0 纯 sshfs） | sshfs 在 mono-repo 上 ls -R 需要 90s+；Mutagen 热同步源码（≤50MB 白名单）+ sshfs 冷兜底 + mergerfs 单一视图，对用户透明 | ✓ Good — v3.0 Phase 31 落地，BASE-01 自动化 PASS |
| Mutagen 二进制 go:embed 集成（Q1 (a)） | 用户体验"一条命令" — 不依赖本机 brew install / 首次下载；4 平台 v0.18.1 二进制 ~49MB 嵌入 cloud-claude | ✓ Good — Phase 31，CI build-images workflow 拉取真实文件 |
| Mutagen daemon 长期复用（Q2 (a)）+ 账号级 flock 单例锁 | 同账号多端连接不重复创建 sync session；后连端只读 sshfs/mergerfs 视图，避免双向同步冲突 | ✓ Good — Phase 32 实现，REQ-F5-D 满足 |
| Mutagen 同步默认 `two-way-resolved` (Q3) | macOS APFS case-insensitive 强制需要 resolved 模式；`two-way-safe` 在大多数场景下冲突堆积需人工 | ✓ Good — Phase 31 Plan 02 决策；M5 APFS 真机签字 deferred-to-ship |
| persistent volume 单 volume `claude-state-{claude_account_id}` (Q4) | 运维更简单（vs. 双 volume creds + cache 分开）；label `com.cloud-cli-proxy.account_id` 支撑 GC | ✓ Good — Phase 30 数据模型 + Phase 33 实现 |
| Entry API 在现有 endpoint 加字段（Q5 (a)） | 向后兼容旧 v2.0 client（`omitempty` + 未知字段忽略）；不引入 `/capabilities` 新 endpoint | ✓ Good — Phase 30 实现，TestAuthResponse_MissingV3Fields_DefaultZero PASS |
| host-agent 不扩展返回 image labels（Q6） | doctor 全走 SSH 在容器内 exec，控制 / 数据平面分离 | ✓ Good — Phase 34 SC#7 守恒 |
| first-sync alpha/beta 安全门 Fatal 不可降级（Q7） | 防 PITFALLS C5 (non-root + Mutagen root 默认反向清空本地仓库)；alpha 空 + beta 非空必须中止 | ✓ Good — Phase 31 实现，MOUNT_MUTAGEN_SAFETY_GUARD |
| per-claude_account tmux session 命名（Q8） | 与 persistent volume 命名一致；多端 attach 同一 session 时语义清晰 | ✓ Good — Phase 32 实现 |
| doctor `--fix` 默认幂等 + stdin y/N + CI `--yes`（Q9） | 避免 destructive 操作意外执行；6 类 Fixer 全幂等 | ✓ Good — Phase 34 实现 |
| mergerfs 2 路 branch（hot + cold）(Q10) | 比 3 路实现简单；后续若需要 3 路可在 entrypoint 扩展 | ✓ Good — Phase 29 实现 |
| Phase 35 `skip-real-hardware` 不阻塞 phase 完成 | 3 项真机签字依赖物理环境（macOS APFS / Ubuntu 25.04 / 2min 拔网），自动化部分 PASS 即可走 ship 前补签 | ✓ Good — STATE.md 已记，跟踪在 35-HUMAN-UAT.md |
| 自研 hot-sync 替代 Mutagen 客户端架构（隐式设计变更） | Mutagen 客户端协议复杂度高于预期；Phase 31 实际用 ssh + tar 自研 hot-sync 实现等价语义；Entry API 仅需 `SupportsMergerfs` 即可触发降级 | ✓ Good — v3.1 REQUIREMENTS.md 已归档，spec/code 漂移由 tech debt 跟踪 |
| git 仓库强约束前置检查（`os.Getwd` + `git rev-parse`） | 防止用户在家目录/非仓库目录误启动导致全量同步；与 MOUNT_REQUIRE_GIT_REPO 错误码配套 | ✓ Good — Phase 36-04，exitConfigError 恒定，单测锁定 |
| hot_sync 单文件熔断走 `MountConfig` 私有 `effective*()` accessor | main.go 未注入字段时避免 `MaxFileBytes=0` 静默关闭熔断；与 Config 层同一 50MB 兜底 | ✓ Good — Phase 36-03，60MB fixture 测试 PASS |
| sshfs FUSE page cache 4 参数默认追加 | 减少冷文件重复读取 RTT； doctor `sshfs_cache_args` check 字符串匹配锁死顺序 | ✓ Good — Phase 36-05，同会话二次 cat SFTP read count = 1 |
| ColdPromoter 5s 防抖窗口 + 指数退避（1/2/4s）+ 3 次熔断 | 避免 inotify 事件风暴；单文件晋升失败不阻塞用户操作；cold 视图始终可读 | ✓ Good — Phase 37-01，4 条核心单测（dedup/backoff/circuit/start-stop）PASS |
| promotion 统计在 writeLastSession 前刷入（mount 就绪时 = 0） | 与 Phase 36 OversizedFiles 同一快照语义；避免回调耦合 | ✓ Good — Phase 37-02，omitempty 零值不出现在 JSON |
| `CLOUD_CLAUDE_NO_PROMOTION=1` 完全跳过（promoter nil + cleanup guard） | 用户明确关闭时零开销；PID 残留清理无条件执行防 rogue 进程 | ✓ Good — Phase 37-02 |
| doctor 本地 check 用 `t.Setenv("HOME", tempDir)` 隔离 | 测试真实 `LoadConfig/LoadLastSession` 路径，生产代码零 var 注入点 | ✓ Good — Phase 36-06，与 36-04 `t.Setenv(PATH="")` 同一思路 |
| v1.3 归档，能力并入 v3.4 | v1.3 的"本地容器"和"指纹伪装"能力与 v3.4 `cloud-claude local` 目标重叠；维护两套本地 Docker 编排逻辑无意义 | 避免重复开发；Phase 18→39（网络）、Phase 19→39（CLI）、Phase 21→entrypoint `MODE=local` |
| 本地版 sing-box 以 sidecar 容器运行，复用 Cloud 版网络模型 | 保持产品一致性：无论 Cloud 还是 Local，网络出口控制策略完全相同；复用 `internal/network` 的 `PrepareHost`/`InjectManagementVeth` | Local 版也走 `managed-user`（`--network=none`）+ `sing-box-gateway` sidecar + veth 注入 |
| 本地版默认深度伪装，不区分 stealth/local 模式 | 容器指纹伪装是核心产品承诺，不是可选项；`MODE=local` 自动启用完整伪装，无需用户选择 | entrypoint `prepare_container_disguise()` 在 `MODE=local` 下额外伪造 /proc/cpuinfo、/proc/meminfo、/proc/1/cgroup |
| channelOpenDirectMsg 使用导出字段名 | `ssh.Marshal` 通过反射读取字段，未导出字段会导致 panic | ✓ Good — Phase 38 Plan 01 |
| `handleConnection` 预 dial 共享 targetClient | 避免 per-channel dial 开销，与 forwarded-tcpip HandleChannelOpen API 一致 | ✓ Good — Phase 38 Plan 02 |
| `isForbiddenTarget` 纯函数设计 | 不依赖 Server 结构体，便于单元测试 | ✓ Good — Phase 38 Plan 01 |
| `handleGlobalRequests` 使用 `ssh.Conn` 接口 | 保持函数签名通用，不绑定 `*ssh.Client` | ✓ Good — Phase 38 Plan 02 |
| `--egress-config` 注入 sing-box outbound JSON | 灵活支持 tun 和 proxy 两种模式，用户可自定义出口配置 | ✓ Good — Phase 39 Plan 02 |
| doctor sshd check 默认报 `SSH_SSHD_FORWARDING_DISABLED` | 与 `checkSSHDKeepaliveDrift` 保持一致的错误处理策略 | ✓ Good — Phase 44 Plan 01 |
| 两段式 sing-box 配置（静态 config.json + 动态 local rule-set 文件 watch） | 让白名单变更不重启 sing-box 进程，零 SSH 断连；sing-box 1.10+ 文件 watch 原生支持 | ✓ Good — v3.5 Phase 45 |
| 拆分 DNS：内网走 dns-local，公网白名单走代理 DoH | 即使白名单流量走 direct，DNS 查询也不让 LAN DNS 留痕；strategy=ipv4_only 防 IPv6 旁路 | ✓ Good — v3.5 Phase 45 |
| 容器 `/etc/resolv.conf` 改为 `:ro` bind mount，唯一 nameserver = sing-box tun IP（172.19.0.1） | 关闭旧 8.8.8.8 占位的旁路；`/etc/nsswitch.conf` 强制 `hosts: files dns` 禁 mdns/myhostname/wins | ✓ Good — v3.5 Phase 45 |
| `ContainerProxyProvider.PrepareHost` 拆分为 `PrepareGateway` + `PrepareHost` | 让 worker 容器创建前 gateway 已起、DNS 源文件已写盘，消除 entrypoint 启动时 tun0 未监听的竞争窗口 | ✓ Good — v3.5 Phase 45 |
| `is_system` 预设双层防御（Go sentinel `ErrSystemBypassPresetImmutable` + SQL `WHERE is_system = FALSE`） | 单层防御可能因 ORM 漏过；双层保证即便绕过 Go 检查 SQL 也拦截 | ✓ Good — v3.5 Phase 45 |
| 双轨审计：DB `host_bypass_audit_log` + `EventRecorder.RecordEvent("bypass.<action>")` 事件流 | 持久化（90 天）+ 实时（SSE 推送）兼顾；任一失败仅 Warn 不阻断主请求 | ✓ Good — v3.5 Phase 46 |
| `QueueHostAction` 第 5 参用专属 `bypassSnapshotID` 形参（非通用 payload string） | 类型签名层面把「借 requestedBy 传 snapshot ID」hack 关闭，调用点 grep 可见即正确 | ✓ Good — v3.5 Phase 47 |
| `ApplyBypassRuleSet` 严格顺序「先 nft 事务 → 后 atomic write」 | nft 失败不动文件，避免「文件已变但 nft 未变」漂移；漂移留给 `VerifyBypassConsistency` 检出 | ✓ Good — v3.5 Phase 47 |
| 健康检查复用 `/dev/tcp/192.168.0.1/53` TCP 半握手 | 不发真实 DNS 报文（避免污染 audit）也能验证 sb-tun0 路由生效 | ✓ Good — v3.5 Phase 47 |
| rollback 不修改 target snapshot 状态，新建 `source='rollback'` pending 行 | 保持 prev snapshot 状态 `applied` 不动，符合 WARN-4 不变式；hash 后缀用 `crypto/rand` 8 字节 hex 绕开 UNIQUE 冲突 | ✓ Good — v3.5 Phase 46+47 |
| `Consistency` endpoint 3s timeout 防 nft 卡死 DoS | DeadlineExceeded → 504 BYPASS_CONSISTENCY_TIMEOUT vs. 一般 500 BYPASS_CONSISTENCY_ERROR 分开，便于前端 UI 区分重试 / 排障 | ✓ Good — v3.5 Phase 47 |
| nft 表族 = `ip cloudproxy`，`nftRunner` / `nftJSONLister` 自带 `nsenter -t <pid> -n --` 包装 | 与 47-02 现状对齐，单点事实源 `bypassNftFamily/Table`；Phase 47 初次验证教训：单测 PASS ≠ 生产路径 OK | ✓ Good — v3.5 Phase 47（commit 60c9896） |
| uat-bypass.yml uat job 用 fixture 自适应 preflight（非 `if:false`） | fixture 缺失时跳过并发 warning，fixture 一旦合入即触发完整 6 场景 × ubuntu-24.04 × I1–I10 CI 守护，无需改 workflow | ✓ Good — v3.5 Phase 47（TD-01 fixed commit 58c0a5f） |

## Evolution

这个文档会在阶段切换和里程碑完成时持续更新。

**每次阶段切换之后**（通过 `$gsd-transition`）：

1. 如果有需求被证伪，移动到"明确不做"并说明原因
2. 如果有需求被验证，移动到"已验证"并标注阶段
3. 如果出现新需求，加入"当前活跃"
4. 如果产生重要决策，补充到"关键决策"
5. 如果"这是什么"已经不准确，就按当前现实更新

**每次里程碑完成之后**（通过 `$gsd-complete-milestone`）：

1. 全量复查所有章节
2. 检查"核心价值"是否仍然是最高优先级
3. 审视"明确不做"的理由是否还成立
4. 用当前产品状态更新"背景"

---
*Last updated: 2026-05-13 — v3.5 网络白名单与 DNS 拆分解析 shipped（tag v3.5），34/34 REQ satisfied，5 项 tech-debt follow-up（TD-01 fixed，TD-02..05 P2），等待 `/gsd-new-milestone` 进入下一里程碑。*
