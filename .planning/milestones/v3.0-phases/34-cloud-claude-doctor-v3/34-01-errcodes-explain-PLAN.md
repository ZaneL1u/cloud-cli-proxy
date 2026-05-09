---
phase: 34-cloud-claude-doctor-v3
plan: 01
type: execute
wave: 1
depends_on: []
autonomous: true
requirements: [REQ-F8-A, REQ-F8-B, REQ-F8-C]
requirements_addressed: [REQ-F8-A, REQ-F8-B, REQ-F8-C]
files_modified:
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/state.go
  - internal/cloudclaude/errcodes/system.go
  - internal/cloudclaude/errcodes/ssh.go
  - internal/cloudclaude/errcodes/auth.go
  - internal/cloudclaude/errcodes/disk.go
  - internal/cloudclaude/errcodes/explanations.go
  - internal/cloudclaude/errcodes/explanations_test.go
  - internal/cloudclaude/errcodes/codes_test.go
  - cmd/cloud-claude/explain.go
  - cmd/cloud-claude/explain_test.go
  - cmd/cloud-claude/main.go

must_haves:
  truths:
    - "Registry 命名空间 8 域闭合 {MOUNT,SESSION,NET,STATE,SYSTEM,SSH,AUTH,DISK}，每条 Code 匹配 ^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$（REQ-F8-A / CONTEXT D-23 / ROADMAP §Phase 34 SC#1）"
    - "每条 Entry 有非空中文 Message + 非空 NextAction（≤ 80 runes），与 v2.0 lower-case 字面量 {auth_failed / auth_expired / entry_token_expired / host_action_failed / entry_config_missing} 不冲突（REQ-F8-B / CONTEXT D-22 / PITFALLS C8）"
    - "每条非 informational Code 在 ExtendedExplanations 中有 ≥ 200 中文字符的长说明；informational 类显式登记到 ExplainExempt（REQ-F8-C / CONTEXT D-18 / ROADMAP §Phase 34 SC#8）"
    - "cloud-claude explain <CODE> 已注册 exit 0 + stdout 含 Format(code) 两段 + 详细说明；未注册 code exit 4（REQ-F8-C / CONTEXT D-17 / ROADMAP §Phase 34 SC#8）"
    - "Phase 33 admin handler 硬编码字面量 STATE_VOLUME_IN_USE_001 补录入 Registry，字面量不变（CONTEXT D-27）"
  artifacts:
    - path: internal/cloudclaude/errcodes/codes.go
      provides: "8 域 Code 常量块扩展（追加 STATE_* / SYSTEM_* / SSH_* / AUTH_* / DISK_* 共 17 条常量字面量）"
      contains: "STATE_LAST_SESSION_MISSING"
    - path: internal/cloudclaude/errcodes/state.go
      provides: "STATE_* 错误码注册（3 条）"
      contains: "STATE_VOLUME_IN_USE_001"
    - path: internal/cloudclaude/errcodes/system.go
      provides: "SYSTEM_* 错误码注册（4 条）"
      contains: "SYSTEM_APPARMOR_FUSERMOUNT3_MISSING"
    - path: internal/cloudclaude/errcodes/ssh.go
      provides: "SSH_* 错误码注册（2 条）"
      contains: "SSH_KNOWN_HOSTS_CONFLICT"
    - path: internal/cloudclaude/errcodes/auth.go
      provides: "AUTH_* 错误码注册（4 条）"
      contains: "AUTH_CONFIG_MISSING"
    - path: internal/cloudclaude/errcodes/disk.go
      provides: "DISK_* 错误码注册（3 条）"
      contains: "DISK_LOCAL_LOW"
    - path: internal/cloudclaude/errcodes/explanations.go
      provides: "ExtendedExplanations map + ExplainExempt set + registerExplanation 防御重复"
      contains: "ExtendedExplanations"
    - path: cmd/cloud-claude/explain.go
      provides: "explain <code> cobra 子命令 + 三段输出 + exit 4 未找到"
      contains: "newExplainCmd"
  key_links:
    - from: "internal/cloudclaude/errcodes/codes.go::const block"
      to: "{state,system,ssh,auth,disk}.go::init() MustRegister"
      via: "常量名与字面值严格一致，init() 调 MustRegister({Code, Severity, Message, NextAction})"
      pattern: "MustRegister\\(Entry\\{"
    - from: "cmd/cloud-claude/explain.go::newExplainCmd"
      to: "errcodes.Lookup + errcodes.Format + errcodes.ExtendedExplanations"
      via: "ExactArgs(1) 大小写敏感匹配；未找到 os.Exit(exitConfigError)=4"
      pattern: "errcodes.Lookup\\(errcodes.Code\\(args\\[0\\]\\)\\)"
    - from: "cmd/cloud-claude/main.go::rootCmd.AddCommand"
      to: "newExplainCmd()"
      via: "AddCommand 注册 + switch os.Args[1] case 追加 \"explain\""
      pattern: "newExplainCmd\\(\\)"
---

<objective>
收口 Phase 31 / 32 / 33 分散落码后遗留的错误码命名空间，交付 8 域闭合的 Registry + cloud-claude explain 子命令。具体实现：
1. `internal/cloudclaude/errcodes/codes.go` 常量块末尾追加 17 个新 Code 字面量（STATE × 3 / SYSTEM × 4 / SSH × 2 / AUTH × 4 / DISK × 3，与 CONTEXT D-21 表逐条对齐）；
2. 新建 5 个域文件 `state.go / system.go / ssh.go / auth.go / disk.go`，每文件一个 `init()` 按 `errcodes/mount.go` 模板调 `MustRegister`；
3. 新建 `explanations.go` 暴露 `ExtendedExplanations map[Code]string` + `ExplainExempt map[Code]struct{}`，init 内注册 ≥ 30 条长说明（17 条新 + 既有 25 条中的非 informational 码）；
4. 新建 `explanations_test.go` 遍历 Registry 断言三条新规则（ExtendedExplanations 覆盖 ≥ 200 runes、v2.0 lower-case 禁入、8 域闭合）；扩展既有 `codes_test.go` 的 `len(reg) < 15` 下限放宽到 `< 30`；
5. 新建 `cmd/cloud-claude/explain.go` + `explain_test.go` + 修改 `cmd/cloud-claude/main.go` 注册子命令（AddCommand + DisableFlagParsing switch 追加）。

Purpose: 本 Plan 作为 Phase 34 的 **Wave 1 基础设施**，为 Wave 2 `cloud-claude doctor` 提供统一错误码查找 + Wave 3 `--fix` 5 类修复提供字面量常量；同时独立交付 REQ-F8-C `cloud-claude explain <code>` CLI（对标 `rustc --explain`）。

Output:
- 1 个修改的 codes.go（+17 常量行）
- 5 个新增域注册文件
- 1 个新增 explanations.go + 1 个新增 explanations_test.go
- 1 个修改的 codes_test.go（下限从 15 调整为 30）
- 2 个新增 cmd/cloud-claude 文件（explain.go + explain_test.go）
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
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/32-ssh-tmux/32-CONTEXT.md
@.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md

# 直接修改对象
@internal/cloudclaude/errcodes/codes.go
@internal/cloudclaude/errcodes/codes_test.go
@internal/cloudclaude/errcodes/mount.go
@internal/cloudclaude/errcodes/net.go
@internal/cloudclaude/errcodes/session.go
@cmd/cloud-claude/main.go
@cmd/cloud-claude/sessions.go
@internal/controlplane/http/admin_claude_accounts.go

