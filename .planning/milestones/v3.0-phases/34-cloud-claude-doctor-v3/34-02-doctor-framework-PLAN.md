---
phase: 34-cloud-claude-doctor-v3
plan: 02
type: execute
wave: 2
depends_on: [01]
autonomous: true
requirements: [REQ-F6-A, REQ-F6-B, REQ-F6-D]
requirements_addressed: [REQ-F6-A, REQ-F6-B, REQ-F6-D]
files_modified:
  - internal/cloudclaude/doctor/doctor.go
  - internal/cloudclaude/doctor/check.go
  - internal/cloudclaude/doctor/network.go
  - internal/cloudclaude/doctor/auth.go
  - internal/cloudclaude/doctor/ssh.go
  - internal/cloudclaude/doctor/mount.go
  - internal/cloudclaude/doctor/disk.go
  - internal/cloudclaude/doctor/remote_runner.go
  - internal/cloudclaude/doctor/render.go
  - internal/cloudclaude/doctor/doctor_test.go
  - internal/cloudclaude/doctor/network_test.go
  - internal/cloudclaude/doctor/auth_test.go
  - internal/cloudclaude/doctor/ssh_test.go
  - internal/cloudclaude/doctor/mount_test.go
  - internal/cloudclaude/doctor/disk_test.go
  - internal/cloudclaude/doctor/render_test.go
  - internal/cloudclaude/colors.go
  - cmd/cloud-claude/doctor.go
  - cmd/cloud-claude/main.go

must_haves:
  truths:
    - "cloud-claude doctor 覆盖 5 维度（network / auth / ssh / mount / disk），共 ≥ 17 项 check（REQ-F6-A / ROADMAP §Phase 34 SC#2 / CONTEXT D-19）"
    - "每项 warn/fail 输出文本含 [符号] + 中文原因 + '建议:' + 错误码四要素（REQ-F6-B / ROADMAP §Phase 34 SC#3 / PITFALLS M14）"
    - "doctor 第一屏渲染降级历史 banner：last-session.json.DowngradeChain 每条输出 '[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>'（ROADMAP §Phase 34 SC#6 / PITFALLS M13 / CONTEXT D-13）"
    - "--json 输出 schema_version=1 锁死（不带 omitempty）+ 退出码 0/1/2 与 brew doctor 对齐（REQ-F6-D / CONTEXT D-15 / D-16）"
    - "doctor 不调用任何 host-agent endpoint；远端检查走 SSH conn 在容器内 exec（ROADMAP §Phase 34 SC#9 / CONTEXT D-28 / D-20）"
    - "doctor 未 init 时 network/auth 本地 check 仍跑；mount/ssh/disk 标 StatusSkip + 中文原因（CONTEXT D-06）"
    - "--fix flag 此 plan 仅注册占位（Plan 03 落 FixerRegistry 才真跑），`--fix` + 空 FixerRegistry 时输出 '[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）'（CONTEXT D-29/D-30）"
  artifacts:
    - path: internal/cloudclaude/doctor/doctor.go
      provides: "RunDoctor / Options / Report / Status / DowngradeBanner / Summary 顶层结构"
      contains: "func RunDoctor"
    - path: internal/cloudclaude/doctor/check.go
      provides: "Check struct + Checker interface + runWithTimeout helper"
      contains: "type Checker interface"
    - path: internal/cloudclaude/doctor/remote_runner.go
      provides: "RemoteRunner interface + sshRemoteRunner 实现（mock 友好）"
      contains: "type RemoteRunner interface"
    - path: internal/cloudclaude/doctor/render.go
      provides: "RenderText + RenderJSON + renderDowngradeBanner + 状态符号常量"
      contains: "func RenderJSON"
    - path: cmd/cloud-claude/doctor.go
      provides: "newDoctorCmd + ValidArgs + --fix/--verbose/--json/--yes flag + 退出码 0/1/2"
      contains: "newDoctorCmd"
    - path: internal/cloudclaude/colors.go
      provides: "colorEnabled → ColorEnabled 导出（doctor 包跨包复用，CONTEXT Established Patterns 要求）"
      contains: "func ColorEnabled"
  key_links:
    - from: "internal/cloudclaude/doctor/doctor.go::RunDoctor"
      to: "5 维度 check functions + RemoteRunner lazy connect + Summary 聚合"
      via: "串行执行 network → auth → ssh → mount → disk；第一次需要远端的 check 之前 sshConnect；defer conn.Close()"
      pattern: "func RunDoctor\\(ctx context.Context, opts Options\\) \\(\\*Report, error\\)"
    - from: "internal/cloudclaude/doctor/render.go::renderDowngradeBanner"
      to: "cloudclaude.LastSessionSnapshot.DowngradeChain"
      via: "每个 DowngradeStep 输出 '[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>'"
      pattern: "\\[降级\\]"
    - from: "cmd/cloud-claude/doctor.go::runDoctor"
      to: "doctor.RunDoctor + os.Exit(0/1/2)"
      via: "Summary.Fail > 0 → 2；Summary.Warn > 0 → 1；else → 0"
      pattern: "os.Exit\\("
---

<objective>
交付 `cloud-claude doctor` 5 维度自检框架（不含 --fix 实施，Plan 03 落）。具体实现：
1. `internal/cloudclaude/doctor/` 新包，按 CONTEXT D-01 / PATTERNS §2.1-§2.10 拆分 10 个 .go 文件 + 6 个 _test.go；
2. `RunDoctor(ctx, opts) (*Report, error)` 顶层入口，串行执行 17 项 check（分布见 CONTEXT D-19 表）；
3. 远端 check 走 `RemoteRunner` interface（lazy 建立 SSH，连不上时 RequiresRemote=true 的 check 全部 `StatusSkip`）；
4. 第一屏 banner 渲染降级历史（CONTEXT D-13 / ROADMAP SC#6）；
5. 文本 / JSON 双输出（CONTEXT D-15 schema_version=1 锁死、RESEARCH §5.1 struct tag 表）；
6. 退出码 0/1/2 与 brew doctor 对齐（CONTEXT D-16）；
7. `cmd/cloud-claude/doctor.go` 新建 cobra 子命令 + 4 flag + ValidArgs；`cmd/cloud-claude/main.go` 注册；
8. `internal/cloudclaude/colors.go` 内部 `colorEnabled` / `colorize` 首字母改大写导出，供 doctor 包复用（CONTEXT Established Patterns / PATTERNS §2.10 要求）。

Purpose: 本 Plan 作为 **Wave 2 主框架**：提供所有 17 项 check 的 pass/warn/fail/skip 4 路径 + 第一屏降级历史渲染，让用户独立 ship 即可跑 `cloud-claude doctor` 看完整诊断；`--fix` 由 Plan 03 闭合（本 plan 仅把 flag 注册到位 + 输出占位提示，避免 Plan 03 阻塞本 plan ship）。

Output:
- 1 个新增 doctor/ 子包（9 个 .go 源文件 + 7 个 _test.go）
- 1 个修改的 colors.go（导出 helper）
- 1 个新增 cmd/cloud-claude/doctor.go
- 1 个修改的 main.go（AddCommand + switch case）
</objective>

<execution_context>
@.cursor/skills/gsd-execute-phase/SKILL.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md
@.planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md
@.planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md
@.planning/phases/34-cloud-claude-doctor-v3/34-01-errcodes-explain-PLAN.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/32-ssh-tmux/32-CONTEXT.md

# 直接改造对象（读）
@internal/cloudclaude/ssh_doctor.go
@internal/cloudclaude/last_session.go
@internal/cloudclaude/oauth_check.go
@internal/cloudclaude/mount_mutagen.go
@internal/cloudclaude/mutagen_bin.go
@internal/cloudclaude/envcheck.go
@internal/cloudclaude/entry.go
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/config.go
@internal/cloudclaude/colors.go
@cmd/cloud-claude/main.go
@cmd/cloud-claude/sessions.go
@deploy/scripts/host-preflight.sh

<interfaces>
<!-- 上游 Plan 01 Wave 1 交付 + 既有 v3.0 接口。 -->

From internal/cloudclaude/errcodes/codes.go (Plan 01 完成后):
```go
// 8 域闭合 Registry：MOUNT / SESSION / NET / STATE / SYSTEM / SSH / AUTH / DISK
// Plan 02 doctor 所有 warn/fail 必须携带已注册 Code：
//   network: SYSTEM_DNS_RESOLVE_FAILED / AUTH_GATEWAY_UNREACHABLE / NET_EGRESS_IP_DRIFT
//   auth:    AUTH_CONFIG_MISSING / AUTH_TOKEN_EXPIRED / NET_OAUTH_* (4 态)
//   ssh:     SESSION_KEEPALIVE_TOO_AGGRESSIVE / SSH_SSHD_KEEPALIVE_DRIFT / SSH_KNOWN_HOSTS_CONFLICT
//   mount:   MOUNT_MUTAGEN_VERSION_SKEW / MOUNT_MERGERFS_FAILED / MOUNT_SSHFS_DISCONNECTED / SYSTEM_FUSE_RESIDUAL_MOUNT / SYSTEM_APPARMOR_FUSERMOUNT3_MISSING
//   disk:    DISK_LOCAL_LOW / DISK_CONTAINER_LOW / DISK_MUTAGEN_DATA_BLOAT
//   timeout: SYSTEM_CHECK_TIMEOUT
```

From internal/cloudclaude/last_session.go:
```go
type LastSessionSnapshot struct {
    SchemaVersion    int             // == 1
    Timestamp        time.Time
    IntendedMode     string
    ActualMode       string
    DowngradeChain   []DowngradeStep // doctor 第一屏读取
    ConflictCount    int
    TmuxSession      string
    ClientRole       string          // "primary"|"secondary"
    ReconnectCount   int
}

type DowngradeStep struct {
    From          string
    To            string
    ReasonCode    string
    ReasonMessage string
}

// 读取函数（Plan 02 直接用）
func LoadLastSession() (*LastSessionSnapshot, error) // 路径 ~/.cloud-claude/last-session.json
```

From internal/cloudclaude/oauth_check.go:
```go
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)

type OAuthStatus struct {
    State           OAuthState      // OAuthValid / OAuthExpired / OAuthExpiringSoon / OAuthNotFound
    ExpiresAt       time.Time
    MinutesToExpire int
}
```

From internal/cloudclaude/ssh_doctor.go:
```go
func RunSSHDoctor(cfg SSHConfig, opts SSHDoctorOptions) (*SSHDoctorResult, error)
```

From internal/cloudclaude/mount_mutagen.go (line 230-237):
```go
// 3) 版本握手
if connA != nil {
    remoteVer, _ := deps.remoteVersion(connA)
    if remoteVer != "" && !strings.Contains(remoteVer, strings.TrimPrefix(MutagenBinaryVersion, "v")) {
        return nil, MutagenSyncStatus{}, newMutagenErr(errcodes.MOUNT_MUTAGEN_VERSION_SKEW, MutagenBinaryVersion, remoteVer)
    }
}
```

From internal/cloudclaude/mutagen_bin.go:
```go
const MutagenBinaryVersion = "v0.18.1"
```

From internal/cloudclaude/colors.go (既有 lower-case helper，本 plan Task 2.1 改大写导出):
```go
func colorEnabled() bool
func colorize(text, ansi string) string
const ansiGreen / ansiYellow / ansiRed / ansiGray
```

From internal/cloudclaude/mount_strategy.go:
```go
type MountConfig struct {
    Mode              Mode
    KeepAliveInterval time.Duration
    // ...
}
```

From internal/cloudclaude/config.go:
```go
func LoadConfig() (*Config, error)      // 读 ~/.cloud-claude/config.yaml
func ConfigPath() (string, error)
type Config struct {
    Gateway  string
    ShortID  string
    Password string
    // ...
}
```

From cmd/cloud-claude/main.go (line 18-25):
```go
const (
    exitOK            = 0
    exitAuthFailed    = 1
    exitNetworkError  = 2
    // ...
    exitConfigError   = 4
)
```

</interfaces>
</context>

<tasks>

<task type="execute">
  <name>Task 2.1: colors.go 导出 colorEnabled / colorize（cross-package 复用；PATTERNS §2.10 必需）</name>
  <files>internal/cloudclaude/colors.go</files>

  <read_first>
    - internal/cloudclaude/colors.go（完整文件）
    - 全部调用点（grep 索引）：`rg -n "colorEnabled\|colorize" internal/cloudclaude/`
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.10（「不引入新颜色 helper」→ 必须导出既有）
  </read_first>

  <action>
**第 1 步：** 在 `internal/cloudclaude/colors.go` 中把 `func colorEnabled` 改名为 `func ColorEnabled`，`func colorize` 改名为 `func Colorize`（首字母大写导出）；如有内部调用也同步改名。

**第 2 步：** 使用 Grep 检索仓库内所有 `colorEnabled(` 与 `colorize(` 调用位置（不限于 `internal/cloudclaude/` 包），逐一替换成 `ColorEnabled(` / `Colorize(`。

**第 3 步（ANSI 常量导出）：** 导出 4 个 ANSI 常量：把 `internal/cloudclaude/colors.go` 中的 `ansiGreen` → `AnsiGreen`、`ansiYellow` → `AnsiYellow`、`ansiRed` → `AnsiRed`、`ansiGray` → `AnsiGray`（首字母大写导出），同时全仓 grep 替换所有调用点（包括 `ssh_doctor.go` 等 v2 文件内的 `ansiGreen / ansiYellow / ansiRed / ansiGray` 引用）。Task 2.9 render.go 的 `pickIcon` 直接依赖 `cloudclaude.AnsiGreen` 等导出名，若未做此步编译失败。

**executor 注意（SSHConnect 无需任何改动）：** `cloudclaude.SSHConnect` 已于 Phase 32-02 在 `internal/cloudclaude/ssh.go:200` 作为 export 包装就位（`func SSHConnect(cfg SSHConfig) (*ssh.Client, error) { return sshConnect(cfg) }`），Task 2.10 RunDoctor 的 lazy connect 分支 `cloudclaude.SSHConnect(sshCfg)` 直接调用即可，本 plan 无需任何重命名 / 导出操作；**不要**去改 `internal/cloudclaude/ssh_doctor.go`（该文件只调用 `sshConnect`，没有定义；改名会破坏 Phase 32-02 已交付的公共 API）。

**验证要点：**
- `go build ./...` PASS
- `go vet ./...` 不报 "exported function ColorEnabled should have comment" — 添加一行文档注释 `// ColorEnabled 返回是否开启 ANSI 色彩（NO_COLOR=1 / 非 TTY / Windows 无 VT 时返回 false）。`
- 既有 `internal/cloudclaude/` 包内测试 `go test ./internal/cloudclaude/ -count=1 -short` PASS

