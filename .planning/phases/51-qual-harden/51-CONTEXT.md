# Phase 51: 代码层质量加固 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, auto_rest accept-all)

<domain>
## Phase Boundary

把 verify.go / namespace.go / nftables / worker cap-drop / test infra 等代码层加固一次性落地，并**闭环 Phase 47/49 暴露的 backend GAP**。这是本里程碑**唯一修改生产代码**的 phase。

**8 个 QUAL 不变量 + 1 个跨 phase GAP 收口**（共 9 plan）：

- QUAL-01 `verify.go::verifyEgressIP` 多源轮询（≥3 个独立回显服务，与 Phase 46 `Vote` 多数派语义对齐）
- QUAL-02 `verify.go::verifyLeakBlocked` 多目标参数化（多 IP × 多端口，与 Phase 46 `DefaultDenyMatrix` 对齐）
- QUAL-03 `verify.go::verifyDNS` 遍历全部 nameserver 行（不止第一行）
- QUAL-04 `namespace.go::GetContainerNetNS` 探测窗口与重试上限暴露给 e2e 配置（默认值保持）
- QUAL-05 `worker_firewall_linux.go` 全部规则加 `counter` 表达式；**并新增 IPv4 `169.254.0.0/16` 显式 drop counter 规则**（闭 Phase 49 GAP-2）
- QUAL-06 worker 容器启动参数：删 `--cap-add SYS_ADMIN`、显式 `--cap-drop NET_RAW`、NET_ADMIN 改运行时 setcap（闭 Phase 49 GAP-1）
- QUAL-07 `go test ./... -race -shuffle=on -count=1` 成为默认测试命令（更新 Makefile / CI workflow）
- QUAL-08 `goleak.VerifyTestMain` 接入（排除 sing-box / pgx 已知泄漏）
- **51-09** 出口 IP 双绑互斥 API pre-check + 稳定 error code（闭 Phase 47 GAP）

**Phase 47/49/50 GAP 映射**：

| 来源 | GAP | 收口 plan |
|------|-----|----------|
| Phase 47 D-47-3 | `host_egress_bindings` 缺 pre-check + 无 4xx error code | **51-09**（新增） |
| Phase 49 GAP-1 | worker `--cap-add NET_ADMIN/SYS_ADMIN`，docker 默认含 NET_RAW | **51-06** |
| Phase 49 GAP-2 | 缺 IPv4 `169.254.0.0/16` 显式 drop nft 规则 | **51-05**（扩展） |
| Phase 50 KILL-04 | gateway 接 `cloudproxy-net-*` 自定义 bridge（源码已就绪） | 无需修源码，已用 skip 兜底 |

**不在本 phase 范围**：

- 完整 artifact 采集脚本（Phase 52）
- 多协议泄漏扩展（mDNS / LLMNR / NetBIOS / SSDP 等，列后续）

**macOS 本地约束**：本 phase 大量修改 Linux-specific 代码（`worker_firewall_linux.go` / `namespace.go` 带 build tag `linux`）。darwin 编译以 `GOOS=linux` 跨编译为准；运行时单测尽量保持平台无关。

</domain>

<decisions>
## Implementation Decisions

### Area 1: verify.go 加固 (QUAL-01 / QUAL-02 / QUAL-03)

- **QUAL-01 verifyEgressIP**：
  - 三源固定（与 Phase 46 一致）：`ip.me` / `ifconfig.io` / `ipinfo.io/ip`
  - 实现：内部 goroutine 并发拉，等所有 source 完成或 ctx 超时，调用 Phase 46 `Vote` 多数派函数（**直接复用**，不重复实现）
  - 返回：原 `IP` 字段 + 新 `VoteResult` 字段（≥2 一致 = ok；全 timeout = inconclusive，调用方决定 skip/fail）
  - **接口契约保护**：现有 `verifyEgressIP` 调用方零破坏（保留旧签名 + 新增 `verifyEgressIPMulti` 函数）；既有单测全绿
- **QUAL-02 verifyLeakBlocked**：
  - 多 target 矩阵参数化，默认值 = Phase 46 `DefaultDenyMatrix`（`1.1.1.1:80 / 8.8.8.8:443 / 9.9.9.9:443 / 169.254.169.254:80`）
  - 内部并发探测 + 任一连通 = fail
  - 暴露 `func verifyLeakBlocked(ctx, targets ...Target) error` 形态
- **QUAL-03 verifyDNS**：
  - 当前实现：可能只读 `/etc/resolv.conf` 第一行（grep 确认）
  - 加固：解析 ALL nameserver 行 + 全部轮询，任一行能解析 = ok
  - 加固后调用方零破坏

### Area 2: namespace.go 加固 (QUAL-04)

