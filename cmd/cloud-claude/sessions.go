// cmd/cloud-claude/sessions.go — Phase 32 Plan 02 Task 2.3
//
// cloud-claude sessions 子命令树（mirror cmd/cloud-claude/sync.go 结构）：
//   - sessions ls       列出当前 tmux 会话（远程 tmux list-sessions）
//   - sessions attach   attach 到指定 tmux 会话（远程 tmux has-session 校验 → exec attach）
//
// 业务逻辑全部在 internal/cloudclaude.RunSessionsLs / RunSessionsAttach（session.go）；
// 本文件仅负责 cobra 路由 + 凭证加载 + SSH 拨号。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sessions",
		Short:         "tmux 会话管理（v3.0 SSH 会话可靠性）",
		Long:          "查看 / attach 容器内由 cloud-claude 创建的 tmux 会话；零控制面改造，纯客户端逻辑。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	lsCmd := &cobra.Command{
		Use:           "ls",
		Short:         "列出当前 tmux 会话",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSessionsLs,
	}
	attachCmd := &cobra.Command{
		Use:           "attach <name>",
		Short:         "attach 到指定 tmux 会话",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSessionsAttach,
	}
	cmd.AddCommand(lsCmd, attachCmd)
	return cmd
}

func runSessionsLs(cmd *cobra.Command, _ []string) error {
	conn, err := connectForSessions(cmd.Context())
	if err != nil {
		return err
	}
	defer conn.Close()
	return cloudclaude.RunSessionsLs(conn, os.Stdout)
}

func runSessionsAttach(cmd *cobra.Command, args []string) error {
	conn, err := connectForSessions(cmd.Context())
	if err != nil {
		return err
	}
	defer conn.Close()
	// hasProxy=false / cwd="/workspace"：sessions attach 直接进既有 tmux session，
	// 不再启 claude，无需 PATH 注入；cwd 占位（runClaudePTYBare 内部不使用）。
	code, err := cloudclaude.RunSessionsAttach(conn, args[0], false, "/workspace")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// connectForSessions 复用 runEnvCheck 的凭证加载 + 认证 + SSH 拨号模板，
// 仅返回已 ConfigureTCPKeepAlive 过的 *ssh.Client。
func connectForSessions(ctx context.Context) (*ssh.Client, error) {
	cfg, err := cloudclaude.LoadConfig()
	if err != nil {
		return nil, err
	}
	client := cloudclaude.NewEntryClient(cfg.Gateway)
	authResp, err := client.AuthenticateAndWait(ctx, cfg.Username, cfg.Password, func(string) {})
	if err != nil {
		return nil, fmt.Errorf("认证失败: %w", err)
	}
	sshCfg := cloudclaude.SSHConfig{
		Host:     authResp.SSHHost,
		Port:     authResp.SSHPort,
		User:     authResp.SSHUser,
		Password: authResp.SSHPass,
	}
	return cloudclaude.SSHConnect(sshCfg)
}
