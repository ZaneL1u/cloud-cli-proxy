---
phase: 45-net-foundation
plan: 02
subsystem: network
tags: [dns-entry-lock, bind-mount, prepare-gateway, call-order, container-dns]
provides:
  - container-dns-bind-mount
  - PrepareGateway-split
  - GatewayConfigDir-export
  - WriteContainerDNSConfig-helper
  - containerExpectedDNS-const
requires:
  - 45-01 (sing-box 拆分 DNS + rule-set placeholder)
affects:
  - internal/network/container_proxy_provider.go
  - internal/network/container_proxy_provider_test.go
  - internal/network/verify.go
  - internal/network/verify_test.go
  - internal/runtime/tasks/worker.go
tech-stack:
  added: []
  patterns:
    - "容器 DNS 入口锁：ro bind mount 把 /etc/resolv.conf 与 /etc/nsswitch.conf 锁死为宿主写盘的源文件"
    - "Provider 职责拆分：PrepareGateway 在 worker 容器创建之前完成所有 gateway 启动 + DNS 源文件写盘；PrepareHost 退化为 connect netns + configure routes"
key-files:
  created: []
  modified:
    - internal/network/container_proxy_provider.go
    - internal/network/container_proxy_provider_test.go
    - internal/network/verify.go
    - internal/network/verify_test.go
    - internal/runtime/tasks/worker.go
    - internal/network/provider.go
decisions:
  - "PrepareGateway 与 PrepareHost 拆分为 Provider 接口的两个方法（而非内部 helper），让 worker.go::createHost 可以在 docker create 之前显式起 gateway"
  - "GatewayConfigDir 提升为导出包级函数，供 internal/runtime/tasks 直接使用，避免循环依赖"
  - "调用顺序由 worker.go 静态文本断言守护（最低保底方案，可在测试不依赖 docker 的环境下跑过）"
  - "containerExpectedDNS 常量化后 verifyDNS 与 EgressConfig.Proxy.DNSServer 解耦；后者保留作为 ProxySpec 透传字段（Plan 01 / Plan 46 仍使用）"
metrics:
  duration: TBD
  tasks_completed: 4
  files_modified: 6
  completed_at: 2026-05-12
requirements_satisfied:
  - BYPASS-DNS-03
  - BYPASS-DNS-04
---

# Phase 45 Plan 02: 容器 DNS 入口锁 + Provider 调用顺序重排 Summary

## One-liner

把 worker 容器 `/etc/resolv.conf` 与 `/etc/nsswitch.conf` 改为只读 bind mount（指向 sing-box tun IP `172.19.0.1` + 禁用 mdns/myhostname/wins），并把 `ContainerProxyProvider.PrepareHost` 拆分为 `PrepareGateway`（worker 创建前起 gateway + 写 DNS 源文件）+ `PrepareHost`（worker 创建后 connect netns + configure routes），保证 worker entrypoint 启动时 tun0 (172.19.0.1) 已监听，DNS 不失败。

## 调用链勘察（Task 1a 输出）

勘察命令输出（在 worktree 内执行，路径相对仓库根）：

```text
$ grep -n "buildCreateArgs|runDocker|buildEgressConfig|provider.PrepareHost|provider.PrepareGateway" internal/runtime/tasks/worker.go | head -40
192:func (w *Worker) buildCreateArgs(...)
351:	args, err := w.buildCreateArgs(request, containerName, hostname)
356:	if err := w.runDocker(ctx, args...); err != nil {      // docker create
360:	if err := w.runDocker(ctx, "start", containerName); err != nil {   // docker start
364:	egressCfg, err := w.buildEgressConfig(ctx, request.HostID)
373:	if err := w.provider.PrepareHost(ctx, spec); err != nil {

$ grep -n "func (p *ContainerProxyProvider)" internal/network/container_proxy_provider.go
26:func (p *ContainerProxyProvider) PrepareHost(...)
166:func (p *ContainerProxyProvider) CleanupHost(...)
171:func (p *ContainerProxyProvider) teardownGateway(...)

$ grep -n "dockerNetworkCreate|dockerRunGateway|waitGatewayHealthy|configureWorkerEgress|gatewayConfigDir|GatewayConfigDir" internal/network/container_proxy_provider.go
62:	configDir := gatewayConfigDir(hostID)
82:	if err := dockerNetworkCreate(...)            // 在 PrepareHost 内
87:	if err := dockerRunGateway(...)               // 在 PrepareHost 内
97:	if err := waitGatewayHealthy(ctx, gwName)     // 在 PrepareHost 内
114:	if err := configureWorkerEgress(...)        // 在 PrepareHost 内
184:	_ = os.RemoveAll(gatewayConfigDir(hostID))    // teardownGateway
194:func gatewayConfigDir(hostID string) string {
```

