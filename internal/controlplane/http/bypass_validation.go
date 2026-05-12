package http

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Bypass / Whitelist 相关错误码常量。
// HTTP body 统一形如 {"code": "BYPASS_*", "message": "..."}。
const (
	ErrCodeBypassRuleTooBroad      = "BYPASS_RULE_TOO_BROAD"
	ErrCodeBypassRuleConflictProxy = "BYPASS_RULE_CONFLICT_PROXY"
	ErrCodeBypassLimitExceeded     = "BYPASS_LIMIT_EXCEEDED"
	ErrCodeBypassKeywordTooShort   = "BYPASS_KEYWORD_TOO_SHORT"
	ErrCodeBypassPresetImmutable   = "BYPASS_PRESET_IMMUTABLE"
	ErrCodeBypassSnapshotConflict  = "BYPASS_SNAPSHOT_CONFLICT"
	ErrCodeBypassSnapshotNotFound  = "BYPASS_SNAPSHOT_NOT_FOUND"
	ErrCodeBypassHostNotFound      = "BYPASS_HOST_NOT_FOUND"
	ErrCodeBypassPresetNotFound    = "BYPASS_PRESET_NOT_FOUND"
	ErrCodeBypassRuleNotFound      = "BYPASS_RULE_NOT_FOUND"
	ErrCodeBypassBindingNotFound   = "BYPASS_BINDING_NOT_FOUND"
	ErrCodeBypassInvalidRequest    = "BYPASS_INVALID_REQUEST"
)

// 单 host 自定义规则数上限（命中即 BYPASS_LIMIT_EXCEEDED）。
const bypassRulesPerHostLimit = 1000

// 允许的 rule_type 集合（5 种）。
var bypassAllowedRuleTypes = map[string]bool{
	"ip":             true,
	"cidr":           true,
	"domain":         true,
	"domain_suffix":  true,
	"domain_keyword": true,
}

// bypassTLDBlacklist：硬拦截 TLD 黑名单。
// 来源：常见公开顶级域 + 国家代码顶级域；命中 domain / domain_suffix 即视为「全公网绕过」。
var bypassTLDBlacklist = []string{
	".com", ".net", ".org", ".io", ".cn", ".jp", ".uk", ".de",
	".fr", ".ru", ".info", ".biz", ".co", ".me", ".app", ".dev",
	".ai", ".xyz",
}

// bypassPrivateCIDRs：私有段白名单。落在这些 CIDR 内部的大段 CIDR 不视为「全公网绕过」。
// 覆盖 RFC1918、CGNAT、loopback、link-local、multicast。
var bypassPrivateCIDRs []*net.IPNet

func init() {
	privates := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
	}
	bypassPrivateCIDRs = make([]*net.IPNet, 0, len(privates))
	for _, s := range privates {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			panic(fmt.Sprintf("init bypassPrivateCIDRs: parse %s: %v", s, err))
		}
		bypassPrivateCIDRs = append(bypassPrivateCIDRs, n)
	}
}

var portRangeRegexp = regexp.MustCompile(`^(\d{1,5})(-(\d{1,5}))?$`)

// containsCIDR 判断 needle 是否完全被 hay 中任一项覆盖。
// 完全覆盖定义：hay 的 mask 比 needle 短或相等，且 hay.Contains(needle.IP)。
func containsCIDR(needle *net.IPNet, hay []*net.IPNet) bool {
	if needle == nil {
		return false
	}
	needleOnes, _ := needle.Mask.Size()
	for _, h := range hay {
		if h.IP.To4() == nil && needle.IP.To4() != nil {
			continue
		}
		if h.IP.To4() != nil && needle.IP.To4() == nil {
			continue
		}
		hOnes, _ := h.Mask.Size()
		if hOnes > needleOnes {
			continue
		}
		if h.Contains(needle.IP) {
			return true
		}
	}
	return false
}

