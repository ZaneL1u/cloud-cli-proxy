# Phase 24: 受管镜像 FUSE 硬化与容器参数 - Research

**Researched:** 2026-04-15
**Domain:** Docker FUSE/sshfs 容器集成、容器运行时参数、SSH Proxy 多 session 验证
**Confidence:** HIGH

## Summary

本阶段的核心任务是三件事：在受管镜像中预装 sshfs + fuse3 并配置 FUSE 权限、在 Worker 创建容器时添加 FUSE 设备和 SYS_ADMIN 能力、验证 SSH Proxy 现有代码无需改动。三项工作都有成熟的标准做法，技术风险低。

最关键的陷阱是 `/dev/fuse` 设备在容器内默认权限为 `crw-rw---- root:root`，非 root 用户（workspace UID 1000）无法直接访问。必须在 entrypoint.sh 中修复此权限，否则后续阶段的 sshfs 挂载将在运行时失败。

**Primary recommendation:** 在 Dockerfile 中安装 sshfs + fuse3 并配置 `/etc/fuse.conf`，在 entrypoint.sh 中添加 `/dev/fuse` 权限修复，在 worker.go 的 `createHost()` 中追加 `--device /dev/fuse` 和 `--cap-add SYS_ADMIN`。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 直接在现有 `deploy/docker/managed-user/Dockerfile` 中添加 sshfs + fuse3 包，不创建新的镜像变体。所有用户容器统一具备 FUSE 能力，减少镜像维护负担。
- **D-02:** 在镜像中配置 `/etc/fuse.conf` 启用 `user_allow_other`，允许非 root 用户使用 `-o allow_other` 选项挂载 sshfs。
- **D-03:** Worker 创建容器时对所有容器统一附加 `--device /dev/fuse` 和 `--cap-add SYS_ADMIN`，不做条件区分。cloud-claude 设计意图是所有用户容器都可被连接。
- **D-04:** 新增参数在 `internal/runtime/tasks/worker.go` 的 `createHost()` 函数中添加，与现有 `--cap-add NET_ADMIN` 同级。
- **D-05:** SSH Proxy 保持零改造。通过代码审查确认现有能力并在本 CONTEXT 中记录结论。
- **D-06:** 验证结论以文档形式记录在计划中，不新增自动化测试。

### Claude's Discretion
- sshfs 和 fuse3 的具体 apt 包名选择（`sshfs` + `fuse3` 或更细粒度的包选择）
- Dockerfile 中新增 RUN 指令的位置（合并到现有 apt-get install 还是独立 RUN 层）
- 是否需要在 entrypoint.sh 中添加 FUSE 相关检查逻辑

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SRV-01 | 容器镜像预装 sshfs + fuse3 并配置 user_allow_other | Standard Stack 中确认的 apt 包名和 /etc/fuse.conf 配置方式 |
| SRV-02 | 容器创建时附加 `--device /dev/fuse` + `--cap-add SYS_ADMIN` | Architecture Patterns 中的 worker.go 参数追加模式和 Docker FUSE 标准实践 |
| SRV-03 | SSH Proxy 保持零改造，利用现有多 session channel + exec 转发能力 | SSH Proxy 代码审查结论：handleConnection() 循环和 handleChannel() 全类型转发 |
</phase_requirements>

## Standard Stack

### Core
| Library/Tool | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| sshfs | 3.7.3-1.1build3 | SFTP 文件系统客户端 | Ubuntu 24.04 Noble 官方仓库版本，支持 `-o passive`（slave 模式）|
| fuse3 | 3.14.0-5build1 | FUSE 用户空间文件系统框架 | Ubuntu 24.04 Noble 官方仓库版本，提供 fusermount3 |

### Supporting
| Component | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| /etc/fuse.conf | N/A | FUSE 全局配置文件 | 安装 fuse3 时自动创建，需启用 user_allow_other |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| sshfs + fuse3 单独安装 | 合并到现有 apt-get install 行 | 合并减少镜像层数但降低可读性；推荐合并 |
| SYS_ADMIN capability | moby/moby PR #41880 (seccomp/apparmor profile) | PR 至 2026 年仍未合并，不可用 |
| libfuse2 | fuse3 | fuse3 是 Ubuntu 24.04 的标准版本，sshfs 3.7.x 依赖 fuse3 |

