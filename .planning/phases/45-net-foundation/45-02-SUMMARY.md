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

```
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
