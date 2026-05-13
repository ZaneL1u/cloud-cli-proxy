# Requirements: Cloud CLI Proxy — v3.6 端到端测试体系与网络隔离验证

**Defined:** 2026-05-14
**Core Value:** 给每个用户提供一台开箱即用的 SSH 云主机，并且严格保证其所有出网流量都走受控的指定出口 IP

## v1 Requirements (v3.6 scope)

### E2E 测试基础设施

- [ ] **E2E-01**: `tests/e2e/` 目录存在，使用 testcontainers-go + testify/suite 组织
- [ ] **E2E-02**: Scenario 抽象可声明式描述「控制面 + host-agent + Postgres + N 个用户容器 + sing-box gateway」拓扑
- [ ] **E2E-03**: CI 分为两层：hosted runner 跑非特权测试（Go 单元 + API 集成 + BATS smoke），self-hosted Linux runner 跑特权网络栈 e2e
- [ ] **E2E-04**: 每个 e2e 用例失败时自动归档 artifact（容器日志、nft ruleset、netns 列表、路由表、pg dump）
- [ ] **E2E-05**: 测试 harness 禁止裸 sleep，全部使用 waitFor 条件等待

### 最小可信 e2e 用例（MVS）

- [ ] **MVS-01**: 首次 bootstrap 成功并进入 SSH 会话（curl → 认证 → 容器启动 → SSH banner）
- [ ] **MVS-02**: 容器内 `curl ifconfig.me` 返回绑定的出口 IP（三源轮询：ip.me / ifconfig.io / ipinfo.io）
- [ ] **MVS-03**: 容器内 `dig @1.1.1.1 example.com` 被 tun DNS 接管或被防火墙拒绝
- [ ] **MVS-04**: 容器内直连外网 `bash -c 'echo >/dev/tcp/1.1.1.1/80'` 被防火墙拒绝（扩展目标：8.8.8.8:443、9.9.9.9:443、169.254.169.254:80）
- [ ] **MVS-05**: CLI 错误码契约正确（auth_invalid=10 / account_disabled=11 / account_expired=12 / host_not_found=13 / 其它=1/2）
- [ ] **MVS-06**: 到期用户的运行容器被 ExpiryScanner 自动停止，审计事件 `host.stopped` 落库
- [ ] **MVS-07**: 同一出口 IP 被两个 Host 双绑时，第二次绑定被 API 拒绝
- [ ] **MVS-08**: host-agent 进程强杀后，控制面健康状态 30s 内转 unhealthy；重启后自动恢复
- [ ] **MVS-09**: sing-box 容器崩溃后，用户容器内立即断网（出网失败），不降级到直连
- [ ] **MVS-10**: 用户在容器内 `echo nameserver 8.8.8.8 > /etc/resolv.conf` 后，DNS 仍走 tun 或被拒绝

### 防泄漏对抗测试

- [ ] **LEAK-01**: DNS 明文 UDP/53 旁路：容器内 `dig @8.8.8.8` → host eth0 抓包必须无 udp port 53 非网关流量
- [ ] **LEAK-02**: DoT (853) 旁路：容器内 `kdig +tls @1.1.1.1` → host eth0 必须无 tcp port 853 非网关流量
- [ ] **LEAK-03**: ICMP 阻断：`ping 8.8.8.8` 必须失败（nftables output DROP）
- [ ] **LEAK-04**: IPv6 阻断：`curl -6 ipv6.google.com` 必须失败（disable_ipv6=1 + ip6 table DROP）
- [ ] **LEAK-05**: IMDS 阻断：`curl 169.254.169.254` 和 `curl 169.254.170.2` 必须失败
- [ ] **LEAK-06**: raw socket 拒绝：`python -c "socket.socket(AF_INET, SOCK_RAW)"` 必须 PermissionError
- [ ] **LEAK-07**: link-local 显式 drop：nftables 规则包含 `ip daddr 169.254.0.0/16 drop`
- [ ] **LEAK-08**: capability 审计：worker 容器 CapEff/CapBnd 不含 NET_RAW、NET_ADMIN、SYS_ADMIN

### Kill-switch 压力测试

- [ ] **KILL-01**: `docker kill -SIGKILL` sing-box gateway → 3 秒内容器内 curl 必须失败
- [ ] **KILL-02**: `ip link set tun0 down` → 容器内 curl 必须失败
- [ ] **KILL-03**: Pumba netem delay/loss 注入后，SSH 会话仍存活，但出口 IP 校验可能超时
- [ ] **KILL-04**: 网关容器被 `docker network disconnect` 后，worker 不回落到 host 默认路由

### 代码层质量加固

