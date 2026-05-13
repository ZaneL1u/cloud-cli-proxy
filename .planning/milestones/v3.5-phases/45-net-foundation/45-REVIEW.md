---
phase: 45-net-foundation
reviewed: 2026-05-12T16:00:00Z
depth: standard
files_reviewed: 20
files_reviewed_list:
  - internal/network/call_order_test.go
  - internal/network/container_proxy_dns_test.go
  - internal/network/container_proxy_provider.go
  - internal/network/container_proxy_provider_test.go
  - internal/network/gateway_singbox_config.go
  - internal/network/gateway_singbox_config_test.go
  - internal/network/provider.go
  - internal/network/routing_provider_linux.go
  - internal/network/singbox_provider_linux.go
  - internal/network/verify.go
  - internal/network/verify_test.go
  - internal/runtime/tasks/worker.go
  - internal/runtime/tasks/worker_password_test.go
  - internal/runtime/tasks/worker_volume_test.go
  - internal/store/migrations/0019_host_bypass_rules.sql
  - internal/store/repository/migration_0019_test.go
  - internal/store/repository/models.go
  - internal/store/repository/queries_bypass.go
  - internal/store/repository/queries_bypass_test.go
findings:
  critical: 2
  warning: 8
  info: 5
  total: 15
status: fixed
fix_iteration: 1
fixed_findings:
  - CR-01
  - CR-02
  - WR-01
  - WR-02
  - WR-03
  - WR-04
  - WR-05
  - WR-06
  - WR-07
  - WR-08
skipped_findings:
  - IN-01
  - IN-02
  - IN-03
  - IN-04
  - IN-05
---

# Phase 45 网络配置基础与数据模型 — 代码评审报告

**评审时间：** 2026-05-12T16:00:00Z
**评审深度：** standard
**评审文件数：** 20
**状态：** issues_found（发现 2 BLOCKER / 8 WARNING / 5 INFO）

## 摘要

Phase 45 三个 Plan（sing-box 渲染 v3.5、容器 DNS 入口锁、bypass 数据模型与 19 个 Repository 方法）整体结构清晰，安全意图明确（is_system 双层删除拦截、ro bind mount、占位文件预写、call-order 文本守护测试）。pgx 全部使用参数化占位符，不存在 SQL injection 风险。

但有两处 BLOCKER：

1. **`queries_bypass.go` 把可空 UUID 列处理成空字符串而非 NULL**——`InsertBypassAuditLog` / `CreateBypassRule` / `CreateBypassBinding` 等多处用 `if ptr != nil { arg = *ptr }`，当上层传入指向空串的指针时会把空串塞进 `UUID REFERENCES users(id)` 列，pgx 直接报 syntax error，且 schema 的 `CHECK (host_id IS NOT NULL)` 也拦不住"指向空串的非 nil 指针"。
2. **`createHost` 失败路径不清理 PrepareGateway 写盘的 placeholder / DNS 源文件**——`docker create` / `docker start` / `PrepareHost` 任一失败后，`createHost` 直接 return，外层 `Execute` 不会调 `CleanupHost`，gateway 容器、network、`<DATA_DIR>/gateway/<host_id>/*` 全部残留；下一次 `createHost` 虽然进入 `PrepareGateway` 会 `teardownGateway` 自愈，但任一失败到下次重试之间，残留的 sing-box gateway 容器仍在向上游代理转发流量（资源泄漏 + 计费风险）。

WARNING 集中在：startHost 调用顺序无 call-order 测试守护；call_order_test 用 `\nfunc ` 拆分函数体的方式较脆；`teardownGateway` 内的 `os.Hostname()` 错误被静默忽略；`waitGatewayHealthy` 把已 Running 但 sing-box 还没真正 healthy 的容器误判为 healthy；`mustParseUint` 忽略 strconv 错误；DNS 拦截 hook 的 `nsswitch.conf` 顺序排错可能漏 `myhostname`；以及若干并发安全语义未声明。

INFO 主要是注释中的英文/拼写小问题与一些可读性建议。

---

## Critical Issues

### CR-01: 可空 UUID/JSONB 列被指针解引用塞进空串，pgx 会直接 syntax error，导致 audit log/规则插入失败

