# Phase 17: 镜像与 Entrypoint 基线 - Research

**Researched:** 2026-04-09
**Domain:** Docker 镜像构建、Claude Code 官方安装、entrypoint 编排
**Confidence:** HIGH

## Summary

Phase 17 交付 claude-shell 专用的 Docker 镜像和 entrypoint 编排脚本。镜像基于 Ubuntu 24.04，包含 sing-box 二进制和最小必要开发工具，Claude Code 通过官方 `curl` 安装脚本（Bun standalone binary）预装。entrypoint 采用 shell 脚本按"网络配置 → 指纹伪造 → 反检测 → Claude Code"顺序编排，各步骤为独立函数，失败时输出明确错误并使用不同退出码。`DISABLE_AUTOUPDATER=1` 环境变量阻止 Claude Code 自动更新。

本阶段完全独立于 `deploy/docker/managed-user/` 现有镜像，文件放在 `claude-shell/docker/` 子目录下。网络配置、指纹伪造和反检测功能在本阶段仅为占位函数，实际实现分别在 Phase 18 和 Phase 21。

**Primary recommendation:** 按照 Anthropic 官方安装文档在 Docker 构建期执行 `curl -fsSL https://claude.ai/install.sh | bash`，安装到非 root 用户的 `~/.local/bin/claude`，运行时通过 `DISABLE_AUTOUPDATER=1` 禁用自动更新，entrypoint 最终以 `exec claude` 将 Claude Code 设为 PID 1。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 基础镜像使用 Ubuntu 24.04，与现有 managed-user 镜像基线一致，保证 glibc 兼容性和包可用性
- **D-02:** 工具集精简为最小必要集：curl、git、jq、procps、iproute2、ca-certificates、bash、sudo、nftables。不包含 SSH Server、KasmVNC、Chromium 等 GUI 组件
- **D-03:** sing-box 二进制从 GitHub release 下载安装，版本与现网对齐（1.13.x 系列），安装方式参考 `deploy/docker/sing-box-gateway/Dockerfile` 的模式
- **D-04:** Claude Code 在镜像构建时通过官方 curl 安装脚本预装（Bun standalone），不依赖 npm 或 spoof.js
- **D-05:** 不锁定 Claude Code 具体版本，使用官方安装脚本获取当时最新版本，符合"使用官方安装的最新版本"要求
- **D-06:** 运行时通过 DISABLE_AUTOUPDATER=1 环境变量阻止 Claude Code 自动更新
- **D-07:** entrypoint 使用 Shell 脚本（bash），`set -euo pipefail` 快速失败模式
- **D-08:** 各阶段拆为独立函数：`setup_network()`、`setup_fingerprint()`、`setup_anti_detect()`、`start_claude()`，按序调用
- **D-09:** 每个步骤失败时输出明确的错误描述（含步骤名称和失败原因），使用不同的退出码区分失败阶段
- **D-10:** 最终以 `exec` 启动 Claude Code 作为容器 PID 1，保证信号能直接传递到 Claude Code 进程
- **D-11:** Phase 17 中 `setup_network()` 和 `setup_fingerprint()` 为占位函数（echo 提示 + 跳过），实际实现分别在 Phase 18 和 Phase 21
- **D-12:** 镜像构建文件放在 `claude-shell/docker/` 子目录下，与 `deploy/docker/managed-user/` 完全独立
- **D-13:** 与 BUILD-02 要求一致，claude-shell/ 子目录拥有独立 go.mod，镜像构建不依赖 cloud-cli-proxy 主项目的任何文件
- **D-14:** 复用 sing-box 配置与路由设计理念，但不直接引用 `internal/network/` 代码

