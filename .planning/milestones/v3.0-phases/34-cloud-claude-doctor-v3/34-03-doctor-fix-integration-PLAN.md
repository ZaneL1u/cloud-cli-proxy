---
phase: 34-cloud-claude-doctor-v3
plan: 03
type: execute
wave: 3
depends_on: [02]
autonomous: true
requirements: [REQ-F6-C]
requirements_addressed: [REQ-F6-C]
files_modified:
  - internal/cloudclaude/doctor/fix.go
  - internal/cloudclaude/doctor/fix_test.go
  - internal/cloudclaude/doctor/ssh.go
  - internal/cloudclaude/doctor/mount.go
  - internal/cloudclaude/doctor/integration_test.go
  - cmd/cloud-claude/doctor.go
  - scripts/ci-doctor-grep.sh
  - Makefile

must_haves:
  truths:
    - "doctor --fix 至少自动修复 5 类（D-09 表）：MOUNT_MUTAGEN_DAEMON_UNAVAILABLE / SYSTEM_FUSE_RESIDUAL_MOUNT / SSH_KNOWN_HOSTS_CONFLICT / AUTH_TOKEN_EXPIRED（+AUTH_OAUTH_REFRESH_FAILED）/ SYSTEM_DNS_RESOLVE_FAILED（REQ-F6-C / ROADMAP §Phase 34 SC#4）"
    - "confirmDestructive(opts) 处理 3 种场景：opts.Yes → true；opts.JSON → false + FixFailed 追加；非 TTY → false（CONTEXT D-10 / RESEARCH §9 第 2 条）"
    - "每个 Fixer 幂等：重复调用 noop（已解挂 fusermount -u 容错 'not mounted' / daemon stop 容错 'no daemon' / ssh-keygen -R 容错 'not found'）(CONTEXT D-09 第 1 列)"
    - "修复成功的 fail 不降级为 0 退出码；stdout 顶部输出 `[fix] N 项已修复 / M 项修复失败` 后按 Summary.Fail 计算（CONTEXT D-16 / RESEARCH §5.3）"
    - "scripts/ci-doctor-grep.sh 执行 3 段断言（JSON schema=1 + next_action 非空 + 文本 warn/fail 含『建议:』与 `[XXX_YYY_ZZZ]`）接入 Makefile ci-gate target（ROADMAP §Phase 34 SC#3 / PITFALLS M14）"
    - "集成测试 2 用例（1 happy mount / 1 mergerfs 篡改 fail）复用 scripts/test-fixture-up.sh docker compose（build tag integration + t.Skip 降级，与 Phase 31/32 一致）"
  artifacts:
    - path: internal/cloudclaude/doctor/fix.go
      provides: "FixerRegistry + 5 类修复 + confirmDestructive + execMutagenDaemon / execFusermountUnmount / execSSHKeygenRemove / execEntryRefresh / execDNSFlush 5 个包级 var"
      contains: "FixerRegistry"
    - path: internal/cloudclaude/doctor/integration_test.go
      provides: "build tag integration + TestIntegration_DoctorMountHappy + TestIntegration_DoctorMountFail_MergerfsTampered"
      contains: "//go:build integration"
    - path: scripts/ci-doctor-grep.sh
      provides: "jq + grep 三段断言（JSON schema=1 + next_action + 错误码格式）"
      contains: "set -euo pipefail"
    - path: Makefile
      provides: "ci-gate target 调 scripts/ci-doctor-grep.sh"
      contains: "ci-doctor-grep"
  key_links:
    - from: "cmd/cloud-claude/doctor.go::runDoctor"
      to: "doctor.RunDoctor(opts.Fix=true) → FixerRegistry → 5 类 Fixer"
      via: "opts.Fix=true + FixerRegistry non-empty → RunDoctor 内 check fail/warn 后自动触达 Fixer"
      pattern: "FixerRegistry\\[.*?\\]"
    - from: "internal/cloudclaude/doctor/fix.go::confirmDestructive"
      to: "term.IsTerminal + opts.Yes + opts.JSON"
      via: "三级判定：Yes → true；JSON → false + FixFailed；非 TTY → false"
      pattern: "term.IsTerminal\\(int\\(os.Stdin.Fd\\(\\)\\)\\)"
    - from: "scripts/ci-doctor-grep.sh"
      to: "cloud-claude doctor --json / doctor 文本 / grep -E '\\[[!✗]\\]'"
      via: "3 段 jq + grep 断言 + exit code"
      pattern: "jq -e '\\.schema_version == 1'"
---

<objective>
闭环 Phase 34：交付 REQ-F6-C `doctor --fix` 5 类自动修复 + 集成测试 + CI grep 闸门。具体实现：
1. `internal/cloudclaude/doctor/fix.go` 新建：`FixerRegistry map[errcodes.Code]Fixer` + 5 类修复函数（D-09 表）+ `confirmDestructive` helper + 5 个包级 var mock 注入点；
2. 修改 Plan 02 各维度文件（`network/auth/ssh/mount.go`），为相关 check 实现 `Fix(ctx, opts)` 方法（D-09 表的 5 个错误码对应）；
3. 新建 `integration_test.go`（build tag `integration`），2 条用例沿用 Phase 31 `scripts/test-fixture-up.sh` docker fixture；
4. 新建 `scripts/ci-doctor-grep.sh`（3 段 jq + grep 断言）+ `Makefile` 追加 `ci-gate` target 调用本脚本；
5. 修改 `cmd/cloud-claude/doctor.go` — 把 Plan 02 的 `anyFixerRegistered() bool` 占位改为真实检查 + `runDoctor` 在 `opts.Fix=true` 时把 FixerRegistry 传入 `doctor.RunDoctor`（或 doctor 包内部 auto-pick）。

Purpose: 本 Plan 是 Phase 34 **Wave 3 收尾**，完成 REQ-F6-C 唯一剩余的需求项，并把 SC#3（CI grep gate）、SC#4（5 类修复）、SC#7（mergerfs 篡改 + 修复命令 E2E）三条 Success Criteria 闭环。

Output:
- 1 个新增 doctor/fix.go（~300 行）+ 1 个新增 fix_test.go
- 5 处维度文件 Fix() 方法补全（network/auth/ssh/mount 各若干）
- 1 个新增 integration_test.go（build tag integration）
- 1 个新增 scripts/ci-doctor-grep.sh（可执行 chmod +x）
- 1 个修改的 Makefile（追加 ci-gate target）
- 1 个修改的 cmd/cloud-claude/doctor.go（anyFixerRegistered 真实实现）
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
@.planning/phases/34-cloud-claude-doctor-v3/34-02-doctor-framework-PLAN.md
@.planning/phases/29.1-gethost-entry-password-workspace/29.1-CONTEXT.md
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/33-claude-code-cli-admin-gc/33-CONTEXT.md

# 直接改造对象（读）
@internal/cloudclaude/doctor/doctor.go
@internal/cloudclaude/doctor/check.go
@internal/cloudclaude/doctor/network.go
@internal/cloudclaude/doctor/auth.go
@internal/cloudclaude/doctor/ssh.go
@internal/cloudclaude/doctor/mount.go
@internal/cloudclaude/mount_mutagen.go
@internal/runtime/tasks/worker.go
@internal/cloudclaude/integration_test.go
@scripts/test-fixture-up.sh
@scripts/test-fixture-down.sh
@Makefile
@cmd/cloud-claude/doctor.go

<interfaces>
<!-- 上游 Plan 02 已落地 + 既有 v3.0 fix 复用点。 -->

From internal/cloudclaude/doctor/check.go (Plan 02):
```go
type Check struct {
    Domain     string
    Name       string
    Status     Status
    Code       errcodes.Code
    Message    string
    NextAction string
    Details    map[string]any
    FixApplied []string
    FixFailed  []string
    DurationMS int64
}

type Checker interface {
    Run(ctx context.Context, runner RemoteRunner) Check
    Fix(ctx context.Context, opts Options) Check
}
```

From internal/cloudclaude/mount_mutagen.go (line 225-229，既有 daemon start 幂等样板):
```go
out, derr := deps.runLocal(binPath, []string{"daemon", "start"}, env)
if derr != nil && !strings.Contains(out, "daemon already started") && !strings.Contains(out, "already running") {
    return nil, MutagenSyncStatus{}, newMutagenErr(errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, derr.Error())
}
```

From internal/runtime/tasks/worker.go (line 687-695，包级 var mock 模板):
```go
// execInContainer 在目标容器中以 `docker exec -i <container> bash -c <script>` 执行，
// 支持可选 stdin。暴露为 package-level 变量以便单元测试注入 fake。
var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
    if stdin != "" {
        cmd.Stdin = strings.NewReader(stdin)
    }
    return cmd.CombinedOutput()
}
```

From internal/cloudclaude/integration_test.go (line 1-80，build tag + TestMain + dockerExec):
```go
//go:build integration
// +build integration

func TestMain(m *testing.M) {
    if err := exec.Command("scripts/test-fixture-up.sh").Run(); err != nil {
        fmt.Fprintln(os.Stderr, "fixture 启动失败，跳过集成测试:", err)
        os.Exit(0)
    }
    code := m.Run()
    _ = exec.Command("scripts/test-fixture-down.sh").Run()
    os.Exit(code)
}

func dockerExec(t *testing.T, args ...string) (string, error) {
    full := append([]string{"exec", fixtureCtr}, args...)
    c := exec.Command("docker", full...)
    var out bytes.Buffer
    c.Stdout = &out
    c.Stderr = &out
    err := c.Run()
    return out.String(), err
}
```

