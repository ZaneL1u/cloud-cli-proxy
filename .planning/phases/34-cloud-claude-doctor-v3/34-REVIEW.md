---
phase: 34-cloud-claude-doctor-v3
reviewed: 2026-04-21T00:00:00Z
depth: standard
files_reviewed: 41
files_reviewed_list:
  - Makefile
  - cmd/cloud-claude/doctor.go
  - cmd/cloud-claude/explain.go
  - cmd/cloud-claude/explain_test.go
  - cmd/cloud-claude/main.go
  - internal/cloudclaude/colors.go
  - internal/cloudclaude/doctor/auth.go
  - internal/cloudclaude/doctor/auth_test.go
  - internal/cloudclaude/doctor/check.go
  - internal/cloudclaude/doctor/disk.go
  - internal/cloudclaude/doctor/disk_test.go
  - internal/cloudclaude/doctor/doctor.go
  - internal/cloudclaude/doctor/doctor_test.go
  - internal/cloudclaude/doctor/fix.go
  - internal/cloudclaude/doctor/fix_test.go
  - internal/cloudclaude/doctor/integration_test.go
  - internal/cloudclaude/doctor/mount.go
  - internal/cloudclaude/doctor/mount_test.go
  - internal/cloudclaude/doctor/network.go
  - internal/cloudclaude/doctor/network_test.go
  - internal/cloudclaude/doctor/remote_runner.go
  - internal/cloudclaude/doctor/render.go
  - internal/cloudclaude/doctor/render_test.go
  - internal/cloudclaude/doctor/ssh.go
  - internal/cloudclaude/doctor/ssh_test.go
  - internal/cloudclaude/errcodes/auth.go
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/codes_test.go
  - internal/cloudclaude/errcodes/disk.go
  - internal/cloudclaude/errcodes/explanations.go
  - internal/cloudclaude/errcodes/explanations_test.go
  - internal/cloudclaude/errcodes/ssh.go
  - internal/cloudclaude/errcodes/state.go
  - internal/cloudclaude/errcodes/system.go
  - internal/cloudclaude/input_buffer.go
  - internal/cloudclaude/input_buffer_test.go
  - internal/cloudclaude/last_session.go
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/session.go
  - internal/cloudclaude/session_test.go
  - scripts/ci-doctor-grep.sh
findings:
  critical: 0
  warning: 8
  info: 7
  total: 15
status: issues_found
---

# Phase 34: Code Review Report

**Reviewed:** 2026-04-21
**Depth:** standard
**Files Reviewed:** 41
**Status:** issues_found

## Summary

本次审阅覆盖 Phase 34（cloud-claude doctor v3 + explain + errcodes 8 域闭合）涉及的 41 个源文件，包括：
`cmd/cloud-claude/{doctor,explain,main}.go`、`internal/cloudclaude/doctor/` 全套包、
`internal/cloudclaude/errcodes/` 8 域注册表与长说明、`internal/cloudclaude/{input_buffer,session,mount_strategy,last_session,colors}.go`、
`scripts/ci-doctor-grep.sh` 与 `Makefile`。

整体设计良好：错误码全 `errcodes.Format` 走、降级链严格落 `last-session.json`、JSON schema_version 锁死并被 jq gate 覆盖、
`RemoteRunner` 抽象使单测可注入 fake、Fixer 5 类幂等检测齐备。CI grep gate（`scripts/ci-doctor-grep.sh`）很扎实，
真正闭合了 M14「所有 warn/fail 行必带建议 + 错误码」语义。

需要关注的点集中在三类：

1. **并发安全**（WR-01 / WR-02）：input_buffer.go 在 `SESSION_BUFFER_OVERFLOW` 路径下未持有 `echoMu` 直接写 `localEcho`；
   session.go `pTYAttachOnce` 异步 goroutine 写 `*registryPid` 与主循环复位无同步——这两处都是 Phase 32 既有代码，但在 Phase 34 文件清单内。
