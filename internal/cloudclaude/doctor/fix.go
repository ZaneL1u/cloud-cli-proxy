// Package doctor — Phase 34 Plan 03：doctor --fix 自动修复 + FixerRegistry + confirmDestructive。
//
// 4 类修复（CONTEXT D-09 表，经移除 Mutagen 后）：
//  1. SYSTEM_FUSE_RESIDUAL_MOUNT       → fusermount -u <path>（批量 y/N 确认）
//  2. SSH_KNOWN_HOSTS_CONFLICT         → ssh-keygen -R <host:port>（低危 / 免确认）
//  3. AUTH_TOKEN_EXPIRED / AUTH_OAUTH_REFRESH_FAILED → 重调 EntryClient.AuthenticateAndWait（低危 / 免确认）
//  4. SYSTEM_DNS_RESOLVE_FAILED        → macOS dscacheutil / Linux resolvectl（sudo + y/N 确认）
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

// FixerRegistry 按错误码路由到对应 Fixer。Plan 03 初始化 6 类（5 类业务 + AUTH_OAUTH_REFRESH_FAILED 派生）；v3.1 可扩展。
// 本包初始化 (init()) populate，测试可通过 `originalRegistry := FixerRegistry; FixerRegistry = nil; defer ...` 隔离。
var FixerRegistry = map[errcodes.Code]Fixer{}

// 4 个包级 var mock 注入点（PATTERNS §2.9 / worker.go 样板）。
// 暴露为 package-level 变量以便单元测试注入 fake。
var execFusermountUnmount = realExecFusermountUnmount
var execSSHKeygenRemove = realExecSSHKeygenRemove
var execEntryRefresh = realExecEntryRefresh
var execDNSFlush = realExecDNSFlush

// isTerminalFD 是 term.IsTerminal 的包级 var（便于测试注入非 TTY 场景）。
var isTerminalFD = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func init() {
	FixerRegistry[errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT] = fixFUSEResidualMount
	FixerRegistry[errcodes.SSH_KNOWN_HOSTS_CONFLICT] = fixSSHKnownHostsConflict
	FixerRegistry[errcodes.AUTH_TOKEN_EXPIRED] = fixAuthTokenExpired
	FixerRegistry[errcodes.AUTH_OAUTH_REFRESH_FAILED] = fixAuthOAuthRefreshFailed
	FixerRegistry[errcodes.SYSTEM_DNS_RESOLVE_FAILED] = fixDNSResolveFailed
}

// ----------------------------------------------------------------------------
// confirmDestructive — 三级判定（CONTEXT D-10）：
//  1. opts.Yes=true           → true  （CI 友好）
//  2. opts.JSON=true          → false + 调用方写 FixFailed «JSON 模式禁止交互式修复，请在终端模式重试或追加 --yes»
//  3. 非 TTY（stdin 非 pipe）  → false + 调用方写 FixFailed «非 TTY 环境，请追加 --yes 或在终端重试»
//  4. 否则交互 y/N（提示字面量风格与 mutagen 一致）
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
	_, _ = fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		return true, ""
	}
	return false, "用户取消"
}

// ----------------------------------------------------------------------------
// 1. SYSTEM_FUSE_RESIDUAL_MOUNT — fusermount -u <path>（批量 y/N 确认）
// ----------------------------------------------------------------------------

func fixFUSEResidualMount(ctx context.Context, opts Options, original Check) ([]string, []string) {
	// Details["mountpoints"] 由 checkFUSEResidual 提供（Task 3.3 给 mount.go 的 checkFUSEResidual 加 Details）
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
	if strings.Contains(msg, "not mounted") {
		return true
	}
	if strings.Contains(msg, "no such file") {
		return true
	}
	if strings.Contains(msg, "device or resource busy") {
		return false
	}
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
