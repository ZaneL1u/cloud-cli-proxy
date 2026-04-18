# Phase 29: 受管镜像 v3 + Worker 容器参数扩展 — Research

**Researched:** 2026-04-18
**Domain:** 容器镜像基线（Ubuntu 24.04 base）+ FUSE / 同步工具链预置 + Go/docker 契约扩展 + 宿主机 AppArmor 预检
**Confidence:** HIGH（核心版本号与 URL 已 WebSearch 验证）
**Downstream consumer:** `gsd-planner`，据此生成本阶段 Sub-scope A..F 对应的 6 份 PLAN.md

---

## Summary

Phase 29 交付三件事：

1. **受管镜像 v3.0**：在现有 `deploy/docker/managed-user/Dockerfile` 上增量叠加 `tini`（apt）、`mergerfs 2.41.1`（GitHub release `.deb`）、`mutagen-agent v0.18.1`（GitHub release tarball）、`/etc/tmux.conf`、`/etc/profile.d/cloud-claude.sh`、预建 `/home/claude{,/.claude,/.cache/claude}` + `/workspace{-hot,-cold}` + `/var/lib/claude-persist` 并 `chown 1000:1000`；`sshd_config` 追加 KeepAlive / MaxSessions / MaxStartups。
2. **Worker 契约扩展**：`internal/agentapi/contracts.go` 新增 `VolumeMount` 类型与 `HostActionRequest.Volumes` 字段（`omitempty`），`internal/runtime/tasks/worker.go:createHost` 在 `docker create` 参数拼接处追加 `--mount type=volume,...`；本阶段不 `docker volume create`、不绑定生命周期（留给 Phase 33）。
3. **宿主机配套**：新增 `deploy/host-preflight.sh`（独立脚本）检测 Ubuntu 25.04 AppArmor 的 `fusermount3` profile 是否放开 `dac_override`；CI 新增 bash step 断言镜像未压缩体积 ≤ 700MB；`image.lock` 追加 `image_version / mergerfs_version / mutagen_agent_version / tmux_version_min / supports_mutagen / supports_mergerfs` 六字段。

**Primary recommendation:** 严格按 CONTEXT.md D-01..D-30 的决策执行，但需在实现前澄清 **Conflicts with CONTEXT.md** 一节中列出的 AppArmor override 路径冲突（`docker-default` vs `fusermount3`）——这是本研究发现的唯一与 CONTEXT.md 不一致的技术细节，planner 必须作为 Open Question 保留或与用户对齐。

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| 镜像 v3.0 组件预置（mergerfs / mutagen-agent / tini / tmux.conf） | 容器镜像 / Dockerfile | 构建期 BuildKit cache | 运行时一次性产物，不分发到宿主，不由 Go 控制面注入 |
| 容器启动期资源兜底（chown / mutagen agent extract / FUSE 准备） | 容器 entrypoint（bash） | — | SSH 建立前必须完成；控制面无法 `docker exec` 到 PID 1 之前 |
| `docker create --mount type=volume,...` 参数拼接 | Go 控制面 / worker | — | `HostActionRequest.Volumes` 是 agentapi 契约；worker 是唯一调用 `docker create` 的位置 |
| Volume 生命周期（create / rm）与账号绑定 | **Phase 33，不是本阶段** | — | 本阶段只做契约拼接，volume 必须"已存在"假设 |
| 宿主机 AppArmor override 检测 | `deploy/host-preflight.sh`（bash） | 运维手册 | 必须 sudo；控制面进程无权限；独立脚本可被 Phase 34 doctor 复用 |
| 镜像体积 gate（≤ 700MB） | CI workflow（GitHub Actions bash step） | `docker history` 排障输出 | build 脚本保持单一职责，gate 只在 CI |
| image.lock 元数据维护 | 构建期静态文件 | Phase 30 Entry API 读取 | 单一数据源，本阶段只写入 |

---

## User Constraints（from CONTEXT.md）

### Locked Decisions（D-01..D-30，必须严格遵守，research 不得另提替代）

**镜像演进路径**
- **D-01** 沿用 `deploy/docker/managed-user/Dockerfile` 做增量改造；**不**新建 v3 独立镜像目录。
- **D-02** Base image 保持 `ubuntu:24.04`；**不**升级到 25.04。
- **D-03** 启用 BuildKit cache mount + `--no-install-recommends` + 合并 RUN；镜像未压缩体积硬约束 ≤ 700MB；超标时优先裁剪 Chromium recommends / fonts，**不**裁剪 mergerfs / mutagen-agent / tmux。

**二进制来源 / 版本**
- **D-04** mergerfs 2.41.1：从 `trapexit/mergerfs` GitHub release 下载 `mergerfs_2.41.1.ubuntu-noble_<arch>.deb`，`dpkg -i` 安装；禁止 apt repo。支持 amd64 + arm64。
- **D-05** mutagen-agent v0.18.1：从 `mutagen-io/mutagen` GitHub release 下载 `mutagen_linux_<arch>_v0.18.1.tar.gz`，解压后仅保留 `mutagen-agents.tar.gz` 预放到 `/opt/mutagen-agents.tar.gz`；**runtime 由 Phase 31 触发 agent extract**。
- **D-06** tmux 使用 ubuntu:24.04 apt 仓库版本（3.4 系列）；entrypoint 启动时断言 `tmux -V` ≥ `3.4`；**放宽下限到 3.4**，不追 3.6a PPA。
- **D-07** 构建期写入版本元数据：
  - `/etc/cloud-claude/mutagen.version` ← `v0.18.1`
  - `/etc/cloud-claude/mergerfs.version` ← `2.41.1`
  - `/etc/cloud-claude/tmux.version` ← 运行时 `tmux -V` 回填（构建期可留占位）
- **D-08** libfuse3 采用 ubuntu:24.04 apt 提供的 `libfuse3-3` / `fuse3`；不引入额外 PPA。

**Entrypoint 改造**
- **D-09** 沿用现 entrypoint 骨架，在 `exec /usr/sbin/sshd -D -e` **之前**串行插入：`prepare_fuse` → `prepare_v3_dirs`（二次 `chown`） → `prepare_mutagen_agent`（extract `/opt/mutagen-agents.tar.gz` 到 `/usr/local/libexec/mutagen/agents/`） → `prepare_mergerfs`（仅校验 `mergerfs --version`，**不挂载**） → `exec sshd`。
- **D-10** PID 1 改为 tini；`ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`；tini 通过 `apt-get install -y --no-install-recommends tini` 安装。
- **D-11** mergerfs 挂载参数固定（镜像文档与 host-preflight 与 Phase 31 保持一致）：`category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash`。
- **D-12** mergerfs branch 本阶段锁定 **2 路**：`/workspace-hot=RW:/workspace-cold=NC,RO`；`CLOUD_CLAUDE_MERGERFS_BRANCHES` 环境变量预留 3 路扩展点（未设置时默认 2 路）。
- **D-13** 新增 `/etc/tmux.conf`（容器级默认）：`set -ga terminal-overrides ",*:RGB"` / `set -g window-size latest` / `set -g aggressive-resize on` / `set -g history-limit 50000`；新增 `/etc/profile.d/cloud-claude.sh` 导出 `CLAUDE_CODE_TMUX_TRUECOLOR=1`。
- **D-14** `sshd_config` 追加 `ClientAliveInterval 15` / `ClientAliveCountMax 8` / `MaxSessions 30` / `MaxStartups 60:30:120`；保留现 `PasswordAuthentication yes` / `UsePAM no`。
- **D-15** 容器内**不**启动 systemd / systemd-logind；tini 仅作 PID 1 收割僵尸进程。

**预建目录与用户**
- **D-16** Dockerfile 预建并 `chown 1000:1000`：`/home/claude` / `/home/claude/.claude` / `/home/claude/.cache/claude` / `/workspace-hot` / `/workspace-cold` / `/workspace` / `/var/lib/claude-persist`。
- **D-17** 不重命名、不替换现 `workspace` 用户（UID 1000 / GID 1000）；`/home/claude` 目录属主仍为 UID 1000（名称是 convention，不新建用户）。

**HostActionRequest.Volumes 契约**
- **D-18** `internal/agentapi/contracts.go` 新增 `VolumeMount{Name, Target, ReadOnly, Labels}` + `HostActionRequest.Volumes []VolumeMount \`json:"volumes,omitempty"\``。
- **D-19** worker `createHost` 遍历 `request.Volumes`，追加 `--mount type=volume,src=<Name>,dst=<Target>[,readonly]`；**不**追加 volume label 到容器。
- **D-20** 本阶段 worker **不**调用 `docker volume create`（Phase 33 负责幂等工具函数 `ensureDockerVolume`）；若 volume 不存在则正常返回 `host_action_failed`。
- **D-21** `ClaudeAccountID` 字段**不**在本阶段加。
- **D-22** `Volumes` 字段 `omitempty`；v2.0 旧 agent / 控制面未升级时 JSON 不含该字段，worker 路径不变。

