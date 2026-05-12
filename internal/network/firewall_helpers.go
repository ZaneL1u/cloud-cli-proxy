//go:build linux

package network

import (
	"net"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const ipprotoTCP = 6

func applyIPv6Rules(conn *nftables.Conn, loIfIndex int) {
	policyDrop := nftables.ChainPolicyDrop

	table6 := conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv6,
		Name:   "filter6",
	})

	output6 := conn.AddChain(&nftables.Chain{
		Name:     "output6",
		Table:    table6,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	addOifAcceptRule(conn, table6, output6, loIfIndex)
}

func addOifAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int) {
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
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func addIifAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int) {
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
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func addOifCtEstablishedRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int) {
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
			&expr.Ct{Register: 1, SourceRegister: false, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func addIifCtEstablishedRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int) {
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
			&expr.Ct{Register: 1, SourceRegister: false, Key: expr.CtKeySTATE},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

func addIifTCPDportAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, port uint16) {
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
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte{ipprotoTCP},
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
				Data:     binaryutil.BigEndian.PutUint16(port),
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})
}

// ----- Phase 47 Plan 02: v3.5 bypass-firewall helpers ----------------------
//
// 这一组 helper 把 4 类新规则的 expr 序列构造抽成纯函数（buildXxxExprs），
// 再由配套的 addXxxRule wrapper 负责 conn.AddRule(...) 真实下发。
//
// 拆分理由：内核侧 *nftables.Conn 走 netlink socket，无法在 darwin 开发机或
// 没有 CAP_NET_ADMIN 的 CI 环境下被 mock；而 helpers 单测仅关心 Exprs 序列
// 的字面值是否符合 nft CLI 语义。把构造逻辑暴露为返回 []expr.Any 的纯函数
// 让单测在任何平台都能跑（go test ./internal/network/ 在 macOS 上也能通过）。

// ifNameBytes 把 nft 接口名 padding 成 16 字节零结尾，符合内核 nft_meta_oifname 约定。
const nftIfNameLen = 16

func ifNameBytes(name string) []byte {
	var buf [nftIfNameLen]byte
	copy(buf[:], name)
	return buf[:]
}

// buildOifSkuidIPPortAcceptExprs 构造：oif==ifIndex && skuid==uid && daddr==dstIP && (proto==proto && dport==dport) -> Accept。
// 用于 worker netns output 链「uid 锁 + 直连代理 IP:443」规则。
func buildOifSkuidIPPortAcceptExprs(ifIndex int, uid uint32, dstIP net.IP, dport uint16, proto byte) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIF, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uint32(ifIndex)),
		},
		&expr.Meta{Key: expr.MetaKeySKUID, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uid),
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
	}
}

func addOifSkuidIPPortAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, uid uint32, dstIP net.IP, dport uint16, proto byte) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: buildOifSkuidIPPortAcceptExprs(ifIndex, uid, dstIP, dport, proto),
	})
}

// buildOifNameAcceptExprs 构造：oifname==ifName -> Accept。
// 用接口名而非 ifindex 是因为 sb-tun0 由 sing-box 在 worker netns 内创建，控制面
// 配置 nft 规则的时刻 tun0 尚未存在，无法预先拿到 ifindex。
func buildOifNameAcceptExprs(ifName string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     ifNameBytes(ifName),
		},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

func addOifNameAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifName string) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: buildOifNameAcceptExprs(ifName),
	})
}

// buildOifNamedSetMatchAcceptExprs 构造：oif==ifIndex && daddr in @set -> Accept。
// set 内容由 Phase 47 Plan 01 的 ApplyBypassRuleSet 动态更新。
func buildOifNamedSetMatchAcceptExprs(ifIndex int, set *nftables.Set) []expr.Any {
	return []expr.Any{
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
		&expr.Lookup{
			SourceRegister: 1,
			SetName:        set.Name,
			SetID:          set.ID,
		},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

func addOifNamedSetMatchAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, set *nftables.Set) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: buildOifNamedSetMatchAcceptExprs(ifIndex, set),
	})
}

// buildOifUDPDportDropExprs 构造：oif==ifIndex && proto==udp && dport==port -> Drop。
// 用于 mDNS (5353) / LLMNR (5355) / NetBIOS-NS (137) / NetBIOS-DGM (138) 显式 drop，
// 防止白名单后续 accept 规则误放行 DNS / 拓扑信号外泄。
func buildOifUDPDportDropExprs(ifIndex int, dport uint16) []expr.Any {
	return []expr.Any{
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
			Data:     []byte{ipprotoUDP},
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
		&expr.Verdict{Kind: expr.VerdictDrop},
	}
}

func addOifUDPDportDropRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, dport uint16) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: buildOifUDPDportDropExprs(ifIndex, dport),
	})
}

// buildLogDropExprs 构造链末兜底规则：counter + log prefix "..." + Drop。
// 链最末位置，不绑定 oifname；任何前面未 accept 的包都会撞到这里。
// 对应 nft CLI：counter log prefix "sbfw-drop " drop
func buildLogDropExprs(prefix string) []expr.Any {
	return []expr.Any{
		&expr.Counter{},
		&expr.Log{
			Key:  1 << unix.NFTA_LOG_PREFIX,
			Data: []byte(prefix),
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	}
}

func addLogDropRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, prefix string) {
	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: buildLogDropExprs(prefix),
	})
}