### 当前 createHost 步骤序号 → 行号 → 函数调用

| # | worker.go 行 | 步骤 | 备注 |
|---|--------------|------|------|
| 1 | 282 | `pullImage` | 镜像拉取，可能耗时 |
| 2 | 284-290 | `containerExists` + `rm -f` | 幂等清理旧容器 |
| 3 | 294-344 | claude-state volume 处理 | Phase 33 D-04/05/06 |
| 4 | 351 | `buildCreateArgs` | 拼出 docker create 参数 |
| 5 | 356 | `runDocker("create", ...)` | docker create worker |
| 6 | 360 | `runDocker("start", containerName)` | **docker start worker → entrypoint 启动** |
| 7 | 364 | `buildEgressConfig` | 从 DB 读出 EgressConfig |
| 8 | 373 | `provider.PrepareHost(ctx, spec)` | **gateway 容器才在这里启动** |
| 9 | 379 | `waitForSSH` | 在 PrepareHost 之后 |

### 当前 PrepareHost 内部所有职责（一个方法内做完）

| # | provider.go 行 | 步骤 |
|---|----------------|------|
| a | 47-58 | extractProxyServer + buildGatewaySingBoxConfig（渲染 sing-box config） |
| b | 60 | `teardownGateway`（清旧 gateway） |
| c | 62-69 | `os.MkdirAll(gatewayConfigDir(hostID))` + 写 `config.json` |
| d | 73-80 | 写两个 rule-set placeholder（Plan 01 引入） |
| e | 82-84 | `dockerNetworkCreate`（建用户自定义 bridge） |
| f | 86-90 | `dockerRunGateway`（含 3 个 ro mount，启动 sing-box） |
| g | 92-95 | `dockerNetworkConnect(bridge, gw)` |
| h | 97-100 | `waitGatewayHealthy` |
| i | 102-105 | `dockerNetworkConnect(netName, worker, workerIP)` |
| j | 107-108 | `docker network disconnect bridge worker` |
| k | 114-117 | `configureWorkerEgress`（路由 + 旧 `echo nameserver 8.8.8.8`） |
| l | 119-123 | `applyWorkerFirewall` |
| m | 125-140 | `verifyWorkerNetwork` |
| n | 147-152 | `dockerNetworkConnect(netName, control-plane)` |

### 勘察结论

**结论 A**：当前顺序 = `buildCreateArgs → docker create → docker start → buildEgressConfig → PrepareHost`。

worker 容器在第 6 步（worker.go:360 `docker start`）就进入 entrypoint —— 但此时 **sing-box gateway 容器尚未启动**（要到第 8 步 PrepareHost 内 `dockerRunGateway` 才起）。如果在第 6 步前已经把 `/etc/resolv.conf` bind mount 接管成 `nameserver 172.19.0.1`，worker entrypoint 内任何 DNS 查询（apt update / claude code init / 解析镜像内 hostname）都会失败，因为 172.19.0.1 还没开始监听。

→ 进入 Task 1b 拆分 PrepareGateway，让顺序变为 `PrepareGateway（含 dockerRunGateway + waitGatewayHealthy + WriteContainerDNSConfig） → buildCreateArgs（含 ro bind mount） → docker create → docker start → PrepareHost（仅 connect + routes）`。

