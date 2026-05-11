//go:build linux

package network

import (
	"net"
	"runtime"
	"testing"

	"github.com/google/nftables"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// newTestNetNS 创建一个临时 netns，失败时调用 t.Skip。
// 返回 netns handle、原始 netns handle 和用于清理的名称。
func newTestNetNS(t *testing.T) (netns.NsHandle, netns.NsHandle) {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	// 保存原始命名空间
	origNS, err := netns.Get()
	if err != nil {
		t.Skipf("get original netns: %v", err)
	}

	// 使用当前线程创建新的命名空间
	ns, err := netns.New()
	if err != nil {
		origNS.Close()
		t.Skipf("requires CAP_SYS_ADMIN: %v", err)
	}

	// 在临时 netns 中创建 dummy eth0 接口
	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
	if err := netlink.LinkAdd(dummy); err != nil {
		netns.Set(origNS)
		ns.Close()
		origNS.Close()
		t.Skipf("requires CAP_NET_ADMIN to create dummy interface: %v", err)
	}
	if err := netlink.LinkSetUp(dummy); err != nil {
		netns.Set(origNS)
		ns.Close()
		origNS.Close()
		t.Skipf("failed to bring up dummy eth0: %v", err)
	}

	// lo 接口在新 netns 中默认已存在，但需要确保它 up
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		netns.Set(origNS)
		ns.Close()
		origNS.Close()
		t.Skipf("failed to get lo interface: %v", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		netns.Set(origNS)
		ns.Close()
		origNS.Close()
		t.Skipf("failed to bring up lo: %v", err)
	}

	// 恢复原始命名空间，避免当前线程停留在临时 ns 中
	if err := netns.Set(origNS); err != nil {
		ns.Close()
		origNS.Close()
		t.Skipf("restore original netns: %v", err)
	}

	return ns, origNS
}

// cleanupTestNetNS 清理临时 netns，恢复原始命名空间。
func cleanupTestNetNS(t *testing.T, ns, origNS netns.NsHandle) {
	t.Helper()
	// 确保线程回到原始命名空间
	_ = netns.Set(origNS)
	ns.Close()
	origNS.Close()
}

// getTableByName 从 nftables 连接中查找指定名称的表。
func getTableByName(t *testing.T, conn *nftables.Conn, name string) *nftables.Table {
	t.Helper()

	tables, err := conn.ListTables()
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for _, tbl := range tables {
		if tbl.Name == name {
			return tbl
		}
	}
	return nil
}

// getChainByName 从 nftables 连接中查找指定表和名称的链。
func getChainByName(t *testing.T, conn *nftables.Conn, table *nftables.Table, name string) *nftables.Chain {
	t.Helper()

	chains, err := conn.ListChainsOfTableFamily(table.Family)
	if err != nil {
		t.Fatalf("list chains: %v", err)
	}
	for _, ch := range chains {
		if ch.Table.Name == table.Name && ch.Name == name {
			return ch
		}
	}
	return nil
}

// TestApplyWorkerFirewallRules_Basic 验证基本防火墙规则创建。
func TestApplyWorkerFirewallRules_Basic(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("ApplyWorkerFirewallRules failed: %v", err)
	}

	// 验证 cloudproxy 表存在
	conn, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn: %v", err)
	}

	tbl := getTableByName(t, conn, "cloudproxy")
	if tbl == nil {
		t.Fatal("cloudproxy table not found")
	}
	if tbl.Family != nftables.TableFamilyIPv4 {
		t.Fatalf("cloudproxy family = %v, want IPv4", tbl.Family)
	}

	// 验证 input 链存在且默认策略为 DROP
	inputChain := getChainByName(t, conn, tbl, "input")
	if inputChain == nil {
		t.Fatal("input chain not found")
	}
	if inputChain.Policy == nil || *inputChain.Policy != nftables.ChainPolicyDrop {
		t.Fatal("input chain policy is not DROP")
	}

	// 验证 output 链存在且默认策略为 DROP
	outputChain := getChainByName(t, conn, tbl, "output")
	if outputChain == nil {
		t.Fatal("output chain not found")
	}
	if outputChain.Policy == nil || *outputChain.Policy != nftables.ChainPolicyDrop {
		t.Fatal("output chain policy is not DROP")
	}

	// 验证规则数量：input 链应该有 lo + ct_established + gwIP + bridgeGW + SSH = 5 条
	inputRules, err := conn.GetRules(tbl, inputChain)
	if err != nil {
		t.Fatalf("get input rules: %v", err)
	}
	if len(inputRules) < 4 {
		t.Fatalf("input rules = %d, want >= 4", len(inputRules))
	}

	// output 链应该有 lo + ct_established + gwIP + DNS UDP + DNS TCP = 5 条
	outputRules, err := conn.GetRules(tbl, outputChain)
	if err != nil {
		t.Fatalf("get output rules: %v", err)
	}
	if len(outputRules) < 4 {
		t.Fatalf("output rules = %d, want >= 4", len(outputRules))
	}

	// 验证 IPv6 表也存在
	tbl6 := getTableByName(t, conn, "cloudproxy6")
	if tbl6 == nil {
		t.Fatal("cloudproxy6 table not found")
	}
	if tbl6.Family != nftables.TableFamilyIPv6 {
		t.Fatalf("cloudproxy6 family = %v, want IPv6", tbl6.Family)
	}
}


