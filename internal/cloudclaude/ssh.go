package cloudclaude

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type SSHConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

// ConnectAndRunClaude 建立 SSH 连接，挂载本地目录到容器 /workspace，
// 然后在远端以 /workspace 为工作目录执行 claude 命令。
// defer 顺序保证清理链：cleanupMount → conn.Close（LIFO）。
func ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string) (int, error) {
	conn, err := sshConnect(cfg)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	cleanupMount, err := mountWorkspace(conn, cwd)
	if err != nil {
		return 0, fmt.Errorf("目录映射失败: %w", err)
	}
	defer cleanupMount()

	return runClaude(conn, claudeArgs)
}

func sshConnect(cfg SSHConfig) (*ssh.Client, error) {
	clientCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	tcpConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败（无法连接 %s）: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

func runClaude(conn *ssh.Client, claudeArgs []string) (int, error) {
	session, err := conn.NewSession()
	if err != nil {
		return 0, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	isTTY := term.IsTerminal(fd)

	if isTTY {
		width, height := 80, 24
		if w, h, err := term.GetSize(fd); err == nil {
			width, height = w, h
		}

		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return 0, fmt.Errorf("设置终端 raw 模式失败: %w", err)
		}
		defer term.Restore(fd, oldState)

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}

		if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
			return 0, fmt.Errorf("申请 PTY 失败: %w", err)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		go func() {
			for range sigCh {
				if w, h, err := term.GetSize(fd); err == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}()
		defer signal.Stop(sigCh)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	claudeCmd := shellescape.QuoteCommand(append([]string{"claude"}, claudeArgs...))
	remoteCmd := "cd /workspace && " + claudeCmd

	if err := session.Start(remoteCmd); err != nil {
		return 0, fmt.Errorf("启动远程 Claude Code 失败: %w", err)
	}

	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		if err == io.EOF {
			return 0, nil
		}
		return 0, fmt.Errorf("SSH 会话异常结束: %w", err)
	}

	return 0, nil
}
