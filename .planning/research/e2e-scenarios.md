# Cloud CLI Proxy 端到端测试场景设计

> 范围：完整 e2e 套件的「测试场景矩阵 + 测试金字塔 + 可观测性 + 抗 flake」。
> 不包含具体实现代码；leak 攻击向量层由 leak-researcher 覆盖，本文件只补「操作层对抗」。

---

## 1. 执行摘要

下表是建议优先实现的 10 个「最小可信 e2e 用例」(MVS, Minimum Viable Suite)。
排序按「价值 ÷ 实现成本」从高到低，先把它们做绿，再扩展到完整矩阵。

| 序号 | 用例 | 验证的产品承诺 | 估算成本 | 关联模块 |
|------|------|----------------|---------|---------|
| MVS-01 | 首次 `bootstrap` 成功并进入 SSH 会话 | 「一行 curl」黄金路径 | 中 | `cmd/cloud-claude`, `internal/controlplane/http/bootstrap_*`, `tests/smoke/bootstrap.bats` |
| MVS-02 | 容器内 `curl ifconfig.me` 返回绑定的出口 IP | 出口 IP 强约束（核心卖点） | 中 | `internal/network/verify.go`, `internal/network/singbox_provider_linux.go` |
| MVS-03 | 容器内 `dig @1.1.1.1 example.com` 被 tun DNS 接管或被拒绝 | DNS 不可泄漏 | 中 | `internal/network/dns.go`, `verify.go:verifyDNS` |
| MVS-04 | 容器内直连外网 `bash -c 'echo >/dev/tcp/1.1.1.1/80'` 被防火墙拒绝 | 默认拒绝兜底 | 低 | `verify.go:verifyLeakBlocked`, `worker_firewall_linux.go` |
| MVS-05 | 用户认证错误码 (`auth_invalid` / `account_disabled` / `account_expired` / `host_not_found`) 行为正确 | CLI 错误码契约（已有 smoke） | 低 | `tests/smoke/bootstrap.bats` 扩展 |
| MVS-06 | 到期用户的运行容器被 `ExpiryScanner` 自动停止，审计事件落库 | 到期治理 | 中 | `internal/controlplane/scheduler/expiry.go` |
| MVS-07 | 同一出口 IP 被两个不同 Host 双绑时，第二次绑定被拒绝 | 出口 IP 双绑互斥 | 低 | `internal/controlplane/http/admin_bindings.go`, `admin_egress_ips.go` |
| MVS-08 | host-agent 进程强杀后，控制面的健康状态在 30s 内转为 unhealthy；重启后自动恢复 | 控制面 ↔ agent 心跳 | 中 | `internal/agent/server.go`, `internal/broadcast/sse.go` |
| MVS-09 | sing-box 容器崩溃后，用户容器内立即「断网」(出网失败)，而不是「降级直连」(默认拒绝守住) | fail-closed 兜底 | 高 | `singbox_provider_linux.go`, nftables 规则 |
| MVS-10 | 用户在容器内 `echo nameserver 8.8.8.8 > /etc/resolv.conf` 之后，DNS 仍然走 tun（或被防火墙拒绝） | 用户态绕过免疫 | 低 | `worker_firewall_linux.go` UDP/53 规则 |

**关键判断：** MVS-02 / MVS-03 / MVS-04 / MVS-09 / MVS-10 是「网络强约束」的硬主线，必须 100% 自动化，且必须能在 Linux 宿主机上跑（macOS 走 sidecar 镜像时部分判据需要调整）。

---

## 2. 完整场景矩阵

### 2.1 用户旅程视角（黄金路径与近邻路径）