**host-preflight 与 AppArmor**
- **D-23** 新增 `deploy/host-preflight.sh`：检测 Ubuntu 25.04+ 时校验 `/etc/apparmor.d/local/docker-default` 含 `capability dac_override,`；缺失时退出码 `1` 打印修复命令（**不自动 sudo**）；非 Ubuntu 25.04 退出码 `0`。
  - ⚠️ 本研究发现 **`/etc/apparmor.d/local/docker-default` 与上游主流修复路径不一致**，详见 "Conflicts with CONTEXT.md" 章节。
- **D-24** 独立脚本，**不**嵌入控制面启动流程。
- **D-25** 运维手册 `docs/`（或 `deploy/README.md`）新增 "v3.0 AppArmor override 部署" 一节。

**image.lock 扩展字段**
- **D-26** 追加 YAML 字段（不破坏现有顺序）：`image_version: v3.0.0` / `mergerfs_version: 2.41.1` / `mutagen_agent_version: v0.18.1` / `tmux_version_min: "3.4"` / `supports_mutagen: true` / `supports_mergerfs: true`。
- **D-27** image.lock 是 Phase 30 Entry API 扩展的单一上游；本阶段仅写入。

**CI 镜像体积 gate（BASE-04）**
- **D-28** CI workflow 新增 bash + `docker image inspect` 内联 step；失败时自动输出 `docker history`。
- **D-29** 失败日志格式固定 `::error::`。
- **D-30** `deploy/docker/managed-user/build-managed-image.sh` 不嵌入体积检查。

### Claude's Discretion（planner / executor 可决）

- tini 二进制是否 apt 或 COPY 静态（优先 apt）
- mergerfs `.deb` 的 checksum 校验（建议 `sha256sum` 硬编码）
- mutagen-agents tarball 的解压位置细节
- Dockerfile RUN 合并粒度
- CI gate 具体文案（保持 `::error::` 前缀）
- host-preflight.sh 在非 Linux 宿主机（macOS / WSL）行为（建议直接退出 0）

### Deferred Ideas（OUT OF SCOPE，不碰）

- tmux 3.6a 升级路径（Phase 35 决定）
- image.lock 拆分为 image-capabilities.yaml（Phase 30 决定）
- host-preflight.sh `--apply` 自动修复
- mergerfs 3 路 branch（通过 env 预留，Phase 31 决策）
- arm64 真机验收（Phase 35 或 v3.1）

---

## Phase Requirements

Phase 29 **无 user-facing REQ-F\* 映射**（所有 F1..F8 行为由 Phase 30/31/32/33 在镜像基础上实现）。Phase 29 的验收基线是以下 3 类硬约束：

| 约束 ID | 来源 | Research Support |
|---------|------|------------------|
| BASE-04 | REQUIREMENTS.md §性能与体验验收基线 | §Standard Stack / §镜像体积估算 章节提供裁剪候选；§Validation 三列表给出 `docker image inspect --format='{{.Size}}'` 断言 |
| Critical Pitfalls C1 / C2 / C3 / C5 / C6 / C7 | REQUIREMENTS.md §Critical Pitfalls | §Don't Hand-Roll + §Common Pitfalls 逐条给防御策略；每条映射到 D-11 / D-12 / D-14 / D-15 / D-16 / D-23 |
| Critical Pitfalls M3 / M4 / M7 / M8 / M12 / M17 / M18 | REQUIREMENTS.md §Critical Pitfalls | §Standard Stack / §Code Examples / §Common Pitfalls 给出具体配置片段 |
| Q10 决议 | REQUIREMENTS.md §Open Questions | D-12 已拍板 2 路 branch + `CLOUD_CLAUDE_MERGERFS_BRANCHES` env 扩展点，Research §10 给出语法与读取位置 |

Success Criteria（ROADMAP §Phase 29 / CONTEXT §Goal，7 条运行时断言）在本研究 §Validation Architecture 三列表中逐条展开为"文件改动 → 静态断言 → 运行时断言"。

---

## Sub-scope 映射（planner 消费入口）

CONTEXT.md 未显式列 Sub-scope 字母；本研究按 Phase 工作主线拆成 6 个物理文件域，供 planner 切分 6 份 plan：

| Sub-scope | 文件改动域 | 对应 CONTEXT 决策 | 研究章节 |
|-----------|-----------|-------------------|---------|
| **A** 镜像构建基线 | Dockerfile / BuildKit cache / apt 清单 / tini / chown | D-01..D-03, D-08, D-10, D-16, D-17 | §1 / §3 / §6 / §7 |
| **B** 二进制预置 | mergerfs `.deb` / mutagen tarball / `/etc/cloud-claude/*.version` | D-04, D-05, D-07 | §1 / §2 |
| **C** entrypoint & 配置 | entrypoint.sh v3 阶段 / sshd_config / `/etc/tmux.conf` / `/etc/profile.d/cloud-claude.sh` | D-06, D-09, D-11, D-13, D-14, D-15 | §4 |
| **D** agentapi & worker 契约 | `internal/agentapi/contracts.go` / `internal/runtime/tasks/worker.go:createHost` + round-trip test | D-18..D-22 | §8 / §9 |
| **E** host-preflight & 文档 | `deploy/host-preflight.sh` / `deploy/README.md` or `docs/` | D-23..D-25 | §5 |
| **F** image.lock + CI gate | `image.lock` YAML 扩展 / GitHub Actions workflow step | D-26..D-30 | §6 / §Validation |

---

## Standard Stack

### Core（必须使用这些版本 / URL / 来源）

| 组件 | 版本 | 来源 URL（VERIFIED via WebSearch 2026-04-18） | 用途 |
|------|------|--------------------------------------------|------|
| mergerfs | 2.41.1 | `https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_{amd64,arm64}.deb` | 联合视图 FS（镜像预装，Phase 31 挂载） |
| Mutagen | v0.18.1 | `https://github.com/mutagen-io/mutagen/releases/download/v0.18.1/mutagen_linux_{amd64,arm64}_v0.18.1.tar.gz` | 热同步 agent tarball 预置 |
| tini | apt default | `apt-get install -y --no-install-recommends tini` → `/usr/bin/tini` | PID 1 僵尸收割 + 信号转发 |
| tmux | `3.4-1ubuntu0.1` | apt ubuntu-noble main | 会话包装（镜像默认 `/etc/tmux.conf`） |
| libfuse3 | `3.16.x` / `fuse3` 3.14+ | apt ubuntu-noble | mergerfs + sshfs + mutagen FUSE 依赖 |
| sshfs | 3.7.x | apt ubuntu-noble（现 Dockerfile 已装） | 冷 branch 兜底（Phase 31 挂载） |
| Ubuntu base | `ubuntu:24.04` | Docker Hub | 保持不变（D-02） |

**验证的 checksum（来自 wakemeops 公开 lock，可作 Dockerfile 硬编码起点；正式入库前 executor 需 `curl + sha256sum` 在 build 环境二次确认）：**

```
mutagen_linux_amd64_v0.18.1.tar.gz  sha256: 7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f
mutagen_linux_arm64_v0.18.1.tar.gz  sha256: bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a
```

mergerfs `.deb` 的 SHA256 GitHub release 页未直接列出；**executor 构建时必须 `curl -fsSL -o /tmp/mergerfs.deb $URL && sha256sum /tmp/mergerfs.deb` 生成并硬编码到 Dockerfile**（标注为"需 executor 首次构建实际验证"）。

### Alternatives Considered（CONTEXT 已拍板，此处仅备忘，不得偏离）

| Instead of | Could Use | Why Not | CONTEXT Ref |
|------------|-----------|---------|-------------|
| mergerfs GitHub `.deb` | apt `mergerfs` 仓库版本 | Ubuntu 24.04 apt 仓库版本 2.33.5，缺 `func.readdir=cor`、`inodecalc=path-hash` 等 v3.0 必需能力 | D-04, PITFALLS M3 |
| mergerfs 2 路 branch | 3 路（含本地 overlay） | Q10 已决议 2 路 + env 预留；实现 3 路会放大 Phase 29 → Phase 31 范围交叉 | D-12 |
| tmux 3.6a PPA / 源码 | apt 3.4 | 维护成本过高；3.4 已支持 `set -ga terminal-overrides ",*:RGB"` + `set -g window-size latest` | D-06 |
| apt tini | COPY static tini | apt 版本足够且更小；与现 Dockerfile 风格一致 | Claude's Discretion |
| 单独 v3 镜像目录 | 增量改造现镜像 | 会破坏 v1.2 deferred KasmVNC / Chromium 用户面 | D-01 |

---

## Architecture Patterns

### System Architecture Diagram

