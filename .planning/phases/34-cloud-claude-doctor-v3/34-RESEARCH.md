# Phase 34 Research: cloud-claude doctor v3 + 错误码统一

**Researched:** 2026-04-21
**Status:** Ready for planning
**Scope note:** CONTEXT.md（D-01..D-30）已把方案拍死；本文件只给 **实施锚点**（命令字面、跨 OS 差异、库函数签名、可复用代码位置），不再重新讨论 gray area。

---

## 1. Domain Background

### 1.1 为什么要做 doctor v3 + 错误码收口（与 v2.0 `ssh doctor` 的本质区别）

v2.0 已经发布 `cloud-claude ssh doctor`（quick-task 260417-0w4），但它只覆盖 `/workspace/.ssh` 目录下的 owner/mode/PEM 尾换行三类问题——**单文件维度**的体检。v3.0 引入三层文件映射（Mutagen + sshfs + mergerfs）、tmux 多端 attach、持久化 claude_account volume 之后，排障面从「一个目录」爆炸到 **five-dimension 运维面**：

| 维度 | v3.0 新增的失败类别（CONTEXT D-19） | v2.0 有无 |
|------|--------------------------------------|-----------|
| network | DNS / gateway HTTP / 出口 IP 漂移 | 无 |
| auth | 本地 config / Entry token / OAuth 过期 | 无 |
| ssh | KeepAlive 一致性 / sshd -T 基线漂移 / known_hosts 冲突 / **workspace_ssh_keys（复用 v2.0）** | 仅 workspace_ssh_keys |
| mount | Mutagen 版本匹配 / mergerfs xattr / sshfs 挂载健康 / FUSE 残留挂载 / AppArmor override | 无 |
| disk | 本地 / 容器 / Mutagen data bloat | 无 |

**关键：doctor v3 不是 `ssh doctor` 的扩展，而是把它降级为 `ssh.workspace_ssh_keys` 单个 check**（CONTEXT D-04 / D-19）。两个入口共存，共享 `RunSSHDoctor` 底层实现；v3.1 才考虑 deprecate 老入口。

### 1.2 为什么错误码统一必须与 doctor 同 phase ship

F8 错误码体系是**横切关注点**：REQ-F8-A/B 的「所有错误路径纳入 `<DOMAIN>_<KIND>_<NUM>`」由 Phase 31/32/33 分散落码（共 25 条 Code），但 REQ-F8-C 的 `cloud-claude explain <code>` **需要一个遍历全量 Registry 的入口**。如果先 ship doctor、再 ship explain，新加的 `STATE_*`/`SYSTEM_*`/`SSH_*`/`AUTH_*`/`DISK_*` 五个前缀（共 ≥ 17 条新码）将无处安置——doctor 五维度的每个 fail/warn 都会报出一个 **未注册** 的 Code 字面量，CI grep 断言（M14 终验）会全线红。因此两块必须同 phase（CONTEXT D-29：Plan 01 先收口错误码，Plan 02 再落 doctor）。

### 1.3 五维度切分的第一原理

D-19 的 5 维度 17 项 check 有一条隐形的依赖链：

```
network  (纯本地，可独立跑；无 init 也能跑) 
   ↓
auth     (本地 config → Entry API → 远端 OAuth 文件；需要 init)
   ↓ （建立 *ssh.Client）
ssh      (本地 config → 远端 sshd -T；需要 SSH conn)
   ↓
mount    (远端容器侧 mergerfs xattr / Mutagen 版本；需要 SSH conn)
   ↓
disk     (需要 mount 拿到 mountpoint 才知道检查哪个路径)
```

CONTEXT D-07 拍板**串行执行**，不做 goroutine 并发（低频运维场景 + 一致性 + 调试容易）。doctor 未 init 时 `mount/ssh/disk` 三维度全部 `StatusSkip`（D-06），不算 fail——这是「doctor 在容器宕机时仍能给出部分诊断」的关键。

---

## 2. Existing Code to Extend

### 2.1 错误码注册示范（Plan 01 照抄的模板）

**`internal/cloudclaude/errcodes/mount.go`** 是五域文件的样板：

```1:20:internal/cloudclaude/errcodes/mount.go
package errcodes

// MOUNT_* 错误码注册。文案与 Phase 31 PLAN.md <errcode_registry> 表逐字符对齐。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_VERSION_SKEW,
		Severity:   SeverityError,
		Message:    "Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only",
		NextAction: "升级容器镜像到 v3.0.0+ 或重装 cloud-claude",
	})
```

**复制要点**：
1. `Code` 常量必须在 `codes.go` 的 `const ( ... )` 块里先声明字面量（保持 grep 可定位）；**Plan 01 必须扩展 `codes.go` 的 const 块**，再在各域文件 `init()` 调 `MustRegister`。
2. 每个域文件顶部加一行 `//nolint:lll` 抑制长行 lint（Message 是中文长句，分行会破坏字面量对齐 + CI grep）。
3. `Severity` 只选 `SeverityInfo/Warn/Error/Fatal` 四档；`Info` 专留给 informational 类（M13 降级提示、APFS 识别、reconnect backoff），会被 `ExplainExempt` 豁免。

### 2.2 `doctor.go / check.go` 顶层入口（新建）

CONTEXT D-01 已定义 struct。**closest analog 是 `ssh_doctor.go:RunSSHDoctor`**（`internal/cloudclaude/ssh_doctor.go:74`）——它的 pattern 是：

```
RunDoctor(ctx, opts) -> *Report    # 对标 RunSSHDoctor(cfg, opts) -> *SSHDoctorResult
  ├─ connect SSH (lazy)            # 对标 sshConnect(cfg)
  ├─ 每个 check: runWithTimeout    # 对标 runSSHSession(conn, script)
  └─ defer conn.Close()
```

