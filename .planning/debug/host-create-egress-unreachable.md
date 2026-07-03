---
status: investigating
trigger: "新建主机时创建失败：prepare host network after create: [net.egress_unreachable] egress connectivity check failed"
created: 2026-06-18T06:33:33Z
updated: 2026-06-18T07:12:00Z
---

## Current Focus

hypothesis: 远端根因已确认：线上 4.2.6 控制面生成 sing-box config 时把 DNS stub 渲染成 unsupported inbound type "dns"，managed-user 容器内 sing-box 解码配置即 FATAL，导致 127.0.0.1:1080 不可用，PrepareHost 出口探测拿不到 IP
test: 已查看远端容器日志、注入的 config、控制面日志和部署镜像 revision；本地 HEAD 中该段已是 direct inbound，说明线上 latest 镜像包含回归提交
expecting: 发布/回滚到使用 direct DNS inbound 的控制面镜像后，新建主机应不再在 config decode 阶段失败；若后续仍失败，再进入代理连通性分支
next_action: 构建并部署修复版 control-plane/managed-user 镜像，或回滚掉 bd97e0b 中 container_singbox_config.go 的 dns inbound 改动；部署后重建/启动 host 验证
reasoning_checkpoint: null
tdd_checkpoint: null

## Symptoms

expected: 管理后台新建主机后，容器完成创建、网络准备和出口校验，最终进入可 SSH 的运行状态
actual: 管理后台提示新建主机创建失败
errors: "prepare host network after create: [net.egress_unreachable] egress connectivity check failed"
reproduction: 在管理后台执行新建主机
started: 待确认

## Eliminated

## Evidence

- timestamp: 2026-06-18T06:33:33Z
  checked: rg 搜索错误码和错误文本
  found: net.egress_unreachable 定义在 internal/network/errors.go，"egress connectivity check failed" 由 internal/network/verify.go 返回；worker.go 在创建主机后用 "prepare host network after create" 包装该错误
  implication: 失败发生在后端 worker 的网络准备/验证阶段，不是前端渲染或任务状态文案问题

- timestamp: 2026-06-18T06:48:00Z
  checked: internal/runtime/tasks/worker.go 创建路径
  found: docker start 和 connectContainerNetworks 成功后，createHost 调用 provider.PrepareHost；PrepareHost 返回错误时被包装为 "prepare host network after create"
  implication: 容器已至少创建并启动到网络验证阶段，失败点在 PrepareHost 内的 verifier.Verify

- timestamp: 2026-06-18T06:48:00Z
  checked: internal/network/container_proxy_provider.go
  found: v4 单容器架构下 PrepareHost 只做网络验证，不再创建 sidecar 网关；验证目标容器名为 cloudproxy-<host_id>
  implication: 需要查用户容器自身的 sing-box、SOCKS inbound、tun0、nft 和上游代理连通性

- timestamp: 2026-06-18T06:48:00Z
  checked: internal/network/verify.go
  found: Linux 下 DockerVerifier 执行 docker exec <container> curl -x socks5h://127.0.0.1:1080 -4 访问 https://ip.me、https://ifconfig.io、https://ipinfo.io/ip；三轮探测拿不到多数派 IP 时 ActualEgressIP 为空，firstNetworkError 返回 net.egress_unreachable
  implication: 该错误不是 expected_ip 填错的典型表现；expected_ip 错但代理可用通常会返回 egress_ip_mismatch 或自动更新 expected IP

- timestamp: 2026-06-18T06:48:00Z
  checked: 本机 Docker 状态
  found: 本机没有正在运行的 46900b50 相关任务或容器，只有两周前旧容器和已停止控制面
  implication: 无法直接确认用户现场是哪一类外部触发；需要用户在实际宿主机上跑最短排查命令

- timestamp: 2026-06-18T07:12:00Z
  checked: 远端 docker ps / inspect
  found: 失败 host 对应容器为 cloudproxy-62d4ec5a-2a16-48f7-aaef-1a1b3ae6b34d，managed-user/control-plane 均为 ghcr.io/zanel1u/cloud-cli-proxy/*:latest，镜像 label version=4.2.6 revision=05936144b7a137a978a28e19fd4d7a19603b8197
  implication: 线上 latest 指向 4.2.6 回归版本

- timestamp: 2026-06-18T07:12:00Z
  checked: 远端用户容器日志
  found: 容器反复输出 "FATAL decode config at /etc/sing-box/config.json: inbounds[1]: unknown inbound type: dns"，entrypoint 每次都停在 waiting for tun0 前后并被 restart policy 拉起
  implication: sing-box 没有成功加载配置，tun0、127.0.0.1:1080 SOCKS 和 DNS lock 都不会就绪；egress_unreachable 是该 FATAL 的下游症状

- timestamp: 2026-06-18T07:12:00Z
  checked: 远端注入的 /var/lib/cloud-cli-proxy/gateway/<host_id>/config.json（仅检查结构，未记录凭据）
  found: inbounds 为 tun + dns + socks，其中第二项是 {"type":"dns","tag":"dns-in","listen":"127.0.0.1","listen_port":53}
  implication: 该 config 与 sing-box 1.13.3 不兼容；DNS stub 应使用 direct inbound 或其它受支持形态，而不是 inbound type "dns"

- timestamp: 2026-06-18T07:12:00Z
  checked: 远端控制面日志
  found: 每分钟 reconcile 都重试 start_host；PrepareHost 网络验证失败字段为 egress_ip_match=false、actual_egress_ip=""、actual_dns="127.0.0.11"，最终 error_code=egress_unreachable
  implication: 控制面持续重新注入同一错误 config，单独手改当前 config 或重启容器不是根治；必须修控制面配置生成并重新部署

- timestamp: 2026-06-18T07:12:00Z
  checked: 本地 git 对比 deployed revision 05936144 与 HEAD
  found: deployed revision 包含 bd97e0b 的改动，将 buildContainerDNSDirectInbound/direct/dns-direct 改为 buildContainerDNSInbound/dns/dns-in；本地 HEAD 已是 direct/dns-direct，并给 direct outbound 设置 bind_interface=eth0
  implication: 根因是 4.2.6 镜像的配置生成回归；当前本地代码方向已经是应部署的修复形态

## Resolution

root_cause: 线上 4.2.6 控制面生成了 sing-box 不支持的 DNS inbound：`type: "dns"`。managed-user 容器内 sing-box 启动时直接 FATAL `unknown inbound type: dns`，所以 tun0 和 SOCKS5 1080 都无法就绪；worker 的 PrepareHost 只能看到出口探测无结果，最终报 `net.egress_unreachable`。这不是上游代理本身不可达的第一因。
fix: 发布/回滚到使用 `type: "direct"` + `tag: "dns-direct"` 的 DNS stub inbound 的控制面镜像，并保留 direct outbound `bind_interface: "eth0"`；线上不能只手改 config，因为控制面每分钟会重新注入错误配置。
verification: 部署修复镜像后重建或启动 host，确认用户容器日志不再出现 `unknown inbound type: dns`，`docker exec cloudproxy-<host_id> curl -x socks5h://127.0.0.1:1080 -4 https://ip.me` 返回受控出口 IP，控制面日志出现 network verification passed。
files_changed: []