| ID | 场景 | 前置条件 | 操作步骤 | 通过判据 | 关联模块 |
|----|------|---------|---------|---------|---------|
| JRN-01 | 注册 + 首次 curl 入口 | 数据库初始化、管理员已通过后台创建用户 + 出口 IP 绑定 | `printf 'user\npass\n' \| sh bootstrap.sh` | 退出码 0；本地 cloud-claude 二进制就位；进入 ssh 提示符 | `bootstrap_script.go`, `cmd/cloud-claude/main.go` |
| JRN-02 | 第二次重连（已存在容器） | JRN-01 已完成 | 重复执行 curl 入口 | 30s 内复用已有容器，不重建 netns；SSH 直接接通 | `runtime_service.go`, `tasks/ssh_handoff.go` |
| JRN-03 | 容器内执行 `claude code` 首次 OAuth | JRN-01 + 容器正常 | 在 SSH 内运行 claude 引导 | OAuth URL 可点开；回调成功；写入 `~/.claude` 持久卷 | `claude_account_persistent_volume` 迁移 (0014) |
| JRN-04 | 长时间空闲后回到 SSH | 已连接的 SSH 会话空闲 > keepalive 阈值 | 等待后敲一个键 | 连接仍存活；或被自动重连无感知 | `internal/cloudclaude/keepalive*.go`, `reconnect.go` |
| JRN-05 | 到期当天用户登入 | 用户 `expires_at = now() - 1m` | 跑 curl 入口 | 返回 `account_expired`，退出码 12 | `bootstrap_auth.go` |
| JRN-06 | 管理员后台扩容用户期限 | 用户已 expired，容器已被清理 | 后台改 expires_at → +30d；用户重新 curl | 新容器被拉起；SSH 接通；审计事件 `user.renewed` + `host.started` 双落库 | `admin_users.go`, `expiry_test.go` |
| JRN-07 | 管理员手动停止用户容器 | 用户 SSH 会话活跃 | 后台点「停止」 | 任务进入 `tasks` 表；agent 收到 stop；用户会话被切断；UI 状态 → stopped；netns 清理 | `admin_hosts.go`, `internal/agent/server.go` |
| JRN-08 | 切换出口 IP 绑定 | 容器在跑 | 后台改 egress IP；触发 rebind | sing-box 配置热替换或容器重启；容器内出口 IP 校验跟随变更 | `admin_bindings.go`, `routing_provider_linux.go` |
| JRN-09 | 审计事件回查 | JRN-07 已完成 | 后台「事件」页过滤该用户 | 看到完整时间线：bootstrap → host.started → host.stopped；附带 operator/reason metadata | `admin_events.go` |

### 2.2 防护视角（不变量必须不被破坏）

| ID | 场景 | 前置条件 | 操作步骤 | 通过判据 | 关联模块 |
|----|------|---------|---------|---------|---------|
| GRD-01 | 容器内默认拒绝有效 | 容器刚启动、tun 还在握手 | 在 0~5s 窗口内尝试外联 | 全部失败（fail-closed 不开口） | `worker_firewall_linux.go` |
| GRD-02 | 出口 IP 校验：actual != expected 立即报错 | 故意把 sing-box outbound 指向另一个节点 | 跑 `VerifyNetworkIntegrity` | 返回 `ErrEgressIPMismatch`；容器被标记 unhealthy | `verify.go:verifyEgressIP` |
| GRD-03 | DNS 强制走 tun | 容器内 `resolvectl status` / `cat /etc/resolv.conf` | 检查 nameserver | 第一个 NS == `ProxySpec.DNSServer` | `verify.go:verifyDNS` |
| GRD-04 | 出口 IP 双绑互斥 | 已有 binding(host=A, egress=X) | 再创建 binding(host=B, egress=X) | API 返回 4xx；DB 唯一约束触发 | `admin_bindings_test.go`, migration 0002 |
| GRD-05 | 过期容器残留检测 | 触发 ExpiryScanner | scanner 跑完后 | DB users.status=expired；hosts.status=stopping/stopped；无残留 docker 容器；无残留 netns | `expiry.go`, `reconciler.go` |
| GRD-06 | namespace 残留 GC | 模拟 agent 崩溃后重启 | 重启 host-agent | reconciler 把孤儿 netns 全部清理；nftables 表清空 | `reconciler.go`, `provider_linux.go:CleanupHost` |
| GRD-07 | nftables 规则残留检测 | 多次创建/销毁容器后 | `nft list ruleset` | 与「期望规则集（基线 + 当前活跃绑定）」逐行比对，零额外/缺失 | `worker_firewall_linux.go` |
| GRD-08 | 管理端口不可作为出网通道 | 通过 veth 拿到 mgmt0 IP | 容器内 `curl --interface mgmt0 http://1.1.1.1` | 失败（mgmt 子网无默认路由 & 防火墙拒绝） | `namespace.go:InjectManagementVeth` 注释明确这是设计意图 |
| GRD-09 | IPv6 出网阻断（如未启用 v6 隧道） | 容器有 IPv6 地址 | 容器内 `curl -6 ifconfig.me` | 失败 / 超时 | nftables ip6 表必须默认拒绝 |
| GRD-10 | conntrack 不泄漏旧绑定 | 重新绑定出口 IP 后 | 立即出网 | 新连接走新出口；旧 conntrack 项被 flush | `routing_provider_linux.go` |