**与 `ssh_doctor.go` 的差异（planner 必须记住）**：
- `ssh_doctor.go` 所有 SSH 动作在一个 session 里跑一个 `scanScript`；doctor v3 **每个 check 独立 SSH session**（`RemoteRunner.RunScript(name, script)`，CONTEXT D-20），便于单 check timeout + mock。
- `ssh_doctor.go` 的 `applyFixes` 是内置的；doctor v3 走 `FixerRegistry map[Code]Fixer` 注册表（CONTEXT D-09 / Plan 03）。

### 2.3 `render.go` 渲染（新建，复刻 `ssh_doctor.go:Print` 风格）

`ssh_doctor.go:370` 的 `Print()` 是直接可抄的样板：

```370:409:internal/cloudclaude/ssh_doctor.go
func (r *SSHDoctorResult) Print() {
	fmt.Println("╭─────────────────────────────────────────╮")
	fmt.Println("│    Cloud Claude SSH 密钥体检报告        │")
	fmt.Println("╰─────────────────────────────────────────╯")
	fmt.Println()
```

**复用要点**：
- Banner 格式（`╭─╮│╰─╯`）+ `section()` / `row()` / `printGroup()` helper 直接抄；添加一个 `downgradeBanner(snap *LastSessionSnapshot)` 作为 **D-13 的第一屏**。
- 状态符号（`[✓][!][✗][~]`）从 CONTEXT D-14 拿到，颜色用 `colors.go` 的 `ansiGreen/Yellow/Red/Gray + colorize + colorEnabled`（`internal/cloudclaude/colors.go:14`），不自造。
- `NO_COLOR` 环境变量由 `colorEnabled` 自动处理（`colors.go:33`），render.go **零额外处理**——只需把 stdout fd 传进去。

### 2.4 `explain.go` cobra 子命令（新建）

**closest analog 是 `cmd/cloud-claude/sessions.go:22`** `newSessionsCmd()`：

```22:47:cmd/cloud-claude/sessions.go
func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sessions",
		Short:         "tmux 会话管理（v3.0 SSH 会话可靠性）",
		...
		SilenceUsage:  true,
		SilenceErrors: true,
	}
```

**复制要点**：`newExplainCmd()` / `newDoctorCmd()` 直接 fork `newSessionsCmd` 的三段式（outer cmd + subcmd + RunE）。注册在 `cmd/cloud-claude/main.go:93` 的 `rootCmd.AddCommand(...)` 调用，并在 `main.go:99` 的 `switch os.Args[1]` case 追加 `"doctor", "explain"`（CONTEXT D-03 / D-29 Plan 01 & 02 已分别挂载）。

### 2.5 `fix.go` 包级 var mock 模式（新建，Plan 03）

`internal/runtime/tasks/worker.go:754` 的 `var execInContainer = ...` 与 `:969` 的 `var ensureDockerVolume = realEnsureDockerVolume` 是**官方模板**（Phase 29.1 / Phase 33 两次验证）：

```
var execMutagenDaemon      = realExecMutagenDaemon
var execFusermountUnmount  = realExecFusermountUnmount
var execSSHKeygenRemove    = realExecSSHKeygenRemove
var execDNSFlush           = realExecDNSFlush
var execEntryRefresh       = realExecEntryRefresh   // wraps EntryClient.AuthenticateAndWait
```

单测通过包级赋值 `execMutagenDaemon = func(...) error { return nil }` 注入 mock。这比 interface 拆分侵入小（CONTEXT Established Patterns + D-24 第 2 条），与 Phase 33 `worker_test.go` 保持一致。

---

## 3. External System Calls (per dimension)

### 3.1 network

| Check | 命令 / 函数 | 预期输出 / 行为 | 跨 OS 注意 |
|-------|-------------|-----------------|-------------|
| `dns_resolve` | `net.LookupHost(host)` + 可选 `net.Resolver.LookupIPAddr(ctx, host)` | `[]string{"a.b.c.d"}` 非空 = pass；error 或空数组 = fail | 走 Go 标准库，系统 resolver 行为差异已被 runtime 抽象；macOS 10.15+ 默认走 mDNSResponder，Linux 走 nsswitch（/etc/resolv.conf） |
| `gateway_reachable` | `http.NewRequestWithContext(ctx,"GET",cfg.Gateway+"/healthz",nil)` + 2s timeout | HTTP 200/204 = pass；connection refused / timeout = fail；5xx = warn | 和 `EntryClient.AuthenticateAndWait` 保持同样的 TLS 行为；doctor 不复用 auth 路径（避免误报 token 过期成网络问题） |
| `egress_ip_visible` | 远端 `curl -sS --max-time 5 https://ifconfig.io` | stdout 为单行 IPv4/IPv6；与 `authResp.ExpectedEgressIP`（如 Entry API 返回）一致 = pass | 远端 `curl` 已在 Phase 29 镜像就位（`deploy/docker/managed-user/Dockerfile`）；ifconfig.io 下线备用 `https://api.ipify.org` 或 `https://checkip.amazonaws.com`（planner 按需开二级回退） |

**关键实现信号**：`gateway_reachable` 必须**不依赖** `EntryClient`，只做裸 HTTP GET——这样 Entry API 挂了 doctor 仍能区分「gateway 不通」与「token 过期」。

### 3.2 auth

| Check | 数据来源 / 函数 | 返回 |
|-------|-----------------|------|
| `config_present` | `cloudclaude.LoadConfig()`（`internal/cloudclaude/config.go`） | `nil, err` → fail + `AUTH_CONFIG_MISSING`；解析失败含「不存在」字串 = fatal（exitConfigError=4） |
| `entry_token_valid` | `EntryClient.AuthenticateAndWait(ctx, shortID, password, cb)` | HTTP 200 + `AuthResponse` 完整 = pass；`authResp.ImageVersion` 同时写入 banner（D-13 第一段） |
| `oauth_credentials` | `cloudclaude.CheckOAuthCredentials(connA, accountID)`（`oauth_check.go:44`） | `OAuthValid` = pass；`OAuthExpiringSoon` = warn + `NET_OAUTH_EXPIRING_SOON`；`OAuthExpired` = fail + `NET_OAUTH_EXPIRED`；`OAuthNotFound` = fail + `NET_OAUTH_NOT_FOUND` |