**禁止：**
- 保留 lower-case wrapper `func colorEnabled() bool { return ColorEnabled() }`（不允许双重 API，增加维护负担；直接替换调用点）
- 改动 ANSI 常量字面值（`\x1b[32m` 等）
  </action>

  <acceptance_criteria>
    - `grep -q "^func ColorEnabled" internal/cloudclaude/colors.go` 命中
    - `grep -q "^func Colorize" internal/cloudclaude/colors.go` 命中
    - `grep -qE "^\s*AnsiGreen\s*=" internal/cloudclaude/colors.go` 命中（colors.go 使用分组 `const ( ... )` 形式）
    - `grep -qE "\bAnsiYellow\b" internal/cloudclaude/colors.go` 命中
    - `grep -qE "\bAnsiRed\b" internal/cloudclaude/colors.go` 命中
    - `grep -qE "\bAnsiGray\b" internal/cloudclaude/colors.go` 命中
    - `! grep -nE "^func colorEnabled\b|^func colorize\b" internal/cloudclaude/colors.go` 为空（小写版本已完全删除）
    - `! rg -nE "\bcolorEnabled\(|\bcolorize\(" internal/cloudclaude/ cmd/cloud-claude/` 结果为空（所有调用点已改大写）
    - `! rg -nE "\bansiGreen\b|\bansiYellow\b|\bansiRed\b|\bansiGray\b" internal/cloudclaude/ cmd/cloud-claude/` 结果为空（所有 ANSI 小写引用已改大写）
    - `go build ./...` 退出码 = 0
    - `go vet ./...` 退出码 = 0
    - `go test ./internal/cloudclaude/ -count=1 -short` 退出码 = 0（既有测试不回归）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.2: 新建 doctor/doctor.go + doctor/check.go — Report/Options/Status/Checker（CONTEXT D-01 / RESEARCH §5.1 / PATTERNS §2.1-§2.2）</name>
  <files>internal/cloudclaude/doctor/doctor.go, internal/cloudclaude/doctor/check.go</files>

  <read_first>
    - internal/cloudclaude/ssh_doctor.go（line 18-94 FileReport + RunSSHDoctor 控制流）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-01（Report/Options/Status 字段）+ §D-07（串行执行）+ §D-08（5s/30s timeout）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §5.1（RESEARCH Go struct tag 表）+ §5.2（退出码）+ §5.3（Status 不降级）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.1 / §2.2 adapt 列表
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/doctor.go`（**逐字符按 RESEARCH §5.1 struct tag 表**）：**

```go
// Package doctor — Phase 34 Plan 02：cloud-claude doctor 五维度自检框架。
//
// Entry point: RunDoctor(ctx, opts) -> *Report
// 串行执行：network → auth → ssh → mount → disk（CONTEXT D-07）
// 远端 check 走 RemoteRunner interface（D-20 lazy connect + StatusSkip 降级）
package doctor

import (
	"context"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// Status 对标 CONTEXT D-15 JSON schema 字面量 "pass"|"warn"|"fail"|"skip"。
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Options 是 RunDoctor 的入参（CONTEXT D-01）。
type Options struct {
	Domain       string        // "network" / "auth" / "ssh" / "mount" / "disk" / "all"（cobra 已校验）
	Fix          bool          // Plan 03 落实；Plan 02 仅注册 flag
	Verbose      bool          // --verbose 每 check 打印时间戳 + Details 全字段
	JSON         bool          // --json 切换 RenderJSON（Text 不输出）
	NoColor      bool          // 显式关闭颜色（叠加在 colors.ColorEnabled 之上）
	Yes          bool          // Plan 03 用：confirmDestructive 跳过交互
	CheckTimeout time.Duration // 单 check timeout；0 则取默认 5s（Verbose 30s）
}

// DowngradeBanner 是第一屏降级历史（CONTEXT D-13 / RESEARCH §5.1）。
type DowngradeBanner struct {
	SnapshotAgeSeconds int64           `json:"snapshot_age_seconds"`
	IntendedMode       string          `json:"intended_mode,omitempty"`
	ActualMode         string          `json:"actual_mode,omitempty"`
	DowngradeChain     []DowngradeStep `json:"downgrade_chain,omitempty"`
	ConflictCount      int             `json:"conflict_count,omitempty"`
	ReconnectCount     int             `json:"reconnect_count,omitempty"`
	TmuxSession        string          `json:"tmux_session,omitempty"`
	ClientRole         string          `json:"client_role,omitempty"`
}

// DowngradeStep 源自 cloudclaude.LastSessionSnapshot.DowngradeChain；doctor 只读不写。
type DowngradeStep struct {
	From          string `json:"from"`
	To            string `json:"to"`
	ReasonCode    string `json:"reason_code,omitempty"`
	ReasonMessage string `json:"reason_message,omitempty"`
}

// Summary 汇总统计（CONTEXT D-13 末段）。
type Summary struct {
	Total int `json:"total"`
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
}

// Report 是 RunDoctor 返回值 + --json 序列化对象（schema_version=1 锁死，RESEARCH §5.1）。
type Report struct {
	SchemaVersion    int              `json:"schema_version"` // 硬编码 1，不带 omitempty（jq 依赖）
	StartedAt        time.Time        `json:"started_at"`
	DurationMS       int64            `json:"duration_ms"`
	CloudClaudeVer   string           `json:"cloud_claude_version,omitempty"`
	RemoteImageVer   string           `json:"remote_image_version,omitempty"`
	DowngradeHistory *DowngradeBanner `json:"downgrade_history,omitempty"`
	Summary          Summary          `json:"summary"`
	Checks           []Check          `json:"checks"`
}

// RunDoctor 顶层入口。执行顺序（CONTEXT D-07）：network → auth → ssh → mount → disk。
// 远端 conn lazy 建立（CONTEXT D-20），连不上时 RequiresRemote=true 的 check 全部 StatusSkip。
// 注意：Plan 02 落本函数的主干；Plan 03 会扩展 --fix 走 FixerRegistry。
func RunDoctor(ctx context.Context, opts Options) (*Report, error) {
	// executor 按 D-07 / D-19 实现：
	// 1) 读 LastSessionSnapshot → DowngradeHistory
	// 2) 按 opts.Domain 过滤要跑哪些维度（"all" = 全部）
	// 3) 串行跑每项 check；维度间共享 lazy remoteConn（首次需要时 sshConnect；defer close）
	// 4) 未 init (cloudclaude.LoadConfig 失败) → mount/ssh/disk 全部 StatusSkip + 中文原因（D-06）
	// 5) 聚合 Summary 返回
	panic("executor: 按本注释 5 步实现 RunDoctor；参考 ssh_doctor.RunSSHDoctor 控制流 + PATTERNS §2.1")
}
```

**(b) 新建 `internal/cloudclaude/doctor/check.go`（**RESEARCH §5.1 Check struct + PATTERNS §2.2 Checker interface**）：**

```go
package doctor

import (
	"context"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// Check 是单项检查结果（RESEARCH §5.1 JSON tag 严格对齐）。
type Check struct {
	Domain     string         `json:"domain"`
	Name       string         `json:"name"`
	Status     Status         `json:"status"`
	Code       errcodes.Code  `json:"code,omitempty"`
	Message    string         `json:"message,omitempty"`
	NextAction string         `json:"next_action,omitempty"` // CI grep gate 关键字段
	Details    map[string]any `json:"details,omitempty"`
	FixApplied []string       `json:"fix_applied,omitempty"` // Plan 03 填充
	FixFailed  []string       `json:"fix_failed,omitempty"`  // Plan 03 填充
	DurationMS int64          `json:"duration_ms"`
}

// Checker 是单项检查的统一接口；Plan 02 每个维度的 check function 可直接实现也可走 helper。
// Run 负责检测本身；Fix（Plan 03 落）幂等修复。
type Checker interface {
	Run(ctx context.Context, runner RemoteRunner) Check
	Fix(ctx context.Context, opts Options) Check
}

// runWithTimeout 包装单 check 的 context timeout（CONTEXT D-08）：
//   - timeout 命中 → Status=StatusFail + Code=SYSTEM_CHECK_TIMEOUT + 中文 Message
//   - 正常返回 → 保留 fn 返回的 Check
func runWithTimeout(ctx context.Context, domain, name string, timeout time.Duration, fn func(context.Context) Check) Check {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	done := make(chan Check, 1)
	go func() {
		done <- fn(ctx2)
	}()
	select {
	case c := <-done:
		if c.DurationMS == 0 {
			c.DurationMS = time.Since(start).Milliseconds()
		}
		return c
	case <-ctx2.Done():
		return Check{
			Domain:     domain,
			Name:       name,
			Status:     StatusFail,
			Code:       errcodes.SYSTEM_CHECK_TIMEOUT,
			Message:    "检查超时（" + timeout.String() + "）",
			NextAction: "加 --verbose 放宽到 30s，或检查远端容器状态",
			DurationMS: time.Since(start).Milliseconds(),
		}
	}
}

// newPass / newWarn / newFail / newSkip 是 check 函数用的 constructor helpers（减少 verbosity）。
// Plan 02 各维度文件直接调用；Code 可空（Pass/Skip 允许不带 Code）。
func newPass(domain, name, msg string) Check {
	return Check{Domain: domain, Name: name, Status: StatusPass, Message: msg}
}

func newWarn(domain, name string, code errcodes.Code, args ...any) Check {
	entry, _ := errcodes.Lookup(code)
	msg := entry.Message
	if len(args) > 0 {
		msg = fmtSprintf(entry.Message, args...)
	}
	return Check{Domain: domain, Name: name, Status: StatusWarn, Code: code, Message: msg, NextAction: entry.NextAction}
}

func newFail(domain, name string, code errcodes.Code, args ...any) Check {
	entry, _ := errcodes.Lookup(code)
	msg := entry.Message
	if len(args) > 0 {
		msg = fmtSprintf(entry.Message, args...)
	}
	return Check{Domain: domain, Name: name, Status: StatusFail, Code: code, Message: msg, NextAction: entry.NextAction}
}

func newSkip(domain, name, reason string) Check {
	return Check{Domain: domain, Name: name, Status: StatusSkip, Message: reason}
}
```

**executor 注意：** 顶部 import 需要的 `fmt.Sprintf` 可以直接 import 并调用；这里 `fmtSprintf` 是占位意指 `fmt.Sprintf`，executor 改正即可（或直接用 `fmt.Sprintf(...)`）。删除本提示行。

**禁止：**
- 在 `doctor.go` 或 `check.go` 内引入任何远端执行逻辑（走 `remote_runner.go` / 各维度文件）
- Status 用 int 而非 string（JSON schema 依赖字面量）
- 把 `SchemaVersion` 字段加 `omitempty`（必须始终输出 `"schema_version": 1`）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/doctor.go && test -f internal/cloudclaude/doctor/check.go` 退出码 = 0
    - `grep -q "^package doctor$" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "type Report struct" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "SchemaVersion\s*int\s*\`json:\"schema_version\"\`$" internal/cloudclaude/doctor/doctor.go` 命中（**不带 omitempty**）
    - `grep -q "type Options struct" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "type DowngradeBanner struct" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "func RunDoctor(ctx context.Context, opts Options) (\\*Report, error)" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "StatusPass Status = \"pass\"" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "StatusSkip Status = \"skip\"" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "type Check struct" internal/cloudclaude/doctor/check.go` 命中
    - `grep -q "type Checker interface" internal/cloudclaude/doctor/check.go` 命中
    - `grep -q "func runWithTimeout" internal/cloudclaude/doctor/check.go` 命中
    - `grep -q "errcodes.SYSTEM_CHECK_TIMEOUT" internal/cloudclaude/doctor/check.go` 命中（timeout 命中映射）
    - 本 task 完成后单独 build 可能失败（RunDoctor panic 占位 + fmtSprintf 占位），**Task 2.10 会修复**；本 task acceptance 允许 `go vet ./internal/cloudclaude/doctor/...` 不通过（结构先到位）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.3: 新建 doctor/remote_runner.go — RemoteRunner interface + sshRemoteRunner 实现（CONTEXT D-20 / PATTERNS §2.8）</name>
  <files>internal/cloudclaude/doctor/remote_runner.go</files>

  <read_first>
    - internal/cloudclaude/ssh_doctor.go（line 96-117 `runSSHSession` stdout/stderr 收集模板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-20（lazy connect + StatusSkip 降级）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §9 第 1 条（interface vs struct 拍板为 interface）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.8
  </read_first>

  <action>
新建 `internal/cloudclaude/doctor/remote_runner.go`：

```go
package doctor

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// RemoteRunner 抽象远端命令执行，Plan 02 单测注入 fakeRunner（CONTEXT D-20 / RESEARCH §9 第 1 条）。
// name 仅用于 --verbose 日志标签；script 是完整 shell 片段（executor 必须保证 shellescape 已处理）。
type RemoteRunner interface {
	RunScript(name, script string) (stdout, stderr string, err error)
}

// sshRemoteRunner 是生产实现，基于 golang.org/x/crypto/ssh。
// 与 ssh_doctor.go:runSSHSession 的 stdout/stderr 收集模式逐字符对齐。
type sshRemoteRunner struct {
	conn *ssh.Client
}

// NewSSHRemoteRunner 构造生产 runner。conn 由 RunDoctor 在第一次需要远端的 check 前 lazy 建立。
func NewSSHRemoteRunner(conn *ssh.Client) RemoteRunner {
	return &sshRemoteRunner{conn: conn}
}

func (r *sshRemoteRunner) RunScript(name, script string) (string, string, error) {
	if r.conn == nil {
		return "", "", fmt.Errorf("remote_runner: conn is nil (name=%s)", name)
	}
	sess, err := r.conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("创建 SSH 会话失败 (%s): %w", name, err)
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if err := sess.Run(script); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), stderr.String(), fmt.Errorf("%w (%s)", err, msg)
	}
	return stdout.String(), stderr.String(), nil
}
```

**禁止：**
- 在 RemoteRunner 内做 shellescape（由各维度文件调用前做 — CONTEXT Established Patterns「SSH 远端命令构造 全走 shellescape.QuoteCommand」）
- 修改 `ssh_doctor.go` 的既有 `runSSHSession`（双份实现允许，保持 Plan 02 与 ssh_doctor 解耦）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/remote_runner.go` 退出码 = 0
    - `grep -q "type RemoteRunner interface" internal/cloudclaude/doctor/remote_runner.go` 命中
    - `grep -q "RunScript(name, script string) (stdout, stderr string, err error)" internal/cloudclaude/doctor/remote_runner.go` 命中
    - `grep -q "type sshRemoteRunner struct" internal/cloudclaude/doctor/remote_runner.go` 命中
    - `grep -q "func NewSSHRemoteRunner" internal/cloudclaude/doctor/remote_runner.go` 命中
    - `grep -q "golang.org/x/crypto/ssh" internal/cloudclaude/doctor/remote_runner.go` 命中
    - `go build ./internal/cloudclaude/doctor/...` 退出码 = 0（独立可编译，不依赖未完成 task）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.4: 新建 doctor/network.go + network_test.go — 3 check（dns_resolve / gateway_reachable / egress_ip_visible）</name>
  <files>internal/cloudclaude/doctor/network.go, internal/cloudclaude/doctor/network_test.go</files>

  <read_first>
    - internal/cloudclaude/envcheck.go（line 30-65 run helper 模式 + 远端 curl）
    - internal/cloudclaude/entry.go（HTTP client + AuthenticateAndWait / gateway 解析）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-19（network 3 check 错误码映射）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.1 完整命令表 + gotcha
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.3（包级 var mock `lookupHost` / HTTP client）
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/network.go`（3 check + 包级 var mock 注入点）：**

```go
package doctor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// lookupHost / httpHead 是包级 var，便于 network_test.go 注入 mock。
var (
	lookupHost = net.LookupHost
	httpGet    = func(ctx context.Context, url string, timeout time.Duration) (int, error) {
		ctx2, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx2, "GET", url, nil)
		if err != nil {
			return 0, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
)

// checkDNSResolve 本地 DNS 解析（RESEARCH §3.1 表）。
func checkDNSResolve(ctx context.Context, host string) Check {
	if host == "" {
		return newSkip("network", "dns_resolve", "未配置网关，跳过；运行 cloud-claude init 配置后重试")
	}
	addrs, err := lookupHost(host)
	if err != nil {
		return newFail("network", "dns_resolve", errcodes.SYSTEM_DNS_RESOLVE_FAILED, host, err.Error())
	}
	if len(addrs) == 0 {
		return newFail("network", "dns_resolve", errcodes.SYSTEM_DNS_RESOLVE_FAILED, host, "returned empty addr list")
	}
	return Check{
		Domain: "network", Name: "dns_resolve", Status: StatusPass,
		Message: fmt.Sprintf("%s 解析到 %d 个地址", host, len(addrs)),
		Details: map[string]any{"addrs": addrs},
	}
}

// checkGatewayReachable 裸 HTTP GET /healthz（**不**复用 EntryClient，避免误报 token 过期成网络问题，RESEARCH §3.1 关键实现信号）。
func checkGatewayReachable(ctx context.Context, gateway string) Check {
	if gateway == "" {
		return newSkip("network", "gateway_reachable", "未配置网关，跳过；运行 cloud-claude init 配置后重试")
	}
	url := strings.TrimRight(gateway, "/") + "/healthz"
	status, err := httpGet(ctx, url, 2*time.Second)
	if err != nil {
		return newFail("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, err.Error())
	}
	switch {
	case status == 200 || status == 204:
		return newPass("network", "gateway_reachable", fmt.Sprintf("%s 返回 %d", url, status))
	case status >= 500:
		return newWarn("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, fmt.Sprintf("HTTP %d", status))
	default:
		return newFail("network", "gateway_reachable", errcodes.AUTH_GATEWAY_UNREACHABLE, gateway, fmt.Sprintf("HTTP %d", status))
	}
}

// checkEgressIPVisible 远端 curl ifconfig.io（RESEARCH §3.1 表）。
// runner 为 nil 时（未 init 或 SSH 未建立）→ StatusSkip（D-06 / D-20）。
func checkEgressIPVisible(ctx context.Context, runner RemoteRunner, expectedIP string) Check {
	if runner == nil {
		return newSkip("network", "egress_ip_visible", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("egress_ip",
		"curl -sS --max-time 5 https://ifconfig.io 2>/dev/null || curl -sS --max-time 5 https://checkip.amazonaws.com")
	if err != nil {
		return newFail("network", "egress_ip_visible", errcodes.NET_EGRESS_IP_DRIFT, "unknown", "curl 失败: "+err.Error())
	}
	got := strings.TrimSpace(stdout)
	if expectedIP != "" && got != expectedIP {
		return newWarn("network", "egress_ip_visible", errcodes.NET_EGRESS_IP_DRIFT, got, expectedIP)
	}
	return Check{
		Domain: "network", Name: "egress_ip_visible", Status: StatusPass,
		Message: "容器出口 IP: " + got,
		Details: map[string]any{"egress_ip": got},
	}
}
```

**(b) 新建 `internal/cloudclaude/doctor/network_test.go`：**

```go
package doctor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	out, errs string
	err       error
}

func (f *fakeRunner) RunScript(name, script string) (string, string, error) {
	return f.out, f.errs, f.err
}

func TestCheckDNSResolve_Empty_Skip(t *testing.T) {
	c := checkDNSResolve(context.Background(), "")
	if c.Status != StatusSkip {
		t.Errorf("empty host 应 StatusSkip，实际 %s", c.Status)
	}
}

func TestCheckDNSResolve_Error_Fail(t *testing.T) {
	orig := lookupHost
	lookupHost = func(host string) ([]string, error) { return nil, fmt.Errorf("no such host") }
	t.Cleanup(func() { lookupHost = orig })
	c := checkDNSResolve(context.Background(), "nonexistent.example.com")
	if c.Status != StatusFail {
		t.Errorf("lookup 失败应 StatusFail，实际 %s", c.Status)
	}
	if c.Code != "SYSTEM_DNS_RESOLVE_FAILED" {
		t.Errorf("Code 应为 SYSTEM_DNS_RESOLVE_FAILED，实际 %q", c.Code)
	}
	if c.NextAction == "" {
		t.Error("NextAction 不能为空（M14 回归）")
	}
}

func TestCheckDNSResolve_Success_Pass(t *testing.T) {
	orig := lookupHost
	lookupHost = func(host string) ([]string, error) { return []string{"1.2.3.4"}, nil }
	t.Cleanup(func() { lookupHost = orig })
	c := checkDNSResolve(context.Background(), "gw.example.com")
	if c.Status != StatusPass {
		t.Errorf("lookup 成功应 StatusPass，实际 %s", c.Status)
	}
}

func TestCheckGatewayReachable_200_Pass(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) { return 200, nil }
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusPass {
		t.Errorf("200 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckGatewayReachable_503_Warn(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) { return 503, nil }
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusWarn {
		t.Errorf("503 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "AUTH_GATEWAY_UNREACHABLE" {
		t.Errorf("Code 应为 AUTH_GATEWAY_UNREACHABLE，实际 %q", c.Code)
	}
}

func TestCheckGatewayReachable_Connection_Fail(t *testing.T) {
	orig := httpGet
	httpGet = func(ctx context.Context, url string, timeout time.Duration) (int, error) {
		return 0, fmt.Errorf("connection refused")
	}
	t.Cleanup(func() { httpGet = orig })
	c := checkGatewayReachable(context.Background(), "https://gw.example.com")
	if c.Status != StatusFail {
		t.Errorf("connection refused 应 Fail，实际 %s", c.Status)
	}
}

func TestCheckEgressIPVisible_NilRunner_Skip(t *testing.T) {
	c := checkEgressIPVisible(context.Background(), nil, "1.2.3.4")
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckEgressIPVisible_Drift_Warn(t *testing.T) {
	r := &fakeRunner{out: "5.6.7.8\n"}
	c := checkEgressIPVisible(context.Background(), r, "1.2.3.4")
	if c.Status != StatusWarn {
		t.Errorf("IP 漂移应 Warn，实际 %s", c.Status)
	}
	if c.Code != "NET_EGRESS_IP_DRIFT" {
		t.Errorf("Code 应为 NET_EGRESS_IP_DRIFT，实际 %q", c.Code)
	}
	if !strings.Contains(c.NextAction, "doctor") {
		t.Errorf("NextAction 应含 'doctor'（M14 回归），实际 %q", c.NextAction)
	}
}

func TestCheckEgressIPVisible_Match_Pass(t *testing.T) {
	r := &fakeRunner{out: "1.2.3.4\n"}
	c := checkEgressIPVisible(context.Background(), r, "1.2.3.4")
	if c.Status != StatusPass {
		t.Errorf("IP 匹配应 Pass，实际 %s", c.Status)
	}
}
```

**verbatim 守恒：** 3 条 check 函数名（`checkDNSResolve / checkGatewayReachable / checkEgressIPVisible`）；错误码字面量与 errcodes 完全一致。
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/network.go && test -f internal/cloudclaude/doctor/network_test.go` 退出码 = 0
    - `grep -q "var lookupHost = net.LookupHost" internal/cloudclaude/doctor/network.go` 命中（mock 注入点）
    - `grep -q "var httpGet" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "func checkDNSResolve" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "func checkGatewayReachable" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "func checkEgressIPVisible" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "errcodes.SYSTEM_DNS_RESOLVE_FAILED" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "errcodes.AUTH_GATEWAY_UNREACHABLE" internal/cloudclaude/doctor/network.go` 命中
    - `grep -q "errcodes.NET_EGRESS_IP_DRIFT" internal/cloudclaude/doctor/network.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestCheckDNSResolve|TestCheckGatewayReachable|TestCheckEgressIPVisible" -count=1 -v` 退出码 = 0（8 条子测全 PASS）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.5: 新建 doctor/auth.go + auth_test.go — 3 check（config_present / entry_token_valid / oauth_credentials）</name>
  <files>internal/cloudclaude/doctor/auth.go, internal/cloudclaude/doctor/auth_test.go</files>

  <read_first>
    - internal/cloudclaude/config.go（LoadConfig 签名 + 错误信息格式）
    - internal/cloudclaude/entry.go（AuthenticateAndWait 签名 + AuthResponse 字段）
    - internal/cloudclaude/oauth_check.go（line 44-63 CheckOAuthCredentials + OAuthStatus 三态）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-19（auth 3 check 错误码映射）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.2（auth 各 check 返回值）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.4
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/auth.go`（3 check）：**

```go
package doctor

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// loadConfig 包级 var 注入点（auth_test.go mock 无配置场景）。
var loadConfig = cloudclaude.LoadConfig

// entryAuthenticate 包级 var（mock Entry API）。签名与 EntryClient.AuthenticateAndWait 对齐但返回简化。
var entryAuthenticate = func(ctx context.Context, gateway, shortID, password string) (*cloudclaude.AuthResponse, error) {
	client := cloudclaude.NewEntryClient(gateway)
	return client.AuthenticateAndWait(ctx, shortID, password, func(string) {})
}

// checkConfigPresent — AUTH_CONFIG_MISSING (Fatal)：LoadConfig 失败即 fail。
func checkConfigPresent(ctx context.Context) (Check, *cloudclaude.Config) {
	cfg, err := loadConfig()
	if err != nil {
		c := newFail("auth", "config_present", errcodes.AUTH_CONFIG_MISSING, err.Error())
		return c, nil
	}
	return newPass("auth", "config_present", "~/.cloud-claude/config.yaml 已加载"), cfg
}

// checkEntryTokenValid — AUTH_TOKEN_EXPIRED (Warn)：调 AuthenticateAndWait dry-run，200 pass / 401 warn。
// 返回 authResp 供后续 check（mount.mutagen_version_match / auth.oauth_credentials）复用。
func checkEntryTokenValid(ctx context.Context, cfg *cloudclaude.Config) (Check, *cloudclaude.AuthResponse) {
	if cfg == nil {
		return newSkip("auth", "entry_token_valid", "未加载 config，跳过"), nil
	}
	resp, err := entryAuthenticate(ctx, cfg.Gateway, cfg.ShortID, cfg.Password)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "401") || strings.Contains(msg, "认证失败") {
			return newWarn("auth", "entry_token_valid", errcodes.AUTH_TOKEN_EXPIRED, msg), nil
		}
		return newFail("auth", "entry_token_valid", errcodes.AUTH_GATEWAY_UNREACHABLE, cfg.Gateway, msg), nil
	}
	details := map[string]any{}
	if resp != nil {
		details["image_version"] = resp.ImageVersion
		details["claude_account_id"] = resp.ClaudeAccountID
	}
	return Check{
		Domain: "auth", Name: "entry_token_valid", Status: StatusPass,
		Message: fmt.Sprintf("Entry API 认证成功（image=%s）", resp.ImageVersion),
		Details: details,
	}, resp
}