**File:** `internal/store/repository/queries_bypass.go:545-568, 381-394, 447-467, 504-525`
**Issue:**
多个 Create*/Insert* 方法采用统一模式：

```go
var actorArg any
if params.ActorID != nil {
    actorArg = *params.ActorID
}
```

当上层调用方传入 `*string` 指针指向**空字符串**（`ptr := "" ; params.ActorID = &ptr`，例如从 admin handler 还未登录的匿名上下文、或来自上层 `mapstructure`/JSON 反序列化时空字段被指针化的情况），`actorArg` 会变成空串 `""`，pgx 把空串当 UUID 发送给 PostgreSQL，PG 直接抛 `invalid input syntax for type uuid: ""`，导致整条 audit log / rule / binding / snapshot 插入失败。

同样问题出现在：
- `InsertBypassAuditLog`：`ActorID` / `TargetID` 两个 UUID 列；`ActorIP`/`Note` 已用 `nullIfEmpty`，但 ActorID/TargetID 没有。
- `CreateBypassRule`：`HostID *string` 路径（scope='host' 时必须有值，但 caller 传 `&""` 会绕过空检测，触发 PG 类型错误而不是 schema 的 `host_id IS NOT NULL` 拦截）。
- `CreateBypassBinding`：`PresetID` / `RuleID` 同模式。
- `CreateBypassSnapshot`：`CreatedBy` 同模式。
- `UpdateBypassPreset` / `UpdateBypassRule`：`COALESCE($N, col)` 对 `nil` 走 fallback、对空串走 SET ""，业务语义"未指定字段不更新"会被空串静默改写（特别是 `value` 列也允许被设置为空串）。

`schema 的 CHECK (host_id IS NOT NULL)` 无法拦截"非 nil 指针指向空串"——PG 看到的是空串、不是 NULL。

**Fix:**
所有可空 UUID/可选字符串字段一律走 `nullIfEmpty` 等价检查；指针非 nil 但值为空也必须转 NULL。建议改用如下统一 helper：

```go
// nullableUUIDArg returns nil (SQL NULL) when ptr is nil OR points to empty string.
func nullableUUIDArg(ptr *string) any {
    if ptr == nil || *ptr == "" {
        return nil
    }
    return *ptr
}

// 调用点（举例 InsertBypassAuditLog）：
if err := r.db.QueryRow(ctx, insertBypassAuditLogSQL,
    nullableUUIDArg(params.ActorID),
    nullIfEmpty(params.ActorIP),
    params.Action, params.TargetKind,
    nullableUUIDArg(params.TargetID),
    nullIfEmptyJSON(params.Before),
    nullIfEmptyJSON(params.After),
    nullIfEmpty(params.Note),
).Scan(&id, &createdAt); err != nil { ... }
```

对 `CreateBypassRule` 等场景，应在方法体头部显式校验：
```go
if params.Scope == "host" && (params.HostID == nil || *params.HostID == "") {
    return BypassRule{}, fmt.Errorf("create bypass rule: scope=host requires non-empty host_id")
}
if params.Scope == "global" && params.HostID != nil && *params.HostID != "" {
    return BypassRule{}, fmt.Errorf("create bypass rule: scope=global must not carry host_id")
}
```
让数据层用 Go 错误而不是 pgx 类型错误反馈违规。同步给 `Update*` 方法的"COALESCE 空串"语义补 unit 测试，明确 `*Value = "" 视为不更新还是清空`。

---

### CR-02: createHost 中 docker create/start/PrepareHost 任一失败，PrepareGateway 已写的 placeholder + DNS 源文件 + 已启动的 gateway 容器都不会被回滚

**File:** `internal/runtime/tasks/worker.go:374-414` 与 `internal/network/container_proxy_provider.go:53-142`
**Issue:**
`createHost` 在 line 379 调 `w.provider.PrepareGateway(ctx, spec)`，`ContainerProxyProvider.PrepareGateway` 内部：
- line 88-105：写 `config.json` / `whitelist-cidrs.json` / `whitelist-domains.json` / `resolv.conf` / `nsswitch.conf` 到 `<DATA_DIR>/gateway/<host_id>/`；
- line 114：`dockerNetworkCreate`；
- line 119：`dockerRunGateway` 启动 sing-box gateway 容器（监听 tproxy:7892 + tun0 DNS）；
- line 124：`dockerNetworkConnect` 把 gateway 连到 bridge。