<interfaces>
<!-- 既有契约，executor 直接复用，无需 grep 探索。 -->

From internal/cloudclaude/errcodes/codes.go:
```go
type Code string
type Severity int
const ( SeverityInfo Severity = iota; SeverityWarn; SeverityError; SeverityFatal )

type Entry struct {
    Code       Code
    Severity   Severity
    Message    string
    NextAction string
}

func MustRegister(e Entry)            // codeRe + 空串 + 重复三重防御 panic
func Lookup(c Code) (Entry, bool)
func Registry() map[Code]Entry        // 浅拷贝
func Format(c Code, args ...any) string  // "[<CODE>] msg\n  建议: next_action"

var codeRe = regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`)
```

From internal/cloudclaude/errcodes/mount.go（样板）:
```go
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
    // ... 更多
}
```

From cmd/cloud-claude/main.go (line 93 / 97-102):
```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())

if len(os.Args) > 1 {
    switch os.Args[1] {
    case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":
        rootCmd.DisableFlagParsing = false
    }
}
```

From internal/controlplane/http/admin_claude_accounts.go（Phase 33 D-21 硬编码字面量）:
```go
// 字面量 "STATE_VOLUME_IN_USE_001" 已硬编码在此 handler，本 plan 仅补录到 Registry（不改 handler）
```
</interfaces>
</context>

<tasks>

<task type="execute">
  <name>Task 1.1: errcodes/codes.go 常量块追加 17 个新 Code 字面量（CONTEXT D-21 / D-23）</name>
  <files>internal/cloudclaude/errcodes/codes.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/codes.go（特别是 line 119-146 const 块）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-21（17 条新码完整表）+ §D-23（8 域闭合）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.1..§1.5（五域文件模板 + adapt 表）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §2.1（errcodes 注册示范复制要点）
  </read_first>

  <action>
在 `internal/cloudclaude/errcodes/codes.go` 的常量块（line 119-146，以 `NET_TCP_KEEPALIVE_UNSUPPORTED` 结尾）**内部、闭合右括号 `)` 之前**追加如下 17 行常量字面量（**变量名与字面值必须逐字符一致**，否则 `MustRegister` 编译期匹配不上）：

```go
	// Phase 34 D-21 新增：8 域闭合第 4-8 域（STATE / SYSTEM / SSH / AUTH / DISK）

	// STATE_* （持久化 / volume / 容器状态）
	STATE_LAST_SESSION_MISSING   Code = "STATE_LAST_SESSION_MISSING"
	STATE_VOLUME_IN_USE_001      Code = "STATE_VOLUME_IN_USE_001"
	STATE_CONTAINER_NOT_RUNNING  Code = "STATE_CONTAINER_NOT_RUNNING"

	// SYSTEM_* （OS / kernel / FUSE / DNS / timeout）
	SYSTEM_APPARMOR_FUSERMOUNT3_MISSING Code = "SYSTEM_APPARMOR_FUSERMOUNT3_MISSING"
	SYSTEM_FUSE_RESIDUAL_MOUNT          Code = "SYSTEM_FUSE_RESIDUAL_MOUNT"
	SYSTEM_DNS_RESOLVE_FAILED           Code = "SYSTEM_DNS_RESOLVE_FAILED"
	SYSTEM_CHECK_TIMEOUT                Code = "SYSTEM_CHECK_TIMEOUT"

	// SSH_* （known_hosts / sshd 基线漂移）
	SSH_KNOWN_HOSTS_CONFLICT   Code = "SSH_KNOWN_HOSTS_CONFLICT"
	SSH_SSHD_KEEPALIVE_DRIFT   Code = "SSH_SSHD_KEEPALIVE_DRIFT"

	// AUTH_* （CLI 配置 / Entry token / OAuth refresh）
	AUTH_CONFIG_MISSING         Code = "AUTH_CONFIG_MISSING"
	AUTH_GATEWAY_UNREACHABLE    Code = "AUTH_GATEWAY_UNREACHABLE"
	AUTH_TOKEN_EXPIRED          Code = "AUTH_TOKEN_EXPIRED"
	AUTH_OAUTH_REFRESH_FAILED   Code = "AUTH_OAUTH_REFRESH_FAILED"

	// NET_* 扩展（doctor network.egress_ip 检查）
	NET_EGRESS_IP_DRIFT Code = "NET_EGRESS_IP_DRIFT"

	// DISK_* （本地 / 容器 disk usage）
	DISK_LOCAL_LOW          Code = "DISK_LOCAL_LOW"
	DISK_CONTAINER_LOW      Code = "DISK_CONTAINER_LOW"
	DISK_MUTAGEN_DATA_BLOAT Code = "DISK_MUTAGEN_DATA_BLOAT"
```

注意：`NET_EGRESS_IP_DRIFT` 属于 NET_* 前缀扩展（不单独建 `net_egress.go`，Task 1.4 会把它登记到 `errcodes/auth.go` 的 init 里——auth 维度的 gateway/egress 检查天然归属同一 domain_group；或登记到单独 `network.go`，executor 二选一，PLAN 允许——acceptance_criteria 仅校验 `Lookup(NET_EGRESS_IP_DRIFT)` 命中）。

**禁止**：
- 改动既有 26 行常量顺序或任意变量名
- 在 const 块外新建独立 `const (...)` 块
  </action>

  <acceptance_criteria>
    - `grep -c "STATE_LAST_SESSION_MISSING\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "STATE_VOLUME_IN_USE_001\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "SYSTEM_APPARMOR_FUSERMOUNT3_MISSING\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "SYSTEM_CHECK_TIMEOUT\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "SSH_KNOWN_HOSTS_CONFLICT\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "SSH_SSHD_KEEPALIVE_DRIFT\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "AUTH_CONFIG_MISSING\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "AUTH_OAUTH_REFRESH_FAILED\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "NET_EGRESS_IP_DRIFT\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "DISK_LOCAL_LOW\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "DISK_CONTAINER_LOW\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `grep -c "DISK_MUTAGEN_DATA_BLOAT\s*Code\s*=" internal/cloudclaude/errcodes/codes.go` 输出 = 1
    - `go build ./internal/cloudclaude/errcodes/...` 退出码 = 0（既有 init() 不引用新常量故 build 独立通过；Task 1.2-1.4 补完 init 后 MustRegister 正则 + 防重断言再次 build 验证）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.2: 新建 errcodes/state.go — STATE_* 3 条（CONTEXT D-21 + D-27 补 STATE_VOLUME_IN_USE_001 / PATTERNS §1.1）</name>
  <files>internal/cloudclaude/errcodes/state.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/mount.go（样板，line 1-91）
    - internal/cloudclaude/errcodes/net.go（NET_OAUTH_* 样板参考 Severity 选择）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-21（STATE 3 条字面量）+ §D-27（Phase 33 STATE_VOLUME_IN_USE_001 字面量不变）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.1（state.go adapt 表，逐字符 Message/NextAction）
    - internal/controlplane/http/admin_claude_accounts.go（仅查看 `STATE_VOLUME_IN_USE_001` 硬编码位置，不改动）
  </read_first>

  <action>
新建 `internal/cloudclaude/errcodes/state.go`，**逐字符复刻 mount.go 头部 + init 模式**：