```
┌──────────────────────────────── build time ────────────────────────────────┐
│                                                                             │
│  Dockerfile (ubuntu:24.04)                                                  │
│    │                                                                        │
│    ├─ RUN --mount=type=cache,target=/var/cache/apt,sharing=locked           │
│    │     apt-get install -y --no-install-recommends                         │
│    │       {existing v2 packages} tini {libfuse3 ensure}                    │
│    │                                                                        │
│    ├─ RUN curl .../mergerfs_2.41.1.ubuntu-noble_${ARCH}.deb                 │
│    │     && sha256sum -c  && dpkg -i /tmp/mergerfs.deb                      │
│    │                                                                        │
│    ├─ RUN curl .../mutagen_linux_${ARCH}_v0.18.1.tar.gz                     │
│    │     && tar xzf && mv mutagen-agents.tar.gz /opt/                       │
│    │                                                                        │
│    ├─ COPY entrypoint.sh / sshd_config / tmux.conf / cloud-claude.sh        │
│    │                                                                        │
│    ├─ RUN mkdir -p /home/claude/{.claude,.cache/claude} /workspace{-hot,    │
│    │     -cold} /var/lib/claude-persist                                     │
│    │     && chown -R 1000:1000 /home/claude /workspace{-hot,-cold} ...      │
│    │                                                                        │
│    ├─ RUN echo "v0.18.1"  > /etc/cloud-claude/mutagen.version               │
│    │     echo "2.41.1"  > /etc/cloud-claude/mergerfs.version                │
│    │                                                                        │
│    └─ ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]    │
│                                                                             │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────── CI gate (BASE-04) ──────────────────────────────┐
│  SIZE=$(docker image inspect --format='{{.Size}}' $IMAGE_NAME)              │
│  (( SIZE > 700*1024*1024 )) && { docker history ... ; exit 1 ; }            │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
                                      ▼
┌───────────────────────── host-agent worker (createHost) ───────────────────┐
│  args := []string{"create", "--name", ..., ..., "-v", bindMount}            │
│  for _, vm := range request.Volumes {                                       │
│      opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)   │
│      if vm.ReadOnly { opts += ",readonly" }                                 │
│      args = append(args, "--mount", opts)                                   │
│  }                                                                          │
│  args = append(args, labelsAndEnv..., imageName)                            │
│  runDocker(ctx, args...)      ← 不做 docker volume create（Phase 33 职责）  │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
                                      ▼
┌───────────────────── runtime: tini PID 1 → entrypoint.sh ──────────────────┐
│  PID 1 = /usr/bin/tini (zombie reap + signal forward)                       │
│    └─ /usr/local/bin/entrypoint.sh (existing skeleton + v3 stages)          │
│         ├─ existing: ssh-keygen, chpasswd, chown /workspace, ipv6 off,      │
│         │            /dev/fuse chmod, kasmvnc                                │
│         ├─ prepare_v3_dirs  : chown -R 1000:1000 /home/claude /workspace-*  │
│         ├─ prepare_mutagen_agent : tar xzf /opt/mutagen-agents.tar.gz       │
│         │                     -C /usr/local/libexec/mutagen/agents/         │
│         ├─ prepare_mergerfs : mergerfs --version > /dev/null || exit 1      │
│         ├─ assert: [[ "$(tmux -V | awk '{print $2}')" =~ ^3\.[4-9] ]]       │
│         └─ exec /usr/sbin/sshd -D -e                                        │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
                                      ▼
┌───────────────── operator (optional, sudo required) ───────────────────────┐
│  deploy/host-preflight.sh                                                   │
│    ├─ . /etc/os-release && case "$VERSION_ID" in                            │
│    │    25.04|25.10) check_apparmor_override ; exit $? ;;                   │
│    │    *)           echo "no override needed" ; exit 0 ;;                  │
│    │    esac                                                                │
│    └─ (see §5 for override path conflict)                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Recommended Dockerfile Skeleton（骨架，executor 补全）

```dockerfile
# syntax=docker/dockerfile:1.7
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/workspace
ENV WORKSPACE_USER=workspace
ENV WORKSPACE_UID=1000
ENV WORKSPACE_GID=1000

# 1) BuildKit cache: 禁掉 docker-clean 让 cache mount 生效
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' \
        > /etc/apt/apt.conf.d/keep-cache

# 2) v2 + v3 apt 合并安装（注意 tini 新增）
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server bash zsh curl git tmux sudo ca-certificates jq \
        procps iproute2 nodejs npm locales \
        fluxbox pcmanfm dbus-x11 \
        fonts-liberation fonts-noto-cjk \
        xdg-utils xclip xsel gnupg libegl1 libgl1 \
        x11-utils x11-xserver-utils xterm \
        sshfs fuse3 libfuse3-3 \
        tini

# 3) FUSE user_allow_other（沿用 v2）
RUN sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf \
    && chmod a+r /etc/fuse.conf

# 4) KasmVNC / Chromium 沿用 v2（略 - 保留现 RUN 块不动）

# 5) mergerfs 2.41.1（GitHub release, 双架构）
ARG MERGERFS_VERSION=2.41.1
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL -o /tmp/mergerfs.deb \
        "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb" \
    && echo "${MERGERFS_SHA256_${ARCH}} /tmp/mergerfs.deb" | sha256sum -c - \
    && dpkg -i /tmp/mergerfs.deb \
    && rm /tmp/mergerfs.deb \
    && mergerfs --version

# 6) Mutagen agent v0.18.1（仅保留 mutagen-agents.tar.gz）
ARG MUTAGEN_VERSION=v0.18.1
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL -o /tmp/mutagen.tar.gz \
        "https://github.com/mutagen-io/mutagen/releases/download/${MUTAGEN_VERSION}/mutagen_linux_${ARCH}_${MUTAGEN_VERSION}.tar.gz" \
    && echo "${MUTAGEN_SHA256_${ARCH}} /tmp/mutagen.tar.gz" | sha256sum -c - \
    && mkdir -p /opt \
    && tar -xzf /tmp/mutagen.tar.gz -C /tmp mutagen-agents.tar.gz \
    && mv /tmp/mutagen-agents.tar.gz /opt/mutagen-agents.tar.gz \
    && rm /tmp/mutagen.tar.gz

# 7) 版本元数据
RUN mkdir -p /etc/cloud-claude \
    && echo "${MUTAGEN_VERSION}" > /etc/cloud-claude/mutagen.version \
    && echo "${MERGERFS_VERSION}" > /etc/cloud-claude/mergerfs.version \
    && echo "runtime-filled" > /etc/cloud-claude/tmux.version

# 8) locale / 用户（沿用 v2，略）

# 9) 预建目录 + chown（C5 / M17 防御 + REQ-F7 兼容）
RUN mkdir -p \
        /home/claude/.claude \
        /home/claude/.cache/claude \
        /workspace-hot \
        /workspace-cold \
        /var/lib/claude-persist \
    && chown -R ${WORKSPACE_UID}:${WORKSPACE_GID} \
        /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist

# 10) tmux.conf + profile.d（M7 / M8 防御）
COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf
COPY deploy/docker/managed-user/cloud-claude.sh /etc/profile.d/cloud-claude.sh
RUN chmod 0644 /etc/tmux.conf /etc/profile.d/cloud-claude.sh

# 11) sshd_config / entrypoint / 其它 COPY（沿用 v2）
COPY deploy/docker/managed-user/sshd_config /etc/ssh/sshd_config
COPY deploy/docker/managed-user/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /workspace
EXPOSE 22 6080

# 12) tini 作 PID 1（C7 + M4 防御）
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
```

### entrypoint.sh v3 阶段骨架（插入位置：现 `exec /usr/sbin/sshd -D -e` 之前）

```bash
#!/usr/bin/env bash
set -euo pipefail

log() { echo "[entrypoint] $*"; }

# ====== 现有 v2.0 阶段保持不动：ssh-keygen, user rename, chpasswd,
#        chown /workspace, ipv6 off, /dev/fuse chmod, kasmvnc 配置,
#        Xvnc 启动, fluxbox 启动, chromium 预热 ======

# ====== v3 stages（D-09）======

prepare_v3_dirs() {
  log "v3: chown -R 1000:1000 /home/claude /workspace-hot /workspace-cold"
  chown -R "${WORKSPACE_UID:-1000}:${WORKSPACE_GID:-1000}" \
    /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist \
    2>/dev/null || true
}

prepare_mutagen_agent() {
  local src=/opt/mutagen-agents.tar.gz
  local dest=/usr/local/libexec/mutagen/agents
  if [[ ! -f "$src" ]]; then
    log "v3: mutagen agent tarball missing at $src — FATAL"
    exit 1
  fi
  mkdir -p "$dest"
  # 幂等：已展开则跳过（避免重复 CPU）
  if [[ ! -f "$dest/.extracted" ]]; then
    tar -xzf "$src" -C "$dest"
    touch "$dest/.extracted"
  fi
  log "v3: mutagen agents ready at $dest"
}

