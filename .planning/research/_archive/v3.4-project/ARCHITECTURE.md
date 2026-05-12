# Architecture Research — v3.0 远端开发体验升级

**Domain:** 容器化 SSH 云主机 + 透明远程 CLI 的文件映射与会话可靠性升级
**Researched:** 2026-04-18
**Confidence:** HIGH（基于 v2.0 真实代码 + Mutagen/mergerfs 官方文档）
**Mode:** 架构集成研究（Feature → 组件 → Phase 切分）

> 本文不重做 v2.0 已交付的单宿主机 + 控制面 + host-agent + 容器 + sing-box tun 模型研究。
> 前提：v2.0 的网络模型（sing-box tun 全隧道 + nftables 默认拒绝 + 三重校验）**完全不动**。
> 焦点：v3.0 的 8 项 feature 如何插入到既有组件上，并给 roadmapper 一份可直接拆 phase 的清单。

---

## 1. v2.0 现状快照（改造基准线）

**代码路径参考（所有 v3.0 改造都发生在这些位置）：**

| 组件 | 代码位置 | v2.0 现状 |
|------|---------|----------|
| cloud-claude 入口 | `cmd/cloud-claude/main.go` | cobra 根命令 + `init` / `env check` / `ssh doctor` 子命令 |
| CLI 核心流程 | `internal/cloudclaude/ssh.go:ConnectAndRunClaude` | 三阶段：`sshConnect` → `mountWorkspace` → `runClaude` |
| 本地映射 | `internal/cloudclaude/mount.go` | sshfs passive + 嵌入式 SFTP server（pkg/sftp） |
| 命令代理 | `internal/cloudclaude/execproxy.go` | 本地 Unix socket + 容器内 wrapper 脚本 |
| Entry API | `internal/controlplane/http/entry.go:Auth` | `POST /v1/entry/{shortId}/auth` → `{ssh_user, ssh_pass, ssh_host, ssh_port, status}` |
| Worker | `internal/runtime/tasks/worker.go:createHost` | `docker create --network bridge --cap-add NET_ADMIN,SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined -v <home_dir>:/workspace` |
| 受管镜像 | `deploy/docker/managed-user/Dockerfile` | Ubuntu 24.04 + sshd + sshfs/fuse3 + **tmux 已预装** + claude-code npm + KasmVNC + Chromium |
| 容器 entrypoint | `deploy/docker/managed-user/entrypoint.sh` | sshd 前台启动 + KasmVNC + 用户改名 + FUSE 权限修复 |
| claude wrapper | `deploy/docker/managed-user/claude-wrapper.sh` | `/usr/local/bin/claude` → `exec claude-real "$@"` |
| 数据模型 | `internal/store/migrations/0007_auth_unification.sql` | `claude_accounts (id, user_id, host_id, email, display_name, status)` |
| agent 契约 | `internal/agentapi/contracts.go:HostActionRequest` | 已有 `Labels map[string]string`、`HomeMount`，无 `Volumes` 字段 |

**关键发现：**
- `tmux` 在 Dockerfile 第 16 行**已经安装**，F4 会话恢复所需的运行时已就绪，不需要新包。
- `HostActionRequest` 已有 `Labels` 和单一 `-v home:/workspace` 挂载；F7 需要**扩展为可追加任意 volume 挂载**（或在 agent 内推断 claude_account volume 名）。
- `ConnectAndRunClaude` 是串行的三阶段流水线，`mountWorkspace` 启动 sshfs 并阻塞等 `mountpoint -q` 就绪（默认 10s 超时）；Mutagen 可以与之并发。
- Entry API 返回结构**极简**（只返回 SSH 连接四元组），没有容器元数据——v3.0 需要决定扩展这里还是新增 endpoint。

---

## 2. CLI 启动流程改造（F1-F5）

### v2.0 启动序列（串行、无并发点）

```
main.runRoot
  ├── LoadConfig
  ├── EntryClient.AuthenticateAndWait       ← HTTP 轮询直到 status=ready
  └── ConnectAndRunClaude(cwd, args, proxyCommands)
        ├── sshConnect                      ← TCP + SSH 握手 (~300ms~1s)
        ├── mountWorkspace(conn, cwd, cwd)  ← 启动容器内 sshfs + 本地 SFTP server
        │     ├── sshRun: mkdir -p + chown
        │     ├── NewSession → exec "sshfs : <remote> -o passive -f"
        │     ├── 启动本地 sftp.Server
        │     └── waitForMount: 轮询 mountpoint -q（≤10s）
        ├── (可选) ExecProxy.Start + InstallWrappers
        └── runClaude                       ← PTY + session.Start(remoteCmd)
```

