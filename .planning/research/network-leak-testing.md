# 容器全隧道网络的对抗性测试与防泄漏验证方法

> 研究范围：仅围绕 Cloud CLI Proxy 单宿主机方案中"每个用户容器的所有出网流量必须强制走 sing-box tun 隧道、不允许 DNS/WebRTC/IPv6/ICMP/raw socket 旁路泄漏"这一安全承诺，调研业界对抗性测试与自动化断言方法。本文不修改任何项目代码，输出供决策参考。
>
> 假设的攻击者模型：拥有用户容器内的 root shell（用户本人就是潜在攻击者），可以任意修改 `/etc/resolv.conf`、任意发起到任意目的的 socket、加载内核模块以外的任何用户态操作。host 与 worker 之间的边界（nftables、netns、sing-box）被视为可信防御层。

---

## 1. 执行摘要

针对当前实现（worker 容器跑在独立 netns，由 sidecar gateway 容器 sing-box `tun + auto_route` 承接出网，并配合 nftables 双 default-deny 链 + IPv6 全丢规则），**最优先必须落地的 5 个泄漏测试**是：

1. **`tcpdump` 双视角抓包断言** — 在 worker netns 与 host eth0 同时抓 53/443/任意端口，攻击者从容器内主动发往 1.1.1.1:53、8.8.4.4:443、9.9.9.9:853、169.254.169.254、随机 UDP 高端口等"陷阱目的"时，**host eth0 必须只看到指向 sing-box server IP 的密文流量**，绝不能出现裸 53/53 over TLS/IMDS 包。这是唯一可以从外部观察、不依赖容器内自报的断言面。
2. **kill-switch 对抗测试** — 主动 `docker stop` sidecar gateway 或 `ip link set tun0 down`，立即在 worker 容器内执行 `curl --max-time 3 https://1.1.1.1`，必须 100% timeout / connection refused，不能回落到 host 默认路由。这是项目"不允许直连外网"承诺的最关键反向证据。
3. **DoH / DoT 旁路堵塞断言** — 容器内尝试 `curl --resolve cloudflare-dns.com:443:1.1.1.1 https://cloudflare-dns.com/dns-query?dns=...`、`kdig +tls @1.1.1.1 example.com`、连接到 `dns.google:443`、`dns.quad9.net:853` 等已知 DoH/DoT 终结点；流量必须经隧道送出，且从 host 视角看不到任何到 DoH/DoT 公网终结点的明文目的连接（因为隧道会把它当普通 443 流量加密转发，这本身是 OK 的，关键断言是 host 上无 `dst != gateway` 的 UDP/TCP 53/853 包，详见第 2 节）。
4. **IMDS / link-local 全段封堵** — `curl http://169.254.169.254/`、`curl http://169.254.170.2/`、`ping 169.254.1.1` 全部应失败；这是单宿主机但部署在云上时的高危泄漏向量。
5. **IPv6 与 raw socket 反向断言** — `curl -6 https://ipv6.google.com`、`ping6 ::1`、`python -c "import socket; s=socket.socket(socket.AF_INET6, socket.SOCK_DGRAM); s.sendto(b'x', ('2001:4860:4860::8888', 53))"`、以及 `python -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, 1)"` 必须全部失败（前两个由 IPv6 全 drop 规则与 `disable_ipv6=1` 拦截，后一个应在 capability 层就被拒）。

上述五项是 v1 上线前必须有自动化覆盖的"红线测试"。第 4 节给出完整矩阵，第 5 节给出本仓库已有 `verify*.go` / `worker_firewall_linux_test.go` 之外、能直接加进去的具体测试。

---

## 2. 泄漏向量清单

### 2.1 DNS 泄漏

| 子向量 | 威胁描述 | 攻击者手段 | 测试方法 | 通过判据 |
|---|---|---|---|---|
| **明文 53/udp 旁路** | 容器内进程绕过本地 resolver，直接发 UDP 53 到 8.8.8.8、114.114.114.114 等 | `dig @8.8.8.8 example.com`、`nslookup example.com 1.1.1.1`、`python socket sendto :53` | host eth0 / nftables counter 双断言：① host 抓包 `tcpdump -i eth0 -nn 'udp port 53'` 必须只出现 gateway → 隧道目的；② nftables `output` 链上的 DROP 计数在攻击发起后必须有增量 | 攻击者在容器内执行后，host 任意接口上**不应**出现源 IP=worker、目的端口=53 的明文 UDP；并且 dig 应在 1-3 秒内被 nftables drop（无 ICMP unreachable 反馈） |
| **DoT (DNS-over-TLS, 853)** | 进程使用 `kdig +tls`、systemd-resolved DoT、cloudflared 之类，连接 1.1.1.1:853 / dns.google:853 | `kdig +tls @1.1.1.1 example.com`、`openssl s_client -connect dns.google:853` | host 抓 853：tcpdump filter `tcp port 853`；nftables 上要么放进隧道（gateway 走 853 到 sing-box server 实际是密文 443 协议出口，所以理论上 host eth0 不会看到 853），要么完全 drop | host eth0 上**不应**出现 `dst port 853` 的非隧道流量 |
| **DoH (443)** | DoH 走 443，与正常 HTTPS 无法在端口层区分；攻击者用 `curl --resolve cloudflare-dns.com:443:1.1.1.1 https://cloudflare-dns.com/dns-query?...` | 同上 | DoH 的难点是它"长得就是普通 HTTPS"。本项目场景下，sing-box tun + auto_route 会把它劫持进隧道，于是它**经隧道**到达 DoH 终结点是允许的（用户预期就是出网都走代理）。真正要防的是绕过隧道。所以断言变成：host 上**根本不存在 worker → 任意公网 IP 的直连**，无论端口；既然 DoH 也是 TCP 443，落入这条断言天然被覆盖。可选加固：在 sing-box 路由规则里把 cloudflare-dns.com / dns.google / dns.quad9.net / mozilla.cloudflare-dns.com 等 DoH 域名 reject，让用户级 DoH 也不能用 | host 上唯一允许的 `dst` 是 sing-box server 的 IP；任何到 1.1.1.1、8.8.8.8、9.9.9.9、AdGuard、Quad9、NextDNS、OpenDNS 的 443 直连必须 0 字节 |
| **/etc/resolv.conf 漂移** | 攻击者覆写 `/etc/resolv.conf` 指向公网 DNS（已知现在 worker 容器 root 可写） | `echo 'nameserver 8.8.8.8' > /etc/resolv.conf; dig example.com` | resolver 怎么写都不影响 nftables 出口拦截；测试断言是即便写了 8.8.8.8，实际 DNS 解析仍要么走隧道要么失败 | 同 53/udp 旁路用例 |
| **canary 域名 / DoH 自动启用** | Firefox 之类客户端依赖 `use-application-dns.net` canary 决定是否启用 DoH（Mozilla 设计）| 对本项目其实**不适用**（用户用 SSH，不跑浏览器），仅在以后做浏览器场景才需要 | 无需在 v1 测 | n/a |
| **mDNS / LLMNR / SSDP** | 容器内可能尝试 `5353/udp`、`5355/udp`、`1900/udp` 发现局域网设备，泄漏 worker IP / hostname | 启动 `avahi-browse`、`nmblookup`、`systemd-resolved` | host eth0 上不应出现这些目的端口的多播 / 广播 | 同 DNS 主断言；nftables IPv4 output 当前只放 53、gwIP，5353 / 5355 / 1900 已被默认 DROP |

