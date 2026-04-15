package cloudclaude

import (
	"fmt"
	"io"
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
// 将 localDir 映射到容器内 /workspace。
// 返回的 cleanup 函数按正确顺序关闭所有资源。
func mountWorkspace(conn *ssh.Client, localDir string) (cleanup func(), err error) {
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

	if err := sshfsSession.Start("sshfs : /workspace -o passive -f"); err != nil {
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

	check := func() error {
		sess, err := conn.NewSession()
		if err != nil {
			return err
		}
		defer sess.Close()
		return sess.Run("mountpoint -q /workspace")
	}

	if err := waitForMount(check, 200*time.Millisecond, 10*time.Second); err != nil {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn)
		return nil, fmt.Errorf("等待挂载就绪失败: %w", err)
	}

	cleanup = func() {
		sshfsSession.Close()
		<-sftpDone
		server.Close()
		fusermountCleanup(conn)
	}
	return cleanup, nil
}

// waitForMount 轮询 check 函数直到挂载就绪或超时。
// check 由调用方注入，便于单元测试。
func waitForMount(check func() error, interval, timeout time.Duration) error {
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
				MountPath: "/workspace",
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

// fusermountCleanup 通过短生命周期 session 执行防御性卸载。
// 所有错误静默忽略——此函数为兜底措施，失败不影响退出码。
func fusermountCleanup(conn *ssh.Client) {
	sess, err := conn.NewSession()
	if err != nil {
		return
	}
	defer sess.Close()
	_ = sess.Run("fusermount -u /workspace 2>/dev/null || true")
}
