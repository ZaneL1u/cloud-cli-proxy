package http

import (
	"net"
	"testing"
)

func TestValidateBypassRule(t *testing.T) {
	proxyIPs := []string{"203.0.113.10"}

	tests := []struct {
		name                 string
		ruleType             string
		value                string
		port                 string
		confirmRisky         bool
		proxyIPs             []string
		currentHostRuleCount int
		wantCode             string
		wantIsRisky          bool
		wantErr              bool
	}{
		// ip 合法
		{name: "ip valid", ruleType: "ip", value: "8.8.8.8", wantCode: "", wantErr: false},
		// ip 命中 proxy
		{name: "ip conflict proxy", ruleType: "ip", value: "203.0.113.10", proxyIPs: proxyIPs, wantCode: ErrCodeBypassRuleConflictProxy, wantErr: true},
		// cidr 全公网 v4
		{name: "cidr 0.0.0.0/0 too broad", ruleType: "cidr", value: "0.0.0.0/0", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// cidr 全公网 v6
		{name: "cidr ::/0 too broad", ruleType: "cidr", value: "::/0", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// cidr v4 prefix < 16 非私有
		{name: "cidr 1.0.0.0/8 too broad", ruleType: "cidr", value: "1.0.0.0/8", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// cidr v4 prefix < 16 非私有 (8.0.0.0/8)
		{name: "cidr 8.0.0.0/8 too broad", ruleType: "cidr", value: "8.0.0.0/8", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// cidr 私有段放行
		{name: "cidr 10.0.0.0/8 private ok", ruleType: "cidr", value: "10.0.0.0/8", wantCode: "", wantErr: false},
		// cidr 私有段子段放行（172.20.0.0/14 在 172.16.0.0/12 内）
		{name: "cidr 172.20.0.0/14 inside private ok", ruleType: "cidr", value: "172.20.0.0/14", wantCode: "", wantErr: false},
		// cidr 包含 proxy
		{name: "cidr covers proxy", ruleType: "cidr", value: "203.0.113.0/24", proxyIPs: proxyIPs, wantCode: ErrCodeBypassRuleConflictProxy, wantErr: true},
		// cidr 合法（/24）
		{name: "cidr 192.0.2.0/24 ok", ruleType: "cidr", value: "192.0.2.0/24", wantCode: "", wantErr: false},
		// domain 长度 3
		{name: "domain too short", ruleType: "domain", value: "ab", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// domain 命中 TLD
		{name: "domain .com TLD", ruleType: "domain", value: ".com", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// domain 合法
		{name: "domain ok", ruleType: "domain", value: "corp.internal", wantCode: "", wantErr: false},
		// domain_suffix 命中 TLD（去前导点后等同 .cn）
		{name: "domain_suffix .cn TLD", ruleType: "domain_suffix", value: ".cn", wantCode: ErrCodeBypassRuleTooBroad, wantErr: true},
		// domain_suffix 合法
		{name: "domain_suffix corp.internal ok", ruleType: "domain_suffix", value: "corp.internal", wantCode: "", wantErr: false},
		// domain_keyword < 4 无 confirm
		{name: "domain_keyword too short no confirm", ruleType: "domain_keyword", value: "abc", wantCode: ErrCodeBypassKeywordTooShort, wantErr: true},
		// domain_keyword < 4 with confirm
		{name: "domain_keyword too short with confirm", ruleType: "domain_keyword", value: "abc", confirmRisky: true, wantCode: "", wantIsRisky: true, wantErr: false},
		// domain_keyword 合法
		{name: "domain_keyword 4+ ok", ruleType: "domain_keyword", value: "abcd", wantCode: "", wantErr: false},
		// ruleType invalid
		{name: "rule_type invalid", ruleType: "regex", value: "abc", wantErr: true},
		// value empty
		{name: "value empty", ruleType: "ip", value: "  ", wantErr: true},
		// port 合法
		{name: "port 80-443 ok", ruleType: "ip", value: "8.8.8.8", port: "80-443", wantCode: "", wantErr: false},
		// port 非法（超 5 位即 regex 不过）
		{name: "port too long invalid", ruleType: "ip", value: "8.8.8.8", port: "999999", wantErr: true},
		// port 数值越界
		{name: "port over 65535", ruleType: "ip", value: "8.8.8.8", port: "70000", wantErr: true},
		// host rule count 1000 limit
		{name: "limit exceeded", ruleType: "ip", value: "8.8.8.8", currentHostRuleCount: 1000, wantCode: ErrCodeBypassLimitExceeded, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isRisky, code, err := ValidateBypassRule(tt.ruleType, tt.value, tt.port, tt.confirmRisky, tt.proxyIPs, tt.currentHostRuleCount)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v (code=%q)", err, tt.wantErr, code)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if isRisky != tt.wantIsRisky {
				t.Errorf("isRisky = %v, want %v", isRisky, tt.wantIsRisky)
			}
		})
	}
}

func TestContainsCIDR(t *testing.T) {
	// 单独覆盖 helper 的几个分支。
	if !containsCIDR(mustCIDR(t, "10.1.0.0/16"), bypassPrivateCIDRs) {
		t.Error("10.1.0.0/16 should be contained in 10.0.0.0/8")
	}
	if containsCIDR(mustCIDR(t, "8.0.0.0/8"), bypassPrivateCIDRs) {
		t.Error("8.0.0.0/8 should not be contained in any private range")
	}
	if containsCIDR(mustCIDR(t, "127.0.0.1/32"), bypassPrivateCIDRs) == false {
		t.Error("127.0.0.1/32 should be contained in 127.0.0.0/8")
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse cidr %q: %v", s, err)
	}
	return n
}