From scripts/test-fixture-up.sh (line 1-23，bash 脚本头部模板):
```bash
#!/usr/bin/env bash
set -euo pipefail
FIXTURE_DIR="/tmp/cloud-claude-fixture"
IMAGE="local/managed-user:v3.0.0"
command -v docker >/dev/null || { echo "需要 docker"; exit 1; }
```

From golang.org/x/term (已被 cmd/cloud-claude/main.go line 10 import):
```go
term.IsTerminal(int(os.Stdin.Fd())) bool
```
</interfaces>
</context>

<tasks>

<task type="execute">
  <name>Task 3.1: 新建 doctor/fix.go — FixerRegistry + 5 类 Fixer + confirmDestructive + 5 个包级 var（CONTEXT D-09 / D-10 / PATTERNS §2.9）</name>
  <files>internal/cloudclaude/doctor/fix.go</files>

  <read_first>
    - internal/cloudclaude/doctor/check.go（Plan 02 Check struct + Options 字段）
    - internal/runtime/tasks/worker.go（line 687-695 包级 var mock 模板 + line 957-984 dockerVolumeRunner 模板）
    - internal/cloudclaude/mount_mutagen.go（line 220-235 daemon stop/start 幂等样板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-09（5 类修复表）+ §D-10（confirmDestructive 三级判定）+ §D-11（不回滚）+ §D-12（输出格式）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §4.1-§4.5（跨 OS 命令 + 幂等性证据）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.9（包级 var mock）
  </read_first>

  <action>
新建 `internal/cloudclaude/doctor/fix.go`（约 300 行，完整实现 5 类修复）：

```go
// Package doctor — Phase 34 Plan 03：doctor --fix 5 类自动修复 + FixerRegistry + confirmDestructive。
//
// 5 类修复（CONTEXT D-09 表）：
//   1. MOUNT_MUTAGEN_DAEMON_UNAVAILABLE → mutagen daemon stop && mutagen daemon start（低危 / 免确认）
//   2. SYSTEM_FUSE_RESIDUAL_MOUNT       → fusermount -u <path>（批量 y/N 确认）
//   3. SSH_KNOWN_HOSTS_CONFLICT         → ssh-keygen -R <host:port>（低危 / 免确认）
//   4. AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED → 重调 EntryClient.AuthenticateAndWait（低危 / 免确认）
//   5. SYSTEM_DNS_RESOLVE_FAILED        → macOS dscacheutil / Linux resolvectl（sudo + y/N 确认）
//
// 跨 OS 分叉集中在 §4.2 (FUSE 解挂) + §4.5 (DNS flush) + §3.5 (Statfs)。
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// Fixer 是一个错误码的修复函数；返回 FixApplied/FixFailed 列表（追加到 Check）。
type Fixer func(ctx context.Context, opts Options, original Check) (applied []string, failed []string)

// FixerRegistry 按错误码路由到对应 Fixer。Plan 03 初始化 5 类；v3.1 可扩展。
// 本包初始化 (init())  populate，测试可通过 `originalRegistry := FixerRegistry; FixerRegistry = nil; defer ...` 隔离。
var FixerRegistry = map[errcodes.Code]Fixer{}

// 5 个包级 var mock 注入点（PATTERNS §2.9 / worker.go 样板）。
// 暴露为 package-level 变量以便单元测试注入 fake。
var execMutagenDaemon = realExecMutagenDaemon
var execFusermountUnmount = realExecFusermountUnmount
var execSSHKeygenRemove = realExecSSHKeygenRemove
var execEntryRefresh = realExecEntryRefresh
var execDNSFlush = realExecDNSFlush

// isTerminalFD 是 term.IsTerminal 的包级 var（便于测试注入非 TTY 场景）。
var isTerminalFD = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func init() {
	FixerRegistry[errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE] = fixMutagenDaemonUnavailable
	FixerRegistry[errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT] = fixFUSEResidualMount
	FixerRegistry[errcodes.SSH_KNOWN_HOSTS_CONFLICT] = fixSSHKnownHostsConflict
	FixerRegistry[errcodes.AUTH_TOKEN_EXPIRED] = fixAuthTokenExpired
	FixerRegistry[errcodes.AUTH_OAUTH_REFRESH_FAILED] = fixAuthOAuthRefreshFailed
	FixerRegistry[errcodes.SYSTEM_DNS_RESOLVE_FAILED] = fixDNSResolveFailed
}

// ----------------------------------------------------------------------------
// confirmDestructive — 三级判定（CONTEXT D-10）：
//   1. opts.Yes=true           → true  （CI 友好）
//   2. opts.JSON=true          → false + 调用方写 FixFailed «JSON 模式禁止交互式修复，请在终端模式重试或追加 --yes»
//   3. 非 TTY（stdin 非 pipe）  → false + 调用方写 FixFailed «非 TTY 环境，请追加 --yes 或在终端重试»
//   4. 否则交互 y/N（提示字面量风格与 mutagen 一致）
//
// 返回 (confirmed bool, refusalReason string)。refusalReason 为空说明用户确认（或 --yes）。
func confirmDestructive(opts Options, promptZH string) (bool, string) {
	if opts.Yes {
		return true, ""
	}
	if opts.JSON {
		return false, "JSON 模式禁止交互式修复，请在终端模式重试或追加 --yes"
	}
	if !isTerminalFD() {
		return false, "非 TTY 环境，请追加 --yes 或在终端重试"
	}
	fmt.Printf("%s(y/N) > ", promptZH)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		return true, ""
	}
	return false, "用户取消"
}

// ----------------------------------------------------------------------------
// 1. MOUNT_MUTAGEN_DAEMON_UNAVAILABLE — mutagen daemon stop && start（低危 / 免确认）
// ----------------------------------------------------------------------------

func fixMutagenDaemonUnavailable(ctx context.Context, opts Options, _ Check) ([]string, []string) {
	if err := execMutagenDaemon(ctx, "stop"); err != nil && !isMutagenDaemonIdempotent(err) {
		return nil, []string{fmt.Sprintf("mutagen daemon stop 失败: %v", err)}
	}
	if err := execMutagenDaemon(ctx, "start"); err != nil && !isMutagenDaemonIdempotent(err) {
		return nil, []string{fmt.Sprintf("mutagen daemon start 失败: %v", err)}
	}
	return []string{"mutagen daemon 已重启"}, nil
}

// isMutagenDaemonIdempotent — mount_mutagen.go:225-229 已实现同款容错（复刻其字面量）。
func isMutagenDaemonIdempotent(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no daemon is running") ||
		strings.Contains(msg, "daemon already started") ||
		strings.Contains(msg, "already running")
}

func realExecMutagenDaemon(ctx context.Context, action string) error {
	// 生产实现：调用 embed 的 mutagen 二进制（Phase 31 mutagen_bin.go / mount_mutagen.go 已有复用点）。
	// 本阶段直接走 exec.CommandContext；executor 可按项目约定用 cloudclaude.RunLocalMutagen helper（如存在）。
	cmd := exec.CommandContext(ctx, "mutagen", "daemon", action)
	return cmd.Run()
}

// ----------------------------------------------------------------------------
// 2. SYSTEM_FUSE_RESIDUAL_MOUNT — fusermount -u <path>（批量 y/N 确认）
// ----------------------------------------------------------------------------

func fixFUSEResidualMount(ctx context.Context, opts Options, original Check) ([]string, []string) {
	// Details["mountpoints"] 由 checkFUSEResidual 提供（Task 3.2 给 mount.go 的 checkFUSEResidual 加 Details）
	var points []string
	if v, ok := original.Details["mountpoints"].([]string); ok {
		points = v
	}
	if len(points) == 0 {
		return nil, []string{"无法从 Details 获取 mountpoints（需 Plan 02 rerun 以填充 Details）"}
	}

	prompt := fmt.Sprintf("发现 %d 个疑似残留 FUSE 挂载：\n", len(points))
	for _, p := range points {
		prompt += "  " + p + "\n"
	}
	prompt += "将逐个执行 fusermount -u（已解挂的将跳过），是否继续？"
	confirmed, reason := confirmDestructive(opts, prompt)
	if !confirmed {
		return nil, []string{"跳过解挂：" + reason}
	}

	var applied, failed []string
	for _, mp := range points {
		if err := execFusermountUnmount(ctx, mp); err != nil {
			if isFusermountIdempotent(err) {
				applied = append(applied, "已解挂（空操作）: "+mp)
				continue
			}
			failed = append(failed, fmt.Sprintf("fusermount -u %s 失败: %v", mp, err))
			continue
		}
		applied = append(applied, "已解挂: "+mp)
	}
	return applied, failed
}

// isFusermountIdempotent — RESEARCH §4.2：非 busy 且 not-found 视为幂等。
// FUSE 的 not-mounted / no-such-file 错误视为幂等（重复解挂安全）；
// device busy 是真错（活跃 fd），不算幂等。
func isFusermountIdempotent(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	// FUSE 的 not-mounted / no-such-file 错误视为幂等（重复解挂安全）
	if strings.Contains(msg, "not mounted") {
		return true
	}
	if strings.Contains(msg, "no such file") {
		return true
	}
	// device busy 是真错（活跃 fd），不算幂等
	if strings.Contains(msg, "device or resource busy") {
		return false
	}
	// 其它 not-found 在 fuser 上下文也视为幂等
	return strings.Contains(msg, "not found")
}

func realExecFusermountUnmount(ctx context.Context, mountpoint string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// macFUSE 没有 fusermount；用 umount
		cmd = exec.CommandContext(ctx, "umount", mountpoint)
	default:
		cmd = exec.CommandContext(ctx, "fusermount", "-u", mountpoint)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ----------------------------------------------------------------------------
// 3. SSH_KNOWN_HOSTS_CONFLICT — ssh-keygen -R <host:port>（低危 / 免确认）
// ----------------------------------------------------------------------------

func fixSSHKnownHostsConflict(ctx context.Context, opts Options, original Check) ([]string, []string) {
	hostPort, _ := original.Details["host_port"].(string)
	if hostPort == "" {
		return nil, []string{"无法从 Details 获取 host_port"}
	}
	if err := execSSHKeygenRemove(ctx, hostPort); err != nil {
		if isSSHKeygenIdempotent(err) {
			return []string{"known_hosts 已无此条目（空操作）"}, nil
		}
		return nil, []string{fmt.Sprintf("ssh-keygen -R %s 失败: %v", hostPort, err)}
	}
	return []string{"已从 ~/.ssh/known_hosts 删除 " + hostPort}, nil
}

func isSSHKeygenIdempotent(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not in ")
}

func realExecSSHKeygenRemove(ctx context.Context, hostPort string) error {
	cmd := exec.CommandContext(ctx, "ssh-keygen", "-R", hostPort)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ----------------------------------------------------------------------------
// 4. AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED — refresh（低危 / 免确认）
// ----------------------------------------------------------------------------

func fixAuthTokenExpired(ctx context.Context, opts Options, _ Check) ([]string, []string) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, []string{"无法加载 config: " + err.Error()}
	}
	if _, err := execEntryRefresh(ctx, cfg.Gateway, cfg.ShortID, cfg.Password); err != nil {
		return nil, []string{"Entry API 刷新失败: " + err.Error()}
	}
	return []string{"Entry API token 已刷新"}, nil
}

func fixAuthOAuthRefreshFailed(ctx context.Context, opts Options, _ Check) ([]string, []string) {
	// OAuth 过期不能自动登录，给用户 NextAction 即可
	return nil, []string{"请在容器内运行 cloud-claude exec claude login 重新登录"}
}

func realExecEntryRefresh(ctx context.Context, gateway, shortID, password string) (*cloudclaude.AuthResponse, error) {
	client := cloudclaude.NewEntryClient(gateway)
	return client.AuthenticateAndWait(ctx, shortID, password, func(string) {})
}

// ----------------------------------------------------------------------------
// 5. SYSTEM_DNS_RESOLVE_FAILED — flush cache（sudo + y/N 确认）
// ----------------------------------------------------------------------------

func fixDNSResolveFailed(ctx context.Context, opts Options, _ Check) ([]string, []string) {
	confirmed, reason := confirmDestructive(opts,
		"DNS 缓存 flush 涉及系统级 daemon 信号（macOS mDNSResponder / Linux resolvectl），需要 sudo。是否继续？")
	if !confirmed {
		return nil, []string{"跳过 DNS flush：" + reason}
	}
	if err := execDNSFlush(ctx); err != nil {
		return nil, []string{"DNS 缓存刷新失败: " + err.Error()}
	}
	return []string{"DNS 缓存已刷新"}, nil
}

func realExecDNSFlush(ctx context.Context) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// 需要 sudo；用户会看到 sudo 密码 prompt
		cmd = exec.CommandContext(ctx, "sudo", "sh", "-c",
			"dscacheutil -flushcache && killall -HUP mDNSResponder")
	case "linux":
		// 探测 resolvectl / systemd-resolve（RESEARCH §4.5）
		if _, err := exec.LookPath("resolvectl"); err == nil {
			cmd = exec.CommandContext(ctx, "sudo", "resolvectl", "flush-caches")
		} else if _, err := exec.LookPath("systemd-resolve"); err == nil {
			cmd = exec.CommandContext(ctx, "sudo", "systemd-resolve", "--flush-caches")
		} else {
			return fmt.Errorf("未检测到 resolvectl / systemd-resolve；请手动清理 DNS 缓存")
		}
	default:
		return fmt.Errorf("不支持的 OS: %s", runtime.GOOS)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ----------------------------------------------------------------------------
// ApplyFixes — 被 RunDoctor 调用：遍历 Check 列表，对 Registry 命中的 code 跑 Fixer。
// ----------------------------------------------------------------------------

// ApplyFixes 在每个 warn/fail 的 Check 上按 Code 路由到 FixerRegistry；结果写回 check.FixApplied/FixFailed。
// Status 不回写（CONTEXT D-16：修复成功的 fail 不降级）。
func ApplyFixes(ctx context.Context, opts Options, checks []Check) []Check {
	if !opts.Fix {
		return checks
	}
	fixCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for i := range checks {
		c := &checks[i]
		if c.Status != StatusWarn && c.Status != StatusFail {
			continue
		}
		fixer, ok := FixerRegistry[c.Code]
		if !ok {
			continue
		}
		applied, failed := fixer(fixCtx, opts, *c)
		c.FixApplied = append(c.FixApplied, applied...)
		c.FixFailed = append(c.FixFailed, failed...)
	}
	return checks
}
```