**签名确认**（不要重新实现）：

```go
// oauth_check.go:44
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)

// OAuthStatus:
type OAuthStatus struct {
    State           OAuthState
    ExpiresAt       time.Time
    MinutesToExpire int
}
```

OAuth 检查内部已做了 `timeout 2 cat`（`oauth_check.go:59`）、JSON 解析容错（`parseExpiresAt`），doctor `auth.oauth_credentials` 只需把 `connA` 传进去然后 switch state 映射错误码。

### 3.3 ssh

| Check | 命令 | 跨 OS 差异 |
|-------|------|-------------|
| `keepalive_config` | 读 `MountConfig.KeepAliveInterval`（`mount_strategy.go:76`）；断言 `>= 15s` | 纯本地 struct 字段校验，无 OS 差异 |
| `sshd_keepalive_drift` | 远端 `sshd -T \| grep -E '^(clientalive(interval\|countmax))\b'` | 容器内 OpenSSH 8.9+（Ubuntu 24.04+）；`sshd -T` 需要 root 或 sudo。**实际容器内 sshd config 从 `/etc/ssh/sshd_config`，`sshd -T` 无 sudo 也能跑（读 effective config）**。基线 15/8 来自 `deploy/docker/managed-user/sshd_config` + Phase 29 D-14 |
| `known_hosts` | 本地读 `~/.ssh/known_hosts`（+ `$KnownHostsFile` 如 ssh_config 有覆盖），用 `ssh.ParseKnownHosts` 解析 | **文件选择优先级**：`~/.ssh/known_hosts` 默认；用户 ssh_config `KnownHostsFile` 覆盖（v3.0 doctor **本阶段不解析 ssh_config**，D-25 + Claude's Discretion 第 7 条：只读默认路径，v3.1 再做） |
| `workspace_ssh_keys` | 复用 `RunSSHDoctor(sshCfg, SSHDoctorOptions{Fix:false})`（`ssh_doctor.go:74`） | 无新差异；结果包装成 Check.Details（D-19「无错误码，本阶段不强迁移」） |

**`known_hosts` 冲突判定**：Entry API `authResp.SSHHost:authResp.SSHPort` 与 known_hosts 里**该主机的 fingerprint** 不一致。核心代码草稿：

```go
// internal/cloudclaude/doctor/ssh.go (planner 参考)
hostPort := net.JoinHostPort(authResp.SSHHost, strconv.Itoa(authResp.SSHPort))
// ssh.ParseKnownHosts 返回 marker, hosts, pubkey, comment, rest, err
// 可用 ssh/knownhosts.New(files...) → HostKeyCallback，对一行做一次模拟拨号即能识别冲突
import "golang.org/x/crypto/ssh/knownhosts"
```

**gotcha**：`~/.ssh/known_hosts` 可能含 HASHED hostname（`|1|base64|base64|`），`ssh.ParseKnownHosts` **可以**解析散列行但不能反解匹配——必须用 `ssh/knownhosts` 包的 `HostKeyCallback` 做一次 fake handshake 才能精准判冲突。planner 在 Plan 02 Task 2.3 拍定：是 (a) 简化：字符串匹配 hostPort 不冲突判 skip；还是 (b) 全量 `knownhosts.New` 包。推荐 (b)，已在 go.sum（`golang.org/x/crypto` 间接引入）。

### 3.4 mount

| Check | 命令 | 预期输出 | 跨 OS 注意 |
|-------|------|----------|-------------|
| `mutagen_version_match` | 本地 `MutagenBinaryVersion`（常量，`mutagen_bin.go:25` = `"v0.18.1"`）vs 远端 `cat /etc/cloud-claude/mutagen.version` | 两字符串 `TrimPrefix(v) + Contains` 比对，与现网 `mountMutagen` §3 版本握手逻辑（`mount_mutagen.go:233-235`）**严格一致** | 远端文件由 Phase 29 `Dockerfile` 写入；缺失时按 `MOUNT_MUTAGEN_VERSION_SKEW` 报 |
| `mergerfs_branches` | 远端 `getfattr --only-values -n user.mergerfs.branches /workspace/.mergerfs 2>/dev/null` + `mount \| grep mergerfs` | xattr 输出必须含字面量 `RW` 和 `NC,RO`（CONTEXT 建议 **substring 匹配**，D-Discretion §3）；mount line 必须含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff` 全 6 参数 | mergerfs 2.41 静态编译（Phase 29）；`getfattr` 跨发行版字段格式差异：CONTEXT 建议用 `--only-values` 只取值（避开 `# file: xxx` header 行） |
| `sshfs_mountpoint` | 远端 `mountpoint -q /workspace-cold; echo $?` | `0` = 已挂（pass）；`32` = 未挂（warn）；其它 = fail | **macOS sshfs（macFUSE）没有 `mountpoint` 命令**——但 sshfs_mountpoint 是**远端**（容器内 Linux）check，`mountpoint` 来自 util-linux 必然存在。本地 `fuse_residual`（见下行）才需要 macOS 分支 |
| `fuse_residual` | 本地：macOS `mount \| grep -E 'macfuse\|osxfuse'`；Linux `mount \| grep -E 'fuse\.sshfs\|fuse\.mergerfs'` | 若匹配旧 mount point（不属于当前 cloud-claude PID），列入 `SYSTEM_FUSE_RESIDUAL_MOUNT` | 运行时判 OS 走 `runtime.GOOS`；避免误杀当前会话：比对 mountpoint 路径是否属于 `~/.cloud-claude/mount/current-session/` |
| `apparmor_fusermount3` | 本地 Go 改写 `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3`（§6 之后） | 按 Ubuntu 25.04+ / aa-status / override 文件三级 gate | 非 Linux / 非 Ubuntu / `aa-status` 不存在 → `StatusSkip`（informational） |