**阻塞点：** `waitForMount` 之前不能启动 `runClaude`；Mutagen 初次同步（F1）也不能与 claude 进程并发，否则 Claude Code 启动时看到的 `/workspace` 可能缺文件。

### v3.0 目标序列（有明确并发点 + 会话包装）

```
main.runRoot (cloud-claude v3.0)
  ├── LoadConfig + mount-mode 参数解析          ← F2: --mount-mode flag
  ├── EntryClient.AuthenticateAndWait            ← F6: 增加 "正在检测网络…" 进度回调
  │
  ├── sshConnect (长心跳)                        ← F3: ClientConfig.KeepAlive + Timeout↑
  │
  ├─┬─ [并发开始] ────────────────────────────────
  │ │
  │ ├── 冷兜底分支：mountSSHFSCold                ← F1: sshfs → /mnt/cold
  │ │      启动 → waitForMount（≤10s，失败降级）
  │ │
  │ ├── 热同步分支：startMutagenSync              ← F1: mutagen sync create
  │ │      创建 session(alpha=cwd, beta=/mnt/hot) → 等待 "Watching" 或首轮同步就绪
  │ │      （不等全量完成，只等 Mutagen daemon accept + agent 握手）
  │ │
  │ ├─ [并发结束] ────────────────────────────────
  │
  ├── mergerfsCompose                              ← F1: 容器内 mergerfs /mnt/hot:/mnt/cold=RO → /workspace
  │     （由 agent 或 entrypoint 在容器启动时常驻；CLI 只负责触发/校验）
  │
  ├── autoFallback 决策                            ← F2: 任一分支失败 → 降级模式 + 明确告知
  │     - mutagen 失败 → sshfs-only 模式
  │     - sshfs 失败 → mutagen-only 模式（冷文件不可见）
  │     - 两者都失败 → 终止 + 错误码
  │
  ├── (可选) ExecProxy.Start                       ← v2.0 已有
  │
  ├── attachOrStartSession                         ← F4+F5: tmux 默认壳 + 多端 attach
  │     ├── 探测容器内是否存在 "cloud-claude-<uid>" tmux session
  │     │     - 存在 + 非 --new-session → attach（F5 多端）
  │     │     - 存在 + --new-session → 新建带后缀 session
  │     │     - 不存在 → tmux new-session -s ... (F4)
  │     └── 透传 TTY/signal/winch/退出码（保留 v2.0 所有既有能力）
  │
  └── runClaude-inside-tmux                        ← claude 在 tmux session 内运行
        错误路径统一包装 v2.0 错误码 (F8)
```

### 并发点可行性

| 并发组 | 可否并发 | 依据 |
|--------|---------|------|
| Mutagen start ‖ sshfs mount | ✅ 可并发 | 两者目标目录不同（`/mnt/hot` vs `/mnt/cold`），独立的 SSH session channel，互不阻塞 |
| Mutagen 首轮同步 ‖ Claude Code 启动 | ⚠️ 不建议 | Claude Code 启动时扫描 workspace，同步未稳定会看到不一致状态。**应等 Mutagen 首轮同步完成再进 tmux**。 |
| mergerfs 组合 ‖ Mutagen sync | ❌ 不能 | mergerfs 要求 hot + cold 分支都已 ready 才能 union mount |
| SSH 握手 ‖ EntryAPI 轮询 | ✅ 已可并发（v2.0 串行） | 可以在 `Authenticate` 首次返回 `ready` 后立即开始 TCP dial，Mutagen daemon 也可本地预热 |

**建议：** 保留"SSH 握手 → (Mutagen ‖ sshfs) → mergerfs → tmux → claude"五阶段管线，把并发点局限在第二阶段（Mutagen 启动 ‖ sshfs 挂载）。首次连接到能交互 ≤8s 的验收基线意味着必须把这两个分支做成并发。

### cloud-claude 包结构改造

| 新增文件 | 职责 | 备注 |
|---------|------|------|
| `internal/cloudclaude/mount_mutagen.go` | Mutagen 会话生命周期（create/monitor/terminate）+ 白名单过滤配置 | 调用 `mutagen` CLI 或嵌入式 Go client |
| `internal/cloudclaude/mount_merge.go` | mergerfs union 编排（触发容器内 mount） | 通过 ssh exec 执行，或依赖 agent 注入 |
| `internal/cloudclaude/mount_strategy.go` | `--mount-mode` 解析 + 降级状态机 | F2 核心 |
| `internal/cloudclaude/session.go` | tmux attach / new / conflict 决策 | F4+F5 |
| `internal/cloudclaude/session_health.go` | SSH KeepAlive + 断线重连探测 | F3 |
| `internal/cloudclaude/doctor.go` | 5 维度自检 + 一键修复 | F6（新增 `cloud-claude doctor` 子命令） |
| `internal/cloudclaude/errcodes.go` | 错误码常量 + 中文消息模板 | F8 |

