package network

import (
	"context"
	"log/slog"
	"os"
	"runtime"
)

// Provider 定义了每个 host 的网络隔离的建立与拆除契约。
//
// 并发语义（Phase 45 WR-08 修正）：
//   - 同一 hostID 上的 Prepare* / Cleanup* 调用 **必须由调用方串行化**。
//     实现内部不持锁；多个 goroutine 用同一 hostID 并发调用会让写盘 /
//     RemoveAll / verify 等步骤交错，导致 config.json 被反复 wipe / write
//     等竞态。
//   - 不同 hostID 之间互不影响，可并行调用。
//
// 单宿主机控制面通过任务队列（同一 hostID 一次只有一个 worker）天然满足
// 串行约束；如果未来把 Provider 嵌入到管理多并发的 SDK / agent 中，调用方
// 必须自行加 per-hostID mutex。
//
// Phase 54-01 单容器化后，host 创建路径上 PrepareGateway 必须先于 worker
// 容器的 docker create / start 调用：
//   - PrepareGateway 负责「mkdir SingBoxConfigDir + 写 sing-box config」，保证
//     worker 容器启动时 ro bind mount 引用的 /etc/sing-box/config.json 已存在；
//   - PrepareHost 在 worker 容器 docker start 之后调用，仅负责
//     verifier.Verify（出口 IP / DNS / leak 三检）。
//
// CleanupHost 是反操作：清理 host 端 SingBoxConfigDir 目录（best-effort 幂等）。
// 容器自身的销毁由 worker.stopHost / rebuildHost 路径上的 docker stop / rm 负责。
type Provider interface {
	PrepareGateway(context.Context, HostNetworkSpec) error
	PrepareHost(context.Context, HostNetworkSpec) error
	CleanupHost(context.Context, HostNetworkSpec) error
}

// NetworkVerifier 是网络验证的抽象接口。
// 不同平台 / 部署形态提供各自的验证实现。
type NetworkVerifier interface {
	Verify(ctx context.Context, containerName string, egress EgressConfig) (VerifyResult, error)
}

// NewProvider 返回平台适配的 Provider 实现。
//
// Verifier 选择规则（按优先级）：
//   1. SKIP_EGRESS_VERIFY=true → NopVerifier（Windows/macOS Docker Desktop 开发环境）
//   2. runtime.GOOS != "linux" → NopVerifier（非 Linux 原生环境）
//   3. 默认 → DockerVerifier（Linux 生产环境，通过 docker exec 做真实网络验证）
func NewProvider(logger *slog.Logger) Provider {
	var verifier NetworkVerifier
	if os.Getenv("SKIP_EGRESS_VERIFY") == "true" {
		verifier = &NopVerifier{}
	} else if runtime.GOOS == "linux" {
		verifier = &DockerVerifier{}
	} else {
		verifier = &NopVerifier{}
	}
	return NewContainerProxyProvider(logger, verifier)
}
