package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

const (
	exitOK             = 0
	exitAuthFailed     = 1
	exitNetworkError   = 2
	exitTimeout        = 3
	exitConfigError    = 4
	exitInternalError  = 5
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cloud-claude",
		Short: "透明远程 Claude Code CLI",
		Long:  "连接远端云主机并启动 Claude Code 交互会话。\n首次使用请先运行 cloud-claude init 配置网关与凭证。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRoot,
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "配置网关地址与用户凭证",
		Long:  "交互式输入或通过环境变量/flag 配置网关地址、short_id 和密码，\n写入 ~/.cloud-claude/config.yaml。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runInit,
	}

	initCmd.Flags().String("gateway", "", "网关地址（含 scheme，如 https://gw.example.com）")
	initCmd.Flags().String("short-id", "", "用户或主机 short_id")
	initCmd.Flags().String("password", "", "登录密码（建议交互式输入）")

	rootCmd.AddCommand(initCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %s\n", err)
		os.Exit(exitInternalError)
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	gateway, _ := cmd.Flags().GetString("gateway")
	shortID, _ := cmd.Flags().GetString("short-id")
	password, _ := cmd.Flags().GetString("password")

	if gateway == "" {
		gateway = os.Getenv("CLOUD_CLAUDE_GATEWAY")
	}
	if shortID == "" {
		shortID = os.Getenv("CLOUD_CLAUDE_SHORT_ID")
	}
	if password == "" {
		password = os.Getenv("CLOUD_CLAUDE_PASSWORD")
	}

	if gateway == "" {
		fmt.Print("网关地址 (如 https://gw.example.com): ")
		fmt.Scanln(&gateway)
	}
	if shortID == "" {
		fmt.Print("Short ID: ")
		fmt.Scanln(&shortID)
	}
	if password == "" {
		fmt.Print("密码: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("读取密码失败: %w", err)
		}
		fmt.Println()
		password = string(pw)
	}

	gateway = strings.TrimRight(gateway, "/")

	cfg := &cloudclaude.Config{
		Gateway:  gateway,
		ShortID:  shortID,
		Password: password,
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置无效: %w", err)
	}

	if err := cloudclaude.SaveConfig(cfg); err != nil {
		return err
	}

	path, _ := cloudclaude.ConfigPath()
	fmt.Printf("配置已保存到 %s\n", path)
	return nil
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := cloudclaude.LoadConfig()
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			fmt.Fprintln(os.Stderr, "错误: "+err.Error())
			os.Exit(exitConfigError)
		}
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitConfigError)
		return nil
	}

	client := cloudclaude.NewEntryClient(cfg.Gateway)

	fmt.Println("正在连接云主机...")

	authResp, err := client.AuthenticateAndWait(
		cmd.Context(),
		cfg.ShortID,
		cfg.Password,
		func(msg string) {
			fmt.Printf("\r%s", msg)
		},
	)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "认证失败"):
			fmt.Fprintln(os.Stderr, "错误: "+errMsg)
			os.Exit(exitAuthFailed)
		case strings.Contains(errMsg, "网关不可达") || strings.Contains(errMsg, "网关地址无效"):
			fmt.Fprintln(os.Stderr, "错误: "+errMsg)
			os.Exit(exitNetworkError)
		case strings.Contains(errMsg, "超时"):
			fmt.Fprintln(os.Stderr, "错误: "+errMsg)
			os.Exit(exitTimeout)
		default:
			fmt.Fprintln(os.Stderr, "错误: "+errMsg)
			os.Exit(exitInternalError)
		}
		return nil
	}

	fmt.Println("\r正在进入 Claude Code 会话...")

	sshCfg := cloudclaude.SSHConfig{
		Host:     authResp.SSHHost,
		Port:     authResp.SSHPort,
		User:     authResp.SSHUser,
		Password: authResp.SSHPass,
	}

	if err := cloudclaude.ConnectAndRunClaude(sshCfg); err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitInternalError)
	}

	return nil
}
