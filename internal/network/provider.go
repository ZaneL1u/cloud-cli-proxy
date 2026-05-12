package network

import "context"

// Provider defines the contract for setting up and tearing down per-host
// network isolation.
//
// 并发语义（Phase 45 WR-08 修正）：
//   - 同一 hostID 上的 Prepare* / Cleanup* 调用 **必须由调用方串行化**。
//     实现内部不持锁；多个 goroutine 用同一 hostID 并发调用会让
//     teardownGateway 与 dockerNetworkCreate 等步骤交错，导致 network
//     冲突、配置目录被反复 wipe / write 等竞态。
//   - 不同 hostID 之间互不影响，可并行调用。
//
// 单宿主机控制面通过任务队列（同一 hostID 一次只有一个 worker）天然满足
// 串行约束；如果未来把 Provider 嵌入到管理多并发的 SDK / agent 中，调用方
// 必须自行加 per-hostID mutex。
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
