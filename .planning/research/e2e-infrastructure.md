# Cloud CLI Proxy — e2e 测试框架与 CI 基础设施研究

> 研究范围：测试框架选型 + CI 基础设施 + 测试运行环境。
> 仅用于辅助决策，不修改任何项目代码。
> 项目根目录引用一律使用相对路径。

## 1. 执行摘要

**推荐组合：Go `testing` + `testify/suite` + `testcontainers-go`，配合 GitHub Actions「自托管 Linux 裸金属（或长寿命 VM）runner」+ 已有 BATS 烟雾测试分层。**

具体落点：

1. 把 `internal/**/*_test.go` 留作 Go 单元测试，新增 `tests/e2e/`（Go 文件）放跨进程 e2e。
2. 用 `testcontainers-go` 起 Postgres、宿主代理依赖容器；控制面进程直接 `os/exec` 起，便于打开 CPU/race/coverage。
3. 涉及 `tun` + `nftables` + `network namespace` 的强网络断言一律标记为 `//go:build linux && privileged`，只在「自托管 Linux runner」或「专门 spin 的临时 VM runner」跑；GitHub Actions hosted `ubuntu-latest` 因为不能开 KVM 嵌套虚拟化、`--privileged` 行为受限、`ip netns exec` 受限，跑不动完整网络栈。
4. BATS 用例继续守 bootstrap CLI 的退出码契约（已有 `tests/smoke/bootstrap.bats`），不要把它扩成完整 e2e。
5. 测试库间数据库隔离用「Postgres template + database-per-test」（IntegreSQL 思路）或最简单的「container-per-suite + schema-per-test」，避免 transactional rollback 在控制面自身 `BEGIN/COMMIT` 代码路径下失效。

---

## 2. Go e2e 测试框架对比

### 2.1 候选清单

| 维度 | `testing` + `os/exec` 裸写 | `ory/dockertest v4` | `testcontainers-go` | `testify/suite` |
|---|---|---|---|---|
| 容器编排 | 完全手写 | 内置 Pool/Resource，`NewPoolT`/`RunT` 与 `*testing.T` 集成 | `testcontainers.Run` + `CleanupContainer(t, c)` | 与上面 3 个搭配用，不独立提供容器能力 |
| 崩溃后清理 | 无（CI runner 崩了就是孤儿容器） | 无 reaper，依赖 `AutoRemove`/`Expire(60)` 兜底 | 有 **Ryuk** 旁路 reaper，标签匹配清理，最稳 | — |
| 等待策略 | 手写轮询 | 内置 `Retry` / `RetryWithBackoff` | 内置 `wait.ForLog` / `wait.ForListeningPort` / `wait.ForSQL` 等多种 | — |
| 多语言一致 | — | Go-only | Java/Node/Python/Go 同款语义 | — |
| 模块丰富度 | — | 中（社区模块少） | 高（Postgres、Redis、Kafka、LocalStack 等官方模块） | — |
| 发行节奏 | — | v4 (2025) 才有大版本，近年节奏偏慢 | 月度发布，企业赞助多 | 稳定 |
| 已知坑 | 自己造轮子 | v3 时期更新 docker 客户端会破测试 | v0.32.0 出现 `TESTCONTAINERS_RYUK_DISABLED` 被忽略的回归 | — |
| 调试体验 | 全靠 `t.Logf` | 容器对象上 `Logs`/`Exec` | 容器对象上 `Logs`/`Exec`，容器自带可见标签 | — |
| CI 兼容性 | 由你决定 | DinD 也能跑（无 reaper） | hosted GH Actions 直接跑；DinD/k8s runner 推荐 `TESTCONTAINERS_RYUK_DISABLED=true` | — |

> 数据来源见末尾「参考资料」。

### 2.2 选型理由