**业界"陷阱 DNS"做法**：测试侧起一台 unbound / dnsmasq（在 host 或测试机上），把任意域名都解析成 honey IP；如果 worker 解析后命中了 honey IP，说明 DNS 走了陷阱。本项目可以用更简单的方式：sing-box 已经把 DNS 全转发，所以"陷阱"放到 sing-box 的 DNS server 配置里即可（任何被解析的查询都会被该 DNS 服务器看见，无需在 worker 里部署陷阱）。

**参考**：
- DNS leak via tcpdump 双视角：[Inspecting DNS Traffic via tcpdump](https://noc.org/learn/inspecting-dns-tcpdump)、[Debug Pod-to-Service Connectivity with tcpdump](https://oneuptime.com/blog/post/2026-02-09-debug-pod-service-connectivity-tcpdump/view)
- DoH canary 机制：[Mozilla canary domain](https://support.mozilla.org/en-US/kb/canary-domain-use-application-dnsnet)、[Wikipedia DNS over HTTPS](https://en.wikipedia.org/wiki/DNS_over_HTTPS)
- DoH 阻断的"分层手法"：[Cisco Umbrella - DoH to block or not](https://umbrella.cisco.com/blog/doh-dns-over-https-to-block-or-not-to-block)、[DNSFilter DoH glossary](https://www.dnsfilter.com/glossary/doh)
- Mullvad 的 DNS / WebRTC leak 检测拆解：[DeepWiki - DNS and WebRTC Leak Detection](https://deepwiki.com/mullvad/browser-extension/4.2-dns-and-webrtc-leak-detection)

### 2.2 IPv6 泄漏

| 子向量 | 威胁描述 | 攻击者手段 | 测试方法 | 通过判据 |
|---|---|---|---|---|
| **默认 IPv6 出网** | 容器有 v6 地址并直连 v6 公网（Docker 在 host 启用 IPv6 时尤其危险） | `curl -6 https://ipv6.google.com`、`ping6 2001:4860:4860::8888` | 当前已有 `--sysctl net.ipv6.conf.all.disable_ipv6=1` + nftables `ip6 filter` 双 chain 全 DROP，再用 `tcpdump -i any 'ip6'` 旁证 | worker netns 内**任何** v6 流量都失败；host 上 `ip6tables`/`nft list ruleset` 显示 cloudproxy6 表 input6/output6 默认 DROP；`curl -6` 必须 timeout |
| **SLAAC / RA 攻击面** | 不可控的 IPv6 路由器通告改写 worker 路由 | 攻击者无法主动注入 RA（需要 L2），但需要确认 worker 不会自动接受 | 启动后检查 `cat /proc/sys/net/ipv6/conf/eth0/accept_ra` 应该为 0（因为 disable_ipv6=1 时 RA 不会被处理）| `disable_ipv6=1` 隐含禁止 RA |
| **临时地址 / privacy ext** | 同上 | 同上 | 同上 | 同上 |
| **回环 v6（`::1`）** | 不构成出网泄漏，但会触发用户工具误认为有 v6 | n/a | n/a | n/a |

**注**：业界有两种取舍——禁用 IPv6（Mullvad、gluetun 默认；本项目选择此路径，简洁可控）vs 让 IPv6 也走隧道（Mullvad 高级模式、AirVPN）。前者更保险，缺点是用户 IPv6-only 的目的服务（如某些教育 / 政府站点）会失败。v1 选择全禁是正确的。

**参考**：
- gluetun IPv6 leak 文档：[gluetun-wiki/setup/advanced/ipv6.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/ipv6.md)、[Gluetun IPv6 issue #70](https://github.com/qdm12/gluetun-wiki/issues/70)
- Mullvad IPv6 处理：[Mullvad VPN Review 2026](https://www.safetydetectives.com/best-vpns/mullvad/) 提到 Mullvad 是少数同时支持 IPv6 不泄漏的 VPN
- sing-box `strict_route` 与 IPv6 行为：[sing-box TUN inbound 文档](https://sing-box.sagernet.org/configuration/inbound/tun/)、[sing-box issue #1123](https://github.com/SagerNet/sing-box/issues/1123)。当前网关侧 `strict_route: false`，但因为 worker netns 已经在路由表与防火墙双层堵死，容器无法独立绕开

### 2.3 WebRTC / STUN 泄漏

| 子向量 | 威胁描述 | 攻击者手段 | 测试方法 | 通过判据 |
|---|---|---|---|---|
| **STUN UDP 直连** | 浏览器或 WebRTC 应用通过 STUN 发现真实出口 IP；STUN 默认 UDP 3478 / TCP 3478 / TLS 5349 | `python -m aiortc.examples ...` 或自建 `stun-client stun.l.google.com 19302` | host 抓 `udp port 3478`、`udp port 19302` 等高 STUN 端口；本项目网络规则已默认 DROP 所有非 gateway 目的 UDP/TCP | 容器内 STUN 包根本到不了公网，自然不会得到真实 IP；UDP 不在防火墙白名单内 |
| **是否值得测** | 业界 WebRTC 泄漏几乎全部针对浏览器场景。本项目 v1 只暴露 SSH 会话，用户不会跑浏览器；且 sing-box tun 兜底把 UDP 也接管。**v1 不必专门跑 aiortc / headless chromium 测试**，但应在防火墙单元测试里增加"UDP 任意目的端口（非 gateway IP）必须被 DROP"用例，以覆盖此类未来的 UDP 攻击面 | n/a | 在 `worker_firewall_linux_test.go` 加一条断言：output 链规则集里不存在"允许任意目的 UDP"的规则；output 默认 policy 是 DROP | nftables `output` chain policy = DROP，UDP 仅 DNS 53 与 gateway-bound 被放行 |

**参考**：
- aiortc 用作 headless STUN 测试：[aiortc on GitHub](https://github.com/aiortc/aiortc)、[Python WebRTC basics with aiortc](https://dev.to/whitphx/python-webrtc-basics-with-aiortc-48id)
- WebRTC leak 原理：[BrowserLeaks WebRTC Test](https://browserleaks.com/webrtc)、[BrowserScan Leak Test](https://browserscan.org/tools/leak-test)、[mullvad.net/check](https://mullvad.net/en/check)
- Mullvad WebRTC 检测内部实现：[DeepWiki Connection Monitoring](https://deepwiki.com/mullvad/browser-extension/4-connection-monitoring)

### 2.4 路由 / kill-switch 验证

| 子向量 | 威胁描述 | 攻击者手段 | 测试方法 | 通过判据 |
|---|---|---|---|---|
| **sing-box 进程崩溃** | gateway 容器 OOM / panic，worker 不应自动回退到 host 默认路由 | `docker kill -s SIGKILL cloudproxy-gw-<id>` | 立刻在 worker 内 `curl --max-time 3 https://1.1.1.1` | 必须 100% timeout 或 `Couldn't connect to server`，不允许任何 2xx |
| **tun 设备被关掉** | sing-box 仍在但 tun 设备 down | `docker exec gw ip link set tun0 down` | 同上 | 同上 |
| **默认路由被改写** | 攻击者用 root 改 worker 的默认路由指向 bridgeGW 而非 gwIP | `ip route replace default via 10.99.x.1` | 改完路由不重要——nftables `output` 仍然 DROP 一切非 gwIP 的目的；可在测试里同时验证：① 路由改完后 `curl --max-time 3 https://1.1.1.1`；② 路由改完后 `curl --max-time 3 https://<gateway_ip>`（应该也会被 DROP，因为 nftables 是按目的 IP 匹配 gwIP，bridgeGW 不在白名单）| `curl` 失败；nftables drop 计数应增加 |
| **gateway 容器被拔网** | 模拟"上游 IP 黑了" | `docker network disconnect bridge cloudproxy-gw-<id>`（保留 cloudproxy-net）| sing-box outbound 失败，worker 通过 tun 发出的所有 TCP 都会 timeout 但**不会回落到 host** | worker `curl` 失败；host 的 eth0 上看不到任何 worker IP 的源流量 |
| **预连接窗口期** | 容器启动早期、防火墙还没装、用户进程已经在跑（gluetun issue #3285 的核心问题）| 难以从 worker 内主动复现，但要审计代码：`configureWorkerEgress` 前是否曾允许默认路由出网？| 检视 `container_proxy_provider.go` 的启动顺序：当前是 disconnect bridge → set default route → apply firewall → verify。中间窗口存在但很短；可以加测试断言：在 firewall apply 之前 worker 不应该被允许执行任何用户代码（v1 不必、v2 必须）| 启动流程内不应有"网络可用、防火墙未就绪、用户脚本已运行"的中间状态 |
| **reboot leak** | Mullvad / ProtonVPN 都在重启时存在短暂泄漏；本项目同样需考虑：宿主机 reboot 后 control-plane / host-agent 起步前，旧的 worker 容器仍在 → 此时若 docker daemon 也自动启动，worker 可能短暂可达旧的失效隧道 | host 系统重启 | 测试：reboot 后 host eth0 抓 5 分钟，检查是否有 worker IP 的流量 | 推荐做法：在 docker run 时给 worker 加 `restart: no`，或在 control-plane 启动前先 `docker rm -f` 所有 cloudproxy-* 容器；可在 systemd unit `ExecStartPre=` 加这一步 |

**业界 kill-switch 测试模式参考**：
- **gluetun**：核心是把消费容器用 `network_mode: service:gluetun`，共享 gateway 网络栈，gateway 一倒消费容器整个失去网络。等价于本项目"worker 在独立 netns、唯一出口是 gateway"。测试方法：`docker restart gluetun` 后立即在消费容器内 curl，必须 hang/timeout。参考 [SimpleHomelab Gluetun guide](https://www.simplehomelab.com/gluetun-docker-guide/)、[DEV.to gluetun killswitch](https://dev.to/gab_builds/how-to-actually-set-up-the-gluetun-vpn-killswitch-49j6)
- **官方 gluetun-wiki test-your-setup**：3 个核心检查（公网 IP、IPv6 leak via ipv6.ipleak.net、DNS leak via dnsleaktest.com）。参考 [gluetun-wiki/setup/test-your-setup.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/test-your-setup.md)
- **vpn-recon "5 层防御"**：含 11 条 test-security.sh 测试，分层断言。参考 [drewburchfield/vpn-recon](https://github.com/drewburchfield/vpn-recon/blob/master/README.md)
- **Mullvad killswitch**：底层是 nftables chain `mullvad_killswitch`，建议测试时 `nft monitor trace` 看 drop 计数。参考 [Mullvad VPN Linux killswitch 介绍](https://sites.google.com/view/mullvad-vpn-on-linux-advanced-/home)、[Privacy Guides 对 killswitch 的怀疑](https://discuss.privacyguides.net/t/please-do-not-completely-trust-the-kill-switch-functions-of-vpn-clients/29350)
- **RTINGS killswitch 测试方法**：三种失败场景下断言无泄漏，参考 [RTINGS Kill Switch Robustness](https://www.rtings.com/vpn/tests/kill-switch-robustness)
- **gluetun pre-connect leak 提案**：iptables 装好之前的 ~15ms 窗口期，参考 [gluetun issue #3285](https://github.com/qdm12/gluetun/issues/3285) —— 本项目要参考其"先开内部隔离子网，等 sing-box 健康再打开外网"的思路

### 2.5 协议层 / 其他低层泄漏

| 子向量 | 威胁描述 | 攻击者手段 | 测试方法 | 通过判据 |
|---|---|---|---|---|
| **raw socket / `SOCK_RAW`** | `CAP_NET_RAW` 默认会给 Docker 容器开，允许伪造任意包 / 透明代理。容器内可以构造任意源地址的 UDP 包绕路由 | `python -c "import socket; s=socket.socket(socket.AF_INET, socket.SOCK_RAW, 1)"`、`hping3` | ① `cat /proc/1/status \| grep Cap`、用 `capsh --decode` 确认 `NET_RAW` 是否被 drop；② 即便 raw socket 能创建，nftables `output` 仍按目的 IP 匹配，应将其 drop | 推荐项目 docker run 加 `--cap-drop=NET_RAW`；测试里断言 `/proc/1/status` 的 CapEff 不含 NET_RAW |
| **ICMP echo (`ping`)** | 默认 `ping_group_range=0 1000000000` 允许任何用户 ping，但 raw 不需要；ICMP 不在 nftables 白名单 | `ping -c1 -W2 8.8.8.8` | nftables `output` 链默认 DROP，ICMP 协议 53 端口规则不含，所以被 DROP | `ping` 必须超时 / `Operation not permitted` |
| **SCTP / DCCP / 任意 L4** | 容器内 `python` 起 SCTP socket 直连公网 | `python -c "import socket; s=socket.socket(socket.AF_INET, socket.SOCK_STREAM, 132)"`（SCTP 协议号 132）| nftables `output` 链按 `meta l4proto` 只放 UDP+TCP 53、TCP+UDP to gwIP；SCTP 协议号 132 完全不在白名单 | 同 ping，0% 成功率 |
| **UDP 任意高端口** | QUIC（443/UDP）、TURN、自定义 UDP 隧道 | `nc -u -w2 1.1.1.1 12345 < /dev/zero` | host eth0 抓 `udp and not host <gw_ip>` 应空 | 0 包到达 host eth0 |
| **链路本地 169.254.x.x** | 云元数据 (`169.254.169.254`、ECS `169.254.170.2`)、Azure IMDS（同 169.254.169.254 + Metadata 头）；AWS IMDS 还有 IPv6 `fd00:ec2::254` | `curl -m2 http://169.254.169.254/latest/meta-data/`、`curl -m2 http://169.254.170.2`、`curl --resolve metadata:80:169.254.169.254 http://metadata/` | nftables output 不放 link-local 任何端口；强烈建议显式加一条 `output ip daddr 169.254.0.0/16 drop` 在白名单之上做"安全网"（即便有人误改白名单也兜底）| `curl` timeout；nftables drop 计数 +1 |
| **DHCP / DHCPv6 (`67/68`、`546/547`)** | 容器 dhclient 发包 | `dhclient eth0`、`busybox udhcpc -i eth0` | 同上，UDP 67/68 不在白名单 | 失败 |
| **PTP / NTP / 各类时间协议** | UDP 123 等 | `ntpdate pool.ntp.org` | UDP 123 不在白名单（如果以后要给容器同步时间，必须显式加规则并明确该规则会让 123/udp 经隧道出网）| 失败 |
| **eBPF / XDP / nft 自身被绕过** | 容器内若有 `CAP_NET_ADMIN` 可以装载自己的 nft 规则覆盖；当前 worker 容器**不能**有 NET_ADMIN | 审计：`docker inspect` 的 `HostConfig.CapAdd / CapDrop` | 应该 drop `NET_ADMIN`；测试断言 `/proc/1/status` 的 CapBnd 不含 NET_ADMIN | 容器 NET_ADMIN 必须不存在 |

**参考**：
- AWS IMDS 泄漏 + 防御：[AWS IMDS docs](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html)、[Cloud Metadata Abuse by UNC2903 (Google Cloud)](https://cloud.google.com/blog/topics/threat-intelligence/cloud-metadata-abuse-unc2903/)、[Datadog Security Labs - Securing IMDS](https://securitylabs.datadoghq.com/articles/misconfiguration-spotlight-imds/)、[SANS Cloud IMDS](https://www.sans.org/blog/cloud-instance-metadata-services-imds-)、[Sysdig LMDeploy honeypot writeup](https://webflow.sysdig.com/blog/cve-2026-33626-how-attackers-exploited-lmdeploy-llm-inference-engines-in-12-hours)
- 容器 capability hardening：[Snyk - Drop default capabilities](https://learn.snyk.io/lesson/container-does-not-drop-all-default-capabilities/)、[Kyverno drop-cap-net-raw](https://kyverno.io/policies/best-practices/require-drop-cap-net-raw/require-drop-cap-net-raw/)、[Datadog Security Labs Container capabilities](https://securitylabs.datadoghq.com/articles/container-security-fundamentals-part-3/)、[antiTree containers using ping without CAP_NET_RAW](https://www.antitree.com/2019/01/containers-using-ping-without-cap_net_raw/)
- Docker `ping_group_range` 默认行为：参见 antiTree 文章，提示 drop `NET_RAW` 并不一定阻止 ping，要双管齐下

### 2.6 测试断言模式

#### 正向断言（出网必须成功并到指定 IP）
1. **公网 IP 自报**：`curl --max-time 8 -4 https://ip.me`、`https://ifconfig.io`、`https://ipinfo.io/ip` 三个独立源全部应返回**预期 egress IP** 字符串；若任一返回不同 IP 即视为出口归属漂移。当前 `verify.go` 只用了 `ip.me`，建议加冗余源。
2. **DNS 解析必须经隧道**：在 sing-box 一侧（gateway）日志里观察是否能看到查询；或在 worker 里 `dig +short example.com` 必须返回结果（说明 DoH 上游通），且解析路径走 sing-box 的 `dns-remote` 出站。
3. **预期 DNS 服务器**：`grep nameserver /etc/resolv.conf` 必须 == `expected.Proxy.DNSServer`。当前 `verify.go` 已有，保留。

#### 反向断言（必须失败）
4. **直连公网 80/443 IP 段（绕开 DNS）**：`bash -c 'echo > /dev/tcp/1.1.1.1/80'` 必须失败。当前 `verify.go` 有此项。建议把目标扩展为多组 IP（1.1.1.1、8.8.8.8、9.9.9.9、169.254.169.254、AWS 元数据），分别独立断言以便快速定位是哪个规则没生效。
5. **UDP 公网直连**：`echo x \| nc -u -w2 8.8.8.8 12345` 必须失败。
6. **link-local**：`curl -m2 http://169.254.169.254/`、`curl -m2 http://169.254.170.2/` 必须失败。
7. **IPv6**：`curl -6 -m3 https://ipv6.google.com`、`ping6 -c1 -W2 2001:4860:4860::8888` 必须失败。
8. **raw socket / ICMP**：`ping -c1 -W2 8.8.8.8` 必须失败；`python -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, 1)"` 应抛 `PermissionError`。

#### 观察点
| 观察点 | 工具 | 用于断言 |
|---|---|---|
| **worker netns 内** | `tcpdump -i eth0` 经 `nsenter` 进 worker 抓 | 看 worker 主动发了哪些包；用于"是否真的发起了攻击" |
| **gateway 容器内** | `docker exec gw tcpdump -i tun0`、`docker exec gw tcpdump -i eth0` | 看 sing-box 是否真的接管了流量 |
| **host eth0 / mgmt 接口** | `tcpdump -i <wan> 'not host <singbox_server_ip>'` | 黄金断言：除了 sing-box 上游服务器，host 上不应有 worker 引发的任何流量 |
| **nftables drop counter** | `nft list ruleset -a`，或在 chain 上加 `counter` 表达式 | drop 计数器在攻击发起后必须 +1；用计数器是比抓包更稳定的断言面 |
| **conntrack** | `conntrack -L` 在 worker netns 内 | 看 conntrack 表里是否记录了"应该被 drop 的连接被 NEW 了"|
| **`ss -tuln`** | 在 worker netns 内列监听 | 主要用来确认 sshd 监听 :22；不是泄漏的主要采证点 |

**把上面写成稳定 Go test 的关键**：用 `runtime.LockOSThread()` + `netns.New()`（已经在 `worker_firewall_linux_test.go` 用了）创建临时 netns，然后用 Go 的 `nftables` 客户端直接读 chain policy 与 rule expression，而**不**靠跑 `tcpdump` 子进程。抓包测试只用于 BATS / shell-based 集成测试（已有 `tests/smoke/bootstrap.bats` 起骨架）。

---

## 3. 参考实现摘录与链接

### 3.1 sing-box 自身的防泄漏开关
- **`strict_route: true`**：当 `auto_route` 开启时，限制非 tun 接口的 53 端口出网，并在 Windows 装 WFP 过滤、Linux 配合 `auto_redirect` 改写 SO_BINDTODEVICE 流量。本项目 gateway 配置当前 `strict_route: false`（见 `gateway_singbox_config.go:33`），靠 worker 容器内独立的 nftables + 路由表来兜底，理论上效果等价。但建议补一份评估文档：是否在 gateway 侧也开 `strict_route`，多一层保险。
  - 参考：[sing-box TUN inbound](https://sing-box.sagernet.org/configuration/inbound/tun/)、[sing-box changelog 关于 strict_route 的演进](https://sing-box.sagernet.org/changelog/)、[GitHub issue #2707 strict_route inconsistency](https://github.com/SagerNet/sing-box/issues/2707)

### 3.2 gluetun 测试套件抽出可借鉴的断言
```sh
# 公网 IPv4
docker run --rm --network=container:gluetun alpine:3.22 \
  sh -c "apk add wget && wget -qO- https://ipinfo.io"
# IPv6 leak (本项目预期失败)
docker run --rm --network=container:gluetun alpine:3.22 \
  sh -c "apk add curl && curl -6 --silent https://ipv6.ipleak.net/json/"
# kill switch：重启 gateway 后立即测试
docker restart cloudproxy-gw-<id>; sleep 1
docker exec cloudproxy-<id> curl --max-time 3 https://1.1.1.1   # 必须失败
```
来源：[gluetun-wiki/setup/test-your-setup.md](https://github.com/qdm12/gluetun-wiki/blob/main/setup/test-your-setup.md)、[gluetun-wiki advanced ipv6](https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/ipv6.md)、[gluetun deepwiki troubleshooting](https://deepwiki.com/qdm12/gluetun-wiki/6-troubleshooting)。

### 3.3 vpn-recon test-security.sh 的 11 项检查
分层结构：Kill switch → Pre-flight IP check → Visual confirmation → Fail-hard curl timeouts → Network isolation。每一层都有独立的 boolean 断言。建议本项目把验证脚本结构化成 5 层，与 Phase verify 阶段绑定。来源：[drewburchfield/vpn-recon](https://github.com/drewburchfield/vpn-recon/blob/master/README.md)。

### 3.4 Mullvad killswitch 在 Linux 的 chain 结构
```
# nft list ruleset 输出
table inet mullvad_killswitch {
  chain output {
    type filter hook output priority filter;
    policy drop;
    ct state established,related accept
    oifname "wg-mullvad" accept
    oifname "lo" accept
    ...
  }
}
```
完全等价于本项目 `cloudproxy` 表的 output 链。来源：[Mullvad VPN Linux Killswitch](https://sites.google.com/view/mullvad-vpn-on-linux-advanced-/home)。

### 3.5 WireGuard fwmark killswitch pattern
```
PostUp = iptables -I OUTPUT ! -o %i -m mark ! --mark $(wg show %i fwmark) \
  -m addrtype ! --dst-type LOCAL -j REJECT
```
这种用 fwmark 区分"出隧道流量 vs 应被 drop"的模式比按目的 IP 白名单更鲁棒；不过本项目 sing-box 不天然写 fwmark，沿用目的 IP 白名单是合理的。来源：[Arch Linux Forums VPN killswitch](https://bbs.archlinux.org/viewtopic.php?id=239644)。

### 3.6 tailscale natlab —— 纯内存网络拓扑模拟
`tailscale.com/tstest/natlab` 提供 `Machine` / `Firewall` / `NATType` 抽象，可以**完全不用 root、不用 VM** 模拟"主机 + NAT + 防火墙"的网络拓扑，测包是否被 drop。对本项目落地启示：
- v1 可以**不**引入这层，因为本项目的核心防御都依赖真实 Linux netns + nftables，模拟不可信。
- 但 v2+ 想做"单元测试覆盖控制面对网络故障的反应"时，natlab 是值得借鉴的 API 设计。
- 参考：[pkg.go.dev/tailscale.com/tstest/natlab](https://pkg.go.dev/tailscale.com/tstest/natlab)、[tailscale issue #586](https://github.com/tailscale/tailscale/issues/586)

### 3.7 IMDS hardening 业界做法
- AWS IMDSv2 + hop limit = 1：让 SSRF 拿不到 token、容器（默认 hop=2）也拿不到。这是云上跑本项目时必须配置。
- 容器层 host-based 防御示例（Mandiant/Google）：
  ```
  iptables -A OUTPUT -m owner --uid-owner root -d 169.254.169.254 -j ACCEPT
  iptables -A OUTPUT -d 169.254.169.254 -j REJECT
  ```
- Stratus Red Team `aws.credential-access.ec2-steal-instance-credentials` 可用作端到端验证。
- 参考：[Datadog Securing IMDS](https://securitylabs.datadoghq.com/articles/misconfiguration-spotlight-imds/)、[Hacking The Cloud SSRF IMDS](https://hackingthe.cloud/aws/exploitation/ec2-metadata-ssrf/)、[Qualys IMDSv1 risks](https://blog.qualys.com/vulnerabilities-threat-research/2024/09/12/totalcloud-insights-unmasking-aws-instance-metadata-service-v1-imdsv1-the-hidden-flaw-in-aws-security)、[Elastic IMDS API detection](https://www.elastic.co/guide/en/security/8.19/unusual-instance-metadata-service-imds-api-request.html)

---

## 4. 防护功能测试矩阵

下表是建议的"红线测试矩阵"，按"触发点 → 验证手段 → 失败诊断"组织。Y = 当前已有 / 接近，需补强；N = 当前空白，建议加。

| 测试名 | 触发点（容器内攻击）| 验证手段（断言面）| 失败时的诊断方式 | 现状 |
|---|---|---|---|---|
| egress-ipv4-match | `curl -4 https://ip.me`、`https://ifconfig.io`、`https://ipinfo.io/ip` | 三源返回必须 == 预期 egress IP | 任一不一致：① 检查 sing-box outbound 配置；② 检查 sing-box server 实际归属；③ 如果有源失败，DNS 或上游链路问题 | Y（verify.go 单源，建议扩到 3 源）|
| dns-resolv-conf | `cat /etc/resolv.conf` | nameserver 列表 == proxy.DNSServer | 不一致：configureWorkerEgress 写文件失败 | Y |
| dns-leak-clearport-udp | `dig @8.8.8.8 example.com`、`nslookup example.com 1.1.1.1` | host eth0 抓 `udp port 53 and not host <gw_server>` 必须空；worker 内 `dig` 必须 timeout（因为 nftables drop）| host 抓到包 → nftables output 链 53 规则错误；worker dig 成功 → 路由 + 防火墙双失效 | **N**（需新增） |
| dns-leak-dot | `kdig +tls @1.1.1.1 example.com` | host eth0 抓 `tcp port 853 and not host <gw_server>` 必须空 | 同上 | **N** |
| dns-leak-doh | `curl --resolve cloudflare-dns.com:443:1.1.1.1 https://cloudflare-dns.com/dns-query?...` | host eth0 抓 `dst not host <gw_server> and port 443` 必须空 | 出现到 1.1.1.1:443 直连 → output 默认 policy 不是 drop，或 gwIP 规则太宽 | **N** |
| direct-tcp-public | `echo > /dev/tcp/1.1.1.1/80`、对 8.8.8.8:443、9.9.9.9:443 各做一遍 | curl 必须失败 | 任意一条成功 → output 默认 policy 或 ct established 规则错误 | Y（已有 1.1.1.1:80 单点，建议扩多目的）|
| direct-udp-public | `nc -u -w2 8.8.8.8 12345 < /dev/zero` | host eth0 抓应空 | 出现包 → output UDP 白名单太宽 | **N** |
| link-local-imds | `curl -m2 http://169.254.169.254/`、`curl -m2 http://169.254.170.2/` | curl 必须 timeout / refused | 任一返回 200 → 必须立刻新增显式 `ip daddr 169.254.0.0/16 drop` 规则 | **N**（建议加显式拒绝规则与对应测试）|
| ipv6-disabled | `curl -6 -m3 https://ipv6.google.com`、`ping6 -c1 -W2 2001:4860:4860::8888` | 全部失败 | 任一成功 → disable_ipv6 sysctl 未生效 or ip6 nftables 规则错误 | 部分 Y（已有 ip6 table DROP，但缺反向集成测试断言）|
| icmp-blocked | `ping -c1 -W2 8.8.8.8` | 必须失败 | 成功 → output 链漏放或 ICMP 协议在白名单里 | **N** |
| raw-socket-denied | `python -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, 1)"` | 必须 `PermissionError` | 成功 → CAP_NET_RAW 未 drop | **N**（建议 docker run 加 `--cap-drop=NET_RAW` 后再加测试）|
| sctp-denied | `python -c "import socket; socket.socket(socket.AF_INET, socket.SOCK_STREAM, 132).connect(('1.1.1.1', 80))"` | 必须失败 | 成功 → nftables 按 l4proto 应自动 drop（白名单仅 UDP 53、TCP 53、TCP+UDP to gwIP），但要测试覆盖确认 | **N** |
| killswitch-gateway-down | `docker kill -s SIGKILL cloudproxy-gw-<id>; sleep 1; docker exec worker curl -m3 https://1.1.1.1` | curl 必须失败 | 成功 → worker 路由或防火墙允许了非 gateway 流量；nft drop counter 应同时增 | **N**（最关键的红线测试，必须加） |
| killswitch-tun-down | `docker exec gw ip link set tun0 down; docker exec worker curl -m3 https://1.1.1.1` | 同上 | 同上 | **N** |
| killswitch-route-rewrite | `docker exec worker ip route replace default via 10.99.x.1; docker exec worker curl -m3 https://1.1.1.1` | curl 必须失败 | 成功 → output 白名单按目的 IP 漏放 bridgeGW（当前实现实际允许了 bridgeGW input，但 output 没明显放 bridgeGW，需要测试再确认）| **N** |
| reboot-leak | host 重启后 5 分钟内 host eth0 抓包 | 不应有 worker IP 的源流量 | 出现包 → control-plane 启动前残留容器；建议加 systemd `ExecStartPre=docker rm -f cloudproxy-*` | **N**（运维侧 SOP） |
| capability-audit | `cat /proc/1/status \| grep Cap` | CapEff、CapBnd 不含 NET_RAW、NET_ADMIN、SYS_ADMIN | 含 → docker run 参数缺 --cap-drop | **N**（v1 前必须审计 worker 启动参数）|
| firewall-counter-increment | 每次反向断言失败前后对比 `nft list ruleset -a` 的 counter | 每条反向断言执行后对应 chain 的 drop counter 应 +N | counter 不增 → nftables 规则根本没匹配上，可能匹配字段错误 | **N**（建议在 nftables 规则里全部带 `counter` 表达式，便于断言） |
| metadata-egress-trap | sing-box 一侧捕获 DNS 解析的所有目标；若出现 `metadata.google.internal`、`instance-data` 等敏感名 | sing-box log + 规则 reject | 出现 → 业务层 SSRF，但本项目不必拦截（用户自愿）| 可选 |

---

## 5. 本项目落地建议

### 5.1 现在就能加进 `internal/network/*_test.go` 的（不需要起完整栈）

这些都是单元测试级别，复用 `worker_firewall_linux_test.go` 已经验证可用的 `newTestNetNS` + `nftables.New(WithNetNSFd)` 模式：

1. **`TestApplyWorkerFirewallRules_OutputPolicyIsDrop`** —— 显式断言 cloudproxy IPv4 output chain 的 policy 是 `ChainPolicyDrop`（防 regression：避免有人某次重构把 policy 改成 Accept 还能跑通其他用例）。
2. **`TestApplyWorkerFirewallRules_NoWildcardOutput`** —— 遍历 output 链所有 rule，断言不存在"出向 ACCEPT 且 dst IP 不在 `[gwIP]`、目的端口不在 `[53]`"的规则。可以通过解析 `expr.Any` 切片实现。
3. **`TestApplyWorkerFirewallRules_NoLinkLocalAccept`** —— 断言 cloudproxy 表里没有任何放行 169.254.0.0/16 的规则；并建议**新增**一条显式 `ip daddr 169.254.0.0/16 drop` 在白名单之前（即便不必要，做安全网），同时新增 `TestApplyWorkerFirewallRules_ExplicitLinkLocalDrop`。
4. **`TestApplyWorkerFirewallRules_IPv6OutputOnlyLo`** —— 当前已经有，但建议把"只允许 lo"做成可枚举的断言：遍历 output6 所有 rule，断言仅一条 iif=lo accept，policy=drop。**已部分有**（`TestApplyWorkerFirewallRules_IPv6Rules` 检验数量为 1），保留。
5. **`TestApplyWorkerFirewallRules_RulesHaveCounters`** —— 建议把 nftables 规则全加 `&expr.Counter{}`，单测断言每条 rule 至少含一个 counter expr，方便集成测试用 `nft list ruleset` diff counters。需要先改实现（在 `worker_firewall_linux.go` 的 `addOifAcceptRule` 等 helper 里加 counter expr）。
6. **`TestVerifyResult_AllChecks_ExpandedTargets`** —— 把 `verify.go::verifyLeakBlocked` 的目标列表参数化（当前硬编码 1.1.1.1:80），加入 8.8.8.8:443、169.254.169.254:80、9.9.9.9:443，每个独立报告。VerifyResult 增加 `LeakTargetsBlocked map[string]bool`。
7. **`TestVerifyResult_DNSCheck_ExpectMultipleNameservers`** —— 当前只校验第一行 `nameserver`；建议遍历所有 nameserver 行，全部都必须 == 预期值，避免 `nameserver 10.x.x.x` + `nameserver 8.8.8.8` 同时存在但只检查第一行漏检。
8. **`TestGatewaySingBoxConfig_DNSStrategyIPv4Only`** —— `gateway_singbox_config_test.go` 已经覆盖大头，建议再加：sing-box JSON 解析后断言 `dns.strategy == "ipv4_only"`，防止有人改成 `prefer_ipv4` 导致解析到 IPv6 时走回外部。
9. **`TestGatewaySingBoxConfig_RouteHasHijackDNS`** —— 断言 `route.rules` 里一定有 `port: 53` + `action: hijack-dns`，否则 sing-box 不会把 worker 发给 53 的明文 DNS 劫持进自己的 dns 出站。
10. **`TestApplyWorkerFirewallRules_CapabilityAudit`** —— 单独写一个 helper 检查（结合 v1 上线前对 worker docker run 参数的代码审计）：worker 容器启动时必须包含 `--cap-drop=NET_RAW`、`--cap-drop=NET_ADMIN`（除非启动脚本里有兜底说明）。当前 `internal/network/container_proxy_provider.go` 没出现 worker 的 docker run 参数，需要去看 host-agent 的容器启动模块再补这条测试。

### 5.2 需要起完整栈（Linux + Docker + sing-box）才能测的

放在 `tests/smoke/` BATS 或新建 `tests/leak-probe/`：

11. **`leak-probe-dns-udp-direct.bats`**：起完整栈 → 容器内 `dig @8.8.8.8 example.com -t A +time=2 +tries=1` → 应失败；host 抓 `udp port 53` 应空。
12. **`leak-probe-dot.bats`**：`kdig +tls @1.1.1.1 example.com` → 失败。
13. **`leak-probe-icmp.bats`**：`ping -c1 -W2 8.8.8.8` → 失败。
14. **`leak-probe-imds-aws.bats`** 与 **`leak-probe-imds-ecs.bats`**：分别对 169.254.169.254 与 169.254.170.2。
15. **`killswitch-gateway-kill.bats`**：`docker kill -s SIGKILL cloudproxy-gw-<id>`，3 秒内 curl 必须失败；sing-box 重新拉起后，验证 worker 自愈或保持 drop。
16. **`killswitch-tun-down.bats`**：`docker exec gw ip link set tun0 down`。
17. **`reboot-leak.bats`**：手动用例（无法自动化重启 CI runner），但写明 SOP 即可。
18. **`pcap-assert.bats`**：在 host eth0 上 `tcpdump -i eth0 -w out.pcap` 跑 5 秒，期间在 worker 容器内主动制造 10 种攻击；测试后 `tcpdump -r out.pcap 'not host <singbox_server_ip>'` 输出必须为空，否则报告哪个攻击穿透了。这是最强的"黑盒"断言。

### 5.3 sing-box 配置侧建议

19. 考虑给 gateway sing-box tun inbound 加 `strict_route: true`：可以多一层 sing-box 内部的安全网（Linux 下会改写 SO_BINDTODEVICE 流量），代价是 sing-box 自身需要更多权限，并可能与已有的 nftables 规则发生 priority 冲突。建议先做 spike：起测试栈，开 `strict_route` 后跑完整 leak probe，看是否破坏现有流量并量化收益。
20. 在 sing-box 的 `route.rules` 里加一条 `domain_suffix` 拒绝列表：`cloudflare-dns.com / dns.google / dns.quad9.net / mozilla.cloudflare-dns.com / dns.adguard.com / dns.nextdns.io` 这类公共 DoH 终结点 → 让攻击者就算能 reach 隧道，DoH 域名也不能解析；与防火墙形成纵深。仅作可选加固。

### 5.4 容器启动参数侧建议

21. worker 容器：`--cap-drop=ALL --cap-add=CHOWN --cap-add=SETUID --cap-add=SETGID --cap-add=SYS_CHROOT --cap-add=DAC_OVERRIDE`（OpenSSH 服务所需），**显式不要** NET_RAW / NET_ADMIN / SYS_ADMIN。
22. `--security-opt=no-new-privileges`。
23. `--sysctl net.ipv6.conf.all.disable_ipv6=1`（已有，保留）。
24. `--sysctl net.ipv4.conf.all.rp_filter=1`（反向路径过滤，防 spoof）。
25. 不要给 worker 任何 `--privileged`、`/dev` 挂载（除非必要）。

### 5.5 持续运营建议

26. nftables 所有规则带 `counter`，host-agent 周期性把 drop counter delta 上报；任何 worker 的 drop counter 持续上涨意味着用户在尝试某种泄漏，可以用来做 anomaly detection。
27. 在 control-plane 启动前 `ExecStartPre=` 一次性 `docker rm -f cloudproxy-* && nft delete table inet cloudproxy && nft delete table ip6 cloudproxy6 || true`，避免 reboot 后残留容器形成 race。
28. `verify.go::VerifyNetworkIntegrity` 现在只在创建容器后跑一次；建议加一个"定期 reverify"循环（host-agent 每 N 分钟跑一次），把 drop / route 漂移当 incident 上报。

---

## 6. 引用清单（按节展开版本见各节链接）

### 业界 VPN / 防泄漏项目
- gluetun 主仓与 wiki: https://github.com/qdm12/gluetun ，https://github.com/qdm12/gluetun-wiki/blob/main/setup/test-your-setup.md ，https://github.com/qdm12/gluetun-wiki/blob/main/setup/advanced/ipv6.md
- gluetun pre-connect isolation 提案：https://github.com/qdm12/gluetun/issues/3285
- vpn-recon（5 层防御 + test-security.sh）：https://github.com/drewburchfield/vpn-recon/blob/master/README.md
- torrent-vpn-stack：https://github.com/ddmoney420/torrent-vpn-stack
- DyonR/docker-qbittorrentvpn iptables killswitch：https://github.com/DyonR/docker-qbittorrentvpn/blob/master/README.md
- Mullvad killswitch on Linux：https://sites.google.com/view/mullvad-vpn-on-linux-advanced-/home
- Privacy Guides 对 killswitch 的怀疑：https://discuss.privacyguides.net/t/please-do-not-completely-trust-the-kill-switch-functions-of-vpn-clients/29350 ，https://discuss.privacyguides.net/t/your-vpn-kill-switch-wont-stop-all-leaks/27039
- RTINGS killswitch 测试方法：https://www.rtings.com/vpn/tests/kill-switch-robustness

### Mullvad / WebRTC / DNS 检测
- Mullvad 连接检查工具：https://mullvad.net/en/check ，https://mullvad.net/en/help/dns-leaks
- Mullvad browser extension DNS / WebRTC leak detection：https://deepwiki.com/mullvad/browser-extension/4.2-dns-and-webrtc-leak-detection ，https://deepwiki.com/mullvad/browser-extension/4-connection-monitoring
- BrowserLeaks WebRTC test：https://browserleaks.com/webrtc
- BrowserScan leak test：https://browserscan.org/tools/leak-test
- Top10VPN do-i-leak：https://www.top10vpn.com/tools/do-i-leak/

### DoH / canary domain
- Mozilla canary domain：https://support.mozilla.org/en-US/kb/canary-domain-use-application-dnsnet
- Mozilla 配置 DoH 关闭：https://support.mozilla.org/en-US/kb/configuring-networks-disable-dns-over-https
- Wikipedia DoH：https://en.wikipedia.org/wiki/DNS_over_HTTPS
- Cisco Umbrella DoH：https://umbrella.cisco.com/blog/doh-dns-over-https-to-block-or-not-to-block ，https://umbrella.cisco.com/blog/doh-whats-all-the-fuss-about-dns-over-https
- DNSFilter DoH：https://www.dnsfilter.com/glossary/doh

### sing-box 与防泄漏配置
- sing-box TUN inbound 文档：https://sing-box.sagernet.org/configuration/inbound/tun/
- sing-box changelog：https://sing-box.sagernet.org/changelog/
- sing-box strict_route 文档不一致 issue：https://github.com/SagerNet/sing-box/issues/2707
- sing-box IPv6 阻断 issue：https://github.com/SagerNet/sing-box/issues/1123
- sing-box client manual：https://sing-box.sagernet.org/manual/proxy/client/

### Kubernetes default-deny / tcpdump 取证
- Calico default deny：https://docs.tigera.io/calico/latest/network-policy/get-started/kubernetes-default-deny
- NetworkPolicy Best Practices：https://pauldally.medium.com/networkpolicy-best-practices-9a388e41c7c9
- Container & K8s network debugging：https://www.youngju.dev/blog/network/2026-03-08-container-k8s-network-debugging.en
- DNS tcpdump 教程：https://noc.org/learn/inspecting-dns-tcpdump
- kubectl debug + tcpdump：https://oneuptime.com/blog/post/2026-02-09-kubectl-debug-network-tools/view ，https://oneuptime.com/blog/post/2026-02-09-debug-pod-service-connectivity-tcpdump/view

### 容器 capability / IMDS hardening
- Snyk drop default caps：https://learn.snyk.io/lesson/container-does-not-drop-all-default-capabilities/
- Kyverno drop-cap-net-raw：https://kyverno.io/policies/best-practices/require-drop-cap-net-raw/require-drop-cap-net-raw/
- Datadog container capabilities：https://securitylabs.datadoghq.com/articles/container-security-fundamentals-part-3/
- antiTree CAP_NET_RAW & ping：https://www.antitree.com/2019/01/containers-using-ping-without-cap_net_raw/
- moby/moby CAP_NET_RAW issue：https://github.com/moby/moby/issues/41886
- AWS IMDS docs：https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html ，https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html
- Google Cloud Metadata abuse UNC2903：https://cloud.google.com/blog/topics/threat-intelligence/cloud-metadata-abuse-unc2903/
- Datadog IMDS hardening：https://securitylabs.datadoghq.com/articles/misconfiguration-spotlight-imds/
- SANS Cloud IMDS：https://www.sans.org/blog/cloud-instance-metadata-services-imds-
- Hacking The Cloud SSRF IMDS：https://hackingthe.cloud/aws/exploitation/ec2-metadata-ssrf/
- Sysdig honeypot writeup：https://webflow.sysdig.com/blog/cve-2026-33626-how-attackers-exploited-lmdeploy-llm-inference-engines-in-12-hours
- Elastic IMDS detection rule：https://www.elastic.co/guide/en/security/8.19/unusual-instance-metadata-service-imds-api-request.html

### Tailscale natlab
- tailscale.com/tstest/natlab GoDoc：https://pkg.go.dev/tailscale.com/tstest/natlab
- tailscale issue #586 natlab：https://github.com/tailscale/tailscale/issues/586

### WebRTC headless testing
- aiortc GitHub：https://github.com/aiortc/aiortc
- aiortc 教程 dev.to：https://dev.to/whitphx/python-webrtc-basics-with-aiortc-48id