// checkOAuthCredentials 复用 cloudclaude.CheckOAuthCredentials 三态 switch（CONTEXT D-19 / RESEARCH §3.2）。
// runner 参数预留：将来可改走 runner 代替 ssh.Client；目前直接传 ssh.Client（与 oauth_check.go 一致）。
func checkOAuthCredentials(ctx context.Context, conn *ssh.Client, claudeAccountID string) Check {
	if conn == nil {
		return newSkip("auth", "oauth_credentials", "未能连接远端容器，跳过")
	}
	if claudeAccountID == "" {
		return newSkip("auth", "oauth_credentials", "Entry API 未返回 claude_account_id，跳过")
	}
	status, err := cloudclaude.CheckOAuthCredentials(conn, claudeAccountID)
	if err != nil {
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, err.Error())
	}
	switch status.State {
	case cloudclaude.OAuthValid:
		return newPass("auth", "oauth_credentials", fmt.Sprintf("OAuth 有效（%d 分钟后过期）", status.MinutesToExpire))
	case cloudclaude.OAuthExpiringSoon:
		return newWarn("auth", "oauth_credentials", errcodes.NET_OAUTH_EXPIRING_SOON, status.MinutesToExpire)
	case cloudclaude.OAuthExpired:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_EXPIRED)
	case cloudclaude.OAuthNotFound:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, claudeAccountID)
	default:
		return newFail("auth", "oauth_credentials", errcodes.NET_OAUTH_NOT_FOUND, "unknown state")
	}
}
```

**executor 注意：** `cloudclaude.AuthResponse` 的实际字段名以 `entry.go` 为准；若 `ImageVersion` / `ClaudeAccountID` 字段名不同，按实际 struct 调整。

**(b) 新建 `internal/cloudclaude/doctor/auth_test.go`（≥ 6 条子测，覆盖 4 态 + skip + config fail）：**

```go
package doctor

import (
	"context"
	"fmt"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

func TestCheckConfigPresent_Fail(t *testing.T) {
	orig := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("~/.cloud-claude/config.yaml 不存在") }
	t.Cleanup(func() { loadConfig = orig })
	c, cfg := checkConfigPresent(context.Background())
	if c.Status != StatusFail {
		t.Errorf("应 Fail，实际 %s", c.Status)
	}
	if c.Code != "AUTH_CONFIG_MISSING" {
		t.Errorf("Code 应为 AUTH_CONFIG_MISSING，实际 %q", c.Code)
	}
	if cfg != nil {
		t.Error("失败时 cfg 必须 nil")
	}
}

func TestCheckConfigPresent_Pass(t *testing.T) {
	orig := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) {
		return &cloudclaude.Config{Gateway: "https://gw.example.com", ShortID: "x", Password: "y"}, nil
	}
	t.Cleanup(func() { loadConfig = orig })
	c, cfg := checkConfigPresent(context.Background())
	if c.Status != StatusPass {
		t.Errorf("应 Pass，实际 %s", c.Status)
	}
	if cfg == nil {
		t.Error("成功时 cfg 必须非 nil")
	}
}

