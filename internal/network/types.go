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