```go
package errcodes

// STATE_* 错误码注册（Phase 34 D-21 + D-27）。
// STATE_VOLUME_IN_USE_001 字面量与 Phase 33 admin_claude_accounts.go 硬编码保持一致（D-27 兼容已部署 frontend）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       STATE_LAST_SESSION_MISSING,
		Severity:   SeverityInfo,
		Message:    "未找到上次会话快照（%s）",
		NextAction: "首次运行 cloud-claude 后再 doctor 即可看到",
	})

	MustRegister(Entry{
		Code:       STATE_VOLUME_IN_USE_001,
		Severity:   SeverityError,
		Message:    "持久化 volume %s 仍被容器持有，DELETE 拒绝",
		NextAction: "先停止容器：cloud-claude admin hosts stop <id>",
	})

	MustRegister(Entry{
		Code:       STATE_CONTAINER_NOT_RUNNING,
		Severity:   SeverityWarn,
		Message:    "主机 %s 状态为 %s，远端 doctor 检查跳过",
		NextAction: "运行 cloud-claude admin hosts start <id> 启动容器",
	})
}
```

**verbatim 守恒**（不得微调）：
- 文件头三行注释 + `//nolint:lll` 指令（样板一致）
- 每条 `Code: XXX,` 变量名与 Task 1.1 定义的 const 严格一致
- `STATE_VOLUME_IN_USE_001` 字面量值保持 Phase 33 已部署的 `STATE_VOLUME_IN_USE_001`（不允许重命名，D-27）
- NextAction ≤ 80 runes（codes_test.go:43 强制断言）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/errcodes/state.go` 退出码 = 0
    - `grep -c "^package errcodes$" internal/cloudclaude/errcodes/state.go` 输出 = 1
    - `grep -c "^//nolint:lll" internal/cloudclaude/errcodes/state.go` 输出 = 1
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/state.go` 输出 = 3
    - `grep -q "Code:\s*STATE_LAST_SESSION_MISSING" internal/cloudclaude/errcodes/state.go` 命中
    - `grep -q "Code:\s*STATE_VOLUME_IN_USE_001" internal/cloudclaude/errcodes/state.go` 命中
    - `grep -q "Code:\s*STATE_CONTAINER_NOT_RUNNING" internal/cloudclaude/errcodes/state.go` 命中
    - `grep -q "SeverityInfo" internal/cloudclaude/errcodes/state.go` 命中（STATE_LAST_SESSION_MISSING）
    - `grep -q "SeverityError" internal/cloudclaude/errcodes/state.go` 命中（STATE_VOLUME_IN_USE_001）
    - `grep -q "SeverityWarn" internal/cloudclaude/errcodes/state.go` 命中（STATE_CONTAINER_NOT_RUNNING）
    - `go build ./internal/cloudclaude/errcodes/...` 退出码 = 0（init 运行无 panic）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.3: 新建 errcodes/system.go — SYSTEM_* 4 条（CONTEXT D-21 / PATTERNS §1.2）</name>
  <files>internal/cloudclaude/errcodes/system.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/mount.go（样板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-21 SYSTEM 4 条表
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.2（字面 Message/NextAction）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §3.4 fuse_residual / §4.5 DNS flush / §8.3 AppArmor 5-Gate
  </read_first>

  <action>
新建 `internal/cloudclaude/errcodes/system.go`：

```go
package errcodes

// SYSTEM_* 错误码注册（Phase 34 D-21）。
// 覆盖 OS / kernel / FUSE / DNS / timeout 类；由 doctor network + mount 维度命中。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SYSTEM_APPARMOR_FUSERMOUNT3_MISSING,
		Severity:   SeverityError,
		Message:    "AppArmor 缺 fusermount3 override（%s）",
		NextAction: "按 host-preflight.sh 写入 capability dac_override 行",
	})

	MustRegister(Entry{
		Code:       SYSTEM_FUSE_RESIDUAL_MOUNT,
		Severity:   SeverityWarn,
		Message:    "发现 %d 个残留 FUSE 挂载: %s",
		NextAction: "运行 cloud-claude doctor mount --fix 自动解挂",
	})

	MustRegister(Entry{
		Code:       SYSTEM_DNS_RESOLVE_FAILED,
		Severity:   SeverityError,
		Message:    "DNS 解析 %s 失败: %s",
		NextAction: "运行 cloud-claude doctor network --fix 刷新 DNS 缓存",
	})

	MustRegister(Entry{
		Code:       SYSTEM_CHECK_TIMEOUT,
		Severity:   SeverityWarn,
		Message:    "检查 %s 超时（>%s）",
		NextAction: "加 --verbose 放宽到 30s，或检查远端容器状态",
	})
}
```

**verbatim 守恒：** Message 模板 + NextAction 字面量必须与 CONTEXT D-21 / PATTERNS §1.2 逐字符一致（未来 doctor check 输出时的字面量对齐依赖）。

**禁止：** 添加额外 SYSTEM_* 码（本阶段 4 条定稿；新增需 plan-checker 审批）。
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/errcodes/system.go` 退出码 = 0
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/system.go` 输出 = 4
    - `grep -q "Code:\s*SYSTEM_APPARMOR_FUSERMOUNT3_MISSING" internal/cloudclaude/errcodes/system.go` 命中
    - `grep -q "Code:\s*SYSTEM_FUSE_RESIDUAL_MOUNT" internal/cloudclaude/errcodes/system.go` 命中
    - `grep -q "Code:\s*SYSTEM_DNS_RESOLVE_FAILED" internal/cloudclaude/errcodes/system.go` 命中
    - `grep -q "Code:\s*SYSTEM_CHECK_TIMEOUT" internal/cloudclaude/errcodes/system.go` 命中
    - `grep -q 'NextAction: "运行 cloud-claude doctor mount --fix 自动解挂"' internal/cloudclaude/errcodes/system.go` 命中（verbatim 锚点）
    - `grep -q 'NextAction: "运行 cloud-claude doctor network --fix 刷新 DNS 缓存"' internal/cloudclaude/errcodes/system.go` 命中
    - `go build ./internal/cloudclaude/errcodes/...` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.4: 新建 errcodes/ssh.go + errcodes/auth.go + errcodes/disk.go（SSH 2 + AUTH 4 + DISK 3 + NET_EGRESS_IP_DRIFT 1 = 10 条）</name>
  <files>internal/cloudclaude/errcodes/ssh.go, internal/cloudclaude/errcodes/auth.go, internal/cloudclaude/errcodes/disk.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/mount.go（样板）
    - internal/cloudclaude/errcodes/net.go（NET_OAUTH_* 三态样板，auth 维度语义参考）
    - internal/cloudclaude/errcodes/session.go（SSH 相关 Severity 选择参考）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-21 SSH 2 + AUTH 4 + DISK 3 + NET 1 条表
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.3 / §1.4 / §1.5（逐字 Message/NextAction）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §9 第 7 条（AUTH_OAUTH_REFRESH_FAILED.NextAction 与 NET_OAUTH_EXPIRED / NET_OAUTH_NOT_FOUND 保持一致）
  </read_first>

  <action>
**(a) 新建 `internal/cloudclaude/errcodes/ssh.go`（SSH 2 条）：**