### 2.3 故障视角（依赖坏掉时表现如何）

| ID | 场景 | 故障注入方式 | 通过判据 | 关联模块 |
|----|------|-------------|---------|---------|
| FLT-01 | sing-box 进程崩溃 | `docker kill <gateway-ctr>` 或 `pkill -9 sing-box` | 容器内立即出网失败；30s 内 control-plane 检测到 unhealthy；自动重启 sing-box 后恢复 | `singbox_provider_linux.go` |
| FLT-02 | 出口节点不可达 | iptables DROP 入口节点出方向 | sing-box 重连退避；容器内连接超时；不降级到直连 | sing-box 行为 + nftables 默认拒绝 |
| FLT-03 | host-agent 重启 | `systemctl restart host-agent` 或在 dev 模式下 `kill -TERM` | 既有用户会话不掉（OpenSSH 进程独立）；新建容器在 agent 恢复后能继续 | `internal/agent/server.go` reconciler |
| FLT-04 | Postgres 短暂不可用 | `docker stop postgres` 持续 10s 后再启动 | 控制面 API 在故障期间返回 5xx；恢复后健康检查转 healthy；无脏数据 | `pgx` 连接池配置 |
| FLT-05 | Docker daemon 重启 | `systemctl restart docker` | 容器随 daemon 起来；host-agent reconciler 把 netns/tun 重新接上；用户能重连 | reconciler.go |
| FLT-06 | 磁盘满 (/var/lib/docker) | `fallocate -l <free-space>` 占满 | 新建容器失败，错误码 `runtime.disk_full`（或类似）；既有容器正常；告警事件落库 | `runtime_service.go` |
| FLT-07 | 镜像拉取超时 | iptables 模拟 registry 慢 | `worker_pull_timeout_test` 覆盖单元层；e2e 验证 task 进入 failed，UI 显示明确原因 | `worker_pull_timeout_test.go` |
| FLT-08 | sing-box 配置非法 | 后台填错 outbound JSON | API 在保存前用 sing-box 自带 validate 拦截；不会落到容器内 | `gateway_singbox_config.go` |
| FLT-09 | tun 设备无法创建（缺失 `/dev/net/tun`） | 在 macOS host 上、未启用 sidecar | 启动报错明确指向 sidecar 缺失；不静默降级 | `provider_factory.go` |
| FLT-10 | 时钟跳变 | `date -s` 改宿主时间 | 过期判定仍按 DB 时间；不出现误判扫描 | `expiry.go` |

### 2.4 并发视角（资源竞态与限额）

| ID | 场景 | 操作步骤 | 通过判据 | 关联模块 |
|----|------|---------|---------|---------|
| CCR-01 | 同时创建 N 个容器 (N=10/50/100) | 并发触发 bootstrap | 所有 bootstrap 要么成功要么明确失败；不出现「成功了但 netns 没接上」的部分态 | `runtime/tasks/dispatcher.go` |
| CCR-02 | 出口 IP 池耗尽 | 池=3，发起 4 个绑定请求 | 第 4 个返回明确错误 `egress.pool_exhausted`；前 3 个不受影响 | `admin_egress_ips.go` |
| CCR-03 | API 限流 | 100 RPS 打 `/bootstrap` | 控制面用 token bucket / per-IP 限流；返回 429；正常用户不受邻居影响 | `controlplane/http/router.go` |
| CCR-04 | 同一用户多端同时登入 | 两台机器同时跑 curl | 两个 SSH 会话都成功；DB 中 user 只有一个 host 记录或允许多 session（产品决定） | `tasks/ssh_handoff.go` |
| CCR-05 | netns 命名冲突 | 短时间内同一 hostID 被复用 | `mgmtSubnetIndex` 哈希碰撞概率可接受；冲突时报错而非静默覆盖 | `namespace.go:mgmtSubnetIndex` |
| CCR-06 | reconciler 与 dispatcher 并发改同一 host | 用户重连同时管理员手动停止 | 最终态收敛到「停止」；不会出现「DB 是 running 但容器没了」 | `reconciler.go` |