2. **Goroutine 泄漏 / 超时不可中断**（WR-03）：`doctor/check.go::runWithTimeout` 用 select + buffered channel 兜底超时，
   但被它包装的 `RemoteRunner.RunScript` 接口没有 ctx 参数，超时后底层 SSH session 仍在跑直到自然结束，造成 goroutine + SSH session 连带泄漏。
3. **防御性 nil/类型断言**（WR-04 / WR-07）：`auth.go::checkEntryTokenValid` 在 `err==nil` 时直接读 `resp.ImageVersion`；
   `fix.go::fixFUSEResidualMount` 把 `Details["mountpoints"]` 强转 `[]string`，对 JSON 反序列化场景不生效。

无 Critical 安全/数据丢失问题。下面按严重度列具体条目。

---

## Warnings

### WR-01: `BufferedStdin.handleReconnecting` 在 ringBuf 溢出路径写 `localEcho` 时未持有 `echoMu`

**File:** `internal/cloudclaude/input_buffer.go:99-110`
**Issue:**
`handleReconnecting` 第一段先 `b.ringMu.Lock()` 拿环形缓冲锁，然后在容量溢出时直接写 `b.localEcho`：

```go
b.ringMu.Lock()
if len(b.ringBuf) >= RingBufCapacity {
    ...
    if b.localEcho != nil {
        fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
    }
}
b.ringBuf = append(b.ringBuf, c)
b.ringMu.Unlock()
```

这里完全没有持 `echoMu`。但同一时刻 `Flush()`（外层 reconnect 成功回调）可能正持 `echoMu` 调用 `closeGrayIfOpenLocked()` 写
`AnsiReset` 到 `localEcho`，或者 `Run` 在 `StateConnected` 分支持 `echoMu` 调 `closeGrayIfOpenLocked`。
两个 goroutine 并发写同一 `io.Writer` → 与文件头部 `// 锁顺序：echoMu 外 / ringMu 内` 的契约直接冲突，`go test -race` 会报。
生产路径下 `localEcho` 通常是 `os.Stderr`，单次 `Write` 系统调用基本原子，但混插 ANSI 转义后可视效果可能错位；测试侧的 `syncBuffer`
明确加锁规避，已是症状证据。

**Fix:**

```go
b.ringMu.Lock()
overflowed := false
if len(b.ringBuf) >= RingBufCapacity {
    drop := 1024
    if drop > len(b.ringBuf) {
        drop = len(b.ringBuf)
    }
    b.ringBuf = b.ringBuf[drop:]
    overflowed = true
}
b.ringBuf = append(b.ringBuf, c)
b.ringMu.Unlock()

if overflowed && b.localEcho != nil {
    b.echoMu.Lock()
    fmt.Fprintln(b.localEcho, errcodes.Format(errcodes.SESSION_BUFFER_OVERFLOW))
    b.echoMu.Unlock()
}
```

把 echo 写出移出 ringMu 临界区并补上 echoMu，与下面 `if b.localEcho != nil` 灰色 echo 段一致即可。

---

### WR-02: `pTYAttachOnce` 异步 goroutine 写 `*registryPid`，与主循环并发读写无同步

**File:** `internal/cloudclaude/session.go:774-788`（写）+ `internal/cloudclaude/session.go:692, 654-658, 666`（读 / 复位）
**Issue:**
`pTYAttachOnce` 启动一个 goroutine 调 `writeClientFile` 后写 `*registryPid = pid`：

```go
if *registryPid == 0 {
    go func() {
        ...
        pid, werr := writeClientFile(conn, sessionName, ...)
        ...
        *registryPid = pid
    }()
}
waitErr := session.Wait()
```

但 `runClaudePTYWithReconnect` 主循环在 `pTYAttachOnce` 返回后会立刻：

```go
defer func() {
    if registryPid > 0 {
        _ = removeClientFile(conn, registryPid)
    }
}()
...
if registryPid > 0 {
    _ = removeClientFile(conn, registryPid)
    registryPid = 0
}
...
registryPid = 0  // 第 692 行 reconnect 后复位
```

