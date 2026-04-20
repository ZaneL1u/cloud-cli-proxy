package cloudclaude

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

type SSHConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

// ConnectAndRunClaude 是 v2.0 兼容入口（保持签名不变）。
// 内部转 ConnectAndRunClaudeV3 + Mode=ModeSSHFSOnly + 默认 MountConfig，
// 让旧调用继续走 sshfs-only 路径，与 v2.0 行为一致。
func ConnectAndRunClaude(cfg SSHConfig, claudeArgs []string, cwd string, proxyCommands []string) (int, error) {
	mountCfg := MountConfig{
		Mode:              ModeSSHFSOnly,
		KeepAliveInterval: 15 * time.Second,
		KeepAliveCountMax: 4,
	}
	return ConnectAndRunClaudeV3(cfg, claudeArgs, cwd, proxyCommands, mountCfg, nil)
}

// ConnectAndRunClaudeV3 是 Phase 31 主入口。
//
// 流程：
//  1. 建立 conn-A（控制 + 远端探测） / conn-B（数据通道，本阶段保留接口）
//  2. 用 authResp 字段（ClaudeAccountID / ImageVersion / SupportsMutagen /
//     SupportsMergerfs）补全 mountCfg；NoColor / Logger / LastSessionPath /
//     SyncSessionLock 取默认值
//  3. 调 MountWorkspace 按 cfg.Mode 调度三层 mount + 三段式进度 + banner
//  4. 启动 ExecProxy（沿用 v2.0 行为）
//  5. TODO(plan-03): mount ready 后、runClaude 前插入 OAuth credentials 检查
//  6. runClaude 在 conn-A 上启动远程 claude，沿用 v2.0 PTY/window resize 逻辑
//
// 任何 mount error 已被 errcodes.Format 包装，可直接 stderr 输出。
func ConnectAndRunClaudeV3(cfg SSHConfig, claudeArgs []string, cwd string,
	proxyCommands []string, mountCfg MountConfig, authResp *AuthResponse,
) (int, error) {
	connA, err := sshConnect(cfg)
	if err != nil {
		return 0, err
	}
	defer connA.Close()

	connB, err := sshConnect(cfg)
	if err != nil {
		return 0, err
	}
	defer connB.Close()

	if authResp != nil {
		if mountCfg.ClaudeAccountID == "" {
			mountCfg.ClaudeAccountID = authResp.ClaudeAccountID
		}
		if mountCfg.ImageVersion == "" {
			mountCfg.ImageVersion = authResp.ImageVersion
		}
		mountCfg.SupportsMutagen = authResp.SupportsMutagen
		mountCfg.SupportsMergerfs = authResp.SupportsMergerfs
	}
	if mountCfg.LastSessionPath == "" {
		if home, herr := os.UserHomeDir(); herr == nil {
			mountCfg.LastSessionPath = filepath.Join(home, ".cloud-claude", "last-session.json")
		}
	}
	if mountCfg.Logger == nil {
		mountCfg.Logger = os.Stderr
	}
	if mountCfg.SyncSessionLock == nil {
		mountCfg.SyncSessionLock = func(_ string) (func(), error) {
			return func() {}, nil
		}
	}
	mountCfg.Cwd = cwd

	cleanupMount, _, mErr := MountWorkspace(connA, connB, mountCfg)
	if mErr != nil {
		return 0, fmt.Errorf("文件映射失败: %w", mErr)
	}
	defer cleanupMount()

	var proxy *ExecProxy
	if len(proxyCommands) > 0 {
		proxy = NewExecProxy(cwd)
		if err := proxy.Start(); err != nil {
			return 0, fmt.Errorf("启动命令代理失败: %w", err)
		}
		defer proxy.Stop()

		if err := InstallWrappers(cwd, proxyCommands, cwd); err != nil {
			return 0, fmt.Errorf("安装命令代理脚本失败: %w", err)
		}
	}

	// OAuth 检查（CONTEXT D-22 / D-23 / D-24；Plan 03 Task 3.2）：
	//   - claude_account_id 缺失 → 跳过 + 中文提示（不阻塞 mount）
	//   - mount ready 后执行；Expired / NotFound 命中 → cleanup mount + 退出非 0
	//   - ExpiringSoon (< 5min) 仅警告，不阻断
	//
	// 退出码引用 cloudclaude.Exit* 命名常量（避开 v2.0 main.go ExitConfigError=4 /
	// ExitInternalError=5 撞码；OAuthNotFound=6 / OAuthExpired=7）。
	if mountCfg.ClaudeAccountID == "" {
		fmt.Fprintln(mountCfg.Logger, "[!] gateway 未返回 claude_account_id，跳过 OAuth 过期检查（建议升级 gateway 至 v3.0）")
	} else {
		status, oauthErr := CheckOAuthCredentials(connA, mountCfg.ClaudeAccountID)
		if oauthErr != nil {
			// CheckOAuthCredentials 内部已收敛错误到 OAuthNotFound；理论上 err 不会非 nil。
			fmt.Fprintln(mountCfg.Logger, "[!] OAuth 检查异常: "+oauthErr.Error())
		} else {
			switch status.State {
			case OAuthExpired:
				fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_EXPIRED, mountCfg.ClaudeAccountID))
				return ExitOAuthExpired, nil
			case OAuthNotFound:
				fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_NOT_FOUND, mountCfg.ClaudeAccountID))
				return ExitOAuthNotFound, nil
			case OAuthExpiringSoon:
				fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_EXPIRING_SOON, status.MinutesToExpire))
			case OAuthValid:
				// 不输出（避免噪音）
			}
		}
	}

	// [Phase 32 D-15 / D-28 / D-29] tmux 探测 + 路由：
	//   - DetectTmux 失败 → SESSION_TMUX_UNAVAILABLE warning + 走 v2.0 runClaude（不阻塞启动 / REQ-F4-C）
	//   - 成功 → runClaudeWithSession（tmux new-session -A 包装 + RunKeepAlive + Reconnector + BufferedStdin）
	//
	// 注：ConnectAndRunClaudeV3 当前签名无 context.Context（Phase 31 未引入），
	// 这里使用 context.Background()；用户 Ctrl+C 由 PTY 主循环 / SIGWINCH goroutine
	// 自然处理；后续若需 ctx-cancel 由独立 phase 调整签名。
	sessionCfg := SessionConfig{
		AccountID:         mountCfg.ClaudeAccountID,
		ShortID:           mountCfg.SessionShortID,
		TakeOver:          mountCfg.SessionTakeOver,
		KeepAliveInterval: mountCfg.KeepAliveInterval,
		KeepAliveCountMax: mountCfg.KeepAliveCountMax,
		ReconnectEnabled:  true,
		NoColor:           mountCfg.NoColor,
		Cwd:               cwd,
		LocalHostname:     mountCfg.LocalHostname,
		LastSessionPath:   mountCfg.LastSessionPath,
	}
	available, _, reason := DetectTmux(connA)
	sessionCfg.TmuxAvailable = available
	if !available {
		fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.SESSION_TMUX_UNAVAILABLE, reason))
		return runClaude(connA, claudeArgs, cwd, len(proxyCommands) > 0)
	}
	return runClaudeWithSession(context.Background(), connA, cfg, claudeArgs, sessionCfg, len(proxyCommands) > 0)
}

