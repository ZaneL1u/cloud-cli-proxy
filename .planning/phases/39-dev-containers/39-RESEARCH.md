# Phase 39: 本地 Dev Containers 支持 - Research

**Researched:** 2026-05-07
**Domain:** Go CLI / Docker Container Lifecycle / Dev Containers / sing-box 本地适配
**Confidence:** HIGH

## Summary

Phase 39 需要实现 `cloud-claude local` 子命令组，让用户在本地机器上一键启动 managed-user 容器。核心工作分为三块：(1) CLI 子命令扩展（cobra `local up/down/status`）；(2) entrypoint.sh 的 `MODE=local` 分支改造；(3) 可选的 sing-box egress 注入。

代码库已有完整的 CLI 骨架（cobra）、Docker 容器操作模式（`exec.CommandContext("docker", ...)` 而非 Go Docker SDK）、managed-user 镜像和 entrypoint 脚本、sing-box 配置生成逻辑、以及 `.devcontainer/devcontainer.json` 模板。Phase 39 的工作是在这些既有模式上做增量扩展，不引入新依赖或架构变更。

**Primary recommendation:** 复用 worker.go 的 `runDocker` + `buildCreateArgs` 模式，将容器操作逻辑提取为可复用的 `internal/local` 包；entrypoint.sh 前段增加 `MODE=local` 条件分支跳过 KasmVNC 桌面栈。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- `cloud-claude local` 作为子命令组，采用 cobra 模式（与现有 init/env/ssh 一致）
- `local` 本身默认行为 = `local up`（启动容器）
- `local down` 停止并移除容器
- `local status` 显示容器运行状态、端口映射
- 容器使用固定标签 `cloud-claude-local` 便于 `down`/`status` 识别
- 入口 `MODE` 环境变量控制行为：`MODE=remote`（默认，现有行为）vs `MODE=local`
- `MODE=local` 跳过：KasmVNC 配置/启动、Xvnc/fluxbox/pcmanfm/Chromium 桌面栈、prepare_v3_dirs 等远程专有阶段
- `MODE=local` 保留：sshd 启动、sing-box 启动（如有 egress 配置）、用户密码设置、SSH keygen
- KasmVNC 跳过逻辑放在 entrypoint 前段，用 `if [ "$MODE" != "local" ]; then` 包裹整个桌面栈
- 直接调用本地 Docker API（Go docker client SDK），不连接 control-plane
- 自动检测并拉取/使用 managed-user 镜像（复用现有镜像名约定）
- SSH 端口随机分配或通过 `--port` flag 指定，publish 到宿主机 127.0.0.1
- 启动完成后输出连接信息：host, port, user, password
- `cloud-claude local --egress-config <file>` 接受 sing-box outbound JSON 文件路径
- 文件通过 docker cp 或 bind mount 注入到容器内固定路径 `/etc/cloud-claude/sing-box-outbound.json`
- 容器 entrypoint 检测到该文件时自动启动 sing-box tun 模式
- 未提供 `--egress-config` 时容器不启动 sing-box（纯本地开发场景，无隧道开销）
- macOS 宿主机无 root 权限做 tun 设备，使用 SOCKS/HTTP 代理模式
- `--egress-config` 文件中若指定 socks/http 出站协议，entrypoint 自动切换为代理模式而非 tun
- 容器内设置 `ALL_PROXY` / `HTTP_PROXY` 环境变量指向本地代理端口
- 已有 `.devcontainer/devcontainer.json` 模板可复用
- 传入 `MODE=local` 环境变量到容器（通过 `containerEnv` 或 `runArgs --env`）
- SSH 端口通过 `forwardPorts` 暴露给 VS Code Remote-SSH

### Claude's Discretion

- 容器命名和标签策略（`cloud-claude-local` + 项目路径哈希区分多项目）
- `local status` 输出格式（table 或 key-value）
- SSH 密码自动生成方式（随机密码 vs 固定默认密码）
- 容器资源限制（CPU/memory 默认值）
- 错误提示文案和连接信息输出格式

