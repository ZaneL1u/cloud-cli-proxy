# Phase 29: 受管镜像 v3 + Worker 容器参数扩展 - Context

**Gathered:** 2026-04-18
**Status:** Ready for planning

<domain>
## Phase Boundary

交付 v3.0 受管镜像基线与 Worker 容器参数契约扩展。范围包含：

1. 在现有 `deploy/docker/managed-user/` 镜像上增量引入 mergerfs 2.41.1 + mutagen-agent v0.18.1 + tmux 3.6a + libfuse3 3.18.x + tini PID 1，并把 mount / entrypoint / sshd / tmux 参数按 PITFALLS C1 / C2 / C3 / C5 / C7 与 M3 / M4 / M7 / M8 / M12 / M17 的要求固定下来。
2. 预建 `/home/claude/.claude`、`/home/claude/.cache/claude`、`/workspace-hot`、`/workspace-cold`、`/workspace` 并 `chown 1000:1000`。
3. 在 `internal/agentapi/contracts.go` 新增 `VolumeMount` 类型与 `HostActionRequest.Volumes` 字段，在 `internal/runtime/tasks/worker.go:createHost` 的 `docker create` 参数拼接处接受 `--mount type=volume,...`；**不做** `docker volume create` 与生命周期绑定（留给 Phase 33）。
4. 新增 `deploy/host-preflight.sh` 用于宿主机（特别是 Ubuntu 25.04）AppArmor override 检测与修复命令输出。
5. CI 中新增镜像未压缩体积 ≤ 700MB 的 bash 断言 step（BASE-04 一次落地，Phase 35 二次回归）。
6. `deploy/docker/managed-user/image.lock` 凸至 `v3.0.0` 并新增能力元数据字段。

本阶段**不交付**任何 user-facing REQ-F\* 行为（所有可观察行为由 Phase 30 / 31 / 32 / 33 在镜像基础上实现），也**不扩展 host-agent endpoint**（沿用 `/agent/host/action`）。

</domain>

<decisions>
## Implementation Decisions

### 镜像演进路径

- **D-01**：沿用 `deploy/docker/managed-user/Dockerfile` 做**增量改造**——不新建 v3 独立镜像目录。v3 组件（mergerfs / mutagen-agent / tmux 配置 / tini）叠在现有层之上；KasmVNC / Chromium / Fluxbox / fonts-noto-cjk 这些 v1.2 deferred 用户面组件保留，确保 v3.0 升级对 v1.2 延迟项零破坏。
- **D-02**：Base image 保持 `ubuntu:24.04`，不升级到 25.04。Ubuntu 25.04 的 AppArmor override 问题由宿主机侧 `deploy/host-preflight.sh` 解决，不通过提升 base 镜像规避。
- **D-03**：构建启用 BuildKit cache mount（`--mount=type=cache,target=/var/cache/apt` 等），所有 `apt-get install` 加 `--no-install-recommends`；多个相关 RUN 合并以减少 layer 数；镜像未压缩体积硬约束 ≤ 700MB（CI gate）。若超标，优先裁剪 Chromium 相关 recommends 与 fonts，不裁剪 mergerfs / mutagen-agent / tmux 的 v3 基线组件。

### 二进制来源 / 版本