`PrepareGateway` **自身**在 124/129 之后失败会 `p.teardownGateway` 清理。但**返回成功后**：

```go
// worker.go createHost
if err := w.provider.PrepareGateway(ctx, spec); err != nil { ... }   // ✓ 成功
args, err := w.buildCreateArgs(...)                                   // ① 这里失败：泄漏
if err := w.runDocker(ctx, args...); err != nil { return err }        // ② 这里失败：泄漏
if err := w.runDocker(ctx, "start", containerName); err != nil { ... }// ③ 这里失败：泄漏
if err := w.provider.PrepareHost(ctx, spec); err != nil { ... }       // ④ PrepareHost 内部失败时调 teardown，
                                                                       //   但 prepare worker 容器连接前的 dockerNetworkConnect 失败仍会触发，OK
if err := w.waitForSSH(...); err != nil { return err }                // ⑤ 这里失败：泄漏
```

`Execute` 失败路径只做了 `docker stop containerName`（worker.go:108）+ `UpdateHostStatus(failed)`，**没有调 `provider.CleanupHost`**。

结果：①/②/③/⑤ 任一失败，gateway 容器仍 running、netcetwork 仍存在、`<DATA_DIR>/gateway/<host_id>/` 仍占据磁盘，且 sing-box gateway 仍在向上游代理建立连接（出口 IP 计费/流量泄漏面攻击面）。下次 `createHost` 才借 `PrepareGateway` 第 85 行的 `p.teardownGateway` 自愈，但在两次之间残留窗口可达数小时（直到用户重试或运维介入）。

`rebuildHost` 路径（line 522）显式调了 `CleanupHost`；`stopHost`（line 492）也调。`createHost` 失败路径是唯一缺口。

**Fix:**
在 `worker.go::createHost` 失败路径补 cleanup；最小 patch 是在每个 return err 之前加一次 `CleanupHost`：

```go
// worker.go createHost — 加 cleanup defer 守护 PrepareGateway 写盘
if egressCfg != nil {
    spec := network.HostNetworkSpec{HostID: request.HostID, Egress: egressCfg}
    if err := w.provider.PrepareGateway(ctx, spec); err != nil {
        w.recordNetworkError(ctx, request.HostID, err)
        return fmt.Errorf("prepare gateway before create: %w", err)
    }
    // 一旦 PrepareGateway 成功，任何后续失败都必须 CleanupHost
    var createSucceeded bool
    defer func() {
        if !createSucceeded {
            _ = w.provider.CleanupHost(context.Background(), spec)
        }
    }()
    // … buildCreateArgs / docker create / start / PrepareHost / waitForSSH …
    createSucceeded = true
}
```

注意：`defer` 用 `context.Background()` 而非 `ctx`，避免 task 超时取消时 cleanup 也被中断。Cleanup 内部本来就是 best-effort（teardownGateway 全部 `_ =` 忽略错误），所以幂等。

或更保守：把 cleanup 改成 explicit `if err != nil { cleanup(); return }` 模式，避免 defer 改变错误返回时机带来的隐式控制流。

---

## Warnings

### WR-01: startHost 与 rebuildHost 的调用顺序未被 call-order 测试守护，仅 createHost 有

**File:** `internal/network/call_order_test.go:20-72` 与 `internal/runtime/tasks/worker.go:416-478, 499-543`
**Issue:**
`call_order_test.go` 只测试 `createHost` 函数体内的 5 个 marker 顺序。但 `startHost` 也有同样的硬约束（PrepareGateway → docker start → PrepareHost），重构时同样可能被改错；`rebuildHost` 间接走 `stopHost → CleanupHost → createHost`，也需要 CleanupHost 在 createHost 之前。