prepare_mergerfs_check() {
  if ! command -v mergerfs >/dev/null 2>&1; then
    log "v3: mergerfs binary missing — FATAL"
    exit 1
  fi
  local ver
  ver=$(mergerfs --version 2>&1 | head -n1 || true)
  log "v3: mergerfs available ($ver) — mount deferred to cloud-claude (Phase 31)"
}

assert_tmux_version() {
  local tmux_ver
  tmux_ver=$(tmux -V 2>/dev/null | awk '{print $2}' || true)
  case "$tmux_ver" in
    3.4*|3.5*|3.6*|3.7*|3.8*|3.9*|[4-9].*)
      log "v3: tmux ${tmux_ver} >= 3.4 ok"
      # 回填 tmux 运行时版本
      echo "$tmux_ver" > /etc/cloud-claude/tmux.version
      ;;
    *)
      log "v3: tmux version ${tmux_ver} < 3.4 — FATAL"
      exit 1
      ;;
  esac
}

# Order matters — 串行 + 快速失败（M4 防御）
prepare_v3_dirs
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version

# 最后：exec sshd（保持为 foreground，接收 tini 转发信号）
exec /usr/sbin/sshd -D -e
```

### tmux.conf（独立文件，复制到 /etc/tmux.conf）

```tmux
# v3.0 baseline — PITFALLS M7 / M8 defense
set -ga terminal-overrides ",*:RGB"
set -g  window-size latest
set -g  aggressive-resize on
set -g  history-limit 50000
```

### /etc/profile.d/cloud-claude.sh

```sh
#!/bin/sh
# v3.0 baseline — Claude Code truecolor hint (PITFALLS M8)
export CLAUDE_CODE_TMUX_TRUECOLOR=1
```

### sshd_config 追加片段（D-14）

```
# v3.0 baseline — PITFALLS M12 / REQ-F3-A 服务端基线
ClientAliveInterval 15
ClientAliveCountMax 8
MaxSessions 30
MaxStartups 60:30:120
```

### Go 契约 diff 示意

**`internal/agentapi/contracts.go`（新增）：**

```go
// VolumeMount 描述 docker create --mount type=volume 的最小契约。
// Phase 29 仅支持 named volume；生命周期（create/rm）由 Phase 33 管理。
type VolumeMount struct {
	Name     string            `json:"name"`
	Target   string            `json:"target"`
	ReadOnly bool              `json:"read_only,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"` // 审计用，不写入容器
}

type HostActionRequest struct {
	// ... 现有字段不动 ...
	SSHKeys []SSHKeyEntry `json:"ssh_keys,omitempty"`

	// v3.0 新增
	Volumes []VolumeMount `json:"volumes,omitempty"`
}
```

**`internal/runtime/tasks/worker.go:createHost`（插入点：现 `-v bind` 之后、`Labels` 循环之前）：**

```go
// ... 现 "-v <homeDir>:<homeMount>" 追加之后 ...

for _, vm := range request.Volumes {
	if vm.Name == "" || vm.Target == "" {
		return fmt.Errorf("invalid volume mount: name=%q target=%q", vm.Name, vm.Target)
	}
	opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)
	if vm.ReadOnly {
		opts += ",readonly"
	}
	args = append(args, "--mount", opts)
}

for key, value := range request.Labels {
	args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
}
```

**Round-trip 测试骨架（延续 `ssh_inject_test.go` 风格，新增独立测试文件 `worker_volume_test.go` 或合并到 contracts 测试）：**

```go
func TestHostActionRequest_VolumesOmitempty(t *testing.T) {
	// 空 Volumes 不出现在 JSON（D-22 向后兼容）
	req := HostActionRequest{TaskID: "t1", Action: ActionCreateHost}
	buf, _ := json.Marshal(req)
	if strings.Contains(string(buf), "volumes") {
		t.Fatalf("empty Volumes must be omitempty, got: %s", buf)
	}

	// 非空则出现
	req.Volumes = []VolumeMount{{Name: "claude-state-abc", Target: "/var/lib/claude-persist"}}
	buf, _ = json.Marshal(req)
	if !strings.Contains(string(buf), `"volumes"`) {
		t.Fatalf("non-empty Volumes must serialize, got: %s", buf)
	}

	// 旧 agent 反序列化新 JSON 不破
	var parsed HostActionRequest
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if len(parsed.Volumes) != 1 || parsed.Volumes[0].Name != "claude-state-abc" {
		t.Fatalf("round-trip lost data: %+v", parsed)
	}
}
```

### host-preflight.sh 骨架（D-23）

```bash
#!/usr/bin/env bash
set -euo pipefail

# deploy/host-preflight.sh — v3.0 宿主机预检
# 检测 Ubuntu 25.04+ AppArmor override；非 Linux / 非 25.04 直接 exit 0。

os_id=""; os_ver=""
if [[ -r /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  os_id="${ID:-}"; os_ver="${VERSION_ID:-}"
fi

case "$(uname -s 2>/dev/null || echo unknown):${os_id}" in
  Linux:ubuntu) ;;  # 继续
  *)
    echo "[preflight] host is not Ubuntu Linux (uname=$(uname -s) id=${os_id}); no override needed"
    exit 0
    ;;
esac

# Ubuntu <= 24.04 无需 override（没有 /etc/apparmor.d/fusermount3 profile）
case "$os_ver" in
  22.04|23.*|24.*|24.04)
    echo "[preflight] Ubuntu ${os_ver} does not ship AppArmor fusermount3 profile — override not required"
    exit 0
    ;;
  25.04|25.10|26.*)
    :  # 继续检测
    ;;
  *)
    echo "[preflight] unknown Ubuntu version ${os_ver} — defaulting to 'override required'"
    ;;
esac

# ⚠️ 路径冲突：CONTEXT D-23 写 /etc/apparmor.d/local/docker-default，但上游社区修复是
# /etc/apparmor.d/local/fusermount3。详见 RESEARCH §Conflicts with CONTEXT.md。
# planner 必须先与用户对齐。下方代码按 "fusermount3 路径 + CONTEXT 兼容" 双写。

required_cap="capability dac_override,"
local_override_path="/etc/apparmor.d/local/fusermount3"
ok=0

if [[ -f "$local_override_path" ]] && grep -qF "$required_cap" "$local_override_path"; then
  ok=1
fi

if (( ok == 1 )); then
  echo "[preflight] AppArmor fusermount3 override present — OK"
  exit 0
fi

cat <<EOF >&2
[preflight] AppArmor override missing on Ubuntu ${os_ver}.
[preflight] Docker containers with FUSE (mergerfs/sshfs/mutagen) will hit
[preflight] "fusermount3: mount failed: Permission denied".
[preflight]
[preflight] Fix:
  sudo tee -a /etc/apparmor.d/local/fusermount3 <<'OVERRIDE'
  capability dac_override,
  OVERRIDE
  sudo apparmor_parser -r /etc/apparmor.d/fusermount3
[preflight]
[preflight] Verify: /etc/apparmor.d/local/fusermount3 must contain the capability line above.
EOF
exit 1
```

### CI gate bash 片段（D-28..D-30）

```yaml
- name: Assert managed-user image size ≤ 700MB (BASE-04)
  shell: bash
  run: |
    set -euo pipefail
    IMAGE_NAME="$(awk -F': ' '$1 == "local_dev_image_name" { print $2 }' \
      deploy/docker/managed-user/image.lock)"
    SIZE=$(docker image inspect --format='{{.Size}}' "$IMAGE_NAME")
    LIMIT=$((700 * 1024 * 1024))
    if (( SIZE > LIMIT )); then
      echo "::error::image size ${SIZE} bytes exceeds BASE-04 limit ${LIMIT} bytes"
      docker history "$IMAGE_NAME" --no-trunc --format "table {{.Size}}\t{{.CreatedBy}}"
      exit 1
    fi
    echo "image size ${SIZE} bytes (limit ${LIMIT} bytes) — PASS"