- **D-04**：mergerfs 2.41.1 从官方 GitHub release（`trapexit/mergerfs`）下载 `mergerfs_2.41.1.ubuntu-noble_<arch>.deb` 并通过 `dpkg -i` 安装（**禁止 apt repo**，PITFALLS M3）。镜像同时支持 `amd64` + `arm64`，构建时按 `$(dpkg --print-architecture)` 选 deb。
- **D-05**：mutagen-agent v0.18.1 从 Mutagen GitHub release（`mutagen-io/mutagen`）下载 `mutagen_linux_<arch>_v0.18.1.tar.gz`，解压后只保留 agent bundle `mutagen-agents.tar.gz`，预放到 `/opt/mutagen-agents.tar.gz`。运行时由 cloud-claude（Phase 31）触发 agent extract；本阶段只做预置 + 版本标记。同样支持 amd64 + arm64。
- **D-06**：tmux 使用 `ubuntu:24.04` apt 仓库版本（3.4 系列），**entrypoint 启动时断言 `tmux -V` 字符串 ≥ `3.4`**——不强制 3.6a 上游依赖（PPA 引入维护成本过高）；ROADMAP 所列 3.6a 为上限期望，本阶段**放宽下限到 3.4** 以换取镜像体积与稳定性。若 Phase 35 真机验收发现 3.4 下 `terminal-overrides RGB` 行为不达标，则回流到本阶段补 PPA 或从源编译（open follow-up 记录于 `<deferred>`）。
- **D-07**：在镜像构建阶段写入元数据文件，供 cloud-claude 与 doctor 读取做版本比对（防御 PITFALLS C4 Mutagen 版本漂移）：
  - `/etc/cloud-claude/mutagen.version` ← `v0.18.1`
  - `/etc/cloud-claude/mergerfs.version` ← `2.41.1`
  - `/etc/cloud-claude/tmux.version` ← 运行时 `tmux -V` 回填（构建阶段静态占位）
- **D-08**：libfuse3 采用 `ubuntu:24.04` apt 提供的 `libfuse3-3` / `fuse3` 系列（3.16–3.18 区间均可满足 mergerfs 2.41.1 要求），不引入额外 PPA。

### Entrypoint 改造

- **D-09**：不重写 entrypoint，沿用现 `deploy/docker/managed-user/entrypoint.sh` 骨架，在 `exec /usr/sbin/sshd -D -e` **之前**插入 v3 阶段（串行、快速失败、每步有明确 log 前缀）：
  1. `prepare_fuse`：`chmod 666 /dev/fuse` + 断言 `/dev/fuse` 存在（现存逻辑强化）
  2. `prepare_v3_dirs`：二次 `chown -R 1000:1000 /home/claude /workspace-hot /workspace-cold`（Dockerfile 已预建，此处兜底 C5 / M17）
  3. `prepare_mutagen_agent`：校验 `/opt/mutagen-agents.tar.gz` 存在，extract 到 `/usr/local/libexec/mutagen/agents/`（供 Phase 31 的 Mutagen daemon 使用）
  4. `prepare_mergerfs`（可选）：默认**不**在 entrypoint 挂 mergerfs——mergerfs 由 cloud-claude 在 SSH 会话建立后按 `--mount-mode` 动态挂载（Phase 31）。entrypoint 仅校验 `mergerfs --version` 可执行
  5. `wait` + `exec /usr/sbin/sshd -D -e`
- **D-10**：PID 1 改为 `tini`——Dockerfile `ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`。tini 通过 `apt-get install -y --no-install-recommends tini` 安装（ubuntu:24.04 官方包，避免额外下载）。
- **D-11**：mergerfs 挂载参数固定（由 cloud-claude 在 Phase 31 下发，但镜像文档与 host-preflight 断言保持一致）：`category.create=ff,func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,inodecalc=path-hash`。
- **D-12**：mergerfs branch 拓扑本阶段锁定为 **2 路**：`/workspace-hot=RW:/workspace-cold=NC,RO`（解决 Q10）。entrypoint 与镜像文档通过 `CLOUD_CLAUDE_MERGERFS_BRANCHES` 环境变量预留 3 路扩展点（未设置时默认 2 路），Phase 31 若决策 3 路无需动镜像。
- **D-13**：新增 `/etc/tmux.conf`（容器级默认）：
  ```
  set -ga terminal-overrides ",*:RGB"
  set -g window-size latest
  set -g aggressive-resize on
  set -g history-limit 50000
  ```
  新增 `/etc/profile.d/cloud-claude.sh` 导出 `CLAUDE_CODE_TMUX_TRUECOLOR=1`（PITFALLS M7 / M8）。
