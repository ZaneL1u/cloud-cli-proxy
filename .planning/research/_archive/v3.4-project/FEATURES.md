# Feature Research — v3.0 远端开发体验升级

**Domain:** 容器化远程开发 CLI / 云端 SSH 开发环境
**Researched:** 2026-04-18
**Confidence:** HIGH（核心行为全部以官方文档、release notes、issue 讨论和竞品源码交叉验证）

---

## 研究范围与对标产品

| 对标产品 | 主要参考维度 |
|----------|--------------|
| VS Code Remote-SSH | 断线重连体验、`reconnectionGraceTime`、keepalive 策略 |
| GitHub Codespaces / Dev Containers | 持久化 home、dotfiles、named volume 挂载范式、`postCreateCommand` |
| JetBrains Gateway / Toolbox | 远程会话列表、Running 指示器、IDE 设置跨重启保留 |
| Mosh | 弱网 UX：本地 echo、`Last contact N seconds ago` 警告、roaming |
| Eternal Terminal | 基于序列号的透明重连、`-x` 杀掉旧 session |
| tmate / tmux | 多端 attach 行为、`new-session -A`、`attach -d`、`detach-client -a` |
| Mutagen | `sync list`/`monitor`/`flush` 状态可见性、`two-way-safe` 默认语义 |
| mergerfs | 联合目录性能特征、多分支故障模式 |
| Homebrew / flutter doctor / npm doctor | CLI 诊断输出的行业范式 |
| Claude Code 官方 | `~/.claude/.credentials.json` 与 `CLAUDE_CONFIG_DIR` 持久化契约 |

---

## 跨 Feature 横向主题（先约束再下沉）

以下四条是所有 feature 共享的行为契约，**不应在单个 feature 内重复决策**。

### M1 · 多端同账号连接的推荐 UX 模式

**业界主流分成三派**：

| 模式 | 代表产品 | 默认行为 |
|------|----------|----------|
| "独占 + 踢人" | `tmux attach -d`、`ssh -O check`、Eternal Terminal `-x` | 新客户端接入时**主动挤掉**旧的，旧客户端收到断开 |
| "共享观察 + 可切换只读" | tmate（`-r` 只读）、tmux 默认 | 所有端同时看到同一终端，按键默认都能写 |
| "并行多实例" | JetBrains Gateway / VS Code Remote | 每端一个独立后台 IDE，互不干扰 |

**对我们的推荐**（综合"单容器 + 共享 tmux session + 中文使用者直觉"）：

- **默认行为 = 共享 attach，不踢人。** 第二端 `cloud-claude` 进入后，直接 `tmux attach -t claude` 和第一端看同一屏幕、同一 claude 进程。依据：符合用户直觉（"我两个屏都能看 claude 在干嘛"），和 tmate / tmux 原生默认一致，无数据损失风险。
- **显式开关 `--new-session`：** 起一个独立 tmux session（`session_name = claude-<short_id>`），两端互不干扰。用户明确表达"我要并行做两件事"时使用。
- **显式开关 `--take-over` / `--kick`：** 强制挤掉所有其它端并独占（`tmux detach-client -a` + `tmux attach -d`），给"我在家里忘了断开，公司再上"场景兜底。
- **检测并提示：** cloud-claude 连上后，如果发现当前 session 已有 N 个 client attach，在顶部输出一行中文提示（参考 tmate 的 `show-messages` 和 Mosh 开屏 banner）：

```
检测到另外 1 个会话正在 attach（来源：mac-home / 5 分钟前活跃）
按 Ctrl-b Shift+D 查看并管理连接，或使用 --new-session 开独立会话。
```

> **参考：** tmux `new-session -A -s NAME`、`attach-session -d`、`detach-client -a`；tmate `show-messages`；Eternal Terminal `-x` 参数设计。

### M2 · 文件同步状态可见性

**业界现状**：