### Deferred Ideas (OUT OF SCOPE)

- LOCAL-05: `--sync-config` 从云端拉取 egress IP 配置 — 后续版本
- LOCAL-06: 本地容器预热镜像 — 后续版本
- LOCAL-07: Windows Docker Desktop 支持验证 — 后续版本
- UX-03: doctor 本地模式适配 — Phase 41 Doctor 扩展
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| LOCAL-01 | `cloud-claude local` 子命令支持一键启动本地容器 | CLI 结构：cobra 子命令注册模式已验证（main.go line 93）；Docker 操作：复用 worker.go 的 runDocker + buildCreateArgs 模式；镜像管理：image.lock 已有 `local_dev_image_name` 字段 |
| LOCAL-02 | 本地容器支持 Dev Containers 配置 | `.devcontainer/devcontainer.json` 已存在且引用 managed-user 镜像；需微调加入 `MODE=local` 环境变量和 `forwardPorts` |
| LOCAL-03 | 本地容器支持 sing-box 全隧道（可选配置） | `buildSingBoxConfig()` 已实现完整配置生成（singbox_config.go）；`outbound_parse.go` 已有 JSON 解析逻辑；macOS 需 SOCKS/HTTP 代理模式替代 tun |
| LOCAL-04 | entrypoint 支持 `MODE=local` 分支 | entrypoint.sh 已有清晰的 KasmVNC + 桌面栈 + v3 stages 结构；用条件分支包裹桌面栈即可；sing-box 启动需独立逻辑 |
| UX-02 | `cloud-claude local` 支持 `down` / `status` 子命令 | 复用 `docker stop/rm` 和 `docker inspect` 模式（worker.go line 1096）；容器标签 `cloud-claude-local` 用于过滤 |
</phase_requirements>

## Standard Stack

### Core

| Library/Tool | Version | Purpose | Why Standard |
|-------------|---------|---------|-------------|
| cobra | v1.9.x (已在 go.mod) | CLI 子命令框架 | 项目已使用，local 子命令复用同一模式 |
| Docker CLI | 28.x (宿主机) | 容器操作 | 项目全量使用 `exec.Command("docker", ...)` 而非 Go Docker SDK |
| sing-box | 协议稳定 | 隧道出网 | 项目已集成，配置生成在 internal/network/ |

### Supporting

| Library/Tool | Version | Purpose | When to Use |
|-------------|---------|---------|-------------|
| crypto/rand | Go stdlib | SSH 密码自动生成 | Claude's Discretion：本地容器密码 |
| encoding/json | Go stdlib | egress config 文件解析 | --egress-config 文件读取 |

**No new dependencies needed.** Phase 39 全部用现有项目内模式和 Go 标准库。

## Architecture Patterns

### Recommended Package Structure

```
internal/
├── local/
│   ├── local.go          # LocalManager：容器生命周期管理（up/down/status）
│   ├── local_test.go     # 单元测试
│   ├── container.go      # Docker 容器操作封装（create/start/stop/remove/inspect）
│   ├── egress.go         # egress config 注入和 sing-box 启动检测
│   └── password.go       # 本地密码生成
cmd/cloud-claude/
├── local.go              # cobra local 子命令组定义（up/down/status）
├── local_test.go         # CLI 级测试（可选）
deploy/docker/managed-user/
├── entrypoint.sh         # MODE=local 分支改造
.devcontainer/
├── devcontainer.json     # 更新支持 MODE=local
```

### Pattern 1: CLI 子命令注册（cobra）

与现有 `envCmd`/`sshCmd` 一致的嵌套子命令模式：

```go
// Source: cmd/cloud-claude/main.go line 53-67
localCmd := &cobra.Command{
    Use:   "local",
    Short: "本地容器管理",
}
localUpCmd := &cobra.Command{
    Use:   "up",
    Short: "启动本地容器",
    RunE:  runLocalUp,
}
localCmd.AddCommand(localUpCmd)
rootCmd.AddCommand(localCmd)
```

同时需要在 `DisableFlagParsing` switch 中添加 `"local"` 识别（line 99）。

### Pattern 2: Docker 操作封装（CLI 模式）

复用 worker.go 的 `runDocker` 模式：

```go
// Source: internal/runtime/tasks/worker.go line 1107
func runDocker(ctx context.Context, args ...string) error {
    cmd := exec.CommandContext(ctx, "docker", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
    }
    return nil
}
```

**关键决策（Claude's Discretion）：** 不使用 Go Docker SDK（`github.com/docker/docker/client`），因为项目 100% 使用 CLI 模式。保持一致。

### Pattern 3: Entrypoint MODE 分支

```bash
# entrypoint.sh 前段添加
MODE="${MODE:-remote}"

# 公共阶段（MODE=local 和 MODE=remote 都需要）
# - SSH setup（sshd host keys）
# - CONTAINER_USER/CONTAINER_PASSWORD 设置
# - SSH keygen
# - passwd 设置 + 验证
# - IPv6 禁用
# - /dev/fuse 设置

if [ "$MODE" != "local" ]; then
  # 远程专有阶段
  # - KasmVNC 配置 + 启动
  # - 桌面栈（fluxbox, pcmanfm, chromium）
  # - v3 stages（prepare_v3_dirs, prepare_persistent_state, prepare_container_disguise, prepare_mergerfs_check, assert_tmux_version）
fi

# 本地模式：可选 sing-box 启动
if [ "$MODE" = "local" ] && [ -f /etc/cloud-claude/sing-box-outbound.json ]; then
  # 检测协议类型，选择 tun 或 proxy 模式
  # 启动 sing-box
fi

# 公共结尾：exec sshd -D
exec /usr/sbin/sshd -D -e
```

### Anti-Patterns to Avoid

- **不要引入 Go Docker SDK**：项目全量使用 CLI，引入 SDK 会增加依赖且与 worker 模式不一致
- **不要复用 RuntimeSpec/Service**：这些是控制面专用（依赖 postgres），local 模式无 DB
- **不要硬编码端口**：使用 `0:22`（Docker 自动分配）+ `docker inspect` 提取实际端口

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 容器端口查询 | 自己解析 Docker API | `docker inspect --format` | 已有 worker.go 模式 |
| 随机密码生成 | Math/rand | `crypto/rand` + `encoding/hex` | 安全性 |
| JSON 解析 | 自定义解析器 | `encoding/json` + 现有 `outbound_parse.go` | 已验证 |

## Common Pitfalls

### Pitfall 1: Docker Desktop 端口分配
**What goes wrong:** `0:22` 端口映射在 Docker Desktop 下需要通过 `docker inspect` 获取实际 host 端口
**Why it happens:** Docker Desktop 使用 vpnkit 做端口转发，与 Linux bridge 模式不同
**How to avoid:** 启动后用 `docker inspect --format='{{(index (index .NetworkSettings.Ports "22/tcp") 0).HostPort}}'` 获取实际端口
**Warning signs:** SSH 连接被拒绝

### Pitfall 2: macOS tun 不可用
**What goes wrong:** sing-box tun 模式在 macOS Docker Desktop 内需要 root + /dev/net/tun
**Why it happens:** Docker Desktop VM 内的 Linux 有 tun 支持，但容器默认不 privileged
**How to avoid:** 对 socks/http 协议使用代理模式（设置 ALL_PROXY），不走 tun；仅 tun 协议需要 `--cap-add NET_ADMIN --device /dev/net/tun`
**Warning signs:** sing-box 启动失败，权限错误

### Pitfall 3: 容器命名冲突
**What goes wrong:** 多项目同时运行 `local up`，容器名冲突
**Why it happens:** 固定容器名 `cloud-claude-local`
**How to avoid:** 容器名 = `cloud-claude-local-{project_path_hash}`，用项目路径 MD5 前 8 位区分
**Warning signs:** docker create 报 name already in use