- **QUAL-04 GetContainerNetNS**：
  - 暴露 `ProbeWindow time.Duration` 和 `MaxRetries int` 两个 config 字段（之前是硬编码常量）
  - 通过 functional option：`GetContainerNetNS(ctx, id, opts ...Option)` 形态
  - 默认值与现有硬编码一致（不破坏现有行为）
  - e2e harness 可通过 option 传入更短窗口加速测试

### Area 3: nftables 加固 (QUAL-05) + Phase 49 GAP-2 收口

- **QUAL-05 worker_firewall_linux.go 全规则加 counter**：
  - 现有 nftables 规则全部插入 `counter` 表达式（不破坏行为，仅观测）
  - 给规则起 comment（如 `comment "linklocal-drop"`）方便 `ParseNftRules` 识别
- **新增 IPv4 `169.254.0.0/16` 显式 drop**（闭 Phase 49 GAP-2）：
  - 在 worker netns nft `output` 链白名单**之前**插入 `ip daddr 169.254.0.0/16 counter drop comment "linklocal-drop"`
  - 顺序：先 link-local drop（最高优先级）→ 白名单 → 末尾兜底 drop
  - **既有 LEAK-05 IMDS 用例预期由 fail → pass**（之前被链末兜底挡住，counter 命中数 0；现在 counter 命中数 ≥1，且 ParseNftRules 能识别）
- **既有 LEAK-07 link-local 用例**预期由 fail → pass

### Area 4: worker cap 加固 (QUAL-06) + Phase 49 GAP-1 收口

- **删 `--cap-add SYS_ADMIN`**：grep `internal/runtime/tasks/worker.go:217-218` 找到，删除
- **显式 `--cap-drop NET_RAW`**：docker 默认含 NET_RAW，需显式 drop
- **NET_ADMIN 改运行时 setcap**：worker 容器启动后用 `setcap cap_net_admin=+ep /path/to/binary` 在启动脚本里加，避免容器级 NET_ADMIN
  - **重要**：如果 host-agent 当前依赖 NET_ADMIN 做 namespace / nftables 操作，可能需要保留 NET_ADMIN（折中：保留 NET_ADMIN，仅删 NET_RAW + SYS_ADMIN）；以源码实际依赖为准
- **既有 LEAK-06 / LEAK-08 用例**预期由 fail → pass

### Area 5: 测试 infra 加固 (QUAL-07 / QUAL-08)

- **QUAL-07 默认 -race -shuffle=on**：
  - 更新 `Makefile`（如有 `test` target）：`go test ./... -race -shuffle=on -count=1`
  - 更新 `.github/workflows/ci.yml` 主 test job 加 `-race -shuffle=on`
  - **保留** `tests/e2e/` 不带 `-race`（e2e 跑容器，race detector 性能开销大，且不适合 e2e 用例）
- **QUAL-08 goleak.VerifyTestMain**：
  - 在主 package（control-plane / host-agent / cloud-claude）`TestMain` 加 `goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("github.com/sagernet/sing-box/..."), goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"))`
  - **新依赖**：`go.uber.org/goleak`，写入 `go.mod`（这是本里程碑唯一允许新增的依赖）
  - 先跑一遍现有测试，把所有未知 goroutine 泄漏的 top function 加入 IgnoreList，避免噪音

### Area 6: 双绑互斥 API 收口 (51-09 新增)

- **位置**：`internal/controlplane/http/admin_bindings.go`（grep 确认实际路径）的 POST 处理函数
- **逻辑**：
  ```go
  // 1. 先 SELECT FROM host_egress_bindings WHERE egress_ip_id=?
  //    如果 row 存在且 host_id != requested host → 返回 409 Conflict
  //    error code: "egress_ip_already_bound"
  //    error message: 中文 "出口 IP 已绑定到其它宿主机"
  // 2. 否则继续原有 INSERT 路径
  ```
- **error code 常量**：在 `internal/controlplane/http/errors.go`（或类似）定义 `ErrCodeEgressIPAlreadyBound = "egress_ip_already_bound"`
- **现有 Phase 47 用例**：预期由 PARTIAL → PASS（`BindEgressIPResponse.ErrorCode` 字段命中常量）
- **生产代码加测**：新增 `admin_bindings_test.go` 单测覆盖双绑 case

### Area 7: 风险控制

- **零回归保证**：每个 QUAL plan 落地后必须跑 `go test ./...`（不限 tests/e2e）确保所有既有测试通过
- **按顺序 commit**：每个 QUAL 一个 commit，便于回滚（QUAL-07/08 改测试 infra，放最后；QUAL-05/06/51-09 改生产代码，先跑全量测试再 commit）
- **Linux runner 跑通**：本 phase 改的 nftables / worker cap / namespace 都是 Linux-only，必须 deferred-to-CI 验证；darwin 上 cross-compile 通过即可
- **Phase 47/49 e2e 用例 fix 验证**：51-05/06/51-09 落地后，跑 Phase 47/49 既有用例（darwin 只验编译；Linux 验证留 CI）

