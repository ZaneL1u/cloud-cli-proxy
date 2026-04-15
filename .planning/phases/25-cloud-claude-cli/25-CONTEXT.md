# Phase 25: cloud-claude CLI 骨架与连接 - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

交付用户侧 Go 二进制 `cloud-claude` 的最小可用闭环：`cloud-claude init` 将网关与凭据写入 `~/.cloud-claude/config.yaml`；无参数运行 `cloud-claude` 时完成控制面认证、在主机未就绪时等待或重试、建立到 SSH Proxy 的会话并进入容器内 Claude Code 交互环境。

本阶段**不包含**：`claude` 子命令参数的原样透传（Phase 26）、TTY/信号/退出码完整语义（Phase 26）、sshfs 目录映射（Phase 27）、SSH Proxy / Worker 服务端改动（延续 Phase 24 零改造结论）。

</domain>

<decisions>
## Implementation Decisions

### CLI 结构与依赖
- **D-01:** 使用 `github.com/spf13/cobra` 组织子命令；根命令对应「无参数主流程」，`init` 为独立子命令。与 `.planning/research/STACK.md` 中对 v2.0 客户端栈的推荐一致。
- **D-02:** 新增依赖：`cobra`、`gopkg.in/yaml.v3`（序列化 `config.yaml`）、`golang.org/x/term`（密码无回显）。SSH 使用仓库已采用的 `golang.org/x/crypto/ssh`；在 Phase 25 实施前将 `golang.org/x/crypto` 与全仓对齐（见 STATE.md 待办）。

### 配置模型与文件约定
- **D-03:** 配置文件路径固定为 `~/.cloud-claude/config.yaml`（与 CLI-02 / ROADMAP 一致）。配置目录权限 `0700`，文件权限 `0600`。
- **D-04:** `config.yaml` 至少包含：`gateway`（控制面 HTTPS 基础 URL，含 scheme，无尾部斜杠）、`short_id`（用户或主机 short_id，与 Entry 路由一致）、`password`（用户登录密码，用于 Entry 认证）。**不**在 Phase 25 引入 OAuth 等并行认证路径。

### `cloud-claude init` 交互
- **D-05:** 默认交互式：`gateway`、`short_id`、密码（stdin 关闭回显）。提供等价环境变量（建议前缀 `CLOUD_CLAUDE_`）与非交互 flag，便于 CI/脚本写入同一配置文件。

### 控制面集成（就绪与 SSH 参数）
- **D-06:** 使用现有 `POST {gateway}/v1/entry/{shortId}/auth`（JSON `password`）获取 SSH 参数与状态；行为与 `internal/controlplane/http/entry.go` 及 bootstrap 文档一致，**不新增**专用 cloud-claude API。
- **D-07:** 当响应 `status` 为 `not_ready`（或主机非 running 时的提示）时，CLI 在超时窗口内按固定间隔重试同一 auth 请求，并在终端输出中文等待提示；超时时失败退出。响应中 `ssh_host` 来自请求 `Host` 头，故用户配置的 `gateway` 主机名应与实际访问控制面的主机名一致（私有部署时在文档中说明）。
- **D-08:** 可选：在首次 auth 前对 `gateway` 发起轻量可达性检查（例如已有健康或根路径）；失败归类为网关不可达（退出码见下）。

### SSH 与会话
- **D-09:** 使用 auth 成功返回的 `ssh_user`、`ssh_pass`、`ssh_host`、`ssh_port` 建立 SSH 连接。主机密钥策略与现有 Entry 脚本一致：`StrictHostKeyChecking=no`、`UserKnownHostsFile=/dev/null`（Phase 25）；不在此阶段实现密钥钉扎。
- **D-10:** 单连接上打开一个 `session`，申请 PTY，在容器内执行远程命令以进入 Claude Code 交互（例如直接 `exec` 容器内 `claude`，或等价登录 shell）。**不**要求本阶段传递任意额外 argv（完整透传属 Phase 26）。

### 错误提示与退出码
- **D-11:** 所有面向用户的错误信息为**中文**。建议退出码：`0` 成功；`1` 认证/凭据错误；`2` 网络或网关不可达；`3` 超时或主机持续未就绪；`4` 配置缺失或无效；其他错误 `5`。具体文案需覆盖：网关不可达、认证失败、主机未就绪超时。

### 自有网关（CLI-05）
- **D-12:** `gateway` 完全由配置文件决定，**无**硬编码生产默认 URL；若缺少配置且非 `init` 流程，应明确报错并提示先执行 `cloud-claude init`。

### Claude's Discretion
- 健康检查 URL 路径、轮询间隔与默认超时秒数。
- `cobra` 子命令命名是否预留 `version`/`doctor` 等占位（不影响 MVP）。
- 远程启动命令的确切字符串（依赖受管镜像内 `claude` 是否已在 PATH）。

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 需求与路线图
- `.planning/REQUIREMENTS.md` — CLI-01、CLI-02、CLI-04、CLI-05
- `.planning/ROADMAP.md` — Phase 25 Goal、Success Criteria、依赖 Phase 24

### 控制面 Entry 契约
- `internal/controlplane/http/entry.go` — `Auth()` 返回字段、`ssh_host` 自 `r.Host` 推导、`ssh_port` 固定 `2222`、未 running 时 `not_ready`

### SSH Proxy
- `internal/sshproxy/proxy.go` — 客户端经 Proxy 接入容器；Phase 24 已确认多 session 能力，本阶段仅单交互 session

### 栈与架构研究
- `.planning/research/STACK.md` — cobra、yaml、x/term、x/crypto
- `.planning/research/ARCHITECTURE.md` — `cmd/cloud-claude`、数据流与 Pattern 1

### 用户文档（行为对齐）
- `docs/zh/reference/api.md` — Entry 短链接与 auth 示例（若英文为主则同步 `docs/en/reference/api.md`）

### 前序阶段结论
- `.planning/phases/24-fuse/24-CONTEXT.md` — FUSE/多 session 前置、SSH Proxy 零改造

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable资产
- `internal/controlplane/http/entry.go`：Entry 认证与 SSH 参数返回的**唯一真实契约**，`cloud-claude` HTTP 客户端应与此对齐。
- `internal/sshproxy/proxy.go`：SSH 服务端行为与端口约定（文档中 `:2222`）的参照。

### 既定模式
- 仓库已有 `golang.org/x/crypto`；新增 `cmd/cloud-claude` 与现有 `cmd/control-plane` 并列，符合 `.planning/research/ARCHITECTURE.md` 中的目录建议。

### 集成点
- 客户端仅通过 HTTPS 调用控制面 Entry API + 通过 TCP 连接 SSH Proxy；不绕过现有任务系统单独造启动路径（与 Phase 3 bootstrap 原则一致：主机需已 running）。

</code_context>

<specifics>
## Specific Ideas

无额外产品偏好 — `/gsd-discuss-phase 25 --auto` 采用推荐默认（与 Entry 脚本及研究文档一致）。

</specifics>

<deferred>
## Deferred Ideas

- `claude` CLI 参数原样透传、SIGWINCH/信号/退出码 — Phase 26
- sshfs slave 与当前目录映射 — Phase 27
- 静默降级到本地 `claude` — 项目明确不采纳（Out of Scope）

</deferred>

---

*Phase: 25-cloud-claude-cli*
*Context gathered: 2026-04-15*