**`fuse_residual` 本地扫描的具体命令**（planner 可直接抄）：

```bash
# Linux
mount | awk '/fuse\.(sshfs|mergerfs)/ {print $3}'   # 输出 mountpoint 列表
# macOS  
mount | awk '/(macfuse|osxfuse)/ {print $3}'
```

### 3.5 disk

| Check | 命令 / 函数 | 阈值（硬编码，CONTEXT Discretion §5） |
|-------|-------------|----------------------------------------|
| `local_disk` | Go `syscall.Statfs(~/.cloud-claude/, &stat)`；available = `stat.Bavail * stat.Bsize` | `< 500MB` = warn；`< 100MB` = fail（fatal 级别阻塞 mount） |
| `container_disk` | 远端 `df -BM --output=avail /workspace \| tail -1` | 同上阈值 |
| `mutagen_data_size` | 本地 `du -sh ~/.cloud-claude/mutagen/ 2>/dev/null` → 解析 `K/M/G` 单位 | `> 1GB` = warn（不 fail） |

**跨 OS `Statfs` 签名差异**：

```go
// Linux (syscall.Statfs_t)    (Bavail uint64, Bsize int64)
// Darwin (syscall.Statfs_t)   (Bavail uint64, Bsize uint32)
// planner: 用 golang.org/x/sys/unix.Statfs 统一签名，避免 build tag 分叉
import "golang.org/x/sys/unix"
var stat unix.Statfs_t
if err := unix.Statfs(path, &stat); err != nil { ... }
avail := int64(stat.Bavail) * int64(stat.Bsize)
```

`golang.org/x/sys` 已在间接依赖图里（`go.mod` 通过 `crypto/ssh` 链路引入）。

---

## 4. `--fix` 5 Classes — Implementation Notes

CONTEXT D-09 表锁死 5 类。下面给出**具体命令序列 + 幂等性证据 + 预期 stderr/stdout**。

### 4.1 `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE` → restart daemon

```
MUTAGEN_DATA_DIRECTORY=~/.cloud-claude/mutagen ./mutagen daemon stop   # 不存在时 exit 1 + stderr "no daemon is running"
MUTAGEN_DATA_DIRECTORY=~/.cloud-claude/mutagen ./mutagen daemon start  # 已启动时 stdout "daemon is already running" 或 exit 0
```

- 幂等性证据：`mountMutagen` 的 §2 daemon start 已实现 `strings.Contains(out, "daemon already started")` 容错（`mount_mutagen.go:227`），**直接复用** `deps.runLocal` helper。
- 无需用户确认（数据无关，低危）。

### 4.2 `SYSTEM_FUSE_RESIDUAL_MOUNT` → 批量 unmount

```bash
# Linux
fusermount -u <mountpoint>   # 成功 exit 0；挂载中且有活跃 fd 时 exit 1 + stderr "Device or resource busy"
# macOS (macFUSE)
umount <mountpoint>          # macOS 没有 fusermount；需要 umount 或 diskutil unmount
```

**幂等性**：已解挂的 mountpoint 第二次跑 `fusermount -u` 会 exit 1 + stderr "not mounted"——`fix.go` 必须把这个 stderr 视为 OK（`strings.Contains(stderr, "not mounted")`）。

**危险性**：有活跃 fd 时 unmount 会失败——**整批 y/N 确认**（D-09 表要求）。Prompt 文案：

```
发现 2 个疑似残留 FUSE 挂载：
  /Users/alice/cloud-claude-session-abc
  /Users/alice/cloud-claude-session-def
将逐个执行 fusermount -u（已解挂的将跳过），是否继续？(y/N) 
```

### 4.3 `SSH_KNOWN_HOSTS_CONFLICT` → ssh-keygen -R

```bash
ssh-keygen -R "<host>:<port>"   # port ≠ 22 时必须带 :port
```

- 幂等性：不存在 entry 时 exit 0 + stdout `not found in /Users/alice/.ssh/known_hosts`。
- **文件路径**：CONTEXT Discretion §8 决定只读 `~/.ssh/known_hosts`（默认），不解析 ssh_config `KnownHostsFile`。ssh-keygen 接受 `-f <file>` 指定非默认路径，但 v3.0 不做。
- 无需用户确认（低危，只清 known_hosts 条目）。

### 4.4 `AUTH_TOKEN_EXPIRED` / `AUTH_OAUTH_REFRESH_FAILED` → refresh

```go
// Entry token 刷新：直接重跑 AuthenticateAndWait（内部已实现幂等刷新）
authResp, err := EntryClient.AuthenticateAndWait(ctx, shortID, password, func(string){})
// OAuth 过期：CLI 不自动登录（需要用户在容器内交互），只输出 next_action
// next_action: "在容器内运行 cloud-claude exec claude login 重新登录"
```

与 `NET_OAUTH_NOT_FOUND` 的 `NextAction`（`errcodes/net.go:24`）完全一致。

### 4.5 `SYSTEM_DNS_RESOLVE_FAILED` → flush cache

跨 OS 命令矩阵：

| OS | 命令 | 需要 sudo | macOS 版本 |
|----|------|-----------|-------------|
| macOS Sonoma+ (14+) | `sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder` | **是** | 14.0+ |
| Linux + systemd-resolved (modern) | `sudo resolvectl flush-caches` | **是** | systemd 239+ |
| Linux + systemd-resolved (legacy) | `sudo systemd-resolve --flush-caches` | **是** | systemd < 239（deprecated 但仍在 Ubuntu 20.04 有用） |
| Linux + nscd | `sudo systemctl restart nscd` | **是** | 罕见 |