```go
package errcodes

// SSH_* 错误码注册（Phase 34 D-21）。known_hosts 冲突 + sshd 基线漂移。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SSH_KNOWN_HOSTS_CONFLICT,
		Severity:   SeverityWarn,
		Message:    "~/.ssh/known_hosts 中 %s 的 fingerprint 与本次握手不一致",
		NextAction: "运行 cloud-claude doctor ssh --fix 自动 ssh-keygen -R",
	})

	MustRegister(Entry{
		Code:       SSH_SSHD_KEEPALIVE_DRIFT,
		Severity:   SeverityWarn,
		Message:    "远端 sshd ClientAlive 配置 (%s) 与基线 (15/8) 不一致",
		NextAction: "重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）",
	})
}
```

**(b) 新建 `internal/cloudclaude/errcodes/auth.go`（AUTH 4 条 + NET_EGRESS_IP_DRIFT 1 条）：**

```go
package errcodes

// AUTH_* 错误码注册（Phase 34 D-21）。本地 config / Entry token / OAuth refresh。
// 同文件附带 NET_EGRESS_IP_DRIFT（doctor network.egress_ip_visible 命中 → 与 auth/gateway 语义同组）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       AUTH_CONFIG_MISSING,
		Severity:   SeverityFatal,
		Message:    "~/.cloud-claude/config.yaml 不存在或解析失败: %s",
		NextAction: "运行 cloud-claude init 重新配置网关与凭证",
	})

	MustRegister(Entry{
		Code:       AUTH_GATEWAY_UNREACHABLE,
		Severity:   SeverityError,
		Message:    "网关 %s 不可达: %s",
		NextAction: "检查网络与 gateway 配置，或运行 cloud-claude init",
	})

	MustRegister(Entry{
		Code:       AUTH_TOKEN_EXPIRED,
		Severity:   SeverityWarn,
		Message:    "Entry API token 已过期或 401: %s",
		NextAction: "运行 cloud-claude doctor auth --fix 自动刷新",
	})

	MustRegister(Entry{
		Code:       AUTH_OAUTH_REFRESH_FAILED,
		Severity:   SeverityError,
		Message:    "Claude OAuth 刷新失败: %s",
		NextAction: "在容器内运行 cloud-claude exec claude login 重新登录",
	})

	MustRegister(Entry{
		Code:       NET_EGRESS_IP_DRIFT,
		Severity:   SeverityWarn,
		Message:    "容器出口 IP (%s) 与 Entry API 期望值 (%s) 不一致",
		NextAction: "检查代理出口配置，或运行 cloud-claude doctor network",
	})
}
```

**(c) 新建 `internal/cloudclaude/errcodes/disk.go`（DISK 3 条）：**

```go
package errcodes

// DISK_* 错误码注册（Phase 34 D-21）。本地 + 容器 disk usage 警戒线。
// 阈值（500MB / 100MB / 1GB）硬编码在 doctor/disk.go，不进 Message（CONTEXT Discretion §5）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       DISK_LOCAL_LOW,
		Severity:   SeverityWarn,
		Message:    "本地 ~/.cloud-claude/ 可用空间 %dMB，低于警戒线",
		NextAction: "清理 ~/.cloud-claude/mutagen/ 或释放本地磁盘",
	})

	MustRegister(Entry{
		Code:       DISK_CONTAINER_LOW,
		Severity:   SeverityWarn,
		Message:    "容器内 /workspace 可用空间 %dMB，低于警戒线",
		NextAction: "清理容器内大文件，或联系管理员扩容 volume",
	})

	MustRegister(Entry{
		Code:       DISK_MUTAGEN_DATA_BLOAT,
		Severity:   SeverityWarn,
		Message:    "Mutagen 数据目录 ~/.cloud-claude/mutagen/ 已达 %s，超过 1GB 警戒线",
		NextAction: "运行 mutagen daemon stop && rm -rf ~/.cloud-claude/mutagen/sessions/",
	})
}
```

**verbatim 守恒：** 所有 Message / NextAction 字面量必须与 CONTEXT D-21 / PATTERNS §1.3-§1.5 对齐，不允许改写。

**禁止：**
- 在 auth.go 外建独立 `network.go` 注册 `NET_EGRESS_IP_DRIFT`（本 plan 选定 auth.go 内登记，同一 init 避免跨文件 init 顺序依赖）
- NextAction 超过 80 runes（codes_test.go:43 断言）
- `AUTH_OAUTH_REFRESH_FAILED.NextAction` 与 `net.go` 中 `NET_OAUTH_EXPIRED.NextAction` 不一致（RESEARCH §9 第 7 条要求字面量一致）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/errcodes/ssh.go && test -f internal/cloudclaude/errcodes/auth.go && test -f internal/cloudclaude/errcodes/disk.go` 退出码 = 0
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/ssh.go` 输出 = 2
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/auth.go` 输出 = 5（AUTH 4 + NET_EGRESS_IP_DRIFT 1）
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/disk.go` 输出 = 3
    - `grep -q "Code:\s*SSH_KNOWN_HOSTS_CONFLICT" internal/cloudclaude/errcodes/ssh.go` 命中
    - `grep -q "Code:\s*SSH_SSHD_KEEPALIVE_DRIFT" internal/cloudclaude/errcodes/ssh.go` 命中
    - `grep -q "Code:\s*AUTH_CONFIG_MISSING" internal/cloudclaude/errcodes/auth.go` 命中
    - `grep -q "Code:\s*AUTH_OAUTH_REFRESH_FAILED" internal/cloudclaude/errcodes/auth.go` 命中
    - `grep -q "Code:\s*NET_EGRESS_IP_DRIFT" internal/cloudclaude/errcodes/auth.go` 命中
    - `grep -q "Code:\s*DISK_LOCAL_LOW" internal/cloudclaude/errcodes/disk.go` 命中
    - `grep -q "Code:\s*DISK_MUTAGEN_DATA_BLOAT" internal/cloudclaude/errcodes/disk.go` 命中
    - `grep -q 'NextAction: "在容器内运行 cloud-claude exec claude login 重新登录"' internal/cloudclaude/errcodes/auth.go` 命中（字面量与 net.go NET_OAUTH_* 一致）
    - `go build ./internal/cloudclaude/errcodes/...` 退出码 = 0（所有 init panic 防御通过）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.5: 新建 errcodes/explanations.go — ExtendedExplanations + ExplainExempt + registerExplanation（CONTEXT D-02 / D-18 / PATTERNS §1.6）</name>
  <files>internal/cloudclaude/errcodes/explanations.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/codes.go（line 59-77 MustRegister 三段 panic 防御模板）
    - internal/cloudclaude/errcodes/mount.go + session.go + net.go（既有 Code 字面量完整清单）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-02（ExtendedExplanations + ExplainExempt）+ §D-18（5 段 ≥ 200 字符模板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.6（verbatim 防御 panic 模式 + 示例 entry）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §6.2 TestAllCodesHaveExplanations 断言要求
    - .planning/research/PITFALLS.md C2 / C4 / C6 / C8 / M13 / M14（长说明「关联文档」字段引用源）
  </read_first>

  <action>
新建 `internal/cloudclaude/errcodes/explanations.go`（结构完整，init 内注册至少 **30 条** entry：17 条新 Phase 34 + 既有 13 条非 informational 码，见下文完整清单）：

**文件头 + 数据结构 + registerExplanation helper（**verbatim 复刻 PATTERNS §1.6**）：**