// TestApplyWorkerFirewallRules_InvalidNetNS 验证无效 netns handle 返回错误。
func TestApplyWorkerFirewallRules_InvalidNetNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	// 传入无效的 netns handle
	invalidNS := netns.NsHandle(0)

	err := ApplyWorkerFirewallRules(invalidNS, gwIP, bridgeGW, 22, nil)
	if err == nil {
		t.Fatal("expected error for invalid netns, got nil")
	}

	// 验证返回的是 NetworkError
	netErr, ok := err.(*NetworkError)
	if !ok {
		t.Fatalf("expected *NetworkError, got %T", err)
	}
	if netErr.Type != ErrTunnelSetupFailed {
		t.Fatalf("error type = %v, want %v", netErr.Type, ErrTunnelSetupFailed)
	}
}

// TestCleanupWorkerFirewallRules 验证清理功能正确删除表。
func TestCleanupWorkerFirewallRules(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	// 先应用规则
	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("ApplyWorkerFirewallRules failed: %v", err)
	}

	// 验证表存在
	conn, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn: %v", err)
	}
	if getTableByName(t, conn, "cloudproxy") == nil {
		t.Fatal("cloudproxy table should exist before cleanup")
	}
	if getTableByName(t, conn, "cloudproxy6") == nil {
		t.Fatal("cloudproxy6 table should exist before cleanup")
	}

	// 清理
	err = CleanupWorkerFirewallRules(ns)
	if err != nil {
		t.Fatalf("CleanupWorkerFirewallRules failed: %v", err)
	}

	// 验证表已删除
	conn2, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn after cleanup: %v", err)
	}

	if getTableByName(t, conn2, "cloudproxy") != nil {
		t.Fatal("cloudproxy table should be deleted after cleanup")
	}
	if getTableByName(t, conn2, "cloudproxy6") != nil {
		t.Fatal("cloudproxy6 table should be deleted after cleanup")
	}
}

// TestCleanupWorkerFirewallRules_Idempotent 验证多次清理不会报错。
func TestCleanupWorkerFirewallRules_Idempotent(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	// 先应用规则
	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("ApplyWorkerFirewallRules failed: %v", err)
	}

	// 第一次清理
	err = CleanupWorkerFirewallRules(ns)
	if err != nil {
		t.Fatalf("first cleanup failed: %v", err)
	}

	// 第二次清理（表已不存在）
	err = CleanupWorkerFirewallRules(ns)
	if err != nil {
		t.Fatalf("second cleanup should be idempotent, got error: %v", err)
	}

	// 第三次清理
	err = CleanupWorkerFirewallRules(ns)
	if err != nil {
		t.Fatalf("third cleanup should be idempotent, got error: %v", err)
	}
}