### Claude's Discretion

- worker `--cap-drop NET_RAW` 与 `--cap-add NET_ADMIN` 的精确权衡（按源码实际依赖决定保留哪些 cap）
- `goleak.VerifyTestMain` 在哪些 package 接入（建议至少 `cmd/control-plane` / `cmd/host-agent` / `cmd/cloud-claude` 三个主包）
- `verify.go` 重构是否同时拆分文件（建议不拆，保持兼容）
- 51-09 双绑 error code 命名（`egress_ip_already_bound` 还是 `EGRESS_IP_ALREADY_BOUND`，按现有 error code 命名风格）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 46 `Vote` / `DefaultDenyMatrix`**：QUAL-01 / QUAL-02 直接复用
- **Phase 49 `ParseNftRules` / `ParseProcCapabilities`**：QUAL-05 / QUAL-06 落地后 e2e 用例自动转 PASS
- **Phase 47 `ParseBindEgressIPResponse`**：51-09 落地后 `ErrorCode` 字段命中
- **既有 186 个 `_test.go`**：QUAL-07 -race -shuffle 必须保证全绿
- **`go.mod`**：本 phase 唯一允许新增依赖 `go.uber.org/goleak`

### Established Patterns

- **build tag**：`*_linux.go` 文件 + `//go:build linux` 用于平台 specific 代码
- **错误包装**：`fmt.Errorf("...: %w", err)`
- **中文沟通 + 错误消息**：用户面向错误消息（HTTP 4xx response）默认中文
- **structured logging**：`log/slog` key-value

### Integration Points

- **生产代码改动文件清单**：
  - `internal/network/verify.go`（QUAL-01..03）
  - `internal/network/namespace.go`（QUAL-04）
  - `internal/network/worker_firewall_linux.go`（QUAL-05）
  - `internal/runtime/tasks/worker.go`（QUAL-06）
  - `internal/controlplane/http/admin_bindings.go`（51-09，路径以 grep 为准）
  - `internal/controlplane/http/errors.go`（51-09 error code 常量，路径以 grep 为准）
  - `Makefile`（QUAL-07）
  - `.github/workflows/ci.yml`（QUAL-07）
  - 各 `cmd/<service>/main_test.go` 或 `cmd/<service>/testmain_test.go`（QUAL-08）
- **`go.mod` / `go.sum`**：新增 `go.uber.org/goleak`
- **不动 tests/e2e/**：本 phase 不写新 e2e 用例；之前 Phase 47/49 用例自动从 GAP → PASS

</code_context>

<specifics>
## Specific Ideas

- **`verify.go::verifyEgressIPMulti` 签名**：
  ```go
  func verifyEgressIPMulti(ctx context.Context, sources []string) (VerifyResult, error)
  type VerifyResult struct {
      IP      string
      Vote    VoteOutcome // 复用 Phase 46
      Sources map[string]string
  }
  ```
- **`namespace.go::GetContainerNetNS` 新签名**：
  ```go
  func GetContainerNetNS(ctx context.Context, id string, opts ...Option) (string, error)
  type Option func(*config)
  func WithProbeWindow(d time.Duration) Option
  func WithMaxRetries(n int) Option
  ```
- **`worker_firewall_linux.go` 新 nft 规则**（伪 nft 表达式）：
  ```
  add rule ip cloudproxy worker-output ip daddr 169.254.0.0/16 counter drop comment "linklocal-drop"
  ```
- **51-09 admin_bindings.go 双绑 pre-check**（伪代码）：
  ```go
  var existing Binding
  err := tx.QueryRow(ctx, "SELECT host_id FROM host_egress_bindings WHERE egress_ip_id=$1", egressIPID).Scan(&existing.HostID)
  if err == nil && existing.HostID != requestedHostID {
      return ErrEgressIPAlreadyBound // 转 409 + error code
  }
  ```

</specifics>

<deferred>
## Deferred Ideas

- **多协议泄漏扩展**（mDNS / LLMNR / NetBIOS / SSDP 等）：本 phase 锁 8 + 1 plan，新协议属后续
- **verify.go 完整重构**：本 phase 仅加固，不重构（保持调用方兼容）
- **goroutine 泄漏深度排查**：本 phase 仅接入 goleak + 排除已知泄漏 top function，未知泄漏深度排查属后续
- **CI workflow 性能优化**（`-race` 会让测试变慢）：本 phase 不做基线对比，性能优化属后续
- **51-09 双绑互斥同时引入并发安全**（如果发现是 race condition 而非简单 pre-check 缺失）：本 phase 仅加 pre-check + error code；race condition 修复属后续
- **Linux runner 真机签字**：deferred-to-CI

</deferred>
