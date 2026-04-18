# Phase 29: 受管镜像 v3 + Worker 容器参数扩展 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-18
**Phase:** 29-v3-worker
**Mode:** `--auto`（Claude 按推荐默认自动拍板）
**Areas discussed:** 镜像演进路径、二进制来源与架构、entrypoint 改造、HostActionRequest.Volumes 契约、host-preflight / AppArmor、CI 镜像体积 gate、image.lock 扩展、mergerfs branch 拓扑（Q10）

---

## Area 1 — 镜像演进路径

| Option | Description | Selected |
|--------|-------------|----------|
| A. 增量改造 `deploy/docker/managed-user/Dockerfile` | v3 组件叠在现有镜像上，KasmVNC/Chromium 保留以兼容 v1.2 deferred 用户面；单一镜像交付 | ✓ |
| B. 新建 `deploy/docker/managed-user-v3/` 独立镜像 | v1.2 用户面继续用 v2 镜像，v3 用精简镜像；双镜像维护成本高 | |
| C. 拆 base 镜像 + KasmVNC variant（multi-stage） | 理论最优体积，但改造幅度远超 Phase 29 的范围；重构风险大 | |

**User's choice（auto）:** A — 推荐默认，最小改造范围 + 零 v1.2 破坏
**Notes:** v3.0 定位是 v2.0 的"体验升级"，PROJECT.md 明确"零增量特权，复用 v2.0 已开放通道"。独立镜像或拆分方案都会把 Phase 29 从一个"基线镜像升级"扩大为架构重构，不符合 ROADMAP 的 Goal 表述。

---

## Area 2 — mergerfs / mutagen-agent 二进制来源 + 架构覆盖

| Option | Description | Selected |
|--------|-------------|----------|
| A. GitHub release 静态 deb + amd64 + arm64 | mergerfs `trapexit/mergerfs` release + mutagen GitHub release tarball；双架构 | ✓ |
| B. 仅 amd64 单架构 | 构建最简；但宿主机 arm64 Mac / AWS Graviton 未来不兼容 | |
| C. 自建 apt repo / apt 官方包 | PITFALLS M3 明确禁止 apt 安装 mergerfs（版本滞后）；Mutagen 无官方 apt 包 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** STACK.md + PITFALLS M3 已将 mergerfs 锁死为 GitHub release 静态 deb；Mutagen 仅 GitHub release 可选。双架构是未来扩展保险，实际 CI 以 amd64 为主线，arm64 作为 build matrix 副路径。

---

## Area 3 — HostActionRequest.Volumes 契约形态

| Option | Description | Selected |
|--------|-------------|----------|
| A. 最小契约 `{Name, Target, ReadOnly, Labels}` 仅支持 named volume | 对齐 Phase 33 实际用例；JSON omitempty 保兼容 | ✓ |
| B. 全 `--mount` 语义（type=volume/bind/tmpfs + options） | 过度设计，v3.0 只需 named volume | |
| C. 拆 `CreateVolumeOptions` + `MountSpec` 两结构 | 把 volume 生命周期塞进 Phase 29，越界 Phase 33 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** Phase 33 的 `claude-state-{account_id}` volume 是唯一消费者，最小契约即可覆盖。`omitempty` 保证 v2.0 旧控制面 / 旧 host-agent 不破。

---

## Area 4 — entrypoint 改造策略

| Option | Description | Selected |
|--------|-------------|----------|
| A. 沿用现 entrypoint 骨架 + SSH 启动前插入 v3 阶段 | 增量插入 `prepare_fuse` / `prepare_v3_dirs` / `prepare_mutagen_agent` / `prepare_mergerfs` + tini PID 1 | ✓ |
| B. 全重写为 `entrypoint-v3.sh` | 重写成本高，KasmVNC/Chromium 逻辑需要重新搬一遍 | |
| C. 双 entrypoint（env 切分） | 维护两套路径，未来升级需双写 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** Phase 17 CONTEXT.md 已经确立"entrypoint 快速失败 + 串行编排"模式；Phase 29 延续即可。v3 阶段的职责是**预置资源**（目录、agent tarball、FUSE），真正的 mount 动作由 cloud-claude 在 SSH 会话建立后按 `--mount-mode` 执行（Phase 31 边界）。

---

## Area 5 — host-preflight 与 AppArmor override 部署形式

| Option | Description | Selected |
|--------|-------------|----------|
| A. `deploy/host-preflight.sh` 独立脚本 + 打印修复命令 | 运维手动运行，Phase 34 doctor 可调用；不自动 sudo | ✓ |
| B. 嵌入控制面启动逻辑自动检测 | 控制面进程不能 sudo，不能真正 apply | |
| C. 合并到 Phase 34 doctor host 维度 | 部署时机晚于 v3.0 cloud-claude 首次连接，用户会先踩 AppArmor 坑 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** Ubuntu 25.04 的 AppArmor override 必须运维在宿主机准备阶段完成，不能延后到 cloud-claude 运行时发现。独立 `host-preflight.sh` + 运维手册是最低摩擦部署形式。Phase 34 doctor 会调用同一脚本做"既有部署是否健康"的 runtime 验证（边界复用而非功能重叠）。

