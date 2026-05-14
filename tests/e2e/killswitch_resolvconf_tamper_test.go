//go:build e2e && linux

// killswitch_resolvconf_tamper_test.go 是 Phase 48 Plan 02 / MVS-10 的 e2e 主用例：
//
//   - 入口：基线 worker 容器内 `dig example.com` 必须 Tunneled；
//   - 后台启 host eth0 tcpdump（独立 oracle，BPF：src worker and udp port 53 and dst 8.8.8.8）；
//   - 容器内 `bash -c 'cat > /etc/resolv.conf'` 改写为 `nameserver 8.8.8.8`；
//     成功 = TamperApplied；ro bind mount 抗住 = TamperRejected；都合法。
//   - 容器内立即跑 `dig +short +time=N +tries=1 example.com`，得到 DNSProbeResult；
//   - tcpdump 退出后包数必须 0（不允许任何 UDP/53 → 8.8.8.8 直连流量）；
//   - ClassifyResolvConfDNSOutcome(tamper, dns, packets) 合成裁决，!ok → t.Fatalf。
//
// darwin 上不参与编译；本文件依赖 GoldenPath / TamperResolvConf / ProbeDNSFromUser /
// TcpdumpOnHostEth0 真实拓扑，要求 Linux + docker + Step 2..7 实现 + host eth0
// 抓包能力（CI runner）。

package e2e

import (
	"context"
	"testing"
	"time"
)

// TestKillSwitch_ResolvConfTamper_GoldenPath 验证 MVS-10。
//
// 总 timeout 60s：30s 基线 + 8s tamper+dig+tcpdump + 22s 缓冲。
func TestKillSwitch_ResolvConfTamper_GoldenPath(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host not yet populated (scenario step 7 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baselineDNS, err := g.ProbeDNSFromUser(ctx, "example.com", 5*time.Second)
	if err != nil {
		t.Skipf("baseline dns probe unavailable (worker container handle): %v", err)
		return
	}
	if baselineDNS != DNSResultTunneled {
		t.Skipf("baseline dns not tunneled (got %s); 极可能 CI 出口受限，避免外网抖动 false-fail", baselineDNS)
		return
	}

	workerName, err := g.workerDockerName()
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}

	bpf := "src host " + workerIP +
		" and udp port 53 and dst host " + ResolvConfTamperContract.Nameserver

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, 5*time.Second)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	tamper, err := g.TamperResolvConf(ctx, ResolvConfTamperContract.Nameserver)
	if err != nil {
		t.Fatalf("tamper resolv.conf: %v", err)
	}

	dnsResult, err := g.ProbeDNSFromUser(ctx, "example.com", 5*time.Second)
	if err != nil {
		t.Fatalf("probe dns after tamper: %v", err)
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar reported err (host eth0 在 runner 上可能不可抓包，转 Skip): %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI (hosted ubuntu-24.04 with sudo)")
		return
	}

	ok, reason := ClassifyResolvConfDNSOutcome(tamper, dnsResult, tdRes.packets)
	t.Logf("MVS-10 tamper=%s dns=%s leakedPackets=%d worker=%s bpf=%q reason=%s",
		tamper, dnsResult, tdRes.packets, workerIP, bpf, reason)
	if !ok {
		t.Fatalf("MVS-10 fail: %s (tamper=%s dns=%s packets=%d)",
			reason, tamper, dnsResult, tdRes.packets)
	}
}
