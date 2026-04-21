package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// newSyncCmd 构造 cloud-claude sync 父命令树。
//
// 自研 hot sync 当前只暴露 conflicts 查询：
// 读取 ~/.cloud-claude/last-session.json 的 conflict_count，提示用户最近一次会话
// 是否出现自动 resolved 的双向冲突。逐文件清单暂未持久化。
func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "热同步相关工具",
		Long:          "查看 cloud-claude 自研热同步的最近一次冲突计数等状态。\n当前仅实现 sync conflicts 子命令；逐文件冲突清单留后续版本补齐。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	conflictsCmd := &cobra.Command{
		Use:           "conflicts",
		Short:         "查看最近一次热同步冲突计数",
		Long:          "读取 ~/.cloud-claude/last-session.json 中的 conflict_count。\n当前自研热同步会在运行时实时提示冲突，但尚未持久化逐文件清单。",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSyncConflicts,
	}
	cmd.AddCommand(conflictsCmd)
	return cmd
}

// runSyncConflicts 仅读取本地上次会话快照，不发起 SSH 连接。
// 当前实现不再依赖 Mutagen daemon，也不维护逐文件冲突清单。
func runSyncConflicts(cmd *cobra.Command, args []string) error {
	snap, err := cloudclaude.LoadLastSession()
	if err != nil {
		return fmt.Errorf("无法读取最近一次会话快照: %w", err)
	}
	if snap.ConflictCount <= 0 {
		fmt.Println("最近一次会话未记录热同步冲突。")
		return nil
	}
	fmt.Printf("最近一次会话记录到 %d 个热同步冲突。\n", snap.ConflictCount)
	fmt.Println("当前版本仅持久化冲突计数；逐文件冲突会在运行 cloud-claude 时实时提示。")
	return nil
}