```

---

## Don't Hand-Roll

| 问题 | 不要自建 | 使用 | 原因 |
|------|----------|------|------|
| PID 1 / zombie reap / SIGTERM 转发 | 自写 bash trap + wait 循环 | `tini`（apt 包） | M4 / C7 防御；bash trap 处理不了复杂 fork 树 |
| mergerfs 安装 | 从源码编译 / apt 版本 | GitHub release `.deb` | apt 版本 2.33.5 缺关键参数（M3） |
| Mutagen agent 多架构分发 | 自行拆分 / 重打包 | 官方 `mutagen-agents.tar.gz` | daemon 严格版本匹配 agent；自己切片会 handshake 失败（C4） |
| BuildKit cache 清理 | 自己 `apt-get clean` + `rm -rf` 的循环 | `--mount=type=cache,sharing=locked` + 移除 `docker-clean` | M18；减少 layer 字节数 |
| 文件所有权兜底 | 依赖 Dockerfile 单次 `chown` | Dockerfile `chown` + entrypoint 再 `chown` 双保险 | M17；named volume 首次挂载会继承镜像权限但 runtime 可能已早期生成 |
| Ubuntu 25.04 AppArmor 绕开 | 禁用全局 AppArmor / `privileged` 容器 | `/etc/apparmor.d/local/fusermount3` 追加 `capability dac_override,` + `apparmor_parser -r` | C6；privileged 不能绕过（已验证：bug #2111105）；全局禁用有安全代价 |
| docker volume 生命周期 | 本阶段自行 `docker volume create` | **Phase 33 的 `ensureDockerVolume`**（待实现） | D-20 明确划界 |
| JSON 向后兼容 | 自行版本化 schema | `json:"volumes,omitempty"` + Go encoding/json 默认忽略未知字段 | 延续 `SSHKeys` 已有模式（代码 §Reusable Assets） |

**Key insight:** v3.0 镜像的每个"新组件"都有成熟替代在上游；从 CONTEXT.md 到 STACK.md 的多道决策已经把技术选型锁死，research 的工作是**把官方推荐路径精确到 URL + 版本 + checksum + 命令**，而不是再找第二条路。

---

## Runtime State Inventory

Phase 29 是**镜像升级 + Go 契约扩展**，涉及重建容器与升级镜像，需要列出现有运行时状态的处置：

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data（容器内） | 现有 v2.0 容器里的 `/workspace` bind mount 下用户数据 | **None** — v3 镜像保持 `/workspace` 挂载点（D-17），用户文件不动；仅新增 `/workspace-hot` / `/workspace-cold` 空目录，由 Phase 31 挂载层消费 |
| Live service config | `image.lock` 现有字段（`image_name` / `base_image` / `ssh_port` / `home_mount` / `default_user` / `rebuild_mode_default` / `factory_reset_mode`） | 追加 6 字段，**不改现有字段顺序**（D-26）；Phase 30 / 33 读取 |
| OS-registered state | 宿主机 systemd / pm2 / launchd 服务 | **None** — Phase 29 不新增宿主机服务；`deploy/host-preflight.sh` 为独立脚本（D-24），由运维按需执行 |
| Secrets / env vars | 现 entrypoint 读取 `CONTAINER_USER` / `CONTAINER_SSH_PASSWORD` 等 | **None** — v3 新增的 env 变量 `CLAUDE_CODE_TMUX_TRUECOLOR=1`（profile.d 导出）与 `CLOUD_CLAUDE_MERGERFS_BRANCHES`（预留，未来才读）均为新增，不改现有语义 |
| Build artifacts / 已部署镜像 | GHCR 中已 ship 的 `v2.0.x` 镜像；开发者本地 `docker image ls` 缓存 | `image.lock` 中 `image_version: v3.0.0` 显式新 tag（D-26）；旧 v2 容器**运行中不受影响**（Phase 30..33 负责渐进推滚） |
| Docker named volumes | 现宿主机无 `claude-state-*` volumes（Phase 33 才引入） | **None** — 本阶段 worker 假设"若 Volumes 传入则 volume 已存在"（D-20）；现阶段控制面无调用方，契约就绪等待 Phase 33 消费 |

**Canonical question — "After every file in the repo is updated, what runtime systems still have the old string cached, stored, or registered?"**

回答：**仅 GHCR 中已 ship 的 v2 镜像 tag 会延续（这是预期行为，不是要清理的状态）**；生产容器会在 Phase 30 的 controlled rollout 中重建。无其它滞留状态。

---

## Common Pitfalls（对 C1 / C2 / C3 / C5 / C6 / C7 + M3 / M4 / M7 / M8 / M12 / M17 / M18 的防御映射）

### C1 — mergerfs 默认 readdir 串行（90s ls）
**本阶段防御：** Dockerfile 固化 `/etc/cloud-claude/mergerfs.version = 2.41.1`；镜像 README / host-preflight 文档写死参数字符串 `category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash`（D-11）。实际挂载由 Phase 31 执行。
**断言（Phase 31 消费）：** `mount | grep mergerfs | grep -E 'func\.readdir=cor:4.*cache\.readdir=true'`

### C2 — mergerfs 默认 `category.create=pfrd` 随机落盘
**本阶段防御：** 在镜像文档与 Phase 31 研究共识中固定 `category.create=ff`（first-found，D-11）；2.41.x 起 `ff` 是显式必填——不依赖默认。
**断言：** `getfattr -n user.mergerfs.category.create /workspace` == `ff`

### C3 — sshfs 抖动级联 mergerfs 挂死
**本阶段防御：** sshfs 在镜像中已装（v2 遗留），本阶段**不改** sshfs；参数由 Phase 31 在 cloud-claude 端传入 `ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,reconnect,-o auto_cache`。镜像层职责仅是保证 `sshfs -V` ≥ 3.7.x。

### C5 — Mutagen 首次同步清空 /workspace（非 root 用户权限冲突）
**本阶段防御：** 
1. Dockerfile 预建 `/home/claude{,/.claude,/.cache/claude}` 并 `chown -R 1000:1000`（D-16）
2. entrypoint `prepare_v3_dirs` 二次 `chown`（兜底 named volume 首挂）
3. `/workspace-hot` / `/workspace-cold` 同样预建 + chown
**断言：** `docker exec <c> stat -c '%u:%g' /home/claude/.claude` == `1000:1000`

### C6 — Ubuntu 25.04 AppArmor 阻断嵌套 FUSE
**本阶段防御：** `deploy/host-preflight.sh` 检测 25.04+ 宿主机；缺 override 时退出 1 并打印修复命令（D-23..D-25）。注意 **CONTEXT 中标的 override 路径需按本 research §Conflicts 一节确认**。

### C7 — systemd-logind 杀掉 tmux server
**本阶段防御：** 容器内**不**启动 systemd / systemd-logind（D-15）；tini 作 PID 1，不经 logind 管 SSH session（D-10）。
**断言：** `docker exec <c> ps -o pid,comm -p 1 | awk 'NR==2{print $2}'` == `tini`；`docker exec <c> pgrep systemd-logind` 返回空 + exit=1

### M3 — Debian/Ubuntu 源 mergerfs 版本太旧
**本阶段防御：** D-04 强制 GitHub release `.deb`，Dockerfile 硬编码 SHA256；CI 镜像构建日志里应包含 `mergerfs --version` 输出行。
**断言：** `grep -R "2\.41\.1" /etc/cloud-claude/mergerfs.version`

### M4 — entrypoint 顺序错误
**本阶段防御：** `prepare_v3_dirs → prepare_mutagen_agent → prepare_mergerfs_check → assert_tmux_version → exec sshd` 严格串行（D-09）；每步 fail 即 `exit 1`；不用 `&` 后台化任何 v3 阶段。

### M7 / M8 — tmux 多端尺寸 / Claude Code 颜色灰
**本阶段防御：** `/etc/tmux.conf` 硬编码 4 行（D-13）；`/etc/profile.d/cloud-claude.sh` 导出 `CLAUDE_CODE_TMUX_TRUECOLOR=1`。
**断言：** `docker exec <c> cat /etc/tmux.conf | grep -F ",*:RGB"` 有命中；`docker exec <c> bash -lc 'echo $CLAUDE_CODE_TMUX_TRUECOLOR'` == `1`

### M12 — ControlMaster MaxSessions=10
**本阶段防御：** `sshd_config` 追加 `MaxSessions 30` / `MaxStartups 60:30:120`（D-14）。
**断言：** `docker exec <c> grep -E '^MaxSessions 30' /etc/ssh/sshd_config`

### M17 — named volume UID 差异
**本阶段防御：** Dockerfile `chown -R 1000:1000` 预建目录 + entrypoint 二次 chown（D-16）。named volume 首次挂载会继承镜像中目标路径的权限；若 volume 是在旧镜像版本下初始化，entrypoint 的二次 chown 会纠正。

### M18 — 镜像体积超 700MB
**本阶段防御：** D-03 + D-28..D-30（CI gate）。裁剪候选见下文 §镜像体积估算。

---

## 镜像体积估算与裁剪候选

### 现 v2.0 Dockerfile 各大层大致占比（基于 ubuntu:24.04 + 已装包）

| 层 | 估算未压缩 | 说明 |
|----|-----------|------|
| `ubuntu:24.04` base | ~80 MB | 官方 |
| 第一批 apt（openssh-server / bash / zsh / curl / git / tmux / sudo / ca-certificates / jq / procps / iproute2 / nodejs+npm / locales / sshfs / fuse3 / libegl1 / libgl1） | ~300-350 MB | nodejs+npm 约 150 MB，locales 约 180 MB（但 `--no-install-recommends` 已减 ~100 MB） |
| 桌面族（fluxbox / pcmanfm / dbus-x11 / fonts-liberation / **fonts-noto-cjk** / xdg-utils / xclip / xsel / x11-utils / x11-xserver-utils / xterm / gnupg） | ~250-320 MB | **fonts-noto-cjk 单包 ~220MB**，裁剪首选 |
| KasmVNC 1.4.0 `.deb` | ~120-160 MB | 不建议裁剪（v1.2 KasmVNC 阶段成果） |
| Chromium（Debian bookworm） | ~200-260 MB | `--no-install-recommends` 可减 ~40MB |
| `npm install -g @anthropic-ai/claude-code` + claude-wrapper | ~70-100 MB | Node 运行时已装，不重复 |
| **v3 新增**：tini（apt） + mergerfs `.deb` + mutagen-agents.tar.gz（~100 MB 包含多架构 agent） + libfuse3-3（已在） + /etc/tmux.conf + /etc/profile.d/* | ~102 MB | **mutagen-agents 最大** |

**估算原始合计：~1.1-1.4 GB 未压缩**。要达 ≤ 700 MB，必须裁剪。

### 裁剪候选（按 CONTEXT D-03 "优先裁剪 Chromium / fonts，不动 v3 基线"）

| 候选 | 预计减少 | 影响面 | 风险 |
|------|---------|--------|------|
| `fonts-noto-cjk` → `fonts-noto-cjk-core` | ~150 MB | KasmVNC 里 CJK 字符可能缺少变体 | 低 — core 包含 Sans/Serif 常用字重 |
| Chromium 明确 recommends 剔除（`-t bookworm` 加显式排除 `--no-install-recommends` 已在） | ~20-40 MB | 已做，无额外空间 | — |
| 移除 `dbus-x11` / `pcmanfm` / `xdg-utils`（桌面启动器可能需要） | ~30-80 MB | Fluxbox 右键菜单会缺桌面集成 | 中 — v1.2 KasmVNC 阶段依赖，**不建议** |
| `npm install -g claude-code` 后清 `~/.npm` cache | ~30-50 MB | 无行为影响 | 低 — Dockerfile 后 `npm cache clean --force` |
| `/opt/mutagen-agents.tar.gz` 只保留 `linux/{amd64,arm64}`（裁其它 20+ 架构） | ~80 MB | Mutagen 只运行在 Linux 容器，其它架构 agent 永不用 | 低 — **推荐**，需 `tar --delete` 后再 `gzip` 重打包（executor 实现） |
| 把 Xvnc / KasmVNC www assets 按需精简 | ~20 MB | Web UI 国际化资源缺失 | 中 — v1.2 成果 |

**推荐裁剪组合（planner 起点）：** `fonts-noto-cjk-core` + `npm cache clean` + mutagen-agents 只保留 linux/{amd64,arm64} → 预计减少 ~250-280 MB → 落地目标 800-900 → **可能仍超 700MB，需 executor 首次构建测量后决策第二轮**。

**需 executor curl 实际验证：** 最终构建的镜像体积（`docker image inspect --format='{{.Size}}'`）是唯一权威数字，以上估算仅供 planner 排序裁剪优先级。

---

## Code Examples（来自官方源）

### mergerfs 2.41.1 Ubuntu Noble `.deb` 安装（VERIFIED URL）

```bash
# Source: https://github.com/trapexit/mergerfs/releases/tag/2.41.1
# Asset: mergerfs_2.41.1.ubuntu-noble_amd64.deb (432 KB)
#        mergerfs_2.41.1.ubuntu-noble_arm64.deb (409 KB)
ARCH=$(dpkg --print-architecture)
curl -fsSL -o /tmp/mergerfs.deb \
  "https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_${ARCH}.deb"