- **D-14**：`sshd_config` 在现有基础上追加：
  ```
  ClientAliveInterval 15
  ClientAliveCountMax 8
  MaxSessions 30
  MaxStartups 60:30:120
  ```
  并保留现有 `PasswordAuthentication yes` / `UsePAM no` 等字段（REQ-F3-A 服务端基线，PITFALLS M12）。
- **D-15**：容器内**不**启动 systemd / systemd-logind（PITFALLS C7）。tini 仅作 PID 1 收割僵尸进程，sshd 与后续 tmux / KasmVNC 由 entrypoint 显式 fork。

### 预建目录与用户

- **D-16**：Dockerfile 构建时预建以下目录并 `chown 1000:1000`：
  - `/home/claude`（新增，供 REQ-F7 持久化 symlink 目标）
  - `/home/claude/.claude`
  - `/home/claude/.cache/claude`
  - `/workspace-hot`（Mutagen 热同步 branch 挂载点）
  - `/workspace-cold`（sshfs 冷 branch 挂载点）
  - `/workspace`（mergerfs 合并视图挂载点，与现有 v2.0 挂载点保持一致）
  - `/var/lib/claude-persist`（Phase 33 named volume 挂载点，本阶段预建 + chown）
- **D-17**：现有 `workspace` 用户（UID 1000 / GID 1000）不重命名、不替换。`home-dir` 保持 `/workspace`（兼容 v2.0 entrypoint 密码同步 + SSH key 注入逻辑）。`/home/claude` 目录属主也是 UID 1000——与 `workspace` 用户同属主，名称只是 convention，不再新建用户避免现有 SSH / sudoers 逻辑级联改造。

### HostActionRequest.Volumes 契约

- **D-18**：在 `internal/agentapi/contracts.go` 新增：
  ```go
  type VolumeMount struct {
      Name     string            `json:"name"`                // docker named volume 名
      Target   string            `json:"target"`              // 容器内挂载路径
      ReadOnly bool              `json:"read_only,omitempty"`
      Labels   map[string]string `json:"labels,omitempty"`    // 仅用于日志与审计，不写入容器
  }
  ```
  并给 `HostActionRequest` 增加 `Volumes []VolumeMount \`json:"volumes,omitempty"\``。
- **D-19**：worker `createHost` 遍历 `request.Volumes`，为每个元素在 `docker create` args 中追加：
  `--mount type=volume,src=<Name>,dst=<Target>[,readonly]`
  **不**追加 `--label com.cloud-cli-proxy.volume=<Name>`（labels 只用于上层审计，不写到容器）。
- **D-20**：worker 本阶段**不**调用 `docker volume create`——Phase 33 负责通过新增工具函数 `ensureDockerVolume(name, labels)` 幂等创建。本阶段 worker 假设 volume 已存在；若 `docker create` 因 volume 不存在失败，则正常返回错误（与现有错误码 `host_action_failed` 一致）。
- **D-21**：`ClaudeAccountID` 字段**不**在本阶段新增——Phase 30 `migration 0014` 与 `HostActionRequest.ClaudeAccountID` 共同交付，本阶段仅确保 `Volumes` 字段增加后 v2.0 旧客户端反序列化不破（单元测试 round-trip 验证 `omitempty` 行为）。
- **D-22**：向后兼容性：`Volumes` 字段为 `omitempty`，v2.0 agent / 控制面若未升级，`HostActionRequest` JSON 不包含该字段，worker 路径不变。

### host-preflight 与 AppArmor override