**选择策略**（planner 按这个优先级）：
1. `runtime.GOOS == "darwin"` → macOS 命令
2. Linux：先探测 `command -v resolvectl` → modern；再探测 `command -v systemd-resolve` → legacy；都没有 → warn + skip fix
3. **v3.0 不支持 nscd / dnsmasq** 的自动 flush——输出 next_action 让用户手动

**危险性**：需要 sudo + `killall -HUP mDNSResponder` 会触发系统级 daemon 信号——**y/N 确认**（D-09 表要求）。

---

## 5. JSON Schema & Exit Codes

### 5.1 Go struct tags（schema_version=1 lock）

```go
// internal/cloudclaude/doctor/doctor.go
type Report struct {
    SchemaVersion     int                `json:"schema_version"`                    // 硬编码 1，不加 omitempty（探针依赖）
    StartedAt         time.Time          `json:"started_at"`
    DurationMS        int64              `json:"duration_ms"`
    CloudClaudeVer    string             `json:"cloud_claude_version,omitempty"`
    RemoteImageVer    string             `json:"remote_image_version,omitempty"`
    DowngradeHistory  *DowngradeBanner   `json:"downgrade_history,omitempty"`       // 指针 + omitempty：last-session.json 缺失时整块消失
    Summary           Summary            `json:"summary"`
    Checks            []Check            `json:"checks"`
}

type Check struct {
    Domain     string            `json:"domain"`
    Name       string            `json:"name"`
    Status     string            `json:"status"`                       // "pass"|"warn"|"fail"|"skip"（小写，对标 D-15）
    Code       string            `json:"code,omitempty"`               // pass/skip 时为空
    Message    string            `json:"message,omitempty"`
    NextAction string            `json:"next_action,omitempty"`        // CI grep gate 的关键字段
    Details    map[string]any    `json:"details,omitempty"`
    FixApplied []string          `json:"fix_applied,omitempty"`
    FixFailed  []string          `json:"fix_failed,omitempty"`
    DurationMS int64             `json:"duration_ms"`
}
```

**schema_version=1 锁死机制**：
1. `SchemaVersion int` **不带 omitempty**——JSON 始终输出 `"schema_version": 1`，jq 脚本可稳定 assert。
2. 未来新字段一律加 `omitempty` 尾随追加（CONTEXT D-15 / Established Patterns「omitempty 新字段 + 不动 schema_version」）。
3. **测试锁**：`render_test.go` 必须有一个 `TestJSONSchemaV1Lock`：

```go
// 伪代码
func TestJSONSchemaV1Lock(t *testing.T) {
    r := &Report{SchemaVersion: 1, Summary: Summary{}, Checks: []Check{}}
    raw, _ := json.Marshal(r)
    var m map[string]any
    json.Unmarshal(raw, &m)
    // 核心字段存在
    if _, ok := m["schema_version"]; !ok { t.Fatal("schema_version 缺失") }
    if _, ok := m["summary"]; !ok { t.Fatal("summary 缺失") }
    if _, ok := m["checks"]; !ok { t.Fatal("checks 缺失") }
    // schema_version 是 1（float64 是 json.Unmarshal 默认数字类型）
    if m["schema_version"].(float64) != 1 { t.Fatal("schema_version ≠ 1") }
}
```

如果有人后续改成 schema_version=2 或移除字段，这个测试立刻红。

### 5.2 退出码（brew doctor 对齐，CONTEXT D-16）

| 场景 | 退出码 | 定义 |
|------|--------|------|
| 全 pass 或 pass+skip | 0 | `ExitOK` |
| ≥1 warn 且 0 fail | 1 | **新常量：`ExitDoctorWarn = 1`**（与 v2.0 `ExitAuthFailed=1` 撞值；doctor 子命令独立 os.Exit，不走 root 路径） |
| ≥1 fail | 2 | **新常量：`ExitDoctorFail = 2`**（与 `ExitNetworkError=2` 撞值；同上） |
| `explain` 未找到 code | 4 | 复用 `ExitConfigError`（CONTEXT D-17） |

**撞值是可接受的**——`doctor` / `explain` 子命令不走 `runRoot` 路径，由各自 `RunE` 调 `os.Exit(code)` 直接退出；`exitcodes.go` 不需要新增常量（节省字面量噪音）。planner 可在 Plan 02 的 `cmd/cloud-claude/doctor.go` 内定义文件级 const：

```go
const (
    doctorExitOK   = 0
    doctorExitWarn = 1
    doctorExitFail = 2
)
```

### 5.3 `--fix` 不降级退出码

CONTEXT D-16 第 2 句：**「修复成功的 fail 不降级为 0」**。实现：
- `Check.Status` 始终记录**原始检测**结果（跑 fix 前的状态）
- `Check.FixApplied/FixFailed` 记录修复动作，不回写 Status
- stdout 顶部额外输出 `[fix] 2 项已修复 / 1 项修复失败`
- 退出码按 Status 计（fail 优先于 warn）

---

## 6. CI Gates (M14 + REQ-F8-A/B verification)

### 6.1 `scripts/ci-doctor-grep.sh`

CONTEXT D-24 第 5 条 + ROADMAP §Phase 34 SC#3 要求「所有 `[!]` 与 `[✗]` 行必须含"建议:"子串」。完整 bash 伪代码：