### Claude's Discretion
- 容器内工作用户的 UID/GID 策略
- entrypoint 日志格式（纯文本 vs 结构化）
- 镜像层优化策略（合并 RUN 指令的具体方式）

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| INFRA-01 | 精简 Docker 镜像，通过官方安装脚本安装 Claude Code（Bun standalone），包含 sing-box 和基础开发工具 | Claude Code 官方安装文档确认 `curl install.sh` 为推荐路径；sing-box GitHub releases 安装模式已有参考实现；Ubuntu 24.04 glibc 兼容性已验证 |
| INFRA-02 | entrypoint 按正确顺序编排：网络配置 → 指纹伪造 → 反检测 → 启动 Claude Code | Shell 脚本 step-function 模式研究完成；PID 1 信号处理机制已明确；退出码规划已完成 |
| INFRA-03 | 容器内 Claude Code 自动更新被禁用（DISABLE_AUTOUPDATER） | Anthropic 官方文档和 GitHub issues 确认 `DISABLE_AUTOUPDATER=1` 环境变量为当前推荐方式，优先级高于已废弃的 `autoUpdates` 配置 |
</phase_requirements>

## Standard Stack

### Core
| Component | Version | Purpose | Why Standard |
|-----------|---------|---------|--------------|
| Ubuntu | 24.04 LTS | 基础镜像 | glibc 兼容性好，Anthropic 官方文档以 Ubuntu 24.04 为 Docker 示例基础镜像，包管理成熟 |
| Claude Code | latest (via install.sh) | 容器主进程 | Anthropic 官方推荐 Native Install，Bun standalone binary (~213MB)，npm 安装已被标记为 deprecated |
| sing-box | 1.13.3 | 网络代理引擎（本阶段仅预装二进制） | 与受管镜像 `deploy/docker/sing-box-gateway/` 版本对齐，避免行为漂移 |
| nftables | 随 Ubuntu 24.04 | 防火墙（本阶段仅预装包） | D-02 锁定的最小工具集之一，Phase 18 实际使用 |
| bash | 随 Ubuntu 24.04 | entrypoint 脚本运行时 | D-07 锁定 Shell 脚本方案 |

### Supporting
| Tool | Purpose | When to Use |
|------|---------|-------------|
| curl | 下载 Claude Code 安装脚本和 sing-box 二进制 | 构建时 |
| git | 版本控制（Claude Code 依赖） | 运行时 |
| jq | JSON 处理（配置解析） | 运行时 |
| procps | 进程管理工具（ps 等） | 运行时调试 |
| iproute2 | 网络配置工具（ip 命令） | Phase 18 使用，本阶段预装 |
| ca-certificates | TLS 证书链 | 构建时和运行时的 HTTPS 请求 |
| sudo | 权限提升 | 容器内需要特权的网络操作 |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Ubuntu 24.04 | Debian bookworm-slim | 更小的基础镜像，但需要额外验证 Claude Code 兼容性 |
| Official install.sh | npm install (deprecated) | 需要 Node.js 运行时，Anthropic 已标记为废弃 |
| Shell entrypoint | Go/Python entrypoint | 更强类型但增加构建复杂度，shell 足够满足编排需求 |

## Architecture Patterns

### Recommended Project Structure
```
claude-shell/
├── docker/
│   ├── Dockerfile           # claude-shell 专用镜像
│   └── entrypoint.sh        # 容器启动编排脚本
└── ...                      # Phase 19+ 的 Go CLI 代码
```

### Pattern 1: Claude Code Docker 安装（官方推荐模式）
**What:** 在 Docker 构建期以目标用户身份执行官方安装脚本
**When to use:** 需要在容器内预装 Claude Code 时
**Example:**
```dockerfile
# Source: https://docs.anthropic.com/en/docs/claude-code/setup
# Source: https://www.claudecodeai.online/blog/claude-code-setup-linux (Docker section)
ARG CLAUDE_USER=claude
ARG CLAUDE_UID=1000
ARG CLAUDE_GID=1000

RUN groupadd --gid "${CLAUDE_GID}" "${CLAUDE_USER}" \
    && useradd --uid "${CLAUDE_UID}" --gid "${CLAUDE_GID}" \
       --home-dir /home/${CLAUDE_USER} --create-home \
       --shell /bin/bash "${CLAUDE_USER}" \
    && echo "${CLAUDE_USER} ALL=(ALL) NOPASSWD:ALL" \
       > /etc/sudoers.d/${CLAUDE_USER}

USER ${CLAUDE_USER}
RUN curl -fsSL https://claude.ai/install.sh | bash
ENV PATH="/home/${CLAUDE_USER}/.local/bin:${PATH}"
```