这个 goroutine **没有任何 join 机制**：当 SSH session 在 goroutine 完成 `writeClientFile` 之前断开（reconnect 触发或正常退出），
主循环可能：(a) 读到 `registryPid=0` 跳过清理，但 goroutine 几毫秒后写入 pid，留下 orphan 注册表条目；
(b) 在主循环已 `registryPid = 0` 之后，goroutine 又把 pid 写回去，下一轮 attach 用错的 pid 调 `removeClientFile`。
`go test -race` 会确诊。

**Fix:**
建议改为同步等待写完再继续，或者改用 `sync.Once` + channel 给主循环一个等待点。最小修复：

```go
type registryWrite struct {
    pid int
    err error
}
ch := make(chan registryWrite, 1)
if *registryPid == 0 {
    go func() {
        clientRole := "primary"
        if sessionCfg.IsSecondaryClient {
            clientRole = "secondary"
        }
        pid, werr := writeClientFile(conn, sessionName, sessionCfg.AccountID, sessionCfg.LocalHostname, clientRole)
        ch <- registryWrite{pid, werr}
    }()
} else {
    close(ch) // 兜底：本来就有 pid，直接避免阻塞
}

waitErr := session.Wait()

select {
case rw := <-ch:
    if rw.err == nil {
        *registryPid = rw.pid
    } else if rw.err != nil {
        fmt.Fprintln(os.Stderr, "[!] writeClientFile 失败:", rw.err)
    }
default:
}
```

或者直接同步调 `writeClientFile` 不开 goroutine（写入是几十毫秒级 SSH RTT，PTY 主路径已有 `session.Wait`，没必要并发）。

---

### WR-03: `runWithTimeout` 在超时后 goroutine 仍在跑，`RemoteRunner.RunScript` 无 ctx 不可中断

**File:** `internal/cloudclaude/doctor/check.go:35-63` + `internal/cloudclaude/doctor/remote_runner.go:13-15`
**Issue:**
`runWithTimeout` 用 buffered channel + select 实现超时：

```go
done := make(chan Check, 1)
go func() {
    done <- fn(ctx2)
}()
select {
case c := <-done: ...
case <-ctx2.Done():
    return Check{... StatusFail / SYSTEM_CHECK_TIMEOUT ...}
}
```

但被包装的 `fn(ctx2)` 内部最终会调 `RemoteRunner.RunScript(name, script string)`——`RunScript` 接口签名**没有 ctx**：

```go
type RemoteRunner interface {
    RunScript(name, script string) (stdout, stderr string, err error)
}
```

生产实现 `sshRemoteRunner.RunScript` 走 `sess.Run(script)` 是同步阻塞调用，对 ctx 视而不见。
当 timeout 命中 → doctor 立刻返回 `SYSTEM_CHECK_TIMEOUT` 给上层，但底层 SSH session 与 goroutine 仍然挂着，
直到远端命令自己结束（或 `RunDoctor` 的 `defer closeRemote()` 关 conn 时被强制断开）。

最坏场景：5 个远端 check 串行各超 5s，第一个 hung → 看着像 25s 全跑完，实际宿主机有 5 个 leaked goroutine + 5 个 hung
SSH session 直到 conn close。verbose 模式 30s 放大这个窗口。

**Fix:**
给 `RemoteRunner.RunScript` 加 ctx 参数：

```go
type RemoteRunner interface {
    RunScript(ctx context.Context, name, script string) (stdout, stderr string, err error)
}
```

生产实现里在另一个 goroutine 跑 `sess.Run`，select on ctx：

```go
func (r *sshRemoteRunner) RunScript(ctx context.Context, name, script string) (string, string, error) {
    sess, err := r.conn.NewSession()
    if err != nil { return "", "", err }
    defer sess.Close()
    var stdout, stderr bytes.Buffer
    sess.Stdout = &stdout
    sess.Stderr = &stderr
    done := make(chan error, 1)
    go func() { done <- sess.Run(script) }()
    select {
    case err := <-done:
        ... // 原有逻辑
    case <-ctx.Done():
        _ = sess.Signal(ssh.SIGTERM) // 远端 SIGTERM；conn 还活着的话能强终
        _ = sess.Close()
        return stdout.String(), stderr.String(), ctx.Err()
    }
}
```

