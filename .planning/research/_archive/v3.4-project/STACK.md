# Stack Research — v3.0 远端开发体验升级

**Domain:** 容器化远程开发体验（CLI 透明代理 + 容器内 SSH 会话 + 三层文件系统）
**Researched:** 2026-04-18
**Confidence:** HIGH

> 本文档面向 v3.0 milestone 的 8 项 feature（F1–F8），只列**必须新增 / 升级**的组件。v2.0 已有的 Go 1.26.1、PostgreSQL 18.x、Docker Engine 28.x、sing-box（含 tun + nftables 默认拒绝）、OpenSSH 10.2p1、React 19/Vite 8、`pkg/sftp`、cobra、shellescape 全部沿用，本文不重复列出。

---

## 关键结论速查

| 决策项 | 唯一推荐 | 版本 | 集成形式 |
|--------|----------|------|----------|
| 热同步引擎 | **Mutagen** | v0.18.1 | CLI 端嵌入二进制 + 容器预放 agent tarball |
| 冷兜底网络 FS | **sshfs**（升级） | 3.7.5 | 容器内已装，仅升级 |
| 联合视图 | **mergerfs** | 2.41.1 | 容器内预装 + entrypoint 显式锁定策略 |
| 会话恢复 | **tmux** | 3.6a | 容器内预装 + entrypoint 默认包一层 |
| 弱网容忍 | **OpenSSH `ServerAliveInterval` + cloud-claude 自实现重连** | 复用现有 OpenSSH 10.2p1 | CLI 端实现，不新引入 mosh / autossh / ET |
| 状态持久化 | **Docker named volume** | Docker Engine 28.x（已有） | 控制面在 worker 创建容器时附加 volume |
| FUSE 库 | **libfuse3** | 3.18.x（仅跟随 Debian/Ubuntu 镜像基线） | 容器内已装，确认 ABI |

**不引入**：mosh、autossh、Eternal Terminal、zellij、dtach、abduco、unison、syncthing、rsync watcher。原因见下文每节"为什么不用替代方案"。

---

## 1. 热同步层：Mutagen（F1 / F2）

### 推荐与版本

| 项目 | 取值 | 来源 |
|------|------|------|
| 客户端 / 守护进程 | Mutagen v0.18.1 | https://github.com/mutagen-io/mutagen/releases/tag/v0.18.0（v0.18.1 元数据见 GitHub 仓库 latest release: 2025-02-24） |
| 构建语言 | Go 1.22+（与 v2.0 Go 1.26.1 工具链一致） | https://pkg.go.dev/github.com/mutagen-io/mutagen |
| 协议 | 双向 rsync 衍生算法 + 文件系统 watcher（macFSEvents / inotify） | https://mutagen.io/documentation/synchronization/ |
| 传输方式（v3.0 采用） | OpenSSH 透传（`ssh://` URL，复用 cloud-claude 现有 SSH 通道与认证流） | https://mutagen.io/documentation/transports/ssh/ |

**当前稳定版**：v0.18.1（2025-02-24）。GitHub 仓库 `latest_release` 字段确认；v0.18.0 是该次 minor 的初始版本（2024-10-24），引入 `--ignore-syntax=docker` 等特性，v0.18.1 为依赖更新与 bug 修复。Mutagen 官方仍在 0.x 阶段，Mutagen 自己声明的兼容承诺是"每个 minor 系列至少在下一 minor 发布后再支持一个月"。

### 为什么是 Mutagen

1. **真正的双向 + 实时 watcher**：Mutagen 是少数同时具备低延迟 watcher、双向冲突合并和 SSH 传输的工具。本项目场景"用户本地编辑 → 容器里 `claude` 实时看到"是 Mutagen 的标准用例。
2. **OpenSSH 透传**：传输层直接调用本机 `ssh`/`scp`，不嵌入第三方 SSH 库，自动复用 cloud-claude 已经握手好的 SSH 配置和密钥。这条对项目意义重大：v2.0 的 SSH proxy + Entry API 认证流不需要任何改造。
3. **Docker URL 不强制需要**：Mutagen 也支持 `docker://container/path` 形式（自动用 `docker cp` 拷 agent + `docker exec` 运行 agent，见 `pkg/agent/transport/docker/transport.go`）。**v3.0 不采用** Docker transport，因为它需要客户端有 Docker daemon 访问权（用户本地不一定有也不应有）。坚持 SSH transport。
4. **Agent 自动安装**：客户端首次连接远端时，Mutagen 用 `scp` 把和客户端版本严格匹配的 agent 二进制拷到 `~/.mutagen/agents/<version>/` 并执行。失败则中止 session（不会"半同步"）。
5. **`.dockerignore` 风格忽略语法**（v0.18 引入）：可以直接复用项目里已有的 ignore 文件做白名单/黑名单过滤，恰好命中 F1 的"≤50MB、按扩展名 / 路径过滤"要求。

### 为什么不用替代方案

| 替代 | 不选的事实依据 |
|------|----------------|
| **Unison** | OCaml 实现，无主动 watcher（必须主动触发或定时轮询），双向冲突解决体验远差于 Mutagen，社区维护强度低。 |
| **Syncthing** | 设计目标是 P2P 多端共享，启动开销和 metadata 占用大，最小同步周期是数秒级，无法满足"≤8s 首轮同步"的体验目标。需要在两端跑常驻 daemon + 端口暴露，与单 SSH 通道模型冲突。 |
| **rsync + inotifywait** | 没有冲突合并；事件→rsync 之间会丢事件（inotify queue overflow 是著名问题）；首次大目录 rsync 无并行；上手心智负担反而高。 |
| **lsyncd** | 单向同步定位，且 5 年没有有意义的 release。 |
| **devpod 风格 SCP 一次性同步** | 不是实时方案，与"本地编辑实时反馈"目标背道而驰。 |