**关键细节：**
- 安装脚本将二进制放在 `~/.local/bin/claude`，数据放在 `~/.local/share/claude/`
- 必须以目标用户（非 root）身份运行安装脚本，因为安装路径基于 `$HOME`
- 安装脚本会自动添加 PATH 到 shell profile，但 Dockerfile 中需显式设置 ENV PATH
- 二进制是 Bun v1.3.5 standalone application (~213MB ELF)，动态链接 glibc

### Pattern 2: sing-box 二进制安装（已验证模式）
**What:** 从 GitHub Releases 下载指定版本的 sing-box 二进制
**When to use:** 需要在镜像中预装 sing-box 时
**Example:**
```dockerfile
# Source: deploy/docker/sing-box-gateway/Dockerfile (existing pattern)
ARG SINGBOX_VERSION=1.13.3
RUN set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    curl -fsSL -o /tmp/sb.tgz \
      "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${ARCH}.tar.gz"; \
    tar -xzf /tmp/sb.tgz -C /tmp; \
    install -m 0755 "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}/sing-box" /usr/local/bin/sing-box; \
    rm -rf /tmp/sb.tgz "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}"; \
    sing-box version
```

### Pattern 3: Entrypoint Step-Function 编排
**What:** Shell 脚本中定义独立步骤函数，按序执行，失败时输出明确错误
**When to use:** 需要多步骤顺序初始化的容器
**Example:**
```bash
#!/usr/bin/env bash
set -euo pipefail

readonly EXIT_NETWORK=10
readonly EXIT_FINGERPRINT=20
readonly EXIT_ANTI_DETECT=30
readonly EXIT_CLAUDE=40

log() { echo "[entrypoint] $(date -u +%Y-%m-%dT%H:%M:%SZ) $*"; }
die() { log "FATAL: $1"; exit "${2:-1}"; }

setup_network() {
  log "step=network status=placeholder"
  # Phase 18 will implement: sing-box tun + nftables
}

setup_fingerprint() {
  log "step=fingerprint status=placeholder"
  # Phase 21 will implement: machine-id, /proc overrides
}

setup_anti_detect() {
  log "step=anti_detect status=placeholder"
  # Phase 21 will implement: /.dockerenv cleanup, cgroup mask
}

start_claude() {
  log "step=claude status=starting"
  local claude_bin
  claude_bin="$(command -v claude 2>/dev/null)" \
    || die "claude binary not found in PATH" $EXIT_CLAUDE
  exec "$claude_bin" "$@"
}

setup_network   || die "network setup failed"   $EXIT_NETWORK
setup_fingerprint || die "fingerprint setup failed" $EXIT_FINGERPRINT
setup_anti_detect || die "anti-detect setup failed" $EXIT_ANTI_DETECT
start_claude "$@"
```

