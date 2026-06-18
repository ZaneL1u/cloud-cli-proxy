package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// ---- T3 builder 单测 ----

func TestBuildContainerSingBoxConfig_TunInboundAddress(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if !strings.Contains(s, `"address": [`) || !strings.Contains(s, `"172.19.0.1/30"`) {
		t.Errorf("container tun inbound address mismatch:\n%s", s)
	}
}

func TestBuildContainerSingBoxConfig_RouteFinalProxyOut(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if !strings.Contains(s, `"final": "proxy-out"`) {
		t.Errorf("route.final must be proxy-out:\n%s", s)
	}
}

func TestBuildContainerSingBoxConfig_DirectForProxyServerIP(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if !strings.Contains(s, `"1.2.3.4/32"`) {
		t.Errorf("route rule missing proxy server IP cidr direct rule:\n%s", s)
	}
}

// TestBuildContainerSingBoxConfig_BypassRuleSets 验证 v4.x config 始终
// 包含 bypass-cidrs / bypass-domains 两个 rule_set 引用及对应 route/DNS 规则。
func TestBuildContainerSingBoxConfig_BypassRuleSets(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if !strings.Contains(s, "bypass-cidrs") || !strings.Contains(s, "bypass-domains") {
		t.Errorf("container config must include bypass rule_set references:\n%s", s)
	}
	if !strings.Contains(s, "whitelist-cidrs.json") || !strings.Contains(s, "whitelist-domains.json") {
		t.Errorf("container config must reference whitelist rule-set files:\n%s", s)
	}
}

func TestBuildContainerSingBoxConfig_DNSStubServers(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if !strings.Contains(s, `"tag": "dns-local"`) || !strings.Contains(s, `"tag": "dns-proxy"`) || !strings.Contains(s, `"tag": "dns-direct"`) {
		t.Errorf("dns.servers must include dns-local + dns-proxy + dns-direct:\n%s", s)
	}
}

func TestBuildContainerSingBoxConfig_DNSStubInboundUsesSupportedDirect(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(cfg, &m); err != nil {
		t.Fatal(err)
	}
	inbounds, ok := m["inbounds"].([]any)
	if !ok {
		t.Fatalf("inbounds missing or wrong type: %#v", m["inbounds"])
	}
	for _, inbound := range inbounds {
		in, ok := inbound.(map[string]any)
		if !ok {
			continue
		}
		if in["type"] == "dns" {
			t.Fatalf("container sing-box config must not use unsupported dns inbound: %#v", in)
		}
		if in["listen"] == "127.0.0.1" && in["listen_port"] == float64(53) {
			if in["type"] != "direct" {
				t.Fatalf("127.0.0.1:53 inbound type = %v, want direct", in["type"])
			}
			if in["tag"] != "dns-direct" {
				t.Fatalf("127.0.0.1:53 inbound tag = %v, want dns-direct", in["tag"])
			}
			if in["sniff"] != true {
				t.Fatalf("127.0.0.1:53 direct inbound must enable sniff, got %#v", in["sniff"])
			}
			return
		}
	}
	t.Fatalf("missing DNS inbound listening on 127.0.0.1:53:\n%s", string(cfg))
}

func TestBuildContainerSingBoxConfig_DirectRouteUsesEth0(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(cfg, &m); err != nil {
		t.Fatal(err)
	}
	route, ok := m["route"].(map[string]any)
	if !ok {
		t.Fatalf("route missing or wrong type: %#v", m["route"])
	}
	if got := route["default_interface"]; got != "eth0" {
		t.Fatalf("route.default_interface = %v, want eth0", got)
	}
	outbounds, ok := m["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds missing or wrong type: %#v", m["outbounds"])
	}
	for _, outbound := range outbounds {
		out, ok := outbound.(map[string]any)
		if !ok {
			continue
		}
		if out["tag"] == "direct" {
			if got := out["bind_interface"]; got != "eth0" {
				t.Fatalf("direct outbound bind_interface = %v, want eth0", got)
			}
			return
		}
	}
	t.Fatalf("missing direct outbound:\n%s", string(cfg))
}

func TestBuildContainerSingBoxConfig_DNSHijackScopedToStubAndRejectsOtherDNS(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(cfg, &m); err != nil {
		t.Fatal(err)
	}
	route, ok := m["route"].(map[string]any)
	if !ok {
		t.Fatalf("route missing or wrong type: %#v", m["route"])
	}
	rules, ok := route["rules"].([]any)
	if !ok {
		t.Fatalf("route.rules missing or wrong type: %#v", route["rules"])
	}
	var hijackIndex, rejectIndex = -1, -1
	for i, rule := range rules {
		r, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		if r["protocol"] == "dns" && r["action"] == "hijack-dns" {
			if r["inbound"] != "dns-direct" {
				t.Fatalf("dns hijack rule must be scoped to dns-direct inbound, got %#v", r)
			}
			hijackIndex = i
		}
		if r["protocol"] == "dns" && r["action"] == "reject" {
			if _, scoped := r["inbound"]; scoped {
				t.Fatalf("dns reject rule must cover non-stub DNS traffic, got inbound-scoped rule %#v", r)
			}
			rejectIndex = i
		}
	}
	if hijackIndex == -1 {
		t.Fatalf("missing dns-direct hijack-dns rule:\n%s", string(cfg))
	}
	if rejectIndex == -1 {
		t.Fatalf("missing fallback DNS reject rule:\n%s", string(cfg))
	}
	if rejectIndex <= hijackIndex {
		t.Fatalf("DNS reject rule must follow stub hijack rule, hijack=%d reject=%d", hijackIndex, rejectIndex)
	}
}