**Installation (Dockerfile):**
```dockerfile
RUN apt-get update \
    && apt-get install -y --no-install-recommends sshfs fuse3 \
    && rm -rf /var/lib/apt/lists/*
```

**Version verification:** Ubuntu 24.04 Launchpad 确认 sshfs 3.7.3-1.1build3 (universe) 和 fuse3 3.14.0-5build1 (noble release)。

## Architecture Patterns

### Dockerfile 修改方案

**推荐：合并到现有 apt-get install 块。** sshfs 和 fuse3 是基础系统工具，与现有的 openssh-server、curl、git 等属于同一层级。独立 RUN 层会增加不必要的镜像层数。

```dockerfile
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server \
        bash \
        ...existing packages... \
        sshfs \
        fuse3 \
    && rm -rf /var/lib/apt/lists/*
```

在 apt-get 安装之后，添加 FUSE 配置：

```dockerfile
RUN sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf \
    && chmod a+r /etc/fuse.conf
```

### entrypoint.sh 修改方案

**必须添加 /dev/fuse 权限修复。** Docker `--device /dev/fuse` 传入的设备节点默认权限为 `crw-rw---- root:root`，workspace 用户（UID 1000）无法访问。entrypoint.sh 以 root 身份运行，是修复权限的正确位置。

```bash
# 确保非 root 用户可以访问 FUSE 设备
if [ -c /dev/fuse ]; then
  chmod 666 /dev/fuse
fi
```

### worker.go 参数追加

现有代码在 `createHost()` 中通过 `args = append(args, ...)` 构建 docker create 参数。新增 FUSE 参数应紧跟现有 `--cap-add NET_ADMIN` 之后：

```go
args := []string{
    "create",
    "--name", containerName,
    "--network", "bridge",
    "--cap-add", "NET_ADMIN",
    "--cap-add", "SYS_ADMIN",
    "--device", "/dev/fuse",
    "--label", "cloud-cli-proxy.managed=true",
    // ...rest of args
}
```

### SSH Proxy 零改造验证

代码审查确认以下事实（均来自 `internal/sshproxy/proxy.go`）：

1. **多 session channel 支持：** `handleConnection()` 第 203 行 `for newChan := range chans` 循环接受所有 session channel，每个 channel 启动独立 goroutine 处理。单个 SSH 连接可承载任意数量的并发 session channel。

2. **全类型请求转发：** `handleChannel()` 第 261-283 行双向转发所有请求类型，包括 pty-req、shell、exec、env、window-change（客户端→目标）和 exit-status、exit-signal（目标→客户端）。

3. **sshfs slave/passive 模式兼容性：** cloud-claude 将通过现有 SSH 连接开启第二个 session channel，在其中执行 `sftp-server` 子系统。SSH Proxy 的 `handleChannel()` 为每个新 channel 建立独立的到目标容器的 session，完整转发 exec 请求，天然支持此模式。

**结论：SSH Proxy 无需任何代码改动。**

### Anti-Patterns to Avoid
- **不要使用 `--privileged` 模式：** 授予全部能力，安全风险远大于 `--cap-add SYS_ADMIN`
- **不要在 Dockerfile RUN 阶段 chmod /dev/fuse：** 构建时 /dev/fuse 不存在，设备节点只在容器运行时由 `--device` 注入
- **不要将 workspace 用户添加到 fuse 组来解决权限问题：** Docker 注入的 /dev/fuse 属于 root:root，fuse 组在容器内不拥有该设备

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| FUSE 配置解析 | 手写 /etc/fuse.conf 文件 | `sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf` | fuse3 安装时自动生成带注释的配置文件，sed 修改更可靠 |
| 设备权限管理 | udev 规则或复杂权限方案 | entrypoint.sh 中 `chmod 666 /dev/fuse` | 容器环境没有运行 udevd，直接 chmod 是最简单可靠的方式 |
| SSH 多路复用验证 | 自动化集成测试 | 代码审查 + 文档记录 | SSH Proxy 代码未改动，审查代码路径即可确认 |

