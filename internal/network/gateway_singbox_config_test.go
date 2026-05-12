package network

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildGatewaySingBoxConfig(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildGatewaySingBoxConfig(outbound, "10.0.0.1", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check top-level keys
	for _, key := range []string{"log", "dns", "inbounds", "outbounds", "route"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	// Verify route has final = "proxy-out"
	route, _ := parsed["route"].(map[string]any)
	if final, _ := route["final"].(string); final != "proxy-out" {
		t.Errorf("route.final = %q, want %q", final, "proxy-out")
	}

	// Verify outbounds contains two entries (proxy-out + direct)
	outbounds, _ := parsed["outbounds"].([]any)
	if outbounds == nil {
		t.Fatal("outbounds is not an array")
	}
	if len(outbounds) != 2 {
		t.Errorf("outbounds has %d entries, want 2", len(outbounds))
	}
}

func TestBuildGatewayProxyOutbound(t *testing.T) {
	userConfig := json.RawMessage(`{"type":"socks","server":"example.com","server_port":1080,"dns_server":"10.0.0.1","bind_interface":"eth0"}`)
	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// dns_server should be removed
	if _, ok := parsed["dns_server"]; ok {
		t.Error("dns_server should have been removed")
	}
	// bind_interface should be removed
	if _, ok := parsed["bind_interface"]; ok {
		t.Error("bind_interface should have been removed")
	}
	// tag should be set to proxy-out
	if tag, _ := parsed["tag"].(string); tag != "proxy-out" {
		t.Errorf("tag = %q, want %q", tag, "proxy-out")
	}
	// server should be replaced with resolved IP
	if server, _ := parsed["server"].(string); server != "1.2.3.4" {
		t.Errorf("server = %q, want %q", server, "1.2.3.4")
	}
}

func TestBuildGatewayProxyOutbound_RealityWithUTLS(t *testing.T) {
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"reality":{
				"enabled":true,
				"public_key":"test-key"
			}
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check that utls was added to tls config
	tls, ok := parsed["tls"].(map[string]any)
	if !ok {
		t.Fatal("tls config missing")
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatal("utls should have been added for reality")
	}
	if enabled, _ := utls["enabled"].(bool); !enabled {
		t.Error("utls.enabled should be true")
	}
	if fp, _ := utls["fingerprint"].(string); fp != "chrome" {
		t.Errorf("utls.fingerprint = %q, want %q", fp, "chrome")
	}
}

func TestBuildGatewayProxyOutbound_RealityWithExistingUTLS(t *testing.T) {
	// When utls is already configured, it should not be overwritten
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"reality":{
				"enabled":true,
				"public_key":"test-key"
			},
			"utls":{
				"enabled":true,
				"fingerprint":"firefox"
			}
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	tls, _ := parsed["tls"].(map[string]any)
	utls, _ := tls["utls"].(map[string]any)
	if fp, _ := utls["fingerprint"].(string); fp != "firefox" {
		t.Errorf("utls.fingerprint should remain 'firefox', got %q", fp)
	}
}

func TestBuildGatewayProxyOutbound_NoTLS(t *testing.T) {
	// Config without TLS should not cause issues
	userConfig := json.RawMessage(`{"type":"socks","server":"example.com","server_port":1080}`)
	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if tag, _ := parsed["tag"].(string); tag != "proxy-out" {
		t.Errorf("tag = %q, want %q", tag, "proxy-out")
	}
}

func TestBuildGatewayProxyOutbound_ResolvedIPEmpty(t *testing.T) {
	// When resolvedIP is empty, the original server should be kept
	userConfig := json.RawMessage(`{"type":"socks","server":"proxy.example.com","server_port":1080}`)
	out, err := buildGatewayProxyOutbound(userConfig, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// Server should remain unchanged when resolvedIP is empty
	if server, _ := parsed["server"].(string); server != "proxy.example.com" {
		t.Errorf("server = %q, want %q", server, "proxy.example.com")
	}
}

func TestBuildGatewayProxyOutbound_TLSNoReality(t *testing.T) {
	// TLS without reality should not trigger utls addition
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"server_name":"example.com"
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	tls, _ := parsed["tls"].(map[string]any)
	// utls should NOT have been added since reality is not enabled
	if _, ok := tls["utls"]; ok {
		t.Error("utls should not have been added when reality is not present")
	}
}

func TestBuildGatewayProxyOutbound_RealityDisabled(t *testing.T) {
	// TLS with reality present but disabled should not trigger utls addition
	userConfig := json.RawMessage(`{
		"type":"vless",
		"server":"example.com",
		"server_port":443,
		"tls":{
			"enabled":true,
			"reality":{
				"enabled":false,
				"public_key":"test-key"
			}
		}
	}`)

	out, err := buildGatewayProxyOutbound(userConfig, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	tls, _ := parsed["tls"].(map[string]any)
	if _, ok := tls["utls"]; ok {
		t.Error("utls should not have been added when reality is disabled")
	}
}

func TestBuildGatewaySingBoxConfig_ProxyServerIPInRoute(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildGatewaySingBoxConfig(outbound, "10.0.0.1", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The route rules should include a direct bypass for the proxy server IP
	cfgStr := string(cfg)
	if !strings.Contains(cfgStr, "1.2.3.4/32") {
		t.Error("route rules should contain proxy server IP /32 bypass")
	}
}

func TestBuildGatewaySingBoxConfig_InvalidOutbound(t *testing.T) {
	// Invalid JSON in outbound should cause buildGatewayProxyOutbound to fail
	outbound := json.RawMessage(`{invalid`)
	_, err := buildGatewaySingBoxConfig(outbound, "10.0.0.1", "1.2.3.4")
	if err == nil {
		t.Fatal("expected error for invalid outbound JSON")
	}
}

// ---- Phase 45 Plan 01 新增：6 类断言覆盖 BYPASS-NET-01..04 / BYPASS-DNS-01..02 ----

// renderTestConfig 是 Phase 45 新增测试的共享 helper：
// 用固定 vless outbound + dnsServer="" + 给定 proxy IP 渲染 sing-box config，
// 并返回 unmarshal 后的 map 供按下标 / key 断言。
func renderTestConfig(t *testing.T, proxyIP string) map[string]any {
	t.Helper()
	proxyRaw := json.RawMessage(`{"type":"vless","tag":"proxy","server":"placeholder","server_port":443}`)
	raw, err := buildGatewaySingBoxConfig(proxyRaw, "", proxyIP)
	if err != nil {
		t.Fatalf("buildGatewaySingBoxConfig: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return cfg
}

// asMap 安全地把 any 转 map[string]any，失败时 t.Fatalf。
func asMap(t *testing.T, v any, msg string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected map[string]any, got %T", msg, v)
	}
	return m
}

// asStrSlice 把 []any 转 []string；非字符串元素跳过。
func asStrSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// containsStr 检查切片是否含字符串 needle。
func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestBuildGatewaySingBoxConfig_RuleSetReferences(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	route := asMap(t, cfg["route"], "route")
	ruleSet, ok := route["rule_set"].([]any)
	if !ok {
		t.Fatalf("route.rule_set is not an array: %T", route["rule_set"])
	}
	if len(ruleSet) != 2 {
		t.Fatalf("route.rule_set length = %d, want 2", len(ruleSet))
	}

	expected := []struct {
		tag  string
		path string
	}{
		{"whitelist-cidrs", "/etc/sing-box/whitelist-cidrs.json"},
		{"whitelist-domains", "/etc/sing-box/whitelist-domains.json"},
	}
	for i, want := range expected {
		entry := asMap(t, ruleSet[i], "rule_set entry")
		if got, _ := entry["type"].(string); got != "local" {
			t.Errorf("rule_set[%d].type = %q, want %q", i, got, "local")
		}
		if got, _ := entry["tag"].(string); got != want.tag {
			t.Errorf("rule_set[%d].tag = %q, want %q", i, got, want.tag)
		}
		if got, _ := entry["format"].(string); got != "source" {
			t.Errorf("rule_set[%d].format = %q, want %q", i, got, "source")
		}
		if got, _ := entry["path"].(string); got != want.path {
			t.Errorf("rule_set[%d].path = %q, want %q", i, got, want.path)
		}
	}
}

func TestBuildGatewaySingBoxConfig_RouteRulesOrder(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	route := asMap(t, cfg["route"], "route")
	rules, ok := route["rules"].([]any)
	if !ok {
		t.Fatalf("route.rules is not an array: %T", route["rules"])
	}
	if len(rules) != 6 {
		t.Fatalf("route.rules length = %d, want 6", len(rules))
	}

	// [0] action=sniff，sniffer 含 tls/http/quic/dns
	r0 := asMap(t, rules[0], "rules[0]")
	if got, _ := r0["action"].(string); got != "sniff" {
		t.Errorf("rules[0].action = %q, want sniff", got)
	}
	sniffer := asStrSlice(r0["sniffer"])
	for _, p := range []string{"tls", "http", "quic", "dns"} {
		if !containsStr(sniffer, p) {
			t.Errorf("rules[0].sniffer missing %q (got %v)", p, sniffer)
		}
	}

	// [1] protocol=dns，action=hijack-dns
	r1 := asMap(t, rules[1], "rules[1]")
	if got, _ := r1["protocol"].(string); got != "dns" {
		t.Errorf("rules[1].protocol = %q, want dns", got)
	}
	if got, _ := r1["action"].(string); got != "hijack-dns" {
		t.Errorf("rules[1].action = %q, want hijack-dns", got)
	}

	// [2] ip_cidr=[<proxy_ip>/32], outbound=direct
	r2 := asMap(t, rules[2], "rules[2]")
	ipCIDR := asStrSlice(r2["ip_cidr"])
	if !containsStr(ipCIDR, "5.6.7.8/32") {
		t.Errorf("rules[2].ip_cidr missing 5.6.7.8/32 (got %v)", ipCIDR)
	}
	if got, _ := r2["outbound"].(string); got != "direct" {
		t.Errorf("rules[2].outbound = %q, want direct", got)
	}

	// [3] ip_is_private=true, outbound=direct
	r3 := asMap(t, rules[3], "rules[3]")
	if v, _ := r3["ip_is_private"].(bool); !v {
		t.Errorf("rules[3].ip_is_private = %v, want true", r3["ip_is_private"])
	}
	if got, _ := r3["outbound"].(string); got != "direct" {
		t.Errorf("rules[3].outbound = %q, want direct", got)
	}

	// [4] rule_set=[whitelist-cidrs], outbound=direct
	r4 := asMap(t, rules[4], "rules[4]")
	rs4 := asStrSlice(r4["rule_set"])
	if !containsStr(rs4, "whitelist-cidrs") {
		t.Errorf("rules[4].rule_set missing whitelist-cidrs (got %v)", rs4)
	}
	if got, _ := r4["outbound"].(string); got != "direct" {
		t.Errorf("rules[4].outbound = %q, want direct", got)
	}

	// [5] rule_set=[whitelist-domains], outbound=direct
	r5 := asMap(t, rules[5], "rules[5]")
	rs5 := asStrSlice(r5["rule_set"])
	if !containsStr(rs5, "whitelist-domains") {
		t.Errorf("rules[5].rule_set missing whitelist-domains (got %v)", rs5)
	}
	if got, _ := r5["outbound"].(string); got != "direct" {
		t.Errorf("rules[5].outbound = %q, want direct", got)
	}

	// route.final = proxy-out
	if got, _ := route["final"].(string); got != "proxy-out" {
		t.Errorf("route.final = %q, want proxy-out", got)
	}
}

func TestBuildGatewaySingBoxConfig_TunHardening(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	inbounds, ok := cfg["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatalf("inbounds missing or empty: %T", cfg["inbounds"])
	}
	in0 := asMap(t, inbounds[0], "inbounds[0]")

	if got, _ := in0["type"].(string); got != "tun" {
		t.Errorf("inbounds[0].type = %q, want tun", got)
	}
	if v, _ := in0["strict_route"].(bool); !v {
		t.Errorf("inbounds[0].strict_route = %v, want true", in0["strict_route"])
	}
	if v, _ := in0["auto_route"].(bool); !v {
		t.Errorf("inbounds[0].auto_route = %v, want true", in0["auto_route"])
	}
	if v, _ := in0["endpoint_independent_nat"].(bool); !v {
		t.Errorf("inbounds[0].endpoint_independent_nat = %v, want true", in0["endpoint_independent_nat"])
	}
	if got, _ := in0["stack"].(string); got != "system" {
		t.Errorf("inbounds[0].stack = %q, want system", got)
	}

	route := asMap(t, cfg["route"], "route")
	if got, _ := route["default_interface"].(string); got != "eth0" {
		t.Errorf("route.default_interface = %q, want eth0", got)
	}
	if _, ok := route["auto_detect_interface"]; ok {
		t.Errorf("route.auto_detect_interface should be removed; got %v", route["auto_detect_interface"])
	}
}

func TestBuildGatewaySingBoxConfig_SniffAndHijack(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	route := asMap(t, cfg["route"], "route")
	rules, _ := route["rules"].([]any)
	if len(rules) < 2 {
		t.Fatalf("route.rules length = %d, want >= 2", len(rules))
	}

	// 第一条必须是 action=sniff（不是 outbound）
	r0 := asMap(t, rules[0], "rules[0]")
	if got, _ := r0["action"].(string); got != "sniff" {
		t.Errorf("rules[0].action = %q, want sniff", got)
	}
	if _, ok := r0["outbound"]; ok {
		t.Errorf("rules[0] should not have outbound (sniff is a non-routing action)")
	}

	// 第二条必须是 action=hijack-dns（不是旧 outbound=dns）
	r1 := asMap(t, rules[1], "rules[1]")
	if got, _ := r1["action"].(string); got != "hijack-dns" {
		t.Errorf("rules[1].action = %q, want hijack-dns", got)
	}
	if got, _ := r1["outbound"].(string); got == "dns" {
		t.Errorf("rules[1].outbound = %q, must not use old outbound:\"dns\" pattern", got)
	}
}

func TestBuildGatewaySingBoxConfig_SplitDNS(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	dns := asMap(t, cfg["dns"], "dns")

	// dns.servers: 长度 2，含 dns-local 与 dns-proxy
	servers, ok := dns["servers"].([]any)
	if !ok {
		t.Fatalf("dns.servers is not an array: %T", dns["servers"])
	}
	if len(servers) != 2 {
		t.Fatalf("dns.servers length = %d, want 2", len(servers))
	}

	var local, proxy map[string]any
	for _, s := range servers {
		m := asMap(t, s, "dns server")
		switch m["tag"] {
		case "dns-local":
			local = m
		case "dns-proxy":
			proxy = m
		}
	}
	if local == nil {
		t.Fatal("dns.servers missing tag=dns-local")
	}
	if proxy == nil {
		t.Fatal("dns.servers missing tag=dns-proxy")
	}

	if got, _ := local["type"].(string); got != "local" {
		t.Errorf("dns-local.type = %q, want local", got)
	}
	if got, _ := proxy["type"].(string); got != "https" {
		t.Errorf("dns-proxy.type = %q, want https", got)
	}
	if got, _ := proxy["server"].(string); got != "1.1.1.1" {
		t.Errorf("dns-proxy.server = %q, want 1.1.1.1", got)
	}
	if got, _ := proxy["detour"].(string); got != "proxy-out" {
		t.Errorf("dns-proxy.detour = %q, want proxy-out", got)
	}
	if got, _ := proxy["domain_resolver"].(string); got != "dns-local" {
		t.Errorf("dns-proxy.domain_resolver = %q, want dns-local", got)
	}

	// dns.rules: 长度 2
	rules, ok := dns["rules"].([]any)
	if !ok {
		t.Fatalf("dns.rules is not an array: %T", dns["rules"])
	}
	if len(rules) != 2 {
		t.Fatalf("dns.rules length = %d, want 2", len(rules))
	}
	// 第一条：domain_suffix=[.lan,.local,.internal] → dns-local
	r0 := asMap(t, rules[0], "dns.rules[0]")
	suf := asStrSlice(r0["domain_suffix"])
	for _, want := range []string{".lan", ".local", ".internal"} {
		if !containsStr(suf, want) {
			t.Errorf("dns.rules[0].domain_suffix missing %q (got %v)", want, suf)
		}
	}
	if got, _ := r0["server"].(string); got != "dns-local" {
		t.Errorf("dns.rules[0].server = %q, want dns-local", got)
	}
	// 第二条：rule_set=[whitelist-domains] → dns-proxy
	r1 := asMap(t, rules[1], "dns.rules[1]")
	rs := asStrSlice(r1["rule_set"])
	if !containsStr(rs, "whitelist-domains") {
		t.Errorf("dns.rules[1].rule_set missing whitelist-domains (got %v)", rs)
	}
	if got, _ := r1["server"].(string); got != "dns-proxy" {
		t.Errorf("dns.rules[1].server = %q, want dns-proxy", got)
	}

	// dns.final = dns-proxy, strategy = ipv4_only
	if got, _ := dns["final"].(string); got != "dns-proxy" {
		t.Errorf("dns.final = %q, want dns-proxy", got)
	}
	if got, _ := dns["strategy"].(string); got != "ipv4_only" {
		t.Errorf("dns.strategy = %q, want ipv4_only", got)
	}
}

func TestBuildGatewaySingBoxConfig_DirectOutboundBindEth0(t *testing.T) {
	cfg := renderTestConfig(t, "5.6.7.8")
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds is not an array: %T", cfg["outbounds"])
	}
	var direct map[string]any
	for _, o := range outbounds {
		m := asMap(t, o, "outbound")
		if tag, _ := m["tag"].(string); tag == "direct" {
			direct = m
			break
		}
	}
	if direct == nil {
		t.Fatal("outbounds missing tag=direct entry")
	}
	if got, _ := direct["type"].(string); got != "direct" {
		t.Errorf("direct.type = %q, want direct", got)
	}
	if got, _ := direct["bind_interface"].(string); got != "eth0" {
		t.Errorf("direct.bind_interface = %q, want eth0 (Pitfall 1 防回环)", got)
	}
}