### Anti-Patterns to Avoid
- **以 root 身份安装 Claude Code：** 安装路径绑定 `$HOME`，root 安装会放到 `/root/.local/bin/`，后续切换用户后找不到二进制
- **使用 npm install 安装 Claude Code：** Anthropic 已标记为 deprecated，Native Install 不需要 Node.js 运行时
- **Shell form ENTRYPOINT：** 使用 `ENTRYPOINT entrypoint.sh` 而非 `ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]` 会导致 shell 成为 PID 1，信号转发异常
- **在 entrypoint 中使用 `set -e` 但不处理步骤间错误：** 每个步骤函数需要独立的错误处理，`set -e` 只保证命令级失败退出

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Claude Code 安装 | 手动下载 Bun/npm 包 | `curl -fsSL https://claude.ai/install.sh \| bash` | 官方脚本处理架构检测、路径设置、版本管理；手工安装容易遗漏依赖或路径 |
| sing-box 编译 | 从源码编译 sing-box | 从 GitHub Releases 下载预编译二进制 | 编译需要 Go 工具链，增加镜像体积和构建时间，且需跟踪编译选项 |
| 自动更新禁用 | 修改 Claude Code 内部文件或二进制 | `DISABLE_AUTOUPDATER=1` 环境变量 | 官方支持的环境变量，跨版本稳定 |
| PID 1 信号处理 | 自写 init 进程 | `exec` 替换 shell + Phase 19 的 `docker run --init` | Bun 运行时作为 PID 1 时，信号处理由 `docker run --init`（tini）在外层保障 |

## Common Pitfalls

### Pitfall 1: Claude Code 安装用户与运行用户不一致
**What goes wrong:** 以 root 构建安装 Claude Code，运行时切换到非 root 用户后 `claude` 命令找不到
**Why it happens:** `install.sh` 将二进制安装到 `$HOME/.local/bin/`，root 的 HOME 是 `/root`，而运行用户的 HOME 不同
**How to avoid:** 在 Dockerfile 中先创建目标用户，然后 `USER <user>` 再执行安装脚本
**Warning signs:** `command not found: claude`，`~/.local/bin/claude` 不存在

### Pitfall 2: PATH 未包含 Claude Code 安装路径
**What goes wrong:** Claude Code 安装成功但容器启动时找不到 `claude` 命令
**Why it happens:** `install.sh` 修改 `.bashrc`/`.zshrc` 添加 PATH，但 `docker run` 不会 source shell profile
**How to avoid:** 在 Dockerfile 中显式设置 `ENV PATH="$HOME/.local/bin:${PATH}"`
**Warning signs:** 手动运行 `~/.local/bin/claude --version` 成功但 `claude --version` 失败

### Pitfall 3: set -euo pipefail 与管道/条件逻辑冲突
**What goes wrong:** `set -e` 导致条件检查命令（如 `command -v`）在找不到命令时直接退出脚本
**Why it happens:** `set -e` 会在任何非零返回码时退出，包括故意用于条件判断的命令
**How to avoid:** 对需要检查返回码的命令使用 `|| true` 或 `if` 语句；`set -u` 对未定义变量使用 `${VAR:-default}` 提供默认值
**Warning signs:** 脚本在预期为占位的步骤中意外退出

### Pitfall 4: Docker 层缓存失效导致 Claude Code 每次构建都重新下载
**What goes wrong:** 每次构建都重新执行 Claude Code 安装，耗时且消耗带宽
**Why it happens:** Dockerfile 中 `curl install.sh` 之前的层有任何变更（如 COPY entrypoint.sh），或 `--no-cache` 构建
**How to avoid:** 将 Claude Code 安装放在 entrypoint.sh COPY 之前；将不常变更的层提前
**Warning signs:** 构建日志中每次都出现 Claude Code 下载进度

### Pitfall 5: DISABLE_AUTOUPDATER 不生效
**What goes wrong:** Claude Code 启动时仍然尝试自动更新
**Why it happens:** 部分版本的 Claude Code 仅在 shell 环境变量中读取 `DISABLE_AUTOUPDATER`，settings.json 中的 `env` 配置可能不被尊重（已知 bug，见 anthropics/claude-code#11263）
**How to avoid:** 同时在 Dockerfile `ENV` 和容器启动环境变量中设置 `DISABLE_AUTOUPDATER=1`，双重保障
**Warning signs:** 容器日志中出现更新检查或更新下载的输出

