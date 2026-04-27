package errcodes

// MOUNT_* 错误码注册。文案与 Phase 31 PLAN.md <errcode_registry> 表逐字符对齐。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       MOUNT_HOT_SYNC_FAILED,
		Severity:   SeverityError,
		Message:    "热同步失败: %s",
		NextAction: "检查当前目录可读写、远端 staging 路径权限，或回退到 sshfs-only",
	})

	MustRegister(Entry{
		Code:       MOUNT_SSHFS_FAILED,
		Severity:   SeverityError,
		Message:    "sshfs 挂载失败: %s",
		NextAction: "检查 /dev/fuse 是否可用，或运行 cloud-claude doctor ssh",
	})

	MustRegister(Entry{
		Code:       MOUNT_SSHFS_DISCONNECTED,
		Severity:   SeverityWarn,
		Message:    "sshfs 已断开 ≥15 秒，已从 mergerfs 摘除 /workspace-cold",
		NextAction: "网络恢复后运行 cloud-claude doctor mount --fix 重新挂载",
	})

	MustRegister(Entry{
		Code:       MOUNT_MERGERFS_FAILED,
		Severity:   SeverityError,
		Message:    "mergerfs 挂载失败: %s",
		NextAction: "检查容器是否启用 SYS_ADMIN + /dev/fuse，或运行 cloud-claude doctor mount",
	})

	MustRegister(Entry{
		Code:       MOUNT_AUTO_DOWNGRADED,
		Severity:   SeverityWarn,
		Message:    "文件映射已从 %s 降级到 %s，原因: [%s] %s",
		NextAction: "运行 cloud-claude doctor mount 查看详细修复建议",
	})

	MustRegister(Entry{
		Code:       MOUNT_FORCE_MODE_FAILED,
		Severity:   SeverityFatal,
		Message:    "--mount-mode=%s 模式下 %s 层失败: %s",
		NextAction: "移除 --mount-mode flag 让自动降级生效，或运行 cloud-claude doctor mount",
	})

	MustRegister(Entry{
		Code:       MOUNT_APFS_CASE_INSENSITIVE,
		Severity:   SeverityInfo,
		Message:    "检测到 macOS APFS case-insensitive 文件系统，已强制启用 two-way-resolved 同步模式",
		NextAction: "无需操作；如需 case-sensitive 行为请创建 case-sensitive APFS 卷",
	})

	MustRegister(Entry{
		Code:       MOUNT_REQUIRE_GIT_REPO,
		Severity:   SeverityError,
		Message:    "当前目录 %s 不在 git 仓库内，cloud-claude 拒绝挂载以避免误同步整个家目录",
		NextAction: "cd 到 git 仓库根目录后重试，或在当前目录运行 git init 后再启动 cloud-claude",
	})

	MustRegister(Entry{
		Code:       MOUNT_OVERSIZED_FILE_SKIPPED,
		Severity:   SeverityWarn,
		Message:    "%s (%dMB) 超过 hot_sync_max_file_mb=%d 阈值，已跳过热同步，由 cold sshfs 兜底",
		NextAction: "编辑 ~/.cloud-claude/config.yaml 调整 hot_sync_max_file_mb，或在 .gitignore 加入该路径",
	})

	// WR-01：doctor mount git_proxy_enabled / default_ignore_loaded 两条 check
	// 之前误用 AUTH_CONFIG_MISSING（Severity=Fatal + Sprintf 占位符），
	// 渲染出「~/.cloud-claude/config.yaml 不存在或解析失败: ...」误导文案。
	// 注册下述两个 Warn 级专属 Code，Message 不带占位符。
	MustRegister(Entry{
		Code:       MOUNT_GIT_PROXY_DISABLED,
		Severity:   SeverityWarn,
		Message:    "proxy_commands 未包含 git，git 子命令不会走本地代理转发",
		NextAction: "编辑 ~/.cloud-claude/config.yaml 的 proxy_commands 字段加入 git 后重启 cloud-claude",
	})

	MustRegister(Entry{
		Code:       MOUNT_DEFAULT_IGNORE_DISABLED,
		Severity:   SeverityWarn,
		Message:    "CLOUD_CLAUDE_NO_DEFAULT_IGNORE=1，默认二进制黑名单已禁用，大文件可能进入热同步",
		NextAction: "如非排查需要，请 unset CLOUD_CLAUDE_NO_DEFAULT_IGNORE 后重启 cloud-claude",
	})

	MustRegister(Entry{
		Code:       MOUNT_PROMOTER_FAILED,
		Severity:   SeverityWarn,
		Message:    "远程 cold-promoter 启动失败: %s，降级为无晋升模式（cold 分支仍可读写）",
		NextAction: "检查容器内 SSH session 是否可用，或设 CLOUD_CLAUDE_NO_PROMOTION=1 禁用晋升",
	})
}
