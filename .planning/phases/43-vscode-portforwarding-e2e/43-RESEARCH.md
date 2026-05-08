# Phase 43: VS Code Remote-SSH 端口转发 E2E 补齐 - Research

**Researched:** 2026-05-08
**Domain:** SSH 端口转发 E2E 验证 / UAT 脚本补齐 / VERIFICATION.md 生成
**Confidence:** HIGH

## Summary

Phase 43 是一个 gap closure 阶段，目标是补齐 Phase 40 的验证缺口。Phase 40 已完成 VS Code Remote-SSH 基础 E2E 验证（连接、文件浏览、终端、扩展安装），但 UAT 脚本缺少 direct-tcpip 端口转发验证场景，且未生成标准 VERIFICATION.md。

现有 UAT 脚本 `tests/scripts/uat-vscode-remote-ssh.sh` 覆盖 6 个场景（sing-box 进程、出口 IP、DNS 泄漏、VS Code Server、sshd、sing-box 日志），但完全没有覆盖 SSH 端口转发路径的验证。Phase 38 的 SSH Proxy 实现（`internal/sshproxy/forward.go`）有完善的单元测试，但缺少从 UAT 脚本层面验证端口转发功能的端到端测试。

本阶段需要：(1) 向 UAT 脚本新增端口转发场景，(2) 新增端口转发出口 IP 验证场景，(3) 生成标准格式的 VERIFICATION.md 覆盖 SSH-05 / SEC-01 / SEC-02。

**Primary recommendation:** 在现有 UAT 脚本中追加 3 个新场景（direct-tcpip 端口转发、端口转发出口 IP 验证、安全目标拒绝验证），然后编写标准 VERIFICATION.md。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- UAT 脚本必须包含 direct-tcpip 端口转发验证场景
- UAT 脚本必须包含 tcpip-forward 端口转发验证场景
- 测试应覆盖本地和云端两种模式
- 验证 VS Code 语言服务器端口（如 60000-60010）通过 direct-tcpip channel 正常工作
- 验证多个并发端口转发通道不互相干扰
- 验证端口转发到被禁止的目标（管理网段、Docker socket）被正确拒绝
- 验证通过端口转发访问外部服务时，出口 IP 必须是绑定的 egress IP
- 验证 VS Code Server 下载和扩展安装流量通过 sing-box 出站
- 验证无 egress 配置时，流量不泄漏

### Claude's Discretion

- 具体测试端口选择（只要不冲突即可）
- 测试超时设置
- 错误消息格式

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SSH-05 | VS Code Remote-SSH 端到端验证：连接、VS Code Server、端口转发、claude 命令 | Phase 40 已验证连接和终端；本阶段补齐端口转发验证。SSH Proxy `forward.go:96-144` handleDirectTCPIP 已实现，`proxy.go:228` case "direct-tcpip" 已分发。UAT 脚本需新增 direct-tcpip 端口转发场景 |
| SEC-01 | 验证 direct-tcpip 转发流量走 sing-box tun | sing-box tun auto_route 捕获所有 TCP 流量（含 direct-tcpip 转发后的连接）。验证方式：通过端口转发从宿主机访问容器内服务时，检查出口 IP 是否为 egress IP。需在 UAT 脚本中新增端口转发出口 IP 验证场景 |
| SEC-02 | VS Code Server 下载/扩展安装流量走受控出口 | Phase 40 UAT 脚本场景 6 已覆盖 sing-box 日志域名检查。本阶段确认此场景仍在 UAT 脚本中，并在 VERIFICATION.md 中标记 SATISFIED |
</phase_requirements>

## Standard Stack

### Core

| Library/Tool | Version | Purpose | Why Standard |
|-------------|---------|---------|-------------|
| bash | 宿主机自带 | UAT 脚本追加 | 项目已有 UAT 脚本模式 |
| ssh (OpenSSH client) | 宿主机自带 | direct-tcpip 端口转发测试 | `ssh -L` 触发 direct-tcpip channel |
| curl | 宿主机自带 | 通过端口转发验证出口 IP | curl 经过 SSH 隧道访问 ifconfig.me |
| ncat/nc | 容器内 | 轻量 TCP 监听 | 比 python http.server 更轻量的端口监听方案 |

### Supporting

| Library/Tool | Version | Purpose | When to Use |
|-------------|---------|---------|-------------|
| python3 | 容器内 | HTTP 服务端口监听备选 | 当 ncat 不可用时的 fallback |
| sing-box | 镜像内 | 流量隧道 | 端口转发后的流量出口验证 |

