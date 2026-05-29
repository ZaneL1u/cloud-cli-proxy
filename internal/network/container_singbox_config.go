package network

import (
	"encoding/json"
	"fmt"
)

// bypassRuleSetDir 容器内白名单 rule-set 文件目录，与 bypass_reload_apply.go::bypassContainerDir 对齐。
const bypassRuleSetDir = "/etc/cloud-claude/bypass"

// buildContainerSingBoxConfig 渲染 v4.0 单容器架构 sing-box config JSON。
//
// bypass 白名单通过 route.rule_set (type=local) 引用容器内文件：
//   - /etc/cloud-claude/bypass/whitelist-cidrs.json   → rule_set "bypass-cidrs"
//   - /etc/cloud-claude/bypass/whitelist-domains.json → rule_set "bypass-domains"
//
// DNS 和 route 规则均引用这些 rule_set。sing-box 1.12+ 支持 rule_set 在 route 块内
// 定义 + DNS/route 规则引用，文件 watcher 自动热加载。
//
// 文件由 ApplyBypassRuleSet 通过 docker exec 写入。
func buildContainerSingBoxConfig(outboundRaw json.RawMessage, _ /*dnsServer*/, proxyServerIP string) ([]byte, error) {
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
	dnsDirectIn, err := buildContainerDNSDirectInbound()
	if err != nil {
		return nil, err
	}

	cfg := map[string]any{
		"log":      map[string]any{"level": "info"},
		"dns":      buildContainerDNS(),
		"inbounds": []json.RawMessage{tunIn, dnsDirectIn},
		"outbounds": []json.RawMessage{proxyOut, directOut},
		"route":    buildContainerRoute(proxyServerIP),
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// buildContainerDNS 渲染 DNS 模块。
// dns-direct (UDP 8.8.8.8 detour=direct) 用于 bypass 域名的直连解析。
func buildContainerDNS() map[string]any {
	return map[string]any{
		"servers": []map[string]any{
			{
				"tag":  "dns-local",
				"type": "local",
			},
			{
				"tag":    "dns-direct",
				"type":   "udp",
				"server": "8.8.8.8",
				"detour": "direct",
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
			{
				"rule_set": "bypass-domains",
				"action":   "route",
				"server":   "dns-direct",
			},
		},
		"final":    "dns-proxy",
		"strategy": "ipv4_only",
	}
}

// buildContainerRoute 渲染 route 块（含 rule_set 定义和引用）。
func buildContainerRoute(proxyServerIP string) map[string]any {
	return map[string]any{
		"default_interface":       "eth0",
		"default_domain_resolver": map[string]any{"server": "dns-local"},
		"rules": []map[string]any{
			{"action": "sniff", "sniffer": []string{"tls", "http", "quic", "dns"}},
			{"protocol": "dns", "action": "hijack-dns"},
			{"ip_cidr": []string{proxyServerIP + "/32"}, "action": "route", "outbound": "direct"},
			{"ip_is_private": true, "action": "route", "outbound": "direct"},
			{"rule_set": "bypass-cidrs", "action": "route", "outbound": "direct"},
			{"rule_set": "bypass-domains", "action": "route", "outbound": "direct"},
		},
		"final": "proxy-out",
		"rule_set": []map[string]any{
			{
				"type":   "local",
				"tag":    "bypass-cidrs",
				"format": "source",
				"path":   bypassRuleSetDir + "/whitelist-cidrs.json",
			},
			{
				"type":   "local",
				"tag":    "bypass-domains",
				"format": "source",
				"path":   bypassRuleSetDir + "/whitelist-domains.json",
			},
		},
	}
}

// buildContainerDNSDirectInbound 渲染 direct inbound 监听 127.0.0.1:53。
func buildContainerDNSDirectInbound() (json.RawMessage, error) {
	raw, err := json.Marshal(map[string]any{
		"type":        "direct",
		"tag":         "dns-direct",
		"listen":      "127.0.0.1",
		"listen_port": 53,
		"sniff":       true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal container dns direct inbound: %w", err)
	}
	return raw, nil
}

// buildContainerTunInbound 渲染容器内 tun inbound。
func buildContainerTunInbound() (json.RawMessage, error) {
	raw, err := json.Marshal(map[string]any{
		"type":         "tun",
		"tag":          "tun-in",
		"address":      []string{"172.19.0.1/30"},
		"auto_route":   true,
		"strict_route": true,
		"stack":        "gvisor",
		"sniff":        true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal container tun inbound: %w", err)
	}
	return raw, nil
}

// buildGatewayProxyOutbound 渲染 proxy outbound。
func buildGatewayProxyOutbound(userConfig json.RawMessage, resolvedIP string) (json.RawMessage, error) {
	var m map[string]any
	if err := json.Unmarshal(userConfig, &m); err != nil {
		return nil, fmt.Errorf("parse outbound config: %w", err)
	}
	delete(m, "dns_server")
	delete(m, "bind_interface")
	m["tag"] = "proxy-out"
	if resolvedIP != "" {
		m["server"] = resolvedIP
	}
	if tls, ok := m["tls"].(map[string]any); ok {
		if reality, ok := tls["reality"].(map[string]any); ok {
			if enabled, _ := reality["enabled"].(bool); enabled {
				if _, hasUtls := tls["utls"]; !hasUtls {
					tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
				}
			}
		}
	}
	return json.Marshal(m)
}

// buildGatewayDirectOutbound 渲染 direct outbound (bind eth0)。
func buildGatewayDirectOutbound() (json.RawMessage, error) {
	raw, err := json.Marshal(map[string]any{
		"type":           "direct",
		"tag":            "direct",
		"bind_interface": "eth0",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal direct outbound: %w", err)
	}
	return raw, nil
}
