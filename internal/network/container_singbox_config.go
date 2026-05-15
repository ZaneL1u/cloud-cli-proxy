package network

import (
	"encoding/json"
	"fmt"
)

// buildContainerSingBoxConfig 渲染 v4.0 (Phase 54) 单容器架构下，user 容器内自跑
// sing-box 的 config JSON。
//
// 与 v3.5 buildGatewaySingBoxConfig 的差异：
//   - v3.5 sing-box 跑在 sidecar gateway 容器（隔离 bridge 网络），address
//     172.19.0.1/30 是 gateway tun0 IP；
//   - v4.0 sing-box 跑在 user 容器自身 netns，tun0 仍可保留同一 address（仅容器
//     内可见，不与 docker bridge 网段冲突）；
//   - v4.0 容器内 entrypoint lock_resolv_conf 已写死 nameserver 127.0.0.1，
//     sing-box DNS 模块通过 hijack-dns route 规则接管；
//   - v4.0 不引入 v3.5 whitelist rule-set（bypass 白名单是 v3.5 网络架构特有，
//     v4.0 同容器先不带，待 v4.1 评估）。
//
// helper 复用：buildGatewayProxyOutbound / buildGatewayDirectOutbound /
// buildGatewayDNS 来自 gateway_singbox_config.go（54-CONTEXT D-54-7 dead-code
// 留待 v4.1 一次性清理；本 plan 直接复用以减少 diff 面积）。
func buildContainerSingBoxConfig(outboundRaw json.RawMessage, dnsServer, proxyServerIP string) ([]byte, error) {
	_ = dnsServer
	proxyOut, err := buildGatewayProxyOutbound(outboundRaw, proxyServerIP)
	if err != nil {
		return nil, err
	}
	directOut, err := buildGatewayDirectOutbound()
	if err != nil {
		return nil, err
	}
	tunIn, err := buildContainerTunInbound()
	if err != nil {
		return nil, err
	}

	cfg := map[string]any{
		"log":       map[string]any{"level": "info"},
		"dns":       buildContainerDNS(),
		"inbounds":  []json.RawMessage{tunIn},
		"outbounds": []json.RawMessage{proxyOut, directOut},
		"route": map[string]any{
			"default_interface":       "eth0",
			"default_domain_resolver": map[string]any{"server": "dns-local"},
			"rules":                   buildContainerRouteRules(proxyServerIP),
			"final":                   "proxy-out",
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// buildContainerTunInbound 渲染容器内 tun inbound。
//
// 与 buildGatewayTunInbound 的关键差异：v4.0 同容器架构下不需要
// endpoint_independent_nat —— 流量走向是 user 进程 → tun0 → sing-box → eth0
// 单向链路，无回程 NAT 需求；strict_route + auto_route 与 v3.5 对齐，stack
// 仍走内核 system 栈（性能优于 gvisor）。
func buildContainerTunInbound() (json.RawMessage, error) {
	raw, err := json.Marshal(map[string]any{
		"type":         "tun",
		"tag":          "tun-in",
		"address":      []string{"172.19.0.1/30"},
		"auto_route":   true,
		"strict_route": true,
		"stack":        "system",
		"sniff":        true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal container tun inbound: %w", err)
	}
	return raw, nil
}

// buildContainerDNS 渲染容器内 sing-box dns 模块（精简版）：
//   - dns-local（type=local）解析内网 .lan / .local / .internal 等后缀
//   - dns-proxy（type=https，server=1.1.1.1，detour=proxy-out）走 DoH 解析公网
//
// 与 buildGatewayDNS 的差异：v4.0 删除 v3.5 whitelist-domains rule-set 引用；
// 仅保留按域名后缀直连内网的 1 条规则；其余流量经 final=dns-proxy 走代理 DoH。
// dns.strategy 仍 ipv4_only —— 容器内全禁 IPv6 与 entrypoint nft 规则配套。
func buildContainerDNS() map[string]any {
	return map[string]any{
		"servers": []map[string]any{
			{
				"tag":  "dns-local",
				"type": "local",
			},
			{
				"tag":             "dns-proxy",
				"type":            "https",
				"server":          "1.1.1.1",
				"domain_resolver": "dns-local",
				"detour":          "proxy-out",
			},
		},
		"rules": []map[string]any{
			{
				"domain_suffix": []string{".lan", ".local", ".internal"},
				"action":        "route",
				"server":        "dns-local",
			},
		},
		"final":    "dns-proxy",
		"strategy": "ipv4_only",
	}
}

// buildContainerRouteRules 渲染容器内 sing-box route.rules（精简版，无 v3.5
// whitelist rule-set 引用 —— bypass 白名单是 v3.5 网络架构特有，v4.0 同容器
// 待 v4.1 评估）。
//
// 4 条规则（顺序必须保留）：
//  1. action=sniff（嗅探 tls/http/quic/dns）
//  2. protocol=dns + action=hijack-dns（让所有 DNS 流量经 sing-box DNS 模块）
//  3. ip_cidr=<proxy_ip>/32 → direct（避免 sing-box 回环访问自身代理服务器）
//  4. ip_is_private=true → direct（RFC1918 / CGNAT / 链路本地 / ULA）
//
// route.final=proxy-out 在调用方设置兜底。
func buildContainerRouteRules(proxyServerIP string) []map[string]any {
	return []map[string]any{
		{"action": "sniff", "sniffer": []string{"tls", "http", "quic", "dns"}},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_cidr": []string{proxyServerIP + "/32"}, "action": "route", "outbound": "direct"},
		{"ip_is_private": true, "action": "route", "outbound": "direct"},
	}
}