### 2.5 对抗视角（操作层绕过尝试，补 leak-researcher 之外）

> leak-researcher 已覆盖 DNS rebinding、WebRTC、IPv6、socket-level bypass 等核心向量；
> 这里只补「用户在 SSH 内能动手做的事」的端到端验证。

| ID | 场景 | 用户操作 | 通过判据 | 关联模块 |
|----|------|---------|---------|---------|
| ADV-01 | 直接改 `/etc/resolv.conf` 指向 8.8.8.8 | `echo 'nameserver 8.8.8.8' > /etc/resolv.conf` | 该 53 端口 UDP/TCP 出方向被防火墙拒绝；或 DNS 仍被 tun 截获（NAT 53 → tun-dns） | `worker_firewall_linux.go` UDP/53 规则 |
| ADV-02 | 安装并启动 systemd-resolved | `apt install systemd-resolved && systemctl start systemd-resolved` | 镜像内不应有 systemd；即使装上也无法绕过 nftables | 用户镜像 Dockerfile |
| ADV-03 | 在容器内跑 `wireguard-go` 嵌套隧道 | 启动 wg-quick 试图建立 udp/51820 出连 | 失败（nftables 只放行 tun 接口的流量） | nftables conntrack mark |
| ADV-04 | tcp over icmp / dns-tunnel 工具（iodine、dnstt） | 启动 iodine client | ICMP echo 受限；DNS 出方向受限；无法建立隧道 | nftables ICMP rate-limit、DNS 出方向规则 |
| ADV-05 | 修改 `/etc/nsswitch.conf` 或 `nss-mymachines` | 改本地解析顺序 | 不影响：所有 udp/53 都被强制走 tun-dns | 同 ADV-01 |
| ADV-06 | 利用容器内 docker.sock / privileged | 检查容器是否能访问宿主 docker | 容器无 docker.sock 挂载；非 privileged；caps 收紧 | 用户容器 Dockerfile + runtime 启动参数 |
| ADV-07 | 在容器内启动 raw socket | `python3 -c 'import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW)'` | 失败：`CAP_NET_RAW` 已移除 | runtime 启动参数 |
| ADV-08 | 容器内创建新的 network namespace 试图逃逸 | `unshare -n bash` | 失败（user-ns 限制）或新 ns 仍受 nftables 约束 | runtime 启动参数 + nftables 规则覆盖范围 |
| ADV-09 | 篡改 `/etc/hosts` 把 control-plane 指向恶意 IP | 改 hosts 之后跑 curl 入口 | 控制面访问走 mgmt veth，路由表硬编码；hosts 无法改变路径 | `namespace.go` mgmt 路由 |
| ADV-10 | 绑定到 mgmt veth 接口对外发包 | `curl --interface mgmt0 https://example.com` | 失败（mgmt 子网无默认网关 + nftables 拒绝） | GRD-08 同源 |

---

## 3. 测试金字塔分层

### 3.1 分层判断标准

| 层级 | 起栈成本 | 适合什么 | 运行频率 |
|------|---------|----------|---------|
| 单元 | 进程内 | 纯逻辑、JSON 解析、状态机、SQL 拼接、字符串校验 | 每次 commit / pre-commit |
| 集成 | host-agent + Docker + Postgres，无 sing-box | DB schema、HTTP 契约、reconciler 状态机、runtime 任务调度 | 每个 PR / push |
| e2e | 全栈 + 真实出口节点 | 网络强约束、用户旅程、跨进程时序、外网真实回连 | nightly + release-gate |

### 3.2 场景按层归类（用上表 ID 引用）