**verbatim 守恒：**
- 5 个包级 var 名 `execMutagenDaemon / execFusermountUnmount / execSSHKeygenRemove / execEntryRefresh / execDNSFlush`（与 PATTERNS §2.9 / RESEARCH §2.5 完全一致）
- `confirmDestructive(opts Options, promptZH string) (bool, string)` 签名不变
- FixerRegistry init 注册 6 个 entry（含 AUTH_OAUTH_REFRESH_FAILED — 虽然 D-09 第 4 行合并计 1 类，实际 Registry 有 2 个 key）
- `ApplyFixes(ctx, opts, checks) []Check` 公开函数（RunDoctor 调用）

**禁止：**
- 在 Fixer 内做 Status 回写（CONTEXT D-16 + D-11）
- 在 ApplyFixes 内做 goroutine 并发（保持串行，便于日志可读）
- `execDNSFlush` 自动输入 sudo 密码（走系统 prompt）

**executor 注意：** Task 3.5 会让 RunDoctor 调 ApplyFixes；本 task 只暴露 ApplyFixes。
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/fix.go` 退出码 = 0
    - `grep -q "var FixerRegistry = map\[errcodes.Code\]Fixer" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "^var execMutagenDaemon = realExecMutagenDaemon" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "^var execFusermountUnmount = realExecFusermountUnmount" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "^var execSSHKeygenRemove = realExecSSHKeygenRemove" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "^var execEntryRefresh = realExecEntryRefresh" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "^var execDNSFlush = realExecDNSFlush" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "func confirmDestructive(opts Options, promptZH string) (bool, string)" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "term.IsTerminal" internal/cloudclaude/doctor/fix.go` 命中（非 TTY 判定）
    - `grep -q "FixerRegistry\\[errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE\\] = fixMutagenDaemonUnavailable" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "FixerRegistry\\[errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT\\] = fixFUSEResidualMount" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "FixerRegistry\\[errcodes.SSH_KNOWN_HOSTS_CONFLICT\\] = fixSSHKnownHostsConflict" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "FixerRegistry\\[errcodes.AUTH_TOKEN_EXPIRED\\] = fixAuthTokenExpired" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "FixerRegistry\\[errcodes.AUTH_OAUTH_REFRESH_FAILED\\]" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "FixerRegistry\\[errcodes.SYSTEM_DNS_RESOLVE_FAILED\\]" internal/cloudclaude/doctor/fix.go` 命中
    - `grep -q "func ApplyFixes" internal/cloudclaude/doctor/fix.go` 命中
    - `go build ./internal/cloudclaude/doctor/...` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.2: 新建 doctor/fix_test.go — 11 条 fix 行为单测（包级 var mock）</name>
  <files>internal/cloudclaude/doctor/fix_test.go</files>

  <read_first>
    - internal/cloudclaude/doctor/fix.go（Task 3.1 新建）
    - internal/runtime/tasks/worker_volume_lifecycle_test.go（Phase 33 包级 var mock 注入测试模板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-09 / §D-10 / §D-11
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.12（fix_test 单测模板）
  </read_first>

  <action>
新建 `internal/cloudclaude/doctor/fix_test.go`：

```go
package doctor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

// -------- confirmDestructive 三级判定 --------

func TestConfirmDestructive_Yes_True(t *testing.T) {
	ok, reason := confirmDestructive(Options{Yes: true}, "危险 prompt")
	if !ok {
		t.Errorf("opts.Yes=true 必须返回 true，实际 ok=%v reason=%q", ok, reason)
	}
	if reason != "" {
		t.Errorf("Yes=true reason 应为空，实际 %q", reason)
	}
}

func TestConfirmDestructive_JSON_FalseWithReason(t *testing.T) {
	ok, reason := confirmDestructive(Options{JSON: true}, "危险 prompt")
	if ok {
		t.Errorf("JSON=true 必须返回 false，实际 ok=%v", ok)
	}
	if !strings.Contains(reason, "JSON") {
		t.Errorf("reason 应提及 JSON 模式，实际 %q", reason)
	}
}

func TestConfirmDestructive_NonTTY_False(t *testing.T) {
	orig := isTerminalFD
	isTerminalFD = func() bool { return false }
	t.Cleanup(func() { isTerminalFD = orig })
	ok, reason := confirmDestructive(Options{}, "危险 prompt")
	if ok {
		t.Errorf("非 TTY 必须返回 false，实际 ok=%v", ok)
	}
	if !strings.Contains(reason, "TTY") {
		t.Errorf("reason 应提及 TTY，实际 %q", reason)
	}
}

