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
		buf.Bavail = 1024 * 1024 // 1M blocks
		buf.Bsize = 4096         // 4K = 4 GB available
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
		buf.Bavail = 100 * 1024 // ~400MB
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
		buf.Bavail = 10 * 1024 // ~40MB
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
		"12K":   0,
		"500K":  0,
		"1024K": 1,
		"3M":    3,
		"1.5G":  1536,
		"2T":    2 * 1024 * 1024,
		"bad":   0,
	}
	for in, want := range cases {
		if got := parseDuHumanToMB(in); got != want {
			t.Errorf("parseDuHumanToMB(%q) = %d, want %d", in, got, want)
		}
	}
}

var _ = fmt.Sprintf
