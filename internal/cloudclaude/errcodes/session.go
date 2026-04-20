package errcodes

// SESSION_* 错误码注册（Phase 32）。文案与 32-RESEARCH.md §8 行 1056-1092 逐字符对齐。
//
//nolint:lll

func init() {
	MustRegister(Entry{
		Code:       SESSION_KEEPALIVE_TOO_AGGRESSIVE,
		Severity:   SeverityFatal,
		Message:    "SSH KeepAlive 间隔 %s 低于 15s 下限",
		NextAction: "调整 keepalive_interval 至 >= 15s，或移除该配置使用默认值",
	})
	MustRegister(Entry{
		Code:       SESSION_TMUX_UNAVAILABLE,
		Severity:   SeverityWarn,
		Message:    "容器内 tmux 不可用：%s，会话恢复已禁用",
		NextAction: "检查容器镜像是否升级到 v3.0.0，或运行 cloud-claude doctor mount",
	})
	MustRegister(Entry{
		Code:       SESSION_NOT_FOUND,
		Severity:   SeverityError,
		Message:    "tmux 会话 %s 不存在",
		NextAction: "运行 cloud-claude sessions ls 查看当前会话列表",
	})
	MustRegister(Entry{
		Code:       SESSION_TAKEOVER_NOTIFIED,
		Severity:   SeverityInfo,
		Message:    "已通知其它 %d 个客户端断开（session: %s）",
		NextAction: "无需操作；其它客户端 3 秒后将看到中断提示",
	})
	MustRegister(Entry{
		Code:       SESSION_TAKEOVER_FAILED,
		Severity:   SeverityError,
		Message:    "tmux detach-client 命令失败: %s",
		NextAction: "运行 cloud-claude sessions ls 检查会话状态，或 cloud-claude doctor",
	})
	MustRegister(Entry{
		Code:       SESSION_SYNC_LOCKED,
		Severity:   SeverityWarn,
		Message:    "账号 %s 已有另一端在执行 Mutagen sync，本端只读 sshfs 视图",
		NextAction: "无需操作；如需独占同步，请先关闭另一端 cloud-claude",
	})
	MustRegister(Entry{
		Code:       SESSION_BUFFER_OVERFLOW,
		Severity:   SeverityWarn,
		Message:    "本地输入缓冲已满（4KB），部分历史输入已丢弃",
		NextAction: "等待网络恢复后重新输入丢失部分；避免在断网期间粘贴大段内容",
	})
}