- **D-23**：新增 `deploy/host-preflight.sh`，独立可执行脚本，职责：
  1. 检测宿主机发行版（`/etc/os-release`）是否为 Ubuntu 25.04+
  2. 若是，检查 `/etc/apparmor.d/local/fusermount3` 是否包含 `capability dac_override,`
  3. 缺失时退出码 `1`，打印**修复命令**（不自动 sudo 执行）：
     ```
     sudo tee -a /etc/apparmor.d/local/fusermount3 <<EOF
     capability dac_override,
     EOF
     sudo apparmor_parser -r /etc/apparmor.d/fusermount3
     ```
  4. 非 Ubuntu 25.04 宿主机退出码 `0` + 提示"当前宿主机无需 override"

  > **路径修正说明（2026-04-18）**：本决策最初误写为 `/etc/apparmor.d/local/docker-default`。实际上 Ubuntu 25.04 在 fuse 3.14.0-10 中新增了独立的 `/etc/apparmor.d/fusermount3` profile，才是真正拦截 `capability dac_override` 的执行者；修改 `docker-default` 的 local override 无法影响该 profile。经 29-RESEARCH.md §Conflicts with CONTEXT.md 论证（证据链：Launchpad bug #2111105、moby#50013、sysbox#947、stargz-snapshotter#2144），统一修正为 `fusermount3` 路径，以真正防御 Critical Pitfall C6。详见 29-DISCUSSION-LOG.md §修正记录 2026-04-18。
- **D-24**：`deploy/host-preflight.sh` **不**嵌入 cloud-cli-proxy 控制面启动流程——保持独立脚本，由运维手动运行 / CI 工作流作为可选 step / Phase 34 `doctor host` 维度调用。理由：宿主机级改动需 sudo，不能由控制面进程 silent 执行。
- **D-25**：运维手册 `docs/` 中新增一节 `v3.0 AppArmor override 部署`（或在 deploy/ README 中），包含：override 内容、`apparmor_parser -r` 刷新命令、回滚命令、如何验证 override 生效。

### image.lock 扩展字段

- **D-26**：`deploy/docker/managed-user/image.lock` 追加字段（YAML 追加，不破坏现有字段顺序）：
  ```yaml
  image_version: v3.0.0
  mergerfs_version: 2.41.1
  mutagen_agent_version: v0.18.1
  tmux_version_min: "3.4"
  supports_mutagen: true
  supports_mergerfs: true
  ```
  现有 `image_name` / `base_image` / `ssh_port` / `home_mount` / `default_user` / `rebuild_mode_default` / `factory_reset_mode` 全部保留不变。
- **D-27**：image.lock 是 Phase 30 Entry API 扩展的**单一上游数据源**——Phase 30 读取 image.lock 后把 `image_version` / `supports_mutagen` / `supports_mergerfs` 写入 `AuthResponse`。本阶段仅写入字段，不做 runtime 读取。

### CI 镜像体积 gate（BASE-04）

- **D-28**：在 CI workflow（GitHub Actions）新增 step，使用 bash + `docker image inspect` 实现，不引入第三方 action：
  ```bash
  SIZE=$(docker image inspect --format='{{.Size}}' "$IMAGE_NAME")
  LIMIT=$((700 * 1024 * 1024))
  if (( SIZE > LIMIT )); then
    echo "::error::image size $SIZE bytes exceeds BASE-04 limit $LIMIT bytes"
    docker history "$IMAGE_NAME" --format "table {{.Size}}\t{{.CreatedBy}}"
    exit 1
  fi
  echo "image size $SIZE bytes (limit $LIMIT bytes) — PASS"
  ```
- **D-29**：失败时 CI 日志自动输出 `docker history` 以便排查膨胀层，便于 Phase 35 二次回归或未来维护。
- **D-30**：build 脚本 `deploy/docker/managed-user/build-managed-image.sh` 不嵌入体积检查——CI gate 单一职责，本地开发允许超标。

### Claude's Discretion

以下细节由 planner / executor 根据实现便利性决定：

- tini 二进制是否从 apt 安装或 COPY 静态二进制（优先 apt，体积更可控）
- mergerfs `.deb` 下载的 checksum 校验方式（`sha256sum` vs GPG 签名；建议 `sha256sum` 硬编码到 Dockerfile）
- mutagen-agents tarball 的解压位置（`/opt/mutagen-agents.tar.gz` 预放 + runtime extract 到 `/usr/local/libexec/mutagen/agents/`，由 entrypoint 阶段 `prepare_mutagen_agent` 完成）
- Dockerfile RUN 指令的合并粒度（层数与 cache 命中的权衡）
- CI gate 的具体文案与报错格式（保持 `::error::` 前缀即可）
- host-preflight.sh 在非 Linux 宿主机（macOS / WSL）上的行为（建议：直接退出 0 并提示"非 Linux 宿主机无需检查"）