- [ ] **QUAL-01**: verify.go `verifyEgressIP` 支持多源轮询（≥3 个独立 IP 回显服务）
- [ ] **QUAL-02**: verify.go `verifyLeakBlocked` 目标参数化（多 IP × 多端口）
- [ ] **QUAL-03**: verify.go `verifyDNS` 遍历全部 nameserver 行（不只检查第一行）
- [ ] **QUAL-04**: namespace.go `GetContainerNetNS` 重试上限暴露给 e2e 配置
- [ ] **QUAL-05**: worker_firewall_linux.go 所有规则带 `counter` 表达式
- [ ] **QUAL-06**: worker 容器启动参数包含 `--cap-drop=NET_RAW --cap-drop=NET_ADMIN`
- [ ] **QUAL-07**: `go test ./... -race -shuffle=on -count=1` 成为默认测试命令
- [ ] **QUAL-08**: goleak.VerifyTestMain 接入，排除 sing-box/pgx 已知泄漏

### 可观测性与诊断

- [ ] **OBS-01**: `tests/e2e/harness/collect-artifacts.sh` 存在且可在失败 trap 中调用
- [ ] **OBS-02**: artifact 目录结构包含 logs / network / docker / postgres / system 子目录
- [ ] **OBS-03**: CI workflow 在 e2e 失败时自动 `actions/upload-artifact@v4` 归档

## v2 Requirements (延后)

### 完整场景矩阵扩展

- **E2E-FULL-01**: 并发创建 N=50/100 容器，无 netns 残留、无端口冲突
- **E2E-FULL-02**: 出口 IP 切换后 conntrack flush，新连接走新出口
- **E2E-FULL-03**: Tetragon TracingPolicy 作为内核级 oracle，零违规事件断言
- **E2E-FULL-04**: host 重启后 systemd ExecStartPre 清理残留容器 + 规则
- **E2E-FULL-05**: Hurl 控制面 API 断言覆盖（用户 CRUD / IP 绑定 / 生命周期）

### 性能与稳定性

- **E2E-PERF-01**: 冷启动端到端时间 ≤ 15s（e2e 环境基准）
- **E2E-PERF-02**: e2e 完整矩阵运行时间 ≤ 60min（nightly）

## Out of Scope

| Feature | Reason |
|---------|--------|
| macOS/Windows 的 sidecar 路径完整 e2e | GH hosted runner 不支持 Docker Desktop 完整网络栈；需自购 Mac Mini runner，成本过高 |
| WebRTC 专门测试 | v1 只暴露 SSH 会话，用户不跑浏览器；WebRTC 向量由 nftables UDP default-deny 自然覆盖 |
| 浏览器前端 e2e（Playwright） | 本里程碑聚焦网络正确性；后台 UI 测试可后续补充 |
| 镜像 CVE 扫描（Trivy/Grype） | 安全左移，不属于功能 e2e |
| 多宿主机编排测试 | v1 明确单宿主机 |
| 计费/商业化流程测试 | v1 不做计费 |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| E2E-01 | Phase 45 | Pending |
| E2E-02 | Phase 45 | Pending |
| E2E-03 | Phase 45 | Pending |
| E2E-04 | Phase 45 | Pending |
| E2E-05 | Phase 45 | Pending |
| MVS-01 | Phase 46 | Pending |
| MVS-02 | Phase 46 | Pending |
| MVS-03 | Phase 46 | Pending |
| MVS-04 | Phase 46 | Pending |
| MVS-05 | Phase 46 | Pending |
| MVS-06 | Phase 47 | Pending |
| MVS-07 | Phase 47 | Pending |
| MVS-08 | Phase 47 | Pending |
| MVS-09 | Phase 48 | Pending |
| MVS-10 | Phase 48 | Pending |
| LEAK-01 | Phase 49 | Pending |
| LEAK-02 | Phase 49 | Pending |
| LEAK-03 | Phase 49 | Pending |
| LEAK-04 | Phase 49 | Pending |
| LEAK-05 | Phase 49 | Pending |
| LEAK-06 | Phase 49 | Pending |
| LEAK-07 | Phase 49 | Pending |
| LEAK-08 | Phase 49 | Pending |
| KILL-01 | Phase 50 | Pending |
| KILL-02 | Phase 50 | Pending |
| KILL-03 | Phase 50 | Pending |
| KILL-04 | Phase 50 | Pending |
| QUAL-01 | Phase 51 | Pending |
| QUAL-02 | Phase 51 | Pending |
| QUAL-03 | Phase 51 | Pending |
| QUAL-04 | Phase 51 | Pending |
| QUAL-05 | Phase 51 | Pending |
| QUAL-06 | Phase 51 | Pending |
| QUAL-07 | Phase 51 | Pending |
| QUAL-08 | Phase 51 | Pending |
| OBS-01 | Phase 52 | Pending |
| OBS-02 | Phase 52 | Pending |
| OBS-03 | Phase 52 | Pending |

**Coverage:**
- v1 requirements: 38 total
- Mapped to phases: 38
- Unmapped: 0 ✓

---
*Requirements defined: 2026-05-14*
*Last updated: 2026-05-14 after milestone v3.6 initialization*