---

## TBD: Task 1b / 2 / 3 输出（待 commit 后回填）

## Task 1b 输出：PrepareGateway 拆分 + createHost 顺序重排

**提交**：`a0db9cf refactor(45-02): split PrepareGateway out of ContainerProxyProvider and reorder createHost`

### 接口契约变化

`internal/network/provider.go` 中 `Provider` 接口新增 `PrepareGateway(context.Context, HostNetworkSpec) error`，三个实现者全部就位：

| Provider                 | PrepareGateway 行为                | 备注                                                  |
| ------------------------ | ---------------------------------- | ----------------------------------------------------- |
| `ContainerProxyProvider` | 完整 gateway 启动 + DNS 源文件写盘 | sidecar gateway 容器模型主路径                        |
| `SingBoxProvider`        | no-op                              | 进程注入模型依赖 worker 容器先在；保留接口对齐        |
| `RoutingProvider`        | 透传 `singbox.PrepareGateway`      | 顶层注入点                                            |

### ContainerProxyProvider 职责拆分

| 阶段             | 函数                                  | 子步骤范围（详见 Task 1a 勘察表）                                                          |
| ---------------- | ------------------------------------- | ------------------------------------------------------------------------------------------ |
| `PrepareGateway` | `container_proxy_provider.go:53-142`  | a~h（写 sing-box config + 2 个 placeholder + DNS 源文件 → 起 gateway + healthy）           |
| `PrepareHost`    | `container_proxy_provider.go:157-236` | i~n（connect netns + 断 bridge + 配 default route + firewall + verify + 控制面接入）       |

PrepareHost 内的 `configureWorkerEgress` **不再** 写 `/etc/resolv.conf`，对应步骤 k 的变更见下文 **tryConfigureWorkerEgress 关键变更**。

### worker.go 调用顺序

`createHost` / `startHost` 两条主路径都重排为：

```text
buildEgressConfig
  → (egressCfg != nil) provider.PrepareGateway(ctx, spec)
    → buildCreateArgs(...) (含 ro bind mount，Task 2 在该函数内注入)
      → runDocker(ctx, "create", ...)
        → runDocker(ctx, "start", containerName)
          → (egressCfg != nil) provider.PrepareHost(ctx, spec)
            → waitForSSH
```

该顺序由 `internal/network/call_order_test.go::TestWorker_CreateHost_CallOrder` 静态文本断言守护（**不带 build tag**，macOS 也能跑），读 `internal/runtime/tasks/worker.go::createHost` 函数体内 5 个标识符的字节位置严格升序。

### B3 cross-plan sync

`gatewayConfigDir(hostID)`（小写）已重命名为导出版 `GatewayConfigDir(hostID)`，acceptance：

```text
$ grep -rn 'gatewayConfigDir\b' internal/
(no output)
```

### tryConfigureWorkerEgress 关键变更

删除 `echo 'nameserver 8.8.8.8' > /etc/resolv.conf` 一行：DNS 入口锁阶段后 worker 容器内 `/etc/resolv.conf` 被 ro bind mount 接管，docker exec 写入会被拒绝。脚本现在只负责 default route 配置。

### 验证（Task 1b）

```text
$ go build ./internal/network/... ./internal/runtime/...    # OK
$ grep -rn 'gatewayConfigDir\b' internal/                    # 0 matches
$ go test ./internal/network/ -run TestWorker_CreateHost_CallOrder
ok   github.com/zanel1u/cloud-cli-proxy/internal/network    0.503s
```

---

## Task 2 输出：bind-mount 注入 + containerExpectedDNS 常量化

**提交**：`76f4c67 feat(45-02): bind-mount /etc/resolv.conf and /etc/nsswitch.conf as read-only into worker container`

### buildCreateArgs 签名扩展