```bash
#!/usr/bin/env bash
# scripts/ci-doctor-grep.sh — M14 终验脚本（Phase 34 SC#3）
# 在 CI 中 cloud-claude 已 build 好但 ~/.cloud-claude/config.yaml 未必存在：
# - doctor 未 init 时 network/auth 走本地仍会跑，mount/ssh/disk → StatusSkip
# - 只要能产出一份 report 就能 assert
set -euo pipefail

BIN="${1:-./cloud-claude}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# ---------- (1) JSON 模式：所有 warn/fail check 必有 next_action ----------
# doctor 子命令单次跑会退出码 0/1/2，| 会吞掉非零退出；用 || true 托底
"$BIN" doctor --json > "$WORK/report.json" || true

# 检查 JSON 合法
jq empty "$WORK/report.json" >/dev/null 2>&1 \
  || { echo "FAIL: doctor --json 输出非合法 JSON" >&2; exit 1; }

# 检查 schema_version
jq -e '.schema_version == 1' "$WORK/report.json" >/dev/null \
  || { echo "FAIL: schema_version ≠ 1" >&2; exit 1; }

# 所有 status ∈ {warn,fail} 的 check 必须有非空 next_action
MISSING=$(jq -r '.checks[] | select(.status=="warn" or .status=="fail")
                 | select((.next_action // "") == "")
                 | "\(.domain).\(.name)"' "$WORK/report.json")
if [ -n "$MISSING" ]; then
  echo "FAIL: 以下 warn/fail check 缺 next_action:" >&2
  echo "$MISSING" >&2
  exit 1
fi

# ---------- (2) 文本模式：所有 [!]/[✗] 行必须含 "建议:" ----------
# NO_COLOR=1 保证无 ANSI 污染 grep
NO_COLOR=1 "$BIN" doctor > "$WORK/report.txt" || true

# 找所有以 "[!]" 或 "[✗]" 开头（可能前置空白）的行
BAD=$(grep -nE '^\s*\[[!✗]\]' "$WORK/report.txt" | grep -v "建议:" || true)
if [ -n "$BAD" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺 '建议:' 子串:" >&2
  echo "$BAD" >&2
  exit 1
fi

# ---------- (3) 每条 warn/fail 必须含错误码：[XXX_YYY_ZZZ] ----------
BAD_CODE=$(grep -nE '^\s*\[[!✗]\]' "$WORK/report.txt" \
            | grep -vE '错误码:\s*[A-Z]+_[A-Z]+_[A-Z0-9]+' || true)
if [ -n "$BAD_CODE" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺错误码:" >&2
  echo "$BAD_CODE" >&2
  exit 1
fi

echo "OK: cloud-claude doctor M14 gate passed."
```

**CI 挂载位置**：planner 在 Plan 03 收尾 task 把这个脚本加入 `.github/workflows/ci.yml`（如有）或 Makefile target；run_in_background=false 确保 exit code 传播。

### 6.2 errcodes Registry 测试扩展

CONTEXT D-02 / D-21 / D-22 的 3 个核心断言加到**现有** `internal/cloudclaude/errcodes/codes_test.go`：

```go
func TestAllCodesHaveExplanations(t *testing.T) {
    for code := range Registry() {
        if _, isExempt := ExplainExempt[code]; isExempt {
            continue
        }
        exp, ok := ExtendedExplanations[code]
        if !ok {
            t.Errorf("code %s 缺 ExtendedExplanations 且未登记到 ExplainExempt", code)
            continue
        }
        if utf8.RuneCountInString(exp) < 200 {
            t.Errorf("code %s ExtendedExplanations 长度 %d < 200 中文字符",
                code, utf8.RuneCountInString(exp))
        }
    }
}

func TestNoLegacyLowercaseCodes(t *testing.T) {
    // v2.0 现网 code 字面量（D-22 防御）——禁止出现在 Registry
    legacy := []string{
        "auth_failed", "auth_expired", "entry_token_expired",
        "host_action_failed", "entry_config_missing",
    }
    for code := range Registry() {
        for _, bad := range legacy {
            if string(code) == bad {
                t.Errorf("v2.0 lower-case 字面量 %q 不应出现在 Registry（D-22）", bad)
            }
        }
    }
}

func TestAllDomainsClosed(t *testing.T) {
    // CONTEXT D-23：DOMAIN ∈ {MOUNT,SESSION,NET,STATE,SYSTEM,SSH,AUTH,DISK}
    allowed := map[string]bool{
        "MOUNT": true, "SESSION": true, "NET": true, "STATE": true,
        "SYSTEM": true, "SSH": true, "AUTH": true, "DISK": true,
    }
    for code := range Registry() {
        domain := string(code)[:strings.Index(string(code), "_")]
        if !allowed[domain] {
            t.Errorf("code %s DOMAIN %q 未在 8 域闭合列表", code, domain)
        }
    }
}
```

---

## 7. Cross-Phase Integration Points

### 7.1 Phase 29 → Phase 34（镜像侧基线）

| Phase 29 落地物 | 位置 | Phase 34 doctor 对应 check |
|-----------------|------|----------------------------|
| `/etc/cloud-claude/mutagen.version` 文件 | `deploy/docker/managed-user/Dockerfile`（Phase 29 D-05） | `mount.mutagen_version_match` 读此文件与 `MutagenBinaryVersion` 比对 |
| mergerfs mount 参数 + branches xattr | `deploy/docker/managed-user/entrypoint.sh`（Phase 29 D-11） | `mount.mergerfs_branches` 断言 6 参数 + RW/NC,RO |
| sshd `ClientAliveInterval 15` / `ClientAliveCountMax 8` | `deploy/docker/managed-user/sshd_config`（Phase 29 D-14） | `ssh.sshd_keepalive_drift` 远端 `sshd -T` 对比 |
| AppArmor override 路径 `/etc/apparmor.d/local/fusermount3` | `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3`（Phase 29 D-23） | `mount.apparmor_fusermount3` Go 改写（不调 shell 脚本） |

### 7.2 Phase 31 → Phase 34

| Phase 31 接口 | 使用点 |
|---------------|---------|
| `errcodes.Registry() / Lookup / Format / MustRegister / codeRe` | Plan 01 五域文件 + Plan 02 所有 check 报码 + Plan 02 explain 子命令 |
| `LastSessionSnapshot` schema（`last_session.go:15`，含 `DowngradeChain/ConflictCount/IntendedMode/ActualMode`） | Plan 02 `doctor/render.go:renderDowngradeBanner()` 读取第一屏 |
| `CheckOAuthCredentials(connA, accountID) -> *OAuthStatus`（`oauth_check.go:44`） | Plan 02 `doctor/auth.go:checkOAuthCredentials()` 直接复用 |
| `MutagenBinaryVersion` 常量（`mutagen_bin.go:25`） | Plan 02 `doctor/mount.go:checkMutagenVersionMatch()` |
| `exitcodes.go` 9 常量 | Plan 02 `doctor.go/explain.go` 退出码选择 |

