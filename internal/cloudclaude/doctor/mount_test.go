package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

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

func TestCheckMergerfsBranches_AllPresent_Pass(t *testing.T) {
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

// ── Phase 36 D-13 新增 5 项 check 矩阵测试 ───────────────────────────────────

// scriptedRunner 按 script name 返回不同 stdout（mount_test 内部第二个 mock）。
type scriptedRunner struct {
	scriptResults map[string]string
}

func (s *scriptedRunner) RunScript(name, script string) (string, string, error) {
	if s.scriptResults != nil {
		if v, ok := s.scriptResults[name]; ok {
			return v, "", nil
		}
	}
	return "", "", nil
}

// --- checkSSHFSCacheArgs ---

func TestCheckSSHFSCacheArgs_Pass_AllParamsPresent(t *testing.T) {
	rr := &scriptedRunner{
		scriptResults: map[string]string{
			"sshfs_mount": "user@host:/ on /workspace-cold type fuse.sshfs " +
				"(rw,nosuid,nodev,relatime,user_id=0,group_id=0,allow_other," +
				"passive,reconnect,ServerAliveInterval=15,ServerAliveCountMax=3," +
				"ConnectTimeout=10,cache=yes,kernel_cache,auto_cache,cache_timeout=300)",
		},
	}
	c := checkSSHFSCacheArgs(context.Background(), rr)
	if c.Status != StatusPass {
		t.Errorf("4 参数全在应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckSSHFSCacheArgs_Fail_MissingKernelCache(t *testing.T) {
	rr := &scriptedRunner{
		scriptResults: map[string]string{
			// 缺 kernel_cache
			"sshfs_mount": "user@host:/ on /workspace-cold type fuse.sshfs " +
				"(rw,cache=yes,auto_cache,cache_timeout=300)",
		},
	}
	c := checkSSHFSCacheArgs(context.Background(), rr)
	if c.Status != StatusFail {
		t.Errorf("缺 kernel_cache 应 Fail，实际 %s", c.Status)
	}
	if c.Code != errcodes.MOUNT_SSHFS_FAILED {
		t.Errorf("Code 应为 MOUNT_SSHFS_FAILED，实际 %q", c.Code)
	}
	if !strings.Contains(c.Message, "kernel_cache") {
		t.Errorf("Message 应提及 kernel_cache，实际 %q", c.Message)
	}
}

func TestCheckSSHFSCacheArgs_Skip_NilRunner(t *testing.T) {
	c := checkSSHFSCacheArgs(context.Background(), nil)
	if c.Status != StatusSkip {
		t.Errorf("nil runner 应 Skip，实际 %s", c.Status)
	}
}

// --- checkOversizedFilesCount ---
//
// LoadLastSession() 通过 os.UserHomeDir() 读取 $HOME/.cloud-claude/last-session.json，
// 这里通过 t.Setenv("HOME", t.TempDir()) 隔离，构造 last-session.json 控制三种状态。

func TestCheckOversizedFilesCount_Skip_NoSessionFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := checkOversizedFilesCount(context.Background())
	if c.Status != StatusSkip {
		t.Errorf("无 last-session.json 应 Skip，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckOversizedFilesCount_Pass_EmptyList(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dir := filepath.Join(tmpHome, ".cloud-claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "last-session.json"),
		[]byte(`{"schema_version":1,"timestamp":"2026-04-23T00:00:00Z","intended_mode":"full","actual_mode":"full","downgrade_chain":[],"conflict_count":0,"apfs_case_insensitive":false}`),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := checkOversizedFilesCount(context.Background())
	if c.Status != StatusPass {
		t.Errorf("空 OversizedFiles 应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckOversizedFilesCount_Warn_NonEmpty(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dir := filepath.Join(tmpHome, ".cloud-claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "last-session.json"),
		[]byte(`{"schema_version":1,"timestamp":"2026-04-23T00:00:00Z","intended_mode":"full","actual_mode":"full","downgrade_chain":[],"conflict_count":0,"apfs_case_insensitive":false,"oversized_files":[{"path":"big.bin","size_bytes":62914560}]}`),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := checkOversizedFilesCount(context.Background())
	if c.Status != StatusWarn {
		t.Errorf("非空 OversizedFiles 应 Warn，实际 %s (msg=%q)", c.Status, c.Message)
	}
	if c.Code != errcodes.MOUNT_OVERSIZED_FILE_SKIPPED {
		t.Errorf("Code 应为 MOUNT_OVERSIZED_FILE_SKIPPED，实际 %q", c.Code)
	}
	if got, _ := c.Details["oversized_count"].(int); got != 1 {
		t.Errorf("Details.oversized_count 应为 1，实际 %v", c.Details["oversized_count"])
	}
}

// --- checkRequireGitRepo ---

func TestCheckRequireGitRepo_Pass_InGitRepo(t *testing.T) {
	c := checkRequireGitRepo(context.Background())
	if c.Status != StatusPass {
		t.Errorf("当前 workspace 应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckRequireGitRepo_Fail_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Skipf("chdir 到临时目录失败，跳过：%v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	c := checkRequireGitRepo(context.Background())
	if c.Status != StatusFail {
		t.Errorf("非 git 目录应 Fail，实际 %s (msg=%q)", c.Status, c.Message)
	}
	if c.Code != errcodes.MOUNT_REQUIRE_GIT_REPO {
		t.Errorf("Code 应为 MOUNT_REQUIRE_GIT_REPO，实际 %q", c.Code)
	}
}

// --- checkGitProxyEnabled ---

func TestCheckGitProxyEnabled_Pass_ContainsGit(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dir := filepath.Join(tmpHome, ".cloud-claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("gateway: https://example.com\nusername: test\npassword: test\nproxy_commands: [git, curl]\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := checkGitProxyEnabled(context.Background())
	if c.Status != StatusPass {
		t.Errorf("含 git proxy 应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckGitProxyEnabled_Warn_NoGit(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dir := filepath.Join(tmpHome, ".cloud-claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("gateway: https://example.com\nusername: test\npassword: test\nproxy_commands: [curl]\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c := checkGitProxyEnabled(context.Background())
	if c.Status != StatusWarn {
		t.Errorf("不含 git proxy 应 Warn，实际 %s (msg=%q)", c.Status, c.Message)
	}
	// WR-01：从 AUTH_CONFIG_MISSING 改为专属 MOUNT_GIT_PROXY_DISABLED Code。
	if c.Code != errcodes.MOUNT_GIT_PROXY_DISABLED {
		t.Errorf("Code 应为 MOUNT_GIT_PROXY_DISABLED，实际 %q", c.Code)
	}
}

func TestCheckGitProxyEnabled_Skip_NoConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := checkGitProxyEnabled(context.Background())
	if c.Status != StatusSkip {
		t.Errorf("config 未 init 应 Skip，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

// --- checkDefaultIgnoreLoaded ---

func TestCheckDefaultIgnoreLoaded_Pass_NotSet(t *testing.T) {
	t.Setenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE", "")
	c := checkDefaultIgnoreLoaded(context.Background())
	if c.Status != StatusPass {
		t.Errorf("env 未设应 Pass，实际 %s (msg=%q)", c.Status, c.Message)
	}
}

func TestCheckDefaultIgnoreLoaded_Warn_Set(t *testing.T) {
	t.Setenv("CLOUD_CLAUDE_NO_DEFAULT_IGNORE", "1")
	c := checkDefaultIgnoreLoaded(context.Background())
	if c.Status != StatusWarn {
		t.Errorf("env=1 应 Warn，实际 %s", c.Status)
	}
	// WR-01：从 AUTH_CONFIG_MISSING 改为专属 MOUNT_DEFAULT_IGNORE_DISABLED Code。
	if c.Code != errcodes.MOUNT_DEFAULT_IGNORE_DISABLED {
		t.Errorf("Code 应为 MOUNT_DEFAULT_IGNORE_DISABLED，实际 %q", c.Code)
	}
}
