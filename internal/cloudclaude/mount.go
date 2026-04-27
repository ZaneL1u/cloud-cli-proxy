// Package cloudclaude — 文件挂载共享 helper。
//
// Phase 31 拆分后 mount.go 仅承载 sshfs / hotsync / merge / strategy 四档共享 helper：
//   - MountNotReadyError / channelRWC：基础类型
//   - waitForMount / fusermountCleanup / cleanupStaleFUSE / rmdirChain / sshRun / shellQuote
//
// 具体 mount 实现见 mount_{sshfs,hotsync,merge,strategy}.go。
package cloudclaude

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// MountNotReadyError 表示挂载点在超时时间内未就绪。
type MountNotReadyError struct {
	MountPath string
	Timeout   time.Duration
	LastErr   error
}

func (e *MountNotReadyError) Error() string {
	return fmt.Sprintf("挂载 %s 超时（%v）: %v", e.MountPath, e.Timeout, e.LastErr)
}

func (e *MountNotReadyError) Unwrap() error {
	return e.LastErr
}

// channelRWC 将 SSH session 的 stdin/stdout pipe 适配为 io.ReadWriteCloser，
// 供 sftp.NewServer 使用。
// Reader = StdoutPipe()（读取 sshfs 输出的 SFTP 请求）
// WriteCloser = StdinPipe()（向 sshfs stdin 写入 SFTP 响应）
type channelRWC struct {
	io.Reader
	io.WriteCloser
}

func (c *channelRWC) Close() error {
	return c.WriteCloser.Close()
}

// waitForMount 轮询 check 函数直到挂载就绪或超时。
func waitForMount(mountPath string, check func() error, interval, timeout time.Duration) error {
	var lastErr error
	if err := check(); err == nil {
		return nil
	} else {
		lastErr = err
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			return &MountNotReadyError{
				MountPath: mountPath,
				Timeout:   timeout,
				LastErr:   lastErr,
			}
		case <-ticker.C:
			if err := check(); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
}

// fusermountCleanup 防御性卸载指定挂载点。
func fusermountCleanup(conn *ssh.Client, remotePath string) {
	_ = sshRun(conn, fmt.Sprintf("fusermount -u %s 2>/dev/null || true", shellQuote(remotePath)))
}

// cleanupStaleFUSE 清理可能因上次异常退出而残留的 FUSE 挂载。
func cleanupStaleFUSE(conn *ssh.Client, remotePath string) {
	_ = sshRun(conn, fmt.Sprintf("fusermount -u %s 2>/dev/null || true", shellQuote(remotePath)))
}

// rmdirChain 从叶子目录开始向上逐级删除空目录，遇到非空即停。
func rmdirChain(conn *ssh.Client, path string) {
	for path != "/" && path != "." && path != "" {
		if err := sshRun(conn, fmt.Sprintf("sudo rmdir %s 2>/dev/null || rmdir %s 2>/dev/null", shellQuote(path), shellQuote(path))); err != nil {
			return
		}
		parent := filepath.Dir(path)
		if parent == path {
			return
		}
		path = parent
	}
}

// sshRun 在 SSH 连接上执行一条命令，返回错误。
func sshRun(conn *ssh.Client, cmd string) error {
	sess, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.Run(cmd)
}

// shellQuote 为 shell 参数添加单引号转义。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