```go
func (w *Worker) buildCreateArgs(
    request agentapi.HostActionRequest,
    containerName, hostname string,
    egressCfg *network.EgressConfig,   // ← 新增第 4 参数
) ([]string, error)
```

当 `egressCfg != nil && egressCfg.Proxy != nil` 时，追加两条 ro bind mount：

```go
gwDir := network.GatewayConfigDir(request.HostID)
args = append(args,
    "-v", gwDir+"/resolv.conf:/etc/resolv.conf:ro",
    "-v", gwDir+"/nsswitch.conf:/etc/nsswitch.conf:ro",
)
```

源文件由 `PrepareGateway` 在 docker create 之前写入 `<DATA_DIR>/gateway/<host_id>/`。守卫条件 `egressCfg != nil && egressCfg.Proxy != nil` 保证 macOS 开发环境（无 sing-box egress）或测试用例不会因不存在的源文件失败。

### internal/network/verify.go 常量化

新增包级常量：

```go
const containerExpectedDNS = "172.19.0.1"
```

`VerifyNetworkIntegrity` 内的 `verifyDNS` 预期值切换为该常量，与 `EgressConfig.Proxy.DNSServer` 字段（描述 sing-box → 上游 DNS）正式解耦：

```go
// Phase 45 Plan 02：容器 /etc/resolv.conf 被 ro bind mount 锁死为 tun0
// (172.19.0.1)，与 EgressConfig.Proxy.DNSServer（gateway → 上游 DNS）解耦
verifyDNS(ctx, prefix, containerExpectedDNS, &result)
```

`firstNetworkError` 在 DNS mismatch 分支也使用常量构建消息与 Metadata，移除原本的 `if expected.Proxy != nil { expectedDNS = expected.Proxy.DNSServer }` 分支。

### 验证（Task 2）

```text
$ go build ./internal/network/... ./internal/runtime/...   # OK
$ go test ./internal/network/ -run TestWorker_CreateHost_CallOrder
ok   github.com/zanel1u/cloud-cli-proxy/internal/network    0.503s
```

---

## Task 3 输出：测试同步 + TestContainerProxy_* 新增

**提交**：本次 Task 3 提交（见末尾）

### 删除 / 修改

| 文件 | 变更 |
|---|---|
| `container_proxy_provider_test.go::TestTryConfigureWorkerEgress_ScriptContainsKeyCommands` | 删除 `echo 'nameserver 8.8.8.8' > /etc/resolv.conf` 期望项；新增反向断言：脚本必须**不含** `> /etc/resolv.conf` 与 `nameserver 8.8.8.8`（防 BYPASS-DNS-03 回归） |
| `container_proxy_provider_test.go::buildWorkerEgressScript` 辅助 | 删除尾部 `echo 'nameserver 8.8.8.8' > /etc/resolv.conf` 行，与生产 `tryConfigureWorkerEgress` 同步 |
| `verify_test.go::TestFirstNetworkError_DNSLeak` | `Metadata["expected_dns"]` 期望从 `10.0.0.1` 改为 `172.19.0.1`（常量值） |
| `verify_test.go::TestFirstNetworkError_DNSLeak_NilProxy` | 原断言「Proxy nil ⇒ expected_dns 空」改为「expected_dns 仍是常量 172.19.0.1」（与常量化语义一致） |

`validate_test.go` 内的 `10.0.0.1` 全部保留：那是 `ProxySpec.DNSServer` 字段语义（透传到 sing-box dns_server），未受本 Plan 影响。

### 新增测试（`internal/network/container_proxy_dns_test.go`，**无 build tag**）