更糟的是测试用 `strings.Index` 找**首次出现**的 marker，如果有人把 PrepareGateway 调用复制到 startHost 函数体的第一行又留了一个在第二个位置，测试也不会发现。

**Fix:**
扩展 `call_order_test.go`，加 `TestWorker_StartHost_CallOrder` 与 `TestWorker_RebuildHost_CallOrder`；用相同的 `startSig`/`endSig`+`strings.Index` 模式覆盖：
- startHost：`PrepareGateway` < `docker_start` < `PrepareHost`
- rebuildHost：`stopHost`/`CleanupHost` < `createHost`

每个测试单独定位函数体，避免标志符在跨函数误匹配。

---

### WR-02: call_order_test 使用 `\nfunc ` 拆分函数体不健壮——遇到嵌套匿名函数会被截断

**File:** `internal/network/call_order_test.go:36-42`
**Issue:**
```go
rest := content[startIdx+len(startSig):]
endRel := strings.Index(rest, "\nfunc ")
if endRel < 0 { t.Fatalf(...) }
body := rest[:endRel]
```

如果将来 `createHost` 内嵌一个 `go func() { ... }()` 或 `defer func() {...}()` 用换行格式，`\nfunc ` 串会出现在闭包**内部**（虽然典型 gofmt 不会换行 `func(` 到新行，但 long signature 闭包会）。一旦截断点偏前，后续 PrepareHost / docker_start 等 marker 就会被误判为"未找到"，CI 直接 fatal，与 call-order 是否真正违规无关。

**Fix:**
改用更准确的"顶层 `func ` 启始"匹配：扫描 `\nfunc ` 后必须紧跟函数名，且首列非缩进（顶层）。最简单的鲁棒法：

```go
// 找下一个出现在第 0 列的 "func " 行
lines := strings.Split(rest, "\n")
acc := 0
endRel := -1
for i, ln := range lines {
    if i > 0 && strings.HasPrefix(ln, "func ") {
        endRel = acc
        break
    }
    acc += len(ln) + 1 // +1 是 \n
}
if endRel < 0 { ... }
body := rest[:endRel]
```

或者直接 `go/parser` 解析 AST（注意 build tag 影响），但纯文本足够。

---

### WR-03: `waitGatewayHealthy` 把"docker inspect Running=true"等同于"sing-box healthy"，实际 sing-box 可能仍在初始化 tun0

**File:** `internal/network/container_proxy_provider.go:369-387`
**Issue:**
```go
cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", gwName)
out, err := cmd.Output()
if err == nil && strings.TrimSpace(string(out)) == "true" {
    logs, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "120", gwName).CombinedOutput()
    s := string(logs)
    if strings.Contains(s, "FATAL") || strings.Contains(s, "panic:") {
        return fmt.Errorf("gateway sing-box failed: %s", strings.TrimSpace(s))
    }
    time.Sleep(500 * time.Millisecond)
    return nil
}
```

判定为 healthy 的条件是：容器 Running=true 且 logs 不含 FATAL/panic。但 sing-box 启动需要：（1）解析 config，（2）建立 tun0，（3）启动 DoH 拨号到 1.1.1.1，（4）建立 proxy outbound。前 500ms 内 tun0 可能还没建好，worker 容器 docker create 用 ro bind mount 引用的 `/etc/resolv.conf` 指向 `172.19.0.1` —— 但 tun0 还没监听！worker 启动后立即跑 `getent hosts ...` 会得到 SERVFAIL（甚至连不上 172.19.0.1）。

更隐蔽的：sing-box 启动失败日志格式不只 "FATAL"/"panic:" 两种关键字。例如 sing-box 1.11 在 config 解析错误时输出 `start service: ...` 或 `unmarshal:`，这些都不会被 `Contains("FATAL")` 抓到，函数返回 nil（误判 healthy）。

**Fix:**
- 真正等待 tun0 就绪：把 `waitForTun0` 这种**对 gateway 容器内部 tun0**的探测搬过来：
  ```go
  cmd := exec.CommandContext(ctx, "docker", "exec", gwName, "ip", "link", "show", "tun0")
  if cmd.Run() == nil { /* tun0 ready */ }
  ```