### 与 v2.0 的兼容性

- **SSH 通道**：复用 SSH proxy 同一条连接的同一会话基础设施。Mutagen agent 直接在容器用户 home 跑，不需要额外端口、不需要修改 sshd_config、不破坏 v2.0 的 channel multiplex 行为。
- **AppArmor unconfined + SYS_ADMIN + /dev/fuse**：Mutagen agent 是纯用户态 Go 进程，不需要任何额外特权。当前 AppArmor / capability 配置足够。
- **sing-box tun + nftables 默认拒绝**：Mutagen 流量走 SSH 隧道，从容器视角进出的都是 22 端口的 TCP，已经在 v2.0 的 nftables 白名单里。**不需要新加任何防火墙例外**。
- **SFTP server 共存**：v2.0 在客户端跑 `pkg/sftp` 的嵌入 SFTP server 给 sshfs slave 用。Mutagen 使用的是独立的 agent → agent 协议，不复用 SFTP，因此两条通道互不干扰，可以同时运行。

### 集成形式

1. **客户端**：cloud-claude 二进制内嵌一份 `mutagen` 客户端（要么 `go install` 然后 `go:embed`，要么用 `os/exec` 调用旁路安装的 mutagen——**推荐 embed 静态调用**，避免用户机器有无 mutagen 的差异）。
2. **容器内**：受管镜像 build 阶段下载 `mutagen-agents.tar.gz`（Mutagen 官方为每次 release 同时发 agent bundle）解压到 `/opt/mutagen/agents/<version>/`，并在用户 home 软链接 `~/.mutagen/agents/<version>/` 指向它。**目的**：避免每次启动容器时 Mutagen 再 `scp` 一遍 agent（agent 包 ~80MB），冷启动时间能省 2–4s。如果版本不匹配自动 fallback 到 Mutagen 默认的 scp 安装路径。
3. **mutagen daemon**：客户端侧 Mutagen 需要本地 daemon。cloud-claude 启动时检测并 `mutagen daemon start`，退出时不停（让长会话复用）。
4. **session 命名**：以 `claude-account-{id}` 命名，方便 doctor 检查。

### 配置（建议默认）

```yaml
# 客户端写入 ~/.mutagen.yml（cloud-claude 自动生成）
sync:
  defaults:
    mode: two-way-resolved        # 本地优先，避免远端误删覆盖本地
    ignore:
      syntax: docker              # 复用项目已有 .dockerignore 风格规则
      vcs: true                   # 自动忽略 .git/.hg/.svn
      paths:
        - "node_modules/**"
        - ".venv/**"
        - "target/**"
        - "dist/**"
        - "build/**"
        - "*.log"
        - "*.tmp"
    permissions:
      defaultFileMode: "0644"
      defaultDirectoryMode: "0755"
    symbolicLinks:
      mode: portable
    watch:
      mode: portable              # 跨平台 watcher
```

**白名单上限策略**（F1 的"≤50MB"要求）：cloud-claude 在创建 session 前先 `du -sb` 扫描候选目录，超过 50MB 的目录拒绝放进 Mutagen branch，自动改用 sshfs 兜底。Mutagen 自己没有"总量超过则拒绝"的开关，必须在 CLI 层守门。

---

## 2. 联合视图：mergerfs（F1 / F2）

### 推荐与版本

| 项目 | 取值 | 来源 |
|------|------|------|
| mergerfs | 2.41.1 | https://github.com/trapexit/mergerfs/releases (latest 2025-11-19) |
| 依赖 libfuse | bundled libfuse（mergerfs 自带，源码 `/libfuse` 目录） | https://github.com/trapexit/mergerfs/tree/master/libfuse |
| 安装包 | 优先用 trapexit 的 deb（Debian/Ubuntu）或 static linux_amd64 tarball | release assets 列表 |

mergerfs 2.41.0（2025-11-12）是大改版本，引入 IO passthrough、统计/日志增强、fsck 工具，并且**默认 `category.create` 从 `epmfs` 改成了 `pfrd`**（来源：https://github.com/trapexit/mergerfs/discussions/1571 与 https://github.com/trapexit/mergerfs/blob/d8918458/mkdocs/docs/faq/configuration_and_policies.md "Why is pfrd the default create policy?"）。2.41.1 是紧随其后的修复版本。

### 为什么是 mergerfs

1. **唯一在 union FS 领域仍然活跃维护的 FUSE 方案**。OverlayFS 在内核态但要求所有 lower 在同一文件系统上，**不能跨 FUSE 挂载**（Mutagen 同步目录和 sshfs 挂载点不可能在同一底层 FS）。aufs 已退出主线内核。unionfs-fuse 多年无 release。
2. **branch mode（RO / NC / RW）原生支持**：可以把 sshfs branch 标 `=NC,RO`，让 mergerfs 永远不在 sshfs 上创建新文件，只读它，正好对应 F1"sshfs 冷兜底懒读"的语义。
3. **`func.readdir=cor` 并发 readdir**：源码 `src/fuse_readdir_cor.cpp` 实现 thread pool 并发打开 + 并发读所有 branch 的目录，对网络 branch（sshfs）尤其重要，是把 `ls -R` 体验拉回到本地 1.5x 范围内的关键。
4. **`category.create=ff`**：first-found 语义保证只要 Mutagen branch 是 RW 且有空间，所有新建都落在 Mutagen 上。配合 sshfs branch 标 NC/RO，写入只能落在 Mutagen branch，与 F1 设计一致。

### 为什么不用替代方案