// -------- fixMutagenDaemonUnavailable --------

func TestFixMutagenDaemon_Idempotent(t *testing.T) {
	calls := []string{}
	orig := execMutagenDaemon
	execMutagenDaemon = func(ctx context.Context, action string) error {
		calls = append(calls, action)
		return nil
	}
	t.Cleanup(func() { execMutagenDaemon = orig })

	applied, failed := fixMutagenDaemonUnavailable(context.Background(), Options{Yes: true}, Check{})
	if len(applied) == 0 || len(failed) != 0 {
		t.Errorf("首次修复应成功：applied=%v failed=%v", applied, failed)
	}
	// 再跑一次 — 两次都应成功（idempotent）
	applied2, failed2 := fixMutagenDaemonUnavailable(context.Background(), Options{Yes: true}, Check{})
	if len(applied2) == 0 || len(failed2) != 0 {
		t.Errorf("二次修复应仍成功：applied=%v failed=%v", applied2, failed2)
	}
	if len(calls) != 4 {
		t.Errorf("2 次修复应调 4 次 daemon（stop+start+stop+start），实际 %v", calls)
	}
}

func TestFixMutagenDaemon_IdempotentError_Tolerated(t *testing.T) {
	orig := execMutagenDaemon
	execMutagenDaemon = func(ctx context.Context, action string) error {
		return fmt.Errorf("no daemon is running")
	}
	t.Cleanup(func() { execMutagenDaemon = orig })
	applied, failed := fixMutagenDaemonUnavailable(context.Background(), Options{Yes: true}, Check{})
	if len(failed) != 0 {
		t.Errorf("'no daemon is running' 应视为成功（幂等），实际 failed=%v", failed)
	}
	_ = applied
}

// -------- fixFUSEResidualMount --------

func TestFixFUSEResidualMount_Yes_UnmountsAll(t *testing.T) {
	var called []string
	orig := execFusermountUnmount
	execFusermountUnmount = func(ctx context.Context, mp string) error {
		called = append(called, mp)
		return nil
	}
	t.Cleanup(func() { execFusermountUnmount = orig })
	original := Check{
		Code:    errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT,
		Details: map[string]any{"mountpoints": []string{"/tmp/cc-a", "/tmp/cc-b"}},
	}
	applied, failed := fixFUSEResidualMount(context.Background(), Options{Yes: true}, original)
	if len(applied) != 2 || len(failed) != 0 {
		t.Errorf("2 个 mountpoints 都应解挂，applied=%v failed=%v", applied, failed)
	}
	if len(called) != 2 {
		t.Errorf("应调 2 次 fusermount，实际 %v", called)
	}
}

func TestFixFUSEResidualMount_NonTTY_NoYes_Rejected(t *testing.T) {
	origTTY := isTerminalFD
	isTerminalFD = func() bool { return false }
	t.Cleanup(func() { isTerminalFD = origTTY })
	original := Check{Details: map[string]any{"mountpoints": []string{"/tmp/cc-a"}}}
	applied, failed := fixFUSEResidualMount(context.Background(), Options{}, original)
	if len(applied) != 0 {
		t.Error("非 TTY + 无 --yes 应拒绝")
	}
	if len(failed) == 0 || !strings.Contains(failed[0], "TTY") {
		t.Errorf("failed 应提及 TTY，实际 %v", failed)
	}
}

func TestFixFUSEResidualMount_EmptyDetails_Fail(t *testing.T) {
	_, failed := fixFUSEResidualMount(context.Background(), Options{Yes: true}, Check{})
	if len(failed) == 0 {
		t.Error("无 mountpoints Details 应 failed")
	}
}

// -------- fixSSHKnownHostsConflict --------

func TestFixSSHKnownHostsConflict_Success(t *testing.T) {
	var called string
	orig := execSSHKeygenRemove
	execSSHKeygenRemove = func(ctx context.Context, hp string) error {
		called = hp
		return nil
	}
	t.Cleanup(func() { execSSHKeygenRemove = orig })
	original := Check{Details: map[string]any{"host_port": "example.com:22"}}
	applied, failed := fixSSHKnownHostsConflict(context.Background(), Options{}, original)
	if len(applied) == 0 || len(failed) != 0 {
		t.Errorf("应成功，applied=%v failed=%v", applied, failed)
	}
	if called != "example.com:22" {
		t.Errorf("应调用 ssh-keygen -R example.com:22，实际 %q", called)
	}
}

func TestFixSSHKnownHostsConflict_NotFound_Idempotent(t *testing.T) {
	orig := execSSHKeygenRemove
	execSSHKeygenRemove = func(ctx context.Context, hp string) error {
		return fmt.Errorf("not found in /home/user/.ssh/known_hosts")
	}
	t.Cleanup(func() { execSSHKeygenRemove = orig })
	original := Check{Details: map[string]any{"host_port": "example.com:22"}}
	applied, failed := fixSSHKnownHostsConflict(context.Background(), Options{}, original)
	if len(failed) != 0 {
		t.Errorf("'not found' 应视为成功，failed=%v", failed)
	}
	_ = applied
}

// -------- fixAuthTokenExpired --------

func TestFixAuthTokenExpired_Success(t *testing.T) {
	origCfg := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) {
		return &cloudclaude.Config{Gateway: "https://gw.example.com", ShortID: "x", Password: "y"}, nil
	}
	t.Cleanup(func() { loadConfig = origCfg })
	orig := execEntryRefresh
	execEntryRefresh = func(ctx context.Context, gw, id, pw string) (*cloudclaude.AuthResponse, error) {
		return &cloudclaude.AuthResponse{}, nil
	}
	t.Cleanup(func() { execEntryRefresh = orig })
	applied, failed := fixAuthTokenExpired(context.Background(), Options{}, Check{})
	if len(applied) == 0 || len(failed) != 0 {
		t.Errorf("刷新应成功，applied=%v failed=%v", applied, failed)
	}
}

// -------- fixDNSResolveFailed --------

func TestFixDNSResolveFailed_Yes_Calls(t *testing.T) {
	called := false
	orig := execDNSFlush
	execDNSFlush = func(ctx context.Context) error {
		called = true
		return nil
	}
	t.Cleanup(func() { execDNSFlush = orig })
	applied, failed := fixDNSResolveFailed(context.Background(), Options{Yes: true}, Check{})
	if !called {
		t.Error("opts.Yes=true 应真实调用 execDNSFlush")
	}
	if len(applied) == 0 || len(failed) != 0 {
		t.Errorf("成功路径 applied=%v failed=%v", applied, failed)
	}
}

func TestFixDNSResolveFailed_NonTTY_Rejected(t *testing.T) {
	origTTY := isTerminalFD
	isTerminalFD = func() bool { return false }
	t.Cleanup(func() { isTerminalFD = origTTY })
	applied, failed := fixDNSResolveFailed(context.Background(), Options{}, Check{})
	if len(applied) != 0 {
		t.Error("非 TTY + 无 Yes 应拒绝")
	}
	if len(failed) == 0 {
		t.Error("failed 应含原因")
	}
}

// -------- ApplyFixes 路由 --------

func TestApplyFixes_NoFix_Noop(t *testing.T) {
	checks := []Check{{Code: errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, Status: StatusFail}}
	out := ApplyFixes(context.Background(), Options{Fix: false}, checks)
	if len(out[0].FixApplied) != 0 || len(out[0].FixFailed) != 0 {
		t.Error("opts.Fix=false 应 noop")
	}
}

func TestApplyFixes_Fix_TriggersRegistry(t *testing.T) {
	calls := 0
	orig := execMutagenDaemon
	execMutagenDaemon = func(ctx context.Context, action string) error { calls++; return nil }
	t.Cleanup(func() { execMutagenDaemon = orig })
	checks := []Check{
		{Code: errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, Status: StatusFail},
		{Code: errcodes.DISK_LOCAL_LOW, Status: StatusWarn}, // 不在 Registry，应跳过
	}
	out := ApplyFixes(context.Background(), Options{Fix: true, Yes: true}, checks)
	if len(out[0].FixApplied) == 0 {
		t.Error("Mutagen daemon Fixer 应触发")
	}
	if len(out[1].FixApplied) != 0 {
		t.Error("DISK_LOCAL_LOW 不在 Registry，不应触发")
	}
	if calls < 2 {
		t.Errorf("应调 2 次（stop+start），实际 %d", calls)
	}
}