// SSHConnect 暴露给 cmd 层 sessions 子命令使用（Phase 32 Task 2.3）。
// 仅是 sshConnect 的 export 包装，行为完全一致：
//
//   - 拨号 + ssh.NewClientConn + ssh.NewClient
//   - 拨号成功后 best-effort ConfigureTCPKeepAlive（Phase 32 Plan 01）
//
// 调用方 (cmd/cloud-claude/sessions.go) 用此入口拿 *ssh.Client 直接喂给
// internal/cloudclaude.RunSessionsLs / RunSessionsAttach。
func SSHConnect(cfg SSHConfig) (*ssh.Client, error) { return sshConnect(cfg) }

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

	// [Phase 32 D-04] TCP keepalive — best-effort，失败仅 warning 不阻塞。
	if tc, ok := tcpConn.(*net.TCPConn); ok {
		if e := ConfigureTCPKeepAlive(tc, 15*time.Second); e != nil {
			fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.NET_TCP_KEEPALIVE_UNSUPPORTED, e.Error()))
		}
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, clientCfg)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("SSH 握手失败: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

func runClaude(conn *ssh.Client, claudeArgs []string, remoteCwd string, hasProxy bool) (int, error) {
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
	var remoteCmd string
	if hasProxy {
		binDir := remoteCwd + "/.cloud-claude/bin"
		remoteCmd = fmt.Sprintf("export PATH=%s:$PATH && cd %s && %s",
			shellescape.Quote(binDir), shellescape.Quote(remoteCwd), claudeCmd)
	} else {
		remoteCmd = fmt.Sprintf("cd %s && %s", shellescape.Quote(remoteCwd), claudeCmd)
	}

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
