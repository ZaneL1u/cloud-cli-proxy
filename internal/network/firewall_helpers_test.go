//go:build linux

package network

import (
	"bytes"
	"net"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
)

// 本测试文件覆盖 Phase 47 Plan 02 引入的 5 个 buildXxxExprs 纯函数 + BypassSingboxUID
// 系列常量。设计上把构造 []expr.Any 的逻辑从 addXxxRule wrapper 中独立出来，是为了
// 让单测在任何平台都能跑（不依赖 *nftables.Conn 的真实 netlink socket）。
//
// addXxxRule wrapper 由 conn.AddRule 直接调用 build 函数，无独立逻辑，因此不单独测；
// ConfigureBypassFirewall 的下发顺序由 bypass_firewall_test.go 覆盖。

// findExpr 在 exprs 序列中查找第一个匹配 typeFn 的元素并返回索引；找不到返回 -1。
func findExpr(exprs []expr.Any, typeFn func(expr.Any) bool) int {
	for i, e := range exprs {
		if typeFn(e) {
			return i
		}
	}
	return -1
}

// TestBypassSingboxUID 验证常量值与 gateway 容器对齐。
func TestBypassSingboxUID(t *testing.T) {
	if BypassSingboxUID != 1000 {
		t.Fatalf("BypassSingboxUID = %d, want 1000", BypassSingboxUID)
	}
	if BypassNftSetName != "whitelist_v4" {
		t.Fatalf("BypassNftSetName = %q, want %q", BypassNftSetName, "whitelist_v4")
	}
	if BypassNftLogPrefix != "sbfw-drop " {
		t.Fatalf("BypassNftLogPrefix = %q, want %q", BypassNftLogPrefix, "sbfw-drop ")
	}
}

// TestBuildOifSkuidIPPortAcceptExprs 验证 oifname + skuid + dstIP + dport(tcp) accept 的 expr 序列。
func TestBuildOifSkuidIPPortAcceptExprs(t *testing.T) {
	exprs := buildOifSkuidIPPortAcceptExprs(2, BypassSingboxUID, net.ParseIP("203.0.113.10"), 443, ipprotoTCP)
	if len(exprs) == 0 {
		t.Fatal("expected non-empty exprs")
	}

	// 必须出现 MetaKeyOIF + MetaKeySKUID
	if findExpr(exprs, func(e expr.Any) bool {
		m, ok := e.(*expr.Meta)
		return ok && m.Key == expr.MetaKeyOIF
	}) < 0 {
		t.Fatal("missing MetaKeyOIF")
	}
	if findExpr(exprs, func(e expr.Any) bool {
		m, ok := e.(*expr.Meta)
		return ok && m.Key == expr.MetaKeySKUID
	}) < 0 {
		t.Fatal("missing MetaKeySKUID")
	}

	// uid 字面值通过 Cmp 校验
	foundUID := false
	for _, e := range exprs {
		c, ok := e.(*expr.Cmp)
		if !ok {
			continue
		}
		if bytes.Equal(c.Data, binaryutil.NativeEndian.PutUint32(BypassSingboxUID)) {
			foundUID = true
			break
		}
	}
	if !foundUID {
		t.Fatal("uid cmp data not found")
	}

	// 必须以 Verdict Accept 结尾
	last, ok := exprs[len(exprs)-1].(*expr.Verdict)
	if !ok || last.Kind != expr.VerdictAccept {
		t.Fatalf("last expr verdict = %v, want Accept", last)
	}

	// proxyIP 必须以 4 字节出现
	foundDstIP := false
	for _, e := range exprs {
		c, ok := e.(*expr.Cmp)
		if !ok {
			continue
		}
		if bytes.Equal(c.Data, net.ParseIP("203.0.113.10").To4()) {
			foundDstIP = true
			break
		}
	}
	if !foundDstIP {
		t.Fatal("dst ip 203.0.113.10 cmp data not found")
	}

	// dport 443 必须以 BE uint16 出现
	foundDport := false
	wantDport := binaryutil.BigEndian.PutUint16(443)
	for _, e := range exprs {
		c, ok := e.(*expr.Cmp)
		if !ok {
			continue
		}
		if bytes.Equal(c.Data, wantDport) {
			foundDport = true
			break
		}
	}
	if !foundDport {
		t.Fatal("dport 443 cmp data not found")
	}
}

// TestBuildOifNameAcceptExprs 验证 oifname=sb-tun0 accept 的 expr 序列（16 字节 zero-pad）。
func TestBuildOifNameAcceptExprs(t *testing.T) {
	exprs := buildOifNameAcceptExprs("sb-tun0")

	if findExpr(exprs, func(e expr.Any) bool {
		m, ok := e.(*expr.Meta)
		return ok && m.Key == expr.MetaKeyOIFNAME
	}) < 0 {
		t.Fatal("missing MetaKeyOIFNAME")
	}

	// 接口名 cmp 数据应当为 16 字节，"sb-tun0" + zero-pad
	var want [16]byte
	copy(want[:], "sb-tun0")
	foundName := false
	for _, e := range exprs {
		c, ok := e.(*expr.Cmp)
		if !ok {
			continue
		}
		if bytes.Equal(c.Data, want[:]) {
			foundName = true
			break
		}
	}
	if !foundName {
		t.Fatalf("oifname cmp data not match 16-byte sb-tun0 zero-pad")
	}

	last, ok := exprs[len(exprs)-1].(*expr.Verdict)
	if !ok || last.Kind != expr.VerdictAccept {
		t.Fatal("last expr not Verdict Accept")
	}
}

