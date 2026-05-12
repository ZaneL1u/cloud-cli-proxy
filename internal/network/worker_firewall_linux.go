//go:build linux

package network

import (
	"fmt"
	"net"
	"runtime"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// ApplyWorkerFirewallRules 在 worker 容器 netns 内设置严格的 nftables 默认 DROP 规则。
// 规则设计：
//   - INPUT 默认 DROP：允许 lo、ESTABLISHED/RELATED、来自 gwIP 的流量、SSH(22)
//   - OUTPUT 默认 DROP：允许 lo、ESTABLISHED/RELATED、到 gwIP 的所有流量（代理隧道）、UDP/TCP 53（DNS）
//     + Phase 47 Plan 02：白名单 set / uid 锁 / mDNS/LLMNR/NetBIOS 显式 drop / 链末 log drop
//   - IPv6 全部丢弃（已有 --sysctl net.ipv6.conf.all.disable_ipv6=1，再保险一层）
//
// 关键：规则基于接口索引（eth0 为隔离网络接口，lo 为回环），防止 Docker reconnect bridge 后新接口被滥用。
// 使用 github.com/google/nftables 库，在宿主机上通过 nftables.WithNetNSFd(int(containerNS)) 操作 worker netns。
//
// Phase 47 Plan 02：proxyIP 参数携带 v3 容器代理服务器 IP（对应 EgressConfig.Proxy 的
// 解析结果）。proxyIP == nil 时 ConfigureBypassFirewall 会 skip uid 锁规则，保持与
// Phase 1+ 旧路径（无代理 IP 场景）兼容。
func ApplyWorkerFirewallRules(containerNS netns.NsHandle, gwIP, bridgeGW, proxyIP net.IP, sshPort uint16) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 进入目标 netns 获取接口索引
	originalNS, err := netns.Get()
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("get current netns: %v", err),
		}
	}
	defer originalNS.Close()

	if err := netns.Set(containerNS); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("set container netns: %v", err),
		}
	}
	defer netns.Set(originalNS)

	eth0Link, err := netlink.LinkByName("eth0")
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("get eth0 interface: %v", err),
		}
	}
	eth0IfIndex := eth0Link.Attrs().Index

	loLink, err := netlink.LinkByName("lo")
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("get lo interface: %v", err),
		}
	}
	loIfIndex := loLink.Attrs().Index

	// 切回原始 netns 后操作 nftables
	if err := netns.Set(originalNS); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("restore original netns: %v", err),
		}
	}

	conn, err := nftables.New(nftables.WithNetNSFd(int(containerNS)))
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("open nftables conn: %v", err),
		}
	}

	applyWorkerIPv4Rules(conn, eth0IfIndex, loIfIndex, gwIP, bridgeGW, proxyIP, sshPort)
	applyWorkerIPv6Rules(conn, loIfIndex)

	if err := conn.Flush(); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("apply worker firewall rules: %v", err),
		}
	}

	return nil
}

// CleanupWorkerFirewallRules 清理 worker 容器 netns 内的 nftables cloudproxy 表
func CleanupWorkerFirewallRules(containerNS netns.NsHandle) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	conn, err := nftables.New(nftables.WithNetNSFd(int(containerNS)))
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("open nftables conn for cleanup: %v", err),
		}
	}

	// 删除 IPv4 cloudproxy 表
	tables, err := conn.ListTables()
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("list nftables tables: %v", err),
		}
	}

	for _, t := range tables {
		if t.Name == "cloudproxy" || t.Name == "cloudproxy6" {
			conn.DelTable(t)
		}
	}

	if err := conn.Flush(); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("cleanup worker firewall rules: %v", err),
		}
	}

	return nil
}

