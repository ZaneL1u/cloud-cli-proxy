package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
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