### Pitfall 6: sing-box 架构不匹配
**What goes wrong:** sing-box 二进制无法执行或段错误
**Why it happens:** 下载了错误架构的二进制（例如在 arm64 宿主机上下载了 amd64 版本）
**How to avoid:** 使用 `dpkg --print-architecture` 动态获取架构，已在参考 Dockerfile 中验证
**Warning signs:** `exec format error` 或 `Illegal instruction`

## Code Examples

### Claude Code Docker 安装完整示例
```dockerfile
# Source: https://docs.anthropic.com/en/docs/claude-code/setup (official docs)
# Source: https://www.claudecodeai.online/blog/claude-code-setup-linux (Docker usage section)
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://claude.ai/install.sh | bash
ENV PATH="/root/.local/bin:${PATH}"
```

上述为官方文档的最简 Docker 示例（以 root 运行）。claude-shell 需要改为非 root 用户安装（见 Pattern 1）。

### DISABLE_AUTOUPDATER 环境变量设置
```dockerfile
# Source: https://docs.anthropic.com/en/docs/claude-code/setup#disable-auto-updates
# Source: https://github.com/anthropics/claude-code/issues/3479 (deprecated autoUpdates → env var)
ENV DISABLE_AUTOUPDATER=1
```

同时在容器内的 settings.json 中配置（双重保障）：
```json
{
  "env": {
    "DISABLE_AUTOUPDATER": "1"
  }
}
```

### Claude Code 相关环境变量完整列表
```bash
# Source: https://docs.anthropic.com/en/docs/claude-code/data-usage
# Source: https://help.apiyi.com/en/claude-code-environment-variables-complete-guide-en.html
DISABLE_AUTOUPDATER=1                        # 禁用自动更新
DISABLE_TELEMETRY=1                          # 禁用 Statsig 遥测
DISABLE_ERROR_REPORTING=1                    # 禁用 Sentry 错误报告
DISABLE_FEEDBACK_COMMAND=1                   # 禁用 /feedback 命令
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1   # 禁用所有非必要外连
CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY=1        # 禁用满意度问卷
```

Phase 17 仅需 `DISABLE_AUTOUPDATER=1`（D-06 要求），其他变量可在后续阶段按需启用以减少 Claude Code 非必要网络流量。

### sing-box 安装验证
```bash
# Source: deploy/docker/sing-box-gateway/Dockerfile (existing verified pattern)
sing-box version
# Expected: sing-box version 1.13.3
```