- **`testcontainers-go` 作为主力**：模块生态、Ryuk 兜底、等待策略覆盖、社区活跃度都是 dockertest 给不出来的。Cloud CLI Proxy 的 e2e 至少要起 Postgres + 用户容器 + sing-box 容器，等待策略和清理都是高频痛点。
- **`testify/suite` 做用例组织**：本项目 e2e 一定会复用「起 Postgres → 起控制面 → 注入种子数据 → 跑断言」这条公共前置；suite 的 `SetupSuite`/`SetupTest`/`TearDownSuite` 比 `TestMain` 灵活，又能直接和 testcontainers 的 helper 拼接。参考 testcontainers-go 官方示例里用 `testhelpers/containers.go` 提取容器构造的写法。
- **`dockertest` 不选**：v4 已经够好用，但模块生态相比 testcontainers 还是落后一个量级；本项目里没有「必须 Go-only 极简依赖」的硬要求，不值得为它降低长期可维护性。
- **裸写 `testing` 留作 fallback**：当 testcontainers 把简单事搞复杂时（比如直接拉起 host-agent 这种 Go 二进制），用 `exec.Command` 起一个长生命周期子进程更直接。

### 2.3 BATS / shell harness 的角色边界

- 现状：`tests/smoke/bootstrap.bats` 守的是 `bootstrap` CLI 的 **退出码契约**（10/11/12/13/2/1），用 mock HTTP server 模拟控制面响应。这是合适的——CLI 边界测试用 BATS 最划算。
- 边界建议：
  - **保留** BATS 跑「CLI ↔ HTTP API」契约层，不引入真容器。
  - **不要** 让 BATS 扩张到「真启动栈 + 跑容器 + 断言 nft 规则」，原因是 shell 调试体验差、无法做并行隔离、失败时没有结构化诊断。
  - `tests/scripts/uat-*.sh`、`test/bootstrap/e2e_bootstrap_ssh.sh` 这种 ~600-844 行的脚本，下一步应该按主题拆迁到 Go e2e，shell 只留「真·一次性手工 UAT 脚本」。

---

## 3. CI 环境矩阵

Cloud CLI Proxy 的网络强约束（tun 设备、`ip netns`、`nft`、`iptables-nft`、`MASQUERADE`、`ip_forward`）让 CI 环境的可行域比一般 Go 项目窄很多。

### 3.1 能力矩阵

| 环境 | Docker | `--privileged` 用户容器 | `CAP_NET_ADMIN` | `/dev/net/tun` | `ip netns exec` | `nftables` | KVM 嵌套虚拟化 | systemd | 备注 |
|---|---|---|---|---|---|---|---|---|---|
| GitHub Actions `ubuntu-latest` (hosted) | ✅ 预装 | ✅（job 步骤里 `docker run --privileged` 可以） | ✅ | ✅（设备节点存在） | ⚠️ 受限（`--privileged` 比 `--cap-add=all` 多做的事 act/容器内拿不全） | ✅ `iptables-nft` 比 legacy 兼容性好 | ❌（仅 larger runner 有） | ❌（容器没有 PID1=systemd） | 适合「单元 + 容器化 Postgres + 控制面 in-process」一类，不适合完整网络栈断言 |
| GitHub Actions `ubuntu-latest` Larger Runner | 同上 | 同上 | 同上 | 同上 | 同上 | 同上 | ✅（x86_64，需 udev rule + `KERNEL=="kvm"`） | ❌ | 可以再起一台 QEMU VM 跑 e2e，但费用高 |
| GitHub Actions self-hosted runner（裸机 Linux） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | 最贴近生产；缺点是公网仓库的安全风险 |
| GitHub Actions self-hosted runner（VM，KVM/Firecracker） | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | 视 VM 内核而定 | ✅ | 适合 OSS：每次任务起一台 VM，跑完销毁；类似 Actuated 的玩法 |
| Docker-in-Docker (`docker:dind`) on hosted runner | ⚠️ | 可以但要 `--privileged` 套娃 | ⚠️ | ⚠️ | ❌ 实践中很容易踩坑 | ⚠️ | ❌ | ❌ | testcontainers-go 在这里**强烈建议** `TESTCONTAINERS_RYUK_DISABLED=true`，并且 Ryuk 自己会建 `reaper_default` bridge 网络，可能和你的 netns 冲突 |
| Docker-out-of-Docker（挂 `/var/run/docker.sock`） | ✅ | ✅（看宿主） | ✅ | ✅ | ⚠️ | ✅ | 看宿主 | 看宿主 | 比 DinD 干净，但所有容器都落在宿主 docker 里，需要小心 label 清理 |
| `nektos/act` 本地复现 | ✅ | 只能通过 `--container-options "--privileged"`（非 workflow `options:`） | 同上 | 同上 | 同上 | 同上 | ❌ | ❌ | 适合调试 workflow 语法，**不要**作为正式 e2e 入口 |