- 或者更严格：用 `docker exec gwName ss -lnu '( sport = :53 )'` 探测 sing-box DNS 监听到位。
- 把日志关键字检测扩展为正则 `(?i)\b(fatal|panic|error|failed to start)\b`，并把"启动失败"的样本日志列在 docstring。

---

### WR-04: `teardownGateway` 内 `os.Hostname()` 的错误被静默吞掉，导致控制面网络断开失败时无审计

**File:** `internal/network/container_proxy_provider.go:250-252`
**Issue:**
```go
if cpID, _ := os.Hostname(); cpID != "" {
    _ = exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run()
}
```

`os.Hostname()` 错误被忽略，`docker network disconnect` 命令本身也被 `_ =` 吞掉。后果：如果 `os.Hostname()` 在某些容器化部署里返回错误，控制面**永远不会**被断开 isolated network；如果 disconnect 失败（极少见，但内部 dockerd 状态机异常时可能），残留连接也无任何审计 trace，难以排障。

`PrepareHost` 中（line 222-225）对应位置至少 `Warn` 了日志。`teardownGateway` 没 logger 注入，没法 Warn。

**Fix:**
要么给 `teardownGateway` 注入 logger（与 `PrepareHost` 一致），把所有 `_ =` 路径改为 `if err := ...; err != nil { p.logger.Warn(...) }`；要么至少把 `cpID` 失败的分支记录到 events（通过把 logger 作为 `p` 字段——本来就有 `p.logger`，line 252 没用到，明显疏漏）。

```go
func (p *ContainerProxyProvider) teardownGateway(ctx context.Context, hostID string) {
    netName := networkName(hostID)
    gwName := gatewayContainerName(hostID)
    workerName := workerContainerName(hostID)

    cleanupWorkerFirewall(ctx, workerName)

    cpID, err := os.Hostname()
    if err != nil {
        p.logger.Warn("teardown: get control-plane hostname failed", "error", err)
    } else if cpID != "" {
        if err := exec.CommandContext(ctx, "docker", "network", "disconnect", "-f", netName, cpID).Run(); err != nil {
            p.logger.Warn("teardown: disconnect control-plane from isolated network failed",
                "host_id", hostID, "cp_id", cpID, "error", err)
        }
    }
    // … 其余命令同理
}
```

---

### WR-05: `mustParseUint` 静默吞掉 strconv 错误，错误输入返回 0 可能导致下游路由错乱

**File:** `internal/network/singbox_provider_linux.go:265-272`
**Issue:**
```go
func mustParseUint(b []byte) uint64 {
    s := string(b)
    for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
        s = s[:len(s)-1]
    }
    v, _ := strconv.ParseUint(s, 10, 64)
    return v
}
```

虽然这次提交里 `mustParseUint` 没有新调用方（只是被 review 的旧函数被一并 ship），但函数名 `mustParseUint` 与"静默返回 0"行为严重不符——`must*` 在 Go 习惯里意味着失败 panic，调用方读到 `mustParseUint(input)` 会以为是经过校验的值，直接拿去拼 IP/PID 字符串就会触发后续路由错乱（PID=0 是 systemd，nsenter -t 0 -n 直接挂掉 host！）。

**Fix:**
重命名 `mustParseUint` → `parseUintOrZero` 或干脆删掉这个 helper（让调用方显式 `strconv.ParseUint` 处理 error）；如果保留，至少加一行注释明确"失败返回 0，调用方必须自己 0 检查"，并 grep 所有调用方加 0 校验。

---

### WR-06: `nsswitch.conf` 没拒绝 `resolve`（systemd-resolved 入口），如果 worker 镜像装了 systemd-resolved，DNS 仍会走它而非 tun0

**File:** `internal/network/container_proxy_provider.go:28-38`
**Issue:**
锁定的 `hosts:          files dns` 只允许 NSS 走 `/etc/hosts` 与 `/etc/resolv.conf` 的标准解析。但如果 base 镜像 `cloud-cli-proxy-sing-gateway`（或将来的 worker image）安装了 `libnss-resolve` 包，且 systemd-resolved 监听 127.0.0.53，那 musl/glibc 是按 `hosts` 行的**模块名**找 `.so`，本配置已经显式排除了 `resolve`、`mdns`、`myhostname`，所以这条 OK。