### Folded Todos

无（`todo match-phase 29` 返回 0 条匹配）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 项目规划与需求

- `.planning/PROJECT.md` — 项目核心价值、v3.0 约束、Key Decisions
- `.planning/REQUIREMENTS.md` §F1 / F2 / F4 / F7 — v3.0 功能需求（本阶段提供前置镜像层）
- `.planning/REQUIREMENTS.md` §性能与体验验收基线 — **BASE-04**（镜像体积 ≤ 700MB CI gate，本阶段一次落地）
- `.planning/REQUIREMENTS.md` §Critical Pitfalls — **C1 / C2 / C3 / C5 / C6 / C7** 均需在本阶段防御
- `.planning/REQUIREMENTS.md` §Open Questions — **Q10**（mergerfs 2 路 vs 3 路）本阶段拍板为 2 路 + 环境变量扩展点
- `.planning/ROADMAP.md` §Phase 29 — 官方 Goal / Scope / Success Criteria 声明（10 条 success criteria 是 plan-phase 验收基线）
- `.planning/STATE.md` — v3.0 milestone 当前进度与已固化决策

### 研究基线

- `.planning/research/SUMMARY.md` §2.1 技术决策（新增 / 升级组件版本清单）
- `.planning/research/SUMMARY.md` §5 TOP 10 Critical Pitfalls
- `.planning/research/STACK.md` — mergerfs 2.41.1 / mutagen v0.18.1 / tmux / libfuse3 版本与理由
- `.planning/research/PITFALLS.md` C1 / C2 / C3 / C5 / C6 / C7 — mergerfs、sshfs、Mutagen、AppArmor、systemd-logind 坑位细节与修复
- `.planning/research/PITFALLS.md` M3（禁止 apt mergerfs）/ M4（entrypoint 串行编排）/ M7 / M8（tmux truecolor / resize）/ M11 / M12（sshd KeepAlive）/ M17（预建目录 chown）/ M18（BuildKit cache）
- `.planning/research/ARCHITECTURE.md` §host-agent 边界（本阶段**不**扩展 host-agent endpoint）
- `.planning/research/FEATURES.md` §镜像能力探测字段

### 既有代码（直接改造对象）

- `deploy/docker/managed-user/Dockerfile` — 增量改造目标，v2.0 已 ship
- `deploy/docker/managed-user/entrypoint.sh` — 增量插入 v3 阶段
- `deploy/docker/managed-user/sshd_config` — 追加 KeepAlive / MaxSessions / MaxStartups
- `deploy/docker/managed-user/image.lock` — 扩展 v3.0 能力元数据字段
- `deploy/docker/managed-user/build-managed-image.sh` — build 脚本不变，CI 侧加 gate
- `internal/agentapi/contracts.go` — 新增 `VolumeMount` 类型与 `HostActionRequest.Volumes` 字段
- `internal/runtime/tasks/worker.go` §`createHost` — 在 `docker create` args 拼接处接受 `--mount type=volume,...`

### 既有决策参考（延续模式）

- `.planning/phases/17-image-entrypoint-baseline/17-CONTEXT.md` — claude-shell 阶段的 entrypoint 串行编排 / 快速失败约定（模式复用，但 claude-shell 是独立镜像，本阶段走 managed-user 增量路径）

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `deploy/docker/managed-user/entrypoint.sh` 现有结构（SSH host key 生成 → 用户同步 → 密码 → `chown /workspace` → IPv6 关闭 → `/dev/fuse` chmod → KasmVNC → `exec sshd`）已经具备"串行编排 + 快速失败"的骨架，v3 阶段插入成本低
- `internal/runtime/tasks/worker.go:createHost` 已经把 `docker create` args 拼接成可增量扩展的 slice（memory / cpu / labels / env / volume），新增 `--mount type=volume,...` 只需一个循环追加
- `internal/agentapi/contracts.go` 已采用 `omitempty` JSON tag 做向后兼容（如 `SSHKeys / SSHPublicKey`），直接延续即可保证旧 client 不破
- `deploy/docker/managed-user/image.lock` 已经是 YAML key-value 结构，追加字段为纯增量操作