| 替代 | 不选的事实依据 |
|------|----------------|
| **OverlayFS（kernel）** | 要求 lower/upper 在同一文件系统；FUSE-on-FUSE 不被 overlayfs 支持；本项目 lower 是 sshfs（FUSE）+ Mutagen 同步目录，无法满足。 |
| **unionfs-fuse** | 上次 release 2018 年，与现代 libfuse3 兼容性差。 |
| **aufs** | 已从主线内核移除（v3.x 时代），Debian/Ubuntu 不再分发。 |
| **bind mount + 软链接合并** | 不是"合并目录视图"，遇到同名 entry 没有去重，开发者调试体验差。 |

### 与 v2.0 的兼容性

- **AppArmor unconfined + SYS_ADMIN + /dev/fuse**：v2.0 已经为 sshfs 一并放开，mergerfs 同样是 FUSE 用户态，**复用同一组特权，零增量**。验证依据：sshfs 在 worker `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined` 已工作，mergerfs 不需要额外 capability。
- **FUSE on FUSE**：mergerfs 的 branch 是另一个 FUSE 挂载（sshfs）这种"FUSE on FUSE"在 Debian 12 / Ubuntu 24.04 + libfuse3 ≥ 3.10 完全支持，trapexit 官方文档专门列了 SSHFS 兼容性说明（https://trapexit.github.io/mergerfs/latest/remote_filesystems/）。
- **sing-box tun + nftables**：mergerfs 完全本地，没有任何网络流量。
- **mergerfs 可由非 root mount**：传 `-o allow_other` 时需要 `/etc/fuse.conf` 里 `user_allow_other`。worker 启动容器时 entrypoint 用 root mount 然后 `chown` 给 claude 用户更省事。

### 关键配置（必须显式声明，不能用默认值）

```bash
# entrypoint.sh 内部，root 身份执行
mergerfs \
  -o category.create=ff \
  -o category.action=epall \
  -o category.search=ff \
  -o func.readdir=cor:4 \
  -o cache.attr=30 \
  -o cache.entry=30 \
  -o cache.negative-entry=10 \
  -o cache.readdir=true \
  -o cache.files=off \
  -o dropcacheonclose=false \
  -o parallel-direct-writes=true \
  -o moveonenospc=false \
  -o inodecalc=path-hash \
  -o minfreespace=128M \
  -o noforget \
  -o allow_other \
  -o nonempty \
  /workspace-hot=RW:/workspace-cold=NC,RO \
  /workspace
```

**逐项解释（这些不是 vibes，是来自 mergerfs 官方文档的明确建议）**：

- `category.create=ff`：first-found，配合 branch 顺序 `hot 在前、cold 在后` + cold 标 NC 后，所有 create 必落 hot。**必须显式声明**，因为 2.41+ 的默认是 `pfrd`，会按"剩余空间百分比加权随机"分到 cold（违反我们的设计）。
- `func.readdir=cor:4`：concurrent open & read，4 个工作线程。来源 https://deepwiki.com/trapexit/mergerfs/6.2-concurrency-and-threading 与 issue #1589。**注意已知 bug**：`cor` + NFS 导出 + RO/RW 混合 branch 时会出现 "ls 偶发文件重复"（issue #1589, 2025-11-23）。我们**不通过 NFS 二次导出 mergerfs**，因此不受影响。如果未来要改，回退到 `cosr`。
- `cache.attr=30 cache.entry=30 cache.negative-entry=10 cache.readdir=true`：让 kernel page cache 替我们吸收一部分 sshfs 高延迟。trapexit 在 issue #893 明确推荐这一组用于带 sshfs branch 的场景。
- `cache.files=off`：直接 IO，避免 mergerfs + sshfs + page cache 三层重复 cache 引发的内存占用。`claude` 工作流不依赖 mmap。trapexit 官方建议中除非应用强依赖 mmap（rtorrent / sqlite），否则一律 `off`。
- `inodecalc=path-hash`：保证重启后 inode 稳定，避免编辑器 / git 出现"文件变了"的误判。
- `noforget`：避免 kernel 在内存压力下让 mergerfs 忘掉 inode 映射（开发场景文件少，开销可忽略）。
- `nonempty`：允许在 `/workspace` 已经存在内容时挂载（v2.0 有时会先 cd 再 mount，需要这个）。
- 不开 `cache.writeback`：v2.0 的写回模型由 Mutagen 双向同步保证，mergerfs 层写回会和 Mutagen 的事件触发产生竞争，反而拖慢"编辑→远端可见"。

### 兜底降级（F2）

cloud-claude 检测三层任一失败时，按顺序降级并告知用户：

| 失败项 | 检测方式 | 降级行为 | 用户提示（中文） |
|--------|----------|----------|------------------|
| Mutagen daemon 启动失败 / agent 拷贝失败 | `mutagen sync create` 退出码 ≠0 / 60s 内未达到 watching | 跳过 mutagen，sshfs branch 直接挂在 `/workspace` | "热同步未就绪，已切到 sshfs-only 模式（编辑会经过网络，IO 会更慢）" |
| sshfs 挂载失败 | sshfs 进程退出 / mountpoint 检测失败 | 跳过 sshfs，仅 Mutagen 同步目录暴露成 `/workspace` | "冷兜底未就绪，已切到 mutagen-only 模式（仅同步白名单内文件）" |
| mergerfs 挂载失败 | mergerfs 进程异常 / mountpoint 检测失败 | 直接 bind mount mutagen branch（如可用）或 sshfs branch | "联合视图未就绪，已切到单层挂载，请稍后用 `cloud-claude doctor` 排查" |

---

## 3. 冷兜底：sshfs（F1）

### 推荐与版本

