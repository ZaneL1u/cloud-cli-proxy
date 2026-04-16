package cloudclaude

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
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

// mountWorkspace 在 SSH 连接上开启 sshfs session 并启动嵌入式 SFTP server，
// 将 localDir 映射到容器内 remotePath（用户的真实 CWD 路径）。
// 返回的 cleanup 函数按正确顺序关闭所有资源并移除创建的目录。
func mountWorkspace(conn *ssh.Client, localDir, remotePath string) (cleanup func(), err error) {
	// 清理可能残留的上一次 FUSE mount（异常退出场景）
	cleanupStaleFUSE(conn, remotePath)

	// 在容器内创建挂载目标目录并确保当前用户可写（sshfs 要求挂载点由执行用户拥有）
	mkdirCmd := fmt.Sprintf(
		"sudo mkdir -p %s && sudo chown $(id -u):$(id -g) %s",
		shellQuote(remotePath), shellQuote(remotePath),
	)
	if err := sshRun(conn, mkdirCmd); err != nil {
		return nil, fmt.Errorf("创建远端挂载目录失败: %w", err)
	}

	sshfsSession, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 sshfs session 失败: %w", err)
	}

	stdin, err := sshfsSession.StdinPipe()
	if err != nil {
		sshfsSession.Close()
		return nil, fmt.Errorf("获取 sshfs stdin pipe 失败: %w", err)
	}

	stdout, err := sshfsSession.StdoutPipe()
	if err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("获取 sshfs stdout pipe 失败: %w", err)
	}

	sshfsCmd := fmt.Sprintf("sshfs : %s -o passive -f", shellQuote(remotePath))
	if err := sshfsSession.Start(sshfsCmd); err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("启动 sshfs 失败: %w", err)
	}

	rwc := &channelRWC{Reader: stdout, WriteCloser: stdin}

	server, err := sftp.NewServer(rwc, sftp.WithServerWorkingDirectory(localDir))
	if err != nil {
		stdin.Close()
		sshfsSession.Close()
		return nil, fmt.Errorf("创建 SFTP server 失败: %w", err)
	}

	sftpDone := make(chan error, 1)
	go func() {
		sftpDone <- server.Serve()
	}()

	checkCmd := fmt.Sprintf("mountpoint -q %s", shellQuote(remotePath))
	check := func() error {
		sess, err := conn.NewSession()
		if err != nil {
			return err
		}
		defer sess.Close()
		return sess.Run(checkCmd)
	}

	if err := waitForMount(remotePath, check, 200*time.Millisecond, 10*time.Second); err != nil {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn, remotePath)
		return nil, fmt.Errorf("等待挂载就绪失败: %w", err)
	}

	cleanup = func() {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn, remotePath)
		rmdirChain(conn, remotePath)
	}
	return cleanup, nil
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