### Established Patterns

- docker 容器创建参数拼接：先静态 args → 条件追加（memory / cpu）→ 追加 env → 追加 `-v bind` → 追加 labels → 追加 image name。v3 `--mount volume` 适合插在 `-v bind` 之后、labels 之前
- sing-box-gateway 与 claude-shell 两个独立镜像都用 `deploy/docker/<name>/` 作为根，managed-user 作为主线镜像同构
- `deploy/docker/managed-user/entrypoint.sh` 使用 `set -euo pipefail` + 明确的 echo 前缀做日志（现有风格，v3 阶段继承）

### Integration Points

- Phase 30 的 Entry API 扩展会从 `image.lock` 读取 `image_version` / `supports_mutagen` / `supports_mergerfs` 字段
- Phase 31 的 cloud-claude Mutagen 逻辑依赖 `/etc/cloud-claude/mutagen.version` 元数据与 `/opt/mutagen-agents.tar.gz` 预放
- Phase 32 的 SSH KeepAlive 基线依赖本阶段 `sshd_config` 服务端参数
- Phase 33 的 persistent volume 挂载使用本阶段定义的 `HostActionRequest.Volumes` 契约
- Phase 34 的 doctor mount 维度从 `/etc/cloud-claude/mergerfs.version` 做版本校验
- `deploy/host-preflight.sh` 被 Phase 34 doctor host 维度与 Phase 35 真机验收共同调用

</code_context>

<specifics>
## Specific Ideas

- 用户明确要求（PROJECT.md / STATE.md）：镜像升级**零增量特权**，复用 v2.0 已开放的 FUSE / SYS_ADMIN / AppArmor unconfined 通道，不新增 `--privileged` 或新 capability
- REQUIREMENTS.md 明确不做（OOS-A3）：不用 bcachefs / OverlayFS 替代 mergerfs——锁死本阶段技术选型
- 用户倾向（研究结论 §2.1）：mergerfs 静态 deb > apt 仓库（PITFALLS M3），mutagen-agent GitHub release tarball > 编译
- tmux 3.6a 期望 vs 3.4 实际：本阶段**放宽到 3.4**，将 3.6a 上限需求作为 Phase 35 真机验收的 open follow-up（记录于 `<deferred>`）

</specifics>

<deferred>
## Deferred Ideas

### Reviewed Todos (not folded)

无（`todo match-phase 29` 零匹配）。

### 阶段内确认但不交付的 follow-up

- **tmux 3.6a 升级路径**：本阶段以 ubuntu:24.04 apt 默认 tmux（≥ 3.4）交付。若 Phase 35 真机验收发现 `terminal-overrides RGB` 或 `window-size latest` 在 3.4 下行为异常，需回流新增 PPA 或源码编译任务。现状：**不入 Phase 29 scope**，由 Phase 35 结果决定
- **image.lock 切分为 image-capabilities.yaml**：Phase 30 若发现 Entry API 需要的能力字段远超当前 6 个，再考虑单独拆文件。当前一文件够用
- **host-preflight.sh 自动修复模式**：本阶段只检测 + 打印命令，不加 `--apply` 自动 sudo。运维手册反馈后再评估
- **mergerfs 3 路 branch（含本地覆盖层）**：通过 `CLOUD_CLAUDE_MERGERFS_BRANCHES` env 预留扩展点，Phase 31 决策是否启用
- **arm64 真机验收**：本阶段镜像构建支持 arm64，但 CI 与真机验收以 amd64 为主线；arm64 集成测试推迟到 Phase 35（若时间允许）或 v3.1

</deferred>

---

*Phase: 29-v3-worker*
*Context gathered: 2026-04-18*