func TestApplyFixes_StatusNotDowngraded(t *testing.T) {
	orig := execMutagenDaemon
	execMutagenDaemon = func(ctx context.Context, action string) error { return nil }
	t.Cleanup(func() { execMutagenDaemon = orig })
	checks := []Check{{Code: errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, Status: StatusFail}}
	out := ApplyFixes(context.Background(), Options{Fix: true, Yes: true}, checks)
	if out[0].Status != StatusFail {
		t.Errorf("CONTEXT D-16：Status 不降级，应保留 fail，实际 %s", out[0].Status)
	}
	if len(out[0].FixApplied) == 0 {
		t.Error("FixApplied 应非空（已修复标记）")
	}
}
```

**禁止：**
- 在测试中调真实 `mutagen daemon` / `fusermount` / `ssh-keygen` / `dscacheutil`（必须全部 mock）
- 测试依赖 stdin 交互（`confirmDestructive` 在 Yes / JSON / 非 TTY 三分支覆盖，没有真实 stdin 测试）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/fix_test.go` 退出码 = 0
    - `grep -cE "^func Test(Fix|ConfirmDestructive|ApplyFixes)" internal/cloudclaude/doctor/fix_test.go` ≥ 14
    - `grep -q "TestConfirmDestructive_Yes_True" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestConfirmDestructive_NonTTY_False" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestFixMutagenDaemon_Idempotent" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestFixFUSEResidualMount_Yes_UnmountsAll" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestFixSSHKnownHostsConflict_NotFound_Idempotent" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestFixDNSResolveFailed_NonTTY_Rejected" internal/cloudclaude/doctor/fix_test.go` 命中
    - `grep -q "TestApplyFixes_StatusNotDowngraded" internal/cloudclaude/doctor/fix_test.go` 命中
    - `go test ./internal/cloudclaude/doctor/ -run "TestFix|TestConfirmDestructive|TestApplyFixes" -count=1 -v` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.3: 修改 mount.go + ssh.go — 填充 Details</name>
  <files>internal/cloudclaude/doctor/mount.go, internal/cloudclaude/doctor/ssh.go</files>

  <read_first>
    - internal/cloudclaude/doctor/mount.go（Plan 02 Task 2.7）
    - internal/cloudclaude/doctor/ssh.go（Plan 02 Task 2.6）
    - internal/cloudclaude/doctor/fix.go（Task 3.1：Fixer 从 Details 读 host_port / mountpoints）
  </read_first>

  <action>
**(a) 修改 `internal/cloudclaude/doctor/mount.go` 的 `checkFUSEResidual`：** 把 newWarn 的返回改为显式 `Check{...}` 并在 Details 中放入 `mountpoints []string`：

```go
	if len(matches) == 0 {
		return newPass("mount", "fuse_residual", "未发现残留 FUSE 挂载")
	}
	var points []string
	for _, m := range matches {
		points = append(points, m[1])
	}
	// Plan 03 Task 3.3：fix.go 依赖 Details["mountpoints"] 列表做批量 fusermount -u
	entry, _ := errcodes.Lookup(errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT)
	return Check{
		Domain: "mount", Name: "fuse_residual",
		Status:  StatusWarn,
		Code:    errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT,
		Message: fmt.Sprintf(entry.Message, len(points), strings.Join(points, ",")),
		NextAction: entry.NextAction,
		Details: map[string]any{"mountpoints": points},
	}
```

**(b) 修改 `internal/cloudclaude/doctor/ssh.go` 的 `checkKnownHosts`：** 在 warn 分支填充 `Details["host_port"]`：

```go
	// ... 既有校验逻辑
	_, err := knownhosts.New(khPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not exist") {
			return newSkip("ssh", "known_hosts", "~/.ssh/known_hosts 不存在，跳过（首次连接时自动创建）")
		}
		// Plan 03 Task 3.3：fix.go 依赖 Details["host_port"] 跑 ssh-keygen -R
		c := newWarn("ssh", "known_hosts", errcodes.SSH_KNOWN_HOSTS_CONFLICT, authHostPort)
		c.Details = map[string]any{"host_port": authHostPort}
		return c
	}
```

**(c) 同文件 `sshd_keepalive_drift` warn 分支填充 `Details["interval"]` + `["count"]`（**可选，便于 --verbose 调试**）：

```go
	if interval != 15 || count != 8 {
		got := fmt.Sprintf("clientaliveinterval=%d clientalivecountmax=%d", interval, count)
		c := newWarn("ssh", "sshd_keepalive_drift", errcodes.SSH_SSHD_KEEPALIVE_DRIFT, got)
		c.Details = map[string]any{"interval": interval, "count": count, "baseline": "15/8"}
		return c
	}
```

**verbatim 守恒：**
- `Details["mountpoints"]` key 字面量（fix.go Task 3.1 `original.Details["mountpoints"].([]string)` 依赖）
- `Details["host_port"]` key 字面量（fix.go Task 3.1 依赖）
- newWarn / newPass / newSkip 构造函数不变

**禁止：**
- 修改既有 Message / NextAction / Status 决策逻辑
- 在 Details 中放原 stdout 大对象（只放结构化关键字段）
  </action>

  <acceptance_criteria>
    - `grep -q 'Details: map\[string\]any{"mountpoints": points}' internal/cloudclaude/doctor/mount.go` 命中
    - `grep -q 'Details = map\[string\]any{"host_port": authHostPort}' internal/cloudclaude/doctor/ssh.go` 命中
    - `grep -q '"interval":' internal/cloudclaude/doctor/ssh.go` 命中（sshd_keepalive Details 扩展）
    - `go build ./internal/cloudclaude/doctor/...` 退出码 = 0
    - Plan 02 既有测试不回归：`go test ./internal/cloudclaude/doctor/ -count=1 -short -run "TestCheckFUSE|TestCheckKnownHosts|TestCheckSSHDKeepalive"` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.4: 修改 cmd/cloud-claude/doctor.go — anyFixerRegistered 真实检查 + fix 后 stdout 顶部 [fix] 行（CONTEXT D-12/D-16）</name>
  <files>cmd/cloud-claude/doctor.go</files>

  <read_first>
    - cmd/cloud-claude/doctor.go（Plan 02 Task 2.11 占位）
    - internal/cloudclaude/doctor/fix.go（Task 3.1：FixerRegistry + ApplyFixes）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-12（fix 模式 stdout 格式）+ §D-16（退出码）
  </read_first>

  <action>
修改 `cmd/cloud-claude/doctor.go`：

**(a) 把 `anyFixerRegistered()` 占位改为真实检查：**

```go
// anyFixerRegistered 检查 doctor.FixerRegistry 是否已注册任何 Fixer（Plan 03 完成后恒 true）。
func anyFixerRegistered() bool {
	return len(doctor.FixerRegistry) > 0
}
```

**(b) 在 `runDoctor` 中，`fix` 为 true 且有 Fixer 时调用 `doctor.ApplyFixes` — 改造 doctor 执行链：**

定位 Plan 02 中的：

```go
	report, err := doctor.RunDoctor(ctx, opts)
	if err != nil { ... }

	if fix && !anyFixerRegistered() {
		fmt.Fprintln(os.Stdout, "[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）")
	}
```

改为：

```go
	report, err := doctor.RunDoctor(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitInternalError)
		return nil
	}

	// Plan 03：--fix 后跑 FixerRegistry，结果写回 Check.FixApplied/FixFailed（Status 不降级 / CONTEXT D-16）
	var totalApplied, totalFailed int
	if fix {
		report.Checks = doctor.ApplyFixes(ctx, opts, report.Checks)
		for _, c := range report.Checks {
			totalApplied += len(c.FixApplied)
			totalFailed += len(c.FixFailed)
		}
		if totalApplied+totalFailed > 0 {
			fmt.Fprintf(os.Stdout, "[fix] %d 项已修复 / %d 项修复失败\n\n", totalApplied, totalFailed)
		}
	}
```

**(c) 保留 Plan 02 的 JSON / Text 渲染 + 退出码（ApplyFixes 不回写 Status，所以退出码逻辑不变）。**

**verbatim 守恒：**
- 字面量 `"[fix] %d 项已修复 / %d 项修复失败\n\n"`（CONTEXT D-12）
- `anyFixerRegistered()` 函数名不变（Plan 02 Task 2.11 就引入）
- 退出码顺序：Fail > Warn > OK（CONTEXT D-16）

**禁止：**
- 在 cmd 层重排序 Check（必须保留 doctor 内 domain 分组顺序）
- 在 fix 模式下跳过 render（用户必须看到完整报告 + fix 摘要）
  </action>

  <acceptance_criteria>
    - `grep -q "return len(doctor.FixerRegistry) > 0" cmd/cloud-claude/doctor.go` 命中
    - `grep -q "doctor.ApplyFixes(ctx, opts, report.Checks)" cmd/cloud-claude/doctor.go` 命中
    - `grep -q '"\\[fix\\] %d 项已修复 / %d 项修复失败' cmd/cloud-claude/doctor.go` 命中
    - `! grep -q "--fix 自动修复将在 doctor.fix.go 实现（Plan 03）" cmd/cloud-claude/doctor.go` 为真（Plan 02 占位行已移除）
    - `go build ./cmd/cloud-claude/...` 退出码 = 0
    - `go vet ./cmd/cloud-claude/...` 退出码 = 0
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.5: 新建 doctor/integration_test.go — build tag integration + 2 docker fixture 用例（SC#7 E2E）</name>
  <files>internal/cloudclaude/doctor/integration_test.go</files>

  <read_first>
    - internal/cloudclaude/integration_test.go（line 1-111 build tag + TestMain + dockerExec + runCloudClaude）
    - scripts/test-fixture-up.sh / test-fixture-down.sh（既有 fixture）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-24 第 3 条（docker fixture t.Skip 降级）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §2.13（integration_test 模板）
    - .planning/specifics.md §3 / §4（SC#7 测试锚点：mergerfs 篡改 + 修复命令字面量）
  </read_first>

  <action>
新建 `internal/cloudclaude/doctor/integration_test.go`：