**复用的 v2.0 代码：**
- `sshConnect`（保持）
- `runClaude`（**拆为两部分**：PTY setup + command build，command 改为 `tmux new/attach ... -- claude`）
- `ExecProxy` / `InstallWrappers`（完全保持）
- `waitForMount`（复用给 Mutagen 监控）

**被替换的 v2.0 代码：**
- `mountWorkspace` → 新 `mountStrategy.Mount()`（内部编排 3 种模式）
- 旧 `mountWorkspace` 逻辑迁移到 `mount_sshfs.go` 作为降级分支

---

## 3. 受管镜像改造（F1/F4/F6/F7 交叉）

**当前已具备：** `tmux`（F4）、`sshfs` + `fuse3`（F1 冷兜底）、`claude-real`、`sudo`、`openssh-server`。

### 新增镜像组件

| 组件 | 类型 | 版本/来源 | 为什么在镜像而不是 entrypoint 注入 |
|------|------|----------|---------------------------------|
| `mergerfs` | apt 包 | `mergerfs` (Ubuntu 24.04 universe ≥ 2.34) | 用户态 FUSE 模块，必须 **预装**；启动时再装会延长首次进容器时间 |
| `mutagen-agent` | 二进制 | `mutagen-agent_linux_amd64/arm64` from GitHub release | 必须预装于 `/usr/local/bin/`，Mutagen daemon 远端握手时期望 agent 已经在 PATH |
| `dtach` | apt 包 | 可选，降级备选（tmux 功能过剩时） | 建议不装，tmux 已够用 |
| `jq` / `curl` | 已有 | - | doctor 自检用 |

### 新增 entrypoint 注入逻辑（必须在容器启动时动态执行）

| 注入动作 | 时机 | 依据 |
|---------|------|------|
| `mkdir -p /mnt/hot /mnt/cold /workspace` + `chown workspace` | entrypoint 前段 | mergerfs 三路径必须提前存在 |
| 启动 mergerfs 守护进程：`mergerfs -o category.create=ff,moveonenospc=false /mnt/hot=RW:/mnt/cold=RO /workspace` | entrypoint 后台启动（在 sshd 之前） | F1 核心；category.create=ff → 新文件落 hot 分支 |
| `/etc/fuse.conf` 启用 `user_allow_other`（**v2.0 已做**） | Dockerfile 已完成 | 无需改 |
| `mutagen-agent install` 到 `/usr/local/bin/`（如果用 embedded agent） | 不需要，预装即可 | Mutagen 只需 agent 在 PATH，初次连接会自动使用 |
| 创建 `/var/lib/claude-persist` 挂载点并 chown | entrypoint | F7：卷在 docker create 时挂入，entrypoint 负责权限修复 |
| 如果 `/var/lib/claude-persist` 非空，软链到 `/workspace/.claude` 和 `/workspace/.cache/claude` | entrypoint | F7：让 claude-code 透明使用持久卷 |

### 镜像不改动

- OpenSSH / KasmVNC / Chromium / Xvnc 全部保留
- `/usr/local/bin/claude` wrapper 保留（因为 F4 需要 `tmux` 包装，**wrapper 里不用加 tmux**，tmux 由 cloud-claude CLI 在 SSH 层发起）

### 镜像版本凸变

**必须凸 `image.lock`：** Dockerfile 改动（增加 mergerfs + mutagen-agent）+ entrypoint 改动（启动 mergerfs）。建议版本跳到 `v2.0.0 → v3.0.0`，标记一个大版本边界，便于旧容器按需重建。

---

## 4. 容器启动参数改造（Worker）

### v2.0 `createHost` 当前参数（`internal/runtime/tasks/worker.go:157`）

```
--name <containerName>
--network bridge
--cap-add NET_ADMIN --cap-add SYS_ADMIN
--device /dev/fuse
--security-opt apparmor=unconfined
--label cloud-cli-proxy.managed=true + host_id=...
--hostname <hostname>
--shm-size 1g
--sysctl net.ipv6.conf.all.disable_ipv6=1
-e TZ=... LANG=... CONTAINER_USER=... CONTAINER_SSH_PASSWORD=...
-v <homeDir>:/workspace
```

### v3.0 必须新增