| 项目 | 取值 | 来源 |
|------|------|------|
| sshfs | 3.7.5 | https://github.com/libfuse/sshfs/releases/tag/sshfs-3.7.5（2025-11-11，新维护团队接手后的首个 release） |
| libfuse 依赖 | ≥ 3.1.0；推荐 3.18.x | https://github.com/libfuse/sshfs（README installation） |

v2.0 已经在生产用 sshfs（具体版本视镜像基线，需要核对 `image.lock`）。v3.0 应当**至少升到 3.7.5**，因为：
- 3.7.4 之前已经停更（最后官方版本 3.7.3 是 2022-05-26）
- 3.7.5 修了一系列内存泄漏和 readdir cache 相关 bug（明确列出 "Fix memleak in cache after readlink" / "Fill stat info when returning cached data for readdir"），对长会话稳定性有直接帮助
- 主线开始重新有维护者

### 为什么继续用 sshfs（而不是别的网络 FS）

v2.0 已经做出选择并验证，本节只补充 v3.0 视角的复核：

| 替代 | 不选的事实依据 |
|------|----------------|
| **NFSv4** | 需要在容器内额外开 RPC 端口，与"--network=none + tun" 出网模型冲突；且 NFS 通过 sing-box tun 出去再回来不可行。 |
| **rclone mount** | 通用对象存储/云盘视角，针对源码场景的 readdir / stat 性能远差于 sshfs。 |
| **WebDAV / davfs2** | 协议本身性能差，无法稳定支持 fsync。 |

### sshfs branch 端的性能调优

来源：https://deepwiki.com/libfuse/sshfs/4.3-performance-tuning + trapexit 官方对 sshfs branch 的建议。

```bash
# entrypoint.sh，sshfs 挂 cold branch 到 /workspace-cold
sshfs \
  -o slave \
  -o reconnect \
  -o ServerAliveInterval=15 \
  -o ServerAliveCountMax=3 \
  -o auto_unmount \
  -o cache=yes \
  -o dcache_timeout=60 \
  -o dcache_stat_timeout=60 \
  -o dcache_dir_timeout=60 \
  -o dcache_max_size=20000 \
  -o max_conns=4 \
  -o Compression=no \
  -o Ciphers=aes128-gcm@openssh.com \
  -o kernel_cache \
  -o big_writes \
  -o no_readahead=false \
  passive@localhost:/ /workspace-cold
```

**理由**：
- `dcache_timeout=60` / `dcache_dir_timeout=60`：sshfs 默认 20s 太激进，会和 mergerfs 的 `cache.attr=30` 不协调。统一拉到 60s。
- `max_conns=4`：sshfs 3.x 的并发 SFTP 通道，开 4 条已经能把单连接顺序读的瓶颈冲掉，再多收益不明显且占容器内存。
- `Compression=no`：本项目流量已经在 sing-box tun 出口走过一次，没必要再压（CPU 才是瓶颈）。
- `Ciphers=aes128-gcm@openssh.com`：v2.0 用 OpenSSH 10.2，arcfour 已被 OpenSSH 9.x 移除。`aes128-gcm` 是 OpenSSH 默认顺位且有 AES-NI 硬件加速。**不要再写文档里到处看到的 `arcfour`，那是 OpenSSH 7.5 之前的建议**。
- `slave`：保留 v2.0 的 slave 模式，继续用 cloud-claude 端的嵌入 SFTP server。

### 与 v2.0 的兼容性

完全兼容，仅是版本升级。sshfs 3.7.5 ABI 与 3.7.3 相同，slave 模式行为不变。

---

## 4. 会话恢复：tmux（F4 / F5）

### 推荐与版本

| 项目 | 取值 | 来源 |
|------|------|------|
| tmux | 3.6a | https://github.com/tmux/tmux/releases/tag/3.6a（2025-12-05） |
| 依赖 libevent | ≥ 2.x | https://github.com/tmux/tmux/ README |
| 依赖 ncurses | 系统包即可 | 同上 |

3.6 是 2025-11-26 发布的大版本（5.x → 3.5.x 之后第一个新功能 minor），3.6a 是其 5 天后的补丁版本。

### 为什么是 tmux

1. **多客户端 attach 同一 session**是 tmux 的内建模型（`tmux attach -t name` 多次执行就是 F5 默认行为；`-d` 标志独占就是 `--new-session` 语义）。
2. **socket-per-session** 简洁可控：每个 claude_account 容器内只跑一个 tmux server，socket 在 `/tmp/tmux-${UID}/default`，cloud-claude 用 `tmux ls / has-session / new-session -d -s claude / attach -t claude` 即可完成所有控制。
3. **长期稳定**：协议在 3.x 系列基本稳定，session 文件可以跨 patch 版本恢复。
4. **Debian/Ubuntu 包仓库实时跟进**：可直接 apt 装，免编译。
5. **键位 / mouse / 256-color 全支持**：用户已经习惯的体验，无任何学习成本（默认前缀 `Ctrl-b`，用户可在镜像里覆盖）。

### 为什么不用替代方案

| 替代 | 不选的事实依据 |
|------|----------------|
| **dtach** | 上次 release v0.9 是 2016 年，2016 年至今没有任何新功能。**不支持多 client 共享一个 session**（无法满足 F5）。 |
| **abduco** | v0.6 是 2016 年最后一次 release（GitHub 页 release date 2020-02-11 是 retag）。同样不支持多 client attach。 |
| **screen** | UTF-8 / 256color 支持差，配置心智负担更高，没有任何 tmux 没有的能力。 |
| **zellij** | v0.44.1（2026-04-07）功能很丰富但 release notes 自己承认**"sessions have never been backwards compatible. Each version upgrade would orphan existing sessions"**。镜像升级 = 用户运行中的 session 全部丢失，与 F4"运行中的 claude 进程不丢"承诺直接冲突。 |
| **Eternal Terminal** | 解决的是"传输+session 一体"问题，需要 etserver 监听 2022 端口（TCP）+ 自有协议。我们已经统一通过 v2.0 的 SSH proxy + Entry API 走 22，引入 ET 等于把用户从一个统一通道拽到两个通道。详见下一节"为什么不用 ET 做 SSH 弱网容忍"。 |