但 `wins`/`mdns4_minimal`/`mdns6`/`mdns4` 等扩展也都没列入 forbidden 集合，测试 `container_proxy_dns_test.go:54` 的 forbidden 列表只检查 `mdns`/`myhostname`/`wins`/`nis_plus` 四个串。`mdns4_minimal` 包含 `mdns` 子串能被该测试捕到；`mdns6` 也包含；但 `dns_sd` / `resolve` 等 NSS 模块名**不在测试列表里**，如果镜像维护者将来在 nsswitch 里加进去也不会被这层守护测试发现。

**Fix:**
扩充 `container_proxy_dns_test.go` 的 forbidden 列表，参考 RHEL/Debian 官方 NSS 模块名清单：
```go
for _, forbidden := range []string{
    "mdns", "myhostname", "wins", "nis_plus",
    "resolve",    // systemd-resolved
    "dns_sd",     // Avahi DNS-SD
    "lwres",      // BIND lwresd
} {
    if strings.Contains(string(nsswitchData), forbidden) { ... }
}
```

同时也可考虑在 `nsswitchConfContent` 顶部加一行注释："# managed by cloud-cli-proxy; do not edit (overridden via ro bind mount)" 作为现场调试 hint。

---

### WR-07: `verifyDNS` 只校验 `/etc/resolv.conf` 第一行 nameserver；多 nameserver / 文件可读但被改空内容会被漏检

**File:** `internal/network/verify.go:86-112`
**Issue:**
```go
var firstNS string
for _, line := range strings.Split(string(out), "\n") {
    line = strings.TrimSpace(line)
    if strings.HasPrefix(line, "nameserver") {
        fields := strings.Fields(line)
        if len(fields) >= 2 {
            firstNS = fields[1]
            break  // ← 只取第一行就 return
        }
    }
}
result.DNSCorrect = firstNS == expectedDNS
```

虽然 ro bind mount 保证文件内容是 `nameserver 172.19.0.1\noptions ndots:0 single-request-reopen\n`，但：
- 如果有人在 `resolvConfContent` 后面加入第二行 `nameserver 8.8.8.8`（fallback），第一行仍是 172.19.0.1，verifyDNS 会通过，但容器实际查询会在 172.19.0.1 超时后 fallback 到 8.8.8.8（漏洞窗口）。
- 如果 `cat` 命令本身失败但 stderr 漏给了 stdout（极不常见），firstNS=""，`"" == "172.19.0.1"` 为 false，错误归类为 DNS leak——但 ActualDNS="" 让运维误以为 resolv.conf 被清空，实际可能是 nsenter 出错。这两个分支需要区分。

**Fix:**
- 显式校验 resolv.conf 内容**整体相等**于 `resolvConfContent`，而不仅"第一行 nameserver"：
  ```go
  expected := resolvConfContent
  if string(out) != expected {
      result.DNSCorrect = false
      result.ActualDNS = firstNS  // 仍 keep 第一行用于日志
      return
  }
  result.DNSCorrect = true
  result.ActualDNS = "172.19.0.1"
  ```
  这样任何额外的 fallback nameserver / 注释行都会立即被识别为 DNS lock-in 被破坏。
- 当 `err != nil`，区分"nsenter 失败"与"resolv.conf 缺失"：把 stderr 抓出来 log，回填到 result.ActualDNS 为 sentinel 串如 `"<read failed: <err>>"`。

---

### WR-08: 并发 PrepareGateway 同 hostID 时 teardown 与 setup 交错——文档已说"safe for concurrent use" 但实现不持锁

**File:** `internal/network/provider.go:18-22` 与 `internal/network/container_proxy_provider.go:53-142`
**Issue:**
`Provider` 接口 docstring 明文："Implementations must be safe for concurrent use." 但 `ContainerProxyProvider` 没有任何 mutex / sync.Map，多个 goroutine 用同一个 hostID 调用 `PrepareGateway` 时：