**Key insight:** 这个阶段的三项任务都是对现有系统的小幅增量修改，使用标准工具和简单配置即可完成，不需要发明任何自定义方案。

## Common Pitfalls

### Pitfall 1: /dev/fuse 非 root 用户 Permission Denied
**What goes wrong:** 非 root 用户（workspace）执行 sshfs 挂载时报 `fuse: failed to open /dev/fuse: Permission denied`。
**Why it happens:** Docker `--device /dev/fuse` 注入的设备节点默认权限为 `crw-rw---- root:root`，只有 root 和 root 组用户可以读写。
**How to avoid:** 在 entrypoint.sh 中以 root 身份执行 `chmod 666 /dev/fuse`。entrypoint.sh 以 root 运行，在切换到 workspace 用户之前执行此操作。
**Warning signs:** 容器启动后以 workspace 用户运行 `ls -la /dev/fuse` 看到权限不是 `crw-rw-rw-`。

### Pitfall 2: /etc/fuse.conf 权限不可读
**What goes wrong:** 非 root 用户使用 `-o allow_other` 挂载时报 `fusermount: failed to open /etc/fuse.conf: Permission denied`。
**Why it happens:** 某些发行版中 `/etc/fuse.conf` 默认权限可能不允许非 root 用户读取。
**How to avoid:** 在 Dockerfile 中同时执行 `chmod a+r /etc/fuse.conf`，确保所有用户都可以读取配置文件。
**Warning signs:** `fusermount` 报错提及 `Permission denied` 并引用 `/etc/fuse.conf`。

### Pitfall 3: SYS_ADMIN 能力遗漏
**What goes wrong:** 只添加了 `--device /dev/fuse` 但没有 `--cap-add SYS_ADMIN`，sshfs 报 `fuse: mount failed: Operation not permitted`。
**Why it happens:** Docker 默认的 seccomp profile 阻止 mount 系统调用。SYS_ADMIN 能力是执行 mount 的前提。
**How to avoid:** `--device /dev/fuse` 和 `--cap-add SYS_ADMIN` 必须同时使用。
**Warning signs:** 容器内执行任何 FUSE 挂载操作报 `Operation not permitted`。

### Pitfall 4: fuse.conf 中 user_allow_other 未启用
**What goes wrong:** sshfs 挂载可以成功，但使用 `-o allow_other` 时报错。
**Why it happens:** fuse3 安装后 `/etc/fuse.conf` 中 `user_allow_other` 默认是注释状态。
**How to avoid:** Dockerfile 中必须 `sed -i 's/^#user_allow_other/user_allow_other/'` 取消注释。
**Warning signs:** 容器内 `grep user_allow_other /etc/fuse.conf` 显示行首有 `#`。

### Pitfall 5: AppArmor 阻止 FUSE 挂载（生产环境）
**What goes wrong:** 开发环境正常但生产 Linux 宿主上 FUSE 挂载失败。
**Why it happens:** 某些 Linux 发行版的默认 AppArmor profile 限制容器内的 mount 操作。
**How to avoid:** 本阶段不处理——Phase 28 (SRV-04) 专门验证 FUSE + AppArmor/seccomp 兼容性。
**Warning signs:** 日志中出现 `apparmor="DENIED"` 相关条目。

## Code Examples

### Dockerfile：添加 sshfs + fuse3 到现有 apt-get install 块

```dockerfile
# Source: Ubuntu 24.04 Launchpad (sshfs 3.7.3, fuse3 3.14.0)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server \
        bash \
        ...existing packages... \
        sshfs \
        fuse3 \
    && rm -rf /var/lib/apt/lists/*
```

### Dockerfile：配置 /etc/fuse.conf

```dockerfile
# Source: libfuse 官方文档，Ubuntu fuse.conf 默认格式
RUN sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf \
    && chmod a+r /etc/fuse.conf
```

### entrypoint.sh：修复 /dev/fuse 权限

```bash
# Source: Docker for-linux issue #321, serverfault.com/questions/1058058
if [ -c /dev/fuse ]; then
  chmod 666 /dev/fuse
fi
```

### worker.go：添加 FUSE 设备和 SYS_ADMIN 能力

