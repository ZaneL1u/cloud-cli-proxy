package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/local"
	"github.com/zanel1u/cloud-cli-proxy/internal/runtime"
)

func newLocalCmd() *cobra.Command {
	localCmd := &cobra.Command{
		Use:   "local",
		Short: "本地容器管理",
		Long:  "在本地机器上启动、停止和查看 managed-user 容器，无需连接 control-plane。",
		// Default behavior = local up
		RunE: runLocalUp,
	}

	localUpCmd := &cobra.Command{
		Use:   "up",
		Short: "启动本地容器",
		Long:  "启动 managed-user 容器并输出 SSH 连接信息。",
		RunE:  runLocalUp,
	}
	localUpCmd.Flags().Int("port", 0, "SSH 端口（默认 0 = 自动分配）")
	localUpCmd.Flags().String("egress-config", "", "sing-box outbound JSON 文件路径")

	localDownCmd := &cobra.Command{
		Use:   "down",
		Short: "停止并清理本地容器",
		RunE:  runLocalDown,
	}

	localStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "显示本地容器运行状态",
		RunE:  runLocalStatus,
	}

	localCmd.AddCommand(localUpCmd, localDownCmd, localStatusCmd)
	return localCmd
}

func runLocalUp(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %w", err)
	}

	port, _ := cmd.Flags().GetInt("port")
	egressConfig, _ := cmd.Flags().GetString("egress-config")

	// Load image name from image.lock
	imageName := ""
	spec, err := runtime.LoadRuntimeSpec("deploy/docker/managed-user/image.lock")
	if err == nil {
		imageName = spec.ImageName
	}

	opts := local.LocalOptions{
		ProjectDir:   cwd,
		Port:         port,
		EgressConfig: egressConfig,
		ImageName:    imageName,
	}

	mgr := local.NewLocalManager(opts)

	fmt.Println("正在启动本地容器...")
	info, err := mgr.Up(cmd.Context())
	if err != nil {
		return fmt.Errorf("启动本地容器失败: %w", err)
	}

	fmt.Println()
	fmt.Println("本地容器已启动!")
	fmt.Println()
	fmt.Printf("SSH 连接信息:\n")
	fmt.Printf("  Host:     %s\n", info.Host)
	fmt.Printf("  Port:     %s\n", info.Port)
	fmt.Printf("  User:     %s\n", info.User)
	fmt.Printf("  Password: %s\n", info.Password)
	fmt.Println()
	fmt.Printf("快速连接:  ssh %s@%s -p %s\n", info.User, info.Host, info.Port)
	fmt.Printf("VS Code:   ssh %s@%s -p %s\n", info.User, info.Host, info.Port)

	return nil
}

func runLocalDown(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %w", err)
	}

	mgr := local.NewLocalManager(local.LocalOptions{ProjectDir: cwd})
	if err := mgr.Down(cmd.Context()); err != nil {
		return fmt.Errorf("停止本地容器失败: %w", err)
	}

	fmt.Println("本地容器已停止并清理")
	return nil
}

func runLocalStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %w", err)
	}

	mgr := local.NewLocalManager(local.LocalOptions{ProjectDir: cwd})
	status, err := mgr.Status(cmd.Context())
	if err != nil {
		return fmt.Errorf("查询容器状态失败: %w", err)
	}

	if status.Status == "not_found" {
		fmt.Println("本地容器未运行")
		return nil
	}

	fmt.Printf("容器:     %s\n", status.Name)
	fmt.Printf("状态:     %s\n", status.Status)
	fmt.Printf("镜像:     %s\n", status.Image)
	fmt.Printf("SSH 端口: %s\n", status.PortMapping)
	fmt.Printf("创建时间: %s\n", status.CreatedAt)

	return nil
}