1. G1 执行到 line 85 `p.teardownGateway(ctx, hostID)`（清干净）
2. G1 line 88 创建 configDir、写文件
3. G2 执行到 line 85 `p.teardownGateway(ctx, hostID)`（把 G1 写的全删了！）
4. G1 line 114 `dockerNetworkCreate` 成功（因为 G2 已经把 G1 的 network 删了）
5. G2 line 88 重新写文件、line 114 `dockerNetworkCreate` 失败（network 已经存在，name 冲突）

虽然实际上控制面通过 task queue 保证同一 hostID 一次只有一个 worker 在跑（task 表 unique 或同步原语），但 docstring 的"safe for concurrent use" 与实现不符——如果将来 host-agent 走 SDK 形式被嵌入到管理多并发的服务里，会踩这个坑。

**Fix:**
两选一：
- **改文档**：把 docstring 改为 "Implementations are NOT goroutine-safe for the same host_id; callers must serialize per-host_id."。
- **加锁**：在 `ContainerProxyProvider` 里加 `sync.Map[hostID]*sync.Mutex`，PrepareGateway/PrepareHost/CleanupHost 入口取锁：
  ```go
  type ContainerProxyProvider struct {
      logger *slog.Logger
      locks  sync.Map // hostID -> *sync.Mutex
  }
  func (p *ContainerProxyProvider) lockFor(hostID string) func() {
      v, _ := p.locks.LoadOrStore(hostID, &sync.Mutex{})
      m := v.(*sync.Mutex)
      m.Lock()
      return m.Unlock
  }
  func (p *ContainerProxyProvider) PrepareGateway(ctx context.Context, spec HostNetworkSpec) error {
      defer p.lockFor(spec.HostID)()
      // …
  }
  ```

---

## Info

### IN-01: `gateway_singbox_config.go::buildGatewayDNS` 注释里 `domain_resolver=dns-local 用于解析 1.1.1.1 的 SNI 主机名（这里是 IP，但保持配置完整性）` 半截子说明不清

**File:** `internal/network/gateway_singbox_config.go:96-100`
**Issue:**
注释提到 "1.1.1.1 的 SNI 主机名（这里是 IP）" 但读者会困惑：既然是 IP 就没 SNI 需要解析，为什么还配 `domain_resolver`？实际语义是 sing-box 1.11+ 的 DoH outbound 必须显式声明 `domain_resolver` 否则启动会拒绝配置（防止递归解析死锁）。

**Fix:** 改为：
```go
// 备注：sing-box 1.11+ 强制 DoH server 显式声明 domain_resolver 防止递归解析；
// 即使本案 server 已是 IP（1.1.1.1）无需 DNS 解析，配置完整性也要求此字段。
```

---

### IN-02: `models.go:182-184` HostWithClaudeAccount docstring 提到"PersistentVolumeName 空 = 未分配 volume 或无 account"——但 struct 字段是 `string` 非 `*string`，无法区分两者

**File:** `internal/store/repository/models.go:182-185`
**Issue:**
```go
type HostWithClaudeAccount struct {
    Host
    PersistentVolumeName string `json:"persistent_volume_name,omitempty"`
}
```

注释说"空 = 该 host 关联 account 未分配 volume 或无 account"，但实际 string 类型空值 ("") 与 SQL 的 NULL 是两种状态，pgx scan 到 string 类型遇到 NULL 会报错。如果用 LEFT JOIN（注释明示 LEFT JOIN）则 NULL 必然出现。

**Fix:** 改成 `*string` 让 NULL 与空串区分，或保持 string 但显式在 SELECT 用 `COALESCE(persistent_volume_name, '') AS persistent_volume_name`。检查现有 `GetHostWithClaudeAccount` 查询的 SELECT 列表（不在本评审范围，但作为下游 follow-up 提示）。

---

### IN-03: 0019 migration 用 `IF NOT EXISTS` 已 idempotent，但 seed INSERT 用 `ON CONFLICT (slug) DO NOTHING` ——重复 apply 的"name/description/rules 漂移"无 detection

**File:** `internal/store/migrations/0019_host_bypass_rules.sql:92-102`
**Issue:**
migration 重复 apply 时：
- 表存在 → `IF NOT EXISTS` skip ✓
- seed 存在 → `ON CONFLICT (slug) DO NOTHING` skip ✓