### 3.2 关键结论

1. **完整 e2e（用户容器 + sing-box tun + nft + netns）不能跑在 hosted runner 的 job 容器里**。原因：`ip netns exec` 在容器里即使加了 `--cap-add=NET_ADMIN --cap-add=SYS_ADMIN` 也不一定行，必须 `--privileged`；而 GitHub Actions 的 workflow-level `container.options: --privileged` 历史上不可靠（actions-runner-controller#578）。
2. **唯一稳的做法**：在 `runs-on: ubuntu-latest` 的 job **步骤里**直接 `docker run --privileged --cap-add=NET_ADMIN --device=/dev/net/tun -v /var/run/netns:/var/run/netns ...`，而不是把整个 job 框在 `container:` 字段里。这样 runner 的 root 权限就能透下去。
3. **大杀器仍然是 self-hosted runner**。k3s 在 GitHub Actions 上跑完整 E2E 直到 nested virt 落地才解锁（k3s-io/k3s#9659），它的经验是：「不绕」——直接 self-hosted。
4. **DinD 不适合本项目**。Ryuk 默认 bridge 网络会和你的隧道 netns 抢资源；并且 `--privileged` DinD 容器里再起 `--privileged` 用户容器，是双层套娃，调试时很痛。
5. **act 只用于 workflow YAML 语法验证**，跨 job 矩阵、`services:`、KVM、`--privileged` 行为都和真实 GH Actions 不一致。

### 3.3 推荐分层