### 7.3 Phase 32 → Phase 34

| Phase 32 字段 | 使用点 |
|---------------|---------|
| `LastSessionSnapshot.TmuxSession/ClientRole/ReconnectCount`（`last_session.go:27-29`） | Plan 02 downgrade banner 追加渲染；`ClientRole="secondary"` 时 `mount.mutagen_version_match` check → `StatusSkip`（CONTEXT §specifics 末条） |
| `SESSION_* / NET_RECONNECT_*` codes | 现有 Registry 成员，不新增；ExtendedExplanations 需为每条（除 `SESSION_TAKEOVER_NOTIFIED`/`NET_RECONNECT_BACKOFF` 两条 Info）补一条 ≥200 字符说明 |

### 7.4 Phase 33 → Phase 34

| Phase 33 字面量 | Phase 34 动作 |
|-----------------|----------------|
| `STATE_VOLUME_IN_USE_001`（`internal/controlplane/http/admin_claude_accounts.go:107`） | Plan 01 `errcodes/state.go` 追加 `const STATE_VOLUME_IN_USE_001 Code = "STATE_VOLUME_IN_USE_001"` + `MustRegister` + ExtendedExplanations 一条 |

**字面量不变**：Phase 33 admin handler 的硬编码字符串 `"STATE_VOLUME_IN_USE_001"` **保持不变**（兼容已部署 frontend，CONTEXT D-27）；Plan 01 只是补录入 Registry，前端 / 运维脚本无感知。

---

## 8. Gotchas & Risk Mitigations

### 8.1 Pitfall C2 — mergerfs 参数断言

**禁止用** strict 正则匹配整行 mount 输出（跨 kernel 版本 `mount` 输出顺序会变）。**必须用** 6 个独立 `strings.Contains` 断言：

```go
want := []string{
    "func.readdir=cor:4", "cache.attr=30", "cache.entry=30",
    "cache.readdir=true", "cache.files=off", "category.create=ff",
}
for _, w := range want {
    if !strings.Contains(mountLine, w) {
        return fail(MOUNT_MERGERFS_FAILED, "缺少参数 "+w, ...)
    }
}
```

xattr 解析用 `--only-values`（§3.4）——其它发行版 / getfattr 版本可能在第一行加 `# file:` 注释，`--only-values` 绕开这层。

### 8.2 Pitfall C4 — Mutagen 版本漂移

已实现于 `mount_mutagen.go:233-235`（生产路径），doctor 只做**只读断言**：

```go
remoteVer, _ := deps.remoteVersion(connA)   // cat /etc/cloud-claude/mutagen.version
localVer := MutagenBinaryVersion            // "v0.18.1"
if !strings.Contains(remoteVer, strings.TrimPrefix(localVer, "v")) {
    // fail with MOUNT_MUTAGEN_VERSION_SKEW + 提示升级容器镜像
}
```

**gotcha**：`MutagenBinaryVersion` 是带 `v` 前缀的 `"v0.18.1"`，远端文件通常是无前缀的 `"0.18.1"`——`TrimPrefix + Contains` 双保险（已在生产代码）。

### 8.3 Pitfall C6 — AppArmor fusermount3

Go 改写 shell 脚本的**精确逻辑**（对应 `host-preflight.sh:16-74`）：

```go
// internal/cloudclaude/doctor/mount.go (planner 参考)
func checkAppArmorFusermount3() Check {
    // Gate 1: only Linux
    if runtime.GOOS != "linux" { return skip("非 Linux，跳过") }
    
    // Gate 2: /etc/os-release 存在 + ID=ubuntu
    osRel, err := os.ReadFile("/etc/os-release")
    if err != nil { return skip("无 /etc/os-release，跳过") }
    if !regexp.MustCompile(`(?m)^ID=ubuntu\b`).Match(osRel) { return skip("非 Ubuntu，跳过") }
    
    // Gate 3: Ubuntu >= 25.04
    //   VERSION_ID="25.04" 解析为 (25, 4)；< (25, 4) 跳过
    
    // Gate 4: aa-status 存在（exec.LookPath）+ 输出含 "fusermount3"
    //   不存在 → skip "apparmor-utils 未安装（此版本 Ubuntu 未启用 fusermount3 profile）"
    
    // Gate 5: 读 /etc/apparmor.d/local/fusermount3
    //   文件不存在 → fail SYSTEM_APPARMOR_FUSERMOUNT3_MISSING
    //   存在但无 `^\s*capability\s+dac_override` → fail
    //   hit → pass
    override := "/etc/apparmor.d/local/fusermount3"
    content, err := os.ReadFile(override)
    if err != nil { return fail(SYSTEM_APPARMOR_FUSERMOUNT3_MISSING, ...) }
    if !regexp.MustCompile(`(?m)^\s*capability\s+dac_override\b`).Match(content) {
        return fail(SYSTEM_APPARMOR_FUSERMOUNT3_MISSING, ...)
    }
    return pass()
}
```

**不要 exec `apparmor_parser`**——只读文件即可判断，单测容易（writable tempdir + mock path override）。

### 8.4 Pitfall C8 — 错误码命名空间冲突

已由 `TestNoLegacyLowercaseCodes`（§6.2）防御；但还有一个陷阱：**explain 子命令的 args[0] 大小写敏感**（CONTEXT D-17）。如果用户输错 `cloud-claude explain mount_mutagen_version_skew`，必须返回 exit 4，**不做大小写自动修正**——否则 v2.0 lower-case 字面量和 v3.0 Upper-case 字面量在 `explain` 语义上会混淆。

### 8.5 Pitfall M13 — 降级历史可见性

