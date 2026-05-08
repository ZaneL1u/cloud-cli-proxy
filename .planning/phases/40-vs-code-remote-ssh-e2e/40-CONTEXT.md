# Phase 40: VS Code Remote-SSH E2E 验证 - Context

**Gathered:** 2026-05-08
**Status:** Ready for planning

<domain>
## Phase Boundary

验证 VS Code Remote-SSH 可以完整连接到 Cloud 版 managed-user 容器（通过 SSH Proxy 2222 端口），VS Code Server 在容器内自动下载启动，端口转发正常工作，且所有流量严格走 sing-box tun 出口。不涉及新功能开发，纯粹是端到端验证 + 必要的修复/适配。

</domain>

<decisions>
## Implementation Decisions

### 验证方式
- 手动 E2E 验证为主，辅以脚本化检查（非 Playwright 自动化）
- 原因：VS Code Remote-SSH 的连接过程涉及桌面端 GUI 交互，完全自动化成本过高
- 提供 `tests/scripts/uat-vscode-remote-ssh.sh` 脚本覆盖可脚本化的检查项（出口 IP 验证、进程检测、流量路由检测）
- 手动测试 checklist 写在计划文档中，记录每一步的操作和预期结果

### 测试环境
- 使用 `cloud-claude local` 启动本地 managed-user 容器（Phase 39 已完成）
- 容器必须启用 egress 配置（sing-box tun 模式），以验证流量约束
- 宿主机上安装 VS Code + Remote-SSH 扩展作为测试客户端
- SSH 连接信息从 `cloud-claude local` 输出获取（host, port, user, password）

### 出口 IP 验证策略
- 容器内执行 `curl ifconfig.me` 或等效命令，返回 IP 必须等于 egress 配置的 ExpectedIP
- VS Code 端口转发场景：在容器内启动一个 HTTP 服务，通过 VS Code 端口转发从宿主机访问，检查请求来源 IP
- VS Code Server 下载流量验证：检查容器内 sing-box 日志，确认 `update.code.visualstudio.com` 域名走 proxy-out 出站
- DNS 泄漏验证：容器内 `nslookup` 必须走 tun 捕获的 DNS（8.8.8.8 via sing-box），不能走宿主机 DNS

### 代码修复策略
- 如果验证发现问题，只做最小修复以通过验证标准
- 不扩展功能范围（如增加新的 CLI 命令或新的网络模式）
- 修复范围限于：entrypoint.sh 适配、sing-box 配置补全、端口转发规则调整、SSH 配置修正

### Claude's Discretion
- 测试脚本的具体检查项和输出格式
- 手动 checklist 的详细程度
- 验证过程中发现的 edge case 是否需要额外修复
- 日志采集和诊断信息的展示方式

</decisions>

<specifics>
## Specific Ideas

- 验证应覆盖 "happy path" 和 "egress 强约束" 两个维度
- Happy path：VS Code 连接 → 文件浏览 → 终端（运行 claude）→ 端口转发 → 扩展安装
- Egress 强约束：所有出口流量（HTTP/DNS/VS Code 更新）都必须走 sing-box，逐项验证
- 如果 sing-box tun 在测试环境中无法正常工作（如 macOS 无 /dev/net/tun），需要文档说明 Linux 宿主机的验证步骤

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `cloud-claude local` 子命令（Phase 39）：一键启动本地容器，输出 SSH 连接信息，可直接用于 E2E 测试
- `deploy/docker/managed-user/entrypoint.sh`：MODE=local 分支已支持 sing-box tun/proxy 启动
- `internal/network/container_proxy_provider.go`：端口映射 DNAT + MASQUERADE + 隧道转发逻辑（远程模式）
- `internal/network/gateway_singbox_config.go`：sing-box 配置生成，包含 DNS hijack 和路由规则
- `tests/scripts/uat-v31-promotion.sh`：现有 UAT 脚本模式可参考
- `cmd/cloud-claude/doctor.go`：现有诊断框架可扩展

### Established Patterns
- UAT 脚本模式：shell 脚本 + 断言函数 + 彩色输出 + 结果汇总
- sing-box 配置模式：tun inbound + proxy outbound + DNS hijack + route rules
- 容器网络模式：隔离网络 + gateway sidecar + worker 断开 bridge

### Integration Points
- SSH Proxy 2222 端口：控制面的 SSH 入口，转发到容器 sshd
- `cloud-claude local`：测试入口点
- VS Code Remote-SSH 扩展：测试客户端
- sing-box gateway：流量验证目标

</code_context>

<deferred>
## Deferred Ideas

- Phase 41: Doctor 扩展覆盖 Remote-SSH 场景诊断
- 自动化 VS Code E2E 测试（Playwright + VS Code Extension Host）— 后续版本
- 多容器并发 Remote-SSH 验证 — 后续版本

</deferred>

---

*Phase: 40-vs-code-remote-ssh-e2e*
*Context gathered: 2026-05-08*