```go
// Source: 现有 createHost() args 切片构建模式
args := []string{
    "create",
    "--name", containerName,
    "--network", "bridge",
    "--cap-add", "NET_ADMIN",
    "--cap-add", "SYS_ADMIN",
    "--device", "/dev/fuse",
    "--label", "cloud-cli-proxy.managed=true",
    "--label", fmt.Sprintf("cloud-cli-proxy.host_id=%s", request.HostID),
    "--hostname", hostname,
    "--shm-size", "1g",
    "--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
}
```

### 验证 sshfs 可用性（手动检查命令）

```bash
# 容器内以 workspace 用户执行
whoami           # 应输出 workspace
ls -la /dev/fuse # 应显示 crw-rw-rw-
sshfs --version  # 应显示 SSHFS version 3.7.3
grep user_allow_other /etc/fuse.conf # 应显示未注释的 user_allow_other
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `-o slave` (sshfs 旧版参数名) | `-o passive` (sshfs 3.x 新参数名) | sshfs 3.x | 两者功能相同，3.7.3 同时支持，但推荐使用 passive |
| `fusermount` (fuse2) | `fusermount3` (fuse3) | fuse3 | Ubuntu 24.04 默认 fuse3，卸载命令为 fusermount3 |
| moby/moby PR #41880 去除 SYS_ADMIN 需求 | 仍未合并 | 2021-至今 | 截至 2026 年仍需要 `--cap-add SYS_ADMIN` |

**Deprecated/outdated:**
- fuse2 (`libfuse2`): Ubuntu 24.04 中 sshfs 3.7.x 依赖 fuse3，不需要安装 fuse2

## Open Questions

1. **entrypoint.sh 中 FUSE 检查的位置**
   - What we know: entrypoint.sh 以 root 身份运行，最后通过 `exec /usr/sbin/sshd -D -e` 前台运行 sshd。chmod 需要在此之前执行。
   - What's unclear: 是否需要验证 `/dev/fuse` 存在后才做 chmod（容器可能在没有 `--device /dev/fuse` 的情况下启动旧版本容器）。
   - Recommendation: 使用 `if [ -c /dev/fuse ]` 条件判断，兼容旧版本容器创建路径。已反映在代码示例中。

## Sources

### Primary (HIGH confidence)
- Ubuntu Launchpad (launchpad.net/ubuntu/noble) — 确认 sshfs 3.7.3-1.1build3 和 fuse3 3.14.0-5build1 在 Ubuntu 24.04 Noble 中可用
- libfuse/sshfs GitHub (github.com/libfuse/sshfs) — 确认 `-o passive` 和 `-o slave` 选项的官方文档
- Docker for-linux Issue #321 — 确认 `--device /dev/fuse --cap-add SYS_ADMIN` 是标准 FUSE-in-Docker 方案
- moby/moby PR #41880 — 确认截至 2026 年去除 SYS_ADMIN 需求的 PR 仍未合并
- serverfault.com/questions/1058058 — 确认 /dev/fuse 非 root 用户 Permission Denied 问题和解决方案

### Secondary (MEDIUM confidence)
- unix.stackexchange.com/questions/37168 — 确认 /etc/fuse.conf 需要 `chmod a+r` 才能被非 root 用户读取

### Codebase (HIGH confidence — 直接代码审查)
- `internal/sshproxy/proxy.go` — handleConnection() 第 203 行循环 + handleChannel() 第 261-283 行全类型转发
- `internal/runtime/tasks/worker.go` — createHost() 第 153-163 行 args 切片构建模式
- `deploy/docker/managed-user/Dockerfile` — 第 9-39 行现有 apt-get install 块
- `deploy/docker/managed-user/entrypoint.sh` — 第 60-178 行初始化流程

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Ubuntu 官方仓库版本确认，apt 包名明确
- Architecture: HIGH - 现有代码模式清晰，修改方案直接
- Pitfalls: HIGH - 多个独立来源交叉验证的已知问题
- SSH Proxy 验证: HIGH - 直接代码审查，无推测成分

**Research date:** 2026-04-15
**Valid until:** 2026-05-15（稳定领域，30 天有效期）
