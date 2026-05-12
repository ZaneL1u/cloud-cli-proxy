package network

import "context"

// Provider defines the contract for setting up and tearing down per-host
// network isolation. Implementations must be safe for concurrent use.
//
// Phase 45 Plan 02 起，host 创建路径上 PrepareGateway 必须先于 worker 容器
// 的 docker create / start 调用：
//   - PrepareGateway 负责「create network + start gateway + 等 sing-box
//     healthy + 写 sing-box config / placeholder / DNS 源文件」，保证 worker
//     容器启动时 ro bind mount 引用的源文件已存在、tun0 (172.19.0.1) 已监听；
//   - PrepareHost 在 worker 容器 docker start 之后调用，仅负责「connect
//     worker netns + configure routes + verify + 把控制面接入隔离网络」。
//
// CleanupHost 与 PrepareGateway 是反操作：它会停止 gateway 容器、删除网络、
// 清理 worker firewall 与配置目录。
type Provider interface {
	PrepareGateway(context.Context, HostNetworkSpec) error
	PrepareHost(context.Context, HostNetworkSpec) error
	CleanupHost(context.Context, HostNetworkSpec) error
}