```
┌──────────────────────────────────────────────────────────────┐
│ Layer 1: hosted ubuntu-latest                                │
│  • go test ./...   （单元 + internal 包测试）                 │
│  • web/admin build  （已存在）                                 │
│  • BATS smoke      （bootstrap CLI 退出码）                    │
│  • testcontainers-go：仅起 Postgres + 控制面 in-process       │
│    断言 HTTP API、迁移、权限模型等非特权路径                   │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ Layer 2: self-hosted Linux runner  (单独 label 例如 e2e-host)│
│  • 完整栈：control-plane + host-agent + Postgres + 用户容器  │
│  • netns / nftables / tun 强断言                              │
│  • build tag 控制：//go:build linux && privileged             │
│  • 失败时 artifact：容器 logs、nft list ruleset、ip route 等  │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│ Layer 3: 临时云 VM（可选，公网 OSS 友好）                     │
│  • 借鉴 Actuated 模型：每个 PR 起一台一次性 KVM VM            │
│  • 等到本项目对外开放贡献时再考虑                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 4. 测试生命周期与脏数据治理

### 4.1 容器清理

| 场景 | 推荐做法 |
|---|---|
| testcontainers-go 正常 case | `testcontainers.CleanupContainer(t, ctr)` **放在 `require.NoError(t, err)` 之前**，nil 安全；不要再手写 `t.Cleanup(func(){ ctr.Terminate(ctx) })` |
| testcontainers-go 在 self-hosted runner | 保持 Ryuk 开启；如果 runner 是长寿命的，孤儿容器会被 Ryuk 标签 `org.testcontainers=true` 自动回收 |
| testcontainers-go 在 DinD/k8s runner | `TESTCONTAINERS_RYUK_DISABLED=true` + `TESTCONTAINERS_CHECKS_DISABLE=true`；用 Kubedock 或 Testcontainers Cloud 顶替清理 |
| 自起的 docker 容器（非 testcontainers） | 一律打 `--label cloudcliproxy.e2e.run=$RUN_ID`，suite teardown 里用 `docker ps -aq --filter label=cloudcliproxy.e2e.run=$RUN_ID` 一锅端 |
| netns / 物理 tun 设备 | 用 `defer netlink.LinkDel(link)` + `defer netns.DeleteNamed(name)`；测试名作为命名空间名前缀 `e2e_<test>_<run>`，便于事后人工清理 |
| nftables 规则 | 给本项目自己用的 table 加专属名字 `inet cloudcliproxy_e2e`，teardown 直接 `nft delete table inet cloudcliproxy_e2e` |

### 4.2 数据库隔离

数据来源参考「参考资料」的 `mtekmir` / Storj 文章和 IntegreSQL。

| 策略 | 适用 | 不适用 |
|---|---|---|
| Transaction rollback（go-txdb） | 测试代码本身不开事务；最快 | **不适用**于 Cloud CLI Proxy：控制面要做容器生命周期、出口 IP 绑定，肯定有自己的 `BEGIN/COMMIT`，rollback 法会污染语义 |
| Schema-per-test（`search_path`） | 测试代码可以开事务；速度居中 | 如果代码里有 `SET search_path` 或跨 schema 引用就会被污染 |
| Database-per-test（template clone） | 强隔离、可并行；IntegreSQL 把 template clone 做到接近瞬时 | 增加一个外部组件（IntegreSQL）或要自己包装 `CREATE DATABASE ... TEMPLATE`；为小项目可能过度 |

**给本项目的建议**：阶段一直接 `container-per-suite + schema-per-test` 起步，container 共享，每个测试 `CREATE SCHEMA e2e_<rand>; SET search_path TO e2e_<rand>, public;`，teardown 时 `DROP SCHEMA ... CASCADE`。够快、够干净、不引入额外服务。后续如果并发瓶颈到了再上 IntegreSQL。

### 4.3 测试质量加固

| 工具 | 用途 | 接入成本 |
|---|---|---|
| `go test -race` | 数据竞争检测；e2e 是多 goroutine + 真容器，race 价值高 | 0 成本，直接加 flag |
| `go test -shuffle=on` | 打乱 test 顺序，揭穿隐式依赖（共享 schema、共享 DB 单例） | 0 成本，CI 里加 flag |
| `go test -count=N` | 反复跑同一组 e2e，揭穿 flaky | CI 上 PR 不开，定时 nightly 开 |
| `uber-go/goleak` | 抓 leaked goroutine；本项目网络栈 + 长连接 SSH 容易留 goroutine | 在 `TestMain` 里 `defer goleak.VerifyNone(m)`，配合 `IgnoreCurrent` 排除 sing-box / pgx 池 |
| `t.Parallel()` | 跨 suite 并发；前提是上文的命名空间 + schema 隔离做扎实 | 渐进引入 |

### 4.4 失败可观测性

每个 e2e 失败必须自动留下：

1. 所有相关容器 `docker logs --tail=2000`，落到 `${RUNNER_TEMP}/e2e-artifacts/<test_name>/<container>.log`。
2. 宿主 `nft list ruleset`、`ip -all netns list`、`ip route show table all`、`ip link show` 快照。
3. 关键 socket 探针：`ss -tnp`、`curl -sv http://control-plane/healthz`。
4. Postgres 端：`pg_dump --schema-only` + 关键表 `\copy ... csv`。
5. GitHub Actions `actions/upload-artifact@v4`，retention 7-14 天足够。

参考 k3s `tests/docker/test-helpers` 的 `test-run-sonobuoy` 收集模式，以及 cilium `.github/workflows/conformance-ipsec-e2e.yaml` 的 dump-on-failure step 设计。

---

## 5. 参考开源项目的具体做法