func applyWorkerIPv4Rules(conn *nftables.Conn, eth0IfIndex, loIfIndex int, gwIP, bridgeGW, proxyIP net.IP, sshPort uint16) {
	policyDrop := nftables.ChainPolicyDrop

	table := conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   "cloudproxy",
	})

	// INPUT 链：默认 DROP
	inputChain := conn.AddChain(&nftables.Chain{
		Name:     "input",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	// lo 接口允许
	addIifAcceptRule(conn, table, inputChain, loIfIndex)

	// ESTABLISHED/RELATED 允许（eth0）
	addIifCtEstablishedRule(conn, table, inputChain, eth0IfIndex)

	// 来自 gwIP 的流量允许（eth0）
	addIifSrcIPAcceptRule(conn, table, inputChain, eth0IfIndex, gwIP)

	// 来自 bridgeGW 的流量允许（eth0）—— 控制面经 docker network connect 接入时的入站
	addIifSrcIPAcceptRule(conn, table, inputChain, eth0IfIndex, bridgeGW)

	// SSH 端口允许
	addIifTCPDportAcceptRule(conn, table, inputChain, eth0IfIndex, sshPort)

	// OUTPUT 链：默认 DROP
	outputChain := conn.AddChain(&nftables.Chain{
		Name:     "output",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	// lo 接口允许
	addOifAcceptRule(conn, table, outputChain, loIfIndex)

	// ESTABLISHED/RELATED 允许（eth0）
	addOifCtEstablishedRule(conn, table, outputChain, eth0IfIndex)

	// 到 gwIP 的所有流量允许（eth0，代理隧道）
	addOifDstIPAcceptRule(conn, table, outputChain, eth0IfIndex, gwIP)

	// DNS UDP 53 允许
	addOifProtoDstPortAcceptRule(conn, table, outputChain, eth0IfIndex, 53, ipprotoUDP)

	// DNS TCP 53 允许
	addOifProtoDstPortAcceptRule(conn, table, outputChain, eth0IfIndex, 53, ipprotoTCP)

	// Phase 47 Plan 02：v3.5 白名单 set + uid 锁 + mDNS/LLMNR/NetBIOS drop + 链末 log drop。
	// ConfigureBypassFirewall 内部任何 AddRule / AddSet 失败都会在 conn.Flush 阶段集中暴露，
	// 此处忽略返回的 *Set（Phase 47 Plan 01 的 ApplyBypassRuleSet 通过 nft -f 引用 set name 即可）。
	// 错误处理：仅 conn.AddSet 立即返回错误的极端场景；为不破坏 applyWorkerIPv4Rules 的 void 签名，
	// 将其作为告警吸收 —— 真正的下发失败由后续 conn.Flush 报。
	_, _ = ConfigureBypassFirewall(conn, table, outputChain, eth0IfIndex, proxyIP)
}

// applyWorkerIPv6Rules 在 worker netns 内强制 IPv6 全部 drop（仅放行 lo）。
//
// I6 双保险：worker 容器启动参数已 --sysctl net.ipv6.conf.all.disable_ipv6=1
// 与 default.disable_ipv6=1（见 internal/runtime/tasks/worker.go::buildCreateArgs），
// 容器内 IPv6 协议栈不工作；此处 IPv6 表 input6 / output6 policy=drop + 仅放 lo
// 是 nft 层的第二道保险，防止未来某次配置回退（去掉 sysctl）导致 IPv6 流量静默逃逸。
// 两层任意一层失效不会立刻泄漏，必须同时失效才会泄漏 —— 这是 fail-closed 的最强形态。
//
// 同时也是 ip6tables 默认 drop 的等价物：由于 inet 表 family=ipv6 的 nft 规则已经
// 在 worker netns 内强制 drop，无需再额外调用 ip6tables -P FORWARD DROP / OUTPUT DROP。
func applyWorkerIPv6Rules(conn *nftables.Conn, loIfIndex int) {
	policyDrop := nftables.ChainPolicyDrop

	table6 := conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   "cloudproxy6",
	})

	input6 := conn.AddChain(&nftables.Chain{
		Name:     "input6",
		Table:    table6,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	output6 := conn.AddChain(&nftables.Chain{
		Name:     "output6",
		Table:    table6,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	// 只允许 lo
	addIifAcceptRule(conn, table6, input6, loIfIndex)
	addOifAcceptRule(conn, table6, output6, loIfIndex)
}

// addIifSrcIPAcceptRule 匹配进入接口和源 IP，允许通过
func addIifSrcIPAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, srcIP net.IP) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIF, Register: 1},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(uint32(ifIndex)),
			},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       12,
				Len:          4,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     srcIP.To4(),
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

// addOifDstIPAcceptRule 匹配出去接口和目标 IP，允许通过
func addOifDstIPAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, dstIP net.IP) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyOIF, Register: 1},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(uint32(ifIndex)),
			},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       16,
				Len:          4,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dstIP.To4(),
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

// addOifProtoDstPortAcceptRule 匹配出去接口、协议和目标端口，允许通过
func addOifProtoDstPortAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, dport uint16, proto byte) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyOIF, Register: 1},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(uint32(ifIndex)),
			},
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte{proto},
			},
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2,
				Len:          2,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(dport),
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}
