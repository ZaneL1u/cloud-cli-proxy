package errcodes

// NET_OAUTH_* 错误码注册。文案与 Phase 31 PLAN.md <errcode_registry> 表逐字符对齐。

func init() {
	MustRegister(Entry{
		Code:       NET_OAUTH_EXPIRED,
		Severity:   SeverityFatal,
		Message:    "Claude OAuth 凭证已过期（账号: %s）",
		NextAction: "在容器内运行 cloud-claude exec claude login 重新登录",
	})

	MustRegister(Entry{
		Code:       NET_OAUTH_EXPIRING_SOON,
		Severity:   SeverityWarn,
		Message:    "Claude OAuth 凭证将在 %d 分钟后过期",
		NextAction: "建议尽快 cloud-claude exec claude login",
	})

	MustRegister(Entry{
		Code:       NET_OAUTH_NOT_FOUND,
		Severity:   SeverityFatal,
		Message:    "容器内未找到 Claude OAuth 凭证文件（账号: %s）",
		NextAction: "在容器内运行 cloud-claude exec claude login 完成首次登录",
	})
}

// NET_RECONNECT_* / NET_TCP_KEEPALIVE_UNSUPPORTED 注册（Phase 32 D-04 / D-05）。
// 文案与 32-RESEARCH.md §8 行 1093-1130 逐字符对齐。
func init() {
	MustRegister(Entry{
		Code:       NET_RECONNECT_BACKOFF,
		Severity:   SeverityInfo,
		Message:    "网络中断，正在重连（已等待 %s）",
		NextAction: "按 Enter 立即重试，或等待自动重连",
	})
	MustRegister(Entry{
		Code:       NET_RECONNECT_GAVE_UP,
		Severity:   SeverityFatal,
		Message:    "重连失败（已重试 %d 次，耗时 %s）",
		NextAction: "请检查网络后重新运行 cloud-claude，或运行 cloud-claude doctor 诊断",
	})
	MustRegister(Entry{
		Code:       NET_TCP_KEEPALIVE_UNSUPPORTED,
		Severity:   SeverityWarn,
		Message:    "TCP keepalive 平台特化失败：%s",
		NextAction: "无需操作；SSH 应用层 keepalive 仍生效，弱网检测可能略慢",
	})
}