// ValidateBypassRule 对 Bypass 规则做 5 硬 1 软护栏校验。
//
// 返回值：
//   - isRisky：仅在 domain_keyword 软拦截通过（< 4 字符 + confirm_risky=true）路径为 true；其它正常路径恒为 false。
//   - code：BYPASS_* 错误码；若空字符串则表示参数级 400 错误（由 caller 自行包装），非空表示业务护栏命中。
//   - err：人类可读消息；nil 表示合法。
//
// caller 约定：
//   - port 空字符串表示「不限制端口」，否则形如 `80` 或 `80-443`；
//   - proxyIPs 是当前所有 EgressIP 的 IP 列表（用于 CONFLICT_PROXY 检查）；
//   - currentHostRuleCount 仅在 caller 创建 host scope 规则时传入；新增前 ≥ 1000 命中 LIMIT_EXCEEDED。
func ValidateBypassRule(ruleType, value, port string, confirmRisky bool, proxyIPs []string, currentHostRuleCount int) (isRisky bool, code string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, "", errors.New("value is required")
	}
	if !bypassAllowedRuleTypes[ruleType] {
		return false, "", fmt.Errorf("unsupported rule_type %q, allowed: ip, cidr, domain, domain_suffix, domain_keyword", ruleType)
	}

	// host 规则数硬上限（新增前判断）。
	if currentHostRuleCount >= bypassRulesPerHostLimit {
		return false, ErrCodeBypassLimitExceeded, fmt.Errorf("rules per host exceeds limit %d", bypassRulesPerHostLimit)
	}

	if err := validateBypassPort(port); err != nil {
		return false, "", err
	}

	switch ruleType {
	case "ip":
		ip := net.ParseIP(value)
		if ip == nil {
			return false, "", fmt.Errorf("invalid ip address %q", value)
		}
		for _, p := range proxyIPs {
			if strings.TrimSpace(p) == value {
				return false, ErrCodeBypassRuleConflictProxy, fmt.Errorf("rule ip %q conflicts with proxy egress ip", value)
			}
		}
		return false, "", nil

	case "cidr":
		_, ipnet, perr := net.ParseCIDR(value)
		if perr != nil {
			return false, "", fmt.Errorf("invalid cidr %q: %v", value, perr)
		}
		if ipnet.String() == "0.0.0.0/0" || ipnet.String() == "::/0" {
			return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("cidr %q is the entire internet", value)
		}
		// v4 prefix < /16 且不在私有段白名单内 → 视为大段公网。
		if ipnet.IP.To4() != nil {
			ones, _ := ipnet.Mask.Size()
			if ones < 16 && !containsCIDR(ipnet, bypassPrivateCIDRs) {
				return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("cidr %q is too broad (v4 prefix < /16 and not in private space)", value)
			}
		}
		// CIDR 覆盖任一 proxy IP → conflict。
		for _, p := range proxyIPs {
			pip := net.ParseIP(strings.TrimSpace(p))
			if pip != nil && ipnet.Contains(pip) {
				return false, ErrCodeBypassRuleConflictProxy, fmt.Errorf("cidr %q covers proxy egress ip %q", value, p)
			}
		}
		return false, "", nil

	case "domain":
		if len(value) < 4 {
			return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("domain %q is too short", value)
		}
		lower := strings.ToLower(value)
		for _, tld := range bypassTLDBlacklist {
			if lower == tld {
				return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("domain %q is a TLD and forbidden", value)
			}
		}
		return false, "", nil

	case "domain_suffix":
		// 去掉前导点，便于 TLD 比较。
		normalized := strings.TrimPrefix(value, ".")
		if len(normalized) < 4 {
			return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("domain_suffix %q is too short", value)
		}
		lower := "." + strings.ToLower(normalized)
		for _, tld := range bypassTLDBlacklist {
			if lower == tld {
				return false, ErrCodeBypassRuleTooBroad, fmt.Errorf("domain_suffix %q hits TLD blacklist", value)
			}
		}
		return false, "", nil

	case "domain_keyword":
		if len(value) < 4 {
			if !confirmRisky {
				return false, ErrCodeBypassKeywordTooShort, fmt.Errorf("domain_keyword %q is shorter than 4 chars; pass confirm_risky=true to accept the risk", value)
			}
			// 软拦截通过：标记 isRisky=true 让 caller 落库时持久化风险位。
			return true, "", nil
		}
		return false, "", nil
	}

	return false, "", fmt.Errorf("unreachable: ruleType=%s", ruleType)
}

// validateBypassPort 校验端口或端口范围；空字符串放行。
func validateBypassPort(port string) error {
	port = strings.TrimSpace(port)
	if port == "" {
		return nil
	}
	m := portRangeRegexp.FindStringSubmatch(port)
	if m == nil {
		return fmt.Errorf("invalid port %q, expected `port` or `start-end` with 1-5 digits each", port)
	}
	start, err := strconv.Atoi(m[1])
	if err != nil || start < 1 || start > 65535 {
		return fmt.Errorf("invalid port %q, start out of range 1-65535", port)
	}
	if m[3] == "" {
		return nil
	}
	end, err := strconv.Atoi(m[3])
	if err != nil || end < 1 || end > 65535 {
		return fmt.Errorf("invalid port %q, end out of range 1-65535", port)
	}
	if end < start {
		return fmt.Errorf("invalid port %q, end < start", port)
	}
	return nil
}