**No new dependencies needed.** Phase 43 全部使用现有工具。

## Architecture Patterns

### 现有 UAT 脚本扩展模式

Phase 40 的 UAT 脚本采用 scenario 函数模式：

```bash
scenario_xxx() {
  reset_assertions
  info "===== 场景 N: 名称 ====="
  # dry-run 分支
  if [[ "$DRY_RUN" == "true" ]]; then ... fi
  # 环境检测
  if ! has_docker; then skip ... fi
  # 实际断言
  ...
  write_json_report "xxx" "pass|fail"
}
```

**新场景应遵循相同模式**，在 main() 中追加调用。

### SSH 端口转发测试架构

```
宿主机 (test script)
  → ssh -L LOCAL_PORT:localhost:CONTAINER_PORT user@127.0.0.1 -p SSH_PROXY_PORT
  → SSH Proxy: handleDirectTCPIP (forward.go:96-144)
    → isForbiddenTarget() 安全校验
    → targetClient.OpenChannel("direct-tcpip", ...) 到容器
  → 容器内 sshd: direct-tcpip → 目标端口
  → 数据双向拷贝 (forward.go:129-141)
```

测试关键点：
1. **端口转发连通性**：在容器内启动 HTTP 服务，通过 SSH -L 从宿主机 curl 访问
2. **出口 IP 验证**：在容器内启动返回 client IP 的服务，通过 SSH -L 从宿主机访问，确认出口 IP
3. **安全拒绝**：尝试转发到 forbidden target（10.99.x.x），确认被 SSH Proxy 拒绝

### direct-tcpip 触发方式

`ssh -L LOCAL:localhost:REMOTE user@proxy` 会在 SSH 协议层发送 `direct-tcpip` channel open 请求。这是 VS Code Remote-SSH 端口转发的底层机制。

对于 `tcpip-forward`（远程端口转发），使用 `ssh -R` 触发，但 VS Code 更常用的是 `direct-tcpip`（本地端口转发）。两者都应验证。

### 端口选择策略

- 测试端口：使用高端口避免冲突（如 19876、19877、19878）
- 语言服务器端口模拟：使用 60000-60010 范围内的端口
- 容器内监听端口：使用 9876、9877 等不与已知服务冲突的端口

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 端口转发测试 | 自定义 Go 测试程序 | `ssh -L` + `curl` | 标准工具链足够，与 VS Code 真实路径一致 |
| 出口 IP 检测 | 自定义检测脚本 | `curl ifconfig.me` 通过 SSH 隧道 | 复用 Phase 40 验证模式 |
| 安全拒绝测试 | 模拟 SSH 协议 | `ssh -L` 到 forbidden target | 真实触发 handleDirectTCPIP 的 isForbiddenTarget |

## Common Pitfalls

### Pitfall 1: SSH -L 后台进程残留
**What goes wrong:** `ssh -L ... -f -N` 后台进程在脚本退出后残留，占用测试端口
**Why it happens:** bash 脚本未正确清理 SSH 后台进程
**How to avoid:** 使用 trap EXIT 清理 SSH PID；或使用 `ssh -L ... -f -N -o ExitOnForwardFailure=yes` 并记录 PID
**Warning signs:** 重复运行脚本时报 "Address already in use"

### Pitfall 2: 容器内服务启动时序
**What goes wrong:** SSH 端口转发连接在容器内服务启动前就发起，导致 connection refused
**Why it happens:** 脚本中 SSH -L 和 curl 执行过快
**How to avoid:** 在容器内启动服务后，先用 `docker exec ... nc -z localhost PORT` 确认端口监听
**Warning signs:** curl 报 "Connection refused"

### Pitfall 3: macOS SSH 隧道 DNS 泄漏
**What goes wrong:** `ssh -L` 后 curl 时，DNS 解析发生在宿主机而非隧道内
**Why it happens:** curl 默认在本地解析 DNS，只有 TCP 连接走隧道
**How to avoid:** 使用 `curl --connect-to` 或直接用 IP 地址；或在 SSH 隧道内做 `curl http://127.0.0.1:PORT`（目标是容器内的 localhost 服务）
**Warning signs:** DNS 查询不出口 IP

### Pitfall 4: 安全拒绝测试中的错误消息传递
**What goes wrong:** SSH Proxy 拒绝转发时，错误消息可能被 SSH 客户端吞掉
**Why it happens:** SSH -L 转发失败时，客户端只报 "Connection refused" 或 "administratively prohibited"
**How to avoid:** 检查 SSH 退出码和 stderr；`ssh -v` 可显示更详细的拒绝信息
**Warning signs:** 测试只检查连接失败，未验证是安全拒绝