```go
// Package errcodes — Phase 34 D-02 / D-18：
// 为 cloud-claude explain 子命令提供每个非 informational Code 的长说明。
// ExplainExempt 登记 informational 类（不需要长说明）。
//
//nolint:lll // 长说明字面量不折行
package errcodes

import (
	"fmt"
	"sync"
)

var (
	explainMu            sync.RWMutex
	ExtendedExplanations = map[Code]string{}

	// ExplainExempt：informational 类豁免长说明（Severity==SeverityInfo 的降级提示 / APFS 识别 / *_BACKOFF / *_NOTIFIED 等）。
	ExplainExempt = map[Code]struct{}{
		MOUNT_APFS_CASE_INSENSITIVE: {},
		MOUNT_AUTO_DOWNGRADED:       {},
		SESSION_TAKEOVER_NOTIFIED:   {},
		NET_RECONNECT_BACKOFF:       {},
		STATE_LAST_SESSION_MISSING:  {},
	}
)

// registerExplanation 与 MustRegister 同语义防御重复注册。
// 由 init() 调用，问题在进程启动时即暴露（与 MustRegister 对齐）。
func registerExplanation(c Code, text string) {
	if text == "" {
		panic(fmt.Sprintf("errcodes: code %q ExtendedExplanations 不能为空", c))
	}
	explainMu.Lock()
	defer explainMu.Unlock()
	if _, exists := ExtendedExplanations[c]; exists {
		panic(fmt.Sprintf("errcodes: 重复注册 ExtendedExplanations %q", c))
	}
	ExtendedExplanations[c] = text
}
```

**init() 内必须注册以下 ≥ 30 条 Code 的长说明**（每条 ≥ 200 中文字符，按 CONTEXT D-18 五段模板：触发场景 / 根本原因 / 复现方式（可选）/ 修复路径 / 关联文档）：

**Phase 34 新增 16 条（除 STATE_LAST_SESSION_MISSING 已豁免）：**
1. `STATE_VOLUME_IN_USE_001`
2. `STATE_CONTAINER_NOT_RUNNING`
3. `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING`
4. `SYSTEM_FUSE_RESIDUAL_MOUNT`
5. `SYSTEM_DNS_RESOLVE_FAILED`
6. `SYSTEM_CHECK_TIMEOUT`
7. `SSH_KNOWN_HOSTS_CONFLICT`
8. `SSH_SSHD_KEEPALIVE_DRIFT`
9. `AUTH_CONFIG_MISSING`
10. `AUTH_GATEWAY_UNREACHABLE`
11. `AUTH_TOKEN_EXPIRED`
12. `AUTH_OAUTH_REFRESH_FAILED`
13. `NET_EGRESS_IP_DRIFT`
14. `DISK_LOCAL_LOW`
15. `DISK_CONTAINER_LOW`
16. `DISK_MUTAGEN_DATA_BLOAT`

**既有 Phase 31/32/33 非 informational 码 14 条（排除 4 条 ExplainExempt）：**
17. `MOUNT_MUTAGEN_VERSION_SKEW`（PATTERNS §1.6 已给样板，直接用）
18. `MOUNT_MUTAGEN_WHITELIST_REJECT`
19. `MOUNT_MUTAGEN_SAFETY_GUARD`
20. `MOUNT_MUTAGEN_DAEMON_UNAVAILABLE`
21. `MOUNT_MUTAGEN_SYNC_FAILED`
22. `MOUNT_MUTAGEN_TRANSPORT_FAILED`
23. `MOUNT_SSHFS_FAILED`
24. `MOUNT_SSHFS_DISCONNECTED`
25. `MOUNT_MERGERFS_FAILED`
26. `MOUNT_FORCE_MODE_FAILED`
27. `NET_OAUTH_EXPIRED`
28. `NET_OAUTH_EXPIRING_SOON`
29. `NET_OAUTH_NOT_FOUND`
30. `SESSION_KEEPALIVE_TOO_AGGRESSIVE`
31. `SESSION_TMUX_UNAVAILABLE`
32. `SESSION_NOT_FOUND`
33. `SESSION_TAKEOVER_FAILED`
34. `SESSION_SYNC_LOCKED`
35. `SESSION_BUFFER_OVERFLOW`
36. `NET_RECONNECT_GAVE_UP`
37. `NET_TCP_KEEPALIVE_UNSUPPORTED`

**每条按以下模板（≥ 200 中文字符）：**

```go
	registerExplanation(MOUNT_MUTAGEN_VERSION_SKEW, `触发场景：cloud-claude 客户端 embed 的 Mutagen 二进制版本与容器内 /etc/cloud-claude/mutagen.version 不匹配。
根本原因：Mutagen agent / client 协议版本必须严格一致，否则 sync session 创建会握手失败。cloud-claude 在启动热同步前会做 TrimPrefix+Contains 双保险比对，版本漂移立即降级到 sshfs-only。
复现方式：docker exec <ctr> sed -i 's/v0.18.1/v0.99.99/' /etc/cloud-claude/mutagen.version
修复路径：升级容器镜像到 v3.0.0+（含 mutagen v0.18.1 agent），或重装 cloud-claude；也可运行 cloud-claude doctor mount --fix 自动重启 daemon 复测。
关联文档：.planning/research/PITFALLS.md C4`)
```

**executor 按上述模板为所有 ≥ 30 条 Code 写长说明**，字面量参考每条 Code 对应的 Severity / Message 场景自然语言展开；每条 `utf8.RuneCountInString(text) >= 200`（explanations_test.go 强制断言）。

**禁止：**
- 使用 tab 而非空格缩进（保持文件 gofmt 稳定）
- 在 init() 中 `ExtendedExplanations[c] = text` 直接赋值（必须走 `registerExplanation` 防御重复）
- 为 `ExplainExempt` 中的 5 条 Code 再注册 ExtendedExplanations（避免冗余）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/errcodes/explanations.go` 退出码 = 0
    - `grep -c "^package errcodes$" internal/cloudclaude/errcodes/explanations.go` 输出 = 1
    - `grep -q "var\s*ExtendedExplanations\s*=\s*map\[Code\]string" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "ExplainExempt\s*=\s*map\[Code\]struct{}" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "func registerExplanation(c Code, text string)" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "panic(fmt.Sprintf(\"errcodes: 重复注册 ExtendedExplanations" internal/cloudclaude/errcodes/explanations.go` 命中（防御三段式）
    - `grep -c "registerExplanation(" internal/cloudclaude/errcodes/explanations.go` ≥ 31（1 函数定义 + ≥ 30 注册调用）
    - `grep -q "registerExplanation(MOUNT_MUTAGEN_VERSION_SKEW" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "registerExplanation(STATE_VOLUME_IN_USE_001" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "registerExplanation(AUTH_OAUTH_REFRESH_FAILED" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "registerExplanation(DISK_MUTAGEN_DATA_BLOAT" internal/cloudclaude/errcodes/explanations.go` 命中
    - `grep -q "MOUNT_APFS_CASE_INSENSITIVE: {}" internal/cloudclaude/errcodes/explanations.go` 命中（ExplainExempt）
    - `grep -q "STATE_LAST_SESSION_MISSING:\s*{}" internal/cloudclaude/errcodes/explanations.go` 命中（ExplainExempt）
    - `go build ./internal/cloudclaude/errcodes/...` 退出码 = 0（init panic 防御通过）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.6: 新建 errcodes/explanations_test.go + 扩展 codes_test.go（RESEARCH §6.2 三断言 + 下限 30）</name>
  <files>internal/cloudclaude/errcodes/explanations_test.go, internal/cloudclaude/errcodes/codes_test.go</files>

  <read_first>
    - internal/cloudclaude/errcodes/codes_test.go（既有 5 条测试，line 1-85）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §6.2（TestAllCodesHaveExplanations + TestNoLegacyLowercaseCodes + TestAllDomainsClosed 三份完整 Go 代码）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §1.7（explanations_test.go 结构）+ §1.8（codes_test.go 下限调整）
  </read_first>

  <action>
**(a) 修改 `internal/cloudclaude/errcodes/codes_test.go`（**仅 1 处）：** 把 line 13-15：

```go
	if len(reg) < 15 {
		t.Fatalf("注册表条目不足：want >= 15, got %d", len(reg))
	}