| 参数 | 为什么 | 新增位置 |
|------|-------|---------|
| `-v <claudeAccountVolumeName>:/var/lib/claude-persist` | F7 Claude Code 状态持久化 | `worker.go:createHost` args 拼接处；volume 名由 claude_account.id 推导 |
| `--label cloud-cli-proxy.claude_account_id=<uuid>` | 关联 volume ↔ account ↔ host | 便于 GC 和审计 |
| `--label cloud-cli-proxy.mount_modes=mutagen+sshfs+mergerfs` | doctor/admin 可观测 | 标记容器能力集（v3 vs v2 镜像） |

### v3.0 保持不变

- `--cap-add SYS_ADMIN` + `--device /dev/fuse` + `apparmor=unconfined`：mergerfs 和 sshfs 都是 FUSE，**刚好复用 v2.0 已经开通的能力**，**不需要新增 capabilities 或 device**。
- `--network bridge`：不变。sing-box tun 注入是后续由 `provider.PrepareHost` 做的，和启动参数解耦。
- `--shm-size 1g`：不变。

### Volume 生命周期决策

| 决策点 | 推荐方案 | 原因 |
|-------|---------|------|
| Volume 命名 | `claude-persist-<claude_account_id>` | 与 `claude_accounts.id` 一一对应，便于运维查找 |
| 创建时机 | 容器首次 create 时 `docker volume create --label cloud-cli-proxy.claude_account_id=... --label managed=true` | 幂等，不依赖外部脚本 |
| 删除时机 | **只在 claude_account 硬删除时触发**，host rebuild 不删 | F7 核心承诺：容器重建不丢登录 |
| `rebuild --wipe-/workspace` | **保留 claude-persist，只清空 /workspace** | v2.0 逻辑已经只删 `homeDir`，volume 天然不受影响 |

### Worker 改造工作量

- `createHost` 参数拼接：**S**（10-20 行）
- `HostActionRequest` 新增 `ClaudeAccountID string` 字段 + 控制面下发：**S**
- Volume 创建/GC 辅助函数：**S**
- 总体 Worker 改造：**S-M**

---

## 5. 控制面扩展（仅必要项）

### 必须改

| 改动 | 原因 | 范围 |
|-----|------|------|
| `claude_accounts` 表新增 `persistent_volume_name TEXT`（可空，首次 attach 时 lazy 写入） | F7 持久卷 ↔ account 映射 | migration + repository + model struct |
| `HostActionRequest` 新增 `ClaudeAccountID` + `PersistentVolumeName` + `Volumes []VolumeMount{Source,Target,ReadOnly}` | 把持久卷挂载从控制面下发到 agent | agentapi contracts + control-plane start_host task 构建处 |
| 启动 host 时查询当前绑定的 claude_account 并解析 volume 名 | start_host 任务路径 | `internal/controlplane/tasks/start_host.go` 或等效位置 |

### 可以不改（v3.0 不做）

| 诱惑 | 为什么不做 |
|-----|-----------|
| `claude_accounts.preferred_mount_mode` 字段 | 用户偏好在 CLI 本地 config (`~/.cloud-claude/config.yaml`) 持久化足够，v3.0 admin 不做相关管理界面 |
| 多端 session 状态实时同步到控制面 | tmux session 完全由容器内管理，控制面不需知晓；如要可观测，走 host-agent 的 `ContainerStatusResponse` 扩展 |
| 新增 admin 管理页面展示 mount 模式 / session 数 | v3.0 out of scope 明确包含这个；只在 host 详情页可以加一行 `image_version` / `mount_modes` label 展示，**不做新页面** |
| 新增 "session 管理" REST endpoint | v3.0 CLI 完全在 SSH 层解决多端协作，不需要服务端介入 |

### Entry API 兼容性策略（关键）

**向后兼容约束：** v2.0 已发布的 cloud-claude 客户端依赖 `{ssh_user, ssh_pass, ssh_host, ssh_port, status}` 结构，**不能破坏**。

**建议：扩展字段、不破坏旧字段**

```json5
// v3.0 AuthResponse 扩展（JSON 响应，新增字段旧客户端安全忽略）
{
  "status": "ready",
  "ssh_user": "...", "ssh_pass": "...", "ssh_host": "...", "ssh_port": 2222,
  // 新增：
  "image_version": "v3.0.0",         // 客户端据此决定走 v3 流程还是 v2 流程
  "supports_mutagen": true,           // 是否可以跑 mutagen-agent
  "supports_mergerfs": true,          // 是否容器里有 /workspace/(hot|cold)
  "claude_account_id": "<uuid>",      // F7: CLI 本地缓存 mutagen session 名用
  "persistent_volume": "claude-persist-<uuid>"  // 仅展示/调试用
}
```

