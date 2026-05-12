//go:build linux

package network

import (
	"net"
	"testing"
)

// TestConfigureBypassFirewall_Order 验证 8 条规则按设计顺序进入 plan：
//   1. mDNS drop  (5353)
//   2. LLMNR drop (5355)
//   3. NetBIOS-NS drop (137)
//   4. NetBIOS-DGM drop (138)
//   5. oifname=sb-tun0 accept
//   6. uid+proxyIP+tcp 443 accept
//   7. oifname=eth0 + @whitelist_v4 accept
//   8. 链末 log drop
//
// "先 drop 后 accept" 是 nft 自上而下匹配语义下的关键 —— 任何白名单 accept
// 规则都不能放行 mDNS / LLMNR / NetBIOS（T-47-09）。
func TestConfigureBypassFirewall_Order(t *testing.T) {
	proxyIP := net.ParseIP("198.51.100.20")
	plans := computeBypassRulePlans(proxyIP)
	if len(plans) != 8 {
		t.Fatalf("plans len = %d, want 8 (with proxyIP)", len(plans))
	}

	type expect struct {
		Kind  string
		Dport uint16
	}
	want := []expect{
		{Kind: "udp-drop", Dport: 5353},
		{Kind: "udp-drop", Dport: 5355},
		{Kind: "udp-drop", Dport: 137},
		{Kind: "udp-drop", Dport: 138},
		{Kind: "oifname-accept"},
		{Kind: "uid-port-accept", Dport: 443},
		{Kind: "set-match-accept"},
		{Kind: "log-drop"},
	}
	for i, w := range want {
		got := plans[i]
		if got.Kind != w.Kind {
			t.Errorf("plan[%d].Kind = %s, want %s", i, got.Kind, w.Kind)
		}
		if w.Dport != 0 && got.Dport != w.Dport {
			t.Errorf("plan[%d].Dport = %d, want %d", i, got.Dport, w.Dport)
		}
	}

	// uid-port-accept 必须携带 proxyIP + uid=BypassSingboxUID
	uidPlan := plans[5]
	if !uidPlan.DstIP.Equal(proxyIP) {
		t.Errorf("uid plan DstIP = %v, want %v", uidPlan.DstIP, proxyIP)
	}
	if uidPlan.UID != BypassSingboxUID {
		t.Errorf("uid plan UID = %d, want %d", uidPlan.UID, BypassSingboxUID)
	}

	// oifname-accept 必须是 sb-tun0
	if plans[4].IfName != "sb-tun0" {
		t.Errorf("oifname plan IfName = %q, want %q", plans[4].IfName, "sb-tun0")
	}

	// set-match-accept 必须引用 whitelist_v4
	if plans[6].SetName != BypassNftSetName {
		t.Errorf("set-match plan SetName = %q, want %q", plans[6].SetName, BypassNftSetName)
	}
}

// TestConfigureBypassFirewall_LogPrefix 验证链末 log drop 的 prefix 字面值。
// 必须是 "sbfw-drop "（含尾部空格），与 nft CLI `log prefix "sbfw-drop "` 输出对齐。
func TestConfigureBypassFirewall_LogPrefix(t *testing.T) {
	plans := computeBypassRulePlans(net.ParseIP("198.51.100.20"))
	last := plans[len(plans)-1]
	if last.Kind != "log-drop" {
		t.Fatalf("last plan kind = %s, want log-drop", last.Kind)
	}
	if last.Prefix != "sbfw-drop " {
		t.Fatalf("last plan prefix = %q, want %q", last.Prefix, "sbfw-drop ")
	}
}

// TestConfigureBypassFirewall_ProxyIPNil 验证 proxyIP == nil 时 uid+443 规则被 skip，
// 其它 7 条规则按序保留。兼容 Phase 1+ 旧路径（无代理 IP 场景）。
func TestConfigureBypassFirewall_ProxyIPNil(t *testing.T) {
	plans := computeBypassRulePlans(nil)
	if len(plans) != 7 {
		t.Fatalf("plans len = %d, want 7 (proxyIP=nil)", len(plans))
	}
	for _, p := range plans {
		if p.Kind == "uid-port-accept" {
			t.Errorf("unexpected uid-port-accept plan with proxyIP=nil")
		}
	}
	// 顺序仍是：4 个 udp-drop / oifname-accept / set-match-accept / log-drop
	expectedOrder := []string{
		"udp-drop", "udp-drop", "udp-drop", "udp-drop",
		"oifname-accept", "set-match-accept", "log-drop",
	}
	for i, k := range expectedOrder {
		if plans[i].Kind != k {
			t.Errorf("plan[%d].Kind = %s, want %s", i, plans[i].Kind, k)
		}
	}
}

// TestConfigureBypassFirewall_DNSPortsCovered 显式断言 mDNS / LLMNR / NetBIOS 四个端口
// 都被覆盖到，防御未来重构时意外漏掉某个端口（DNS 旁路风险）。
func TestConfigureBypassFirewall_DNSPortsCovered(t *testing.T) {
	plans := computeBypassRulePlans(nil)
	required := map[uint16]bool{5353: false, 5355: false, 137: false, 138: false}
	for _, p := range plans {
		if p.Kind == "udp-drop" {
			required[p.Dport] = true
		}
	}
	for port, ok := range required {
		if !ok {
			t.Errorf("UDP dport %d not covered by udp-drop plan", port)
		}
	}
}