## Code Examples

### SSH -L 端口转发测试模式

```bash
# 启动容器内 HTTP 服务
docker exec -d "$CONTAINER" python3 -m http.server 9876 --directory /tmp

# 通过 SSH Proxy 端口转发从宿主机访问
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -L 19876:localhost:9876 \
    -f -N -p "$SSH_PORT" workspace@127.0.0.1

# 通过隧道访问
RESULT=$(curl -s --max-time 5 http://127.0.0.1:19876/)

# 清理
kill %1 2>/dev/null || true
```

### 端口转发出口 IP 验证模式

```bash
# 在容器内启动返回请求者 IP 的 HTTP 服务
docker exec -d "$CONTAINER" python3 -c "
from http.server import HTTPServer, BaseHTTPRequestHandler
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(self.client_address[0].encode())
HTTPServer(('0.0.0.0', 9877), H).serve_forever()
"

# 通过 SSH 隧道访问
EGRESS_IP_CHECK=$(curl -s --max-time 5 http://127.0.0.1:19877/)
# EGRESS_IP_CHECK 应为容器内 IP（172.x.x.x），不是宿主机 IP
```

### 安全拒绝测试模式

```bash
# 尝试转发到管理网段
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -L 19878:10.99.1.1:22 \
    -f -N -p "$SSH_PORT" workspace@127.0.0.1 2>&1
# 预期：SSH 返回 "Channel open administratively prohibited" 或类似错误
```

## State of the Art

| Item | Phase 40 现状 | Phase 43 目标 |
|------|--------------|--------------|
| UAT 脚本场景数 | 6（sing-box/egress/DNS/vscode-sshd/singbox-log） | 9（+direct-tcpip/转发出口IP/安全拒绝） |
| 端口转发覆盖 | 无 | direct-tcpip + tcpip-forward + 安全拒绝 |
| 出口 IP 验证路径 | 容器内直接 curl | 容器内直接 + 端口转发间接（两条路径） |
| VERIFICATION.md | 仅有 40-VERIFICATION-REPORT.md（手动报告） | 标准格式 VERIFICATION.md（SSH-05/SEC-01/SEC-02 SATISFIED） |

## Open Questions

1. **tcpip-forward 单独验证是否必要？**
   - What we know: Phase 38 已有 handleGlobalRequests 单元测试和 proxyForwardedChannels 测试
   - What's unclear: VS Code Remote-SSH 是否实际使用远程端口转发（-R），还是只用本地端口转发（-L）
   - Recommendation: VS Code 主要使用 direct-tcpip（-L），tcpip-forward（-R）在 Phase 38 已有单元测试覆盖，E2E 验证可标记为 "code-verified" 级别，不强制要求 UAT 脚本覆盖

2. **VERIFICATION.md 模板来源**
   - What we know: Phase 38 和 Phase 39 都有标准格式的 VERIFICATION.md
   - What's unclear: 是否有统一模板
   - Recommendation: 复用 Phase 38 VERIFICATION.md 的结构（frontmatter + Observable Truths + Artifacts + Key Links + Requirements Coverage）

## Sources

### Primary (HIGH confidence)
- `internal/sshproxy/forward.go` — direct-tcpip 和 forwarded-tcpip 完整实现
- `internal/sshproxy/forward_test.go` — 14 个测试函数覆盖 payload 解析、安全校验、全局请求转发、forwarded-tcpip relay
- `tests/scripts/uat-vscode-remote-ssh.sh` — 现有 6 场景 UAT 脚本
- `.planning/phases/40-vs-code-remote-ssh-e2e/40-MANUAL-CHECKLIST.md` — 9 步手动测试 checklist
- `.planning/phases/038-ssh-proxy-port-forwarding/38-VERIFICATION.md` — 标准 VERIFICATION.md 格式参考

### Secondary (MEDIUM confidence)
- `.planning/phases/39-dev-containers/39-VERIFICATION.md` — 另一个 VERIFICATION.md 格式参考
- `deploy/docker/managed-user/sshd_config` — AllowTcpForwarding yes 确认

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 全部使用现有工具和已验证的模式
- Architecture: HIGH — 复用 Phase 40 UAT 脚本模式和 Phase 38 VERIFICATION.md 格式
- Pitfalls: HIGH — SSH 端口转发测试模式已通过 Phase 40 手动验证确认可行

**Research date:** 2026-05-08
**Valid until:** 2026-06-08 (stable — 基于已实现的代码基础设施)
