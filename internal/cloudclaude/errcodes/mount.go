package errcodes

// MOUNT_* 错误码注册。文案与 Phase 31 PLAN.md <errcode_registry> 表逐字符对齐。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_VERSION_SKEW,
		Severity:   SeverityError,
		Message:    "Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only",
		NextAction: "升级容器镜像到 v3.0.0+ 或重装 cloud-claude",
	})

	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_WHITELIST_REJECT,
		Severity:   SeverityError,
		Message:    "同步候选目录 %s 体积 %dMB（>50MB），已自动降级 sshfs。当前最大子目录: %s",
		NextAction: "在 .mutagen.yml 添加 ignore 规则，或运行 du -sh %s/* 查看大目录",
	})

	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_SAFETY_GUARD,
		Severity:   SeverityFatal,
		Message:    "检测到本地目录 %s 为空但容器内 /workspace-hot 已有文件，拒绝同步以防反向清空",
		NextAction: "如确认从远端拉取，先 cloud-claude exec rsync /workspace-hot/ ./",
	})

	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_DAEMON_UNAVAILABLE,
		Severity:   SeverityError,
		Message:    "Mutagen daemon 启动失败: %s",
		NextAction: "检查 ~/.cloud-claude/mutagen/ 目录权限，或重启 cloud-claude",
	})

	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_SYNC_FAILED,
		Severity:   SeverityError,
		Message:    "Mutagen sync 创建失败: %s",
		NextAction: "检查 SSH 连通性，或运行 cloud-claude doctor mount",
	})

	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_TRANSPORT_FAILED,
		Severity:   SeverityError,
		Message:    "Mutagen ssh 子进程启动失败: %s",
		NextAction: "检查本机 ssh 客户端是否可用，或安装 sshpass 作为后备",
	})

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
}
