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
	diskWarnMB = 500
	diskFailMB = 100
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