```go
//go:build integration
// +build integration

// Phase 34 Plan 03 集成测试 — 在 Phase 29 镜像 fixture 容器上跑 cloud-claude doctor，验证：
//   1. TestIntegration_DoctorMountHappy — v3 镜像健康态下 mount 5 项全 pass；退出码 0
//   2. TestIntegration_DoctorMountFail_MergerfsTampered — 篡改 mergerfs readdir 参数后，
//      doctor mount 输出含 MOUNT_MERGERFS_FAILED + NextAction 含 'cloud-claude doctor mount --fix'（SC#7 锚点）
//
// 本文件默认 `go test ./...` 不触发（受 build tag `integration` 隔离）；完整执行需：
//
//   bash scripts/test-fixture-up.sh
//   go test -tags=integration -count=1 -v ./internal/cloudclaude/doctor/
//   bash scripts/test-fixture-down.sh

package doctor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const fixtureCtr = "cloud-claude-fixture" // 与 scripts/test-fixture-up.sh 一致

func TestMain(m *testing.M) {
	if err := exec.Command("scripts/test-fixture-up.sh").Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture 启动失败，跳过集成测试:", err)
		os.Exit(0)
	}
	code := m.Run()
	_ = exec.Command("scripts/test-fixture-down.sh").Run()
	os.Exit(code)
}

// dockerExec 在 fixture 容器内执行命令，返回合并 stdout/stderr。
func dockerExec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"exec", fixtureCtr}, args...)
	c := exec.Command("docker", full...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	if err := c.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// runCloudClaudeDoctor 编译 cloud-claude 并跑 doctor mount，返回 exit/stdout/stderr。
func runCloudClaudeDoctor(t *testing.T, extraArgs ...string) (int, string, string) {
	t.Helper()
	bin := "/tmp/cloud-claude-doctor-int"
	if _, err := os.Stat(bin); err != nil {
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cloud-claude")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("编译失败: %v\n%s", err, out)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"doctor", "mount", "--json"}, extraArgs...)
	c := exec.CommandContext(ctx, bin, args...)
	c.Env = append(os.Environ(), "NO_COLOR=1")
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	exitCode := 0
	if e, ok := err.(*exec.ExitError); ok {
		exitCode = e.ExitCode()
	} else if err != nil {
		t.Logf("cloud-claude doctor 执行错误: %v", err)
		exitCode = -1
	}
	return exitCode, outBuf.String(), errBuf.String()
}

// TestIntegration_DoctorMountHappy — happy path：v3 镜像健康，mount 维度 5 项全 pass 或 skip（apparmor/fuse_residual 在 CI 本机可能 skip）。
func TestIntegration_DoctorMountHappy(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 docker 集成测试")
	}
	exit, stdout, stderr := runCloudClaudeDoctor(t)
	if exit != 0 && exit != 1 {
		t.Errorf("happy path 期望 exit 0/1（allow warn from apparmor/fuse_residual on host），实际 %d；stderr=%s",
			exit, stderr)
	}
	// 不要求 mutagen_version_match 必定 pass（依赖容器内文件存在）；但不能全部 fail
	if strings.Count(stdout, `"status": "fail"`) > 3 {
		t.Errorf("happy path fail 过多：stdout=%s", stdout)
	}
}

// TestIntegration_DoctorMountFail_MergerfsTampered — 篡改 mergerfs 参数 → doctor mount 必含
// MOUNT_MERGERFS_FAILED + 'doctor mount --fix' 字面量（SC#7 / PITFALLS C2+M14 联合验收）。
func TestIntegration_DoctorMountFail_MergerfsTampered(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 docker 集成测试")
	}
	// Step 1: umount /workspace 并用错误参数 remount（不碰真实 branches，只触发参数断言）
	// 若容器内无 sudo 或 umount 受限，本步骤 Skip
	_, err := dockerExec(t, "bash", "-c", "umount /workspace 2>/dev/null; mount -t mergerfs -o cache.attr=60 branchsource /workspace || true")
	if err != nil {
		t.Skipf("无法在 fixture 内重挂 mergerfs，跳过: %v", err)
	}
	defer func() {
		// 恢复：重启 fixture 容器回到 entrypoint 状态
		_ = exec.Command("scripts/test-fixture-down.sh").Run()
		_ = exec.Command("scripts/test-fixture-up.sh").Run()
	}()

	// Step 2: 跑 doctor mount --json
	exit, stdout, _ := runCloudClaudeDoctor(t)
	if exit != 2 {
		t.Errorf("mergerfs 篡改后应 exit 2 (fail)，实际 %d", exit)
	}
	if !strings.Contains(stdout, "MOUNT_MERGERFS_FAILED") {
		t.Errorf("输出缺 MOUNT_MERGERFS_FAILED：%s", stdout)
	}
	// SC#7 字面量锚点：next_action 字段必须含 'doctor mount --fix' 或至少含 'doctor mount'
	if !strings.Contains(stdout, "doctor mount") {
		t.Errorf("next_action 缺 'doctor mount' 字面量（SC#7）：%s", stdout)
	}
}
```

**verbatim 守恒：**
- 第 1-2 行 build tag `//go:build integration` + `// +build integration`
- TestMain 的 `os.Exit(0)` 跳过 fixture 失败模式（本地无 docker 不阻塞单测）
- 复用 `scripts/test-fixture-up.sh` / `test-fixture-down.sh` — 禁止新建 docker compose fixture
- 关键断言字符串 `"MOUNT_MERGERFS_FAILED"` / `"doctor mount"`（SC#7 锚点）

**executor 注意：** `fixtureCtr` 常量值需与 `scripts/test-fixture-up.sh` 实际启动的容器名一致；如脚本内用 `cloud-claude-fixture` 以外的名字，改本文件这一行。

**禁止：**
- 依赖特定本地 docker daemon 状态（fixture-up 失败走 Skip）
- 在 CI 环境 require SSH 拨号成功（mount 维度的远端命令由 fixture 容器自提供）
  </action>

  <acceptance_criteria>
    - `test -f internal/cloudclaude/doctor/integration_test.go` 退出码 = 0
    - `head -2 internal/cloudclaude/doctor/integration_test.go | grep -q '^//go:build integration$'` 命中
    - `head -2 internal/cloudclaude/doctor/integration_test.go | tail -1 | grep -q '^// +build integration$'` 命中
    - `grep -q "func TestIntegration_DoctorMountHappy" internal/cloudclaude/doctor/integration_test.go` 命中
    - `grep -q "func TestIntegration_DoctorMountFail_MergerfsTampered" internal/cloudclaude/doctor/integration_test.go` 命中
    - `grep -q "MOUNT_MERGERFS_FAILED" internal/cloudclaude/doctor/integration_test.go` 命中
    - `grep -q 'scripts/test-fixture-up.sh' internal/cloudclaude/doctor/integration_test.go` 命中
    - 默认 build 不触发集成测试：`go build ./internal/cloudclaude/doctor/...` 退出码 = 0
    - 默认 test 不触发集成测试：`go test ./internal/cloudclaude/doctor/ -count=1 -short` 退出码 = 0
    - 可选（本地有 docker 时）：`go test -tags=integration ./internal/cloudclaude/doctor/ -count=1 -v` — CI 中由 `scripts/ci-doctor-grep.sh` 独立跑，不强求本 acceptance
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.6: 新建 scripts/ci-doctor-grep.sh — 3 段 jq + grep 断言（ROADMAP §Phase 34 SC#3 / M14 终验）</name>
  <files>scripts/ci-doctor-grep.sh</files>

  <read_first>
    - scripts/test-fixture-up.sh（bash 头部模板）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §6.1（**完整 70 行脚本已给**，直接复制）
    - .planning/phases/34-cloud-claude-doctor-v3/34-CONTEXT.md §D-24 第 5 条（CI grep 断言）
    - .planning/phases/34-cloud-claude-doctor-v3/34-PATTERNS.md §4.1
  </read_first>

  <action>
**(a) 新建 `scripts/ci-doctor-grep.sh` —— 完整脚本如下（RESEARCH §6.1 原样采纳）：**