| 层级 | 归属场景 ID |
|------|-------------|
| **单元** | 出口 IP 池分配算法、`mgmtSubnetIndex` 碰撞测试、`outbound_parse_test.go`、`singbox_config_test.go`、错误码映射、`expiry_test.go` 状态机、`bootstrap.bats` 错误码部分 |
| **集成** | JRN-07 / JRN-09 (审计落库)、GRD-04 / GRD-05 (用 fake 网络 provider 替代 sing-box)、CCR-01 (N≤10) / CCR-02 / CCR-03、FLT-04 / FLT-08 / FLT-10、ADV-09 (路由层) |
| **e2e (必须)** | MVS-01~10、GRD-01 / GRD-02 / GRD-03 / GRD-06 / GRD-07 / GRD-08 / GRD-09 / GRD-10、FLT-01 / FLT-02 / FLT-03 / FLT-05 / FLT-06 / FLT-09、CCR-01 (N≥50)、CCR-04 / CCR-05 / CCR-06、ADV-01~ADV-10 |
| **e2e (可选/手工 UAT)** | JRN-03 (OAuth 真实回调)、JRN-04 (弱网长跑，已有 `uat-network-resilience.sh`)、FLT-07 (镜像拉取慢) |

### 3.3 推荐比例与运行节奏

| 层 | 测试条数目标 | 单次运行预算 | 触发时机 |
|----|-------------|-------------|---------|
| 单元 | ~400 | < 60s | 每 commit (pre-push) |
| 集成 | ~80 | < 5min | 每 PR + main 推送 |
| e2e（核心 10 条 MVS）| 10 | < 15min | 每 PR（合并门禁） |
| e2e（完整矩阵）| ~60 | < 60min | nightly + release-gate |

**理由：** 网络相关 e2e 单条平均成本 60~120s（容器拉起 + tun 握手 + 真实 curl 出口节点）。把 MVS 限到 10 条以内是为了保住 PR 反馈 ≤ 15min 的开发者体验。

---

## 4. 可观测性 / 失败诊断

### 4.1 失败时必须自动归档的 artifact 清单

| 类别 | 项 | 采集命令（示意） | 用途 |
|------|----|-----------------|------|
| 应用日志 | control-plane | `docker logs --timestamps cloud-cli-proxy-control-plane` | 看 API 时序、SQL、调度决策 |
| 应用日志 | host-agent | `journalctl -u host-agent --since '-15min'` 或 `docker logs host-agent` | 看 dispatcher / reconciler 决策 |
| 应用日志 | sing-box | `docker logs <gateway-ctr>` | 看握手 / outbound 选择 / DNS 行为 |
| 应用日志 | sshd | `docker exec <user-ctr> journalctl -u ssh` 或读容器内 `/var/log/auth.log` | 看登入失败原因 |
| 网络状态 | nftables 规则 | `nft list ruleset` | 关键证据：默认拒绝是否还在 |
| 网络状态 | netns 列表 | `ip netns list` | 检查 netns 残留 |
| 网络状态 | netns 路由 | `for ns in $(ip netns list \| awk '{print $1}'); do ip -n $ns route; done` | 出口路由完整性 |
| 网络状态 | netns 地址 | `for ns ...; do ip -n $ns addr; done` | mgmt veth + tun 设备状态 |
| 网络状态 | conntrack | `conntrack -L` | 看是否有未预期的直连连接 |
| Docker 状态 | `docker ps -a --no-trunc` | 容器存活状态 |
| Docker 状态 | `docker inspect <each-ctr>` | 启动参数、挂载、网络模式 |
| Docker 状态 | `docker network inspect bridge` | 子网/网关一致性 |
| 抓包（高价值，可选）| pcap on tun | `tcpdump -i tun-<id> -w tun.pcap` | 真实出网流量证据 |
| 抓包 | pcap on host veth | `tcpdump -i mgmt-<id> -w mgmt.pcap` | 管理面是否被滥用 |
| Postgres | 关键表 dump | `pg_dump -t users -t hosts -t egress_ips -t egress_bindings -t events -t tasks` | 状态机收敛性证据 |
| 系统 | 内核日志 | `dmesg --since '-15min'` | OOM、tun 加载失败等 |
| 系统 | 磁盘 | `df -h && du -sh /var/lib/docker/*` | 排查 FLT-06 类问题 |
| 测试本身 | bats / go test 输出 | 默认即有 | 用例断言失败定位 |