---

## Area 6 — CI 镜像 ≤ 700MB gate 实现形式

| Option | Description | Selected |
|--------|-------------|----------|
| A. bash + `docker image inspect` 内联 step | 零第三方依赖；失败时自动打印 `docker history` | ✓ |
| B. GitHub Actions marketplace action（如 `ghcr.io/...`） | 引入第三方依赖，审计面扩大 | |
| C. dive / docker-slim 工具链 | 工具链复杂，分析价值 > 断言价值 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** BASE-04 只需"大于即 fail"的硬断言，bash 足够。`docker history` 自动输出把 Phase 35 二次回归与未来维护的排障路径一起固化。

---

## Area 7 — image.lock 扩展字段范围 + Q10 mergerfs branch 拓扑

| Option | Description | Selected |
|--------|-------------|----------|
| A. image.lock 追加 6 个 v3 能力字段 + mergerfs 2 路 branch + env 预留 3 路扩展 | 单一数据源 + Q10 折中（现在 2 路，保留扩展） | ✓ |
| B. 仅加 `image_version`，其它能力运行时探测 | Phase 30 Entry API 需要多次 SSH exec，慢 | |
| C. image-capabilities.yaml 单独文件 | Phase 30 未见超 6 字段需求，拆分为时过早 | |

**User's choice（auto）:** A — 推荐默认
**Notes:** Q10 在 REQUIREMENTS.md 标注需在 Phase 29 / 31 共同定稿。本阶段锁 2 路实现 + 环境变量扩展点，Phase 31 若发现"写入落盘需独立 overlay 层"的强需求再启用 3 路，**不改镜像**。

---

## Claude's Discretion

- tini 二进制安装方式（apt vs COPY 静态；倾向 apt）
- mergerfs `.deb` 下载的 checksum 校验实现（`sha256sum` 硬编码；GPG 按需）
- Dockerfile RUN 指令合并粒度（layer 数与 cache 的权衡由 planner 决定）
- CI gate 失败文案（保持 `::error::` 前缀即可）
- host-preflight.sh 在 non-Linux 宿主机（macOS / WSL）的行为（建议直接退出 0 + 提示信息）

## Deferred Ideas

- tmux 3.6a 升级路径（本阶段放宽为 ≥ 3.4，Phase 35 验收后回流）
- image.lock 拆分为 image-capabilities.yaml（Phase 30 需求量决定）
- host-preflight.sh `--apply` 自动修复模式（运维反馈后评估）
- mergerfs 3 路 branch（通过 env 预留，Phase 31 决策）
- arm64 真机验收（推迟到 Phase 35 或 v3.1）

---

## 修正记录 2026-04-18 — D-23 AppArmor override 路径

**背景：** `gsd-phase-researcher`（29-RESEARCH.md）通过 WebSearch 2026-04-18 实时验证时发现，D-23 最初指定的 `/etc/apparmor.d/local/docker-default` 路径与上游主流修复方式不一致。

**技术原因：** Ubuntu 25.04 在 fuse 3.14.0-10 中新增了独立的 `/etc/apparmor.d/fusermount3` profile，它才是在 25.04+ 下拦截 `capability dac_override` 的真正执行者。修改 `docker-default` 的 local override 无法影响这个独立 profile——按原 D-23 实现，host-preflight.sh 即使通过，容器内 FUSE 挂载（mergerfs / sshfs / mutagen）仍会报 `fusermount3: mount failed: Permission denied`，连 `--privileged` 也无法绕过。Critical Pitfall **C6** 的防御会实际失效。

**证据链：**
- Launchpad bug [#2111105](https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105)
- moby/moby#50013
- nestybox/sysbox#947
- containerd/stargz-snapshotter#2144

**候选方案：**

| Option | Description | Selected |
|--------|-------------|----------|
| A. 修正 D-23 路径为 `/etc/apparmor.d/local/fusermount3`，同步更新运维手册与 Success Criteria #7 断言 | 技术正确，单一路径，运维文档清晰 | ✓ |
| B. host-preflight.sh 双检测 `docker-default` + `fusermount3`，任一通过即 OK | 更保守但运维文档会歧义 | |
| C. 保留 D-23 原文，接受在 Ubuntu 25.04 真机上 C6 复现风险 | 等于放弃 C6 防御 | |

**User's choice（2026-04-18，显式澄清）:** A — 修正为 `fusermount3` 路径

**影响范围：**
- D-23：检测与修复命令路径改为 `/etc/apparmor.d/local/fusermount3`；`apparmor_parser -r /etc/apparmor.d/fusermount3`
- D-25：运维手册按新路径撰写
- Success Criteria #7（ROADMAP）：在 25.04 宿主机缺 override 时，host-preflight.sh 断言 `fusermount3` 文件内容，退出码 1 并打印正确修复命令
- 29-RESEARCH.md §host-preflight.sh 骨架已按 `fusermount3` 路径实现，planner 可直接消费

**Notes:** D-23 文本已在 29-CONTEXT.md 内就地修正并附修正说明；不删除修正历史，便于后续审计追溯。