```

改为：

```go
	// Phase 34 D-21：17 条新 + 25 条既有 = ≥ 42 条；下限放宽到 30 留余量。
	if len(reg) < 30 {
		t.Fatalf("注册表条目不足：want >= 30, got %d", len(reg))
	}
```

**其它既有测试一字不动。**

**(b) 新建 `internal/cloudclaude/errcodes/explanations_test.go`（**verbatim 复制 RESEARCH §6.2 三份断言**）：**

```go
package errcodes

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestAllCodesHaveExplanations 遍历 Registry：每条 Code 在 ExtendedExplanations 或 ExplainExempt 中至少出现一次；
// 非豁免码长说明 ≥ 200 中文字符（CONTEXT D-18 / RESEARCH §6.2）。
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
		if n := utf8.RuneCountInString(exp); n < 200 {
			t.Errorf("code %s ExtendedExplanations 长度 %d < 200 中文字符", code, n)
		}
	}
}

// TestNoLegacyLowercaseCodes 防御 PITFALLS C8：v2.0 现网 lower-case 字面量禁出现在 Registry（CONTEXT D-22）。
func TestNoLegacyLowercaseCodes(t *testing.T) {
	legacy := []string{
		"auth_failed",
		"auth_expired",
		"entry_token_expired",
		"host_action_failed",
		"entry_config_missing",
	}
	for code := range Registry() {
		for _, bad := range legacy {
			if string(code) == bad {
				t.Errorf("v2.0 lower-case 字面量 %q 不应出现在 Registry（D-22）", bad)
			}
		}
	}
}

// TestAllDomainsClosed 断言 8 域闭合（CONTEXT D-23）：
//   DOMAIN ∈ {MOUNT, SESSION, NET, STATE, SYSTEM, SSH, AUTH, DISK}
func TestAllDomainsClosed(t *testing.T) {
	allowed := map[string]bool{
		"MOUNT":  true,
		"SESSION": true,
		"NET":    true,
		"STATE":  true,
		"SYSTEM": true,
		"SSH":    true,
		"AUTH":   true,
		"DISK":   true,
	}
	for code := range Registry() {
		idx := strings.Index(string(code), "_")
		if idx < 0 {
			t.Errorf("code %s 不含下划线，无法解析 DOMAIN", code)
			continue
		}
		domain := string(code)[:idx]
		if !allowed[domain] {
			t.Errorf("code %s DOMAIN %q 未在 8 域闭合列表", code, domain)
		}
	}
}

// TestExplainExemptOnlyInformational 防御：ExplainExempt 中的 Code 必须在 Registry 有 SeverityInfo（或不存在，兼容未来扩展）；
// 禁止把高严重度 code 误放进豁免集合。
func TestExplainExemptOnlyInformational(t *testing.T) {
	for code := range ExplainExempt {
		entry, ok := Lookup(code)
		if !ok {
			// 未注册 code 放进 ExplainExempt 无害（但应删），仅 warn
			t.Logf("warn: ExplainExempt 中 %s 在 Registry 未找到（可考虑移除）", code)
			continue
		}
		if entry.Severity != SeverityInfo {
			t.Errorf("ExplainExempt 不应包含非 Info Code %s（Severity=%s）", code, entry.Severity)
		}
	}
}
```

**禁止：**
- 在 explanations_test.go 中使用 `init()` 修改 Registry（保持 pure functional 断言）
- 在 codes_test.go 中删改既有 5 条测试
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/errcodes/explanations_test.go` 退出码 = 0
    - `grep -q "if len(reg) < 30" internal/cloudclaude/errcodes/codes_test.go` 命中
    - `grep -q "func TestAllCodesHaveExplanations" internal/cloudclaude/errcodes/explanations_test.go` 命中
    - `grep -q "func TestNoLegacyLowercaseCodes" internal/cloudclaude/errcodes/explanations_test.go` 命中
    - `grep -q "func TestAllDomainsClosed" internal/cloudclaude/errcodes/explanations_test.go` 命中
    - `grep -q "func TestExplainExemptOnlyInformational" internal/cloudclaude/errcodes/explanations_test.go` 命中
    - `grep -qE '"MOUNT":\s*true' internal/cloudclaude/errcodes/explanations_test.go` 命中（8 域字面量）
    - `grep -qE '"DISK":\s*true' internal/cloudclaude/errcodes/explanations_test.go` 命中
    - `go test ./internal/cloudclaude/errcodes/ -run "TestErrcodesRegistry|TestAllCodesHaveExplanations|TestNoLegacyLowercaseCodes|TestAllDomainsClosed|TestExplainExemptOnlyInformational" -count=1 -v` 退出码 = 0
    - 既有测试不回归：`go test ./internal/cloudclaude/errcodes/ -count=1` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.7: 新建 cmd/cloud-claude/explain.go + 修改 main.go 注册子命令（CONTEXT D-03 / D-17 / PATTERNS §3.2）</name>
  <files>cmd/cloud-claude/explain.go, cmd/cloud-claude/main.go</files>

  <read_first>
    - cmd/cloud-claude/main.go（特别是 line 85-102 AddCommand + switch os.Args[1]）
    - cmd/cloud-claude/sessions.go（line 22-47 newSessionsCmd / line 37-44 attachCmd ExactArgs(1) 三段式）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-03 / §D-17（explain 三段输出 + exit 4）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §3.2（explain.go verbatim 模式）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §8.4（大小写敏感必须保留，禁止自动修正）
  </read_first>

  <action>
**(a) 新建 `cmd/cloud-claude/explain.go`：**

```go
// cmd/cloud-claude/explain.go — Phase 34 Plan 01 Task 1.7
//
// cloud-claude explain <code> 子命令：对标 rustc --explain。
// 数据源 = errcodes.Lookup + errcodes.Format + errcodes.ExtendedExplanations。
// 大小写敏感匹配（CONTEXT D-17 / RESEARCH §8.4），未注册 code exit 4 (= exitConfigError)。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

func newExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "explain <code>",
		Short:         "解释 cloud-claude 错误码（对标 rustc --explain）",
		Long:          "对给定错误码（大小写敏感）输出统一三要素 + 详细中文说明 + 修复路径。\n未注册错误码返回 exit 4。",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runExplain,
	}
	cmd.Flags().Bool("verbose", false, "输出关联的 doctor check 名（如已登记）")
	return cmd
}

func runExplain(cmd *cobra.Command, args []string) error {
	code := errcodes.Code(args[0])

	// 段 1：Format(code) 两段（错误码 + 中文原因 + 建议）
	entry, ok := errcodes.Lookup(code)
	if !ok {
		fmt.Fprintf(os.Stderr, "未找到错误码 %s；运行 cloud-claude doctor 查看可用检查项\n", args[0])
		os.Exit(exitConfigError)
		return nil
	}
	fmt.Println(errcodes.Format(code))

	// 段 2：空行 + 详细说明
	fmt.Println()
	fmt.Println("详细说明：")
	if exp, hasExp := errcodes.ExtendedExplanations[code]; hasExp {
		fmt.Println(exp)
	} else {
		fmt.Println("未提供详细说明，运行 cloud-claude doctor <domain> 查看相关检查项")
	}

	// 段 3（--verbose）：Severity + 未来 RelatedChecks（Plan 02 optional explain_index.go）
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		fmt.Println()
		fmt.Printf("Severity: %s\n", entry.Severity)
		// RelatedChecks 由 doctor 包 optional 提供；此处保留扩展点
	}
	return nil
}
```

