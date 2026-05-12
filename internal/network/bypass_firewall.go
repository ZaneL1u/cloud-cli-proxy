//go:build linux

package network

import (
	"fmt"
	"net"

	"github.com/google/nftables"
)

// bypassRulePlan 描述 ConfigureBypassFirewall 将下发的一条规则。
// 拆出 "plan" 这一层是为了让单测可以在不持有真实 netlink socket 的情况下
// 断言下发顺序、规则种类与关键字面值（dport / proxyIP / log prefix）。
type bypassRulePlan struct {
	Kind    string // "udp-drop" / "oifname-accept" / "uid-port-accept" / "set-match-accept" / "log-drop"
	Dport   uint16 // 仅 udp-drop / uid-port-accept 有效
	IfName  string // 仅 oifname-accept 有效（sb-tun0）
	DstIP   net.IP // 仅 uid-port-accept 有效（proxyIP）
	UID     uint32 // 仅 uid-port-accept 有效
	Prefix  string // 仅 log-drop 有效
	SetName string // 仅 set-match-accept 有效
}

// computeBypassRulePlans 计算 ConfigureBypassFirewall 将依次下发的规则计划。
// 顺序约束（nft 链自上而下匹配）：
//
//	1-4. mDNS / LLMNR / NetBIOS UDP drop —— 必须出现在任何 accept 规则之前，
//	     防止白名单 accept 误放行 DNS / 拓扑信号外泄（防御 T-47-09）。
//	5.   oifname=sb-tun0 accept —— 给 sing-box tun 出向开口。
//	6.   uid==1000 && daddr==proxyIP && tcp/443 accept —— 直连代理（仅当 proxyIP!=nil）。
//	     uid 锁防止 worker 容器内 uid≠1000 的进程伪装 sing-box 直连（防御 T-47-06）。
//	7.   oifname=eth0 && daddr in @whitelist_v4 accept —— 命名 set 白名单逃逸。
//	8.   链末 log drop 兜底，syslog 计数器记录所有未匹配 accept 的包。
//
// proxyIP == nil 时跳过第 6 条，保持 Phase 1+ 旧路径（无代理 IP 场景）兼容。
func computeBypassRulePlans(proxyIP net.IP) []bypassRulePlan {
	plans := []bypassRulePlan{
		{Kind: "udp-drop", Dport: 5353},
		{Kind: "udp-drop", Dport: 5355},
		{Kind: "udp-drop", Dport: 137},
		{Kind: "udp-drop", Dport: 138},
		{Kind: "oifname-accept", IfName: "sb-tun0"},
	}
	if proxyIP != nil {
		plans = append(plans, bypassRulePlan{
			Kind:  "uid-port-accept",
			UID:   BypassSingboxUID,
			DstIP: proxyIP,
			Dport: 443,
		})
	}
	plans = append(plans, bypassRulePlan{Kind: "set-match-accept", SetName: BypassNftSetName})
	plans = append(plans, bypassRulePlan{Kind: "log-drop", Prefix: BypassNftLogPrefix})
	return plans
}

// ConfigureBypassFirewall 在已存在的 output 链上追加 v3.5 白名单 / uid 锁 /
// DNS 旁路阻断 / 链末 log drop 一整套规则，并在 table 内创建空 set `whitelist_v4`
// （type=ipv4_addr / flags=interval / auto-merge）。set 内容由 Phase 47 Plan 01
// 的 ApplyBypassRuleSet 通过 `nft -f - flush set ... add element ...` 动态填充。
//
// 调用约束：必须在 applyWorkerIPv4Rules 完成基础规则（lo / ct established / gwIP /
// SSH / DNS 53）之后调用。本函数下发的规则在 nft 自上而下匹配下，先 drop 后 accept
// 的顺序保证 mDNS / LLMNR / NetBIOS 不会被任何后续 accept 规则误放行。
//
// 错误处理：set 创建失败立即返回错误；AddRule 内部失败由 conn.Flush 阶段统一暴露
// （沿用本包既有模式，参见 applyWorkerIPv4Rules）。
func ConfigureBypassFirewall(conn *nftables.Conn, table *nftables.Table, output *nftables.Chain, eth0IfIndex int, proxyIP net.IP) (*nftables.Set, error) {
	set := &nftables.Set{
		Table:    table,
		Name:     BypassNftSetName,
		KeyType:  nftables.TypeIPAddr,
		Interval: true,
	}
	if err := conn.AddSet(set, nil); err != nil {
		return nil, fmt.Errorf("add nft set %s: %w", BypassNftSetName, err)
	}

	plans := computeBypassRulePlans(proxyIP)
	for _, p := range plans {
		switch p.Kind {
		case "udp-drop":
			addOifUDPDportDropRule(conn, table, output, eth0IfIndex, p.Dport)
		case "oifname-accept":
			addOifNameAcceptRule(conn, table, output, p.IfName)
		case "uid-port-accept":
			addOifSkuidIPPortAcceptRule(conn, table, output, eth0IfIndex, p.UID, p.DstIP, p.Dport, ipprotoTCP)
		case "set-match-accept":
			addOifNamedSetMatchAcceptRule(conn, table, output, eth0IfIndex, set)
		case "log-drop":
			addLogDropRule(conn, table, output, p.Prefix)
		default:
			return nil, fmt.Errorf("unknown bypass rule plan kind: %s", p.Kind)
		}
	}
	return set, nil
}
