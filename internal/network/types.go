package network

import (
	"context"
	"encoding/json"
)

const (
	TunnelTypeProxy = "proxy"
)

// ProxySpec holds proxy tunnel parameters for sing-box based egress.
type ProxySpec struct {
	OutboundConfig json.RawMessage // sing-box outbound JSON config
	DNSServer      string          // tunnel-side DNS server IP
}

// EgressConfig carries the validated egress binding for a host.
type EgressConfig struct {
	EgressIPID string     // egress_ips.id
	ExpectedIP string     // expected egress IP address (e.g. "1.2.3.4")
	TunnelType string     // "proxy"
	Proxy      *ProxySpec // proxy config

	// UpdateExpectedIP 为可选的出口 IP 自动纠正回调。
	// 当 SOCKS5 探测一致返回与 ExpectedIP 不同的 IP 时，verifier 调用此回调
	// 更新 egress_ips.ip_address，避免因用户填错代理服务器 IP 导致创建失败。
	UpdateExpectedIP func(ctx context.Context, newIP string) error
}

// HostNetworkSpec carries everything the network Provider needs to wire a container.
type HostNetworkSpec struct {
	HostID       string
	ContainerPID uint32        // container init PID, populated after docker start
	Egress       *EgressConfig // nil when Provider should skip network setup
}

// BypassSingboxUID 是容器内 sing-box 进程的 uid（与 Dockerfile 对齐）。
// uid 锁确保仅 sing-box 能直连代理服务器，其余进程 uid 一律走 tun0。
// 暴露该常量是为了未来如果切到同 netns 模式时保持单点变更。
const BypassSingboxUID uint32 = 1000

// BypassNftSetName 是 worker netns nft inet 表内白名单 set 的名字。
// Phase 47 Plan 01 的 ApplyBypassRuleSet 通过 `nft -f - flush set <table> whitelist_v4`
// 动态更新该 set 内容（type ipv4_addr / flags interval / auto-merge）。
const BypassNftSetName = "whitelist_v4"

// BypassNftLogPrefix 是链末兜底 drop 规则的 log prefix，syslog 中可用此前缀过滤计数。
// 尾部留一个空格与 nft CLI `log prefix "sbfw-drop "` 输出对齐。
const BypassNftLogPrefix = "sbfw-drop "