### 与 v2.0 的兼容性

- **TTY/信号/exit code 透传**：tmux 自己处理 SIGWINCH 和 PTY，对 cloud-claude 的 v2.0 透传逻辑透明。
- **AppArmor / capability**：tmux 完全用户态，零增量。
- **socket 路径**：`/tmp/tmux-${UID}` 是普通 Unix socket，不受 sing-box tun 影响。

### 集成形式

1. **镜像 build**：apt 装 `tmux`。
2. **entrypoint.sh / login shell**：检测到通过 SSH 进入交互 shell 时执行：
   ```bash
   if [ -z "$TMUX" ] && [ -t 1 ]; then
     exec tmux new-session -A -s claude
   fi
   ```
   `-A` 表示 "attach if exists, else create"——天然命中 F5 的"默认 attach 同一 session"。
3. **cloud-claude `--new-session`**：远端命令改为 `tmux new-session -d -s claude-$(date +%s) && tmux attach -t claude-$(...)` 强制建独立 session。
4. **冲突提示**：cloud-claude 在 attach 前先 `tmux list-clients -t claude`，若已有 client 且未传 `--new-session`，给出中文提示并继续 attach（共享）；如果用户已经传了 `--new-session`，则建新名字 session 并提示当前总共有几个独立 session。

---

## 5. SSH 弱网容忍（F3）

### 推荐：**保持 OpenSSH，不引入新组件**

| 决定项 | 取值 |
|--------|------|
| 客户端 SSH 库 | 复用 v2.0 的 `golang.org/x/crypto/ssh`（cloud-claude 内置） |
| 心跳 | `ServerAliveInterval=15`、`ServerAliveCountMax=4` 对应 60s 静默后客户端断 |
| 服务器端 | `ClientAliveInterval=15`、`ClientAliveCountMax=8` 对应 120s 容忍（让客户端先反应） |
| 重连 | cloud-claude 自实现 reconnect with exponential backoff（1s → 2s → 4s → 8s → 30s 上限），重连复用本地缓存的 Entry API token，**不重新弹密码** |
| TCP 层 | 客户端 socket 启用 `SO_KEEPALIVE`、`TCP_USER_TIMEOUT=30000`（Linux）/ `TCP_KEEPALIVE`（macOS） |

### 为什么不引入 mosh

**核心阻断：mosh 与 v2.0 的 sing-box tun + nftables 默认拒绝模型不兼容。**

1. **mosh 走 UDP 60000–61000**（来源：https://oneuptime.com/blog/post/2026-03-20-mosh-ipv6-configuration/view 与 https://bbs.archlinux.org/viewtopic.php?id=229142）。v2.0 的 nftables 默认拒绝策略只对 TCP 22（SSH 上行）和 sing-box tun 出站做了白名单。引入 mosh 必须：
   - 在宿主机网卡上额外暴露 UDP 60000-61000 给所有用户容器（多租户冲突）
   - 在容器 netns 与 mgmt veth 之间转发这段 UDP（破坏"所有出网走 tun"承诺）
   - 在控制面新增"分配 UDP 端口"逻辑（多租户冲突）
2. **mosh 自己上次 release 是 2022-10-27 的 1.4.0**（https://github.com/mobile-shell/mosh/releases），3 年没有官方 release。仅 1.4g 在发行版打补丁层。
3. **mosh 不能传输任意二进制 / scrollback**：与 cloud-claude "TTY/信号/退出码完全透传"承诺有冲突（mosh 自己有屏幕预测和重绘协议，不是 raw bytes）。

**结论**：mosh 在本项目**没有合适的兼容方案**。哪怕同意为它单独开 UDP 端口（完全可以做到，只要 nftables 加一行 `udp dport 60000-61000 accept`、宿主机 SNAT 带容器源 IP），也得不偿失：把"严格出口控制"这个产品核心承诺的边界搞复杂了。

### 为什么不引入 autossh

- autossh 只解决"ssh 进程死了自动重启"的问题，但 cloud-claude 自己就是 wrapper（处理认证、挂载、tmux、错误码）。再套 autossh 等于嵌套两层 wrapper，错误码、信号、终端恢复全部要重新打洞。
- autossh 最近一次 GitHub release 是 1.4f（2021-04-02），主线开发停滞，仅靠发行版打 1.4g 补丁。

**正确做法**：把 autossh 干的事（"ssh 退出 → 等待 → 重连"）直接写在 cloud-claude 里，配合现有 Entry API token 缓存做无密码重连。

### 为什么不引入 Eternal Terminal

- ET v6.2.11（2025-07-22）是好工具，但它是**完整替换 SSH 的协议**：客户端连 etserver:2022（TCP），SSH 仅在初始握手用一次。这意味着：
  1. 必须在容器内或宿主机起 etserver 守护进程（额外端口、额外认证、额外审计面）
  2. v2.0 的 SSH proxy + Entry API 认证流要么作废、要么和 ET 并存（运维成本翻倍）
- ET 的"reconnect"能力对应到 mosh 思路（隧道断了内部状态机自愈），它的优势在 NAT 漫游场景。本项目用户主要是固定办公网络，价值有限。

### 与 v2.0 的兼容性

不引入新组件 = 零兼容性风险。仅扩展 cloud-claude 自身代码。

---

## 6. 状态持久化：Docker named volume（F7）

### 推荐与命名规范

