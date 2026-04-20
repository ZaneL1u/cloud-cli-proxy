package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
)

var Version = "dev"

const (
	exitOK            = 0
	exitAuthFailed    = 1
	exitNetworkError  = 2
	exitTimeout       = 3
	exitConfigError   = 4
	exitInternalError = 5
)

func main() {
	rootCmd := &cobra.Command{
		Use:                "cloud-claude",
		Short:              "透明远程 Claude Code CLI",
		Long:               "连接远端云主机并启动 Claude Code 交互会话。\n首次使用请先运行 cloud-claude init 配置网关与凭证。",
		Version:            Version,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE:               runRoot,
	}

	initCmd := &cobra.Command{
		Use:           "init",
		Short:         "配置网关地址与用户凭证",
		Long:          "交互式输入或通过环境变量/flag 配置网关地址、short_id 和密码，\n写入 ~/.cloud-claude/config.yaml。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runInit,
	}

	initCmd.Flags().String("gateway", "", "网关地址（含 scheme，如 https://gw.example.com）")
	initCmd.Flags().String("short-id", "", "用户或主机 short_id")
	initCmd.Flags().String("password", "", "登录密码（建议交互式输入）")

	envCmd := &cobra.Command{
		Use:           "env",
		Short:         "环境相关工具",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	envCheckCmd := &cobra.Command{
		Use:           "check",
		Short:         "检测远端容器的时区、语言、出口 IP、工具链等环境信息",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runEnvCheck,
	}
	envCmd.AddCommand(envCheckCmd)

	sshCmd := &cobra.Command{
		Use:           "ssh",
		Short:         "SSH 密钥相关工具",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	sshDoctorCmd := &cobra.Command{
		Use:           "doctor",
		Short:         "体检远端容器 /workspace/.ssh 下的密钥文件（owner/mode/PEM 尾换行）",
		Long:          "扫描远端 /workspace/.ssh 下所有文件，报告 owner 不一致、mode 不规范、PEM 私钥缺末尾换行等常见问题。\n带 --fix 时会尝试自动修复（chown / chmod / 追加换行）。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSSHDoctor,
	}
	sshDoctorCmd.Flags().Bool("fix", false, "尝试自动修复发现的问题（chown / chmod / 追加 PEM 尾换行）")
	sshCmd.AddCommand(sshDoctorCmd)

	// PersistentFlags 注册 --mount-mode；因 rootCmd.DisableFlagParsing=true，
	// runRoot 内会手动解析并从 args 中剥离，避免透传给远端 claude。
	// 这里注册仅用于 --help 显示与 cobra Mark Hidden 等元数据。
	rootCmd.PersistentFlags().String("mount-mode", "auto",
		"文件映射模式: auto|full|mutagen-only|sshfs-only")

	rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd())

	// DisableFlagParsing 会阻止 cobra 识别子命令，
	// 在检测到已知子命令时关闭它以恢复正常路由。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init", "env", "ssh", "sync", "sessions", "help", "--help", "-h":
			rootCmd.DisableFlagParsing = false
		}
	}

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

func runEnvCheck(cmd *cobra.Command, args []string) error {
	cfg, err := cloudclaude.LoadConfig()
	if err != nil {
		return err
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
		return fmt.Errorf("认证失败: %w", err)
	}

	fmt.Println("\r正在检测远端环境...")

	sshCfg := cloudclaude.SSHConfig{
		Host:     authResp.SSHHost,
		Port:     authResp.SSHPort,
		User:     authResp.SSHUser,
		Password: authResp.SSHPass,
	}

	result, err := cloudclaude.RunEnvCheck(sshCfg)
	if err != nil {
		return fmt.Errorf("环境检测失败: %w", err)
	}

	fmt.Println()
	result.Print()
	return nil
}