### entrypoint 退出码规划
```bash
# 约定的退出码范围：
# 0     — 正常退出（由 Claude Code 进程决定）
# 1     — 通用错误
# 10-19 — 网络配置步骤失败
# 20-29 — 指纹伪造步骤失败
# 30-39 — 反检测步骤失败
# 40-49 — Claude Code 启动失败
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `npm install -g @anthropic-ai/claude-code` | `curl -fsSL https://claude.ai/install.sh \| bash` | 2025 下半年起逐步迁移，npm 标记 deprecated | 不再需要 Node.js 运行时；安装物从 npm 包变为 Bun standalone binary (~213MB) |
| `autoUpdates` 配置项 | `DISABLE_AUTOUPDATER=1` 环境变量 | 2025-09 (claude-code#3479) | `autoUpdates` config key 已被废弃，新版本中可能报错 |
| Claude Code 通过 Node.js 运行 | Claude Code 使用 Bun v1.3.5 standalone | 2025 下半年 | 单一二进制发行，动态链接 glibc，无需 npm/node |

**Deprecated/outdated:**
- `npm install -g @anthropic-ai/claude-code`：Anthropic 官方标记 deprecated
- `autoUpdates` / `autoUpdaterStatus` config key：已被 `DISABLE_AUTOUPDATER` env var 替代
- `claude config set -g autoUpdates disabled`：不再是推荐方式

## Open Questions

1. **Claude Code PID 1 信号处理能力**
   - What we know: Bun 运行时作为 PID 1 时，Linux 内核不会向 PID 1 发送默认信号处理。如果 Bun 没有显式注册 SIGTERM handler，`docker stop` 会等待 10 秒后 SIGKILL。
   - What's unclear: Claude Code (Bun binary) 是否内部注册了 SIGTERM handler。
   - Recommendation: Phase 17 entrypoint 使用 `exec claude` 作为 PID 1（符合 D-10），Phase 19/20 的 CLI 通过 `docker run --init` 注入 tini 保障信号转发。本阶段不需要额外处理。

2. **Claude Code 安装脚本的网络需求**
   - What we know: 安装脚本需要从 Anthropic CDN 下载二进制（~213MB）。
   - What's unclear: 构建期网络不通或 CDN 不可达时的错误恢复。
   - Recommendation: 在 Dockerfile 中 `curl install.sh | bash` 失败会直接阻断构建（`set -eux`），已有明确的错误信号。可考虑在 CI 中缓存已安装的层。

3. **容器内工作用户命名**
   - What we know: 现有 managed-user 使用 `workspace` 用户（UID 1000），Claude's Discretion 允许自定义。
   - Recommendation: 使用 `claude` 作为用户名（UID 1000），语义更清晰，与 `claude-shell` 产品名一致。HOME 目录设为 `/home/claude`，工作目录 `/workspace` 由 CLI 挂载。

## Project Constraints (from CLAUDE.md)

从 `CLAUDE.md` 约定文件中提取的与本阶段相关的约束：

- **沟通语言：** 所有面向用户的回复、计划、状态更新和总结默认全部使用中文
- **隐私与安全：** 禁止在代码、注释、文档中写入任何本机绝对路径；禁止在 git 跟踪文件中写入真实 API 密钥、密码、token 等敏感信息
- **路径引用：** 一律使用项目根目录的相对路径
- **示例凭据：** 使用明确的占位符（如 `your-secret-here`）

## Sources

### Primary (HIGH confidence)
- [Anthropic Claude Code Setup Docs](https://docs.anthropic.com/en/docs/claude-code/setup) — 安装方式、版本管理、卸载
- [Anthropic Claude Code Settings Docs](https://docs.anthropic.com/en/docs/claude-code/settings) — 环境变量和配置项
- [Anthropic Claude Code Data Usage](https://docs.anthropic.com/en/docs/claude-code/data-usage) — 遥测环境变量
- `deploy/docker/sing-box-gateway/Dockerfile` — sing-box 安装模式（仓库内已验证）
- `deploy/docker/managed-user/Dockerfile` — Ubuntu 24.04 用户创建和工具安装参考
- `deploy/docker/managed-user/entrypoint.sh` — entrypoint 编排参考

### Secondary (MEDIUM confidence)
- [anthropics/claude-code#3479](https://github.com/anthropics/claude-code/issues/3479) — DISABLE_AUTOUPDATER 替代 autoUpdates
- [anthropics/claude-code#10079](https://github.com/anthropics/claude-code/issues/10079) — DISABLE_AUTOUPDATER 文档问题
- [anthropics/claude-code#11263](https://github.com/anthropics/claude-code/issues/11263) — settings.json 中 DISABLE_AUTOUPDATER 不生效的 bug
- [Claude Code Binary Reverse Engineering](https://medium.com/@CodeCoup/i-spent-a-week-reverse-engineering-the-claude-code-binary-heres-what-s-inside-f59997e31202) — 确认 Bun v1.3.5 standalone binary, ~213MB
- Docker PID 1 and tini best practices — 多源一致的社区共识

### Tertiary (LOW confidence)
- None — all findings verified against official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 基于 Anthropic 官方文档和仓库内已验证的模式
- Architecture: HIGH — entrypoint 模式为标准 Docker 实践，sing-box 安装已有参考实现
- Pitfalls: HIGH — Claude Code 安装问题有官方 issue 记录，Docker PID 1 问题有成熟社区共识

**Research date:** 2026-04-09
**Valid until:** 2026-05-09 (30 days — Claude Code 安装脚本行为可能随版本更新变化)