各 check 函数同步把 `ctx` 传下去（绝大多数 check 已经接收 ctx 了，只是没传给 runner）。

---

### WR-04: `checkEntryTokenValid` 在 err==nil 时直接 `resp.ImageVersion`，对 nil resp 缺防御

**File:** `internal/cloudclaude/doctor/auth.go:39-57`
**Issue:**

```go
resp, err := entryAuthenticate(ctx, cfg.Gateway, cfg.ShortID, cfg.Password)
if err != nil { ... }
details := map[string]any{}
if resp != nil {
    details["image_version"] = resp.ImageVersion
    ...
}
return Check{
    ...
    Message: fmt.Sprintf("Entry API 认证成功（image=%s）", resp.ImageVersion),
    ...
}, resp
```

`if resp != nil` 守住了 details，但下一行 `Message: fmt.Sprintf(... resp.ImageVersion)` 对 resp 直接解引用，**没守 nil**。
生产路径 `client.AuthenticateAndWait` 返回 `(nil, nil)` 应该是不可能的，但：

1. 测试已经把 `entryAuthenticate` 替换成包级 var；任何后续测试若返回 `(nil, nil)`，立即 panic。
2. 单元测试 `TestCheckEntryTokenValid_Success_Pass` 永远返回非 nil resp，不会暴露。

加上 nil 守护成本极低，符合 Plan 02 D-19 的"严守边界"风格。

**Fix:**

```go
if err != nil { ... }
if resp == nil {
    return newSkip("auth", "entry_token_valid", "Entry API 返回空响应"), nil
}
details := map[string]any{
    "image_version":     resp.ImageVersion,
    "claude_account_id": resp.ClaudeAccountID,
}
return Check{
    Domain: "auth", Name: "entry_token_valid", Status: StatusPass,
    Message: fmt.Sprintf("Entry API 认证成功（image=%s）", resp.ImageVersion),
    Details: details,
}, resp
```

---

### WR-05: `fixMutagenDaemonUnavailable` 调系统 PATH 上的 mutagen，与 embed 二进制不一致

**File:** `internal/cloudclaude/doctor/fix.go:107-111`
**Issue:**

```go
func realExecMutagenDaemon(ctx context.Context, action string) error {
    cmd := exec.CommandContext(ctx, "mutagen", "daemon", action)
    return cmd.Run()
}
```

cloud-claude 实际跑 mutagen 用的是 `internal/cloudclaude/mutagen_bin/<os>_<arch>/mutagen`（Phase 31 embed 路径，
`MutagenBinaryVersion` 常量与 mount.go::checkMutagenVersionMatch 里读取的远端 `/etc/cloud-claude/mutagen.version` 比对）。
当 `--fix` 修复 `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` 时，doctor 重启的是 **PATH 上的另一个 mutagen**——
如果用户全局没装 mutagen，`exec` 直接找不到二进制；如果装了但版本与 embed 不同，stop/start 操作的是错误的 daemon
（甚至可能让本来 healthy 的版本被改坏）。

**Fix:**
把 mutagen 二进制路径解析逻辑提到导出函数（mount_mutagen.go 里应该已经有 `mutagenBinaryPath()` 之类的辅助），
让 `realExecMutagenDaemon` 走它：

```go
import "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude" // 已 import

func realExecMutagenDaemon(ctx context.Context, action string) error {
    bin := cloudclaude.MutagenBinaryPath() // 需要在 cloudclaude 包导出
    if bin == "" {
        return fmt.Errorf("内置 mutagen 二进制不可用")
    }
    cmd := exec.CommandContext(ctx, bin, "daemon", action)
    return cmd.Run()
}
```

如果 `cloudclaude` 包暂时没有导出 path 解析，可以直接复用 `cloudclaude.MutagenBinaryVersion` 同模块的辅助函数。
同样问题适用未来若加 `mutagen sync ...` 类修复。

---

### WR-06: `checkContainerDisk` 解析 df 输出过于脆弱

**File:** `internal/cloudclaude/doctor/disk.go:62-71`
**Issue:**