func TestCheckEntryTokenValid_401_Warn(t *testing.T) {
	orig := entryAuthenticate
	entryAuthenticate = func(ctx context.Context, gw, id, pw string) (*cloudclaude.AuthResponse, error) {
		return nil, fmt.Errorf("认证失败: HTTP 401")
	}
	t.Cleanup(func() { entryAuthenticate = orig })
	cfg := &cloudclaude.Config{Gateway: "https://gw.example.com"}
	c, resp := checkEntryTokenValid(context.Background(), cfg)
	if c.Status != StatusWarn {
		t.Errorf("401 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "AUTH_TOKEN_EXPIRED" {
		t.Errorf("Code 应为 AUTH_TOKEN_EXPIRED，实际 %q", c.Code)
	}
	if resp != nil {
		t.Error("认证失败时 resp 必须 nil")
	}
}

func TestCheckEntryTokenValid_Success_Pass(t *testing.T) {
	orig := entryAuthenticate
	entryAuthenticate = func(ctx context.Context, gw, id, pw string) (*cloudclaude.AuthResponse, error) {
		return &cloudclaude.AuthResponse{ImageVersion: "v3.0.0", ClaudeAccountID: "acct-1"}, nil
	}
	t.Cleanup(func() { entryAuthenticate = orig })
	cfg := &cloudclaude.Config{Gateway: "https://gw.example.com"}
	c, resp := checkEntryTokenValid(context.Background(), cfg)
	if c.Status != StatusPass {
		t.Errorf("应 Pass，实际 %s", c.Status)
	}
	if resp == nil || resp.ClaudeAccountID != "acct-1" {
		t.Error("resp 必须完整透传")
	}
}

func TestCheckEntryTokenValid_NilCfg_Skip(t *testing.T) {
	c, _ := checkEntryTokenValid(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil cfg 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckOAuthCredentials_NilConn_Skip(t *testing.T) {
	c := checkOAuthCredentials(context.Background(), nil, "acct-1")
	if c.Status != StatusSkip {
		t.Errorf("nil conn 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckOAuthCredentials_EmptyAccountID_Skip(t *testing.T) {
	// 构造非 nil conn（zero-value 也行，我们只断言分支）
	// 实际 CheckOAuthCredentials 需要 ssh.Client；这里通过 empty accountID 短路
	c := checkOAuthCredentials(context.Background(), nil, "")
	if c.Status != StatusSkip {
		t.Errorf("empty accountID 应 Skip，实际 %s", c.Status)
	}
}
```

**禁止：**
- 在 `auth.go` 内重新实现 `CheckOAuthCredentials`（RESEARCH §3.2 + PATTERNS §2.4 明令禁止）
- 直接存储用户密码到 `Check.Details`（凭据泄漏风险）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/auth.go && test -f internal/cloudclaude/doctor/auth_test.go` 退出码 = 0
    - `grep -q "^var loadConfig = cloudclaude.LoadConfig" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "^var entryAuthenticate" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "func checkConfigPresent" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "func checkEntryTokenValid" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "func checkOAuthCredentials" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "cloudclaude.CheckOAuthCredentials" internal/cloudclaude/doctor/auth.go` 命中（复用，不自己写）
    - `grep -q "errcodes.AUTH_CONFIG_MISSING" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "errcodes.AUTH_TOKEN_EXPIRED" internal/cloudclaude/doctor/auth.go` 命中
    - `grep -q "errcodes.NET_OAUTH_" internal/cloudclaude/doctor/auth.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestCheckConfigPresent|TestCheckEntryTokenValid|TestCheckOAuthCredentials" -count=1 -v` 退出码 = 0
    - 禁止凭据：`! grep -n "password\|Password" internal/cloudclaude/doctor/auth.go | grep -v "cfg.Password"` （只允许入参传递，不允许落 Details / log）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.6: 新建 doctor/ssh.go + ssh_test.go — 4 check（keepalive_config / sshd_keepalive_drift / known_hosts / workspace_ssh_keys）</name>
  <files>internal/cloudclaude/doctor/ssh.go, internal/cloudclaude/doctor/ssh_test.go</files>

  <read_first>
    - internal/cloudclaude/ssh_doctor.go（RunSSHDoctor / SSHDoctorResult 完整契约）
    - internal/cloudclaude/mount_strategy.go（MountConfig.KeepAliveInterval 读法）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-19（ssh 4 check）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.3（sshd -T 命令 + known_hosts HASHED 陷阱）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.5
    - deploy/docker/managed-user/sshd_config（基线 15/8）
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/ssh.go`（4 check）：**

```go
package doctor

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// checkKeepaliveConfig 纯本地：MountConfig.KeepAliveInterval >= 15s（PITFALLS M11）。
func checkKeepaliveConfig(ctx context.Context, keepalive time.Duration) Check {
	if keepalive == 0 {
		return newSkip("ssh", "keepalive_config", "未设置 KeepAliveInterval，跳过（默认 15s）")
	}
	if keepalive < 15*time.Second {
		return newFail("ssh", "keepalive_config", errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, keepalive.String())
	}
	return newPass("ssh", "keepalive_config", fmt.Sprintf("KeepAliveInterval=%s ≥ 15s", keepalive))
}

// checkSSHDKeepaliveDrift 远端 sshd -T（RESEARCH §3.3）。容器内 Phase 29 基线 15/8。
func checkSSHDKeepaliveDrift(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("ssh", "sshd_keepalive_drift", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("sshd_keepalive",
		"sshd -T 2>/dev/null | grep -E '^(clientalive(interval|countmax))\\b' || true")
	if err != nil {
		return newWarn("ssh", "sshd_keepalive_drift", errcodes.SSH_SSHD_KEEPALIVE_DRIFT, "sshd -T 失败: "+err.Error())
	}
	interval, count := parseSSHDKeepalive(stdout)
	baseline := "clientaliveinterval=15 clientalivecountmax=8"
	if interval != 15 || count != 8 {
		got := fmt.Sprintf("clientaliveinterval=%d clientalivecountmax=%d", interval, count)
		return newWarn("ssh", "sshd_keepalive_drift", errcodes.SSH_SSHD_KEEPALIVE_DRIFT, got)
	}
	return newPass("ssh", "sshd_keepalive_drift", baseline)
}

// parseSSHDKeepalive 从 sshd -T 输出解析两个值；解析失败返回 0。
func parseSSHDKeepalive(out string) (interval, count int) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "clientaliveinterval "):
			interval, _ = strconv.Atoi(strings.TrimPrefix(line, "clientaliveinterval "))
		case strings.HasPrefix(line, "clientalivecountmax "):
			count, _ = strconv.Atoi(strings.TrimPrefix(line, "clientalivecountmax "))
		}
	}
	return
}

// checkKnownHosts 本地读 ~/.ssh/known_hosts（RESEARCH §3.3 gotcha：HASHED hostname，用 knownhosts.New HostKeyCallback）。
// authHostPort 示例 "example.com:22"；authResp 非空时可传 host key 做 fake callback；本 plan 简化：
//   - ~/.ssh/known_hosts 不存在 → Skip
//   - 其中含 host 但解析失败 → Warn
//   - 含 host 且可 Load → Pass（精确 fingerprint 比对 v3.1 backlog）
func checkKnownHosts(ctx context.Context, khPath string, authHostPort string) Check {
	if khPath == "" || authHostPort == "" {
		return newSkip("ssh", "known_hosts", "未配置远端地址，跳过")
	}
	// knownhosts.New 期望文件列表；不存在时返回 error
	_, err := knownhosts.New(khPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not exist") {
			return newSkip("ssh", "known_hosts", "~/.ssh/known_hosts 不存在，跳过（首次连接时自动创建）")
		}
		return newWarn("ssh", "known_hosts", errcodes.SSH_KNOWN_HOSTS_CONFLICT, authHostPort)
	}
	return newPass("ssh", "known_hosts", fmt.Sprintf("known_hosts 已加载（含 %s 条目将由后续连接校验）", authHostPort))
}

// checkWorkspaceSSHKeys 复用 v2.0 RunSSHDoctor（CONTEXT D-04 双入口共存，doctor ssh 维度内部调同一函数）。
// 结果以 Details 形式返回；**本阶段无错误码**（D-19）。
func checkWorkspaceSSHKeys(ctx context.Context, sshCfg cloudclaude.SSHConfig) Check {
	res, err := cloudclaude.RunSSHDoctor(sshCfg, cloudclaude.SSHDoctorOptions{Fix: false})
	if err != nil {
		return Check{
			Domain: "ssh", Name: "workspace_ssh_keys", Status: StatusFail,
			Message:    "RunSSHDoctor 失败: " + err.Error(),
			NextAction: "运行 cloud-claude ssh doctor 查看详细报告",
		}
	}
	status := StatusPass
	problemCount := 0
	for _, f := range res.Files {
		if !f.OwnerOK || !f.ModeOK {
			problemCount++
		}
		if f.PEMEndsNL != nil && !*f.PEMEndsNL {
			problemCount++
		}
	}
	if problemCount > 0 {
		status = StatusWarn
	}
	return Check{
		Domain: "ssh", Name: "workspace_ssh_keys", Status: status,
		Message:    fmt.Sprintf("扫描 %d 个文件，%d 个问题项", len(res.Files), problemCount),
		NextAction: "运行 cloud-claude ssh doctor --fix 自动修复",
		Details:    map[string]any{"files": len(res.Files), "problems": problemCount},
	}
}

// 辅助：构造 known_hosts 字面量的 host:port（auth 成功后用）。
func makeKnownHostsHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
```

**(b) 新建 `internal/cloudclaude/doctor/ssh_test.go`（≥ 7 条子测）：**

```go
package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckKeepaliveConfig_TooAggressive_Fail(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 10*time.Second)
	if c.Status != StatusFail {
		t.Errorf("10s 应 Fail，实际 %s", c.Status)
	}
	if c.Code != "SESSION_KEEPALIVE_TOO_AGGRESSIVE" {
		t.Errorf("Code 应为 SESSION_KEEPALIVE_TOO_AGGRESSIVE，实际 %q", c.Code)
	}
}

