package network

import (
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
}

// HostNetworkSpec carries everything the network Provider needs to wire a container.
type HostNetworkSpec struct {
	HostID       string
	ContainerPID uint32        // container init PID, populated after docker start
	Egress       *EgressConfig // nil when Provider should skip network setup
}

// BypassSingboxUID 是 gateway 容器内 sing-box 进程的 uid（与 sing-gateway Dockerfile 对齐）。
// 在 worker netns 的 nft output 链做 uid 锁：仅该 uid 能 TCP 连到代理服务器 IP:443。
// 注意：worker netns 与 gateway netns 是两个 namespace，meta skuid 匹配的是
// worker 容器内的进程 uid。v3.5 设计下 sing-box 跑在 gateway 容器，worker 容器内
// 没有 sing-box，故 uid 锁实际作用是：worker 容器内任何 uid 都不能直连代理服务器
// （除非通过 sb-tun0），这是 fail-closed 的最强形态。暴露该常量是为了未来如果切到
// sidecar 同 netns 模式时保持单点变更。
const BypassSingboxUID uint32 = 1000

// BypassNftSetName 是 worker netns nft inet 表内白名单 set 的名字。
// Phase 47 Plan 01 的 ApplyBypassRuleSet 通过 `nft -f - flush set <table> whitelist_v4`
// 动态更新该 set 内容（type ipv4_addr / flags interval / auto-merge）。
const BypassNftSetName = "whitelist_v4"

// BypassNftLogPrefix 是链末兜底 drop 规则的 log prefix，syslog 中可用此前缀过滤计数。
// 尾部留一个空格与 nft CLI `log prefix "sbfw-drop "` 输出对齐。
const BypassNftLogPrefix = "sbfw-drop "