// TestBuildContainerSingBoxConfig_NoEndpointIndependentNAT 锁与 v3.5 gateway
// 的差异点：v4.0 单容器架构下不需要 endpoint_independent_nat（流量单向）。
func TestBuildContainerSingBoxConfig_NoEndpointIndependentNAT(t *testing.T) {
	outbound := json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	cfg, err := buildContainerSingBoxConfig(outbound, "1.1.1.1", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(cfg)
	if strings.Contains(s, "endpoint_independent_nat") {
		t.Errorf("container config should NOT carry v3.5 endpoint_independent_nat:\n%s", s)
	}
}

// ---- T4 写盘 + 权限单测 ----

// Test_writeContainerSingBoxConfig_FilePermissionRoot9000Mode0640 锁 D-54-2
// 严格权限契约：只在 Linux + euid=0 下跑（CI runner sudo 模式）；其他环境
// SKIP 以避开 chown EPERM 噪音。
func Test_writeContainerSingBoxConfig_FilePermissionRoot9000Mode0640(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("chown root:9000 requires linux + root; skipping on non-linux")
	}
	if os.Geteuid() != 0 {
		t.Skip("test requires euid=0 to chown root:singbox; skipping under non-root user")
	}
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)

	egress := &EgressConfig{
		EgressIPID: "eip-1",
		ExpectedIP: "9.9.9.9",
		TunnelType: TunnelTypeProxy,
		Proxy: &ProxySpec{
			OutboundConfig: json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`),
			DNSServer:      "1.1.1.1",
		},
	}
	if err := writeContainerSingBoxConfig("h-test", egress); err != nil {
		t.Fatalf("writeContainerSingBoxConfig: %v", err)
	}
	path := filepath.Join(tmp, "gateway", "h-test", "config.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("perm = %o, want 0o640", got)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat sys not *syscall.Stat_t")
	}
	if st.Uid != 0 {
		t.Errorf("uid = %d, want 0 (root)", st.Uid)
	}
	if st.Gid != 9000 {
		t.Errorf("gid = %d, want 9000 (singbox)", st.Gid)
	}
}

func Test_writeContainerSingBoxConfig_NilEgressErrors(t *testing.T) {
	if err := writeContainerSingBoxConfig("h", nil); err == nil {
		t.Errorf("nil egress should return error")
	}
	if err := writeContainerSingBoxConfig("h", &EgressConfig{}); err == nil {
		t.Errorf("nil proxy should return error")
	}
}

// Test_writeContainerSingBoxConfig_WritesValidJSON 在 darwin / 非 root 下仍能
// 验证 config.json 内容合法（chown 失败时文件已经被 os.WriteFile 写入磁盘，
// 顺序：write → chown(失败) → chmod 不再执行；但 file 本身合法可读）。
func Test_writeContainerSingBoxConfig_WritesValidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DATA_DIR", tmp)
	egress := &EgressConfig{
		EgressIPID: "eip-1",
		ExpectedIP: "9.9.9.9",
		TunnelType: TunnelTypeProxy,
		Proxy: &ProxySpec{
			OutboundConfig: json.RawMessage(`{"type":"socks","server":"1.2.3.4","server_port":1080}`),
			DNSServer:      "1.1.1.1",
		},
	}
	// darwin / 非 root 下 chown 会失败，writeContainerSingBoxConfig 会返回 err；
	// 此用例只关注 JSON 内容合法，不关注 perm/owner，故捕获错误并继续读文件。
	_ = writeContainerSingBoxConfig("h", egress)
	path := filepath.Join(tmp, "gateway", "h", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Errorf("written config not valid JSON: %v", err)
	}
	if _, ok := m["inbounds"]; !ok {
		t.Errorf("config missing inbounds key")
	}
}

// TestIsChownPermissionError 锁字符串子串匹配：mkdir / write 的 EPERM 不应被
// 识别为 chown EPERM（避免误降级真错误）。
func TestIsChownPermissionError(t *testing.T) {
	cases := []struct {
		name string
		errS string
		want bool
	}{
		{"chown_eperm", "chown root:singbox: chown /tmp/x: operation not permitted", true},
		{"chown_eacces", "chown root:singbox: chown /tmp/x: permission denied", true},
		{"mkdir_eperm", "mkdir config dir: mkdir /tmp/x: operation not permitted", false},
		{"write_eperm", "write config: open /tmp/x: permission denied", false},
		{"chmod_eperm", "chmod 0640: chmod /tmp/x: operation not permitted", false},
		{"nil", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errS != "" {
				err = &stringErr{s: tt.errS}
			}
			if got := isChownPermissionError(err); got != tt.want {
				t.Errorf("isChownPermissionError(%q) = %v, want %v", tt.errS, got, tt.want)
			}
		})
	}
}

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }
