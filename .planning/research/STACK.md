# Stack Research

**Domain:** v2.0 `cloud-claude` 透明远程 CLI（Go 客户端 + 目录映射 + TTY/信号 + 本地配置）  
**Researched:** 2026-04-15  
**Confidence:** HIGH（Go 模块版本以 pkg.go.dev 默认 tag 为准）；MEDIUM（sshfs/Mutagen 与具体发行版组合需在集成阶段实测）

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go toolchain | 与仓库 `go` directive 对齐（建议 ≥ 1.25，与 `golang.org/x/crypto` v0.50.x 声明一致） | 单一 `cloud-claude` 二进制 | 与现有控制面/host-agent 同一语言栈，复用模块与发布流程。 |
| `golang.org/x/crypto` | **v0.50.0**（pkg.go.dev 默认） | SSH 客户端、`ssh.Session`、PTY、`Subsystem`（SFTP） | 仓库已在 `internal/sshproxy` 使用；客户端应与之**同大版本线**升级，避免两套 `ssh` 行为分叉。 |
| `golang.org/x/term` | **v0.42.0**（`crypto` v0.50.0 依赖链一致） | 本地 TTY 尺寸读取、必要时 raw 模式、`ReadPassword` | 与 `crypto/ssh` 的 WindowChange/PTY 模型配套，生态标准选择。 |
| OpenSSH 语义（无独立版本号） | 由远端容器镜像与宿主 `ssh` 决定 | 会话/exec、SFTP 子系统、端口转发 | 目录映射若走 **SSH 隧道 + SFTP/sshfs**，协议层保持 IETF/OpenSSH 事实标准，避免自造帧格式。 |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/spf13/cobra` | **v1.10.2** | 子命令（`init`、默认转发 `claude`）、POSIX 风格 flag、与 `kubectl`/`gh` 一致的体验 | 需要丰富帮助、completion、`PersistentPreRun` 统一前置逻辑时（推荐默认）。 |
| `github.com/urfave/cli/v3` | **v3.8.0** | 轻量声明式 CLI、依赖极少 | 希望**最小依赖**、且子命令结构简单时；与 Cobra 二选一即可。 |
| `github.com/spf13/viper` | **v1.21.0** | 合并配置文件 + 环境变量 + flag，支持 `~/.cloud-claude/config` | 需要 `CLOUD_CLAUDE_*` 覆盖文件、多配置文件路径时。 |
| `gopkg.in/yaml.v3` | **v3.0.1** | 解析/写出 `config.yaml` | 采用**手写配置加载**、或 Viper 的 YAML 后端时（二者取一，避免重复抽象）。 |
| `github.com/pkg/sftp` | **v1.13.10** | SFTP 客户端/服务端（与 `golang.org/x/crypto/ssh` 配套） | **容器内 sshfs 挂载**或 **本机暴露 SFTP 供远端挂载** 时；勿选 **v2.0.0-alpha** 上生产。 |
| `github.com/coder/websocket` | **v1.8.14** | WebSocket 上承载字节流（`NetConn`） | 仅当产品要求 **HTTP/WebSocket 侧车隧道**（例如经网关 Upgrade）时再引入；非默认路径。 |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `staticcheck` / `golangci-lint` | 客户端与共享 `ssh` 代码的静态检查 | 升级 `x/crypto` 后跑一遍，避免弃用 API。 |
| 交叉编译矩阵 | `GOOS`/`GOARCH` 与 macOS/Linux 用户 | `cloud-claude` 若在用户本机运行，需与目标 OS 对齐信号与 TTY 行为。 |

## Installation

```bash
# 客户端模块（示例：在独立 module 或主模块中）
go get golang.org/x/crypto@v0.50.0
go get golang.org/x/term@v0.42.0
go get github.com/spf13/cobra@v1.10.2
go get github.com/spf13/viper@v1.21.0
go get gopkg.in/yaml.v3@v3.0.1
# 若实现 SFTP 目录同步：
go get github.com/pkg/sftp@v1.13.10
# 可选：WebSocket 隧道
go get github.com/coder/websocket@v1.8.14
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|---------------------------|
| `cobra` +（可选）`viper` | `urfave/cli/v3` + 手写 YAML | 更强依赖控制、CLI 面积极简时。 |
| **SSH 原生能力**（`Session` + PTY + `SIGWINCH` + `Forward*`） | 完全自建 WebSocket 应用协议 | 只有当你**不能**走 SSH（企业只允许 443/WSS）时；成本显著更高。 |
| **容器内 sshfs + FUSE**（镜像装 `fuse`/`sshfs`，`--device /dev/fuse`） | **Mutagen**（独立同步守护进程） | 需要**近似实时的双向同步**、可接受额外二进制与会话管理时；Mutagen 对「开发机 ↔ 远端」场景成熟。 |
| `github.com/pkg/sftp` **v1.13.x** | `github.com/pkg/sftp/v2`（**v2.0.0-alpha**） | API 稳定、需可预测行为时用 v1；v2 仅适合愿意跟 alpha 的实验分支。 |
| `github.com/coder/websocket` | `github.com/gorilla/websocket` | 需要更广社区示例与 `PreparedMessage` 等特性时；新项目更推荐 Coder fork 的上下文模型。 |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `golang.org/x/net/websocket` | 官方已标记弃用（见 `coder/websocket` README 引用） | `github.com/coder/websocket` 或 Gorilla（按上表）。 |
| `nhooyr.io/websocket` 默认路径 | pkg.go.dev 标明 **deprecated**，指向 Coder 维护仓库 | `github.com/coder/websocket`。 |
| 第二套未对齐的 `golang.org/x/crypto` 版本 | 与现有 `sshproxy` 行为不一致，PTY/SFTP 边界难排障 | 全仓统一升级到同一 `x/crypto` minor。 |
| 在 v2.0 同时引入 **完整** 同步栈（例如 Syncthing） | 运维与网络面爆炸，偏离「透明 CLI」 | 先用 sshfs **或** Mutagen **或** SFTP 一条路径验证。 |

## Stack Patterns by Variant

**若产品主线是「SSH 已可达网关/容器」：**

- 客户端使用 **`golang.org/x/crypto/ssh` 连接**（或沿用你们已有 bootstrap 信息），目录映射优先 **反向 SFTP + 容器内 sshfs** 或 **Mutagen over SSH**。
- TTY：`Session.RequestPty` + `ssh.WindowChange` + `signal.Notify` 转发 `SIGWINCH`；`SIGINT`/`SIGTERM` 按 OpenSSH 语义映射到会话（注意远端进程组与 `-t` 行为）。

**若必须经 HTTP 网关（仅 WSS）：**

- 用 **`github.com/coder/websocket` 的 `NetConn`** 把 WebSocket 变成 `net.Conn`，再在其上跑 **TLS/自定义 framing + SFTP** 或你们自定义块同步协议；集成难度 **高**，仅在有明确合规/网络约束时采用。

**若强调「零 FUSE、弱侵入」：**

- 考虑 **Mutagen** 同步到容器内固定路径，再 `exec` `claude`；不装 sshfs，但需管理 Mutagen 生命周期与冲突解决。

## v2.0 专项：目录映射方案对比

| 方案 | 技术要素 | 集成难度（1–5） | 优点 | 代价 / 风险 |
|------|-----------|-----------------|------|-------------|
| **容器内 sshfs（FUSE）** | 镜像：`fuse3`、`sshfs`；Docker：`--device /dev/fuse`，常需 `CAP_SYS_ADMIN` 等；隧道指向「本机 SFTP 或反向端口」 | **4** | 远端呈现 POSIX 路径（如 `/workspace`），对 `claude` 透明 | FUSE 权限、延迟与缓存语义、排障需内核/容器知识。 |
| **Mutagen** | 发行版二进制 **v0.18.1**（GitHub Release，2026-02 验证） | **3–4** | 双向同步与冲突策略成熟，适合开发目录 | 额外进程与版本捆绑；需定义与容器生命周期对齐。 |
| **纯 SFTP（`pkg/sftp` + SSH 子系统）** | 不挂载，仅同步或按需读写 | **3** | Go 侧可控、无 FUSE | 「实时映射」需自建轮询/inotify 策略，否则是准实时。 |
| **WebSocket + SFTP/自定义** | `coder/websocket` + 应用层 | **5** | 穿透仅 443 的环境 | 协议与网关开发量大，**默认不推荐**。 |

**建议：** 优先在里程碑内定一条「主路径」——若 `PROJECT.md` 已承诺 FUSE + sshfs，则以 **sshfs 主路径 + Mutagen 作备选** 写进实现计划；WebSocket 方案保留为私有部署受限网络的可选阶段。

## TTY / 信号：库与职责划分

| 能力 | 推荐实现 | 说明 |
|------|-----------|------|
| PTY 申请 | `ssh.Session.RequestPty` | 与远端 `claude` TUI 兼容。 |
| 窗口变化 | `golang.org/x/term.GetSize` + `ssh.WindowChange` | 注册 `SIGWINCH`。 |
| 退出码 | `Session.Wait` / `ExitError` | 透传给用户 shell 的 `$?` 期望。 |
| 本地 raw 模式 | `term.MakeRaw`（仅当 stdin 是 TTY） | 避免双重 raw；与 `ssh` 会话复制循环搭配。 |

**一般不需要**单独引入 `github.com/creack/pty`，除非客户端还要在**本机**再起一个带 PTY 的子进程（与「远端执行 claude」主路径不同）。

## 配置：`~/.cloud-claude/`

| 组件 | 版本 | 用途 |
|------|------|------|
| `gopkg.in/yaml.v3` v3.0.1 | 序列化 `config.yaml` | 与 `PROJECT.md` 中 `~/.cloud-claude/config.yaml` 一致。 |
| `github.com/spf13/viper` v1.21.0 | 可选 | 需要 env 覆盖、多路径搜索时再引入；否则仅 YAML + `os.UserHomeDir` 即可降低依赖。 |

敏感信息（token、SSH 私钥）：优先 **文件权限 0600** + 可选 **`filippo.io/age`** 等工具加密；是否加密留待实现阶段再选，本研究不强制版本。

## 明确不需要为 v2.0 新增的栈

| 领域 | 说明 |
|------|------|
| 控制面 HTTP API、PostgreSQL、JWT 管理后台 | v1.x 已具备；客户端只消费已有能力或通过 SSH。 |
| Docker / sing-box / nftables | 宿主机网络栈不变；客户端不直接操控。 |
| React / Vite | 本里程碑无新前端需求。 |
| 新 SSH 服务端协议 | 继续 `golang.org/x/crypto/ssh` + 现有 `sshproxy` 模型。 |

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `golang.org/x/crypto@v0.50.0` | `golang.org/x/term@v0.42.0` | `crypto` v0.50.0 的 go.mod 依赖链已拉齐 `x/term`。 |
| `github.com/pkg/sftp@v1.13.10` | `golang.org/x/crypto` ≥ 其 go.mod 声明 | 升级时以 **较新者** 为准做一次 `go mod tidy`。 |
| `github.com/spf13/cobra@v1.10.2` | `pflag` 新命名（`ParseErrorsAllowlist`） | 若项目仍用旧 `pflag` API，需跟随 Cobra 发布说明升级。 |

## Sources

- https://pkg.go.dev/golang.org/x/crypto — 默认版本 **v0.50.0**（验证日期以页面为准）
- https://pkg.go.dev/golang.org/x/term — 默认版本 **v0.42.0**
- https://pkg.go.dev/github.com/spf13/cobra — **v1.10.2**
- https://github.com/spf13/cobra/releases/tag/v1.10.2 — 发布说明
- https://pkg.go.dev/github.com/spf13/viper — **v1.21.0**
- https://pkg.go.dev/gopkg.in/yaml.v3 — **v3.0.1**
- https://pkg.go.dev/github.com/pkg/sftp — **v1.13.10**（v2 为 alpha）
- https://pkg.go.dev/github.com/coder/websocket — **v1.8.14**（替代已弃用的 `nhooyr.io/websocket`）
- https://pkg.go.dev/github.com/urfave/cli/v3 — **v3.8.0**
- https://github.com/mutagen-io/mutagen/releases/tag/v0.18.1 — Mutagen 版本示例
- 仓库现状：`go.mod` 中 `golang.org/x/crypto v0.37.0` — v2.0 客户端开发时建议**统一升级**并在本节版本表归档

---
*Stack research for: cloud-cli-proxy v2.0 cloud-claude 客户端栈增量*  
*Researched: 2026-04-15*
