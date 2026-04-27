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
		return fmt.Errorf("not found in /tmp/known_hosts")
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
		return &cloudclaude.Config{Gateway: "https://gw.example.com", Username: "x", Password: "y"}, nil
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
	checks := []Check{{Code: errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT, Status: StatusFail}}
	out := ApplyFixes(context.Background(), Options{Fix: false}, checks)
	if len(out[0].FixApplied) != 0 || len(out[0].FixFailed) != 0 {
		t.Error("opts.Fix=false 应 noop")
	}
}

func TestApplyFixes_Fix_TriggersRegistry(t *testing.T) {
	calls := 0
	orig := execFusermountUnmount
	execFusermountUnmount = func(ctx context.Context, mp string) error { calls++; return nil }
	t.Cleanup(func() { execFusermountUnmount = orig })
	checks := []Check{
		{Code: errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT, Status: StatusFail, Details: map[string]any{"mountpoints": []string{"/tmp/test-fuse"}}},
		{Code: errcodes.DISK_LOCAL_LOW, Status: StatusWarn}, // 不在 Registry，应跳过
	}
	out := ApplyFixes(context.Background(), Options{Fix: true, Yes: true}, checks)
	if len(out[0].FixApplied) == 0 {
		t.Error("FUSE residual Fixer 应触发")
	}
	if len(out[1].FixApplied) != 0 {
		t.Error("DISK_LOCAL_LOW 不在 Registry，不应触发")
	}
	if calls != 1 {
		t.Errorf("应调 1 次 fusermount，实际 %d", calls)
	}
}

func TestApplyFixes_StatusNotDowngraded(t *testing.T) {
	orig := execFusermountUnmount
	execFusermountUnmount = func(ctx context.Context, mp string) error { return nil }
	t.Cleanup(func() { execFusermountUnmount = orig })
	checks := []Check{{Code: errcodes.SYSTEM_FUSE_RESIDUAL_MOUNT, Status: StatusFail, Details: map[string]any{"mountpoints": []string{"/tmp/test-fuse"}}}}
	out := ApplyFixes(context.Background(), Options{Fix: true, Yes: true}, checks)
	if out[0].Status != StatusFail {
		t.Errorf("CONTEXT D-16：Status 不降级，应保留 fail，实际 %s", out[0].Status)
	}
	if len(out[0].FixApplied) == 0 {
		t.Error("FixApplied 应非空（已修复标记）")
	}
}