- **Mutagen CLI：** 必须显式 `mutagen sync list`、`mutagen sync monitor <session>` 才能看状态，状态不会自动冒泡到终端；用户实际使用时只能靠 grep "Conflicts" / "problems" 判断。([Mutagen Issue #277](https://github.com/mutagen-io/mutagen/issues/277))
- **Docker Desktop + Mutagen：** GUI 有一条状态栏显示 syncing / idle；CLI 模式完全依赖 `mutagen monitor`。
- **VS Code Dev Containers：** 完全隐藏 bind mount / named volume 的差异，用户只能靠 `:cached` 修饰词间接了解。

**对我们的推荐**：

1. **每次 `cloud-claude` 启动时打印一次当前 mount mode**（占一行）：

   ```
   ✓ 工作目录已映射到 /workspace（模式：hybrid — mutagen 热同步 + sshfs 冷兜底）
   ```

2. **容器内提供 `/workspace/.cloud-claude-status` 虚文件**（或 `cloud-claude status` 子命令），透出：
   - 当前 mount mode
   - 最近一次 mutagen sync 完成时间 + 待同步文件数
   - sshfs 是否健康
   - mergerfs 是否 ready

3. **Conflict 提示主动向上冒泡**：mutagen 一旦出现 conflict（`two-way-safe` 下），在用户下一次按回车时在 prompt 上方插入一行警告：
   ```
   ⚠ 有 3 个文件同步冲突，运行 cloud-claude sync conflicts 查看
   ```
   依据：Mutagen 社区最痛的反馈就是"冲突发生了用户不知道"（Issue #277）。

### M3 · 弱网容忍的具体阈值

借鉴 Mosh 和 Eternal Terminal 的实际行为：

| 事件 | 阈值 | UX |
|------|------|----|
| keepalive 心跳 | 每 3 秒（参考 Mosh heartbeat） | 无 UI |
| "稍有迟钝" | >1.5s RTT 或 3s 无回包 | 在 prompt 顶部显示灰色 `…` 标识 |
| "明显断连" | >8s 无回包 | 顶部出现黄色提示：`网络抖动中（12 秒未响应，按 Ctrl-b ? 退出）` |
| "超时但仍保活" | >30s 无回包 | 红色提示 `网络已断 35s，正在自动重试…`；输入被本地缓冲，**不提交到服务端** |
| "确认失败" | >10 分钟无回包 且 SSH TCP 已 reset | 提示 `连接已断开，按 Enter 重连（容器内会话已保留，运行中进程不会丢失）` |

> **业界锚点：**
> - Mosh 默认的 "Last contact N seconds ago" 只在无心跳时显示，没有固定秒数阈值，但社区共识是 5–10 秒后开始显示。([Mosh Issue #925](https://github.com/mobile-shell/mosh/issues/925))
> - VS Code `remote.SSH.reconnectionGraceTime` 默认 3 小时（2025.11 落地），远大于前端等待。
> - Linux conntrack UDP 默认 30s，这是 Mosh/ET 实际断连的物理下限。([Moshi 修复指南](https://getmoshi.app/articles/fix-mosh-connection-failed))

### M4 · Doctor 命令的输出格式范式

**行业范式（稳定、可抄）**：

| 产品 | 符号 | 颜色 | 每项输出结构 |
|------|------|------|--------------|
| `flutter doctor` | `[✓]` `[!]` `[✗]` | 绿/黄/红 | 一行标题 + 缩进的细节和"运行 X 修复"建议 |
| `brew doctor` | `opoo` 警告（黄色）+ 结尾 `Your system is ready to brew.` | 黄/紫 | 每项失败才输出，通过则沉默 |
| `npm doctor` | 表格形式（名称/状态/建议值/实际值） | 默认无色 | 每项给"建议动作"列 |

**我们应当遵循的范式**：

1. **符号统一：** `[✓]` 通过 / `[!]` 警告 / `[✗]` 失败。
2. **颜色可关闭：** 尊重 `NO_COLOR` 环境变量，管道输出时自动关色（`IsTerminal()` 判断，参考 `hexview` 模式）。
3. **每项失败必须带"下一步该做什么"**（flutter 模式）：
   ```
   [✗] FUSE 模块
       原因：容器内 /dev/fuse 不可写
       解决：在宿主机上 sudo modprobe fuse && 重启容器
             或运行 cloud-claude doctor --fix
   ```
4. **`--verbose` / `-v` 展开每项内部探测细节**（flutter doctor -v 约定）。
5. **通过时的单行总结**：`全部 5 项检查通过，可以开始使用 cloud-claude。`
6. **退出码语义：** 0 = 全通过；1 = 有警告；2 = 有失败（参考 `brew doctor` 非零退出）。

---

## F1 · 三层文件系统架构（Mutagen + sshfs + mergerfs）

### Table stakes（必须具备）

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| 对用户完全透明的统一目录入口 | Codespaces、Dev Containers 都只暴露单一 `/workspace` | 容器内只有 `/workspace` 一个路径，不要求用户理解 hot/cold 分支 |
| 首次连接有明确 "准备中" 提示 | Dev Containers 启动时显示 "Cloning..." / "Pulling image..."；Codespaces 显示阶段化进度 | cloud-claude 输出 `初始化文件映射 (1/3)：热同步源码中 ...` 三段式进度 |
| Git 元数据（`.git/`）在容器内表现一致 | Dev Containers 和 VS Code Remote 都确保 `git status` 在容器内可用 | `.git/` 必须在热同步范围内，并且 `rg`、`git status` 延迟 ≤ 本地 1.5× |
| 支持跨平台本地路径（macOS / Linux） | Mutagen / Mutagen Compose 的核心卖点 | Darwin 和 Linux 两种本地端都验证过 |

### Differentiators（加分）

| 行为 | 价值 | 复杂度 |
|------|------|--------|
| 用户首次命中未同步文件时的"秒级懒拉" | 对标 Codespaces prebuild：用户感觉不到冷热差别 | HIGH（需要 sshfs readdir + mergerfs 回源） |
| 白名单可定制（通过 `~/.cloud-claude/sync.yaml`） | Mutagen 本身支持 `ignores` 配置；给"我不想同步 `node_modules`"的用户逃生路径 | MEDIUM |
| 显示"本次同步跳过了哪些大文件"（>50MB 被归到冷层） | 比 Mutagen 原生 UX 清楚 | LOW |
| 提供 `cloud-claude sync flush` 主动触发一次完整同步 | 对标 `mutagen sync flush` | LOW |

### Anti-features（具体为什么不做）

| 不做的功能 | 表面上诱人的理由 | 为什么对我们**不适合** | 替代方案 |
|------------|------------------|------------------------|----------|
| Web UI 显示同步进度条 | "用户想看同步状态" | cloud-claude 是 CLI 工具，用户 90% 时间在终端；引入 Web 控制面会放大网关认证/鉴权复杂度，且与 v1.0 架构方向相悖 | 在终端顶部插入单行状态 + `cloud-claude status` 子命令 |
| 让用户配置 `alpha-wins-all` / `beta-wins-all` 冲突模式 | Mutagen 原生支持 5 种 | 新手完全不理解 alpha/beta；远程开发场景下 99% 是 "本地编辑为准"，强默认 = `two-way-safe` 本地优先最稳；开放选项反而放大风险（Mutagen Issue #533 就是用户误选 `two-way-resolved` 导致冲突）| 固化为 `two-way-safe`，冲突时用中文提示让用户**手动删除**败者 |
| 让 Claude Code 直接读写 `/cold`（sshfs 分支） | 避免同步滞后 | sshfs 延迟极高（社区公认写比 NFS 慢 3-10 倍，[mergerfs 文档已指出](https://trapexit.github.io/mergerfs/latest/remote_filesystems/)）；Claude 的 `rg` / `glob` 会扫全目录导致接口雪崩 | 强制所有写入路由到 `/hot`（mergerfs policy=`epmfs` + 写入时优先 alpha） |
| 支持双向 Git（容器内提交直接 push） | "无缝开发" | 容器出网受 sing-box 全隧道限制，Git push 会走受控出口，和本地凭据仓库不一致，引入凭据同步的大坑 | v3.0 强制 "在本地 commit，在容器内 run/build"，把 Git 凭据留在本地 |
| 用 `cachefs` / `bcachefs` 替代 mergerfs | 更新技术听起来"性能更好" | mergerfs 是 FUSE 联合文件系统中唯一在**非 CoW 场景**下稳定的（overlayfs 要求 lowerdir 不可写，和 sshfs + mutagen 同时落盘冲突）| 保持 mergerfs，v3.1 再评估 |

### 用户感知的失败模式（可观察）

| 失败场景 | 用户看到的屏幕 | 错误码提示 |
|----------|----------------|------------|
| Mutagen agent 无法启动 | 连接 2 秒后红色提示：`[✗] 热同步失败（mutagen agent 无响应），已自动降级到 sshfs-only 模式` | `MOUNT_MUTAGEN_001`，建议 `cloud-claude doctor --fix` |
| sshfs 挂载失败（FUSE 不可用） | `[✗] 冷兜底挂载失败，FUSE 内核模块未加载` + 退出码 9 | `MOUNT_SSHFS_002`，建议本地运行 `cloud-claude doctor` |
| mergerfs 合并失败 | `[!] 文件合并层失败，已回退到纯 mutagen 模式（部分文件可能暂不可见）` | `MOUNT_MERGE_003`，降级但仍可工作 |
| 用户本地磁盘满导致同步停滞 | prompt 顶部 `⚠ 同步已暂停：本地磁盘剩余 <100MB` | `MOUNT_DISK_FULL_004` |
| 冲突文件堆积超过 10 个 | `⚠ 有 12 个文件处于冲突状态，运行 cloud-claude sync conflicts 查看` | `MOUNT_CONFLICT_005`（非阻断） |

### 可直接转写的 REQ-ID 候选

- **REQ-F1-A：** 用户执行 `cloud-claude` 后，容器内工作目录 `/workspace` 必须同时承载源码（热同步）和其它文件（懒拉），用户无需感知分层。
- **REQ-F1-B：** 首次连接建立到 prompt 可输入必须 ≤ 8s（含首轮 Mutagen 同步完成），用户能看到三段式进度。
- **REQ-F1-C：** 在 10k 文件源码树执行 `rg .` / `ls -R` 的延迟必须 ≤ 本地 1.5 倍。

---

## F2 · 降级路径与 `--mount-mode` 手动切换

### Table stakes

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| 降级**必须对用户可见**，不能静默 | VS Code Remote 在 SSH fallback 时会在底部状态栏显示 | 降级时红色/黄色提示，不允许 cloud-claude 悄悄切到 sshfs-only |
| 提供手动模式开关 | Dev Containers 有 `workspaceMount` 显式配置 | `--mount-mode=auto\|full\|mutagen-only\|sshfs-only` |
| 降级决策可复现 | Mutagen `mutagen sync create -m=...` 显式模式 | `--mount-mode` 的结果记录在 `~/.cloud-claude/last-session.log` |

### Differentiators

| 行为 | 价值 | 复杂度 |
|------|------|--------|
| 启动时自动探测"最优模式"（如本地在 macOS 且磁盘 >10GB → full；内网机器 → mutagen-only） | 减少用户手动调 | MEDIUM |
| 降级后在下次启动自动回升（恢复尝试） | 避免用户永远卡在降级态 | LOW |
| 在 prompt 上方显示当前 mode（`[hybrid]` / `[sshfs-only]`） | 对标 Mosh `[mosh]` 窗口标题前缀 | LOW |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 自动"升级"：检测到网络变好自动从 sshfs-only 升级到 hybrid | 中途切换挂载点会让进行中的 Claude 进程看不到文件（FUSE 挂载路径变化），用户正在写的代码可能看不到变更；应只在下次会话升级 |
| 支持 `--mount-mode=none`（完全不挂载本地） | 这等于回到 v1.0 Web SSH 体验，破坏 cloud-claude 的核心承诺；用户真要这个需求应该用 `ssh` 原生命令 |

### 用户感知的失败模式

| 场景 | 用户看到 |
|------|----------|
| auto 模式三层都失败 | `[✗] 文件映射失败：mutagen/sshfs/mergerfs 均不可用。退出码 9，建议 cloud-claude doctor` |
| mutagen-only 模式下用户尝试访问未同步文件 | 容器内 `ls /workspace/node_modules/` 返回空，用户需要主动 `cloud-claude sync extend node_modules/` |
| sshfs-only 模式下 `rg` 慢 | 顶部提示 `[!] 当前 sshfs-only 模式，大目录搜索较慢（>5s），建议切回 hybrid` |

### REQ-ID 候选

- **REQ-F2-A：** CLI 支持 `--mount-mode=auto\|full\|mutagen-only\|sshfs-only` 参数，默认 `auto`。
- **REQ-F2-B：** 三层挂载任一失败时必须在 2 秒内降级到下一档并在终端显示具体原因。
- **REQ-F2-C：** 当前 mount mode 必须在每次连接时以彩色标签形式显示（颜色可被 `NO_COLOR` 关闭）。

---

## F3 · SSH 弱网容忍 + 自动重连提示

### Table stakes

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| keepalive 配置合理 | Microsoft Q&A 推荐 `ClientAliveInterval 60 / ClientAliveCountMax 5` | 使用 `ServerAliveInterval=15`、`ServerAliveCountMax=8`（即 120s 才 TCP reset） |
| 客户端有独立的重连预算 | VS Code `maxReconnectionAttempts: 8` | cloud-claude 默认重连 5 次，指数退避（1s → 2s → 4s → 8s → 16s） |
| 断网期间不要吃键盘 | Mosh 本地 echo 模式 | 断网时输入**暂存在本地**并显示为暗灰色（未确认），重连后统一提交；和 Mosh 的 underline 语义一致 |
| 重连后无需重新认证 | Eternal Terminal 透明重连 | 缓存 Entry API token 到进程内存，断网 <5min 内直接复用 |

### Differentiators

| 行为 | 价值 |
|------|------|
| 本地 echo 只在 RTT > 150ms 时启用（`--predict=adaptive` 等价） | 对标 Mosh 默认行为，低延迟用户无感知 |
| 在提示栏显示实时 RTT（`~45ms ↔ cloud-claude-edge-01`） | 让用户知道"抖动是真实发生的" |
| 支持 IP roaming（手机 4G → WiFi 不断连） | Mosh 核心卖点，需要底层走 UDP，v3.0 可以先不做，v3.1 评估 |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 用 UDP 代替 TCP（自己做 Mosh） | 重造轮子；Mosh 协议要求 mosh-server 在容器内，会污染镜像；收益相比"tmux + SSH 自动重连"并不明显；宿主机 nftables 还要额外开 UDP 60000-61000 端口 |
| 断网自动杀掉运行中的 claude | VS Code 这点做得很差（[Issue #274774](https://github.com/microsoft/vscode/issues/274774) 大量吐槽 "Reconnect → Reload → 丢失未保存"），我们必须避开 |
| 弱网下默默降低帧率 / 屏幕刷新 | Mosh 的做法，但它是 UDP；我们基于 SSH，做不到且强做会让 claude 的流式输出错位 |

### 用户感知的失败模式

| 场景 | 用户看到 |
|------|----------|
| 网络抖动 <30s | 顶部黄色 `⏳ 网络抖动中 (12s)...`；输入被暂存（灰色未确认文字） |
| 断网 30s - 5min | 红色 `● 网络已断开 2m30s，自动重试中 (4/5)`；光标变灰 |
| 重连失败（第 5 次仍失败） | `[✗] 重连失败，按 Enter 重试；容器内会话已保留（F4 会话恢复），运行中的 claude 进程没有丢失` |
| 网关主动重启（SSH banner 变了） | `[!] 检测到网关重启，已自动重新建立连接` |

### REQ-ID 候选

- **REQ-F3-A：** SSH 客户端必须配置 ServerAliveInterval=15s，断网 <30s 不触发 TCP reset。
- **REQ-F3-B：** 断网期间用户键入的字符必须在本地缓冲并以"未确认"样式显示，重连后按序提交。
- **REQ-F3-C：** 重连失败时 prompt 必须显示具体的失败原因和下一步操作（按 Enter 重试 / 运行 doctor）。

---

## F4 · 会话恢复（tmux/dtach 默认包装）

### Table stakes

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| 断网后运行中的进程不死 | Eternal Terminal、Mosh、tmate、Codespaces 默认行为 | SSH session 启动时自动 `tmux new-session -A -s claude-<user>` |
| 重连后回到同一屏幕 | ET 的 BackedReader/Writer 序列号机制 | tmux 自动 reattach，屏幕内容从 tmux 的 history buffer 恢复 |
| 支持用户主动 detach | tmux `Ctrl-b d`、Mosh `Ctrl-^ .` | `Ctrl-b d` 后 cloud-claude 自动退出本地终端，但容器内 claude 继续跑 |

### Differentiators

| 行为 | 价值 |
|------|------|
| 提供 `cloud-claude sessions ls` 查看当前用户名下所有活着的会话 | 类似 `tmux ls` 但带中文标签和最后活跃时间 |
| 支持 `cloud-claude attach <session-id>` 接入指定会话 | 多任务并行场景 |
| session 默认命名规则 `claude-<timestamp>-<cwd-basename>` | 让用户一眼看出"这是哪个项目的会话" |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 容器内同时跑多个 tmux server | 会和用户自己启动的 tmux 冲突；单 server 单 session 最简单 |
| 默认把 vim/less 的屏幕内容重放 | tmux 自带 history，无需额外实现；自己做的版本容易和 claude 的流式 TUI 冲突 |
| 会话永不过期 | 空闲容器会被 v3.1 的回收策略清理；会话**应该**随容器生命周期消亡，不能让用户误以为 "我的 session 是永久的"；默认 session 在容器 stop 时一并消失，明确告知 |

### 用户感知的失败模式

| 场景 | 用户看到 |
|------|----------|
| tmux 不在 PATH 或镜像残缺 | 启动时提示 `[!] 容器内 tmux 不可用，会话恢复已禁用`，但不阻塞 |
| 重连时 tmux socket 丢失 | `[!] 无法恢复上次会话（socket 已失效），将启动新会话`，给出 `cloud-claude sessions ls` 提示 |
| 用户主动 `Ctrl-b d` detach | 本地终端退出，提示 `✓ 会话已保留（claude-20260418-proj），下次 cloud-claude 会自动 attach` |

### REQ-ID 候选

- **REQ-F4-A：** 容器内 SSH 会话默认用 tmux 包装，断网后重连必须恢复到同一会话。
- **REQ-F4-B：** 用户可通过 `cloud-claude sessions ls/attach` 管理多个并行会话。
- **REQ-F4-C：** tmux 不可用时 cloud-claude 不得阻塞启动，但必须明确告知用户"会话恢复功能已禁用"。

---

## F5 · 多端同账号 attach 同 session

### Table stakes

> 本节的核心 UX 决策见 **M1**，这里只补充具体实现动作。

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| 新端连上时必须检测既有 client | tmux `session_many_attached` 格式化字段 | `tmux list-clients -t claude-<user>` |
| 给用户明确的"已有人在"提示 | tmate `show-messages` 输出会列出所有 client URL | cloud-claude 打印一行 banner 说明"共 N 个会话 attach 中" |
| 支持"接管"操作 | tmux `attach -d` / `detach-client -a` | `--take-over` 或交互式 `y/N` 询问 |

### Differentiators

| 行为 | 价值 |
|------|------|
| 区分客户端来源（通过本地 hostname 打 tag） | 用户看到 `来源：macbook-home / linux-work` 而非 UUID |
| `cloud-claude sessions who` 列出当前连接者 + 最后活跃时间 | 类似 tmate 的 `show-messages`，但中文友好 |
| `--new-session` 独立会话默认命名为 `claude-<cwd-basename>-<n>` | 避免 n 个并行 session 命名冲突 |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 默认独占（新连接踢掉旧的） | 违反 M1 推荐；用户日常场景 99% 是"我想两边都看"，强独占会让"忘了断开 mac → linux 上无法继续"变成必须手动处理的故障 |
| 实时显示对方光标/选中区（像 VS Code Live Share） | 这是 IDE 级协作功能，tmux 做不到；v1 范围内不切实际；v3.0 的定位是"同账号 attach"，不是"协作编辑" |
| 阻止多端写入（强制只读） | tmate 的 `-r` 模式适合演示场景；我们的用户是"同一个人多个设备"，强制只读反而不方便 |
| 多端冲突时弹出 modal 对话框 | CLI 没有 modal；tmux 原生也不弹框，只在 status-line 显示；我们应该在 prompt 上方打一条中文提示，而不是打断输入 |

### 用户感知的失败模式

| 场景 | 用户看到 |
|------|----------|
| 另一端已 attach，当前端默认 attach 成功 | `✓ 已 attach 到会话 claude-proj（另 1 个会话正在共享：mac-home）` |
| 用户用 `--new-session` 但容器已到 session 上限（默认 5） | `[✗] 会话数已达上限 (5/5)，请先关闭一个或使用 cloud-claude sessions kill <id>` |
| 用户用 `--take-over` | 其他端收到提示：`⚠ 本会话已被另一端接管（来自 linux-work），本地已退出` + 退出码 130 |

### REQ-ID 候选

- **REQ-F5-A：** 默认行为是多端共享 attach 同一 session，不踢人不报错。
- **REQ-F5-B：** 第二端 attach 成功后必须在 banner 显示其它 client 的来源和活跃时间（中文）。
- **REQ-F5-C：** `--new-session` 创建独立 session，`--take-over` 强制独占并通知其它端。

---

## F6 · `cloud-claude doctor` 全面升级

### Table stakes（严格遵循 M4 范式）

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| `[✓]` `[!]` `[✗]` 符号 + 颜色 | flutter doctor、brew doctor | 全部五维度（network/auth/ssh/mount/disk）用相同符号 |
| 每项失败带"如何修复" | flutter doctor 的 "To resolve this, run: ..." | 每条失败附中文"下一步该做什么" |
| `--verbose` 展开探测细节 | flutter doctor -v | 展示 SSH banner、Mutagen session ID、mergerfs 挂载点等 |
| 支持 `NO_COLOR` | 业界共识（hexview / ripgrep / docker） | 管道到 file 自动关色 |

### Differentiators

| 行为 | 价值 |
|------|------|
| `--fix` 一键修复（能修的修） | 对标 `flutter doctor --android-licenses`；先从"重启 mutagen agent"、"卸载残留 sshfs"这类幂等操作做起 |
| `--json` 输出（供脚本消费） | 对标 npm-audit-report 的思路；方便 CI 侧接入 |
| `doctor network` 等子命令只跑单维度 | 对标 `npm doctor connection` / `brew doctor --list-checks` |
| 和 F8 错误码体系联动：每项失败输出 `错误码 MOUNT_MUTAGEN_001` | 让用户可以复制错误码到搜索或 issue | 

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 自动上报"诊断报告"到服务端 | 涉及数据脱敏/合规，v3.0 不值得开坑；让用户自己复制粘贴输出即可 |
| 实时监控模式（`doctor --watch`） | 会变成"迷你 top"，和 `cloud-claude status` 功能重叠；doctor 定位是"一次性诊断"，保持 KISS |
| 自动修改用户本地 SSH config | 不可逆；参考 VS Code Remote 的踩坑（[Issue #8910](https://github.com/microsoft/vscode-remote-release/issues/8910) 大量用户抱怨默认改动无法关闭）；给出建议但不执行 |
| 给每条检查打 "完美/良好/一般" 星级 | flutter doctor 就是三档，多档会让用户 decision fatigue |

### 用户感知的失败模式

```
$ cloud-claude doctor
Doctor summary (运行 cloud-claude doctor -v 查看详情):
[✓] network   — 网关可达，延迟 45ms
[✓] auth      — token 有效（剩余 23 天）
[!] ssh       — 主机密钥变更
                建议：运行 cloud-claude doctor --fix 清理 known_hosts
[✗] mount     — mutagen agent 无响应
                错误码：MOUNT_MUTAGEN_001
                建议：运行 cloud-claude doctor --fix（会自动重启 agent）
[✓] disk      — 本地剩余 45GB，容器剩余 20GB

发现 1 个错误和 1 个警告。运行 --fix 自动修复可修复项。
```

退出码：0 全通过 / 1 只有警告 / 2 有失败（对标 brew doctor）。

### REQ-ID 候选

- **REQ-F6-A：** `cloud-claude doctor` 必须覆盖 network/auth/ssh/mount/disk 五维度。
- **REQ-F6-B：** 每项检查输出必须包含符号、简短原因、中文修复建议、错误码。
- **REQ-F6-C：** `doctor --fix` 能自动修复至少 5 种常见失败（mutagen agent 无响应、FUSE 残留挂载、known_hosts 冲突、token 过期需刷新、DNS 缓存污染）。

---

## F7 · Claude Code 登录态持久化

### Table stakes（官方契约）

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| `~/.claude/.credentials.json` 必须在容器重建时保留 | Anthropic 官方确认：credentials.json 是 OAuth token 的落盘位置（[Issue #1736](https://github.com/anthropics/claude-code/issues/1736)） | 独立 Docker named volume `claude-creds-<account_id>`，mount 到 `/root/.claude` |
| 同时持久化 `~/.claude/` **整个目录**（含 `.claude.json`、settings.json） | 同上 issue：用户反馈只 mount `.credentials.json` 不够，必须整目录 | volume mount 到 `/root/.claude` 而不是单文件 bind |
| 设置 `CLAUDE_CONFIG_DIR` 环境变量 | 官方推荐的 devcontainer 范式 | 容器 entry 设置 `CLAUDE_CONFIG_DIR=/root/.claude` |
| 不同 claude_account 之间完全隔离 | 避免"小王的 token 被小李看到" | volume 命名 = `claude-creds-<claude_account_uuid>`，和业务层 claude_accounts 表一对一 |

### Differentiators

| 行为 | 价值 |
|------|------|
| `~/.cache/claude` 也独立 volume，加速重连后的 embedding / context 复用 | 减少首次对话延迟 |
| 每次容器启动检测 credentials 是否过期（`expiresAt` 字段） | 如过期，提前提示用户在本地 `cloud-claude claude login`（refresh token） |
| 当前账号在 banner 显示 | `Claude 账号：user@example.com（订阅到期 2026-06-15）` |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 用 `ANTHROPIC_API_KEY` 环境变量替代 OAuth | 按官方文档，API key 和 Claude Pro OAuth 是**互斥**两套，混用会让 Pro 用户被强制降级到按量付费（[Issue #5767](https://github.com/anthropics/claude-code/issues/5767) 明确说明）；我们的目标用户是订阅制 |
| 把 credentials 存到 PostgreSQL 统一管理 | 增加"谁能读到 token"的合规面；文件级 Docker volume 已经足够，访问控制走 volume 权限 + 容器绑定 |
| 跨用户共享 credentials | Anthropic TOS 禁止；且一旦撤销需要手动清理所有关联 |
| 容器内提供 `claude login` 交互 OAuth | 容器出网受 sing-box 全隧道，OAuth 回调 URL 无法稳定工作；应该在**本地**用浏览器完成 OAuth，把 `.credentials.json` 推上去 |

### 用户感知的失败模式

| 场景 | 用户看到 |
|------|----------|
| Volume 挂载失败 | `[✗] Claude 账号状态加载失败（volume claude-creds-<id> 不存在）`，退出码 10 |
| Token 已过期 | `[!] Claude 登录已过期（2026-04-10 失效），需要在本地重新登录` + 引导命令 |
| 容器重建后首次进入 | 无 prompt 变化，claude 直接可用（这本身就是验收标准） |

### REQ-ID 候选

- **REQ-F7-A：** `~/.claude/` 必须通过独立 Docker named volume 持久化，volume 命名粒度为单个 claude_account。
- **REQ-F7-B：** 容器重建后，未过期的 OAuth credentials 必须保留，用户无需重新登录。
- **REQ-F7-C：** credentials 过期时 cloud-claude 必须在连接建立前给出明确中文提示，不能让 claude 进程进入报错后才发现。

---

## F8 · 错误码与中文提示统一升级

### Table stakes

| 行为 | 业界来源 | 我们的具体化 |
|------|----------|--------------|
| 每条错误有稳定的 code | AWS、Kubernetes、Docker 都是稳定 code 制 | `CLOUDCLAUDE_<DOMAIN>_<NUM>` 形如 `MOUNT_MUTAGEN_001` |
| 每条错误必须给"下一步怎么做" | flutter doctor 模式 | 中文一句话建议 + 可选命令 |
| 非零退出码语义清晰 | sysexits.h 或 brew 的 0/1/2 | 继承 v2.0 的 7 种错误码 + v3.0 新增文件映射/会话相关 |
| 错误码可搜索（文档页 / --help） | `rustc --explain E0308` 范式 | `cloud-claude explain MOUNT_MUTAGEN_001` 输出详细说明 |

### Differentiators

| 行为 | 价值 |
|------|------|
| 错误码和 doctor 对齐：doctor 输出的码能直接 `cloud-claude explain` | 形成闭环 |
| errors 文档在线可查（后期生成静态文档站） | 友好新人 |
| 错误上下文包含相关日志的最后 N 行（放在 `--verbose` 下） | 对标 npm audit 的详尽模式 |

### Anti-features

| 不做 | 为什么 |
|------|--------|
| 错误码全局纯数字（如 `E1001`） | 用户看到 `E1012` 完全记不住是什么；AWS 多 domain 前缀已经证明可读性远胜纯数字 |
| 把 stacktrace 直接打给用户 | Go panic 的 stacktrace 对非开发者用户噪音极大；默认隐藏，`--verbose` 或 `CLOUDCLAUDE_DEBUG=1` 才显示 |
| 用 emoji 替代中文提示（🚫 / ✅） | 终端 emoji 宽度问题多（Unicode double-width），且 CI 日志容易乱码；CLI 规范（flutter / brew）都用 ASCII 符号 + 颜色 |
| 每种错误都抛出新的退出码 | sysexits.h 建议 ≤16 种；细粒度放在错误码字符串里，退出码保持 0/1/2/3/...≤15 |

### 用户感知的失败模式

```
[✗] 启动失败：无法挂载文件映射
    错误码：MOUNT_MERGE_003
    原因：mergerfs 不可执行（容器镜像版本不匹配）
    建议：运行 cloud-claude explain MOUNT_MERGE_003 查看详细解释
          或运行 cloud-claude doctor 检查环境
退出码：9
```

### REQ-ID 候选

- **REQ-F8-A：** v3.0 所有新错误路径必须纳入统一错误码体系，code 格式 `<DOMAIN>_<KIND>_<NUM>`。
- **REQ-F8-B：** 每条错误输出必须包含 code、中文原因、中文下一步建议，三项缺一不可。
- **REQ-F8-C：** `cloud-claude explain <code>` 子命令必须对每个 code 给出详细中文说明和常见修复步骤。

---

## 汇总：Anti-Feature 清单（方便 REQUIREMENTS.md 的 Out of Scope 章节）

| # | 不做的功能 | 影响的 feature | 参考理由（一句话） |
|---|-----------|----------------|---------------------|
| A1 | Web UI 显示同步进度 | F1 | CLI 工具的范围偏离，网关架构放大认证复杂度 |
| A2 | 暴露 Mutagen 的 5 种冲突解决模式给用户 | F1 | 99% 场景下 two-way-safe 够用，开放反而造成误配置 |
| A3 | mergerfs 升级成 bcachefs/overlayfs | F1 | overlayfs CoW 语义和 sshfs 写入冲突；mergerfs 是现阶段唯一稳定方案 |
| A4 | `--mount-mode=none` | F2 | 破坏 cloud-claude 核心承诺，用户应改用原生 ssh |
| A5 | 运行中自动升级 mount mode | F2 | 动态改挂载点会让 Claude 进程"看不到文件" |
| A6 | 用 UDP/Mosh 协议替代 SSH | F3 | 需改镜像、开新端口，收益和 tmux+SSH 自动重连相当 |
| A7 | 断网自动杀掉 claude 进程 | F3/F4 | VS Code Remote 的踩坑，用户会丢失未保存工作 |
| A8 | 会话永不过期 | F4 | 违背容器生命周期契约，会给 v3.1 资源回收埋雷 |
| A9 | 默认独占（新端踢旧端） | F5 | 违反用户多设备直觉，常见场景是"两个屏都想看" |
| A10 | 实时协作光标（Live Share 风格） | F5 | 超出 CLI 能力范围，tmux 做不到 |
| A11 | doctor 自动上报远程 | F6 | 合规风险，v3.0 不值得开坑 |
| A12 | doctor 自动改 SSH config | F6 | 不可逆，VS Code Remote 这块口碑极差 |
| A13 | 用 `ANTHROPIC_API_KEY` 替代 OAuth | F7 | 官方明确 API key 和 Pro OAuth 互斥 |
| A14 | 容器内做 `claude login` 交互 OAuth | F7 | 全隧道出网会打断 OAuth 回调 |
| A15 | 错误用 emoji 提示 | F8 | 终端宽度 / CI 日志兼容性差 |

---

## Feature Dependencies

```
F7 (Claude 持久化) ──独立──> （可提前做，不依赖其它）

F1 (三层 mount) ──enables──> F2 (模式切换/降级)
                 └──enables──> F6 (doctor mount 维度)

F3 (SSH 弱网) ──depends on──> F4 (tmux 会话) ← 否则重连回来进程丢了没意义
                              │
F4 (tmux 会话) ──enables──> F5 (多端 attach)

F8 (错误码) ──enhances──> F1,F2,F3,F4,F5,F6,F7  (横切关注点)
```

### 依赖说明

- **F4 必须先于 F3 交付：** 没有 tmux 包装，SSH 重连回来 claude 进程已死，F3 就只是"能重连但看不到东西"。
- **F1 和 F2 天然绑定：** 降级路径是三层架构的前提条件，单独做 F1 而不做 F2 会在首次降级失败时体验崩盘。
- **F7 独立：** 可以放在 v3.0 第一阶段，先把"用户不再天天重新登录 Claude"这个痛点解决，独立于挂载架构。
- **F8 贯穿：** 应作为所有 feature 并行推进的**代码规范**而非独立 phase，新写的错误路径直接使用新码。

---

## 核心对标产品矩阵（直接引用）

| Feature | Mosh | Eternal Terminal | tmate/tmux | VS Code Remote | Codespaces | Dev Containers | 我们的定位 |
|---------|------|------------------|------------|----------------|------------|----------------|-----------|
| 断网保活 | ✓（UDP） | ✓（backed TCP） | ✓（session） | △（8次重试） | ✓ | ✓ | tmux + SSH 自动重连（F3+F4） |
| 本地 echo | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✓（F3） |
| 文件同步 | ✗ | ✗ | ✗ | bind mount | bind | bind + named vol | **三层架构**（F1，差异化） |
| 会话恢复 | N/A | ✓ | ✓ | △ | ✓ | ✓ | ✓（F4） |
| 多端 attach | ✗（独占） | ✗ | ✓ | ✗ | ✗ | ✗ | ✓（F5，默认共享） |
| 持久化 home | ✗ | ✗ | N/A | △ | ✓ | ✓（named vol） | ✓（F7） |
| Doctor 工具 | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✓（F6，差异化） |

**我们的差异化定位**：把 Codespaces 级的"开箱即用 + 持久化 home"带到**自建单宿主机 + 强约束出口 IP** 场景；同时把 CLI 工具链的 doctor 体验（flutter 级）纳入核心交付。

---

## 来源（Sources）

### 官方文档
- [VS Code Remote SSH — ReconnectionGraceTime PR #273060](https://github.com/microsoft/vscode/issues/280450) — 2025.11 引入 3h grace time（HIGH）
- [Mosh 官方 README](https://github.com/mobile-shell/mosh/) — 核心 UX：roaming、predictive echo、contact lost 警告（HIGH）
- [Mosh manpage](https://manpages.debian.org/testing/mosh/mosh.1.en.html) — `--predict=adaptive` 默认行为（HIGH）
- [Eternal Terminal — How It Works](https://eternalterminal.dev/howitworks) — BackedReader/Writer 序列号机制（HIGH）
- [Eternal Terminal `-x` 参数](https://github.com/MisterTea/EternalTerminal/issues/225) — 官方推荐用 `-x` 杀旧 session（HIGH）
- [tmate manpage (Ubuntu Jammy)](https://manpages.ubuntu.com/manpages/jammy/man1/tmate.1.html) — attach/detach 语义、只读模式（HIGH）
- [tmux multi-client detach](https://stackoverflow.com/questions/22138211/how-do-i-disconnect-all-other-users-in-tmux) — `tmux attach -d`、`detach-client -a`（HIGH）
- [Mutagen File Synchronization](https://mutagen.io/documentation/synchronization) — 四种 sync mode、冲突解决（HIGH）
- [Mutagen Issue #277 — 状态可见性](https://github.com/mutagen-io/mutagen/issues/277) — JSON 输出和监控的官方迭代（HIGH）
- [mergerfs Performance docs](https://github.com/trapexit/mergerfs/blob/d8918458/mkdocs/docs/performance.md) — 调优配置（HIGH）
- [mergerfs Remote Filesystems](https://trapexit.github.io/mergerfs/latest/remote_filesystems/) — sshfs 作为 branch 的建议（HIGH）
- [VS Code Dev Containers — Improve disk performance](https://code.visualstudio.com/remote/advancedcontainers/improve-performance) — named volume 替代 bind mount 范式（HIGH）
- [GitHub Codespaces — Personalizing](https://docs.github.com/en/codespaces/customizing-your-codespace/personalizing-github-codespaces-for-your-account) — dotfiles + Settings Sync 机制（HIGH）
- [JetBrains Gateway Docs](https://www.jetbrains.com/help/idea/remote-development-a.html) — Running 指示器 UX（HIGH）
- [JetBrains 2025.2 Remote Highlights](https://blog.jetbrains.com/platform/2025/07/bringing-remote-closer-to-local-2025-2-highlights/) — IDE 设置跨重启保留（HIGH）
- [Flutter Doctor 文档](https://flutterfever.com/flutter-doctor-command/) — `[✓]` `[!]` `[✗]` 符号范式（HIGH）
- [Homebrew doctor.rb 源码](https://github.com/Homebrew/brew/blob/cbc2b248/Library/Homebrew/cmd/doctor.rb) — 颜色 + `opoo` 警告格式（HIGH）
- [npm-doctor 官方文档](https://docs.npmjs.com/cli/v11/commands/npm-doctor) — 五维度检查范式（HIGH）

### 讨论 / 踩坑实录
- [Anthropic Claude Code Issue #1736](https://github.com/anthropics/claude-code/issues/1736) — `~/.claude` 持久化社区共识（HIGH）
- [Claude Code Issue #22066](https://github.com/anthropics/claude-code/issues/22066) — OAuth token 在 Docker 内不持久的 bug 模式（MEDIUM）
- [Moshi 的 Mosh 故障排查指南](https://getmoshi.app/articles/fix-mosh-connection-failed) — conntrack UDP 超时 30s、NAT 重写端口（MEDIUM）
- [Mosh Issue #925 — heartbeat 间隔](https://github.com/mobile-shell/mosh/issues/925) — 3s 心跳、server 端断开策略（HIGH）
- [VS Code Issue #274774 — 休眠后无法重连](https://github.com/microsoft/vscode/issues/274774) — Reconnect 体验缺陷的真实案例（MEDIUM）
- [Microsoft Q&A — SSH keepalive 配置](https://learn.microsoft.com/en-ca/answers/questions/5651168/) — 业界推荐值 `ClientAliveInterval 60`（MEDIUM）

### 技术背景
- [Docker Mount Performance 对比](https://eastondev.com/blog/en/posts/dev/20251217-docker-mount-comparison/) — Mac 上 bind mount 比 volume 慢 3.5×，支撑 F1 的 Mutagen 热同步决策（MEDIUM）

---

## 置信度评估

| 维度 | Confidence | 依据 |
|------|-----------|------|
| 多端 attach UX 模式 | HIGH | tmux、tmate、ET 三个源码级一致 |
| 弱网阈值数值 | MEDIUM | Mosh 官方未给明确秒数，30s/conntrack 是基础设施层共识 |
| Mutagen 冲突默认策略 | HIGH | 官方文档明确 two-way-safe 是默认 |
| doctor 输出范式 | HIGH | flutter/brew/npm 三家范式高度一致 |
| Claude Code credentials 契约 | HIGH | Anthropic 官方 issue + devcontainer 生态验证 |
| mergerfs 替代方案判断 | MEDIUM | 基于 mergerfs 官方文档，无直接"对比 bcachefs"数据 |
| Anti-feature 判断 | HIGH | 每条都有具体竞品踩坑或协议约束支撑 |

---

## Roadmap 推荐的 Phase 切分方向

基于依赖关系，建议规划时按以下顺序切分（不占用 roadmap 决策权，只给信号）：

1. **F8 错误码基础设施**（横切，第一阶段）
2. **F7 Claude 登录持久化**（独立，用户价值最直接）
3. **F4 tmux 会话包装** → **F3 SSH 弱网容忍** → **F5 多端 attach**（依赖链）
4. **F1 三层 mount** → **F2 模式切换**（耦合推进）
5. **F6 doctor 升级**（最后，需要前面所有 feature 的检查维度都落地）

---

*Researched: 2026-04-18*
*Domain: 容器化远程开发 CLI / 云端 SSH 开发环境*
*Consumers: gsd-research-synthesizer、后续 REQUIREMENTS.md、phase planning*