```go
stdout, _, err := runner.RunScript("container_disk",
    "df -BM --output=avail /workspace 2>/dev/null | tail -1")
...
s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stdout), "M"))
avail, err := strconv.ParseInt(s, 10, 64)
```

`df --output=avail` 在某些 busybox / Alpine 环境下不被支持（busybox 的 df 没有 `--output` flag），
`-BM` 在某些极简镜像也没有。失败时 `2>/dev/null` 把 stderr 吃掉，stdout 只剩 header（"Avail" 字面量）→
`tail -1` 拿到 header 行 → 解析失败 → newSkip。整个事故被静默吞了。

更好的做法是：

1. 用 `awk` 提取数值，防御 header 行；
2. 探测 df 兼容性失败时给一个明确 Code（如 `STATE_CONTAINER_NOT_RUNNING` 或新 `DISK_DF_UNSUPPORTED`），而不是 Skip。

也注意，当 stdout=`"Avail\n"` 时 `TrimSuffix("Avail", "M")` = `"Avail"`，`ParseInt` 失败 → 走 Skip 分支，
caller 看到 `"无法解析 df 输出: Avail"`——还能定位，但用户读起来不知道是兼容性问题。

**Fix:**

```go
script := `df -P /workspace 2>/dev/null | awk 'NR==2 {print $4}'`
stdout, _, err := runner.RunScript("container_disk", script)
if err != nil {
    return newSkip("disk", "container_disk", "df 失败: "+err.Error())
}
s := strings.TrimSpace(stdout)
if s == "" {
    return newSkip("disk", "container_disk", "/workspace 不存在或不可访问")
}
// df -P 返回 1K-blocks，需要 /1024 转 MB
kb, err := strconv.ParseInt(s, 10, 64)
if err != nil {
    return newSkip("disk", "container_disk", "无法解析 df 输出: "+s)
}
avail := kb / 1024
```

`-P` 是 POSIX 标记 busybox 也支持，避免 `--output` 兼容性问题。或保留 `-BM` 但 awk 提取 + 单元测试 mock 一下。

---

### WR-07: `fixFUSEResidualMount` 的 `[]string` 类型断言对 JSON round-trip 不生效

**File:** `internal/cloudclaude/doctor/fix.go:117-125`
**Issue:**

```go
var points []string
if v, ok := original.Details["mountpoints"].([]string); ok {
    points = v
}
if len(points) == 0 {
    return nil, []string{"无法从 Details 获取 mountpoints（需 Plan 02 rerun 以填充 Details）"}
}
```

`Details map[string]any` 在 in-memory 路径下 `mount.go::checkFUSEResidual` 直接塞 `[]string`，断言成功，OK。
但任何把 Check 序列化成 JSON 再反序列化的场景（`--json` 输出后由外部脚本读回，或未来增加 `--from-report` 等），
`[]string` 在 JSON 里是 `[...]`，反序列化回 `map[string]any` 会变成 `[]any`，类型断言**直接失败**，
fixer 报错 "无法从 Details 获取 mountpoints"。

PHASE 34 当前没有这种 round-trip 路径，但 fix.go 里其它处（如 `host_port` 是 string 不受影响）已经有同款风险铺垫。
建议加 fallback：

**Fix:**

```go
var points []string
switch v := original.Details["mountpoints"].(type) {
case []string:
    points = v
case []any:
    for _, item := range v {
        if s, ok := item.(string); ok {
            points = append(points, s)
        }
    }
}
if len(points) == 0 {
    return nil, []string{"无法从 Details 获取 mountpoints"}
}
```

同样对未来其它 `[]X` Details 字段做 helper：

```go
func detailsStringSlice(d map[string]any, key string) []string { ... }
```

---

### WR-08: `renderDowngradeBanner` 重新从 age 构造 timestamp，丢失原始精度且与时钟漂移共振

**File:** `internal/cloudclaude/doctor/render.go:78-80` + `doctor.go:267`
**Issue:**

`convertSnapshotToBanner` 把 `snap.Timestamp` 转成秒级 age：

```go
age := int64(time.Since(snap.Timestamp).Seconds())
```

`renderDowngradeBanner` 又拿 age 倒推：

