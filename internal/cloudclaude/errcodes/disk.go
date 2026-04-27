package errcodes

// DISK_* 错误码注册（Phase 34 D-21）。本地 + 容器 disk usage 警戒线。
// 阈值（500MB / 100MB / 1GB）硬编码在 doctor/disk.go，不进 Message（CONTEXT Discretion §5）。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       DISK_LOCAL_LOW,
		Severity:   SeverityWarn,
		Message:    "本地 ~/.cloud-claude/ 可用空间 %dMB，低于警戒线",
		NextAction: "清理 ~/.cloud-claude/hotsync/ 或释放本地磁盘",
	})

	MustRegister(Entry{
		Code:       DISK_CONTAINER_LOW,
		Severity:   SeverityWarn,
		Message:    "容器内 /workspace 可用空间 %dMB，低于警戒线",
		NextAction: "清理容器内大文件，或联系管理员扩容 volume",
	})

	MustRegister(Entry{
		Code:       DISK_HOTSYNC_DATA_BLOAT,
		Severity:   SeverityWarn,
		Message:    "HotSync 数据目录 ~/.cloud-claude/hotsync/ 已达 %s，超过 1GB 警戒线",
		NextAction: "运行 rm -rf ~/.cloud-claude/hotsync/sessions/ 后重启 cloud-claude",
	})
}