**客户端协议：** cloud-claude v3.0 读取 `supports_mutagen` 和 `supports_mergerfs`；两者都 false → 自动走 v2 sshfs 模式（等价于 `--mount-mode=sshfs-only`）。这样**同一二进制可以兼容新老容器**，避免用户被迫重建所有 host。

### 控制面改造工作量

- migration + repository 新字段：**S**
- HostActionRequest 扩展 + start_host 构建 volume 信息：**S**
- Entry API 扩展返回字段：**S**
- 总体控制面改造：**S**（单个 phase 可完成）

---

## 6. host-agent 扩展

### 不需要扩展的部分（重要结论）

**F4（tmux 会话恢复）和 F5（多端 attach）完全在 CLI ↔ 容器 SSH 层解决，host-agent 不参与。**

原因：
- tmux session 状态在容器内，CLI 通过 SSH `tmux has-session -t <name>` 探测
- 多端冲突决策是 CLI 逻辑（询问用户 or 按 `--new-session` 行为）
- host-agent 的 Unix socket 契约（创建/启停/重建 host）与运行时 session 管理正交

**F3（SSH 长心跳）** 完全在 cloud-claude 的 `ssh.ClientConfig` 配置和客户端应用层实现，host-agent 和容器内 sshd 配置（`deploy/docker/managed-user/sshd_config`）最多需要确认 `ClientAliveInterval` / `ClientAliveCountMax` 合理，**但这是 sshd_config 改动，不是 agent 接口改动**。

### 需要扩展的部分

| 改动 | F | 范围 |
|-----|---|------|
| `Volumes` 字段解析和 docker create 参数拼接 | F7 | `internal/runtime/tasks/worker.go` |
| Docker volume 预创建（幂等 `docker volume create`） | F7 | Worker 新增辅助 |
| 扩展 `ContainerStatusResponse` 返回 image_version/labels（可选，供 doctor 用） | F6 | 有利于本地 `cloud-claude doctor` 走 Entry API 后经 SSH 查容器 label 做自检 |

### doctor 与 agent 的边界

`cloud-claude doctor` 5 维度自检中：

| 维度 | 探测路径 | 是否经 agent |
|-----|---------|------------|
| network | 本地 TCP dial gateway + 容器内 `curl ifconfig.me` | **不经 agent**（走 SSH） |
| auth | Entry API 401/403 测试 | **不经 agent** |
| ssh | SSH 握手 + keepalive 往返 | **不经 agent** |
| mount（三层） | SSH 探测 `mountpoint -q /workspace` + `mergerfs -V` + `mutagen sync list` | **不经 agent** |
| disk | SSH `df -h /workspace /var/lib/claude-persist` | **不经 agent** |

**结论：doctor 完全是本地 + SSH 实现，不给 host-agent 加 endpoint。**

---

## 7. 数据流图（v3.0 完整）

### 7.1 文件映射数据流（Mutagen + sshfs + mergerfs）