但如果将来运维误改了 production 的 seed（比如 SQL 手动 UPDATE 了 `loopback` 的 rules CIDR 集合），下一次 migration apply **不会**纠正回 canonical 内容。Phase 47 host-agent 依赖这两个 seed 的 rules 生成 sing-box rule-set；漂移会导致 silent 白名单偏差。

**Fix:**
两选一：
- **接受漂移**：在 docstring 明确"系统预设的 rules JSONB 是 immutable 文档化值，运维不得手改"，并在 Phase 46 admin handler 加 `READ-ONLY` 路径（已通过 SQL 层的 `AND is_system = FALSE` 兜底）。
- **每次 migration 强制重写 system seed**：把 `ON CONFLICT DO NOTHING` 改为 `ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, rules=EXCLUDED.rules, updated_at=NOW() WHERE host_bypass_presets.is_system = TRUE`。这样 migration 每次 apply 都会"自愈"系统预设。

推荐第二种（更对齐 GitOps "声明式 schema" 原则），但需要在 PR description 里明确"再次 apply 0019 会覆盖运维手改的系统预设"。

---

### IN-04: 测试 `TestGatewayConfigDir_PathSanitization` 验证传入恶意 hostID（含 `..`、`\x00`、`/`）只断言 path 拼接结果，没验证拒绝写入

**File:** `internal/network/container_proxy_provider_test.go:135-157`
**Issue:**
测试只是验证 `GatewayConfigDir("host/with/slash")` 返回 `/var/lib/cloud-cli-proxy/gateway/host/with/slash`——但**没有**验证后续 `WriteContainerDNSConfig` / `PrepareGateway` 拒绝这种危险 hostID。实际 hostID 来源是 DB hosts 表的 UUID，pgx UUID 类型校验阻止恶意值，所以风险在 v1 是低的。但测试名 `PathSanitization` 暗示"已经做了 sanitization"——实际并没有，命名误导。

**Fix:**
- 改名为 `TestGatewayConfigDir_PathTraversalSemantics`，并加注释明确"hostID 上游必须是 UUID；本测试只验证拼接行为，不验证拒绝危险输入"。
- 或在 `GatewayConfigDir`/`WriteContainerDNSConfig` 入口加 UUID 格式校验（regexp 或 `uuid.Parse`），把"路径 traversal 防护"前推到边界，但这会引入新依赖且与控制面已有 UUID 边界重复，trade-off 倾向"不在数据层重复校验"。

---

### IN-05: `worker.go:213-215` 平台分支 `runtime.GOOS != "linux"` 注释挪到 buildCreateArgs 顶部更友好

**File:** `internal/runtime/tasks/worker.go:213-215`
**Issue:**
```go
// macOS/Windows: expose SSH port via host port mapping because Docker Desktop
// cannot route directly to container internal IPs from the host.
if runtime.GOOS != "linux" {
    args = append(args, "-p", "0:22")
}
```

注释解释了"为什么 macOS 加 -p 0:22"。但同一函数 line 278 起的 DNS bind mount 也涉及 Linux-only 路径假设（`/etc/resolv.conf`/`/etc/nsswitch.conf` 是 Linux glibc 路径，不在 macOS 的容器内表达里），却没有平台守卫。

实际上 worker 容器本身一定是 Linux 容器（即使宿主是 macOS Docker Desktop），所以 `/etc/resolv.conf` 在容器里**确实**存在；这段代码 OK。但读者在 buildCreateArgs 顶部看不到"宿主 vs 容器"OS 的区分提示，容易误以为 macOS 上 bind mount 也要绕。

**Fix:** 在 buildCreateArgs 函数顶部加一行 doc：
```go
// buildCreateArgs 组装 `docker create` 命令行。注意：所有 `--mount` /
// 文件路径假设的是**容器内**的 Linux 视图，不依赖宿主 OS；只有 host port
// 映射（-p 0:22）和宿主路径相关的 bind mount 才需要 runtime.GOOS 守卫。
```

---

_Reviewed: 2026-05-12T16:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
