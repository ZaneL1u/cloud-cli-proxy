package network

import (
	"encoding/json"
	"fmt"
)

// buildGatewaySingBoxConfig builds sing-box JSON for the sidecar gateway (tun mode).
// tun + auto_route captures all forwarded traffic from the worker container.
func buildGatewaySingBoxConfig(outboundRaw json.RawMessage, dnsServer, proxyServerIP string) ([]byte, error) {
	if dnsServer == "" {
		dnsServer = "1.1.1.1"
	}

	proxyOut, err := buildGatewayProxyOutbound(outboundRaw, proxyServerIP)
	if err != nil {
		return nil, err
	}
	directOut, err := json.Marshal(map[string]any{
		"type": "direct",
		"tag":  "direct",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal direct outbound: %w", err)
	}

	tunIn, err := json.Marshal(map[string]any{
		"type":         "tun",
		"tag":          "tun-in",
		"address":      []string{"172.19.0.1/30"},
		"auto_route":   true,
		"strict_route": false,
		"stack":        "mixed",
		"sniff":        true,
		"sniff_override_destination": true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tun inbound: %w", err)
	}

	routeRules := []map[string]any{
		{"ip_cidr": []string{proxyServerIP + "/32"}, "outbound": "direct"},
		{"port": 53, "action": "hijack-dns"},
	}

	cfg := map[string]any{
		"log": map[string]any{"level": "info"},
		"dns": map[string]any{
			"servers": []map[string]any{
				{"tag": "dns-remote", "type": "tcp", "server": dnsServer, "detour": "proxy-out"},
			},
			"strategy": "ipv4_only",
		},
		"inbounds":  []json.RawMessage{json.RawMessage(tunIn)},
		"outbounds": []json.RawMessage{proxyOut, directOut},
		"route": map[string]any{
			"rules":                 routeRules,
			"final":                 "proxy-out",
			"auto_detect_interface": true,
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

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