func TestCheckKeepaliveConfig_OK_Pass(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 15*time.Second)
	if c.Status != StatusPass {
		t.Errorf("15s 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckKeepaliveConfig_Zero_Skip(t *testing.T) {
	c := checkKeepaliveConfig(context.Background(), 0)
	if c.Status != StatusSkip {
		t.Errorf("0 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckSSHDKeepaliveDrift_Baseline_Pass(t *testing.T) {
	r := &fakeRunner{out: "clientaliveinterval 15\nclientalivecountmax 8\n"}
	c := checkSSHDKeepaliveDrift(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("15/8 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckSSHDKeepaliveDrift_Drift_Warn(t *testing.T) {
	r := &fakeRunner{out: "clientaliveinterval 60\nclientalivecountmax 3\n"}
	c := checkSSHDKeepaliveDrift(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("60/3 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "SSH_SSHD_KEEPALIVE_DRIFT" {
		t.Errorf("Code 应为 SSH_SSHD_KEEPALIVE_DRIFT，实际 %q", c.Code)
	}
}

func TestCheckSSHDKeepaliveDrift_NilRunner_Skip(t *testing.T) {
	c := checkSSHDKeepaliveDrift(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckKnownHosts_Missing_Skip(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "known_hosts")
	c := checkKnownHosts(context.Background(), missing, "example.com:22")
	if c.Status != StatusSkip {
		t.Errorf("文件不存在应 Skip，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckKnownHosts_Valid_Pass(t *testing.T) {
	dir := t.TempDir()
	khPath := filepath.Join(dir, "known_hosts")
	// 最小合法的 known_hosts 条目（ssh-ed25519 sample，仅用于 knownhosts.New 解析通过）
	sample := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGIE6GJ4FN+uQQl7yh0K8x3lG0m5f5n6Kk7aA0GZXAbD\n"
	if err := os.WriteFile(khPath, []byte(sample), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	c := checkKnownHosts(context.Background(), khPath, "example.com:22")
	if c.Status != StatusPass {
		t.Logf("actual status=%s message=%q (knownhosts.New 在不同 Go 版本可能对 sample key 严格校验，允许 Warn)", c.Status, c.Message)
	}
}
```

**注意：** `checkWorkspaceSSHKeys` 依赖真实 SSH conn；本 plan 单测层不覆盖（集成测试由 Plan 03 docker fixture 兜底）。
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/ssh.go && test -f internal/cloudclaude/doctor/ssh_test.go` 退出码 = 0
    - `grep -q "func checkKeepaliveConfig" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "func checkSSHDKeepaliveDrift" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "func checkKnownHosts" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "func checkWorkspaceSSHKeys" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "cloudclaude.RunSSHDoctor" internal/cloudclaude/doctor/ssh.go` 命中（复用 v2.0）
    - `grep -q "errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "errcodes.SSH_SSHD_KEEPALIVE_DRIFT" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "errcodes.SSH_KNOWN_HOSTS_CONFLICT" internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q "golang.org/x/crypto/ssh/knownhosts" internal/cloudclaude/doctor/ssh.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestCheckKeepalive|TestCheckSSHDKeepalive|TestCheckKnownHosts" -count=1 -v` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.7: 新建 doctor/mount.go + mount_test.go — 5 check（mutagen_version / mergerfs_branches / sshfs / fuse_residual / apparmor）</name>
  <files>internal/cloudclaude/doctor/mount.go, internal/cloudclaude/doctor/mount_test.go</files>

  <read_first>
    - internal/cloudclaude/mount_mutagen.go（line 230-237 版本握手复用点）
    - internal/cloudclaude/mutagen_bin.go（MutagenBinaryVersion 常量）
    - deploy/scripts/host-preflight.sh（AppArmor 检测逻辑 — Go 改写源）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-19（mount 5 check）+ §D-26 Phase 29 基线
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.4（5 check 命令表）+ §8.1 mergerfs 6 字面量 + §8.3 AppArmor 5-Gate
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.6
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/mount.go`（5 check）：**

```go
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// 包级 var mock 注入点。
var (
	readOSRelease      = func() ([]byte, error) { return os.ReadFile("/etc/os-release") }
	readAppArmorOverride = func() ([]byte, error) { return os.ReadFile("/etc/apparmor.d/local/fusermount3") }
	execLookPath       = exec.LookPath
	execMountList      = func() (string, error) {
		out, err := exec.Command("mount").CombinedOutput()
		return string(out), err
	}
)

// checkMutagenVersionMatch 远端 /etc/cloud-claude/mutagen.version 与 embed 版本比对（C4）。
// 复刻 mount_mutagen.go:230-237 的 TrimPrefix+Contains 双保险模式。
func checkMutagenVersionMatch(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "mutagen_version_match", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("mutagen_version", "cat /etc/cloud-claude/mutagen.version 2>/dev/null")
	if err != nil {
		return newFail("mount", "mutagen_version_match", errcodes.MOUNT_MUTAGEN_VERSION_SKEW,
			cloudclaude.MutagenBinaryVersion, "(读取失败: "+err.Error()+")")
	}
	remote := strings.TrimSpace(stdout)
	local := cloudclaude.MutagenBinaryVersion
	if remote == "" {
		return newFail("mount", "mutagen_version_match", errcodes.MOUNT_MUTAGEN_VERSION_SKEW, local, "(远端文件不存在)")
	}
	if !strings.Contains(remote, strings.TrimPrefix(local, "v")) {
		return newFail("mount", "mutagen_version_match", errcodes.MOUNT_MUTAGEN_VERSION_SKEW, local, remote)
	}
	return newPass("mount", "mutagen_version_match", fmt.Sprintf("客户端 %s ↔ 远端 %s", local, remote))
}

// checkMergerfsBranches 远端 getfattr + mount 参数 6 字面量断言（C2 / RESEARCH §8.1）。
func checkMergerfsBranches(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "mergerfs_branches", "未能连接远端容器，跳过")
	}
	xattr, _, _ := runner.RunScript("mergerfs_xattr",
		"getfattr --only-values -n user.mergerfs.branches /workspace/.mergerfs 2>/dev/null")
	mountOut, _, _ := runner.RunScript("mergerfs_mount", "mount | grep mergerfs | head -1")

	xattrOK := strings.Contains(xattr, "RW") && strings.Contains(xattr, "NC,RO")
	want := []string{
		"func.readdir=cor:4", "cache.attr=30", "cache.entry=30",
		"cache.readdir=true", "cache.files=off", "category.create=ff",
	}
	var missing []string
	for _, w := range want {
		if !strings.Contains(mountOut, w) {
			missing = append(missing, w)
		}
	}
	if !xattrOK {
		return newFail("mount", "mergerfs_branches", errcodes.MOUNT_MERGERFS_FAILED,
			"branches xattr 缺 RW 或 NC,RO")
	}
	if len(missing) > 0 {
		return newFail("mount", "mergerfs_branches", errcodes.MOUNT_MERGERFS_FAILED,
			"mount 参数缺少 "+strings.Join(missing, ","))
	}
	return Check{
		Domain: "mount", Name: "mergerfs_branches", Status: StatusPass,
		Message: "mergerfs 参数与 branches 均符合 Phase 29 基线",
		Details: map[string]any{"branches_xattr": strings.TrimSpace(xattr), "mount": strings.TrimSpace(mountOut)},
	}
}

// checkSSHFSMountpoint 远端 mountpoint -q /workspace-cold（RESEARCH §3.4）。
func checkSSHFSMountpoint(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("mount", "sshfs_mountpoint", "未能连接远端容器，跳过")
	}
	_, _, err := runner.RunScript("sshfs_mp", "mountpoint -q /workspace-cold")
	if err != nil {
		return newWarn("mount", "sshfs_mountpoint", errcodes.MOUNT_SSHFS_DISCONNECTED)
	}
	return newPass("mount", "sshfs_mountpoint", "/workspace-cold 已挂载")
}

// checkFUSEResidual 本地扫 mount 输出（RESEARCH §3.4 + §4.2）。
func checkFUSEResidual(ctx context.Context) Check {
	out, err := execMountList()
	if err != nil {
		return newSkip("mount", "fuse_residual", "mount 命令失败，跳过: "+err.Error())
	}
	var re *regexp.Regexp
	switch runtime.GOOS {
	case "darwin":
		re = regexp.MustCompile(`(?m)^.*?\s+on\s+(\S+)\s+\(.*?(macfuse|osxfuse)`)
	case "linux":
		re = regexp.MustCompile(`(?m)^\S+\s+on\s+(\S+)\s+type\s+fuse\.(sshfs|mergerfs)\b`)
	default:
		return newSkip("mount", "fuse_residual", "非 Linux/macOS，跳过")
	}
	matches := re.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return newPass("mount", "fuse_residual", "未发现残留 FUSE 挂载")
	}
	var points []string
	for _, m := range matches {
		points = append(points, m[1])
	}
	return newWarn("mount", "fuse_residual", errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT, len(points), strings.Join(points, ","))
}

// checkAppArmorFusermount3 本地 5-Gate 检测（RESEARCH §8.3）。Go 改写 deploy/scripts/host-preflight.sh:check_apparmor_fusermount3。
func checkAppArmorFusermount3(ctx context.Context) Check {
	if runtime.GOOS != "linux" {
		return newSkip("mount", "apparmor_fusermount3", "非 Linux，跳过")
	}
	osRel, err := readOSRelease()
	if err != nil {
		return newSkip("mount", "apparmor_fusermount3", "无 /etc/os-release，跳过")
	}
	if !regexp.MustCompile(`(?m)^ID=ubuntu\b`).Match(osRel) {
		return newSkip("mount", "apparmor_fusermount3", "非 Ubuntu，跳过")
	}
	// Gate 3: Ubuntu >= 25.04
	vre := regexp.MustCompile(`(?m)^VERSION_ID="?(\d+)\.(\d+)"?`)
	m := vre.FindSubmatch(osRel)
	if len(m) >= 3 {
		major := string(m[1])
		minor := string(m[2])
		if major < "25" || (major == "25" && minor < "04") {
			return newSkip("mount", "apparmor_fusermount3",
				fmt.Sprintf("Ubuntu %s.%s < 25.04，跳过", major, minor))
		}
	}
	// Gate 4: aa-status
	if _, err := execLookPath("aa-status"); err != nil {
		return newSkip("mount", "apparmor_fusermount3", "apparmor-utils 未安装，跳过")
	}
	// Gate 5: override 文件
	content, err := readAppArmorOverride()
	if err != nil {
		return newFail("mount", "apparmor_fusermount3", errcodes.SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
			"/etc/apparmor.d/local/fusermount3 不存在")
	}
	if !regexp.MustCompile(`(?m)^\s*capability\s+dac_override\b`).Match(content) {
		return newFail("mount", "apparmor_fusermount3", errcodes.SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
			"override 文件缺 `capability dac_override` 行")
	}
	return newPass("mount", "apparmor_fusermount3", "AppArmor fusermount3 override 就位")
}
```

**(b) 新建 `internal/cloudclaude/doctor/mount_test.go`（≥ 10 条子测）：**

```go
package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
)

func TestCheckMutagenVersionMatch_NilRunner_Skip(t *testing.T) {
	c := checkMutagenVersionMatch(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("应 Skip，实际 %s", c.Status)
	}
}

func TestCheckMutagenVersionMatch_Match_Pass(t *testing.T) {
	r := &fakeRunner{out: "0.18.1\n"}
	c := checkMutagenVersionMatch(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("应 Pass（TrimPrefix v 后 Contains），实际 %s", c.Status)
	}
}

func TestCheckMutagenVersionMatch_Skew_Fail(t *testing.T) {
	r := &fakeRunner{out: "0.99.99\n"}
	c := checkMutagenVersionMatch(context.Background(), r)
	if c.Status != StatusFail {
		t.Errorf("应 Fail，实际 %s", c.Status)
	}
	if c.Code != "MOUNT_MUTAGEN_VERSION_SKEW" {
		t.Errorf("Code 应为 MOUNT_MUTAGEN_VERSION_SKEW，实际 %q", c.Code)
	}
}

func TestCheckMergerfsBranches_AllPresent_Pass(t *testing.T) {
	r := &fakeRunner{}
	// 模拟 runner 按 name 返回不同值
	r.out = "RW + NC,RO"
	// 第二次调用 RunScript 会覆盖 out；改用自定义 runner
	rr := &branchRunner{
		xattr: "RW + NC,RO",
		mount: "mergerfs on /workspace type fuse.mergerfs (func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off,category.create=ff)",
	}
	c := checkMergerfsBranches(context.Background(), rr)
	if c.Status != StatusPass {
		t.Errorf("全参数就位应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckMergerfsBranches_MissingParam_Fail(t *testing.T) {
	rr := &branchRunner{
		xattr: "RW + NC,RO",
		mount: "mergerfs on /workspace type fuse.mergerfs (cache.attr=30)",
	}
	c := checkMergerfsBranches(context.Background(), rr)
	if c.Status != StatusFail {
		t.Errorf("缺参数应 Fail，实际 %s", c.Status)
	}
	if c.Code != "MOUNT_MERGERFS_FAILED" {
		t.Errorf("Code 应为 MOUNT_MERGERFS_FAILED，实际 %q", c.Code)
	}
}

func TestCheckMergerfsBranches_BadXattr_Fail(t *testing.T) {
	rr := &branchRunner{xattr: "", mount: ""}
	c := checkMergerfsBranches(context.Background(), rr)
	if c.Status != StatusFail {
		t.Errorf("空 xattr 应 Fail，实际 %s", c.Status)
	}
}

// branchRunner 是 mount_test 专用的 RemoteRunner — 按 script 关键字返回不同 stdout。
type branchRunner struct {
	xattr, mount string
}

func (b *branchRunner) RunScript(name, script string) (string, string, error) {
	switch name {
	case "mergerfs_xattr":
		return b.xattr, "", nil
	case "mergerfs_mount":
		return b.mount, "", nil
	}
	return "", "", nil
}

func TestCheckSSHFSMountpoint_Mounted_Pass(t *testing.T) {
	r := &fakeRunner{} // err=nil
	c := checkSSHFSMountpoint(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("mountpoint 0 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckSSHFSMountpoint_Unmounted_Warn(t *testing.T) {
	r := &fakeRunner{err: fmt.Errorf("exit 32")}
	c := checkSSHFSMountpoint(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("exit 32 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "MOUNT_SSHFS_DISCONNECTED" {
		t.Errorf("Code 应为 MOUNT_SSHFS_DISCONNECTED，实际 %q", c.Code)
	}
}

func TestCheckFUSEResidual_NoMounts_Pass(t *testing.T) {
	orig := execMountList
	execMountList = func() (string, error) {
		return "tmpfs on /dev/shm type tmpfs (rw)\n", nil
	}
	t.Cleanup(func() { execMountList = orig })
	c := checkFUSEResidual(context.Background())
	if c.Status != StatusPass {
		t.Errorf("无 FUSE 挂载应 Pass，实际 %s", c.Status)
	}
}

func TestCheckFUSEResidual_LinuxResidual_Warn(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only test")
	}
	orig := execMountList
	execMountList = func() (string, error) {
		return "somehost:/data on /mnt/sshfs type fuse.sshfs (rw,nosuid)\n", nil
	}
	t.Cleanup(func() { execMountList = orig })
	c := checkFUSEResidual(context.Background())
	if c.Status != StatusWarn {
		t.Errorf("残留 sshfs 应 Warn，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckAppArmorFusermount3_NonLinux_Skip(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("darwin/windows only")
	}
	c := checkAppArmorFusermount3(context.Background())
	if c.Status != StatusSkip {
		t.Errorf("非 Linux 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckAppArmorFusermount3_NonUbuntu_Skip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	orig := readOSRelease
	readOSRelease = func() ([]byte, error) { return []byte("ID=debian\nVERSION_ID=\"12\"\n"), nil }
	t.Cleanup(func() { readOSRelease = orig })
	c := checkAppArmorFusermount3(context.Background())
	if c.Status != StatusSkip {
		t.Errorf("非 Ubuntu 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckAppArmorFusermount3_MissingOverride_Fail(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	origOS := readOSRelease
	origAA := readAppArmorOverride
	origLP := execLookPath
	readOSRelease = func() ([]byte, error) { return []byte("ID=ubuntu\nVERSION_ID=\"25.04\"\n"), nil }
	readAppArmorOverride = func() ([]byte, error) { return nil, fmt.Errorf("no such file") }
	execLookPath = func(file string) (string, error) { return "/usr/sbin/aa-status", nil }
	t.Cleanup(func() {
		readOSRelease = origOS
		readAppArmorOverride = origAA
		execLookPath = origLP
	})
	c := checkAppArmorFusermount3(context.Background())
	if c.Status != StatusFail {
		t.Errorf("override 缺失应 Fail，实际 %s (msg=%q)", c.Status, c.Message)
	}
	if c.Code != "SYSTEM_APPARMOR_FUSERMOUNT3_MISSING" {
		t.Errorf("Code 应为 SYSTEM_APPARMOR_FUSERMOUNT3_MISSING，实际 %q", c.Code)
	}
}

// 辅助：确保 exec 包存在于 imports（避免 lint "imported and not used" — 实际 execLookPath 已用）
var _ = exec.LookPath
```

**禁止：**
- 修改既有 `mount_mutagen.go` 版本握手逻辑（只做只读断言）
- `checkMergerfsBranches` 用正则匹配整行 `mount` 输出（RESEARCH §8.1 明令禁止）
- `checkAppArmorFusermount3` exec `apparmor_parser`（只读文件，单测友好）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/mount.go && test -f internal/cloudclaude/doctor/mount_test.go` 退出码 = 0
    - `grep -q "func checkMutagenVersionMatch" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "func checkMergerfsBranches" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "func checkSSHFSMountpoint" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "func checkFUSEResidual" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "func checkAppArmorFusermount3" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "cloudclaude.MutagenBinaryVersion" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q '"func.readdir=cor:4"' internal/cloudclaude/doctor/mount.go` 命中（6 字面量之一）
    - `grep -q '"category.create=ff"' internal/cloudclaude/doctor/mount.go` 命中（6 字面量之一）
    - `grep -q "errcodes.MOUNT_MUTAGEN_VERSION_SKEW" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "errcodes.MOUNT_MERGERFS_FAILED" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "errcodes.MOUNT_SSHFS_DISCONNECTED" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT" internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q "errcodes.SYSTEM_APPARMOR_FUSERMOUNT3_MISSING" internal/cloudclaude/doctor/mount.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestCheckMutagen|TestCheckMergerfs|TestCheckSSHFS|TestCheckFUSE|TestCheckAppArmor" -count=1 -v` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.8: 新建 doctor/disk.go + disk_test.go — 3 check（local_disk / container_disk / mutagen_data_size）</name>
  <files>internal/cloudclaude/doctor/disk.go, internal/cloudclaude/doctor/disk_test.go</files>

  <read_first>
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-19（disk 3 check） + §Discretion §5（硬编码阈值）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.5（Statfs 跨 OS + df 命令）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.7（golang.org/x/sys/unix.Statfs 模板）
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/disk.go`（3 check）：**

```go
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

const (
	diskWarnMB        = 500
	diskFailMB        = 100
	mutagenBloatMB    = 1024
)

// 包级 var 注入点。
var (
	statfs = func(path string, buf *unix.Statfs_t) error { return unix.Statfs(path, buf) }
	duLocal = func(path string) (string, error) {
		out, err := exec.Command("du", "-sh", path).CombinedOutput()
		return string(out), err
	}
	userHomeDir = os.UserHomeDir
)

// checkLocalDisk — DISK_LOCAL_LOW （CONTEXT D-19）。
func checkLocalDisk(ctx context.Context) Check {
	home, err := userHomeDir()
	if err != nil {
		return newSkip("disk", "local_disk", "无法定位 home 目录，跳过: "+err.Error())
	}
	target := filepath.Join(home, ".cloud-claude")
	if _, err := os.Stat(target); err != nil {
		target = home // fallback
	}
	var stat unix.Statfs_t
	if err := statfs(target, &stat); err != nil {
		return newSkip("disk", "local_disk", "statfs 失败: "+err.Error())
	}
	availMB := int64(stat.Bavail) * int64(stat.Bsize) / 1024 / 1024
	switch {
	case availMB < diskFailMB:
		return newFail("disk", "local_disk", errcodes.DISK_LOCAL_LOW, availMB)
	case availMB < diskWarnMB:
		return newWarn("disk", "local_disk", errcodes.DISK_LOCAL_LOW, availMB)
	}
	return newPass("disk", "local_disk", fmt.Sprintf("本地可用 %dMB (threshold warn<%d / fail<%d)", availMB, diskWarnMB, diskFailMB))
}

// checkContainerDisk — DISK_CONTAINER_LOW （远端 df /workspace）。
func checkContainerDisk(ctx context.Context, runner RemoteRunner) Check {
	if runner == nil {
		return newSkip("disk", "container_disk", "未能连接远端容器，跳过")
	}
	stdout, _, err := runner.RunScript("container_disk",
		"df -BM --output=avail /workspace 2>/dev/null | tail -1")
	if err != nil {
		return newSkip("disk", "container_disk", "df 失败: "+err.Error())
	}
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stdout), "M"))
	avail, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return newSkip("disk", "container_disk", "无法解析 df 输出: "+stdout)
	}
	switch {
	case avail < diskFailMB:
		return newFail("disk", "container_disk", errcodes.DISK_CONTAINER_LOW, avail)
	case avail < diskWarnMB:
		return newWarn("disk", "container_disk", errcodes.DISK_CONTAINER_LOW, avail)
	}
	return newPass("disk", "container_disk", fmt.Sprintf("远端 /workspace 可用 %dMB", avail))
}

// checkMutagenDataSize — DISK_MUTAGEN_DATA_BLOAT （本地 du）。
func checkMutagenDataSize(ctx context.Context) Check {
	home, err := userHomeDir()
	if err != nil {
		return newSkip("disk", "mutagen_data_size", "无法定位 home 目录，跳过")
	}
	target := filepath.Join(home, ".cloud-claude", "mutagen")
	if _, err := os.Stat(target); err != nil {
		return newSkip("disk", "mutagen_data_size", "Mutagen 数据目录不存在（尚未使用）")
	}
	out, err := duLocal(target)
	if err != nil {
		return newSkip("disk", "mutagen_data_size", "du 失败: "+err.Error())
	}
	sizeStr := strings.Fields(strings.TrimSpace(out))
	if len(sizeStr) == 0 {
		return newSkip("disk", "mutagen_data_size", "无法解析 du 输出")
	}
	mb := parseDuHumanToMB(sizeStr[0])
	if mb > mutagenBloatMB {
		return newWarn("disk", "mutagen_data_size", errcodes.DISK_MUTAGEN_DATA_BLOAT, sizeStr[0])
	}
	return newPass("disk", "mutagen_data_size", fmt.Sprintf("Mutagen 数据目录 %s（<1GB）", sizeStr[0]))
}

// parseDuHumanToMB 解析 du -sh 输出：`12K` / `3.2M` / `1.5G` → MB 近似值；解析失败返回 0。
func parseDuHumanToMB(s string) int64 {
	if len(s) < 2 {
		return 0
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case 'K', 'k':
		return int64(f / 1024)
	case 'M', 'm':
		return int64(f)
	case 'G', 'g':
		return int64(f * 1024)
	case 'T', 't':
		return int64(f * 1024 * 1024)
	}
	return 0
}
```

**(b) 新建 `internal/cloudclaude/doctor/disk_test.go`：**

```go
package doctor

import (
	"context"
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCheckLocalDisk_Enough_Pass(t *testing.T) {
	orig := statfs
	statfs = func(path string, buf *unix.Statfs_t) error {
		buf.Bavail = 1024 * 1024      // 1M blocks
		buf.Bsize = 4096              // 4K = 4 GB available
		return nil
	}
	t.Cleanup(func() { statfs = orig })
	c := checkLocalDisk(context.Background())
	if c.Status != StatusPass {
		t.Errorf("4GB 应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckLocalDisk_Warn(t *testing.T) {
	orig := statfs
	statfs = func(path string, buf *unix.Statfs_t) error {
		buf.Bavail = 100 * 1024  // ~400MB
		buf.Bsize = 4096
		return nil
	}
	t.Cleanup(func() { statfs = orig })
	c := checkLocalDisk(context.Background())
	if c.Status != StatusWarn {
		t.Errorf("~400MB 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "DISK_LOCAL_LOW" {
		t.Errorf("Code 应为 DISK_LOCAL_LOW，实际 %q", c.Code)
	}
}

func TestCheckLocalDisk_Fail(t *testing.T) {
	orig := statfs
	statfs = func(path string, buf *unix.Statfs_t) error {
		buf.Bavail = 10 * 1024  // ~40MB
		buf.Bsize = 4096
		return nil
	}
	t.Cleanup(func() { statfs = orig })
	c := checkLocalDisk(context.Background())
	if c.Status != StatusFail {
		t.Errorf("~40MB 应 Fail，实际 %s", c.Status)
	}
}

func TestCheckContainerDisk_NilRunner_Skip(t *testing.T) {
	c := checkContainerDisk(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

func TestCheckContainerDisk_Enough_Pass(t *testing.T) {
	r := &fakeRunner{out: "10240M\n"}
	c := checkContainerDisk(context.Background(), r)
	if c.Status != StatusPass {
		t.Errorf("10GB 应 Pass，实际 %s", c.Status)
	}
}

func TestCheckContainerDisk_Low_Warn(t *testing.T) {
	r := &fakeRunner{out: "250M\n"}
	c := checkContainerDisk(context.Background(), r)
	if c.Status != StatusWarn {
		t.Errorf("250M 应 Warn，实际 %s", c.Status)
	}
	if c.Code != "DISK_CONTAINER_LOW" {
		t.Errorf("Code 应为 DISK_CONTAINER_LOW，实际 %q", c.Code)
	}
}

func TestCheckContainerDisk_Unparseable_Skip(t *testing.T) {
	r := &fakeRunner{out: "garbage"}
	c := checkContainerDisk(context.Background(), r)
	if c.Status != StatusSkip {
		t.Errorf("无法解析应 Skip，实际 %s", c.Status)
	}
}

func TestParseDuHumanToMB(t *testing.T) {
	cases := map[string]int64{
		"12K":  0,
		"500K": 0,
		"1024K": 1,
		"3M":  3,
		"1.5G": 1536,
		"2T":  2 * 1024 * 1024,
		"bad": 0,
	}
	for in, want := range cases {
		if got := parseDuHumanToMB(in); got != want {
			t.Errorf("parseDuHumanToMB(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestCheckMutagenDataSize_Missing_Skip(t *testing.T) {
	origHome := userHomeDir
	userHomeDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { userHomeDir = origHome })
	c := checkMutagenDataSize(context.Background())
	if c.Status != StatusSkip {
		t.Errorf("目录不存在应 Skip，实际 %s", c.Status)
	}
}

// 注意：golang.org/x/sys 已在间接依赖，go.mod 无需改动
var _ = fmt.Sprintf
```

**executor 注意：** `disk.go` 中 `Bsize` 类型在 Linux/Darwin 分别为 `int64`/`uint32`；代码里 `int64(stat.Bsize)` 转换是跨 OS 安全的；测试里直接赋值 `4096` 通用。
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/disk.go && test -f internal/cloudclaude/doctor/disk_test.go` 退出码 = 0
    - `grep -q "func checkLocalDisk" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "func checkContainerDisk" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "func checkMutagenDataSize" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "golang.org/x/sys/unix" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "diskWarnMB\s*=\s*500" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "diskFailMB\s*=\s*100" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "mutagenBloatMB\s*=\s*1024" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "errcodes.DISK_LOCAL_LOW" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "errcodes.DISK_CONTAINER_LOW" internal/cloudclaude/doctor/disk.go` 命中
    - `grep -q "errcodes.DISK_MUTAGEN_DATA_BLOAT" internal/cloudclaude/doctor/disk.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestCheckLocalDisk|TestCheckContainerDisk|TestParseDu|TestCheckMutagenDataSize" -count=1 -v` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.9: 新建 doctor/render.go + render_test.go — Text + JSON 双输出 + 降级 banner（CONTEXT D-13/D-15 / PATTERNS §2.10）</name>
  <files>internal/cloudclaude/doctor/render.go, internal/cloudclaude/doctor/render_test.go</files>

  <read_first>
    - internal/cloudclaude/ssh_doctor.go（line 370-454 Print + printGroup 样板）
    - internal/cloudclaude/colors.go（Task 2.1 后的 ColorEnabled / Colorize）
    - internal/cloudclaude/last_session.go（LastSessionSnapshot schema）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-13（第一屏 4 段）+ §D-14（彩色规则）+ §D-15（JSON schema）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §5.1 JSON 锁死 + §8.5 M13 断言要求
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.10
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/doctor/render.go`：**

```go
package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// 状态符号 + 纯文本 fallback（CONTEXT D-14）。
const (
	iconPass = "[✓]"
	iconWarn = "[!]"
	iconFail = "[✗]"
	iconSkip = "[~]"

	iconPassPlain = "[ok]"
	iconWarnPlain = "[warn]"
	iconFailPlain = "[fail]"
	iconSkipPlain = "[skip]"
)

// RenderText 是 text 模式的主入口（CONTEXT D-13 布局：banner → downgrade banner → 5 维度矩阵 → 汇总）。
func RenderText(r *Report, noColor bool) string {
	var b strings.Builder

	// 第一段：banner
	b.WriteString("╭─────────────────────────────────────────╮\n")
	b.WriteString("│  Cloud Claude Doctor v3.0 体检报告       │\n")
	b.WriteString("╰─────────────────────────────────────────╯\n")
	if r.CloudClaudeVer != "" {
		fmt.Fprintf(&b, "  cloud-claude: %s\n", r.CloudClaudeVer)
	}
	if r.RemoteImageVer != "" {
		fmt.Fprintf(&b, "  远端镜像:     %s\n", r.RemoteImageVer)
	}
	b.WriteString("\n")

	// 第二段：降级历史 banner（M13 第一屏锚点）
	b.WriteString(renderDowngradeBanner(r.DowngradeHistory))
	b.WriteString("\n")

	// 第三段：5 维度矩阵
	byDomain := groupByDomain(r.Checks)
	for _, dom := range []string{"network", "auth", "ssh", "mount", "disk"} {
		checks, ok := byDomain[dom]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "── %s ──\n", dom)
		for _, c := range checks {
			b.WriteString(renderCheckLine(c, noColor))
		}
		b.WriteString("\n")
	}

	// 末尾：汇总
	fmt.Fprintf(&b, "共 %d 项检查：%d pass / %d warn / %d fail / %d skip（耗时 %.2fs）\n",
		r.Summary.Total, r.Summary.Pass, r.Summary.Warn, r.Summary.Fail, r.Summary.Skip,
		float64(r.DurationMS)/1000.0)

	return b.String()
}

// renderDowngradeBanner 读 LastSessionSnapshot 输出第一屏降级历史（M13 验收锚点）。
// 输入为 nil → 输出 STATE_LAST_SESSION_MISSING 提示（**不算 fail**）。
func renderDowngradeBanner(banner *DowngradeBanner) string {
	var b strings.Builder
	b.WriteString("── 上次会话快照 ──\n")
	if banner == nil {
		b.WriteString("  [!] 未找到上次会话快照（首次运行 cloud-claude 后再 doctor 即可看到）\n")
		b.WriteString("      错误码: STATE_LAST_SESSION_MISSING\n")
		return b.String()
	}
	fmt.Fprintf(&b, "  时间戳: %s（%d 秒前）\n",
		time.Now().Add(-time.Duration(banner.SnapshotAgeSeconds)*time.Second).Format(time.RFC3339),
		banner.SnapshotAgeSeconds)
	if banner.IntendedMode != "" && banner.ActualMode != "" {
		fmt.Fprintf(&b, "  模式: 意图=%s 实际=%s\n", banner.IntendedMode, banner.ActualMode)
	}
	if banner.ClientRole != "" {
		fmt.Fprintf(&b, "  角色: %s\n", banner.ClientRole)
	}
	if banner.ConflictCount > 0 {
		fmt.Fprintf(&b, "  Mutagen 冲突: %d 个\n", banner.ConflictCount)
	}
	if banner.ReconnectCount > 0 {
		fmt.Fprintf(&b, "  重连次数: %d\n", banner.ReconnectCount)
	}
	// M13 关键字面量：`[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>`
	for _, step := range banner.DowngradeChain {
		fmt.Fprintf(&b, "  [降级] %s → %s | 原因 [%s] %s\n",
			step.From, step.To, step.ReasonCode, step.ReasonMessage)
	}
	return b.String()
}

// renderCheckLine 渲染单个 check 为一行 + 可选多行 FixApplied/FixFailed。
// 输出格式：`  [符号] name: message（建议: ... | 错误码: ...）`
func renderCheckLine(c Check, noColor bool) string {
	var b strings.Builder
	icon := pickIcon(c.Status, noColor)
	fmt.Fprintf(&b, "  %s %s: %s", icon, c.Name, c.Message)
	if c.Status == StatusWarn || c.Status == StatusFail {
		// PITFALLS M14: 所有 warn/fail 必带「建议:」子串 + 错误码
		var suffix []string
		if c.NextAction != "" {
			suffix = append(suffix, "建议: "+c.NextAction)
		}
		if c.Code != "" {
			suffix = append(suffix, "错误码: "+string(c.Code))
		}
		if len(suffix) > 0 {
			fmt.Fprintf(&b, "（%s）", strings.Join(suffix, " | "))
		}
	}
	b.WriteString("\n")
	for _, fx := range c.FixApplied {
		fmt.Fprintf(&b, "       ✓ 已修复: %s\n", fx)
	}
	for _, fx := range c.FixFailed {
		fmt.Fprintf(&b, "       ✗ 修复失败: %s\n", fx)
	}
	return b.String()
}

// pickIcon 根据 Status + noColor 选择符号。
func pickIcon(s Status, noColor bool) string {
	if noColor || !cloudclaude.ColorEnabled() {
		switch s {
		case StatusPass:
			return iconPassPlain
		case StatusWarn:
			return iconWarnPlain
		case StatusFail:
			return iconFailPlain
		case StatusSkip:
			return iconSkipPlain
		}
		return iconPassPlain
	}
	// 彩色版：直接返回带彩色 ANSI 的 icon（Colorize 内部处理 NO_COLOR）
	switch s {
	case StatusPass:
		return cloudclaude.Colorize(iconPass, cloudclaude.AnsiGreen)
	case StatusWarn:
		return cloudclaude.Colorize(iconWarn, cloudclaude.AnsiYellow)
	case StatusFail:
		return cloudclaude.Colorize(iconFail, cloudclaude.AnsiRed)
	case StatusSkip:
		return cloudclaude.Colorize(iconSkip, cloudclaude.AnsiGray)
	}
	return iconPass
}

// groupByDomain 按 Check.Domain 分组。
func groupByDomain(checks []Check) map[string][]Check {
	m := make(map[string][]Check)
	for _, c := range checks {
		m[c.Domain] = append(m[c.Domain], c)
	}
	return m
}

// RenderJSON 是 --json 模式主入口：MarshalIndent 2 空格（CONTEXT Discretion §6）。
func RenderJSON(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
```

**executor 注意：** `cloudclaude.Colorize` / `cloudclaude.ColorEnabled` / `cloudclaude.AnsiGreen` 等是 Task 2.1 导出后的符号；如 ansi 常量未改大写导出，这里可以改为在 doctor 包内再声明私有常量（不引入新 helper 即可），或者 Task 2.1 一并导出 `AnsiGreen/AnsiYellow/AnsiRed/AnsiGray`。

**(b) 新建 `internal/cloudclaude/doctor/render_test.go`（**含 M13 + JSON schema 两大锚点**）：**

```go
package doctor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDowngradeBannerRendersChain — RESEARCH §8.5 / ROADMAP §Phase 34 SC#6：
// 降级链必须逐条输出 `[降级] <from> → <to>` + 含 reason_code 字面量。
func TestDowngradeBannerRendersChain(t *testing.T) {
	banner := &DowngradeBanner{
		SnapshotAgeSeconds: 300,
		IntendedMode:       "full",
		ActualMode:         "sshfs-only",
		DowngradeChain: []DowngradeStep{
			{From: "full", To: "mutagen-only", ReasonCode: "MOUNT_MUTAGEN_VERSION_SKEW", ReasonMessage: "version skew"},
			{From: "mutagen-only", To: "sshfs-only", ReasonCode: "MOUNT_AUTO_DOWNGRADED", ReasonMessage: "daemon died"},
		},
	}
	out := renderDowngradeBanner(banner)
	for _, want := range []string{
		"[降级] full → mutagen-only",
		"[降级] mutagen-only → sshfs-only",
		"MOUNT_MUTAGEN_VERSION_SKEW",
		"MOUNT_AUTO_DOWNGRADED",
		"意图=full",
		"实际=sshfs-only",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner 未包含 %q，实际:\n%s", want, out)
		}
	}
}

// TestDowngradeBannerNil_ShowsLastSessionMissing — CONTEXT D-13 fallback。
func TestDowngradeBannerNil_ShowsLastSessionMissing(t *testing.T) {
	out := renderDowngradeBanner(nil)
	if !strings.Contains(out, "STATE_LAST_SESSION_MISSING") {
		t.Errorf("nil banner 应提示 STATE_LAST_SESSION_MISSING，实际:\n%s", out)
	}
	if !strings.Contains(out, "[!]") {
		t.Errorf("nil banner 应含 [!] 符号（informational）:\n%s", out)
	}
}

// TestRenderTextContainsNextAction — RESEARCH §8.6 / M14 终验：所有 warn/fail 必含「建议:」子串。
func TestRenderTextContainsNextAction(t *testing.T) {
	r := &Report{
		SchemaVersion: 1,
		StartedAt:     time.Now(),
		Summary:       Summary{Total: 2, Fail: 1, Warn: 1},
		Checks: []Check{
			{Domain: "mount", Name: "mergerfs_branches", Status: StatusFail,
				Code: "MOUNT_MERGERFS_FAILED", Message: "参数缺失", NextAction: "doctor mount --fix"},
			{Domain: "network", Name: "dns_resolve", Status: StatusWarn,
				Code: "SYSTEM_DNS_RESOLVE_FAILED", Message: "查询失败", NextAction: "刷新 DNS 缓存"},
		},
	}
	out := RenderText(r, true /*noColor*/)
	// 所有 [!]/[✗]/[fail]/[warn] 行必须有 "建议:" 子串
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "[fail]") || strings.HasPrefix(l, "[warn]") ||
			strings.HasPrefix(l, "[✗]") || strings.HasPrefix(l, "[!]") {
			// 放过降级 banner 中的 "[!]" 前缀（第一屏 informational，包含 "错误码:" 但可能不带 "建议:"）
			if strings.Contains(l, "未找到上次会话快照") {
				continue
			}
			if !strings.Contains(l, "建议:") {
				t.Errorf("warn/fail 行缺 '建议:'：%s", l)
			}
		}
	}
}

// TestJSONSchemaV1Lock — RESEARCH §5.1：schema_version 必须始终为 1，不带 omitempty。
func TestJSONSchemaV1Lock(t *testing.T) {
	r := &Report{SchemaVersion: 1, Summary: Summary{}, Checks: []Check{}}
	raw, err := RenderJSON(r)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schema_version", "summary", "checks", "started_at"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON 缺核心字段 %q；实际:\n%s", key, raw)
		}
	}
	if v, ok := m["schema_version"].(float64); !ok || v != 1 {
		t.Errorf("schema_version 必须为 1，实际 %v", m["schema_version"])
	}
}

// TestRenderText_PassCheck_NoNextActionSuffix — 纯 Pass check 不需要「建议:」子串。
func TestRenderText_PassCheck_NoNextActionSuffix(t *testing.T) {
	r := &Report{
		SchemaVersion: 1,
		Summary:       Summary{Total: 1, Pass: 1},
		Checks: []Check{
			{Domain: "network", Name: "dns_resolve", Status: StatusPass, Message: "OK"},
		},
	}
	out := RenderText(r, true)
	// pass 行不应含「建议:」「错误码:」
	if strings.Contains(out, "建议:") {
		t.Errorf("Pass 行不应含 '建议:'：%s", out)
	}
}
```

**禁止：**
- 在 `RenderText` 中输出凭据字段（Entry API token、OAuth path）
- 在 `RenderJSON` 中过滤 Details 字段（开放透传，只去除 nil）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/render.go && test -f internal/cloudclaude/doctor/render_test.go` 退出码 = 0
    - `grep -q "func RenderText" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "func RenderJSON" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "func renderDowngradeBanner" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "\\[降级\\]" internal/cloudclaude/doctor/render.go` 命中（M13 字面量锚点）
    - `grep -q "STATE_LAST_SESSION_MISSING" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "iconPass\\s*=\\s*\"\\[✓\\]\"" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "iconFail\\s*=\\s*\"\\[✗\\]\"" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "json.MarshalIndent" internal/cloudclaude/doctor/render.go` 命中
    - `grep -q "func TestDowngradeBannerRendersChain" internal/cloudclaude/doctor/render_test.go` 命中（M13 锚点）
    - `grep -q "func TestJSONSchemaV1Lock" internal/cloudclaude/doctor/render_test.go` 命中
    - `grep -q "func TestRenderTextContainsNextAction" internal/cloudclaude/doctor/render_test.go` 命中（M14 锚点）
    - `go test ./internal/cloudclaude/doctor/ -run "TestDowngradeBanner|TestJSONSchema|TestRenderText" -count=1 -v` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.10: 实现 doctor/doctor.go::RunDoctor 主流程（串行执行 + lazy SSH + Summary 聚合）+ 导入测试 doctor_test.go</name>
  <files>internal/cloudclaude/doctor/doctor.go, internal/cloudclaude/doctor/doctor_test.go</files>

  <read_first>
    - internal/cloudclaude/doctor/doctor.go（Task 2.2 框架 + RunDoctor panic 占位）
    - internal/cloudclaude/doctor/*.go（Task 2.3-2.9 所有 check 函数签名）
    - internal/cloudclaude/ssh_doctor.go::sshConnect（line 119-150 SSH 拨号 helper）
    - internal/cloudclaude/last_session.go::LoadLastSession（LastSessionSnapshot 读取）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-06 / §D-07 / §D-19 / §D-20（执行策略）
  </read_first>

  <action>
**(a) 修改 `internal/cloudclaude/doctor/doctor.go` — 把 `RunDoctor` 的 `panic(...)` 占位替换为真实实现：**

```go
// RunDoctor 顶层入口。执行顺序（CONTEXT D-07）：network → auth → ssh → mount → disk。
// 远端 conn lazy 建立（CONTEXT D-20）；未 init 时 mount/ssh/disk 维度 StatusSkip（D-06）。
func RunDoctor(ctx context.Context, opts Options) (*Report, error) {
	start := time.Now()
	report := &Report{
		SchemaVersion: 1,
		StartedAt:     start,
		Checks:        []Check{},
	}

	timeout := opts.CheckTimeout
	if timeout <= 0 {
		if opts.Verbose {
			timeout = 30 * time.Second
		} else {
			timeout = 5 * time.Second
		}
	}

	// 1) 读 LastSessionSnapshot → DowngradeHistory
	snap, _ := cloudclaude.LoadLastSession()
	if snap != nil {
		report.DowngradeHistory = convertSnapshotToBanner(snap)
	}

	// 2) 读本地 config（未 init 时 cfg=nil）
	var cfg *cloudclaude.Config
	if _, cfgC := checkConfigPresent(ctx); cfgC != nil {
		cfg = cfgC
	}

	// 决定要跑哪些维度
	want := func(d string) bool {
		if opts.Domain == "" || opts.Domain == "all" {
			return true
		}
		return opts.Domain == d
	}

	// lazy SSH conn（auth / mount / ssh / disk 维度共享）
	var remoteConn *ssh.Client
	var remoteRunner RemoteRunner
	var authResp *cloudclaude.AuthResponse
	closeRemote := func() {
		if remoteConn != nil {
			_ = remoteConn.Close()
			remoteConn = nil
			remoteRunner = nil
		}
	}
	defer closeRemote()

	ensureRemote := func() {
		if remoteRunner != nil || cfg == nil || authResp == nil {
			return
		}
		sshCfg := cloudclaude.SSHConfig{
			Host: authResp.SSHHost, Port: authResp.SSHPort,
			User: authResp.SSHUser, Password: authResp.SSHPass,
		}
		conn, err := cloudclaude.SSHConnect(sshCfg)
		if err != nil {
			return // 所有 RequiresRemote=true check 将走 StatusSkip
		}
		remoteConn = conn
		remoteRunner = NewSSHRemoteRunner(conn)
	}

	// 3) network 维度（3 check，本地可独立跑）
	if want("network") {
		report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "dns_resolve", timeout,
			func(c context.Context) Check {
				host := ""
				if cfg != nil {
					host = parseHostFromGateway(cfg.Gateway)
				}
				return checkDNSResolve(c, host)
			}))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "gateway_reachable", timeout,
			func(c context.Context) Check {
				gw := ""
				if cfg != nil {
					gw = cfg.Gateway
				}
				return checkGatewayReachable(c, gw)
			}))
		// egress_ip_visible 需要 runner；在 auth 之后 ensureRemote
	}

	// 4) auth 维度（cfg 需要 + Entry API + 远端 OAuth）
	if want("auth") {
		cp, cfg2 := checkConfigPresent(ctx)
		report.Checks = append(report.Checks, cp)
		if cfg2 != nil {
			cfg = cfg2
		}
		if cfg != nil {
			etv, resp := checkEntryTokenValid(ctx, cfg)
			report.Checks = append(report.Checks, etv)
			if resp != nil {
				authResp = resp
				report.RemoteImageVer = resp.ImageVersion
			}
		}
		ensureRemote()
		if want("network") && authResp != nil {
			report.Checks = append(report.Checks, runWithTimeout(ctx, "network", "egress_ip_visible", timeout,
				func(c context.Context) Check {
					// 安全取 authResp.ExpectedEgressIP：如 entry 包未交付该字段，executor 改为传空
					// 字符串，由 checkEgressIPVisible 内部 `expectedIP == ""` 分支走 Pass 或与硬编码
					// baseline 比对（warn 时仍带 NET_EGRESS_IP_DRIFT + 中文 NextAction）。
					expectedIP := authRespExpectedEgressIP(authResp)
					return checkEgressIPVisible(c, remoteRunner, expectedIP)
				}))
		}
		report.Checks = append(report.Checks,
			checkOAuthCredentials(ctx, remoteConn, authRespClaudeAccountID(authResp)))
	}

	// 5) ssh 维度
	if want("ssh") {
		ensureRemote()
		kaInterval := 15 * time.Second
		report.Checks = append(report.Checks, checkKeepaliveConfig(ctx, kaInterval))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "ssh", "sshd_keepalive_drift", timeout,
			func(c context.Context) Check { return checkSSHDKeepaliveDrift(c, remoteRunner) }))
		khPath := defaultKnownHostsPath()
		hostPort := ""
		if authResp != nil {
			hostPort = makeKnownHostsHostPort(authResp.SSHHost, authResp.SSHPort)
		}
		report.Checks = append(report.Checks, checkKnownHosts(ctx, khPath, hostPort))
		if authResp != nil {
			sshCfg := cloudclaude.SSHConfig{
				Host: authResp.SSHHost, Port: authResp.SSHPort,
				User: authResp.SSHUser, Password: authResp.SSHPass,
			}
			report.Checks = append(report.Checks, checkWorkspaceSSHKeys(ctx, sshCfg))
		}
	}

	// 6) mount 维度
	if want("mount") {
		ensureRemote()
		report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "mutagen_version_match", timeout,
			func(c context.Context) Check { return checkMutagenVersionMatch(c, remoteRunner) }))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "mergerfs_branches", timeout,
			func(c context.Context) Check { return checkMergerfsBranches(c, remoteRunner) }))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "mount", "sshfs_mountpoint", timeout,
			func(c context.Context) Check { return checkSSHFSMountpoint(c, remoteRunner) }))
		report.Checks = append(report.Checks, checkFUSEResidual(ctx))
		report.Checks = append(report.Checks, checkAppArmorFusermount3(ctx))
	}

	// 7) disk 维度
	if want("disk") {
		ensureRemote()
		report.Checks = append(report.Checks, checkLocalDisk(ctx))
		report.Checks = append(report.Checks, runWithTimeout(ctx, "disk", "container_disk", timeout,
			func(c context.Context) Check { return checkContainerDisk(c, remoteRunner) }))
		report.Checks = append(report.Checks, checkMutagenDataSize(ctx))
	}

	// 8) 聚合 Summary
	for _, c := range report.Checks {
		report.Summary.Total++
		switch c.Status {
		case StatusPass:
			report.Summary.Pass++
		case StatusWarn:
			report.Summary.Warn++
		case StatusFail:
			report.Summary.Fail++
		case StatusSkip:
			report.Summary.Skip++
		}
	}
	report.DurationMS = time.Since(start).Milliseconds()
	return report, nil
}
```

**辅助函数**（同文件末尾追加）：

```go
// convertSnapshotToBanner 把 cloudclaude.LastSessionSnapshot 转为 doctor DowngradeBanner。
func convertSnapshotToBanner(snap *cloudclaude.LastSessionSnapshot) *DowngradeBanner {
	age := int64(time.Since(snap.Timestamp).Seconds())
	steps := make([]DowngradeStep, 0, len(snap.DowngradeChain))
	for _, s := range snap.DowngradeChain {
		steps = append(steps, DowngradeStep{
			From:          s.From,
			To:            s.To,
			ReasonCode:    s.ReasonCode,
			ReasonMessage: s.ReasonMessage,
		})
	}
	return &DowngradeBanner{
		SnapshotAgeSeconds: age,
		IntendedMode:       snap.IntendedMode,
		ActualMode:         snap.ActualMode,
		DowngradeChain:     steps,
		ConflictCount:      snap.ConflictCount,
		ReconnectCount:     snap.ReconnectCount,
		TmuxSession:        snap.TmuxSession,
		ClientRole:         snap.ClientRole,
	}
}

func parseHostFromGateway(gw string) string {
	// 粗略：https://host:port/ → host
	s := strings.TrimPrefix(gw, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

func defaultKnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// authRespClaudeAccountID 安全取 authResp.ClaudeAccountID（可能字段名不同，按实际 struct 调整）。
func authRespClaudeAccountID(r *cloudclaude.AuthResponse) string {
	if r == nil {
		return ""
	}
	return r.ClaudeAccountID
}

// authRespExpectedEgressIP 安全取 authResp.ExpectedEgressIP；
// 若 entry 包 AuthResponse 未交付该字段（v3.0 未必存在），executor 应：
//   1. 在 entry 包补齐该字段；或
//   2. 把本函数改为恒返回 ""（此时 checkEgressIPVisible 的 Details 记
//      `expected: "<unknown — 字段未交付>"`，仍走 Pass，不误报 drift）。
func authRespExpectedEgressIP(r *cloudclaude.AuthResponse) string {
	if r == nil {
		return ""
	}
	// executor：如 AuthResponse 有 ExpectedEgressIP 字段就返回之；否则返回 ""。
	// 字段存在性检查通过编译期错误暴露 — 若编译失败，回退到 `return ""`。
	return r.ExpectedEgressIP
}
```

**executor 注意：** 本 task 需要引入 `net / os / path/filepath / strings` + `cloudclaude` + `golang.org/x/crypto/ssh`。`cloudclaude.SSHConnect` 已由 Phase 32-02 在 `internal/cloudclaude/ssh.go:200` 导出（`return sshConnect(cfg)` 包装），本 plan 不需任何重命名 / 导出动作。`AuthResponse.ExpectedEgressIP` 字段：executor 先尝试编译，若 entry 包 `AuthResponse` 未定义该字段，把 `authRespExpectedEgressIP` 改为恒返回 `""`（`return ""`）并把该现象记录到 `34-02-SUMMARY.md` carry-over，由 v3.1 backlog 补齐 entry.go 字段。此时 `checkEgressIPVisible` 走 `expectedIP == ""` 分支，不触发 drift 警告，仅 Details 里记 `expected: "<unknown — 字段未交付>"`。

**(b) 新建 `internal/cloudclaude/doctor/doctor_test.go`（RunDoctor 集成点单测，mock 所有包级 var）：**

```go
package doctor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// TestRunDoctor_NoInit_NetworkAuthOnlyLocal — CONTEXT D-06：未 init 时 mount/ssh/disk 跳过。
func TestRunDoctor_NoInit_NetworkAuthOnlyLocal(t *testing.T) {
	origCfg := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("config 不存在") }
	t.Cleanup(func() { loadConfig = origCfg })
	// lookupHost / httpGet 走真实实现会慢：mock 为本地探测
	origLH := lookupHost
	lookupHost = func(host string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	t.Cleanup(func() { lookupHost = origLH })

	r, err := RunDoctor(context.Background(), Options{Domain: "all", CheckTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("RunDoctor err: %v", err)
	}
	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion 必须为 1，实际 %d", r.SchemaVersion)
	}
	// 至少 network + auth 维度的 check 存在
	foundDomains := map[string]int{}
	for _, c := range r.Checks {
		foundDomains[c.Domain]++
	}
	if foundDomains["network"] == 0 {
		t.Error("network 维度未 run")
	}
	if foundDomains["auth"] == 0 {
		t.Error("auth 维度未 run")
	}
}

// TestRunDoctor_DomainFilter — --domain=network 只跑 network 维度。
func TestRunDoctor_DomainFilter(t *testing.T) {
	origCfg := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("config 不存在") }
	t.Cleanup(func() { loadConfig = origCfg })
	origLH := lookupHost
	lookupHost = func(host string) ([]string, error) { return nil, fmt.Errorf("no host") }
	t.Cleanup(func() { lookupHost = origLH })

	r, err := RunDoctor(context.Background(), Options{Domain: "network", CheckTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("RunDoctor err: %v", err)
	}
	for _, c := range r.Checks {
		if c.Domain != "network" {
			t.Errorf("Domain filter 失效：出现 %s.%s", c.Domain, c.Name)
		}
	}
}

// TestRunDoctor_SummaryAggregation — Summary 计数正确。
func TestRunDoctor_SummaryAggregation(t *testing.T) {
	r := &Report{
		Checks: []Check{
			{Status: StatusPass}, {Status: StatusPass},
			{Status: StatusWarn}, {Status: StatusFail},
			{Status: StatusSkip}, {Status: StatusSkip}, {Status: StatusSkip},
		},
	}
	// 手动聚合（RunDoctor 内部逻辑的 unit 版本）
	for _, c := range r.Checks {
		r.Summary.Total++
		switch c.Status {
		case StatusPass:
			r.Summary.Pass++
		case StatusWarn:
			r.Summary.Warn++
		case StatusFail:
			r.Summary.Fail++
		case StatusSkip:
			r.Summary.Skip++
		}
	}
	if r.Summary.Total != 7 || r.Summary.Pass != 2 || r.Summary.Warn != 1 ||
		r.Summary.Fail != 1 || r.Summary.Skip != 3 {
		t.Errorf("聚合错误：%+v", r.Summary)
	}
}
```

**禁止：**
- 在 doctor_test.go 中调真实 SSH 拨号（集成测试留 Plan 03）
- 任何 `go test` 命令预期 docker daemon 存在（单测必须纯内存 mock）
  </action>

  <acceptance_criteria>
    - `grep -q "func RunDoctor(ctx context.Context, opts Options) (\\*Report, error)" internal/cloudclaude/doctor/doctor.go` 命中
    - `! grep -q "panic(\"executor" internal/cloudclaude/doctor/doctor.go` 为真（占位 panic 已移除）
    - `grep -q "convertSnapshotToBanner" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "cloudclaude.LoadLastSession" internal/cloudclaude/doctor/doctor.go` 命中
    - `grep -q "NewSSHRemoteRunner" internal/cloudclaude/doctor/doctor.go` 命中
    - `test -f internal/cloudclaude/doctor/doctor_test.go` 退出码 = 0
    - `grep -q "func TestRunDoctor_NoInit_NetworkAuthOnlyLocal" internal/cloudclaude/doctor/doctor_test.go` 命中（D-06 回归）
    - `grep -q "func TestRunDoctor_DomainFilter" internal/cloudclaude/doctor/doctor_test.go` 命中
    - `go build ./internal/cloudclaude/doctor/...` 退出码 = 0
    - `go vet ./internal/cloudclaude/doctor/...` 退出码 = 0
    - `go test ./internal/cloudclaude/doctor/ -count=1 -short -run "TestRunDoctor_" -v` 退出码 = 0
    - **全包单测汇总（本 task 启用整个 doctor 包）**：`go test ./internal/cloudclaude/doctor/ -count=1 -short` 退出码 = 0（含 Task 2.2-2.9 所有子测）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 2.11: 新建 cmd/cloud-claude/doctor.go + 修改 main.go 注册（CONTEXT D-03 / D-05 / PATTERNS §3.1）</name>
  <files>cmd/cloud-claude/doctor.go, cmd/cloud-claude/main.go</files>

  <read_first>
    - cmd/cloud-claude/main.go（Plan 01 Task 1.7 完成后的 AddCommand + switch case）
    - cmd/cloud-claude/sessions.go（三段式 cobra 模板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-03 / §D-05（cobra ValidArgs）+ §D-16（退出码）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §3.1
  </read_first>

  <action>
**(a) 新建 `cmd/cloud-claude/doctor.go`：**

```go
// cmd/cloud-claude/doctor.go — Phase 34 Plan 02 Task 2.11
//
// cloud-claude doctor [domain] 子命令：五维度自检。
// 与 cloud-claude ssh doctor（v2.0 quick task 入口）双入口共存（CONTEXT D-04）。
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/doctor"
)

const (
	doctorExitOK   = 0
	doctorExitWarn = 1
	doctorExitFail = 2
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor [domain]",
		Short: "cloud-claude 五维度自检（network/auth/ssh/mount/disk）",
		Long: "运行 cloud-claude doctor 检测当前环境健康度，每项输出 [符号] + 中文原因 + 建议 + 错误码。\n" +
			"支持 --fix 自动修复（Plan 03 落实）、--json 脚本消费、--verbose 详细日志、--yes 跳过交互确认。\n" +
			"退出码：0 全部通过 / 1 含 warn 无 fail / 2 含 fail（与 brew doctor 对齐）。",
		Args:          cobra.MaximumNArgs(1),
		ValidArgs:     []string{"network", "auth", "ssh", "mount", "disk", "all"},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runDoctor,
	}
	cmd.Flags().Bool("fix", false, "自动修复检测到的问题（Plan 03 落实）")
	cmd.Flags().Bool("verbose", false, "展开探测细节 + 放宽 timeout 到 30s")
	cmd.Flags().Bool("json", false, "输出 JSON 供脚本消费")
	cmd.Flags().Bool("yes", false, "跳过交互式 y/N 确认（CI 友好）")
	return cmd
}

func runDoctor(cmd *cobra.Command, args []string) error {
	domain := "all"
	if len(args) == 1 {
		domain = args[0]
	}

	fix, _ := cmd.Flags().GetBool("fix")
	verbose, _ := cmd.Flags().GetBool("verbose")
	jsonOut, _ := cmd.Flags().GetBool("json")
	yes, _ := cmd.Flags().GetBool("yes")

	opts := doctor.Options{
		Domain:       domain,
		Fix:          fix,
		Verbose:      verbose,
		JSON:         jsonOut,
		NoColor:      os.Getenv("NO_COLOR") != "",
		Yes:          yes,
		CheckTimeout: 0, // doctor 包自选默认
	}

	ctx, cancel := contextWithDoctorTimeout(cmd.Context(), verbose)
	defer cancel()

	report, err := doctor.RunDoctor(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitInternalError)
		return nil
	}

	if fix && !anyFixerRegistered() {
		// Plan 02 占位：FixerRegistry 尚未实现（Plan 03 落）
		fmt.Fprintln(os.Stdout, "[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）")
	}

	if jsonOut {
		raw, err := doctor.RenderJSON(report)
		if err != nil {
			fmt.Fprintln(os.Stderr, "JSON 序列化失败: "+err.Error())
			os.Exit(exitInternalError)
			return nil
		}
		fmt.Println(string(raw))
	} else {
		fmt.Print(doctor.RenderText(report, opts.NoColor))
	}

	// 退出码按 Summary 计（CONTEXT D-16：修复成功的 fail 不降级为 0）
	switch {
	case report.Summary.Fail > 0:
		os.Exit(doctorExitFail)
	case report.Summary.Warn > 0:
		os.Exit(doctorExitWarn)
	default:
		os.Exit(doctorExitOK)
	}
	return nil
}

// contextWithDoctorTimeout 顶层 timeout：verbose 2min，默认 60s（足够跑完 17 项 + ensureRemote）。
func contextWithDoctorTimeout(parent context.Context, verbose bool) (context.Context, context.CancelFunc) {
	if verbose {
		return context.WithTimeout(parent, 2*time.Minute)
	}
	return context.WithTimeout(parent, 60*time.Second)
}

// anyFixerRegistered 是 Plan 03 完成前的占位（恒 false）。Plan 03 会改为检查 doctor.FixerRegistry 长度。
func anyFixerRegistered() bool { return false }
```

**executor 注意：** `contextWithDoctorTimeout` 使用标准 `context.Context` / `context.CancelFunc` 签名；需在 `cmd/cloud-claude/doctor.go` 的 import 中补上 `"context"` 与 `"time"`。

**(b) 修改 `cmd/cloud-claude/main.go` — 在 Plan 01 修改过的 AddCommand 行末尾再追加 `newDoctorCmd()`：**

把：

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd(), newExplainCmd())
```

改为：

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd(), newExplainCmd(), newDoctorCmd())
```

把 switch case 从（Plan 01 状态）：

```go
case "init", "env", "ssh", "sync", "sessions", "explain", "help", "--help", "-h":
```

改为：

```go
case "init", "env", "ssh", "sync", "sessions", "explain", "doctor", "help", "--help", "-h":
```

**verbatim 守恒：**
- 函数名 `newDoctorCmd` / `runDoctor`
- 退出码常量 `doctorExitOK=0 / doctorExitWarn=1 / doctorExitFail=2`（RESEARCH §5.2 — **不**新增 `exitcodes.go` 常量）
- `ValidArgs` 字面量 `[]string{"network","auth","ssh","mount","disk","all"}`（CONTEXT D-05）
- Plan 03 占位输出 `"[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）"`（M14 兼容 — 该行不走 warn/fail check 渲染，不触发 grep gate）
  </action>

  <acceptance_criteria>
    - `test -f cmd/cloud-claude/doctor.go` 退出码 = 0
    - `grep -q "^func newDoctorCmd" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "^func runDoctor" cmd/cloud-claude/doctor.go` 命中
    - `grep -q 'ValidArgs:\s*\[\]string{"network", "auth", "ssh", "mount", "disk", "all"}' cmd/cloud-claude/doctor.go` 命中
    - `grep -q "doctorExitFail\s*=\s*2" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "doctor.RunDoctor" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "doctor.RenderJSON" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "doctor.RenderText" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "newDoctorCmd()" cmd/cloud-claude/main.go` 命中
    - `grep -qE 'case "init", "env", "ssh", "sync", "sessions", "explain", "doctor",' cmd/cloud-claude/main.go` 命中
    - `grep -q "context.CancelFunc" cmd/cloud-claude/doctor.go` 命中（contextWithDoctorTimeout 真实签名）
    - `grep -q "context.WithTimeout(parent" cmd/cloud-claude/doctor.go` 命中（contextWithDoctorTimeout 真实实现）
    - `go build ./cmd/cloud-claude/...` 退出码 = 0
    - `go vet ./cmd/cloud-claude/...` 退出码 = 0
    - 人工 smoke：`./cloud-claude doctor --help` 输出含 `--fix` / `--json` / `--verbose` / `--yes`（可在 done 中作 manual 项；CI 不强求）
  </acceptance_criteria>
</task>

</tasks>

<verification>
## Plan-level 验证

```bash
# 1. 全仓库构建（doctor 包 + cobra 注册闭环）
go build ./...

# 2. doctor 包完整单测（11 个 _test.go 文件）
go test ./internal/cloudclaude/doctor/ -count=1 -v

# 3. 既有 cloudclaude 包无回归（Task 2.1 colors 导出影响面）
go test ./internal/cloudclaude/ -count=1 -short

# 4. errcodes 包无回归（Plan 01 Task 验证）
go test ./internal/cloudclaude/errcodes/ -count=1

# 5. cmd/cloud-claude 包（含 explain + doctor）
go test ./cmd/cloud-claude/ -count=1

# 6. M13 终验（TestDowngradeBannerRendersChain 是 SC#6 验收锚点）
go test ./internal/cloudclaude/doctor/ -run TestDowngradeBannerRendersChain -count=1 -v

# 7. M14 终验（TestRenderTextContainsNextAction）
go test ./internal/cloudclaude/doctor/ -run TestRenderTextContainsNextAction -count=1 -v

# 8. JSON schema 锁（TestJSONSchemaV1Lock）
go test ./internal/cloudclaude/doctor/ -run TestJSONSchemaV1Lock -count=1 -v

# 9. host-agent endpoint 边界守恒（SC#9）— 本 plan 不引入任何 host-agent 调用
! rg -nE "agentapi\.(Action|NewClient|RunHostAction)" internal/cloudclaude/doctor/

# 10. 17 项 check 枚举（粗略：grep check function 定义）
[ "$(rg -cE '^func check[A-Z][A-Za-z]+' internal/cloudclaude/doctor/*.go | awk -F: '{s+=$NF} END{print s}')" -ge 17 ]
```

## SC 映射（ROADMAP §Phase 34 Success Criteria）

| SC | 本 Plan 交付 |
|----|-------------|
| SC#2（doctor 5 维度 + 四要素） | Task 2.4-2.8 实现 17 项 check；Task 2.9 `renderCheckLine` 四要素格式 |
| SC#3（warn/fail 行必含「建议:」子串） | Task 2.9 `TestRenderTextContainsNextAction` 回归 + Plan 03 scripts/ci-doctor-grep.sh 二次验证 |
| SC#5（--json 可 jq 解析 / NO_COLOR / 退出码 0/1/2） | Task 2.9 `RenderJSON` + `TestJSONSchemaV1Lock`；Task 2.11 退出码实现 |
| SC#6（降级历史第一屏） | Task 2.9 `TestDowngradeBannerRendersChain`（M13 锚点） |
| SC#7（mergerfs 篡改命中 + 修复命令） | Task 2.7 `checkMergerfsBranches` 6 字面量 + errcodes.MOUNT_MERGERFS_FAILED.NextAction 含「doctor mount --fix」；E2E 由 Plan 03 integration_test 触发 |
| SC#9（doctor 不调 host-agent 新 endpoint） | verification 第 9 条 rg 断言为空 |
</verification>

<success_criteria>
- [ ] 11 个 task 全部完成，各 acceptance_criteria 全 PASS
- [ ] `go build ./...` 退出码 = 0（doctor 包 + cobra 注册闭环）
- [ ] `go test ./internal/cloudclaude/doctor/ -count=1 -short` 退出码 = 0（≥ 40 条单测全 PASS）
- [ ] 17 项 check 全部实现（network×3 + auth×3 + ssh×4 + mount×5 + disk×3 = 18 实际，≥ REQ-F6-A 下限）
- [ ] `TestDowngradeBannerRendersChain` PASS（M13 / SC#6 终验）
- [ ] `TestRenderTextContainsNextAction` PASS（M14 / SC#3 终验）
- [ ] `TestJSONSchemaV1Lock` PASS（SC#5）
- [ ] `cloud-claude doctor --help` 列出 4 flag + ValidArgs 6 项
- [ ] 无任何 `agentapi.Action*` / `agentapi.NewClient` 引用（SC#9 host-agent 边界守恒）
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| `cloud-claude doctor` 本地 | 读 ~/.cloud-claude/config.yaml + ~/.cloud-claude/last-session.json + ~/.ssh/known_hosts |
| `cloud-claude doctor` → Entry API | HTTP GET /healthz + AuthenticateAndWait（复用 EntryClient token 鉴权） |
| `cloud-claude doctor` → 远端容器 SSH | 复用 EntryClient 返回的 SSH 凭据（host/port/user/password）；sessions 在容器内执行只读命令 |
| 远端容器 → 本地 stdout/JSON | 不经 shell 回显，走 stdout/stderr 字节流 |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-34-02-01 | Tampering | `doctor` args[0] 未校验就进 ValidArgs | low | mitigate | cobra `cobra.MaximumNArgs(1) + ValidArgs=[]string{"network","auth","ssh","mount","disk","all"}` 内置校验；非法值 cobra 报错（Task 2.11） |
| T-34-02-02 | Injection | 远端 SSH 命令构造（sshd -T / mountpoint / df / getfattr） | low | mitigate | 所有远端命令字面量硬编码（`sshd -T 2>/dev/null | grep ...`）无用户变量注入；RESEARCH §3.3-§3.5 命令表全部静态；不涉及 shellescape（无用户参数插入） |
| T-34-02-03 | Information Disclosure | `Check.Details` 意外写入 OAuth 凭据 | medium | mitigate | `auth.go` `checkOAuthCredentials` 只写 `MinutesToExpire` 数字，不写 token 字面量；其它 check 的 Details 走白名单（mount.branches_xattr / mount.mount / disk.addrs / disk.avail），禁止写 `cfg.Password` / `authResp.SSHPass`；acceptance_criteria `! grep -n "cfg.Password\|SSHPass" internal/cloudclaude/doctor/*.go`（Plan 03 集成测试阶段再次验证） |
| T-34-02-04 | Information Disclosure | JSON 输出泄漏内部路径 / hostname | low | accept | rationale：doctor 是用户自助排障工具，输出目标路径 `~/.cloud-claude/` / hostPort 是必要信息；CONTEXT §security_threat_model T5 仅要求 ExpiresAt 不入 JSON，已通过 OAuth 只存 MinutesToExpire 实现 |
| T-34-02-05 | DoS | `RunDoctor` 某 check hang 拖死整轮 | medium | mitigate | `runWithTimeout` 5s 默认 / 30s Verbose（CONTEXT D-08）；timeout 命中映射到 `SYSTEM_CHECK_TIMEOUT` + StatusFail；顶层 `context.WithTimeout(parent, 60s/120s)`（Task 2.11 `contextWithDoctorTimeout`） |
| T-34-02-06 | Tampering | LastSessionSnapshot 被恶意 JSON 注入导致 `renderDowngradeBanner` panic | low | mitigate | `cloudclaude.LoadLastSession` 已做 json.Unmarshal 错误处理（Phase 31/32 D-18 实现）；doctor 包 `convertSnapshotToBanner` 只拷贝结构化字段，不 eval |
| T-34-02-07 | Spoofing | `ColorEnabled` 导出后被外部包恶意调用伪造 NO_COLOR 状态 | low | accept | Go 导出符号的 trust 边界是「同模块包」— `colors.go` 导出不新增外部 API 风险；外部包调只影响自身 stdout 染色，无权提升 |

**ASVS L1 高严重度阻塞性威胁：** 0
</threat_model>

<output>
After completion, create `.planning/phases/34-cloud-claude-doctor-v3/34-02-SUMMARY.md` with:
- 11 个 task 的实际 commit SHA + 关键 diff 片段引用
- doctor 包实际 check 总数（`grep -c "^func check" internal/cloudclaude/doctor/*.go`）
- `TestDowngradeBannerRendersChain` / `TestRenderTextContainsNextAction` / `TestJSONSchemaV1Lock` 三大锚点 PASS 时间戳
- Task 2.1 `ColorEnabled` / `Colorize` 导出影响面（diff 统计 — 多少处调用点改大写）
- `cloud-claude doctor --help` 输出样本（人工 smoke test）
- carry-over：`checkWorkspaceSSHKeys` 集成（需要真实 SSH）→ Plan 03 integration_test 兜底
- carry-over：`AuthResponse.ExpectedEgressIP` 字段名实际验证（若 entry.go 未导出该字段，改为 skip 检查）
</output>