| 用例 | 断言点 |
|---|---|
| `TestContainerProxy_WriteContainerDNSConfig_WritesFiles` | tmpdir 设置 `DATA_DIR` → 调 `WriteContainerDNSConfig("test-host-dns")` → 确认 `resolv.conf` 内容等于 `resolvConfContent` 且首行为 `nameserver 172.19.0.1`；确认 `nsswitch.conf` 内容等于 `nsswitchConfContent`，包含 `hosts: files dns`，且不含 `mdns/myhostname/wins/nis_plus` |
| `TestContainerProxy_ResolvConfBindMount` | 文本断言 `worker.go::buildCreateArgs` 含 `gwDir+"/resolv.conf:/etc/resolv.conf:ro"`；并确认守卫条件 `egressCfg != nil && egressCfg.Proxy != nil` 存在 |
| `TestContainerProxy_NsswitchBindMount` | 文本断言 `worker.go::buildCreateArgs` 含 `gwDir+"/nsswitch.conf:/etc/nsswitch.conf:ro"`，且源路径派生自 `network.GatewayConfigDir(request.HostID)` |

### 附带修复（Rule 3 - Blocking）

`internal/runtime/tasks/worker_password_test.go` 与 `worker_volume_test.go` 中 5 处 `buildCreateArgs` 调用因签名扩展而编译失败，全部追加第 4 参数 `nil` 修复。

### 验证（Task 3）

```text
$ go build ./... && go vet ./internal/network/... ./internal/runtime/...    # OK
$ go test ./internal/network/... ./internal/runtime/... -count=1 -short
ok   github.com/zanel1u/cloud-cli-proxy/internal/runtime         0.224s
ok   github.com/zanel1u/cloud-cli-proxy/internal/runtime/tasks   1.058s
ok   github.com/zanel1u/cloud-cli-proxy/internal/network         0.389s
```

---

## 提交清单

| Task | 提交 | 说明 |
|---|---|---|
| 1a | `00444d5` docs(45-02): record call-chain survey for container DNS entry lock | 勘察 only，read-only |
| 1b | `a0db9cf` refactor(45-02): split PrepareGateway out of ContainerProxyProvider and reorder createHost | B1+B2+B3：PrepareGateway 拆分 / GatewayConfigDir 导出 / call-order 测试 |
| 2 | `76f4c67` feat(45-02): bind-mount /etc/resolv.conf and /etc/nsswitch.conf as read-only into worker container | ro bind mount 注入 + containerExpectedDNS 常量化 |
| 3 | （本次提交） test(45-02): sync DNS assertions and add container DNS bind-mount tests | 测试同步 + TestContainerProxy_* |

---

## 偏离规划文件

- **Rule 3 - Blocking**：Task 2 扩展 `buildCreateArgs` 签名后，`internal/runtime/tasks/worker_password_test.go` 与 `worker_volume_test.go` 共 5 处旧调用编译失败。自动补 nil egressCfg 参数（不影响测试语义，原测试不关心 egress 行为），在 Task 3 提交中一并落地。
- **测试位置**：`TestWorker_CreateHost_CallOrder` 与三个 `TestContainerProxy_*` 都放到**无 build tag** 的独立文件（`call_order_test.go` / `container_proxy_dns_test.go`），保证 macOS 开发机本地也能跑通；linux-tagged 的 `container_proxy_provider_test.go` 只保留指针注释。

---

## Self-Check

- [x] `internal/network/call_order_test.go` 已创建（无 build tag）
- [x] `internal/network/container_proxy_dns_test.go` 已创建（无 build tag）
- [x] `internal/network/container_proxy_provider_test.go` 中旧 `echo nameserver 8.8.8.8` 期望已删除
- [x] `internal/network/verify.go` 含 `containerExpectedDNS = "172.19.0.1"` 常量
- [x] `internal/runtime/tasks/worker.go::buildCreateArgs` 在 `egressCfg.Proxy != nil` 分支注入两条 ro bind mount
- [x] `grep -rn 'gatewayConfigDir\b' internal/` 0 命中（B3 acceptance）
- [x] `go build ./...` 通过
- [x] `go test ./internal/network/... ./internal/runtime/... -count=1 -short` 全部通过
- [x] 提交链 `00444d5 → a0db9cf → 76f4c67 → (Task 3)` 完整

## Self-Check: PASSED