| 项目 | 取值 |
|------|------|
| 形式 | Docker named volume（`docker volume create`） |
| 命名规范 | `claude-state-{claude_account_id}`，例 `claude-state-7e8f9a1b` |
| 挂载点 | 容器内 `/home/claude/.claude` 与 `/home/claude/.cache/claude` |
| 挂载方式 | `docker run` 时 `--mount type=volume,src=claude-state-{id},dst=/home/claude/.claude` 与同形式 dst=...cache/claude |
| 驱动 | 默认 `local` 驱动（落在宿主机 `/var/lib/docker/volumes/`） |
| 备份 | 集成进现有 `scripts/backup.sh`，`tar` 整个 `/var/lib/docker/volumes/claude-state-*/_data` |

### 为什么是 named volume

| 替代 | 不选的事实依据 |
|------|----------------|
| **bind mount 到宿主机目录** | 路径要在 control-plane 和 host-agent 间约定，破坏"特权边界"；权限管理要做 chown 给容器内 claude 用户，跨内核 / 内核 namespace 容易搞错。 |
| **tmpfs** | 反方向，tmpfs 是非持久化。 |
| **额外的 PostgreSQL 字段存登录态** | Claude Code 的登录态是它自己的二进制配置文件（OAuth token + cache），由它自己管理，控制面不应解析。 |
| **挂宿主机 `/etc/letsencrypt` 风格的 host path** | 同 bind mount 缺点 + 跨账户隔离更难。 |

### 与 v2.0 的兼容性

- **worker.go 改造点**：`createContainer` 函数在 `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined --network=none` 之外追加两个 `--mount type=volume,src=claude-state-{account_id},dst=/home/claude/.claude` 与同形式 dst=`/home/claude/.cache/claude`。
- **首次创建**：worker 在 `Container.Start` 之前确保 volume 存在（`docker volume create` 幂等），并通过 entrypoint 在容器内 `chown -R claude:claude /home/claude/.claude /home/claude/.cache/claude`。
- **重建容器不丢登录态**：Phase 11 已经实现的 "用户重建主机" 路径会调用 worker 销毁旧容器、起新容器，但**只要不删 volume**，新容器仍挂在同一 volume 上，Claude Code 启动时自动复用 token。
- **删除 claude_account**：在 admin 删 account 时同步 `docker volume rm claude-state-{id}`，并记一条事件。

### 多端冲突（与 F5 关系）

Claude Code 自己的进程是 single-process，多端 SSH attach 进同一个 tmux session 时，`claude` 进程也只有一个，因此 `~/.claude` 不会出现多 writer 冲突。如果用户用 `--new-session` 强制开新 session，会启动第二个 `claude` 进程，**它们读写同一份 `~/.claude` 文件**——这是 Claude Code 自身的能力边界，不是 v3.0 要解决的事，但 doctor 应当能检测到"同一 volume 下有多个 claude 进程在跑"并提示。

---

## 7. 错误码体系扩展（F8）

### 推荐：沿用 v2.0 现有 7 错误码格式，新增分类前缀

v2.0 已经定义了 7 个稳定的退出码（CLOUDCLAUDE_E_*），bootstrap 脚本和 cloud-claude 共用。v3.0 的新错误路径**不要新发明编码体系**，只在分类前缀上扩展：

| 前缀 | 含义 | 示例新增码 |
|------|------|------------|
| `MOUNT_*` | 三层文件系统相关 | `MOUNT_MUTAGEN_FAILED`、`MOUNT_SSHFS_DOWN`、`MOUNT_MERGERFS_DEGRADED`、`MOUNT_DISK_FULL` |
| `SESSION_*` | 会话恢复 / 多端 | `SESSION_TMUX_NOT_FOUND`、`SESSION_ATTACH_CONFLICT`、`SESSION_NEW_FORCED` |
| `NET_*` | SSH 弱网 / 重连 | `NET_HEARTBEAT_TIMEOUT`、`NET_RECONNECT_GIVEUP`、`NET_TOKEN_EXPIRED` |
| `STATE_*` | 持久化 volume | `STATE_VOLUME_MISSING`、`STATE_VOLUME_PERMISSION` |

每条错误必须满足 v2.0 已有的契约：
1. 退出码（保留 v2.0 表，超出范围用新区段如 50–59 给 mount，60–69 给 session）
2. 中文描述（短）
3. "下一步该做什么"（短，可以引用 `cloud-claude doctor [子检查]`）
4. BATS 测试覆盖到错误码 → 提示文案的稳定映射

不引入新依赖，纯 Go 内部 `errors.Is` / 自定义 error type 即可。

---

## 8. doctor 升级（F6）—— 复用现有 cobra subcommand

不引入新组件。`cloud-claude doctor` 在 v2.0 的 cobra 树下扩展子命令：

| 子命令 | 检查项 | 调用方式 |
|--------|--------|----------|
| `doctor network` | sing-box tun 出口 IP / DNS / WebRTC 泄漏 | 复用 v1.1 的三重校验 API |
| `doctor auth` | Entry API token 是否过期 / 能否刷新 | 调用控制面 |
| `doctor ssh` | SSH 连通 / 心跳 / 端口 | 本地 ssh probe |
| `doctor mount` | mutagen session 状态 / sshfs mountpoint 存在 / mergerfs 进程 | `mutagen sync list` + `mountpoint -q` + `pgrep mergerfs` |
| `doctor disk` | volume 占用 / 可用空间 / inode | `df` + `docker volume inspect` |

每项支持 `--fix` 自动修复（重启 mutagen / 重 mount sshfs / 重 mount mergerfs / 清理 mutagen 残留 session），不能修复的明确指引"请管理员检查 X"。

---

## Recommended Stack 一表汇总

