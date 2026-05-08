# Phase 41: Doctor 扩展与收尾 - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning (--auto 模式，推荐默认值)

<domain>
## Phase Boundary

`cloud-claude doctor` 新增 remote-ssh 诊断维度，覆盖 VS Code Remote-SSH 场景的三项检查；同时补齐 v3.4 所有需求对应的错误码注册和 explain 长说明。不涉及新功能，只扩展诊断覆盖范围。

</domain>

<decisions>
## Implementation Decisions

### 新增维度：remote-ssh（独立于现有 ssh 维度）
- 新建 `remote-ssh` 维度，不复用现有 `ssh` 维度
- 现有 ssh 维度聚焦连接健康（keepalive、known_hosts、workspace keys）；remote-ssh 聚焦 VS Code Remote-SSH 特有问题
- 维度名称：`remote-ssh`，cobra 子命令可输入 `cloud-claude doctor remote-ssh` 或 `cloud-claude doctor all`
- 远端检查走现有 RemoteRunner 接口，保持 lazy connect 模式不变

### VS Code Server 进程检测
- 通过远端执行 `pgrep -f vscode-server` 检测进程是否存活
- 同时检测 VS Code Server 默认端口（远端 `ss -tlnp | grep vscode-server`）确认服务就绪
- 两个 check 项：`vscode_server_process`（进程存在性）和 `vscode_server_port`（端口监听）
- 进程不存在 → Skip（用户可能未使用 VS Code）；进程存在但端口未监听 → Warn

### ~/.vscode-server/ 磁盘占用
- 远端执行 `du -sh ~/.vscode-server/` 获取总大小
- 阈值：≥500MB → Warn（建议清理 extensions cache）；≥2GB → Fail（建议完整清理）
- 清理建议分级：
  - 轻量：`rm -rf ~/.vscode-server/extensions-cache/`（仅缓存）
  - 中量：`rm -rf ~/.vscode-server/*/extensions/*/`（所有扩展，保留配置）
  - 完整：`rm -rf ~/.vscode-server/`（完全清理，VS Code 重连时会重建）
- 复用现有 `parseDuHumanToMB()` 函数（disk.go 已有）解析 du 输出

### Forwarding Channel 检测
- 远端检测 SSH forwarding 代理 socket 是否正常：`ss -xp | grep forwarding`
- 检测是否存在防火墙规则拦截转发流量：`iptables -L OUTPUT -n | grep -c DROP` 简单计数
- 仅在 VS Code Server 进程存在时执行（依赖 `vscode_server_process` 结果）
- forwarding socket 不存在 → Skip；socket 存在但有 DROP 规则 → Warn

### 错误码注册（v3.4 闭合）
- 新增 `SSH_*` 域错误码（沿用现有 SSH_ 前缀）：
  - `SSH_VSCODE_SERVER_NOT_RUNNING` — VS Code Server 进程不存在（Severity: Info）
  - `SSH_VSCODE_PORT_NOT_LISTENING` — VS Code Server 进程存在但端口未监听（Severity: Warn）
  - `SSH_FORWARDING_SOCKET_MISSING` — forwarding socket 不存在（Severity: Info）
  - `SSH_FORWARDING_BLOCKED` — 防火墙规则可能拦截 forwarding（Severity: Warn）
- 新增 `DISK_*` 域错误码：
  - `DISK_VSCODE_SERVER_WARN` — ~/.vscode-server/ 超过 500MB（Severity: Warn）
  - `DISK_VSCODE_SERVER_BLOAT` — ~/.vscode-server/ 超过 2GB（Severity: Error）

### Explain 子命令覆盖
- 为每个新增错误码注册 `registerExplanation()` 长说明（≥200 中文字符）
- 五段模板：触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档
- `SSH_VSCODE_SERVER_NOT_RUNNING` 和 `SSH_FORWARDING_SOCKET_MISSING` 级别为 Info，加入 `ExplainExempt`（豁免长说明）
- 其余 4 个 Warn/Error 级别码必须有完整长说明

### Claude's Discretion
- 具体阈值（500MB / 2GB）可在实现时根据实际使用场景微调
- forwarding 检测的精确命令可在实现时调整
- 新增 check 的 timeout 沿用现有默认（5s / verbose 30s）

</decisions>

<specifics>
## Specific Ideas

- 复用 `disk.go` 中已有的 `parseDuHumanToMB()` 函数解析 du 输出，不重复造轮
- 远端脚本执行模式与现有 `ssh.go`、`disk.go` 一致：`runner.RunScript("check_name", "shell commands")`
- 新错误码注册文件：`errcodes/remote_ssh.go`（SSH_* 前缀码）+ 更新 `errcodes/disk.go`（DISK_* 前缀码）
- doctor 维度文件：新建 `internal/cloudclaude/doctor/remote_ssh.go`

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `RemoteRunner` 接口 + `SSHRemoteRunner`：远端命令执行基础设施，remote-ssh 维度直接复用
- `runWithTimeout()`：单 check 超时包装，新 check 统一使用
- `newPass/newWarn/newFail/newSkip`：构造 helper，新 check 函数直接调用
- `parseDuHumanToMB()`（disk.go:76）：du 输出解析，磁盘占用 check 复用
- `errcodes.MustRegister()` + `registerExplanation()`：错误码和长说明注册模式

### Established Patterns
- 维度文件组织：每个维度一个 .go 文件（network.go, auth.go, ssh.go, mount.go, disk.go），remote-ssh 新建 remote_ssh.go
- 远端 check 模式：`if runner == nil { return newSkip(...) }` 降级为 Skip
- 错误码命名：`DOMAIN_KIND_DETAIL`，全大写下划线
- RunDoctor() 中维度串行执行：在 disk 之后追加 remote-ssh 维度
- --domain 过滤：`want("remote-ssh")` 函数已有的 `want()` 闭包直接支持

### Integration Points
- `RunDoctor()`（doctor.go:85）：在 disk 维度之后、Summary 聚合之前插入 remote-ssh 维度块
- `cmd/cloud-claude/doctor.go`：cobra 命令的 Domain 枚举需要加入 "remote-ssh"
- `errcodes/codes.go`：新增 Code 常量声明
- `errcodes/explanations.go`：新增 registerExplanation() 调用 + ExplainExempt 更新

</code_context>

<deferred>
## Deferred Ideas

- `~/.vscode-server` 持久化 volume 支持（SSH-06）— 属于后续需求，不在本次 doctor 检查范围内
- VS Code 多窗口/多工作区 forwarding 优化（SSH-07）— 同上
- doctor --fix 自动清理 .vscode-server — 本次只做诊断和建议，自动修复可作为后续增强

</deferred>

---

*Phase: 41-doctor*
*Context gathered: 2026-05-08 (--auto 模式)*
