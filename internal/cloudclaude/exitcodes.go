// Package cloudclaude 退出码常量（Phase 31 引入）。
//
// 设计原则：
//  1. 与 v2.0 cmd/cloud-claude/main.go 现有 exit* 常量数值完全对齐（0-5）
//  2. v3.0 新增 OAuth / mount 等错误路径占用 6-8（避开 v2.0 行为）
//  3. 全部 ≤ 125（POSIX shell 退出码限制；> 125 与 SIGINT(130) /
//     SIGKILL(137) 等信号编码冲突）
//  4. ssh.go ConnectAndRunClaudeV3 / mount_strategy.go MountWorkspace
//     与 Plan 03 OAuth 检查应引用这些常量而非裸数字
//
// CONTEXT D-22 原约定 OAuth NotFound=4 / Expired=5，与 v2.0 ConfigError=4 /
// InternalError=5 撞码；本文件按 plan-checker 反馈修订为 6/7。
// Phase 34 doctor `cloud-claude explain` 子命令将复用此表。
package cloudclaude

const (
	// 0-5：与 v2.0 cmd/cloud-claude/main.go exit* 常量完全对齐（不可改值）
	ExitOK            = 0
	ExitAuthFailed    = 1
	ExitNetworkError  = 2
	ExitTimeout       = 3
	ExitConfigError   = 4
	ExitInternalError = 5

	// 6-8：v3.0 Phase 31 新增（OAuth + mount force）
	ExitOAuthNotFound    = 6 // /home/claude/.claude/.credentials.json 不存在或解析失败（D-22 第 3 条）
	ExitOAuthExpired     = 7 // expiresAt < now（D-22 第 3 条）
	ExitMountForceFailed = 8 // --mount-mode=full|hot-only|sshfs-only 任一档失败
)