```go
fmt.Fprintf(&b, "  时间戳: %s（%d 秒前）\n",
    time.Now().Add(-time.Duration(banner.SnapshotAgeSeconds)*time.Second).Format(time.RFC3339),
    banner.SnapshotAgeSeconds)
```

两个 `time.Now()` 之间总会差几十毫秒到几秒，倒推出的 RFC3339 时间戳与 `last-session.json` 真实写入时刻可能差几秒——
排障时人会拿这个时间戳去 grep 日志，结果对不上。

更干净的做法：让 `DowngradeBanner` 直接持原始 `time.Time`（或 RFC3339 字符串），age 在 render 阶段算。

**Fix:**

```go
// doctor.go DowngradeBanner
type DowngradeBanner struct {
    Timestamp          time.Time `json:"timestamp"`            // 直接保留
    SnapshotAgeSeconds int64     `json:"snapshot_age_seconds"` // 兼容字段
    ...
}

// convertSnapshotToBanner
return &DowngradeBanner{
    Timestamp:          snap.Timestamp,
    SnapshotAgeSeconds: int64(time.Since(snap.Timestamp).Seconds()),
    ...
}

// renderDowngradeBanner
fmt.Fprintf(&b, "  时间戳: %s（%d 秒前）\n",
    banner.Timestamp.Format(time.RFC3339),
    banner.SnapshotAgeSeconds)
```

JSON schema 添加 `timestamp` 不破坏 schema_version=1（新增字段向后兼容，jq 现存查询不影响）。

---

## Info

### IN-01: `RunDoctor` 调 `checkConfigPresent` 两次

**File:** `internal/cloudclaude/doctor/doctor.go:110, 176`
**Issue:**
第一次（line 110）只为了拿 `cfg`，结果 Check 被丢弃；
第二次（line 176）才把 Check append 到 report。两次调用即两次 `LoadConfig`（读盘 + YAML 解析）。
**Fix:**
缓存第一次的 Check：

```go
firstCfgCheck, cfg := checkConfigPresent(ctx)
...
if want("auth") {
    report.Checks = append(report.Checks, firstCfgCheck)
    if cfg != nil { ... }
}
```

注意 want("auth") false 时不要 append（避免 domain filter 被破坏）。

---

### IN-02: `checkAppArmorFusermount3` 用字符串比较 Ubuntu 版本

**File:** `internal/cloudclaude/doctor/mount.go:147-153`
**Issue:**

```go
major := string(m[1])
minor := string(m[2])
if major < "25" || (major == "25" && minor < "04") { ... }
```

字符串字典序比较在 Ubuntu 现行版本号（"20", "22", "24", "25"）下侥幸正确，但语义错误。
Ubuntu 进到三位数版本（VERSION_ID="100" 这种远期假设）会出现 "100" < "25" 的翻车；
更现实的 minor=10 vs 04 → "10" < "04" = false（巧合正确），但 "9" > "10"（少见）。
**Fix:**
解析为 int 再比：

```go
mj, _ := strconv.Atoi(string(m[1]))
mn, _ := strconv.Atoi(string(m[2]))
if mj < 25 || (mj == 25 && mn < 4) { ... }
```

---

### IN-03: `checkKnownHosts` 用 `strings.Contains(err.Error(), "no such file")` 嗅探 fs 错误

**File:** `internal/cloudclaude/doctor/ssh.go:73-76`
**Issue:**

```go
if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not exist") { ... }
```

依赖 errno 文本（i18n、libc 版本可能变）。
**Fix:**

```go
import "io/fs"
if errors.Is(err, fs.ErrNotExist) {
    return newSkip("ssh", "known_hosts", "~/.ssh/known_hosts 不存在...")
}
```

`knownhosts.New` 底层走 `os.Open`，错误能通过 `errors.Is` 命中。

---

### IN-04: `RenderText` banner 框线宽度与中文字符不对齐

**File:** `internal/cloudclaude/doctor/render.go:31-33`
**Issue:**

```go
b.WriteString("╭─────────────────────────────────────────╮\n")
b.WriteString("│  Cloud Claude Doctor v3.0 体检报告       │\n")
b.WriteString("╰─────────────────────────────────────────╯\n")
```