**(b) 修改 `cmd/cloud-claude/main.go`：**

把 line 93 的：

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())
```

改为（**在末尾追加 `newExplainCmd()`**）：

```go
rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd(), newExplainCmd())
```

把 line 97-102 的 switch 改为（**在 case 列表末尾追加 `"explain"`**）：

```go
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init", "env", "ssh", "sync", "sessions", "explain", "help", "--help", "-h":
			rootCmd.DisableFlagParsing = false
		}
	}
```

**verbatim 守恒：**
- 函数名 `newExplainCmd` / `runExplain`
- Short 文案 `"解释 cloud-claude 错误码（对标 rustc --explain）"`
- 未找到 stderr 模板 `"未找到错误码 %s；运行 cloud-claude doctor 查看可用检查项\n"`（与 CONTEXT D-17 字面一致，CI grep 依赖）
- `os.Exit(exitConfigError)` — 复用 main.go:23 的 `exitConfigError = 4` 常量（不新增常量）
- fallback 文案 `"未提供详细说明，运行 cloud-claude doctor <domain> 查看相关检查项"`

**禁止：**
- 大小写自动修正（`strings.ToUpper(args[0])` 等）— PITFALLS C8 / RESEARCH §8.4
- 在 explain 子命令内做 stdin 交互 / 模糊匹配（CONTEXT deferred）
- 修改既有 `exitInternalError` / `runRoot` 等，只加不改
  </action>

  <acceptance_criteria>
    - `test -f cmd/cloud-claude/explain.go` 退出码 = 0
    - `grep -q "^func newExplainCmd()" cmd/cloud-claude/explain.go` 命中
    - `grep -q "^func runExplain(cmd \*cobra.Command, args \[\]string) error" cmd/cloud-claude/explain.go` 命中
    - `grep -q "cobra.ExactArgs(1)" cmd/cloud-claude/explain.go` 命中
    - `grep -q 'errcodes.Lookup(errcodes.Code(args\[0\]))' cmd/cloud-claude/explain.go` 命中
    - `grep -q '"未找到错误码 %s；运行 cloud-claude doctor 查看可用检查项\\\\n"' cmd/cloud-claude/explain.go` 命中
    - `grep -q "os.Exit(exitConfigError)" cmd/cloud-claude/explain.go` 命中
    - `grep -q "newExplainCmd()" cmd/cloud-claude/main.go` 命中
    - `grep -qE 'case "init", "env", "ssh", "sync", "sessions", "explain",' cmd/cloud-claude/main.go` 命中
    - `go build ./cmd/cloud-claude/...` 退出码 = 0
    - `go vet ./cmd/cloud-claude/...` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 1.8: 新建 cmd/cloud-claude/explain_test.go — os/exec 子进程级测试（PATTERNS §3.3 / RESEARCH 子进程测试模板）</name>
  <files>cmd/cloud-claude/explain_test.go</files>

  <read_first>
    - cmd/cloud-claude/explain.go（Task 1.7 新建）
    - internal/cloudclaude/integration_test.go（line 86-111 `runCloudClaude` go build + exec.CommandContext + ExitError 断言模式）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §3.3（explain_test.go 测试用例）
  </read_first>

  <action>
新建 `cmd/cloud-claude/explain_test.go`：

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// buildOnceExplainBin 把 cloud-claude 编译到 /tmp/cloud-claude-explain-test，子测试共享；
// 与 internal/cloudclaude/integration_test.go:86-111 的 runCloudClaude 同 pattern。
func buildOnceExplainBin(t *testing.T) string {
	t.Helper()
	bin := "/tmp/cloud-claude-explain-test"
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	cmd := exec.Command("go", "build", "-o", bin, "./")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译 cloud-claude 失败: %v\n%s", err, out)
	}
	return bin
}

func runExplainBin(t *testing.T, bin string, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), outBuf.String(), errBuf.String()
	}
	if err != nil {
		t.Logf("cloud-claude explain 执行错误: %v", err)
		return -1, outBuf.String(), errBuf.String()
	}
	return 0, outBuf.String(), errBuf.String()
}

// TestExplain_KnownCode_Exit0 — 覆盖 REQ-F8-C / ROADMAP §Phase 34 SC#8：
// cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW 必须 exit 0 + stdout 含错误码字面量 + "建议:" 子串。
func TestExplain_KnownCode_Exit0(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, stdout, stderr := runExplainBin(t, bin, "explain", "MOUNT_MUTAGEN_VERSION_SKEW")
	if code != 0 {
		t.Fatalf("known code 应 exit 0，实际 %d；stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "MOUNT_MUTAGEN_VERSION_SKEW") {
		t.Errorf("stdout 未包含错误码字面量: %q", stdout)
	}
	if !strings.Contains(stdout, "建议:") {
		t.Errorf("stdout 未包含 '建议:' 子串（Format 两段 + NextAction）: %q", stdout)
	}
	if !strings.Contains(stdout, "详细说明：") {
		t.Errorf("stdout 未包含 '详细说明：' 段（段 2 锚点）: %q", stdout)
	}
}

// TestExplain_UnknownCode_Exit4 — 覆盖 CONTEXT D-17：
// cloud-claude explain FAKE_CODE_X 必须 exit 4 + stderr 含 "未找到错误码"。
func TestExplain_UnknownCode_Exit4(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, _, stderr := runExplainBin(t, bin, "explain", "FAKE_CODE_X")
	if code != 4 {
		t.Fatalf("unknown code 应 exit 4 (exitConfigError)，实际 %d", code)
	}
	if !strings.Contains(stderr, "未找到错误码") {
		t.Errorf("stderr 未包含 '未找到错误码' 字面量: %q", stderr)
	}
	if !strings.Contains(stderr, "FAKE_CODE_X") {
		t.Errorf("stderr 未回显原输入 FAKE_CODE_X: %q", stderr)
	}
}

// TestExplain_CaseSensitive_LowerCaseUnknown — 覆盖 RESEARCH §8.4 / PITFALLS C8：
// cloud-claude explain mount_mutagen_version_skew（小写）必须 exit 4，禁止自动修正。
func TestExplain_CaseSensitive_LowerCaseUnknown(t *testing.T) {
	bin := buildOnceExplainBin(t)
	code, _, stderr := runExplainBin(t, bin, "explain", "mount_mutagen_version_skew")
	if code != 4 {
		t.Fatalf("lower-case 输入应 exit 4（禁止自动修正），实际 %d；stderr=%q", code, stderr)
	}
}
```

**verbatim 守恒：** 三条测试函数名不变（CI 过滤用），断言字面量 `"未找到错误码"` / `"建议:"` / `"详细说明："` 与 Task 1.7 输出严格对齐。

