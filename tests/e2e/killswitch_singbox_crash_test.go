//go:build e2e && linux

// killswitch_singbox_crash_test.go 是 Phase 48 Plan 01 / MVS-09 的 e2e 主用例：
//
//   - 入口：基线 worker 容器内 `curl https://ifconfig.io` 必须 exit 0；
//   - 后台启 host eth0 tcpdump（独立 oracle，BPF：src worker and not dst gateway）；
//   - `docker kill --signal=KILL <gateway>`；
//   - 容器内立即跑 `curl --max-time 3 <url>`，期望非 0 退出（kill-switch 兜住）；
//   - tcpdump 退出后包数必须 0；
//   - ClassifyKillswitchResult 合成裁决，非 OK → t.Fatalf。
//
// darwin 上不参与编译；本文件依赖 GoldenPath / KillGateway / TcpdumpOnHostEth0
// 等真实拓扑，要求 Linux + docker + Step 2..7 实现 + host eth0 抓包能力（CI runner）。

package e2e

import (
	"context"
	"testing"
	"time"
)

// TestKillSwitch_SingboxCrash_GoldenPath 验证 MVS-09。
//
// 总 timeout 60s：30s 基线 + 8s kill+probe+tcpdump + 22s 缓冲。
func TestKillSwitch_SingboxCrash_GoldenPath(t *testing.T) {
	g := StartGoldenPath(t)
	if g == nil {
		return
	}
	if g.Host == nil || g.Host.ID == "" {
		t.Skipf("golden path host not yet populated (scenario step 7 未实现)")
		return
	}
	if g.Gateway == nil || (g.Gateway.ContainerID == "" && g.Gateway.HostID == "") {
		t.Skipf("golden path gateway not yet populated (scenario step 4..6 未实现)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const probeURL = "https://ifconfig.io"

	baselineExit, err := g.ProbeOutboundFromUser(ctx, probeURL, 5*time.Second)
	if err != nil {
		t.Skipf("baseline probe unavailable (worker container handle): %v", err)
		return
	}
	if baselineExit != 0 {
		t.Skipf("baseline egress not working (exit=%d); 极可能 CI 出口被屏蔽，避免外网抖动 false-fail", baselineExit)
		return
	}

	workerName, err := g.workerDockerName()
	if err != nil {
		t.Skipf("worker container name unavailable: %v", err)
		return
	}
	gatewayName, err := g.gatewayDockerName()
	if err != nil {
		t.Skipf("gateway container name unavailable: %v", err)
		return
	}

	workerIP, err := g.InspectContainerIPv4(ctx, workerName, "")
	if err != nil {
		t.Skipf("worker container ipv4 not available: %v", err)
		return
	}
	gatewayIP := g.Gateway.GatewayIP
	if gatewayIP == "" {
		var ipErr error
		gatewayIP, ipErr = g.InspectContainerIPv4(ctx, gatewayName, "")
		if ipErr != nil {
			t.Skipf("gateway ipv4 not available: %v", ipErr)
			return
		}
	}

	bpf := "src host " + workerIP + " and not dst host " + gatewayIP

	type tcpdumpResult struct {
		packets int
		err     error
	}
	dumpCh := make(chan tcpdumpResult, 1)
	go func() {
		packets, dErr := g.TcpdumpOnHostEth0(ctx, bpf, 5, KillswitchTimingContract.TcpdumpWindow)
		dumpCh <- tcpdumpResult{packets: packets, err: dErr}
	}()

	if err := g.KillGateway(ctx); err != nil {
		t.Fatalf("kill gateway: %v", err)
	}

	probeExit, probeErr := g.ProbeOutboundFromUser(ctx, probeURL, KillswitchTimingContract.ProbeMaxLatency)
	if probeErr != nil {
		t.Fatalf("probe outbound after kill: %v", probeErr)
	}

	var tdRes tcpdumpResult
	select {
	case tdRes = <-dumpCh:
	case <-ctx.Done():
		t.Fatalf("tcpdump goroutine did not finish before ctx deadline: %v", ctx.Err())
	}

	if tdRes.err != nil {
		t.Logf("tcpdump sidecar reported err (可能 host eth0 在 runner 上不可抓包，转 Skip): %v", tdRes.err)
		t.Skipf("host eth0 tcpdump oracle unavailable; deferred-to-CI (hosted ubuntu-24.04 with sudo)")
		return
	}

	verdict := ClassifyKillswitchResult(probeExit, tdRes.packets)
	t.Logf("MVS-09 verdict=%s probeExit=%d leakedPackets=%d worker=%s gateway=%s bpf=%q",
		verdict, probeExit, tdRes.packets, workerIP, gatewayIP, bpf)
	if verdict != KillswitchOK {
		t.Fatalf("MVS-09 kill-switch fail: verdict=%s probeExit=%d packets=%d",
			verdict, probeExit, tdRes.packets)
	}
}