sha256sum -c <<<"${MERGERFS_SHA256} /tmp/mergerfs.deb"   # executor fill SHA256
dpkg -i /tmp/mergerfs.deb
mergerfs --version   # expect: mergerfs 2.41.1
```

### Mutagen v0.18.1 agent bundle 提取（VERIFIED URL + SHA256）

```bash
# Source: https://github.com/mutagen-io/mutagen/releases/tag/v0.18.1
# SHA256 (from wakemeops.com ops2deb.lock.yml, 2025-06-16):
#   linux_amd64: 7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f
#   linux_arm64: bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a
ARCH=$(dpkg --print-architecture)
curl -fsSL -o /tmp/mutagen.tar.gz \
  "https://github.com/mutagen-io/mutagen/releases/download/v0.18.1/mutagen_linux_${ARCH}_v0.18.1.tar.gz"
sha256sum -c <<<"${MUTAGEN_SHA256} /tmp/mutagen.tar.gz"
# Tarball 内容：mutagen（CLI 二进制） + mutagen-agents.tar.gz（multi-arch agent bundle）
tar -tzf /tmp/mutagen.tar.gz
# mutagen
# mutagen-agents.tar.gz
tar -xzf /tmp/mutagen.tar.gz -C /tmp mutagen-agents.tar.gz
mv /tmp/mutagen-agents.tar.gz /opt/mutagen-agents.tar.gz
```

### BuildKit apt cache mount（去 docker-clean 后）

```dockerfile
# Source: https://docs.docker.com/build/cache/optimize
# 注意：必须先移除 docker-clean 才能让 cache mount 保留 .deb
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' \
        > /etc/apt/apt.conf.d/keep-cache

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends tini fuse3 libfuse3-3
```

启用：本地 `DOCKER_BUILDKIT=1 docker build ...` 或 CI 使用 `docker buildx build`（默认启用 BuildKit）。Dockerfile 顶部 `# syntax=docker/dockerfile:1.7` 确保 cache mount 语法可用。

### tini PID 1

```dockerfile
# Source: https://github.com/krallin/tini (README "Option 2: Dockerfile apt")
RUN apt-get install -y --no-install-recommends tini
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
```

验证：`docker exec <c> ps -o pid,comm,args -p 1 | head`，PID 1 的 comm 必须是 `tini`。

### docker `--mount type=volume` 语法（VERIFIED via Docker Docs）

```bash
# Source: https://docs.docker.com/reference/cli/docker/container/run/#mount
# 只读（注意是 readonly，不是 ro）：
docker run --mount type=volume,source=claude-state-abc,target=/var/lib/claude-persist,readonly nginx

# 读写（默认）：
docker run --mount type=volume,src=claude-state-abc,dst=/var/lib/claude-persist nginx
```

**关键点：**
1. `src` / `source` 与 `dst` / `target` 都可；项目选 `src`/`dst` 与现 worker.go 现有 `-v` 形式风格一致
2. `readonly` 是无值标志（不写就是读写），**不是 `,ro`**（`,ro` 是 `-v` 的语法）
3. volume 必须已存在（D-20：Phase 33 负责创建）

### Ubuntu 25.04 AppArmor fusermount3 override

```bash
# Source: https://github.com/moby/moby/issues/50013#issuecomment (2025-09-26)
# Source: https://github.com/nestybox/sysbox/issues/947 (ctalledo, 2025-07-15)
sudo tee -a /etc/apparmor.d/local/fusermount3 <<'OVERRIDE'
capability dac_override,
OVERRIDE
sudo apparmor_parser -r /etc/apparmor.d/fusermount3

# 验证
sudo apparmor_status | grep fusermount3
```

---

## Conflicts with CONTEXT.md

### AppArmor override 路径与 CONTEXT D-23 冲突

**CONTEXT.md D-23 指定路径：** `/etc/apparmor.d/local/docker-default`

**实际上游主流修复路径：** `/etc/apparmor.d/local/fusermount3`（Ubuntu 25.04 在 fuse 3.14.0-10 中首次引入 `/etc/apparmor.d/fusermount3` profile；bug #2111105 / moby#50013 / nestybox/sysbox#947 一致指向 `fusermount3` 而非 `docker-default`）

**权威来源：**
- https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105（Launchpad bug 报告）
- https://github.com/moby/moby/issues/50013#issuecomment （Moby 维护者引用）
- https://github.com/nestybox/sysbox/issues/947 （同一现象不同容器运行时复现）
- https://github.com/containerd/stargz-snapshotter/issues/2144

**技术原因：**
Ubuntu 25.04 之前（含 24.04）`/etc/apparmor.d/fusermount3` 这个 profile **不存在**。修改 `/etc/apparmor.d/local/docker-default` 不会影响新引入的 `fusermount3` 独立 profile——后者才是在 Ubuntu 25.04+ 下拦截 `capability dac_override` 的执行者。按 CONTEXT 当前描述，host-preflight.sh 即使检测通过，实际容器 FUSE mount 仍会报 `fusermount3: mount failed: Permission denied`。

