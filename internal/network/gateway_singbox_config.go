package network

import (
	"encoding/json"
	"fmt"
)

// Rule-set placeholder 常量：Phase 45 引入。
// sing-box 1.11+ 支持 route.rule_set[type=local] 引用本地文件；这里把白名单 cidrs /
// domains 两个 rule-set 的 tag、容器内路径与空规则集占位字面量集中定义，供渲染层
// 与 PrepareHost 写盘逻辑共享。占位文件由 Phase 47 host-agent 原子替换。
const (
	ruleSetWhitelistCIDRsName   = "whitelist-cidrs"
	ruleSetWhitelistDomainsName = "whitelist-domains"
	ruleSetWhitelistCIDRsPath   = "/etc/sing-box/whitelist-cidrs.json"
	ruleSetWhitelistDomainsPath = "/etc/sing-box/whitelist-domains.json"

	// ruleSetPlaceholder 是 sing-box rule-set source format v3 的空规则集字面量。
	// sing-box 1.11+ 支持；用作 Phase 45 placeholder，Phase 47 由 host-agent 覆盖。
	ruleSetPlaceholder = `{"version":3,"rules":[]}` + "\n"
)

// buildGatewaySingBoxConfig builds sing-box JSON for the sidecar gateway (tun mode).
// tun + auto_route captures all forwarded traffic from the worker container.
//
// dnsServer kept for signature stability; split-DNS uses fixed 1.1.1.1 via DoH. See Plan 45-02.
func buildGatewaySingBoxConfig(outboundRaw json.RawMessage, dnsServer, proxyServerIP string) ([]byte, error) {
	proxyOut, err := buildGatewayProxyOutbound(outboundRaw, proxyServerIP)
	if err != nil {
		return nil, err
	}
	directOut, err := buildGatewayDirectOutbound()
	if err != nil {
		return nil, err
	}
	tunIn, err := buildGatewayTunInbound()
	if err != nil {
		return nil, err
	}

	cfg := map[string]any{
		"log":       map[string]any{"level": "info"},
		"dns":       buildGatewayDNS(),
		"inbounds":  []json.RawMessage{tunIn},
		"outbounds": []json.RawMessage{proxyOut, directOut},
		"route": map[string]any{
			"default_interface": "eth0",
			"rule_set":          buildGatewayRouteRuleSet(),
			"rules":             buildGatewayRouteRules(proxyServerIP),
			"final":             "proxy-out",
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// buildGatewayTunInbound 渲染 tun inbound：Phase 45 启用三参加固
// （strict_route / auto_route / endpoint_independent_nat），stack 由 mixed 升级为 system。
// sniff 保留布尔开关；sniff_override_destination 由 route.rules 第一条
// action=sniff 替代，不再在 inbound 上重复声明。
func buildGatewayTunInbound() (json.RawMessage, error) {
	raw, err := json.Marshal(map[string]any{
		"type":                     "tun",
		"tag":                      "tun-in",
		"address":                  []string{"172.19.0.1/30"},
		"auto_route":               true,
		"strict_route":             true,
		"endpoint_independent_nat": true,
		"stack":                    "system",
		"sniff":                    true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tun inbound: %w", err)
	}
	return raw, nil
}

// buildGatewayDirectOutbound 渲染 direct outbound。
// 显式 bind_interface=eth0 + route.default_interface=eth0 双重保险，
// 防止 auto_route 把 direct 流量回环到 tun（RESEARCH §Pitfall 1）。
// 注意：tag 保留旧值 "direct"（不改为 RESEARCH 中的 "direct-out"），
// 以兼容现有 gateway_singbox_config_test.go 断言，术语漂移由 Phase 47 文档统一。
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

// buildGatewayDNS 渲染拆分 DNS：
//   - dns-local（type=local）解析内网 .lan/.local/.internal 后缀
//   - dns-proxy（type=https，server=1.1.1.1，detour=proxy-out）走 DoH 解析公网白名单域名，
//     domain_resolver=dns-local 用于解析 1.1.1.1 的 SNI 主机名（这里是 IP，但保持配置完整性）
//
// final=dns-proxy；strategy=ipv4_only（v3.5 容器内全禁 IPv6）。
func buildGatewayDNS() map[string]any {
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
			{
				"rule_set": []string{ruleSetWhitelistDomainsName},
				"action":   "route",
				"server":   "dns-proxy",
			},
		},
		"final":    "dns-proxy",
		"strategy": "ipv4_only",
	}
}

// buildGatewayRouteRules 渲染 6 条 route.rules（固定顺序）：
//  1. action=sniff（嗅探 tls/http/quic/dns，替代旧 sniff_override_destination）
//  2. protocol=dns + action=hijack-dns（替代旧 outbound:"dns"）
//  3. ip_cidr=<proxy_ip>/32 → direct（避免 sing-box 自己访问代理 server 时回环）
//  4. ip_is_private=true → direct（RFC1918 / CGNAT / 链路本地 / ULA 内置匹配）
//  5. rule_set=whitelist-cidrs → direct（动态 IP/CIDR 白名单）
//  6. rule_set=whitelist-domains → direct（动态域名白名单）
//
// route.final=proxy-out 在调用方设置，作为兜底。
func buildGatewayRouteRules(proxyServerIP string) []map[string]any {
	return []map[string]any{
		{"action": "sniff", "sniffer": []string{"tls", "http", "quic", "dns"}},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_cidr": []string{proxyServerIP + "/32"}, "action": "route", "outbound": "direct"},
		{"ip_is_private": true, "action": "route", "outbound": "direct"},
		{"rule_set": []string{ruleSetWhitelistCIDRsName}, "action": "route", "outbound": "direct"},
		{"rule_set": []string{ruleSetWhitelistDomainsName}, "action": "route", "outbound": "direct"},
	}
}

// buildGatewayRouteRuleSet 渲染两个 local rule-set 引用：
// type=local 让 sing-box 把文件内容作为规则集源；format=source 对应 v3 schema；
// path 指向 gateway 容器内 /etc/sing-box/ 下的两个 placeholder 文件
// （由 PrepareHost 写盘 + dockerRunGateway ro mount 注入）。
func buildGatewayRouteRuleSet() []map[string]any {
	return []map[string]any{
		{
			"type":   "local",
			"tag":    ruleSetWhitelistCIDRsName,
			"format": "source",
			"path":   ruleSetWhitelistCIDRsPath,
		},
		{
			"type":   "local",
			"tag":    ruleSetWhitelistDomainsName,
			"format": "source",
			"path":   ruleSetWhitelistDomainsPath,
		},
	}
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