```
┌─────────────────────────────── 本地（开发者 macOS/Linux） ─────────────────────────────┐
│                                                                                        │
│   用户 cwd: ~/project                                                                   │
│     │                                                                                  │
│     ├─── Mutagen daemon ──(per-user, long-lived)──► SSH channel ──┐                    │
│     │      alpha=~/project                                         │                   │
│     │      ignore: .git/, node_modules/, ≤50MB, ext:{.go,.ts,...}  │                   │
│     │                                                              │                   │
│     ├─── cloud-claude CLI ──────────────────────── SSH master ─────┼─►                 │
│     │    （cobra 入口）                                              │                  │
│     │                                                              │                   │
│     └─── 嵌入式 SFTP server (pkg/sftp) ─── SSH channel ─────────────┤                   │
│          workdir=~/project                                         │                   │
│          为 sshfs passive 模式提供 SFTP 端点                        │                   │
│                                                                    │                   │
└────────────────────────────────────────────────────────────────────┼───────────────────┘
                                                                     │
                                                  TCP/22 over sing-box tun (EGRESS IP)
                                                                     │
┌────────────────────────────────────────────────────────────────────▼───────────────────┐
│                        容器（--network=none + tun 注入）                                 │
│                                                                                        │
│   sshd (OpenSSH 10.2p1) ──┬── session 1: cloud-claude command channel                  │
│                           ├── session 2: sshfs (passive mode) reads stdin              │
│                           │   → mount /mnt/cold（FUSE）                                 │
│                           ├── session 3: mutagen-agent                                 │
│                           │   → writes /mnt/hot（FUSE via agent internal）              │
│                           └── session 4+: tmux attach/new                              │
│                                                                                        │
│   ┌──────────────────────────────────────────────────────────────────────┐             │
│   │              mergerfs FUSE union（entrypoint 启动）                    │             │
│   │   /mnt/hot (RW)  +  /mnt/cold (RO)   →   /workspace                  │             │
│   │   policy: category.create=ff → 新文件落 hot                           │             │
│   │                                                                      │             │
│   │   读路径：先查 hot，未命中查 cold                                     │             │
│   │   写路径：hot，Mutagen watcher 捕获 → 反向同步到 alpha                 │             │
│   └──────────────────────────────────────────────────────────────────────┘             │
│                                   │                                                    │
│                                   ▼                                                    │
│   Claude Code（在 tmux session 内） 读写 /workspace                                      │
│                                   │                                                    │
│   /var/lib/claude-persist（Docker named volume，F7）                                     │
│     symlink → /workspace/.claude（登录态） /workspace/.cache/claude（缓存）              │
│                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 多端 SSH attach 数据流（F4+F5）

```
  Mac cloud-claude ──┐
                     │
  Linux cloud-claude ┼───► 同一 container 的 sshd 
                     │        │
  （更多端…） ──────┘        │
                              ├──► tmux session "cloud-claude-<uid>"（由第一个 attach 创建）
                              │     │
                              │     └── Claude Code 进程（运行中）
                              │
                              └──► 每个新 SSH session 跑 `tmux attach -t cloud-claude-<uid>`
                                   → 共享输入/输出，PTY 尺寸跟最新接入端
                                   
  (断网恢复)
  cloud-claude 检测 KeepAlive 失败 → 提示"网络抖动，正在重连..."
    → 建立新 SSH 连接 → tmux attach 回到原 session → Claude Code 进程和历史无损
```

### 7.3 CLI 启动时序

```
T=0    cloud-claude
T+50ms LoadConfig
T+100ms AuthenticateAndWait（网关 HTTP）
T+500ms status=ready 返回（含 image_version/supports_mutagen）
T+600ms sshConnect（TCP + SSH 握手）
T+1.2s  SSH ready
        ┌─────────────────┬──────────────────┐
T+1.3s  sshfs mount       mutagen session    （并发）
        启动                create
T+2.5s  mountpoint-q OK    daemon Watching
        └─────────────────┴──────────────────┘
T+2.6s  mergerfs 确认（SSH exec mount -l | grep /workspace）
T+2.7s  ExecProxy + wrapper 安装（如配置）
T+2.8s  tmux has-session？
          → 不存在：tmux new-session -s cloud-claude-<uid> -- claude ...
          → 存在（单端）：tmux attach-session
T+3s    PTY raw + 进入 claude 交互
        （剩余 Mutagen 首轮同步继续后台跑，1-5s 内稳定）
```

**验收基线（≤8s 首次连接到能交互）** 在上述时序下**可达**——主要风险是 Mutagen 初次全量扫描大仓库时间，白名单（`.git/` 忽略、`≤50MB` 忽略）是关键。

---

## 8. 建议的 Phase 拆分（4-7 个）

**核心依赖拓扑：**

```
Phase A (镜像+Worker 基建)
   │
   ├─► Phase B (CLI 文件映射重构) ───────────────┐
   │     └─ 依赖 A 的 mergerfs/mutagen-agent      │
   │                                              │
   ├─► Phase C (控制面 + Entry API + 持久卷)     │
   │     └─ 可与 B 并行（接触不同代码路径）       │
   │                                              │
   │                             ├─► Phase D (会话可靠性：tmux+长心跳+多端)
   │                             │     └─ 依赖 B 的 CLI 架构骨架
   │                             │
   │                             └─► Phase E (doctor + 错误码 + 降级策略)
   │                                   └─ 依赖 B/C/D 都具备，做总装验收
   │
   └─► Phase F (E2E 稳定化 + 性能验收)
         └─ 所有前置 phase 完成后做