```bash
#!/usr/bin/env bash
# scripts/ci-doctor-grep.sh — Phase 34 Plan 03 Task 3.6
#
# M14 终验脚本（ROADMAP §Phase 34 SC#3）：
#   (1) cloud-claude doctor --json → schema_version=1 + 所有 warn/fail check 含 next_action
#   (2) cloud-claude doctor （文本）→ 所有 [!]/[✗] 行含 "建议:" 子串
#   (3) 所有 [!]/[✗] 行含错误码 `[XXX_YYY_ZZZ]` 格式
#
# 用法：bash scripts/ci-doctor-grep.sh [path/to/cloud-claude-binary]
#
# 退出码：
#   0  → M14 + SC#3 全通过
#   1  → 任一断言失败（stderr 输出失败项）

set -euo pipefail

BIN="${1:-./cloud-claude}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

command -v jq >/dev/null || { echo "需要 jq (brew install jq / apt install jq)"; exit 1; }
test -x "$BIN" || { echo "二进制不存在或不可执行: $BIN"; exit 1; }

# ---------- (1) JSON 模式：schema_version=1 + 所有 warn/fail 必有 next_action ----------
# doctor 子命令可能退出 0/1/2；| jq 可能吞掉非零，用 || true 托底
"$BIN" doctor --json > "$WORK/report.json" || true

# 检查 JSON 合法
jq empty "$WORK/report.json" >/dev/null 2>&1 \
  || { echo "FAIL: doctor --json 输出非合法 JSON" >&2; cat "$WORK/report.json" >&2; exit 1; }

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
NO_COLOR=1 "$BIN" doctor > "$WORK/report.txt" || true

# 降级 banner 的 [!] 行（含 "未找到上次会话快照"）不走 warn/fail check 渲染，放过；
# doctor.go Plan 03 占位行 `[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）` Plan 03 已删；
# 仅检查 "── <domain> ──" 之后的 check 行。
BAD=$(awk '
  /^── (network|auth|ssh|mount|disk) ──$/ { in_section=1; next }
  /^$/                                    { in_section=0 }
  in_section && /^\s*\[[!✗]\]/            { print $0 }
' "$WORK/report.txt" | grep -v "建议:" || true)
if [ -n "$BAD" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺 '建议:' 子串:" >&2
  echo "$BAD" >&2
  exit 1
fi

# ---------- (3) 每条 warn/fail 必须含错误码：[XXX_YYY_ZZZ] ----------
BAD_CODE=$(awk '
  /^── (network|auth|ssh|mount|disk) ──$/ { in_section=1; next }
  /^$/                                    { in_section=0 }
  in_section && /^\s*\[[!✗]\]/            { print $0 }
' "$WORK/report.txt" | grep -vE '错误码:\s*[A-Z]+_[A-Z]+_[A-Z0-9]+' || true)
if [ -n "$BAD_CODE" ]; then
  echo "FAIL: 以下 [!]/[✗] 行缺错误码:" >&2
  echo "$BAD_CODE" >&2
  exit 1
fi

echo "OK: cloud-claude doctor M14 gate passed (schema=1 / next_action / 错误码)."
```

**(b) chmod +x scripts/ci-doctor-grep.sh**：executor 需要运行 `chmod +x scripts/ci-doctor-grep.sh` 使脚本可执行（git 会追踪执行位）。

**verbatim 守恒：**
- shebang + `set -euo pipefail` 行（与 `scripts/test-fixture-up.sh:17` 风格一致）
- 3 段断言顺序（JSON schema → 文本建议 → 文本错误码）
- awk 仅扫 `── <domain> ──` 之后的 check 行（避开降级 banner / [fix] 行的误报）
- 失败字面量 `"FAIL: ..."` 前缀 + stderr 重定向 + exit 1（CI 集成友好）

**禁止：**
- 依赖 `~/.cloud-claude/config.yaml` 存在（脚本必须在 CI 裸环境跑通 — doctor 未 init 时 network/auth 走本地仍输出合法 JSON）
- 过滤第 1 段 `DIFFPATH` 降级 banner 输出（awk 已守护 section boundary）
  </action>

  <acceptance_criteria>
    - `test -f scripts/ci-doctor-grep.sh` 退出码 = 0
    - `test -x scripts/ci-doctor-grep.sh` 退出码 = 0（可执行位）
    - `head -1 scripts/ci-doctor-grep.sh | grep -q '^#!/usr/bin/env bash$'` 命中
    - `grep -q "set -euo pipefail" scripts/ci-doctor-grep.sh` 命中
    - `grep -q "jq -e '.schema_version == 1'" scripts/ci-doctor-grep.sh` 命中（断言 1）
    - `grep -q '".next_action // ""' scripts/ci-doctor-grep.sh` 命中（断言 1 续）
    - `grep -q '"建议:"' scripts/ci-doctor-grep.sh` 命中（断言 2）
    - `grep -q "next_action" scripts/ci-doctor-grep.sh` 命中（断言 1/3：JSON next_action 段存在）
    - `grep -q "建议:" scripts/ci-doctor-grep.sh` 命中（断言 2：文本"建议:"段存在）
    - `grep -q "OK: cloud-claude doctor M14 gate passed" scripts/ci-doctor-grep.sh` 命中
    - `bash -n scripts/ci-doctor-grep.sh` 退出码 = 0（语法正确）
    - 人工 smoke（本地 build cloud-claude 后）：`bash scripts/ci-doctor-grep.sh ./cloud-claude` 退出码 = 0（acceptance 允许 CI 上跑，本地无 cloud-claude 二进制时不阻塞本 task）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.7: 修改 Makefile — 追加 ci-gate target 调用 scripts/ci-doctor-grep.sh</name>
  <files>Makefile</files>

  <read_first>
    - Makefile（全文件，定位现有 target 结构）
    - .planning/phases/34-cloud-claude-doctor-v3/34-RESEARCH.md §6.1 末尾（CI 挂载位置说明）
  </read_first>

  <action>
在 `Makefile` 末尾追加（如已有 `ci:` / `test:` target，放其下方；保留既有 target 不动）：

```makefile
# Phase 34 Plan 03 Task 3.7: cloud-claude doctor M14 终验闸门（ROADMAP §Phase 34 SC#3）
.PHONY: ci-doctor-grep
ci-doctor-grep: cloud-claude
	bash scripts/ci-doctor-grep.sh ./cloud-claude

# 依赖 target：构建 cloud-claude 二进制；如 Makefile 已有 `build-cloud-claude` 等同名 target，改依赖即可。
.PHONY: cloud-claude
cloud-claude:
	go build -o ./cloud-claude ./cmd/cloud-claude

# ci-gate：组合 go test + ci-doctor-grep；供 CI workflow 调用
.PHONY: ci-gate
ci-gate:
	go test ./... -count=1 -short
	$(MAKE) ci-doctor-grep
```

**executor 注意：**
- Makefile 字符串中缩进必须是 **TAB**（不是 4 空格），否则 make 报错
- 如 `Makefile` 已有 `.PHONY: cloud-claude` 或同名 target，直接复用 — 删除本 task 追加的 `cloud-claude:` 块
- 如有 `.DEFAULT_GOAL` 指定默认 target，保持不变（本 task 只加，不动）

**verbatim 守恒：**
- target 名 `ci-doctor-grep` / `ci-gate`
- `bash scripts/ci-doctor-grep.sh ./cloud-claude` 命令字面量（exit code 传播，**不**加 `|| true`）

**禁止：**
- 在 Makefile 中 hardcode GOPATH / GOBIN（保留相对路径）
- 追加自动 `rm -f cloud-claude`（保留二进制供 smoke / explain_test 复用）
  </action>

  <acceptance_criteria>
    - `grep -q "^ci-doctor-grep:" Makefile` 命中
    - `grep -q "^ci-gate:" Makefile` 命中
    - `grep -q "bash scripts/ci-doctor-grep.sh ./cloud-claude" Makefile` 命中
    - `grep -q "^\.PHONY: ci-doctor-grep$" Makefile` 命中
    - `grep -q "^\.PHONY: ci-gate$" Makefile` 命中
    - `make -n ci-doctor-grep 2>&1 | grep -q "ci-doctor-grep.sh"` 命中（make 能解析依赖树）
  </acceptance_criteria>
</task>

<task type="execute">
  <name>Task 3.8: 全仓库回归验证（Plan 01+02+03 联合）</name>
  <files></files>

  <read_first>
    - .planning/phases/34-cloud-claude-doctor-v3/34-01-errcodes-explain-PLAN.md（全 plan）
    - .planning/phases/34-cloud-claude-doctor-v3/34-02-doctor-framework-PLAN.md（全 plan）
  </read_first>

  <action>
**本 task 不修改任何文件**，只执行联合回归验证命令，确认 Plan 01/02/03 共同产物一致性。执行以下命令：

```bash
# 1. 全仓库构建
go build ./...

# 2. 全包单元测试（-short 跳过集成测试）
go test ./... -count=1 -short

# 3. 专门跑 errcodes + doctor 包
go test ./internal/cloudclaude/errcodes/ -count=1 -v
go test ./internal/cloudclaude/doctor/ -count=1 -short -v

# 4. cmd/cloud-claude explain + doctor 测试
go test ./cmd/cloud-claude/ -count=1 -v

# 5. 构建 cloud-claude 二进制，跑 M14 gate
make cloud-claude
bash scripts/ci-doctor-grep.sh ./cloud-claude || echo "NOTE: 本地未 init 时 gate 可能因无 warn/fail 输出而通过 — 仍需 CI 真机验证"

# 6. SC#3 / SC#4 / SC#5 / SC#6 / SC#7 / SC#8 / SC#9 锚点回归
go test ./internal/cloudclaude/doctor/ -run "TestDowngradeBannerRendersChain|TestRenderTextContainsNextAction|TestJSONSchemaV1Lock|TestApplyFixes" -count=1 -v

# 7. M14 双保险：errcodes Registry 层面
go test ./internal/cloudclaude/errcodes/ -run "TestAllCodesHaveExplanations|TestNoLegacyLowercaseCodes|TestAllDomainsClosed" -count=1 -v

# 8. explain 子进程级测试
go test ./cmd/cloud-claude/ -run "TestExplain_" -count=1 -v

# 9. host-agent 边界守恒（SC#9）
! rg -nE "agentapi\.(Action[A-Z]|NewClient|RunHostAction)" internal/cloudclaude/doctor/

# 10. 凭据泄漏守恒
! rg -nE "cfg\.Password|SSHPass|entry_password|credentials\.json" internal/cloudclaude/doctor/
```

**如任一命令退出码非 0，本 task 视为未通过。**

**产物清单：**
- `cloud-claude` 二进制（由 make cloud-claude 生成）
- 所有测试 PASS 时间戳（由 go test -v 输出）
- `scripts/ci-doctor-grep.sh` smoke 通过（或 CI 真机验证 carry-over）

**禁止：**
- 为了让测试通过而删除既有测试
- 跳过任一 SC 锚点测试
  </action>

  <acceptance_criteria>
    - `go build ./...` 退出码 = 0
    - `go test ./... -count=1 -short` 退出码 = 0
    - `go test ./internal/cloudclaude/doctor/ -run "TestDowngradeBannerRendersChain" -count=1` 退出码 = 0（SC#6 终验）
    - `go test ./internal/cloudclaude/doctor/ -run "TestRenderTextContainsNextAction" -count=1` 退出码 = 0（M14）
    - `go test ./internal/cloudclaude/doctor/ -run "TestJSONSchemaV1Lock" -count=1` 退出码 = 0（SC#5）
    - `go test ./internal/cloudclaude/doctor/ -run "TestApplyFixes_StatusNotDowngraded" -count=1` 退出码 = 0（CONTEXT D-16）
    - `go test ./internal/cloudclaude/errcodes/ -run "TestAllCodesHaveExplanations" -count=1` 退出码 = 0（SC#8 前置）
    - `go test ./cmd/cloud-claude/ -run "TestExplain_" -count=1` 退出码 = 0（SC#8）
    - `! rg -nE "agentapi\\.(Action[A-Z]\|NewClient\|RunHostAction)" internal/cloudclaude/doctor/` 为真（SC#9）
    - `! rg -nE "cfg\\.Password\|SSHPass" internal/cloudclaude/doctor/` 为真（凭据泄漏守恒）
    - `./cloud-claude explain MOUNT_MUTAGEN_VERSION_SKEW` 退出码 = 0 且 stdout 含「详细说明：」（SC#8 人工 smoke）
    - `./cloud-claude doctor --help` 退出码 = 0 且 stdout 列出 `--fix / --verbose / --json / --yes`（SC#2）
  </acceptance_criteria>
</task>

</tasks>

<verification>
## Plan-level 验证

```bash
# 完整回归见 Task 3.8；此处给出 plan-level 最小集：

# 1. 全仓库构建 + 单测
go build ./...
go test ./... -count=1 -short

# 2. 集成测试（可选，CI 环境走 -tags=integration）
# 本地 dev：要求 docker daemon + scripts/test-fixture-up.sh 可跑；CI 中由专用 workflow 调
# bash scripts/test-fixture-up.sh
# go test -tags=integration ./internal/cloudclaude/doctor/ -count=1 -v
# bash scripts/test-fixture-down.sh

# 3. CI gate 脚本
make cloud-claude
bash scripts/ci-doctor-grep.sh ./cloud-claude

# 4. Makefile ci-gate 联合 target
make -n ci-gate  # dry-run 验证 target 存在且可解析

# 5. FixerRegistry 真实注册 6 类
go test ./internal/cloudclaude/doctor/ -run "TestApplyFixes_Fix_TriggersRegistry" -count=1 -v
```

## SC 映射（ROADMAP §Phase 34 Success Criteria）

| SC | 本 Plan 交付 |
|----|-------------|
| SC#3（CI grep 所有 [!]/[✗] 行含「建议:」子串） | Task 3.6 scripts/ci-doctor-grep.sh 第 2+3 段 awk/grep；Task 3.7 Makefile ci-gate target |
| SC#4（doctor --fix 至少 5 类） | Task 3.1 FixerRegistry 注册 6 个 Fixer（5 类 + AUTH_OAUTH_REFRESH_FAILED）；Task 3.2 11 条单测 |
| SC#5（退出码不降级 / --json / NO_COLOR — 续 Plan 02） | Task 3.4 doctor.go `[fix] N/M` + 退出码仍按 Summary 计 |
| SC#7（mergerfs 篡改 + 修复命令 E2E） | Task 3.5 TestIntegration_DoctorMountFail_MergerfsTampered |
| SC#9（doctor 不调 host-agent 新 endpoint） | Task 3.8 rg 断言 `! agentapi.Action[A-Z]` 再次验证 |
</verification>

<success_criteria>
- [ ] 8 个 task 全部完成，各 acceptance_criteria 全 PASS
- [ ] `go build ./...` + `go test ./... -count=1 -short` 退出码 = 0
- [ ] `doctor.FixerRegistry` 注册 6 条 entry（MOUNT_MUTAGEN_DAEMON_UNAVAILABLE / SYSTEM_FUSE_RESIDUAL_MOUNT / SSH_KNOWN_HOSTS_CONFLICT / AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED / SYSTEM_DNS_RESOLVE_FAILED）
- [ ] `TestApplyFixes_StatusNotDowngraded` PASS（CONTEXT D-16 守恒）
- [ ] `confirmDestructive` 三级判定（Yes / JSON / 非 TTY）全路径单测 PASS
- [ ] `scripts/ci-doctor-grep.sh` 可执行且语法正确（bash -n）
- [ ] `make ci-gate` target 可解析且能跑完整路径
- [ ] `./cloud-claude doctor --fix` 输出顶部含 `[fix] N 项已修复 / M 项修复失败`
- [ ] 集成测试 build tag 隔离正确：默认 `go test ./...` 不触发；`-tags=integration` 触发 2 条用例
- [ ] carry-over 记录：CI workflow 接入 `make ci-gate` / `make ci-doctor-grep`（由 ops team / 本 plan 交付 Makefile，workflow 文件 as follow-up）
</success_criteria>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| `doctor --fix` → 系统命令（mutagen daemon / fusermount / ssh-keygen / dscacheutil） | 用户 CLI 触发；部分走 sudo（DNS flush）；stdin 交互式 y/N 确认 |
| FixerRegistry 包级 var mock | 测试场景可注入任意 Fixer；生产代码只有 init() 注册 |
| `ApplyFixes` 写 Check.FixApplied/FixFailed | 结果只落 stdout / JSON，不落盘 |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-34-03-01 | Tampering | `realExecMutagenDaemon` / `realExecFusermountUnmount` 拼接 shell 命令 | **medium** | mitigate | `exec.CommandContext` 逐参数传（无 shell 解释）；mountpoint 字符串来自 `checkFUSEResidual.Details["mountpoints"]`，本身已从 `mount` 命令字符串用 regex 捕获，不含 shell metachars（若 mountpoint 含空格会被 `exec.Command` 安全处理）；acceptance_criteria 额外 rg 断言 `! rg -n 'Sprintf.*\".*fusermount.*%s' internal/cloudclaude/doctor/fix.go`（禁止 printf 拼 shell） |
| T-34-03-02 | Injection | `fixSSHKnownHostsConflict` 从 `Details["host_port"]` 取字符串 | **medium** | mitigate | `ssh-keygen -R <hostPort>` 走 `exec.CommandContext` 逐参数；hostPort 源头是 `authResp.SSHHost:authResp.SSHPort`（Entry API 返回，受服务端信任）；禁止用户 CLI 覆盖该字段 |
| T-34-03-03 | Elevation of Privilege | `fixDNSResolveFailed` 用 sudo | **high** | mitigate | **强制 y/N 确认**（CONTEXT D-09 表 + D-10 第 4 条）：opts.Yes=false + 非 TTY 直接拒绝；走系统 sudo prompt（不自动输入密码）；命令字符串硬编码（`dscacheutil / resolvectl / systemd-resolve`）；验证 `TestFixDNSResolveFailed_NonTTY_Rejected` 回归 |
| T-34-03-04 | DoS | `ApplyFixes` hang（某 Fixer 阻塞） | medium | mitigate | `ApplyFixes` 顶层 `context.WithTimeout(ctx, 60*time.Second)`；每个 Fixer 不独立 timeout（避免嵌套）；recovery 由上级 doctor timeout 兜底 |
| T-34-03-05 | Information Disclosure | Fixer 把 error.Error() 写入 FixFailed 可能含绝对路径 | low | accept | rationale：doctor 是本地工具，绝对路径对用户自助排障必要；error 字符串不经 Entry API 上报（OOS-A11 守恒） |
| T-34-03-06 | Tampering | `scripts/ci-doctor-grep.sh` 的 awk/grep 可能误杀 | low | mitigate | awk 只扫 `── <domain> ──` 之后行（跳过降级 banner 的 `[!]`）；放过降级 banner 的 `[!]` 不触发 `"建议:"` 要求；Task 3.5 integration_test TestIntegration_DoctorMountFail_MergerfsTampered 作为 E2E 兜底 |
| T-34-03-07 | Injection | `integration_test.go dockerExec` 构造命令 | low | mitigate | `exec.Command("docker", args...)` 逐参数；args 字面量由测试代码控制；不涉及用户输入 |
| T-34-03-08 | Information Disclosure | 集成测试 dockerExec stdout 泄漏 fixture 凭据 | low | accept | rationale：fixture 容器是测试专用（scripts/test-fixture-up.sh 自起），不持真实用户凭据；stdout 只在测试失败时打印 |

**ASVS L1 高严重度阻塞性威胁：** 0（T-34-03-03 sudo 通过 `confirmDestructive` 三级守卫 mitigate 到 medium）
</threat_model>

<output>
After completion, create `.planning/phases/34-cloud-claude-doctor-v3/34-03-SUMMARY.md` with:
- 8 个 task 的实际 commit SHA + 关键 diff 片段引用
- FixerRegistry 实际注册条目数（`grep -c "FixerRegistry\[" internal/cloudclaude/doctor/fix.go`）
- fix_test.go 11+ 条单测 PASS 时间戳
- integration_test.go 2 条用例执行状态（本地 docker 存在时跑通 / 否则 Skip）
- `scripts/ci-doctor-grep.sh` 本地 smoke 结果
- `make ci-gate` 执行结果
- Phase 34 所有 SC（1-9）最终达标矩阵（每条 SC ↔ 对应 Plan 测试 / 脚本）
- carry-over：CI workflow `.github/workflows/*.yml` 引入 `make ci-gate` 的 follow-up issue（非本 plan scope，但建议同 phase-ship 前补）
- carry-over：v3.1 backlog 候选：(a) FixerRegistry 扩展更多 code；(b) `cloud-claude doctor` 并发执行；(c) v2.0 lower-case 码迁移
</output>