### 4.2 业界做法对照

- **testcontainers-go**：失败时调用 `container.Logs(ctx)` 把容器日志写入 `t.Failed()` 分支的 artifact 目录。我们应在 e2e helper 里复用同一模式。
- **k3s e2e**：使用 `_artifacts/<test-name>/` 目录约定，所有节点的 `journalctl` + `crictl` + `kubectl describe` 都打包进去；CI 把整个目录上传为 build artifact。我们对齐这套目录约定。
- **kind**：`kind export logs` 一键导出全部节点日志到指定目录，提供 `--name` 和 `--logs-dir` 两个参数。我们应该提供类似的单入口脚本。

### 4.3 建议实现：`tests/e2e/harness/collect-artifacts.sh`

设计要点（不实现，仅契约）：

```bash
# 调用约定
#   bash tests/e2e/harness/collect-artifacts.sh <output-dir> [--test-name=<name>] [--with-pcap]
#
# 输出目录结构
#   <output-dir>/
#     ├── meta.json               # 测试名、退出码、时间戳、git sha、uname -a
#     ├── logs/
#     │   ├── control-plane.log
#     │   ├── host-agent.log
#     │   ├── gateway-<id>.log
#     │   └── user-<id>.log
#     ├── network/
#     │   ├── nft-ruleset.txt
#     │   ├── netns-list.txt
#     │   ├── netns-routes.txt
#     │   ├── netns-addrs.txt
#     │   └── conntrack.txt
#     ├── docker/
#     │   ├── ps.txt
#     │   ├── inspect-<id>.json    # 每个容器一份
#     │   └── network-bridge.json
#     ├── postgres/
#     │   ├── users.csv
#     │   ├── hosts.csv
#     │   ├── egress_ips.csv
#     │   ├── egress_bindings.csv
#     │   ├── events.csv
#     │   └── tasks.csv
#     ├── pcaps/                    # 仅 --with-pcap 时存在
#     │   ├── tun.pcap
#     │   └── mgmt.pcap
#     └── system/
#         ├── dmesg.txt
#         ├── df.txt
#         └── uname.txt

# 设计约束
#   1. 必须能在测试失败 trap 中调用，本身不能 panic。
#   2. 任何子命令失败都不能阻塞其它项的采集（每条都 || true）。
#   3. 对 pcap 单独开关，默认关（pcap 体积大、有隐私）。
#   4. 不假定能 sudo；nft / ip netns 在无权限时降级为「no permission」占位文件。
#   5. CI 调用方负责把整个目录归档为 zip 上传。
```

测试 harness（Go 或 bats）的失败钩子应统一调用此脚本，并把目录路径打印在断言失败的最后一行，方便人快速 grep。

---

## 5. 抗 flake 模式与本项目风险点

### 5.1 网络 e2e flake 的常见根因

| 根因 | 表现 | 标准缓解 |
|------|------|---------|
| DNS 解析慢/失败 | 第一次 `curl ifconfig.me` 超时 | 用 IP 直连出口节点；或先 warm-up 等 tun DNS ready |
| 容器启动竞态 | `docker run` 返回后立即 exec，进程未就绪 | 显式 wait-for-readiness：探 `ssh -o ConnectTimeout=1` 或 `pgrep sshd` |
| `time.Sleep` 滥用 | 同一用例时快时慢 | 替换为 polling + 条件等待（retry-with-condition） |
| TIME_WAIT 端口耗尽 | 反复重连后 connect refused | 用例之间随机端口；测试前 `sysctl net.ipv4.ip_local_port_range` 扩大 |
| conntrack 表项遗留 | 切换出口后第一个请求仍走旧路径 | 切换后 `conntrack -F`；或在断言前等一个完整 TCP 关闭周期 |
| sing-box 握手抖动 | 第一次出口 IP 校验拿到空字符串 | 出口 IP 校验做 3 次重试，每次间隔 2s，全失败才算失败 |
| 时间假设硬编码 | CI 机器慢时超时 | 所有等待用 `eventually` 风格（最多 N 秒，每 M 毫秒一次） |
| 真实外网依赖 | `ip.me` 临时不可达 | 维护 2~3 个 IP 回显端点轮询；或自建一个最小回显服务 |
| 端口/子网冲突 | 并发用例互相踩 | mgmt 子网用 hostID 哈希（已实现），但要单测哈希碰撞；端口动态分配 |