```

### Phase 清单

| # | Phase 名称 | 范围 | Features | 工作量 | Depends on | 可并行 |
|---|-----------|------|----------|-------|-----------|-------|
| **1** | **受管镜像 v3 + Worker 容器参数扩展** | Dockerfile 加 mergerfs/mutagen-agent；entrypoint 启动 mergerfs；Worker 支持 `Volumes` 字段；image.lock 凸到 v3.0.0 | F1 基建，F7 基建 | **M** | - | 独立 |
| **2** | **控制面数据模型 + Entry API 扩展** | claude_accounts 新增 `persistent_volume_name`；HostActionRequest 新增 volume/account_id 字段；Entry API 新增 `image_version/supports_*` 返回；start_host 构建 volume | F7 控制面侧，F1/F2 握手字段 | **S-M** | - | 与 Phase 1 并行 |
| **3** | **CLI 三层文件映射重构** | cloud-claude 拆分 `mount_strategy` + `mount_mutagen` + `mount_sshfs` + `mount_merge`；实现 `--mount-mode` 降级；Mutagen 白名单配置 | F1, F2 | **L** | 1, 2 | - |
| **4** | **SSH 会话可靠性与 tmux 包装** | `session.go` tmux attach/new；SSH `KeepAlive` + 断线重连；`--new-session` flag；多端冲突中文提示 | F3, F4, F5 | **M** | 3（复用 CLI 架构） | 可与 Phase 5 并行 |
| **5** | **Claude Code 状态持久化（F7 CLI 端 + 镜像端 symlink）** | entrypoint symlink `/var/lib/claude-persist` ↔ `~/.claude`、`~/.cache/claude`；docker volume create / GC；admin host 详情页可选展示 volume 名 | F7 完整闭环 | **S-M** | 1, 2 | 可与 Phase 4 并行 |
| **6** | **cloud-claude doctor v3 + 错误码统一** | `doctor` 5 维度子命令；错误码常量 + 中文"下一步"文案；新架构所有错误路径纳入 v2.0 错误码体系 | F6, F8 | **M** | 3, 4, 5 | 最后做 |
| **7** | **E2E 稳定化 + 性能验收** | `rg`/`ls -R` 基准测试；弱网抖动 30s 验证；10k 文件源码树验证；部署文档/运维手册更新 | 验收 | **S-M** | 1-6 | 最后做 |

**合并选项（如果要压到 4 个 phase）：**
- Phase 1+2 合并为 "v3 基建"（镜像 + Worker + 控制面，一次 migration）
- Phase 4+5 合并为 "会话与持久化"
- Phase 6+7 合并为 "收尾"

**不建议合并：** Phase 3（CLI 文件映射）一定要独立，因为这是整个 v3.0 最大的技术风险点——Mutagen+mergerfs+sshfs 的三层编排和降级路径复杂度高。

---

## 9. v2.0 代码变更清单（替换/扩展/保持）

| 文件 | 状态 | 说明 |
|------|-----|------|
| `cmd/cloud-claude/main.go` | **扩展** | 加 `--mount-mode` flag + `doctor` 子命令；runRoot 调用 MountStrategy 而非直接 ConnectAndRunClaude |
| `internal/cloudclaude/mount.go` | **重命名为 mount_sshfs.go** | 逻辑本体保留，作为降级分支 |
| `internal/cloudclaude/ssh.go:ConnectAndRunClaude` | **重构** | 拆为 `sshConnect` + 暴露的 Claude 会话启动器（接收 tmux session name） |
| `internal/cloudclaude/ssh.go:runClaude` | **扩展** | 远程命令从 `cd ... && claude` 改为 `tmux new/attach ... -- claude`（由 session.go 构造） |
| `internal/cloudclaude/execproxy.go` | **保持** | 不动 |
| `internal/cloudclaude/execproxy_scripts.go` | **保持** | 不动 |
| `internal/cloudclaude/config.go` | **扩展** | 新增 mount-mode 持久化字段；保持现有字段 |
| `internal/cloudclaude/entry.go` | **扩展** | `AuthResponse` 新增 `ImageVersion` / `SupportsMutagen` / `SupportsMergerfs` / `ClaudeAccountID` 字段（JSON 兼容） |
| `internal/cloudclaude/envcheck.go` | **保持** | doctor 可以复用部分逻辑，但独立文件 |
| `internal/cloudclaude/ssh_doctor.go` | **保持** | 现有 ssh-key doctor 继续独立，新 `doctor` 是更宏观的 5 维度自检 |
| `internal/controlplane/http/entry.go:Auth` | **扩展** | 返回增加字段；查找 claude_account 和 image version |
| `internal/controlplane/http/...` 新增 endpoint | **不需要** | v3.0 只扩展 entry.go 返回 |
| `internal/runtime/tasks/worker.go:createHost` | **扩展** | args 拼接新增 volume/label；其他保持 |
| `internal/agentapi/contracts.go:HostActionRequest` | **扩展** | 新增 `ClaudeAccountID`、`Volumes []VolumeMount` |
| `internal/store/migrations/` | **新增 migration** | `0014_claude_account_persistent_volume.sql` |
| `deploy/docker/managed-user/Dockerfile` | **扩展** | +mergerfs +mutagen-agent |
| `deploy/docker/managed-user/entrypoint.sh` | **扩展** | 启动 mergerfs；volume symlink；清理逻辑 |
| `deploy/docker/managed-user/image.lock` | **凸版本** | v3.0.0 |
| `deploy/docker/managed-user/sshd_config` | **扩展** | 确认 `ClientAliveInterval 30` + `ClientAliveCountMax 10`（容忍 5 分钟静默） |
| `deploy/docker/managed-user/claude-wrapper.sh` | **保持** | tmux 由 cloud-claude CLI 层控制，wrapper 不动 |
| `internal/network/*`（sing-box tun / nftables） | **保持** | v3.0 不动网络模型 |
| `internal/sshproxy/*` | **保持** | v2.0 已确认零改造支持 multi-session |
| `control-plane/` + `host-agent/` 服务二进制 | **保持** | 只是结构体字段扩展，主流程不变 |
| `web/`（React admin SPA） | **可选小改** | host 详情页可加 `image_version` 展示；不新增页面 |

---

## 10. 已知风险与降级策略

### HIGH 风险

1. **Mutagen daemon 与 sing-box tun 的 SSH 连接冲突**
   - 现象：Mutagen daemon 管理 SSH session，但容器网络走 sing-box tun，从客户端视角一切正常，但如果 tun 路由表变化（出口 IP 重新绑定）会中断长连接
   - 缓解：Mutagen 内置 session 重连；cloud-claude 在 `PrepareHost` 期间不要切换 egress；文档明确说明"切换出口 IP 会中断同步 session"

2. **mergerfs + FUSE + AppArmor 嵌套 FUSE 兼容性**
   - 现象：mergerfs 是 FUSE，sshfs/mergerfs 都走 `/dev/fuse`，内核有并发限制
   - 缓解：v2.0 已开 `apparmor=unconfined`；测试 3 路并发（sshfs + mutagen-agent FUSE + mergerfs）；Phase 7 列为硬性验收项

3. **Mutagen 首轮同步时间不可控**
   - 现象：10k 文件仓库首次 `mutagen sync create` 可能 30s+，违反 ≤8s 基线
   - 缓解：白名单严格（ignore `.git/`、`node_modules/`、`≤50MB`）；默认模式 = `one-way-safe`（alpha→beta）加速；允许 `--mount-mode=sshfs-only` 绕过

### MEDIUM 风险

4. **tmux session 跨端 PTY 尺寸冲突**
   - 现象：Mac (80x24) + Linux (120x40) 同时 attach，tmux 按最小尺寸渲染
   - 缓解：文档明确；或 `cloud-claude` 连接时提示"当前 session 已被 N 端 attach，尺寸 XxY"

5. **持久卷 orphan**
   - 现象：claude_account 删除但 volume 没删
   - 缓解：控制面删除 account 时触发 agent 的 `volume rm`；定时 reconciler 扫 `docker volume ls --filter label=managed=true` 对账

### LOW 风险

6. **错误码爆炸**
   - 现象：3 种 mount 模式 × 5 种失败路径 = 15+ 错误码
   - 缓解：Phase 6 设计错误码命名规范（`mount_<layer>_<reason>`），统一到 v2.0 错误码映射表

---

## 11. Sources

- Mutagen 官方文档（daemon + synchronization）— https://mutagen.io/documentation/introduction/daemon, https://mutagen.io/documentation/synchronization（HIGH 置信度）
- mergerfs 官方文档（remote filesystems + SSHFS 兼容性）— https://trapexit.github.io/mergerfs/latest/remote_filesystems/（HIGH）
- mergerfs-docker 参考（cap_add/device 配置范式）— https://github.com/hvalev/mergerfs-docker（HIGH）
- v2.0 真实代码审读：
  - `cmd/cloud-claude/main.go`
  - `internal/cloudclaude/{ssh,mount,entry,config}.go`
  - `internal/runtime/tasks/worker.go`
  - `internal/controlplane/http/entry.go`
  - `internal/agentapi/contracts.go`
  - `deploy/docker/managed-user/{Dockerfile,entrypoint.sh,claude-wrapper.sh}`
  - `internal/store/migrations/0007_auth_unification.sql`
- v2.0 里程碑复盘 `.planning/RETROSPECTIVE.md`（复用现有基础设施是 v2.0 成功关键）

---

*Architecture integration research for: cloud-cli-proxy v3.0 远端开发体验升级*
*Researched: 2026-04-18*
*Confidence: HIGH — 所有 v2.0 组件已源码级验证；Mutagen/mergerfs 行为基于官方文档*