func runSSHDoctor(cmd *cobra.Command, args []string) error {
	fix, _ := cmd.Flags().GetBool("fix")

	cfg, err := cloudclaude.LoadConfig()
	if err != nil {
		return err
	}

	client := cloudclaude.NewEntryClient(cfg.Gateway)

	fmt.Println("正在连接云主机...")
	authResp, err := client.AuthenticateAndWait(
		cmd.Context(),
		cfg.ShortID,
		cfg.Password,
		func(msg string) { fmt.Printf("\r%s", msg) },
	)
	if err != nil {
		return fmt.Errorf("认证失败: %w", err)
	}

	fmt.Println("\r正在体检远端 /workspace/.ssh ...")

	sshCfg := cloudclaude.SSHConfig{
		Host:     authResp.SSHHost,
		Port:     authResp.SSHPort,
		User:     authResp.SSHUser,
		Password: authResp.SSHPass,
	}

	result, err := cloudclaude.RunSSHDoctor(sshCfg, cloudclaude.SSHDoctorOptions{Fix: fix})
	if err != nil {
		return fmt.Errorf("体检失败: %w", err)
	}

	fmt.Println()
	result.Print()
	return nil
}

func runRoot(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-v" || args[0] == "version") {
		fmt.Printf("cloud-claude %s\n", Version)
		return nil
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	// 因 DisableFlagParsing=true，cobra 不会自动解析 PersistentFlags；
	// 这里手工扫描 --mount-mode / --new-session / --take-over 并从 args 中剥离，
	// 剩余 args 透传给远端 claude（防止 unknown flag 干扰 claude CLI）。
	mountMode := "auto"
	newSession := false
	takeOver := false
	filtered := args[:0]
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--mount-mode" && i+1 < len(args):
			mountMode = args[i+1]
			i++
			continue
		case strings.HasPrefix(args[i], "--mount-mode="):
			mountMode = strings.TrimPrefix(args[i], "--mount-mode=")
			continue
		case args[i] == "--new-session":
			newSession = true
			continue
		case args[i] == "--take-over":
			takeOver = true
			continue
		}
		filtered = append(filtered, args[i])
	}
	args = filtered

	mode, err := cloudclaude.ParseMode(mountMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: --mount-mode 必须是 auto / full / mutagen-only / sshfs-only 之一")
		os.Exit(exitConfigError)
	}

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
		fmt.Fprintln(os.Stderr, "错误: "+errMsg)
		switch {
		case strings.Contains(errMsg, "认证失败"),
			strings.Contains(errMsg, "账号未激活"),
			strings.Contains(errMsg, "未找到对应主机"):
			os.Exit(exitAuthFailed)
		case strings.Contains(errMsg, "网关不可达"),
			strings.Contains(errMsg, "网关地址无效"),
			strings.Contains(errMsg, "认证请求失败"):
			os.Exit(exitNetworkError)
		case strings.Contains(errMsg, "超时"):
			os.Exit(exitTimeout)
		default:
			os.Exit(exitInternalError)
		}
		return nil
	}

	fmt.Println("\r正在映射工作目录并进入 Claude Code 会话...")

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: 无法获取当前工作目录: "+err.Error())
		os.Exit(exitInternalError)
	}

	sshCfg := cloudclaude.SSHConfig{
		Host:     authResp.SSHHost,
		Port:     authResp.SSHPort,
		User:     authResp.SSHUser,
		Password: authResp.SSHPass,
	}

	mountCfg := cloudclaude.MountConfig{
		Mode:              mode,
		KeepAliveInterval: 15 * time.Second,
		KeepAliveCountMax: 4,
		NoColor:           os.Getenv("NO_COLOR") != "",
	}

	// [Phase 32 D-29] 注入 cobra flag 透传 + 本机 hostname。
	mountCfg.SessionTakeOver = takeOver
	if newSession {
		mountCfg.SessionShortID = cloudclaude.GenerateShortSessionID()
	}
	if hostname, _ := os.Hostname(); hostname != "" {
		mountCfg.LocalHostname = hostname
	}

	// [Phase 32 D-03 第 4 条] keepalive_interval < 15s 启动期校验（REQ-F3-A / PITFALLS M11）。
	// 防御未来用户通过环境变量 / config 覆盖默认 15s。
	if mountCfg.KeepAliveInterval > 0 && mountCfg.KeepAliveInterval < 15*time.Second {
		fmt.Fprintln(os.Stderr,
			errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, mountCfg.KeepAliveInterval.String()))
		os.Exit(exitConfigError)
	}

	exitCode, err := cloudclaude.ConnectAndRunClaudeV3(
		sshCfg, args, cwd, cfg.EffectiveProxyCommands(), mountCfg, authResp,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误: "+err.Error())
		os.Exit(exitInternalError)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
