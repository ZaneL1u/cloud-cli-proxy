package errcodes

// Phase 41: remote-ssh doctor 维度错误码注册。
// VS Code Server 进程/端口检测 + forwarding socket 检测 + .vscode-server 磁盘占用。
//
//nolint:lll // 单行 Message 较长属于设计要求

func init() {
	MustRegister(Entry{
		Code:       SSH_VSCODE_SERVER_NOT_RUNNING,
		Severity:   SeverityInfo,
		Message:    "VS Code Server 进程未运行（用户可能未使用 VS Code Remote-SSH）",
		NextAction: "无需操作；如需使用 VS Code Remote-SSH，请通过 VS Code 连接容器",
	})

	MustRegister(Entry{
		Code:       SSH_VSCODE_PORT_NOT_LISTENING,
		Severity:   SeverityWarn,
		Message:    "VS Code Server 进程存在但未监听端口，服务可能未就绪",
		NextAction: "等待 VS Code Server 完成启动，或重启 VS Code Remote-SSH 连接",
	})

	MustRegister(Entry{
		Code:       SSH_FORWARDING_SOCKET_MISSING,
		Severity:   SeverityInfo,
		Message:    "SSH forwarding socket 不存在（可能未建立 VS Code forwarding）",
		NextAction: "无需操作；forwarding 会在 VS Code Remote-SSH 连接时自动建立",
	})

	MustRegister(Entry{
		Code:       SSH_FORWARDING_BLOCKED,
		Severity:   SeverityWarn,
		Message:    "检测到 OUTPUT 链存在 DROP 规则，可能拦截 SSH forwarding 流量",
		NextAction: "检查容器内 iptables OUTPUT 链规则，确认 forwarding 流量未被拦截",
	})

	MustRegister(Entry{
		Code:       DISK_VSCODE_SERVER_WARN,
		Severity:   SeverityWarn,
		Message:    "~/.vscode-server/ 占用 %dMB，超过 500MB 警戒线",
		NextAction: "清理 ~/.vscode-server/extensions-cache/ 或删除不常用的扩展",
	})

	MustRegister(Entry{
		Code:       DISK_VSCODE_SERVER_BLOAT,
		Severity:   SeverityError,
		Message:    "~/.vscode-server/ 占用 %dMB，超过 2GB 严重警戒线",
		NextAction: "运行 rm -rf ~/.vscode-server/ 完全清理（VS Code 重连时会自动重建）",
	})
}