### Pitfall 4: entrypoint.sh MODE 分支位置
**What goes wrong:** 把 MODE 检查放在 SSH setup 之后太远，导致 KasmVNC 函数被定义但不执行，bash 变量残留
**Why it happens:** entrypoint.sh 顶部定义了函数（write_desktop_config 等），KasmVNC 配置在函数调用之前
**How to avoid:** MODE 检查放在 SSH 公共阶段之后、KasmVNC yaml 写入之前；函数定义保留不动（bash 函数定义不执行）
**Warning signs:** 本地容器仍启动 Xvnc

## Code Examples

### 容器 inspect 获取端口

```go
// Source: internal/runtime/tasks/worker.go line 1096 (adapted)
func inspectContainerPort(ctx context.Context, containerName string) (string, error) {
    cmd := exec.CommandContext(ctx, "docker", "inspect",
        "--format={{(index (index .NetworkSettings.Ports \"22/tcp\") 0).HostPort}}",
        containerName)
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("inspect port: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}
```

### 随机密码生成

```go
import (
    "crypto/rand"
    "encoding/hex"
)

func generatePassword(length int) (string, error) {
    b := make([]byte, length)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return hex.EncodeToString(b)[:length], nil
}
```

### 容器名生成（含项目路径哈希）

```go
import (
    "crypto/md5"
    "fmt"
    "path/filepath"
)

func localContainerName(projectDir string) string {
    abs, _ := filepath.Abs(projectDir)
    hash := md5.Sum([]byte(abs))
    return fmt.Sprintf("cloud-claude-local-%x", hash[:4])
}
```

### sing-box 代理模式检测

```go
// Source: internal/network/outbound_parse.go (adapted)
func detectSingBoxMode(outboundJSON []byte) (string, error) {
    var config struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(outboundJSON, &config); err != nil {
        return "", err
    }
    switch config.Type {
    case "socks", "http":
        return "proxy", nil
    default:
        return "tun", nil
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 无本地模式 | `cloud-claude local` 子命令 | Phase 39 (this) | 新功能 |
| entrypoint 仅 remote | MODE 分支 | Phase 39 (this) | 同一镜像 dual-use |
| 无 sing-box 本地模式 | --egress-config 注入 | Phase 39 (this) | 可选隧道 |

## Open Questions

1. **Docker Desktop `/dev/fuse` 映射**
   - What we know: macOS Docker Desktop 支持 `--device /dev/fuse`，但需要 Docker Desktop 设置中启用
   - What's unclear: 不同 Docker Desktop 版本的 fuse 支持一致性
   - Recommendation: devcontainer.json 保留 `--device /dev/fuse`，启动失败时给出提示但不阻塞

2. **sing-box 容器内安装**
   - What we know: 当前 managed-user 镜像的 Dockerfile 中未安装 sing-box
   - What's unclear: sing-box 是通过宿主机网络 namespace 注入还是容器内独立安装
   - Recommendation: Phase 39 先假设 sing-box 二进制已在镜像中（或通过 bind mount 注入），后续补 Dockerfile 层

## Sources

### Primary (HIGH confidence)
- `cmd/cloud-claude/main.go` — CLI 骨架和 cobra 注册模式
- `internal/runtime/tasks/worker.go` — Docker CLI 操作模式（runDocker, buildCreateArgs, inspectContainer）
- `deploy/docker/managed-user/entrypoint.sh` — 容器入口脚本结构
- `deploy/docker/managed-user/image.lock` — 镜像名和版本元数据
- `.devcontainer/devcontainer.json` — 已有 Dev Container 模板
- `internal/network/singbox_config.go` — sing-box 配置生成
- `internal/network/outbound_parse.go` — outbound JSON 解析

### Secondary (MEDIUM confidence)
- `internal/network/types.go` — ProxySpec / EgressConfig 类型定义

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 全部复用现有项目依赖，无新引入
- Architecture: HIGH — cobra + Docker CLI 模式已在项目中成熟使用
- Pitfalls: MEDIUM — macOS Docker Desktop 特性细节需实际验证

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (stable — 项目模式已确立)