中文字符 "体检报告" 是全角，每字占 2 列。当前手写空格按等宽英文 padding，在终端里中间行会比上下两行短，视觉错位。
**Fix:**
按可视宽度（runewidth.StringWidth）计算并 pad，或者干脆用纯英文 banner，或者把上下框线对齐到中文实际宽度：

```go
title := "  Cloud Claude Doctor v3.0 体检报告"
// 计算可视列数：英文 1 + 中文 2，pad 到固定宽度
```

非阻塞，但既然已经用了 box-drawing，对齐应一并做完。

---

### IN-05: `realExecDNSFlush` 直接调 sudo，可能在 CI/non-interactive 环境卡死

**File:** `internal/cloudclaude/doctor/fix.go:262-274`
**Issue:**
`opts.Yes=true` 跳过 `confirmDestructive` 内部 prompt，但底层 sudo 自己会要密码。
若用户没有 NOPASSWD sudoers 或者运行环境没有 TTY（CI），sudo 将无限阻塞等密码 → cloud-claude 看着就是 hang。
60s 顶层 timeout（`ApplyFixes` 用的 `context.WithTimeout(ctx, 60*time.Second)`）能兜底，但 60s 阻塞已经很差。
**Fix:**
- 探测 `sudo -n true`（non-interactive 模式）确认 sudo 不会 prompt 再决定走不走；否则 `FixFailed: "DNS flush 需要 sudo NOPASSWD 或交互终端"`。
- 或者文档明确「非交互环境（CI）请避免 doctor --fix --yes 触发 DNS flush」。

---

### IN-06: `runExplain` / `runDoctor` 在 `os.Exit` 后还有 `return nil` 死代码

**File:** `cmd/cloud-claude/explain.go:37-38`、`cmd/cloud-claude/doctor.go:71-73, 92-94, 104-110`
**Issue:**

```go
fmt.Fprintf(os.Stderr, "未找到错误码 %s...", args[0])
os.Exit(exitConfigError)
return nil
```

`os.Exit` 立即终止进程，`return nil` 不可达。Cobra `RunE` 的契约要求返回 error 或 nil，但因为已经 SilenceErrors 且要直接控制 exit code，
这种写法常见。问题在于：

1. Lint（go vet/staticcheck）会报 unreachable。
2. 单测没法走 happy path 验证 return（已经被 os.Exit 截断）。
3. 测试只能通过 build binary + exec 子进程拿 exit code（explain_test.go 已经这样做了，OK）。

**Fix:**
要么用 `return fmt.Errorf("...")` 让 cobra 自己返回非零（main.go 已经处理 SilenceErrors=true），
然后在 `main.go` 顶层 switch error 转 exit code；要么加 `// unreachable` 注释静默 lint。
建议 doctor.go 把 `os.Exit` 抽到 main.go，runE 只返回 error，让控制流统一。

---

### IN-07: `RunDoctor::ensureRemote` 在 SSH 连不上时会被多次重复尝试

**File:** `internal/cloudclaude/doctor/doctor.go:135-151`
**Issue:**

```go
ensureRemote := func() {
    if remoteRunner != nil || cfg == nil || authResp == nil {
        return
    }
    ...
    conn, err := cloudclaude.SSHConnect(sshCfg)
    if err != nil {
        return
    }
    remoteConn = conn
    remoteRunner = NewSSHRemoteRunner(conn)
}
```

短路条件 `remoteRunner != nil` 在失败时永远不成立 → 每个维度（auth/ssh/mount/disk）调用 `ensureRemote` 都会重新拨号。
SSH 拨号默认 `cloudclaude.SSHConnect` 没有 timeout（或者很长），最坏 4 个域 × 拨号 timeout = 体检整体超时。

**Fix:**
加一个 `attempted` 标记：

```go
attempted := false
ensureRemote := func() {
    if remoteRunner != nil || attempted || cfg == nil || authResp == nil {
        return
    }
    attempted = true
    ...
}
```

---

_Reviewed: 2026-04-21_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
