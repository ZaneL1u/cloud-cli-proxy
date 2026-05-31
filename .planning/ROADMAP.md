# Roadmap: Cloud CLI Proxy

## Milestones

- ⏳ **v4.0 sing-box 同容器化** — Phases 53-56 (in progress, started 2026-05-16) — **breaking change**
- ✅ **v1.0 MVP** — Phases 1-6 (shipped 2026-03-28) — [Archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 支持代理协议出网** — Phases 7-10 (shipped 2026-03-28) — [Archive](milestones/v1.1-ROADMAP.md)
- ⏸️ **v1.2 用户自助面板与 Bootstrap 重设计** — Phases 11-16 (partially shipped, remaining deferred)
- 🗃️ **v1.3 claude-shell 本地透明代理** — Phases 17-23 (archived, capabilities merged into v3.4)
- ✅ **v2.0 cloud-claude 透明远程 CLI** — Phases 24-28 (shipped 2026-04-15) — [Archive](milestones/v2.0-ROADMAP.md)
- ✅ **v3.0 远端开发体验升级** — Phases 29-35 (shipped 2026-04-23) — [Archive](milestones/v3.0-ROADMAP.md)
- ✅ **v3.1 映射语义补齐与懒加载** — Phases 36-37 (shipped 2026-04-24) — [Archive](milestones/v3.1-ROADMAP.md)
- ✅ **v3.4 多形态容器接入** — Phases 38-44 (shipped 2026-05-08) — [Archive](milestones/v3.4-ROADMAP.md)
- ✅ **v3.5 网络白名单与 DNS 拆分解析** — Phases 45-47 (shipped 2026-05-13) — [Archive](milestones/v3.5-ROADMAP.md)
- ✅ **v3.6 端到端测试体系与网络隔离验证** — Phases 45-52 (shipped 2026-05-14) — [Archive](milestones/v3.6-ROADMAP.md)

## Phases

### ⏳ v4.0 sing-box 同容器化 (Phases 53-56) — IN PROGRESS

**Milestone goal:** 把 sing-box 从独立 `cloudproxy-gw-*` 容器搬进用户容器内部，消灭"user 容器 default 路由不指向 gw"这一整类回归泄漏；单容器架构带来 breaking change，故升 major v4.0。

**Locked decisions:** D-V4-1..6（见 `.planning/PROJECT.md` § Current Milestone）

- [x] Phase 53: 镜像与 entrypoint 基线（IMG-01..04 + EP-01..04 + NET-01..04，12 REQ）
- [x] Phase 54: 控制面单容器化（CTRL-01..05，5 REQ）
- [x] Phase 55: e2e 重构 + 同容器安全断言（SEC-01..03 + E2E-V4-01..06，9 REQ）
- [x] Phase 56: CI paths 扩面 + Makefile 入口（CI-01..03，3 REQ）

---

#### Phase 53: 镜像与 entrypoint 基线

**Goal:** managed-user 镜像内置 sing-box + entrypoint 串接启动序列，本地手工 `docker run` 起来后 `curl ip.me` 走绑定的出口 IP；用户 SSH 进来非 root、无 NET_ADMIN、读不到 sing-box config。

**Depends on:** v3.6 worker firewall ruleset、v3.6 sing-box config schema、v3.6 镜像构建路径

**Requirements:** IMG-01 / IMG-02 / IMG-03 / IMG-04 / EP-01 / EP-02 / EP-03 / EP-04 / NET-01 / NET-02 / NET-03 / NET-04

**Success criteria:**

1. 本地 `docker run --rm --device /dev/net/tun --cap-add NET_ADMIN -v $config:/etc/sing-box/config.json:ro managed-user:v4-dev` 起来后，容器内 `curl https://ip.me` 返回绑定的出口 IP，不是宿主真实 IP。
2. 容器内 `ps -o uid,user,comm -p $(pidof sing-box)` 显示 uid=9000 (`singbox` 账号)，验证 setuid 降权生效。
3. 容器内 `cat /etc/sing-box/config.json` 失败（文件已 rm 或权限拒绝）。
4. 容器内 user shell `getpcaps $$` 输出空 cap 集合，`ip link set tun0 down` 返回 `Operation not permitted`。
5. `docker exec <container> kill -9 $(pidof sing-box)` 触发后容器在 ≤3s 内退出，docker `restart=on-failure` 在 ≤5s 内拉起新容器。

---

#### Phase 54: 控制面单容器化

**Goal:** control-plane / host-agent 不再创建 gw 容器；`container_proxy_provider` 简化为单容器路径；`cloudproxy-net-*` 自定义 bridge 退役；双绑互斥契约不变。

**Depends on:** Phase 53（managed-user v4 镜像已构建可用）

**Requirements:** CTRL-01 / CTRL-02 / CTRL-03 / CTRL-04 / CTRL-05

**Success criteria:**

1. 创建一台 host 后只生成一个 docker 容器（`cloudproxy-<id>`），`docker network ls` 不再出现 `cloudproxy-net-<HostID>`。
2. `internal/network/container_proxy_provider.go` 净行数减少 ≥ 300 行；teardownGateway 路径整体删除。
3. sing-box config 由 host-agent 在 PrepareHost 时注入 user 容器，文件权限 root:root 0600；容器启动后该文件被 rm（`docker exec ls /etc/sing-box/` 不见 config.json）。
4. v3.6 51-09 双绑互斥 pre-check（`ErrCodeEgressIPAlreadyBound` + 409 + 双语 message）API 行为契约保持不变。
5. `deploy/docker/sing-box/` 目录退役；Makefile `gateway-image` target 删除；镜像构建产物不再产出 gw 镜像。

---

#### Phase 55: e2e 重构 + 同容器安全断言

**Goal:** `harness.Scenario` builder 单容器化；v3.6 LEAK/KILL/MVS 用例迁移到新架构；新增 SEC-01..03 三条同容器架构特有的安全断言。

**Depends on:** Phase 53 + Phase 54（v4 镜像 + 控制面就绪后才能跑端到端用例）

**Requirements:** SEC-01 / SEC-02 / SEC-03 / E2E-V4-01 / E2E-V4-02 / E2E-V4-03 / E2E-V4-04 / E2E-V4-05 / E2E-V4-06

**Success criteria:**

1. `harness.Scenario` builder API `.WithSingBoxGateway(...)` 合并进 `.WithUser(...)`；所有旧调用点迁移完成，单容器拓扑覆盖。
2. v3.6 LEAK-01..08 用例迁移后继续绿（抓包视角改为 host eth0 + 容器 veth pair，断言语义不变）。
3. v3.6 KILL-01..04 用例迁移：`docker kill <gw>` 改为 `docker exec <user> kill -9 $(pidof sing-box)`；新增断言 "PID 1 死 → 容器死 → 出网立即断"。
4. v3.6 MVS-01..10 + GoldenPath 用例迁移，出口 IP / DNS / default-deny / 错误码契约保持不变。
5. 新增 e2e 用例 `SEC-01`（用户 kill sing-box 必须失败）、`SEC-02`（用户读 config 必须失败）、`SEC-03`（用户 cap 集合必须为空）；三条新用例独立可跑且全绿。
6. 删除 v3.6 期间 cross-container 协调辅助代码（gw 启动同步、network connect 等待等）；删除净行数 ≥ 150 行。

---

#### Phase 56: CI paths 扩面 + Makefile 入口

**Goal:** 把 v3.6 "偶尔改坏没拦住" 的 CI 触发盲区一并堵掉，本地 `make e2e` 一条命令入口落地。

**Depends on:** Phase 55（e2e 套件迁移完成后扩 paths 才有意义）

**Requirements:** CI-01 / CI-02 / CI-03

**Success criteria:**

1. `.github/workflows/e2e.yml` paths filter 扩面：新增 `internal/controlplane/http/**` / `internal/store/**` / `deploy/docker/**` / `Makefile` / `go.mod` / `go.sum`；测试 PR 修改其中任一路径都能触发 e2e job。
2. `make e2e` 等价 `go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=15m`，本地一条命令跑通；新人按 README 走能直接跑起。
3. `make e2e` 内串 `lint-no-bare-sleep` + `go vet -tags=e2e ./tests/e2e/...`，行为与 CI lint job 对齐；本地任一 step 失败时退出码与 CI 一致。

---

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

<details>
<summary>✅ v3.6 端到端测试体系与网络隔离验证 (Phases 45-52) — SHIPPED 2026-05-14</summary>

- [x] Phase 45: 测试基础设施与 CI 骨架 (5/5 plans) — completed 2026-05-14
- [x] Phase 46: MVS 黄金路径与出口 IP 验证 (5/5 plans) — completed 2026-05-14
- [x] Phase 47: MVS 治理与心跳验证 (3/3 plans) — completed 2026-05-14
- [x] Phase 48: Kill-switch 核心验证 (2/2 plans) — completed 2026-05-14
- [x] Phase 49: 防泄漏对抗测试 (8/8 plans) — completed 2026-05-14
- [x] Phase 50: Kill-switch 压力测试 (4/4 plans) — completed 2026-05-14
- [x] Phase 51: 代码层质量加固 (9/9 plans, 含 51-09 收口) — completed 2026-05-14
- [x] Phase 52: 可观测性与诊断 (3/3 plans) — completed 2026-05-14

8 phase / 39 plan / 38 v1 REQ satisfied；8 项 tech debt 全部非阻塞 ship，详见 `milestones/v3.6-MILESTONE-AUDIT.md`；9 项 human verification 全部 deferred-to-CI（前置 TD-3 Scenario.Start Step 2..7 真实接入）。

<details>
<summary>📦 详细 phase 计划（已归档原文）</summary>

### Phase 45: 测试基础设施与 CI 骨架

**Goal**: 在本仓库长出可复用的 e2e 骨架（testcontainers-go + testify/suite + Scenario 抽象 + 双层 CI runner + 失败 artifact 归档 + waitFor 替代裸 sleep），让后续所有网络栈用例都能基于同一套 harness 写真实跑得通的测试。

**Depends on**: Phase 47 (v3.5 sing-box 两段式静态配置 + 拆分 DNS + uat-bypass.sh CI 守护已就绪)

**Requirements**: E2E-01 / E2E-02 / E2E-03 / E2E-04 / E2E-05

**Plans**: 5 plans

- [x] 45-01-PLAN.md — `tests/e2e/` 目录骨架 + testcontainers-go + testify/suite 接入 — completed 2026-05-14
- [~] 45-02-PLAN.md — Scenario 抽象骨架 + Step 1（postgres testcontainer）真实实现 + Step 2..7 留 TODO — partial 2026-05-14（端到端启动序列由 Phase 46 第一个用例落地时补齐，详见 45-02-SUMMARY.md）
- [x] 45-03-PLAN.md — waitFor helper + 4 个语义化变体（Log/Port/HTTP/Exec）+ DumpHook 占位 — completed 2026-05-14
- [x] 45-04-PLAN.md — 失败 artifact 归档 hook + 5 子目录占位 + ArtifactDumper + BaseSuite 集成 — completed 2026-05-14
- [x] 45-05-PLAN.md — CI 双层 workflow（hosted ubuntu-24.04 runner + paths 强制守护 + 失败 PR comment）+ lint-no-bare-sleep 守护脚本 — completed 2026-05-14

> 注：Phase 45 plan 编号意图与初稿微调 —— Plan 03（原文案"CI 双层架构"）改为 waitFor helper（CONTEXT.md §Area 4 决策），CI 接线挪到 Plan 05；同时 ROADMAP 草稿中 E2E-03"self-hosted Linux runner"已根据 CONTEXT.md §Area 3 调整为 hosted ubuntu-24.04（与 v3.5 uat-bypass.yml 同款）。

**Details:**

1. `tests/e2e/` 目录存在，使用 testcontainers-go + testify/suite 组织，suite 拥有 `SetupSuite/TearDownSuite` 生命周期，可 `go test ./tests/e2e/...` 单独跑通最小烟雾用例。
2. Scenario 抽象可声明式描述「控制面 + host-agent + Postgres + N 个用户容器 + sing-box gateway」拓扑，通过 builder API 一行声明用户数、出口 IP 数、协议类型，不必复制粘贴启动代码。
3. `.github/workflows/` 至少存在两个 e2e job：hosted runner 跑非特权（Go 单元 + API 集成 + BATS smoke），self-hosted Linux runner 跑特权网络栈 e2e；两者 job name 在 README 中可被指引到。
4. 任何 e2e 用例失败时自动调用归档 hook，artifact 目录至少包含容器日志 / `nft list ruleset` / `ip netns list` / `ip route` / `pg_dump`，CI 上 `actions/upload-artifact@v4` 拿得到。
5. 测试 harness 内禁止 `time.Sleep` 用作"等待条件就绪"，统一通过 `waitFor(ctx, predicate, timeout)` helper；CI 上有 grep/lint 守护防回退。

### Phase 46: MVS 黄金路径与出口 IP 验证

**Goal**: 把"首次 bootstrap → 进入 SSH → 出网经由绑定的出口 IP → DNS 走 tun → 直连外网被拒绝 → CLI 错误码契约稳定"这条用户主路径用 e2e 跑通，作为 MVS（Minimum Viable Suite）第一组真实可信用例。

**Depends on**: Phase 45 (e2e 骨架与 Scenario 抽象就绪)

**Requirements**: MVS-01 / MVS-02 / MVS-03 / MVS-04 / MVS-05

**Plans**: 5 plans

- [x] 46-01-PLAN.md — bootstrap 黄金路径骨架 + `GoldenPath`/`StartGoldenPath` + 24 个纯函数单测 — completed 2026-05-14
- [x] 46-02-PLAN.md — 出口 IP 三源轮询 + `Vote` 多数派裁决（公网等值断言 deferred-to-CI）— completed 2026-05-14
- [x] 46-03-PLAN.md — DNS OR 语义（`ClassifyDNSResult` Tunneled/Denied 二选一即 PASS）— completed 2026-05-14
- [x] 46-04-PLAN.md — 默认拒绝矩阵（`DefaultDenyMatrix` 4 target × 3s timeout × 并发）— completed 2026-05-14
- [x] 46-05-PLAN.md — CLI 错误码契约（被测 binary 为 `cloud-bootstrap.sh`，exit code 10/11/12/13 与 `bootstrap_errors.go` 源真相 cross-check）— completed 2026-05-14

> 注：MVS-05 被测 binary 与 ROADMAP 草稿描述偏差 —— grep 源码后实际错误码 10/11/12/13 由 `internal/controlplane/http/bootstrap_errors.go` 定义、`deploy/bootstrap/cloud-bootstrap.sh` `case "$error_code"` 映射，按 CONTEXT §Area 3「以源码为准」决策落地为 bootstrap.sh。详见 `46-05-SUMMARY.md` 与 `46-VERIFICATION.md` ROADMAP 偏差节。

**Details:**

1. `bootstrap` e2e 跑完一轮：scenario 起容器 → 模拟 curl 认证 → 等待 `host.ready` → expect 子进程拿到 SSH banner，无人工步骤、无裸 sleep。
2. 容器内出口 IP 校验从 ≥3 个独立回显源拉，多数派一致才判 PASS；若某源全部超时，e2e 不直接 fail，而是按"投票"语义裁决，避免外网抖动误报。
3. DNS 测试同时覆盖"被 tun 接管返回正常 A 记录"和"被防火墙明确拒绝"两种语义，断言至少其一成立，并 dump nft `counter` 命中数。
4. 直连外网测试遍历多 IP × 多端口矩阵（1.1.1.1:80 / 8.8.8.8:443 / 9.9.9.9:443 / 169.254.169.254:80），全部必须超时或被拒绝，任何一个连通即 fail。
5. CLI 错误码用真实 cloud-claude binary 触发各场景（错密码 / 禁用账号 / 过期账号 / 不存在的 host），断言 exit code 与文档表一致。

### Phase 47: MVS 治理与心跳验证

**Goal**: 把到期容器自动停止 + 出口 IP 双绑互斥 + host-agent 心跳与恢复三条治理路径变成 e2e 自动用例，杜绝"上线后才发现治理逻辑没生效"。

**Depends on**: Phase 46 (黄金路径 e2e 就绪后才有共享的容器/host fixture)

**Requirements**: MVS-06 / MVS-07 / MVS-08

**Plans**: 3 plans

- [x] 47-01-PLAN.md — 到期容器自动停止 + `host.stop.expired` 审计事件（环境变量 `EXPIRY_SCAN_INTERVAL`）— completed 2026-05-14
- [x] 47-02-PLAN.md — 出口 IP 双绑互斥（写预期行为，backend pre-check 缺失列 Phase 51 QUAL-04 修复）— completed 2026-05-14
- [x] 47-03-PLAN.md — host-agent 心跳与恢复（`pkill -9` 进程级 + `/healthz` checks.agent 轮询）— completed 2026-05-14

> 注：CONTEXT 草案与源码偏差 5 项（事件名 `host.stop.expired` / env 名 `EXPIRY_SCAN_INTERVAL` / 双绑缺 pre-check / 仅全局 `/healthz` 无 per-host / 健康枚举为 `ok/warning/degraded` × `ok/unreachable`），全部以源码为准，详见 47-VERIFICATION.md §ROADMAP 偏差节。

**Details:**

1. 用 fake clock 或缩短 ExpiryScanner 周期，让到期用户的运行容器在 e2e 内被自动停止；审计事件 `host.stopped` 必须出现在 `events` 表，行内含触发原因。
2. 同一出口 IP 通过 API 绑定到 Host A 之后，再尝试绑定到 Host B 必须返回 4xx + 稳定错误码（不允许 500），且现有绑定不被破坏。
3. `kill -9` host-agent 后，控制面 health 状态必须在 30s 内转 unhealthy；重启 host-agent 后 30s 内自动转回 healthy，全过程不依赖人工 force resync。

### Phase 48: Kill-switch 核心验证

**Goal**: 验证两条最关键的 kill-switch：sing-box 崩溃后用户容器立即断网（不降级直连），以及容器内手工篡改 resolv.conf 仍然走 tun 或被拒绝。

**Depends on**: Phase 46 (需要黄金路径 e2e 起好的容器 + 出口 IP 抓取能力)

**Requirements**: MVS-09 / MVS-10

**Plans**: 2 plans

- [x] 48-01-PLAN.md — sing-box 崩溃后容器断网（`docker kill` → ≤3s curl 失败 + privileged sidecar 抓包零非网关流量）— completed 2026-05-14
- [x] 48-02-PLAN.md — resolv.conf 篡改免疫（`ClassifyResolvConfDNSOutcome` Tunneled/Denied OR 语义 + host eth0 抓包零 UDP/53→8.8.8.8）— completed 2026-05-14

> 注：host eth0 tcpdump 改走 `docker run --network host --cap-add NET_RAW/NET_ADMIN netshoot` privileged sidecar 路径（CONTEXT §Area 3 Claude's Discretion 允许的备选），新增 `E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP` env 覆盖。

**Details:**

1. `docker kill` sing-box gateway 之后，3 秒内容器内 `curl https://ifconfig.me` 必须失败；用 `tcpdump -i eth0` 确认 host eth0 没有出现来自该容器 IP 的非网关流量。
2. 容器内 `echo nameserver 8.8.8.8 > /etc/resolv.conf` 执行成功（因为 ro bind mount 之上还能在用户态做 cp + mv），但随后 `dig example.com` 仍只走 tun 或被防火墙明确拒绝；host eth0 抓包必须无 udp/53 → 8.8.8.8 流量。

### Phase 49: 防泄漏对抗测试

**Goal**: 把 8 条防泄漏不变量（DNS 明文 / DoT / ICMP / IPv6 / IMDS / raw socket / link-local / capability）固化为可重复跑的 e2e 用例，对抗常见旁路绕过手段。

**Depends on**: Phase 45 (artifact 归档 + Scenario 已就绪) + Phase 47 (Bypass 治理用例已经覆盖白名单基线)

**Requirements**: LEAK-01 / LEAK-02 / LEAK-03 / LEAK-04 / LEAK-05 / LEAK-06 / LEAK-07 / LEAK-08

**Plans**: 8 plans

- [x] 49-01-PLAN.md — LEAK-01 DNS 明文 UDP/53 旁路检测（`dig @8.8.8.8` + host eth0 抓包断言）— completed 2026-05-14
- [x] 49-02-PLAN.md — LEAK-02 DoT (853) 旁路检测（`kdig +tls @1.1.1.1` + host eth0 抓包断言）— completed 2026-05-14
- [x] 49-03-PLAN.md — LEAK-03 ICMP 阻断（`ping 8.8.8.8` 必须失败）— completed 2026-05-14
- [x] 49-04-PLAN.md — LEAK-04 IPv6 阻断（`curl -6 ipv6.google.com` 必须失败 + `disable_ipv6=1` 双保险）— completed 2026-05-14
- [x] 49-05-PLAN.md — LEAK-05 IMDS 阻断（`169.254.169.254` 与 `169.254.170.2` 必须失败）— completed 2026-05-14
- [x] 49-06-PLAN.md — LEAK-06 raw socket 拒绝（`SOCK_RAW` 必须 PermissionError）— completed 2026-05-14（gap → Phase 51 QUAL-06）
- [x] 49-07-PLAN.md — LEAK-07 link-local 显式 drop（nftables 规则覆盖 `169.254.0.0/16`）— completed 2026-05-14（gap → Phase 51 QUAL-06/07）
- [x] 49-08-PLAN.md — LEAK-08 capability 审计（worker CapEff/CapBnd 不含 NET_RAW/NET_ADMIN/SYS_ADMIN）— completed 2026-05-14（gap → Phase 51 QUAL-06）

**Details:**

1. 每条 LEAK-* 用例都通过 host eth0 抓包 / `nft list ruleset` / `getpcaps` 等内核或宿主机视角的"独立 oracle"做断言，不依赖容器自身报告。
2. 失败时自动归档抓包文件 + nft ruleset + capability dump 到 artifact，方便事后复盘。
3. 8 条用例可单独跑也可作为 `tests/e2e/leak/...` 套件一起跑，整组耗时 ≤ 5 分钟。

### Phase 50: Kill-switch 压力测试

**Goal**: 通过 SIGKILL / `tun0 down` / Pumba netem 故障注入 / `docker network disconnect` 等手段对 kill-switch 做压力测试，确认任何故障姿势下 worker 都不会回落到 host 默认路由。

**Depends on**: Phase 48 (核心 kill-switch 用例已稳定可作为基线对照)

**Requirements**: KILL-01 / KILL-02 / KILL-03 / KILL-04

**Plans**: 4 plans

- [x] 50-01-PLAN.md — KILL-01 SIGKILL timing 严格化（≤3s + host eth0 抓包零非网关）— completed 2026-05-14
- [x] 50-02-PLAN.md — KILL-02 `ip link set tun0 down` 容器 curl 失败 — completed 2026-05-14
- [x] 50-03-PLAN.md — KILL-03 Pumba netem 1000ms delay（SSH 存活 + 出口 IP 允许 inconclusive）— completed 2026-05-14
- [x] 50-04-PLAN.md — KILL-04 `docker network disconnect` worker 不 fallback host bridge — completed 2026-05-14

> 注：gateway 实际接 `cloudproxy-net-<HostID>` 自定义 bridge（非默认 bridge）；KILL-04 用例带 backend GAP 兜底 `t.Skipf` 分支自动流转 Phase 51。Pumba 固定 `gaiaadm/pumba:0.10.0` tag 避免 latest 漂移。

**Details:**

1. KILL-01..02 关注"硬故障 → 立即断网"延迟必须 ≤ 3 秒，期间不允许任何明文请求逃逸到 host eth0。
2. KILL-03 要确立"网络劣化下的可观察行为契约"：SSH 控制流必须存活，出口 IP 校验允许超时但不允许给出错误的 IP；e2e 把这条契约写死。
3. KILL-04 重点在"控制面意外 disconnect 网络后是否会自动 fallback 到 host bridge"，必须证明不会，且容器内 curl 失败、host eth0 无来自该容器的非网关流量。

### Phase 51: 代码层质量加固

**Goal**: 把 verify.go 多源轮询 / 多目标泄漏检测 / 全 nameserver 校验，namespace.go 探测窗口参数化，nftables 规则加 counter，worker cap-drop NET_RAW/NET_ADMIN，以及 `go test -race -shuffle=on` 与 goleak.VerifyTestMain 这 8 条质量改造一次性落地。

**Depends on**: Phase 46 + Phase 49 (e2e 用例已经能反向暴露 verify/namespace/firewall 的不足)

**Requirements**: QUAL-01 / QUAL-02 / QUAL-03 / QUAL-04 / QUAL-05 / QUAL-06 / QUAL-07 / QUAL-08

**Plans**: 8 plans

- [x] 51-01-PLAN.md — verify.go `verifyEgressIPMulti` 三源 + `voteEgressIP`（复用 Phase 46 Vote 语义）— completed 2026-05-14
- [x] 51-02-PLAN.md — verify.go `verifyLeakBlockedMulti` 多 target 默认值对齐 `DefaultDenyMatrix` — completed 2026-05-14
- [x] 51-03-PLAN.md — verify.go `parseAllNameservers` 遍历全 nameserver 行 — completed 2026-05-14
- [x] 51-04-PLAN.md — namespace.go `WithProbeWindow / WithMaxRetries` functional option — completed 2026-05-14
- [x] 51-05-PLAN.md — worker_firewall_linux 全规则 `expr.Counter` + 新增 `169.254.0.0/16 drop`（闭 Phase 49 GAP-2）— completed 2026-05-14
- [x] 51-06-PLAN.md — worker 显式 `--cap-drop NET_RAW` + 删 `SYS_ADMIN`，NET_ADMIN 按 sing-box tun 依赖保留（部分闭 Phase 49 GAP-1）— completed 2026-05-14
- [x] 51-07-PLAN.md — Makefile + ci.yml `-race -shuffle=on -count=1` 默认 — completed 2026-05-14
- [x] 51-08-PLAN.md — `cmd/cloud-claude/testmain_test.go` goleak.VerifyTestMain 接入 — completed 2026-05-14

> 51-09（新增收口）：`internal/controlplane/http/admin_bindings.go` 双绑 pre-check + `ErrCodeEgressIPAlreadyBound` + 409 + 双语 message（**闭 Phase 47 D-47-3**）— completed 2026-05-14。

> 注：QUAL-06 实际落地保留 `--cap-add NET_ADMIN`（CONTEXT §Area 4 允许的折中），原因是 sing-box 需在 worker netns 创建 tun0 设备。Phase 49 LEAK-08 fixture `proc_status_clean.txt` NET_ADMIN 期望需在 e2e 后续校准。

**Details:**

1. QUAL-01..03 改造完成后，一次性回归跑 Phase 46 + Phase 49 的全部 e2e 用例必须仍通过，且 verify.go 行为对外行为契约不变（仅"内部更稳健"）。
2. QUAL-05 落地后，所有 firewall 规则在 `nft list ruleset` 输出中带 counter；新增的 nft counter 在 e2e artifact 里可读，作为后续排障一手证据。
3. QUAL-07/08 改造后 CI 默认就跑 `-race -shuffle=on -count=1`，goleak 会拦截非已知模式的 goroutine 泄漏。

### Phase 52: 可观测性与诊断

**Goal**: 把 e2e 失败时的"事后排障"做成一键工程：脚本统一收集容器日志 / 网络状态 / Docker 元信息 / Postgres dump / 系统状态，CI 自动 upload-artifact，开发者无需手工 ssh 到 runner。

**Depends on**: Phase 45 (e2e CI 骨架就绪) + Phase 51 (nft counter 加好后 artifact 内容更有用)

**Requirements**: OBS-01 / OBS-02 / OBS-03

**Plans**: 3 plans

- [x] 52-01-PLAN.md — `tests/e2e/harness/collect-artifacts.sh` 一键采集脚本 + 6 单测 — completed 2026-05-14
- [x] 52-02-PLAN.md — 5 子目录 README 模板 + metadata.txt + 单测扩展 — completed 2026-05-14
- [x] 52-03-PLAN.md — DumpHook 切到脚本 + `.github/workflows/e2e.yml` `if: failure()` + `upload-artifact@v4` 完整化 — completed 2026-05-14

**Details:**

1. `collect-artifacts.sh` 在任何 e2e 用例失败时被自动触发，输出目录树固定为 logs/network/docker/postgres/system 五个子目录，每个子目录 README 写明"里面是什么"。
2. CI failure path 上传的 artifact 文件在 GitHub Actions UI 中可见，体积保持在 100MB 以内（必要时压缩或滚动裁剪）。
3. 本地开发者也可手动 `bash tests/e2e/harness/collect-artifacts.sh ./out` 复用同一套采集逻辑，与 CI 行为对齐。

</details>

</details>

## Progress

**Active milestone:** v4.0 sing-box 同容器化 — Phases 53-56 — breaking change

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 53-56. v4.0 sing-box 同容器化 | v4.0 | 0/— | In Progress | — |
| 1-6. v1.0 MVP | v1.0 | 19/19 | Complete | 2026-03-28 |
| 7-10. v1.1 代理协议出网 | v1.1 | 11/11 | Complete | 2026-03-28 |
| 11-12. v1.2 认证与自助面板 | v1.2 | 5/5 | Partial | 2026-03-29 |
| 17-23. claude-shell 本地代理 | v1.3 | — | Archived | — |
| 24-28. v2.0 cloud-claude | v2.0 | 7/7 | Complete | 2026-04-15 |
| 29-35. v3.0 远端开发体验升级 | v3.0 | 30/30 | Complete | 2026-04-23 |
| 36-37. v3.1 映射语义补齐与懒加载 | v3.1 | 11/11 | Complete | 2026-04-24 |
| 38-44. v3.4 多形态容器接入 | v3.4 | 14/14 | Complete | 2026-05-08 |
| 45-47. v3.5 白名单与 DNS 拆分 | v3.5 | 10/10 | Complete | 2026-05-13 |
| 45-52. v3.6 端到端测试体系 | v3.6 | 39/39 | Complete | 2026-05-14 |

### Phase 57: 资源限制可配置化

**Goal:** 允许管理员在创建和停止主机时手动设置内存、CPU 和磁盘限制，支持"无限制"选项。数据库列改为 nullable（NULL = 无限制），API 使用指针类型区分三态（省略=默认 / 0=无限制 / 正值=限制），新增 PATCH 端点，前端提供直观的预设+自定义选择控件。
**Requirements**: RES-01（无限制语义）/ RES-02（PATCH API）/ RES-03（前端控件）/ RES-04（磁盘限制执行）
**Depends on:** Phase 56
**Plans:** 3/3 plans complete

Plans:
- [x] 57-01-PLAN.md — 数据库迁移 + Go 数据模型类型变更（RES-01）
- [x] 57-02-PLAN.md — API 三态解析 + PATCH 端点 + Worker --storage-opt（RES-02, RES-04）
- [x] 57-03-PLAN.md — 前端资源限制选择器 + 创建表单 + 详情页编辑（RES-03）

---

*Last updated: 2026-05-29 — Phase 57 planned (3 plans, 2 waves, 4 REQ covered).*