// TestApplyWorkerFirewallRules_IPv6Rules 验证 IPv6 规则创建。
func TestApplyWorkerFirewallRules_IPv6Rules(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("ApplyWorkerFirewallRules failed: %v", err)
	}

	conn, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn: %v", err)
	}

	// 验证 IPv6 表
	tbl6 := getTableByName(t, conn, "cloudproxy6")
	if tbl6 == nil {
		t.Fatal("cloudproxy6 table not found")
	}
	if tbl6.Family != nftables.TableFamilyIPv6 {
		t.Fatalf("cloudproxy6 family = %v, want IPv6", tbl6.Family)
	}

	// 验证 input6 链默认 DROP
	input6 := getChainByName(t, conn, tbl6, "input6")
	if input6 == nil {
		t.Fatal("input6 chain not found")
	}
	if input6.Policy == nil || *input6.Policy != nftables.ChainPolicyDrop {
		t.Fatal("input6 chain policy is not DROP")
	}

	// 验证 output6 链默认 DROP
	output6 := getChainByName(t, conn, tbl6, "output6")
	if output6 == nil {
		t.Fatal("output6 chain not found")
	}
	if output6.Policy == nil || *output6.Policy != nftables.ChainPolicyDrop {
		t.Fatal("output6 chain policy is not DROP")
	}

	// 验证 input6 只有 lo 允许规则（1 条）
	input6Rules, err := conn.GetRules(tbl6, input6)
	if err != nil {
		t.Fatalf("get input6 rules: %v", err)
	}
	if len(input6Rules) != 1 {
		t.Fatalf("input6 rules = %d, want 1", len(input6Rules))
	}

	// 验证 output6 只有 lo 允许规则（1 条）
	output6Rules, err := conn.GetRules(tbl6, output6)
	if err != nil {
		t.Fatalf("get output6 rules: %v", err)
	}
	if len(output6Rules) != 1 {
		t.Fatalf("output6 rules = %d, want 1", len(output6Rules))
	}
}

// TestApplyWorkerFirewallRules_CustomSSHPort 验证自定义 SSH 端口。
func TestApplyWorkerFirewallRules_CustomSSHPort(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")
	customSSHPort := uint16(2222)

	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, customSSHPort, nil)
	if err != nil {
		t.Fatalf("ApplyWorkerFirewallRules failed: %v", err)
	}

	conn, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn: %v", err)
	}

	tbl := getTableByName(t, conn, "cloudproxy")
	if tbl == nil {
		t.Fatal("cloudproxy table not found")
	}

	inputChain := getChainByName(t, conn, tbl, "input")
	if inputChain == nil {
		t.Fatal("input chain not found")
	}

	inputRules, err := conn.GetRules(tbl, inputChain)
	if err != nil {
		t.Fatalf("get input rules: %v", err)
	}

	// lo + ct_established + gwIP + bridgeGW + SSH = 5 条
	if len(inputRules) != 5 {
		t.Fatalf("input rules = %d, want 5", len(inputRules))
	}
}



// TestApplyThenCleanupThenApply 验证清理后可以重新应用规则。
func TestApplyThenCleanupThenApply(t *testing.T) {
	ns, origNS := newTestNetNS(t)
	defer cleanupTestNetNS(t, ns, origNS)

	gwIP := net.ParseIP("10.0.0.1")
	bridgeGW := net.ParseIP("172.18.0.1")

	// 第一次应用
	err := ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("first ApplyWorkerFirewallRules failed: %v", err)
	}

	// 清理
	err = CleanupWorkerFirewallRules(ns)
	if err != nil {
		t.Fatalf("CleanupWorkerFirewallRules failed: %v", err)
	}

	// 第二次应用
	err = ApplyWorkerFirewallRules(ns, gwIP, bridgeGW, 22)
	if err != nil {
		t.Fatalf("second ApplyWorkerFirewallRules failed: %v", err)
	}

	// 验证规则存在
	conn, err := nftables.New(nftables.WithNetNSFd(int(ns)))
	if err != nil {
		t.Fatalf("open nftables conn: %v", err)
	}

	tbl := getTableByName(t, conn, "cloudproxy")
	if tbl == nil {
		t.Fatal("cloudproxy table not found after re-apply")
	}

	inputChain := getChainByName(t, conn, tbl, "input")
	if inputChain == nil {
		t.Fatal("input chain not found")
	}

	inputRules, err := conn.GetRules(tbl, inputChain)
	if err != nil {
		t.Fatalf("get input rules: %v", err)
	}

	// 基础 5 条
	if len(inputRules) != 5 {
		t.Fatalf("input rules = %d, want 5", len(inputRules))
	}
}