**Research 推荐（planner 决策点）：**

方案 A（**推荐**，技术正确）：修正 D-23 路径为 `/etc/apparmor.d/local/fusermount3`，更新运维手册与脚本；`apparmor_parser -r /etc/apparmor.d/fusermount3`

方案 B（双检）：host-preflight.sh 同时检测 `docker-default` 与 `fusermount3` 两处，只要 `fusermount3` 通过即视为 OK；更谨慎但运维文档会歧义

方案 C：保留 CONTEXT 原文，接受在 Ubuntu 25.04 真机上可能复现 C6 坑位

**Planner action required:** 在 discuss 阶段或首个 plan commit 前向用户澄清；默认假设方案 A，但若用户坚持 CONTEXT 原决策，research 已在 §host-preflight 骨架中以注释显式标记两者差异，并实际使用 `fusermount3` 路径（可由 executor 按最终决策调整）。

---

## State of the Art（与本阶段相关）

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Ubuntu 24.04 及之前 AppArmor：`/etc/apparmor.d/local/docker-default` + `docker run --security-opt apparmor=unconfined` 足以让容器内 FUSE 挂载 | Ubuntu 25.04+：新增 `/etc/apparmor.d/fusermount3` profile 独立拦截 `fusermount3` 二进制 | Ubuntu 25.04 (fuse 3.14.0-10) | v3.0 必须在 25.04 宿主机上部署 override，否则所有 FUSE 容器挂载失败——privileged 模式也无效 |
| mergerfs 2.33.x（Debian/Ubuntu apt 默认）`func.readdir=seq` | mergerfs 2.41.1 支持 `func.readdir=cor:N` 并行读目录 + `inodecalc=path-hash` 稳定 inode | mergerfs 2.40+（2024-08） | 10k 文件 `ls -R` 从 30-90s 降到 < 2s（C1 / M1 防御） |
| Mutagen 0.17.x 仅支持 gitignore 风格忽略 | Mutagen 0.18.x 引入 `--ignore-syntax=docker`（`.dockerignore` 风格） | Mutagen 0.18.0（2024-10） | 更契合本项目已有 `.dockerignore` 资产（Phase 31 消费） |
| Docker `-v src:dst:ro` 简短语法 | Docker 推荐 `--mount type=volume,src=,dst=,readonly` | Docker 官方自 18.09+ 推荐 | 本阶段契约按新语法（D-19） |

**Deprecated / 不推荐（但 v2.0 已在用的要保留兼容）：**
- Dockerfile 现 `-v homeDir:homeMount` bind mount 的简短写法：保持，不迁移到 `--mount`（v2.0 兼容 + 非新增字段）。**仅新增的 Volumes 字段**使用 `--mount type=volume` 新语法。

---

## Assumptions Log

本研究**所有关键版本 / URL / 语法均通过 WebSearch 2026-04-18 实时验证**。残留的 `[ASSUMED]` 级别主张：

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | mergerfs `.deb` SHA256 在 Dockerfile 中硬编码——具体数值需 executor 首次 `curl + sha256sum` 生成，本研究未列出 | §Standard Stack / §Code Examples | 低 — executor 首次构建必须计算，不是基于假设实施；若 upstream 重新打包会立刻失败告警 |
| A2 | `mutagen_linux_<arch>_v0.18.1.tar.gz` 内部结构为 `mutagen` + `mutagen-agents.tar.gz` 两个顶层文件（无子目录） | §Standard Stack / §Code Examples | 中 — WakeMeOps blueprint 与 Mutagen GitHub README 两处佐证；executor 构建时需 `tar -tzf` 一次性打印核对 |
| A3 | fonts-noto-cjk-core 裁剪后保留常用中日韩字符，对 KasmVNC 桌面显示无可感知影响 | §镜像体积估算 | 中 — 需 Phase 35 真机验收；若用户面感知，回退到 full fonts-noto-cjk 并在 Chromium / KasmVNC 侧另找空间 |
| A4 | mutagen-agents.tar.gz 裁剪为仅 linux/{amd64,arm64} 可减约 80MB | §镜像体积估算 | 中 — 需 executor `tar -tzf` 确认架构目录分布后再定方案 |
| A5 | `--mount type=volume,...,readonly` 语法对 Docker Engine 28.x 兼容 | §Code Examples / §Go 契约 diff | 低 — Docker 官方 Docs 2023+ 一直列出该语法；v2.0 Docker 28.x 已在生产 |
| A6 | 现有 `ssh_inject_test.go` 风格（`fakeContainer` + `execInContainer` 变量注入）可直接复用到 `worker_volume_test.go` 做 JSON round-trip | §Go 契约 diff | 低 — 读取原测试文件已确认 `execInContainer` 是 package-level var，且 Volumes 字段的测试不需要实际 docker exec |

**需 executor 首次构建实际验证（must-verify-before-commit）：**
- A1: mergerfs `.deb` SHA256（amd64 + arm64 各一个）
- A2: mutagen tarball 内部结构 `tar -tzf` 输出
- A4: mutagen-agents.tar.gz 多架构目录清单

---

## Open Questions（不含冲突项，已在 §Conflicts 单列）

1. **mutagen-agents.tar.gz 解压目标路径**
   - CONTEXT D-05 写：运行时由 cloud-claude（Phase 31）触发 agent extract
   - CONTEXT D-09-3 写：`prepare_mutagen_agent` 在 entrypoint 阶段 extract 到 `/usr/local/libexec/mutagen/agents/`
   - 两者有重复——**CONTEXT 内部一致性问题**。
   - 研究推断：D-05 的"runtime by cloud-claude"指的是 Mutagen daemon 会根据版本匹配**再次校验并使用**解压后的 agent；entrypoint 只是**预解压**一次（幂等 touch `.extracted` 标记）。
   - Planner action：按 D-09-3 解压 + touch 标记；D-05 的"runtime trigger"理解为 cloud-claude 负责启动 daemon，daemon 自然消费 `/usr/local/libexec/mutagen/agents/`。

2. **tmux `-V` 版本断言的松紧度**
   - CONTEXT D-06 写"entrypoint 启动时断言 `tmux -V` 字符串 ≥ `3.4`"
   - 研究：ubuntu:24.04 apt 稳定版 `3.4-1ubuntu0.1`，但若未来 ubuntu 更新到 3.5+，entrypoint 应放行
   - 研究推断的正则 `^(3\.[4-9]|[4-9]\.)` 接受 3.4-3.9 与 4.x+；planner 决定是否再严格。

3. **BuildKit 启用路径**
   - `build-managed-image.sh` 现仅 `docker build -f ... -t ...`；D-30 约束不在此脚本加 CI 逻辑，但 BuildKit cache mount 需要 BuildKit enabled
   - 研究推断：在脚本顶部加 `export DOCKER_BUILDKIT=1`（本地开发）+ CI workflow 用 `docker buildx build` 或显式设置同环境变量；CI step 与 build script 解耦原则不违反。

4. **`CLOUD_CLAUDE_MERGERFS_BRANCHES` env 的读取位置**
   - CONTEXT D-12 说 env 预留 3 路扩展点，"Phase 31 若决策 3 路无需动镜像"
   - 研究推断：env 变量**不在 entrypoint 读取**（entrypoint 不挂载 mergerfs）；由 Phase 31 cloud-claude 在执行 `mergerfs` CLI 前 `${CLOUD_CLAUDE_MERGERFS_BRANCHES:-"/workspace-hot=RW:/workspace-cold=NC,RO"}`。
   - 本阶段**只需文档化** env 名称，不写读取代码。

---

## Environment Availability

Phase 29 是**构建期 + 镜像内** operations，宿主机外部依赖仅 CI 侧：

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker Engine + BuildKit | Dockerfile cache mount | ✓（CONTEXT STACK） | 28.x | — |
| curl | Dockerfile 内下载 .deb / .tar.gz | ✓ | apt default | — |
| sha256sum | Dockerfile 校验 | ✓ | coreutils | — |
| dpkg | mergerfs `.deb` 安装 | ✓ | apt default | — |
| tar | mutagen tarball 解压 | ✓ | apt default | — |
| GitHub releases 外网可达（build 期） | mergerfs / mutagen 下载 | ✓（CI + 开发机） | — | 若 GitHub 不可达，可把 `.deb` / `.tar.gz` 预下载到 `deploy/docker/managed-user/vendor/` 由 Dockerfile `COPY`——**本阶段不采用**，但 planner 可作为 contingency |
| Ubuntu 25.04 测试宿主机 | host-preflight.sh 真机验证 | 不确定 | — | Phase 35 真机验收；本阶段仅保证脚本语法正确 + 单元逻辑分支覆盖 |

**Missing with no blocking:** 无。
**Missing with fallback:** Ubuntu 25.04 真机验收推到 Phase 35（deferred）。

