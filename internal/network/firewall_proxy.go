//go:build linux

package network

import (
	"fmt"
	"net"
	"runtime"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netns"
)

const ipprotoUDP = 17

// ApplyProxyFirewallRules installs nftables rules inside the container network
// namespace. Uses tun0 and adds a whitelist for the proxy server on mgmt0.
func ApplyProxyFirewallRules(containerNS netns.NsHandle, tunIfIndex, loIfIndex, mgmtIfIndex int, proxyServerIP net.IP, proxyServerPort uint16) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	conn, err := nftables.New(nftables.WithNetNSFd(int(containerNS)))
	if err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("open nftables conn: %v", err),
		}
	}

	applyProxyIPv4Rules(conn, tunIfIndex, loIfIndex, mgmtIfIndex, proxyServerIP, proxyServerPort)
	applyIPv6Rules(conn, loIfIndex)

	if err := conn.Flush(); err != nil {
		return &NetworkError{
			Type:    ErrTunnelSetupFailed,
			Message: fmt.Sprintf("apply proxy firewall rules: %v", err),
		}
	}

	return nil
}

func applyProxyIPv4Rules(conn *nftables.Conn, tunIfIndex, loIfIndex, mgmtIfIndex int, proxyServerIP net.IP, proxyServerPort uint16) {
	policyDrop := nftables.ChainPolicyDrop

	table := conn.AddTable(&nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   "filter",
	})

	outputChain := conn.AddChain(&nftables.Chain{
		Name:     "output",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	addOifAcceptRule(conn, table, outputChain, loIfIndex)
	addOifAcceptRule(conn, table, outputChain, tunIfIndex)
	addOifDstPortAcceptRule(conn, table, outputChain, mgmtIfIndex, proxyServerIP, proxyServerPort, ipprotoTCP)
	addOifDstPortAcceptRule(conn, table, outputChain, mgmtIfIndex, proxyServerIP, proxyServerPort, ipprotoUDP)
	addOifCtEstablishedRule(conn, table, outputChain, mgmtIfIndex)

	inputChain := conn.AddChain(&nftables.Chain{
		Name:     "input",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &policyDrop,
	})

	addIifAcceptRule(conn, table, inputChain, loIfIndex)
	addIifAcceptRule(conn, table, inputChain, tunIfIndex)
	addIifTCPDportAcceptRule(conn, table, inputChain, mgmtIfIndex, 22)
	addIifCtEstablishedRule(conn, table, inputChain, mgmtIfIndex)
}

func addOifDstPortAcceptRule(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ifIndex int, dstIP net.IP, dport uint16, proto byte) {
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
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       16,
				Len:          4,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dstIP.To4(),
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