### 5.1 k3s（Go + 容器编排，最贴近）

- 主仓库：[`github.com/k3s-io/k3s`](https://github.com/k3s-io/k3s)
- 测试目录约定（来自 `tests/integration/README.md`）：
  - `tests/integration/<TEST_NAME>/<TEST_NAME>_int_test.go`
  - 函数名 `Test_Integration<TEST_NAME>`
  - BDD 风格（Ginkgo + Gomega），不过他们也承认这套对 GH Actions 不友好
- 关键约束：「Integration tests run with elevated privileges to manage system services and network configuration.」——他们直接把 e2e 跑在 self-hosted / Drone CI 上，不在 hosted runner 上跑全套。
- 在 GH Actions 上扩展 E2E 的 issue：[`k3s-io/k3s#9659`](https://github.com/k3s-io/k3s/issues/9659)，结论是「直到 GH Actions 开放 nested virt 才能跑」，本项目可以引以为戒。
- 失败 artifact：通过 `GOCOVERDIR` 收集运行时 coverage（不仅 unit），跨进程合并；本项目控制面 + host-agent 也是分进程，这套思路可以照抄。

### 5.2 cilium（kernel/netns 极端复杂）

- [`Documentation/contributing/testing/e2e.rst`](https://github.com/cilium/cilium/blob/main/Documentation/contributing/testing/e2e.rst)
- 用 **LVH (Little VM Helper)** 起带不同 kernel 的 VM 在 GH Actions 里跑——本质上是用 KVM 嵌套 + larger runner。对单宿主机的 SSH 云主机平台来说太重，**不直接照抄**，但「失败时 dump kernel 状态」「kernel matrix」是值得借鉴的纪律。
- IPsec e2e 单独 workflow：`.github/workflows/conformance-ipsec-e2e.yaml`——按「网络特性」拆 workflow 而不是塞一个大 workflow，对本项目「按 sing-box / nft / netns 分别拆 job」很有借鉴价值。

### 5.3 testcontainers-go（自家 CI）

- 仓库：[`github.com/testcontainers/testcontainers-go`](https://github.com/testcontainers/testcontainers-go)
- 自家测试在 hosted `ubuntu-latest` 上跑，因为它只用到 Docker socket，不要 netns。
- 文章：[Docker 官方：Running Testcontainers Tests Using GitHub Actions and Testcontainers Cloud](https://www.docker.com/blog/running-testcontainers-tests-using-github-actions/) 给出了最干净的 minimal workflow，本项目「Layer 1」可以直接抄。
- 已知坑：[`testcontainers-go#2701`](https://github.com/testcontainers/testcontainers-go/issues/2701)（v0.32.0 `TESTCONTAINERS_RYUK_DISABLED` 回归）、[`#2817`](https://github.com/testcontainers/testcontainers-go/issues/2817)（GitLab CI 网络规则下 Ryuk 启不来）——升级时盯紧版本。

### 5.4 Coder（Go + Docker 容器开发环境，业务域接近）

- 仓库：[`github.com/coder/coder`](https://github.com/coder/coder)
- 业务域和本项目（容器化开发主机 + 受控网络）有相当重合，但 coder 的容器编排往 k8s 走，e2e 主要靠 Terraform + Kubernetes provider 跑——和本项目「单宿主机 + 强网络」路线不一样，**不要照搬**它的目录结构。
- 值得参考的是它的 GitHub Action「Coder CLI setup」模式：把 CLI 的 e2e 入口做成 reusable composite action，方便外部仓库引用——本项目 `bootstrap.sh` 后续也能走这个套路。

### 5.5 lachiejames 文章（dockerized e2e 实战）

- [Elevate Your CI/CD: Dockerized E2E Tests with GitHub Actions](https://lachiejames.com/elevate-your-ci-cd-dockerized-e2e-tests-with-github-actions/)
- 给出最朴素的 pipeline：「`actions/checkout` → `docker compose up -d` → 跑 e2e 容器 → `actions/upload-artifact` 收 logs / videos / reports」——`tests/scripts/uat-*.sh` 现状其实就是这个流程，把它的 shell 部分迁到 Go 即可。

---

## 6. 给本项目的具体落地建议（分阶段）

### 阶段 0：守住现状（0.5 天）

不写代码，只做配置：

1. 在 `.github/workflows/ci.yml` 的 `go-test` job 加上 `-race -shuffle=on -count=1`。
2. 在 `Makefile` 的 `test-go` 同步：`go test ./... -race -shuffle=on -count=1`。
3. 检查 BATS smoke 是否在 CI 里跑——目前 `ci.yml` 没看到 `test-smoke` job，建议补一个 job（hosted runner 跑得起来，因为它只 mock HTTP）。

### 阶段 1：在 hosted runner 上接入 testcontainers-go（1-2 天）

1. 引入依赖：`go get github.com/testcontainers/testcontainers-go@latest`、`github.com/testcontainers/testcontainers-go/modules/postgres`、`github.com/stretchr/testify@latest`、`go.uber.org/goleak`。
2. 新建 `tests/e2e/api/` 目录，目标：起 Postgres、`go run ./cmd/control-plane`（in-process 或子进程），跑 HTTP API 契约用例。
3. 这一层在 hosted `ubuntu-latest` 上跑，添加一个 `go-e2e-api` job，不需要任何特权。
4. 验收：覆盖现有 `tests/scripts/uat-v31-promotion.sh` 中可迁移的 HTTP 断言部分（promotion、过期、出口 IP CRUD），不碰 SSH/网络隔离。

### 阶段 2：在 self-hosted Linux runner 上跑特权 e2e（3-5 天）

1. 准备一台 Linux 物理机（或 OSS 友好的一次性 VM 模板），安装 Docker 28.x、`nftables`、`iptables-nft`、加载 `tun` 模块、`ip_forward=1`。
2. 注册成 GitHub Actions self-hosted runner，打 `e2e-host` label。
3. 新建 `tests/e2e/network/`（`//go:build linux && privileged`）：
   - 起完整栈：Postgres、control-plane、host-agent、sing-box gateway、用户容器。
   - 断言项：
     - 用户容器默认网关 = sing-box tun
     - `nft list ruleset` 包含本项目专属 table 且无残留默认规则
     - 从用户容器内 `curl ifconfig.me` 出口 IP == 绑定 IP（mock 一个本地 echo 服务即可）
     - 杀掉 sing-box 后 `curl` 必须失败（验证 fail-closed）
4. 新建 `go-e2e-privileged` job：`runs-on: [self-hosted, e2e-host]`，必跑步骤包括：
   - `go test ./tests/e2e/network/... -race -tags=privileged -count=1`
   - 失败时 `nft list ruleset > $RUNNER_TEMP/nft.txt`、`docker logs ...`，再 `actions/upload-artifact@v4`。
5. **不要**把 `go-e2e-privileged` 设为 PR 必须通过——先做 nightly + 手动触发（`workflow_dispatch`），稳定后再 promote 到 PR gate。

### 阶段 3：测试质量沉淀（持续）

1. 在 `tests/e2e/` 加 `internal/testutil`：封装容器构造、schema 隔离、artifact 收集，全 e2e 用例共享。
2. 把现有 `tests/scripts/uat-vscode-remote-ssh.sh` 的可断言部分迁到 `tests/e2e/ssh/`，长尾的人工 UAT 留在 shell。
3. 引入 `goleak.VerifyTestMain`，特别关注 sing-box / pgx pool 的 goroutine。
4. 评估 IntegreSQL（如果到了「同时 20+ 并发 e2e」的规模）。

### 阶段 4：可选扩展

- 临时云 VM runner（Actuated 风格）：本项目走开源贡献路线后再考虑。
- KVM 嵌套：除非要测「不同 Linux kernel × sing-box」矩阵，否则不需要。
- Testcontainers Cloud：闭源团队场景下省事，但本项目没有这个痛点，先不上。

---

## 7. 参考资料

### 测试框架

- [testcontainers-go 仓库](https://github.com/testcontainers/testcontainers-go)
- [testcontainers-go 官方文档](https://golang.testcontainers.org/)
- [testcontainers-go Postgres 模块](https://golang.testcontainers.org/modules/postgres/)
- [ory/dockertest 仓库](https://github.com/ory/dockertest)
- [dockertest v3 godoc](https://pkg.go.dev/github.com/ory/dockertest/v3)
- [Docker：Running Testcontainers Tests Using GitHub Actions and Testcontainers Cloud](https://www.docker.com/blog/running-testcontainers-tests-using-github-actions/)
- [Issue: TESTCONTAINERS_RYUK_DISABLED 在 v0.32.0 失效](https://github.com/testcontainers/testcontainers-go/issues/2701)
- [Issue: Ryuk 在 GitLab CI 网络规则下启动失败](https://github.com/testcontainers/testcontainers-go/issues/2817)
- [filipsnastins/testcontainers-github-actions 示例仓库](https://github.com/filipsnastins/testcontainers-github-actions)

### CI / 特权 / Docker

- [GitHub Actions self-hosted 控制器 issue: --privileged 不被透传](https://github.com/actions/actions-runner-controller/issues/578)
- [moby/moby#41888: ip netns exec 在容器内需要 --privileged](https://github.com/moby/moby/issues/41888)
- [marcoguerri：CAP_NET_ADMIN for non-root user in Docker container](https://marcoguerri.github.io/2023/10/13/capabilities-and-docker.html)
- [GitHub community discussion: Enable nested virtualization](https://github.com/orgs/community/discussions/160591)
- [Red Hat Developer：End-to-end testing with self-hosted runners in GitHub Actions](https://developers.redhat.com/articles/2023/07/25/end-end-testing-self-hosted-runners-github-actions)
- [Actuated：How to run KVM guests in your GitHub Actions](https://actuated.com/blog/kvm-in-github-actions)
- [nektos/act 仓库](https://github.com/nektos/act)
- [nektos/act#947: --privileged 选项支持](https://github.com/nektos/act/issues/947)
- [nektos/act#835: 步骤容器 network mode 限制](https://github.com/nektos/act/issues/835)

### 数据库隔离

- [mtekmir：Isolating Integration Tests in Go (schema-per-test)](https://mtekmir.com/blog/golang-sql-integration-test-isolation/)
- [Storj Engineering Blog：Go Integration Tests with Postgres](https://storj.dev/blog/go-integration-tests-with-postgres)
- [DATA-DOG/go-txdb 仓库](https://github.com/DATA-DOG/go-txdb)
- [allaboutapps/integresql 仓库](https://github.com/allaboutapps/integresql)
- [jackc/pgx#1627: pgx.Conn 可见未提交事务（连接复用陷阱）](https://github.com/jackc/pgx/issues/1627)

### 开源参考

- [k3s tests/TESTING.md](https://github.com/k3s-io/k3s/blob/main/tests/TESTING.md)
- [k3s tests/integration/README.md](https://github.com/k3s-io/k3s/blob/main/tests/integration/README.md)
- [k3s issue#9659：Implement E2E testing in GitHub Actions](https://github.com/k3s-io/k3s/issues/9659)
- [k3s 单元与集成测试 DeepWiki](https://deepwiki.com/k3s-io/k3s/6.2-unit-and-integration-tests)
- [Cilium e2e 文档（main 分支）](https://github.com/cilium/cilium/blob/main/Documentation/contributing/testing/e2e.rst)
- [Cilium CI / GitHub Actions 文档](https://docs.cilium.io/en/latest/contributing/testing/ci/)
- [coder/coder 仓库](https://github.com/coder/coder)
- [Lachie James：Elevate Your CI/CD: Dockerized E2E Tests with GitHub Actions](https://lachiejames.com/elevate-your-ci-cd-dockerized-e2e-tests-with-github-actions/)