---

## Validation Architecture（三列表：文件改动 → 静态断言 → 运行时断言）

> Nyquist 未启用（`nyquist_validation_enabled=false`），本表按 CONTEXT 要求保留作为 planner 的验收清单来源。

### Success Criteria 断言映射（7 条 ROADMAP 基线 + 额外内部约束）

| # | Success Criterion | 文件改动 | 静态断言（build / lint） | 运行时断言（docker exec / CI） |
|---|-------------------|----------|-------------------------|-------------------------------|
| SC1 | `mount \| grep mergerfs` 含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff` | 镜像 README / `/etc/cloud-claude/mergerfs.params`（可选静态文件） | `grep -F 'func.readdir=cor:4' deploy/docker/managed-user/README.md`（文档一致性）| **Phase 31 消费** — 本阶段可 skip 运行时（仅 entrypoint `mergerfs --version` 通过即达标） |
| SC2 | `getfattr -n user.mergerfs.branches /workspace/.mergerfs` 返回 `RW + NC,RO` | （同上，Phase 31 消费） | — | **Phase 31 消费** |
| SC3 | `/home/claude/.claude` 属主 `1000:1000`；`mutagen-agent --version == /etc/cloud-claude/mutagen.version` | Dockerfile D-16 / D-07 + entrypoint `prepare_v3_dirs` + `prepare_mutagen_agent` | Dockerfile `grep -E 'chown.*1000:1000.*/home/claude'`；`grep -F "v0.18.1" Dockerfile` | `docker exec <c> stat -c '%u:%g' /home/claude/.claude` == `1000:1000`；`docker exec <c> sh -c '/usr/local/libexec/mutagen/agents/linux_amd64/mutagen-agent version'` == `v0.18.1`（Phase 31 消费） |
| SC4 | `tmux -V` ≥ `3.4`；`/etc/tmux.conf` 含 `terminal-overrides ",*:RGB"` 与 `window-size latest` | `/etc/tmux.conf`（新增 COPY 文件）+ entrypoint `assert_tmux_version` | `grep -F ',*:RGB' deploy/docker/managed-user/tmux.conf`；`grep -F 'window-size latest' deploy/docker/managed-user/tmux.conf` | `docker exec <c> tmux -V \| awk '{print $2}'` 匹配 `^(3\.[4-9]\|[4-9]\.)`；`docker exec <c> cat /etc/tmux.conf \| grep -F ',*:RGB'` |
| SC5 | PID 1 = `tini`，无 systemd / systemd-logind | Dockerfile `ENTRYPOINT ["/usr/bin/tini", "--", ...]` | `grep -F '"/usr/bin/tini"' deploy/docker/managed-user/Dockerfile`；Dockerfile 中 `! grep -E '\bsystemd\b' deploy/docker/managed-user/Dockerfile`（排除 `systemd-helper` 之类间接包名时需精化） | `docker exec <c> ps -o pid=,comm= -p 1` → `1 tini`；`docker exec <c> pgrep -x systemd-logind` 退出码 == 1（进程不存在） |
| SC6 | `docker image inspect --format='{{.Size}}' < 700*1024*1024` | CI workflow 新增 step（D-28）| `grep -F 'docker image inspect' .github/workflows/*.yml` | CI job 跑 `SIZE=$(docker image inspect ...) ; (( SIZE > 700*1024*1024 )) && exit 1` |
| SC7 | `host-preflight.sh` 在 Ubuntu 25.04 缺 override 时 exit=1 且打印修复命令 | `deploy/host-preflight.sh` + `deploy/README.md` | `shellcheck deploy/host-preflight.sh`；`bash -n deploy/host-preflight.sh` | 单元测试：在 CI 容器 mock `/etc/os-release=ID=ubuntu\nVERSION_ID=25.04` + 空 `/etc/apparmor.d/local/fusermount3` → 期望 exit 1 + stdout 含"Fix:"；mock VERSION_ID=24.04 → exit 0 |

### 额外断言（CONTEXT 决策一致性 + 契约）

| 断言 ID | 目标 | 静态断言 | 运行时断言 |
|---------|------|---------|-----------|
| V-01 | `HostActionRequest.Volumes` omitempty | `go vet ./internal/agentapi/...` + round-trip test（`TestHostActionRequest_VolumesOmitempty`） | `go test ./internal/agentapi/...` 通过 |
| V-02 | worker `createHost` 正确拼接 `--mount type=volume,...` | `go vet` + 新增 unit test 捕获 `args` 切片（mock `runDocker`） | `go test ./internal/runtime/tasks/...` 通过 |
| V-03 | `image.lock` 新字段正确 | `grep -E '^image_version: v3\.0\.0$' deploy/docker/managed-user/image.lock`；YAML 解析测试（`yaml.Unmarshal`） | — |
| V-04 | sshd_config 追加了 4 行 | `grep -E '^(ClientAliveInterval 15\|ClientAliveCountMax 8\|MaxSessions 30\|MaxStartups 60:30:120)$' deploy/docker/managed-user/sshd_config` 命中 4 次 | `docker exec <c> grep -E '^MaxSessions' /etc/ssh/sshd_config` |
| V-05 | C5 / M17 chown 到位 | Dockerfile `grep -Eq 'chown.*1000:1000.*(/home/claude|/workspace-hot|/workspace-cold|/var/lib/claude-persist)'` | `docker exec <c> stat -c '%u:%g' /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist` 全部 `1000:1000` |
| V-06 | 向后兼容：v2.0 旧 request（无 Volumes 字段）不破 | round-trip test：`{"task_id":"t","host_id":"h","action":"create_host"}` → Unmarshal → 不 panic、`Volumes` 为 nil | — |

---

## Sources

### Primary（HIGH confidence — VERIFIED 2026-04-18）

- **mergerfs 2.41.1 release**：https://github.com/trapexit/mergerfs/releases/tag/2.41.1 （asset 名 / 大小 / 下载 URL）
- **Mutagen v0.18.1 release**：https://github.com/mutagen-io/mutagen/releases/tag/v0.18.1 （asset 名 / 下载 URL）
- **Mutagen SHA256**：https://github.com/upciti/wakemeops/blob/main/blueprints/devops/mutagen/ops2deb.lock.yml （2025-06-16 时间戳）
- **Docker `--mount` 语法**：https://docs.docker.com/reference/cli/docker/container/run/#mount
- **BuildKit cache mount**：https://docs.docker.com/build/cache/optimize
- **tini 官方 README**：https://github.com/krallin/tini（apt 安装 + ENTRYPOINT 模式）
- **Ubuntu tmux 3.4-1ubuntu0.1**：https://launchpad.net/ubuntu/noble/+source/tmux/+changelog
- **AppArmor fusermount3（Ubuntu 25.04）**：
  - https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105
  - https://github.com/moby/moby/issues/50013
  - https://github.com/nestybox/sysbox/issues/947 （ctalledo 2025-07-15 给出完整 override 语法）
  - https://github.com/containerd/stargz-snapshotter/issues/2144

### Secondary（MEDIUM confidence — 来自 CONTEXT 引用 / 内部研究）

- `.planning/research/STACK.md` §1 §2 §4 —— mergerfs / Mutagen / tmux 版本和理由
- `.planning/research/PITFALLS.md` C1 / C2 / C3 / C5 / C6 / C7 / M3 / M4 / M7 / M8 / M12 / M17 / M18
- `.planning/research/ARCHITECTURE.md` §host-agent 边界

### Tertiary（LOW confidence，已标记 ASSUMED）

- A1-A6 见 Assumptions Log

---

## Metadata

**Confidence breakdown:**
- Standard stack（版本 / URL / SHA256）: HIGH — GitHub release 页面 + WakeMeOps ops2deb 双源
- Architecture patterns（entrypoint 顺序 / Dockerfile 骨架 / Go 契约）: HIGH — CONTEXT 已决议 + 现代码风格延续
- Pitfalls 防御映射: HIGH — 基于 PITFALLS.md + WebSearch 上游 bug 追踪
- AppArmor override 路径: HIGH for `fusermount3`（上游证据链完整）；**CONTEXT 当前路径需修正**（见 §Conflicts）
- 镜像体积裁剪预测: MEDIUM — 估算基于典型包大小，executor 首次构建必须实测

**Research date:** 2026-04-18
**Valid until:** 2026-07-18 （~90 days；mergerfs / Mutagen / tmux / AppArmor 相关生态变化慢；如 Ubuntu 25.10 LTS 发布或 mergerfs 2.42 出则需要更新）

---

## RESEARCH COMPLETE — Phase 29 v3 镜像 + Worker Volumes 契约所需的 10 项可落地答案已给出，核心冲突（AppArmor override 路径）已在 §Conflicts with CONTEXT.md 单列待 planner 决策