### 新增 / 升级核心组件

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Mutagen | 0.18.1 | 源码热同步白名单（F1） | 唯一在 SSH transport 下双向 + 实时 watcher + 冲突合并都成熟的方案；客户端零依赖（OpenSSH 直接调） |
| mergerfs | 2.41.1 | 联合视图（F1 / F2） | union FS 唯一活跃维护者；branch RO/NC + cor readdir 直接命中"sshfs 兜底 + 写入只落 hot"语义 |
| sshfs | 3.7.5 | 冷兜底懒读（F1） | 新维护团队接手后的首个稳定版，修了 readdir cache 多个 leak |
| tmux | 3.6a | 会话恢复 + 多端 attach（F4 / F5） | 唯一同时支持持久 session、多 client 共享、跨 patch 兼容的成熟方案 |

### 沿用 v2.0 不变

| Technology | Version（v2.0 已定） | 为什么 v3.0 不动 |
|------------|---------------------|------------------|
| Go | 1.26.1 | cloud-claude 与 control-plane 同栈 |
| OpenSSH | 10.2p1 | SSH 弱网用现有客户端 + ServerAliveInterval 已够；不引入 mosh/ET |
| sing-box | 1.13.3 | 出网模型不变 |
| Docker Engine | 28.x | volume / FUSE / cap 模型完全沿用 |
| PostgreSQL | 18.x | 控制面只新增 claude_account → volume 名映射列 |
| libfuse3 | 3.18.x | 镜像基线 |
| pkg/sftp | v2.0 选定版本 | sshfs slave 模式继续用 |

### 不引入（重要）

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| mosh | UDP 60000-61000 与 sing-box tun + nftables 默认拒绝模型不兼容；mosh 1.4.0 自 2022 起无新 release | OpenSSH ServerAliveInterval + cloud-claude 自实现 reconnect |
| autossh | 仅做"ssh 死了重启"，cloud-claude 已是 wrapper，套娃增加错误码复杂度；上次 release 2021 | cloud-claude 内置重连逻辑 |
| Eternal Terminal | 替换 SSH 协议体，破坏 v2.0 SSH proxy 单通道模型 | tmux + ssh 重连组合已能满足 F3+F4 |
| zellij | session 不跨版本兼容（官方明确说明），镜像升级会丢用户运行中状态 | tmux 3.6a |
| dtach / abduco | 不支持多 client 共享 session，无法满足 F5 | tmux 3.6a |
| OverlayFS | 不支持 FUSE-on-FUSE | mergerfs |
| unison / syncthing / lsyncd / rsync+inotify | 见上文逐条理由 | Mutagen |
| `category.create=pfrd`（mergerfs 2.41 默认） | 会让 create 按"剩余空间百分比"随机分配，violates "写入必落 hot branch" 设计 | 显式 `category.create=ff` 并把 sshfs 标 `=NC,RO` |

---

## Version Compatibility

| 组合 | 状态 | 备注 |
|------|------|------|
| Mutagen 0.18.1 + OpenSSH 10.2p1 | ✓ 兼容 | Mutagen 直接调本机 ssh/scp，不嵌入第三方 SSH 库 |
| Mutagen agent + Linux amd64 容器 | ✓ 兼容 | Mutagen 官方为 linux_amd64 单独打包 agent |
| sshfs 3.7.5 + libfuse 3.18.x | ✓ 兼容 | sshfs README 明确要求 ≥ 3.1.0 |
| mergerfs 2.41.1 + sshfs 3.7.5 branch | ✓ 兼容 | trapexit 官方 remote_filesystems.md 列出 SSHFS 兼容性 |
| mergerfs 2.41.1 + AppArmor unconfined + SYS_ADMIN | ✓ 兼容 | v2.0 已为 sshfs 同样配置，零增量 |
| mergerfs `func.readdir=cor` + 我们的 mount 拓扑（不通过 NFS 二次导出） | ✓ 兼容 | issue #1589 的 cor + NFS 重复 entry bug 不命中我们 |
| tmux 3.6a + Debian 12 / Ubuntu 24.04 镜像基线 | ✓ 兼容 | apt 直装；libevent 2.x 与 ncurses 系统包默认满足 |
| Docker named volume + apparmor=unconfined + --network=none | ✓ 兼容 | volume 是 mount namespace 范畴，不受 network namespace 影响 |

---

## 安装 / 镜像 build 基线（增量）

```dockerfile
# 在 v2.0 的受管镜像 Dockerfile 末尾追加
ARG MUTAGEN_VERSION=0.18.1
ARG MERGERFS_VERSION=2.41.1
ARG SSHFS_VERSION=3.7.5
ARG TMUX_PACKAGE=tmux

# tmux + 升级 sshfs 到 3.7.5
RUN apt-get update && apt-get install -y --no-install-recommends \
      ${TMUX_PACKAGE} \
      libglib2.0-0 \
      meson ninja-build pkg-config \
      ca-certificates curl xz-utils \
    && rm -rf /var/lib/apt/lists/*

# 编译 sshfs 3.7.5（如果系统包版本低于 3.7.5；否则 apt 装即可）
# 此处略，构建脚本里按 image.lock 走

# mergerfs 静态二进制
RUN curl -fsSL "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs-${MERGERFS_VERSION}-static-linux_amd64.tar.gz" \
      | tar -xz -C /usr/local --strip-components=2 \
    && /usr/local/bin/mergerfs --version

# Mutagen agent bundle（不放客户端，客户端在 cloud-claude 二进制里 embed）
RUN mkdir -p /opt/mutagen/agents/${MUTAGEN_VERSION} \
    && curl -fsSL "https://github.com/mutagen-io/mutagen/releases/download/v${MUTAGEN_VERSION}/mutagen_agents_v${MUTAGEN_VERSION}.tar.gz" \
       -o /tmp/mutagen-agents.tar.gz \
    && tar -xzf /tmp/mutagen-agents.tar.gz -C /opt/mutagen/agents/${MUTAGEN_VERSION} \
    && rm /tmp/mutagen-agents.tar.gz

# entrypoint.sh 中追加：mergerfs 挂载 + tmux 自动 attach 逻辑
COPY rootfs/usr/local/bin/cloud-claude-mount-stack.sh /usr/local/bin/
COPY rootfs/etc/profile.d/cloud-claude-tmux.sh /etc/profile.d/
```