**禁止：** `t.Parallel()`（避免二进制 build 竞争）；构建到 repo 内路径（污染 git）。
  </action>

  <acceptance_criteria>
    - `test -f cmd/cloud-claude/explain_test.go` 退出码 = 0
    - `grep -q "func TestExplain_KnownCode_Exit0" cmd/cloud-claude/explain_test.go` 命中
    - `grep -q "func TestExplain_UnknownCode_Exit4" cmd/cloud-claude/explain_test.go` 命中
    - `grep -q "func TestExplain_CaseSensitive_LowerCaseUnknown" cmd/cloud-claude/explain_test.go` 命中
    - `go test ./cmd/cloud-claude/ -run "TestExplain_" -count=1 -v` 退出码 = 0（3 条子测全 PASS，需先 build 成功）
  </acceptance_criteria>
</task>

</tasks>

<verification>
## Plan-level 验证

```bash
# 1. 全仓库构建（errcodes init + cobra 注册闭环）
go build ./...

# 2. errcodes 包完整测试（既有 5 条 + 新 4 条 = 9 条）
go test ./internal/cloudclaude/errcodes/ -count=1 -v

# 3. explain 子进程级测试（3 条）
go test ./cmd/cloud-claude/ -run TestExplain_ -count=1 -v

# 4. 既有仓库测试无回归
go test ./... -count=1 -short

# 5. errcodes Registry 基数 grep 断言（≥ 42 条）
go run -tags none ./dev/registry-count 2>/dev/null || \
  grep -cE '^\s*[A-Z_]+ *Code *= *"[A-Z_0-9]+"$' internal/cloudclaude/errcodes/codes.go
# 应输出 ≥ 42

# 6. v2.0 lower-case 字面量禁入（C8 终验）
! grep -nE '"(auth_failed|auth_expired|entry_token_expired|host_action_failed|entry_config_missing)"' \
    internal/cloudclaude/errcodes/codes.go \
    internal/cloudclaude/errcodes/state.go \
    internal/cloudclaude/errcodes/system.go \
    internal/cloudclaude/errcodes/ssh.go \
    internal/cloudclaude/errcodes/auth.go \
    internal/cloudclaude/errcodes/disk.go

# 7. ExtendedExplanations 字面量 ≥ 30 条注册
[ "$(grep -c 'registerExplanation(' internal/cloudclaude/errcodes/explanations.go)" -ge 31 ]
```

## SC 映射（ROADMAP §Phase 34 Success Criteria）

| SC | 本 Plan 交付 |
|----|-------------|
| SC#1（Registry 无重复 / 中文消息 / next_action / 命名前缀无冲突） | Task 1.1-1.4 注册 17 新码；Task 1.6 `TestAllDomainsClosed` + `TestNoLegacyLowercaseCodes` |
| SC#8（cloud-claude explain <code> 中文说明 + 修复步骤） | Task 1.5 ExtendedExplanations ≥ 30 条；Task 1.7 explain.go 三段输出；Task 1.8 三子测断言 |
| SC#9（doctor 不调用 host-agent 新 endpoint） | 本 Plan 纯 CLI + errcodes 注册，无 host-agent 交互，天然满足 |
</verification>

<success_criteria>
- [ ] 8 个 task 全部完成，各 acceptance_criteria 全 PASS
- [ ] `go build ./...` 退出码 = 0（errcodes init panic 防御通过 + cobra 注册闭环）
- [ ] errcodes 包 Registry 条目数 ≥ 42（`len(Registry()) >= 42`，含 17 条新 + 25 条既有）
- [ ] `go test ./internal/cloudclaude/errcodes/ -count=1` 退出码 = 0（9 条测试全 PASS）
- [ ] `go test ./cmd/cloud-claude/ -run TestExplain_ -count=1` 退出码 = 0（3 条子测全 PASS）
- [ ] `TestAllCodesHaveExplanations` 遍历 Registry 无 missing（除 ExplainExempt 5 条）
- [ ] `TestNoLegacyLowercaseCodes` 5 条 v2.0 字面量全部 not-in-Registry
- [ ] `TestAllDomainsClosed` 8 域闭合断言 PASS
- [ ] v3.0 现网运维无感：`STATE_VOLUME_IN_USE_001` 在 admin_claude_accounts.go 字面量不变
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| 用户输入 `cloud-claude explain <code>` args[0] → errcodes.Lookup | 用户可输入任意字符串；Lookup 内部仅做 map 查找，无 shell / 反射 |
| errcodes init() → Registry map | init 启动时执行；panic 暴露配置错误（`MustRegister` / `registerExplanation` 三重防御） |
| cobra parseArgs → main.go switch os.Args[1] | 字符串相等比较，无 shell 解释 |

## STRIDE Threat Register（Phase 34 Plan 01 是纯 CLI + 注册表，攻击面极窄）

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-34-01-01 | Tampering | `explain.go` args[0] 未校验就查 Registry | low | mitigate | `errcodes.Lookup(errcodes.Code(args[0]))` 仅 map 读取，无字符串拼 shell；`ExactArgs(1)` 限制参数数量；大小写敏感禁自动修正（RESEARCH §8.4 / PITFALLS C8 Task 1.8 `TestExplain_CaseSensitive_LowerCaseUnknown` 回归） |
| T-34-01-02 | Information Disclosure | ExtendedExplanations 长说明意外写入 OAuth 路径 / token | low | mitigate | 本阶段长说明均为**静态字符串**（无 %s 占位从用户输入注入），CI grep 断言 `! grep -nE "credentials\.json|\.claude/\.credentials" explanations.go`；审阅时手动 grep OAuth/token 关键词（acceptance 保留） |
| T-34-01-03 | DoS | `buildOnceExplainBin` 在 CI 中首次耗时 go build（~30s） | low | accept | `buildOnce` 模式复用二进制；超时 15s / test 默认 10min 充裕；rationale：测试是 dev-time 操作，非生产 |
| T-34-01-04 | Spoofing | 外部包通过 init() 顺序攻击 Registry 注册（注册非预期 code） | low | accept | Go 语言 init() 仅在本模块内执行；外部包无 import 即无法触达 `MustRegister`；`registerExplanation` 防御重复注册；rationale：同模块信任边界内 |
| T-34-01-05 | Tampering | Phase 33 硬编码字面量 `STATE_VOLUME_IN_USE_001` 被本 plan 误改 | medium | mitigate | Task 1.2 verbatim 守恒字段 `Code: STATE_VOLUME_IN_USE_001`；acceptance_criteria `grep -q "STATE_VOLUME_IN_USE_001"` 双重断言；D-27 CONTEXT 明确字面量不变，D-23 `TestAllDomainsClosed` 回归 |

**ASVS L1 高严重度阻塞性威胁：** 0
</threat_model>

<output>
After completion, create `.planning/phases/34-cloud-claude-doctor-v3/34-01-SUMMARY.md` with:
- 8 个 task 的实际 commit SHA + 关键 diff 片段引用
- `len(errcodes.Registry())` 最终数值（期望 ≥ 42）
- `ExtendedExplanations` 条目数 + `ExplainExempt` 条目数
- 3 个 `TestExplain_*` 子进程级测试 PASS 时间戳
- v2.0 lower-case grep 断言实测为空的证据
- carry-over：是否有未覆盖到 ExtendedExplanations 的既有 Code（提供给 Plan 02 doctor check 用）
</output>
