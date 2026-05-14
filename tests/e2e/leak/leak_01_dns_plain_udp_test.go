//go:build e2e && linux

// leak_01_dns_plain_udp_test.go 是 Phase 49 LEAK-01 的 e2e 主用例：
//
//   - 容器内 `dig +short +time=3 +tries=1 @8.8.8.8 example.com` 必须**非 0**
//     退出或返回空 stdout（Blocked=true）。
//   - host eth0 抓包 BPF：`udp port 53 and dst host 8.8.8.8 and src host
//     <workerIP>` 必须 0 包。
//   - 任一不满足 → t.Fatalf。
//
// 与 Phase 46 MVS-03 OR 语义（Tunneled / Denied 都接受）显式区分：本 LEAK
// 强约束**纯阻断**，不接受 dig 拿到 A 记录的 Tunneled 分支。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/zanel1u/cloud-cli-proxy/tests/e2e"
)

func TestLeak_01_DNSPlainUDP_BlockedByHostEth0(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureLeakWorkerTools(t, g)
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	workerName, err := workerInspectName(ctx, g)
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}

	bpf := "udp port 53 and dst host 8.8.8.8 and src host " + workerIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, 5*time.Second)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	digRes, err := g.DigPlainDNS(ctx, "8.8.8.8", "example.com")
	if err != nil {
		t.Fatalf("dig plain dns: %v", err)
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar reported err: %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI")
		return
	}

	verdict := e2e.ClassifyLeakProbe(digRes, true)
	t.Logf("LEAK-01 verdict=%s blocked=%v reason=%q exit=%d packets=%d worker=%s bpf=%q",
		verdict, digRes.Blocked, digRes.Reason, digRes.ExitCode, tdRes.packets, workerIP, bpf)

	if tdRes.packets > 0 {
		t.Fatalf("LEAK-01 host eth0 抓到 %d 个 UDP/53→8.8.8.8 包，明文 DNS 直连泄漏 (worker=%s)",
			tdRes.packets, workerIP)
	}
	if !digRes.Blocked {
		t.Fatalf("LEAK-01 dig 仍返回 A 记录（Reason=%q）；本 LEAK 严格不接受 Tunneled 语义",
			digRes.Reason)
	}
}