// TestBuildOifNamedSetMatchAcceptExprs 验证 oif + daddr lookup set 的 expr 序列。
func TestBuildOifNamedSetMatchAcceptExprs(t *testing.T) {
	set := &nftables.Set{Name: BypassNftSetName, ID: 7, KeyType: nftables.TypeIPAddr, Interval: true}
	exprs := buildOifNamedSetMatchAcceptExprs(3, set)

	// 必有 Lookup expr，且 SetName 一致
	idx := findExpr(exprs, func(e expr.Any) bool {
		_, ok := e.(*expr.Lookup)
		return ok
	})
	if idx < 0 {
		t.Fatal("missing Lookup expr")
	}
	lk := exprs[idx].(*expr.Lookup)
	if lk.SetName != BypassNftSetName {
		t.Fatalf("Lookup.SetName = %q, want %q", lk.SetName, BypassNftSetName)
	}
	if lk.SetID != set.ID {
		t.Fatalf("Lookup.SetID = %d, want %d", lk.SetID, set.ID)
	}

	// daddr payload 必须存在（offset=16,len=4）
	foundDaddr := false
	for _, e := range exprs {
		p, ok := e.(*expr.Payload)
		if !ok {
			continue
		}
		if p.Base == expr.PayloadBaseNetworkHeader && p.Offset == 16 && p.Len == 4 {
			foundDaddr = true
			break
		}
	}
	if !foundDaddr {
		t.Fatal("missing payload daddr (offset=16,len=4)")
	}

	last, ok := exprs[len(exprs)-1].(*expr.Verdict)
	if !ok || last.Kind != expr.VerdictAccept {
		t.Fatal("last expr not Verdict Accept")
	}
}

// TestBuildOifUDPDportDropExprs 验证 mDNS / LLMNR / NetBIOS UDP drop。
func TestBuildOifUDPDportDropExprs(t *testing.T) {
	ports := []uint16{5353, 5355, 137, 138}
	for _, port := range ports {
		exprs := buildOifUDPDportDropExprs(2, port)
		if len(exprs) == 0 {
			t.Fatalf("port %d: empty exprs", port)
		}

		// 必有 L4PROTO 比较为 udp（17）
		foundUDP := false
		for _, e := range exprs {
			c, ok := e.(*expr.Cmp)
			if !ok {
				continue
			}
			if bytes.Equal(c.Data, []byte{ipprotoUDP}) {
				foundUDP = true
				break
			}
		}
		if !foundUDP {
			t.Fatalf("port %d: L4PROTO=udp(17) cmp not found", port)
		}

		// 必有目标端口的 BE uint16 比较
		wantPort := binaryutil.BigEndian.PutUint16(port)
		foundPort := false
		for _, e := range exprs {
			c, ok := e.(*expr.Cmp)
			if !ok {
				continue
			}
			if bytes.Equal(c.Data, wantPort) {
				foundPort = true
				break
			}
		}
		if !foundPort {
			t.Fatalf("port %d: dport cmp not found", port)
		}

		// 最后一定是 Drop verdict（不是 Accept）
		last, ok := exprs[len(exprs)-1].(*expr.Verdict)
		if !ok {
			t.Fatalf("port %d: last expr not Verdict, got %T", port, exprs[len(exprs)-1])
		}
		if last.Kind != expr.VerdictDrop {
			t.Fatalf("port %d: last verdict = %v, want Drop", port, last.Kind)
		}
	}
}

// TestBuildLogDropExprs 验证链末 counter + log + drop 兜底规则。
func TestBuildLogDropExprs(t *testing.T) {
	exprs := buildLogDropExprs(BypassNftLogPrefix)

	// 必有 Counter
	if findExpr(exprs, func(e expr.Any) bool {
		_, ok := e.(*expr.Counter)
		return ok
	}) < 0 {
		t.Fatal("missing Counter expr")
	}
	// 必有 Log 且 Data 字面值 == BypassNftLogPrefix
	idx := findExpr(exprs, func(e expr.Any) bool {
		_, ok := e.(*expr.Log)
		return ok
	})
	if idx < 0 {
		t.Fatal("missing Log expr")
	}
	lg := exprs[idx].(*expr.Log)
	if string(lg.Data) != BypassNftLogPrefix {
		t.Fatalf("Log.Data = %q, want %q", string(lg.Data), BypassNftLogPrefix)
	}
	// Drop verdict 末尾
	last, ok := exprs[len(exprs)-1].(*expr.Verdict)
	if !ok || last.Kind != expr.VerdictDrop {
		t.Fatal("last expr not Verdict Drop")
	}
}