**D-13 第一屏要求**：`last-session.json` 解析失败 / 文件不存在 → 输出 `[!] 未找到上次会话快照（首次运行 cloud-claude 后再 doctor 即可看到）` + 错误码 `STATE_LAST_SESSION_MISSING`，但**不算 fail**（Status=Info / Skip）。SC#6 验收锚点：

```
Plan 02 必须有一个 TestDowngradeBannerRendersChain：
  1. 构造 LastSessionSnapshot{IntendedMode:"full", ActualMode:"sshfs-only", 
     DowngradeChain:[{From:"full",To:"mutagen-only",ReasonCode:"MOUNT_MUTAGEN_VERSION_SKEW",ReasonMessage:"..."},
                     {From:"mutagen-only",To:"sshfs-only",ReasonCode:"MOUNT_AUTO_DOWNGRADED",ReasonMessage:"..."}]}
  2. WriteLastSession 到 tempdir
  3. 调 RunDoctor(... DomainAll)；读 stdout
  4. assert 含 "[降级] full → mutagen-only"
  5. assert 含 "[降级] mutagen-only → sshfs-only"
  6. assert 含 "MOUNT_MUTAGEN_VERSION_SKEW"
```

### 8.6 Pitfall M14 — doctor 必须给修复命令

由 `scripts/ci-doctor-grep.sh` §6.1 的 3 段 assert 双保险（JSON 字段 + 文本 grep + 错误码 grep）。**任何 NextAction 为空的 Entry 在启动时就 panic**（`codes.go:68` 已实现）；唯一剩的风险是 `Check.NextAction` 在 render 时被意外剥离——**Plan 02 必须有一个 `TestRenderTextContainsNextAction`** 覆盖所有 status=warn|fail 路径。

---

## 9. Open Questions for Planner

以下是 CONTEXT Discretion 留给 planner 的细节，**不需要再跟用户确认**，但 planner 要在 PLAN.md 显式落字：

1. **`RemoteRunner` interface vs struct**：推荐 interface `RemoteRunner interface { RunScript(name, script string) (stdout, stderr string, err error) }`，Plan 02 每个维度测试文件可注入 mock；生产实现 `sshRemoteRunner{conn *ssh.Client}`。
2. **`confirmDestructive` 非 TTY 处理**：检测 `term.IsTerminal(int(os.Stdin.Fd()))` → false 时直接拒绝 + `FixFailed` 追加中文提示（与 CONTEXT D-10 第 2 条一致）。
3. **`mount.mergerfs_branches` xattr 解析容错**：substring 匹配 `RW` 和 `NC,RO` 两个字面量即可；不做 CSV 严格拆分。
4. **`--verbose` 输出冗余度**：每 check 打印 `[开始] domain.name` 和 `[结束] domain.name (XXms)` 两行；Details 全字段 JSON-pretty；**禁止把整个 ssh script stdout 都打出来**（可能泄漏 `/etc/cloud-claude/mutagen.version` 以外的敏感路径）。
5. **`disk` 阈值硬编码 vs config**：硬编码 500MB / 100MB / 1GB；v3.1 再 config 化。
6. **`doctor --json` 缩进**：默认 `json.MarshalIndent(..., "", "  ")`；jq `-c` 由用户侧处理。
7. **`AUTH_OAUTH_REFRESH_FAILED` 修复路径**：`NextAction` 字面量与 `NET_OAUTH_NOT_FOUND` 完全一致（「在容器内运行 cloud-claude exec claude login 重新登录」）。
8. **`SSH_KNOWN_HOSTS_CONFLICT` fix 命令**：`ssh-keygen -R "<host>:<port>"` 不带 `-f`（读默认 `~/.ssh/known_hosts`）；port=22 时也带 port，保持命令字面量一致性（ssh-keygen 接受 `-R host:22`）。
9. **`explain` 模糊匹配**：本阶段精确匹配；`cloud-claude doctor --json | jq '.checks[].code'` 间接枚举（CONTEXT deferred）。
10. **`doctor` 不带 init 时的远端 check**：全部 `StatusSkip` + 中文原因「未配置网关，跳过；运行 cloud-claude init 配置后重试」（D-06）。

---

## RESEARCH COMPLETE

- **doctor v3 的本质是把 v2.0 `ssh doctor` 降级为五维度中的单个 check**，共用 `RunSSHDoctor` 底层；`cmd/cloud-claude/main.go:93/99` 的 cobra 注册 + DisableFlagParsing case 是两个新子命令的统一挂载点。
- **错误码收口必须与 explain 同 phase**，否则 `STATE_*/SYSTEM_*/SSH_*/AUTH_*/DISK_*` 五前缀无宿主；Plan 01 照抄 `errcodes/mount.go` pattern，Registry 测试扩展 3 条新断言（ExtendedExplanations 覆盖、v2.0 lower-case 禁入、8 域闭合）。
- **跨 OS 差异集中在 `--fix` 3 类**：FUSE 解挂（fusermount vs umount）、DNS flush（macOS dscacheutil/killall -HUP vs Linux resolvectl/systemd-resolve）、Statfs 签名（用 `golang.org/x/sys/unix` 抹平）——其它远端命令（mergerfs xattr、mutagen.version、sshd -T、mountpoint、df）全部在容器内 Linux 执行，无跨 OS 问题。
- **M13/M14 两条 pitfall 靠 `scripts/ci-doctor-grep.sh`（3 段 jq/grep 断言）+ `TestRenderTextContainsNextAction` + `TestDowngradeBannerRendersChain` 三层回归** 闭环；errcodes Registry 层面 `MustRegister` 强制 NextAction 非空（`codes.go:68` 已实现）。
- **Plan 拆分严格 Wave 1/2/3 串行**（CONTEXT D-30）；Plan 02 注册 `--fix` flag 占位，Plan 03 才落 FixerRegistry 5 类——避免 Plan 03 阻塞 Plan 02 ship。