### 5.2 本项目高风险等待点（结合代码）

| 位置 | 风险 | 建议 |
|------|------|------|
| `internal/network/namespace.go:46` `GetContainerNetNS` 5 次重试，每次 300ms | 容器启动慢于 1.5s 时直接失败 | 暴露重试上限给 e2e 配置；CI 慢机调高 |
| `internal/network/verify.go:62` `curl https://ip.me --max-time 10` | 真实外网依赖，单点 | 改为多端点轮询 |
| `internal/network/verify.go:113` `timeout 3 bash -c 'echo >/dev/tcp/...'` | 3s 在跨洲网络上可能不够 | 保留 3s 但加注释说明这是「期待失败」，超时也算通过 |
| `tests/smoke/bootstrap.bats:8` `POLL_TIMEOUT=3` | mock server 启动慢时 flake | 把 mock 启动改成「ready 探测」而非定时 |
| `scheduler/expiry.go` Scan 频率 | e2e 等待过期事件落库时不可控 | 测试中暴露 trigger 接口，跳过定时器，直接调 `Scan(ctx)` |
| `runtime/tasks/dispatcher.go` 任务轮询 | e2e 等任务完成时 sleep | 加 SSE/channel 通知，让测试用条件等待 |
| `internal/cloudclaude/keepalive*.go` | OS 行为差异（darwin/linux） | macOS e2e 与 Linux e2e 分开，不要混在同一 matrix |
| sing-box outbound 首次握手 | 跨地区节点 RTT 高时第一个请求超时 | 容器启动后跑一次「warm-up」让握手完成，再进入断言阶段 |

### 5.3 通用工程化抗 flake 规范（建议写入 e2e harness）

1. **禁止裸 sleep。** 任何 `time.Sleep` / `sleep N` 都必须包成 `waitFor(cond, timeout, interval)`。
2. **固定 RNG。** 测试涉及哈希/采样的固定 seed；mgmtSubnetIndex 的并发用例用确定性 hostID。
3. **显式同步点。** 控制面要为 e2e 暴露「等待状态收敛」的探针接口（如 `/internal/wait?host=X&status=running`），仅在 dev/test 构建启用。
4. **重试要带条件、要有上限。** 真实外网请求 ≤ 3 次重试，间隔 2s。永远不要无限重试。
5. **失败一次就归档全部 artifact。** 不依赖人 SSH 进去复现。
6. **隔离副作用。** 每个 e2e 用例跑前/跑后都必须自检 nftables / netns / 容器是否回到基线；否则用例之间的污染会以 flake 形式出现。
7. **明确「期待失败」的判据。** `verifyLeakBlocked` 这类「连得通就算坏」的用例，要在用例名里写清楚 `MustFail_*`，避免人误读。

---

## 6. 与其他研究方向的边界

| 别的研究方向 | 本文件是否覆盖 |
|-------------|--------------|
| 协议层 leak（DNS rebinding、WebRTC、IPv6 leak、socket bypass、`SO_BINDTODEVICE` 绕过）| 不覆盖；归 leak-researcher。本文只做「用户操作层对抗」(ADV-01~ADV-10) |
| 性能基线（冷启动 SLO、并发吞吐曲线）| 不覆盖；并发场景只做正确性，不做性能阈值 |
| 安全审计（CVE 扫描、镜像漏洞、依赖供应链）| 不覆盖 |
| UI/UX 验收 | 不覆盖；JRN-09 只验后端审计落库 |

---

## 7. 一句话结论

> e2e 套件的价值集中在 10 个 MVS 用例 + 30 个 GRD/FLT/ADV 矩阵；
> 真正决定它能不能跑稳的是「artifact 自动归档」和「禁止裸 sleep」这两条工程纪律；
> 业务层 flake 风险集中在 `verify.go` 的真实外网依赖与 `namespace.go` 的容器启动重试窗口。