---

## Stack Patterns by Variant

**v3.0 默认（推荐）**：Mutagen + sshfs + mergerfs 三层 + tmux 自动 attach + 自实现重连。

**降级路径**（cloud-claude 自动选择，对应 F2）：
- 如果宿主机 FUSE 不可用：`--mount-mode=mutagen-only`，仅 Mutagen 挂在 `/workspace`，sshfs / mergerfs 跳过。同时 doctor 报警。
- 如果 Mutagen daemon 启动失败：`--mount-mode=sshfs-only`，回到 v2.0 行为。
- 如果用户显式指定 `--mount-mode=full`：强制三层，任一失败直接退出（不降级，便于排错）。

**镜像最小化场景**（如果未来要做 "minimal claude image"）：可仅打包 sshfs + mergerfs，让 Mutagen agent 完全靠 scp 安装；冷启动会慢 2–4s，但镜像小 ~80MB。这是未来 v3.1 的优化空间，v3.0 不做。

---

## Sources

### 核心组件版本与文档（HIGH confidence — 来自官方 GitHub release / 官方 docs）

- https://github.com/mutagen-io/mutagen — Mutagen latest release v0.18.1（2025-02-24）
- https://github.com/mutagen-io/mutagen/releases/tag/v0.18.0 — v0.18 release notes（`.dockerignore` syntax）
- https://mutagen.io/documentation/transports/ssh/ — Mutagen SSH transport 实现细节（用本机 OpenSSH 的 ssh/scp）
- https://mutagen.io/documentation/transports/docker/ — Mutagen Docker transport（说明为什么我们选 SSH transport 而不是 Docker transport）
- https://github.com/trapexit/mergerfs/releases/tag/2.41.0 — mergerfs 2.41.0 release notes（IO passthrough、默认 create policy 改成 pfrd）
- https://github.com/trapexit/mergerfs（latest release 2.41.1，2025-11-19）
- https://github.com/trapexit/mergerfs/discussions/1571 — 2.41.0 discussion 确认默认值变更
- https://github.com/trapexit/mergerfs/blob/d8918458/mkdocs/docs/faq/configuration_and_policies.md — "Why is pfrd the default" 与如何强制路径关联（`ff` / `epmfs`）
- https://github.com/trapexit/mergerfs/blob/d8918458/mkdocs/docs/config/functions_categories_policies.md — 完整 policy 列表 + 默认值表
- https://trapexit.github.io/mergerfs/latest/remote_filesystems/ — sshfs as branch 官方建议
- https://deepwiki.com/trapexit/mergerfs/6.2-concurrency-and-threading — `func.readdir=cor` / `cosr` / `seq` 实现差异
- https://github.com/trapexit/mergerfs/issues/1589 — `cor` + NFS 导出 + RO/RW 混合 branch 的已知 bug（不影响我们）
- https://github.com/trapexit/mergerfs/issues/893 — sshfs branch 性能讨论 + cache.attr/entry/readdir 调优建议（trapexit 本人发言）
- https://github.com/libfuse/sshfs/releases/tag/sshfs-3.7.5 — sshfs 3.7.5 release（2025-11-11，新维护团队首版）
- https://github.com/libfuse/sshfs/blob/master/ChangeLog.rst — 完整 changelog
- https://deepwiki.com/libfuse/sshfs/4.3-performance-tuning — sshfs 性能调优（`dcache_timeout` / `max_conns` / `Compression` / `Ciphers`）
- https://github.com/tmux/tmux/releases/tag/3.6a — tmux 3.6a release（2025-12-05）
- https://github.com/tmux/tmux/releases/tag/3.6 — tmux 3.6 release（2025-11-26，首版）

### 不选方案的事实依据（HIGH confidence）

- https://github.com/mobile-shell/mosh/releases — mosh 1.4.0 是 2022-10-27 至今唯一 release
- https://oneuptime.com/blog/post/2026-03-20-mosh-ipv6-configuration/view — mosh 端口范围 60000-61000 / 仅 UDP 的明确说明
- https://bbs.archlinux.org/viewtopic.php?id=229142 — mosh 防火墙规则示例（确认 UDP only）
- https://github.com/Autossh/autossh/blob/main/CHANGES — autossh 1.4f / 1.4g changelog，确认主线开发停滞
- https://github.com/MisterTea/EternalTerminal — Eternal Terminal v6.2.11（2025-07-22）；README 确认 etserver 默认监听 2022 TCP
- https://github.com/zellij-org/zellij/releases/tag/v0.44.0 — zellij 0.44.0 release notes 明确"sessions have never been backwards compatible"
- https://github.com/martanne/abduco — abduco 上次 release 2020（v0.6 retag 2016）
- https://github.com/crigler/dtach — dtach v0.9（2016）

---

*Stack research for: cloud-claude v3.0 远端开发体验升级（F1–F8）*
*Researched: 2026-04-18*
*Confidence: HIGH — 所有版本号通过 GitHub release page 直接核实；所有"不用 X"都有具体事实链接；mergerfs 2.41 默认值改变与 mosh/UDP 不兼容两条最关键风险已交叉验证*
